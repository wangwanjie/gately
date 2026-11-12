package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// shrinkBodyLimits 调小请求体读取限制，结束后恢复。
func shrinkBodyLimits(t *testing.T) {
	t.Helper()
	oi, ot, bi, bt, bg, br, bd := bodyIdleTimeout, bodyTotalTimeout, bulkIdleTimeout, bulkTotalTimeout, bulkRateGrace, bulkMinRate, bodyDrainTimeout
	bodyIdleTimeout, bodyTotalTimeout = 300*time.Millisecond, 1500*time.Millisecond
	bulkIdleTimeout, bulkTotalTimeout, bulkRateGrace, bulkMinRate = 600*time.Millisecond, 5*time.Second, 500*time.Millisecond, 1000
	bodyDrainTimeout = 300 * time.Millisecond
	t.Cleanup(func() {
		bodyIdleTimeout, bodyTotalTimeout, bulkIdleTimeout, bulkTotalTimeout, bulkRateGrace, bulkMinRate, bodyDrainTimeout = oi, ot, bi, bt, bg, br, bd
	})
}

// readAllHandler 读完整个请求体，读取失败返回 400，成功返回读到的字节数。
func readAllHandler(w http.ResponseWriter, r *http.Request) {
	n, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "read %d", n)
}

// slowRequest 建立连接并发送请求头与前 first 字节的请求体，随后停止发送；返回连接与读取响应的函数。
func slowRequest(t *testing.T, srv *httptest.Server, path string, length int, first string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", path, length, first)
	return conn, bufio.NewReader(conn)
}

// waitResponse 等待服务器在 limit 内给出响应或断开连接；返回耗时与响应状态行（断开时为空）。
func waitResponse(conn net.Conn, br *bufio.Reader, limit time.Duration) (time.Duration, string, bool) {
	start := time.Now()
	_ = conn.SetReadDeadline(start.Add(limit))
	line, err := br.ReadString('\n')
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return time.Since(start), "", false
		}
		return time.Since(start), "", true
	}
	return time.Since(start), strings.TrimSpace(line), true
}

// 慢速客户端发出请求头和一小部分请求体后停止发送：不加限制时服务器一直等待，加限制后很快被断开。
func TestSlowBodyRejected(t *testing.T) {
	shrinkBodyLimits(t)

	// 对照：没有限制时，处理函数一直阻塞在读取请求体上
	bare := httptest.NewServer(http.HandlerFunc(readAllHandler))
	defer bare.Close()
	conn, br := slowRequest(t, bare, "/api/login", 1000, `{"a":`)
	if _, _, done := waitResponse(conn, br, 2*time.Second); done {
		t.Fatal("对照组：没有限制时服务器不应主动结束慢速请求")
	}
	// 先断开客户端，让阻塞在读取上的处理函数返回，否则 bare.Close 会一直等待
	conn.Close()

	guarded := httptest.NewServer(limitBodyRead(http.HandlerFunc(readAllHandler)))
	defer guarded.Close()
	conn, br = slowRequest(t, guarded, "/api/login", 1000, `{"a":`)
	elapsed, status, done := waitResponse(conn, br, 2*time.Second)
	if !done {
		t.Fatal("加限制后慢速请求应被结束")
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("应在读取间隔内结束，实际 %v", elapsed)
	}
	if status != "" && !strings.Contains(status, "400") {
		t.Fatalf("响应异常: %q", status)
	}
}

// 以“每个间隔发一个字节”的方式拖慢请求体：普通接口受总时长限制，大文件接口受最低速率限制。
func TestDripBodyRejected(t *testing.T) {
	shrinkBodyLimits(t)
	srv := httptest.NewServer(limitBodyRead(http.HandlerFunc(readAllHandler)))
	defer srv.Close()

	drip := func(path string, interval, limit time.Duration) (time.Duration, bool) {
		conn, br := slowRequest(t, srv, path, 100000, "")
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			for {
				select {
				case <-stop:
					return
				case <-time.After(interval):
					if _, err := conn.Write([]byte("x")); err != nil {
						return
					}
				}
			}
		}()
		elapsed, _, done := waitResponse(conn, br, limit)
		return elapsed, done
	}

	// 普通接口：间隔小于读取间隔也不行，总时长到点即结束
	elapsed, done := drip("/api/ddns/tasks", 100*time.Millisecond, 4*time.Second)
	if !done || elapsed > 2500*time.Millisecond {
		t.Fatalf("普通接口的滴灌请求应在总时长内被结束: done=%v elapsed=%v", done, elapsed)
	}
	// 大文件接口：宽限期之后平均速率低于下限即结束，早于总时长上限
	elapsed, done = drip("/api/system/restore", 100*time.Millisecond, 4*time.Second)
	if !done || elapsed > 3*time.Second {
		t.Fatalf("大文件接口的低速请求应在最低速率检查下被结束: done=%v elapsed=%v", done, elapsed)
	}
}

// 正常速度的上传不受影响：大文件接口的耗时超过普通接口的总时长上限也能完成。
func TestBulkUploadAllowed(t *testing.T) {
	shrinkBodyLimits(t)
	srv := httptest.NewServer(limitBodyRead(http.HandlerFunc(readAllHandler)))
	defer srv.Close()

	const chunks, chunkSize = 20, 4096
	conn, br := slowRequest(t, srv, "/api/system/upgrade/upload", chunks*chunkSize, "")
	chunk := bytes.Repeat([]byte("y"), chunkSize)
	start := time.Now()
	for i := 0; i < chunks; i++ {
		if _, err := conn.Write(chunk); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, status, done := waitResponse(conn, br, 3*time.Second)
	if !done || !strings.Contains(status, "200") {
		t.Fatalf("正常速度的大文件上传应成功: %q", status)
	}
	if time.Since(start) < bodyTotalTimeout {
		t.Fatalf("测试应覆盖超过普通接口总时长的上传，实际 %v", time.Since(start))
	}

	// 同样的速度发给普通接口会被总时长限制中断
	conn, br = slowRequest(t, srv, "/api/ddns/tasks", chunks*chunkSize, "")
	go func() {
		for i := 0; i < chunks; i++ {
			if _, err := conn.Write(chunk); err != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	_, status, done = waitResponse(conn, br, 4*time.Second)
	if !done || strings.Contains(status, "200") {
		t.Fatalf("普通接口的超长请求体应被中断: %q", status)
	}
}

// 没有请求体的请求与读完请求体之后的长时间响应不受影响（流式响应不会被取消）。
func TestStreamingResponseUnaffected(t *testing.T) {
	shrinkBodyLimits(t)
	srv := httptest.NewServer(limitBodyRead(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		// 响应时长远超读取间隔，期间请求上下文不应被取消
		select {
		case <-time.After(time.Second):
			_, _ = io.WriteString(w, "done")
		case <-r.Context().Done():
			http.Error(w, "canceled", http.StatusInternalServerError)
		}
	})))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ddns/tasks", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "done" {
		t.Fatalf("读完请求体后的长响应被打断: %q", body)
	}
	resp, err = http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "done" {
		t.Fatalf("无请求体的请求被打断: %q", body)
	}
}

func TestIsBulkUpload(t *testing.T) {
	yes := []string{
		"POST /api/system/restore", "POST /api/system/upgrade/upload", "POST /api/import/lucky",
		"POST /api/docker/images/load", "POST /api/storage/files/k1/upload", "POST /api/webterminal/sftp/k1/upload",
		"POST /api/webterminal/sftp/k1/write", "POST /api/docker/containers/abc/files/write", "POST /api/docker/volumes/v/import",
		"POST /api/webservice/geoip/upload",
	}
	no := []string{"POST /api/login", "POST /api/ddns/tasks", "PUT /api/system/settings", "GET /api/system/restore", "POST /api/account/totp/enable"}
	for _, s := range yes {
		method, path, _ := strings.Cut(s, " ")
		if !isBulkUpload(httptest.NewRequest(method, path, nil)) {
			t.Errorf("%s 应归为大文件接口", s)
		}
	}
	for _, s := range no {
		method, path, _ := strings.Cut(s, " ")
		if isBulkUpload(httptest.NewRequest(method, path, nil)) {
			t.Errorf("%s 不应归为大文件接口", s)
		}
	}
}

// 读完请求体后处理函数再读一次（确认 EOF）不能重新设置读取期限，否则到点会取消请求上下文。
func TestReadAfterEOFKeepsContext(t *testing.T) {
	shrinkBodyLimits(t)
	srv := httptest.NewServer(limitBodyRead(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = r.Body.Read(make([]byte, 1))
		select {
		case <-time.After(time.Second):
			_, _ = io.WriteString(w, "done")
		case <-r.Context().Done():
			http.Error(w, "canceled", http.StatusInternalServerError)
		}
	})))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ddns/tasks", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "done" {
		t.Fatalf("读完后再读一次导致请求被打断: %q", body)
	}
}

// 大文件接口不限总时长；已读字节很多时平均速率的计算不能溢出（旧写法超过约 8.6 GiB 即误判为过慢）。
func TestBulkRateNoOverflow(t *testing.T) {
	gb := &guardedBody{
		ReadCloser: io.NopCloser(strings.NewReader("x")),
		rc:         http.NewResponseController(httptest.NewRecorder()),
		start:      time.Now().Add(-2 * time.Hour),
		idle:       bulkIdleTimeout, total: bulkTotalTimeout, grace: bulkRateGrace, minRate: bulkMinRate,
		n: 10 << 30, length: -1,
	}
	if _, err := gb.Read(make([]byte, 8)); err != nil {
		t.Fatalf("2 小时读了 10 GiB 不应判为过慢: %v", err)
	}
	slow := &guardedBody{
		ReadCloser: io.NopCloser(strings.NewReader("x")),
		rc:         http.NewResponseController(httptest.NewRecorder()),
		start:      time.Now().Add(-time.Hour),
		idle:       bulkIdleTimeout, total: bulkTotalTimeout, grace: bulkRateGrace, minRate: bulkMinRate,
		n: 1 << 20, length: -1,
	}
	if _, err := slow.Read(make([]byte, 8)); err != errBodyTooSlow {
		t.Fatalf("1 小时只读 1 MiB 应判为过慢，实际 %v", err)
	}
}
