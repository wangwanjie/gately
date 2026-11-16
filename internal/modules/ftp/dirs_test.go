package ftp

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// 用户目录不能与配置目录重叠：保存时拒绝；升级前已保存的配置照常启动，但这个目录不再提供任何内容。
func TestUserDirsOutsideConfigDir(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
	share := filepath.Join(base, "share")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := config.Open(conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conf, "account.json"), []byte(`{"sessionSecret":"top-secret-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	old := Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, SharedUsers: true, Users: []User{
		{Name: "leak", Password: "p1", Root: conf},
		{Name: "multi", Password: "p2", Mounts: []mountfs.Mount{{Name: "ok", Path: share}, {Name: "up", Path: base}}},
		{Name: "fine", Password: "p3", Root: share},
	}}
	if err := st.Save(ID, old); err != nil {
		t.Fatal(err)
	}
	// 共享用户同样是升级前保存的
	if err := st.Save(fileusers.ID, map[string]any{"users": []fileusers.User{{Name: "shared", Password: "p4", Root: conf}}}); err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if m.err != "" {
		t.Fatalf("存量配置不应让服务启动失败: %s", m.err)
	}
	// login 以用户身份登录并返回其文件系统
	login := func(user, pass string) afero.Fs {
		t.Helper()
		drv, err := m.drv.AuthUser(&fakeClient{}, user, pass)
		if err != nil {
			t.Fatalf("%s 登录失败: %v", user, err)
		}
		return drv.(afero.Fs)
	}
	read := func(fs afero.Fs, name string) (string, error) {
		b, err := afero.ReadFile(fs, name)
		return string(b), err
	}
	for _, c := range [][3]string{
		{"leak", "p1", "/account.json"},
		{"shared", "p4", "/account.json"},
		{"multi", "p2", "/up/conf/account.json"},
		{"multi", "p2", "/up/share/hello.txt"},
	} {
		if body, err := read(login(c[0], c[1]), c[2]); err == nil || strings.Contains(body, "top-secret-value") {
			t.Fatalf("%s 读取 %s 不应成功: %q", c[0], c[2], body)
		}
	}
	// 写入同样无处落地
	if err := afero.WriteFile(login("leak", "p1"), "/x.json", []byte("{}"), 0o644); err == nil {
		t.Fatal("向被停止提供的目录写入不应成功")
	}
	if body, err := read(login("multi", "p2"), "/ok/hello.txt"); err != nil || body != "hello" {
		t.Fatalf("多目录用户的其他目录应照常可用: %q %v", body, err)
	}
	if body, err := read(login("fine", "p3"), "/hello.txt"); err != nil || body != "hello" {
		t.Fatalf("其他用户应照常可用: %q %v", body, err)
	}
	warned := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, "配置目录重叠") {
			warned++
		}
	}
	if warned != 2 {
		t.Fatalf("应为两个被停止提供的本地用户目录各写一条警告，实际 %d", warned)
	}

	// 保存时拒绝
	mux := http.NewServeMux()
	m.Routes(secretsRouter{mux})
	put := func(s Settings) (int, string) {
		b, _ := json.Marshal(s)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/settings", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	for name, u := range map[string]User{
		"配置目录":    {Name: "x", Password: "p", Root: conf},
		"上级目录":    {Name: "x", Password: "p", Root: base},
		"多目录中的一个": {Name: "x", Password: "p", Mounts: []mountfs.Mount{{Name: "a", Path: share}, {Name: "b", Path: filepath.Join(conf, "sub")}}},
	} {
		if code, body := put(Settings{ListenIP: "127.0.0.1", Port: port, Users: []User{u}}); code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("%s 应被拒绝: %d %s", name, code, body)
		}
	}
	if code, body := put(Settings{ListenIP: "127.0.0.1", Port: port, Users: []User{{Name: "x", Password: "p", Root: share}}}); code != 200 {
		t.Fatalf("无关目录应能保存: %d %s", code, body)
	}
}
