package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wangwanjie/gately/internal/binstall"
)

// pkgsURL 是 Tailscale 官方静态程序包的发布地址，测试中替换。
var pkgsURL = "https://pkgs.tailscale.com/stable"

// tarballArch 返回 Tailscale 静态包中平台 p 的架构名。官方只为 Linux 提供静态包，
// macOS 与 Windows 需要使用官方安装程序，此时只能在设置中填写已安装程序的路径。
func tarballArch(p binstall.Platform) (string, error) {
	if p.OS != "linux" {
		return "", binstall.Unsupported("Tailscale 只为 Linux 提供可直接运行的程序包，%s 请使用官方安装程序", p)
	}
	switch p.Arch {
	case "amd64", "arm64", "386", "arm", "riscv64", "mips", "mipsle", "mips64", "mips64le":
		return p.Arch, nil
	}
	return "", binstall.Unsupported("Tailscale 没有提供 %s 的程序", p)
}

var (
	tsVersionRe = regexp.MustCompile(`(?m)^\s*v?(\d+\.\d+\.\d+)`)
	// tsVersionArgRe 校验用户输入的版本号，它会拼进下载地址
	tsVersionArgRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// installSpec 描述 Tailscale 的在线安装：静态包中同时包含 tailscale 与 tailscaled。
func installSpec() binstall.Spec {
	return binstall.Spec{
		Key:   ID,
		Name:  "Tailscale",
		Files: []string{"tailscale", "tailscaled"},
		Check: func(p binstall.Platform) error {
			_, err := tarballArch(p)
			return err
		},
		Resolve: func(ctx context.Context, client *http.Client, p binstall.Platform, version string) (*binstall.Release, error) {
			arch, err := tarballArch(p)
			if err != nil {
				return nil, err
			}
			version = strings.TrimPrefix(version, "v")
			if version == "" {
				text, err := binstall.FetchText(ctx, client, pkgsURL+"/?mode=json")
				if err != nil {
					return nil, err
				}
				var idx struct {
					TarballsVersion string `json:"TarballsVersion"`
				}
				if err := json.Unmarshal([]byte(text), &idx); err != nil || idx.TarballsVersion == "" {
					return nil, fmt.Errorf("解析 Tailscale 版本信息失败")
				}
				version = idx.TarballsVersion
			} else if !tsVersionArgRe.MatchString(version) {
				return nil, fmt.Errorf("版本号格式应为 1.90.2")
			}
			name := fmt.Sprintf("tailscale_%s_%s.tgz", version, arch)
			sum, err := binstall.FetchText(ctx, client, pkgsURL+"/"+name+".sha256")
			if err != nil {
				return nil, fmt.Errorf("没有找到版本 %s: %v", version, err)
			}
			return &binstall.Release{Version: version, Asset: binstall.Asset{
				Name:   name,
				URL:    pkgsURL + "/" + name,
				SHA256: strings.ToLower(strings.TrimSpace(sum)),
			}}, nil
		},
		Probe: func(path string) (string, error) {
			return binstall.ProbeOutput(path, tsVersionRe, "version")
		},
	}
}

// newInstaller 创建 Tailscale 的安装器，安装或回滚后用新程序重启。
func (m *Module) newInstaller() *binstall.Installer {
	return binstall.New(installSpec(), binstall.Options{
		BinDir: filepath.Join(m.store.Dir(), "bin"),
		Logs:   m.logs,
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
