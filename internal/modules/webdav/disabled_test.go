package webdav

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestWebDAVDisabledUsers 验证停用的本地用户与共享用户即使密码正确也无法登录，停用的本地用户不回落到同名共享用户，
// 重新启用后恢复；共享用户停用立即生效，不需要重启服务。
func TestWebDAVDisabledUsers(t *testing.T) {
	dir := t.TempDir()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if err := fileusers.Default.Save([]fileusers.User{
		{Name: "sam", Password: "ps", Root: dir},
		{Name: "dup", Password: "shared", Root: dir},
	}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, SharedUsers: true, Users: []User{
		{Name: "on", Password: "p1", Root: dir},
		{Name: "off", Password: "p2", Root: dir, Disabled: true},
		{Name: "dup", Password: "local", Root: dir, Disabled: true},
	}}
	if err := m.settings.Validate(); err != nil {
		t.Fatal(err)
	}
	m.restartLocked()
	defer m.Stop()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	get := func(user, pass string) int {
		req, _ := http.NewRequest("PROPFIND", base+"/", nil)
		req.Header.Set("Depth", "0")
		req.SetBasicAuth(user, pass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := get("on", "p1"); c != 207 {
		t.Fatalf("启用的用户应能登录: %d", c)
	}
	if c := get("off", "p2"); c != 401 {
		t.Fatalf("停用的本地用户应被拒绝: %d", c)
	}
	// 停用的本地用户占用用户名，同名共享用户也不能登录
	if c := get("dup", "shared"); c != 401 {
		t.Fatalf("停用本地用户的同名共享用户应被拒绝: %d", c)
	}
	if c := get("dup", "local"); c != 401 {
		t.Fatalf("停用的本地用户应被拒绝: %d", c)
	}
	if c := get("sam", "ps"); c != 207 {
		t.Fatalf("启用的共享用户应能登录: %d", c)
	}
	// 停用共享用户立即生效，已缓存的处理器被丢弃
	if err := fileusers.Default.Save([]fileusers.User{{Name: "sam", Password: "ps", Root: dir, Disabled: true}}); err != nil {
		t.Fatal(err)
	}
	if c := get("sam", "ps"); c != 401 {
		t.Fatalf("停用的共享用户应被拒绝: %d", c)
	}
	// 重新启用后恢复登录
	if err := fileusers.Default.Save([]fileusers.User{{Name: "sam", Password: "ps", Root: dir}}); err != nil {
		t.Fatal(err)
	}
	if c := get("sam", "ps"); c != 207 {
		t.Fatalf("重新启用的共享用户应能登录: %d", c)
	}
}

// TestDisabledUsersCountAsUsers 验证停用的用户仍算作已添加的用户：全部停用时仍可启用服务，
// 只靠共享用户启用时也可以停用最后一个共享用户。
func TestDisabledUsersCountAsUsers(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	s := Settings{Enable: true, Users: []User{{Name: "a", Password: "p", Root: dir, Disabled: true}}}
	if err := s.Validate(); err != nil {
		t.Fatalf("全部本地用户停用时仍应可以启用: %v", err)
	}
	if err := fileusers.Default.Save([]fileusers.User{{Name: "s", Password: "p", Root: dir}}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.settings = Settings{Enable: true, SharedUsers: true}
	m.mu.Unlock()
	if err := fileusers.Default.Save([]fileusers.User{{Name: "s", Password: "p", Root: dir, Disabled: true}}); err != nil {
		t.Fatalf("停用最后一个共享用户不应被拦截: %v", err)
	}
	s = Settings{Enable: true, SharedUsers: true}
	if err := s.Validate(); err != nil {
		t.Fatalf("共享用户全部停用时仍应可以启用: %v", err)
	}
	// 旧配置没有 disabled 字段，读取后为启用
	var u User
	if err := json.Unmarshal([]byte(`{"name":"x","password":"p","root":"/a"}`), &u); err != nil || u.Disabled {
		t.Fatalf("旧配置应视为启用: %+v %v", u, err)
	}
}
