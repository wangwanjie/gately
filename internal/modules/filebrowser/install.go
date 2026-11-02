package filebrowser

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/proc"
)

// releaseRepo 是 FileBrowser Quantum 的 GitHub 仓库（GitHub 只把 stable 标为 latest，beta 不会作为最新版安装）。
const releaseRepo = "gtsteffaniak/filebrowser"

// assetName 返回上游发布中对应平台的文件名。FileBrowser Quantum 直接发布可执行文件，不打包。
func assetName(p binstall.Platform) (string, error) {
	switch p.OS + "/" + p.Arch {
	case "linux/amd64", "linux/arm64", "linux/riscv64", "darwin/amd64", "darwin/arm64", "freebsd/amd64":
		return p.OS + "-" + p.Arch + "-filebrowser", nil
	case "linux/arm":
		switch p.ARM {
		case "6", "7":
			return "linux-armv" + p.ARM + "-filebrowser", nil
		}
		return "", binstall.Unsupported("FileBrowser Quantum 没有提供 ARMv%s 的程序", p.ARM)
	case "windows/amd64":
		return "filebrowser.exe", nil
	}
	return "", binstall.Unsupported("FileBrowser Quantum 没有提供 %s 的程序", p)
}

var (
	quantumVersionRe = regexp.MustCompile(`Version\s*:\s*(\S+)`)
	legacyVersionRe  = regexp.MustCompile(`File Browser (v\S+)`)
)

// probeVersion 执行 `filebrowser version` 返回版本，以及程序是否为 FileBrowser Quantum。
// 原版 filebrowser 输出 “File Browser v2.x/提交”，Quantum 输出多行信息，其中有 “Version : v2.0.2-stable”。
func probeVersion(bin string) (string, bool) {
	out, _ := proc.Output(5*time.Second, bin, []string{"version"}, nil)
	text := string(out)
	if strings.Contains(text, "Quantum") {
		if m := quantumVersionRe.FindStringSubmatch(text); m != nil {
			return m[1], true
		}
		return "", true
	}
	if m := legacyVersionRe.FindStringSubmatch(text); m != nil {
		return m[1], false
	}
	return "", false
}

// installSpec 描述 FileBrowser Quantum 的在线安装。
func installSpec() binstall.Spec {
	return binstall.Spec{
		Key:   ID,
		Name:  "FileBrowser Quantum",
		Files: []string{binName},
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
			return binstall.GitHubAssetRelease(ctx, client, rel, rel.TagName, name)
		},
		Probe: func(path string) (string, error) {
			v, quantum := probeVersion(path)
			if !quantum {
				return v, errors.New("不是 FileBrowser Quantum")
			}
			return v, nil
		},
	}
}

// newInstaller 创建 FileBrowser Quantum 的安装器，安装或回滚后用新程序重启已启用的实例。
func (m *Module) newInstaller() *binstall.Installer {
	return binstall.New(installSpec(), binstall.Options{
		BinDir: filepath.Join(m.store.Dir(), "bin"),
		Logs:   m.logs,
		Client: func() *http.Client { return m.client() },
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
			// 让设置页重新探测版本，并用新程序重启实例
			m.versionPath = ""
			m.restartAllLocked()
		},
	})
}

// ProgramInstallers 返回模块托管的程序安装器，供 Gately 自动检查更新时一并检查托管程序。
func (m *Module) ProgramInstallers() []*binstall.Installer {
	return []*binstall.Installer{m.install}
}
