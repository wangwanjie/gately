package webdav

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

type dirsRouter struct{ *http.ServeMux }

func (r dirsRouter) HandleFunc(p string, h http.HandlerFunc) { r.ServeMux.HandleFunc(p, h) }

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
	const secret = `{"sessionSecret":"top-secret-value"}`
	if err := os.WriteFile(filepath.Join(conf, "account.json"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	// 升级前保存的配置：一个用户的目录就是配置目录，一个多目录用户挂了包含配置目录的上级目录
	old := Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{
		{Name: "leak", Password: "p1", Root: conf},
		{Name: "multi", Password: "p2", Mounts: []mountfs.Mount{{Name: "ok", Path: share}, {Name: "up", Path: base}}},
		{Name: "fine", Password: "p3", Root: share},
	}}
	if err := st.Save(ID, old); err != nil {
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
	get := func(path, user, pass string) (int, string) {
		req, _ := http.NewRequest("GET", "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
		req.SetBasicAuth(user, pass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	for _, c := range [][3]string{
		{"/account.json", "leak", "p1"},
		{"/up/conf/account.json", "multi", "p2"},
		{"/up/share/hello.txt", "multi", "p2"},
	} {
		code, body := get(c[0], c[1], c[2])
		if code == 200 || strings.Contains(body, "top-secret-value") {
			t.Fatalf("%s 以 %s 读取 %s 不应成功: %d %s", "WebDAV", c[1], c[0], code, body)
		}
	}
	// 其余目录与用户不受影响
	if code, body := get("/ok/hello.txt", "multi", "p2"); code != 200 || body != "hello" {
		t.Fatalf("多目录用户的其他目录应照常可用: %d %s", code, body)
	}
	if code, body := get("/hello.txt", "fine", "p3"); code != 200 || body != "hello" {
		t.Fatalf("其他用户应照常可用: %d %s", code, body)
	}
	warned := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, "配置目录重叠") {
			warned++
		}
	}
	if warned != 2 {
		t.Fatalf("应为两个被停止提供的目录各写一条警告，实际 %d", warned)
	}

	// 保存时拒绝
	mux := http.NewServeMux()
	m.Routes(dirsRouter{mux})
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
