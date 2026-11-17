package app

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/modules/registry"
	"github.com/wangwanjie/gately/tools/demo/gen"
)

const demoTestAdminPassword = "Demo-Admin-Test-2026!"

// newDemoApp 用演示数据生成器的输出启动演示模式实例（不监听，只返回处理器），返回应用与服务器。
func newDemoApp(t *testing.T, portBase int) (*App, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GATELY_DEMO_SNAPSHOT", filepath.Join(t.TempDir(), "snap"))
	if err := gen.Generate(gen.Options{Dir: dir, PortBase: portBase}); err != nil {
		t.Fatal(err)
	}
	mods, certs := registry.New()
	t.Cleanup(func() { demo.Set(false) })
	a, err := New(Options{ConfigDir: dir, Modules: mods, Certs: certs, Requires: registry.Requires(), Absent: registry.Absent(),
		InitialPassword: demoTestAdminPassword, Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return a, srv
}

func demoClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func demoAdmin(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	c := demoClient()
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": demoTestAdminPassword}); code != 200 {
		t.Fatalf("管理员登录失败 %d %s", code, body)
	}
	return c
}

// 演示模式下即使是管理员会话，列出的能力也一律被拒绝，并返回统一的提示。
func TestDemoBlocksAdminCapabilities(t *testing.T) {
	_, srv := newDemoApp(t, 0)
	admin := demoAdmin(t, srv)
	cases := []struct{ name, method, path string }{
		{"Web 终端", "POST", "/api/webterminal/connections"},
		{"Web 终端会话", "GET", "/api/webterminal/sessions"},
		{"Docker 管理", "GET", "/api/docker/containers"},
		{"Docker 操作", "POST", "/api/docker/containers/x/start"},
		{"存储文件读取", "GET", "/api/storage/files/x/list"},
		{"存储文件下载", "GET", "/api/storage/files/x/download"},
		{"存储文件写入", "PUT", "/api/storage/files/x/write"},
		{"备份目录下载", "GET", "/api/backup/local/x"},
		{"计划任务执行", "POST", "/api/cron/tasks/cr-clean/run"},
		{"计划任务修改", "PUT", "/api/cron/tasks/cr-clean"},
		{"DDNS 同步", "POST", "/api/ddns/tasks/d-cf/sync"},
		{"STUN 脚本", "PUT", "/api/stun/rules/st-web"},
		{"证书签发", "POST", "/api/ssl/certs/c-wild/obtain"},
		{"证书脚本", "PUT", "/api/ssl/certs/c-wild"},
		{"WOL 关机命令", "POST", "/api/wol/devices/w-nas/shutdown"},
		{"WOL 唤醒", "POST", "/api/wol/devices/w-nas/wake"},
		{"在线安装", "POST", "/api/frp/install"},
		{"托管程序安装", "POST", "/api/tailscale/install"},
		{"在线升级", "POST", "/api/system/upgrade"},
		{"上传升级包", "POST", "/api/system/upgrade/upload"},
		{"检查更新", "GET", "/api/system/update"},
		{"备份导出", "GET", "/api/system/backup"},
		{"备份还原", "POST", "/api/system/restore"},
		{"诊断包", "GET", "/api/system/diagnostics"},
		{"创建 API 令牌", "POST", "/api/tokens"},
		{"通知测试发送", "POST", "/api/notify/channels/n-tg/test"},
		{"Webhook 发送", "POST", "/api/notify/test"},
		{"IP 列表订阅拉取", "POST", "/api/webservice/iplists/ipl-office/refresh"},
		{"出站代理设置", "PUT", "/api/system/settings"},
		{"重启", "POST", "/api/system/restart"},
	}
	for _, c := range cases {
		var body io.Reader
		if c.method != "GET" {
			body = strings.NewReader("{}")
		}
		req, _ := http.NewRequest(c.method, srv.URL+c.path, body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := admin.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || !bytes.Contains(data, []byte(demo.Message)) {
			t.Errorf("%s：管理员 %s %s 应被演示模式拒绝，实际 %d %s", c.name, c.method, c.path, resp.StatusCode, data)
		}
	}
}

// 各执行点自带检查：不经过管理接口也不会执行脚本、命令、对外请求。
func TestDemoExecutionPoints(t *testing.T) {
	demo.Set(true)
	defer demo.Set(false)
	if err := demo.Check(); err != demo.ErrUnavailable {
		t.Fatalf("Check 应返回统一错误，实际 %v", err)
	}
	if !demo.Blocked("POST", "/api/anything") || demo.Blocked("GET", "/api/portforward/rules") {
		t.Fatal("写请求应被拒绝，普通读请求应放行")
	}
	demo.Set(false)
	if demo.Blocked("POST", "/api/anything") || demo.Check() != nil {
		t.Fatal("关闭演示模式后不应拦截")
	}
}

// 访客一键进入得到只读会话，写操作得到清楚的提示。
func TestDemoVisitorReadonly(t *testing.T) {
	_, srv := newDemoApp(t, 0)
	c := demoClient()
	if code, body := doJSON(t, c, "GET", srv.URL+"/api/demo", nil); code != 200 || !bytes.Contains(body, []byte(`"enabled":true`)) {
		t.Fatalf("演示信息异常 %d %s", code, body)
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login/demo", nil); code != 200 {
		t.Fatalf("一键进入失败 %d %s", code, body)
	}
	if code, body := doJSON(t, c, "GET", srv.URL+"/api/session", nil); code != 200 || !bytes.Contains(body, []byte(`"readonly"`)) {
		t.Fatalf("应为只读会话 %d %s", code, body)
	}
	// 演示数据有内容且运行状态为运行中
	code, body := doJSON(t, c, "GET", srv.URL+"/api/portforward/rules", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"running":true`)) {
		t.Fatalf("端口转发规则应显示运行中 %d %.200s", code, body)
	}
	code, body = doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", map[string]any{"name": "x"})
	if code != 403 || !bytes.Contains(body, []byte(demo.Message)) {
		t.Fatalf("访客写操作应得到演示模式提示 %d %s", code, body)
	}
	// 管理员账号存在，登录页之外的接口对其可用
	if code, _ := doJSON(t, demoAdmin(t, srv), "GET", srv.URL+"/api/status", nil); code != 200 {
		t.Fatalf("管理员应能读取状态 %d", code)
	}
}

// 非演示模式下没有一键进入入口。
func TestDemoLoginUnavailableOutsideDemo(t *testing.T) {
	_, srv, _ := loginApp(t)
	c := demoClient()
	if code, body := doJSON(t, c, "GET", srv.URL+"/api/demo", nil); code != 200 || !bytes.Contains(body, []byte(`"enabled":false`)) {
		t.Fatalf("非演示模式应返回 enabled=false: %d %s", code, body)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/login/demo", nil); code != 404 {
		t.Fatalf("非演示模式不应有一键进入，实际 %d", code)
	}
}

// 演示数据全部取自文档保留的地址与域名，不含真实 IP 与域名。
func TestDemoDataUsesReservedAddresses(t *testing.T) {
	dir := t.TempDir()
	if err := gen.Generate(gen.Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if bad := gen.FindUnreservedAddresses(dir); len(bad) > 0 {
		t.Fatalf("演示数据含非保留地址或域名: %v", bad)
	}
}

// 重启后配置目录恢复为启动时的快照。
func TestDemoSnapshotRestore(t *testing.T) {
	a, _ := newDemoApp(t, 0)
	snap := demo.SnapshotDir(a.opts.ConfigDir)
	if !demo.HasSnapshot(snap) {
		t.Fatal("初始化后应已保存快照")
	}
	f := filepath.Join(a.opts.ConfigDir, "cron.json")
	if err := writeFileForTest(f, "{}"); err != nil {
		t.Fatal(err)
	}
	if err := demo.Restore(a.opts.ConfigDir, snap); err != nil {
		t.Fatal(err)
	}
	if data := readFileForTest(f); strings.TrimSpace(data) == "{}" {
		t.Fatal("恢复后应回到快照中的内容")
	}
}

// 演示模式启动后本进程只监听管理端口：所有数据面服务都不启动，演示数据里启用的监听器、转发规则等不会真的监听。
func TestDemoListensOnlyAdminPort(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("需要 lsof 检查本进程监听的端口")
	}
	admin := freeTCPPort(t)
	base := 41000 + (time.Now().Nanosecond()/1000)%15000
	a, hs := newDemoApp(t, base)
	// 测试用的 httptest 服务器也会占用端口，先关掉，只留下 Run 启动的服务
	hs.Close()
	a.opts.Listen = "127.0.0.1:" + strconv.Itoa(admin)
	a.settings.Listen = a.opts.Listen
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	defer func() {
		cancel()
		<-done
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", a.opts.Listen, 200*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("管理端口未启动")
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	out, _ := exec.Command("lsof", "-nP", "-a", "-p", strconv.Itoa(pidOf()), "-iTCP", "-iUDP").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "LISTEN") && !strings.Contains(line, "UDP") {
			continue
		}
		if strings.Contains(line, ":"+strconv.Itoa(admin)) {
			continue
		}
		t.Errorf("演示模式下出现了管理端口以外的监听: %s", line)
	}
	if !strings.Contains(string(out), ":"+strconv.Itoa(admin)) {
		t.Fatalf("未在输出中找到管理端口，无法确认检查有效: %s", out)
	}
}
