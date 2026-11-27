package sysstat

import (
	"os"
	"path/filepath"
	"strings"
)

// containerEnv 是判断容器环境所需的信息，便于在测试里注入。
type containerEnv struct {
	// MarkerExists 报告标记文件是否存在
	MarkerExists func(path string) bool
	// EnvContainer 为环境变量 container 的值（systemd 与 LXC、Podman 等约定）
	EnvContainer string
	// Pid1Comm 为 PID 1 的进程名，读取不到时为空
	Pid1Comm string
	// Cgroup 为 /proc/1/cgroup 的内容，读取不到时为空
	Cgroup string
	// ExeName 为当前可执行文件名
	ExeName string
}

// containerMarkerFiles 存在任一文件即视为运行在 Docker / Podman 容器中。
var containerMarkerFiles = []string{"/.dockerenv", "/run/.containerenv"}

// cgroupContainerWords 出现在 /proc/1/cgroup 中说明 PID 1 处于容器的 cgroup 内。
var cgroupContainerWords = []string{"docker", "kubepods", "containerd", "libpod", "crio", "lxc", "podman"}

// containerInitNames 是容器里常见的 PID 1：直接运行程序、shell 入口脚本或极简 init。
// 宿主机的 PID 1 一般是 systemd、init、procd、launchd 等，不在其中。
var containerInitNames = map[string]bool{
	"sh": true, "bash": true, "ash": true, "dash": true,
	"tini": true, "dumb-init": true, "s6-svscan": true, "catatonit": true,
}

// detectContainer 判断是否运行在容器中。除 Docker / Podman 的标记文件外，
// 还认 container 环境变量、cgroup 与 PID 1：MikroTik RouterOS 等容器运行时不会创建 /.dockerenv。
func detectContainer(e containerEnv) bool {
	for _, f := range containerMarkerFiles {
		if e.MarkerExists != nil && e.MarkerExists(f) {
			return true
		}
	}
	if e.EnvContainer != "" {
		return true
	}
	cg := strings.ToLower(e.Cgroup)
	for _, w := range cgroupContainerWords {
		if strings.Contains(cg, w) {
			return true
		}
	}
	comm := strings.TrimSpace(e.Pid1Comm)
	if comm == "" {
		return false
	}
	// PID 1 就是 Gately 自己（直接作为容器入口）或常见的容器入口进程
	if containerInitNames[comm] {
		return true
	}
	exe := strings.TrimSuffix(e.ExeName, ".exe")
	return exe != "" && strings.HasPrefix(exe, comm) && len(comm) >= 4
}

// InContainer 判断 Gately 是否运行在容器中。
func InContainer() bool {
	exe, _ := os.Executable()
	env := containerEnv{
		MarkerExists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		EnvContainer: os.Getenv("container"),
		ExeName:      filepath.Base(exe),
	}
	if b, err := os.ReadFile("/proc/1/comm"); err == nil {
		env.Pid1Comm = string(b)
	}
	if b, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		env.Cgroup = string(b)
	}
	return detectContainer(env)
}
