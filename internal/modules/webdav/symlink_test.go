package webdav

import (
	"io"
	"log"
	"net"
	"net/http"
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

// TestSymlinkEscape：共享目录内指向目录外的符号链接，经 WebDAV 读、写、列目录、移动、复制都不能越出；
// 指向共享目录内的链接照常可用。单目录与多目录两种用户都检查。
func TestSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root, out := filepath.Join(base, "root"), filepath.Join(base, "out")
	_ = os.MkdirAll(root, 0o755)
	_ = os.MkdirAll(out, 0o755)
	_ = os.WriteFile(filepath.Join(out, "secret.txt"), []byte("top-secret"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o644)
	if err := os.Symlink(filepath.Join(out, "secret.txt"), filepath.Join(root, "link-file")); err != nil {
		t.Skip(err)
	}
	_ = os.Symlink(out, filepath.Join(root, "link-dir"))
	_ = os.Symlink("inside.txt", filepath.Join(root, "link-in"))

	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{
		{Name: "single", Password: "p", Root: root},
		{Name: "multi", Password: "p", Mounts: []mountfs.Mount{{Name: "m", Path: root}}},
	}}
	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	defer m.Stop()
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)

	for _, c := range []struct{ user, prefix string }{{"single", ""}, {"multi", "/m"}} {
		do := func(method, path, body string, hdr map[string]string) (int, string) {
			req, _ := http.NewRequest(method, baseURL+c.prefix+path, strings.NewReader(body))
			req.SetBasicAuth(c.user, "p")
			for k, v := range hdr {
				req.Header.Set(k, strings.ReplaceAll(v, "{base}", baseURL+c.prefix))
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return resp.StatusCode, string(b)
		}
		if code, body := do("GET", "/link-file", "", nil); code < 400 || strings.Contains(body, "top-secret") {
			t.Fatalf("%s: 不应能读取目录外的文件: %d %q", c.user, code, body)
		}
		if code, body := do("GET", "/link-dir/secret.txt", "", nil); code < 400 || strings.Contains(body, "top-secret") {
			t.Fatalf("%s: 不应能经目录链接读取目录外的文件: %d", c.user, code)
		}
		if code, _ := do("PUT", "/link-dir/new.txt", "x", nil); code < 400 {
			t.Fatalf("%s: 不应能在目录外建文件: %d", c.user, code)
		}
		if code, _ := do("PUT", "/link-file", "overwrite", nil); code < 400 {
			t.Fatalf("%s: 不应能覆盖目录外的文件: %d", c.user, code)
		}
		if code, body := do("PROPFIND", "/link-dir", "", map[string]string{"Depth": "1"}); code == http.StatusMultiStatus && strings.Contains(body, "secret.txt") {
			t.Fatalf("%s: 不应能列出目录外的目录", c.user)
		}
		// 根目录列表照常返回，但不包含越出的链接
		code, body := do("PROPFIND", "/", "", map[string]string{"Depth": "1"})
		if code != http.StatusMultiStatus || !strings.Contains(body, "inside.txt") || !strings.Contains(body, "link-in") {
			t.Fatalf("%s: 根目录列表应正常: %d", c.user, code)
		}
		if strings.Contains(body, "link-file") || strings.Contains(body, "link-dir") {
			t.Fatalf("%s: 根目录列表不应包含指向目录外的链接", c.user)
		}
		if code, _ := do("COPY", "/link-file", "", map[string]string{"Destination": "{base}/copied.txt"}); code < 400 {
			t.Fatalf("%s: 不应能复制目录外的文件: %d", c.user, code)
		}
		if code, _ := do("COPY", "/inside.txt", "", map[string]string{"Destination": "{base}/link-dir/copied.txt"}); code < 400 {
			t.Fatalf("%s: 不应能复制到目录外: %d", c.user, code)
		}
		if code, _ := do("MOVE", "/link-dir/secret.txt", "", map[string]string{"Destination": "{base}/stolen.txt"}); code < 400 {
			t.Fatalf("%s: 不应能把目录外的文件移入: %d", c.user, code)
		}
		if code, _ := do("MOVE", "/inside.txt", "", map[string]string{"Destination": "{base}/link-dir/moved.txt"}); code < 400 {
			t.Fatalf("%s: 不应能把文件移到目录外: %d", c.user, code)
		}
		if code, body := do("GET", "/link-in", "", nil); code != 200 || body != "inside" {
			t.Fatalf("%s: 目录内的链接应可读: %d %q", c.user, code, body)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(out, "secret.txt")); string(b) != "top-secret" {
		t.Fatalf("目录外的文件被改动: %q", b)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 1 {
		t.Fatalf("目录外出现了新内容: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(root, "inside.txt")); err != nil {
		t.Fatal("目录内的文件不应被移走")
	}
}
