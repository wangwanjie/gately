package knock

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// nftTable 是敲门模块在 nftables 中使用的表（inet 族，同时处理 IPv4 与 IPv6），与入侵防护的 gately 表相互独立。
const nftTable = "gately_knock"

// nftBackend 通过 nft 命令写入规则。结构：
//
//	prerouting 链（优先级 -160：连接跟踪之后、DNAT 之前）只把发往受保护端口的包跳到 gate 链；
//	gate 链依次放行：目标不是本机（转发给其他设备的流量）、回环接口、已建立的连接、TCP 中不是发起连接的包、
//	免敲门网段、敲门放行集合中的 地址 . 协议 . 端口，其余丢弃。
//
// 在 prerouting 判断可以同时保护本机服务与 DNAT 到容器的端口（匹配的是 DNAT 之前的原始端口）；
// 丢弃写在 Gately 自己的表中，与系统防火墙（fw4、firewalld 等）各表的放行互不影响：任一基础链丢弃即被丢弃。
type nftBackend struct {
	bin string
	ver string
	run func(ctx context.Context, stdin, bin string, args ...string) (string, error)
}

func (b *nftBackend) kind() string    { return BackendNFT }
func (b *nftBackend) version() string { return b.ver }

// nftScript 生成重建整张表的脚本：先确保表存在再删除，避免首次写入时删除不存在的表导致整批失败。
func nftScript(plan fwPlan, now time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "table inet %s {}\ndelete table inet %s\n", nftTable, nftTable)
	fmt.Fprintf(&sb, "table inet %s {\n", nftTable)
	ports := make([]string, 0, len(plan.Ports))
	for _, a := range plan.Ports {
		ports = append(ports, fmt.Sprintf("%s . %d", a.Proto, a.Port))
	}
	fmt.Fprintf(&sb, "\tset ports {\n\t\ttype inet_proto . inet_service\n\t\telements = { %s }\n\t}\n", strings.Join(ports, ", "))
	sb.WriteString("\tset allow4 {\n\t\ttype ipv4_addr . inet_proto . inet_service\n\t\tflags timeout\n\t}\n")
	sb.WriteString("\tset allow6 {\n\t\ttype ipv6_addr . inet_proto . inet_service\n\t\tflags timeout\n\t}\n")
	var ex4, ex6 []string
	for _, p := range plan.Exempt {
		if p.Addr().Is4() {
			ex4 = append(ex4, p.String())
		} else {
			ex6 = append(ex6, p.String())
		}
	}
	writeIntervalSet(&sb, "exempt4", "ipv4_addr", ex4)
	writeIntervalSet(&sb, "exempt6", "ipv6_addr", ex6)
	sb.WriteString("\tchain prerouting {\n\t\ttype filter hook prerouting priority -160; policy accept;\n")
	sb.WriteString("\t\tmeta l4proto . th dport @ports jump gate\n\t}\n")
	sb.WriteString("\tchain gate {\n")
	sb.WriteString("\t\tfib daddr type != local return\n")
	sb.WriteString("\t\tiifname \"lo\" return\n")
	sb.WriteString("\t\tct state established,related return\n")
	// TCP 只拦截发起连接的 SYN：启用前已建立、但还没有被连接跟踪记录的连接（宿主机此前没有任何防火墙规则使用连接跟踪时），
	// 后续包会被当作新连接，只看连接状态会把它们丢弃，正在使用的 SSH 会话因此卡死；不带 SYN 的包无法建立新连接
	sb.WriteString("\t\tmeta l4proto tcp tcp flags & (syn | ack) != syn return\n")
	sb.WriteString("\t\tip saddr @exempt4 return\n")
	sb.WriteString("\t\tip6 saddr @exempt6 return\n")
	sb.WriteString("\t\tip saddr . meta l4proto . th dport @allow4 counter return comment \"allowed\"\n")
	sb.WriteString("\t\tip6 saddr . meta l4proto . th dport @allow6 counter return comment \"allowed\"\n")
	sb.WriteString("\t\tcounter drop comment \"dropped\"\n")
	sb.WriteString("\t}\n}\n")
	sb.WriteString(nftAddStmts(plan.Grants, now))
	return sb.String()
}

// writeIntervalSet 写入网段集合；auto-merge 合并重叠的网段（例如白名单与内网网段重叠），否则 nft 报错。
func writeIntervalSet(sb *strings.Builder, name, typ string, elems []string) {
	fmt.Fprintf(sb, "\tset %s {\n\t\ttype %s\n\t\tflags interval\n\t\tauto-merge\n", name, typ)
	if len(elems) > 0 {
		fmt.Fprintf(sb, "\t\telements = { %s }\n", strings.Join(elems, ", "))
	}
	sb.WriteString("\t}\n")
}

// nftElem 返回放行在集合中的元素与集合名。
func nftElem(g fwGrant) (set, elem string) {
	set = "allow4"
	if g.Addr.Is6() {
		set = "allow6"
	}
	return set, fmt.Sprintf("%s . %s . %d", g.Addr, g.Access.Proto, g.Access.Port)
}

// remainSeconds 返回剩余秒数（向上取整），已到期返回 0。
func remainSeconds(until, now time.Time) int {
	d := until.Sub(now)
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}

// nftAddStmts 生成写入放行的语句。已存在的元素不会因再次 add 刷新超时，所以先 add（保证存在）、
// 再 delete、最后带超时 add，整批在一个事务中完成。
func nftAddStmts(grants []fwGrant, now time.Time) string {
	var sb strings.Builder
	for _, g := range grants {
		sec := remainSeconds(g.Until, now)
		if sec == 0 || !g.Addr.IsValid() {
			continue
		}
		set, elem := nftElem(g)
		fmt.Fprintf(&sb, "add element inet %s %s { %s }\n", nftTable, set, elem)
		fmt.Fprintf(&sb, "delete element inet %s %s { %s }\n", nftTable, set, elem)
		fmt.Fprintf(&sb, "add element inet %s %s { %s timeout %ds }\n", nftTable, set, elem, sec)
	}
	return sb.String()
}

func (b *nftBackend) apply(ctx context.Context, plan fwPlan, now time.Time) error {
	_, err := b.run(ctx, nftScript(plan, now), b.bin, "-f", "-")
	return err
}

func (b *nftBackend) add(ctx context.Context, grants []fwGrant, now time.Time) error {
	s := nftAddStmts(grants, now)
	if s == "" {
		return nil
	}
	_, err := b.run(ctx, s, b.bin, "-f", "-")
	return err
}

// del 删除放行：先 add 再 delete，元素已经到期消失时也不会让整批失败。
func (b *nftBackend) del(ctx context.Context, grants []fwGrant) error {
	var sb strings.Builder
	for _, g := range grants {
		if !g.Addr.IsValid() {
			continue
		}
		set, elem := nftElem(g)
		fmt.Fprintf(&sb, "add element inet %s %s { %s }\n", nftTable, set, elem)
		fmt.Fprintf(&sb, "delete element inet %s %s { %s }\n", nftTable, set, elem)
	}
	if sb.Len() == 0 {
		return nil
	}
	_, err := b.run(ctx, sb.String(), b.bin, "-f", "-")
	return err
}

// nftCounterRe 匹配带注释的计数规则，如 `counter packets 3 bytes 180 drop comment "dropped"`。
var nftCounterRe = regexp.MustCompile(`counter packets (\d+) bytes \d+ .*comment "(allowed|dropped)"`)

func parseNFTCounts(out string) fwCounts {
	var c fwCounts
	for _, m := range nftCounterRe.FindAllStringSubmatch(out, -1) {
		n, _ := strconv.ParseUint(m[1], 10, 64)
		if m[2] == "dropped" {
			c.dropped += n
		} else {
			c.allowed += n
		}
	}
	return c
}

// check 读取整张表：表、prerouting 链的跳转或 gate 链被外部删除时都视为失效，需要重建。
func (b *nftBackend) check(ctx context.Context) (bool, fwCounts, error) {
	out, err := b.run(ctx, "", b.bin, "list", "table", "inet", nftTable)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") || strings.Contains(err.Error(), "does not exist") {
			return false, fwCounts{}, nil
		}
		return false, fwCounts{}, err
	}
	if !strings.Contains(out, "hook prerouting") || !strings.Contains(out, "jump gate") || !strings.Contains(out, `comment "dropped"`) {
		return false, fwCounts{}, nil
	}
	return true, parseNFTCounts(out), nil
}

func (b *nftBackend) remove(ctx context.Context) error {
	_, err := b.run(ctx, fmt.Sprintf("table inet %s {}\ndelete table inet %s\n", nftTable, nftTable), b.bin, "-f", "-")
	return err
}

func (b *nftBackend) text(ctx context.Context) (string, error) {
	return b.run(ctx, "", b.bin, "list", "table", "inet", nftTable)
}
