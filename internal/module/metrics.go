package module

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// 指标类型
const (
	MetricCounter = "counter"
	MetricGauge   = "gauge"
	// MetricHistogram 的采样点由 Family 归组：_bucket、_sum、_count 共用同一组 HELP / TYPE
	MetricHistogram = "histogram"
)

// Sample 是一个 Prometheus 指标采样点。
type Sample struct {
	Name string
	// Family 为 HELP / TYPE 所属的指标族名称，留空时取 Name。直方图的 _bucket / _sum / _count 填基础名称
	Family string
	// Help 与 Type 取同名指标第一个采样点的值
	Help   string
	Type   string
	Labels map[string]string
	Value  float64
}

// Metricer 由希望导出 Prometheus 指标的模块实现。
type Metricer interface {
	Metrics() []Sample
}

func (s Sample) family() string {
	if s.Family != "" {
		return s.Family
	}
	return s.Name
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

// WriteSamples 以 Prometheus 文本格式（0.0.4）写出采样点。同名指标聚合在一起，仅输出一次 HELP / TYPE。
func WriteSamples(w io.Writer, samples []Sample) error {
	var order []string
	groups := map[string][]Sample{}
	for _, s := range samples {
		fam := s.family()
		if _, ok := groups[fam]; !ok {
			order = append(order, fam)
		}
		groups[fam] = append(groups[fam], s)
	}
	for _, name := range order {
		g := groups[name]
		if g[0].Help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, g[0].Help); err != nil {
				return err
			}
		}
		typ := g[0].Type
		if typ == "" {
			typ = MetricGauge
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", name, typ); err != nil {
			return err
		}
		for _, s := range g {
			if _, err := fmt.Fprintf(w, "%s%s %s\n", s.Name, formatLabels(s.Labels), strconv.FormatFloat(s.Value, 'g', -1, 64)); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatLabels(l map[string]string) string {
	if len(l) == 0 {
		return ""
	}
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+`="`+labelEscaper.Replace(l[k])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
