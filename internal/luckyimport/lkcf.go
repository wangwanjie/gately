// Package luckyimport 解析 Lucky 的 .lkcf 配置文件，用于迁移到 Gately。
//
// Lucky 3.0 的新格式（以 "LKCF" 开头）见 lkcf_v2.go。
//
// 2.x 文件格式（针对 Lucky 2.27.2 导出的配置）：
//
//	[0]      版本号，固定为 0x01
//	[1:33]   AES-256 密钥
//	[33:49]  CBC 初始向量
//	[49:]    AES-256-CBC 密文，PKCS#7 填充，明文为 JSON（部分版本可能再经 gzip 压缩）
package luckyimport

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	versionV1 = 0x01
	keySize   = 32
	ivSize    = aes.BlockSize
	headerLen = 1 + keySize + ivSize
	// 解压后的明文上限，防止压缩炸弹
	maxPlainSize = 64 << 20
)

// Decrypt 解密 .lkcf 内容并返回 JSON 明文。若输入本身就是 JSON（已导出的明文），原样返回。
func Decrypt(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed, nil
	}
	if isV2(data) {
		plain, err := decryptV2(data)
		if err != nil {
			return nil, err
		}
		if !json.Valid(plain) {
			return nil, errors.New("解密结果不是 JSON，文件可能已损坏")
		}
		return plain, nil
	}
	if len(data) < headerLen+aes.BlockSize {
		return nil, errors.New("文件太短，不是有效的 lkcf")
	}
	if data[0] != versionV1 {
		return nil, fmt.Errorf("不支持的 lkcf 版本 %d", data[0])
	}
	key := data[1 : 1+keySize]
	iv := data[1+keySize : headerLen]
	ct := data[headerLen:]
	if len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("密文长度不是分组大小的整数倍")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ct)
	plain, err = pkcs7Unpad(plain)
	if err != nil {
		return nil, err
	}
	if len(plain) >= 2 && plain[0] == 0x1f && plain[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(plain))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		plain, err = io.ReadAll(io.LimitReader(zr, maxPlainSize))
		if err != nil {
			return nil, err
		}
	}
	if !json.Valid(plain) {
		return nil, errors.New("解密结果不是 JSON，文件可能已损坏")
	}
	return plain, nil
}

func pkcs7Unpad(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("明文为空")
	}
	n := int(b[len(b)-1])
	if n == 0 || n > aes.BlockSize || n > len(b) {
		return nil, errors.New("PKCS#7 填充无效")
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, errors.New("PKCS#7 填充无效")
		}
	}
	return b[:len(b)-n], nil
}

// Document 是解密后的 Lucky 配置，按顶层字段拆分，由各模块自行识别。
type Document map[string]json.RawMessage

// Parse 解密并拆分顶层字段。
func Parse(data []byte) (Document, error) {
	plain, err := Decrypt(data)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := json.Unmarshal(plain, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// ParseAny 解析 .lkcf，或已解密的明文 JSON（便于测试与手工修改后导入）。
func ParseAny(data []byte) (Document, error) {
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '{' {
		var doc Document
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	return Parse(data)
}

// Report 是单个模块的导入结果。
type Report struct {
	Module   string   `json:"module"`
	Title    string   `json:"title"`
	Found    int      `json:"found"`
	Imported int      `json:"imported"`
	Warnings []string `json:"warnings,omitempty"`
}

// Options 控制导入行为。
type Options struct {
	// DryRun 只解析不写入
	DryRun bool
	// KeepEnabled 保留 Lucky 中的启用状态；为 false 时导入的条目一律停用，
	// 避免与仍在运行的 Lucky 争用端口、或在测试机上把线上域名解析到错误地址
	KeepEnabled bool
	// ResolveThirdParty 把 Lucky 第三方登录身份的 Key 解析为 Gately 的允许列表项（提供方key:标识）；
	// all 为真时返回全部启用的身份，missing 为身份表中找不到的 Key 数量。未导入身份表时为 nil
	ResolveThirdParty func(keys []string, all bool) (entries []string, missing int)
	// ResolveStore 把 Lucky 存储管理中的存储 Key 解析为本地目录，供 FTP / WebDAV 的挂载使用；
	// 无法解析时 why 为原因。为 nil 时引用存储的挂载会被跳过
	ResolveStore func(key string) (dir, why string)
	// IPFilter 为本次上传的 Lucky IP 过滤配置中的名单（见 IPFilterLists），供端口转发与 STUN 规则导入安全模式；
	// 未上传 IP 过滤配置时为 nil
	IPFilter map[string]IPFilterList
}

// Importer 由支持从 Lucky 迁移的模块实现。
type Importer interface {
	// ImportLucky 识别 doc 中属于本模块的字段并导入。不包含本模块数据时返回 nil 报告。
	// 已存在同名条目时跳过，重复导入不会产生重复数据。
	ImportLucky(doc Document, opts Options) (*Report, error)
}

// Skipped 生成"已存在同名条目"的提示。
func Skipped(name string) string {
	return name + ": 已存在同名条目，跳过"
}
