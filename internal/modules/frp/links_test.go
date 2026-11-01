package frp

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestClientAccessValidate 校验只用于生成代理链接的访问地址设置。
func TestClientAccessValidate(t *testing.T) {
	a := ClientAccess{HTTPScheme: " HTTPS ", HTTPPort: 443, HTTPSPort: 8443, SubdomainHost: " Frp.Example.com. "}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.HTTPScheme != "https" || a.SubdomainHost != "frp.example.com" {
		t.Fatalf("应规范化: %+v", a)
	}
	if err := (&ClientAccess{}).Validate(); err != nil {
		t.Fatalf("留空应合法: %v", err)
	}
	for _, bad := range []ClientAccess{
		{HTTPScheme: "ftp"}, {HTTPPort: -1}, {HTTPPort: 65536}, {HTTPSPort: 70000},
		{SubdomainHost: "*.example.com"}, {SubdomainHost: "example.com:8080"}, {SubdomainHost: "a/b"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v 应报错", bad)
		}
	}
	c := Client{Name: "x", Access: ClientAccess{HTTPScheme: "javascript"}}
	if err := c.Validate(); err == nil {
		t.Fatal("实例校验应包含访问地址")
	}
}

// TestProxyDomainsValidate 自定义域名统一为 Host 的形式，子域名只能是一级名称。
func TestProxyDomainsValidate(t *testing.T) {
	p := Proxy{Name: "web", Type: TypeHTTP, LocalPort: 80, CustomDomains: []string{" A.Example.com ", "中文.com", "*.Wild.com", "a.example.com"}, Subdomain: " NAS "}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.CustomDomains, ",") != "a.example.com,xn--fiq228c.com,*.wild.com" || p.Subdomain != "nas" {
		t.Fatalf("规范化结果错误: %v %q", p.CustomDomains, p.Subdomain)
	}
	for _, bad := range []Proxy{
		{Name: "a", Type: TypeHTTP, LocalPort: 80, CustomDomains: []string{"a.com:8080"}},
		{Name: "a", Type: TypeHTTP, LocalPort: 80, CustomDomains: []string{"a.com/x"}},
		{Name: "a", Type: TypeHTTPS, LocalPort: 443, CustomDomains: []string{"user@a.com"}},
		{Name: "a", Type: TypeHTTP, LocalPort: 80, Subdomain: "a.b"},
		{Name: "a", Type: TypeHTTP, LocalPort: 80, Subdomain: "*"},
		{Name: "a", Type: TypeHTTP, LocalPort: 80, Subdomain: "a b"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v 应报错", bad)
		}
	}
}

// TestImportSkipsBadCustomDomains 导入时只去掉写错的自定义域名，代理照常导入。
func TestImportSkipsBadCustomDomains(t *testing.T) {
	conf := `serverAddr = "1.2.3.4"

[[proxies]]
name = "web"
type = "http"
localPort = 80
customDomains = ["ok.example.com", "bad.com:8080"]
`
	res, err := ParseImport(RoleClient, conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proxies) != 1 || strings.Join(res.Proxies[0].CustomDomains, ",") != "ok.example.com" {
		t.Fatalf("应只去掉写错的域名: %+v", res.Proxies)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "bad.com:8080") {
		t.Fatalf("应提示忽略的域名: %v", res.Warnings)
	}
}

// TestClientAccessNoRestart 只改访问地址不重启 frpc，改名称或连接参数照常重启。
func TestClientAccessNoRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	dir := t.TempDir()
	frpc := filepath.Join(dir, "fake-frpc")
	writeFakeFrp(t, frpc)
	m, srv, _ := newTestModule(t, dir)
	api := srv.URL + "/api/frp"
	doJSON(t, "PUT", api+"/settings", Settings{FrpcPath: frpc}, nil)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	var c ClientView
	if code := doJSON(t, "POST", api+"/clients", Client{Name: "家里", Enable: true, ClientSettings: ClientSettings{ServerAddr: "a.example.com"}}, &c); code != 200 {
		t.Fatalf("新增实例失败 %d", code)
	}
	pid := func() int {
		m.mu.Lock()
		defer m.mu.Unlock()
		if rt := m.clients[c.Key]; rt != nil && rt.sup.State().Running {
			return rt.sup.State().PID
		}
		return 0
	}
	waitFor(t, "frpc 运行", func() bool { return pid() != 0 })
	first := pid()

	upd := c.Client
	upd.Access = ClientAccess{HTTPPort: 8080, HTTPSPort: 8443, SubdomainHost: "frp.example.com"}
	upd.Remark = "改备注"
	if code := doJSON(t, "PUT", api+"/clients/"+c.Key, upd, nil); code != 200 {
		t.Fatalf("保存失败 %d", code)
	}
	if got := pid(); got != first {
		t.Fatalf("只改访问地址不应重启 frpc：%d → %d", first, got)
	}
	if m.cfg.Clients[0].Access.HTTPPort != 8080 {
		t.Fatal("访问地址未保存")
	}

	upd.Name = "家里2"
	doJSON(t, "PUT", api+"/clients/"+c.Key, upd, nil)
	waitFor(t, "改名后重启", func() bool { p := pid(); return p != 0 && p != first })
}
