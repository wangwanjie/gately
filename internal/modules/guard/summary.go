package guard

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/module"
)

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	fw := m.fw.snapshot()
	items := []module.SummaryItem{{Label: "封禁中", Value: fmt.Sprint(banlist.Default.Len())}}
	switch {
	case fw.Active:
		var dropped uint64
		for _, n := range fw.Dropped {
			dropped += n
		}
		items = append(items, module.SummaryItem{Label: "防火墙", Value: "已生效", Level: "ok"},
			module.SummaryItem{Label: "已丢弃", Value: fmt.Sprint(dropped)})
	case fw.Enabled:
		items = append(items, module.SummaryItem{Label: "防火墙", Value: "未生效", Level: "warn"})
	}
	m.mu.Lock()
	failed := 0
	for _, r := range m.jails {
		if r.jail.Enable && r.errorText() != "" {
			failed++
		}
	}
	m.mu.Unlock()
	if failed > 0 {
		items = append(items, module.SummaryItem{Label: "规则异常", Value: fmt.Sprint(failed), Level: "warn"})
	}
	return items
}

// Metrics 实现 module.Metricer。
func (m *Module) Metrics() []module.Sample {
	out := []module.Sample{{Name: "gately_guard_banned", Help: "IP addresses or networks currently banned", Type: module.MetricGauge, Value: float64(banlist.Default.Len())}}
	fw := m.fw.snapshot()
	active := 0.0
	if fw.Active {
		active = 1
	}
	out = append(out, module.Sample{Name: "gately_guard_firewall_active", Help: "Whether the nftables rules are in place (1) or not (0)", Type: module.MetricGauge, Value: active})
	rules := make([]string, 0, len(fw.Dropped))
	for k := range fw.Dropped {
		rules = append(rules, k)
	}
	sort.Strings(rules)
	for _, k := range rules {
		out = append(out, module.Sample{Name: "gately_guard_dropped_packets_total", Help: "Packets dropped by the firewall rules", Type: module.MetricCounter,
			Labels: map[string]string{"rule": k}, Value: float64(fw.Dropped[k])})
	}
	m.mu.Lock()
	keys := make([]string, 0, len(m.jails))
	for k := range m.jails {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := m.jails[k]
		out = append(out, module.Sample{Name: "gately_guard_jail_matches_total", Help: "Log lines matched by the jail", Type: module.MetricCounter,
			Labels: map[string]string{"jail": r.jail.Name}, Value: float64(r.stats.matches.Load())})
	}
	for _, k := range keys {
		r := m.jails[k]
		out = append(out, module.Sample{Name: "gately_guard_jail_bans_total", Help: "Bans triggered by the jail", Type: module.MetricCounter,
			Labels: map[string]string{"jail": r.jail.Name}, Value: float64(r.stats.bans.Load())})
	}
	m.mu.Unlock()
	return out
}

// SelfCheck 实现 module.SelfChecker：报告防火墙同步失败、规则读取日志出错与被排除的诱饵端口。
func (m *Module) SelfCheck(_ context.Context, _ module.CheckEnv) []module.CheckResult {
	fw := m.fw.snapshot()
	s := m.cur()
	var out []module.CheckResult
	switch {
	case fw.Active:
		out = append(out, module.CheckResult{ID: "guard.firewall", Status: module.CheckOK, Params: map[string]string{"bans": strconv.Itoa(banlist.Default.Len())}})
	case fw.Enabled && fw.Reason == ReasonOS:
		// 非 Linux 平台本来就没有 nftables，只作提示
		out = append(out, module.CheckResult{ID: "guard.os", Status: module.CheckInfo})
	case fw.Enabled && fw.Reason == ReasonUnsupported:
		// 内核没有 nf_tables（如群晖 DSM）无法通过配置解决，与非 Linux 平台一样只作提示
		out = append(out, module.CheckResult{ID: "guard.unsupported", Status: module.CheckInfo})
	case fw.Enabled && fw.Reason != "":
		out = append(out, module.CheckResult{ID: "guard." + fw.Reason, Status: module.CheckWarn, Params: map[string]string{"error": fw.Error}})
	}
	if s.Trap.Enable && fw.Active && len(fw.TrapSkipped) > 0 {
		ports := make([]string, 0, len(fw.TrapSkipped))
		for _, p := range fw.TrapSkipped {
			ports = append(ports, strconv.Itoa(p))
		}
		out = append(out, module.CheckResult{ID: "guard.trapSkipped", Status: module.CheckInfo, Params: map[string]string{"ports": strings.Join(ports, ", ")}})
	}
	if s.Scan.Enable && !fw.Active {
		out = append(out, module.CheckResult{ID: "guard.scanUnavailable", Status: module.CheckWarn})
	}
	if s.Trap.Enable && !fw.Active {
		out = append(out, module.CheckResult{ID: "guard.trapUserspace", Status: module.CheckInfo})
	}
	m.mu.Lock()
	for _, j := range m.cfg.Jails {
		r := m.jails[j.Key]
		if !j.Enable || r == nil {
			continue
		}
		if e := r.errorText(); e != "" {
			out = append(out, module.CheckResult{ID: "guard.jailError", Status: module.CheckWarn, Params: map[string]string{"jail": j.Name, "error": e}})
		}
	}
	m.mu.Unlock()
	return out
}
