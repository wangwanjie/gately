package cloudflared

import (
	"context"
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/wangwanjie/gately/internal/binstall"
)

// releaseRepo 是 cloudflared 的 GitHub 仓库。
const releaseRepo = "cloudflare/cloudflared"

// assetName 返回 cloudflared 发布中平台 p 的文件名：Linux 与 Windows 为可执行文件本身，macOS 为 tgz 压缩包。
// 32 位 ARM 中 armhf 为 ARMv7 硬浮点，arm 兼容 ARMv6 及更早的软浮点。
func assetName(p binstall.Platform) (string, error) {
	switch p.OS + "/" + p.Arch {
	case "linux/amd64", "linux/arm64", "linux/386":
		return "cloudflared-linux-" + p.Arch, nil
	case "linux/arm":
		if p.ARM == "7" {
			return "cloudflared-linux-armhf", nil
		}
		return "cloudflared-linux-arm", nil
	case "darwin/amd64", "darwin/arm64":
		return "cloudflared-darwin-" + p.Arch + ".tgz", nil
	case "windows/amd64", "windows/386":
		return "cloudflared-windows-" + p.Arch + ".exe", nil
	}
	return "", binstall.Unsupported("cloudflared 没有提供 %s 的程序", p)
}

var cloudflaredVersionRe = regexp.MustCompile(`version (\S+)`)

// installSpec 描述 cloudflared 的在线安装。
func installSpec() binstall.Spec {
	return binstall.Spec{
		Key:   ID,
		Name:  "cloudflared",
		Files: []string{"cloudflared"},
		Check: func(p binstall.Platform) error {
			_, err := assetName(p)
			return err
		},
		Resolve: func(ctx context.Context, client *http.Client, p binstall.Platform, version string) (*binstall.Release, error) {
			name, err := assetName(p)
			if err != nil {
				return nil, err
			}
			rel, err := binstall.FetchGitHubRelease(ctx, client, releaseRepo, version)
			if err != nil {
				return nil, err
			}
			// 旧版本的文件没有 GitHub 摘要时，从发布说明中的 SHA256 列表查找
			return binstall.GitHubAssetRelease(ctx, client, rel, rel.TagName, name)
		},
		Probe: func(path string) (string, error) {
			return binstall.ProbeOutput(path, cloudflaredVersionRe, "--version")
		},
	}
}

// newInstaller 创建 cloudflared 的安装器，安装或回滚后用新程序重启已启用的隧道。
func (m *Module) newInstaller() *binstall.Installer {
	return binstall.New(installSpec(), binstall.Options{
		BinDir: filepath.Join(m.store.Dir(), "bin"),
		Logs:   m.logs,
		Client: func() *http.Client { return m.httpClient() },
		GetProxy: func() string {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.cfg.Install.Proxy
		},
		SetProxy: func(proxy string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			old := m.cfg.Install
			m.cfg.Install.Proxy = proxy
			if err := m.saveLocked(); err != nil {
				m.cfg.Install = old
				return err
			}
			return nil
		},
		OnChange: func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			// 文件路径不变，清空版本缓存以重新探测
			m.versionPath = ""
			m.restartAllLocked()
		},
	})
}

// ProgramInstallers 返回模块托管的程序安装器，供 Gately 自动检查更新时一并检查托管程序。
func (m *Module) ProgramInstallers() []*binstall.Installer {
	return []*binstall.Installer{m.install}
}
