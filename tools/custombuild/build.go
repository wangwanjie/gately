package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// build 构建前端与各目标的安装包，产物写到 out：
//   - tar：gately_<版本>_<profile>_<目标>.tar.gz，内含 gately（Windows 为 gately.exe）、LICENSE、README.md，与发布页同结构
//   - fpk：调用 deploy/fnos/build.sh（需要 fnpack），输出 gately_<版本>_<profile>_<目标>.fpk
//   - openwrt：只生成裸二进制 out/bin/<目标>/gately，ipk / apk 由调用方在带 apk-tools 的 Alpine 环境中用
//     deploy/openwrt/build.sh 打包（与发布流程相同），打包后按同样的规则命名
//   - docker：不在这里构建，由调用方用仓库根目录的 Dockerfile 加 resolve 输出的 dockerArgs 执行 docker buildx
func build(r resolved, version, out string, skipWeb bool) error {
	if len(r.Targets) == 0 {
		return fmt.Errorf("没有指定构建目标")
	}
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		return err
	}
	// 镜像由 Dockerfile 自己构建前端与二进制，这里跳过
	var bins []target
	for _, t := range r.Targets {
		if t.Kind == "docker" {
			fmt.Println("跳过 docker 目标：请用 Dockerfile 与 resolve 输出的 dockerArgs 构建镜像")
			continue
		}
		bins = append(bins, t)
	}
	if len(bins) == 0 {
		return nil
	}
	if !skipWeb {
		fmt.Printf("构建前端，语言 %s\n", r.Locales)
		if err := run("web", []string{"GATELY_LOCALES=" + r.Locales}, "pnpm", "install", "--frozen-lockfile"); err != nil {
			return err
		}
		if err := run("web", []string{"GATELY_LOCALES=" + r.Locales}, "pnpm", "build"); err != nil {
			return err
		}
	}
	commit := "unknown"
	if b, err := exec.Command("git", "rev-parse", "--short=7", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(b))
	}
	ldflags := fmt.Sprintf("-s -w -X github.com/wangwanjie/gately/internal/version.Version=%s -X github.com/wangwanjie/gately/internal/version.Commit=%s -X github.com/wangwanjie/gately/internal/version.BuildDate=%s",
		version, commit, time.Now().UTC().Format(time.RFC3339))
	for _, t := range bins {
		name := fmt.Sprintf("gately_%s_%s_%s", version, r.Profile, t.ID)
		exe := "gately"
		if t.GOOS == "windows" {
			exe = "gately.exe"
		}
		binDir := filepath.Join(out, "bin", t.ID)
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return err
		}
		bin := filepath.Join(binDir, exe)
		fmt.Printf("编译 %s（%s/%s）\n", t.ID, t.GOOS, t.GOARCH)
		// mips 路由器大多没有硬件浮点，使用软浮点以免运行时报非法指令（与发布流程一致）
		env := []string{"GOOS=" + t.GOOS, "GOARCH=" + t.GOARCH, "GOMIPS=softfloat", "CGO_ENABLED=0"}
		if err := run(".", env, "go", "build", "-trimpath", "-tags", r.Tags, "-ldflags", ldflags, "-o", bin, "./cmd/gately"); err != nil {
			return err
		}
		switch t.Kind {
		case "tar":
			if err := writeTarGz(filepath.Join(out, name+".tar.gz"), map[string]string{exe: bin, "LICENSE": "LICENSE", "README.md": "README.md"}, []string{exe, "LICENSE", "README.md"}); err != nil {
				return err
			}
		case "fpk":
			abs, _ := filepath.Abs(out)
			if err := run(".", nil, "deploy/fnos/build.sh", version, t.Platform, bin, abs); err != nil {
				return err
			}
			// deploy/fnos/build.sh 输出 gately_<去掉 v 的版本>_fnos_<平台>.fpk，改为带组合键的名称
			src := filepath.Join(out, fmt.Sprintf("gately_%s_fnos_%s.fpk", strings.TrimPrefix(version, "v"), t.Platform))
			if err := os.Rename(src, filepath.Join(out, name+".fpk")); err != nil {
				return err
			}
		case "openwrt":
			// 由调用方打包，见函数注释
		default:
			return fmt.Errorf("未知的目标类型: %s", t.Kind)
		}
	}
	return nil
}

// run 在 dir 中执行命令，输出直接打到终端；env 追加到当前环境变量之后。
func run(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s 失败: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// writeTarGz 按 order 的顺序把文件写入 tar.gz；files 为包内名称到本地路径的映射。
func writeTarGz(path string, files map[string]string, order []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, name := range order {
		src, err := os.Open(files[name])
		if err != nil {
			return err
		}
		st, err := src.Stat()
		if err != nil {
			src.Close()
			return err
		}
		hdr := &tar.Header{Name: name, Mode: int64(st.Mode().Perm()), Size: st.Size(), ModTime: st.ModTime()}
		if err := tw.WriteHeader(hdr); err != nil {
			src.Close()
			return err
		}
		_, err = io.Copy(tw, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}
