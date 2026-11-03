package notify

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/proxyx"
)

// TestAllHTTPKindsUseProxy 确认每种 HTTP 渠道都经渠道的代理发送（发送函数统一从 context 取客户端）。
func TestAllHTTPKindsUseProxy(t *testing.T) {
	var got atomic.Int32
	// 路径以 /c200 开头的按“业务码 200”返回（Bark、PushPlus），其余返回业务码 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		if strings.HasPrefix(r.URL.Path, "/c200") {
			_, _ = w.Write([]byte(`{"code":200}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"errcode":0,"status":1}`))
	}))
	defer target.Close()
	var hits atomic.Int32
	proxy := recordingProxy(t, &hits)
	c0 := target.URL + "/c0"
	chans := []Channel{
		{Kind: KindTelegram, URL: c0, Secret: "T", Target: "1"},
		{Kind: KindBark, URL: target.URL + "/c200", Secret: "K"},
		{Kind: KindWeCom, URL: c0},
		{Kind: KindDingTalk, URL: c0, Secret: "S"},
		{Kind: KindFeishu, URL: c0, Secret: "S"},
		{Kind: KindNtfy, URL: c0, Target: "topic"},
		{Kind: KindGotify, URL: c0, Secret: "G"},
		{Kind: KindPushover, URL: c0, Secret: "P", Target: "U"},
		{Kind: KindDiscord, URL: c0},
		{Kind: KindSlack, URL: c0},
		{Kind: KindServerChan, URL: c0, Secret: "S"},
		{Kind: KindPushPlus, URL: target.URL + "/c200", Secret: "P"},
	}
	for _, ch := range chans {
		before := hits.Load()
		ch.Proxy = proxy
		if err := send(context.Background(), ch, testMsg()); err != nil {
			t.Fatalf("%s 发送失败: %v", ch.Kind, err)
		}
		if hits.Load() != before+1 {
			t.Fatalf("%s 应经代理发送一次，代理收到 %d 次", ch.Kind, hits.Load()-before)
		}
		// 同一渠道改为直连后不经过代理
		ch.Proxy = proxyx.Direct
		if err := send(context.Background(), ch, testMsg()); err != nil {
			t.Fatalf("%s 直连发送失败: %v", ch.Kind, err)
		}
		if hits.Load() != before+1 {
			t.Fatalf("%s 直连时不应经过代理", ch.Kind)
		}
	}
	if int(got.Load()) != len(chans)*2 {
		t.Fatalf("目标应收到 %d 次请求，实际 %d", len(chans)*2, got.Load())
	}
}

// TestSMTPReceivesEffectiveProxy 确认邮件渠道把合并后的代理设置交给拨号函数。
func TestSMTPReceivesEffectiveProxy(t *testing.T) {
	old := smtpDial
	t.Cleanup(func() { smtpDial = old })
	var seen []string
	smtpDial = func(ctx context.Context, setting, addr string, timeout time.Duration) (net.Conn, error) {
		seen = append(seen, setting+"|"+addr)
		return nil, io.ErrUnexpectedEOF
	}
	m := New()
	m.cfg = fileConfig{
		Settings: Settings{Proxy: proxyx.Update},
		Channels: []Channel{
			{Key: "a", Name: "A", Enable: true, Kind: KindSMTP, Host: "smtp.example.com", Port: 465},
			{Key: "b", Name: "B", Enable: true, Kind: KindSMTP, Host: "smtp.example.com", Port: 25, Proxy: proxyx.Direct},
		},
	}
	for _, ch := range m.cfg.Channels {
		c := ch
		c.Proxy = effectiveProxy(c, m.cfg.Settings)
		_ = send(context.Background(), c, testMsg())
	}
	want := []string{"update|smtp.example.com:465", "direct|smtp.example.com:25"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("SMTP 拨号收到的代理设置不符: %v，期望 %v", seen, want)
	}
}

// TestHandleTestUsesGlobalProxy 确认“发送测试通知”同样合并全局代理：渠道留空跟随全局，direct 强制直连。
func TestHandleTestUsesGlobalProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer target.Close()
	var hits atomic.Int32
	proxy := recordingProxy(t, &hits)

	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cfg.Settings = Settings{Proxy: proxy}
	m.cfg.Channels = []Channel{
		{Key: "follow", Name: "跟随", Enable: true, Kind: KindSlack, URL: target.URL},
		{Key: "direct", Name: "直连", Enable: true, Kind: KindSlack, URL: target.URL, Proxy: proxyx.Direct},
	}
	m.mu.Unlock()
	mux := http.NewServeMux()
	m.Routes(routerFunc(func(p string, h http.HandlerFunc) { mux.HandleFunc(p, h) }))
	test := func(key string) int {
		req := httptest.NewRequest(http.MethodPost, "/channels/"+key+"/test", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	if code := test("follow"); code != http.StatusOK || hits.Load() != 1 {
		t.Fatalf("跟随全局的渠道应经全局代理发送测试通知：%d，代理 %d 次", code, hits.Load())
	}
	if code := test("direct"); code != http.StatusOK || hits.Load() != 1 {
		t.Fatalf("direct 渠道发送测试通知不应经过代理：%d，代理 %d 次", code, hits.Load())
	}
	// 测试发送不改写保存的渠道配置
	m.mu.Lock()
	saved := m.cfg.Channels[0].Proxy
	m.mu.Unlock()
	if saved != "" {
		t.Fatalf("测试发送不应把全局代理写入渠道配置，得到 %q", saved)
	}
}
