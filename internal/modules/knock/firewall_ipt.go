package knock

// iptables + ipset 后端：内核没有 nf_tables（如群晖 DSM）或系统没有 nft 命令时使用。
//
// 结构与 nftables 后端一致，写在 mangle 表 Gately 自己的链中（mangle 先于 filter 处理，
// 不受群晖防火墙、Docker 在 filter 表插入的放行规则影响）：
//
//	mangle PREROUTING → GATELY_KNOCK：按协议与端口把发往受保护端口的包跳到 GATELY_KNOCK_GATE；
//	GATELY_KNOCK_GATE：目标不是本机、回环接口、已建立的连接、TCP 中不是发起连接的包、免敲门网段依次 RETURN，
//	每个端口一个 ipset 集合（hash:ip，带超时）中的来源 RETURN，其余 DROP。
//
// 群晖内核只有 hash:ip 类型的集合，所以按端口分集合，不用 hash:ip,port。
// 入侵防护的 iptables 后端要求自己的跳转是 PREROUTING 第一条；本模块的跳转只要求排在 Gately 自己的跳转之后、
// 其他规则之前，两者不会互相挪动。

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/spa"
)

const (
	iptChain = "GATELY_KNOCK"
	iptGate  = "GATELY_KNOCK_GATE"
	// iptSetPrefix 为本模块 ipset 集合名的前缀
	iptSetPrefix = "gkn"
	// iptMultiport 为 multiport 一条规则最多容纳的端口数
	iptMultiport = 15
	// iptMaxTimeout 为 ipset 元素超时的上限（秒，约 24.8 天）；放行时长最长 1 天，不会触及，这里只做保护
	iptMaxTimeout = 2147483
)

type iptFam struct {
	v6                 bool
	bin, save, restore string
}

// iptBackend 通过 iptables / ip6tables 与 ipset 命令写入规则。
type iptBackend struct {
	run   func(ctx context.Context, stdin, bin string, args ...string) (string, error)
	fams  []iptFam
	ipset string
	ver   string
}

// findIPT 查找 iptables（优先 legacy 版本）、对应的 restore 命令与 ipset，缺少时返回 nil；ip6tables 缺失或不可用时只处理 IPv4。
func findIPT(ctx context.Context) *iptBackend {
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
		fam, ok := findIPTFam(name, v6)
		if !ok {
			if !v6 {
				return nil
			}
			continue
		}
		// mangle 表读不出来（内核禁用 IPv6、缺少 ip6table_mangle 等）时不使用该地址族
		if _, err := b.run(ctx, "", fam.bin, "-w", "-t", "mangle", "-S", "PREROUTING"); err != nil {
			if !v6 {
				return nil
			}
			continue
		}
		b.fams = append(b.fams, fam)
	}
	v, _ := b.run(ctx, "", b.fams[0].bin, "-V")
	sv, _ := b.run(ctx, "", ipset, "version")
	sv, _, _ = strings.Cut(strings.TrimSpace(sv), ",")
	b.ver = strings.TrimSpace(strings.TrimSpace(v) + ", " + sv)
	return b
}

func findIPTFam(name string, v6 bool) (iptFam, bool) {
	f := iptFam{v6: v6}
	if p := lookCmd(name + "-legacy"); p != "" {
		f.bin, f.restore = p, lookCmd(name+"-legacy-restore")
	} else if p := lookCmd(name); p != "" {
		f.bin, f.restore = p, lookCmd(name+"-restore")
	}
	return f, f.bin != "" && f.restore != ""
}

func (b *iptBackend) kind() string    { return BackendIPT }
func (b *iptBackend) version() string { return b.ver }

// iptSetName 返回端口在某个地址族的集合名，如 gkn4_tcp_22。
func iptSetName(v6 bool, a spa.Access) string {
	fam := "4"
	if v6 {
		fam = "6"
	}
	return fmt.Sprintf("%s%s_%s_%d", iptSetPrefix, fam, a.Proto, a.Port)
}

// iptRules 生成一个地址族的 iptables-restore 内容。
func iptRules(plan fwPlan, v6 bool) string {
	var sb strings.Builder
	sb.WriteString("*mangle\n")
	fmt.Fprintf(&sb, ":%s - [0:0]\n:%s - [0:0]\n", iptChain, iptGate)
	for _, proto := range []string{"tcp", "udp"} {
		var ports []string
		for _, a := range plan.Ports {
			if a.Proto == proto {
				ports = append(ports, strconv.Itoa(a.Port))
			}
		}
		for i := 0; i < len(ports); i += iptMultiport {
			chunk := ports[i:min(i+iptMultiport, len(ports))]
			fmt.Fprintf(&sb, "-A %s -p %s -m multiport --dports %s -j %s\n", iptChain, proto, strings.Join(chunk, ","), iptGate)
		}
	}
	fmt.Fprintf(&sb, "-A %s -m addrtype ! --dst-type LOCAL -j RETURN\n", iptGate)
	fmt.Fprintf(&sb, "-A %s -i lo -j RETURN\n", iptGate)
	fmt.Fprintf(&sb, "-A %s -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN\n", iptGate)
	// TCP 只拦截发起连接的 SYN，原因见 nftables 后端
	fmt.Fprintf(&sb, "-A %s -p tcp ! --syn -j RETURN\n", iptGate)
	for _, p := range plan.Exempt {
		if p.Addr().Is6() != v6 {
			continue
		}
		fmt.Fprintf(&sb, "-A %s -s %s -j RETURN\n", iptGate, p)
	}
	for _, a := range plan.Ports {
		fmt.Fprintf(&sb, "-A %s -p %s --dport %d -m set --match-set %s src -j RETURN\n", iptGate, a.Proto, a.Port, iptSetName(v6, a))
		fmt.Fprintf(&sb, "-A %s -p %s --dport %d -j DROP\n", iptGate, a.Proto, a.Port)
	}
	sb.WriteString("COMMIT\n")
	return sb.String()
}

// ipsetAddStmts 生成 ipset restore 的写入语句；-exist 让已有元素刷新超时。
func (b *iptBackend) ipsetAddStmts(grants []fwGrant, now time.Time) []string {
	var out []string
	for _, g := range grants {
		sec := min(remainSeconds(g.Until, now), iptMaxTimeout)
		if sec == 0 || !b.hasFam(g.Addr.Is6()) {
			continue
		}
		out = append(out, fmt.Sprintf("add %s %s timeout %d", iptSetName(g.Addr.Is6(), g.Access), g.Addr, sec))
	}
	return out
}

func (b *iptBackend) hasFam(v6 bool) bool {
	for _, f := range b.fams {
		if f.v6 == v6 {
			return true
		}
	}
	return false
}

func (b *iptBackend) ipsetRestore(ctx context.Context, stmts []string) error {
	if len(stmts) == 0 {
		return nil
	}
	_, err := b.run(ctx, strings.Join(stmts, "\n")+"\n", b.ipset, "restore", "-exist")
	return err
}

func (b *iptBackend) apply(ctx context.Context, plan fwPlan, now time.Time) error {
	// 先建集合并写入放行（规则引用的集合必须存在），再写链，最后把跳转放到正确位置
	var create []string
	for _, f := range b.fams {
		family := "inet"
		if f.v6 {
			family = "inet6"
		}
		for _, a := range plan.Ports {
			name := iptSetName(f.v6, a)
			create = append(create, fmt.Sprintf("create %s hash:ip family %s timeout 0", name, family), "flush "+name)
		}
	}
	// 放行先于跳转写入，跳转生效时已授权的地址不会被丢弃
	create = append(create, b.ipsetAddStmts(plan.Grants, now)...)
	if err := b.ipsetRestore(ctx, create); err != nil {
		return err
	}
	// 先写完全部地址族的链，最后再插跳转：某个地址族写入失败时，其他地址族的拦截还没有生效
	for _, f := range b.fams {
		if _, err := b.run(ctx, iptRules(plan, f.v6), f.restore, "-w", "--noflush"); err != nil {
			return err
		}
	}
	for _, f := range b.fams {
		if err := b.fixJump(ctx, f); err != nil {
			return err
		}
	}
	b.destroyUnused(ctx, plan)
	return nil
}

// fixJump 保证 PREROUTING 中有且只有一条跳到 GATELY_KNOCK 的规则，且它之前只有 Gately 自己的跳转。
func (b *iptBackend) fixJump(ctx context.Context, f iptFam) error {
	out, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-S", "PREROUTING")
	if err != nil {
		return err
	}
	if jumpOK(out) {
		return nil
	}
	// 删除全部已有的跳转（位置不对或重复），再插入到 Gately 其他跳转（入侵防护的 GATELY_PRE 要求排第一）之后、
	// 其他规则之前，两个模块都不会再挪动对方
	for range 8 {
		if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-D", "PREROUTING", "-j", iptChain); err != nil {
			break
		}
	}
	if out, err = b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-S", "PREROUTING"); err != nil {
		return err
	}
	pos := strconv.Itoa(leadingGatelyJumps(out) + 1)
	_, err = b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-I", "PREROUTING", pos, "-j", iptChain)
	return err
}

// leadingGatelyJumps 返回 PREROUTING 开头连续的 Gately 其他跳转规则数（不含本模块的跳转）。
func leadingGatelyJumps(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "-A PREROUTING") {
			continue
		}
		if !strings.HasPrefix(l, "-A PREROUTING -j GATELY_") || l == "-A PREROUTING -j "+iptChain {
			break
		}
		n++
	}
	return n
}

// jumpOK 判断 iptables -S PREROUTING 的输出中跳转规则是否恰好一条，且排在其他非 Gately 规则之前。
func jumpOK(out string) bool {
	count, ok := 0, true
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "-A PREROUTING") {
			continue
		}
		if l == "-A PREROUTING -j "+iptChain {
			count++
			continue
		}
		if count == 0 && !strings.HasPrefix(l, "-A PREROUTING -j GATELY_") {
			ok = false
		}
	}
	return count == 1 && ok
}

// destroyUnused 删除不再使用的本模块集合（端口被移出规则后），失败时忽略。
func (b *iptBackend) destroyUnused(ctx context.Context, plan fwPlan) {
	out, err := b.run(ctx, "", b.ipset, "list", "-n")
	if err != nil {
		return
	}
	keep := map[string]bool{}
	for _, f := range b.fams {
		for _, a := range plan.Ports {
			keep[iptSetName(f.v6, a)] = true
		}
	}
	for _, name := range strings.Fields(out) {
		if strings.HasPrefix(name, iptSetPrefix+"4_") || strings.HasPrefix(name, iptSetPrefix+"6_") {
			if !keep[name] {
				_, _ = b.run(ctx, "", b.ipset, "destroy", name)
			}
		}
	}
}

func (b *iptBackend) add(ctx context.Context, grants []fwGrant, now time.Time) error {
	return b.ipsetRestore(ctx, b.ipsetAddStmts(grants, now))
}

func (b *iptBackend) del(ctx context.Context, grants []fwGrant) error {
	var stmts []string
	for _, g := range grants {
		if b.hasFam(g.Addr.Is6()) {
			stmts = append(stmts, fmt.Sprintf("del %s %s", iptSetName(g.Addr.Is6(), g.Access), g.Addr))
		}
	}
	return b.ipsetRestore(ctx, stmts)
}

// check 确认各地址族的链与跳转仍然存在（跳转被挪到其他规则之后时修正位置），并汇总计数。
func (b *iptBackend) check(ctx context.Context) (bool, fwCounts, error) {
	var c fwCounts
	for _, f := range b.fams {
		out, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-S", "PREROUTING")
		if err != nil {
			return false, c, err
		}
		if !strings.Contains(out, "-j "+iptChain) {
			return false, c, nil
		}
		if !jumpOK(out) {
			if err := b.fixJump(ctx, f); err != nil {
				return false, c, err
			}
		}
		list, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-L", iptGate, "-v", "-x", "-n")
		if err != nil {
			return false, c, nil
		}
		fc := parseIPTCounts(list)
		c.dropped += fc.dropped
		c.allowed += fc.allowed
	}
	return true, c, nil
}

// parseIPTCounts 解析 iptables -L -v -x -n 的输出：DROP 规则计为拦截，匹配集合的 RETURN 计为放行。
func parseIPTCounts(out string) fwCounts {
	var c fwCounts
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		n, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		switch {
		case f[2] == "DROP":
			c.dropped += n
		case f[2] == "RETURN" && strings.Contains(l, "match-set "+iptSetPrefix):
			c.allowed += n
		}
	}
	return c
}

func (b *iptBackend) remove(ctx context.Context) error {
	var firstErr error
	for _, f := range b.fams {
		for range 8 {
			if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-D", "PREROUTING", "-j", iptChain); err != nil {
				break
			}
		}
		for _, ch := range []string{iptChain, iptGate} {
			_, _ = b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-F", ch)
		}
		for _, ch := range []string{iptChain, iptGate} {
			if _, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-X", ch); err != nil && firstErr == nil && !strings.Contains(err.Error(), "No chain") && !strings.Contains(err.Error(), "does not exist") {
				firstErr = err
			}
		}
	}
	b.destroyUnused(ctx, fwPlan{})
	return firstErr
}

func (b *iptBackend) text(ctx context.Context) (string, error) {
	var sb strings.Builder
	for _, f := range b.fams {
		for _, ch := range []string{"PREROUTING", iptChain, iptGate} {
			out, err := b.run(ctx, "", f.bin, "-w", "-t", "mangle", "-S", ch)
			if err != nil {
				return "", err
			}
			if ch == "PREROUTING" {
				// 只显示本模块的跳转
				var keep []string
				for _, l := range strings.Split(out, "\n") {
					if strings.Contains(l, iptChain) {
						keep = append(keep, l)
					}
				}
				out = strings.Join(keep, "\n") + "\n"
			}
			sb.WriteString(out)
		}
	}
	out, err := b.run(ctx, "", b.ipset, "list", "-n")
	if err == nil {
		for _, name := range strings.Fields(out) {
			if strings.HasPrefix(name, iptSetPrefix+"4_") || strings.HasPrefix(name, iptSetPrefix+"6_") {
				if s, err := b.run(ctx, "", b.ipset, "list", name); err == nil {
					sb.WriteString(s)
				}
			}
		}
	}
	return sb.String(), nil
}
