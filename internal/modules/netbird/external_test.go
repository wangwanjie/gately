package netbird

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubExternal 替换外部 NetBird 检测用到的网卡列表、套接字判断与状态查询，测试结束后恢复。
func stubExternal(t *testing.T, ifaces func() []string, sock bool, query func(global []string) (*rawStatus, error)) {
	t.Helper()
	oi, os_, oq, og := listKernelIfaces, socketExists, queryExternal, ifaceGrace
	listKernelIfaces = ifaces
	socketExists = func(string) bool { return sock }
	queryExternal = func(_ string, global []string) (*rawStatus, error) { return query(global) }
	ifaceGrace = 300 * time.Millisecond
	t.Cleanup(func() { listKernelIfaces, socketExists, queryExternal, ifaceGrace = oi, os_, oq, og })
}

func noIfaces() []string { return nil }

func notReachable([]string) (*rawStatus, error) { return nil, errors.New("connection refused") }

// TestDetectExternalKernel 验证启动前的检测：外部守护进程已连接且使用内核网卡时冲突；未连接或用户态不冲突；
// 不可达时忽略；本机一直存在 wt 网卡时在等待期结束后判定冲突，网卡很快消失（刚停止的自有实例）不冲突。
func TestDetectExternalKernel(t *testing.T) {
	targets := []externalTarget{{global: []string{"--daemon-addr=unix:///var/run/netbird.sock"}}}
	cases := []struct {
		name string
		st   *rawStatus
		want string
	}{
		{"已连接内核", &rawStatus{DaemonStatus: "Connected", KernelInterface: true}, "系统中的 NetBird 服务正以内核模式连接"},
		{"外部实例", nil, ""},
		{"已连接用户态", &rawStatus{DaemonStatus: "Connected", KernelInterface: false}, ""},
		{"未登录", &rawStatus{DaemonStatus: "NeedsLogin", KernelInterface: true}, ""},
	}
	for _, c := range cases {
		stubExternal(t, noIfaces, false, func([]string) (*rawStatus, error) { return c.st, nil })
		got := detectExternalKernel(context.Background(), "netbird", targets)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q", c.name, got)
		}
	}
	stubExternal(t, noIfaces, false, func([]string) (*rawStatus, error) {
		return &rawStatus{DaemonStatus: "Connected", KernelInterface: true}, nil
	})
	named := []externalTarget{{name: "公司", global: []string{"--daemon-addr=tcp://127.0.0.1:41731"}}}
	if got := detectExternalKernel(context.Background(), "netbird", named); !strings.HasPrefix(got, "外部实例 公司 正以内核模式连接") {
		t.Errorf("Gately 中的外部实例应按名称提示: %q", got)
	}
	stubExternal(t, noIfaces, false, notReachable)
	if got := detectExternalKernel(context.Background(), "netbird", targets); got != "" {
		t.Errorf("不可达的守护进程应忽略: %q", got)
	}

	stubExternal(t, func() []string { return []string{"wt0"} }, false, notReachable)
	if got := detectExternalKernel(context.Background(), "netbird", nil); !strings.Contains(got, "wt0") || !strings.Contains(got, "ip link delete wt0") {
		t.Errorf("一直存在的 wt0 应判定冲突并提示删除残留网卡: %q", got)
	}

	var mu sync.Mutex
	calls := 0
	stubExternal(t, func() []string {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls <= 1 {
			return []string{"wt0"}
		}
		return nil
	}, false, notReachable)
	if got := detectExternalKernel(context.Background(), "netbird", nil); got != "" {
		t.Errorf("很快消失的网卡不应判定冲突: %q", got)
	}

	stubExternal(t, func() []string { return []string{"wt0"} }, false, notReachable)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := detectExternalKernel(ctx, "netbird", nil); got != "" {
		t.Errorf("实例停止中不应报告冲突: %q", got)
	}
}

// TestExternalTargets 验证要检查的外部守护进程：Gately 中已启用的外部实例（未填地址时为系统默认套接字），
// 系统默认套接字存在且未被托管实例或外部实例使用时加入；停用的外部实例与实例自身不检查。
func TestExternalTargets(t *testing.T) {
	m := moduleIn(t, t.TempDir())
	ext := Options{ManageDaemon: false}
	ext2 := Options{ManageDaemon: false, DaemonAddr: "tcp://127.0.0.1:41731"}
	k := Options{ManageDaemon: true}
	m.cfg.Instances = []Instance{
		{Key: "k", Name: "kernel", Enable: true, Options: k},
		{Key: "e1", Name: "sys", Enable: true, Options: ext},
		{Key: "e2", Name: "tcp", Enable: true, Options: ext2},
		{Key: "e3", Name: "off", Enable: false, Options: ext2},
	}
	stubExternal(t, noIfaces, true, notReachable)
	m.mu.Lock()
	got := m.externalTargetsLocked(&m.cfg.Instances[0])
	m.mu.Unlock()
	var labels []string
	for _, x := range got {
		labels = append(labels, x.name+" "+strings.Join(x.global, " "))
	}
	want := []string{"sys --daemon-addr=unix:///var/run/netbird.sock", "tcp --daemon-addr=tcp://127.0.0.1:41731"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Errorf("外部实例占用系统套接字时不应重复检查: %v", labels)
	}

	m.cfg.Instances = m.cfg.Instances[:1]
	m.mu.Lock()
	got = m.externalTargetsLocked(&m.cfg.Instances[0])
	m.mu.Unlock()
	if len(got) != 1 || got[0].name != "" {
		t.Errorf("系统默认套接字存在时应检查: %+v", got)
	}
	// 托管实例自己使用系统默认套接字时不当作外部服务
	m.cfg.Instances[0].DaemonAddr = "unix://" + defaultDaemonSocket
	m.mu.Lock()
	got = m.externalTargetsLocked(&m.cfg.Instances[0])
	m.mu.Unlock()
	if len(got) != 0 {
		t.Errorf("托管实例使用的套接字不应检查: %+v", got)
	}
}

// TestForeignIfaceOnSave 验证保存与启用内核实例时的快速检查：本机已有不属于 Gately 的 wt 网卡时返回 400，
// 用户态实例与停用的内核实例不受影响。
func TestForeignIfaceOnSave(t *testing.T) {
	m := moduleIn(t, t.TempDir())
	m.cfg.Instances = nil
	srv := serve(t, m)
	base := srv.URL + "/instances"
	stubExternal(t, func() []string { return []string{"wt0"} }, false, notReachable)

	kernel := Instance{Name: "k", Enable: true, Options: Options{ManageDaemon: true, Port: 51830}}
	if code, msg := errResp(t, "POST", base, kernel); code != 400 || !strings.Contains(msg, "wt0") {
		t.Fatalf("已有外部 wt0 时应拒绝启用内核实例: %d %q", code, msg)
	}
	kernel.Enable = false
	var created Instance
	if code := doJSON(t, "POST", base, kernel, &created); code != 200 || created.Key == "" {
		t.Fatalf("停用的内核实例应能保存: %d", code)
	}
	if code, msg := errResp(t, "PUT", base+"/"+created.Key+"/enable", map[string]bool{"enable": true}); code != 400 || !strings.Contains(msg, "wt0") {
		t.Fatalf("启用时应拒绝: %d %q", code, msg)
	}
	user := Instance{Name: "u", Enable: true, Options: DefaultOptions()}
	if code := doJSON(t, "POST", base, user, nil); code != 200 {
		t.Fatalf("用户态实例不受网卡影响: %d", code)
	}
}

// TestKernelStartBlockedByExternal 验证启动内核实例前检测到外部内核 NetBird 时不启动守护进程并显示原因；
// 用户态实例照常启动。
func TestKernelStartBlockedByExternal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("使用 shell 脚本模拟程序")
	}
	dir := t.TempDir()
	m := moduleIn(t, dir)
	fakeDaemonScript(t, dir)
	stubExternal(t, noIfaces, true, func([]string) (*rawStatus, error) {
		return &rawStatus{DaemonStatus: "Connected", KernelInterface: true}, nil
	})
	u := DefaultOptions()
	u.Port = 51831
	m.cfg.Instances = []Instance{
		{Key: "k", Name: "kernel", Enable: true, Options: Options{ManageDaemon: true, Port: 51830}},
		{Key: "u", Name: "user", Enable: true, Options: u},
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "内核实例显示冲突原因", func() bool { return strings.Contains(runOf(m, "k").getErr(), "系统中的 NetBird 服务") })
	waitFor(t, "用户态实例启动", func() bool { return pidOf(m, "u") != 0 })
	if pidOf(m, "k") != 0 {
		t.Error("检测到外部内核 NetBird 时不应启动守护进程")
	}
	if _, err := os.Stat(filepath.Join(dir, "netbird", "k", "pid")); err == nil {
		t.Error("内核实例的守护进程不应运行过")
	}
}
