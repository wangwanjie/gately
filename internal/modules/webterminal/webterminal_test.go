package webterminal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

// startSSHServer 启动一个只接受指定公钥的 SSH 服务端：申请 pty 后把输入加上前缀回显。
func startSSHServer(t *testing.T, clientPub ssh.PublicKey) (string, ssh.PublicKey) {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hostPriv)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if string(k.Marshal()) == string(clientPub.Marshal()) {
			return nil, nil
		}
		return nil, io.EOF
	}}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, _ := nch.Accept()
					go func() {
						for req := range creqs {
							_ = req.Reply(req.Type == "pty-req" || req.Type == "shell" || req.Type == "window-change", nil)
						}
					}()
					go func() {
						buf := make([]byte, 1024)
						for {
							n, err := ch.Read(buf)
							if err != nil {
								return
							}
							_, _ = ch.Write(append([]byte("echo:"), buf[:n]...))
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String(), hostSigner.PublicKey()
}

func newKey(t *testing.T) (string, ssh.PublicKey) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(priv)
	return string(pem.EncodeToMemory(block)), signer.PublicKey()
}

type router struct{ mux *http.ServeMux }

func (r router) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	r.mux.HandleFunc(method+" /api/webterminal"+path, h)
}

func newModule(t *testing.T) (*Module, *httptest.Server) {
	st, _ := config.Open(t.TempDir())
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	// 多数用例关注终端功能本身，默认视为已接受用户协议；门禁用例单独覆盖
	m.cfg.Security.AgreementAccepted = true
	mux := http.NewServeMux()
	m.Routes(router{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { _ = m.Stop() })
	return m, srv
}

// openWS 连接终端，返回读取函数与写函数。
func openWS(t *testing.T, srv *httptest.Server, key string) (*websocket.Conn, context.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ws, _, err := websocket.Dial(ctx, strings.Replace(srv.URL, "http", "ws", 1)+"/api/webterminal/connections/"+key+"/ws?cols=100&rows=30", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	return ws, ctx
}

// readUntil 读取消息直到包含 want。
func readUntil(t *testing.T, ws *websocket.Conn, ctx context.Context, want string) string {
	t.Helper()
	var all strings.Builder
	for !strings.Contains(all.String(), want) {
		_, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("等待 %q 失败: %v（已收到 %q）", want, err, all.String())
		}
		all.Write(b)
	}
	return all.String()
}

func TestSSHSessionAndHostKeyTOFU(t *testing.T) {
	keyPEM, pub := newKey(t)
	addr, hostPub := startSSHServer(t, pub)
	host, port, _ := net.SplitHostPort(addr)
	m, srv := newModule(t)
	c := Connection{Key: "c1", Name: "nas", SSH: SSHConfig{Host: host, Username: "root", AuthType: "key", PrivateKey: keyPEM}}
	c.SSH.Port, _ = strconv.Atoi(port)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Connections = []Connection{c}

	ws, ctx := openWS(t, srv, "c1")
	out := readUntil(t, ws, ctx, "首次连接")
	if !strings.Contains(out, Fingerprint(hostPub)) {
		t.Fatalf("首次连接应提示主机指纹: %s", out)
	}
	msg, _ := json.Marshal(clientMsg{T: "i", D: "ls\n"})
	_ = ws.Write(ctx, websocket.MessageText, msg)
	readUntil(t, ws, ctx, "echo:ls")
	resize, _ := json.Marshal(clientMsg{T: "r", C: 120, R: 40})
	_ = ws.Write(ctx, websocket.MessageText, resize)
	ws.Close(websocket.StatusNormalClosure, "")

	m.mu.Lock()
	recorded := m.cfg.Connections[0].SSH.HostKey
	m.mu.Unlock()
	if recorded != Fingerprint(hostPub) {
		t.Fatalf("应记录主机指纹: %q", recorded)
	}

	// 模拟主机密钥被替换（中间人）
	m.mu.Lock()
	m.cfg.Connections[0].SSH.HostKey = "SHA256:forged"
	m.mu.Unlock()
	ws2, ctx2 := openWS(t, srv, "c1")
	readUntil(t, ws2, ctx2, "主机密钥已变化")
}

func TestLocalShellGate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("本机 Shell 仅支持 Unix")
	}
	m, srv := newModule(t)
	m.cfg.Connections = []Connection{{Key: "l", Name: "local", Type: TypeLocal, Local: LocalConfig{Shell: "/bin/sh"}}}
	ws, ctx := openWS(t, srv, "l")
	readUntil(t, ws, ctx, "本机 Shell 未开启")

	m.cfg.Settings.AllowLocalShell = true
	ws2, ctx2 := openWS(t, srv, "l")
	msg, _ := json.Marshal(clientMsg{T: "i", D: "echo gately-$((40+2))\n"})
	_ = ws2.Write(ctx2, websocket.MessageText, msg)
	readUntil(t, ws2, ctx2, "gately-42")
}

func TestValidateAndMasking(t *testing.T) {
	keyPEM, _ := newKey(t)
	bad := []Connection{
		{Name: ""},
		{Name: "a", SSH: SSHConfig{Host: "h"}},
		{Name: "a", SSH: SSHConfig{Host: "h", Username: "u", AuthType: "key", PrivateKey: "garbage"}},
		{Name: "a", Type: "telnet"},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("第 %d 个非法配置未报错", i)
		}
	}
	m, _ := newModule(t)
	c := Connection{Name: "a", SSH: SSHConfig{Host: "h", Username: "u", AuthType: "key", PrivateKey: keyPEM}}
	_ = c.Validate()
	v := m.viewLocked(&c).(Connection)
	if v.SSH.PrivateKey != "********" {
		t.Fatal("私钥应脱敏")
	}
}

func TestImportLucky(t *testing.T) {
	keyPEM, _ := newKey(t)
	raw, _ := json.Marshal([]map[string]any{
		{"name": "pve", "type": "ssh", "sshConfig": map[string]any{"host": "192.168.1.2", "port": 22, "username": "root", "authType": "key", "privateKey": keyPEM, "keepAlive": 60, "proxy": map[string]any{"enabled": true, "host": "127.0.0.1", "port": 1080}}},
		{"name": "old", "type": "telnet"},
	})
	m, _ := newModule(t)
	rep, err := m.ImportLucky(luckyimport.Document{"connections": raw}, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Found != 2 || rep.Imported != 1 || m.cfg.Connections[0].SSH.KeepAlive != 60 || !m.cfg.Connections[0].SSH.Proxy.Enabled {
		t.Fatalf("导入结果错误: %+v", rep)
	}
	if path := os.Getenv("GATELY_LUCKY_TERM_SAMPLE"); path != "" {
		data, _ := os.ReadFile(path)
		doc, err := luckyimport.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := m.ImportLucky(doc, luckyimport.Options{DryRun: true})
		if err != nil || rep.Imported != rep.Found {
			t.Fatalf("真实样本导入失败: %+v %v", rep, err)
		}
		t.Logf("真实样本 %d/%d", rep.Imported, rep.Found)
	}
}

// Lucky 3.0 的每个模块文件都带 securityConfig，其他模块的文件不应被识别为 Web 终端配置
func TestImportLuckyIgnoresOtherModuleSecurityConfig(t *testing.T) {
	m, _ := newModule(t)
	doc := luckyimport.Document{"securityConfig": json.RawMessage(`{"twoFAEnable":true,"twoFAKey":"ABCDEFGH"}`), "DDNSTaskList": json.RawMessage(`[]`)}
	rep, err := m.ImportLucky(doc, luckyimport.Options{})
	if err != nil || rep != nil {
		t.Fatalf("其他模块文件应被忽略: %+v %v", rep, err)
	}
	if m.cfg.Security.TwoFAEnable {
		t.Fatal("不应导入其他模块的二次验证密钥")
	}
}

func TestConnectionTest(t *testing.T) {
	keyPEM, pub := newKey(t)
	addr, hostPub := startSSHServer(t, pub)
	host, port, _ := net.SplitHostPort(addr)
	m, srv := newModule(t)
	c := Connection{Key: "c1", Name: "nas", SSH: SSHConfig{Host: host, Username: "root", AuthType: "key", PrivateKey: keyPEM}}
	c.SSH.Port, _ = strconv.Atoi(port)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := c
	bad.Key, bad.SSH.Port = "c2", 1
	m.cfg.Connections = []Connection{c, bad}
	post := func(key string) map[string]any {
		resp, err := http.Post(srv.URL+"/api/webterminal/connections/"+key+"/test", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	out := post("c1")
	if out["ok"] != true || out["newHostKey"] != Fingerprint(hostPub) {
		t.Fatalf("测试连接应成功并返回主机指纹: %v", out)
	}
	m.mu.Lock()
	recorded := m.cfg.Connections[0].SSH.HostKey
	m.mu.Unlock()
	if recorded != "" {
		t.Fatal("测试连接不应记录主机密钥")
	}
	if out := post("c2"); out["ok"] != false || out["error"] == "" {
		t.Fatalf("不可达的主机应返回错误: %v", out)
	}
}

// ---- 以下为持久会话、Telnet、代理、门禁、导入等用例 ----

// fakeTelnet 是测试用 Telnet 服务端：发起选项协商、提示登录，登录成功后回显输入。
type fakeTelnet struct {
	addr string
	mu   sync.Mutex
	raw  []byte
}

func (f *fakeTelnet) received() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.raw...)
}

func startTelnetServer(t *testing.T) *fakeTelnet {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeTelnet{addr: ln.Addr().String()}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer nc.Close()
				// 协商：请求终端类型与窗口大小，接管回显，抑制 GA
				_, _ = nc.Write([]byte{255, 253, 24, 255, 253, 31, 255, 251, 1, 255, 251, 3, 255, 250, 24, 1, 255, 240})
				_, _ = nc.Write([]byte("login: "))
				stage := 0
				buf := make([]byte, 4096)
				for {
					n, err := nc.Read(buf)
					if n > 0 {
						f.mu.Lock()
						f.raw = append(f.raw, buf[:n]...)
						raw := string(f.raw)
						f.mu.Unlock()
						switch {
						case stage == 0 && strings.Contains(raw, "root\r\x00"):
							stage = 1
							_, _ = nc.Write([]byte("Password: "))
						case stage == 1 && strings.Contains(raw, "secret\r\x00"):
							stage = 2
							_, _ = nc.Write([]byte("welcome\r\n$ "))
						case stage == 2 && !strings.ContainsRune(string(buf[:n]), 255):
							_, _ = nc.Write(append([]byte("echo:"), buf[:n]...))
						}
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	return f
}

// telnetConn 在模块中登记一个指向 addr 的 Telnet 连接。
func telnetConn(t *testing.T, m *Module, key, addr string, user, pass string) {
	host, port, _ := net.SplitHostPort(addr)
	c := Connection{Key: key, Name: key, Type: TypeTelnet, Telnet: TelnetConfig{Host: host, Username: user, Password: pass}}
	c.Telnet.Port, _ = strconv.Atoi(port)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cfg.Connections = append(m.cfg.Connections, c)
	m.mu.Unlock()
}

// dialWS 打开 WebSocket，失败时返回 HTTP 状态码。
func dialWS(t *testing.T, srv *httptest.Server, path string) (*websocket.Conn, context.Context, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ws, resp, err := websocket.Dial(ctx, strings.Replace(srv.URL, "http", "ws", 1)+"/api/webterminal"+path, nil)
	if err != nil {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, ctx, code
	}
	t.Cleanup(func() { ws.CloseNow() })
	return ws, ctx, 101
}

// readBinaryUntil 读取帧直到二进制输出累计包含 want，同时返回见到的会话编号。
func readBinaryUntil(t *testing.T, ws *websocket.Conn, ctx context.Context, want string) (out, sid string) {
	t.Helper()
	var all strings.Builder
	for !strings.Contains(all.String(), want) {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("等待 %q 失败: %v（已收到 %q）", want, err, all.String())
		}
		if typ == websocket.MessageBinary {
			all.Write(b)
			continue
		}
		var msg map[string]string
		_ = json.Unmarshal(b, &msg)
		if msg["t"] == "s" {
			sid = msg["d"]
		}
	}
	return all.String(), sid
}

func sendInput(t *testing.T, ws *websocket.Conn, ctx context.Context, s string) {
	t.Helper()
	b, _ := json.Marshal(clientMsg{T: "i", D: s})
	if err := ws.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func call(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, srv.URL+"/api/webterminal"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", what)
}

func TestTelnetNegotiationAndAutoLogin(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	telnetConn(t, m, "t1", f.addr, "root", "secret")

	ws, ctx, _ := dialWS(t, srv, "/connections/t1/ws?cols=100&rows=30")
	readBinaryUntil(t, ws, ctx, "welcome")
	sendInput(t, ws, ctx, "ls\r")
	out, _ := readBinaryUntil(t, ws, ctx, "echo:ls")
	if strings.Contains(out, "\xff") {
		t.Fatalf("IAC 序列不应进入终端输出: %q", out)
	}
	b, _ := json.Marshal(clientMsg{T: "r", C: 120, R: 40})
	_ = ws.Write(ctx, websocket.MessageText, b)

	raw := f.received()
	for name, want := range map[string][]byte{
		"WILL TTYPE":  {255, 251, 24},
		"WILL NAWS":   {255, 251, 31},
		"DO ECHO":     {255, 253, 1},
		"TTYPE IS":    append([]byte{255, 250, 24, 0}, "xterm-256color"...),
		"NAWS 100x30": {255, 250, 31, 0, 100, 0, 30, 255, 240},
	} {
		if !bytes.Contains(raw, want) {
			t.Errorf("缺少协商应答 %s: %v", name, raw)
		}
	}
	waitFor(t, "NAWS 随窗口调整更新", func() bool {
		return bytes.Contains(f.received(), []byte{255, 250, 31, 0, 120, 0, 40, 255, 240})
	})
}

func TestPersistentSessionReplayAndMultiAttach(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	m.cfg.Settings.SessionKeepAlive = 5
	telnetConn(t, m, "t1", f.addr, "root", "secret")

	ws1, ctx1, _ := dialWS(t, srv, "/connections/t1/ws")
	_, sid := readBinaryUntil(t, ws1, ctx1, "welcome")
	if sid == "" {
		t.Fatal("应先收到会话编号")
	}
	sendInput(t, ws1, ctx1, "first\r")
	readBinaryUntil(t, ws1, ctx1, "echo:first")
	ws1.Close(websocket.StatusNormalClosure, "")

	// 断线后会话保留
	waitFor(t, "会话变为未附着", func() bool {
		code, _ := call(t, srv, "GET", "/sessions/"+sid, nil)
		s := m.hub.get(sid)
		return code == 200 && s != nil && s.view().Attached == 0
	})

	// 重新附着：先回放历史输出
	ws2, ctx2, _ := dialWS(t, srv, "/sessions/"+sid+"/ws")
	out, sid2 := readBinaryUntil(t, ws2, ctx2, "echo:first")
	if sid2 != sid || !strings.Contains(out, "welcome") {
		t.Fatalf("附着应回放缓冲并返回会话编号: sid=%q out=%q", sid2, out)
	}
	// 第二个客户端同时附着：输出广播、输入共享
	ws3, ctx3, _ := dialWS(t, srv, "/sessions/"+sid+"/ws")
	readBinaryUntil(t, ws3, ctx3, "echo:first")
	sendInput(t, ws3, ctx3, "second\r")
	readBinaryUntil(t, ws2, ctx2, "echo:second")
	readBinaryUntil(t, ws3, ctx3, "echo:second")

	// REST：列表、备注、统计
	code, _ := call(t, srv, "PUT", "/sessions/"+sid+"/remark", map[string]string{"remark": "生产机"})
	if code != 200 {
		t.Fatalf("设置备注失败: %d", code)
	}
	v := m.hub.get(sid).view()
	if v.Remark != "生产机" || v.Attached != 2 || v.BytesIn == 0 || v.BytesOut == 0 || v.ConnKey != "t1" || v.Type != TypeTelnet {
		t.Fatalf("会话信息不正确: %+v", v)
	}
	if _, st := call(t, srv, "GET", "/sessions/"+sid+"/stats", nil); st["attached"] != float64(2) {
		t.Fatalf("统计不正确: %v", st)
	}

	// DELETE 结束会话：客户端收到 x 帧
	call(t, srv, "DELETE", "/sessions/"+sid, nil)
	for {
		typ, b, err := ws2.Read(ctx2)
		if err != nil {
			t.Fatal("应先收到结束通知:", err)
		}
		if typ == websocket.MessageText && strings.Contains(string(b), `"t":"x"`) {
			break
		}
	}
	if m.hub.get(sid) != nil {
		t.Fatal("会话应已从表中移除")
	}
	if code, _ := call(t, srv, "GET", "/sessions/"+sid, nil); code != 404 {
		t.Fatalf("已结束的会话应返回 404: %d", code)
	}
}

func TestSessionEndsOnDisconnectWhenKeepAliveZero(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	telnetConn(t, m, "t1", f.addr, "root", "secret")
	ws, ctx, _ := dialWS(t, srv, "/connections/t1/ws")
	readBinaryUntil(t, ws, ctx, "welcome")
	if m.hub.count() != 1 {
		t.Fatal("应有一个活动会话")
	}
	ws.Close(websocket.StatusNormalClosure, "")
	waitFor(t, "会话随浏览器断开而结束", func() bool { return m.hub.count() == 0 })
}

func TestSessionExpiry(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	m.cfg.Settings.SessionKeepAlive = 1
	telnetConn(t, m, "t1", f.addr, "root", "secret")
	ws, ctx, _ := dialWS(t, srv, "/connections/t1/ws")
	_, sid := readBinaryUntil(t, ws, ctx, "welcome")
	hs := m.hub.get(sid)

	// 附着中不会因断线保留超时而被回收，但空闲超时仍生效
	hs.expireCheck(time.Now().Add(10*time.Minute), 0, 1)
	if m.hub.get(sid) == nil {
		t.Fatal("附着中的会话不应因断线保留超时被回收")
	}
	ws.Close(websocket.StatusNormalClosure, "")
	waitFor(t, "会话变为未附着", func() bool { return hs.view().Attached == 0 })
	hs.expireCheck(time.Now().Add(30*time.Second), 0, 1)
	if m.hub.get(sid) == nil {
		t.Fatal("未超过保留时间不应回收")
	}
	hs.expireCheck(time.Now().Add(2*time.Minute), 0, 1)
	if m.hub.get(sid) != nil {
		t.Fatal("超过保留时间应回收")
	}

	// 空闲超时
	ws2, ctx2, _ := dialWS(t, srv, "/connections/t1/ws")
	_, sid2 := readBinaryUntil(t, ws2, ctx2, "welcome")
	m.hub.get(sid2).expireCheck(time.Now().Add(2*time.Minute), 1, 0)
	if m.hub.get(sid2) != nil {
		t.Fatal("空闲超时应结束会话")
	}
}

func TestJanitorAndStop(t *testing.T) {
	old := janitorInterval
	janitorInterval = 20 * time.Millisecond
	defer func() { janitorInterval = old }()
	f := startTelnetServer(t)
	m, srv := newModule(t)
	m.cfg.Settings.IdleTimeout = 1
	telnetConn(t, m, "t1", f.addr, "root", "secret")
	ws, ctx, _ := dialWS(t, srv, "/connections/t1/ws")
	_, sid := readBinaryUntil(t, ws, ctx, "welcome")
	hs := m.hub.get(sid)
	hs.lastActive.Store(time.Now().Add(-2 * time.Minute).UnixNano())
	waitFor(t, "后台回收空闲会话", func() bool { return m.hub.count() == 0 })

	ws2, ctx2, _ := dialWS(t, srv, "/connections/t1/ws")
	readBinaryUntil(t, ws2, ctx2, "welcome")
	_ = m.Stop()
	if m.hub.count() != 0 {
		t.Fatal("Stop 应关闭全部会话")
	}
}

func TestMaxSessions(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	m.cfg.Settings.MaxSessions = 1
	telnetConn(t, m, "t1", f.addr, "root", "secret")
	ws, ctx, _ := dialWS(t, srv, "/connections/t1/ws")
	readBinaryUntil(t, ws, ctx, "welcome")
	if _, _, code := dialWS(t, srv, "/connections/t1/ws"); code != http.StatusTooManyRequests {
		t.Fatalf("超过上限应返回 429: %d", code)
	}
}

func TestHeartbeat(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	telnetConn(t, m, "t1", f.addr, "root", "secret")
	ws, ctx, _ := dialWS(t, srv, "/connections/t1/ws")
	readBinaryUntil(t, ws, ctx, "welcome")
	_ = ws.Write(ctx, websocket.MessageText, []byte(`{"t":"p"}`))
	for {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatal("应收到心跳响应:", err)
		}
		if typ == websocket.MessageText && string(b) == `{"t":"p"}` {
			return
		}
	}
}

func TestReplayBufferTrim(t *testing.T) {
	hs := &hubSession{ringCap: 8, clients: map[*client]struct{}{}, log: nil}
	hs.sess = nil
	for i := 0; i < 5; i++ {
		hs.mu.Lock()
		hs.ring = append(hs.ring, []byte("0123456789")...)
		if len(hs.ring) > 2*hs.ringCap {
			hs.ring = append([]byte(nil), hs.ring[len(hs.ring)-hs.ringCap:]...)
		}
		hs.mu.Unlock()
	}
	if len(hs.ring) > 2*hs.ringCap {
		t.Fatalf("缓冲未被压缩: %d", len(hs.ring))
	}
	// 开头为完整字符时原样回放；开头是被截断的续字节时跳过
	if got := replaySnapshot([]byte("中字")); string(got) != "中字" {
		t.Fatalf("完整字符不应被裁剪: %q", got)
	}
	if string(replaySnapshot([]byte("\x96\xadabc"))) != "abc" {
		t.Fatal("应跳过开头的续字节")
	}
}

func TestAccessGate(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	telnetConn(t, m, "t1", f.addr, "root", "secret")
	m.cfg.Security.AgreementAccepted = false

	if _, _, code := dialWS(t, srv, "/connections/t1/ws"); code != 403 {
		t.Fatalf("未接受协议应拒绝新建会话: %d", code)
	}
	if _, _, code := dialWS(t, srv, "/sessions/x/ws"); code != 403 {
		t.Fatalf("未接受协议应拒绝附着会话: %d", code)
	}
	// 开启本机 Shell 同样受门禁约束
	code, body := call(t, srv, "PUT", "/settings", Settings{AllowLocalShell: true})
	if code != 403 || body["needAgreement"] != true {
		t.Fatalf("开启本机 Shell 应受门禁: %d %v", code, body)
	}
	if code, _ := call(t, srv, "PUT", "/settings", Settings{AllowLocalShell: false}); code != 200 {
		t.Fatal("不开启本机 Shell 的设置修改不受门禁约束")
	}
	call(t, srv, "POST", "/security/agreement", nil)
	if _, st := call(t, srv, "GET", "/security", nil); st["agreementAccepted"] != true {
		t.Fatalf("协议状态不正确: %v", st)
	}
	ws, ctx, code2 := dialWS(t, srv, "/connections/t1/ws")
	if code2 != 101 {
		t.Fatalf("接受协议后应放行: %d", code2)
	}
	readBinaryUntil(t, ws, ctx, "welcome")
}

// totpNow 计算当前时间的 TOTP 验证码（RFC 6238），与被测实现独立。
func totpNow(secret string) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(time.Now().Unix()/30))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000)
}

func TestTwoFAFlow(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)
	telnetConn(t, m, "t1", f.addr, "root", "secret")

	_, setup := call(t, srv, "POST", "/security/2fa/setup", nil)
	secret, _ := setup["secret"].(string)
	if secret == "" || !strings.HasPrefix(setup["url"].(string), "otpauth://") {
		t.Fatalf("setup 应返回密钥与二维码地址: %v", setup)
	}
	if code, _ := call(t, srv, "POST", "/security/2fa/enable", map[string]string{"secret": secret, "code": "000000"}); code != 400 {
		t.Fatal("错误验证码不应启用")
	}
	if code, _ := call(t, srv, "POST", "/security/2fa/enable", map[string]string{"secret": secret, "code": totpNow(secret)}); code != 200 {
		t.Fatal("正确验证码应启用")
	}
	// 启用者当前令牌已视为验证；清空状态模拟新的登录令牌
	m.twofa.clear()
	if _, st := call(t, srv, "GET", "/security/check2fa", nil); st["need"] != true {
		t.Fatalf("应需要二次验证: %v", st)
	}
	if _, _, code := dialWS(t, srv, "/connections/t1/ws"); code != 403 {
		t.Fatalf("未验证应拒绝: %d", code)
	}
	code, body := call(t, srv, "POST", "/security/verify2fa", map[string]string{"code": "123456"})
	if code != 400 {
		t.Fatalf("错误验证码应失败: %d %v", code, body)
	}
	if code, _ := call(t, srv, "POST", "/security/verify2fa", map[string]string{"code": totpNow(secret)}); code != 200 {
		t.Fatal("正确验证码应通过")
	}
	if _, st := call(t, srv, "GET", "/security", nil); st["verified"] != true || st["twoFAEnable"] != true {
		t.Fatalf("验证状态不正确: %v", st)
	}
	ws, ctx, code2 := dialWS(t, srv, "/connections/t1/ws")
	if code2 != 101 {
		t.Fatalf("验证后应放行: %d", code2)
	}
	readBinaryUntil(t, ws, ctx, "welcome")

	// 过期后失效
	m.twofa.mu.Lock()
	for k := range m.twofa.verified {
		m.twofa.verified[k] = time.Now().Add(-time.Second)
	}
	m.twofa.mu.Unlock()
	if _, _, code := dialWS(t, srv, "/connections/t1/ws"); code != 403 {
		t.Fatalf("验证过期后应拒绝: %d", code)
	}
	// 关闭 2FA
	if code, _ := call(t, srv, "POST", "/security/2fa/disable", map[string]string{"code": "000000"}); code != 400 {
		t.Fatal("错误验证码不应关闭")
	}
	if code, _ := call(t, srv, "POST", "/security/2fa/disable", map[string]string{"code": totpNow(secret)}); code != 200 {
		t.Fatal("正确验证码应关闭")
	}
	if m.cfg.Security.TwoFAKey != "" || m.cfg.Security.TwoFAEnable {
		t.Fatal("关闭后应清除密钥")
	}
}

func TestTwoFALockout(t *testing.T) {
	m, srv := newModule(t)
	m.cfg.Security.TwoFAEnable = true
	m.cfg.Security.TwoFAKey = auth.NewTOTPSecret()
	for i := 0; i < maxVerifyFails; i++ {
		call(t, srv, "POST", "/security/verify2fa", map[string]string{"code": "000000"})
	}
	if code, _ := call(t, srv, "POST", "/security/verify2fa", map[string]string{"code": totpNow(m.cfg.Security.TwoFAKey)}); code != http.StatusTooManyRequests {
		t.Fatalf("连续失败后应被锁定: %d", code)
	}
}

// startSocks5 启动测试用 SOCKS5 代理（要求用户名密码），返回地址与记录的目标。
func startSocks5(t *testing.T, user, pass string) (string, *atomic.Value) {
	t.Helper()
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	target := &atomic.Value{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				h := make([]byte, 2)
				io.ReadFull(c, h)
				io.ReadFull(c, make([]byte, h[1]))
				_, _ = c.Write([]byte{5, 2})
				ah := make([]byte, 2)
				io.ReadFull(c, ah)
				u := make([]byte, ah[1])
				io.ReadFull(c, u)
				pl := make([]byte, 1)
				io.ReadFull(c, pl)
				p := make([]byte, pl[0])
				io.ReadFull(c, p)
				if string(u) != user || string(p) != pass {
					_, _ = c.Write([]byte{1, 1})
					return
				}
				_, _ = c.Write([]byte{1, 0})
				rh := make([]byte, 4)
				io.ReadFull(c, rh)
				var host string
				switch rh[3] {
				case 1:
					ip := make([]byte, 4)
					io.ReadFull(c, ip)
					host = net.IP(ip).String()
				case 3:
					l := make([]byte, 1)
					io.ReadFull(c, l)
					d := make([]byte, l[0])
					io.ReadFull(c, d)
					host = string(d)
				}
				pb := make([]byte, 2)
				io.ReadFull(c, pb)
				addr := net.JoinHostPort(host, strconv.Itoa(int(pb[0])<<8|int(pb[1])))
				target.Store(addr)
				up, err := net.Dial("tcp", addr)
				if err != nil {
					_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer up.Close()
				_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go io.Copy(up, c)
				io.Copy(c, up)
			}()
		}
	}()
	return ln.Addr().String(), target
}

// startHTTPProxy 启动测试用 HTTP CONNECT 代理。
func startHTTPProxy(t *testing.T, user, pass string) (string, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "仅支持 CONNECT", 405)
			return
		}
		u, p, ok := r.BasicAuth()
		if pa := r.Header.Get("Proxy-Authorization"); pa != "" {
			req, _ := http.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", pa)
			u, p, ok = req.BasicAuth()
		}
		if !ok || u != user || p != pass {
			http.Error(w, "需要认证", http.StatusProxyAuthRequired)
			return
		}
		up, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		hits.Add(1)
		hj := w.(http.Hijacker)
		c, _, _ := hj.Hijack()
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { io.Copy(up, c); up.Close() }()
		go func() { io.Copy(c, up); c.Close() }()
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), hits
}

func TestTelnetThroughProxies(t *testing.T) {
	f := startTelnetServer(t)
	m, srv := newModule(t)

	socksAddr, target := startSocks5(t, "u", "p")
	sh, sp, _ := net.SplitHostPort(socksAddr)
	telnetConn(t, m, "via-socks", f.addr, "root", "secret")
	m.mu.Lock()
	m.cfg.Connections[0].Telnet.Proxy = Proxy{Enabled: true, Type: ProxySOCKS5, Host: sh, Username: "u", Password: "p"}
	m.cfg.Connections[0].Telnet.Proxy.Port, _ = strconv.Atoi(sp)
	m.mu.Unlock()
	ws, ctx, _ := dialWS(t, srv, "/connections/via-socks/ws")
	readBinaryUntil(t, ws, ctx, "welcome")
	if target.Load() != f.addr {
		t.Fatalf("SOCKS5 应收到目标地址 %s: %v", f.addr, target.Load())
	}

	httpAddr, hits := startHTTPProxy(t, "hu", "hp")
	hh, hp, _ := net.SplitHostPort(httpAddr)
	telnetConn(t, m, "via-http", f.addr, "root", "secret")
	m.mu.Lock()
	idx := len(m.cfg.Connections) - 1
	m.cfg.Connections[idx].Telnet.Proxy = Proxy{Enabled: true, Type: ProxyHTTP, Host: hh, Username: "hu", Password: "hp"}
	m.cfg.Connections[idx].Telnet.Proxy.Port, _ = strconv.Atoi(hp)
	m.mu.Unlock()
	ws2, ctx2, _ := dialWS(t, srv, "/connections/via-http/ws")
	readBinaryUntil(t, ws2, ctx2, "welcome")
	if hits.Load() != 1 {
		t.Fatalf("HTTP 代理应被使用: %d", hits.Load())
	}

	// 代理认证失败：连接失败并提示
	m.mu.Lock()
	m.cfg.Connections[idx].Telnet.Proxy.Password = "bad"
	m.mu.Unlock()
	ws3, ctx3, _ := dialWS(t, srv, "/connections/via-http/ws")
	readUntil(t, ws3, ctx3, "连接失败")
}

func TestSSHThroughProxyAndOptions(t *testing.T) {
	keyPEM, pub := newKey(t)
	addr, _ := startSSHServer(t, pub)
	host, port, _ := net.SplitHostPort(addr)
	m, srv := newModule(t)
	socksAddr, target := startSocks5(t, "u", "p")
	sh, sp, _ := net.SplitHostPort(socksAddr)

	keyPath := filepath.Join(t.TempDir(), "id")
	_ = os.WriteFile(keyPath, []byte(keyPEM), 0o600)
	c := Connection{Key: "c1", Name: "nas", SSH: SSHConfig{Host: host, Username: "root", AuthType: "key", KeyFile: keyPath, WorkDir: "/data/it's", Timeout: 5,
		Proxy: Proxy{Enabled: true, Host: sh, Username: "u", Password: "p"}}}
	c.SSH.Port, _ = strconv.Atoi(port)
	c.SSH.Proxy.Port, _ = strconv.Atoi(sp)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Connections = []Connection{c}

	ws, ctx, _ := dialWS(t, srv, "/connections/c1/ws")
	readUntil(t, ws, ctx, "echo:cd '/data/it'\\''s'")
	if target.Load() != addr {
		t.Fatalf("SSH 应经 SOCKS5 连接 %s: %v", addr, target.Load())
	}
}

func TestDraftConnectionTest(t *testing.T) {
	f := startTelnetServer(t)
	keyPEM, pub := newKey(t)
	addr, _ := startSSHServer(t, pub)
	host, port, _ := net.SplitHostPort(addr)
	m, srv := newModule(t)
	tp, _ := strconv.Atoi(strings.Split(f.addr, ":")[1])

	code, out := call(t, srv, "POST", "/connections/test", Connection{Name: "t", Type: TypeTelnet, Telnet: TelnetConfig{Host: "127.0.0.1", Port: tp}})
	if code != 200 || out["ok"] != true {
		t.Fatalf("Telnet 测试应成功: %d %v", code, out)
	}
	if _, out := call(t, srv, "POST", "/connections/test", Connection{Name: "t", Type: TypeTelnet, Telnet: TelnetConfig{Host: "127.0.0.1", Port: 1, Timeout: 2}}); out["ok"] != false {
		t.Fatalf("不可达应失败: %v", out)
	}
	if code, _ := call(t, srv, "POST", "/connections/test", Connection{Name: "bad", Type: TypeTelnet}); code != 400 {
		t.Fatal("非法草稿应返回 400")
	}

	// 草稿用 key 指向已保存连接，脱敏的私钥由原连接合并
	pn, _ := strconv.Atoi(port)
	saved := Connection{Key: "c1", Name: "nas", SSH: SSHConfig{Host: host, Port: pn, Username: "root", AuthType: "key", PrivateKey: keyPEM}}
	_ = saved.Validate()
	m.cfg.Connections = []Connection{saved}
	draft := saved
	draft.SSH.PrivateKey = crud.SecretMask
	_, out = call(t, srv, "POST", "/connections/test", draft)
	if out["ok"] != true {
		t.Fatalf("应合并已保存的私钥: %v", out)
	}
	m.mu.Lock()
	recorded := m.cfg.Connections[0].SSH.HostKey
	m.mu.Unlock()
	if recorded != "" {
		t.Fatal("测试不应记录主机密钥")
	}
}

func TestMaskingAndKeepSecretForAllSecrets(t *testing.T) {
	m, srv := newModule(t)
	body := Connection{Name: "t", Type: TypeTelnet, Telnet: TelnetConfig{Host: "h", Password: "pw", Proxy: Proxy{Enabled: true, Host: "p", Password: "ppw"}}}
	code, out := call(t, srv, "POST", "/connections", body)
	if code != 200 {
		t.Fatalf("新增失败: %d %v", code, out)
	}
	key := out["key"].(string)
	tel := out["telnet"].(map[string]any)
	if tel["password"] != "********" || tel["proxy"].(map[string]any)["password"] != "********" {
		t.Fatalf("密码应脱敏: %v", tel)
	}
	if out["quickAccessDirs"] == nil {
		t.Fatal("quickAccessDirs 应输出空数组")
	}
	// 回传脱敏值：保留原密文
	body.Telnet.Password, body.Telnet.Proxy.Password = "********", "********"
	body.Remark = "改过"
	if code, _ := call(t, srv, "PUT", "/connections/"+key, body); code != 200 {
		t.Fatal("修改失败")
	}
	c, _ := m.findConn(key)
	if c.Telnet.Password != "pw" || c.Telnet.Proxy.Password != "ppw" || c.Remark != "改过" {
		t.Fatalf("应保留原密文: %+v", c.Telnet)
	}
	// 快速访问目录
	code, qa := call(t, srv, "PUT", "/connections/"+key+"/quickaccess", map[string]any{"dirs": []string{"/etc", " /etc ", "", "/var/log"}})
	if code != 200 || len(qa["dirs"].([]any)) != 2 {
		t.Fatalf("快速访问目录应去重去空: %d %v", code, qa)
	}
	if code, _ := call(t, srv, "PUT", "/connections/none/quickaccess", map[string]any{"dirs": []string{}}); code != 404 {
		t.Fatal("未知连接应返回 404")
	}
	// 修改连接但不带 quickAccessDirs 时沿用旧值
	if code, _ := call(t, srv, "PUT", "/connections/"+key, body); code != 200 {
		t.Fatal("再次修改失败")
	}
	c, _ = m.findConn(key)
	if len(c.QuickAccessDirs) != 2 {
		t.Fatalf("应沿用快速访问目录: %v", c.QuickAccessDirs)
	}
}

func TestSettingsShortcutsLayoutShells(t *testing.T) {
	m, srv := newModule(t)
	code, out := call(t, srv, "GET", "/settings", nil)
	s := out["settings"].(map[string]any)
	if code != 200 || s["bufferSize"] != float64(256) || s["heartbeatInterval"] != float64(30) || s["maxSessions"] != float64(20) {
		t.Fatalf("默认设置不正确: %v", out)
	}
	call(t, srv, "PUT", "/settings", Settings{MaxSessions: 3, BufferSize: 64, HeartbeatInterval: 10, IdleTimeout: 5, SessionKeepAlive: 7})
	if got := m.settingsSnapshot(); got.MaxSessions != 3 || got.BufferSize != 64 || got.SessionKeepAlive != 7 || got.IdleTimeout != 5 {
		t.Fatalf("设置未保存: %+v", got)
	}

	if code, _ := call(t, srv, "PUT", "/globalshortcuts", []Shortcut{{Name: "磁盘", Command: "df -h", SendEnter: true}}); code != 200 {
		t.Fatal("保存快捷命令失败")
	}
	if code, _ := call(t, srv, "PUT", "/globalshortcuts", []Shortcut{{Name: "", Command: "x"}}); code != 400 {
		t.Fatal("空名称应被拒绝")
	}
	m.mu.Lock()
	n := len(m.cfg.Shortcuts)
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("快捷命令数量: %d", n)
	}

	resp, _ := http.Get(srv.URL + "/api/webterminal/splitlayout")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.TrimSpace(string(b)) != "null" {
		t.Fatalf("空布局应为 null: %s", b)
	}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/webterminal/splitlayout", strings.NewReader(`{"mode":"grid","panes":[1,2]}`))
	req.Header.Set("Content-Type", "application/json")
	r2, _ := http.DefaultClient.Do(req)
	r2.Body.Close()
	resp, _ = http.Get(srv.URL + "/api/webterminal/splitlayout")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"grid"`) {
		t.Fatalf("布局应原样存取: %s", b)
	}
	call(t, srv, "DELETE", "/splitlayout", nil)
	resp, _ = http.Get(srv.URL + "/api/webterminal/splitlayout")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.TrimSpace(string(b)) != "null" {
		t.Fatalf("删除后应为 null: %s", b)
	}

	resp, _ = http.Get(srv.URL + "/api/webterminal/shells")
	var shells []string
	_ = json.NewDecoder(resp.Body).Decode(&shells)
	resp.Body.Close()
	if runtime.GOOS != "windows" && len(shells) == 0 {
		t.Fatal("应至少返回一个可用 Shell")
	}
}

func TestOldConfigCompat(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(dir)
	old := map[string]any{
		"settings":    map[string]any{"allowLocalShell": true, "maxSessions": 5},
		"connections": []map[string]any{{"key": "k", "name": "n", "type": "ssh", "enable": true, "ssh": map[string]any{"host": "h", "port": 22, "username": "u", "authType": "password", "password": "p"}}},
	}
	if err := st.Save(ID, old); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(10, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	s := m.cfg.Settings
	if !s.AllowLocalShell || s.MaxSessions != 5 || s.BufferSize != 256 || s.HeartbeatInterval != 30 || s.SessionKeepAlive != 0 {
		t.Fatalf("旧配置应保留已有字段并补默认值: %+v", s)
	}
	if m.cfg.Shortcuts == nil || len(m.cfg.Connections) != 1 || m.cfg.Connections[0].SSH.Password != "p" {
		t.Fatalf("旧连接应保持可用: %+v", m.cfg)
	}
}

func TestLocalShellEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("本机 Shell 仅支持 Unix")
	}
	m, srv := newModule(t)
	m.cfg.Settings.AllowLocalShell = true
	m.cfg.Connections = []Connection{{Key: "l", Name: "local", Type: TypeLocal, Local: LocalConfig{Shell: "/bin/sh", Env: map[string]string{"GATELY_T": "env-ok"}}}}
	ws, ctx, _ := dialWS(t, srv, "/connections/l/ws")
	sendInput(t, ws, ctx, "echo val-$GATELY_T\n")
	readBinaryUntil(t, ws, ctx, "val-env-ok")
}

func TestSSHPool(t *testing.T) {
	keyPEM, pub := newKey(t)
	addr, _ := startSSHServer(t, pub)
	host, port, _ := net.SplitHostPort(addr)
	m, _ := newModule(t)
	c := Connection{Key: "c1", Name: "nas", SSH: SSHConfig{Host: host, Username: "root", AuthType: "key", PrivateKey: keyPEM}}
	c.SSH.Port, _ = strconv.Atoi(port)
	_ = c.Validate()
	m.cfg.Connections = []Connection{c}
	old := poolIdleTimeout
	poolIdleTimeout = 100 * time.Millisecond
	defer func() { poolIdleTimeout = old }()

	ctx := context.Background()
	c1, rel1, err := m.acquireSSH(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	c2, rel2, err := m.acquireSSH(ctx, "c1")
	if err != nil || c1 != c2 {
		t.Fatalf("同一连接应复用: %v", err)
	}
	// 首次拨号会记录主机密钥
	m.mu.Lock()
	hk := m.cfg.Connections[0].SSH.HostKey
	m.mu.Unlock()
	if hk == "" {
		t.Fatal("池拨号应记录主机密钥")
	}
	rel1()
	time.Sleep(250 * time.Millisecond)
	if _, _, err := c1.SendRequest("keepalive@openssh.com", true, nil); err != nil {
		t.Fatal("仍有引用时连接不应被关闭:", err)
	}
	rel2()
	waitFor(t, "空闲后关闭", func() bool {
		_, _, err := c1.SendRequest("keepalive@openssh.com", true, nil)
		return err != nil
	})

	// 重新获取得到新连接；配置变更时清理
	c3, rel3, err := m.acquireSSH(ctx, "c1")
	if err != nil || c3 == c1 {
		t.Fatalf("空闲关闭后应重新拨号: %v", err)
	}
	m.pool.drop("c1")
	rel3()
	waitFor(t, "drop 后关闭", func() bool {
		_, _, err := c3.SendRequest("keepalive@openssh.com", true, nil)
		return err != nil
	})
	if _, _, err := m.acquireSSH(ctx, "nope"); err == nil {
		t.Fatal("未知连接应报错")
	}
}

func TestImportLuckyFull(t *testing.T) {
	raw := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	doc := luckyimport.Document{
		"connections": raw([]map[string]any{
			{"name": "router", "type": "telnet", "quickAccessDirs": []string{"/tmp"}, "telnetConfig": map[string]any{"host": "10.0.0.1", "port": 2323, "username": "admin", "password": "pw", "timeout": 8,
				"proxy": map[string]any{"enabled": true, "type": "http", "host": "127.0.0.1", "port": 3128}}},
			{"name": "box", "type": "ssh", "sshConfig": map[string]any{"host": "h", "username": "u", "authType": "privateKey", "keyFile": "/root/.ssh/id_rsa", "workDir": "/srv", "timeout": 9}},
			{"name": "sh", "type": "local", "localConfig": map[string]any{"shell": "/bin/zsh", "env": map[string]string{"A": "1"}}},
		}),
		"sessionConfig":   raw(map[string]any{"idleTimeout": 30, "bufferSize": 512, "heartbeatInterval": 20, "maxSessions": 8, "sessionKeepAlive": 10}),
		"globalShortcuts": raw([]map[string]any{{"name": "磁盘", "command": "df -h", "sendEnter": true}}),
		"securityConfig":  raw(map[string]any{"twoFAEnable": true, "twoFAKey": "JBSWY3DPEHPK3PXP", "userAgreementAccepted": true}),
	}
	m, _ := newModule(t)
	m.cfg.Security.AgreementAccepted = false
	rep, err := m.ImportLucky(doc, luckyimport.Options{DryRun: true})
	if err != nil || rep.Imported != 3 || len(m.cfg.Connections) != 0 || m.cfg.Security.TwoFAEnable {
		t.Fatalf("试运行不应修改配置: %+v %v", rep, err)
	}
	rep, err = m.ImportLucky(doc, luckyimport.Options{})
	if err != nil || rep.Found != 3 || rep.Imported != 3 {
		t.Fatalf("导入失败: %+v %v", rep, err)
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "暂不支持") {
			t.Errorf("Telnet 与代理现已支持，不应再提示不支持: %s", w)
		}
	}
	tn, ssh, lo := m.cfg.Connections[0], m.cfg.Connections[1], m.cfg.Connections[2]
	if tn.Telnet.Port != 2323 || tn.Telnet.Timeout != 8 || tn.Telnet.Proxy.Type != ProxyHTTP || tn.Telnet.Proxy.Port != 3128 || len(tn.QuickAccessDirs) != 1 {
		t.Fatalf("Telnet 字段映射错误: %+v", tn)
	}
	if ssh.SSH.AuthType != "key" || ssh.SSH.KeyFile != "/root/.ssh/id_rsa" || ssh.SSH.WorkDir != "/srv" || ssh.SSH.Timeout != 9 {
		t.Fatalf("SSH 字段映射错误: %+v", ssh.SSH)
	}
	if lo.Local.Env["A"] != "1" || lo.Local.Shell != "/bin/zsh" {
		t.Fatalf("本机 Shell 字段映射错误: %+v", lo.Local)
	}
	s := m.cfg.Settings
	if s.MaxSessions != 8 || s.BufferSize != 512 || s.IdleTimeout != 30 || s.HeartbeatInterval != 20 || s.SessionKeepAlive != 10 {
		t.Fatalf("会话设置未迁移: %+v", s)
	}
	if len(m.cfg.Shortcuts) != 1 || !m.cfg.Shortcuts[0].SendEnter {
		t.Fatalf("快捷命令未迁移: %+v", m.cfg.Shortcuts)
	}
	if !m.cfg.Security.AgreementAccepted || !m.cfg.Security.TwoFAEnable || m.cfg.Security.TwoFAKey != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("安全配置未迁移: %+v", m.cfg.Security)
	}
	// 再次导入：同名连接跳过，快捷命令不重复
	if _, err := m.ImportLucky(doc, luckyimport.Options{}); err != nil || len(m.cfg.Connections) != 3 || len(m.cfg.Shortcuts) != 1 {
		t.Fatalf("重复导入应去重: %v %d %d", err, len(m.cfg.Connections), len(m.cfg.Shortcuts))
	}
}

// 本机 Shell 运行在容器内时先提示不要在这里重建本容器，不在容器内时不提示
func TestLocalShellContainerNotice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("本机 Shell 仅支持 Unix")
	}
	marker := filepath.Join(t.TempDir(), ".dockerenv")
	old := containerMarkers
	t.Cleanup(func() { containerMarkers = old })
	m, srv := newModule(t)
	m.cfg.Settings.AllowLocalShell = true
	m.cfg.Connections = []Connection{{Key: "l", Name: "local", Type: TypeLocal, Local: LocalConfig{Shell: "/bin/sh"}}}
	for _, inside := range []bool{false, true} {
		containerMarkers = []string{marker}
		if inside {
			_ = os.WriteFile(marker, nil, 0o644)
		}
		ws, ctx, _ := dialWS(t, srv, "/connections/l/ws")
		var notes []string
		for {
			typ, b, err := ws.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var msg map[string]string
			_ = json.Unmarshal(b, &msg)
			if typ == websocket.MessageText && msg["t"] == "n" {
				notes = append(notes, msg["d"])
			}
			// 会话编号在提示之后发送
			if typ == websocket.MessageText && msg["t"] == "s" {
				break
			}
		}
		got := strings.Contains(strings.Join(notes, "\n"), "docker compose up -d")
		if got != inside {
			t.Fatalf("容器内=%v 时提示=%v: %q", inside, got, notes)
		}
		ws.CloseNow()
	}
}
