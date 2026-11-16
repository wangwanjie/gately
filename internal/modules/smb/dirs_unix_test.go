//go:build linux || darwin

package smb

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

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// 用户目录不能与配置目录重叠：保存时拒绝；升级前已保存的配置照常启动，但这个共享不再提供任何内容。
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
	old := Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{
		{Name: "leak", Password: "p1", Root: conf},
		{Name: "multi", Password: "p2", Mounts: []mountfs.Mount{{Name: "ok", Path: share}, {Name: "up", Path: base}}},
	}}
	if err := st.Save(ID, old); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(200, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if m.loadErr != nil {
		t.Fatalf("存量配置不应让整个服务失效: %v", m.loadErr)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if m.err != "" {
		t.Fatalf("存量配置不应让服务启动失败: %s", m.err)
	}
	if !hasLog(m, "用户 leak 的目录 "+conf+" 与 Gately 的配置目录重叠") || !hasLog(m, "用户 multi 的目录 "+base+" 与 Gately 的配置目录重叠") {
		t.Fatal("被停止提供的目录应写入警告日志")
	}

	// 配置目录所在的共享里读不到任何文件
	sess, err := dial(t, port, "leak", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if fs, err := sess.Mount("leak"); err == nil {
		if b, err := fs.ReadFile("account.json"); err == nil {
			t.Fatalf("不应读到配置目录中的文件: %s", b)
		}
		_ = fs.Umount()
	}
	sess, err = dial(t, port, "multi", "p2")
	if err != nil {
		t.Fatal(err)
	}
	if fs, err := sess.Mount("up"); err == nil {
		for _, name := range []string{`conf\account.json`, `share\hello.txt`} {
			if b, err := fs.ReadFile(name); err == nil {
				t.Fatalf("不应经上级目录读到 %s: %s", name, b)
			}
		}
		_ = fs.Umount()
	}
	// 同一用户的其他共享照常可用
	fs, err := sess.Mount("ok")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile("hello.txt"); err != nil || string(b) != "hello" {
		t.Fatalf("其他共享应照常可用: %q %v", b, err)
	}
	_ = fs.Umount()

	// 保存时拒绝
	mux := http.NewServeMux()
	m.Routes(muxRouter{mux})
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
