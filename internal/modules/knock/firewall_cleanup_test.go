package knock

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/spa"
)

func testLogger() *logbuf.Logger {
	return logbuf.New(100, nil).Logger("防火墙")
}

func (b *fakeBackend) setErrs(apply, remove error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applyErr, b.removeErr = apply, remove
}

func (b *fakeBackend) state() (present bool, removed int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.present, b.removed
}

// 写入失败时立即删除写入到一半的规则；删除也失败时，取消拦截与停止仍会清理，不以“已生效”为前提。
func TestFirewallFailureCleanup(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	if !env.m.fw.snapshot().Active {
		t.Fatal("初始应生效")
	}
	// 重建失败：残留规则被删除
	env.be.setErrs(errors.New("ip6tables-restore: line 3 failed"), nil)
	env.m.mu.Lock()
	env.m.applyLocked()
	env.m.mu.Unlock()
	if present, removed := env.be.state(); present || removed != 1 {
		t.Fatalf("写入失败后应删除残留规则: present=%v removed=%d", present, removed)
	}
	if env.m.fw.snapshot().Active {
		t.Fatal("写入失败后不应显示生效")
	}

	// 重建失败且删除也失败：规则残留，停用规则（取消拦截）时再次删除
	env.be.setErrs(errors.New("write failed"), errors.New("remove failed"))
	env.m.mu.Lock()
	env.m.applyLocked()
	env.m.mu.Unlock()
	if present, _ := env.be.state(); !present {
		t.Fatal("删除失败时规则应仍在（模拟）")
	}
	env.be.setErrs(nil, nil)
	env.m.mu.Lock()
	env.m.cfg.Rules[0].Enable = false
	env.m.ruleChangedLocked("r-ssh")
	env.m.mu.Unlock()
	if present, _ := env.be.state(); present {
		t.Fatal("取消拦截时应删除残留规则，即使拦截从未显示生效")
	}

	// 再次启用后写入失败、删除失败，停止模块时清理
	env.m.mu.Lock()
	env.m.cfg.Rules[0].Enable = true
	env.m.ruleChangedLocked("r-ssh")
	env.m.mu.Unlock()
	env.be.setErrs(errors.New("write failed"), errors.New("remove failed"))
	env.m.mu.Lock()
	env.m.applyLocked()
	env.m.mu.Unlock()
	env.be.setErrs(nil, nil)
	_ = env.m.Stop()
	if present, _ := env.be.state(); present {
		t.Fatal("停止模块时应删除残留规则")
	}
}

// 没有写入过规则时取消拦截不调用删除（没有 root 权限的 Linux 不会因此反复报错）。
func TestFirewallCleanupOnlyWhenDirty(t *testing.T) {
	env := newEnv(t)
	_ = env.m.Stop()
	if _, removed := env.be.state(); removed != 0 {
		t.Fatalf("没有写入过规则时不应删除: %d", removed)
	}
}

// fakeRunner 记录 iptables / ipset 命令，fail 返回非空时该命令失败。
type fakeRunner struct {
	mu   sync.Mutex
	cmds []string
	fail func(cmd string) bool
}

func (r *fakeRunner) run(_ context.Context, stdin, bin string, args ...string) (string, error) {
	cmd := strings.TrimSpace(bin + " " + strings.Join(args, " "))
	r.mu.Lock()
	r.cmds = append(r.cmds, cmd)
	fail := r.fail
	r.mu.Unlock()
	if fail != nil && fail(cmd) {
		return "", errors.New("failed: " + cmd)
	}
	if strings.HasSuffix(cmd, "-S PREROUTING") {
		return "-P PREROUTING ACCEPT\n", nil
	}
	return "", nil
}

func (r *fakeRunner) has(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.cmds {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// iptables：IPv6 链写入失败时 IPv4 的跳转还没有插入；防火墙随即删除两个地址族的链与集合。
func TestIPTPartialFailure(t *testing.T) {
	fr := &fakeRunner{fail: func(cmd string) bool { return strings.HasPrefix(cmd, "ip6tables-restore") }}
	be := &iptBackend{run: fr.run, ipset: "ipset", fams: []iptFam{
		{bin: "iptables", restore: "iptables-restore"},
		{v6: true, bin: "ip6tables", restore: "ip6tables-restore"},
	}}
	now := time.Unix(1000, 0)
	plan := fwPlan{Ports: []spa.Access{{Proto: "tcp", Port: 22}}, Grants: []fwGrant{
		{Addr: netip.MustParseAddr("203.0.113.5"), Access: spa.Access{Proto: "tcp", Port: 22}, Until: now.Add(time.Minute)},
	}}
	if err := be.apply(context.Background(), plan, now); err == nil {
		t.Fatal("IPv6 写入失败时应返回错误")
	}
	if fr.has("-I PREROUTING") {
		t.Fatal("全部地址族的链写完之前不应插入跳转")
	}

	f := newFirewall(testLogger())
	f.detect = func(context.Context) (backend, string, string) { return be, "", "" }
	f.apply(context.Background(), plan, nil)
	if f.snapshot().Active {
		t.Fatal("写入失败不应生效")
	}
	for _, want := range []string{"iptables -w -t mangle -X GATELY_KNOCK", "ip6tables -w -t mangle -X GATELY_KNOCK"} {
		if !fr.has(want) {
			t.Fatalf("写入失败后应清理，缺少 %q", want)
		}
	}
}

// iptables 后端没有 ip6tables 时，状态标明 IPv6 不受保护并记录警告。
func TestFirewallNoIPv6(t *testing.T) {
	if (&iptBackend{fams: []iptFam{{bin: "iptables"}}}).coversIPv6() {
		t.Fatal("只有 IPv4 时不应覆盖 IPv6")
	}
	if !(&nftBackend{}).coversIPv6() {
		t.Fatal("nftables 同时处理 IPv4 与 IPv6")
	}
	be := &fakeBackend{noIPv6: true}
	f := newFirewall(testLogger())
	f.detect = func(context.Context) (backend, string, string) { return be, "", "" }
	f.apply(context.Background(), fwPlan{Ports: []spa.Access{{Proto: "tcp", Port: 22}}}, nil)
	if st := f.snapshot(); !st.Active || !st.NoIPv6 {
		t.Fatalf("应生效并标明 IPv6 未覆盖: %+v", st)
	}
}

// 重建规则与收包协程登记放行并发进行：每个登记过的放行最终都在防火墙中，不会被重建时的旧快照覆盖（配合 -race）。
func TestFirewallGrantDuringRebuild(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	r := env.rule("r-ssh")
	const n = 60
	pkts := make([][]byte, n)
	for i := range pkts {
		pkts[i] = env.packet(t, r, spa.Message{})
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range n {
			a := netip.AddrFrom4([4]byte{203, 0, byte(i), 1})
			if err := env.m.handle(pkts[i], a, transportUDP, "UDP"); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range n {
			env.m.mu.Lock()
			env.m.applyLocked()
			env.m.mu.Unlock()
		}
	}()
	wg.Wait()
	waitFor(t, "全部放行写入防火墙", func() bool {
		env.be.mu.Lock()
		defer env.be.mu.Unlock()
		for i := range n {
			k := fwKey{netip.AddrFrom4([4]byte{203, 0, byte(i), 1}), spa.Access{Proto: "tcp", Port: 22}}
			if !env.be.set[k] {
				return false
			}
		}
		return true
	})
}

// 放行恰好在重建取快照之后登记：留在待写入队列中，重建完成后写入，不被清空。
func TestFirewallGrantAfterSnapshot(t *testing.T) {
	be := &fakeBackend{}
	f := newFirewall(testLogger())
	f.detect = func(context.Context) (backend, string, string) { return be, "", "" }
	f.start(context.Background(), nil)
	t.Cleanup(f.stop)
	ssh := spa.Access{Proto: "tcp", Port: 22}
	plan := fwPlan{Ports: []spa.Access{ssh}}
	f.apply(context.Background(), plan, nil)
	g := fwGrant{Addr: netip.MustParseAddr("203.0.113.50"), Access: ssh, Until: time.Now().Add(time.Minute)}
	done := make(chan struct{})
	f.apply(context.Background(), plan, func([]spa.Access) []fwGrant {
		// 快照不含 g；g 在快照之后由“收包协程”登记
		go func() {
			f.grant([]fwGrant{g})
			close(done)
		}()
		time.Sleep(20 * time.Millisecond)
		return nil
	})
	<-done
	waitFor(t, "快照之后登记的放行写入防火墙", func() bool {
		be.mu.Lock()
		defer be.mu.Unlock()
		return be.set[fwKey{g.Addr, g.Access}]
	})
}

// blockingBackend 的 apply 在 release 关闭前一直阻塞，模拟执行很慢的外部命令。
type blockingBackend struct {
	fakeBackend
	entered chan struct{}
	release chan struct{}
}

func (b *blockingBackend) apply(ctx context.Context, plan fwPlan, now time.Time) error {
	close(b.entered)
	<-b.release
	return b.fakeBackend.apply(ctx, plan, now)
}

// 外部命令执行期间（最长 15 秒）收包协程登记放行不被阻塞，放行在命令结束后写入。
func TestFirewallGrantNotBlockedByCommand(t *testing.T) {
	be := &blockingBackend{entered: make(chan struct{}), release: make(chan struct{})}
	f := newFirewall(testLogger())
	f.detect = func(context.Context) (backend, string, string) { return be, "", "" }
	f.start(context.Background(), nil)
	t.Cleanup(f.stop)
	ssh := spa.Access{Proto: "tcp", Port: 22}
	applied := make(chan struct{})
	go func() {
		f.apply(context.Background(), fwPlan{Ports: []spa.Access{ssh}}, nil)
		close(applied)
	}()
	<-be.entered
	g := fwGrant{Addr: netip.MustParseAddr("203.0.113.60"), Access: ssh, Until: time.Now().Add(time.Minute)}
	granted := make(chan struct{})
	go func() {
		f.grant([]fwGrant{g})
		_ = f.snapshot()
		close(granted)
	}()
	select {
	case <-granted:
	case <-time.After(time.Second):
		t.Fatal("grant 与 snapshot 不应等待外部命令")
	}
	close(be.release)
	<-applied
	waitFor(t, "命令结束后写入放行", func() bool {
		be.mu.Lock()
		defer be.mu.Unlock()
		return be.set[fwKey{g.Addr, g.Access}]
	})
}
