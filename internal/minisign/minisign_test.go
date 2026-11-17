package minisign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// 以下向量由 minisign 0.12 命令行生成：minisign -G -W 生成无口令密钥，
// minisign -S 生成预哈希签名，minisign -S -l 生成旧格式签名，消息为 "hello gately\n"。
const (
	cliMsg = "hello gately\n"
	cliPub = `untrusted comment: minisign public key F083B5950D948376
RWR2g5QNlbWD8OPJUNhIpyE34C4Zx2UoNLT8WnqSyrfhurpInl89vziY
`
	cliKey = `untrusted comment: minisign encrypted secret key
RWQAAEIyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAdoOUDZW1g/DuXHplupucGi8kVQKRy9N890MZCerf6bV284iDP+hWXuPJUNhIpyE34C4Zx2UoNLT8WnqSyrfhurpInl89vziYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
`
	cliSigED = `untrusted comment: signature from minisign secret key
RUR2g5QNlbWD8NVfUeJdHtoBYZNZ0YzV+lQOaQm3u+DrtMzVL9KsQGfu52gbTgq60VmVjZmX9KC9Own+mEdnIR6tJDw56cg9gw8=
trusted comment: trusted ED
JRM2zNYDgLQXPOxNuyOv0bupgZQ942rsz1FkY8oxJ4zG+W96lHzN3raefm7q3uhwdP0RewW3MlDXg/o4MhN2DA==
`
	cliSigLegacy = `untrusted comment: signature from minisign secret key
RWR2g5QNlbWD8GXf2cVqhfXl4lXJZA4nqbc5Dtcgc0WQFbhX19XMzrtJuL8tpNbRlpNfcQ0HWdlGKc7ySLpAbJVgeYs4zMStzgk=
trusted comment: trusted legacy
mpW8cTbX7As6OPWSEbYZzAwvx2YR9uL5i+RtWFad0BZaLF/s1DjKC0OA+MoBMNa6j3UL+1PxZcp+fy0sm3VdBA==
`
)

// testKDF 为测试用的轻量口令哈希参数（换算为 scrypt N=2^11），minisign 命令行同样能读取
var testKDF = KDFParams{OpsLimit: 1 << 16, MemLimit: 1 << 24}

func mustVerify(t *testing.T, pub, msg, sig string) *Signature {
	t.Helper()
	pk, err := ParsePublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseSignature([]byte(sig))
	if err != nil {
		t.Fatal(err)
	}
	if err := pk.Verify([]byte(msg), s); err != nil {
		t.Fatalf("验证失败: %v", err)
	}
	return s
}

// minisign 命令行生成的两种签名都能通过验证，受信说明与密钥 ID 解析正确
func TestVerifyMinisignCLIVectors(t *testing.T) {
	s := mustVerify(t, cliPub, cliMsg, cliSigED)
	if s.Algorithm != AlgPrehashed || s.TrustedComment != "trusted ED" || s.KeyID.String() != "F083B5950D948376" {
		t.Fatalf("预哈希签名解析错误: %+v", s)
	}
	s = mustVerify(t, cliPub, cliMsg, cliSigLegacy)
	if s.Algorithm != AlgLegacy || s.TrustedComment != "trusted legacy" {
		t.Fatalf("旧格式签名解析错误: %+v", s)
	}
	// 只给 base64 一行也能解析公钥
	pk, err := ParsePublicKey(strings.Split(cliPub, "\n")[1])
	if err != nil || string(pk.File()) != cliPub {
		t.Fatalf("公钥往返不一致: %v\n%s", err, pk.File())
	}
}

// 用 minisign 命令行生成的私钥签名，结果与命令行签名逐字节相同（Ed25519 签名是确定性的）
func TestSignWithMinisignCLIKey(t *testing.T) {
	k, err := ParseSecretKey([]byte(cliKey), "")
	if err != nil {
		t.Fatal(err)
	}
	sig, err := k.Sign([]byte(cliMsg), "signature from minisign secret key", "trusted ED")
	if err != nil {
		t.Fatal(err)
	}
	if string(sig) != cliSigED {
		t.Fatalf("签名应与 minisign 命令行输出相同:\n%s\n%s", sig, cliSigED)
	}
	if string(k.Public().File()) != cliPub {
		t.Fatal("由私钥得到的公钥应与命令行生成的公钥相同")
	}
}

// 按 minisign 规范独立解析本包生成的签名与公钥，不经过本包的解析代码
func TestSignatureMatchesSpec(t *testing.T) {
	k, err := GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("0123abcd  gately_v1.0.0_linux_amd64.tar.gz\n")
	sig, err := k.Sign(msg, "untrusted text", "gately v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(sig), "\n")
	if len(lines) != 5 || lines[4] != "" {
		t.Fatalf("签名文件应为四行并以换行结尾: %q", sig)
	}
	if lines[0] != "untrusted comment: untrusted text" || lines[2] != "trusted comment: gately v1.0.0" {
		t.Fatalf("说明行格式错误: %q", sig)
	}
	raw, err := base64.StdEncoding.DecodeString(lines[1])
	if err != nil || len(raw) != 74 {
		t.Fatalf("签名行应为 74 字节: %d %v", len(raw), err)
	}
	if string(raw[:2]) != "ED" || !bytes.Equal(raw[2:10], k.ID[:]) {
		t.Fatalf("签名算法应为 ED、密钥 ID 应一致: %x", raw[:10])
	}
	pubRaw, err := base64.StdEncoding.DecodeString(strings.Split(string(k.Public().File()), "\n")[1])
	if err != nil || len(pubRaw) != 42 || string(pubRaw[:2]) != "Ed" || !bytes.Equal(pubRaw[2:10], k.ID[:]) {
		t.Fatalf("公钥格式错误: %x %v", pubRaw, err)
	}
	pub := ed25519.PublicKey(pubRaw[10:])
	h := blake2b.Sum512(msg)
	if !ed25519.Verify(pub, h[:], raw[10:]) {
		t.Fatal("签名应是对 BLAKE2b-512 摘要的 Ed25519 签名")
	}
	global, _ := base64.StdEncoding.DecodeString(lines[3])
	if !ed25519.Verify(pub, append(bytes.Clone(raw[10:]), "gately v1.0.0"...), global) {
		t.Fatal("全局签名应覆盖签名与受信说明")
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	k, _ := GenerateKey(rand.Reader)
	pub := k.Public()
	msg := []byte("abc  file\n")
	sigData, _ := k.Sign(msg, "u", "t")
	s, err := ParseSignature(sigData)
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.Verify([]byte("abd  file\n"), s); err == nil {
		t.Fatal("内容被改动时应验证失败")
	}
	s2 := *s
	s2.TrustedComment = "t2"
	if err := pub.Verify(msg, &s2); err == nil {
		t.Fatal("受信说明被改动时应验证失败")
	}
	// 把预哈希签名改标为旧格式不能通过验证
	s3 := *s
	s3.Algorithm = AlgLegacy
	if err := pub.Verify(msg, &s3); err == nil {
		t.Fatal("改动签名算法标识后应验证失败")
	}
	other, _ := GenerateKey(rand.Reader)
	if err := other.Public().Verify(msg, s); err != ErrKeyMismatch {
		t.Fatalf("密钥 ID 不同应返回 ErrKeyMismatch，实际 %v", err)
	}
	// 密钥 ID 相同但公钥不同（伪造密钥 ID）也不能通过
	forged := other.Public()
	forged.ID = pub.ID
	if err := forged.Verify(msg, s); err == nil {
		t.Fatal("伪造密钥 ID 的公钥不应验证通过")
	}
}

func TestParseSignatureErrors(t *testing.T) {
	lines := strings.Split(cliSigED, "\n")
	cases := map[string]string{
		"空文件":       "",
		"缺少全局签名":    strings.Join(lines[:3], "\n"),
		"缺少不受信说明前缀": "x\n" + strings.Join(lines[1:], "\n"),
		"缺少受信说明前缀":  strings.Join([]string{lines[0], lines[1], "comment", lines[3]}, "\n"),
		"签名行无效":     strings.Join([]string{lines[0], "!!!", lines[2], lines[3]}, "\n"),
		"多余内容":      cliSigED + "extra\n",
		"未知算法":      strings.Replace(cliSigED, "RUR2", "RXR2", 1),
	}
	for name, data := range cases {
		if _, err := ParseSignature([]byte(data)); err == nil {
			t.Errorf("%s: 应解析失败", name)
		}
	}
	if _, err := ParsePublicKey("untrusted comment: x\nnot-base64"); err == nil {
		t.Error("无效公钥应解析失败")
	}
}

func TestSecretKeyRoundTrip(t *testing.T) {
	k, _ := GenerateKey(rand.Reader)
	enc, err := EncodeSecretKey(k, "s3cret", testKDF, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(enc), "untrusted comment: minisign encrypted secret key\n") {
		t.Fatalf("加密私钥的说明行错误: %q", enc)
	}
	got, err := ParseSecretKey(enc, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != k.ID || !bytes.Equal(got.Key, k.Key) {
		t.Fatal("解密后的私钥应与原私钥相同")
	}
	if _, err := ParseSecretKey(enc, "wrong"); err == nil {
		t.Fatal("口令错误时应失败")
	}
	if _, err := ParseSecretKey(enc, ""); err == nil {
		t.Fatal("加密私钥不给口令时应失败")
	}
	// 无口令私钥原样保存
	plain, err := EncodeSecretKey(k, "", testKDF, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ParseSecretKey(plain, ""); err != nil || !bytes.Equal(got.Key, k.Key) {
		t.Fatalf("无口令私钥往返失败: %v", err)
	}
}

// 换算结果与 libsodium 一致：minisign 默认参数为 N=2^20、r=8、p=1
func TestPickScryptParams(t *testing.T) {
	cases := []struct {
		ops, mem uint64
		logN     uint
		r, p     int
	}{
		{DefaultKDF.OpsLimit, DefaultKDF.MemLimit, 20, 8, 1},
		{testKDF.OpsLimit, testKDF.MemLimit, 11, 8, 1},
		// opslimit 远大于内存允许的 N 时，用 p 吸收多余的计算量
		{1 << 30, 1 << 24, 14, 8, 2048},
	}
	for _, c := range cases {
		logN, r, p := pickScryptParams(c.ops, c.mem)
		if logN != c.logN || r != c.r || p != c.p {
			t.Errorf("pickScryptParams(%d, %d) = %d %d %d，期望 %d %d %d", c.ops, c.mem, logN, r, p, c.logN, c.r, c.p)
		}
	}
}
