package easytier

import (
	"context"
	"net/http"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/wangwanjie/gately/internal/binstall"
)

// releaseRepo 是 EasyTier 的 GitHub 仓库。
const releaseRepo = "EasyTier/EasyTier"

// archName 返回 EasyTier 发布包中平台 p 的名称，例如 linux-x86_64、macos-aarch64、windows-x86_64。
// 32 位 ARM 按 GOARM 选择：5 用 arm，6 用 armhf，7 用 armv7；发布的 Linux 程序都是静态链接，不依赖系统 C 库。
func archName(p binstall.Platform) (string, error) {
	switch p.OS + "/" + p.Arch {
	case "linux/amd64":
		return "linux-x86_64", nil
	case "linux/arm64":
		return "linux-aarch64", nil
	case "linux/arm":
		switch p.ARM {
		case "5":
			return "linux-arm", nil
		case "6":
			return "linux-armhf", nil
		}
		return "linux-armv7", nil
	case "linux/mips":
		return "linux-mips", nil
	case "linux/mipsle":
		return "linux-mipsel", nil
	case "linux/riscv64":
		return "linux-riscv64", nil
	case "linux/loong64":
		return "linux-loongarch64", nil
	case "darwin/amd64":
		return "macos-x86_64", nil
	case "darwin/arm64":
		return "macos-aarch64", nil
	case "windows/amd64":
		return "windows-x86_64", nil
	case "windows/arm64":
		return "windows-arm64", nil
	case "windows/386":
		return "windows-i686", nil
	case "freebsd/amd64":
		return "freebsd-13.2-x86_64", nil
	}
	return "", binstall.Unsupported("EasyTier 没有提供 %s 的程序", p)
}

// assetName 返回版本 ver（不带 v）在平台 p 上的发布包名，例如 easytier-linux-x86_64-v2.6.4.zip。
func assetName(ver string, p binstall.Platform) (string, error) {
	arch, err := archName(p)
	if err != nil {
		return "", err
	}
	return "easytier-" + arch + "-v" + ver + ".zip", nil
}

// etVersionRe 只取版本号的数字部分：easytier-core --version 输出 2.6.4-8428a89d，末尾是提交号，
// 而发布标签是 v2.6.4，两者需要能直接比较，否则每次“更新到最新版”都会重新下载。
var etVersionRe = regexp.MustCompile(`(?m)(\d+\.\d+\.\d+)`)

// installFiles 返回需要从发布包中取出的文件。Windows 版的 easytier-core.exe 依赖同目录下的 wintun.dll（TUN 网卡）、
// Packet.dll 与 WinDivert64.sys（UDP 广播中继），缺少任何一个都无法启动（退出码 0xc0000135，找不到 DLL），所以一并安装。
func installFiles(goos string) []string {
	files := []string{coreName, cliName}
	if goos == "windows" {
		files = append(files, "wintun.dll", "Packet.dll", "WinDivert64.sys")
	}
	return files
}

// installSpec 描述 EasyTier 的在线安装：发布包里有 easytier-core（守护进程）与 easytier-cli（状态命令）两个程序，
// 此外的 easytier-web 等管理端程序不需要。
func installSpec() binstall.Spec {
	return binstall.Spec{
		Key:   ID,
		Name:  "EasyTier",
		Files: installFiles(runtime.GOOS),
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
			return binstall.GitHubAssetRelease(ctx, client, rel, ver, name)
		},
		Probe: func(path string) (string, error) {
			return binstall.ProbeOutput(path, etVersionRe, "--version")
		},
	}
}

// newInstaller 创建 EasyTier 的安装器，安装或回滚后用新程序重启全部实例。
func (m *Module) newInstaller() *binstall.Installer {
	return binstall.New(installSpec(), binstall.Options{
		BinDir: filepath.Join(m.store.Dir(), "bin"),
		Logs:   m.logs,
		Client: func() *http.Client { return m.httpClient() },
		GetProxy: func() string {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.cfg.Settings.InstallProxy
		},
		SetProxy: func(proxy string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			old := m.cfg.Settings
			m.cfg.Settings.InstallProxy = proxy
			if err := m.saveLocked(); err != nil {
				m.cfg.Settings = old
				return err
			}
			return nil
		},
		OnChange: func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			// 文件路径不变，清空版本缓存以重新探测，并用新程序重启全部实例
			m.versions = map[string]string{}
			m.syncLocked(func(string) bool { return true })
		},
	})
}

// ProgramInstallers 返回模块托管的程序安装器，供 Gately 自动检查更新时一并检查托管程序。
func (m *Module) ProgramInstallers() []*binstall.Installer {
	return []*binstall.Installer{m.install}
}
