package netbird

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"runtime"
	"sort"
	"time"
)

// 外部内核模式 NetBird 的检测。
//
// 同一台机器上只能有一个内核模式的 NetBird：路由表编号与防火墙标记是固定的，后启动的会覆盖先启动的路由与规则。
// Gately 托管的实例之间由 checkConflicts 保证；系统中另装的 NetBird 服务（或手工运行的 netbird）不受 Gately 管理，
// 这里在启动托管的内核模式实例前检测它，检测到时不启动并提示原因。检测手段有两种，互为补充：
//   - 网卡：内核模式 NetBird 连接后会创建 WireGuard 网卡，默认名为 wt0（--interface-name 可改，惯例为 wt 加数字）。
//     Docker 使用 host 网络时也能看到宿主机的网卡，因此容器内同样有效。
//   - 控制接口：查询 Gately 中配置的外部实例，以及系统默认控制套接字 /var/run/netbird.sock（存在时），
//     状态为已连接且使用内核网卡即判定为冲突；可以发现改了网卡名的外部服务。
//
// 未连接的外部守护进程不占用路由与网卡，不算冲突。连接后才创建网卡或改用其他网卡名、且控制套接字不可见（例如容器未挂载）
// 的外部服务无法发现，属于检测范围之外。

// kernelIfaceRe 匹配内核模式 NetBird 的 WireGuard 网卡名。
var kernelIfaceRe = regexp.MustCompile(`^wt[0-9]+$`)

// defaultDaemonSocket 为 netbird 的系统默认控制套接字。
const defaultDaemonSocket = "/var/run/netbird.sock"

// externalStatusTimeout 为查询外部守护进程状态的超时，比 statusTimeout 短，避免拖慢实例启动与停止。
const externalStatusTimeout = 4 * time.Second

// ifaceGrace 为发现网卡后等待其消失的时长：刚停止的托管内核实例退出时会删除自己的网卡，留出时间避免误判。
var ifaceGrace = 5 * time.Second

// 以下变量在测试中替换。
var (
	// listKernelIfaces 返回本机现有的内核模式 NetBird 网卡名；只在 Linux 上检测
	listKernelIfaces = func() []string {
		if runtime.GOOS != "linux" {
			return nil
		}
		ifs, err := net.Interfaces()
		if err != nil {
			return nil
		}
		var out []string
		for _, it := range ifs {
			if kernelIfaceRe.MatchString(it.Name) {
				out = append(out, it.Name)
			}
		}
		sort.Strings(out)
		return out
	}
	// socketExists 判断系统默认控制套接字是否存在
	socketExists = func(path string) bool {
		fi, err := os.Stat(path)
		return err == nil && fi.Mode()&os.ModeSocket != 0
	}
	// queryExternal 查询外部守护进程的状态，global 为 --daemon-addr 等全局参数
	queryExternal = func(cli string, global []string) (*rawStatus, error) {
		return runStatusTimeout(cli, global, externalStatusTimeout)
	}
	// sleepCtx 等待 d 或 ctx 取消，返回 false 表示已取消
	sleepCtx = func(ctx context.Context, d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
			return true
		}
	}
)

// ownKernelRunningLocked 判断除 except 之外是否有托管的内核模式实例正在运行（其网卡属于 Gately 自己）。调用方持有 mu。
func (m *Module) ownKernelRunningLocked(except string) bool {
	for key, rt := range m.runs {
		if key == except || !rt.inst.kernel() || !rt.inst.Enable {
			continue
		}
		if rt.daemon.State().Running {
			return true
		}
	}
	return false
}

// foreignIfaceErr 在本机已有内核模式 NetBird 网卡、且网卡不属于 Gately 托管的实例时返回冲突原因，用于保存与启用时的快速检查。
// 调用方持有 mu。
func (m *Module) foreignIfaceErr(in *Instance) error {
	if !in.Enable || !in.kernel() {
		return nil
	}
	ifs := listKernelIfaces()
	if len(ifs) == 0 || m.ownKernelRunningLocked(in.Key) {
		return nil
	}
	// 实例自身正在运行时网卡是它自己的（例如修改已启用实例的备注）
	if rt := m.runs[in.Key]; rt != nil && rt.inst.kernel() && rt.daemon.State().Running {
		return nil
	}
	return foreignIfaceError(ifs[0])
}

func foreignIfaceError(name string) error {
	return fmt.Errorf("本机已有内核模式的 NetBird 网卡 %[1]s，系统中可能另有 NetBird 服务以内核模式运行；同一台机器只能有一个内核模式的 NetBird，请停止该服务或为本实例开启用户态网络。若是残留网卡，可执行 ip link delete %[1]s 后重启本实例", name)
}

// externalTarget 是一个要检查的外部守护进程。
type externalTarget struct {
	// name 为 Gately 中外部实例的名称，空串表示系统默认控制套接字上的服务
	name   string
	global []string
}

// externalTargetsLocked 列出启动内核实例 in 前要检查的外部守护进程：Gately 中已启用的外部实例，
// 以及系统默认控制套接字（存在且未被托管实例或已列出的外部实例使用时）。调用方持有 mu。
func (m *Module) externalTargetsLocked(in *Instance) []externalTarget {
	dir := m.store.Dir()
	used := map[string]bool{}
	var out []externalTarget
	for i := range m.cfg.Instances {
		o := &m.cfg.Instances[i]
		addr := o.daemonAddr(dir)
		if o.ManageDaemon {
			used[addr] = true
			continue
		}
		if !o.Enable || o.Key == in.Key {
			continue
		}
		if addr == "" {
			addr = "unix://" + defaultDaemonSocket
		}
		used[addr] = true
		out = append(out, externalTarget{name: o.Name, global: []string{"--daemon-addr=" + addr}})
	}
	sys := "unix://" + defaultDaemonSocket
	if !used[sys] && socketExists(defaultDaemonSocket) {
		out = append(out, externalTarget{global: []string{"--daemon-addr=" + sys}})
	}
	return out
}

// detectExternalKernel 在启动托管的内核模式实例前检测外部内核模式 NetBird，返回冲突原因，空串表示可以启动。
// 不持有模块锁；targets 由调用方持锁取得。ctx 取消时返回空串（实例正在停止，不必再报告）。
func detectExternalKernel(ctx context.Context, cli string, targets []externalTarget) string {
	for _, t := range targets {
		if ctx.Err() != nil {
			return ""
		}
		st, err := queryExternal(cli, t.global)
		if err != nil || st == nil {
			continue
		}
		if st.DaemonStatus == "Connected" && st.KernelInterface {
			// 两种提示分开写，整句参与翻译
			if t.name == "" {
				return "系统中的 NetBird 服务正以内核模式连接；同一台机器只能有一个内核模式的 NetBird，请停止它或为本实例开启用户态网络，然后重启本实例"
			}
			return fmt.Sprintf("外部实例 %s 正以内核模式连接；同一台机器只能有一个内核模式的 NetBird，请停止它或为本实例开启用户态网络，然后重启本实例", t.name)
		}
	}
	// 网卡：可能是刚停止的托管实例尚未删除的网卡，等待一段时间再判定
	deadline := time.Now().Add(ifaceGrace)
	for {
		ifs := listKernelIfaces()
		if len(ifs) == 0 {
			return ""
		}
		if !time.Now().Before(deadline) {
			return foreignIfaceError(ifs[0]).Error()
		}
		if !sleepCtx(ctx, 500*time.Millisecond) {
			return ""
		}
	}
}
