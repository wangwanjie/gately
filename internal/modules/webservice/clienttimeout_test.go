package webservice

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// 访客超时：0 为默认 60 秒，超出范围或为负数时保存失败；断开连接以外的站点类型都可设置。
func TestClientTimeoutsValidate(t *testing.T) {
	var zero ClientTimeouts
	if zero.readIdle() != 60*time.Second || zero.writeIdle() != 60*time.Second {
		t.Fatalf("默认值不符: %v %v", zero.readIdle(), zero.writeIdle())
	}
	for _, v := range []ClientTimeouts{{}, {ReadIdle: 1, WriteIdle: 3600}} {
		s := Site{Domains: []string{"a.test"}, Type: TypeText, Text: "x", ClientTimeouts: v}
		if err := s.validate(false); err != nil || s.ClientTimeouts != v {
			t.Errorf("%+v 应可保存且不改写: %v %+v", v, err, s.ClientTimeouts)
		}
	}
	for _, v := range []ClientTimeouts{{ReadIdle: -1}, {ReadIdle: 3601}, {WriteIdle: -1}, {WriteIdle: 3601}} {
		s := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, ClientTimeouts: v}
		if err := s.validate(false); err == nil {
			t.Errorf("%+v 应保存失败", v)
		}
	}
}

// slowBackend 读完请求体后等待 wait 再回复请求体长度，模拟处理较慢的后端。
func slowBackend(t *testing.T, wait time.Duration) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		time.Sleep(wait)
		fmt.Fprintf(w, "got %d", n)
	}))
	t.Cleanup(b.Close)
	return b
}

// trickle 每隔 gap 写出一段，共 parts 段，模拟持续上传但较慢的访客。
func trickle(parts int, gap time.Duration) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		for i := 0; i < parts; i++ {
			time.Sleep(gap)
			if _, err := pw.Write([]byte("0123456789")); err != nil {
				return
			}
		}
		pw.Close()
	}()
	return pr
}

// h2Client 返回走 HTTP/2 的客户端。
func h2Client() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"},
		ForceAttemptHTTP2: true,
	}}
}

// 数据持续流动的慢速上传、读完请求体后后端处理较久、间隔较长的流式响应都不受空闲超时影响：
// 截止时间每次读写后清除，HTTP/1 读完请求体后的后台读取与 HTTP/2 的请求流定时器不会误伤请求。
func TestClientIdleKeepsLiveRequests(t *testing.T) {
	slow := slowBackend(t, 1500*time.Millisecond)
	sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: 1\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(1500 * time.Millisecond)
		_, _ = io.WriteString(w, "data: 2\n\n")
	}))
	t.Cleanup(sse.Close)
	ct := ClientTimeouts{ReadIdle: 1, WriteIdle: 1}
	l := Listener{TLS: true, HTTP3: true, Sites: []Site{
		{Enable: true, Domains: []string{"secure.test"}, Targets: []string{slow.URL}, ClientTimeouts: ct,
			Locations: []Location{{Prefix: "/events", Targets: []string{sse.URL}}}},
	}}
	addr, _ := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)

	clients := map[string]*http.Client{
		"h2": h2Client(),
		"h3": {Timeout: 20 * time.Second, Transport: &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"}}},
	}
	for name, c := range clients {
		req, _ := http.NewRequest("POST", "https://"+addr+"/up", trickle(4, 400*time.Millisecond))
		req.Host = "secure.test"
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("%s 慢速上传: %v", name, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(body) != "got 40" {
			t.Fatalf("%s 慢速上传与慢后端应成功: %d %s", name, resp.StatusCode, body)
		}
		req, _ = http.NewRequest("GET", "https://"+addr+"/events", nil)
		req.Host = "secure.test"
		resp, err = c.Do(req)
		if err != nil {
			t.Fatalf("%s 流式响应: %v", name, err)
		}
		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(body) != "data: 1\n\ndata: 2\n\n" {
			t.Fatalf("%s 流式响应应完整结束: %v %q", name, err, body)
		}
	}

	// HTTP/1：同样的慢速上传，读完请求体后连接上的后台读取不能因残留的截止时间取消请求
	plain, _ := startRunner(t, Listener{Sites: l.Sites}, nil, nil)
	req, _ := http.NewRequest("POST", "http://"+plain+"/up", trickle(4, 400*time.Millisecond))
	req.Host = "secure.test"
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "got 40" {
		t.Fatalf("HTTP/1 慢速上传与慢后端应成功: %d %s", resp.StatusCode, body)
	}
	resp, body2 := get(t, http.DefaultClient, plain, "secure.test", "/events", nil)
	if resp.StatusCode != 200 || body2 != "data: 1\n\ndata: 2\n\n" {
		t.Fatalf("HTTP/1 流式响应应完整结束: %q", body2)
	}
}

// 请求体发了一半就停住：HTTP/1 按读取请求体超时返回 408，不算后端故障；HTTP/2 同样在超时后结束请求。
func TestClientReadIdleTimeout(t *testing.T) {
	a := slowBackend(t, 0)
	b := slowBackend(t, 0)
	l := Listener{TLS: false, Sites: []Site{{Enable: true, Key: "s1", Domains: []string{"a.test"}, Targets: []string{a.URL, b.URL}, ClientTimeouts: ClientTimeouts{ReadIdle: 1}}}}
	addr, run := startRunner(t, l, nil, nil)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := time.Now()
	fmt.Fprintf(c, "POST /up HTTP/1.1\r\nHost: a.test\r\nContent-Length: 100\r\n\r\n0123456789")
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatalf("应收到响应: %v", err)
	}
	resp.Body.Close()
	took := time.Since(start)
	if resp.StatusCode != http.StatusRequestTimeout || took < 900*time.Millisecond || took > 3*time.Second {
		t.Fatalf("应在约 1 秒后返回 408: %d %v", resp.StatusCode, took)
	}
	for _, up := range siteOf(run).pool.ups {
		if up.passiveFails.Load() != 0 || up.down.Load() {
			t.Fatalf("访客超时不应计入后端被动检查: %s", up.url.Host)
		}
	}

	// HTTP/2：请求体停住后请求同样在超时后结束（返回 408 或重置请求流）
	tl := l
	tl.TLS, tl.Sites = true, []Site{l.Sites[0]}
	tl.Sites[0].Domains = []string{"secure.test"}
	addr2, _ := startRunner(t, tl, fakeCerts{selfSigned(t, "secure.test")}, nil)
	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		_, _ = pw.Write([]byte("0123456789"))
	}()
	req, _ := http.NewRequest("POST", "https://"+addr2+"/up", pr)
	req.Host = "secure.test"
	req.ContentLength = 100
	start = time.Now()
	resp, err = h2Client().Do(req)
	took = time.Since(start)
	if err == nil {
		resp.Body.Close()
	}
	if took > 4*time.Second || (err == nil && resp.StatusCode != http.StatusRequestTimeout) {
		t.Fatalf("HTTP/2 请求体停住后应在超时后结束: %v %v", err, took)
	}
}

// 访客停止读取响应：写出卡住超过设置的秒数后断开，不再无限占用连接；同样的停顿在超时以内则完整收到。
func TestClientWriteIdleTimeout(t *testing.T) {
	const size = 64 << 20
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(size))
		buf := make([]byte, 64<<10)
		for sent := 0; sent < size; sent += len(buf) {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	t.Cleanup(big.Close)
	read := func(writeIdle int) int64 {
		addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Targets: []string{big.URL}, ClientTimeouts: ClientTimeouts{WriteIdle: writeIdle}}}}, nil, nil)
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_ = c.(*net.TCPConn).SetReadBuffer(4096)
		fmt.Fprintf(c, "GET /big HTTP/1.1\r\nHost: a.test\r\n\r\n")
		// 先停住不读，让 Gately 的写出卡住
		time.Sleep(3 * time.Second)
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		n, _ := io.Copy(io.Discard, c)
		return n
	}
	if n := read(1); n >= size {
		t.Fatalf("写出卡住超过 1 秒应断开连接，实际收到 %d 字节", n)
	}
	if n := read(10); n < size {
		t.Fatalf("停顿未超过写出超时时应完整收到，实际 %d 字节", n)
	}
}

// 非反向代理站点同样套用访客超时：固定文本站点正常回复。
func TestClientIdleTextSite(t *testing.T) {
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok", ClientTimeouts: ClientTimeouts{ReadIdle: 1, WriteIdle: 1}}}}, nil, nil)
	resp, body := get(t, http.DefaultClient, addr, "a.test", "/", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "ok") {
		t.Fatalf("固定文本站点: %d %s", resp.StatusCode, body)
	}
}
