package fwopen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func newBackend() backend {
	if runtime.GOOS != "linux" {
		return unsupported{}
	}
	return &cmdBackend{}
}

type unsupported struct{}

func (unsupported) targets(context.Context) ([]target, map[string][]existing, error) {
	return nil, nil, errUnsupported{}
}
func (unsupported) add(context.Context, target, string, Port) error { return errUnsupported{} }
func (unsupported) del(context.Context, target, existing) error     { return errUnsupported{} }

// cmdBackend 通过 nft / iptables / ip6tables 命令读写防火墙。
type cmdBackend struct {
	nft, iptables, ip6tables string
	// iptNFT / ip6tNFT 表示 iptables / ip6tables 是 nf_tables 版本（iptables-nft），其规则也会出现在 nft 中
	iptNFT, ip6tNFT bool
	// chain 不为 nil 时使用专用链模式（内核没有 nf_tables），见 chain.go
	chain *chainState
}

var (
	nftPaths       = []string{"/usr/sbin/nft", "/sbin/nft", "/usr/bin/nft"}
	iptablesPaths  = []string{"/usr/sbin/iptables", "/sbin/iptables", "/usr/bin/iptables"}
	ip6tablesPaths = []string{"/usr/sbin/ip6tables", "/sbin/ip6tables", "/usr/bin/ip6tables"}
	// safeName 限制写入命令的表名与链名，防止异常名称破坏 nft 语句
	safeName = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)
)

func lookPath(name string, candidates []string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, p := range candidates {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return ""
}

func run(ctx context.Context, stdin string, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), classify(msg)
	}
	return out.String(), nil
}

// classify 把权限错误转换为带部署提示的说明。
func classify(msg string) error {
	low := strings.ToLower(msg)
	if strings.Contains(low, "operation not permitted") || strings.Contains(low, "permission denied") {
		return errors.New("没有修改防火墙的权限（需要 root；Docker 部署需要 host 网络与 cap_add: NET_ADMIN）")
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return errors.New(msg)
}

// kernelNoNFT 判断 nft 基础查询的错误是否表示内核没有 nf_tables：
// 未编入 nf_tables 时 netlink 返回 EINVAL，连 nfnetlink 都没有时返回 EPROTONOSUPPORT 等。
func kernelNoNFT(msg string) bool {
	low := strings.ToLower(msg)
	for _, s := range []string{"invalid argument", "protocol not supported", "address family not supported", "operation not supported"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

func (b *cmdBackend) detect(ctx context.Context) {
	b.nft = lookPath("nft", nftPaths)
	b.iptables = lookPath("iptables", iptablesPaths)
	b.ip6tables = lookPath("ip6tables", ip6tablesPaths)
	b.iptNFT, b.ip6tNFT = false, false
	if b.iptables != "" {
		if v, err := run(ctx, "", b.iptables, "-V"); err == nil {
			b.iptNFT = strings.Contains(v, "nf_tables")
		}
	}
	if b.ip6tables != "" {
		if v, err := run(ctx, "", b.ip6tables, "-V"); err == nil {
			b.ip6tNFT = strings.Contains(v, "nf_tables")
		}
	}
}

// bin 返回 iptables 类链对应的命令。
func (b *cmdBackend) bin(t target) string {
	if t.Kind == "ip6tables" {
		return b.ip6tables
	}
	return b.iptables
}

func (b *cmdBackend) targets(ctx context.Context) ([]target, map[string][]existing, error) {
	if b.chain != nil {
		return b.chainTargets(ctx)
	}
	b.detect(ctx)
	if b.nft == "" && b.iptables == "" {
		return nil, nil, errors.New("没有找到 nft 或 iptables 命令，无法放行防火墙")
	}
	var list []target
	have := map[string][]existing{}
	// hasIPFilter / hasIP6Filter 表示 nft 中存在 iptables-nft / ip6tables-nft 的 filter 表 INPUT 链
	hasIPFilter, hasIP6Filter := false, false
	var chainsOut string
	if b.nft != "" {
		out, err := run(ctx, "", b.nft, "-j", "list", "chains")
		chainsOut = out
		switch {
		case err != nil && kernelNoNFT(err.Error()):
			// 内核没有 nf_tables（如群晖 DSM）时 nft 命令存在也无法使用，改用 iptables（legacy）的专用链放行
			if fams := findChainFams(); len(fams) > 0 {
				b.chain = &chainState{fams: fams, known: map[string][]string{}}
				return b.chainTargets(ctx)
			}
			return nil, nil, fmt.Errorf("当前系统内核不支持 nftables，且没有 iptables（legacy）命令，无法放行防火墙: %w", err)
		case err != nil:
			return nil, nil, err
		}
	}
	if b.nft != "" {
		chains, err := parseNFTChains([]byte(chainsOut))
		if err != nil {
			return nil, nil, err
		}
		for _, c := range chains {
			if c.Family == "ip" && c.Table == "filter" && c.Chain == "INPUT" && b.iptNFT {
				// iptables-nft 管理的 INPUT 链用 iptables 命令写入，iptables -L 等工具才能正常显示；
				// 手写的同名表中其他名字的链（如 input）仍按 nft 处理
				hasIPFilter = true
				continue
			}
			if c.Family == "ip6" && c.Table == "filter" && c.Chain == "INPUT" && b.ip6tNFT {
				hasIP6Filter = true
				continue
			}
			rout, err := run(ctx, "", b.nft, "-j", "-a", "list", "chain", c.Family, c.Table, c.Chain)
			if err != nil {
				return nil, nil, err
			}
			busy, rules, err := parseNFTRules([]byte(rout))
			if err != nil {
				return nil, nil, err
			}
			c.Busy = busy
			list = append(list, c)
			have[c.String()] = rules
		}
	}
	// iptables-legacy 的规则不在 nft 中，总是检查；iptables-nft 只在 filter 表已存在时使用，避免凭空创建表
	if b.iptables != "" && (!b.iptNFT || hasIPFilter || b.nft == "") {
		out, err := run(ctx, "", b.iptables, "-w", "-S", "INPUT")
		if err != nil {
			return nil, nil, err
		}
		t, rules := parseIPTables(out)
		list = append(list, t)
		have[t.String()] = rules
	}
	if b.ip6tables != "" && (!b.ip6tNFT || hasIP6Filter || b.nft == "") {
		out, err := run(ctx, "", b.ip6tables, "-w", "-S", "INPUT")
		if err != nil {
			// 内核未启用 IPv6 时 ip6tables 会报错，不影响 IPv4 的放行
			if b.iptables != "" || b.nft != "" {
				return list, have, nil
			}
			return nil, nil, err
		}
		t, rules := parseIPTables(out)
		t.Kind, t.Family = "ip6tables", "ip6"
		list = append(list, t)
		have[t.String()] = rules
	}
	return list, have, nil
}

// nftMatch 生成 nft 规则的匹配部分：地址族、目标地址与端口（区间）。
func nftMatch(t target, p Port) string {
	var parts []string
	switch {
	case p.Addr != "" && p.Family == "6":
		parts = append(parts, "ip6 daddr "+p.Addr)
	case p.Addr != "":
		parts = append(parts, "ip daddr "+p.Addr)
	case t.Family == "inet" && p.Family == "4":
		parts = append(parts, "meta nfproto ipv4")
	case t.Family == "inet" && p.Family == "6":
		parts = append(parts, "meta nfproto ipv6")
	}
	parts = append(parts, p.Proto+" dport "+p.portText())
	return strings.Join(parts, " ")
}

// iptArgs 生成 iptables / ip6tables 规则的匹配参数。
func iptArgs(p Port) []string {
	port := strconv.Itoa(p.Port)
	if p.last() > p.Port {
		port += ":" + strconv.Itoa(p.last())
	}
	args := []string{"-p", p.Proto, "--dport", port}
	if p.Addr != "" {
		args = append(args, "-d", p.Addr)
	}
	return args
}

func (b *cmdBackend) add(ctx context.Context, t target, c string, p Port) error {
	if b.chain != nil {
		return b.chainAdd(ctx, t, c, p)
	}
	if t.Kind == "iptables" || t.Kind == "ip6tables" {
		args := append([]string{"-w", "-I", "INPUT", "1"}, iptArgs(p)...)
		args = append(args, "-m", "comment", "--comment", c, "-j", "ACCEPT")
		_, err := run(ctx, "", b.bin(t), args...)
		return err
	}
	stmt := fmt.Sprintf("insert rule %s %s %s %s counter accept comment %q\n", t.Family, t.Table, t.Chain, nftMatch(t, p), c)
	_, err := run(ctx, stmt, b.nft, "-f", "-")
	return err
}

func (b *cmdBackend) del(ctx context.Context, t target, e existing) error {
	if b.chain != nil {
		return b.chainDel(ctx, t, e)
	}
	if t.Kind == "iptables" || t.Kind == "ip6tables" {
		args := append([]string{"-w", "-D", "INPUT"}, strings.Fields(e.Ref)...)
		_, err := run(ctx, "", b.bin(t), args...)
		return err
	}
	stmt := fmt.Sprintf("delete rule %s %s %s handle %s\n", t.Family, t.Table, t.Chain, e.Ref)
	_, err := run(ctx, stmt, b.nft, "-f", "-")
	return err
}

// parseNFTChains 从 `nft -j list chains` 中取出 ip / ip6 / inet 的 filter 类型 input 基础链，跳过入侵防护使用的 inet gately 表。
func parseNFTChains(raw []byte) ([]target, error) {
	var doc struct {
		Nftables []struct {
			Chain *struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Name   string `json:"name"`
				Type   string `json:"type"`
				Hook   string `json:"hook"`
				Policy string `json:"policy"`
			} `json:"chain"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("解析 nft 输出失败: %w", err)
	}
	var out []target
	for _, it := range doc.Nftables {
		c := it.Chain
		if c == nil || c.Hook != "input" || c.Type != "filter" {
			continue
		}
		if c.Family != "ip" && c.Family != "ip6" && c.Family != "inet" {
			continue
		}
		if c.Family == "inet" && c.Table == "gately" {
			continue
		}
		if !safeName.MatchString(c.Table) || !safeName.MatchString(c.Name) {
			continue
		}
		out = append(out, target{Kind: "nft", Family: c.Family, Table: c.Table, Chain: c.Name, Policy: strings.ToLower(c.Policy)})
	}
	return out, nil
}

// parseNFTRules 从 `nft -j -a list chain` 中取出 Gately 规则，并判断链中是否还有其他规则。
func parseNFTRules(raw []byte) (bool, []existing, error) {
	var doc struct {
		Nftables []struct {
			Rule *struct {
				Handle  int    `json:"handle"`
				Comment string `json:"comment"`
			} `json:"rule"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, nil, fmt.Errorf("解析 nft 输出失败: %w", err)
	}
	busy := false
	var rules []existing
	for _, it := range doc.Nftables {
		if it.Rule == nil {
			continue
		}
		if strings.HasPrefix(it.Rule.Comment, commentPrefix) {
			rules = append(rules, existing{Comment: it.Rule.Comment, Ref: strconv.Itoa(it.Rule.Handle)})
			continue
		}
		busy = true
	}
	return busy, rules, nil
}

// parseIPTables 解析 `iptables -S INPUT` 的输出。
func parseIPTables(out string) (target, []existing) {
	t := target{Kind: "iptables", Family: "ip", Table: "filter", Chain: "INPUT"}
	var rules []existing
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) < 2 || f[1] != "INPUT" {
			continue
		}
		switch f[0] {
		case "-P":
			if len(f) >= 3 {
				t.Policy = strings.ToLower(f[2])
			}
		case "-A":
			c := ""
			for i := 2; i+1 < len(f); i++ {
				if f[i] == "--comment" {
					c = strings.Trim(f[i+1], `"`)
				}
			}
			if strings.HasPrefix(c, commentPrefix) {
				// 删除时按原规则参数匹配，注释可能带引号，去掉后与写入时一致
				ref := make([]string, 0, len(f)-2)
				for _, s := range f[2:] {
					ref = append(ref, strings.Trim(s, `"`))
				}
				rules = append(rules, existing{Comment: c, Ref: strings.Join(ref, " ")})
				continue
			}
			t.Busy = true
		}
	}
	return t, rules
}
