package webterminal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// startExecServer 启动只处理 exec 请求的 SSH 服务端，按命令模拟不同结果：
// echo 输出参数并退出 0；fail 向 stderr 输出 boom 并退出 1；big 输出 100 KB；drop 不返回退出码直接断开（模拟关机）；
// hang 一直不返回。conns 统计 TCP 连接数。
func startExecServer(t *testing.T, clientPub ssh.PublicKey, conns *atomic.Int32) string {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hostPriv)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(k.Marshal(), clientPub.Marshal()) {
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
			conns.Add(1)
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, err := nch.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range creqs {
							if req.Type != "exec" || len(req.Payload) < 4 {
								_ = req.Reply(false, nil)
								continue
							}
							_ = req.Reply(true, nil)
							cmd := string(req.Payload[4:])
							status := func(code byte) {
								_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, code})
								_ = ch.Close()
							}
							switch {
							case strings.HasPrefix(cmd, "echo "):
								_, _ = io.WriteString(ch, strings.TrimPrefix(cmd, "echo ")+"\n")
								status(0)
							case cmd == "fail":
								_, _ = io.WriteString(ch.Stderr(), "boom")
								status(1)
							case cmd == "big":
								_, _ = ch.Write(bytes.Repeat([]byte("x"), 100<<10))
								status(0)
							case cmd == "both":
								// stdout 与 stderr 交替大量写入，客户端两路复制会并发写同一个缓冲
								for i := 0; i < 200; i++ {
									_, _ = io.WriteString(ch, "out-line\n")
									_, _ = io.WriteString(ch.Stderr(), "err-line\n")
								}
								status(0)
							case cmd == "drop":
								_ = ch.Close()
							case cmd == "hang":
							}
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func newExecEnv(t *testing.T) (*Module, *atomic.Int32) {
	t.Helper()
	keyPEM, pub := newKey(t)
	var conns atomic.Int32
	addr := startExecServer(t, pub, &conns)
	host, port, _ := net.SplitHostPort(addr)
	m, _ := newModule(t)
	c := Connection{Key: "s", Name: "pc", SSH: SSHConfig{Host: host, Username: "root", AuthType: "key", PrivateKey: keyPEM}}
	c.SSH.Port, _ = strconv.Atoi(port)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Connections = []Connection{c}
	return m, &conns
}

func TestExec(t *testing.T) {
	m, conns := newExecEnv(t)
	ctx := context.Background()
	if _, err := m.Exec(ctx, "s", "echo hi"); err == nil || !strings.Contains(err.Error(), "主机密钥") {
		t.Fatalf("未记录主机密钥时应拒绝执行: %v", err)
	}
	if conns.Load() != 0 {
		t.Fatal("未记录主机密钥时不应连接")
	}
	// 模拟在 Web 终端中连接过一次：记录主机密钥
	_, release, err := m.acquireSSH(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	release()
	before := conns.Load()
	out, err := m.Exec(ctx, "s", "echo hi")
	if err != nil || out != "hi\n" {
		t.Fatalf("执行结果错误: %q %v", out, err)
	}
	// 每次执行都新建连接，不复用连接池中可能已失效的连接
	if _, err := m.Exec(ctx, "s", "echo again"); err != nil || conns.Load() != before+2 {
		t.Fatalf("应每次新建连接: conns=%d before=%d err=%v", conns.Load(), before, err)
	}
	out, err = m.Exec(ctx, "s", "fail")
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitStatus() != 1 || out != "boom" {
		t.Fatalf("失败的命令应返回退出码与输出: %q %v", out, err)
	}
	out, err = m.Exec(ctx, "s", "big")
	if err != nil || len(out) != execOutputLimit {
		t.Fatalf("输出应截断到上限: %d %v", len(out), err)
	}
	out, err = m.Exec(ctx, "s", "both")
	if err != nil || strings.Count(out, "-line\n") != 400 {
		t.Fatalf("stdout 与 stderr 应都写入输出: %d %v", strings.Count(out, "-line\n"), err)
	}
	_, err = m.Exec(ctx, "s", "drop")
	var missing *ssh.ExitMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("连接在返回退出码前断开应可识别: %v", err)
	}
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := m.Exec(tctx, "s", "hang"); err == nil || !strings.Contains(err.Error(), "超时") || time.Since(start) > 5*time.Second {
		t.Fatalf("应按上下文超时返回: %v", err)
	}
	if _, err := m.Exec(ctx, "s", "  "); err == nil {
		t.Fatal("空命令应报错")
	}
	if _, err := m.Exec(ctx, "missing", "echo hi"); err == nil {
		t.Fatal("不存在的连接应报错")
	}
}

// TestExecHandshakeDeadline 验证目标只接受 TCP 连接、不回应 SSH 握手时（例如正在重启），
// Exec 按调用方的截止时间返回，而不是等到连接超时（最长 120 秒）。
func TestExecHandshakeDeadline(t *testing.T) {
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
			// 不发送 SSH 版本标识，测试结束时随监听关闭
			t.Cleanup(func() { nc.Close() })
		}
	}()
	keyPEM, _ := newKey(t)
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	m, _ := newModule(t)
	c := Connection{Key: "s", Name: "pc", SSH: SSHConfig{Host: host, Username: "root", AuthType: "key", PrivateKey: keyPEM, Timeout: 120}}
	c.SSH.Port, _ = strconv.Atoi(port)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.SSH.HostKey = "SHA256:recorded"
	m.cfg.Connections = []Connection{c}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := m.Exec(ctx, "s", "echo hi"); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("握手卡住时应按上下文截止时间返回: %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("返回过慢: %v", d)
	}
}
