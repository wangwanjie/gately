package webdav

import (
	"io"
	"log"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestAuthFailureEvent：带错误凭据的请求发布文件服务登录失败事件供入侵防护计数；不带凭据的质询请求与登录成功不发布。
func TestAuthFailureEvent(t *testing.T) {
	fileusers.ResetFailureReports()
	t.Cleanup(fileusers.ResetFailureReports)
	ch := make(chan events.Event, 8)
	cancel := events.Subscribe(func(e events.Event) {
		if e.Type == events.FileAuthFailed {
			ch <- e
		}
	})
	defer cancel()
	m := &Module{handlers: map[string]*userHandler{}, logs: logbuf.New(20, log.New(io.Discard, "", 0))}
	m.handlers["u"] = newUserHandler(User{Name: "u", Password: "good", Root: t.TempDir()})
	do := func(remote, user, pass string) {
		req := httptest.NewRequest("PROPFIND", "/", nil)
		req.RemoteAddr = remote
		if user != "" {
			req.SetBasicAuth(user, pass)
		}
		m.serve(httptest.NewRecorder(), req)
	}
	do("198.51.100.20:1000", "", "")
	do("198.51.100.21:1000", "u", "good")
	select {
	case e := <-ch:
		t.Fatalf("质询请求与登录成功不应发布事件: %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
	do("198.51.100.22:1000", "u", "bad")
	select {
	case e := <-ch:
		if e.Fields["ip"] != "198.51.100.22" || e.Fields["service"] != "WebDAV" || e.Fields["user"] != "u" {
			t.Fatalf("事件字段不正确: %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("错误密码应发布登录失败事件")
	}
}
