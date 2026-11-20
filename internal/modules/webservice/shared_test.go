package webservice

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/crud"
)

// totpNow 按 RFC 6238 计算当前验证码（测试独立实现，不依赖被测代码）。
func totpNow(secret string) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(time.Now().Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[off:off+4])&0x7fffffff)%1000000)
}

func TestSharedUsersTOTPAndSSO(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok " + r.Header.Get("Cookie")))
	}))
	defer backend.Close()
	secret := auth.NewTOTPSecret()
	// carol 用于验证分步登录，与 alice 的验证码互不影响（同一验证码只能使用一次）
	secret2 := auth.NewTOTPSecret()
	users, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "pw", TOTPSecret: secret}, {Name: "bot", Password: "botpw"}, {Name: "carol", Password: "pw2", TOTPSecret: secret2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(users[0].Password, "$2") {
		t.Fatal("明文密码应保存为 bcrypt 哈希")
	}
	l := Listener{Sites: []Site{
		{Enable: true, Key: "a", Domains: []string{"a.sso.test"}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true},
		{Enable: true, Key: "b", Domains: []string{"b.sso.test"}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true},
		{Enable: true, Key: "c", Domains: []string{"c.other.test"}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true},
	}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.shared.set(users, []string{"sso.test"})
	noFollow := noRedirect()
	browser := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}

	login := func(host, user, pass, code string) *http.Response {
		form := url.Values{"username": {user}, "password": {pass}, "code": {code}, "next": {"/"}}
		req, _ := http.NewRequest("POST", "http://"+addr+authPath+"/login", strings.NewReader(form.Encode()))
		req.Host = host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	// 登录页不再固定显示验证码输入框
	_, page := get(t, noFollow, addr, "a.sso.test", authPath+"/login", browser)
	if strings.Contains(page, `name="code"`) {
		t.Fatal("登录页不应固定显示验证码输入框")
	}
	postForm := func(host string, form url.Values) (*http.Response, string) {
		req, _ := http.NewRequest("POST", "http://"+addr+authPath+"/login", strings.NewReader(form.Encode()))
		req.Host = host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(b)
	}
	// 密码正确但缺少验证码：弹出验证码对话框并携带待验证凭据
	resp, body := postForm("a.sso.test", url.Values{"username": {"carol"}, "password": {"pw2"}, "next": {"/"}})
	m := regexp.MustCompile(`name="pending" value="([^"]+)"`).FindStringSubmatch(body)
	if resp.StatusCode != 200 || m == nil || !strings.Contains(body, `name="code"`) {
		t.Fatalf("缺少验证码时应进入验证码步骤: %d", resp.StatusCode)
	}
	pending := html.UnescapeString(m[1])
	// 密码错误不进入验证码步骤
	if _, body := postForm("a.sso.test", url.Values{"username": {"carol"}, "password": {"bad"}}); strings.Contains(body, `name="pending"`) {
		t.Fatal("密码错误不应进入验证码步骤")
	}
	// 凭据绑定站点：换到其他站点无效
	if resp, body := postForm("c.other.test", url.Values{"pending": {pending}, "code": {totpNow(secret2)}}); resp.StatusCode != 401 || strings.Contains(body, `name="pending"`) {
		t.Fatalf("待验证凭据不应跨站点使用: %d", resp.StatusCode)
	}
	if resp, body := postForm("a.sso.test", url.Values{"pending": {pending}, "code": {"abc"}}); resp.StatusCode != 401 || !strings.Contains(body, `name="pending"`) {
		t.Fatalf("验证码错误应停留在验证码步骤: %d", resp.StatusCode)
	}
	if resp, _ := postForm("a.sso.test", url.Values{"pending": {pending}, "code": {totpNow(secret2)}, "next": {"/x"}}); resp.StatusCode != 302 || resp.Header.Get("Location") != "/x" {
		t.Fatalf("验证码步骤通过后应登录成功: %d", resp.StatusCode)
	}
	if resp := login("a.sso.test", "carol", "pw", "000000"); resp.StatusCode != 401 && totpNow(secret2) != "000000" {
		t.Fatalf("错误验证码应失败: %d", resp.StatusCode)
	}
	code := totpNow(secret)
	resp = login("a.sso.test", "alice", "pw", code)
	if resp.StatusCode != 302 {
		t.Fatalf("正确的密码与验证码应登录成功: %d", resp.StatusCode)
	}
	var sso *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == ssoCookie {
			sso = c
		}
	}
	if sso == nil || sso.Domain != "sso.test" {
		t.Fatalf("应写出父域的单点登录 Cookie: %+v", resp.Cookies())
	}
	// 同一验证码不能再次使用
	if resp := login("b.sso.test", "alice", "pw", code); resp.StatusCode != 401 {
		t.Fatalf("验证码重放应失败: %d", resp.StatusCode)
	}

	visit := func(host string, c *http.Cookie) (int, string) {
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		req.Host = host
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Accept", "text/html")
		req.AddCookie(c)
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode, string(b)
	}
	// 同域下的另一个站点免登录，且会话 Cookie 不转发给后端
	if code, body := visit("b.sso.test", sso); code != 200 || strings.Contains(body, ssoCookie) {
		t.Fatalf("单点登录应对同域站点生效且不泄露 Cookie: %d %q", code, body)
	}
	// 不在单点登录域下的站点不接受
	if code, _ := visit("c.other.test", sso); code != 302 {
		t.Fatalf("域外站点不应接受单点登录 Cookie: %d", code)
	}
	// 修改 TOTP 后旧会话失效
	users2, _ := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: crud.SecretMask, TOTPSecret: auth.NewTOTPSecret()}, {Name: "bot", Password: crud.SecretMask}}, users)
	if users2[0].Password != users[0].Password {
		t.Fatal("密码占位符应保留原哈希")
	}
	r.auth.shared.set(users2, []string{"sso.test"})
	if code, _ := visit("a.sso.test", sso); code != 302 {
		t.Fatalf("凭据变化后旧会话应失效: %d", code)
	}

	// 基本认证：未启用 TOTP 的共享用户可用，启用了的不可用
	basic := func(user, pass string) int {
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		req.Host = "a.sso.test"
		req.SetBasicAuth(user, pass)
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := basic("bot", "botpw"); c != 200 {
		t.Fatalf("无 TOTP 的共享用户应可用基本认证: %d", c)
	}
	if c := basic("alice", "pw"); c != 401 {
		t.Fatalf("启用 TOTP 的用户不应通过基本认证: %d", c)
	}

	// 校验
	if _, err := normalizeAuthUsers([]AuthUser{{Name: "x", Password: "p", TOTPSecret: "not-base32!"}}, nil); err == nil {
		t.Fatal("无效 TOTP 密钥应校验失败")
	}
	if _, err := normalizeAuthUsers([]AuthUser{{Name: "x"}}, nil); err == nil {
		t.Fatal("新用户缺少密码应校验失败")
	}
	if _, err := validateSSODomain("localhost"); err == nil {
		t.Fatal("单段域名不能作为单点登录域")
	}
}

// ssoList 把测试里的单个单点登录域转成列表，空字符串表示不设置。
func ssoList(d string) []string {
	if d == "" {
		return nil
	}
	return []string{d}
}
