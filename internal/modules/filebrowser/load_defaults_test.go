package filebrowser

import (
	"encoding/json"
	"io"
	"log"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的配置文件。
func initFromRaw(t *testing.T, raw map[string]any) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// 配置文件缺登录方式、管理员用户名、访问路径与共享目录名称时加载后补上默认值，
// 否则生成的 File Browser 配置既不开密码登录也不开免登录，没有任何登录方式。
func TestInitFillsMissingInstanceDefaults(t *testing.T) {
	m := initFromRaw(t, map[string]any{"instances": []any{
		map[string]any{"key": "f1", "name": "files", "enable": true, "port": 18080, "adminPassword": "Secret123",
			"sources": []any{map[string]any{"path": "/srv/data/"}, map[string]any{"path": "/"}}},
		map[string]any{"key": "f2", "name": "proxy", "enable": true, "port": 18081, "authMethod": "proxy",
			"sources": []any{map[string]any{"path": "/srv/media"}}},
	}})
	in := m.cfg.Instances[0]
	if in.AuthMethod != AuthPassword || in.AdminUser != defaultAdminUser || in.BaseURL != "/" {
		t.Fatalf("未补默认值: auth=%q admin=%q base=%q", in.AuthMethod, in.AdminUser, in.BaseURL)
	}
	if in.Sources[0].Name != "data" || in.Sources[1].Name != "root" {
		t.Fatalf("共享目录名称未补默认值: %+v", in.Sources)
	}
	if px := m.cfg.Instances[1]; px.ProxyHeader != defaultProxyHeader {
		t.Fatalf("代理认证请求头未补默认值: %q", px.ProxyHeader)
	}

	// 运行时：生成的 File Browser 配置开启密码登录，管理员与共享目录名称正确
	data, err := in.Config("/d")
	if err != nil {
		t.Fatal(err)
	}
	var c fbConfig
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if !c.Auth.Methods.Password.Enabled || c.Auth.Methods.NoAuth || c.Auth.AdminUsername != "admin" || c.HTTP.BaseURL != "/" {
		t.Fatalf("生成的配置不符: %+v", c.Auth)
	}
	if len(c.Server.Sources) != 2 || c.Server.Sources[0].Name != "data" {
		t.Fatalf("生成的共享目录不符: %+v", c.Server.Sources)
	}
	data, _ = m.cfg.Instances[1].Config("/d")
	c = fbConfig{}
	_ = json.Unmarshal(data, &c)
	if !c.Auth.Methods.Proxy.Enabled || c.Auth.Methods.Proxy.Header != defaultProxyHeader || c.Auth.Methods.Password.Enabled {
		t.Fatalf("代理认证的配置不符: %+v", c.Auth.Methods)
	}
}

// 已有的值保持不变。
func TestInitKeepsExistingInstanceValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{"instances": []any{
		map[string]any{"key": "f1", "name": "files", "enable": true, "port": 18080, "authMethod": "noauth", "adminUser": "boss",
			"baseUrl": "/files", "sources": []any{map[string]any{"path": "/srv/data", "name": "资料"}}},
		map[string]any{"key": "f2", "name": "proxy", "enable": true, "port": 18081, "authMethod": "proxy", "proxyHeader": "X-User",
			"sources": []any{map[string]any{"path": "/srv/media"}}},
	}})
	in := m.cfg.Instances[0]
	if in.AuthMethod != AuthNone || in.AdminUser != "boss" || in.BaseURL != "/files" || in.Sources[0].Name != "资料" {
		t.Fatalf("已有值被改动: %+v", in)
	}
	if in.ProxyHeader != "" {
		t.Fatalf("非代理认证不应补请求头: %q", in.ProxyHeader)
	}
	if px := m.cfg.Instances[1]; px.ProxyHeader != "X-User" {
		t.Fatalf("代理请求头被改动: %q", px.ProxyHeader)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameInstanceDefaults(t *testing.T) {
	i := validInstance()
	i.Sources = []Source{{Path: " /srv/data/ "}}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	if i.AuthMethod != AuthPassword || i.AdminUser != defaultAdminUser || i.BaseURL != "/" || i.Sources[0].Name != "data" || i.Sources[0].Path != "/srv/data" {
		t.Fatalf("保存时默认值不符: %+v", i)
	}
	p := validInstance()
	p.AuthMethod = AuthProxy
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.ProxyHeader != defaultProxyHeader {
		t.Fatalf("代理认证保存时默认请求头不符: %q", p.ProxyHeader)
	}
	bad := validInstance()
	bad.AuthMethod = "magic"
	if err := bad.Validate(); err == nil {
		t.Fatal("未知登录方式应被拒绝")
	}
}
