package webservice

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// basicSite 构造一个启用基本认证、用户密码为 bcrypt 哈希的站点。
func basicSite(t testing.TB) Site {
	h, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	return Site{Key: "s1", Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok", StatusCode: 200,
		BasicAuth: true, Users: []string{"bob:" + string(h)}, scope: "l1/s1"}
}

func basicRequest(user, pass string) *http.Request {
	r := httptest.NewRequest("GET", "http://a.test/", nil)
	r.RemoteAddr = "203.0.113.5:4000"
	r.SetBasicAuth(user, pass)
	return r
}

// BenchmarkBasicAuthBcrypt 衡量带正确基本认证凭据的请求每次的认证开销：浏览器与 WebDAV 等客户端每个请求都带凭据。
func BenchmarkBasicAuthBcrypt(b *testing.B) {
	a := newAuthenticator("secret")
	s := basicSite(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, ok := a.checkUser(httptest.NewRecorder(), basicRequest("bob", "s3cret"), s)
		if !ok || len(v.names) != 1 || v.names[0] != "bob" {
			b.Fatal("认证应通过")
		}
	}
}

// TestBasicAuthCache 验证基本认证验证结果缓存的边界：错误密码、改密码、删用户、别的站点都不能沿用缓存。
func TestBasicAuthCache(t *testing.T) {
	a := newAuthenticator("secret")
	s := basicSite(t)
	check := func(s Site, user, pass string) bool {
		_, ok := a.checkUser(httptest.NewRecorder(), basicRequest(user, pass), s)
		return ok
	}
	if !check(s, "bob", "s3cret") {
		t.Fatal("正确密码应通过")
	}
	// 第二次来自缓存，结果必须一致
	if !check(s, "bob", "s3cret") {
		t.Fatal("缓存命中后应通过")
	}
	if check(s, "bob", "wrong") {
		t.Fatal("错误密码不应通过")
	}
	if check(s, "alice", "s3cret") {
		t.Fatal("其它用户名不应通过")
	}
	// 同一凭据在另一个站点（作用域不同）不能沿用本站点的缓存，也不能在对方用户列表没有该用户时通过
	other := s
	other.scope = "l1/s2"
	other.Users = []string{"carol:plain"}
	if check(other, "bob", "s3cret") {
		t.Fatal("别的站点不应沿用缓存")
	}
	// 修改密码后旧密码立即失效
	changed := s
	h, _ := bcrypt.GenerateFromPassword([]byte("new-pass"), bcrypt.MinCost)
	changed.Users = []string{"bob:" + string(h)}
	if check(changed, "bob", "s3cret") {
		t.Fatal("修改密码后旧密码应失效")
	}
	if !check(changed, "bob", "new-pass") {
		t.Fatal("新密码应通过")
	}
	// 删除用户后凭据失效
	gone := s
	gone.Users = []string{"carol:plain"}
	if check(gone, "bob", "s3cret") {
		t.Fatal("删除用户后应失效")
	}
}
