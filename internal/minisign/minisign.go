// Package minisign 实现 minisign（https://jedisct1.github.io/minisign/）公钥、签名与私钥文件格式，
// 算法为 Ed25519。发布流程用它给 checksums.txt 签名，在线升级用它验证签名；
// 生成的文件可以直接交给 minisign 命令行工具验证。
//
// 公钥：base64(签名算法 "Ed" 2 字节 || 密钥 ID 8 字节 || Ed25519 公钥 32 字节)
//
// 签名文件共四行：
//
//	untrusted comment: <不受签名保护的说明>
//	base64(签名算法 2 字节 || 密钥 ID 8 字节 || 签名 64 字节)
//	trusted comment: <受签名保护的说明>
//	base64(全局签名 64 字节)
//
// 签名算法为 "Ed"（旧格式，直接签文件内容）或 "ED"（预哈希，签文件内容的 BLAKE2b-512）；
// 全局签名是对「签名 64 字节 || 受信说明」的 Ed25519 签名，用来保护受信说明不被篡改。
package minisign

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

const (
	// AlgLegacy 为旧格式签名：直接对文件内容签名
	AlgLegacy = "Ed"
	// AlgPrehashed 为预哈希签名：对文件内容的 BLAKE2b-512 签名，minisign 0.8 起的默认格式
	AlgPrehashed = "ED"

	untrustedPrefix = "untrusted comment: "
	trustedPrefix   = "trusted comment: "

	keyIDLen = 8
)

// KeyID 为密钥 ID，公钥与签名中都带有，用于快速判断签名来自哪把密钥。
type KeyID [keyIDLen]byte

// String 按 minisign 的显示方式输出：把 8 字节按小端序读成整数，以 16 位大写十六进制表示。
func (id KeyID) String() string {
	return fmt.Sprintf("%016X", binary.LittleEndian.Uint64(id[:]))
}

// PublicKey 为 minisign 公钥。
type PublicKey struct {
	ID  KeyID
	Key ed25519.PublicKey
}

// ParsePublicKey 解析公钥：既接受 minisign.pub 文件的完整内容（带 untrusted comment 行），也接受单独的 base64 一行。
func ParsePublicKey(text string) (*PublicKey, error) {
	line := ""
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, untrustedPrefix) {
			continue
		}
		line = l
		break
	}
	raw, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(raw) != 2+keyIDLen+ed25519.PublicKeySize {
		return nil, errors.New("公钥格式无效")
	}
	if string(raw[:2]) != AlgLegacy {
		return nil, errors.New("公钥的签名算法不受支持")
	}
	pk := &PublicKey{Key: ed25519.PublicKey(bytes.Clone(raw[2+keyIDLen:]))}
	copy(pk.ID[:], raw[2:2+keyIDLen])
	return pk, nil
}

// String 返回公钥的 base64 一行，可直接用于 minisign -P。
func (k *PublicKey) String() string {
	raw := make([]byte, 0, 2+keyIDLen+ed25519.PublicKeySize)
	raw = append(raw, AlgLegacy...)
	raw = append(raw, k.ID[:]...)
	raw = append(raw, k.Key...)
	return base64.StdEncoding.EncodeToString(raw)
}

// File 返回与 minisign -G 生成的 minisign.pub 相同格式的文件内容。
func (k *PublicKey) File() []byte {
	return []byte(untrustedPrefix + "minisign public key " + k.ID.String() + "\n" + k.String() + "\n")
}

// Signature 为解析后的签名文件。
type Signature struct {
	// Algorithm 为 AlgLegacy 或 AlgPrehashed
	Algorithm        string
	KeyID            KeyID
	Sig              []byte
	UntrustedComment string
	TrustedComment   string
	GlobalSig        []byte
}

// ParseSignature 解析 .minisig 文件内容。
func ParseSignature(data []byte) (*Signature, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	// 末尾可以有换行，但至少要有四行内容
	if len(lines) < 4 {
		return nil, errors.New("签名文件格式无效：行数不足")
	}
	if !strings.HasPrefix(lines[0], untrustedPrefix) {
		return nil, errors.New("签名文件格式无效：缺少 untrusted comment 行")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(raw) != 2+keyIDLen+ed25519.SignatureSize {
		return nil, errors.New("签名文件格式无效：签名行无法解析")
	}
	if !strings.HasPrefix(lines[2], trustedPrefix) {
		return nil, errors.New("签名文件格式无效：缺少 trusted comment 行")
	}
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != ed25519.SignatureSize {
		return nil, errors.New("签名文件格式无效：全局签名行无法解析")
	}
	for _, l := range lines[4:] {
		if strings.TrimSpace(l) != "" {
			return nil, errors.New("签名文件格式无效：多余的内容")
		}
	}
	s := &Signature{
		Algorithm:        string(raw[:2]),
		Sig:              bytes.Clone(raw[2+keyIDLen:]),
		UntrustedComment: strings.TrimPrefix(lines[0], untrustedPrefix),
		TrustedComment:   strings.TrimPrefix(lines[2], trustedPrefix),
		GlobalSig:        global,
	}
	copy(s.KeyID[:], raw[2:2+keyIDLen])
	if s.Algorithm != AlgLegacy && s.Algorithm != AlgPrehashed {
		return nil, fmt.Errorf("不支持的签名算法 %q", s.Algorithm)
	}
	return s, nil
}

// Encode 输出签名文件内容。
func (s *Signature) Encode() []byte {
	raw := make([]byte, 0, 2+keyIDLen+ed25519.SignatureSize)
	raw = append(raw, s.Algorithm...)
	raw = append(raw, s.KeyID[:]...)
	raw = append(raw, s.Sig...)
	var b bytes.Buffer
	b.WriteString(untrustedPrefix + s.UntrustedComment + "\n")
	b.WriteString(base64.StdEncoding.EncodeToString(raw) + "\n")
	b.WriteString(trustedPrefix + s.TrustedComment + "\n")
	b.WriteString(base64.StdEncoding.EncodeToString(s.GlobalSig) + "\n")
	return b.Bytes()
}

// ErrKeyMismatch 表示签名不是由该公钥对应的私钥生成（密钥 ID 不同）。
var ErrKeyMismatch = errors.New("签名的密钥 ID 与公钥不符")

// signedMessage 返回签名算法实际签署的内容：预哈希为 BLAKE2b-512 摘要，旧格式为原文。
func signedMessage(alg string, msg []byte) []byte {
	if alg == AlgPrehashed {
		h := blake2b.Sum512(msg)
		return h[:]
	}
	return msg
}

// Verify 用公钥验证 msg 的签名，签名与受信说明都通过才返回 nil。
func (k *PublicKey) Verify(msg []byte, s *Signature) error {
	if s.KeyID != k.ID {
		return ErrKeyMismatch
	}
	if !ed25519.Verify(k.Key, signedMessage(s.Algorithm, msg), s.Sig) {
		return errors.New("签名无效，文件内容与签名不符")
	}
	global := make([]byte, 0, len(s.Sig)+len(s.TrustedComment))
	global = append(global, s.Sig...)
	global = append(global, s.TrustedComment...)
	if !ed25519.Verify(k.Key, global, s.GlobalSig) {
		return errors.New("受信说明的签名无效")
	}
	return nil
}
