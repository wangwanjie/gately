package storage

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

type testRouter struct{ *http.ServeMux }

func (r testRouter) HandleFunc(p string, h http.HandlerFunc) { r.ServeMux.HandleFunc(p, h) }

// 本地挂载点不能与配置目录重叠：新增、修改时拒绝；升级前已保存的挂载点不再提供访问。
func TestLocalMountOutsideConfigDir(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
	share := filepath.Join(base, "share")
	for _, d := range []string{filepath.Join(conf, "history"), share} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
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
	// 升级前保存的挂载点
	if err := st.Save(ID, fileConfig{Mounts: []Mount{
		{Key: "old1", Name: "配置", Type: TypeLocal, Enable: true, Path: conf},
		{Key: "old2", Name: "上级", Type: TypeLocal, Enable: true, Path: base},
		{Key: "fine", Name: "共享", Type: TypeLocal, Enable: true, Path: share},
	}}); err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatalf("存量配置不应让模块初始化失败: %v", err)
	}
	defer m.Stop()
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	do := func(method, path string, body any) (int, string) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = strings.NewReader(string(b))
		}
		req := httptest.NewRequest(method, path, rd)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	warned := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, "配置目录重叠") {
			warned++
		}
	}
	if warned != 2 {
		t.Fatalf("应为两个重叠的挂载点各写一条警告，实际 %d", warned)
	}
	for _, p := range []string{
		"/files/old1/download?path=/account.json",
		"/files/old1/list?path=/",
		"/files/old2/download?path=/conf/account.json",
		"/files/old2/list?path=/conf",
		"/files/old2/download?path=/share/hello.txt",
	} {
		if code, body := do("GET", p, nil); code != 400 || strings.Contains(body, "top-secret-value") || !strings.Contains(body, "配置目录") {
			t.Fatalf("%s 应被拒绝: %d %s", p, code, body)
		}
	}
	if code, body := do("POST", "/files/old1/mkdir", map[string]string{"path": "/x"}); code != 400 {
		t.Fatalf("向重叠的挂载点写入应被拒绝: %d %s", code, body)
	}
	if err := m.PutFile("old1", "", "x.json", []byte("{}")); err == nil {
		t.Fatal("备份等内部调用同样不能写入重叠的挂载点")
	}
	if code, body := do("GET", "/files/fine/download?path=/hello.txt", nil); code != 200 || body != "hello" {
		t.Fatalf("其他挂载点应照常可用: %d %s", code, body)
	}

	// 新增与修改
	cases := map[string]string{
		"配置目录":      conf,
		"配置目录内":     filepath.Join(conf, "history"),
		"包含配置目录":    base,
		"含 .. 的写法":  filepath.Join(share, "..", "conf"),
		"末尾带分隔符的写法": conf + string(filepath.Separator),
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(base, "link")
		if err := os.Symlink(conf, link); err != nil {
			t.Fatal(err)
		}
		cases["指向配置目录的符号链接"] = link
		up := filepath.Join(base, "uplink")
		if err := os.Symlink(base, up); err != nil {
			t.Fatal(err)
		}
		cases["经符号链接进入配置目录"] = filepath.Join(up, "conf", "history")
	}
	for name, p := range cases {
		mt := map[string]any{"name": "x", "type": TypeLocal, "enable": true, "path": p}
		if code, body := do("POST", "/mounts", mt); code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("新增 %s（%s）应被拒绝: %d %s", name, p, code, body)
		}
		if code, body := do("PUT", "/mounts/fine", mt); code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("把挂载点改为 %s（%s）应被拒绝: %d %s", name, p, code, body)
		}
		if code, body := do("POST", "/mounts/fine/copy", mt); code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("复制为 %s（%s）应被拒绝: %d %s", name, p, code, body)
		}
	}
	if code, body := do("POST", "/mounts", map[string]any{"name": "ok", "type": TypeLocal, "enable": true, "path": share}); code != 200 {
		t.Fatalf("无关目录应能新增: %d %s", code, body)
	}
}
