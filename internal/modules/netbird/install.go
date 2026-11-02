package netbird

import (
	"context"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wangwanjie/gately/internal/binstall"
)

// releaseRepo 是 NetBird 的 GitHub 仓库。
const releaseRepo = "netbirdio/netbird"

// archName 返回 NetBird 发布包中平台 p 的名称，例如 linux_amd64、linux_armv6、linux_mipsle_softfloat。
// 32 位 ARM 只有 ARMv6 构建（ARMv7 也能运行）；MIPS 选软浮点构建，没有硬件浮点单元的路由器也能运行。
func archName(p binstall.Platform) (string, error) {
	switch p.OS + "/" + p.Arch {
	case "linux/amd64", "linux/arm64", "linux/386",
		"darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64":
		return p.OS + "_" + p.Arch, nil
	case "linux/arm":
		if p.ARM != "5" {
			return "linux_armv6", nil
		}
	case "linux/mips", "linux/mipsle", "linux/mips64", "linux/mips64le":
		return p.OS + "_" + p.Arch + "_softfloat", nil
	}
	return "", binstall.Unsupported("NetBird 没有提供 %s 的程序", p)
}

// assetName 返回版本 ver（不带 v）在平台 p 上的发布包名；Windows 也是 tar.gz。
func assetName(ver string, p binstall.Platform) (string, error) {
	arch, err := archName(p)
	if err != nil {
		return "", err
	}
	return "netbird_" + ver + "_" + arch + ".tar.gz", nil
}

var nbVersionRe = regexp.MustCompile(`(?m)^\s*v?(\d+\.\d+\.\d+\S*)`)

// installSpec 描述 NetBird 的在线安装：发布包中只有一个 netbird 程序，既是命令行也是守护进程。
func installSpec() binstall.Spec {
	return binstall.Spec{
		Key:   ID,
		Name:  "NetBird",
		Files: []string{"netbird"},
		Check: func(p binstall.Platform) error {
			_, err := archName(p)
			return err
		},
		Resolve: func(ctx context.Context, client *http.Client, p binstall.Platform, version string) (*binstall.Release, error) {
			if _, err := archName(p); err != nil {
				return nil, err
			}
			rel, err := binstall.FetchGitHubRelease(ctx, client, releaseRepo, version)
			if err != nil {
				return nil, err
			}
			ver := strings.TrimPrefix(rel.TagName, "v")
			name, _ := assetName(ver, p)
			return binstall.GitHubAssetRelease(ctx, client, rel, ver, name, "netbird_"+ver+"_checksums.txt")
		},
		Probe: func(path string) (string, error) {
			return binstall.ProbeOutput(path, nbVersionRe, "version")
		},
	}
}

// newInstaller 创建 NetBird 的安装器，安装或回滚后用新程序重启。
func (m *Module) newInstaller() *binstall.Installer {
	return binstall.New(installSpec(), binstall.Options{
		BinDir: filepath.Join(m.store.Dir(), "bin"),
		Logs:   m.logBuf,
		Client: func() *http.Client { return m.httpClient() },
		GetProxy: func() string {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.settings.InstallProxy
		},
		SetProxy: func(proxy string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			s := m.settings
			s.InstallProxy = proxy
			if err := m.store.Save(ID, s); err != nil {
				return err
			}
			m.settings = s
			return nil
		},
		OnChange: func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			// 文件路径不变，清空版本缓存以重新探测
			m.versions = map[string]string{}
			m.applyLocked()
		},
	})
}

// ProgramInstallers 返回模块托管的程序安装器，供 Gately 自动检查更新时一并检查托管程序。
func (m *Module) ProgramInstallers() []*binstall.Installer {
	return []*binstall.Installer{m.install}
}
