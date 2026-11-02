package waf

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// fakeDetector 是模拟的雷池检测引擎：请求头或请求体含 "attack"（或测试连接用的示例 SQL 注入）时拦截，其余放行。
type fakeDetector struct {
	ln    net.Listener
	mu    sync.Mutex
	reqs  []map[byte][]byte
	conns atomic.Int64
	// delay 为回包前的等待时间，用于测试超时
	delay atomic.Int64
	// closeAfter 为 true 时每次回包后关闭连接，用于测试复用失效连接的重试
	closeAfter atomic.Bool
}

func newFakeDetector(t *testing.T) *fakeDetector {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDetector{ln: ln}
	go d.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return d
}

func (d *fakeDetector) addr() string { return d.ln.Addr().String() }

func (d *fakeDetector) serve() {
	for {
		c, err := d.ln.Accept()
		if err != nil {
			return
		}
		d.conns.Add(1)
		go d.handle(c)
	}
}

func (d *fakeDetector) handle(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	for {
		secs := map[byte][]byte{}
		var hdr [5]byte
		for {
			if _, err := io.ReadFull(br, hdr[:]); err != nil {
				return
			}
			data := make([]byte, binary.LittleEndian.Uint32(hdr[1:]))
			if _, err := io.ReadFull(br, data); err != nil {
				return
			}
			secs[hdr[0]&t1kMaskTag] = data
			if hdr[0]&t1kMaskLast != 0 {
				break
			}
		}
		if ms := d.delay.Load(); ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		reply := []t1kSection{{tag: t1kTagHeader, data: []byte{t1kActionPass}}}
		if _, ok := secs[t1kTagHeader]; ok {
			d.mu.Lock()
			d.reqs = append(d.reqs, secs)
			d.mu.Unlock()
			if bytes.Contains(secs[t1kTagHeader], []byte("attack")) || bytes.Contains(secs[t1kTagHeader], []byte("UNION%20SELECT")) ||
				bytes.Contains(secs[t1kTagBody], []byte("attack")) {
				reply = []t1kSection{
					{tag: t1kTagHeader, data: []byte{t1kActionBlock}},
					{tag: t1kTagBody, data: []byte("403")},
					{tag: t1kTagExtraHeader, data: []byte("Set-Cookie:sl-session=abc; Path=/\n")},
					{tag: t1kTagExtraBody, data: []byte("<!-- event_id: 05c55167-dfc0-4564-9990-9d2b6aeafce7 -->")},
				}
			}
		}
		var out bytes.Buffer
		_ = writeT1K(&out, reply)
		if _, err := c.Write(out.Bytes()); err != nil {
			return
		}
		if d.closeAfter.Load() {
			return
		}
	}
}

func (d *fakeDetector) last() map[byte][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.reqs) == 0 {
		return nil
	}
	return d.reqs[len(d.reqs)-1]
}

func safeLineInstance(addr string) Instance {
	return Instance{Key: "sl", Name: "sl", Enable: true, SafeLine: SafeLine{Enable: true, Addr: addr}}
}

func TestSafeLineBlocksAndPasses(t *testing.T) {
	d := newFakeDetector(t)
	m := newModule(t, safeLineInstance(d.addr()))
	var got []events.Event
	var mu sync.Mutex
	stop := events.Subscribe(func(e events.Event) {
		if e.Type == events.WAFBlocked {
			mu.Lock()
			got = append(got, e)
			mu.Unlock()
		}
	})
	defer stop()
	h := m.Middleware("sl", ok)

	// 正常请求放行，并带上来源地址、域名等信息
	req := httptest.NewRequest("GET", "http://a.example.com/index.html?page=2", nil)
	req.RemoteAddr = "203.0.113.7:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("正常请求应放行: %d %q", rec.Code, rec.Body.String())
	}
	sent := d.last()
	if !strings.HasPrefix(string(sent[t1kTagHeader]), "GET /index.html?page=2 HTTP/1.1\r\nHost: a.example.com\r\n") {
		t.Fatalf("送检的请求行与 Host 错误: %q", sent[t1kTagHeader])
	}
	extra := string(sent[t1kTagExtra])
	for _, want := range []string{t1kExtraRemoteAddr + "203.0.113.7\n", t1kExtraRemotePort + "4000\n", t1kExtraServerName + "a.example.com\n", t1kExtraScheme + "http\n"} {
		if !strings.Contains(extra, want) {
			t.Fatalf("Extra 段缺少 %q: %q", want, extra)
		}
	}
	if string(sent[t1kTagVersion]) != t1kProto {
		t.Fatalf("协议版本错误: %q", sent[t1kTagVersion])
	}
	if _, ok := sent[t1kTagBody]; ok {
		t.Fatal("没有请求体时不应发送 Body 段")
	}

	// 攻击请求：浏览器看到拦截页并带事件 ID，附加响应头照样写出
	req = httptest.NewRequest("GET", "http://a.example.com/?q=attack", nil)
	req.RemoteAddr = "203.0.113.7:4000"
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "05c55167-dfc0-4564-9990-9d2b6aeafce7") {
		t.Fatalf("攻击请求应被拦截并显示事件 ID: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Set-Cookie") != "sl-session=abc; Path=/" {
		t.Fatalf("应写出检测引擎要求的附加响应头: %v", rec.Header())
	}
	// 非浏览器客户端得到 JSON
	req = httptest.NewRequest("POST", "http://a.example.com/api", strings.NewReader(`{"q":"attack"}`))
	req.RemoteAddr = "203.0.113.8:4000"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	if rec.Code != 403 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body["event_id"] != "05c55167-dfc0-4564-9990-9d2b6aeafce7" {
		t.Fatalf("请求体中的攻击应被拦截并返回 JSON: %d %q", rec.Code, rec.Body.String())
	}

	v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView)
	if v.Blocked != 2 || v.Matched != 2 || v.SafeLineError != "" {
		t.Fatalf("计数错误: %+v", v)
	}
	logs := m.logs.Since(0)
	if len(logs) == 0 || logs[len(logs)-1].Fields[logbuf.FieldRule] != "05c55167-dfc0-4564-9990-9d2b6aeafce7" || logs[len(logs)-1].Fields[logbuf.FieldIP] != "203.0.113.8" {
		t.Fatalf("拦截日志应带事件 ID 与来源: %+v", logs)
	}
	// 事件异步投递，稍等片刻
	for i := 0; i < 100; i++ {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0].Fields["ip"] != "203.0.113.7:4000" {
		t.Fatalf("拦截应发布 WAF 事件供入侵防护计数: %+v", got)
	}
	// 连接复用：多次检测只建一条连接
	if n := d.conns.Load(); n != 1 {
		t.Fatalf("连接应复用，实际建立 %d 条", n)
	}
}

func TestSafeLineDetectMode(t *testing.T) {
	d := newFakeDetector(t)
	in := safeLineInstance(d.addr())
	in.Mode = ModeDetect
	m := newModule(t, in)
	if c := status(t, m.Middleware("sl", ok), "/?q=attack"); c != 200 {
		t.Fatalf("仅检测模式应放行: %d", c)
	}
	v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView)
	if v.Blocked != 0 || v.Matched != 1 {
		t.Fatalf("仅检测模式只计命中: %+v", v)
	}
}

func TestSafeLineBodyRestored(t *testing.T) {
	d := newFakeDetector(t)
	in := safeLineInstance(d.addr())
	in.SafeLine.BodyLimitKB = 1
	m := newModule(t, in)
	var seen []byte
	h := m.Middleware("sl", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = io.ReadAll(r.Body)
	}))
	payload := strings.Repeat("a", 3000)
	req := httptest.NewRequest("POST", "/upload", strings.NewReader(payload))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if string(seen) != payload {
		t.Fatalf("后端收到的请求体应完整: %d 字节", len(seen))
	}
	sent := d.last()
	if len(sent[t1kTagBody]) != 1024 {
		t.Fatalf("只应送检前 1 KB: %d", len(sent[t1kTagBody]))
	}
	if !strings.Contains(string(sent[t1kTagHeader]), "Content-Length: 3000\r\n") {
		t.Fatalf("应补回 Content-Length: %q", sent[t1kTagHeader])
	}
	// 超过上限部分的攻击内容不送检
	req = httptest.NewRequest("POST", "/upload", strings.NewReader(payload+"attack"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("超过上限的部分不送检: %d", rec.Code)
	}
}

func TestSafeLineSkipBody(t *testing.T) {
	d := newFakeDetector(t)
	in := safeLineInstance(d.addr())
	in.SkipBodyPaths = "/api/upload"
	in.WebDAV = true
	m := newModule(t, in)
	h := m.Middleware("sl", ok)
	cases := []struct {
		method, path, ct string
	}{
		{"POST", "/api/upload/x", "text/plain"},
		{"PUT", "/dav/file.txt", "text/plain"},
		{"POST", "/other", "application/octet-stream"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader("attack"))
		req.Header.Set("Content-Type", c.ct)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("%s %s 不应送检请求体: %d", c.method, c.path, rec.Code)
		}
	}
	// 请求头仍然送检
	if c := status(t, h, "/api/upload/attack"); c != 403 {
		t.Fatalf("跳过请求体时 URL 仍应检查: %d", c)
	}
}

func TestSafeLineStreamingBodyNotRead(t *testing.T) {
	d := newFakeDetector(t)
	m := newModule(t, safeLineInstance(d.addr()))
	// 长度未知的 gRPC 请求体不能先读满再转发：用一直阻塞的请求体验证没有读取
	pr, pw := io.Pipe()
	defer pw.Close()
	req := httptest.NewRequest("POST", "/svc.Echo/Stream", pr)
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/grpc")
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		m.Middleware("sl", ok).ServeHTTP(rec, req)
		done <- rec.Code
	}()
	select {
	case c := <-done:
		if c != 200 {
			t.Fatalf("gRPC 请求应放行: %d", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("长度未知的 gRPC 请求体不应被读取")
	}
}

func TestSafeLineFailOpenAndClosed(t *testing.T) {
	// 找一个没有监听的端口
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()

	in := safeLineInstance(addr)
	m := newModule(t, in)
	if c := status(t, m.Middleware("sl", ok), "/?q=attack"); c != 200 {
		t.Fatalf("默认在检测引擎不可用时放行: %d", c)
	}
	v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView)
	if v.SafeLineError == "" || v.SafeLineErrors != 1 {
		t.Fatalf("应记录检测失败: %+v", v)
	}

	in.SafeLine.FailClosed = true
	m = newModule(t, in)
	if c := status(t, m.Middleware("sl", ok), "/"); c != 503 {
		t.Fatalf("设置为拒绝时应返回 503: %d", c)
	}
}

func TestSafeLineTimeout(t *testing.T) {
	d := newFakeDetector(t)
	d.delay.Store(500)
	in := safeLineInstance(d.addr())
	in.SafeLine.TimeoutMs = 100
	m := newModule(t, in)
	start := time.Now()
	if c := status(t, m.Middleware("sl", ok), "/"); c != 200 {
		t.Fatalf("超时后应放行: %d", c)
	}
	if el := time.Since(start); el > 400*time.Millisecond {
		t.Fatalf("应在超时后立即返回，实际 %v", el)
	}
	// 检测引擎恢复后，失败状态随成功检测清除
	d.delay.Store(0)
	status(t, m.Middleware("sl", ok), "/")
	if v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView); v.SafeLineError != "" {
		t.Fatalf("恢复后应清除失败状态: %+v", v)
	}
}

func TestSafeLineRetryStaleConn(t *testing.T) {
	d := newFakeDetector(t)
	d.closeAfter.Store(true)
	m := newModule(t, safeLineInstance(d.addr()))
	h := m.Middleware("sl", ok)
	for i := 0; i < 3; i++ {
		// 上一次的连接已被检测引擎关闭，复用失败后应换新连接重试，检测照常进行
		if c := status(t, h, "/?q=attack"); c != 403 {
			t.Fatalf("第 %d 次: 复用失效连接后应重试: %d", i+1, c)
		}
	}
	if v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView); v.SafeLineErrors != 0 {
		t.Fatalf("重试成功不应计为失败: %+v", v)
	}
}

func TestSafeLineWithRuleSets(t *testing.T) {
	d := newFakeDetector(t)
	in := safeLineInstance(d.addr())
	in.RuleSets = []RuleSet{{Enable: true, Type: RuleCustom, Param: `SecRule ARGS:x "@streq bad" "id:10001,phase:1,deny,status:403"`}}
	m := newModule(t, in)
	h := m.Middleware("sl", ok)
	if c := status(t, h, "/?x=bad"); c != 403 {
		t.Fatalf("规则组仍应生效: %d", c)
	}
	if c := status(t, h, "/?q=attack"); c != 403 {
		t.Fatalf("雷池应先拦截: %d", c)
	}
	if c := status(t, h, "/?x=good"); c != 200 {
		t.Fatalf("正常请求应放行: %d", c)
	}
}

func TestSafeLineReloadClosesConns(t *testing.T) {
	d := newFakeDetector(t)
	m := newModule(t, safeLineInstance(d.addr()))
	status(t, m.Middleware("sl", ok), "/")
	v, _ := m.state.Load("sl")
	old := v.(*compiled).safe.client
	m.buildLocked("sl")
	old.mu.Lock()
	defer old.mu.Unlock()
	if !old.closed || len(old.idle) != 0 {
		t.Fatal("重建实例后应关闭旧连接池")
	}
}

func TestSafeLineValidate(t *testing.T) {
	bad := []SafeLine{
		{Enable: true},
		{Enable: true, Addr: "172.22.222.5"},
		{Enable: true, Addr: "172.22.222.5:0"},
		{Enable: true, Addr: "unix:relative.sock"},
		{Enable: true, Addr: "127.0.0.1:8000", TimeoutMs: 5},
		{Enable: true, Addr: "127.0.0.1:8000", BodyLimitKB: -1},
	}
	for _, s := range bad {
		in := Instance{Name: "x", SafeLine: s}
		if err := in.Validate(); err == nil {
			t.Errorf("应拒绝 %+v", s)
		}
	}
	in := Instance{Name: "x", SafeLine: SafeLine{Enable: true, Addr: " unix:/data/safeline/resources/detector/snserver.sock "}}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	if in.SafeLine.Addr != "unix:/data/safeline/resources/detector/snserver.sock" || in.SafeLine.TimeoutMs != 1000 || in.SafeLine.BodyLimitKB != 1024 {
		t.Fatalf("应补全默认值: %+v", in.SafeLine)
	}
	// 未启用且未填地址时允许保存
	in = Instance{Name: "x"}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeLineUnixSocket(t *testing.T) {
	sock := t.TempDir() + "/sn.sock"
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip(err)
	}
	d := &fakeDetector{ln: ln}
	go d.serve()
	defer ln.Close()
	m := newModule(t, safeLineInstance("unix:"+sock))
	if c := status(t, m.Middleware("sl", ok), "/?q=attack"); c != 403 {
		t.Fatalf("Unix 套接字应可用: %d", c)
	}
}

func TestSafeLineTestAPI(t *testing.T) {
	d := newFakeDetector(t)
	res, err := TestSafeLine(SafeLine{Addr: d.addr()})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked || res.LatencyMs <= 0 || res.EventID != "05c55167-dfc0-4564-9990-9d2b6aeafce7" {
		t.Fatalf("结果错误: %+v", res)
	}
	if _, err := TestSafeLine(SafeLine{Addr: "bad"}); err == nil {
		t.Fatal("地址无效时应报错")
	}
}

func TestT1KReplyLimits(t *testing.T) {
	// 首段缺少起始标志
	var b bytes.Buffer
	b.Write([]byte{t1kTagHeader | t1kMaskLast, 1, 0, 0, 0, '.'})
	if _, err := readT1K(&b); err == nil {
		t.Fatal("首段缺少起始标志应报错")
	}
	// 超大的段
	b.Reset()
	b.Write([]byte{t1kTagHeader | t1kMaskFirst | t1kMaskLast, 0xff, 0xff, 0xff, 0x7f})
	if _, err := readT1K(&b); err == nil {
		t.Fatal("超大的段应报错")
	}
	r := &t1kResult{extraBody: []byte("<!-- event_id: e1impksyjq0gl92le6odi0fnobi270cj TYPE: A -->")}
	if r.eventID() != "e1impksyjq0gl92le6odi0fnobi270cj" {
		t.Fatalf("旧版事件 ID 解析错误: %q", r.eventID())
	}
}
