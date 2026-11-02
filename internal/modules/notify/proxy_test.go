package notify

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/proxyx"
	"github.com/wangwanjie/gately/internal/webhook"
)

func TestPlanUsesEffectiveProxy(t *testing.T) {
	m := New()
	m.cfg = fileConfig{
		Settings: Settings{Proxy: "http://10.0.0.1:3128"},
		Channels: []Channel{
			{Key: "a", Name: "A", Enable: true},
			{Key: "b", Name: "B", Enable: true, Proxy: proxyx.Direct},
			{Key: "c", Name: "C", Enable: true, Proxy: proxyx.Update},
			{Key: "d", Name: "D", Enable: true, Proxy: "socks5://10.0.0.2:1080"},
		},
		Rules: []Rule{{Key: "r", Name: "r", Enable: true, Events: []string{"*"}, Channels: []string{"a", "b", "c", "d"}}},
	}
	got := map[string]string{}
	for _, d := range m.plan(events.Event{Type: events.WAFBlocked, Level: events.LevelWarn}) {
		got[d.ch.Key] = d.ch.Proxy
	}
	want := map[string]string{"a": "http://10.0.0.1:3128", "b": proxyx.Direct, "c": proxyx.Update, "d": "socks5://10.0.0.2:1080"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("渠道 %s 的代理为 %q，应为 %q", k, got[k], v)
		}
	}
	// 合并只发生在副本上，保存的配置不变
	if m.cfg.Channels[0].Proxy != "" {
		t.Fatalf("plan 不应修改保存的渠道配置")
	}
}

func TestSettingsAndChannelProxyValidate(t *testing.T) {
	s := Settings{Proxy: " Direct "}
	if err := s.Validate(); err != nil || s.Proxy != "" {
		t.Fatalf("全局设置的 direct 应归一为留空，得到 %q, %v", s.Proxy, err)
	}
	s = Settings{Proxy: " UPDATE "}
	if err := s.Validate(); err != nil || s.Proxy != proxyx.Update {
		t.Fatalf("update 应保留，得到 %q, %v", s.Proxy, err)
	}
	for _, bad := range []string{"127.0.0.1:7890", "ftp://x", "http://h/path"} {
		s = Settings{Proxy: bad}
		if s.Validate() == nil {
			t.Errorf("全局代理 %q 应被拒绝", bad)
		}
		ch := Channel{Name: "x", Kind: KindSlack, URL: "https://hooks.slack.com/x", Proxy: bad}
		if ch.Validate() == nil {
			t.Errorf("渠道代理 %q 应被拒绝", bad)
		}
	}
	// Webhook 渠道只认渠道级代理，Webhook 自身的代理字段被清空
	ch := Channel{Name: "w", Kind: KindWebhook, Proxy: "direct", Webhook: webhook.Config{URL: "https://example.com/h", Proxy: "http://1.2.3.4:8080"}}
	if err := ch.Validate(); err != nil || ch.Webhook.Proxy != "" || ch.Proxy != proxyx.Direct {
		t.Fatalf("渠道代理校验结果不符: %+v, %v", ch, err)
	}
}

// recordingProxy 是记录请求的 HTTP 正向代理。
func recordingProxy(t *testing.T, hits *atomic.Int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		out, _ := http.NewRequest(r.Method, r.URL.String(), r.Body)
		out.Header = r.Header.Clone()
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
	return srv.URL
}

func TestHTTPChannelsUseProxy(t *testing.T) {
	var got atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		got.Add(1)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer target.Close()
	var hits atomic.Int32
	proxy := recordingProxy(t, &hits)

	chans := []Channel{
		{Kind: KindTelegram, URL: target.URL, Secret: "T", Target: "1", Proxy: proxy},
		{Kind: KindNtfy, URL: target.URL, Target: "topic", Proxy: proxy},
		{Kind: KindWebhook, Webhook: webhook.Config{URL: target.URL + "/hook", Method: "POST", Body: `{"t":"{title}"}`}, Proxy: proxy},
	}
	for i, ch := range chans {
		if err := send(context.Background(), ch, testMsg()); err != nil {
			t.Fatalf("%s 发送失败: %v", ch.Kind, err)
		}
		if int(hits.Load()) != i+1 || int(got.Load()) != i+1 {
			t.Fatalf("%s 应经代理发送：代理 %d 次，目标 %d 次", ch.Kind, hits.Load(), got.Load())
		}
	}
	// 直连时不经过代理
	ch := Channel{Kind: KindTelegram, URL: target.URL, Secret: "T", Target: "1", Proxy: proxyx.Direct}
	if err := send(context.Background(), ch, testMsg()); err != nil || hits.Load() != 3 {
		t.Fatalf("direct 不应经过代理: %v，代理 %d 次", err, hits.Load())
	}
}

// fakeSMTP 是只支持明文会话的最小 SMTP 服务，返回收到的邮件正文。
func fakeSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	mail := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		br := bufio.NewReader(c)
		say := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
		say("220 fake ESMTP")
		var data strings.Builder
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				say("250 fake")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				say("354 go")
				for {
					l, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				mail <- data.String()
				say("250 queued")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("502 unknown")
			}
		}
	}()
	return ln.Addr().String(), mail
}

// connectProxy 是支持 CONNECT 的最小代理，记录请求的目标地址。
func connectProxy(t *testing.T, targets chan<- string) string {
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
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil || req.Method != http.MethodConnect {
					return
				}
				targets <- req.Host
				up, err := net.Dial("tcp", req.Host)
				if err != nil {
					_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
					return
				}
				defer up.Close()
				_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
				go func() { _, _ = io.Copy(up, br) }()
				_, _ = io.Copy(c, up)
			}()
		}
	}()
	return "http://" + ln.Addr().String()
}

func TestSMTPThroughProxy(t *testing.T) {
	addr, mail := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	targets := make(chan string, 1)
	proxy := connectProxy(t, targets)
	ch := Channel{Kind: KindSMTP, Host: host, Security: SecurityNone, From: "a@example.com", Target: "b@example.com", Proxy: proxy}
	ch.Port, _ = strconv.Atoi(port)
	if err := send(context.Background(), ch, testMsg()); err != nil {
		t.Fatalf("经代理发送邮件失败: %v", err)
	}
	select {
	case got := <-targets:
		if got != addr {
			t.Fatalf("CONNECT 目标为 %s，应为 %s", got, addr)
		}
	default:
		t.Fatalf("邮件没有经过代理")
	}
	select {
	case body := <-mail:
		if !strings.Contains(body, "Subject:") {
			t.Fatalf("邮件内容不完整: %q", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("SMTP 服务没有收到邮件")
	}
}

func TestSettingsAPI(t *testing.T) {
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(routerFunc(func(p string, h http.HandlerFunc) { mux.HandleFunc(p, h) }))
	do := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	if w := do(http.MethodPut, `{"proxy":"127.0.0.1:7890"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "代理地址") {
		t.Fatalf("无效代理应返回 400，得到 %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPut, `{"proxy":"socks5://127.0.0.1:1080"}`); w.Code != http.StatusOK {
		t.Fatalf("保存设置失败: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, ""); !strings.Contains(w.Body.String(), "socks5://127.0.0.1:1080") {
		t.Fatalf("读取设置不符: %s", w.Body.String())
	}
	// 重新加载后设置仍在
	m2 := New()
	if err := m2.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if m2.cfg.Settings.Proxy != "socks5://127.0.0.1:1080" {
		t.Fatalf("设置未持久化: %+v", m2.cfg.Settings)
	}
}

type routerFunc func(pattern string, h http.HandlerFunc)

func (f routerFunc) HandleFunc(pattern string, h http.HandlerFunc) { f(pattern, h) }
