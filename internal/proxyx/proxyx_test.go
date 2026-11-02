package proxyx

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	ok := []string{"", "direct", "update", " Direct ", "http://127.0.0.1:7890", "https://p.example.com", "socks5://u:p@127.0.0.1:1080", "socks5h://[::1]:1080", "http://host:8080/"}
	for _, s := range ok {
		if err := Validate(s); err != nil {
			t.Errorf("Validate(%q) = %v，应通过", s, err)
		}
	}
	bad := []string{"127.0.0.1:7890", "ftp://host", "http://", "http://host:0", "http://host:70000", "http://host/path", "http://host?x=1", "socks4://host:1080", "auto"}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("Validate(%q) 应失败", s)
		}
	}
}

func TestResolve(t *testing.T) {
	t.Cleanup(func() { SetUpdateProxy(nil) })
	if u, direct := Resolve(""); u != nil || direct {
		t.Fatalf("空值应为未设置")
	}
	if u, direct := Resolve("DIRECT"); u != nil || !direct {
		t.Fatalf("direct 应为强制直连")
	}
	// 未注入或更新代理为空时，update 按未设置处理
	if u, direct := Resolve("update"); u != nil || direct {
		t.Fatalf("更新代理为空时 update 应为未设置")
	}
	cur := "http://u:secret@10.0.0.1:3128"
	SetUpdateProxy(func() string { return cur })
	if u, _ := Resolve("update"); u == nil || u.Host != "10.0.0.1:3128" {
		t.Fatalf("update 应解析为更新代理，得到 %v", u)
	}
	// 修改设置后立即生效
	cur = "socks5://10.0.0.2:1080"
	if u, _ := Resolve("update"); u == nil || u.Scheme != "socks5" {
		t.Fatalf("更新代理修改后应立即生效，得到 %v", u)
	}
	// 被手工改坏的更新代理不会导致发送中断，按未设置处理
	cur = "not a proxy"
	if u, direct := Resolve("update"); u != nil || direct {
		t.Fatalf("无效的更新代理应按未设置处理")
	}
	if d := Describe("http://u:secret@10.0.0.1:3128"); strings.Contains(d, "secret") {
		t.Fatalf("Describe 不应包含密码: %s", d)
	}
}

// forwardProxy 是一个记录请求的 HTTP 正向代理（只处理 http:// 目标）。
func forwardProxy(t *testing.T, hits *atomic.Int32, wantAuth string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if wantAuth != "" && r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte(wantAuth)) {
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		if !r.URL.IsAbs() {
			http.Error(w, "not a proxy request", http.StatusBadRequest)
			return
		}
		out, _ := http.NewRequest(r.Method, r.URL.String(), r.Body)
		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClientUsesProxy(t *testing.T) {
	t.Cleanup(func() { SetUpdateProxy(nil) })
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer target.Close()
	var hits atomic.Int32
	proxy := forwardProxy(t, &hits, "user:pa ss")
	proxyURL := strings.Replace(proxy.URL, "http://", "http://user:pa%20ss@", 1)

	get := func(setting string) string {
		t.Helper()
		resp, err := Client(setting, 5*time.Second, false).Get(target.URL)
		if err != nil {
			t.Fatalf("%q 请求失败: %v", setting, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := get(proxyURL); got != "ok" || hits.Load() != 1 {
		t.Fatalf("应经代理访问，响应 %q，代理收到 %d 次", got, hits.Load())
	}
	// update：复用更新代理
	SetUpdateProxy(func() string { return proxyURL })
	if got := get("update"); got != "ok" || hits.Load() != 2 {
		t.Fatalf("update 应经更新代理访问，代理收到 %d 次", hits.Load())
	}
	// direct：即使设置了环境变量也直连（httptest 地址是 127.0.0.1，环境变量本就不代理回环地址，这里只确认不经指定代理）
	if got := get("direct"); got != "ok" || hits.Load() != 2 {
		t.Fatalf("direct 不应经过代理，代理收到 %d 次", hits.Load())
	}
	// 同一代理复用同一个 Transport
	if Client(proxyURL, 0, false).Transport != Client(proxyURL, time.Second, false).Transport {
		t.Fatalf("同一代理应复用 Transport")
	}
	if Client(proxyURL, 0, false).Transport == Client(proxyURL, 0, true).Transport {
		t.Fatalf("跳过证书校验的 Transport 应与普通的分开")
	}
}

// connectProxy 是支持 CONNECT 的最小代理：校验认证后连到目标，并在 200 响应后立刻附上 greeting（模拟目标先说话的协议）。
func connectProxy(t *testing.T, wantAuth string, onConn func(c net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				if onConn != nil {
					onConn(c)
					return
				}
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil || req.Method != http.MethodConnect {
					return
				}
				if wantAuth != "" && req.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte(wantAuth)) {
					_, _ = io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
					return
				}
				up, err := net.Dial("tcp", req.Host)
				if err != nil {
					_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
					return
				}
				defer up.Close()
				// 读取目标的欢迎语，与 200 响应一起写出，验证客户端不会丢掉响应头后已缓冲的数据
				greet := make([]byte, 64)
				_ = up.SetReadDeadline(time.Now().Add(2 * time.Second))
				n, _ := up.Read(greet)
				_ = up.SetReadDeadline(time.Time{})
				_, _ = c.Write(append([]byte("HTTP/1.1 200 Connection established\r\n\r\n"), greet[:n]...))
				go func() { _, _ = io.Copy(up, br) }()
				_, _ = io.Copy(c, up)
			}()
		}
	}()
	return ln.Addr().String()
}

// echoServer 先发送欢迎语，再把收到的数据原样返回。
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.WriteString(c, "220 hello\r\n")
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func checkEcho(t *testing.T, conn net.Conn) {
	t.Helper()
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil || line != "220 hello\r\n" {
		t.Fatalf("欢迎语 %q, %v", line, err)
	}
	_, _ = io.WriteString(conn, "ping\n")
	line, err = br.ReadString('\n')
	if err != nil || line != "ping\n" {
		t.Fatalf("回显 %q, %v", line, err)
	}
}

func TestDialConnect(t *testing.T) {
	target := echoServer(t)
	proxy := connectProxy(t, "u:p", nil)
	conn, err := Dial(context.Background(), "http://u:p@"+proxy, target, 5*time.Second)
	if err != nil {
		t.Fatalf("经 CONNECT 代理连接失败: %v", err)
	}
	checkEcho(t, conn)

	// 认证失败：错误信息包含状态，不包含密码
	_, err = Dial(context.Background(), "http://u:wrongpass@"+proxy, target, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "407") || strings.Contains(err.Error(), "wrongpass") {
		t.Fatalf("认证失败应报 407 且隐藏密码，得到 %v", err)
	}
}

func TestDialConnectRejectsBadTarget(t *testing.T) {
	proxy := connectProxy(t, "", nil)
	for _, addr := range []string{"", "smtp.example.com:25\r\nX-Evil: 1", "a b:25"} {
		if _, err := Dial(context.Background(), "http://"+proxy, addr, time.Second); err == nil {
			t.Errorf("目标地址 %q 应被拒绝", addr)
		}
	}
}

func TestDialWithoutProxyIsDirect(t *testing.T) {
	target := echoServer(t)
	for _, s := range []string{"", "direct", "update"} {
		conn, err := Dial(context.Background(), s, target, 5*time.Second)
		if err != nil {
			t.Fatalf("%q 应直连: %v", s, err)
		}
		checkEcho(t, conn)
	}
}

func TestDialConnectStalledProxyHonorsContext(t *testing.T) {
	// 代理接受连接后一直不回应
	proxy := connectProxy(t, "", func(c net.Conn) {
		time.Sleep(5 * time.Second)
		c.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Dial(ctx, "http://"+proxy, "127.0.0.1:25", 10*time.Second)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("代理无响应时应按 context 超时返回，耗时 %v，错误 %v", time.Since(start), err)
	}
}

func TestDialConnectRejectsHugeResponse(t *testing.T) {
	proxy := connectProxy(t, "", func(c net.Conn) {
		defer c.Close()
		_, _ = http.ReadRequest(bufio.NewReader(c))
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nX-Junk: ")
		junk := strings.Repeat("a", 4096)
		for i := 0; i < 16; i++ {
			if _, err := io.WriteString(c, junk); err != nil {
				return
			}
		}
	})
	if _, err := Dial(context.Background(), "http://"+proxy, "127.0.0.1:25", 5*time.Second); err == nil {
		t.Fatalf("超长的代理响应头应被拒绝")
	}
}

// socks5Server 是只支持 CONNECT 的最小 SOCKS5 服务，可选用户名密码认证。
func socks5Server(t *testing.T, user, pass string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				methods := make([]byte, hdr[1])
				_, _ = io.ReadFull(c, methods)
				if user == "" {
					_, _ = c.Write([]byte{5, 0})
				} else {
					_, _ = c.Write([]byte{5, 2})
					ah := make([]byte, 2)
					_, _ = io.ReadFull(c, ah)
					u := make([]byte, ah[1])
					_, _ = io.ReadFull(c, u)
					pl := make([]byte, 1)
					_, _ = io.ReadFull(c, pl)
					p := make([]byte, pl[0])
					_, _ = io.ReadFull(c, p)
					if string(u) != user || string(p) != pass {
						_, _ = c.Write([]byte{1, 1})
						return
					}
					_, _ = c.Write([]byte{1, 0})
				}
				req := make([]byte, 4)
				if _, err := io.ReadFull(c, req); err != nil {
					return
				}
				var host string
				switch req[3] {
				case 1:
					a := make([]byte, 4)
					_, _ = io.ReadFull(c, a)
					host = net.IP(a).String()
				case 3:
					l := make([]byte, 1)
					_, _ = io.ReadFull(c, l)
					h := make([]byte, l[0])
					_, _ = io.ReadFull(c, h)
					host = string(h)
				default:
					return
				}
				pb := make([]byte, 2)
				_, _ = io.ReadFull(c, pb)
				up, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb)))))
				if err != nil {
					_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer up.Close()
				_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestDialSOCKS5(t *testing.T) {
	target := echoServer(t)
	conn, err := Dial(context.Background(), "socks5://"+socks5Server(t, "", ""), target, 5*time.Second)
	if err != nil {
		t.Fatalf("经 SOCKS5 代理连接失败: %v", err)
	}
	checkEcho(t, conn)

	authed := socks5Server(t, "alice", "s3cret")
	conn, err = Dial(context.Background(), "socks5h://alice:s3cret@"+authed, target, 5*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 认证连接失败: %v", err)
	}
	checkEcho(t, conn)
	if _, err := Dial(context.Background(), "socks5://alice:bad@"+authed, target, 5*time.Second); err == nil || strings.Contains(err.Error(), ":bad@") {
		t.Fatalf("SOCKS5 认证失败应报错且隐藏密码，得到 %v", err)
	}
}
