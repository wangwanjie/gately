package knock

import (
	"fmt"

	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/module"
)

// Summary 实现 module.Summarizer：启用的规则数、当前授权数，以及防火墙拦截与监听是否正常。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	enabled := 0
	for _, r := range m.cfg.Rules {
		if r.Enable {
			enabled++
		}
	}
	listenErr := m.listenErr
	m.mu.Unlock()
	items := []module.SummaryItem{
		{Label: "启用规则", Value: fmt.Sprint(enabled)},
		{Label: "当前授权", Value: fmt.Sprint(len(knockgate.Default.List()))},
	}
	if listenErr != "" {
		items = append(items, module.SummaryItem{Label: "敲门端口", Value: "监听失败", Level: "error"})
	}
	fw := m.fw.snapshot()
	switch {
	case fw.Active:
		items = append(items, module.SummaryItem{Label: "防火墙拦截", Value: "已生效", Level: "ok"})
	case fw.Needed:
		items = append(items, module.SummaryItem{Label: "防火墙拦截", Value: "未生效", Level: "warn"})
	}
	return items
}

// Metrics 实现 module.Metricer。
func (m *Module) Metrics() []module.Sample {
	m.stateMu.Lock()
	c := m.counters
	m.stateMu.Unlock()
	fw := m.fw.snapshot()
	active := 0.0
	if fw.Active {
		active = 1
	}
	return []module.Sample{
		{Name: "gately_knock_packets_total", Help: "Knock packets by result", Type: module.MetricCounter, Labels: map[string]string{"result": "accepted"}, Value: float64(c.Accepted)},
		{Name: "gately_knock_packets_total", Help: "Knock packets by result", Type: module.MetricCounter, Labels: map[string]string{"result": "rejected"}, Value: float64(c.Rejected)},
		{Name: "gately_knock_packets_total", Help: "Knock packets by result", Type: module.MetricCounter, Labels: map[string]string{"result": "ignored"}, Value: float64(c.Ignored)},
		{Name: "gately_knock_grants", Help: "Source addresses currently holding a knock grant", Type: module.MetricGauge, Value: float64(len(knockgate.Default.List()))},
		{Name: "gately_knock_firewall_active", Help: "Whether the firewall blocking rules are in place (1) or not (0)", Type: module.MetricGauge, Value: active},
		{Name: "gately_knock_firewall_dropped_total", Help: "New connections to protected ports dropped by the firewall", Type: module.MetricCounter, Value: float64(fw.Dropped)},
	}
}
