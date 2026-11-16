package webservice

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// 静态站点的目录不能与配置目录重叠：保存时拒绝；升级前已保存的站点不再提供任何文件。
func TestStaticRootOutsideConfigDir(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
	www := filepath.Join(base, "www")
	for _, d := range []string{filepath.Join(conf, "history"), www} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 打开配置存储即把目录登记为受保护目录
	if _, err := config.Open(conf); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conf, "account.json"), []byte(`{"sessionSecret":"top-secret-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(www, "index.html"), []byte("home"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 保存时拒绝：站点与默认站点都经过 Validate
	for name, root := range map[string]string{
		"配置目录":     conf,
		"配置目录内":    filepath.Join(conf, "history"),
		"包含配置目录":   base,
		"含 .. 的写法": filepath.Join(www, "..", "conf"),
	} {
		l := Listener{Name: "t", ListenIP: "127.0.0.1", Network: "tcp4", Port: 18080,
			Sites: []Site{{Enable: true, Domains: []string{"s.test"}, Type: TypeStatic, Root: root}}}
		if err := l.Validate(); err == nil || !strings.Contains(err.Error(), "配置目录") {
			t.Errorf("站点目录为 %s（%s）应被拒绝，实际 %v", name, root, err)
		}
		l = Listener{Name: "t", ListenIP: "127.0.0.1", Network: "tcp4", Port: 18080,
			Default: Site{Enable: true, Type: TypeStatic, Root: root, ListDir: true}}
		if err := l.Validate(); err == nil || !strings.Contains(err.Error(), "配置目录") {
			t.Errorf("默认站点目录为 %s（%s）应被拒绝，实际 %v", name, root, err)
		}
	}
	ok := Listener{Name: "t", ListenIP: "127.0.0.1", Network: "tcp4", Port: 18080,
		Sites: []Site{{Enable: true, Domains: []string{"s.test"}, Type: TypeStatic, Root: www}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("无关目录应能保存: %v", err)
	}

	// 升级前保存的站点不经过 Validate，直接按存量配置建立处理器
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	for _, s := range []Site{
		{Key: "a", Name: "漏", Enable: true, Type: TypeStatic, Root: conf, ListDir: true},
		{Key: "b", Name: "上级", Enable: true, Type: TypeStatic, Root: base, ListDir: true, CleanURLs: true},
	} {
		h := &siteHandler{site: s, log: logs.Logger("t")}
		handler := h.newStatic(s)
		for _, p := range []string{"/", "/account.json", "/conf/account.json", "/www/index.html", "/account"} {
			resp := serveOnce(handler, p)
			if resp.code != 403 || strings.Contains(resp.body, "top-secret-value") || strings.Contains(resp.body, "account.json") {
				t.Fatalf("站点 %s 请求 %s 应返回 403 且不含目录内容: %d %s", s.Name, p, resp.code, resp.body)
			}
		}
	}
	warned := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, "配置目录重叠") {
			warned++
		}
	}
	if warned != 2 {
		t.Fatalf("应为两个站点各写一条警告，实际 %d", warned)
	}
	h := &siteHandler{log: logs.Logger("t")}
	if resp := serveOnce(h.newStatic(Site{Type: TypeStatic, Root: www}), "/index.html"); resp.code != 301 && resp.code != 200 {
		t.Fatalf("无关目录的站点应照常提供文件: %d", resp.code)
	}
}

type onceResp struct {
	code int
	body string
}

// serveOnce 用 GET 请求 path 调用一次处理器。
func serveOnce(h http.Handler, path string) onceResp {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return onceResp{rec.Code, rec.Body.String()}
}
