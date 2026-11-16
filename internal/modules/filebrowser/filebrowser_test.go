package filebrowser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

func validInstance() Instance {
	return Instance{Name: "files", Port: 8080, Sources: []Source{{Path: "/srv/data"}}, AdminPassword: "Secret123"}
}

func TestValidateDefaults(t *testing.T) {
	i := validInstance()
	i.Name = " files "
	i.ListenIP = " [::1] "
	i.Sources = []Source{{Path: "/srv/data/"}, {Path: "/mnt/x", Name: "照片", ReadOnly: true}}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	if i.Name != "files" || i.ListenIP != "::1" || i.BaseURL != "/" || i.AuthMethod != AuthPassword || i.AdminUser != "admin" {
		t.Errorf("默认值或规范化错误: %+v", i)
	}
	want := []Source{{Path: "/srv/data", Name: "data"}, {Path: "/mnt/x", Name: "照片", ReadOnly: true}}
	if !slices.Equal(i.Sources, want) {
		t.Errorf("共享目录规范化错误: %+v", i.Sources)
	}

	p := validInstance()
	p.AuthMethod, p.AdminPassword, p.BaseURL = AuthProxy, "", "files/"
	if err := p.Validate(); err != nil || p.ProxyHeader != defaultProxyHeader || p.BaseURL != "/files/" {
		t.Errorf("代理认证不需要密码，且应填默认请求头: %v %+v", err, p)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(i *Instance){
		"空名称":        func(i *Instance) { i.Name = " " },
		"无效监听地址":     func(i *Instance) { i.ListenIP = "localhost" },
		"端口为 0":      func(i *Instance) { i.Port = 0 },
		"端口过大":       func(i *Instance) { i.Port = 70000 },
		"路径含空格":      func(i *Instance) { i.BaseURL = "/a b" },
		"路径含 ..":     func(i *Instance) { i.BaseURL = "/a/../b" },
		"路径含 //":     func(i *Instance) { i.BaseURL = "//evil" },
		"没有目录":       func(i *Instance) { i.Sources = nil },
		"相对目录":       func(i *Instance) { i.Sources = []Source{{Path: "data"}} },
		"目录名重复":      func(i *Instance) { i.Sources = []Source{{Path: "/a/x"}, {Path: "/b/X"}} },
		"目录名含斜杠":     func(i *Instance) { i.Sources = []Source{{Path: "/a", Name: "a/b"}} },
		"未知登录方式":     func(i *Instance) { i.AuthMethod = "ldap" },
		"无效请求头":      func(i *Instance) { i.AuthMethod, i.ProxyHeader = AuthProxy, "X User" },
		"用户名含冒号":     func(i *Instance) { i.AdminUser = "a:b" },
		"密码登录缺密码":    func(i *Instance) { i.AdminPassword = "" },
		"密码太短":       func(i *Instance) { i.AdminPassword = "admin" },
		"密码为脱敏占位符":   func(i *Instance) { i.AdminPassword = crud.SecretMask },
		"未知语言":       func(i *Instance) { i.Locale = "xx" },
		"有效期为负":      func(i *Instance) { i.TokenHours = -1 },
		"访问地址无协议":    func(i *Instance) { i.AccessURLs = []string{"files.example.com"} },
		"访问地址非 http": func(i *Instance) { i.AccessURLs = []string{"ftp://files.example.com/"} },
		"访问地址含查询":    func(i *Instance) { i.AccessURLs = []string{"https://files.example.com/?a=1"} },
		"访问地址含账号":    func(i *Instance) { i.AccessURLs = []string{"https://u:p@files.example.com/"} },
		"访问地址过多": func(i *Instance) {
			for k := 0; k <= maxAccessURLs; k++ {
				i.AccessURLs = append(i.AccessURLs, fmt.Sprintf("https://h%d.example.com/", k))
			}
		},
	}
	for name, mutate := range cases {
		i := validInstance()
		mutate(&i)
		if err := i.Validate(); err == nil {
			t.Errorf("%s 应被拒绝: %+v", name, i)
		}
	}
}

func TestConfigOutput(t *testing.T) {
	i := validInstance()
	i.ListenIP, i.BaseURL, i.TrustProxyHeaders, i.DisableWebDAV, i.Locale = "127.0.0.1", "/files", true, true, "zh-cn"
	// 名称中的引号与换行必须被转义，不能写出额外的配置项
	i.Sources = []Source{{Path: "/srv/a", Name: "a\"\n  noauth: true"}, {Path: "/srv/b", ReadOnly: true}}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := i.Config("/data/fb")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Secret123") {
		t.Fatal("管理员密码不应写入配置文件")
	}
	var c fbConfig
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("配置不是有效的 JSON: %v\n%s", err, data)
	}
	if c.HTTP.Listen != "127.0.0.1" || c.HTTP.Port != 8080 || c.HTTP.BaseURL != "/files" || !c.HTTP.TrustProxyHeaders || !c.HTTP.DisableWebDAV {
		t.Errorf("http 配置错误: %+v", c.HTTP)
	}
	if c.Server.Database.Path != filepath.Join("/data/fb", "filebrowser.sqlite") || c.Server.CacheDir != filepath.Join("/data/fb", "cache") {
		t.Errorf("数据目录错误: %+v", c.Server)
	}
	if len(c.Server.Sources) != 2 || c.Server.Sources[0].Name != "a\"\n  noauth: true" || !c.Server.Sources[1].Config.ReadOnly || !c.Server.Sources[0].Config.DefaultEnabled {
		t.Errorf("共享目录错误: %+v", c.Server.Sources)
	}
	m := c.Auth.Methods
	if !m.Password.Enabled || m.Proxy.Enabled || m.NoAuth || c.Auth.AdminUsername != "admin" || c.Auth.TokenExpirationHours != defaultTokenHours {
		t.Errorf("认证配置错误: %+v", c.Auth)
	}
	if c.UserDefaults == nil || c.UserDefaults.UI.Locale != "zh-cn" {
		t.Errorf("默认语言错误: %+v", c.UserDefaults)
	}
	if c.HTTP.ExternalURL != "" {
		t.Errorf("未填写访问地址时不应写出外部地址: %q", c.HTTP.ExternalURL)
	}

	// 访问地址：去掉空行与重复项，第一个地址的源写入 externalUrl（上游会再拼接 baseURL）
	e := validInstance()
	e.BaseURL = "/files"
	e.AccessURLs = []string{" ", " https://fb.example.com:8443/files/ ", "https://fb.example.com:8443/files/", "http://192.168.1.2:8080/files/"}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(e.AccessURLs) != 2 || e.AccessURLs[0] != "https://fb.example.com:8443/files/" {
		t.Errorf("访问地址规范化错误: %q", e.AccessURLs)
	}
	data, _ = e.Config("/d")
	c = fbConfig{}
	_ = json.Unmarshal(data, &c)
	if c.HTTP.ExternalURL != "https://fb.example.com:8443" {
		t.Errorf("外部地址错误: %q", c.HTTP.ExternalURL)
	}

	// 代理认证：关闭密码登录，写出请求头；未设置语言时不写 userDefaults，避免锁定上游的默认设置
	p := validInstance()
	p.AuthMethod, p.ProxyHeader, p.TokenHours = AuthProxy, "X-Username", 12
	_ = p.Validate()
	data, _ = p.Config("/d")
	c = fbConfig{}
	_ = json.Unmarshal(data, &c)
	if c.Auth.Methods.Password.Enabled || !c.Auth.Methods.Proxy.Enabled || c.Auth.Methods.Proxy.Header != "X-Username" || c.Auth.TokenExpirationHours != 12 {
		t.Errorf("代理认证配置错误: %+v", c.Auth)
	}
	if strings.Contains(string(data), "userDefaults") || strings.Contains(string(data), `"listen"`) {
		t.Errorf("未设置的项不应写出:\n%s", data)
	}

	n := validInstance()
	n.AuthMethod = AuthNone
	_ = n.Validate()
	data, _ = n.Config("/d")
	c = fbConfig{}
	_ = json.Unmarshal(data, &c)
	if !c.Auth.Methods.NoAuth || c.Auth.Methods.Password.Enabled {
		t.Errorf("免登录配置错误: %+v", c.Auth)
	}
}

func TestEnv(t *testing.T) {
	i := validInstance()
	if !slices.Equal(i.Env(), []string{"FILEBROWSER_ADMIN_PASSWORD=Secret123"}) {
		t.Errorf("密码应通过环境变量传递: %v", i.Env())
	}
	i.AdminPassword = ""
	if i.Env() != nil {
		t.Errorf("未设置密码时不应传递: %v", i.Env())
	}
}

func TestAssetName(t *testing.T) {
	cases := map[binstall.Platform]string{
		{OS: "linux", Arch: "amd64"}:         "linux-amd64-filebrowser",
		{OS: "linux", Arch: "arm64"}:         "linux-arm64-filebrowser",
		{OS: "linux", Arch: "arm", ARM: "7"}: "linux-armv7-filebrowser",
		{OS: "linux", Arch: "arm", ARM: "6"}: "linux-armv6-filebrowser",
		{OS: "linux", Arch: "riscv64"}:       "linux-riscv64-filebrowser",
		{OS: "darwin", Arch: "arm64"}:        "darwin-arm64-filebrowser",
		{OS: "windows", Arch: "amd64"}:       "filebrowser.exe",
	}
	for in, want := range cases {
		if got, err := assetName(in); err != nil || got != want {
			t.Errorf("%v: %q %v，期望 %q", in, got, err, want)
		}
	}
	for _, in := range []binstall.Platform{{OS: "linux", Arch: "arm", ARM: "5"}, {OS: "linux", Arch: "mipsle"}, {OS: "linux", Arch: "386"}, {OS: "windows", Arch: "arm64"}} {
		if _, err := assetName(in); err == nil {
			t.Errorf("%v 上游没有提供，应报错", in)
		}
	}
}

// writeScript 写一个 sh 脚本作为假的 filebrowser。
func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

const quantumVersionScript = `if [ "$1" = version ]; then
  echo "2026/10/26 11:27:17 [DEBUG] Default SQLite driver initialized"
  echo "FileBrowser Quantum - A modern web-based file manager"
  printf '\tVersion \t : v2.0.2-stable\n'
  exit 0
fi
`

func TestProbeVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	dir := t.TempDir()
	q := filepath.Join(dir, "q")
	writeScript(t, q, quantumVersionScript)
	if v, ok := probeVersion(q); v != "v2.0.2-stable" || !ok {
		t.Errorf("Quantum 版本识别错误: %q %v", v, ok)
	}
	legacy := filepath.Join(dir, "legacy")
	writeScript(t, legacy, "echo 'File Browser v2.63.23/e8a388f8'\n")
	if v, ok := probeVersion(legacy); v != "v2.63.23/e8a388f8" || ok {
		t.Errorf("原版应识别为非 Quantum: %q %v", v, ok)
	}
}

type testRouter struct{ mux *http.ServeMux }

func (tr testRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	tr.mux.HandleFunc(method+" /api/filebrowser"+path, h)
}

func newTestModule(t *testing.T, dir string) (*Module, *httptest.Server) {
	t.Helper()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: store, Logs: logbuf.New(200, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return m, srv
}

func doJSON(t *testing.T, method, url string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestModuleRunsFakeFilebrowser 用假的 filebrowser 脚本验证：配置写入、密码经环境变量传递、
// 接口返回脱敏、修改时保留密码、删除实例时清理数据目录。
func TestModuleRunsFakeFilebrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	dir := t.TempDir()
	// 脚本把参数与收到的密码写到数据目录（进程工作目录）下，再常驻
	bin := filepath.Join(dir, "bin", "filebrowser")
	writeScript(t, bin, quantumVersionScript+`echo "args=$*" > started
echo "pw=$FILEBROWSER_ADMIN_PASSWORD" >> started
echo "[INFO ] Running at : http://127.0.0.1:8080/"
exec sleep 30
`)
	m, srv := newTestModule(t, dir)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// 共享目录不存在时不启动，并给出原因
	missing := validInstance()
	missing.Enable, missing.Sources = true, []Source{{Path: filepath.Join(t.TempDir(), "missing")}}
	var bad InstanceView
	if code := doJSON(t, http.MethodPost, srv.URL+"/api/filebrowser/instances", missing, &bad); code != 200 || !strings.Contains(bad.Error, "不存在") || bad.State.Running {
		t.Fatalf("共享目录不存在应报错: %d %+v", code, bad)
	}
	doJSON(t, http.MethodDelete, srv.URL+"/api/filebrowser/instances/"+bad.Key, nil, nil)

	inst := validInstance()
	// 共享目录不能是配置目录（dir），另用一个目录
	inst.Enable, inst.Sources = true, []Source{{Path: t.TempDir()}}
	var created InstanceView
	if code := doJSON(t, http.MethodPost, srv.URL+"/api/filebrowser/instances", inst, &created); code != 200 {
		t.Fatalf("创建失败: %d", code)
	}
	if created.AdminPassword != crud.SecretMask {
		t.Errorf("接口应脱敏密码: %q", created.AdminPassword)
	}
	data := m.dataDir(created.Key)
	started := filepath.Join(data, "started")
	waitFor(t, "进程启动", func() bool {
		b, _ := os.ReadFile(started)
		return strings.Contains(string(b), "pw=")
	})
	b, _ := os.ReadFile(started)
	cfgPath := filepath.Join(data, "config.yaml")
	if !strings.Contains(string(b), "args=-c "+cfgPath) || !strings.Contains(string(b), "pw=Secret123") {
		t.Errorf("启动参数或密码错误:\n%s", b)
	}
	if st, err := os.Stat(cfgPath); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("配置文件应为 0600: %v %v", err, st)
	}
	if st, err := os.Stat(data); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("数据目录应为 0700: %v %v", err, st)
	}

	// 原样提交脱敏后的密码，应保留原密码并重启进程
	os.Remove(started)
	created.Name = "renamed"
	if code := doJSON(t, http.MethodPut, srv.URL+"/api/filebrowser/instances/"+created.Key, created.Instance, nil); code != 200 {
		t.Fatalf("修改失败: %d", code)
	}
	waitFor(t, "进程重启", func() bool {
		b, _ := os.ReadFile(started)
		return strings.Contains(string(b), "pw=")
	})
	if b, _ := os.ReadFile(started); !strings.Contains(string(b), "pw=Secret123") {
		t.Errorf("修改后密码应保留:\n%s", b)
	}
	if m.cfg.Instances[0].AdminPassword != "Secret123" || m.cfg.Instances[0].Name != "renamed" {
		t.Errorf("保存结果错误: %+v", m.cfg.Instances[0])
	}

	var list []InstanceView
	doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/instances", nil, &list)
	if len(list) != 1 || !list[0].State.Running || list[0].Error != "" {
		t.Errorf("实例状态错误: %+v", list)
	}
	var sv settingsView
	doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/settings", nil, &sv)
	if sv.Binary != bin || sv.Version != "v2.0.2-stable" || !sv.Quantum || sv.Error != "" {
		t.Errorf("程序信息错误: %+v", sv)
	}

	if code := doJSON(t, http.MethodDelete, srv.URL+"/api/filebrowser/instances/"+created.Key, nil, nil); code != 200 {
		t.Fatalf("删除失败: %d", code)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Errorf("删除实例后数据目录应被清理: %v", err)
	}
}

// TestResolvePrefersInstalled 验证配置目录 bin 下安装的程序优先于 PATH，且能识别出原版。
func TestResolvePrefersInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	dir := t.TempDir()
	pathDir := filepath.Join(dir, "path")
	writeScript(t, filepath.Join(pathDir, "filebrowser"), "echo 'File Browser v2.63.23/e8a388f8'\n")
	t.Setenv("PATH", pathDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	m, srv := newTestModule(t, filepath.Join(dir, "cfg"))

	var sv settingsView
	doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/settings", nil, &sv)
	if sv.Binary != filepath.Join(pathDir, "filebrowser") || sv.Quantum || sv.Error == "" {
		t.Errorf("PATH 中的原版应被识别并提示: %+v", sv)
	}
	writeScript(t, m.localBin(), quantumVersionScript)
	m.mu.Lock()
	m.versionPath = ""
	m.mu.Unlock()
	doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/settings", nil, &sv)
	if sv.Binary != m.localBin() || !sv.Quantum || sv.Error != "" {
		t.Errorf("应优先使用配置目录中安装的程序: %+v", sv)
	}
}

// TestInstall 用本地 HTTP 服务模拟 GitHub 发布，验证经模块接口下载、校验、安装并重新探测版本。
func TestInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	name, err := assetName(binstall.CurrentPlatform())
	if err != nil {
		t.Skip(err)
	}
	payload := []byte("#!/bin/sh\n" + quantumVersionScript)
	sum := sha256.Sum256(payload)
	// 摘要在各轮之间修改，服务端在另一个协程读取，用原子值避免数据竞争
	var digest atomic.Value
	digest.Store("sha256:" + hex.EncodeToString(sum[:]))
	var gh *httptest.Server
	gh = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + releaseRepo + "/releases/latest":
			_ = json.NewEncoder(w).Encode(binstall.GitHubRelease{TagName: "v2.0.2-stable", Assets: []binstall.GitHubAsset{
				{Name: "other", URL: gh.URL + "/other"},
				{Name: name, URL: gh.URL + "/bin", Size: int64(len(payload)), Digest: digest.Load().(string)},
			}})
		case "/bin":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()
	old := binstall.GitHubAPI
	binstall.GitHubAPI = gh.URL
	defer func() { binstall.GitHubAPI = old }()

	run := func(srv *httptest.Server) binstall.State {
		t.Helper()
		var v binstall.View
		doJSON(t, http.MethodPost, srv.URL+"/api/filebrowser/install", map[string]string{}, &v)
		waitFor(t, "安装结束", func() bool {
			doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/install", nil, &v)
			return !v.State.Running
		})
		return v.State
	}

	m, srv := newTestModule(t, t.TempDir())
	st := run(srv)
	if st.Stage != binstall.StageDone || st.Version != "v2.0.2-stable" || st.Error != "" || st.Downloaded != int64(len(payload)) {
		t.Fatalf("安装结果错误: %+v", st)
	}
	if got, err := os.ReadFile(m.localBin()); err != nil || string(got) != string(payload) {
		t.Fatalf("程序未写入: %v", err)
	}
	var sv settingsView
	doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/settings", nil, &sv)
	if sv.Binary != m.localBin() || !sv.Quantum || sv.Version != "v2.0.2-stable" {
		t.Errorf("安装后应使用新程序: %+v", sv)
	}
	// 已是最新版时不再下载
	if st := run(srv); st.Stage != binstall.StageUpToDate {
		t.Errorf("已是最新版应跳过: %+v", st)
	}

	// 摘要不符时中止，不留下程序
	digest.Store("sha256:" + strings.Repeat("0", 64))
	m2, srv2 := newTestModule(t, t.TempDir())
	if st := run(srv2); st.Stage != binstall.StageFailed || !strings.Contains(st.Error, "SHA-256") {
		t.Errorf("校验失败应报错: %+v", st)
	}
	if _, err := os.Stat(m2.localBin()); !os.IsNotExist(err) {
		t.Errorf("校验失败不应安装: %v", err)
	}
}

// TestReset 验证恢复默认配置会停止并删除实例、清除实例数据与程序设置，保留安装的程序。
func TestReset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin", "filebrowser")
	writeScript(t, bin, quantumVersionScript+"exec sleep 30\n")
	m, srv := newTestModule(t, dir)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	inst := validInstance()
	// 共享目录不能是配置目录（dir），另用一个目录
	inst.Enable, inst.Sources = true, []Source{{Path: t.TempDir()}}
	var v InstanceView
	doJSON(t, http.MethodPost, srv.URL+"/api/filebrowser/instances", inst, &v)
	doJSON(t, http.MethodPut, srv.URL+"/api/filebrowser/settings", Settings{BinPath: bin}, nil)
	waitFor(t, "实例启动", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		rt := m.runs[v.Key]
		return rt != nil && rt.sup.State().Running
	})
	if _, err := os.Stat(m.dataDir(v.Key)); err != nil {
		t.Fatalf("实例数据目录应存在: %v", err)
	}

	if code := doJSON(t, http.MethodPost, srv.URL+"/api/filebrowser/reset", nil, nil); code != http.StatusOK {
		t.Fatalf("恢复默认配置失败: %d", code)
	}
	var list []InstanceView
	doJSON(t, http.MethodGet, srv.URL+"/api/filebrowser/instances", nil, &list)
	if len(list) != 0 || len(m.runs) != 0 || m.cfg.Settings.BinPath != "" {
		t.Errorf("实例与设置应被清空: %+v %+v", list, m.cfg.Settings)
	}
	if _, err := os.Stat(filepath.Join(dir, ID)); !os.IsNotExist(err) {
		t.Errorf("实例数据应被删除: %v", err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Errorf("安装的程序应保留: %v", err)
	}
}

// TestSecrets 验证查看管理员密码明文接口返回真实密码，实例不存在时返回 404。
func TestSecrets(t *testing.T) {
	m, srv := newTestModule(t, t.TempDir())
	m.mu.Lock()
	m.cfg.Instances = []Instance{{Key: "k", Name: "fb", AdminPassword: "s3cret"}}
	m.mu.Unlock()
	var got map[string]string
	if code := doJSON(t, "GET", srv.URL+"/api/filebrowser/instances/k/secrets", nil, &got); code != 200 || got["adminPassword"] != "s3cret" {
		t.Fatalf("应返回管理员密码明文: %d %+v", code, got)
	}
	if code := doJSON(t, "GET", srv.URL+"/api/filebrowser/instances/missing/secrets", nil, nil); code != 404 {
		t.Fatalf("实例不存在应返回 404，实际 %d", code)
	}
}
