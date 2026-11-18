package perf

import (
	"io"
	"log"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/sysstat"
)

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的配置文件。
func initFromRaw(t *testing.T, raw map[string]any) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	m := New(nil)
	m.src.Host = func() sysstat.Snapshot { return sysstat.Snapshot{} }
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// 配置文件缺告警级别与最小请求数时加载后补上默认值，否则告警事件没有级别（只接收警告以上的通知规则收不到），
// 窗口类指标没有最小请求数门槛、低流量下也会误报。
func TestInitFillsMissingRuleDefaults(t *testing.T) {
	m := initFromRaw(t, map[string]any{"rules": []any{
		map[string]any{"key": "cpu", "name": "CPU", "enable": true, "metric": MetricHostCPU, "op": OpGT, "threshold": 80, "duration": 1},
		map[string]any{"key": "p95", "name": "P95", "enable": true, "metric": MetricSiteP95, "op": OpGT, "threshold": 500, "duration": 1},
		map[string]any{"key": "qps", "name": "QPS", "enable": true, "metric": MetricSiteQPS, "op": OpGT, "threshold": 100, "duration": 1},
	}})
	cpu, p95, qps := m.cfg.Rules[0], m.cfg.Rules[1], m.cfg.Rules[2]
	if cpu.Level != levelWarn || p95.Level != levelWarn || qps.Level != levelWarn {
		t.Fatalf("告警级别未补默认值: %q %q %q", cpu.Level, p95.Level, qps.Level)
	}
	if p95.MinRequests != DefaultMinRequests {
		t.Fatalf("窗口类指标的最小请求数未补默认值: %d", p95.MinRequests)
	}
	// 主机指标与每秒请求数不使用最小请求数
	if cpu.MinRequests != 0 || qps.MinRequests != 0 {
		t.Fatalf("不使用最小请求数的指标不应补值: cpu=%d qps=%d", cpu.MinRequests, qps.MinRequests)
	}

	// 运行时：条件持续满 1 分钟（13 次采样）后触发的告警事件带 warn 级别
	evs, _ := run(newEngine(), t0, 13, func(int) hostSample { return hostAt(95) }, nil, []Rule{cpu})
	if len(evs) == 0 {
		t.Fatal("CPU 超过阈值应触发告警")
	}
	if evs[0].Level != events.LevelWarn {
		t.Fatalf("告警事件级别应为 warn: %q", evs[0].Level)
	}
}

// 已有的值保持不变。
func TestInitKeepsExistingRuleValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{"rules": []any{
		map[string]any{"key": "p95", "name": "P95", "enable": true, "metric": MetricSiteP95, "op": OpGT, "threshold": 500, "duration": 1,
			"level": "error", "minRequests": 5},
	}})
	if r := m.cfg.Rules[0]; r.Level != levelError || r.MinRequests != 5 {
		t.Fatalf("已有值被改动: level=%q minRequests=%d", r.Level, r.MinRequests)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameRuleDefaults(t *testing.T) {
	r := Rule{Name: "P95", Metric: MetricSiteP95, Op: OpGT, Threshold: 500, Duration: 1}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Level != levelWarn || r.MinRequests != DefaultMinRequests {
		t.Fatalf("保存时默认值不符: level=%q minRequests=%d", r.Level, r.MinRequests)
	}
	cpu := Rule{Name: "CPU", Metric: MetricHostCPU, Op: OpGT, Threshold: 80, Duration: 1, MinRequests: 9}
	if err := cpu.Validate(); err != nil {
		t.Fatal(err)
	}
	if cpu.MinRequests != 0 {
		t.Fatalf("主机指标保存时应清空最小请求数: %d", cpu.MinRequests)
	}
	if err := (&Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: 80, Duration: 1, Level: "fatal"}).Validate(); err == nil {
		t.Fatal("未知告警级别应被拒绝")
	}
}
