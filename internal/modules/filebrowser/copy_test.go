package filebrowser

import (
	"net/http"
	"testing"

	"github.com/wangwanjie/gately/internal/crud"
)

// 复制实例：管理员密码为占位符时沿用模板，副本排在模板之后，响应中的密码脱敏，模板不变。
func TestCopyInstance(t *testing.T) {
	m, srv := newTestModule(t, t.TempDir())
	tpl := validInstance()
	tpl.Key, tpl.Enable = "i1", false
	if err := tpl.Validate(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cfg.Instances = []Instance{tpl}
	m.mu.Unlock()

	draft := validInstance()
	draft.Name, draft.Port, draft.AdminPassword = "files 副本", 8081, crud.SecretMask
	var out struct {
		Instance
	}
	if code := doJSON(t, http.MethodPost, srv.URL+"/api/filebrowser/instances/i1/copy", draft, &out); code != 200 {
		t.Fatalf("复制失败: %d", code)
	}
	if out.AdminPassword != crud.SecretMask {
		t.Fatalf("响应中的密码应脱敏: %q", out.AdminPassword)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Instances) != 2 || m.cfg.Instances[1].AdminPassword != "Secret123" || m.cfg.Instances[1].Port != 8081 {
		t.Fatalf("副本应沿用模板的管理员密码: %+v", m.cfg.Instances)
	}
	if m.cfg.Instances[0].Port != 8080 || m.cfg.Instances[0].Key != "i1" {
		t.Fatalf("模板不应被修改: %+v", m.cfg.Instances[0])
	}
}
