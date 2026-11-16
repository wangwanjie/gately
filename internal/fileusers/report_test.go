package fileusers

import (
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
)

// collectFileAuth 订阅默认总线上的文件服务登录失败事件。
func collectFileAuth(t *testing.T) <-chan events.Event {
	t.Helper()
	ch := make(chan events.Event, 16)
	cancel := events.Subscribe(func(e events.Event) {
		if e.Type == events.FileAuthFailed {
			ch <- e
		}
	})
	t.Cleanup(cancel)
	return ch
}

func expectEvent(t *testing.T, ch <-chan events.Event) events.Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("应发布登录失败事件")
	}
	return events.Event{}
}

func expectNoEvent(t *testing.T, ch <-chan events.Event) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatalf("不应发布事件: %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestReportAuthFailure(t *testing.T) {
	ResetFailureReports()
	now := time.Unix(1000, 0)
	reportNow = func() time.Time { return now }
	t.Cleanup(func() { reportNow = time.Now; ResetFailureReports() })
	ch := collectFileAuth(t)

	ReportAuthFailure("WebDAV", "::ffff:198.51.100.7", strings.Repeat("u", 100))
	e := expectEvent(t, ch)
	if e.Fields["ip"] != "198.51.100.7" || e.Fields["service"] != "WebDAV" || e.Source != "WebDAV" || e.Level != events.LevelWarn {
		t.Fatalf("事件字段不正确: %+v", e)
	}
	if len([]rune(e.Fields["user"])) > 65 {
		t.Fatalf("用户名应截断: %d", len(e.Fields["user"]))
	}
	// 同一来源 5 秒内只发布一次，不论哪个服务
	ReportAuthFailure("FTP", "198.51.100.7", "x")
	expectNoEvent(t, ch)
	// 其他来源不受影响
	ReportAuthFailure("SMB", "2001:db8::1", "")
	if e := expectEvent(t, ch); e.Fields["ip"] != "2001:db8::1" || e.Fields["user"] != "" {
		t.Fatalf("事件字段不正确: %+v", e)
	}
	now = now.Add(reportInterval)
	ReportAuthFailure("FTP", "198.51.100.7", "x")
	if e := expectEvent(t, ch); e.Fields["service"] != "FTP" {
		t.Fatalf("间隔过后应再次发布: %+v", e)
	}
	// 无法解析的地址不发布
	ReportAuthFailure("FTP", "not-an-ip", "x")
	expectNoEvent(t, ch)
}
