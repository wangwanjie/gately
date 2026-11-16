package webdav

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/fsperm/fspermtest"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

type dirsRouter struct{ *http.ServeMux }

func (r dirsRouter) HandleFunc(p string, h http.HandlerFunc) { r.ServeMux.HandleFunc(p, h) }

// startDAV 以 l 中保存的设置 s 启动 WebDAV 模块（模拟升级前已保存的配置）。
func startDAV(t *testing.T, l fspermtest.Layout, users []User) (*Module, string, *logbuf.Buffer) {
	t.Helper()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if err := l.Store.Save(ID, Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: users}); err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	m := New(nil)
	if err := m.Init(module.Env{Store: l.Store, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if m.err != "" {
		t.Fatalf("服务应正常启动: %s", m.err)
	}
	return m, "http://127.0.0.1:" + strconv.Itoa(port), logs
}

// dav 以用户 user 发出 WebDAV 请求，返回状态码与正文。
func dav(t *testing.T, base, user, method, p string, hdr map[string]string, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, base+p, strings.NewReader(body))
	req.SetBasicAuth(user, "pw")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func logCount(logs *logbuf.Buffer, sub string) int {
	n := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, sub) {
			n++
		}
	}
	return n
}

// 用户目录就是配置目录或位于其中：保存时拒绝；升级前已保存的照常启动，但这个目录不再提供任何内容。
func TestUserDirsInsideConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	m, base, logs := startDAV(t, l, []User{
		{Name: "leak", Password: "pw", Root: l.Conf},
		{Name: "multi", Password: "pw", Mounts: []mountfs.Mount{{Name: "ok", Path: l.Media}, {Name: "cfg", Path: filepath.Join(l.Conf, "history")}}},
	})
	for _, c := range [][2]string{{"leak", "/account.json"}, {"leak", "/"}, {"multi", "/cfg/"}} {
		if code, body := dav(t, base, c[0], "GET", c[1], nil, ""); code == 200 || strings.Contains(body, fspermtest.Secret) {
			t.Fatalf("%s 读取 %s 不应成功: %d %s", c[0], c[1], code, body)
		}
	}
	if code, body := dav(t, base, "multi", "GET", "/ok/hello.txt", nil, ""); code != 200 || body != "hello" {
		t.Fatalf("同一用户的其他目录应照常可用: %d %s", code, body)
	}
	if n := logCount(logs, "配置目录重叠"); n != 2 {
		t.Fatalf("应为两个被停止提供的目录各写一条警告，实际 %d", n)
	}
	mux := http.NewServeMux()
	m.Routes(dirsRouter{mux})
	put := func(u User) (int, string) {
		b, _ := json.Marshal(Settings{ListenIP: "127.0.0.1", Port: 1, Users: []User{u}})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/settings", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	for name, u := range map[string]User{
		"配置目录":    {Name: "x", Password: "p", Root: l.Conf},
		"配置目录内":   {Name: "x", Password: "p", Root: filepath.Join(l.Conf, "history")},
		"多目录中的一个": {Name: "x", Password: "p", Mounts: []mountfs.Mount{{Name: "a", Path: l.Media}, {Name: "b", Path: l.Conf}}},
	} {
		if code, body := put(u); code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("%s 应被拒绝: %d %s", name, code, body)
		}
	}
	if code, body := put(User{Name: "x", Password: "p", Root: l.Share}); code != 200 {
		t.Fatalf("包含配置目录的目录应能保存: %d %s", code, body)
	}
}

// 用户目录包含配置目录（整个数据盘）：照常提供，配置目录这棵子树不可见、不可达，改名、复制都进出不了。
func TestUserDirsContainingConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	_, base, logs := startDAV(t, l, []User{
		{Name: "all", Password: "pw", Root: l.Share},
		{Name: "multi", Password: "pw", Mounts: []mountfs.Mount{{Name: "vol", Path: l.Share}}},
	})
	if logCount(logs, "配置目录已自动隐藏") != 2 {
		t.Fatal("应说明两个目录中的配置目录已自动隐藏")
	}
	for user, prefix := range map[string]string{"all": "", "multi": "/vol"} {
		for p, want := range l.VisibleFiles() {
			if code, body := dav(t, base, user, "GET", prefix+"/"+p, nil, ""); code != 200 || body != want {
				t.Errorf("%s 读取 %s 应成功: %d %s", user, p, code, body)
			}
		}
		for _, p := range l.HiddenFiles() {
			if code, body := dav(t, base, user, "GET", prefix+"/"+p, nil, ""); code == 200 || strings.Contains(body, fspermtest.Secret) {
				t.Errorf("%s 读取 %s 不应成功: %d %s", user, p, code, body)
			}
		}
		for _, p := range l.HiddenDirs() {
			if code, body := dav(t, base, user, "PROPFIND", prefix+"/"+p+"/", map[string]string{"Depth": "1"}, ""); code == 207 || strings.Contains(body, "account.json") {
				t.Errorf("%s 列出 %s 不应成功: %d", user, p, code)
			}
		}
		for dir, names := range l.Listings() {
			code, body := dav(t, base, user, "PROPFIND", prefix+"/"+dir, map[string]string{"Depth": "1"}, "")
			if code != 207 {
				t.Fatalf("%s 列出 /%s 失败: %d", user, dir, code)
			}
			for name, want := range names {
				got := strings.Contains(body, "/"+name+"/<") || strings.Contains(body, "/"+name+"<")
				if got != want {
					t.Errorf("%s /%s 的列表中 %s 出现=%v，期望 %v", user, dir, name, got, want)
				}
			}
		}
		// 递归列出整个共享目录（Depth: infinity）同样不含配置目录中的文件
		if code, body := dav(t, base, user, "PROPFIND", prefix+"/", map[string]string{"Depth": "infinity"}, ""); code != 207 || strings.Contains(body, "account.json") || !strings.Contains(body, "keep.txt") {
			t.Errorf("%s 递归列出时不应出现配置目录中的文件: %d", user, code)
		}
		for _, r := range l.Renames() {
			if code, _ := dav(t, base, user, "MOVE", prefix+"/"+r[0], map[string]string{"Destination": base + prefix + "/" + r[1]}, ""); code < 400 {
				t.Errorf("%s MOVE %s → %s 应被拒绝: %d", user, r[0], r[1], code)
			}
		}
		for _, r := range [][2]string{{"1000/docker/gately/account.json", "media/copy.json"}, {"media/hello.txt", "1000/docker/gately/hello.txt"}, {"1000/docker", "media/docker-copy"}} {
			if code, _ := dav(t, base, user, "COPY", prefix+"/"+r[0], map[string]string{"Destination": base + prefix + "/" + r[1]}, ""); code < 400 && r[0] != "1000/docker" {
				t.Errorf("%s COPY %s → %s 应被拒绝: %d", user, r[0], r[1], code)
			}
		}
		// 复制配置目录的上级目录时，副本里没有配置目录
		if code, body := dav(t, base, user, "GET", prefix+"/media/docker-copy/gately/account.json", nil, ""); code == 200 || strings.Contains(body, fspermtest.Secret) {
			t.Errorf("%s 复制上级目录不应带出配置目录: %d", user, code)
		}
		for _, p := range l.Removals() {
			if code, _ := dav(t, base, user, "DELETE", prefix+"/"+p, nil, ""); code < 400 {
				t.Errorf("%s 删除 %s 应被拒绝: %d", user, p, code)
			}
		}
		for _, p := range l.Creates() {
			if code, _ := dav(t, base, user, "PUT", prefix+"/"+p, nil, "x"); code < 400 {
				t.Errorf("%s PUT %s 应被拒绝: %d", user, p, code)
			}
			if code, _ := dav(t, base, user, "MKCOL", prefix+"/"+p+"-dir", nil, ""); code < 400 {
				t.Errorf("%s MKCOL %s 应被拒绝: %d", user, p, code)
			}
		}
		if code, _ := dav(t, base, user, "PUT", prefix+"/media/new.txt", nil, "n"); code >= 300 {
			t.Fatalf("%s 写普通文件应成功: %d", user, code)
		}
		if code, _ := dav(t, base, user, "DELETE", prefix+"/media/new.txt", nil, ""); code >= 300 {
			t.Fatalf("%s 删除普通文件应成功: %d", user, code)
		}
	}
	l.Check(t)
}
