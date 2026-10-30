package apitoken

import (
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
)

func newManager(t *testing.T) (*Manager, *config.Store) {
	t.Helper()
	store, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := Open(store, []string{"ddns", "status", "logs"})
	if err != nil {
		t.Fatal(err)
	}
	return m, store
}

func TestRequiredAndMatch(t *testing.T) {
	cases := []struct {
		method, path, need string
	}{
		{"GET", "/api/ddns/tasks", "ddns:read"},
		{"POST", "/api/ddns/tasks", "ddns:write"},
		{"GET", "/api/ddns/logs", "ddns:read"},
		{"DELETE", "/api/ddns/logs", "ddns:write"},
		{"GET", "/api/status", "status:read"},
		{"GET", "/api/logs", "logs:read"},
		{"POST", "/api/import/lucky", "import:write"},
		{"GET", "/api/session", Any},
		{"GET", "/api/openapi.json", Any},
		// 会返回敏感内容或建立交互通道的 GET 按写权限处理
		{"GET", "/api/ssl/certs/abc/download", "ssl:write"},
		{"GET", "/api/webterminal/connections/abc/ws", "webterminal:write"},
		{"GET", "/api/docker/containers/abc/files/read", "docker:write"},
		{"GET", "/api/docker/containers/abc/logs", "docker:read"},
		// 目录浏览会暴露宿主机、容器与远程主机的文件结构，按写权限处理
		{"GET", "/api/docker/containers/abc/files/list", "docker:write"},
		{"GET", "/api/storage/files/k/list", "storage:write"},
		{"GET", "/api/webterminal/sftp/k/list", "webterminal:write"},
		// 容器详情只需读权限（敏感字段在接口内按权限脱敏），明文接口需要写权限
		{"GET", "/api/docker/containers/abc", "docker:read"},
		{"GET", "/api/docker/containers/abc/secrets", "docker:write"},
		// 查看密钥明文需要写权限
		{"GET", "/api/frp/clients/abc/secrets", "frp:write"},
		{"GET", "/api/tailscale/secrets", "tailscale:write"},
		// 文件服务共享用户由三个服务共用：读取沿用所在服务的读权限，修改需要三个服务的写权限
		{"GET", "/api/ftp/shared-users", "ftp:read"},
		{"PUT", "/api/webdav/shared-users", "webdav:write,ftp:write,smb:write"},
		{"PUT", "/api/smb/shared-users", "webdav:write,ftp:write,smb:write"},
		{"GET", "/api/fileusers", "fileusers:read"},
		{"PUT", "/api/fileusers", "fileusers:write"},
		// 账号与系统级接口永远拒绝令牌
		{"GET", "/api/tokens", Denied},
		{"POST", "/api/tokens/abc/regenerate", Denied},
		{"PUT", "/api/account", Denied},
		{"GET", "/api/system/backup", Denied},
		{"POST", "/api/system/restore", Denied},
		{"POST", "/api/system/restart", Denied},
		{"POST", "/api/system/upgrade", Denied},
		{"POST", "/api/system/upgrade/upload", Denied},
		{"PUT", "/api/system/settings", Denied},
		{"POST", "/api/webterminal/security/2fa/disable", Denied},
		{"GET", "/index.html", Denied},
	}
	for _, c := range cases {
		if got := Required(c.method, c.path); got != c.need {
			t.Errorf("%s %s: 期望 %q，实际 %q", c.method, c.path, c.need, got)
		}
	}

	match := []struct {
		scopes []string
		need   string
		want   bool
	}{
		{[]string{"ddns:read"}, "ddns:read", true},
		{[]string{"ddns:read"}, "ddns:write", false},
		{[]string{"ddns:write"}, "ddns:read", true},
		{[]string{"ddns:write"}, "stun:read", false},
		{[]string{"*:read"}, "stun:read", true},
		{[]string{"*:read"}, "stun:write", false},
		{[]string{"*"}, "stun:write", true},
		{[]string{"*:write"}, "stun:read", true},
		{[]string{"ddns:read"}, Any, true},
		{[]string{"*"}, Denied, false},
		{nil, "ddns:read", false},
		{[]string{"webdav:write"}, "webdav:write,ftp:write,smb:write", false},
		{[]string{"webdav:write", "ftp:write", "smb:read"}, "webdav:write,ftp:write,smb:write", false},
		{[]string{"webdav:write", "ftp:write", "smb:write"}, "webdav:write,ftp:write,smb:write", true},
		{[]string{"*:write"}, "webdav:write,ftp:write,smb:write", true},
	}
	for _, c := range match {
		if got := Match(c.scopes, c.need); got != c.want {
			t.Errorf("Match(%v, %q) = %v，期望 %v", c.scopes, c.need, got, c.want)
		}
	}
}

func TestLifecycle(t *testing.T) {
	m, store := newManager(t)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }

	exp := now.Add(time.Hour)
	v, secret, err := m.Create(Input{Name: " 批量脚本 ", Scopes: []string{"ddns:read", "ddns:read", "status:read"}, ExpiresAt: &exp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, Prefix) || v.Name != "批量脚本" || len(v.Scopes) != 2 {
		t.Fatalf("创建结果异常: %+v %s", v, secret)
	}

	// 磁盘上只有哈希，没有明文
	var raw fileData
	if _, err := store.Load(StoreName, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Tokens[0].Hash == secret || raw.Tokens[0].Hash != hashOf(secret) {
		t.Fatal("应只保存哈希")
	}

	if _, err := m.Authenticate(secret, "1.2.3.4"); err != nil {
		t.Fatalf("有效令牌应通过: %v", err)
	}
	if _, err := m.Authenticate(secret+"x", "1.2.3.4"); err == nil {
		t.Fatal("错误令牌应失败")
	}
	if got := m.List()[0]; got.LastUsedIP != "1.2.3.4" || got.LastUsedAt == nil {
		t.Fatalf("应记录最近使用: %+v", got)
	}

	// 过期后失效，且列表标记为已过期
	now = now.Add(2 * time.Hour)
	if _, err := m.Authenticate(secret, ""); err == nil || m.Peek(secret) {
		t.Fatal("过期令牌应失效")
	}
	if !m.List()[0].Expired {
		t.Fatal("列表应标记已过期")
	}
	// 已过期令牌可以在保持原过期时间的情况下修改其他字段
	if _, err := m.Update(v.ID, Input{Name: "改名", Scopes: []string{"ddns:write"}, ExpiresAt: &exp}); err != nil {
		t.Fatalf("保持原过期时间的修改应成功: %v", err)
	}
	// 设置一个已经过去的新过期时间应被拒绝
	past := now.Add(-time.Minute)
	if _, err := m.Update(v.ID, Input{Name: "改名", Scopes: []string{"ddns:write"}, ExpiresAt: &past}); err == nil {
		t.Fatal("过去的过期时间应被拒绝")
	}
	if _, err := m.Update(v.ID, Input{Name: "改名", Scopes: []string{"ddns:write"}}); err != nil {
		t.Fatal(err)
	}

	// 重新生成后旧明文立即失效
	_, next, err := m.Regenerate(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Authenticate(secret, ""); err == nil {
		t.Fatal("旧明文应失效")
	}
	if _, err := m.Authenticate(next, ""); err != nil {
		t.Fatalf("新明文应有效: %v", err)
	}

	// 停用与删除
	if _, err := m.Update(v.ID, Input{Name: "改名", Scopes: []string{"ddns:write"}, Disabled: true}); err != nil {
		t.Fatal(err)
	}
	if m.Peek(next) {
		t.Fatal("停用的令牌应失效")
	}
	if _, err := m.Delete(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Delete(v.ID); err != ErrNotFound {
		t.Fatalf("重复删除应返回 ErrNotFound: %v", err)
	}
}

func TestValidation(t *testing.T) {
	m, _ := newManager(t)
	bad := []Input{
		{Name: "", Scopes: []string{"ddns:read"}},
		{Name: "x", Scopes: nil},
		{Name: "x", Scopes: []string{"ddns"}},
		{Name: "x", Scopes: []string{"ddns:admin"}},
		{Name: "x", Scopes: []string{"unknown:read"}},
		{Name: strings.Repeat("长", 65), Scopes: []string{"ddns:read"}},
	}
	for _, in := range bad {
		if _, _, err := m.Create(in); err == nil {
			t.Errorf("应拒绝: %+v", in)
		}
	}
	for _, s := range []string{"*", "*:read", "*:write", "ddns:write"} {
		if _, _, err := m.Create(Input{Name: "ok", Scopes: []string{s}}); err != nil {
			t.Errorf("%q 应有效: %v", s, err)
		}
	}
}

func TestLegacyImportAndFlush(t *testing.T) {
	m, store := newManager(t)
	if err := m.ImportLegacy("legacy-secret-value-123"); err != nil {
		t.Fatal(err)
	}
	// 重复导入不新增
	if err := m.ImportLegacy("legacy-secret-value-123"); err != nil {
		t.Fatal(err)
	}
	if got := m.List(); len(got) != 1 || !got[0].Legacy || got[0].Scopes[0] != "*" {
		t.Fatalf("遗留令牌异常: %+v", got)
	}
	if _, err := m.Authenticate("legacy-secret-value-123", "9.9.9.9"); err != nil {
		t.Fatal(err)
	}
	// 最近使用信息节流写盘，Flush 后可从磁盘读到
	m.Flush()
	var raw fileData
	if _, err := store.Load(StoreName, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Tokens[0].LastUsedIP != "9.9.9.9" {
		t.Fatalf("Flush 后应落盘: %+v", raw.Tokens[0])
	}
}
