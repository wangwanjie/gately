package easytier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/wangwanjie/gately/internal/binstall"
)

func TestAssetName(t *testing.T) {
	cases := map[binstall.Platform]string{
		{OS: "linux", Arch: "amd64"}:         "easytier-linux-x86_64-v2.6.4.zip",
		{OS: "linux", Arch: "arm64"}:         "easytier-linux-aarch64-v2.6.4.zip",
		{OS: "linux", Arch: "arm", ARM: "7"}: "easytier-linux-armv7-v2.6.4.zip",
		{OS: "linux", Arch: "arm", ARM: "6"}: "easytier-linux-armhf-v2.6.4.zip",
		{OS: "linux", Arch: "arm", ARM: "5"}: "easytier-linux-arm-v2.6.4.zip",
		{OS: "linux", Arch: "mipsle"}:        "easytier-linux-mipsel-v2.6.4.zip",
		{OS: "linux", Arch: "mips"}:          "easytier-linux-mips-v2.6.4.zip",
		{OS: "linux", Arch: "riscv64"}:       "easytier-linux-riscv64-v2.6.4.zip",
		{OS: "linux", Arch: "loong64"}:       "easytier-linux-loongarch64-v2.6.4.zip",
		{OS: "darwin", Arch: "arm64"}:        "easytier-macos-aarch64-v2.6.4.zip",
		{OS: "darwin", Arch: "amd64"}:        "easytier-macos-x86_64-v2.6.4.zip",
		{OS: "windows", Arch: "amd64"}:       "easytier-windows-x86_64-v2.6.4.zip",
		{OS: "windows", Arch: "arm64"}:       "easytier-windows-arm64-v2.6.4.zip",
		{OS: "windows", Arch: "386"}:         "easytier-windows-i686-v2.6.4.zip",
		{OS: "freebsd", Arch: "amd64"}:       "easytier-freebsd-13.2-x86_64-v2.6.4.zip",
	}
	for p, want := range cases {
		if got, err := assetName("2.6.4", p); err != nil || got != want {
			t.Errorf("%v: %q %v，期望 %q", p, got, err, want)
		}
	}
	for _, p := range []binstall.Platform{{OS: "linux", Arch: "386"}, {OS: "linux", Arch: "ppc64le"}, {OS: "freebsd", Arch: "arm64"}} {
		if _, err := assetName("2.6.4", p); err == nil {
			t.Errorf("%v 上游没有提供，应报错", p)
		}
		if err := installSpec().Check(p); err == nil {
			t.Errorf("%v 应提示不支持", p)
		}
	}
	if m := etVersionRe.FindStringSubmatch("easytier-core 2.6.4-8428a89d\n"); m == nil || m[1] != "2.6.4" {
		t.Errorf("版本解析错误: %v", m)
	}
}

// TestResolve 用本地服务模拟 GitHub，验证最新版与指定版本的资源名、下载地址与摘要。
func TestResolve(t *testing.T) {
	const sum = "61b659eaedba658fa66fe47d17e1426cdd77e5d02fa15fed447bb4357c09dfd6"
	rel := map[string]any{
		"tag_name": "v2.6.4",
		"assets": []map[string]any{
			{"name": "easytier-linux-x86_64-v2.6.4.zip", "browser_download_url": "https://example.com/dl/easytier-linux-x86_64-v2.6.4.zip", "size": 100, "digest": "sha256:" + sum},
			{"name": "easytier-gui_2.6.4_amd64.deb", "browser_download_url": "https://example.com/dl/gui.deb", "digest": "sha256:" + sum},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/EasyTier/EasyTier/releases/latest", "/repos/EasyTier/EasyTier/releases/tags/v2.6.4":
			_ = json.NewEncoder(w).Encode(rel)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := binstall.GitHubAPI
	binstall.GitHubAPI = srv.URL
	defer func() { binstall.GitHubAPI = old }()

	spec := installSpec()
	ctx := context.Background()
	amd64 := binstall.Platform{OS: "linux", Arch: "amd64"}
	got, err := spec.Resolve(ctx, http.DefaultClient, amd64, "")
	if err != nil || got.Version != "2.6.4" || got.Asset.Name != "easytier-linux-x86_64-v2.6.4.zip" || got.Asset.SHA256 != sum {
		t.Fatalf("最新版解析错误: %+v %v", got, err)
	}
	if got, err := spec.Resolve(ctx, http.DefaultClient, amd64, "2.6.4"); err != nil || got.Version != "2.6.4" {
		t.Errorf("指定版本解析错误: %+v %v", got, err)
	}
	if _, err := spec.Resolve(ctx, http.DefaultClient, amd64, "9.9.9"); err == nil {
		t.Error("不存在的版本应报错")
	}
	// 当前版本没有该平台的程序包
	if _, err := spec.Resolve(ctx, http.DefaultClient, binstall.Platform{OS: "linux", Arch: "arm64"}, ""); err == nil {
		t.Error("发布中没有对应平台的程序包时应报错")
	}
	if _, err := spec.Resolve(ctx, http.DefaultClient, binstall.Platform{OS: "linux", Arch: "386"}, ""); err == nil {
		t.Error("不支持的平台应报错")
	}
	if len(spec.Files) != 2 || spec.Files[0] != "easytier-core" || spec.Files[1] != "easytier-cli" {
		t.Errorf("应安装 easytier-core 与 easytier-cli: %v", spec.Files)
	}
}

// TestInstallFiles 验证 Windows 版一并安装依赖库：缺少 wintun.dll 等文件时 easytier-core.exe 无法启动。
func TestInstallFiles(t *testing.T) {
	if got := installFiles("linux"); len(got) != 2 {
		t.Errorf("Linux 只需要两个程序: %v", got)
	}
	got := installFiles("windows")
	for _, want := range []string{"easytier-core", "easytier-cli", "wintun.dll", "Packet.dll", "WinDivert64.sys"} {
		if !slices.Contains(got, want) {
			t.Errorf("Windows 缺少 %s: %v", want, got)
		}
	}
}
