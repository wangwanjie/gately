package notify

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestSendErrorRedactsSecret 覆盖渠道密钥放在请求地址里的情况（Telegram Bot Token、Bark 设备 Key 等）：
// 连接失败时 *url.Error 带出完整地址，密钥不能进入投递记录、待补推队列（接口与 pending.json）和模块日志。
func TestSendErrorRedactsSecret(t *testing.T) {
	// 取一个已关闭的本机端口，连接必然被拒绝
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	_ = ln.Close()

	const secret = "123456789:AAHsecretTokenValue_x"
	logs := logbuf.New(100, nil)
	m := New()
	m.log = logs.Logger("通知")
	m.pend.path = filepath.Join(t.TempDir(), "notify", "pending.json")
	m.cfg = fileConfig{
		Settings: Settings{Retries: intp(0)},
		Channels: []Channel{
			{Key: "tg", Name: "TG", Kind: KindTelegram, Enable: true, URL: base, Secret: secret, Target: "1"},
			{Key: "bark", Name: "Bark", Kind: KindBark, Enable: true, URL: base, Secret: secret},
		},
		Rules: []Rule{{Key: "r", Name: "r", Enable: true, Events: []string{"*"}, MinLevel: events.LevelInfo, Channels: []string{"tg", "bark"}}},
	}
	m.handleSync(events.Event{Type: events.WAFBlocked, Level: events.LevelWarn, Time: time.Now()})

	m.mu.Lock()
	hist := append([]Record(nil), m.history...)
	m.mu.Unlock()
	if len(hist) != 2 {
		t.Fatalf("应有 2 条投递记录，实际 %d", len(hist))
	}
	for _, rec := range hist {
		if rec.OK || rec.Error == "" {
			t.Fatalf("投递应失败: %+v", rec)
		}
		if strings.Contains(rec.Error, "AAHsecret") {
			t.Fatalf("投递记录的错误信息泄露了渠道密钥: %s", rec.Error)
		}
	}
	m.pend.mu.Lock()
	items := append([]pendingItem(nil), m.pend.items...)
	m.pend.mu.Unlock()
	if len(items) != 2 {
		t.Fatalf("应有 2 条待补推消息，实际 %d", len(items))
	}
	for _, it := range items {
		if strings.Contains(it.Error, "AAHsecret") {
			t.Fatalf("待补推消息的错误信息泄露了渠道密钥: %s", it.Error)
		}
	}
	data, err := os.ReadFile(m.pend.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "AAHsecret") {
		t.Fatalf("pending.json 泄露了渠道密钥: %s", data)
	}
	for _, e := range logs.Range(0, 0, 100) {
		if strings.Contains(e.Message, "AAHsecret") {
			t.Fatalf("模块日志泄露了渠道密钥: %s", e.Message)
		}
	}
}
