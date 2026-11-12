package binstall

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// script 返回输出指定版本的假程序。
func script(version string) []byte {
	return []byte("#!/bin/sh\necho \"demo version " + version + "\"\n")
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "demo_1/", Typeflag: tar.TypeDir, Mode: 0o755})
	for name, data := range files {
		_ = tw.WriteHeader(&tar.Header{Name: "demo_1/" + name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(data))})
		_, _ = tw.Write(data)
	}
	_ = tw.WriteHeader(&tar.Header{Name: "demo_1/README", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2})
	_, _ = tw.Write([]byte("hi"))
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func zipData(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, _ := zw.Create("demo_1/" + name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// upstream 模拟上游发布：versions 把版本号映射到下载内容与摘要。
type upstream struct {
	mu       sync.Mutex
	srv      *httptest.Server
	latest   string
	assets   map[string][]byte // 版本 -> 内容
	name     string            // 下载文件名
	sums     map[string]string // 版本 -> 摘要，缺省为内容的真实摘要
	requests atomic.Int64
}

func newUpstream(t *testing.T, name string) *upstream {
	u := &upstream{assets: map[string][]byte{}, sums: map[string]string{}, name: name}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.requests.Add(1)
		u.mu.Lock()
		defer u.mu.Unlock()
		ver := strings.TrimPrefix(r.URL.Path, "/dl/")
		data, ok := u.assets[ver]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) set(version string, data []byte) {
	u.mu.Lock()
	u.assets[version] = data
	u.latest = version
	u.mu.Unlock()
}

func (u *upstream) spec(files ...string) Spec {
	re := regexp.MustCompile(`version (\S+)`)
	return Spec{
		Key:   "demo",
		Name:  "demo",
		Files: files,
		Resolve: func(ctx context.Context, client *http.Client, p Platform, version string) (*Release, error) {
			u.mu.Lock()
			defer u.mu.Unlock()
			if version == "" {
				version = u.latest
			}
			data, ok := u.assets[version]
			if !ok {
				return nil, Unsupported("没有找到版本 %s", version)
			}
			s, ok := u.sums[version]
			if !ok {
				s = sum(data)
			}
			return &Release{Version: version, Asset: Asset{Name: u.name, URL: u.srv.URL + "/dl/" + version, SHA256: s, Size: int64(len(data))}}, nil
		},
		Probe: func(path string) (string, error) { return ProbeOutput(path, re, "--version") },
	}
}

func newInstaller(t *testing.T, spec Spec, onChange func()) *Installer {
	t.Helper()
	return New(spec, Options{
		BinDir:   filepath.Join(t.TempDir(), "bin"),
		Logs:     logbuf.New(100, log.New(io.Discard, "", 0)),
		OnChange: onChange,
	})
}

// wait 等待安装结束并返回进度。
func wait(t *testing.T, in *Installer) State {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := in.State(); !st.Running {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("安装超时")
	return State{}
}

func install(t *testing.T, in *Installer, version string) State {
	t.Helper()
	if _, err := in.Start(version, ""); err != nil {
		t.Fatal(err)
	}
	return wait(t, in)
}

func skipWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
}

// TestInstallUpdateRollback 覆盖压缩包安装、更新时保存旧版本、已是最新版跳过、回滚与切回、旧版本数量上限。
func TestInstallUpdateRollback(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo.tar.gz")
	var changes atomic.Int64
	in := newInstaller(t, up.spec("democ", "demos"), func() { changes.Add(1) })

	pkg := func(v string) []byte {
		return tarGz(t, map[string][]byte{"democ": script(v), "demos": script(v)})
	}
	up.set("1.0.0", pkg("1.0.0"))
	if st := install(t, in, ""); st.Stage != StageDone || st.Version != "1.0.0" {
		t.Fatalf("首次安装失败: %+v", st)
	}
	if got := in.Installed(); !got.Present || got.Version != "1.0.0" || len(got.Missing) != 0 {
		t.Fatalf("安装结果错误: %+v", got)
	}
	if fi, err := os.Stat(filepath.Join(in.BinDir(), "demos")); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("第二个程序应可执行: %v", err)
	}
	if len(in.Backups()) != 0 || changes.Load() != 1 {
		t.Fatalf("首次安装不应有旧版本: %+v %d", in.Backups(), changes.Load())
	}
	entries, _ := os.ReadDir(in.BinDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".install-") {
			t.Errorf("临时目录未清理: %s", e.Name())
		}
	}

	// 已是最新版时不下载
	before := up.requests.Load()
	if st := install(t, in, ""); st.Stage != StageUpToDate || up.requests.Load() != before {
		t.Errorf("已是最新版应跳过: %+v", st)
	}

	up.set("1.1.0", pkg("1.1.0"))
	if st := install(t, in, ""); st.Stage != StageDone || in.Installed().Version != "1.1.0" {
		t.Fatalf("更新失败: %+v", st)
	}
	backups := in.Backups()
	if len(backups) != 1 || backups[0].Version != "1.0.0" {
		t.Fatalf("更新后应保存旧版本: %+v", backups)
	}

	// 回滚后当前版本另存，可再切回
	if v, err := in.Rollback(backups[0].ID); err != nil || v != "1.0.0" || in.Installed().Version != "1.0.0" {
		t.Fatalf("回滚失败: %v %v %+v", v, err, in.Installed())
	}
	backups = in.Backups()
	if len(backups) != 1 || backups[0].Version != "1.1.0" {
		t.Fatalf("回滚后应保存原版本: %+v", backups)
	}
	if out, _ := os.ReadFile(filepath.Join(in.BinDir(), "demos")); !bytes.Equal(out, script("1.0.0")) {
		t.Errorf("回滚应同时替换全部程序")
	}
	if _, err := in.Rollback(backups[0].ID); err != nil || in.Installed().Version != "1.1.0" {
		t.Fatalf("切回失败: %v", err)
	}

	// 指定版本安装，旧版本只保留 MaxBackups 个
	for _, v := range []string{"2.0.0", "2.1.0", "2.2.0", "2.3.0"} {
		up.set(v, pkg(v))
		if st := install(t, in, v); st.Stage != StageDone {
			t.Fatalf("安装 %s 失败: %+v", v, st)
		}
	}
	backups = in.Backups()
	if len(backups) != MaxBackups || backups[0].Version != "2.2.0" || backups[2].Version != "2.0.0" {
		t.Fatalf("旧版本数量或顺序错误: %+v", backups)
	}
	if err := in.DeleteBackup(backups[2].ID); err != nil || len(in.Backups()) != MaxBackups-1 {
		t.Errorf("删除旧版本失败: %v", err)
	}

	// 卸载删除程序与全部旧版本，并通知重启
	before = changes.Load()
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if got := in.Installed(); got.Present || len(in.Backups()) != 0 || changes.Load() != before+1 {
		t.Errorf("卸载后应无程序与旧版本: %+v %+v", got, in.Backups())
	}
	if _, err := os.Stat(filepath.Join(in.BinDir(), "demos")); !os.IsNotExist(err) {
		t.Errorf("卸载应删除全部程序: %v", err)
	}
	if err := in.Uninstall(); err != nil {
		t.Errorf("重复卸载不应报错: %v", err)
	}

	// 非法 ID 不能越出旧版本目录
	for _, id := range []string{"", "..", "../demo", ".", "nope"} {
		if _, err := in.Rollback(id); err != ErrNoBackup {
			t.Errorf("回滚 %q 应报不存在: %v", id, err)
		}
	}
}

// TestInstallFailures 验证校验失败、缺少校验值、压缩包缺文件与程序无法运行时都不改动已安装的版本。
func TestInstallFailures(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo.zip")
	in := newInstaller(t, up.spec("democ", "demos"), nil)
	up.set("1.0.0", zipData(t, map[string][]byte{"democ": script("1.0.0"), "demos": script("1.0.0")}))
	if st := install(t, in, ""); st.Stage != StageDone {
		t.Fatalf("zip 安装失败: %+v", st)
	}

	cases := []struct {
		name string
		data []byte
		sum  *string
		want string
	}{
		{"摘要不符", zipData(t, map[string][]byte{"democ": script("2"), "demos": script("2")}), ptr(strings.Repeat("0", 64)), "SHA-256"},
		{"缺少摘要", zipData(t, map[string][]byte{"democ": script("2"), "demos": script("2")}), ptr(""), "SHA-256"},
		{"缺少文件", zipData(t, map[string][]byte{"democ": script("2")}), nil, "demos"},
		{"无法运行", zipData(t, map[string][]byte{"democ": []byte("#!/bin/sh\nexit 3\n"), "demos": script("2")}), nil, "无法在本机运行"},
		{"不是压缩包", []byte("garbage"), nil, "解压失败"},
	}
	for _, c := range cases {
		up.set("2.0.0", c.data)
		up.mu.Lock()
		delete(up.sums, "2.0.0")
		if c.sum != nil {
			up.sums["2.0.0"] = *c.sum
		}
		up.mu.Unlock()
		st := install(t, in, "")
		if st.Stage != StageFailed || !strings.Contains(st.Error, c.want) {
			t.Errorf("%s: 应失败并提示 %q: %+v", c.name, c.want, st)
		}
		if got := in.Installed(); got.Version != "1.0.0" || len(got.Missing) != 0 || len(in.Backups()) != 0 {
			t.Errorf("%s: 失败后不应改动已安装版本: %+v %+v", c.name, got, in.Backups())
		}
	}
}

func ptr(s string) *string { return &s }

// TestRawBinary 验证上游直接发布可执行文件时的安装。
func TestRawBinary(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo-linux-amd64")
	in := newInstaller(t, up.spec("demo"), nil)
	up.set("3.0.0", script("3.0.0"))
	if st := install(t, in, ""); st.Stage != StageDone || in.Installed().Version != "3.0.0" {
		t.Fatalf("安装失败: %+v", st)
	}
}

// TestProxy 验证单独设置的下载代理生效，且非法代理被拒绝。
func TestProxy(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo-raw")
	up.set("1.0.0", script("1.0.0"))
	var proxied atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 普通 HTTP 代理收到的是完整地址，转发给上游
		proxied.Add(1)
		resp, err := http.Get(r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	in := newInstaller(t, up.spec("demo"), nil)
	if _, err := in.Start("", "ftp://x"); err == nil {
		t.Error("非法代理应被拒绝")
	}
	if _, err := in.Start("", proxy.URL); err != nil {
		t.Fatal(err)
	}
	if st := wait(t, in); st.Stage != StageDone || proxied.Load() == 0 {
		t.Errorf("应经代理下载: %+v %d", st, proxied.Load())
	}
}

func TestFindChecksum(t *testing.T) {
	const a = "053b80b0a23031a038fdbf3c073f59f82b8f1f152aa67971d7383ffafc0dbab0"
	const b = "1D9875210972A5D1A62B2DC4C48DF45AB0AF05D3CA1B5E494A014428259DF803"
	text := "### SHA256 Checksums:\n```\ncloudflared-linux-amd64.deb: " + b + "\ncloudflared-linux-amd64: " + a + "\n```\n" +
		b + "  frp_0.71.1_linux_amd64.tar.gz\n" + a + " *frp_0.71.1_linux_arm64.tar.gz\n"
	cases := map[string]string{
		"cloudflared-linux-amd64":       a,
		"cloudflared-linux-amd64.deb":   strings.ToLower(b),
		"frp_0.71.1_linux_amd64.tar.gz": strings.ToLower(b),
		"frp_0.71.1_linux_arm64.tar.gz": a,
		"cloudflared-linux-arm":         "",
	}
	for name, want := range cases {
		if got := FindChecksum(text, name); got != want {
			t.Errorf("%s: %q，期望 %q", name, got, want)
		}
	}
}

// TestGitHubRelease 验证按标签查找（自动补或去掉前缀 v）与摘要的取用顺序。
func TestGitHubRelease(t *testing.T) {
	const digest = "1111111111111111111111111111111111111111111111111111111111111111"
	const fileSum = "2222222222222222222222222222222222222222222222222222222222222222"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest", "/repos/o/r/releases/tags/v1.2.0":
			_ = json.NewEncoder(w).Encode(GitHubRelease{TagName: "v1.2.0", Assets: []GitHubAsset{
				{Name: "a.tar.gz", URL: srv.URL + "/a", Digest: "sha256:" + digest},
				{Name: "b.tar.gz", URL: srv.URL + "/b"},
				{Name: "sums.txt", URL: srv.URL + "/sums"},
			}})
		case "/sums":
			_, _ = io.WriteString(w, fileSum+"  b.tar.gz\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := GitHubAPI
	GitHubAPI = srv.URL
	defer func() { GitHubAPI = old }()
	ctx := context.Background()

	for _, v := range []string{"", "1.2.0", "v1.2.0"} {
		rel, err := FetchGitHubRelease(ctx, http.DefaultClient, "o/r", v)
		if err != nil || rel.TagName != "v1.2.0" {
			t.Errorf("查找 %q 失败: %v", v, err)
		}
	}
	if _, err := FetchGitHubRelease(ctx, http.DefaultClient, "o/r", "9.9.9"); err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("不存在的版本应报错: %v", err)
	}
	rel, _ := FetchGitHubRelease(ctx, http.DefaultClient, "o/r", "")
	if r, err := GitHubAssetRelease(ctx, http.DefaultClient, rel, rel.TagName, "a.tar.gz", "sums.txt"); err != nil || r.Asset.SHA256 != digest {
		t.Errorf("应优先使用 GitHub 摘要: %+v %v", r, err)
	}
	if r, err := GitHubAssetRelease(ctx, http.DefaultClient, rel, rel.TagName, "b.tar.gz", "sums.txt"); err != nil || r.Asset.SHA256 != fileSum {
		t.Errorf("没有摘要时应读校验文件: %+v %v", r, err)
	}
	if _, err := GitHubAssetRelease(ctx, http.DefaultClient, rel, rel.TagName, "c.tar.gz"); err == nil {
		t.Error("缺少文件应报错")
	}
}

// TestRoutes 验证接口：保存代理、拒绝非法代理、不支持的平台与回滚不存在的版本。
func TestRoutes(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo-raw")
	up.set("1.0.0", script("1.0.0"))
	proxy := ""
	spec := up.spec("demo")
	unsupported := false
	spec.Check = func(p Platform) error {
		if unsupported {
			return Unsupported("demo 没有提供 %s 的程序", p)
		}
		return nil
	}
	in := New(spec, Options{
		BinDir:   filepath.Join(t.TempDir(), "bin"),
		Logs:     logbuf.New(100, log.New(io.Discard, "", 0)),
		GetProxy: func() string { return proxy },
		SetProxy: func(p string) error { proxy = p; return nil },
	})
	mux := http.NewServeMux()
	in.Register(routerFunc(func(pattern string, h http.HandlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.HandleFunc(method+" /api/demo"+path, h)
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	call := func(method, path, body string) (int, View) {
		req, _ := http.NewRequest(method, srv.URL+"/api/demo"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v View
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return resp.StatusCode, v
	}
	if code, _ := call(http.MethodPost, "/install", `{"proxy":"bad"}`); code != http.StatusBadRequest || proxy != "" {
		t.Errorf("非法代理应拒绝且不保存: %d %q", code, proxy)
	}
	// 代理连不上时安装失败，但代理已保存
	if code, v := call(http.MethodPost, "/install", `{"proxy":"http://127.0.0.1:1"}`); code != http.StatusOK || v.Proxy != "http://127.0.0.1:1" {
		t.Errorf("应保存代理: %d %+v", code, v)
	}
	if st := wait(t, in); st.Stage != StageFailed {
		t.Errorf("代理不可用应失败: %+v", st)
	}
	if code, _ := call(http.MethodPost, "/install", `{"proxy":""}`); code != http.StatusOK || proxy != "" {
		t.Errorf("应清除代理: %d %q", code, proxy)
	}
	wait(t, in)
	if _, v := call(http.MethodGet, "/install", ""); !v.Installed.Present || v.Installed.Version != "1.0.0" || v.BinDir == "" {
		t.Errorf("安装信息错误: %+v", v)
	}
	if code, _ := call(http.MethodPost, "/install/rollback", `{"id":"nope"}`); code != http.StatusNotFound {
		t.Errorf("回滚不存在的版本应返回 404: %d", code)
	}
	unsupported = true
	if code, v := call(http.MethodGet, "/install", ""); code != http.StatusOK || v.Unsupported == "" {
		t.Errorf("应提示不支持的平台: %+v", v)
	}
	if code, _ := call(http.MethodPost, "/install", `{}`); code != http.StatusBadRequest {
		t.Errorf("不支持的平台应拒绝安装: %d", code)
	}
}

type routerFunc func(pattern string, h http.HandlerFunc)

func (f routerFunc) HandleFunc(pattern string, h http.HandlerFunc) { f(pattern, h) }

// TestReinstallSameVersion 验证重装同一版本时先移走现有文件再替换（Windows 不能覆盖运行中的程序），
// 不产生旧版本，也不残留临时目录。
func TestReinstallSameVersion(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo.tar.gz")
	in := newInstaller(t, up.spec("democ", "demos"), nil)
	up.set("1.0.0", tarGz(t, map[string][]byte{"democ": script("1.0.0"), "demos": script("1.0.0")}))
	if st := install(t, in, ""); st.Stage != StageDone {
		t.Fatalf("首次安装失败: %+v", st)
	}
	// 模拟 demos 丢失后指定同一版本修复安装
	if err := os.Remove(filepath.Join(in.BinDir(), "demos")); err != nil {
		t.Fatal(err)
	}
	if st := install(t, in, "1.0.0"); st.Stage != StageDone {
		t.Fatalf("同版本重装失败: %+v", st)
	}
	if got := in.Installed(); !got.Present || len(got.Missing) != 0 || got.Version != "1.0.0" {
		t.Fatalf("重装结果错误: %+v", got)
	}
	if len(in.Backups()) != 0 {
		t.Fatalf("同版本重装不应保存旧版本: %+v", in.Backups())
	}
	entries, _ := os.ReadDir(in.BinDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".replaced-") || strings.HasPrefix(e.Name(), ".install-") {
			t.Errorf("临时目录未清理: %s", e.Name())
		}
	}
}

// TestCleanStale 验证创建安装器时清理上次异常退出残留的暂存目录，只删本程序的。
func TestCleanStale(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	keep := []string{".install-other-1", ".versions", "demo"}
	stale := []string{".install-demo-1", ".replaced-demo-2", ".uninstall-demo-3"}
	for _, d := range append(append([]string{}, keep...), stale...) {
		if err := os.MkdirAll(filepath.Join(bin, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	New(Spec{Key: "demo", Files: []string{"demo"}}, Options{BinDir: bin})
	for _, d := range stale {
		if _, err := os.Stat(filepath.Join(bin, d)); !os.IsNotExist(err) {
			t.Errorf("残留目录 %s 应被删除", d)
		}
	}
	for _, d := range keep {
		if _, err := os.Stat(filepath.Join(bin, d)); err != nil {
			t.Errorf("目录 %s 不应被删除: %v", d, err)
		}
	}
}

// TestBusyExclusive 验证安装、回滚、卸载互斥：任一进行时其他操作返回 ErrBusy，不改动进度。
func TestBusyExclusive(t *testing.T) {
	up := newUpstream(t, "demo.tar.gz")
	in := newInstaller(t, up.spec("democ"), nil)
	if err := in.acquire(); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Start("", ""); !errors.Is(err, ErrBusy) {
		t.Errorf("回滚或卸载进行时安装应报忙: %v", err)
	}
	if in.State().Running {
		t.Error("被拒绝的安装不应改动进度")
	}
	if err := in.Uninstall(); !errors.Is(err, ErrBusy) {
		t.Errorf("应报忙: %v", err)
	}
	if _, err := in.Rollback("20260101-000000.000"); !errors.Is(err, ErrBusy) {
		t.Errorf("应报忙: %v", err)
	}
	in.release()
	if err := in.Uninstall(); err != nil {
		t.Errorf("释放后应可卸载: %v", err)
	}
}

// TestExeNameKeepsExtension 验证已带扩展名的依赖文件（如 wintun.dll）在 Windows 下不会被追加 .exe。
func TestExeNameKeepsExtension(t *testing.T) {
	if got := ExeName("wintun.dll"); got != "wintun.dll" {
		t.Errorf("ExeName(wintun.dll) = %q", got)
	}
	want := "tool"
	if runtime.GOOS == "windows" {
		want = "tool.exe"
	}
	if got := ExeName("tool"); got != want {
		t.Errorf("ExeName(tool) = %q，期望 %q", got, want)
	}
}
