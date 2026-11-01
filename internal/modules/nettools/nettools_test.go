package nettools

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()
	s, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: s, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

func waitTask(t *testing.T, m *Module, id string) Task {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		c := m.snapshot(m.tasks[id])
		m.mu.Unlock()
		if c.Status != "running" {
			return c
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("任务超时")
	return Task{}
}

func TestParsePorts(t *testing.T) {
	got, err := ParsePorts("22, 80,8000-8002,80")
	if err != nil || len(got) != 5 || got[0] != 22 || got[4] != 8002 {
		t.Fatalf("解析结果不符: %v %v", got, err)
	}
	for _, bad := range []string{"", "0", "70000", "9-3", "a", "1-2000"} {
		if _, err := ParsePorts(bad); err == nil {
			t.Errorf("%q 应解析失败", bad)
		}
	}
}

func TestDenyList(t *testing.T) {
	m := newTestModule(t)
	if _, err := m.resolve(context.Background(), "169.254.169.254"); err == nil {
		t.Fatal("云元数据地址应默认被禁止")
	}
	d, err := buildDeny([]string{"10.1.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	m.deny = d
	if !m.denied(netip.MustParseAddr("10.1.2.3")) || m.denied(netip.MustParseAddr("10.2.0.1")) {
		t.Fatal("自定义禁止网段判断错误")
	}
	if _, err := buildDeny([]string{"nope"}); err == nil {
		t.Fatal("无效条目应报错")
	}
	for _, bad := range []string{"", "-oProxyCommand=x", "a b", "http://x/y"} {
		if _, err := m.resolve(context.Background(), bad); err == nil {
			t.Errorf("目标 %q 应被拒绝", bad)
		}
	}
}

func TestTCPToolTask(t *testing.T) {
	m := newTestModule(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	task, err := m.start(Request{Tool: ToolTCP, Target: "127.0.0.1", Ports: strconv.Itoa(port) + ",1"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitTask(t, m, task.ID)
	if done.Status != "done" {
		t.Fatalf("任务状态 %s: %s", done.Status, done.Error)
	}
	res, ok := done.Result.([]PortResult)
	if !ok || len(res) != 1 || res[0].Port != port {
		t.Fatalf("应只有一个开放端口: %+v", done.Result)
	}
}

func TestToolErrorsAndLimits(t *testing.T) {
	m := newTestModule(t)
	if _, err := m.start(Request{Tool: "rm"}); err == nil {
		t.Fatal("未知工具应被拒绝")
	}
	task, _ := m.start(Request{Tool: ToolTCP, Target: "169.254.169.254", Ports: "80"})
	if done := waitTask(t, m, task.ID); done.Status != "error" || !strings.Contains(done.Error, "禁止") {
		t.Fatalf("禁止的目标应报错: %+v", done)
	}
	// 并发上限
	block := make(chan struct{})
	m.tools["block"] = func(ctx context.Context, _ *Module, _ Request, _ output) error {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return nil
	}
	var ids []string
	for i := 0; i < maxRunning; i++ {
		tk, err := m.start(Request{Tool: "block"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tk.ID)
	}
	if _, err := m.start(Request{Tool: "block"}); err == nil {
		t.Fatal("超过并发上限应被拒绝")
	}
	// 取消
	m.mu.Lock()
	m.tasks[ids[0]].cancel()
	m.mu.Unlock()
	if done := waitTask(t, m, ids[0]); done.Status != "canceled" {
		t.Fatalf("取消后状态应为 canceled: %s", done.Status)
	}
	close(block)
}

func TestDNSTool(t *testing.T) {
	m := newTestModule(t)
	task, _ := m.start(Request{Tool: ToolDNS, Target: "localhost", Type: "A"})
	done := waitTask(t, m, task.ID)
	if done.Status != "done" || !strings.Contains(strings.Join(done.Lines, "\n"), "127.0.0.1") {
		t.Fatalf("localhost 应解析到 127.0.0.1: %+v", done)
	}
	task, _ = m.start(Request{Tool: ToolDNS, Target: "x", Type: "BOGUS"})
	if done := waitTask(t, m, task.ID); done.Status != "error" {
		t.Fatal("不支持的类型应报错")
	}
}

func TestSpeedTool(t *testing.T) {
	m := newTestModule(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 32<<10)
		for i := 0; i < 200; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	task, _ := m.start(Request{Tool: ToolSpeed, URL: srv.URL, Seconds: 1})
	done := waitTask(t, m, task.ID)
	if done.Status != "done" || !strings.Contains(done.Lines[len(done.Lines)-1], "Mbps") {
		t.Fatalf("测速应成功: %+v", done)
	}
	task, _ = m.start(Request{Tool: ToolSpeed, URL: "ftp://x"})
	if done := waitTask(t, m, task.ID); done.Status != "error" {
		t.Fatal("非 http 地址应报错")
	}
}

// 前端会提交整个表单，标题只能取当前工具使用的字段
func TestTaskTitle(t *testing.T) {
	cases := []struct {
		req  Request
		want string
	}{
		{Request{Tool: ToolSpeed, Target: "192.168.18.1"}, "测速 speed.cloudflare.com"},
		{Request{Tool: ToolSpeed, Target: "192.168.18.1", URL: "https://sgp-ping.vultr.com/vultr.com.1000MB.bin"}, "测速 sgp-ping.vultr.com"},
		{Request{Tool: ToolScan, Target: "192.168.18.1"}, "局域网扫描"},
		{Request{Tool: ToolScan, Target: "192.168.18.1", CIDR: "10.0.0.0/24"}, "局域网扫描 10.0.0.0/24"},
		{Request{Tool: ToolPing, Target: " 1.1.1.1 ", CIDR: "10.0.0.0/24"}, "Ping 1.1.1.1"},
		{Request{Tool: ToolDNS}, "DNS 查询"},
	}
	for _, c := range cases {
		if got := title(c.req); got != c.want {
			t.Errorf("title(%+v) = %q，期望 %q", c.req, got, c.want)
		}
	}
}

// Cloudflare 的 __down 在 bytes 达到 1 亿时返回 403，默认地址必须低于该值
func TestDefaultSpeedURLBytes(t *testing.T) {
	u, err := url.Parse(defaultSpeedURL)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(u.Query().Get("bytes"))
	if err != nil || n <= 0 || n >= 100_000_000 {
		t.Fatalf("默认测速地址 bytes 应在 (0, 1e8) 内: %s", defaultSpeedURL)
	}
}

func TestPingLoopback(t *testing.T) {
	if _, _, err := listenICMP(netip.MustParseAddr("127.0.0.1")); err != nil {
		t.Skipf("当前环境无法创建 ICMP 套接字: %v", err)
	}
	m := newTestModule(t)
	task, _ := m.start(Request{Tool: ToolPing, Target: "127.0.0.1", Count: 2})
	done := waitTask(t, m, task.ID)
	if done.Status != "done" || !strings.Contains(strings.Join(done.Lines, "\n"), "丢包率 0%") {
		t.Fatalf("回环 ping 应成功: %+v", done)
	}
}

func TestScanPrefix(t *testing.T) {
	for _, bad := range []string{"8.8.8.0/24", "127.0.0.0/24", "nope", "fd00::/64"} {
		if _, err := scanPrefix(bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
	p, err := scanPrefix("192.168.1.77/24")
	if err != nil || p.String() != "192.168.1.0/24" {
		t.Fatalf("应规范化为网络地址: %v %v", p, err)
	}
	m := newTestModule(t)
	task, _ := m.start(Request{Tool: ToolScan, CIDR: "10.0.0.0/8"})
	if done := waitTask(t, m, task.ID); done.Status != "error" || !strings.Contains(done.Error, "网段过大") {
		t.Fatalf("过大网段应被拒绝: %+v", done)
	}
}

func TestNormalizeMAC(t *testing.T) {
	for in, want := range map[string]string{"AA:BB:CC:DD:EE:FF": "aa:bb:cc:dd:ee:ff", "a:b:c:d:e:f": "0a:0b:0c:0d:0e:0f", "aa-bb-cc-dd-ee-ff": "aa:bb:cc:dd:ee:ff"} {
		if got := normalizeMAC(in); got != want {
			t.Errorf("normalizeMAC(%q)=%q want %q", in, got, want)
		}
	}
	m := arpLine.FindStringSubmatch("? (192.168.1.1) at a4:b:c:d:e:f on en0 ifscope [ethernet]")
	if m == nil || m[1] != "192.168.1.1" || m[2] != "a4:b:c:d:e:f" {
		t.Fatalf("arp 行解析失败: %v", m)
	}
}
