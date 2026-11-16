package ftp

import (
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestAuthFailureEvent：FTP 密码错误与失败过多被拒绝都发布文件服务登录失败事件，登录成功不发布。
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
	d := &driver{s: Settings{}, users: map[string]User{"rw": {Name: "rw", Password: "p1", Root: t.TempDir()}},
		logs: logbuf.New(20, log.New(io.Discard, "", 0)), throttle: &fileusers.Throttle{}}
	if _, err := d.AuthUser(&fakeClient{addr: &net.TCPAddr{IP: net.ParseIP("198.51.100.30"), Port: 1000}}, "rw", "p1"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		t.Fatalf("登录成功不应发布事件: %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := d.AuthUser(&fakeClient{addr: &net.TCPAddr{IP: net.ParseIP("198.51.100.31"), Port: 1000}}, "rw", "bad"); err == nil {
		t.Fatal("错误密码应登录失败")
	}
	select {
	case e := <-ch:
		if e.Fields["ip"] != "198.51.100.31" || e.Fields["service"] != "FTP" || e.Fields["user"] != "rw" {
			t.Fatalf("事件字段不正确: %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("错误密码应发布登录失败事件")
	}
}
