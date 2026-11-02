package frp

import (
	"context"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wangwanjie/gately/internal/binstall"
)

// releaseRepo 是 frp 的 GitHub 仓库。
const releaseRepo = "fatedier/frp"

// archName 返回 frp 发布包中平台 p 的名称，例如 linux_amd64、linux_arm_hf。
// frp 的 32 位 ARM 包有两种：arm_hf 为 ARMv7 硬浮点，arm 为 ARMv5 软浮点（ARMv6 也用它）。
func archName(p binstall.Platform) (string, error) {
	switch p.OS + "/" + p.Arch {
	case "linux/amd64", "linux/arm64", "linux/riscv64", "linux/loong64",
		"linux/mips", "linux/mipsle", "linux/mips64", "linux/mips64le",
		"darwin/amd64", "darwin/arm64", "freebsd/amd64", "openbsd/amd64",
		"windows/amd64", "windows/arm64":
		return p.OS + "_" + p.Arch, nil
	case "linux/arm":
		if p.ARM == "7" {
			return "linux_arm_hf", nil
		}
		return "linux_arm", nil
	}
	return "", binstall.Unsupported("frp 没有提供 %s 的程序", p)
}

// assetName 返回版本 ver（不带 v）在平台 p 上的发布包名。
func assetName(ver string, p binstall.Platform) (string, error) {
	arch, err := archName(p)
	if err != nil {
		return "", err
	}
	ext := ".tar.gz"
	if p.OS == "windows" {
		ext = ".zip"
	}
	return "frp_" + ver + "_" + arch + ext, nil
}

var frpVersionRe = regexp.MustCompile(`(?m)^\s*v?(\d+\.\d+\.\d+\S*)`)

// installSpec 描述 frp 的在线安装：一个发布包同时包含 frpc 与 frps。
func installSpec() binstall.Spec {
	return binstall.Spec{
		Key:   ID,
		Name:  "frp",
		Files: []string{"frpc", "frps"},
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
			name, _ := assetName(strings.TrimPrefix(rel.TagName, "v"), p)
			return binstall.GitHubAssetRelease(ctx, client, rel, rel.TagName, name, "frp_sha256_checksums.txt")
		},
		Probe: func(path string) (string, error) {
			v, err := binstall.ProbeOutput(path, frpVersionRe, "-v")
			if err != nil {
				return "", err
			}
			return "v" + v, nil
		},
	}
}

// newInstaller 创建 frp 的安装器，安装或回滚后用新程序重启 frpc 与 frps。
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
			for _, b := range m.bins {
				b.versionPath = ""
			}
			m.syncLocked(true)
		},
	})
}

// ProgramInstallers 返回模块托管的程序安装器，供 Gately 自动检查更新时一并检查托管程序。
func (m *Module) ProgramInstallers() []*binstall.Installer {
	return []*binstall.Installer{m.install}
}
