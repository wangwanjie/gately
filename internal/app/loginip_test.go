package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

type newIPCollector struct {
	mu  sync.Mutex
	got []events.Event
}

func (c *newIPCollector) add(e events.Event) {
	if e.Type != events.LoginNewIP {
		return
	}
	c.mu.Lock()
	c.got = append(c.got, e)
	c.mu.Unlock()
}

// wait 等待异步投递的事件数达到 n，超时返回当前数量；n 为 0 时多等一会确认没有事件。
func (c *newIPCollector) wait(n int) []events.Event {
	deadline := time.Now().Add(time.Second)
	if n == 0 {
		time.Sleep(150 * time.Millisecond)
	}
	for time.Now().Before(deadline) {
		c.mu.Lock()
		l := len(c.got)
		c.mu.Unlock()
		if l >= n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]events.Event(nil), c.got...)
}

func newLoginIPApp(t *testing.T, dir string) *App {
	t.Helper()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if _, err := a.auth.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	return a
}

// 首次登录只建立基线，已知地址不重复告警，新地址发布 login.newip，重启后记录仍然有效。
func TestNoteLoginIPEvent(t *testing.T) {
	c := &newIPCollector{}
	defer events.Subscribe(c.add)()
	dir := t.TempDir()

	a := newLoginIPApp(t, dir)
	a.noteLoginIP("1.2.3.4", "admin", "密码")
	if n := len(c.wait(0)); n != 0 {
		t.Fatalf("首次登录不应告警: %d", n)
	}
	a.noteLoginIP("1.2.3.4", "admin", "密码")
	a.noteLoginIP("2001:db8:1:2::1", "admin", "Passkey")
	a.noteLoginIP("2001:db8:1:2::2", "admin", "Passkey")
	got := c.wait(1)
	if len(got) != 1 || got[0].Fields["ip"] != "2001:db8:1:2::1" || got[0].Level != events.LevelWarn {
		t.Fatalf("只应为新的 IPv6 /64 告警一次: %+v", got)
	}

	// 文件落盘且权限收紧，重启后已知地址不再告警，未见过的地址仍告警
	st, err := os.Stat(filepath.Join(dir, loginIPsName+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("文件权限应为 0600: %v", st.Mode().Perm())
	}
	b := newLoginIPApp(t, dir)
	b.noteLoginIP("1.2.3.4", "admin", "密码")
	b.noteLoginIP("9.9.9.9", "admin", "密码")
	got = c.wait(2)
	if len(got) != 2 || got[1].Fields["ip"] != "9.9.9.9" {
		t.Fatalf("重启后应只为 9.9.9.9 告警: %+v", got)
	}
}

// 损坏的记录文件不阻止启动，下次登录重新建立基线。
func TestLoginIPsCorruptFileStillStarts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, loginIPsName+".json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newLoginIPApp(t, dir)
	a.noteLoginIP("1.2.3.4", "admin", "密码")
}

// 经 HTTP 登录接口：错误密码不记录来源，成功登录才记录，并且不影响登录响应。
func TestLoginHandlerRecordsOnlySuccess(t *testing.T) {
	dir := t.TempDir()
	a := newLoginIPApp(t, dir)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	post := func(pw string) int {
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": pw})
		resp, err := http.Post(srv.URL+"/api/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if post("wrong-password") != http.StatusUnauthorized {
		t.Fatal("错误密码应 401")
	}
	if n := len(a.loginIPs.Known()); n != 0 {
		t.Fatalf("失败的登录不应记录来源: %d", n)
	}
	if post(auth.DefaultPassword) != http.StatusOK {
		t.Fatal("正确密码应登录成功")
	}
	known := a.loginIPs.Known()
	if len(known) != 1 || known[0].Key != "127.0.0.1" {
		t.Fatalf("成功登录应记录来源: %+v", known)
	}
}
