package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
)

// stubHostname 在测试期间替换主机名查询。
func stubHostname(t *testing.T, name string, err error) {
	t.Helper()
	old := hostname
	hostname = func() (string, error) { return name, err }
	t.Cleanup(func() { hostname = old })
}

func TestNewMessageServerName(t *testing.T) {
	msg := NewMessage(events.Event{Type: events.DDNSChanged, Title: "解析已更新", Level: events.LevelInfo}, "fnOS-B760")
	if !strings.HasPrefix(msg.Title, "[Gately · fnOS-B760] ") {
		t.Fatalf("标题应带服务器名称: %q", msg.Title)
	}
	if !strings.Contains(msg.Text, "fnOS-B760") {
		t.Fatalf("正文应带服务器名称: %q", msg.Text)
	}
	if msg.vars()["server"] != "fnOS-B760" {
		t.Fatalf("Webhook 变量 server 不符: %q", msg.vars()["server"])
	}
	// 没有服务器名称时保持原来的格式
	msg = NewMessage(events.Event{Type: events.DDNSChanged, Title: "解析已更新", Level: events.LevelInfo}, "")
	if !strings.HasPrefix(msg.Title, "[Gately] ") {
		t.Fatalf("无名称时标题格式不符: %q", msg.Title)
	}
}

func TestServerNameFallback(t *testing.T) {
	stubHostname(t, "aliyun-hk\n", nil)
	if got := serverName(Settings{ServerName: "hk_main"}); got != "hk_main" {
		t.Fatalf("设置的名称优先: %q", got)
	}
	if got := serverName(Settings{}); got != "aliyun-hk" {
		t.Fatalf("留空应使用主机名: %q", got)
	}
	stubHostname(t, "", errors.New("no hostname"))
	if got := serverName(Settings{}); got != "" {
		t.Fatalf("取不到主机名时应为空: %q", got)
	}
}

func TestSettingsValidateServerName(t *testing.T) {
	s := Settings{ServerName: "  vmiss-us  "}
	if err := s.Validate(); err != nil || s.ServerName != "vmiss-us" {
		t.Fatalf("应去掉首尾空白: %q %v", s.ServerName, err)
	}
	for _, bad := range []string{"a\r\nBcc: x@y", strings.Repeat("名", maxServerNameLen+1)} {
		s := Settings{ServerName: bad}
		if err := s.Validate(); err == nil {
			t.Fatalf("应拒绝服务器名称 %q", bad)
		}
	}
}

func TestDeliverUsesServerName(t *testing.T) {
	stubHostname(t, "host-a", nil)
	m, _ := newTestModule(fileConfig{
		Settings: Settings{ServerName: "fnOS-C246"},
		Channels: []Channel{{Key: "a", Name: "A", Enable: true}},
		Rules:    []Rule{{Key: "r", Name: "r", Enable: true, Events: []string{"waf.*"}, Channels: []string{"a"}}},
	})
	m.log = nil
	var title string
	m.sender = func(_ context.Context, _ Channel, msg Message) error {
		title = msg.Title
		return nil
	}
	m.handleSync(events.Event{Type: events.WAFBlocked, Level: events.LevelWarn, Time: time.Now()})
	if !strings.HasPrefix(title, "[Gately · fnOS-C246] ") {
		t.Fatalf("投递的标题应带设置的服务器名称: %q", title)
	}
}
