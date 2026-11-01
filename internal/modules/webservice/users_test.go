package webservice

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestUsersAPI 验证「认证用户」接口：只改共享用户相关设置、批量切换站点的共享用户开关，任一站点失败时整体不保存。
func TestUsersAPI(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	l := Listener{Key: "l1", Name: "main", ListenIP: "127.0.0.1", Port: 18080, Sites: []Site{
		{Key: "open", Enable: true, Domains: []string{"open.test"}, Type: TypeText, Text: "ok"},
		{Key: "local", Enable: true, Domains: []string{"local.test"}, Type: TypeText, Text: "ok", BasicAuth: true, Users: []string{"bob:pw"}},
		{Key: "only", Enable: true, Domains: []string{"only.test"}, Type: TypeText, Text: "ok", WebAuth: true, SharedUsers: true},
		{Key: "fwd", Enable: true, Domains: []string{"fwd.test"}, Type: TypeText, Text: "ok", ForwardAuth: ForwardAuth{URL: "http://auth.test/verify"}},
		{Key: "drop", Enable: true, Domains: []string{"drop.test"}, Type: TypeClose},
	}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Listeners = []Listener{l}
	m.cfg.Settings.TrustedProxies = []string{"10.0.0.1"}

	put := func(req UsersRequest) (int, UsersView, string) {
		raw, _ := json.Marshal(req)
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/users", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		m.handlePutUsers(rec, r)
		var out UsersView
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out, rec.Body.String()
	}
	site := func(key string) Site {
		for _, s := range m.cfg.Listeners[0].Sites {
			if s.Key == key {
				return s
			}
		}
		t.Fatalf("站点 %s 不存在", key)
		return Site{}
	}

	users := UsersSettings{AuthUsers: []AuthUser{{Name: "alice", Password: "alice-pw"}}, SelfServicePassword: true}
	code, out, body := put(UsersRequest{UsersSettings: users, Sites: []SiteSharedUsers{
		{Listener: "l1", Site: "open", SharedUsers: true},
		{Listener: "l1", Site: "local", SharedUsers: true},
	}})
	if code != 200 {
		t.Fatalf("保存失败: %d %s", code, body)
	}
	if s := site("open"); !s.SharedUsers || !s.WebAuth || s.BasicAuth {
		t.Fatalf("未开启认证的站点应同时开启网页认证: %+v", s)
	}
	if s := site("local"); !s.SharedUsers || !s.BasicAuth || s.WebAuth {
		t.Fatalf("已开启基本认证的站点只应加上共享用户: %+v", s)
	}
	if len(m.cfg.Settings.TrustedProxies) != 1 || !m.cfg.Settings.SelfServicePassword || len(m.cfg.Settings.AuthUsers) != 1 {
		t.Fatalf("应只修改共享用户相关设置: %+v", m.cfg.Settings)
	}
	if out.AuthUsers[0].Password != crud.SecretMask || out.AuthUsers[0].ID == "" {
		t.Fatalf("返回的共享用户应已脱敏并带 ID: %+v", out.AuthUsers[0])
	}
	// 断开连接的站点与停用的默认站点不列出
	for _, s := range out.Sites {
		if s.Site == "drop" || s.Default {
			t.Fatalf("不应列出站点 %s", s.Site)
		}
	}

	// 原样提交返回的用户：密码保持不变
	hash := m.cfg.Settings.AuthUsers[0].Password
	if code, _, body := put(UsersRequest{UsersSettings: out.UsersSettings}); code != 200 || m.cfg.Settings.AuthUsers[0].Password != hash {
		t.Fatalf("原样提交不应修改密码: %d %s", code, body)
	}

	// 任一站点无法修改时整体不保存：只靠共享用户认证的站点不能直接取消，转发认证站点不能启用
	for _, bad := range [][]SiteSharedUsers{
		{{Listener: "l1", Site: "local", SharedUsers: false}, {Listener: "l1", Site: "only", SharedUsers: false}},
		{{Listener: "l1", Site: "local", SharedUsers: false}, {Listener: "l1", Site: "fwd", SharedUsers: true}},
		{{Listener: "l1", Site: "missing", SharedUsers: true}},
		{{Listener: "l9", Site: "open", SharedUsers: true}},
	} {
		next := out.UsersSettings
		next.SelfServicePassword = false
		if code, _, _ := put(UsersRequest{UsersSettings: next, Sites: bad}); code != 400 {
			t.Fatalf("应拒绝 %+v，实际 %d", bad, code)
		}
		if !site("local").SharedUsers || !m.cfg.Settings.SelfServicePassword {
			t.Fatalf("失败时不应保存任何修改 %+v", bad)
		}
	}

	// 有本地用户的站点可以取消共享用户
	if code, _, body := put(UsersRequest{UsersSettings: out.UsersSettings, Sites: []SiteSharedUsers{{Listener: "l1", Site: "local", SharedUsers: false}}}); code != 200 || site("local").SharedUsers {
		t.Fatalf("取消共享用户失败: %d %s", code, body)
	}

	// 只靠共享用户认证的站点确认关闭认证后可以取消，网页认证一并关闭
	if code, _, body := put(UsersRequest{UsersSettings: out.UsersSettings, Sites: []SiteSharedUsers{{Listener: "l1", Site: "only", SharedUsers: false, DisableAuth: true}}}); code != 200 {
		t.Fatalf("确认关闭认证后应可取消: %d %s", code, body)
	}
	if s := site("only"); s.SharedUsers || s.WebAuth || s.BasicAuth {
		t.Fatalf("取消后站点应关闭认证: %+v", s)
	}
}

// TestPutSettingsKeepsUsers 验证全局设置省略共享用户、单点登录域、自助改密与 OIDC 时保留原值，提供时照常修改。
func TestPutSettingsKeepsUsers(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	users, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "alice-pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.cfg.Settings.AuthUsers, m.cfg.Settings.SSODomain, m.cfg.Settings.SelfServicePassword = users, "example.com", true
	put := func(body string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/settings", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		m.handlePutSettings(rec, r)
		return rec.Code
	}
	if code := put(`{"enable":true,"autoBanSeconds":60,"trustedProxies":["10.0.0.1"]}`); code != 200 {
		t.Fatalf("保存失败: %d", code)
	}
	s := m.cfg.Settings
	if len(s.AuthUsers) != 1 || s.AuthUsers[0].Password != users[0].Password || s.SSODomain != "example.com" || !s.SelfServicePassword {
		t.Fatalf("省略的认证用户设置应保留: %+v", s)
	}
	if s.AutoBanSeconds != 60 || len(s.TrustedProxies) != 1 {
		t.Fatalf("其他设置应照常保存: %+v", s)
	}
	if _, ok := m.auth.shared.get().checkPassword("alice", "alice-pw"); !ok {
		t.Fatal("保留的共享用户应继续生效")
	}
	if code := put(`{"enable":true,"authUsers":[],"ssoDomain":"","selfServicePassword":false}`); code != 200 {
		t.Fatalf("保存失败: %d", code)
	}
	if s := m.cfg.Settings; len(s.AuthUsers) != 0 || s.SSODomain != "" || s.SelfServicePassword {
		t.Fatalf("显式提供的字段应照常修改: %+v", s)
	}
}
