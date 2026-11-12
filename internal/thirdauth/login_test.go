package thirdauth

import (
	"strings"
	"testing"
	"time"
)

// 签名令牌：正常往返、篡改、类型不符、过期、格式错误都要覆盖。
func TestSignVerify(t *testing.T) {
	secret := []byte("k1")
	type payload struct{ N string }
	tok := Sign(secret, "state", payload{N: "abc"}, time.Minute)

	var got payload
	if err := Verify(secret, "state", tok, &got); err != nil || got.N != "abc" {
		t.Fatalf("往返失败: %v %+v", err, got)
	}
	exp, err := VerifyExp(secret, "state", tok, &got)
	if err != nil || exp <= time.Now().Unix() {
		t.Fatalf("过期时间异常: %d %v", exp, err)
	}

	if err := Verify([]byte("k2"), "state", tok, &got); err == nil {
		t.Fatal("密钥不同应校验失败")
	}
	if err := Verify(secret, "other", tok, &got); err == nil {
		t.Fatal("类型不符应校验失败")
	}
	// 篡改载荷部分
	p, sig, _ := strings.Cut(tok, ".")
	if err := Verify(secret, "state", "x"+p+"."+sig, &got); err == nil {
		t.Fatal("载荷被篡改应校验失败")
	}
	if err := Verify(secret, "state", "nodot", &got); err == nil {
		t.Fatal("缺少分隔符应报格式错误")
	}
	expired := Sign(secret, "state", payload{}, -time.Second)
	if err := Verify(secret, "state", expired, &got); err == nil {
		t.Fatal("过期令牌应被拒绝")
	}
}

func TestRandomNonce(t *testing.T) {
	a, b := RandomNonce(), RandomNonce()
	if len(a) != 24 || a == b {
		t.Fatalf("随机串异常: %q %q", a, b)
	}
}

func TestLoginConfigHelpers(t *testing.T) {
	c := LoginConfig{
		CallbackBase: " https://g.example.com/ ",
		Allowed:      []string{" a ", "", "  ", "b"},
		Providers:    []Provider{{Key: "gh", ClientSecret: "s"}},
	}
	c.Normalize()
	if c.CallbackBase != "https://g.example.com" || len(c.Allowed) != 2 || c.Allowed[0] != "a" {
		t.Fatalf("Normalize 结果异常: %+v", c)
	}
	if c.CallbackURL() != "https://g.example.com"+CallbackPath {
		t.Fatalf("CallbackURL = %s", c.CallbackURL())
	}
	if c.ProviderMap()["gh"].ClientSecret != "s" || c.Secrets()["gh"] != "s" {
		t.Fatal("ProviderMap/Secrets 异常")
	}
}
