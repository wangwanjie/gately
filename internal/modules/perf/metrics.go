package perf

import (
	"fmt"
	"sort"

	"github.com/wangwanjie/gately/internal/module"
)

// Metrics 实现 module.Metricer：导出最近一次采样的主机与进程指标，以及各规则当前触发的告警数。
func (m *Module) Metrics() []module.Sample {
	var out []module.Sample
	add := func(name, help string, v float64) {
		if v == unknown {
			return
		}
		out = append(out, module.Sample{Name: name, Help: help, Type: module.MetricGauge, Value: v})
	}
	if s, ok := m.eng.hist.latest(); ok {
		add("gately_host_cpu_usage_percent", "Host CPU usage in percent", s.CPU)
		add("gately_host_memory_used_percent", "Host memory used in percent", s.Mem)
		add("gately_host_disk_used_percent", "Used percent of the disk holding the config directory", s.Disk)
		add("gately_host_load1", "Host 1 minute load average", s.Load1)
		add("gately_process_cpu_usage_percent", "Gately process CPU usage in percent of all cores", s.ProcCPU)
		if s.ProcRSSMB != unknown {
			add("gately_process_resident_memory_bytes", "Gately process resident memory in bytes", s.ProcRSSMB*(1<<20))
		}
		add("gately_process_open_fds", "Gately process open file descriptors", s.ProcFD)
	}
	m.mu.Lock()
	names := make([]string, 0, len(m.cfg.Rules))
	for _, r := range m.cfg.Rules {
		if r.Enable {
			names = append(names, r.Name)
		}
	}
	m.mu.Unlock()
	firing := m.eng.firingCounts()
	sort.Strings(names)
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, module.Sample{Name: "gately_perf_alerts_firing", Help: "Alert instances currently firing for each enabled performance alert rule",
			Type: module.MetricGauge, Labels: map[string]string{"rule": n}, Value: float64(firing[n])})
	}
	return out
}

// Summary 实现 module.Summarizer：总览页展示告警规则数与触发中的告警数。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	enabled := 0
	for _, r := range m.cfg.Rules {
		if r.Enable {
			enabled++
		}
	}
	m.mu.Unlock()
	nFiring := 0
	for _, a := range m.eng.alerts() {
		if a.State == "firing" {
			nFiring++
		}
	}
	level := "ok"
	if nFiring > 0 {
		level = "error"
	}
	return []module.SummaryItem{
		{Label: "告警规则", Value: fmt.Sprint(enabled), Level: "info"},
		{Label: "触发中的告警", Value: fmt.Sprint(nFiring), Level: level},
	}
}
