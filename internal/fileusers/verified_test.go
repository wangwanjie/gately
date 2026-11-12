package fileusers

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// BenchmarkCheckPassword 对比 bcrypt 哈希密码的逐次完整验证（改动前每个 WebDAV 请求的开销）与带验证缓存的 CheckPassword。
func BenchmarkCheckPassword(b *testing.B) {
	h, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.DefaultCost)
	if err != nil {
		b.Fatal(err)
	}
	stored := string(h)
	b.Run("bcrypt-every-request", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if bcrypt.CompareHashAndPassword([]byte(stored), []byte("s3cret")) != nil {
				b.Fatal("应通过")
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !CheckPassword(stored, "s3cret") {
				b.Fatal("应通过")
			}
		}
	})
}

// TestCheckPasswordCache 验证缓存不会放过错误密码，修改密码（哈希变化）后旧密码立即失效。
func TestCheckPasswordCache(t *testing.T) {
	h1, _ := bcrypt.GenerateFromPassword([]byte("one"), bcrypt.MinCost)
	stored := string(h1)
	if !CheckPassword(stored, "one") || !CheckPassword(stored, "one") {
		t.Fatal("正确密码应通过（含缓存命中）")
	}
	if CheckPassword(stored, "two") || CheckPassword(stored, "") {
		t.Fatal("错误密码不应通过")
	}
	h2, _ := bcrypt.GenerateFromPassword([]byte("two"), bcrypt.MinCost)
	changed := string(h2)
	if CheckPassword(changed, "one") {
		t.Fatal("修改密码后旧密码应失效")
	}
	if !CheckPassword(changed, "two") {
		t.Fatal("新密码应通过")
	}
	// 同一个密码重新哈希（盐不同）后得到不同的哈希，不沿用旧缓存但仍能通过
	h3, _ := bcrypt.GenerateFromPassword([]byte("one"), bcrypt.MinCost)
	if !CheckPassword(string(h3), "one") {
		t.Fatal("重新哈希后的同一密码应通过")
	}
}

// TestVerifiedCacheBounded 验证缓存条目数有上限。
func TestVerifiedCacheBounded(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	stored := string(h)
	for i := 0; i < verifiedMax+50; i++ {
		verifiedStore(verifiedKey(stored, string(rune('a'+i%26))+string(rune(i))), time.Now())
	}
	verified.mu.Lock()
	n := len(verified.m)
	verified.mu.Unlock()
	if n > verifiedMax {
		t.Fatalf("缓存条目数 %d 超过上限 %d", n, verifiedMax)
	}
}
