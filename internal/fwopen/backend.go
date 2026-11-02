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

// cmdBackend 通过 nft / iptables 命令读写防火墙。
type cmdBackend struct {
	nft, iptables string
	// iptNFT 表示 iptables 是 nf_tables 版本（iptables-nft），其规则也会出现在 nft 中
	iptNFT bool
}

var (
	nftPaths      = []string{"/usr/sbin/nft", "/sbin/nft", "/usr/bin/nft"}
	iptablesPaths = []string{"/usr/sbin/iptables", "/sbin/iptables", "/usr/bin/iptables"}
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

func (b *cmdBackend) detect(ctx context.Context) {
	b.nft = lookPath("nft", nftPaths)
	b.iptables = lookPath("iptables", iptablesPaths)
	b.iptNFT = false
	if b.iptables != "" {
		if v, err := run(ctx, "", b.iptables, "-V"); err == nil {
			b.iptNFT = strings.Contains(v, "nf_tables")
		}
	}
}

func (b *cmdBackend) targets(ctx context.Context) ([]target, map[string][]existing, error) {
	b.detect(ctx)
	if b.nft == "" && b.iptables == "" {
		return nil, nil, errors.New("没有找到 nft 或 iptables 命令，无法放行防火墙")
	}
	var list []target
	have := map[string][]existing{}
	// hasIPFilter 表示 nft 中存在 iptables-nft 的 ip filter 表 INPUT 链
	hasIPFilter := false
	if b.nft != "" {
		out, err := run(ctx, "", b.nft, "-j", "list", "chains")
		if err != nil {
			return nil, nil, err
		}
		chains, err := parseNFTChains([]byte(out))
		if err != nil {
			return nil, nil, err
		}
		for _, c := range chains {
			if c.Family == "ip" && c.Table == "filter" && b.iptNFT {
				// iptables-nft 管理的表用 iptables 命令写入，iptables -L 等工具才能正常显示
				hasIPFilter = true
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
	// iptables-legacy 的规则不在 nft 中，总是检查；iptables-nft 只在 ip filter 表已存在时使用，避免凭空创建表
	if b.iptables != "" && (!b.iptNFT || hasIPFilter || b.nft == "") {
		out, err := run(ctx, "", b.iptables, "-w", "-S", "INPUT")
		if err != nil {
			return nil, nil, err
		}
		t, rules := parseIPTables(out)
		list = append(list, t)
		have[t.String()] = rules
	}
	return list, have, nil
}

func (b *cmdBackend) add(ctx context.Context, t target, c string, p Port) error {
	port := strconv.Itoa(p.Port)
	if t.Kind == "iptables" {
		_, err := run(ctx, "", b.iptables, "-w", "-I", "INPUT", "1", "-p", p.Proto, "--dport", port,
			"-m", "comment", "--comment", c, "-j", "ACCEPT")
		return err
	}
	match := p.Proto + " dport " + port
	if t.Family == "inet" {
		// 穿透通道只用 IPv4
		match = "meta nfproto ipv4 " + match
	}
	stmt := fmt.Sprintf("insert rule %s %s %s %s counter accept comment %q\n", t.Family, t.Table, t.Chain, match, c)
	_, err := run(ctx, stmt, b.nft, "-f", "-")
	return err
}

func (b *cmdBackend) del(ctx context.Context, t target, e existing) error {
	if t.Kind == "iptables" {
		args := append([]string{"-w", "-D", "INPUT"}, strings.Fields(e.Ref)...)
		_, err := run(ctx, "", b.iptables, args...)
		return err
	}
	stmt := fmt.Sprintf("delete rule %s %s %s handle %s\n", t.Family, t.Table, t.Chain, e.Ref)
	_, err := run(ctx, stmt, b.nft, "-f", "-")
	return err
}

// parseNFTChains 从 `nft -j list chains` 中取出 IPv4 可用的 filter 类型 input 基础链，跳过入侵防护使用的 inet gately 表。
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
		if c.Family != "ip" && c.Family != "inet" {
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
