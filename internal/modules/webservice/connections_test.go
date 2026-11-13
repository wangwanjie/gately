package webservice

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// waitConn 轮询直到 cond 成立，超时则失败。
func waitConn(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", what)
}

// siteOf 返回监听上第一个站点的处理器。
func siteOf(r *listenerRunner) *siteHandler {
	return r.current().sites[0]
}

// activeOne 等待站点恰好有一个进行中的请求并返回它。
func activeOne(t *testing.T, h *siteHandler) ConnView {
	t.Helper()
	var v ConnsView
	waitConn(t, "出现一个进行中的请求", func() bool {
		v = h.conns.snapshot("", time.Now())
		return v.Total == 1
	})
	return v.Active[0]
}

// idOf 把接口返回的编号还原为数字。
func idOf(t *testing.T, v ConnView) uint64 {
	t.Helper()
	var id uint64
	if _, err := fmt.Sscan(v.ID, &id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestConnsBlockingRequestClose 验证等待后端响应的请求出现在实时列表中（只记录路径不记录查询串），
// 断开后访客收到连接错误、后端请求被取消、计数回落，历史记录为已断开，访问日志记为 444。
func TestConnsBlockingRequestClose(t *testing.T) {
	backendDone := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 一直等到请求被取消
		<-r.Context().Done()
		close(backendDone)
	}))
	defer backend.Close()
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	addr, run := startRunnerLogs(t, Listener{Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"app.test"}, Targets: []string{backend.URL}, AccessLog: true}}}, nil, nil, logs)
	h := siteOf(run)

	errc := make(chan error, 1)
	go func() {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			errc <- err
			return
		}
		defer c.Close()
		_, _ = io.WriteString(c, "GET /slow/path?token=SECRET HTTP/1.1\r\nHost: app.test\r\n\r\n")
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		b, err := io.ReadAll(c)
		if len(b) != 0 {
			err = fmt.Errorf("断开后不应收到响应: %q", b)
		}
		errc <- err
	}()

	v := activeOne(t, h)
	if v.Method != "GET" || v.Path != "/slow/path" || v.Host != "app.test" || v.Proto != "HTTP/1.1" || v.IP != "127.0.0.1" || v.Port == 0 {
		t.Fatalf("进行中请求信息错误: %+v", v)
	}
	if run.current().sites[0].stats.Active.Load() != 1 {
		t.Fatal("进行中计数应为 1")
	}
	if h.conns.closeConn(idOf(t, v)) != true {
		t.Fatal("断开失败")
	}
	if h.conns.closeConn(idOf(t, v)) {
		t.Fatal("重复断开应返回 false")
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	select {
	case <-backendDone:
	case <-time.After(5 * time.Second):
		t.Fatal("断开后后端请求应被取消")
	}
	waitConn(t, "计数回落", func() bool { return h.stats.Active.Load() == 0 && h.conns.snapshot("", time.Now()).Total == 0 })
	hist := h.conns.snapshot("", time.Now()).History
	if len(hist) != 1 || hist[0].Result != "closed" || hist[0].End == nil || hist[0].Path != "/slow/path" {
		t.Fatalf("历史记录错误: %+v", hist)
	}
	if h.conns.closed.Load() != 1 {
		t.Fatal("断开计数应为 1")
	}
	got := logs.Query(0, 0, 0, &logbuf.Filter{Host: "app.test", Status: []logbuf.StatusRange{{Min: 100, Max: 999}}})
	if len(got) != 1 || !strings.Contains(got[0].Message, " 444 ") {
		t.Fatalf("访问日志应记为 444: %+v", got)
	}
	// 管理员断开不算后端错误
	if h.stats.Errors.Load() != 0 {
		t.Fatalf("断开不应计入错误数: %d", h.stats.Errors.Load())
	}
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, "后端错误") {
			t.Fatalf("断开不应记录后端错误: %q", e.Message)
		}
	}
}

// TestConnsStreamingDownloadClose 验证持续下载的流量实时计入，断开后访客的读取中断，历史记录带流量。
func TestConnsStreamingDownloadClose(t *testing.T) {
	chunk := strings.Repeat("x", 4096)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for r.Context().Err() == nil {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
			http.NewResponseController(w).Flush()
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"dl.test"}, Targets: []string{backend.URL}}}}, nil, nil)
	h := siteOf(run)

	req, _ := http.NewRequest("GET", "http://"+addr+"/big.iso", nil)
	req.Host = "dl.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	readErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, resp.Body)
		readErr <- err
	}()
	v := activeOne(t, h)
	waitConn(t, "下行流量增长", func() bool {
		s := h.conns.snapshot("", time.Now())
		return len(s.Active) == 1 && s.Active[0].Down > 32*1024
	})
	h.conns.closeConn(idOf(t, v))
	select {
	case err := <-readErr:
		if err == nil {
			t.Fatal("断开后读取应出错（响应被截断）")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("断开后访客的读取没有结束")
	}
	waitConn(t, "历史记录", func() bool { return len(h.conns.snapshot("", time.Now()).History) == 1 })
	hist := h.conns.snapshot("", time.Now()).History[0]
	if hist.Result != "closed" || hist.Down < 32*1024 {
		t.Fatalf("历史记录错误: %+v", hist)
	}
}

// TestConnsStalledClientClose 验证访客停止读取、处理器阻塞在写出上时，断开仍能立即生效。
func TestConnsStalledClientClose(t *testing.T) {
	chunk := strings.Repeat("y", 64*1024)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for r.Context().Err() == nil {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"dl.test"}, Targets: []string{backend.URL}}}}, nil, nil)
	h := siteOf(run)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// 缩小接收缓冲并且不读取，让服务端很快阻塞在写出上
	_ = c.(*net.TCPConn).SetReadBuffer(4096)
	_, _ = io.WriteString(c, "GET /stall HTTP/1.1\r\nHost: dl.test\r\n\r\n")
	v := activeOne(t, h)
	// 等写出停滞（下行字节数在一段时间内不再增长）
	var last int64 = -1
	waitConn(t, "写出阻塞", func() bool {
		cur := h.conns.snapshot("", time.Now()).Active[0].Down
		stalled := cur == last && cur > 0
		last = cur
		time.Sleep(100 * time.Millisecond)
		return stalled
	})
	h.conns.closeConn(idOf(t, v))
	waitConn(t, "阻塞中的请求被断开", func() bool { return h.conns.snapshot("", time.Now()).Total == 0 })
}

// TestConnsWebSocket 验证已升级的 WebSocket 标记为 websocket 并计入双向流量，断开时访客与后端连接都被关闭。
func TestConnsWebSocket(t *testing.T) {
	backendClosed := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		for {
			line, err := brw.ReadString('\n')
			if err != nil {
				close(backendClosed)
				return
			}
			_, _ = brw.WriteString("echo:" + line)
			_ = brw.Flush()
		}
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"ws.test"}, Targets: []string{backend.URL}}}}, nil, nil)
	h := siteOf(run)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = io.WriteString(c, "GET /ws HTTP/1.1\r\nHost: ws.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("升级失败: %v %v", resp, err)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 3; i++ {
		_, _ = io.WriteString(c, "hello\n")
		if line, _ := br.ReadString('\n'); line != "echo:hello\n" {
			t.Fatalf("转发失败: %q", line)
		}
	}
	// 流量在写完之后才累加，访客读到最后一行时计数可能还没更新，等计数到位再检查
	v := activeOne(t, h)
	for deadline := time.Now().Add(3 * time.Second); (v.Up < 18 || v.Down < 33) && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		v = activeOne(t, h)
	}
	if !v.WebSocket || v.Up < 18 || v.Down < 33 {
		t.Fatalf("WebSocket 标记或流量错误: %+v", v)
	}
	h.conns.closeConn(idOf(t, v))
	if _, err := br.ReadString('\n'); err == nil {
		t.Fatal("断开后访客连接应被关闭")
	}
	select {
	case <-backendClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("断开后后端连接应被关闭")
	}
	waitConn(t, "计数回落", func() bool { return h.conns.snapshot("", time.Now()).Total == 0 && h.stats.Active.Load() == 0 })
	hist := h.conns.snapshot("", time.Now()).History
	if len(hist) != 1 || hist[0].Result != "closed" || !hist[0].WebSocket {
		t.Fatalf("历史记录错误: %+v", hist)
	}
}

// TestConnsHTTP2ResetsOnlyStream 验证 HTTP/2 下断开只重置该请求流，同一连接上的其他请求照常完成，连接不被关闭。
func TestConnsHTTP2ResetsOnlyStream(t *testing.T) {
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hold" {
			<-r.Context().Done()
			return
		}
		<-release
		_, _ = io.WriteString(w, "fine")
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{TLS: true, Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"secure.test"}, Targets: []string{backend.URL}}}},
		fakeCerts{selfSigned(t, "secure.test")}, nil)
	h := siteOf(run)
	var dials atomic.Int32
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"},
		ForceAttemptHTTP2: true,
		DialContext: func(ctx context.Context, network, a string) (net.Conn, error) {
			dials.Add(1)
			return (&net.Dialer{}).DialContext(ctx, network, a)
		},
	}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	do := func(path string) (*http.Response, error) {
		req, _ := http.NewRequest("GET", "https://"+addr+path, nil)
		req.Host = "secure.test"
		return c.Do(req)
	}
	holdErr := make(chan error, 1)
	go func() {
		resp, err := do("/hold")
		if err == nil {
			_, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		holdErr <- err
	}()
	var v ConnView
	select {
	case err := <-holdErr:
		t.Fatalf("请求提前结束: %v", err)
	case <-time.After(300 * time.Millisecond):
		v = activeOne(t, h)
	}
	if v.Proto != "HTTP/2.0" {
		t.Fatalf("应为 HTTP/2: %+v", v)
	}
	okc := make(chan string, 1)
	go func() {
		resp, err := do("/ok")
		if err != nil {
			okc <- err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		okc <- string(b)
	}()
	waitConn(t, "第二个请求进行中", func() bool { return h.conns.snapshot("", time.Now()).Total == 2 })
	h.conns.closeConn(idOf(t, v))
	if err := <-holdErr; err == nil {
		t.Fatal("被断开的请求应出错")
	}
	close(release)
	if got := <-okc; got != "fine" {
		t.Fatalf("同一连接上的其他请求应正常完成: %q", got)
	}
	if n := dials.Load(); n != 1 {
		t.Fatalf("HTTP/2 连接不应被关闭，共拨号 %d 次", n)
	}
}

// TestConnsHTTP3Close 验证 HTTP/3 请求同样可以列出并断开。
func TestConnsHTTP3Close(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{TLS: true, HTTP3: true, Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"secure.test"}, Targets: []string{backend.URL}}}},
		fakeCerts{selfSigned(t, "secure.test")}, nil)
	h := siteOf(run)
	tr := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"}}
	defer tr.Close()
	errc := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest("GET", "https://"+addr+"/h3", nil)
		req.Host = "secure.test"
		resp, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Do(req)
		if err == nil {
			_, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		errc <- err
	}()
	v := activeOne(t, h)
	if v.Proto != "HTTP/3.0" {
		t.Fatalf("应为 HTTP/3: %+v", v)
	}
	h.conns.closeConn(idOf(t, v))
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("被断开的 HTTP/3 请求应出错")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP/3 请求断开后没有结束")
	}
	waitConn(t, "计数回落", func() bool { return h.conns.snapshot("", time.Now()).Total == 0 })
}

// TestConnsUploadAndHistoryThreshold 验证上传字节计入上行；短请求不进历史，持续超过阈值的正常请求以 done 进入历史。
func TestConnsUploadAndHistoryThreshold(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/slow" {
			time.Sleep(connHistoryMinDuration + 100*time.Millisecond)
		}
		fmt.Fprintf(w, "%d", n)
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"up.test"}, Targets: []string{backend.URL}}}}, nil, nil)
	h := siteOf(run)
	post := func(path string, body string) string {
		req, _ := http.NewRequest("POST", "http://"+addr+path, strings.NewReader(body))
		req.Host = "up.test"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(b)
	}
	if got := post("/fast", "abc"); got != "3" {
		t.Fatalf("上传异常: %q", got)
	}
	if n := len(h.conns.snapshot("", time.Now()).History); n != 0 {
		t.Fatalf("短请求不应进入历史，得到 %d 条", n)
	}
	body := strings.Repeat("z", 10000)
	if got := post("/slow", body); got != "10000" {
		t.Fatalf("上传异常: %q", got)
	}
	waitConn(t, "历史记录", func() bool { return len(h.conns.snapshot("", time.Now()).History) == 1 })
	hist := h.conns.snapshot("", time.Now()).History[0]
	if hist.Result != "done" || hist.Method != "POST" || hist.Up != 10000 || hist.DurationMs < connHistoryMinDuration.Milliseconds() {
		t.Fatalf("历史记录错误: %+v", hist)
	}
}

// fakeReq 构造一个登记项，用于直接测试登记表。
func fakeReq(t *testing.T, tr *connTracker, ip string) *activeReq {
	t.Helper()
	r := httptest.NewRequest("GET", "/p?x=1", nil)
	_, a := tr.begin(httptest.NewRecorder(), r, netip.MustParseAddrPort(ip))
	return a
}

// TestConnTrackerHistoryRingAndSearch 验证历史记录有界、最新的在前，搜索同时过滤进行中与历史记录，
// 进行中列表按开始时间排序且有返回上限。
func TestConnTrackerHistoryRingAndSearch(t *testing.T) {
	tr := newConnTracker()
	for i := 0; i < connHistorySize+50; i++ {
		a := fakeReq(t, tr, fmt.Sprintf("10.0.%d.%d:1000", i/256, i%256))
		tr.closeConn(a.id)
		tr.end(a)
	}
	v := tr.snapshot("", time.Now())
	if len(v.History) != connHistorySize {
		t.Fatalf("历史应保留 %d 条，得到 %d", connHistorySize, len(v.History))
	}
	last := connHistorySize + 50 - 1
	if want := fmt.Sprintf("10.0.%d.%d", last/256, last%256); v.History[0].IP != want {
		t.Fatalf("最新的应排在最前: %s != %s", v.History[0].IP, want)
	}
	if v.Closed != int64(connHistorySize+50) {
		t.Fatalf("断开计数错误: %d", v.Closed)
	}
	if got := tr.snapshot("10.0.0.49", time.Now()).History; len(got) != 0 {
		t.Fatalf("被挤出的记录不应再出现: %+v", got)
	}
	if got := tr.snapshot("10.0.0.200", time.Now()).History; len(got) != 1 {
		t.Fatalf("搜索历史失败: %+v", got)
	}

	tr2 := newConnTracker()
	var keep []*activeReq
	for i := 0; i < maxConnList+20; i++ {
		keep = append(keep, fakeReq(t, tr2, fmt.Sprintf("192.168.%d.%d:2000", i/256, i%256)))
	}
	keep = append(keep, fakeReq(t, tr2, "[2001:db8::1]:443"))
	v = tr2.snapshot("", time.Now())
	if v.Total != maxConnList+21 || v.Matched != v.Total || len(v.Active) != maxConnList {
		t.Fatalf("返回上限错误: total=%d matched=%d len=%d", v.Total, v.Matched, len(v.Active))
	}
	if v.Active[0].IP != "192.168.0.0" || v.Active[0].Path != "/p" {
		t.Fatalf("最早开始的应排在最前，且不含查询串: %+v", v.Active[0])
	}
	if v = tr2.snapshot("2001:db8", time.Now()); v.Matched != 1 || len(v.Active) != 1 || v.Active[0].IP != "2001:db8::1" {
		t.Fatalf("搜索 IPv6 失败: %+v", v)
	}
	if n := tr2.closeWhere(func(a *activeReq) bool { return limitKeyAddr(a.client.Addr()) == "2001:db8::/64" }); n != 1 {
		t.Fatalf("按网段断开数量错误: %d", n)
	}
	for _, a := range keep {
		tr2.end(a)
	}
	if v = tr2.snapshot("", time.Now()); v.Total != 0 || len(v.History) != 1 {
		t.Fatalf("注销后应无进行中请求，仅被断开的进入历史: total=%d hist=%d", v.Total, len(v.History))
	}
}

// TestConnsInheritOnReload 验证修改站点配置（热更新）后，旧处理器上进行中的请求仍能列出并断开。
func TestConnsInheritOnReload(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer backend.Close()
	l := Listener{Key: "l1", Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"app.test"}, Targets: []string{backend.URL}}}}
	addr, run := startRunner(t, l, nil, nil)
	go func() {
		req, _ := http.NewRequest("GET", "http://"+addr+"/hold", nil)
		req.Host = "app.test"
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	old := siteOf(run)
	activeOne(t, old)
	l2 := run.cfg
	l2.Sites = []Site{{Enable: true, Key: "s1", Name: "renamed", Domains: []string{"app.test"}, Targets: []string{backend.URL}}}
	if err := run.reload(l2, nil); err != nil {
		t.Fatal(err)
	}
	h := siteOf(run)
	if h == old || h.site.Name != "renamed" {
		t.Fatal("站点应已热更新")
	}
	v := activeOne(t, h)
	if !h.conns.closeConn(idOf(t, v)) {
		t.Fatal("热更新后应能断开旧处理器上的请求")
	}
	waitConn(t, "计数回落", func() bool { return h.conns.snapshot("", time.Now()).Total == 0 })
}

// newConnsModule 启动一个带监听的模块，返回模块与监听地址。
func newConnsModule(t *testing.T, sites []Site, deflt Site) (*Module, string) {
	t.Helper()
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	l := Listener{Key: "l1", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: sites, Default: deflt}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Listeners = []Listener{l}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, fmt.Sprintf("127.0.0.1:%d", port)
}

// callJSON 调用模块接口处理函数并解析响应。
func callJSON(t *testing.T, h http.HandlerFunc, method, target string, body any, remote string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("Content-Type", "application/json")
	if remote != "" {
		req.RemoteAddr = remote
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// TestConnsAPI 验证接口：列表与搜索、默认站点、编号限定在站点内、参数校验、不能封禁自己，
// 断开并封禁时断开该地址在其他站点上的请求，之后的新请求被拒绝。
func TestConnsAPI(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer backend.Close()
	m, addr := newConnsModule(t, []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{backend.URL}},
		{Enable: true, Key: "b", Domains: []string{"b.test"}, Targets: []string{backend.URL}},
	}, Site{Enable: true, Targets: []string{backend.URL}})
	// 测试客户端都经回环连入，回环地址不允许封禁：把回环设为可信代理，用 X-Forwarded-For 带上访客地址
	m.realIP.set([]string{"127.0.0.0/8"}, nil, "")
	// 封禁列表为全局共享，结束时解除，避免影响其他测试
	t.Cleanup(func() { m.auth.unban("203.0.113.5") })

	var wg sync.WaitGroup
	hold := func(host string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", "http://"+addr+"/hold", nil)
			req.Host = host
			req.Header.Set("X-Forwarded-For", "203.0.113.5")
			c := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 10 * time.Second}
			if resp, err := c.Do(req); err == nil {
				resp.Body.Close()
			}
		}()
	}
	hold("a.test")
	hold("b.test")
	hold("unknown.test")
	m.mu.Lock()
	ha, hb, hd := m.findSiteLocked("l1", "a"), m.findSiteLocked("l1", "b"), m.findSiteLocked("l1", "default")
	m.mu.Unlock()
	va, vb := activeOne(t, ha), activeOne(t, hb)
	activeOne(t, hd)

	code, out := callJSON(t, m.handleConns, "GET", "/connections?listener=l1&site=default", nil, "")
	if code != 200 {
		t.Fatalf("默认站点列表失败: %d %v", code, out)
	}
	data := out
	if data["total"].(float64) != 1 || len(data["active"].([]any)) != 1 {
		t.Fatalf("默认站点应有 1 个进行中请求: %v", data)
	}
	if code, out = callJSON(t, m.handleConns, "GET", "/connections?listener=l1&site=a&q=10.9.9.9", nil, ""); code != 200 ||
		out["matched"].(float64) != 0 {
		t.Fatalf("搜索不匹配时应为空: %d %v", code, out)
	}
	if code, _ = callJSON(t, m.handleConns, "GET", "/connections?listener=l1&site=nope", nil, ""); code != 404 {
		t.Fatalf("不存在的站点应返回 404: %d", code)
	}

	// 用站点 a 的接口断开站点 b 的请求应找不到
	if code, _ = callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: vb.ID}, ""); code != 404 {
		t.Fatalf("编号应限定在站点内: %d", code)
	}
	for _, bad := range []closeConnRequest{
		{Listener: "l1", Site: "a", ID: "abc"},
		{Listener: "l1", Site: "a", ID: va.ID, BanSeconds: -2},
		{Listener: "l1", Site: "a", ID: va.ID, BanSeconds: 31 * 24 * 3600},
	} {
		if code, _ = callJSON(t, m.handleCloseConn, "POST", "/connections/close", bad, ""); code != 400 {
			t.Fatalf("参数错误应返回 400: %+v → %d", bad, code)
		}
	}
	// 管理员自己的地址与访客相同时拒绝封禁，也不断开
	if code, _ = callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: va.ID, BanSeconds: 60}, "203.0.113.5:5555"); code != 400 {
		t.Fatalf("不应允许封禁自己的地址: %d", code)
	}
	if ha.conns.snapshot("", time.Now()).Total != 1 {
		t.Fatal("封禁被拒绝时不应断开")
	}

	// 断开并永久封禁：三个站点上来自 203.0.113.5 的请求全部断开
	code, out = callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: va.ID, BanSeconds: -1}, "192.0.2.1:5555")
	if code != 200 {
		t.Fatalf("断开并封禁失败: %d %v", code, out)
	}
	if res := out; res["closed"].(float64) != 3 || res["banKey"] != "203.0.113.5" {
		t.Fatalf("应断开 3 个请求并封禁 203.0.113.5: %v", res)
	}
	wg.Wait()
	for _, h := range []*siteHandler{ha, hb, hd} {
		waitConn(t, "全部断开", func() bool { return h.conns.snapshot("", time.Now()).Total == 0 })
	}
	if !m.auth.banned(netip.MustParseAddr("203.0.113.5")) {
		t.Fatal("地址应已被封禁")
	}
	// 封禁表在测试间共享，永久封禁要解除，免得影响其他用例
	t.Cleanup(func() { m.auth.unban("203.0.113.5") })
	permanent := false
	for _, b := range m.auth.listBans() {
		if b.Key == "203.0.113.5" {
			permanent = b.Permanent
		}
	}
	if !permanent {
		t.Fatalf("banSeconds 为 -1 时应为永久封禁: %+v", m.auth.listBans())
	}
	if resp, _ := get(t, noRedirect(), addr, "a.test", "/", map[string]string{"X-Forwarded-For": "203.0.113.5"}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("封禁后新请求应被拒绝: %d", resp.StatusCode)
	}
	if code, _ = callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: va.ID}, ""); code != 404 {
		t.Fatalf("已结束的连接应返回 404: %d", code)
	}
}

// 断开并封禁拒绝回环地址与可信代理：手动封禁绕过入侵防护白名单，封掉它们会拦截一大批无关访客。
func TestConnsBanRefusesLoopbackAndProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer backend.Close()
	m, addr := newConnsModule(t, []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{backend.URL}},
	}, Site{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, _ := http.NewRequest("GET", "http://"+addr+"/hold", nil)
		req.Host = "a.test"
		c := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 10 * time.Second}
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	m.mu.Lock()
	h := m.findSiteLocked("l1", "a")
	m.mu.Unlock()
	v := activeOne(t, h)
	code, out := callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: v.ID, BanSeconds: 60}, "192.0.2.1:5555")
	if code != 400 || !strings.Contains(fmt.Sprint(out), "回环") {
		t.Fatalf("不应允许封禁回环地址: %d %v", code, out)
	}
	if m.auth.banned(netip.MustParseAddr("127.0.0.1")) || h.conns.snapshot("", time.Now()).Total != 1 {
		t.Fatal("拒绝封禁时不应封禁也不应断开")
	}
	// 只断开不封禁仍然可以
	if code, _ = callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: v.ID}, "192.0.2.1:5555"); code != 200 {
		t.Fatalf("断开回环地址的请求应成功: %d", code)
	}
	<-done

	m.realIP.set([]string{"198.51.100.0/24", "2001:db8:1::/48"}, nil, "")
	for a, want := range map[string]bool{
		"198.51.100.7":        true,
		"::ffff:198.51.100.7": true,
		"2001:db8:1:2::9":     true,
		// IPv6 按 /64 封禁：/64 与可信网段 2001:db8:2:3::/64 重叠
		"2001:db8:2:3::1": true,
		"203.0.113.9":     false,
		"2001:db8:9::1":   false,
		"::1":             true,
		"0.0.0.0":         true,
	} {
		if a == "2001:db8:2:3::1" {
			m.realIP.set([]string{"198.51.100.0/24", "2001:db8:1::/48", "2001:db8:2:3::8/126"}, nil, "")
		}
		if got := m.unbannableReason(netip.MustParseAddr(a)) != ""; got != want {
			t.Errorf("%s 拒绝封禁 = %v，应为 %v", a, got, want)
		}
		m.realIP.set([]string{"198.51.100.0/24", "2001:db8:1::/48"}, nil, "")
	}
}

// deadlineWriter 记录断开时对 ResponseWriter 设置截止时间的次数。
type deadlineWriter struct {
	httptest.ResponseRecorder
	n atomic.Int32
}

func (w *deadlineWriter) SetWriteDeadline(time.Time) error {
	w.n.Add(1)
	return nil
}

func (w *deadlineWriter) SetReadDeadline(time.Time) error {
	w.n.Add(1)
	return nil
}

// 请求结束后再断开不能触碰 ResponseWriter：HTTP/2 会把它放回池中复用，HTTP/1.x 的截止时间会落到同一连接的下一个请求上。
func TestConnCloseAfterEnd(t *testing.T) {
	tr := newConnTracker()
	w := &deadlineWriter{}
	_, a := tr.begin(w, httptest.NewRequest("GET", "/p", nil), netip.MustParseAddrPort("192.0.2.1:1234"))
	tr.end(a)
	if a.close() {
		t.Fatal("已结束的请求不应再断开")
	}
	if w.n.Load() != 0 {
		t.Fatal("已结束的请求不应设置截止时间")
	}

	w2 := &deadlineWriter{}
	_, a2 := tr.begin(w2, httptest.NewRequest("GET", "/p", nil), netip.MustParseAddrPort("192.0.2.1:1235"))
	if !a2.close() || w2.n.Load() != 2 {
		t.Fatalf("进行中的请求应设置读写截止时间: %d", w2.n.Load())
	}
	if a2.close() {
		t.Fatal("重复断开应返回 false")
	}
	tr.end(a2)

	// 断开与结束并发时不能出现数据竞争（go test -race）
	for i := 0; i < 200; i++ {
		_, a := tr.begin(&deadlineWriter{}, httptest.NewRequest("GET", "/p", nil), netip.MustParseAddrPort("192.0.2.1:1"))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			a.close()
		}()
		go func() {
			defer wg.Done()
			tr.end(a)
		}()
		wg.Wait()
	}
}

// BenchmarkConnTracker 衡量每个请求登记与注销的开销（含上下文取消、分片锁与一次分配）。
func BenchmarkConnTracker(b *testing.B) {
	tr := newConnTracker()
	r := httptest.NewRequest("GET", "/p", nil)
	w := httptest.NewRecorder()
	client := netip.MustParseAddrPort("192.0.2.1:1234")
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, a := tr.begin(w, r, client)
			tr.end(a)
		}
	})
}

// BenchmarkSiteServeHTTP 衡量文本站点完整处理一个请求的开销，用于对比登记前后的差异。
func BenchmarkSiteServeHTTP(b *testing.B) {
	h, err := newSiteHandler(Site{Enable: true, Key: "s", Domains: []string{"a.test"}, Type: TypeText, Text: "ok", StatusCode: 200},
		logbuf.New(10, log.New(io.Discard, "", 0)).Logger("b"), newAuthenticator("x"), nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r := httptest.NewRequest("GET", "http://a.test/", nil)
			r.RemoteAddr = "192.0.2.1:1234"
			h.ServeHTTP(httptest.NewRecorder(), r)
		}
	})
}

// 手动封禁接口：0 为 24 小时，-1 为永久，超过 30 天或小于 -1 拒绝。
func TestAddBanDurations(t *testing.T) {
	m, _ := newConnsModule(t, []Site{}, Site{})
	for _, bad := range []banRequest{
		{Key: "203.0.113.1", Seconds: -2},
		{Key: "203.0.113.1", Seconds: 31 * 24 * 3600},
		// 回环与未指定地址封禁后会拦截经本机转发进来的全部访客
		{Key: "127.0.0.1", Seconds: -1},
		{Key: "::1"},
		{Key: "0.0.0.0"},
		{Key: "::ffff:127.0.0.1"},
	} {
		if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", bad, ""); code != 400 {
			t.Fatalf("参数错误应返回 400: %+v → %d", bad, code)
		}
	}
	t.Cleanup(func() {
		for _, k := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"} {
			m.auth.unban(k)
		}
	})
	for _, req := range []banRequest{
		{Key: "203.0.113.1", Seconds: -1},
		{Key: "203.0.113.2"},
		{Key: "203.0.113.3", Seconds: 30 * 24 * 3600},
	} {
		if code, out := callJSON(t, m.handleAddBan, "POST", "/bans", req, ""); code != 200 {
			t.Fatalf("封禁失败: %+v → %d %v", req, code, out)
		}
	}
	got := map[string]BanInfo{}
	for _, b := range m.auth.listBans() {
		got[b.Key] = b
	}
	if !got["203.0.113.1"].Permanent {
		t.Fatalf("-1 应为永久封禁: %+v", got)
	}
	now := time.Now()
	if b := got["203.0.113.2"]; b.Permanent || b.Until.Sub(now) < 23*time.Hour || b.Until.Sub(now) > 25*time.Hour {
		t.Fatalf("0 应为 24 小时: %+v", b)
	}
	if b := got["203.0.113.3"]; b.Permanent || b.Until.Sub(now) < 29*24*time.Hour {
		t.Fatalf("30 天封禁时长不对: %+v", b)
	}
}

// 手动封禁接口同样拒绝可信代理（IPv6 按 /64 与代理网段有重叠也拒绝），其他地址照常封禁。
func TestAddBanRejectsTrustedProxy(t *testing.T) {
	m, _ := newConnsModule(t, []Site{}, Site{})
	m.realIP.set([]string{"198.51.100.0/24", "2001:db8:1::/48"}, nil, "")
	t.Cleanup(func() {
		m.realIP.set(nil, nil, "")
		m.auth.unban("203.0.113.9")
	})
	for _, key := range []string{"198.51.100.7", "2001:db8:1:2::5"} {
		if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: key, Seconds: -1}, ""); code != 400 {
			t.Fatalf("可信代理地址 %s 应拒绝封禁: %d", key, code)
		}
	}
	if code, out := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "203.0.113.9", Seconds: 3600}, ""); code != 200 {
		t.Fatalf("普通地址应能封禁: %d %v", code, out)
	}
}
