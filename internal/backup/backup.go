// Package backup 负责配置备份的打包、加密、解析与保留策略，供手动导出、定时备份与恢复共用。
package backup

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

// 文件名约定
const (
	// NamePrefix 是备份文件名前缀，保留策略只处理带此前缀的文件
	NamePrefix = "gately-backup-"
	// ExtZip 是未加密备份的扩展名
	ExtZip = ".zip"
	// ExtEncrypted 是加密备份的扩展名
	ExtEncrypted = ".zip.enc"
)

// magic 标识加密备份，后接 16 字节盐、12 字节 nonce 与密文。
var magic = []byte("GATELYBK1")

const (
	saltLen  = 16
	nonceLen = 12
	// maxFileSize 限制单个配置文件解压后的大小
	maxFileSize = 32 << 20
	// maxTotalSize 限制全部配置文件解压后的总大小，maxFiles 限制文件数量，防止小体积压缩包解压后耗尽内存
	maxTotalSize = 128 << 20
	maxFiles     = 256
)

// ErrPasswordRequired 表示备份已加密但未提供口令。
var ErrPasswordRequired = errors.New("备份已加密，需要提供备份口令")

// ErrBadPassword 表示口令错误或备份已损坏。
var ErrBadPassword = errors.New("备份口令错误或文件已损坏")

// Name 生成备份文件名。
func Name(now time.Time, encrypted bool) string {
	ext := ExtZip
	if encrypted {
		ext = ExtEncrypted
	}
	return NamePrefix + now.Format("20060102-150405") + ext
}

// IsBackupName 判断文件名是否为本包生成的备份。
func IsBackupName(name string) bool {
	return strings.HasPrefix(name, NamePrefix) && (strings.HasSuffix(name, ExtZip) || strings.HasSuffix(name, ExtEncrypted))
}

// Collect 读取配置目录下全部 *.json。
func Collect(dir string) (map[string][]byte, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", filepath.Base(p), err)
		}
		out[filepath.Base(p)] = data
	}
	return out, nil
}

// Zip 把文件集合打包为 zip，文件按名称排序以保证输出稳定。
func Zip(files map[string][]byte, now time.Time) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Build 打包配置目录。password 非空时加密输出。
func Build(dir, password string, now time.Time) ([]byte, error) {
	files, err := Collect(dir)
	if err != nil {
		return nil, err
	}
	data, err := Zip(files, now)
	if err != nil {
		return nil, err
	}
	if password == "" {
		return data, nil
	}
	return Encrypt(data, password)
}

func deriveKey(password string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(password), salt, 1<<15, 8, 1, 32)
}

// IsEncrypted 判断数据是否为加密备份。
func IsEncrypted(data []byte) bool {
	return bytes.HasPrefix(data, magic)
}

// Encrypt 用口令以 scrypt + AES-256-GCM 加密。
func Encrypt(data []byte, password string) ([]byte, error) {
	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(magic)+saltLen+nonceLen+len(data)+gcm.Overhead())
	out = append(out, magic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, data, magic), nil
}

// Decrypt 解密 Encrypt 的输出。
func Decrypt(data []byte, password string) ([]byte, error) {
	if !IsEncrypted(data) {
		return data, nil
	}
	if password == "" {
		return nil, ErrPasswordRequired
	}
	if len(data) < len(magic)+saltLen+nonceLen {
		return nil, ErrBadPassword
	}
	rest := data[len(magic):]
	salt, rest := rest[:saltLen], rest[saltLen:]
	nonce, sealed := rest[:nonceLen], rest[nonceLen:]
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, sealed, magic)
	if err != nil {
		return nil, ErrBadPassword
	}
	return plain, nil
}

// Parse 解析备份（自动识别是否加密），返回其中的配置文件。
// 只接受位于 zip 根目录、非隐藏的 *.json，且内容必须是合法 JSON。
func Parse(data []byte, password string) (map[string][]byte, error) {
	data, err := Decrypt(data, password)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("不是有效的 zip 文件")
	}
	out := map[string][]byte{}
	total := 0
	for _, f := range zr.File {
		name := path.Base(f.Name)
		if f.FileInfo().IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") || name != f.Name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxFileSize))
		rc.Close()
		if err != nil || !json.Valid(b) {
			return nil, fmt.Errorf("%s 不是有效的 JSON", name)
		}
		if total += len(b); total > maxTotalSize || len(out) >= maxFiles {
			return nil, errors.New("备份解压后过大")
		}
		out[name] = b
	}
	return out, nil
}

// Pick 返回 names 中应被删除的旧备份：只统计备份文件，按文件名（含时间）倒序保留 keep 个。
// keep 小于 1 时不删除任何文件。
func Pick(names []string, keep int) []string {
	if keep < 1 {
		return nil
	}
	var own []string
	for _, n := range names {
		if IsBackupName(n) {
			own = append(own, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(own)))
	if len(own) <= keep {
		return nil
	}
	return own[keep:]
}
