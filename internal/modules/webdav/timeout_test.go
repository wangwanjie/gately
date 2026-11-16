package webdav

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// startTimeoutModule 以缩短的超时启动只有一个读写用户的 WebDAV 服务，返回地址与共享目录。
func startTimeoutModule(t *testing.T) (string, string) {
	t.Helper()
	oldIdle, oldBody := idleTimeout, bodyIdleTimeout
	idleTimeout, bodyIdleTimeout = 400*time.Millisecond, 400*time.Millisecond
	t.Cleanup(func() { idleTimeout, bodyIdleTimeout = oldIdle, oldBody })
	root := t.TempDir()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{{Name: "u", Password: "p", Root: root}}}
	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	t.Cleanup(func() { _ = m.Stop() })
	if m.srv.ReadHeaderTimeout <= 0 || m.srv.IdleTimeout <= 0 {
		t.Fatalf("应设置请求头与空闲超时: %v %v", m.srv.ReadHeaderTimeout, m.srv.IdleTimeout)
	}
	return "127.0.0.1:" + strconv.Itoa(port), root
}

// readUntilClosed 读到连接关闭为止，超过 limit 视为失败，返回耗时。
func readUntilClosed(t *testing.T, c net.Conn, limit time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	_ = c.SetReadDeadline(start.Add(limit))
	if _, err := io.Copy(io.Discard, c); err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			t.Fatalf("%v 内连接未被服务端关闭", limit)
		}
	}
	return time.Since(start)
}

func TestIdleConnectionClosed(t *testing.T) {
	addr, _ := startTimeoutModule(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// 一次完整的请求后保持连接空闲
	fmt.Fprintf(c, "OPTIONS / HTTP/1.1\r\nHost: x\r\nAuthorization: Basic dTpw\r\n\r\n")
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	start := time.Now()
	_ = c.SetReadDeadline(start.Add(3 * time.Second))
	if _, err := br.ReadByte(); err == nil {
		t.Fatal("空闲连接不应收到数据")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("空闲连接应被服务端关闭")
	}
	if d := time.Since(start); d < 300*time.Millisecond {
		t.Fatalf("空闲连接过早被关闭: %v", d)
	}
}

func TestStalledUploadClosed(t *testing.T) {
	addr, root := startTimeoutModule(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// 声明 100 字节，只发 10 字节后停住
	fmt.Fprintf(c, "PUT /stall.bin HTTP/1.1\r\nHost: x\r\nAuthorization: Basic dTpw\r\nContent-Length: 100\r\n\r\n0123456789")
	if d := readUntilClosed(t, c, 3*time.Second); d < 300*time.Millisecond {
		t.Fatalf("上传停住后过早被关闭: %v", d)
	}
	if b, err := os.ReadFile(filepath.Join(root, "stall.bin")); err == nil && len(b) == 100 {
		t.Fatal("未传完的上传不应写成完整文件")
	}
}

func TestSlowSteadyUploadSucceeds(t *testing.T) {
	addr, root := startTimeoutModule(t)
	pr, pw := io.Pipe()
	go func() {
		// 每 150 毫秒发 1 字节，共约 1.5 秒：总时长超过停顿超时，但每次停顿都短于它
		for i := 0; i < 10; i++ {
			time.Sleep(150 * time.Millisecond)
			if _, err := pw.Write([]byte{byte('a' + i)}); err != nil {
				return
			}
		}
		pw.Close()
	}()
	req, _ := http.NewRequest("PUT", "http://"+addr+"/slow.txt", pr)
	req.SetBasicAuth("u", "p")
	req.ContentLength = 10
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("慢速但持续的上传应成功: %d", resp.StatusCode)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "slow.txt")); string(b) != "abcdefghij" {
		t.Fatalf("上传内容不正确: %q", b)
	}
}
