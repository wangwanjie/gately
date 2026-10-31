package protoroute

import (
	"fmt"
	"sort"

	"github.com/wangwanjie/gately/internal/module"
)

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	enabled, conns := 0, int64(0)
	for _, r := range m.cfg.Rules {
		if r.Enable {
			enabled++
		}
	}
	for _, r := range m.runners {
		conns += r.stats.ActiveConns.Load()
	}
	level := "ok"
	if len(m.errors) > 0 {
		level = "error"
	}
	return []module.SummaryItem{
		{Label: "运行中", Value: fmt.Sprintf("%d / %d", len(m.runners), enabled), Level: level},
		{Label: "活动连接", Value: fmt.Sprint(conns)},
	}
}

// Metrics 实现 module.Metricer：每条运行中规则的连接数、流量与各路由的命中数。
func (m *Module) Metrics() []module.Sample {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.runners))
	for k := range m.runners {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []module.Sample
	add := func(name, help, typ string, labels map[string]string, v int64) {
		out = append(out, module.Sample{Name: name, Help: help, Type: typ, Labels: labels, Value: float64(v)})
	}
	for _, k := range keys {
		r := m.runners[k]
		add("gately_protoroute_active_connections", "Connections currently open on the protocol routing rule", module.MetricGauge,
			map[string]string{"rule": r.current().rule.Name}, r.stats.ActiveConns.Load())
	}
	for _, k := range keys {
		r := m.runners[k]
		add("gately_protoroute_connections_total", "Connections accepted by the protocol routing rule", module.MetricCounter,
			map[string]string{"rule": r.current().rule.Name}, r.stats.TotalConns.Load())
	}
	for _, k := range keys {
		r := m.runners[k]
		name := r.current().rule.Name
		add("gately_protoroute_bytes_total", "Bytes relayed by the protocol routing rule", module.MetricCounter, map[string]string{"rule": name, "direction": "in"}, r.stats.BytesIn.Load())
		add("gately_protoroute_bytes_total", "Bytes relayed by the protocol routing rule", module.MetricCounter, map[string]string{"rule": name, "direction": "out"}, r.stats.BytesOut.Load())
	}
	// 各路由与默认目标的累计命中数，route 标签为路由名称，未命名时为序号，默认目标为 default
	for _, k := range keys {
		r := m.runners[k]
		cur := r.current()
		snap := r.snapshot()
		for i, n := range snap.RouteConns {
			label := cur.rule.Routes[i].Name
			if label == "" {
				label = fmt.Sprint(i + 1)
			}
			add("gately_protoroute_route_connections_total", "Connections sent to each route of the protocol routing rule", module.MetricCounter,
				map[string]string{"rule": cur.rule.Name, "route": label}, n)
		}
		add("gately_protoroute_route_connections_total", "Connections sent to each route of the protocol routing rule", module.MetricCounter,
			map[string]string{"rule": cur.rule.Name, "route": "default"}, snap.DefaultConns)
	}
	return out
}
