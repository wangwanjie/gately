package frp

import (
	"net/http"
	"testing"

	"github.com/wangwanjie/gately/internal/crud"
)

// 复制 frpc/frps 实例与代理：占位符沿用模板的令牌、面板密码与 stcp 密钥，新填写的值原样保存；
// 副本排在模板之后，响应中的密钥脱敏，模板本身不变。
func TestCopyInstancesAndProxies(t *testing.T) {
	m, srv, _ := newTestModule(t, t.TempDir())
	m.mu.Lock()
	m.cfg.Clients = []Client{
		{Key: "c1", Name: "家里", ClientSettings: ClientSettings{ServerAddr: "a.example.com", ServerPort: 7000, Token: "ctok"}},
		{Key: "c2", Name: "公司", ClientSettings: ClientSettings{ServerAddr: "b.example.com", ServerPort: 7000}},
	}
	m.cfg.Servers = []Server{{Key: "s1", Name: "frps", ServerSettings: ServerSettings{BindPort: 7000, Token: "stok", DashboardPort: 7500, DashboardUser: "admin", DashboardPwd: "dpw"}}}
	m.cfg.Proxies = []Proxy{{Key: "p1", Client: "c1", Name: "ssh", Type: "stcp", LocalIP: "127.0.0.1", LocalPort: 22, SecretKey: "sk"}}
	m.mu.Unlock()

	var cv ClientView
	body := Client{Name: "家里 副本", ClientSettings: ClientSettings{ServerAddr: "evil.example.com", ServerPort: 7000, Token: crud.SecretMask}}
	if code := doJSON(t, "POST", srv.URL+"/api/frp/clients/c1/copy", body, &cv); code != http.StatusOK {
		t.Fatalf("复制 frpc 实例失败: %d", code)
	}
	if cv.Token != crud.SecretMask || cv.Key == "" || cv.Key == "c1" {
		t.Fatalf("响应应脱敏并带新 key: %+v", cv.Client)
	}
	var ss Server
	srvBody := Server{Name: "frps2", ServerSettings: ServerSettings{BindPort: 7001, Token: crud.SecretMask, DashboardPort: 7501, DashboardUser: "admin", DashboardPwd: "new-pw"}}
	if code := doJSON(t, "POST", srv.URL+"/api/frp/servers/s1/copy", srvBody, &ss); code != http.StatusOK {
		t.Fatalf("复制 frps 实例失败: %d", code)
	}
	proxyBody := Proxy{Client: "c1", Name: "ssh-copy", Type: "stcp", LocalIP: "127.0.0.1", LocalPort: 2222, SecretKey: crud.SecretMask}
	if code := doJSON(t, "POST", srv.URL+"/api/frp/proxies/p1/copy", proxyBody, nil); code != http.StatusOK {
		t.Fatalf("复制代理失败: %d", code)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Clients) != 3 || m.cfg.Clients[1].Key != cv.Key || m.cfg.Clients[2].Key != "c2" {
		t.Fatalf("frpc 副本应排在模板之后: %+v", m.cfg.Clients)
	}
	if m.cfg.Clients[1].Token != "ctok" || m.cfg.Clients[1].ServerAddr != "evil.example.com" {
		t.Fatalf("占位符应沿用模板令牌: %+v", m.cfg.Clients[1])
	}
	if m.cfg.Clients[0].Token != "ctok" || m.cfg.Clients[0].ServerAddr != "a.example.com" {
		t.Fatalf("模板不应被修改: %+v", m.cfg.Clients[0])
	}
	if s := m.cfg.Servers[1]; s.Token != "stok" || s.DashboardPwd != "new-pw" {
		t.Fatalf("frps 副本应沿用令牌、保存新面板密码: %+v", s)
	}
	if m.cfg.Servers[0].DashboardPwd != "dpw" {
		t.Fatalf("模板面板密码不应被修改: %+v", m.cfg.Servers[0])
	}
	if len(m.cfg.Proxies) != 2 || m.cfg.Proxies[1].SecretKey != "sk" || m.cfg.Proxies[1].Name != "ssh-copy" {
		t.Fatalf("代理副本应沿用 stcp 密钥: %+v", m.cfg.Proxies)
	}
}

// 引用不存在的 frpc 实例的代理副本被拒绝，不落盘。
func TestCopyProxyRejectsUnknownClient(t *testing.T) {
	m, srv, _ := newTestModule(t, t.TempDir())
	m.mu.Lock()
	m.cfg.Clients = []Client{{Key: "c1", Name: "家里", ClientSettings: ClientSettings{ServerAddr: "a", ServerPort: 7000}}}
	m.cfg.Proxies = []Proxy{{Key: "p1", Client: "c1", Name: "ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 6022}}
	m.mu.Unlock()
	body := Proxy{Client: "gone", Name: "ssh2", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 6023}
	if code := doJSON(t, "POST", srv.URL+"/api/frp/proxies/p1/copy", body, nil); code != http.StatusBadRequest {
		t.Fatalf("引用不存在的实例应拒绝: %d", code)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Proxies) != 1 {
		t.Fatalf("被拒绝的复制不应新增: %+v", m.cfg.Proxies)
	}
}
