package webservice

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// userPass 拆出站点用户条目中的密码部分。
func userPass(t *testing.T, users []string, name string) string {
	t.Helper()
	for _, u := range users {
		if n, p, _ := strings.Cut(u, ":"); n == name {
			return p
		}
	}
	t.Fatalf("没有用户 %s: %v", name, users)
	return ""
}

// 保存时站点用户的明文密码转为 bcrypt 哈希，已是有效哈希的保持不变；无效哈希、未填密码、过长的密码拒绝。
func TestSiteUsersHashedOnValidate(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("kept"), bcrypt.MinCost)
	s := Site{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "x", BasicAuth: true,
		Users: []string{"alice:pass1234", "bob:" + string(hash), "carol:pa:ss"}}
	l := Listener{Port: 18090, Sites: []Site{s}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	users := l.Sites[0].Users
	if p := userPass(t, users, "alice"); !strings.HasPrefix(p, "$2") || bcrypt.CompareHashAndPassword([]byte(p), []byte("pass1234")) != nil {
		t.Fatalf("明文密码应转为 bcrypt 哈希: %q", p)
	}
	if p := userPass(t, users, "bob"); p != string(hash) {
		t.Fatalf("已有哈希应保持不变: %q", p)
	}
	// 密码中的冒号属于密码
	if !verifyUser(users, "carol", "pa:ss") || !verifyUser(users, "alice", "pass1234") || verifyUser(users, "alice", "wrong") {
		t.Fatal("哈希后的用户应能按原密码验证")
	}
	// 再次校验不会重复哈希（会话凭据指纹保持不变）
	before := strings.Join(l.Sites[0].Users, "\n")
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if after := strings.Join(l.Sites[0].Users, "\n"); after != before {
		t.Fatal("已哈希的密码再次校验时不应改变")
	}
	for _, bad := range []string{"dave:$2a$10$broken", "erin:", "frank:" + crud.SecretMask, "gina:" + strings.Repeat("x", 73), ":nopass"} {
		l := Listener{Port: 18090, Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "x", BasicAuth: true, Users: []string{bad}}}}
		if err := l.Validate(); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}

// 存量配置中的明文密码在下次保存前照常可用。
func TestSiteUsersLegacyPlaintext(t *testing.T) {
	if !verifyUser([]string{"alice:secret"}, "alice", "secret") || verifyUser([]string{"alice:secret"}, "alice", "x") {
		t.Fatal("明文密码应仍可比较")
	}
}

// siteUsersAPI 返回带一个监听（含明文站点用户的存量配置）的模块及其接口调用函数。
func siteUsersAPI(t *testing.T) (*Module, func(method, path string, body any) (int, []byte)) {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	// 存量配置：保存前写入的明文，不经过校验
	m.cfg.Listeners = []Listener{{Key: "l1", ListenIP: "127.0.0.1", Port: 18091, Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "a", BasicAuth: true, Users: []string{"alice:legacy1", "bob:legacy2"}},
		{Key: "b", Enable: true, Domains: []string{"b.test"}, Type: TypeText, Text: "b"},
	}, Default: Site{Enable: true, Type: TypeText, Text: "d", BasicAuth: true, Users: []string{"root:rootpw"}}}}
	mux := http.NewServeMux()
	m.Routes(prefixRouter{mux, "/api/webservice"})
	return m, func(method, path string, body any) (int, []byte) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, rd)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}
}

// 接口只返回掩码；原样提交掩码或留空表示不修改，修改时写新密码，保存的都是哈希；复制的站点沿用原密码。
func TestSiteUsersAPIMaskAndKeep(t *testing.T) {
	m, do := siteUsersAPI(t)
	code, body := do("GET", "/api/webservice/listeners", nil)
	if code != 200 || strings.Contains(string(body), "legacy1") || strings.Contains(string(body), "rootpw") {
		t.Fatalf("列表不应返回站点用户密码: %d %s", code, body)
	}
	var list []Listener
	if err := json.Unmarshal(body, &list); err != nil || len(list) != 1 {
		t.Fatalf("列表格式不对: %v %s", err, body)
	}
	l := list[0]
	if got := l.Sites[0].Users; len(got) != 2 || got[0] != "alice:"+crud.SecretMask || got[1] != "bob:"+crud.SecretMask {
		t.Fatalf("站点用户应显示为掩码: %v", got)
	}
	if got := l.Default.Users; len(got) != 1 || got[0] != "root:"+crud.SecretMask {
		t.Fatalf("默认站点用户应显示为掩码: %v", got)
	}
	// 原样提交：alice 保持掩码，bob 留空，root 保持掩码；新增 carol；同时在 b 站点之后复制出一个不带 key 的站点
	l.Sites[0].Users = []string{"alice:" + crud.SecretMask, "bob:", "carol:newpw"}
	dup := l.Sites[0]
	dup.Key = ""
	dup.Domains = []string{"dup.test"}
	dup.Users = []string{"alice:" + crud.SecretMask}
	l.Sites = append(l.Sites, dup)
	if code, body := do("PUT", "/api/webservice/listeners/l1", l); code != 200 {
		t.Fatalf("保存失败: %d %s", code, body)
	}
	saved := m.cfg.Listeners[0]
	for name, pw := range map[string]string{"alice": "legacy1", "bob": "legacy2", "carol": "newpw"} {
		p := userPass(t, saved.Sites[0].Users, name)
		if !strings.HasPrefix(p, "$2") || bcrypt.CompareHashAndPassword([]byte(p), []byte(pw)) != nil {
			t.Fatalf("%s 应以哈希保存原密码: %q", name, p)
		}
	}
	if p := userPass(t, saved.Default.Users, "root"); bcrypt.CompareHashAndPassword([]byte(p), []byte("rootpw")) != nil {
		t.Fatalf("默认站点用户应沿用原密码: %q", p)
	}
	if p := userPass(t, saved.Sites[2].Users, "alice"); bcrypt.CompareHashAndPassword([]byte(p), []byte("legacy1")) != nil {
		t.Fatalf("复制的站点应沿用原密码: %q", p)
	}
	// 再次原样提交（全部为掩码）：哈希不变
	hashes := strings.Join(saved.Sites[0].Users, "\n")
	_, body = do("GET", "/api/webservice/listeners", nil)
	var all []Listener
	_ = json.Unmarshal(body, &all)
	again := all[0]
	if strings.Contains(string(body), "$2") {
		t.Fatalf("接口不应返回密码哈希: %s", body)
	}
	if code, body := do("PUT", "/api/webservice/listeners/l1", again); code != 200 {
		t.Fatalf("保存失败: %d %s", code, body)
	}
	if got := strings.Join(m.cfg.Listeners[0].Sites[0].Users, "\n"); got != hashes {
		t.Fatal("原样提交掩码时哈希不应改变")
	}
	// 修改密码
	again.Sites[0].Users = []string{"alice:changed", "bob:" + crud.SecretMask, "carol:"}
	if code, body := do("PUT", "/api/webservice/listeners/l1", again); code != 200 {
		t.Fatalf("保存失败: %d %s", code, body)
	}
	if !verifyUser(m.cfg.Listeners[0].Sites[0].Users, "alice", "changed") || verifyUser(m.cfg.Listeners[0].Sites[0].Users, "alice", "legacy1") {
		t.Fatal("修改后应使用新密码")
	}
	// 新用户不能留空或只写掩码
	for _, u := range []string{"dave:", "dave:" + crud.SecretMask} {
		again.Sites[0].Users = []string{u}
		if code, _ := do("PUT", "/api/webservice/listeners/l1", again); code != 400 {
			t.Fatalf("新用户 %q 没有密码时应拒绝: %d", u, code)
		}
	}
	// 复制监听：副本中的掩码沿用模板的密码
	_, body = do("GET", "/api/webservice/listeners", nil)
	_ = json.Unmarshal(body, &all)
	tmpl := all[0]
	tmpl.Key, tmpl.Port, tmpl.Name = "", 18092, "copy"
	for i := range tmpl.Sites {
		tmpl.Sites[i].Domains = []string{"copy" + tmpl.Sites[i].Domains[0]}
	}
	if code, body := do("POST", "/api/webservice/listeners/l1/copy", tmpl); code != 200 {
		t.Fatalf("复制失败: %d %s", code, body)
	}
	cp := m.cfg.Listeners[1]
	if !verifyUser(cp.Sites[0].Users, "alice", "changed") || !verifyUser(cp.Default.Users, "root", "rootpw") {
		t.Fatalf("复制的监听应沿用模板的密码: %v %v", cp.Sites[0].Users, cp.Default.Users)
	}
}
