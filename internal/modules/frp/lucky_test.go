package frp

import (
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

// luckyFRPSample 按 Lucky 2.27.2 前端提交的实例结构构造。
const luckyFRPSample = `{"List":[
  {"Key":"c1","Remark":"home","Enable":true,"Type":"client",
   "Params":{"ServerAddr":"frp.example.com","ServerPort":7001,"User":"me","AuthMethod":"token","Token":"t0k","Protocol":"tcp","TLSEnable":true,"AdminPort":0},
   "proxies":[
     {"name":"ssh","type":"tcp","disabled":false,"annotations":{},"metadatas":{},"localIP":"127.0.0.1","localPort":22,"remotePort":6022,"useEncryption":false,"natTraversal":{"disableAssistedAddrs":false}},
     {"name":"web","type":"http","localIP":"192.168.1.2","localPort":80,"customDomains":["a.example.com"],"useCompression":true},
     {"name":"off","type":"tcp","disabled":true,"localPort":3389,"remotePort":6389},
     {"name":"vnet","type":"tcp","plugin":"virtual_net","remotePort":7000}
   ],
   "visitors":[{"name":"v","type":"stcp"}]},
  {"Key":"c2","Remark":"second","Enable":true,"Type":"client","Params":{"ServerAddr":"b.example.com"}},
  {"Key":"s1","Remark":"srv","Enable":false,"Type":"server",
   "Params":{"BindAddr":"0.0.0.0","BindPort":7100,"Token":"st","VhostHTTPPort":8080,"VhostHTTPSPort":0,"SubDomainHost":"example.com","AllowPorts":"6000-6100, 7000","DashboardPort":7500,"DashboardUser":"admin"}}
]}`

func TestImportLuckyFRP(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	var doc luckyimport.Document
	if err := json.Unmarshal([]byte(luckyFRPSample), &doc); err != nil {
		t.Fatal(err)
	}
	rep, err := m.ImportLucky(doc, luckyimport.Options{})
	if err != nil || rep == nil {
		t.Fatalf("导入失败: %v %v", rep, err)
	}
	// 每个客户端、服务端实例都导入：两个 frpc 与一个 frps
	if rep.Found != 3 || rep.Imported != 3 || len(m.cfg.Clients) != 2 || len(m.cfg.Servers) != 1 {
		t.Fatalf("统计错误: %+v %+v", rep, m.cfg)
	}
	c := m.cfg.Clients[0]
	if c.Enable || c.Name != "home" || c.ServerAddr != "frp.example.com" || c.ServerPort != 7001 || c.Token != "t0k" || c.User != "me" || c.Protocol != "" {
		t.Fatalf("客户端映射错误: %+v", c)
	}
	if c2 := m.cfg.Clients[1]; c2.Name != "second" || c2.ServerAddr != "b.example.com" || c2.ServerPort != 7000 {
		t.Fatalf("第二个客户端映射错误: %+v", c2)
	}
	for _, p := range m.cfg.Proxies {
		if p.Client != c.Key {
			t.Fatalf("代理应归属第一个客户端: %+v", p)
		}
	}
	if len(m.cfg.Proxies) != 3 {
		t.Fatalf("应导入 3 个代理（插件代理跳过），实际 %+v", m.cfg.Proxies)
	}
	if p := m.cfg.Proxies[1]; p.Type != TypeHTTP || p.CustomDomains[0] != "a.example.com" || p.LocalIP != "192.168.1.2" || !p.UseCompression || p.UseEncryption {
		t.Fatalf("http 代理映射错误: %+v", p)
	}
	if m.cfg.Proxies[2].Enable {
		t.Fatal("disabled 代理应保持停用")
	}
	s := m.cfg.Servers[0]
	if s.Enable || s.Name != "srv" || s.BindPort != 7100 || s.Token != "st" || s.VhostHTTPPort != 8080 || s.SubdomainHost != "example.com" || s.AllowPorts != "6000-6100,7000" || s.DashboardPort != 0 || s.BindAddr != "" || s.ProxyBindAddr != "" {
		t.Fatalf("服务端映射错误: %+v", s)
	}
	joined := strings.Join(rep.Warnings, "\n")
	for _, want := range []string{"virtual_net", "访问者", "控制台未迁移"} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少提示 %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "annotations") || strings.Contains(joined, "natTraversal") || strings.Contains(joined, "useEncryption") || strings.Contains(joined, "useCompression") {
		t.Errorf("空值字段不应提示:\n%s", joined)
	}

	// 再次导入：连接同一服务端的客户端追加到已有实例、同名代理跳过，同端口的服务端跳过，不产生重复
	rep, err = m.ImportLucky(doc, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.cfg.Proxies) != 3 || len(m.cfg.Clients) != 2 || len(m.cfg.Servers) != 1 {
		t.Fatalf("重复导入不应产生重复: %+v", m.cfg)
	}
	if joined := strings.Join(rep.Warnings, "\n"); !strings.Contains(joined, "代理追加到该实例") || !strings.Contains(joined, "已使用端口 7100") {
		t.Errorf("缺少重复导入提示:\n%s", joined)
	}
	// 其他模块的 List 配置不应被认领
	other := luckyimport.Document{"List": json.RawMessage(`[{"Type":"tunnel"}]`)}
	if rep, _ := m.ImportLucky(other, luckyimport.Options{}); rep != nil {
		t.Fatal("cloudflared 配置不应被 frp 认领")
	}
}

// TestImportLuckyFRPBindAddr 验证 Lucky 的 frps 监听地址：IP 同时用作连接与代理监听地址，
// 0.0.0.0 与 :: 视为所有地址，不是 IP 的地址只提示、改为所有地址，不会跳过整个实例。
func TestImportLuckyFRPBindAddr(t *testing.T) {
	cases := []struct{ in, want, warn string }{
		{"192.168.1.2", "192.168.1.2", ""},
		{"[::1]", "::1", ""},
		{"::", "", ""},
		{"0.0.0.0", "", ""},
		{"localhost", "", "不是 IP 地址"},
	}
	for _, c := range cases {
		st, _ := config.Open(t.TempDir())
		m := New()
		if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
			t.Fatal(err)
		}
		raw := `{"List":[{"Key":"s1","Remark":"srv","Enable":false,"Type":"server","Params":{"BindAddr":"` + c.in + `","BindPort":7100}}]}`
		var doc luckyimport.Document
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		rep, err := m.ImportLucky(doc, luckyimport.Options{})
		if err != nil || rep == nil {
			t.Fatalf("%s: 导入失败: %v %v", c.in, rep, err)
		}
		if len(m.cfg.Servers) != 1 {
			t.Fatalf("%s: 应导入一个 frps 实例: %+v %v", c.in, m.cfg.Servers, rep.Warnings)
		}
		s := m.cfg.Servers[0]
		if s.BindPort != 7100 || s.BindAddr != c.want || s.ProxyBindAddr != c.want {
			t.Errorf("%s: 监听地址应为 %q: %+v %v", c.in, c.want, s, rep.Warnings)
		}
		joined := strings.Join(rep.Warnings, "\n")
		if c.warn != "" && !strings.Contains(joined, c.warn) || strings.Contains(joined, "跳过") {
			t.Errorf("%s: 提示不符: %s", c.in, joined)
		}
	}
}
