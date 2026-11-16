package storage

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/fsperm/fspermtest"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

type testRouter struct{ *http.ServeMux }

func (r testRouter) HandleFunc(p string, h http.HandlerFunc) { r.ServeMux.HandleFunc(p, h) }

// storageClient 经模块接口操作挂载点。
type storageClient struct {
	t   *testing.T
	mux *http.ServeMux
}

func (c storageClient) do(method, path string, body any) (int, string) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (c storageClient) upload(key, dir, name, content string) int {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/files/"+key+"/upload?path="+url.QueryEscape(dir), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	c.mux.ServeHTTP(rec, req)
	return rec.Code
}

func newStorage(t *testing.T, l fspermtest.Layout) (*Module, storageClient, *logbuf.Buffer) {
	t.Helper()
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	m := New()
	if err := m.Init(module.Env{Store: l.Store, Logs: logs}); err != nil {
		t.Fatalf("存量配置不应让模块初始化失败: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	return m, storageClient{t: t, mux: mux}, logs
}

// 挂载点就是配置目录或位于其中：新增、修改、复制时拒绝；升级前已保存的不再提供访问。
func TestLocalMountInsideConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	if err := l.Store.Save(ID, fileConfig{Mounts: []Mount{
		{Key: "old1", Name: "配置", Type: TypeLocal, Enable: true, Path: l.Conf},
		{Key: "old2", Name: "配置内", Type: TypeLocal, Enable: true, Path: filepath.Join(l.Conf, "history")},
		{Key: "fine", Name: "共享", Type: TypeLocal, Enable: true, Path: l.Media},
	}}); err != nil {
		t.Fatal(err)
	}
	_, c, logs := newStorage(t, l)
	if n := countLogs(logs, "配置目录重叠"); n != 2 {
		t.Fatalf("应为两个位于配置目录中的挂载点各写一条警告，实际 %d", n)
	}
	for _, p := range []string{"/files/old1/download?path=/account.json", "/files/old1/list?path=/", "/files/old2/list?path=/"} {
		if code, body := c.do("GET", p, nil); code != 400 || strings.Contains(body, fspermtest.Secret) || !strings.Contains(body, "配置目录") {
			t.Fatalf("%s 应被拒绝: %d %s", p, code, body)
		}
	}
	if code, body := c.do("GET", "/files/fine/download?path=/hello.txt", nil); code != 200 || body != "hello" {
		t.Fatalf("其他挂载点应照常可用: %d %s", code, body)
	}
	cases := map[string]string{
		"配置目录":      l.Conf,
		"配置目录内":     filepath.Join(l.Conf, "history"),
		"含 .. 的写法":  filepath.Join(l.Media, "..", "1000", "docker", "gately"),
		"末尾带分隔符的写法": l.Conf + string(filepath.Separator),
	}
	if l.Links {
		cases["指向配置目录的符号链接"] = filepath.Join(l.Share, "link")
		cases["经符号链接进入配置目录"] = filepath.Join(l.Share, "up", "gately", "history")
	}
	for name, p := range cases {
		mt := map[string]any{"name": "x", "type": TypeLocal, "enable": true, "path": p}
		for _, req := range [][2]string{{"POST", "/mounts"}, {"PUT", "/mounts/fine"}, {"POST", "/mounts/fine/copy"}} {
			if code, body := c.do(req[0], req[1], mt); code != 400 || !strings.Contains(body, "配置目录") {
				t.Errorf("%s %s 为 %s（%s）应被拒绝: %d %s", req[0], req[1], name, p, code, body)
			}
		}
	}
}

// 挂载点包含配置目录（整个数据盘）：可以保存、照常使用，配置目录这棵子树在文件管理中不可见、不可读写。
func TestLocalMountContainingConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	if err := l.Store.Save(ID, fileConfig{Mounts: []Mount{{Key: "old", Name: "整盘", Type: TypeLocal, Enable: true, Path: l.Share}}}); err != nil {
		t.Fatal(err)
	}
	_, c, logs := newStorage(t, l)
	if countLogs(logs, "配置目录已自动隐藏") != 1 || countLogs(logs, "配置目录重叠") != 0 {
		t.Fatal("包含配置目录的存量挂载点应照常使用，并说明配置目录已自动隐藏")
	}
	// 新增与修改都可以保存
	code, body := c.do("POST", "/mounts", map[string]any{"name": "vol1", "type": TypeLocal, "enable": true, "path": l.Share})
	if code != 200 {
		t.Fatalf("包含配置目录的目录应能挂载: %d %s", code, body)
	}
	if code, body := c.do("PUT", "/mounts/old", map[string]any{"name": "整盘", "type": TypeLocal, "enable": true, "path": filepath.Dir(l.Docker)}); code != 200 {
		t.Fatalf("改为另一个包含配置目录的目录应能保存: %d %s", code, body)
	}
	var mt struct{ Key string }
	_ = json.Unmarshal([]byte(body), &mt)
	exerciseHidden(t, c, mt.Key, l)
}

// exerciseHidden 在挂载点 key（挂载 l.Share）上验证配置目录子树不可见、不可达，其余文件照常可用。
func exerciseHidden(t *testing.T, c storageClient, key string, l fspermtest.Layout) {
	t.Helper()
	for p, want := range l.VisibleFiles() {
		if code, body := c.do("GET", "/files/"+key+"/download?path="+url.QueryEscape("/"+p), nil); code != 200 || body != want {
			t.Errorf("%s 应照常可读: %d %s", p, code, body)
		}
	}
	for _, p := range l.HiddenFiles() {
		if code, body := c.do("GET", "/files/"+key+"/download?path="+url.QueryEscape("/"+p), nil); code == 200 || strings.Contains(body, fspermtest.Secret) {
			t.Errorf("%s 不应读到: %d %s", p, code, body)
		}
	}
	for _, p := range l.HiddenDirs() {
		if code, body := c.do("GET", "/files/"+key+"/list?path="+url.QueryEscape("/"+p), nil); code == 200 || strings.Contains(body, "account.json") {
			t.Errorf("列出 %s 不应成功: %d %s", p, code, body)
		}
	}
	for dir, names := range l.Listings() {
		code, body := c.do("GET", "/files/"+key+"/list?path="+url.QueryEscape("/"+dir), nil)
		if code != 200 {
			t.Fatalf("列出 /%s 失败: %d %s", dir, code, body)
		}
		for name, want := range names {
			if got := strings.Contains(body, `"name":"`+name+`"`); got != want {
				t.Errorf("/%s 的列表中 %s 出现=%v，期望 %v: %s", dir, name, got, want, body)
			}
		}
	}
	for _, r := range l.Renames() {
		if code, _ := c.do("POST", "/files/"+key+"/rename", map[string]string{"from": "/" + r[0], "to": "/" + r[1]}); code == 200 {
			t.Errorf("%s → %s 应被拒绝", r[0], r[1])
		}
	}
	for _, p := range l.Removals() {
		if code, _ := c.do("DELETE", "/files/"+key+"?path="+url.QueryEscape("/"+p), nil); code == 200 {
			t.Errorf("删除 %s 应被拒绝", p)
		}
	}
	for _, p := range l.Creates() {
		if code, _ := c.do("POST", "/files/"+key+"/mkdir", map[string]string{"path": "/" + p}); code == 200 {
			t.Errorf("新建 %s 应被拒绝", p)
		}
		if code := c.upload(key, "/"+filepath.ToSlash(filepath.Dir(p)), filepath.Base(p), "x"); code == 200 {
			t.Errorf("上传到 %s 应被拒绝", p)
		}
	}
	// 普通文件的增删改照常
	if code := c.upload(key, "/media", "new.txt", "n"); code != 200 {
		t.Fatalf("上传普通文件应成功: %d", code)
	}
	if code, body := c.do("POST", "/files/"+key+"/rename", map[string]string{"from": "/media/new.txt", "to": "/1000/docker/gately-old/new.txt"}); code != 200 {
		t.Fatalf("普通文件改名应成功: %d %s", code, body)
	}
	if code, body := c.do("DELETE", "/files/"+key+"?path=/1000/docker/gately-old/new.txt", nil); code != 200 {
		t.Fatalf("删除普通文件应成功: %d %s", code, body)
	}
	l.Check(t)
}

// 挂载根目录同样只隐藏配置目录。
func TestLocalMountFilesystemRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("根目录写法不同")
	}
	l := fspermtest.New(t)
	_, c, _ := newStorage(t, l)
	code, body := c.do("POST", "/mounts", map[string]any{"name": "root", "type": TypeLocal, "enable": true, "path": "/"})
	if code != 200 {
		t.Fatalf("挂载根目录应能保存: %d %s", code, body)
	}
	var mt struct{ Key string }
	_ = json.Unmarshal([]byte(body), &mt)
	real, _ := filepath.EvalSymlinks(l.Conf)
	for _, p := range []string{l.Conf, real} {
		if code, body := c.do("GET", "/files/"+mt.Key+"/download?path="+url.QueryEscape(p+"/account.json"), nil); code == 200 || strings.Contains(body, fspermtest.Secret) {
			t.Fatalf("经根目录挂载读取 %s 不应成功: %d %s", p, code, body)
		}
	}
	if code, body := c.do("GET", "/files/"+mt.Key+"/download?path="+url.QueryEscape(filepath.Join(l.Media, "hello.txt")), nil); code != 200 || body != "hello" {
		t.Fatalf("根目录挂载中的普通文件应可读: %d %s", code, body)
	}
	if _, err := os.Stat(l.Conf); err != nil {
		t.Fatal(err)
	}
}

func countLogs(logs *logbuf.Buffer, sub string) int {
	n := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, sub) {
			n++
		}
	}
	return n
}
