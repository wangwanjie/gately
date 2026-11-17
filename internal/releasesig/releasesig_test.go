package releasesig

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/minisign"
)

// 编进程序的公钥必须都能解析，否则在线升级永远验签失败
func TestTrustedKeysValid(t *testing.T) {
	keys := TrustedKeys()
	if len(keys) == 0 {
		t.Fatal("至少要有一把受信公钥")
	}
	for _, k := range keys {
		if _, err := minisign.ParsePublicKey(k); err != nil {
			t.Fatalf("受信公钥 %q 无效: %v", k, err)
		}
	}
}

func newKey(t *testing.T) (*minisign.PrivateKey, string) {
	t.Helper()
	k, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k, k.Public().String()
}

func TestVerify(t *testing.T) {
	cur, curPub := newKey(t)
	next, nextPub := newKey(t)
	stranger, _ := newKey(t)
	sums := []byte("00ff  gately_v1.0.0_linux_amd64.tar.gz\n")
	sig, _ := cur.Sign(sums, "u", "gately v1.0.0")

	id, err := Verify([]string{curPub}, sums, sig)
	if err != nil || id != cur.ID.String() {
		t.Fatalf("正确的签名应通过: %v %s", err, id)
	}
	// 轮换：第二个位置的公钥同样生效
	nextSig, _ := next.Sign(sums, "u", "t")
	if id, err := Verify([]string{curPub, nextPub}, sums, nextSig); err != nil || id != next.ID.String() {
		t.Fatalf("第二把公钥签名应通过: %v", err)
	}
	if _, err := Verify([]string{curPub}, append(sums, 'x'), sig); err == nil {
		t.Fatal("checksums 被改动时应失败")
	}
	strangerSig, _ := stranger.Sign(sums, "u", "t")
	if _, err := Verify([]string{curPub, nextPub}, sums, strangerSig); err == nil || !strings.Contains(err.Error(), "不受信任") {
		t.Fatalf("未知密钥的签名应报不受信任: %v", err)
	}
	if _, err := Verify([]string{curPub}, sums, []byte("garbage")); err == nil {
		t.Fatal("格式无效的签名应失败")
	}
}

// 发布签名只接受预哈希格式：minisign -l 生成的旧格式签名被拒绝
func TestVerifyRejectsLegacy(t *testing.T) {
	k, pub := newKey(t)
	sums := []byte("x\n")
	sig, _ := k.Sign(sums, "u", "t")
	s, err := minisign.ParseSignature(sig)
	if err != nil {
		t.Fatal(err)
	}
	s.Algorithm = minisign.AlgLegacy
	if _, err := Verify([]string{pub}, sums, s.Encode()); err == nil || !strings.Contains(err.Error(), "ED") {
		t.Fatalf("旧格式签名应被拒绝: %v", err)
	}
}
