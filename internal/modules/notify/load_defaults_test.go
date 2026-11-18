package notify

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的配置文件。
func initFromRaw(t *testing.T, raw map[string]any) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// pipeSMTP 用内存管道代替网络连接，模拟支持 STARTTLS 的邮件服务器，记录拨号地址与收到的命令。
type pipeSMTP struct {
	mu   sync.Mutex
	addr string
	cmds []string
}

func (f *pipeSMTP) dial(_ context.Context, _, addr string, _ time.Duration) (net.Conn, error) {
	f.mu.Lock()
	f.addr = addr
	f.mu.Unlock()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		w := bufio.NewWriter(server)
		r := bufio.NewReader(server)
		reply := func(s string) {
			_, _ = w.WriteString(s + "\r\n")
			_ = w.Flush()
		}
		reply("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.Fields(strings.TrimSpace(line) + " x")[0])
			f.mu.Lock()
			f.cmds = append(f.cmds, cmd)
			f.mu.Unlock()
			switch cmd {
			case "EHLO":
				reply("250-fake")
				reply("250-STARTTLS")
				reply("250 AUTH PLAIN")
			case "STARTTLS":
				// 只记录客户端发起了 STARTTLS，随后断开，不做真实握手
				reply("454 TLS not available in test")
				return
			default:
				reply("250 ok")
			}
		}
	}()
	return client, nil
}

func (f *pipeSMTP) snapshot() (string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addr, append([]string(nil), f.cmds...)
}

// 配置文件缺邮件渠道的加密方式与端口、缺规则最低级别时加载后补上默认值，否则会以明文连到端口 0。
func TestInitFillsMissingChannelDefaults(t *testing.T) {
	m := initFromRaw(t, map[string]any{
		"channels": []any{
			map[string]any{"key": "mail", "name": "mail", "enable": true, "kind": "smtp", "host": "smtp.example.com", "target": "a@example.com", "user": "u", "secret": "p"},
			map[string]any{"key": "tls", "name": "tls", "enable": true, "kind": "smtp", "host": "smtp.example.com", "security": "tls", "target": "a@example.com", "user": "u"},
		},
		"rules": []any{map[string]any{"key": "r1", "name": "all", "enable": true, "events": []string{"*"}, "channels": []string{"mail"}}},
	})
	ch := m.cfg.Channels[0]
	if ch.Security != SecurityStartTLS || ch.Port != 587 {
		t.Fatalf("邮件渠道未补默认值: security=%q port=%d", ch.Security, ch.Port)
	}
	if p := m.cfg.Channels[1].Port; p != 465 {
		t.Fatalf("TLS 邮件渠道默认端口应为 465: %d", p)
	}
	if lv := m.cfg.Rules[0].MinLevel; lv != events.LevelInfo {
		t.Fatalf("规则最低级别未补默认值: %q", lv)
	}

	// 运行时：连到 587 端口并先发起 STARTTLS，不会在明文连接上发送密码
	f := &pipeSMTP{}
	old := smtpDial
	smtpDial = f.dial
	t.Cleanup(func() { smtpDial = old })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := send(ctx, ch, Message{Title: "t", Text: "x"}); err == nil {
		t.Fatal("测试服务器拒绝 STARTTLS，发送应失败")
	}
	addr, cmds := f.snapshot()
	if addr != "smtp.example.com:587" {
		t.Fatalf("拨号地址不符: %q", addr)
	}
	for _, c := range cmds {
		if c == "AUTH" {
			t.Fatalf("不应在明文连接上认证: %v", cmds)
		}
	}
	if len(cmds) < 2 || cmds[len(cmds)-1] != "STARTTLS" {
		t.Fatalf("应先发起 STARTTLS: %v", cmds)
	}
}

// 已有的值保持不变；非邮件渠道不补邮件参数。
func TestInitKeepsExistingChannelValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{
		"channels": []any{
			map[string]any{"key": "mail", "name": "mail", "enable": true, "kind": "smtp", "host": "smtp.example.com", "security": "none", "port": 2525, "target": "a@example.com", "from": "x@example.com"},
			map[string]any{"key": "tg", "name": "tg", "enable": true, "kind": "telegram", "secret": "t", "target": "1"},
		},
		"rules": []any{map[string]any{"key": "r1", "name": "err", "enable": true, "events": []string{"*"}, "minLevel": "error", "channels": []string{"mail"}}},
	})
	if ch := m.cfg.Channels[0]; ch.Security != SecurityNone || ch.Port != 2525 {
		t.Fatalf("已有值被改动: security=%q port=%d", ch.Security, ch.Port)
	}
	if tg := m.cfg.Channels[1]; tg.Security != "" || tg.Port != 0 {
		t.Fatalf("非邮件渠道不应补邮件参数: %+v", tg)
	}
	if lv := m.cfg.Rules[0].MinLevel; lv != events.LevelError {
		t.Fatalf("规则最低级别被改动: %q", lv)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameChannelDefaults(t *testing.T) {
	for _, c := range []struct {
		security string
		port     int
	}{{"", 587}, {SecurityStartTLS, 587}, {SecurityTLS, 465}, {SecurityNone, 25}} {
		ch := Channel{Name: "s", Kind: KindSMTP, Host: "h", Target: "a@b.c", User: "u", Security: c.security}
		if err := ch.Validate(); err != nil {
			t.Fatal(err)
		}
		if ch.Port != c.port || ch.Security == "" {
			t.Fatalf("加密方式 %q 的保存默认值不符: %+v", c.security, ch)
		}
	}
	bad := Channel{Name: "s", Kind: KindSMTP, Host: "h", Target: "a@b.c", User: "u", Security: "ssl3"}
	if err := bad.Validate(); err == nil {
		t.Fatal("未知加密方式应被拒绝")
	}
	r := Rule{Name: "r", Events: []string{"*"}, Channels: []string{"c"}}
	if err := r.Validate(); err != nil || r.MinLevel != events.LevelInfo {
		t.Fatalf("规则保存时默认级别不符: %q %v", r.MinLevel, err)
	}
}
