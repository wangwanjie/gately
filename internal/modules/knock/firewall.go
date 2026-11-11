package knock

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/spa"
)

// 防火墙不可用的原因代码，由前端翻译
const (
	ReasonOS          = "os"
	ReasonMissing     = "missing"
	ReasonPermission  = "permission"
	ReasonUnsupported = "unsupported"
	ReasonError       = "error"
)

// 防火墙后端
const (
	BackendNFT = "nftables"
	BackendIPT = "iptables"
)

// 轮询参数，可在测试中调整
var (
	pollInterval = 10 * time.Second
	// retryEvery 表示写入失败后每隔几次轮询重试一次
	retryEvery = 6
	cmdTimeout = 15 * time.Second
)

// sbinDirs 是 PATH 之外查找命令的目录：以服务方式运行时 PATH 里常常没有 sbin 目录
var sbinDirs = []string{"/usr/sbin", "/sbin", "/usr/bin", "/bin"}

// fwGrant 是一项放行：地址 + 协议端口，到 Until 为止。
type fwGrant struct {
	Addr   netip.Addr
	Access spa.Access
	Until  time.Time
}

type fwKey struct {
	addr   netip.Addr
	access spa.Access
}

// fwPlan 是要写入防火墙的规则。
type fwPlan struct {
	// Ports 为受保护端口，为空时不写规则（删除已有规则）
	Ports []spa.Access
	// Exempt 为无需敲门的网段
	Exempt []netip.Prefix
	// Grants 为仍然有效的放行，重建规则时一并写入
	Grants []fwGrant
}

// fwCounts 是规则计数。
type fwCounts struct {
	dropped, allowed uint64
}

// backend 是写入规则的防火墙实现。
type backend interface {
	kind() string
	version() string
	// coversIPv6 返回是否同时拦截 IPv6（iptables 后端在 ip6tables 不可用时只处理 IPv4）
	coversIPv6() bool
	// apply 删除旧规则，按 plan 重建全部规则与放行
	apply(ctx context.Context, plan fwPlan, now time.Time) error
	// add 写入放行（已存在时刷新到期时间），del 删除放行（不存在时忽略）
	add(ctx context.Context, grants []fwGrant, now time.Time) error
	del(ctx context.Context, grants []fwGrant) error
	// check 返回规则是否仍然存在（会被 nft flush ruleset、防火墙重载等外部操作清除）与计数
	check(ctx context.Context) (bool, fwCounts, error)
	remove(ctx context.Context) error
	text(ctx context.Context) (string, error)
}

// FirewallStatus 是防火墙拦截状态。
type FirewallStatus struct {
	// Needed 表示有启用的规则开启了防火墙拦截
	Needed bool `json:"needed"`
	// Active 表示拦截规则已写入系统防火墙
	Active bool `json:"active"`
	// Reason 为不可用原因代码，Error 为具体错误
	Reason  string `json:"reason,omitempty"`
	Error   string `json:"error,omitempty"`
	Backend string `json:"backend,omitempty"`
	Version string `json:"version,omitempty"`
	// NoIPv6 表示后端只拦截 IPv4（iptables 后端没有可用的 ip6tables），受保护端口经 IPv6 仍可直接访问
	NoIPv6 bool `json:"noIPv6,omitempty"`
	// Ports 为受保护端口
	Ports []string `json:"ports"`
	// Dropped 为被拦截的连接数，Allowed 为敲门放行后建立的连接数（规则重建后累计）
	Dropped   uint64    `json:"dropped"`
	Allowed   uint64    `json:"allowed"`
	CheckedAt time.Time `json:"checkedAt,omitzero"`
}

// firewall 管理拦截规则：设置变化时同步重建，放行与撤销由后台协程异步写入，定期检查规则是否被外部清除。
type firewall struct {
	log *logbuf.Logger
	// detect 查找可用的后端，返回不可用原因与说明，可在测试中替换
	detect func(ctx context.Context) (backend, string, string)

	mu     sync.Mutex
	be     backend
	plan   fwPlan
	grants map[fwKey]time.Time
	status FirewallStatus
	// base 为之前的规则（重建时被删除）累计的计数，界面显示基数与当前计数之和
	base fwCounts
	// pendingAdd / pendingDel 为等待后台协程写入的放行与撤销
	pendingAdd, pendingDel []fwGrant
	// resync 表示增量写入失败，下一次轮询完整重建
	resync bool
	// dirty 表示系统防火墙中可能留有本模块的规则（写入过或写入到一半失败），取消拦截与停止时据此清理，
	// 不依赖 status.Active：写入失败时 Active 为 false，但 iptables 多步写入、nft 旧表都可能已经生效
	dirty bool
	// lastErr 为最近一次记录到日志的错误，相同错误不重复记录
	lastErr  string
	onActive func()

	kick   chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
	now    func() time.Time
}

func newFirewall(log *logbuf.Logger) *firewall {
	return &firewall{log: log, detect: detectBackend, grants: map[fwKey]time.Time{}, kick: make(chan struct{}, 1), now: time.Now,
		status: FirewallStatus{Ports: []string{}}}
}

// detectBackend 查找 nft；没有 nft 或内核没有 nf_tables 时改用 iptables + ipset。
func detectBackend(ctx context.Context) (backend, string, string) {
	if runtime.GOOS != "linux" {
		return nil, ReasonOS, ""
	}
	if bin := lookCmd("nft"); bin != "" {
		b := &nftBackend{bin: bin, run: execCmd}
		_, err := b.run(ctx, "", bin, "list", "tables")
		if err == nil {
			v, _ := b.run(ctx, "", bin, "--version")
			b.ver = strings.TrimSpace(v)
			return b, "", ""
		}
		if isPermission(err) {
			return nil, ReasonPermission, err.Error()
		}
		if !kernelNoNFT(err.Error()) {
			return nil, ReasonError, err.Error()
		}
		// 内核没有 nf_tables（如群晖 DSM），继续查找 iptables
	}
	if b := findIPT(ctx); b != nil {
		return b, "", ""
	}
	if lookCmd("nft") != "" {
		return nil, ReasonUnsupported, ""
	}
	return nil, ReasonMissing, ""
}

func lookCmd(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range sbinDirs {
		p := d + "/" + name
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

func execCmd(ctx context.Context, stdin, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

func isPermission(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Operation not permitted") || strings.Contains(msg, "Permission denied")
}

// kernelNoNFT 判断 nft 基础查询的错误是否表示内核没有 nf_tables。
func kernelNoNFT(msg string) bool {
	low := strings.ToLower(msg)
	for _, s := range []string{"invalid argument", "protocol not supported", "address family not supported", "operation not supported"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// start 启动后台协程，onActive 在协程发现规则生效状态变化时调用（不持有 f.mu）。
func (f *firewall) start(ctx context.Context, onActive func()) {
	lctx, cancel := context.WithCancel(ctx)
	f.mu.Lock()
	f.onActive = onActive
	f.cancel = cancel
	f.done = make(chan struct{})
	done := f.done
	f.mu.Unlock()
	go func() {
		defer close(done)
		f.loop(lctx)
	}()
}

// stop 停止后台协程并删除规则。
func (f *firewall) stop() {
	f.mu.Lock()
	cancel, done := f.cancel, f.done
	f.cancel, f.done = nil, nil
	f.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cleanupLocked(context.Background()) {
		f.log.Infof("已移除防火墙规则，受保护端口恢复原状")
	}
	f.status.Active = false
	f.grants = map[fwKey]time.Time{}
	f.pendingAdd, f.pendingDel = nil, nil
}

// apply 按 plan 重建规则（设置或规则变化时调用）。
func (f *firewall) apply(ctx context.Context, plan fwPlan) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plan = plan
	f.grants = map[fwKey]time.Time{}
	for _, g := range plan.Grants {
		f.grants[fwKey{g.Addr, g.Access}] = g.Until
	}
	f.pendingAdd, f.pendingDel = nil, nil
	f.rebuildLocked(ctx)
}

// rebuildLocked 按当前 plan 与放行重建规则；没有受保护端口时删除规则。
func (f *firewall) rebuildLocked(ctx context.Context) {
	wasActive := f.status.Active
	f.status.Needed = len(f.plan.Ports) > 0
	f.status.Ports = formatPorts(f.plan.Ports)
	f.resync = false
	if !f.status.Needed {
		if wasActive {
			f.keepCountsLocked(ctx)
		}
		if f.cleanupLocked(ctx) {
			f.log.Infof("没有需要拦截的端口，已移除防火墙规则")
		}
		f.status.Active, f.status.Reason, f.status.Error = false, "", ""
		f.lastErr = ""
		return
	}
	if f.be == nil {
		be, reason, msg := f.detect(ctx)
		if be == nil {
			f.status.Active, f.status.Reason, f.status.Error = false, reason, msg
			f.logErrLocked(reason + ":" + msg)
			return
		}
		f.be = be
		f.status.Backend, f.status.Version = be.kind(), be.version()
		f.status.NoIPv6 = !be.coversIPv6()
		if f.status.NoIPv6 {
			f.log.Warnf("没有可用的 ip6tables（或内核没有 IPv6 mangle 表），防火墙拦截只覆盖 IPv4，受保护端口经 IPv6 仍可直接访问")
		}
	}
	if wasActive {
		f.keepCountsLocked(ctx)
	}
	now := f.now()
	plan := f.plan
	plan.Grants = f.liveGrantsLocked(now)
	f.dirty = true
	if err := f.be.apply(ctx, plan, now); err != nil {
		reason := ReasonError
		if isPermission(err) {
			reason = ReasonPermission
		}
		f.status.Active, f.status.Reason, f.status.Error = false, reason, err.Error()
		f.logErrLocked(err.Error())
		// 写入到一半的规则（如 iptables 的 IPv4 已写入、IPv6 失败）或事务回滚后留下的旧表仍会拦截，而状态已是未生效、
		// 放行不会再写入，必须立即删除。删除成功后丢弃后端，下次重试重新探测（例如 nft 被卸载、权限被收回）；
		// 删除也失败时保留后端，取消拦截与停止时继续用它清理
		if f.cleanupLocked(ctx) {
			f.be = nil
		}
		return
	}
	f.status.Active, f.status.Reason, f.status.Error = true, "", ""
	f.status.CheckedAt = now
	if !wasActive || f.lastErr != "" {
		f.log.Infof("已在 %s 中拦截 %s，敲门成功的地址才能访问", f.status.Backend, strings.Join(f.status.Ports, ", "))
	}
	f.lastErr = ""
}

// cleanupLocked 在系统防火墙中可能留有本模块规则时删除它们（幂等：表、链、集合不存在时不报错），
// 返回是否删除了规则。没有后端时重新探测；删除失败时保持 dirty，之后取消拦截、停止或重试时再删。
func (f *firewall) cleanupLocked(ctx context.Context) bool {
	if !f.dirty {
		return false
	}
	if f.be == nil {
		be, _, _ := f.detect(ctx)
		if be == nil {
			return false
		}
		f.be = be
	}
	if err := f.be.remove(ctx); err != nil {
		f.log.Warnf("删除防火墙规则失败: %v", err)
		return false
	}
	f.dirty = false
	return true
}

func (f *firewall) logErrLocked(msg string) {
	if msg == f.lastErr {
		return
	}
	f.lastErr = msg
	switch f.status.Reason {
	case ReasonOS:
		f.log.Warnf("当前系统不支持防火墙拦截，开启防火墙拦截的规则不会生效；协议分流与端口转发按敲门放行不受影响")
	case ReasonMissing:
		f.log.Warnf("没有找到 nft 或 iptables + ipset，防火墙拦截不会生效")
	case ReasonPermission:
		f.log.Warnf("没有修改防火墙的权限（需要 root；Docker 部署需要 host 网络与 cap_add: NET_ADMIN）: %s", f.status.Error)
	case ReasonUnsupported:
		f.log.Warnf("内核没有 nf_tables，且没有找到可用的 iptables + ipset，防火墙拦截不会生效")
	default:
		f.log.Errorf("写入防火墙规则失败: %s", f.status.Error)
	}
}

// liveGrantsLocked 返回未到期的放行，并清除到期记录。
func (f *firewall) liveGrantsLocked(now time.Time) []fwGrant {
	out := make([]fwGrant, 0, len(f.grants))
	for k, until := range f.grants {
		if !now.Before(until) || !slices.Contains(f.plan.Ports, k.access) {
			delete(f.grants, k)
			continue
		}
		out = append(out, fwGrant{Addr: k.addr, Access: k.access, Until: until})
	}
	return out
}

// keepCountsLocked 在删除或重建规则前把当前计数并入基数。
func (f *firewall) keepCountsLocked(ctx context.Context) {
	if f.be == nil {
		return
	}
	if ok, c, err := f.be.check(ctx); err == nil && ok {
		f.base.dropped += c.dropped
		f.base.allowed += c.allowed
	}
	f.status.Dropped, f.status.Allowed = f.base.dropped, f.base.allowed
}

// grant 登记放行，由后台协程写入。只针对受保护端口；已有更晚到期的放行时不缩短。
func (f *firewall) grant(list []fwGrant) {
	f.mu.Lock()
	for _, g := range list {
		if !slices.Contains(f.plan.Ports, g.Access) {
			continue
		}
		k := fwKey{g.Addr, g.Access}
		if old, ok := f.grants[k]; ok && !g.Until.After(old) {
			continue
		}
		f.grants[k] = g.Until
		f.pendingAdd = append(f.pendingAdd, g)
	}
	f.mu.Unlock()
	f.wake()
}

// revoke 撤销放行，由后台协程删除。
func (f *firewall) revoke(list []fwGrant) {
	if len(list) == 0 {
		return
	}
	f.mu.Lock()
	for _, g := range list {
		delete(f.grants, fwKey{g.Addr, g.Access})
		f.pendingDel = append(f.pendingDel, g)
	}
	f.mu.Unlock()
	f.wake()
}

func (f *firewall) wake() {
	select {
	case f.kick <- struct{}{}:
	default:
	}
}

// active 判断拦截规则是否已生效。
func (f *firewall) active() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status.Active
}

// ports 返回受保护端口。
func (f *firewall) ports() []spa.Access {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.plan.Ports)
}

// snapshot 返回当前状态。
func (f *firewall) snapshot() FirewallStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status
	st.Ports = slices.Clone(st.Ports)
	return st
}

// rules 返回写入防火墙的规则文本。
func (f *firewall) rules(ctx context.Context) (string, error) {
	f.mu.Lock()
	be, active := f.be, f.status.Active
	f.mu.Unlock()
	if be == nil || !active {
		return "", errors.New("防火墙拦截未生效")
	}
	return be.text(ctx)
}

func (f *firewall) loop(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.kick:
			f.flush(ctx)
		case <-t.C:
			tick++
			f.poll(ctx, tick)
		}
	}
}

// flush 写入等待中的放行与撤销。失败时标记 resync，由下一次轮询完整重建。
func (f *firewall) flush(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	add, del := f.pendingAdd, f.pendingDel
	f.pendingAdd, f.pendingDel = nil, nil
	if !f.status.Active || f.be == nil {
		return
	}
	if len(del) > 0 {
		if err := f.be.del(ctx, del); err != nil {
			f.log.Warnf("从防火墙删除放行失败，将重建规则: %v", err)
			f.resync = true
		}
	}
	if len(add) > 0 {
		if err := f.be.add(ctx, add, f.now()); err != nil {
			f.log.Warnf("写入防火墙放行失败，将重建规则: %v", err)
			f.resync = true
		}
	}
	if f.resync {
		f.wake()
	}
}

// poll 定期检查规则：被外部清除时重建，写入失败的每隔 retryEvery 次轮询重试，并更新计数。
func (f *firewall) poll(ctx context.Context, tick int) {
	f.mu.Lock()
	before := f.status.Active
	switch {
	case !f.status.Needed:
	case !f.status.Active:
		// 系统不支持时不重试；其余原因（权限、命令出错）定期重试
		if f.status.Reason != ReasonOS && tick%retryEvery == 0 {
			f.rebuildLocked(ctx)
		}
	case f.resync:
		f.log.Infof("重建防火墙规则")
		f.rebuildLocked(ctx)
	default:
		ok, c, err := f.be.check(ctx)
		switch {
		case err != nil:
			f.log.Warnf("读取防火墙规则失败: %v", err)
		case !ok:
			f.log.Warnf("防火墙规则被外部删除（如防火墙重载、nft flush ruleset），已重新写入")
			f.rebuildLocked(ctx)
		default:
			f.status.Dropped, f.status.Allowed = f.base.dropped+c.dropped, f.base.allowed+c.allowed
			f.status.CheckedAt = f.now()
		}
	}
	after := f.status.Active
	onActive := f.onActive
	f.mu.Unlock()
	if before != after && onActive != nil {
		onActive()
	}
}
