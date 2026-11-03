package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/version"
)

func TestVersionNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.5.4", "v0.5.3", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.5.3", "v0.5.3", false},
		{"v0.5.2", "v0.5.3", false},
		{"v0.5.3", "v0.5.3-rc.1", true},
		{"v0.5.3-rc.1", "v0.5.3", false},
		{"v0.5.3-rc.2", "v0.5.3-rc.1", true},
		{"v0.5.4.1", "v0.5.4", true},
		{"v0.5.5", "v0.5.4.1", true},
		{"v0.5.4", "v0.5.4.1", false},
		{"v0.5.4.0", "v0.5.4", false},
		{"v0.5.4.2", "v0.5.4.1", true},
		{"v0.5.4.1", "v0.5.4.1-rc1", true},
		{"v0.5.4.1.1", "v0.5.4", false},
		{"v0.5", "v0.4.9", false},
		{"v0.5.4", "dev", false},
		{"", "v0.5.3", false},
	}
	for _, c := range cases {
		if got := versionNewer(c.a, c.b); got != c.want {
			t.Errorf("versionNewer(%q, %q) = %v，期望 %v", c.a, c.b, got, c.want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	sums := []byte("abc123  gately_v1.0.0_linux_amd64.tar.gz\nDEF456 *gately_v1.0.0_linux_arm64.tar.gz\n")
	if v, ok := checksumFor(sums, "gately_v1.0.0_linux_amd64.tar.gz"); !ok || v != "abc123" {
		t.Fatalf("amd64 校验值错误: %q %v", v, ok)
	}
	if v, ok := checksumFor(sums, "gately_v1.0.0_linux_arm64.tar.gz"); !ok || v != "def456" {
		t.Fatalf("二进制模式的校验值应能识别并转为小写: %q %v", v, ok)
	}
	if _, ok := checksumFor(sums, "gately_v1.0.0_linux_mips.tar.gz"); ok {
		t.Fatal("不存在的文件不应返回校验值")
	}
}

// fakeBinary 返回一个模拟 gately -version 输出的脚本，用作测试中的“新版本”。
func fakeBinary(ver string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\necho '{\"version\":\"%s\",\"commit\":\"x\"}'\n", ver))
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	bin := fakeBinary("v1.0.0")
	got, err := extractBinary(tarGz(t, map[string][]byte{"LICENSE": []byte("x"), "gately": bin}))
	if err != nil || !bytes.Equal(got, bin) {
		t.Fatalf("应从 tar.gz 中取出 gately: %v", err)
	}
	if got, err := extractBinary(bin); err != nil || !bytes.Equal(got, bin) {
		t.Fatalf("非 gzip 内容应原样作为二进制: %v", err)
	}
	if _, err := extractBinary(tarGz(t, map[string][]byte{"README.md": []byte("x")})); err == nil {
		t.Fatal("缺少 gately 时应报错")
	}
}

// setupUpgrade 让在线升级在测试中可用：模拟 Linux 平台，把可执行文件指向临时目录中的旧版本。
func setupUpgrade(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("试运行新版本依赖 shell 脚本")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "gately")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldGOOS, oldExe, oldMarkers, oldVer, oldAPI := upgradeGOOS, upgradeExecutable, containerMarkers, version.Version, releaseAPI
	oldReleases, oldAdvisories := releasesAPI, advisoriesAPI
	oldPkgExe, oldOpkg, oldApk := packagedExecutable, opkgControlFile, apkInstalledDB
	upgradeGOOS = "linux"
	upgradeExecutable = func() (string, error) { return exe, nil }
	containerMarkers = nil
	version.Version = "v0.1.0"
	// 默认视为手动部署：安装包记录指向不存在的文件
	// upgradeSupport 比较的是解析符号链接后的路径（macOS 的临时目录位于 /private 下）
	packagedExecutable = exe
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		packagedExecutable = real
	}
	opkgControlFile = filepath.Join(dir, "opkg", "gately.control")
	apkInstalledDB = filepath.Join(dir, "apk", "installed")
	t.Cleanup(func() {
		upgradeGOOS, upgradeExecutable, containerMarkers, version.Version, releaseAPI = oldGOOS, oldExe, oldMarkers, oldVer, oldAPI
		packagedExecutable, opkgControlFile, apkInstalledDB = oldPkgExe, oldOpkg, oldApk
		releasesAPI, advisoriesAPI = oldReleases, oldAdvisories
	})
	return exe
}

// fakeRelease 模拟 GitHub 的版本查询与下载；sumOverride 非空时 checksums.txt 写入该校验值。
func fakeRelease(t *testing.T, tag string, pkg []byte, sumOverride string) *httptest.Server {
	t.Helper()
	name := fmt.Sprintf("gately_%s_linux_%s.tar.gz", tag, upgradeGOARCH)
	sum := sha256.Sum256(pkg)
	hexSum := hex.EncodeToString(sum[:])
	if sumOverride != "" {
		hexSum = sumOverride
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": tag, "html_url": "https://example.com", "published_at": time.Now(),
				"assets": []map[string]any{
					{"name": name, "browser_download_url": srv.URL + "/dl/" + name},
					{"name": "checksums.txt", "browser_download_url": srv.URL + "/dl/checksums.txt"},
				},
			})
		case "/releases":
			// 检查更新读取发布列表与安全公告
			_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": tag, "html_url": "https://example.com", "published_at": time.Now()}})
		case "/advisories":
			_, _ = w.Write([]byte("[]"))
		case "/dl/" + name:
			_, _ = w.Write(pkg)
		case "/dl/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hexSum, name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	releaseAPI = srv.URL + "/latest"
	releasesAPI, advisoriesAPI = srv.URL+"/releases", srv.URL+"/advisories"
	return srv
}

// waitUpgrade 轮询在线升级进度，直到进入重启、失败或取消状态。
func waitUpgrade(t *testing.T, c *http.Client, base string) upgradeStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		code, body := doJSON(t, c, "GET", base+"/api/system/upgrade/status", nil)
		if code != 200 {
			t.Fatalf("读取升级进度失败: %d %s", code, body)
		}
		var st upgradeStatus
		_ = json.Unmarshal(body, &st)
		if st.State == upgradeStateRestarting || st.State == upgradeStateFailed || st.State == upgradeStateCanceled {
			return st
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("在线升级没有在限定时间内结束")
	return upgradeStatus{}
}

func TestUpgradeOnline(t *testing.T) {
	exe := setupUpgrade(t)
	a, srv, c := loginApp(t)
	newBin := fakeBinary("v0.2.0")
	pkg := tarGz(t, map[string][]byte{"gately": newBin, "LICENSE": []byte("x")})
	fakeRelease(t, "v0.2.0", pkg, "")

	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/update", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"hasUpdate":true`)) || !bytes.Contains(body, []byte(`"supported":true`)) {
		t.Fatalf("检查更新应提示可在线升级: %d %s", code, body)
	}
	code, body = doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil)
	if code != http.StatusAccepted || !bytes.Contains(body, []byte(`"state":"checking"`)) {
		t.Fatalf("在线升级应在后台开始: %d %s", code, body)
	}
	st := waitUpgrade(t, c, srv.URL)
	if st.State != upgradeStateRestarting || st.To != "v0.2.0" || st.From != "v0.1.0" {
		t.Fatalf("在线升级失败: %+v", st)
	}
	if st.Downloaded != int64(len(pkg)) || st.Total != int64(len(pkg)) {
		t.Fatalf("下载进度应为 %d/%d，实际 %d/%d", len(pkg), len(pkg), st.Downloaded, st.Total)
	}
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, newBin) {
		t.Fatal("可执行文件应被替换为新版本")
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "old-binary" {
		t.Fatalf("旧版本应保留为 .old，实际 %q", got)
	}
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("升级后应请求重启")
	}
	// 升级完成等待重启期间，不再接受新的升级请求
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusConflict {
		t.Fatalf("重启前重复升级应返回 409，实际 %d", code)
	}
}

func TestUpgradeAlreadyLatest(t *testing.T) {
	setupUpgrade(t)
	_, srv, c := loginApp(t)
	fakeRelease(t, "v0.1.0", tarGz(t, map[string][]byte{"gately": fakeBinary("v0.1.0")}), "")
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusAccepted {
		t.Fatalf("应在后台开始: %d %s", code, body)
	}
	if st := waitUpgrade(t, c, srv.URL); st.State != upgradeStateFailed || !strings.Contains(st.Error, "v0.1.0") {
		t.Fatalf("已是最新版本时应失败并说明原因: %+v", st)
	}
	// 失败后释放升级锁，可以再次发起
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusAccepted {
		t.Fatalf("失败后应允许重试，实际 %d", code)
	}
	waitUpgrade(t, c, srv.URL)
}

func TestUpgradeRejectsBadChecksum(t *testing.T) {
	exe := setupUpgrade(t)
	_, srv, c := loginApp(t)
	fakeRelease(t, "v0.2.0", tarGz(t, map[string][]byte{"gately": fakeBinary("v0.2.0")}), "0000")
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusAccepted {
		t.Fatalf("应在后台开始: %d %s", code, body)
	}
	if st := waitUpgrade(t, c, srv.URL); st.State != upgradeStateFailed || !strings.Contains(st.Error, "SHA-256") {
		t.Fatalf("校验值不符时应拒绝升级: %+v", st)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old-binary" {
		t.Fatal("校验失败时不应改动可执行文件")
	}
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Fatal("校验失败时不应生成 .old")
	}
}

func TestUpgradeUpload(t *testing.T) {
	exe := setupUpgrade(t)
	_, srv, c := loginApp(t)
	// 无法运行的文件在试运行阶段被拒绝，原文件保持不变
	code, body := uploadFiles(t, c, srv.URL+"/api/system/upgrade/upload", "file", map[string][]byte{"gately": []byte("not-a-binary")})
	if code != http.StatusBadRequest {
		t.Fatalf("无法运行的升级包应被拒绝: %d %s", code, body)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old-binary" {
		t.Fatal("试运行失败时不应改动可执行文件")
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Fatalf("失败后应清理临时文件，目录中还有 %d 个文件", len(entries))
	}
	// 上传旧版本也允许，用于回退
	newBin := fakeBinary("v0.0.9")
	code, body = uploadFiles(t, c, srv.URL+"/api/system/upgrade/upload", "file", map[string][]byte{"gately": newBin})
	if code != 200 || !bytes.Contains(body, []byte(`"to":"v0.0.9"`)) {
		t.Fatalf("上传升级失败: %d %s", code, body)
	}
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, newBin) {
		t.Fatal("可执行文件应被替换为上传的版本")
	}
}

func TestUpgradeUnsupported(t *testing.T) {
	setupUpgrade(t)
	_, srv, c := loginApp(t)
	upgradeGOOS = "windows"
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusConflict {
		t.Fatalf("不支持的平台应返回 409，实际 %d", code)
	}
	upgradeGOOS = "linux"
	marker := filepath.Join(t.TempDir(), ".dockerenv")
	_ = os.WriteFile(marker, nil, 0o644)
	containerMarkers = []string{marker}
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/info", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"reason":"docker"`)) {
		t.Fatalf("容器中应提示不支持在线升级: %d %s", code, body)
	}
}

// 定制构建拒绝在线升级（会装回完整版本），提示到 build.gately.cn 生成；上传升级包仍然可用。
func TestUpgradeCustomBuild(t *testing.T) {
	exe := setupUpgrade(t)
	a, srv, c := loginApp(t)
	a.opts.Absent = []module.Info{{ID: "waf", Title: "WAF"}}
	fakeRelease(t, "v0.2.0", tarGz(t, map[string][]byte{"gately": fakeBinary("v0.2.0")}), "")
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/info", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"custom":true`)) {
		t.Fatalf("系统信息应标明定制构建: %d %s", code, body)
	}
	code, body = doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil)
	if code != http.StatusConflict || !bytes.Contains(body, []byte("build.gately.cn")) {
		t.Fatalf("定制构建在线升级应返回 409 并给出构建器地址: %d %s", code, body)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old-binary" {
		t.Fatal("拒绝在线升级时不应改动可执行文件")
	}
	newBin := fakeBinary("v0.2.0")
	code, body = uploadFiles(t, c, srv.URL+"/api/system/upgrade/upload", "file", map[string][]byte{"gately": newBin})
	if code != 200 || !bytes.Contains(body, []byte(`"to":"v0.2.0"`)) {
		t.Fatalf("定制构建应能上传升级包: %d %s", code, body)
	}
}

// OpenWrt 的 ipk / apk 安装包应提示用包管理器升级，并拒绝在线升级与上传升级
func TestUpgradeOpenWrtPackage(t *testing.T) {
	setupUpgrade(t)
	_, srv, c := loginApp(t)
	reason := func() string {
		t.Helper()
		code, body := doJSON(t, c, "GET", srv.URL+"/api/system/info", nil)
		if code != 200 {
			t.Fatalf("读取系统信息失败: %d %s", code, body)
		}
		var info struct {
			SelfUpgrade upgradeSupport `json:"selfUpgrade"`
		}
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatal(err)
		}
		return info.SelfUpgrade.Reason
	}
	if r := reason(); r != "" {
		t.Fatalf("手动部署应支持在线升级，实际原因 %q", r)
	}

	// apk 数据库中有其他包、没有 gately 时仍按手动部署处理
	_ = os.MkdirAll(filepath.Dir(apkInstalledDB), 0o755)
	_ = os.WriteFile(apkInstalledDB, []byte("P:busybox\nV:1.37.0-r1\n\nP:gately-extra\nV:1\n"), 0o644)
	if r := reason(); r != "" {
		t.Fatalf("未安装 gately 包时不应拒绝在线升级，实际原因 %q", r)
	}
	_ = os.WriteFile(apkInstalledDB, []byte("P:busybox\nV:1.37.0-r1\n\nP:gately\nV:0.5.4-r1\nA:noarch\n"), 0o644)
	if r := reason(); r != upgradeReasonApk {
		t.Fatalf("apk 安装应提示用 apk 升级，实际 %q", r)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusConflict {
		t.Fatalf("安装包部署应拒绝在线升级，实际 %d", code)
	}
	code, _ := uploadFiles(t, c, srv.URL+"/api/system/upgrade/upload", "file", map[string][]byte{"gately": []byte("x")})
	if code != http.StatusConflict {
		t.Fatalf("安装包部署应拒绝上传升级，实际 %d", code)
	}

	_ = os.MkdirAll(filepath.Dir(opkgControlFile), 0o755)
	_ = os.WriteFile(opkgControlFile, []byte("Package: gately\n"), 0o644)
	if r := reason(); r != upgradeReasonOpkg {
		t.Fatalf("opkg 安装应提示用 opkg 升级，实际 %q", r)
	}

	// 程序不在安装包路径时（另行手动部署的副本）仍可在线升级
	packagedExecutable += ".pkg"
	if r := reason(); r != "" {
		t.Fatalf("手动部署的副本应支持在线升级，实际原因 %q", r)
	}
}

func TestUpdateProxySetting(t *testing.T) {
	s := Settings{UpdateProxy: " ftp://x "}
	if err := s.Validate(); err == nil {
		t.Fatal("不支持的代理协议应报错")
	}
	for _, p := range []string{"http://127.0.0.1:7890", "socks5://user:pass@127.0.0.1:1080"} {
		s := Settings{UpdateProxy: p}
		if err := s.Validate(); err != nil {
			t.Fatalf("代理 %s 应合法: %v", p, err)
		}
	}
}

// 设置更新代理后，检查更新应经由该代理访问
func TestUpdateUsesProxy(t *testing.T) {
	setupUpgrade(t)
	a, srv, c := loginApp(t)
	releasesAPI, advisoriesAPI = "http://release.invalid/releases", "http://release.invalid/advisories"
	hits := make(chan string, 4)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 经 HTTP 代理转发的请求行为完整 URL
		hits <- r.URL.String()
		if strings.HasSuffix(r.URL.Path, "/advisories") {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": "v0.2.0"}})
	}))
	t.Cleanup(proxy.Close)
	a.settingsMu.Lock()
	a.settings.UpdateProxy = proxy.URL
	a.settingsMu.Unlock()
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/update", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"latest":"v0.2.0"`)) {
		t.Fatalf("经代理检查更新失败: %d %s", code, body)
	}
	select {
	case u := <-hits:
		if u != releasesAPI {
			t.Fatalf("代理收到的地址应为 %s，实际 %s", releasesAPI, u)
		}
	default:
		t.Fatal("请求没有经过代理")
	}
}

// 下载过程中可以取消：可执行文件保持不变，升级锁释放后可以重新发起
func TestUpgradeCancel(t *testing.T) {
	exe := setupUpgrade(t)
	_, srv, c := loginApp(t)
	pkg := tarGz(t, map[string][]byte{"gately": fakeBinary("v0.2.0")})
	name := fmt.Sprintf("gately_v0.2.0_linux_%s.tar.gz", upgradeGOARCH)
	sum := sha256.Sum256(pkg)
	var rel *httptest.Server
	rel = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0", "assets": []map[string]any{
				{"name": name, "browser_download_url": rel.URL + "/pkg", "size": len(pkg)},
				{"name": "checksums.txt", "browser_download_url": rel.URL + "/sums"},
			}})
		case "/sums":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		case "/pkg":
			// 只发出一部分内容后挂起，模拟缓慢的下载
			w.Header().Set("Content-Length", fmt.Sprint(len(pkg)))
			_, _ = w.Write(pkg[:len(pkg)/2])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}
	}))
	t.Cleanup(rel.Close)
	releaseAPI = rel.URL + "/latest"

	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade/cancel", nil); code != http.StatusConflict {
		t.Fatalf("没有进行中的升级时取消应返回 409，实际 %d", code)
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusAccepted {
		t.Fatalf("应在后台开始: %d %s", code, body)
	}
	// 等到开始下载
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, body := doJSON(t, c, "GET", srv.URL+"/api/system/upgrade/status", nil)
		var st upgradeStatus
		_ = json.Unmarshal(body, &st)
		if st.State == upgradeStateDownloading && st.Downloaded > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("没有进入下载阶段: %s", body)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade/cancel", nil); code != 200 {
		t.Fatalf("取消失败: %d %s", code, body)
	}
	if st := waitUpgrade(t, c, srv.URL); st.State != upgradeStateCanceled || st.Error != "" {
		t.Fatalf("应进入已取消状态: %+v", st)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old-binary" {
		t.Fatal("取消后不应改动可执行文件")
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Fatalf("取消后不应留下其他文件，目录中有 %d 个文件", len(entries))
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/upgrade", nil); code != http.StatusAccepted {
		t.Fatalf("取消后应允许重新发起，实际 %d", code)
	}
	_, _ = doJSON(t, c, "POST", srv.URL+"/api/system/upgrade/cancel", nil)
	waitUpgrade(t, c, srv.URL)
}
