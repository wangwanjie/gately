// Package releasesig 保存 Gately 发布签名的受信公钥，并验证发布中 checksums.txt 的 minisign 签名。
//
// 发布流程（.github/workflows/release.yml）用私钥对 checksums.txt 生成 checksums.txt.minisig；
// checksums.txt 列出全部发布产物的 SHA-256，签名通过后再逐个比对产物的 SHA-256。
// 用户也可以用 minisign 命令行手动验证，见 docs/VERIFY.md。
package releasesig

import (
	"errors"
	"fmt"

	"github.com/wangwanjie/gately/internal/minisign"
)

// 受信公钥（minisign 格式的 base64 一行）。
//
// 轮换密钥时：先把新公钥填入 SecondaryKey 并发布一个版本，让已部署的程序同时信任新旧两把；
// 之后用新私钥签名发布，再在后续版本中把新公钥移到 PrimaryKey、清空或换掉 SecondaryKey。
const (
	// PrimaryKey 为当前的发布签名公钥（密钥 ID 512082B4037581F0），同时公布在 docs/VERIFY.md
	PrimaryKey = "RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/"
	// SecondaryKey 为轮换预留的第二把公钥，空串表示未启用
	SecondaryKey = ""
)

// SignatureFile 为发布中签名文件的名字。
const SignatureFile = "checksums.txt.minisig"

// TrustedKeys 返回编进程序的受信公钥（跳过未启用的空位）。
func TrustedKeys() []string {
	var keys []string
	for _, k := range []string{PrimaryKey, SecondaryKey} {
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// Verify 用 keys 中的任一公钥验证 checksums 的签名，成功时返回签名使用的密钥 ID。
// 只接受预哈希（ED）签名：发布流程只生成这种签名，拒绝旧格式可以避免同一签名被换个算法标识重新解释。
func Verify(keys []string, checksums, sig []byte) (string, error) {
	s, err := minisign.ParseSignature(sig)
	if err != nil {
		return "", err
	}
	if s.Algorithm != minisign.AlgPrehashed {
		return "", errors.New("签名不是预哈希（ED）格式，发布签名只使用该格式")
	}
	for _, text := range keys {
		pk, err := minisign.ParsePublicKey(text)
		if err != nil {
			// 编进程序的公钥由测试保证有效，这里只防御异常
			continue
		}
		if pk.ID != s.KeyID {
			continue
		}
		if err := pk.Verify(checksums, s); err != nil {
			return "", err
		}
		return pk.ID.String(), nil
	}
	return "", fmt.Errorf("签名来自不受信任的密钥 %s", s.KeyID)
}
