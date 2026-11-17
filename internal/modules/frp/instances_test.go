package frp

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

type testRouter struct{ mux *http.ServeMux }

func (tr testRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	tr.mux.HandleFunc(method+" /api/frp"+path, h)
}

func newTestModule(t *testing.T, dir string) (*Module, *httptest.Server, *logbuf.Buffer) {
	t.Helper()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(500, log.New(io.Discard, "", 0))
	m := New()
	if err := m.Init(module.Env{Store: store, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return m, srv, logs
}

func doJSON(t *testing.T, method, url string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// writeFakeFrp 写一个假的 frpc / frps：输出所用配置文件的第一行后常驻。
func writeFakeFrp(t *testing.T, path string) {
	t.Helper()
	script := "#!/bin/sh\necho \"using $2\"\nhead -1 \"$2\"\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// 旧版单实例配置在加载时迁移为实例：已配置的 frpc 迁移为一个实例并认领全部代理，程序路径移到模块设置；
// 未改过的 frps 不迁移；迁移结果立即保存，重复加载 key 不变。
func TestMigrateLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
  "client": {"enable": true, "binPath": "/opt/frpc", "serverAddr": "frp.example.com", "serverPort": 7001, "token": "t0k", "user": "", "protocol": "quic"},
  "server": {"enable": false, "binPath": "", "bindAddr": "", "bindPort": 7000, "token": "", "dashboardAddr": "127.0.0.1"},
  "install": {"proxy": "http://127.0.0.1:7890"},
  "proxies": [{"key": "p1", "name": "ssh", "type": "tcp", "enable": true, "localIp": "127.0.0.1", "localPort": 22, "remotePort": 6022}]
}`
	if err := os.WriteFile(filepath.Join(dir, "frp.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _, _ := newTestModule(t, dir)
	if len(m.cfg.Clients) != 1 || len(m.cfg.Servers) != 0 {
		t.Fatalf("迁移结果错误: %+v", m.cfg)
	}
	c := m.cfg.Clients[0]
	if c.Key == "" || c.Name != "frpc" || !c.Enable || c.ServerAddr != "frp.example.com" || c.ServerPort != 7001 || c.Token != "t0k" || c.Protocol != "quic" {
		t.Fatalf("frpc 实例错误: %+v", c)
	}
	if m.cfg.Proxies[0].Client != c.Key || m.cfg.Settings.FrpcPath != "/opt/frpc" || m.cfg.Install.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("代理归属、程序路径或安装设置错误: %+v", m.cfg)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "frp.json"))
	if strings.Contains(string(data), `"client": {`) || strings.Contains(string(data), `"server": {`) {
		t.Fatalf("迁移后应保存新格式:\n%s", data)
	}
	m2, _, _ := newTestModule(t, dir)
	if len(m2.cfg.Clients) != 1 || m2.cfg.Clients[0].Key != c.Key {
		t.Fatalf("重复加载不应再次迁移: %+v", m2.cfg.Clients)
	}

	// 启用过或改过设置的 frps 迁移为实例；没配置过的 frpc 不迁移
	dir = t.TempDir()
	legacy = `{"client": {"enable": false, "serverAddr": "", "serverPort": 7000}, "server": {"enable": false, "binPath": "/opt/frps", "bindPort": 7100, "token": "s"}, "proxies": []}`
	if err := os.WriteFile(filepath.Join(dir, "frp.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _, _ = newTestModule(t, dir)
	if len(m.cfg.Clients) != 0 || len(m.cfg.Servers) != 1 || m.cfg.Settings.FrpsPath != "/opt/frps" {
		t.Fatalf("迁移结果错误: %+v", m.cfg)
	}
	if s := m.cfg.Servers[0]; s.Name != "frps" || s.Enable || s.BindPort != 7100 || s.Token != "s" || s.DashboardAddr != "127.0.0.1" {
		t.Fatalf("frps 实例错误: %+v", s)
	}
}

// 多个 frpc / frps 实例各自运行独立进程与配置文件；修改代理只重启所属实例；
// 删除实例时停止进程、删除配置文件与所属代理；恢复默认配置清空全部实例。
func TestMultipleInstances(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	dir := t.TempDir()
	frpc, frps := filepath.Join(dir, "fake-frpc"), filepath.Join(dir, "fake-frps")
	writeFakeFrp(t, frpc)
	writeFakeFrp(t, frps)
	m, srv, logs := newTestModule(t, dir)
	api := srv.URL + "/api/frp"
	if code := doJSON(t, "PUT", api+"/settings", Settings{FrpcPath: frpc, FrpsPath: frps}, nil); code != 200 {
		t.Fatalf("保存设置失败 %d", code)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	var a, b ClientView
	if code := doJSON(t, "POST", api+"/clients", Client{Name: "家里", Enable: true, ClientSettings: ClientSettings{ServerAddr: "a.example.com", Token: "ta"}}, &a); code != 200 {
		t.Fatalf("新增实例失败 %d", code)
	}
	if code := doJSON(t, "POST", api+"/clients", Client{Name: "公司", Enable: true, ClientSettings: ClientSettings{ServerAddr: "b.example.com"}}, &b); code != 200 {
		t.Fatalf("新增实例失败 %d", code)
	}
	if a.ServerPort != 7000 || a.Token != crud.SecretMask {
		t.Fatalf("应补默认端口并脱敏令牌: %+v", a)
	}
	if code := doJSON(t, "POST", api+"/clients", Client{Name: "x", Enable: true}, nil); code != 400 {
		t.Fatalf("启用但没有服务端地址应被拒绝，实际 %d", code)
	}
	var pa, pb Proxy
	doJSON(t, "POST", api+"/proxies", Proxy{Client: a.Key, Name: "ssh", Type: TypeTCP, Enable: true, LocalPort: 22, RemotePort: 6022}, &pa)
	doJSON(t, "POST", api+"/proxies", Proxy{Client: b.Key, Name: "ssh", Type: TypeTCP, Enable: true, LocalPort: 22, RemotePort: 7022}, &pb)
	if code := doJSON(t, "POST", api+"/proxies", Proxy{Client: "nope", Name: "x", Type: TypeTCP, LocalPort: 1, RemotePort: 1}, nil); code != 400 {
		t.Fatalf("所属实例不存在应被拒绝，实际 %d", code)
	}

	running := func(runs map[string]*instRuntime, key string) func() bool {
		return func() bool {
			m.mu.Lock()
			defer m.mu.Unlock()
			rt := runs[key]
			return rt != nil && rt.sup.State().Running
		}
	}
	pid := func(runs map[string]*instRuntime, key string) int {
		m.mu.Lock()
		defer m.mu.Unlock()
		return runs[key].sup.State().PID
	}
	waitFor(t, "frpc 家里运行", running(m.clients, a.Key))
	waitFor(t, "frpc 公司运行", running(m.clients, b.Key))
	readCfg := func(bin, key string) string {
		data, err := os.ReadFile(filepath.Join(dir, "frp", bin+"-"+key+".toml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	ca, cb := readCfg("frpc", a.Key), readCfg("frpc", b.Key)
	if !strings.Contains(ca, `serverAddr = "a.example.com"`) || !strings.Contains(ca, "remotePort = 6022") || strings.Contains(ca, "7022") {
		t.Fatalf("实例 家里 的配置错误:\n%s", ca)
	}
	if !strings.Contains(cb, `serverAddr = "b.example.com"`) || !strings.Contains(cb, "remotePort = 7022") || strings.Contains(cb, "6022") {
		t.Fatalf("实例 公司 的配置错误:\n%s", cb)
	}
	if st, _ := os.Stat(filepath.Join(dir, "frp", "frpc-"+a.Key+".toml")); st.Mode().Perm() != 0o600 {
		t.Errorf("配置文件含令牌，权限应为 0600，实际 %v", st.Mode().Perm())
	}
	waitFor(t, "采集实例日志", func() bool {
		for _, e := range logs.Since(0) {
			if e.Source == "frpc:家里" && strings.Contains(e.Message, "serverAddr") {
				return true
			}
		}
		return false
	})

	// 修改 家里 的代理：只重启 家里，公司 的进程不变
	pidA, pidB := pid(m.clients, a.Key), pid(m.clients, b.Key)
	pa.RemotePort = 6122
	if code := doJSON(t, "PUT", api+"/proxies/"+pa.Key, pa, nil); code != 200 {
		t.Fatalf("修改代理失败 %d", code)
	}
	waitFor(t, "家里 重启", func() bool { return pid(m.clients, a.Key) != pidA && running(m.clients, a.Key)() })
	if pid(m.clients, b.Key) != pidB {
		t.Error("修改其他实例的代理不应重启本实例")
	}
	if !strings.Contains(readCfg("frpc", a.Key), "remotePort = 6122") {
		t.Error("配置文件未更新")
	}
	// 代理换到 公司：两个实例都重启
	pidA, pidB = pid(m.clients, a.Key), pid(m.clients, b.Key)
	pa.Client, pa.Name = b.Key, "web"
	if code := doJSON(t, "PUT", api+"/proxies/"+pa.Key, pa, nil); code != 200 {
		t.Fatalf("修改代理失败 %d", code)
	}
	waitFor(t, "两个实例都重启", func() bool { return pid(m.clients, a.Key) != pidA && pid(m.clients, b.Key) != pidB })
	if strings.Contains(readCfg("frpc", a.Key), "[[proxies]]") || !strings.Contains(readCfg("frpc", b.Key), `name = "web"`) {
		t.Error("代理归属变更后配置错误")
	}

	// 停用实例：进程停止
	if code := doJSON(t, "PUT", api+"/clients/"+b.Key+"/enable", map[string]bool{"enable": false}, nil); code != 200 {
		t.Fatalf("停用失败 %d", code)
	}
	waitFor(t, "公司 停止", func() bool { return !running(m.clients, b.Key)() })

	// 删除实例：所属代理一并删除，配置文件删除
	if code := doJSON(t, "DELETE", api+"/clients/"+b.Key, nil, nil); code != 200 {
		t.Fatalf("删除失败 %d", code)
	}
	var proxies []Proxy
	doJSON(t, "GET", api+"/proxies", nil, &proxies)
	if len(proxies) != 0 {
		t.Fatalf("删除实例后应删除其代理: %+v", proxies)
	}
	if _, err := os.Stat(filepath.Join(dir, "frp", "frpc-"+b.Key+".toml")); !os.IsNotExist(err) {
		t.Errorf("删除实例后配置文件应删除: %v", err)
	}
	var saved fileConfig
	_, _ = m.store.Load(ID, &saved)
	if len(saved.Clients) != 1 || len(saved.Proxies) != 0 {
		t.Fatalf("删除结果未保存: %+v", saved)
	}

	// frps 实例：两个实例各自运行
	var s1, s2 ServerView
	doJSON(t, "POST", api+"/servers", Server{Name: "主", Enable: true, ServerSettings: ServerSettings{BindPort: 7100, Token: "st", DashboardPort: 7500, DashboardUser: "admin", DashboardPwd: "dp"}}, &s1)
	doJSON(t, "POST", api+"/servers", Server{Name: "备", Enable: true, ServerSettings: ServerSettings{BindPort: 7200}}, &s2)
	if s1.Token != crud.SecretMask || s1.DashboardPwd != crud.SecretMask || s1.DashboardAddr != "127.0.0.1" {
		t.Fatalf("frps 实例应脱敏并补默认值: %+v", s1)
	}
	waitFor(t, "frps 主运行", running(m.servers, s1.Key))
	waitFor(t, "frps 备运行", running(m.servers, s2.Key))
	if !strings.Contains(readCfg("frps", s1.Key), "bindPort = 7100") || !strings.Contains(readCfg("frps", s2.Key), "bindPort = 7200") {
		t.Fatal("frps 配置错误")
	}
	pid1 := pid(m.servers, s1.Key)
	if code := doJSON(t, "POST", api+"/servers/"+s2.Key+"/restart", nil, nil); code != 200 {
		t.Fatalf("重启失败 %d", code)
	}
	if pid(m.servers, s1.Key) != pid1 {
		t.Error("重启一个实例不应影响其他实例")
	}
	if code := doJSON(t, "POST", api+"/servers/nope/restart", nil, nil); code != 404 {
		t.Fatalf("重启不存在的实例应返回 404，实际 %d", code)
	}
	waitFor(t, "frps 备重启完成", running(m.servers, s2.Key))

	// 概览：多个实例时显示运行数 / 启用数
	sum := m.Summary()
	found := false
	for _, it := range sum {
		if it.Label == "服务端" && it.Value == "2 / 2" && it.Level == "ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("概览应汇总 frps 实例: %+v", sum)
	}

	// 恢复默认配置：停止全部进程、清空配置与生成的文件，保留程序
	if code := doJSON(t, "POST", api+"/reset", nil, nil); code != 200 {
		t.Fatalf("恢复默认配置失败 %d", code)
	}
	waitFor(t, "全部停止", func() bool { return !running(m.clients, a.Key)() && !running(m.servers, s1.Key)() })
	if len(m.cfg.Clients)+len(m.cfg.Servers)+len(m.cfg.Proxies) != 0 || m.cfg.Settings.FrpcPath != "" {
		t.Errorf("配置未恢复默认: %+v", m.cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, "frp")); !os.IsNotExist(err) {
		t.Errorf("生成的配置文件应被删除: %v", err)
	}
	if _, err := os.Stat(frpc); err != nil {
		t.Errorf("程序应保留: %v", err)
	}
}

// 查看密钥明文：GET .../secrets 返回明文；原样提交脱敏占位符时保留原值，提交新值时更新。
func TestSecrets(t *testing.T) {
	m, srv, _ := newTestModule(t, t.TempDir())
	api := srv.URL + "/api/frp"
	var c ClientView
	doJSON(t, "POST", api+"/clients", Client{Name: "家里", ClientSettings: ClientSettings{ServerAddr: "a", Token: "secret-1"}}, &c)
	var sec map[string]string
	if code := doJSON(t, "GET", api+"/clients/"+c.Key+"/secrets", nil, &sec); code != 200 || sec["token"] != "secret-1" {
		t.Fatalf("应返回令牌明文: %d %v", code, sec)
	}
	if code := doJSON(t, "GET", api+"/clients/nope/secrets", nil, nil); code != 404 {
		t.Fatalf("实例不存在应返回 404，实际 %d", code)
	}
	// 前端取回明文后原样提交，与提交占位符等价
	c.Remark = "改备注"
	if code := doJSON(t, "PUT", api+"/clients/"+c.Key, c.Client, nil); code != 200 || m.cfg.Clients[0].Token != "secret-1" {
		t.Fatalf("提交占位符应保留原令牌: %d %+v", code, m.cfg.Clients[0])
	}
	c.Token = "secret-2"
	doJSON(t, "PUT", api+"/clients/"+c.Key, c.Client, nil)
	if m.cfg.Clients[0].Token != "secret-2" {
		t.Fatalf("应更新令牌: %+v", m.cfg.Clients[0])
	}
	// 界面视图包含运行状态等只读字段，原样提交时忽略这些字段，实例字段照常保存
	c.Remark = "视图提交"
	if code := doJSON(t, "PUT", api+"/clients/"+c.Key, c, nil); code != 200 || m.cfg.Clients[0].Remark != "视图提交" || m.cfg.Clients[0].Token != "secret-2" {
		t.Fatalf("提交含状态字段的视图应忽略只读字段并保存: %d %+v", code, m.cfg.Clients[0])
	}
	// 其他未知字段仍拒绝
	if code := doJSON(t, "PUT", api+"/clients/"+c.Key, map[string]any{"name": "x", "bogus": 1}, nil); code != 400 {
		t.Fatalf("未知字段应被拒绝，实际 %d", code)
	}

	var s ServerView
	doJSON(t, "POST", api+"/servers", Server{Name: "主", ServerSettings: ServerSettings{Token: "st", DashboardPort: 7500, DashboardUser: "u", DashboardPwd: "dp"}}, &s)
	doJSON(t, "GET", api+"/servers/"+s.Key+"/secrets", nil, &sec)
	if sec["token"] != "st" || sec["dashboardPwd"] != "dp" {
		t.Fatalf("应返回 frps 密钥明文: %v", sec)
	}
	var p Proxy
	doJSON(t, "POST", api+"/proxies", Proxy{Client: c.Key, Name: "rdp", Type: TypeSTCP, LocalPort: 3389, SecretKey: "sk"}, &p)
	if p.SecretKey != crud.SecretMask {
		t.Fatalf("代理密钥应脱敏: %+v", p)
	}
	doJSON(t, "GET", api+"/proxies/"+p.Key+"/secrets", nil, &sec)
	if sec["secretKey"] != "sk" {
		t.Fatalf("应返回代理密钥明文: %v", sec)
	}
}
