package fwopen

// 专用链模式：内核没有 nf_tables（如群晖 DSM）时使用 iptables（legacy）。
//
// 群晖内核没有 comment 模块，无法用注释标记 Gately 的规则；群晖自带的 iptables 按链名查询（-S INPUT）也不可用。
// 因此改为：每个实例在 filter 表中使用一条自己的链（GTLY_OPEN_<实例>），INPUT 的第一条规则跳到这条链，
// 放行规则写在链中；规则属于哪个 owner 记在内存里，进程重启后链中不认识的规则在下次对账时删除并按登记补回。
// 读取一律用 iptables-save（整张 filter 表），写入用 -N / -A / -D / -I，不依赖按链查询。
// 群晖防火墙开启时会把自己的跳转插到 INPUT 最前面，对账时把 Gately 的跳转挪回第一位。

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// chainFam 是专用链模式下一个地址族使用的命令。
type chainFam struct {
	v6        bool
	bin, save string
}

// chainState 保存专用链模式的状态。
type chainState struct {
	fams []chainFam
	// known 为链中规则（按 ruleKey）对应的注释，由 add 按写入顺序记录。
	// 同一端口可能有多个登记者（例如 STUN 的 UPnP 内部端口正是 Web 服务的监听端口），链中就有多条相同的规则，
	// 每条规则对应一个注释，否则对账会把其中一条当成重复删除，再给另一个登记者补回，每轮来回增删
	known map[string][]string
}

// chainPrefix 为专用链名的前缀，后接实例标识
const chainPrefix = "GTLY_OPEN_"

// chainName 返回本实例的专用链名（iptables 链名最长 28 个字符）。
func chainName() string {
	instMu.RLock()
	defer instMu.RUnlock()
	return chainPrefix + instance
}

// findChainFams 查找 iptables（legacy）及其 save 命令；iptables 本身是 nf_tables 版本时不能使用。
func findChainFams() []chainFam {
	var out []chainFam
	for _, v6 := range []bool{false, true} {
		name := "iptables"
		if v6 {
			name = "ip6tables"
		}
		f := chainFam{v6: v6}
		if p := lookPath(name+"-legacy", legacyCandidates(name+"-legacy")); p != "" {
			f.bin, f.save = p, lookPath(name+"-legacy-save", legacyCandidates(name+"-legacy-save"))
		} else if p := lookPath(name, legacyCandidates(name)); p != "" {
			if v, err := exec.Command(p, "-V").Output(); err == nil && !strings.Contains(string(v), "nf_tables") {
				f.bin, f.save = p, lookPath(name+"-save", legacyCandidates(name+"-save"))
			}
		}
		if f.bin != "" && f.save != "" {
			out = append(out, f)
		}
	}
	return out
}

// chainDirs 为 PATH 之外查找 iptables（legacy）命令的目录，可在测试中替换
var chainDirs = []string{"/usr/sbin", "/sbin", "/usr/bin"}

// legacyCandidates 返回 PATH 之外查找命令的位置。
func legacyCandidates(name string) []string {
	out := make([]string, 0, len(chainDirs))
	for _, d := range chainDirs {
		out = append(out, d+"/"+name)
	}
	return out
}

// kind 返回地址族对应的 target.Kind。
func (f chainFam) kind() string {
	if f.v6 {
		return "ip6tables"
	}
	return "iptables"
}

// famOf 返回 target 对应的地址族命令。
func (c *chainState) famOf(t target) (chainFam, bool) {
	for _, f := range c.fams {
		if f.kind() == t.Kind {
			return f, true
		}
	}
	return chainFam{}, false
}

// savedTable 是 iptables-save -t filter 中与专用链有关的内容。
type savedTable struct {
	policy  string
	input   []string // INPUT 的规则参数（不含 -A INPUT），按顺序
	own     []string // 专用链的规则参数
	hasOwn  bool
	jumpPos int // 跳到专用链的规则在 INPUT 中的位置（从 0 开始），没有为 -1
	jumps   int
}

// parseSavedFilter 解析 iptables-save -t filter 的输出。
func parseSavedFilter(out, own string) savedTable {
	st := savedTable{jumpPos: -1}
	jump := "-j " + own
	for _, raw := range strings.Split(out, "\n") {
		l := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(l, ":INPUT "):
			if f := strings.Fields(l); len(f) >= 2 {
				st.policy = strings.ToLower(f[1])
			}
		case strings.HasPrefix(l, ":"+own+" "):
			st.hasOwn = true
		case strings.HasPrefix(l, "-A INPUT "):
			args := strings.TrimPrefix(l, "-A INPUT ")
			if args == jump {
				if st.jumpPos < 0 {
					st.jumpPos = len(st.input)
				}
				st.jumps++
			}
			st.input = append(st.input, args)
		case strings.HasPrefix(l, "-A "+own+" "):
			st.own = append(st.own, strings.TrimPrefix(l, "-A "+own+" "))
		}
	}
	return st
}

// jumpOK 判断跳到专用链的规则是否只有一条且位置正确：排在它前面的只能是其他 Gately 实例的专用链跳转。
// 同一台主机上的多个实例都要求排在第一位时，允许彼此在前，否则每次对账都会互相挪动。
func (st savedTable) jumpOK() bool {
	if st.jumps != 1 || st.jumpPos < 0 {
		return false
	}
	for _, r := range st.input[:st.jumpPos] {
		fs := strings.Fields(r)
		if len(fs) != 2 || fs[0] != "-j" || !strings.HasPrefix(fs[1], chainPrefix) {
			return false
		}
	}
	return true
}

// busy 判断 INPUT 中是否有跳到专用链以外的规则（可能丢弃流量）。
func (st savedTable) busy(own string) bool {
	for _, r := range st.input {
		if r != "-j "+own {
			return true
		}
	}
	return false
}

// ruleKey 从规则参数中取出协议、目标端口与目标地址，作为与登记端口比对的键。
// iptables-save 会把参数规范化（补上 -m tcp、地址加 /32），这里统一去掉这些差异。
func ruleKey(v6 bool, args string) string {
	f := strings.Fields(args)
	proto, port, addr := "", "", ""
	for i := 0; i+1 < len(f); i++ {
		switch f[i] {
		case "-p":
			proto = f[i+1]
		case "--dport":
			port = f[i+1]
		case "-d":
			addr = strings.TrimSuffix(strings.TrimSuffix(f[i+1], "/32"), "/128")
		}
	}
	fam := "4"
	if v6 {
		fam = "6"
	}
	return fam + "|" + proto + "|" + port + "|" + addr
}

// portKey 返回端口对应的 ruleKey。
func portKey(v6 bool, p Port) string {
	args := strings.Join(iptArgs(p), " ")
	return ruleKey(v6, args)
}

// chainTargets 是专用链模式的 targets：每个地址族一条 INPUT 链。
func (b *cmdBackend) chainTargets(ctx context.Context) ([]target, map[string][]existing, error) {
	c := b.chain
	own := chainName()
	var list []target
	have := map[string][]existing{}
	for i, f := range c.fams {
		out, err := run(ctx, "", f.save, "-t", "filter")
		if err != nil {
			// 内核未启用 IPv6 时 ip6tables 会报错，不影响 IPv4 的放行
			if f.v6 && i > 0 {
				continue
			}
			return nil, nil, err
		}
		st := parseSavedFilter(out, own)
		t := target{Kind: f.kind(), Family: "ip", Table: "filter", Chain: "INPUT", Policy: st.policy, Busy: st.busy(own)}
		if f.v6 {
			t.Family = "ip6"
		}
		// 链中有规则而跳转不在第一位（被群晖防火墙等挤到后面）时挪回第一位
		if len(st.own) > 0 && !st.jumpOK() {
			if err := b.fixChainJump(ctx, f, own, st.jumps); err != nil {
				return nil, nil, err
			}
		}
		used := map[string]int{}
		for _, args := range st.own {
			key := ruleKey(f.v6, args)
			cm := ""
			// 相同的规则按出现顺序依次对应登记的注释
			if n := used[key]; n < len(c.known[key]) {
				cm = c.known[key][n]
			}
			used[key]++
			if cm == "" {
				// 不认识的规则（进程重启前写入的）用不会匹配任何登记的注释，对账时删除并按登记补回
				cm = ownPrefix() + "?:" + ruleKey(f.v6, args)
			}
			have[t.String()] = append(have[t.String()], existing{Comment: cm, Ref: args})
		}
		list = append(list, t)
	}
	return list, have, nil
}

// fixChainJump 删除 INPUT 中全部跳到专用链的规则，再插入到第一位。
func (b *cmdBackend) fixChainJump(ctx context.Context, f chainFam, own string, jumps int) error {
	for range jumps {
		if _, err := run(ctx, "", f.bin, "-w", "-D", "INPUT", "-j", own); err != nil {
			return err
		}
	}
	_, err := run(ctx, "", f.bin, "-w", "-I", "INPUT", "1", "-j", own)
	return err
}

// chainAdd 在专用链中追加放行规则，链或跳转不存在时先创建。
func (b *cmdBackend) chainAdd(ctx context.Context, t target, cm string, p Port) error {
	f, ok := b.chain.famOf(t)
	if !ok {
		return fmt.Errorf("没有 %s 命令", t.Kind)
	}
	own := chainName()
	out, err := run(ctx, "", f.save, "-t", "filter")
	if err != nil {
		return err
	}
	st := parseSavedFilter(out, own)
	if !st.hasOwn {
		if _, err := run(ctx, "", f.bin, "-w", "-N", own); err != nil {
			return err
		}
	}
	if !st.jumpOK() {
		if err := b.fixChainJump(ctx, f, own, st.jumps); err != nil {
			return err
		}
	}
	args := append([]string{"-w", "-A", own}, iptArgs(p)...)
	if _, err := run(ctx, "", f.bin, append(args, "-j", "ACCEPT")...); err != nil {
		return err
	}
	key := portKey(f.v6, p)
	b.chain.known[key] = append(b.chain.known[key], cm)
	return nil
}

// chainDel 从专用链删除一条规则。
func (b *cmdBackend) chainDel(ctx context.Context, t target, e existing) error {
	f, ok := b.chain.famOf(t)
	if !ok {
		return fmt.Errorf("没有 %s 命令", t.Kind)
	}
	args := append([]string{"-w", "-D", chainName()}, strings.Fields(e.Ref)...)
	if _, err := run(ctx, "", f.bin, args...); err != nil {
		return err
	}
	// 只去掉这条规则对应的注释，同一端口其他登记者的规则保留
	key := ruleKey(f.v6, e.Ref)
	list := b.chain.known[key]
	if i := slices.Index(list, e.Comment); i >= 0 {
		list = slices.Delete(slices.Clone(list), i, i+1)
	} else if len(list) > 0 {
		list = list[1:]
	}
	if len(list) == 0 {
		delete(b.chain.known, key)
	} else {
		b.chain.known[key] = list
	}
	return nil
}
