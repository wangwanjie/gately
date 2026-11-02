// Package perf 提供性能监控：后台定时采样主机、Gately 进程与各站点的指标，保留 24 小时分钟级历史，
// 并按告警规则（指标 + 阈值 + 持续时长）在条件持续满足时发布性能告警事件，恢复时发布恢复事件，
// 事件经通知中心送达各渠道。
//
// 历史只保存在内存中，重启后清空；告警规则保存在配置里，运行中的计时与触发状态重启后重新开始。
package perf

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
)

// 指标对象范围。
const (
	ScopeHost     = "host"
	ScopeProcess  = "process"
	ScopeSite     = "site"
	ScopeUpstream = "upstream"
)

// 指标标识。
const (
	MetricHostCPU    = "host.cpu"
	MetricHostMem    = "host.mem"
	MetricHostDisk   = "host.disk"
	MetricProcCPU    = "proc.cpu"
	MetricProcRSS    = "proc.rss"
	MetricSiteActive = "site.active"
	MetricSiteQPS    = "site.qps"
	MetricSite5xx    = "site.err5xx"
	MetricSiteP95    = "site.p95"
	MetricUpstreamC  = "upstream.conns"
)

// 指标单位。
const (
	UnitPercent = "%"
	UnitMB      = "MB"
	UnitQPS     = "req/s"
	UnitMs      = "ms"
	UnitCount   = ""
)

// MetricDef 描述一个可用于告警的指标。
type MetricDef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Scope string `json:"scope"`
	Unit  string `json:"unit"`
	// Window 表示该指标按最近约 60 秒的窗口计算，需要窗口内有足够的请求才有结果（见规则的 MinRequests）
	Window bool `json:"window"`
	// Max 为阈值上限，防止填入无意义的巨大数值
	Max float64 `json:"max"`
}

// Metrics 是全部可用指标，顺序即界面展示顺序。
var Metrics = []MetricDef{
	{MetricHostCPU, "主机 CPU 使用率", ScopeHost, UnitPercent, false, 100},
	{MetricHostMem, "主机内存使用率", ScopeHost, UnitPercent, false, 100},
	{MetricHostDisk, "主机磁盘使用率（配置目录所在分区）", ScopeHost, UnitPercent, false, 100},
	{MetricProcCPU, "Gately 进程 CPU 使用率", ScopeProcess, UnitPercent, false, 100},
	{MetricProcRSS, "Gately 进程常驻内存", ScopeProcess, UnitMB, false, 1048576},
	{MetricSiteActive, "站点进行中的请求数", ScopeSite, UnitCount, false, 10000000},
	{MetricSiteQPS, "站点每秒请求数", ScopeSite, UnitQPS, true, 10000000},
	{MetricSite5xx, "站点 5xx 响应比例", ScopeSite, UnitPercent, true, 100},
	{MetricSiteP95, "站点首字节延迟 P95", ScopeSite, UnitMs, true, 3600000},
	{MetricUpstreamC, "后端节点当前连接数", ScopeUpstream, UnitCount, false, 10000000},
}

// metricByID 按标识查找指标。
func metricByID(id string) (MetricDef, bool) {
	for _, m := range Metrics {
		if m.ID == id {
			return m, true
		}
	}
	return MetricDef{}, false
}

// 规则字段的取值范围。
const (
	// MaxRules 为告警规则数量上限
	MaxRules = 200
	// MaxRuleSites 为一条规则可指定的站点数量上限
	MaxRuleSites = 200
	// MaxDurationMinutes 为持续时长上限（24 小时）
	MaxDurationMinutes = 1440
	// DefaultMinRequests 为窗口类指标的最小请求数默认值
	DefaultMinRequests = 20
	// MaxMinRequests 为最小请求数上限
	MaxMinRequests = 1000000
	// maxRuleNameLen 为规则名称的最大字符数
	maxRuleNameLen = 64
)

// 比较符。
const (
	OpGT = ">"
	OpGE = ">="
	OpLT = "<"
	OpLE = "<="
)

// 告警级别沿用事件级别。
const (
	levelWarn  = "warn"
	levelError = "error"
)

// Rule 是一条告警规则：指标满足 “比较符 阈值” 的状态持续 Duration 分钟后触发告警，条件解除后发恢复通知。
type Rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Metric string `json:"metric"`
	Op     string `json:"op"`
	// Threshold 的单位由指标决定
	Threshold float64 `json:"threshold"`
	// Duration 为条件需要持续满足的分钟数，中途回落则计时清零
	Duration int `json:"duration"`
	// Sites 为站点与后端类指标生效的站点（站点标识，见 /api/perf/sites），留空为全部站点；对主机与进程指标无意义，保存时清空
	Sites []string `json:"sites"`
	// MinRequests 为窗口类指标（QPS 之外的 5xx 比例、延迟）的最小请求数：窗口内请求不足时不判定（视为条件不满足），避免低流量时误报
	MinRequests int `json:"minRequests"`
	// Level 为告警级别 warn / error，留空为 warn
	Level string `json:"level"`
	// Remark 为备注
	Remark string `json:"remark"`
}

// Validate 校验并规范化规则。
func (r *Rule) Validate() error {
	if err := crud.NormalizeGroup(&r.Group); err != nil {
		return err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("规则名称不能为空")
	}
	if len([]rune(r.Name)) > maxRuleNameLen {
		return fmt.Errorf("规则名称不能超过 %d 个字符", maxRuleNameLen)
	}
	r.Remark = strings.TrimSpace(r.Remark)
	if len([]rune(r.Remark)) > 200 {
		return errors.New("备注不能超过 200 个字符")
	}
	def, ok := metricByID(r.Metric)
	if !ok {
		return fmt.Errorf("不支持的指标 %q", r.Metric)
	}
	switch r.Op {
	case OpGT, OpGE, OpLT, OpLE:
	default:
		return fmt.Errorf("不支持的比较符 %q", r.Op)
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) || r.Threshold < 0 || r.Threshold > def.Max {
		return fmt.Errorf("阈值必须在 0 到 %s 之间", strconv.FormatFloat(def.Max, 'f', -1, 64))
	}
	if r.Duration < 1 || r.Duration > MaxDurationMinutes {
		return fmt.Errorf("持续时长必须在 1 到 %d 分钟之间", MaxDurationMinutes)
	}
	switch r.Level {
	case "":
		r.Level = levelWarn
	case levelWarn, levelError:
	default:
		return fmt.Errorf("不支持的告警级别 %q", r.Level)
	}
	if def.Scope == ScopeSite || def.Scope == ScopeUpstream {
		if len(r.Sites) > MaxRuleSites {
			return fmt.Errorf("最多指定 %d 个站点", MaxRuleSites)
		}
		seen := map[string]bool{}
		var sites []string
		for _, s := range r.Sites {
			s = strings.TrimSpace(s)
			if s == "" || seen[s] {
				continue
			}
			if len(s) > 200 {
				return errors.New("站点标识过长")
			}
			seen[s] = true
			sites = append(sites, s)
		}
		r.Sites = sites
	} else {
		r.Sites = nil
	}
	if def.Window && r.Metric != MetricSiteQPS {
		if r.MinRequests == 0 {
			r.MinRequests = DefaultMinRequests
		}
		if r.MinRequests < 1 || r.MinRequests > MaxMinRequests {
			return fmt.Errorf("最小请求数必须在 1 到 %d 之间", MaxMinRequests)
		}
	} else {
		r.MinRequests = 0
	}
	return nil
}

// signature 返回影响判定结果的字段摘要：这些字段不变时，修改名称、级别、备注等不会重置运行中的计时与触发状态。
func (r *Rule) signature() string {
	return fmt.Sprintf("%s|%s|%g|%d|%d|%s", r.Metric, r.Op, r.Threshold, r.Duration, r.MinRequests, strings.Join(r.Sites, ","))
}

// compare 判断 value 与阈值的关系是否满足规则。
func (r *Rule) compare(value float64) bool {
	switch r.Op {
	case OpGT:
		return value > r.Threshold
	case OpGE:
		return value >= r.Threshold
	case OpLT:
		return value < r.Threshold
	case OpLE:
		return value <= r.Threshold
	}
	return false
}

// matchesSite 判断站点标识是否在规则的生效范围内。
func (r *Rule) matchesSite(key string) bool {
	if len(r.Sites) == 0 {
		return true
	}
	for _, s := range r.Sites {
		if s == key {
			return true
		}
	}
	return false
}

// formatValue 按单位格式化指标值，用于通知文案；单位用各语言通用的写法，通知翻译时不需要处理。
func formatValue(unit string, v float64) string {
	switch unit {
	case UnitPercent:
		return strconv.FormatFloat(v, 'f', 1, 64) + "%"
	case UnitMB:
		return strconv.FormatFloat(v, 'f', 0, 64) + " MB"
	case UnitQPS:
		return strconv.FormatFloat(v, 'f', 1, 64) + " req/s"
	case UnitMs:
		return strconv.FormatFloat(v, 'f', 0, 64) + " ms"
	}
	return strconv.FormatFloat(v, 'f', 0, 64)
}
