package webservice

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/fsperm/fspermtest"
	"github.com/wangwanjie/gately/internal/logbuf"
)

type onceResp struct {
	code int
	body string
}

// serveOnce 用 GET 请求 path 调用一次处理器。
func serveOnce(h http.Handler, path string) onceResp {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.URL = &url.URL{Path: path}
	h.ServeHTTP(rec, req)
	return onceResp{rec.Code, rec.Body.String()}
}

func staticListener(root string, deflt bool) Listener {
	l := Listener{Name: "t", ListenIP: "127.0.0.1", Network: "tcp4", Port: 18080}
	if deflt {
		l.Default = Site{Enable: true, Type: TypeStatic, Root: root, ListDir: true}
	} else {
		l.Sites = []Site{{Enable: true, Domains: []string{"s.test"}, Type: TypeStatic, Root: root}}
	}
	return l
}

// 站点目录就是配置目录或位于其中：保存时拒绝；升级前已保存的站点不再提供任何文件。
func TestStaticRootInsideConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	for name, root := range map[string]string{
		"配置目录":     l.Conf,
		"配置目录内":    filepath.Join(l.Conf, "history"),
		"含 .. 的写法": filepath.Join(l.Media, "..", "1000", "docker", "gately"),
	} {
		for _, deflt := range []bool{false, true} {
			ls := staticListener(root, deflt)
			if err := ls.Validate(); err == nil || !strings.Contains(err.Error(), "配置目录") {
				t.Errorf("站点目录为 %s（%s，默认站点=%v）应被拒绝，实际 %v", name, root, deflt, err)
			}
		}
	}
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	h := &siteHandler{log: logs.Logger("t")}
	handler := h.newStatic(Site{Name: "漏", Enable: true, Type: TypeStatic, Root: l.Conf, ListDir: true})
	for _, p := range []string{"/", "/account.json", "/history/"} {
		if r := serveOnce(handler, p); r.code != 403 || strings.Contains(r.body, fspermtest.Secret) || strings.Contains(r.body, "account.json") {
			t.Fatalf("请求 %s 应返回 403 且不含目录内容: %d %s", p, r.code, r.body)
		}
	}
}

// 站点目录包含配置目录：可以保存、照常提供，配置目录这棵子树返回 404，目录列表中不出现。
func TestStaticRootContainingConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	for _, deflt := range []bool{false, true} {
		ls := staticListener(l.Share, deflt)
		if err := ls.Validate(); err != nil {
			t.Fatalf("包含配置目录的站点目录应能保存: %v", err)
		}
	}
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	h := &siteHandler{log: logs.Logger("t")}
	for _, clean := range []bool{false, true} {
		handler := h.newStatic(Site{Name: "整盘", Enable: true, Type: TypeStatic, Root: l.Share, ListDir: true, CleanURLs: clean})
		for p, want := range l.VisibleFiles() {
			if r := serveOnce(handler, "/"+p); r.code != 200 || r.body != want {
				t.Errorf("%s 应照常提供: %d %s", p, r.code, r.body)
			}
		}
		for _, p := range append(l.HiddenFiles(), "1000/docker/gately/account", "1000/docker/gately/") {
			if r := serveOnce(handler, "/"+p); r.code == 200 || strings.Contains(r.body, fspermtest.Secret) || strings.Contains(r.body, "account.json") {
				t.Errorf("%s 不应提供（CleanURLs=%v）: %d %s", p, clean, r.code, r.body)
			}
		}
		for _, p := range l.HiddenDirs() {
			if r := serveOnce(handler, "/"+p+"/"); r.code == 200 || strings.Contains(r.body, "account.json") {
				t.Errorf("列出 %s 不应成功: %d %s", p, r.code, r.body)
			}
		}
		for dir, names := range l.Listings() {
			p := "/" + dir + "/"
			if dir == "" {
				p = "/"
			}
			r := serveOnce(handler, p)
			if r.code != 200 {
				t.Fatalf("列出 %s 失败: %d", p, r.code)
			}
			for name, want := range names {
				// 目录带结尾的 /，符号链接不带
				if got := strings.Contains(r.body, `href="`+name+`/"`) || strings.Contains(r.body, `href="`+name+`"`); got != want {
					t.Errorf("%s 的列表中 %s 出现=%v，期望 %v: %s", p, name, got, want, r.body)
				}
			}
		}
	}
	found := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, "配置目录已自动隐藏") {
			found++
		}
	}
	if found == 0 {
		t.Fatal("应在日志中说明配置目录已自动隐藏")
	}
	l.Check(t)
}
