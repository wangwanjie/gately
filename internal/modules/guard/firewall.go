package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/openports"
)

// tableName 是 Gately 在 nftables 中使用的表（inet 族，同时处理 IPv4 与 IPv6）
const tableName = "gately"

// 防火墙不可用的原因代码，由前端翻译
const (
	ReasonDisabled   = "disabled"
	ReasonOS         = "os"
	ReasonMissing    = "missing"
	ReasonPermission = "permission"
	ReasonError      = "error"
	// ReasonUnsupported 表示内核没有 nf_tables（如群晖 DSM），补装软件包也无法解决，不再重试
	ReasonUnsupported = "unsupported"
)

// 防火墙轮询参数，可在测试中调整
var (
	pollInterval = 10 * time.Second
	// strictGrace 为 IPv6 严格模式在规则写入后延迟启用的时间，等待各服务开始监听、开放端口同步完成
	strictGrace = 15 * time.Second
	// retryEvery 表示规则写入失败后每隔几次轮询重试一次
	retryEvery = 6
	nftTimeout = 15 * time.Second
)

// nftCandidates 是 PATH 之外查找 nft 的位置：以服务方式运行时 PATH 里常常没有 sbin 目录
var nftCandidates = []string{"/usr/sbin/nft", "/sbin/nft", "/usr/bin/nft"}

// FirewallStatus 是防火墙同步状态。
type FirewallStatus struct {
	// Enabled 为设置中是否开启同步
	Enabled bool `json:"enabled"`
	// Active 为规则是否已写入系统防火墙
	Active bool `json:"active"`
	// Reason 为不可用原因代码，Error 为具体错误
	Reason  string `json:"reason,omitempty"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
	// Backend 为写入规则使用的防火墙：nftables，或内核没有 nf_tables 时的 iptables（legacy + ipset）
	Backend string `json:"backend,omitempty"`
	// Dropped 为各类规则丢弃的数据包数：ban（封禁表）、deny（网段与永久封禁）、trap、scan（内核直接封禁的地址）
	Dropped map[string]uint64 `json:"dropped"`
	// Hits 为诱饵端口与扫描检测规则的触发次数
	Hits map[string]uint64 `json:"hits"`
	// OpenPorts 为扫描检测视为开放的端口数
	OpenPorts int `json:"openPorts"`
	// TrapPorts 为实际生效的诱饵端口，TrapSkipped 为因正在监听而排除的诱饵端口
	TrapPorts   string    `json:"trapPorts"`
	TrapSkipped []int     `json:"trapSkipped"`
	CheckedAt   time.Time `json:"checkedAt,omitzero"`
}

// 防火墙后端
const (
	BackendNFT = "nftables"
	BackendIPT = "iptables"
)

// firewall 把封禁表同步到 nftables；内核没有 nf_tables 时改用 iptables（legacy）+ ipset，见 firewall_ipt.go。
type firewall struct {
	log *logbuf.Logger
	// run 执行 nft 命令，可在测试中替换
	run func(ctx context.Context, stdin string, args ...string) (string, error)

	mu     sync.Mutex
	cfg    Settings
	status FirewallStatus
	nft    string
	// ipt 不为 nil 时使用 iptables（legacy）+ ipset 写入规则
	ipt *iptBackend
	// iptClamp 为最近一次提示过的扫描阈值截断，相同时不重复提示
	iptClamp int
	// members 为已写入 ban4 / ban6 的键及其解封时间
	members map[string]time.Time
	// capped 为 iptables 后端中剩余时间超过 ipset 上限、按上限写入的键及其在内核中的到期时间
	capped map[string]time.Time
	// denyText 为最近一次写入 deny4 / deny6 的内容，内容不变时不重写
	denyText string
	// open / trap 为最近一次写入的开放端口与诱饵端口
	open, trap string
	// strictReady 表示 IPv6 严格模式已经就绪：规则刚写入时监听端口可能还没有全部开始监听，
	// 立即启用会把访问这些端口的正常访客封掉，所以要等 strictAt 之后的轮询确认端口同步过再启用
	strictReady bool
	strictAt    time.Time
	// seen 为各端口最近一次读到正在监听的时间。端口短暂停止监听（frps 重连、托管进程或监听器重启）时
	// 在 portGrace 内仍视为开放，避免恰好落在停机窗口的轮询把它移出开放端口集合，让重连的正常访客被严格模式封禁
	seen map[int]time.Time
	// dynWarned 为最近一次提示过的「使用随机端口的模块」，内容不变时不重复提示
	dynWarned string
	// dirty 表示封禁表有变化，等待轮询协程对账写入
	dirty    bool
	importFn func(source string, addrs []netip.Addr)
	// onActive 在轮询协程中发现规则生效状态变化（重试成功或重写失败）时调用，调用时不持有 f.mu
	onActive func()
	// local 为本机网卡所在的 IPv6 网段，忽略内网地址时加入白名单
	local []netip.Prefix
	// lastErr 为最近一次记录到日志的写入错误，重试时相同的错误不重复记录
	lastErr string
	// baseDropped / baseHits 为之前的表（保存设置、外部清除或停止时被删除重建）累计的计数。
	// 内核计数器随表一起归零，界面显示的是基数与当前表计数之和
	baseDropped, baseHits map[string]uint64

	kick   chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
}

func newFirewall(log *logbuf.Logger) *firewall {
	f := &firewall{log: log, members: map[string]time.Time{}, capped: map[string]time.Time{}, baseDropped: map[string]uint64{}, baseHits: map[string]uint64{}, kick: make(chan struct{}, 1)}
	f.run = f.execNFT
	return f
}

// findNFT 查找 nft 可执行文件，返回路径或不可用原因。
func findNFT() (string, string) {
	if runtime.GOOS != "linux" {
		return "", ReasonOS
	}
	if p, err := exec.LookPath("nft"); err == nil {
		return p, ""
	}
	for _, p := range nftCandidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, ""
		}
	}
	return "", ReasonMissing
}

func (f *firewall) execNFT(ctx context.Context, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, nftTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.nft, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

// classify 把 nft 的错误归类为不可用原因。
func classify(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "Operation not permitted") || strings.Contains(msg, "Permission denied") {
		return ReasonPermission
	}
	return ReasonError
}

// kernelUnsupported 判断 nft 基础查询的错误是否表示内核没有 nf_tables：
// 内核未编入 nf_tables 时 netlink 请求返回 EINVAL（Invalid argument），
// 连 nfnetlink 也没有时返回 EPROTONOSUPPORT / EAFNOSUPPORT / EOPNOTSUPP。
func kernelUnsupported(msg string) bool {
	low := strings.ToLower(msg)
	for _, s := range []string{"invalid argument", "protocol not supported", "address family not supported", "operation not supported"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// probeKernelLocked 在写入规则失败后执行最基础的 nft list tables，区分「内核不支持 nf_tables」与规则本身的问题。
// 规则脚本出错（如缺少某个集合类型的模块）时基础查询仍能成功，只有查询本身也失败才认为内核不支持。调用方持有 f.mu。
func (f *firewall) probeKernelLocked(ctx context.Context) string {
	_, err := f.run(ctx, "", "list", "tables")
	if err == nil {
		return ReasonError
	}
	if r := classify(err); r == ReasonPermission {
		return r
	}
	if kernelUnsupported(err.Error()) {
		return ReasonUnsupported
	}
	return ReasonError
}

// start 按设置写入规则并开始轮询；importFn 接收内核规则直接封禁的地址，onActive 见 firewall.onActive。
func (f *firewall) start(ctx context.Context, s Settings, local []netip.Prefix, importFn func(string, []netip.Addr), onActive func()) {
	f.mu.Lock()
	f.importFn, f.onActive, f.local = importFn, onActive, local
	f.mu.Unlock()
	f.apply(ctx, s)
	lctx, cancel := context.WithCancel(ctx)
	f.mu.Lock()
	f.cancel = cancel
	f.done = make(chan struct{})
	done := f.done
	f.mu.Unlock()
	go func() {
		defer close(done)
		f.loop(lctx)
	}()
}

// stop 停止轮询并删除 Gately 的表。
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
	if f.status.Active {
		// 删除表前保留计数，模块重新启动后继续累加
		f.keepCountsLocked(context.Background())
		if err := f.removeLocked(context.Background()); err != nil {
			f.log.Warnf("删除防火墙规则失败: %v", err)
		} else {
			f.log.Infof("已移除防火墙规则")
		}
	}
	f.status.Active = false
	f.members, f.capped = map[string]time.Time{}, map[string]time.Time{}
	f.dirty = false
	// 重新启动后各服务要重新开始监听，严格模式需要再等一轮端口同步
	f.strictReady, f.strictAt = false, time.Time{}
}

// snapshot 返回当前状态。
func (f *firewall) snapshot() FirewallStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status
	st.Dropped = cloneCounts(st.Dropped)
	st.Hits = cloneCounts(st.Hits)
	st.TrapSkipped = append([]int{}, st.TrapSkipped...)
	return st
}

func cloneCounts(m map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// active 判断规则是否已生效。
func (f *firewall) active() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status.Active
}

// change 标记封禁表有变化，由轮询协程与封禁表对账后写入。
// 对账只看封禁表的当前内容，不依赖变化通知的顺序，并发的封禁与解封不会让规则与封禁表不一致。
func (f *firewall) change(banlist.Change) {
	f.mu.Lock()
	if !f.status.Active {
		f.mu.Unlock()
		return
	}
	f.dirty = true
	f.mu.Unlock()
	select {
	case f.kick <- struct{}{}:
	default:
	}
}

// apply 按设置重建整张表并写入当前全部封禁，设置变更或规则被外部清除时调用。
func (f *firewall) apply(ctx context.Context, s Settings) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyLocked(ctx, s)
}

func (f *firewall) applyLocked(ctx context.Context, s Settings) {
	f.cfg = s
	strict := s.Scan.Enable && s.Scan.IPv6Strict
	switch {
	case !strict:
		f.strictReady, f.strictAt = false, time.Time{}
	case !f.strictReady && f.strictAt.IsZero():
		f.strictAt = time.Now().Add(strictGrace)
	}
	wasActive := f.status.Active
	version := f.status.Version
	// 重建会删除旧表，先把旧表的计数并入基数，保存设置后计数不清零
	f.keepCountsLocked(ctx)
	f.status = FirewallStatus{Enabled: s.Firewall, Version: version, Dropped: cloneCounts(f.baseDropped), Hits: cloneCounts(f.baseHits), TrapSkipped: []int{}}
	f.dirty = false
	f.members, f.capped = map[string]time.Time{}, map[string]time.Time{}
	if !s.Firewall {
		if wasActive {
			if err := f.removeLocked(ctx); err != nil {
				f.log.Warnf("删除防火墙规则失败: %v", err)
			}
			f.log.Infof("已关闭防火墙同步，规则已移除")
		}
		f.status.Reason = ReasonDisabled
		return
	}
	if f.nft == "" && f.ipt == nil {
		path, reason := findNFT()
		switch {
		case path != "":
			f.nft = path
			f.status.Version = ""
		case reason == ReasonMissing:
			// 没有 nft 命令时改用 iptables（legacy）+ ipset
			if ipt := findIPTFn(); ipt != nil {
				f.ipt = ipt
				f.status.Version = ipt.version
			}
		}
		if f.nft == "" && f.ipt == nil {
			f.status.Reason = reason
			return
		}
	}
	if f.ipt != nil {
		f.status.Backend = BackendIPT
		f.status.Version = f.ipt.version
	} else {
		f.status.Backend = BackendNFT
	}
	if f.status.Version == "" && f.ipt == nil {
		if out, err := f.run(ctx, "", "--version"); err == nil {
			f.status.Version = strings.TrimSpace(out)
		}
	}
	listening, lerr := f.listeningLocked()
	open, trap, skipped := portSets(s, listening)
	f.warnDynamicLocked(s)
	entries := banlist.Default.List()
	// 严格模式就绪之前按普通的阈值检测处理
	ss := s
	// 读不到监听端口时开放端口集合不完整，严格模式会把所有端口当成未开放，必须退回阈值检测
	ss.Scan.IPv6Strict = strict && f.strictReady && lerr == nil
	var members, capped map[string]time.Time
	var deny string
	var err error
	if f.ipt != nil {
		f.warnClampLocked(ss)
		members, capped, err = f.ipt.rebuild(ctx, ss, f.iptPlanLocked(entries, open, trap), entries, time.Now())
		deny = iptDenyText(entries)
	} else {
		var script string
		script, members, deny = buildScript(ss, f.local, entries, open, trap, time.Now())
		_, err = f.run(ctx, script, "-f", "-")
	}
	if err != nil {
		reason := classify(err)
		if reason == ReasonError && f.ipt == nil {
			reason = f.probeKernelLocked(ctx)
			if reason == ReasonUnsupported {
				// 内核没有 nf_tables 时改用 iptables（legacy）+ ipset 重新写入
				if ipt := findIPTFn(); ipt != nil {
					f.log.Infof("当前系统内核不支持 nftables，改用 iptables（legacy）与 ipset 写入防火墙规则")
					f.nft, f.ipt = "", ipt
					f.status.Version = ipt.version
					f.applyLocked(ctx, s)
					return
				}
			}
		}
		f.status.Reason = reason
		f.status.Error = err.Error()
		// 日志去重的键：内核不支持时只记一次，不随错误文本变化重复记录
		key := err.Error()
		if reason == ReasonUnsupported {
			key = ReasonUnsupported
		}
		if f.lastErr != key {
			f.lastErr = key
			if reason == ReasonUnsupported {
				f.log.Warnf("当前系统内核不支持 nftables（没有 nf_tables），也没有可用的 iptables（legacy）与 ipset，无法同步到系统防火墙，不再重试；封禁只对 Gately 自身的服务生效，诱饵端口改由 Gately 监听。Docker 部署请更新到最新镜像，直接安装请安装 ipset 与 iptables（legacy）后保存一次设置。错误: %v", err)
			} else {
				f.log.Errorf("写入防火墙规则失败: %v", err)
			}
		}
		return
	}
	f.lastErr = ""
	f.status.Active = true
	f.status.OpenPorts = countPorts(open)
	f.status.TrapPorts = formatPorts(trap)
	f.status.TrapSkipped = skipped
	f.members = members
	if capped != nil {
		f.capped = capped
	}
	f.denyText = deny
	f.open, f.trap = formatPorts(open), formatPorts(trap)
	if !wasActive {
		if f.ipt != nil {
			f.log.Infof("防火墙规则已写入 iptables（mangle 表 GATELY_* 链与 ipset 集合，%d 条封禁）", len(entries))
		} else {
			f.log.Infof("防火墙规则已写入 nftables（表 inet %s，%d 条封禁）", tableName, len(entries))
		}
	}
}

// portGrace 为端口停止监听后仍视为开放的时间，覆盖两到三次轮询（每 10 秒一次）
const portGrace = 30 * time.Second

// listeningLocked 读取正在监听的端口，并加上 portGrace 内监听过的端口。调用方持有 f.mu。
func (f *firewall) listeningLocked() (map[int]bool, error) {
	listening, err := listeningPortsFn()
	if err != nil {
		return listening, err
	}
	now := time.Now()
	if f.seen == nil {
		f.seen = map[int]time.Time{}
	}
	for p := range listening {
		f.seen[p] = now
	}
	out := make(map[int]bool, len(f.seen))
	for p, at := range f.seen {
		if now.Sub(at) > portGrace {
			delete(f.seen, p)
			continue
		}
		out[p] = true
	}
	return out, nil
}

// warnDynamicLocked 在 IPv6 严格模式开启、且有模块使用随机端口（如未设置被动端口范围的 FTP）时提示一次：
// 这些端口无法事先列入开放端口，IPv6 访客连接它们会被当成端口扫描封禁。调用方持有 f.mu。
func (f *firewall) warnDynamicLocked(s Settings) {
	cur := ""
	if s.Scan.Enable && s.Scan.IPv6Strict {
		_, dynamic := openports.All()
		cur = strings.Join(dynamic, ",")
	}
	if cur == f.dynWarned {
		return
	}
	f.dynWarned = cur
	if cur != "" {
		f.log.Warnf("IPv6 严格模式下，%s 使用随机分配的端口，IPv6 访客连接这些端口会被当成端口扫描封禁；请为它设置固定的端口范围（如 FTP 被动端口范围）", cur)
	}
}

// portSets 计算扫描检测视为开放的端口与实际生效的诱饵端口（去掉正在监听的端口）。
// 开放端口还包括各模块登记的临时端口（如 FTP 被动端口范围），见 openports。
func portSets(s Settings, listening map[int]bool) (open, trap []portRange, skipped []int) {
	for p := range listening {
		open = append(open, portRange{p, p})
	}
	ex, _ := parsePorts(s.Scan.ExcludePorts)
	open = append(open, ex...)
	extra, _ := openports.All()
	for _, r := range extra {
		open = append(open, portRange{r.Lo, r.Hi})
	}
	open = mergePorts(open)
	traps, _ := parsePorts(s.Trap.Ports)
	trap, skipped = subtractPorts(traps, listening)
	if skipped == nil {
		skipped = []int{}
	}
	return open, trap, skipped
}

// v6Mask 为 IPv6 按 /64 聚合时的掩码
const v6Mask = "ffff:ffff:ffff:ffff::"

// buildScript 生成重建整张表的 nft 脚本。先声明再删除表，保证表不存在时脚本也能执行，且整个替换是原子的。
func buildScript(s Settings, local []netip.Prefix, entries []banlist.Entry, open, trap []portRange, now time.Time) (script string, members map[string]time.Time, deny string) {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s {}\ndelete table inet %s\ntable inet %s {\n", tableName, tableName, tableName)
	for _, set := range []string{"ban4", "trap4", "scan4"} {
		fmt.Fprintf(&b, "\tset %s {\n\t\ttype ipv4_addr\n\t\tflags timeout\n\t}\n", set)
	}
	for _, set := range []string{"ban6", "trap6", "scan6"} {
		fmt.Fprintf(&b, "\tset %s {\n\t\ttype ipv6_addr\n\t\tflags timeout\n\t}\n", set)
	}
	b.WriteString("\tset deny4 {\n\t\ttype ipv4_addr\n\t\tflags interval\n\t}\n")
	b.WriteString("\tset deny6 {\n\t\ttype ipv6_addr\n\t\tflags interval\n\t}\n")
	allow4, allow6 := splitFamilies(mergePrefixes(allowPrefixes(s.ignoreSet(local).prefixes)))
	writeIntervalSet(&b, "allow4", "ipv4_addr", allow4)
	writeIntervalSet(&b, "allow6", "ipv6_addr", allow6)
	writePortSet(&b, "open", open)
	writePortSet(&b, "trap", trap)
	b.WriteString("\tset meter4 {\n\t\ttype ipv4_addr\n\t\tsize 65535\n\t\tflags dynamic,timeout\n\t\ttimeout 1m\n\t}\n")
	b.WriteString("\tset meter6 {\n\t\ttype ipv6_addr\n\t\tsize 65535\n\t\tflags dynamic,timeout\n\t\ttimeout 1m\n\t}\n")

	// 封禁规则放在连接跟踪（-200）之后、目的地址转换（-100）之前的 prerouting：发往本机、转发给容器或内网设备的流量都会被丢弃。
	// 回复方向的包属于本机（或内网设备）主动发起的连接，照常放行，即使对方地址被封禁，本机访问它（如 DNS、软件源）也不受影响
	b.WriteString("\tchain pre {\n\t\ttype filter hook prerouting priority -190; policy accept;\n")
	b.WriteString("\t\tiif \"lo\" accept\n")
	b.WriteString("\t\tct direction reply accept\n")
	for _, set := range []string{"ban", "trap", "scan"} {
		fmt.Fprintf(&b, "\t\tip saddr @%s4 counter drop comment \"%s\"\n", set, set)
		fmt.Fprintf(&b, "\t\tip6 saddr & %s @%s6 counter drop comment \"%s\"\n", v6Mask, set, set)
	}
	b.WriteString("\t\tip saddr @deny4 counter drop comment \"deny\"\n")
	b.WriteString("\t\tip6 saddr @deny6 counter drop comment \"deny\"\n")
	b.WriteString("\t}\n")

	// 诱饵端口与扫描检测只看发往本机的新连接（SYN），白名单地址不参与
	if s.Trap.Enable || s.Scan.Enable {
		b.WriteString("\tchain in {\n\t\ttype filter hook input priority -10; policy accept;\n")
		b.WriteString("\t\tiif \"lo\" accept\n")
		b.WriteString("\t\tip saddr @allow4 accept\n\t\tip6 saddr @allow6 accept\n")
		syn := "tcp flags & (fin|syn|rst|ack) == syn"
		// 诱饵端口集合为空时规则不匹配任何连接，之后端口不再被监听时由轮询补上
		if s.Trap.Enable {
			sec := banSeconds(s, s.Trap.BanSeconds)
			fmt.Fprintf(&b, "\t\tmeta nfproto ipv4 %s tcp dport @trap add @trap4 { ip saddr timeout %ds } counter drop comment \"trap-hit\"\n", syn, sec)
			fmt.Fprintf(&b, "\t\tmeta nfproto ipv6 %s tcp dport @trap add @trap6 { ip6 saddr & %s timeout %ds } counter drop comment \"trap-hit\"\n", syn, v6Mask, sec)
		}
		if s.Scan.Enable {
			sec := banSeconds(s, s.Scan.BanSeconds)
			n := s.Scan.Threshold
			fmt.Fprintf(&b, "\t\tmeta nfproto ipv4 %s tcp dport != @open add @meter4 { ip saddr limit rate over %d/minute burst %d packets } add @scan4 { ip saddr timeout %ds } counter drop comment \"scan-hit\"\n", syn, n, n, sec)
			if s.Scan.IPv6Strict {
				// 严格模式：IPv6 来源第一次连接开放端口以外的端口即封禁（按 /64）
				fmt.Fprintf(&b, "\t\tmeta nfproto ipv6 %s tcp dport != @open add @scan6 { ip6 saddr & %s timeout %ds } counter drop comment \"scan-hit\"\n", syn, v6Mask, sec)
			} else {
				fmt.Fprintf(&b, "\t\tmeta nfproto ipv6 %s tcp dport != @open add @meter6 { ip6 saddr & %s limit rate over %d/minute burst %d packets } add @scan6 { ip6 saddr & %s timeout %ds } counter drop comment \"scan-hit\"\n", syn, v6Mask, n, n, v6Mask, sec)
			}
		}
		b.WriteString("\t}\n")
	}
	b.WriteString("}\n")

	members = map[string]time.Time{}
	var ban4, ban6 []string
	for _, e := range entries {
		elem, set, ok := timedElement(e, now)
		if !ok {
			continue
		}
		members[e.Key] = e.Until
		if set == "ban4" {
			ban4 = append(ban4, elem)
		} else {
			ban6 = append(ban6, elem)
		}
	}
	writeElements(&b, "ban4", ban4)
	writeElements(&b, "ban6", ban6)
	deny = denyScript(entries)
	b.WriteString(deny)
	return b.String(), members, deny
}

// allowPrefixes 把白名单转换为 input 链放行集合的元素：IPv6 按封禁粒度放宽到 /64，
// 否则同一 /64 中其他设备触发诱饵端口或扫描检测时，内核会把白名单地址所在的整个 /64 一起封禁。
func allowPrefixes(in []netip.Prefix) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(in))
	for _, p := range in {
		if p.Addr().Is6() && p.Bits() > 64 {
			p = netip.PrefixFrom(p.Addr(), 64).Masked()
		}
		out = append(out, p)
	}
	return out
}

// banSeconds 返回功能的封禁秒数，0 表示沿用默认时长。
func banSeconds(s Settings, sec int) int {
	if sec > 0 {
		return sec
	}
	return s.BanSeconds
}

// timedElement 返回单个来源、有期限的封禁对应的集合与元素；永久封禁与网段返回 ok=false，写入 deny 集合。
func timedElement(e banlist.Entry, now time.Time) (elem, set string, ok bool) {
	if e.Permanent() || !banlist.IsSingle(e.Key) {
		return "", "", false
	}
	sec := int(e.Until.Sub(now).Seconds()) + 1
	if sec < 1 {
		return "", "", false
	}
	p, _ := banlist.KeyPrefix(e.Key)
	if p.Addr().Is4() {
		return fmt.Sprintf("%s timeout %ds", p.Addr(), sec), "ban4", true
	}
	return fmt.Sprintf("%s timeout %ds", p.Addr(), sec), "ban6", true
}

// denyScript 生成重写 deny4 / deny6 的语句：包含永久封禁与网段封禁，去掉被其他网段包含的项，避免区间冲突。
func denyScript(entries []banlist.Entry) string {
	var prefixes []netip.Prefix
	for _, e := range entries {
		if !e.Permanent() && banlist.IsSingle(e.Key) {
			continue
		}
		if p, ok := banlist.KeyPrefix(e.Key); ok {
			prefixes = append(prefixes, p)
		}
	}
	v4, v6 := splitFamilies(mergePrefixes(prefixes))
	var b strings.Builder
	fmt.Fprintf(&b, "flush set inet %s deny4\nflush set inet %s deny6\n", tableName, tableName)
	writeElements(&b, "deny4", prefixStrings(v4))
	writeElements(&b, "deny6", prefixStrings(v6))
	return b.String()
}

// mergePrefixes 去掉被其他网段包含的网段，返回有序结果。
func mergePrefixes(in []netip.Prefix) []netip.Prefix {
	list := append([]netip.Prefix{}, in...)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Bits() != list[j].Bits() {
			return list[i].Bits() < list[j].Bits()
		}
		return list[i].Addr().Less(list[j].Addr())
	})
	var out []netip.Prefix
	for _, p := range list {
		p = p.Masked()
		covered := false
		for _, q := range out {
			if q.Addr().BitLen() == p.Addr().BitLen() && q.Bits() <= p.Bits() && q.Contains(p.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Addr().Less(out[j].Addr()) })
	return out
}

func splitFamilies(list []netip.Prefix) (v4, v6 []netip.Prefix) {
	for _, p := range list {
		if p.Addr().Is4() {
			v4 = append(v4, p)
		} else {
			v6 = append(v6, p)
		}
	}
	return v4, v6
}

func prefixStrings(list []netip.Prefix) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		if p.IsSingleIP() {
			out = append(out, p.Addr().String())
		} else {
			out = append(out, p.String())
		}
	}
	return out
}

func writeIntervalSet(b *strings.Builder, name, typ string, list []netip.Prefix) {
	fmt.Fprintf(b, "\tset %s {\n\t\ttype %s\n\t\tflags interval\n", name, typ)
	if len(list) > 0 {
		fmt.Fprintf(b, "\t\telements = { %s }\n", strings.Join(prefixStrings(list), ", "))
	}
	b.WriteString("\t}\n")
}

func writePortSet(b *strings.Builder, name string, list []portRange) {
	fmt.Fprintf(b, "\tset %s {\n\t\ttype inet_service\n\t\tflags interval\n", name)
	if len(list) > 0 {
		parts := make([]string, 0, len(list))
		for _, p := range list {
			parts = append(parts, p.String())
		}
		fmt.Fprintf(b, "\t\telements = { %s }\n", strings.Join(parts, ", "))
	}
	b.WriteString("\t}\n")
}

// writeElements 写出 add element 语句，元素较多时分批，避免单条语句过长。
func writeElements(b *strings.Builder, set string, elems []string) {
	for i := 0; i < len(elems); i += 500 {
		end := min(i+500, len(elems))
		fmt.Fprintf(b, "add element inet %s %s { %s }\n", tableName, set, strings.Join(elems[i:end], ", "))
	}
}

// loop 合并写入封禁变化，并定期读取计数与内核直接封禁的地址、刷新监听端口、确认规则仍然存在，规则写入失败时定期重试。
func (f *firewall) loop(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	// 启动时管理界面等服务可能还没开始监听，稍后尽快刷新一次开放端口，避免扫描检测把它们当成未开放端口
	early := time.NewTimer(3 * time.Second)
	defer early.Stop()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-early.C:
			f.poll(ctx)
		case <-f.kick:
			f.flush(ctx)
		case <-t.C:
			tick++
			if tick%retryEvery == 0 {
				f.retry(ctx)
			}
			f.flush(ctx)
			f.poll(ctx)
		}
	}
}

// retry 在规则未生效、且原因可能自行消失（写入出错、稍后才安装 nft）时重新写入。
// 内核不支持（ReasonUnsupported）、权限不足与非 Linux 平台不会自行恢复，不重试；保存设置或重启时会重新检测。
func (f *firewall) retry(ctx context.Context) {
	f.mu.Lock()
	st := f.status
	if st.Active || !st.Enabled || (st.Reason != ReasonError && st.Reason != ReasonMissing) {
		f.mu.Unlock()
		return
	}
	f.applyLocked(ctx, f.cfg)
	active, onActive := f.status.Active, f.onActive
	f.mu.Unlock()
	if active {
		f.log.Infof("防火墙规则重试写入成功")
		if onActive != nil {
			onActive()
		}
	}
}

// flush 把封禁表与已写入 nftables 的内容对账：增删 ban 集合中有期限的单个来源，重写 deny 集合（网段与永久封禁）。
func (f *firewall) flush(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.status.Active || !f.dirty {
		return
	}
	f.dirty = false
	now := time.Now()
	entries := banlist.Default.List()
	if f.ipt != nil {
		f.flushIPTLocked(ctx, entries, now)
		return
	}
	want := map[string]banlist.Entry{}
	for _, e := range entries {
		if _, _, ok := timedElement(e, now); ok {
			want[e.Key] = e
		}
	}
	var stmts []string
	for key, until := range f.members {
		e, keep := want[key]
		if keep && e.Until.Equal(until) {
			continue
		}
		// 自然到期的元素已被内核删除，只有提前解封或需要更新超时时间时才删除
		if until.After(now) {
			stmts = append(stmts, deleteStmt(key))
		}
		delete(f.members, key)
	}
	for key, e := range want {
		if _, ok := f.members[key]; ok {
			continue
		}
		elem, set, _ := timedElement(e, now)
		stmts = append(stmts, fmt.Sprintf("add element inet %s %s { %s }", tableName, set, elem))
		f.members[key] = e.Until
	}
	if deny := denyScript(entries); deny != f.denyText {
		stmts = append(stmts, strings.TrimSpace(deny))
		f.denyText = deny
	}
	if len(stmts) == 0 {
		return
	}
	// 删除排在添加之前，更新超时时间时先删后加
	sort.SliceStable(stmts, func(i, j int) bool {
		return strings.HasPrefix(stmts[i], "delete ") && !strings.HasPrefix(stmts[j], "delete ")
	})
	if _, err := f.run(ctx, strings.Join(stmts, "\n")+"\n", "-f", "-"); err == nil {
		return
	}
	// 整批失败（通常是要删除的元素已被内核清除）时逐条执行，忽略“元素不存在”的错误
	for _, st := range stmts {
		if _, err := f.run(ctx, st+"\n", "-f", "-"); err != nil && !(strings.HasPrefix(st, "delete ") && strings.Contains(err.Error(), "No such file")) {
			f.log.Warnf("更新防火墙失败: %v", err)
		}
	}
}

// setLocal 更新本机所在的 IPv6 网段并重写 input 链的放行集合。
func (f *firewall) setLocal(ctx context.Context, local []netip.Prefix) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.local = local
	if !f.status.Active {
		return
	}
	if f.ipt != nil {
		open, trap := f.iptCurrentPortsLocked()
		if err := f.ipt.restoreChains(ctx, f.effectiveLocked(), f.iptPlanLocked(nil, open, trap), iptInChains); err != nil {
			f.log.Warnf("更新白名单失败: %v", err)
		}
		return
	}
	allow4, allow6 := splitFamilies(mergePrefixes(allowPrefixes(f.cfg.ignoreSet(local).prefixes)))
	var b strings.Builder
	fmt.Fprintf(&b, "flush set inet %s allow4\nflush set inet %s allow6\n", tableName, tableName)
	writeElements(&b, "allow4", prefixStrings(allow4))
	writeElements(&b, "allow6", prefixStrings(allow6))
	if _, err := f.run(ctx, b.String(), "-f", "-"); err != nil {
		f.log.Warnf("更新白名单失败: %v", err)
	}
}

// deleteStmt 返回从 ban 集合删除单个来源的语句。
func deleteStmt(key string) string {
	p, _ := banlist.KeyPrefix(key)
	set := "ban4"
	if p.Addr().Is6() {
		set = "ban6"
	}
	return fmt.Sprintf("delete element inet %s %s { %s }", tableName, set, p.Addr())
}

// keepCountsLocked 把当前显示的计数（基数加当前表计数）记为新的基数，在表被删除或重建前调用。
// 规则生效时直接读取内核的最新计数；读取失败或表已被外部删除时退回最近一次轮询的结果。
func (f *firewall) keepCountsLocked(ctx context.Context) {
	dropped, hits := f.status.Dropped, f.status.Hits
	if f.status.Active && f.ipt != nil {
		if d, h, err := f.ipt.counters(ctx); err == nil {
			dropped, hits = addCounts(f.baseDropped, d), addCounts(f.baseHits, h)
		}
	} else if f.status.Active {
		if out, err := f.run(ctx, "", "-j", "list", "table", "inet", tableName); err == nil {
			var doc nftDoc
			if json.Unmarshal([]byte(out), &doc) == nil {
				d, h := doc.counters()
				dropped, hits = addCounts(f.baseDropped, d), addCounts(f.baseHits, h)
			}
		}
	}
	// 未生效时 status 中的计数就是基数本身，这里重复调用不会重复累加
	if dropped != nil {
		f.baseDropped = cloneCounts(dropped)
	}
	if hits != nil {
		f.baseHits = cloneCounts(hits)
	}
	// 状态同步为刚读到的最新计数：模块停止后再启动时，未生效分支以 status 为基数，
	// 不同步的话会退回停止前最后一次轮询的值，丢掉其间的命中
	f.status.Dropped, f.status.Hits = cloneCounts(f.baseDropped), cloneCounts(f.baseHits)
}

// addCounts 返回两组计数逐项相加的结果。
func addCounts(a, b map[string]uint64) map[string]uint64 {
	out := cloneCounts(a)
	for k, v := range b {
		out[k] += v
	}
	return out
}

// nftDoc 是 nft -j list table 的输出结构（只取用到的字段）。
type nftDoc struct {
	Nftables []struct {
		Set *struct {
			Name string            `json:"name"`
			Elem []json.RawMessage `json:"elem"`
		} `json:"set"`
		Rule *struct {
			Comment string            `json:"comment"`
			Expr    []json.RawMessage `json:"expr"`
		} `json:"rule"`
	} `json:"nftables"`
}

// counters 汇总带注释规则的计数：注释以 -hit 结尾的是触发次数，其余是丢弃的数据包数。
func (d nftDoc) counters() (dropped, hits map[string]uint64) {
	dropped, hits = map[string]uint64{}, map[string]uint64{}
	for _, item := range d.Nftables {
		if item.Rule == nil || item.Rule.Comment == "" {
			continue
		}
		if n, ok := ruleCounter(item.Rule.Expr); ok {
			if name, isHit := strings.CutSuffix(item.Rule.Comment, "-hit"); isHit {
				hits[name] += n
			} else {
				dropped[item.Rule.Comment] += n
			}
		}
	}
	return dropped, hits
}

// elemAddr 解析集合元素：可能是字符串，也可能是带超时信息的对象。
func elemAddr(raw json.RawMessage) (netip.Addr, bool) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var obj struct {
			Elem struct {
				Val string `json:"val"`
			} `json:"elem"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			return netip.Addr{}, false
		}
		s = obj.Elem.Val
	}
	a, err := netip.ParseAddr(s)
	return a, err == nil
}

// ruleCounter 返回规则表达式中计数器的包数。
func ruleCounter(expr []json.RawMessage) (uint64, bool) {
	for _, e := range expr {
		var c struct {
			Counter *struct {
				Packets uint64 `json:"packets"`
			} `json:"counter"`
		}
		if json.Unmarshal(e, &c) == nil && c.Counter != nil {
			return c.Counter.Packets, true
		}
	}
	return 0, false
}

// poll 读取计数与内核直接封禁的地址，并刷新监听端口。
func (f *firewall) poll(ctx context.Context) {
	f.mu.Lock()
	if !f.status.Active {
		f.mu.Unlock()
		return
	}
	if f.ipt != nil {
		f.pollIPTLocked(ctx)
		return
	}
	out, err := f.run(ctx, "", "-j", "list", "table", "inet", tableName)
	if err != nil {
		if strings.Contains(err.Error(), "No such file") {
			// 表被其他程序（例如防火墙重载时清空全部规则）删除，重新写入
			f.log.Warnf("防火墙规则已被外部清除，重新写入")
			f.status.Active = false
			f.applyLocked(ctx, f.cfg)
			active, onActive := f.status.Active, f.onActive
			f.mu.Unlock()
			if !active && onActive != nil {
				onActive()
			}
			return
		}
		f.status.Error = err.Error()
		f.mu.Unlock()
		return
	}
	var doc nftDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		f.status.Error = "无法解析 nft 的 JSON 输出: " + err.Error()
		f.mu.Unlock()
		return
	}
	f.status.Error = ""
	inbox := map[string][]netip.Addr{}
	for _, item := range doc.Nftables {
		if item.Set != nil {
			switch item.Set.Name {
			case "trap4", "trap6", "scan4", "scan6":
				for _, raw := range item.Set.Elem {
					if a, ok := elemAddr(raw); ok {
						inbox[item.Set.Name] = append(inbox[item.Set.Name], a)
					}
				}
			}
		}
	}
	dropped, hits := doc.counters()
	f.status.Dropped, f.status.Hits = addCounts(f.baseDropped, dropped), addCounts(f.baseHits, hits)
	f.status.CheckedAt = time.Now()
	portsOK := f.refreshPortsLocked(ctx)
	if portsOK && f.cfg.Scan.Enable && f.cfg.Scan.IPv6Strict && !f.strictReady && !f.strictAt.IsZero() && !time.Now().Before(f.strictAt) {
		f.strictReady = true
		f.applyLocked(ctx, f.cfg)
		if f.status.Active {
			f.log.Infof("IPv6 严格模式已启用：IPv6 来源连接开放端口以外的端口即被封禁")
		}
	}
	importFn := f.importFn
	f.mu.Unlock()

	// 先写入封禁表（随后由 flush 加入 ban 集合），再从内核的临时集合中删除，避免出现空档
	for name, addrs := range inbox {
		source := banlist.SourceTrap
		if strings.HasPrefix(name, "scan") {
			source = banlist.SourceScan
		}
		if importFn != nil {
			importFn(source, addrs)
		}
	}
	if len(inbox) > 0 {
		f.flush(ctx)
		var stmts []string
		for name, addrs := range inbox {
			elems := make([]string, 0, len(addrs))
			for _, a := range addrs {
				elems = append(elems, a.String())
			}
			stmts = append(stmts, fmt.Sprintf("delete element inet %s %s { %s }", tableName, name, strings.Join(elems, ", ")))
		}
		f.mu.Lock()
		for _, st := range stmts {
			if _, err := f.run(ctx, st+"\n", "-f", "-"); err != nil && !strings.Contains(err.Error(), "No such file") {
				f.log.Warnf("清理临时集合失败: %v", err)
			}
		}
		f.mu.Unlock()
	}
}

// refreshPortsLocked 按当前监听端口更新开放端口与诱饵端口集合。
func (f *firewall) refreshPortsLocked(ctx context.Context) bool {
	listening, err := f.listeningLocked()
	if err != nil {
		return false
	}
	open, trap, skipped := portSets(f.cfg, listening)
	f.warnDynamicLocked(f.cfg)
	o, t := formatPorts(open), formatPorts(trap)
	f.status.OpenPorts = countPorts(open)
	f.status.TrapPorts = t
	f.status.TrapSkipped = skipped
	if o == f.open && t == f.trap {
		return true
	}
	if f.ipt != nil {
		ss := f.effectiveLocked()
		if err := f.ipt.restoreChains(ctx, ss, iptPlan{allow: f.allowLocked(), open: open, trap: trap}, iptInChains); err != nil {
			f.log.Warnf("更新端口集合失败: %v", err)
			return false
		}
		f.open, f.trap = o, t
		return true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "flush set inet %s open\nflush set inet %s trap\n", tableName, tableName)
	if len(open) > 0 {
		fmt.Fprintf(&b, "add element inet %s open { %s }\n", tableName, strings.ReplaceAll(o, ",", ", "))
	}
	if len(trap) > 0 {
		fmt.Fprintf(&b, "add element inet %s trap { %s }\n", tableName, strings.ReplaceAll(t, ",", ", "))
	}
	if _, err := f.run(ctx, b.String(), "-f", "-"); err != nil {
		f.log.Warnf("更新端口集合失败: %v", err)
		return false
	}
	f.open, f.trap = o, t
	return true
}

// rules 返回 Gately 表的当前内容（nft list table 的文本），供界面查看。
func (f *firewall) rules(ctx context.Context) (string, error) {
	f.mu.Lock()
	active := f.status.Active
	f.mu.Unlock()
	if !active {
		return "", errors.New("防火墙规则未生效")
	}
	f.mu.Lock()
	ipt := f.ipt
	f.mu.Unlock()
	if ipt != nil {
		return ipt.text(ctx)
	}
	return f.run(ctx, "", "list", "table", "inet", tableName)
}
