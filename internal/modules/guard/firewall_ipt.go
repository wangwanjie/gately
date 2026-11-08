package guard

// iptables（legacy）+ ipset 后端：内核没有 nf_tables（如群晖 DSM）、或系统没有 nft 命令时使用。
//
// 规则结构与 nftables 后端一一对应，全部写在 mangle 表 Gately 自己的链中：
//   - mangle PREROUTING 第一条跳到 GATELY_PRE：丢弃封禁集合、诱饵端口与扫描检测集合中的来源，再进入 GATELY_DENY
//     （网段与永久封禁）。PREROUTING 在路由之前，发往本机、转发给 Docker 容器与内网设备的流量都会被丢弃；
//     回复方向的包属于本机主动发起的连接，照常放行；
//   - mangle INPUT 第一条跳到 GATELY_IN：诱饵端口与扫描检测，命中时由内核把来源加入集合（SET 目标）并丢弃。
//
// 用 mangle 表而不是 filter 表：群晖防火墙、Docker 会在 filter 表的 INPUT / FORWARD 最前面插入自己的规则，
// 被它们放行的连接不会再经过排在后面的规则；mangle 表先于 filter 表处理，跳转规则的位置由轮询维护。
//
// 群晖内核只有 hash:ip 类型的集合，没有 hash:net，网段与永久封禁写成 GATELY_DENY 中的逐条规则；
// IPv6 集合使用 netmask 64，按 /64 聚合，与 nftables 后端的 `ip6 saddr & ffff:ffff:ffff:ffff::` 一致。
// 群晖内核也没有 comment 模块，规则用专用链区分，不依赖注释。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
)

// Gately 在 mangle 表中使用的链
const (
	iptPre     = "GATELY_PRE"
	iptDeny    = "GATELY_DENY"
	iptIn      = "GATELY_IN"
	iptTrap    = "GATELY_TRAP"
	iptScan    = "GATELY_SCAN"
	iptScanHit = "GATELY_SCANHIT"
	// iptMaxHitcount 为 recent 模块单个来源能记录的最多连接次数，扫描阈值超过时按此值处理
	iptMaxHitcount = 255
	// iptMultiport 为 multiport 一条规则最多容纳的端口数（一个范围占两个）
	iptMultiport = 15
	// iptMaxTimeout 为 ipset 元素超时的上限（秒，约 24.8 天，ipset 按毫秒以 int32 存储）。
	// 封禁剩余时间更长时先按上限写入，临近到期前由对账按剩余时间重新写入
	iptMaxTimeout = 2147483
	// iptRefresh 为按上限写入的元素在到期前多久重新写入
	iptRefresh = 24 * time.Hour
	// iptMaxElem 为每个集合最多容纳的元素数（ipset 默认 65536，长期封禁累计超过时写入会失败）
	iptMaxElem = 1048576
)

// iptChains 为全部自有链；iptInChains 为随设置、白名单与端口变化而重写的 input 部分
var (
	iptChains   = []string{iptPre, iptDeny, iptIn, iptTrap, iptScan, iptScanHit}
	iptInChains = []string{iptIn, iptTrap, iptScan, iptScanHit}
)

// iptJumps 为内建链到自有链的跳转，必须是内建链的第一条规则
var iptJumps = [][2]string{{"PREROUTING", iptPre}, {"INPUT", iptIn}}

// iptCandidates 为 PATH 之外查找命令的目录：以服务方式运行时 PATH 里常常没有 sbin 目录
var iptCandidates = []string{"/usr/sbin", "/sbin", "/usr/bin", "/bin"}

// iptFamily 是一个地址族（IPv4 / IPv6）使用的命令。
type iptFamily struct {
	v6                 bool
	bin, save, restore string
}

// iptBackend 通过 iptables（legacy）与 ipset 命令写入规则。
type iptBackend struct {
	// run 执行命令，可在测试中替换
	run     func(ctx context.Context, stdin, bin string, args ...string) (string, error)
	fams    []iptFamily
	ipset   string
	version string
}

// findIPTFn 查找可用的 iptables（legacy）与 ipset，可在测试中替换
var findIPTFn = findIPT

// findIPT 查找 iptables（legacy）、iptables-save / iptables-restore 与 ipset，缺少任何一个返回 nil。
// 优先使用名为 *-legacy 的命令；iptables 本身是 nf_tables 版本（iptables-nft）时不能使用，内核不支持时它同样无法工作。
// ip6tables 缺失时只处理 IPv4。
func findIPT() *iptBackend {
	if runtime.GOOS != "linux" {
		return nil
	}
	ipset := lookCmd("ipset")
	if ipset == "" {
		return nil
	}
	b := &iptBackend{run: execCmd, ipset: ipset}
	for _, v6 := range []bool{false, true} {
		name := "iptables"
		if v6 {
			name = "ip6tables"
		}
		fam, ok := legacyFamily(name, v6)
		if !ok {
			if !v6 {
				return nil
			}
			continue
		}
		b.fams = append(b.fams, fam)
	}
	ctx, cancel := context.WithTimeout(context.Background(), nftTimeout)
	defer cancel()
	ver, _ := b.run(ctx, "", b.fams[0].bin, "-V")
	sv, _ := b.run(ctx, "", ipset, "version")
	sv, _, _ = strings.Cut(strings.TrimSpace(sv), ",")
	b.version = strings.TrimSpace(strings.TrimSpace(ver) + ", " + sv)
	return b
}

// legacyFamily 查找 name（iptables / ip6tables）的 legacy 版本及其 save / restore 命令。
func legacyFamily(name string, v6 bool) (iptFamily, bool) {
	fam := iptFamily{v6: v6}
	if p := lookCmd(name + "-legacy"); p != "" {
		fam.bin, fam.save, fam.restore = p, lookCmd(name+"-legacy-save"), lookCmd(name+"-legacy-restore")
	} else if p := lookCmd(name); p != "" {
		out, err := exec.Command(p, "-V").Output()
		if err != nil || strings.Contains(string(out), "nf_tables") {
			return fam, false
		}
		fam.bin, fam.save, fam.restore = p, lookCmd(name+"-save"), lookCmd(name+"-restore")
	}
	return fam, fam.bin != "" && fam.save != "" && fam.restore != ""
}

// lookCmd 在 PATH 与常见 sbin 目录中查找命令。
func lookCmd(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range iptCandidates {
		p := d + "/" + name
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// execCmd 执行命令，失败时返回 stderr 的内容。
func execCmd(ctx context.Context, stdin, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, nftTimeout)
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
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

// iptSet 返回集合名，kind 为 ban / trap / scan。
func iptSet(kind string, v6 bool) string {
	if v6 {
		return "gately_" + kind + "6"
	}
	return "gately_" + kind + "4"
}

// iptRecent 返回扫描检测使用的 recent 列表名。
func iptRecent(v6 bool) string {
	if v6 {
		return "gately_meter6"
	}
	return "gately_meter4"
}

// hasV6 判断是否有可用的 ip6tables。
func (b *iptBackend) hasV6() bool {
	for _, f := range b.fams {
		if f.v6 {
			return true
		}
	}
	return false
}

// setNames 返回全部集合名。
func (b *iptBackend) setNames() []string {
	var out []string
	for _, f := range b.fams {
		for _, kind := range []string{"ban", "trap", "scan"} {
			out = append(out, iptSet(kind, f.v6))
		}
	}
	return out
}

// createSets 返回创建全部集合的 ipset restore 语句。IPv6 集合按 /64 聚合。
func (b *iptBackend) createSets() string {
	var sb strings.Builder
	for _, f := range b.fams {
		for _, kind := range []string{"ban", "trap", "scan"} {
			if f.v6 {
				fmt.Fprintf(&sb, "create %s hash:ip family inet6 netmask 64 timeout 0 maxelem %d\n", iptSet(kind, true), iptMaxElem)
			} else {
				fmt.Fprintf(&sb, "create %s hash:ip family inet timeout 0 maxelem %d\n", iptSet(kind, false), iptMaxElem)
			}
		}
	}
	return sb.String()
}

// iptElement 返回单个来源、有期限的封禁对应的 ipset add 语句；永久封禁与网段返回 ok=false，写入 GATELY_DENY。
// 剩余时间超过 ipset 的上限时按上限写入，capped 为 true，exp 为元素在内核中的到期时间。
func iptElement(e banlist.Entry, now time.Time) (stmt string, exp time.Time, capped, ok bool) {
	if e.Permanent() || !banlist.IsSingle(e.Key) {
		return "", time.Time{}, false, false
	}
	sec := int(e.Until.Sub(now).Seconds()) + 1
	if sec < 1 {
		return "", time.Time{}, false, false
	}
	if sec > iptMaxTimeout {
		sec, capped = iptMaxTimeout, true
	}
	p, _ := banlist.KeyPrefix(e.Key)
	return fmt.Sprintf("add %s %s timeout %d", iptSet("ban", p.Addr().Is6()), p.Addr(), sec), now.Add(time.Duration(sec) * time.Second), capped, true
}

// iptSetTimeout 返回 SET 目标使用的超时秒数，不超过 ipset 的上限。
func iptSetTimeout(sec int) int {
	return min(sec, iptMaxTimeout)
}

// iptDelStmt 返回从封禁集合删除单个来源的 ipset 语句。
func iptDelStmt(key string) string {
	p, _ := banlist.KeyPrefix(key)
	return fmt.Sprintf("del %s %s", iptSet("ban", p.Addr().Is6()), p.Addr())
}

// iptDenyPrefixes 返回写入 GATELY_DENY 的网段（永久封禁与网段封禁），去掉被其他网段包含的项。
func iptDenyPrefixes(entries []banlist.Entry) []netip.Prefix {
	var prefixes []netip.Prefix
	for _, e := range entries {
		if !e.Permanent() && banlist.IsSingle(e.Key) {
			continue
		}
		if p, ok := banlist.KeyPrefix(e.Key); ok {
			prefixes = append(prefixes, p)
		}
	}
	return mergePrefixes(prefixes)
}

// iptDenyText 返回 GATELY_DENY 的内容标识，内容不变时不重写。
func iptDenyText(entries []banlist.Entry) string {
	return strings.Join(prefixStrings(iptDenyPrefixes(entries)), ",")
}

// multiportChunks 把端口区间分成 multiport 能容纳的若干组（单个端口占 1，范围占 2）。
func multiportChunks(list []portRange) []string {
	var out []string
	var cur []string
	used := 0
	for _, p := range list {
		cost, text := 1, strconv.Itoa(p.lo)
		if p.hi > p.lo {
			cost, text = 2, fmt.Sprintf("%d:%d", p.lo, p.hi)
		}
		if used+cost > iptMultiport {
			out = append(out, strings.Join(cur, ","))
			cur, used = nil, 0
		}
		cur = append(cur, text)
		used += cost
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, ","))
	}
	return out
}

// iptRules 生成一个地址族在 mangle 表中各自有链的规则（不含表头与链声明）。
// s 为生效的设置（严格模式未就绪时 IPv6Strict 为 false），allow 为白名单，deny 为网段与永久封禁。
func iptRules(s Settings, v6 bool, allow, deny []netip.Prefix, open, trap []portRange) map[string][]string {
	rules := map[string][]string{}
	add := func(chain, rule string) {
		rules[chain] = append(rules[chain], "-A "+chain+" "+rule)
	}
	add(iptPre, "-i lo -j RETURN")
	add(iptPre, "-m conntrack --ctdir REPLY -j RETURN")
	for _, kind := range []string{"ban", "trap", "scan"} {
		add(iptPre, fmt.Sprintf("-m set --match-set %s src -j DROP", iptSet(kind, v6)))
	}
	add(iptPre, "-j "+iptDeny)
	for _, p := range deny {
		if p.Addr().Is6() == v6 {
			add(iptDeny, "-s "+p.String()+" -j DROP")
		}
	}
	if !s.Trap.Enable && !s.Scan.Enable {
		return rules
	}
	// 诱饵端口与扫描检测只看发往本机的新连接（SYN），白名单地址不参与
	add(iptIn, "-i lo -j RETURN")
	for _, p := range allow {
		if p.Addr().Is6() == v6 {
			add(iptIn, "-s "+p.String()+" -j RETURN")
		}
	}
	mask := ""
	if v6 {
		mask = " --mask " + v6Mask
	}
	if s.Trap.Enable {
		// 诱饵端口为空时没有跳转规则，之后端口不再被监听时由轮询补上
		for _, c := range multiportChunks(trap) {
			add(iptIn, "-p tcp --syn -m multiport --dports "+c+" -j "+iptTrap)
		}
		add(iptTrap, fmt.Sprintf("-j SET --add-set %s src --exist --timeout %d", iptSet("trap", v6), iptSetTimeout(banSeconds(s, s.Trap.BanSeconds))))
		add(iptTrap, "-j DROP")
	}
	if s.Scan.Enable {
		add(iptIn, "-p tcp --syn -j "+iptScan)
		for _, c := range multiportChunks(open) {
			add(iptScan, "-p tcp -m multiport --dports "+c+" -j RETURN")
		}
		if v6 && s.Scan.IPv6Strict {
			// 严格模式：IPv6 来源第一次连接开放端口以外的端口即封禁（按 /64）
			add(iptScan, "-j "+iptScanHit)
		} else {
			n := min(s.Scan.Threshold, iptMaxHitcount)
			add(iptScan, fmt.Sprintf("-m recent --name %s%s --set", iptRecent(v6), mask))
			add(iptScan, fmt.Sprintf("-m recent --name %s%s --rcheck --seconds 60 --hitcount %d -j %s", iptRecent(v6), mask, n, iptScanHit))
		}
		add(iptScanHit, fmt.Sprintf("-j SET --add-set %s src --exist --timeout %d", iptSet("scan", v6), iptSetTimeout(banSeconds(s, s.Scan.BanSeconds))))
		add(iptScanHit, "-j DROP")
	}
	return rules
}

// renderRestore 生成 iptables-restore --noflush 的输入：声明的链会被清空后写入新规则，其他链不受影响。
func renderRestore(chains []string, rules map[string][]string) string {
	var sb strings.Builder
	sb.WriteString("*mangle\n")
	for _, c := range chains {
		fmt.Fprintf(&sb, ":%s - [0:0]\n", c)
	}
	for _, c := range chains {
		for _, r := range rules[c] {
			sb.WriteString(r + "\n")
		}
	}
	sb.WriteString("COMMIT\n")
	return sb.String()
}

// iptPlan 是一次完整写入所需的内容。
type iptPlan struct {
	allow, deny []netip.Prefix
	open, trap  []portRange
}

// restoreChains 用 iptables-restore --noflush 重写各地址族中指定的链。
func (b *iptBackend) restoreChains(ctx context.Context, s Settings, plan iptPlan, chains []string) error {
	for _, f := range b.fams {
		rules := iptRules(s, f.v6, plan.allow, plan.deny, plan.open, plan.trap)
		if _, err := b.run(ctx, renderRestore(chains, rules), f.restore, "--noflush"); err != nil {
			return err
		}
	}
	return nil
}

// rebuild 重建全部链与集合并写入当前封禁，返回写入集合的单个来源及其解封时间，
// 以及按上限写入的来源在内核中的到期时间。
// 顺序：清空自有链（解除对集合的引用）→ 重建集合 → 写入规则 → 把跳转规则放到内建链第一位。
func (b *iptBackend) rebuild(ctx context.Context, s Settings, plan iptPlan, entries []banlist.Entry, now time.Time) (members, capped map[string]time.Time, err error) {
	for _, f := range b.fams {
		if _, err := b.run(ctx, renderRestore(iptChains, nil), f.restore, "--noflush"); err != nil {
			return nil, nil, err
		}
	}
	var sb strings.Builder
	for _, name := range b.setNames() {
		_, err := b.run(ctx, "", b.ipset, "destroy", name)
		switch {
		case err == nil, strings.Contains(err.Error(), "does not exist"):
		case strings.Contains(err.Error(), "in use"):
			// 集合仍被其他规则引用（例如手工加过引用它的规则）时无法销毁，清空后沿用；类型不同时下面的 create 会报错
			sb.WriteString("flush " + name + "\n")
		default:
			return nil, nil, err
		}
	}
	members, capped = map[string]time.Time{}, map[string]time.Time{}
	create := b.createSets()
	// 创建在前、清空在后：create 对已存在的同类型集合不报错（-exist）
	sets := create + sb.String()
	sb.Reset()
	sb.WriteString(sets)
	for _, e := range entries {
		stmt, exp, isCapped, ok := iptElement(e, now)
		if !ok {
			continue
		}
		p, _ := banlist.KeyPrefix(e.Key)
		if p.Addr().Is6() && !b.hasV6() {
			continue
		}
		members[e.Key] = e.Until
		if isCapped {
			capped[e.Key] = exp
		}
		sb.WriteString(stmt + "\n")
	}
	if _, err := b.run(ctx, sb.String(), b.ipset, "restore", "-exist"); err != nil {
		return nil, nil, err
	}
	if err := b.restoreChains(ctx, s, plan, iptChains); err != nil {
		return nil, nil, err
	}
	if _, err := b.ensureJumps(ctx); err != nil {
		return nil, nil, err
	}
	return members, capped, nil
}

// saveAll 读取各地址族 mangle 表的内容（带计数）。
func (b *iptBackend) saveAll(ctx context.Context) (map[bool]string, error) {
	out := map[bool]string{}
	for _, f := range b.fams {
		s, err := b.run(ctx, "", f.save, "-c", "-t", "mangle")
		if err != nil {
			return nil, err
		}
		out[f.v6] = s
	}
	return out, nil
}

// iptLine 是 iptables-save 输出中的一条规则：去掉了开头的 [包数:字节数]。
type iptLine struct {
	chain   string
	rule    string
	packets uint64
}

// parseSave 解析 iptables-save -c 的输出，返回声明过的链与按顺序排列的规则。
func parseSave(out string) (chains map[string]bool, lines []iptLine) {
	chains = map[string]bool{}
	for _, raw := range strings.Split(out, "\n") {
		l := strings.TrimSpace(raw)
		if name, ok := strings.CutPrefix(l, ":"); ok {
			chains[strings.Fields(name)[0]] = true
			continue
		}
		var pk uint64
		if strings.HasPrefix(l, "[") {
			cnt, rest, ok := strings.Cut(l[1:], "]")
			if !ok {
				continue
			}
			// 有的 iptables-save 会把计数补空格对齐（如 [      5:       292]）
			pkts, _, _ := strings.Cut(cnt, ":")
			pk, _ = strconv.ParseUint(strings.TrimSpace(pkts), 10, 64)
			l = strings.TrimSpace(rest)
		}
		rest, ok := strings.CutPrefix(l, "-A ")
		if !ok {
			continue
		}
		chain, rule, _ := strings.Cut(rest, " ")
		lines = append(lines, iptLine{chain: chain, rule: rule, packets: pk})
	}
	return chains, lines
}

// iptCounters 汇总各类规则的计数：丢弃的数据包按 ban / trap / scan / deny，触发次数按 trap / scan。
func iptCounters(lines []iptLine) (dropped, hits map[string]uint64) {
	dropped, hits = map[string]uint64{}, map[string]uint64{}
	for _, l := range lines {
		switch {
		case l.chain == iptPre && strings.HasPrefix(l.rule, "-m set --match-set "):
			name := strings.Fields(l.rule)[3]
			kind := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(name, "gately_"), "4"), "6")
			dropped[kind] += l.packets
		case l.chain == iptDeny:
			dropped["deny"] += l.packets
		case l.chain == iptTrap && strings.Contains(l.rule, "--add-set"):
			hits["trap"] += l.packets
		case l.chain == iptScanHit && strings.Contains(l.rule, "--add-set"):
			hits["scan"] += l.packets
		}
	}
	return dropped, hits
}

// counters 读取当前计数（IPv4 与 IPv6 相加）。
func (b *iptBackend) counters(ctx context.Context) (dropped, hits map[string]uint64, err error) {
	saved, err := b.saveAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	dropped, hits = map[string]uint64{}, map[string]uint64{}
	for _, out := range saved {
		_, lines := parseSave(out)
		d, h := iptCounters(lines)
		dropped, hits = addCounts(dropped, d), addCounts(hits, h)
	}
	return dropped, hits, nil
}

// errIPTMissing 表示自有链不存在（被其他程序清除，例如重启网络或防火墙时清空了全部规则）
var errIPTMissing = errors.New("iptables 规则已被清除")

// check 读取计数并确认自有链仍然存在；跳转规则被挤到其他规则之后时挪回第一位，fixed 返回挪动的跳转。
func (b *iptBackend) check(ctx context.Context) (dropped, hits map[string]uint64, fixed []string, err error) {
	saved, err := b.saveAll(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	dropped, hits = map[string]uint64{}, map[string]uint64{}
	for _, out := range saved {
		chains, lines := parseSave(out)
		for _, c := range iptChains {
			if !chains[c] {
				return nil, nil, nil, errIPTMissing
			}
		}
		d, h := iptCounters(lines)
		dropped, hits = addCounts(dropped, d), addCounts(hits, h)
	}
	fixed, err = b.fixJumps(ctx, saved)
	return dropped, hits, fixed, err
}

// ensureJumps 读取当前规则并确保跳转规则位于内建链第一位。
func (b *iptBackend) ensureJumps(ctx context.Context) ([]string, error) {
	saved, err := b.saveAll(ctx)
	if err != nil {
		return nil, err
	}
	return b.fixJumps(ctx, saved)
}

// fixJumps 让每个内建链的第一条规则是跳到自有链：不在第一位时删除全部同样的跳转，再插入到第一位。
func (b *iptBackend) fixJumps(ctx context.Context, saved map[bool]string) ([]string, error) {
	var fixed []string
	for _, f := range b.fams {
		_, lines := parseSave(saved[f.v6])
		for _, j := range iptJumps {
			builtin, own := j[0], j[1]
			jump := "-j " + own
			first, count := true, 0
			seen := false
			for _, l := range lines {
				if l.chain != builtin {
					continue
				}
				if l.rule == jump {
					count++
					if !seen {
						seen = true
					}
				} else if !seen {
					first = false
				}
			}
			if count == 1 && first {
				continue
			}
			for range count {
				if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-D", builtin, "-j", own); err != nil {
					return fixed, err
				}
			}
			if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-I", builtin, "1", "-j", own); err != nil {
				return fixed, err
			}
			if count > 0 {
				fixed = append(fixed, builtin)
			}
		}
	}
	return fixed, nil
}

// applyIPSet 执行一批 ipset 语句（add / del），已存在或不存在的元素不报错。
func (b *iptBackend) applyIPSet(ctx context.Context, stmts []string) error {
	if len(stmts) == 0 {
		return nil
	}
	_, err := b.run(ctx, strings.Join(stmts, "\n")+"\n", b.ipset, "restore", "-exist")
	return err
}

// writeDeny 重写各地址族的 GATELY_DENY。
func (b *iptBackend) writeDeny(ctx context.Context, deny []netip.Prefix) error {
	return b.restoreChains(ctx, Settings{}, iptPlan{deny: deny}, []string{iptDeny})
}

// kernelBans 读取诱饵端口与扫描检测集合中由内核加入的来源，键为集合名。
func (b *iptBackend) kernelBans(ctx context.Context) (map[string][]netip.Addr, error) {
	out := map[string][]netip.Addr{}
	for _, f := range b.fams {
		for _, kind := range []string{"trap", "scan"} {
			name := iptSet(kind, f.v6)
			text, err := b.run(ctx, "", b.ipset, "save", name)
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(text, "\n") {
				fs := strings.Fields(line)
				if len(fs) >= 3 && fs[0] == "add" && fs[1] == name {
					if a, err := netip.ParseAddr(fs[2]); err == nil {
						out[name] = append(out[name], a)
					}
				}
			}
		}
	}
	return out, nil
}

// remove 删除跳转、自有链与集合，停止或关闭同步时调用。
func (b *iptBackend) remove(ctx context.Context) error {
	var errs []error
	saved, err := b.saveAll(ctx)
	if err != nil {
		return err
	}
	for _, f := range b.fams {
		_, lines := parseSave(saved[f.v6])
		for _, j := range iptJumps {
			for _, l := range lines {
				if l.chain == j[0] && l.rule == "-j "+j[1] {
					if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-D", j[0], "-j", j[1]); err != nil {
						errs = append(errs, err)
					}
				}
			}
		}
		if _, err := b.run(ctx, renderRestore(iptChains, nil), f.restore, "--noflush"); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, c := range iptChains {
			if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-X", c); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, name := range b.setNames() {
		if _, err := b.run(ctx, "", b.ipset, "destroy", name); err != nil && !strings.Contains(err.Error(), "does not exist") {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// text 返回自有链与集合的当前内容，供界面查看。
func (b *iptBackend) text(ctx context.Context) (string, error) {
	var sb strings.Builder
	for _, f := range b.fams {
		out, err := b.run(ctx, "", f.save, "-t", "mangle")
		if err != nil {
			return "", err
		}
		label := "IPv4"
		if f.v6 {
			label = "IPv6"
		}
		fmt.Fprintf(&sb, "# %s: %s -t mangle\n", label, f.save)
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "GATELY_") {
				sb.WriteString(line + "\n")
			}
		}
		sb.WriteString("\n")
	}
	for _, name := range b.setNames() {
		out, err := b.run(ctx, "", b.ipset, "list", name)
		if err != nil {
			return "", err
		}
		sb.WriteString(strings.TrimSpace(out) + "\n\n")
	}
	return strings.TrimSpace(sb.String()) + "\n", nil
}

// ---- firewall 中使用 iptables 后端的部分 ----

// removeLocked 删除已写入的规则：nftables 删除 inet gately 表，iptables 删除自有链与集合。调用方持有 f.mu。
func (f *firewall) removeLocked(ctx context.Context) error {
	if f.ipt != nil {
		return f.ipt.remove(ctx)
	}
	_, err := f.run(ctx, "", "delete", "table", "inet", tableName)
	return err
}

// effectiveLocked 返回实际写入规则时使用的设置：IPv6 严格模式在就绪之前按阈值检测处理。调用方持有 f.mu。
func (f *firewall) effectiveLocked() Settings {
	ss := f.cfg
	ss.Scan.IPv6Strict = f.cfg.Scan.Enable && f.cfg.Scan.IPv6Strict && f.strictReady
	return ss
}

// allowLocked 返回诱饵端口与扫描检测不处理的白名单网段。调用方持有 f.mu。
func (f *firewall) allowLocked() []netip.Prefix {
	return mergePrefixes(allowPrefixes(f.cfg.ignoreSet(f.local).prefixes))
}

// iptPlanLocked 汇总一次写入需要的白名单、网段封禁与端口。调用方持有 f.mu。
func (f *firewall) iptPlanLocked(entries []banlist.Entry, open, trap []portRange) iptPlan {
	return iptPlan{allow: f.allowLocked(), deny: iptDenyPrefixes(entries), open: open, trap: trap}
}

// iptCurrentPortsLocked 返回最近一次写入的开放端口与诱饵端口。调用方持有 f.mu。
func (f *firewall) iptCurrentPortsLocked() (open, trap []portRange) {
	open, _ = parsePorts(f.open)
	trap, _ = parsePorts(f.trap)
	return open, trap
}

// warnClampLocked 在扫描阈值超过 recent 模块的上限时提示一次。调用方持有 f.mu。
func (f *firewall) warnClampLocked(s Settings) {
	n := 0
	if s.Scan.Enable && s.Scan.Threshold > iptMaxHitcount {
		n = s.Scan.Threshold
	}
	if n == f.iptClamp {
		return
	}
	f.iptClamp = n
	if n > 0 {
		f.log.Warnf("iptables 的 recent 模块每个来源最多记录 %d 次连接，扫描检测阈值 %d 按 %d 处理", iptMaxHitcount, n, iptMaxHitcount)
	}
}

// flushIPTLocked 把封禁表与 ipset 集合、GATELY_DENY 对账。调用方持有 f.mu。
func (f *firewall) flushIPTLocked(ctx context.Context, entries []banlist.Entry, now time.Time) {
	want := map[string]banlist.Entry{}
	for _, e := range entries {
		if _, _, _, ok := iptElement(e, now); !ok {
			continue
		}
		if p, _ := banlist.KeyPrefix(e.Key); p.Addr().Is6() && !f.ipt.hasV6() {
			continue
		}
		want[e.Key] = e
	}
	var dels, adds []string
	for key, until := range f.members {
		e, keep := want[key]
		exp, isCapped := f.capped[key]
		// 按上限写入的元素临近到期时重新写入，延续到真正的解封时间
		if keep && e.Until.Equal(until) && (!isCapped || exp.Sub(now) > iptRefresh) {
			continue
		}
		// 自然到期的元素已被内核删除，只有提前解封或需要更新超时时间时才删除
		if until.After(now) && (!isCapped || exp.After(now)) {
			dels = append(dels, iptDelStmt(key))
		}
		delete(f.members, key)
		delete(f.capped, key)
	}
	for key, e := range want {
		if _, ok := f.members[key]; ok {
			continue
		}
		stmt, exp, isCapped, _ := iptElement(e, now)
		adds = append(adds, stmt)
		f.members[key] = e.Until
		if isCapped {
			f.capped[key] = exp
		}
	}
	// 删除排在添加之前，更新超时时间时先删后加
	if err := f.ipt.applyIPSet(ctx, append(dels, adds...)); err != nil {
		f.log.Warnf("更新防火墙失败: %v", err)
	}
	if deny := iptDenyText(entries); deny != f.denyText {
		if err := f.ipt.writeDeny(ctx, iptDenyPrefixes(entries)); err != nil {
			f.log.Warnf("更新防火墙失败: %v", err)
			return
		}
		f.denyText = deny
	}
}

// pollIPTLocked 是 poll 的 iptables 版本：确认规则仍在、维护跳转位置、读取计数与内核直接封禁的地址、刷新端口。
// 调用方持有 f.mu，本函数返回前释放。
func (f *firewall) pollIPTLocked(ctx context.Context) {
	dropped, hits, fixed, err := f.ipt.check(ctx)
	if errors.Is(err, errIPTMissing) {
		// 自有链被其他程序删除（例如防火墙或网络重启时清空了全部规则），重新写入
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
	if err != nil {
		f.status.Error = err.Error()
		f.mu.Unlock()
		return
	}
	if len(fixed) > 0 {
		f.log.Infof("防火墙跳转规则被其他规则挤到了 %s 链后面，已移回第一位", strings.Join(fixed, "、"))
	}
	f.status.Error = ""
	f.status.Dropped, f.status.Hits = addCounts(f.baseDropped, dropped), addCounts(f.baseHits, hits)
	f.status.CheckedAt = time.Now()
	// 按上限写入的长期封禁临近到期时，标记由下一次对账重新写入
	for _, exp := range f.capped {
		if exp.Sub(f.status.CheckedAt) <= iptRefresh {
			f.dirty = true
			break
		}
	}
	portsOK := f.refreshPortsLocked(ctx)
	if portsOK && f.cfg.Scan.Enable && f.cfg.Scan.IPv6Strict && !f.strictReady && !f.strictAt.IsZero() && !time.Now().Before(f.strictAt) {
		f.strictReady = true
		f.applyLocked(ctx, f.cfg)
		if f.status.Active {
			f.log.Infof("IPv6 严格模式已启用：IPv6 来源连接开放端口以外的端口即被封禁")
		}
	}
	importFn, ipt := f.importFn, f.ipt
	inbox, kerr := ipt.kernelBans(ctx)
	f.mu.Unlock()
	if kerr != nil || len(inbox) == 0 {
		return
	}
	// 先写入封禁表（随后由 flush 加入封禁集合），再从内核的临时集合中删除，避免出现空档
	var stmts []string
	for name, addrs := range inbox {
		source := banlist.SourceTrap
		if strings.HasPrefix(name, "gately_scan") {
			source = banlist.SourceScan
		}
		if importFn != nil {
			importFn(source, addrs)
		}
		for _, a := range addrs {
			stmts = append(stmts, fmt.Sprintf("del %s %s", name, a))
		}
	}
	f.flush(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ipt == ipt {
		if err := ipt.applyIPSet(ctx, stmts); err != nil {
			f.log.Warnf("清理临时集合失败: %v", err)
		}
	}
}
