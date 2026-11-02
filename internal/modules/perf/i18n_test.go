package perf

import (
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/modules/webservice"
)

// 告警与恢复通知按服务端语言翻译后不能残留中文（规则名与站点名为用户内容，测试里使用英文）。
func TestAlertMessagesTranslate(t *testing.T) {
	e := newEngine()
	rules := []Rule{
		{Key: "a", Name: "cpu", Enable: true, Metric: MetricHostCPU, Op: OpGT, Threshold: 80, Duration: 1, Level: levelWarn},
		{Key: "b", Name: "p95", Enable: true, Metric: MetricSiteP95, Op: OpGT, Threshold: 1, Duration: 1, MinRequests: 5, Level: levelError},
		{Key: "c", Name: "conns", Enable: true, Metric: MetricUpstreamC, Op: OpGE, Threshold: 1, Duration: 1, Level: levelWarn},
		{Key: "d", Name: "rss", Enable: true, Metric: MetricProcRSS, Op: OpGT, Threshold: 1, Duration: 1, Level: levelWarn},
	}
	site := func(i int) []webservice.PerfSite {
		s := siteAt("l/a", i, 5, 0, 26, 0)
		s.Upstreams = []webservice.PerfUpstream{{URL: "http://10.0.0.1", Healthy: true, Conns: 3}}
		return []webservice.PerfSite{s}
	}
	evs, now := run(e, t0, 25, func(int) hostSample { return hostAt(95) }, site, rules)
	stop := func(int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", 25, 5, 0, 26, 0)} }
	more, _ := run(e, now, 20, func(int) hostSample { h := hostAt(10); h.ProcRSSMB = 0.5; return h }, stop, rules)
	evs = append(evs, more...)
	// 4 条触发；CPU、内存与延迟（无法判定）各恢复一次，后端节点消失时静默清除
	if len(evs) != 7 {
		t.Fatalf("应有 4 条触发与 3 条恢复: %d", len(evs))
	}
	for _, lang := range i18n.Langs[1:] {
		for _, e := range evs {
			for _, s := range []string{e.Title, e.Message, e.Source} {
				out := i18n.Tr(lang, s)
				// 日语与繁体中文的译文本身含汉字，只检查确实被翻译了
				cjk := lang == "zh-TW" || lang == "ja"
				if (!cjk && hasHan(out)) || (cjk && hasHan(s) && out == s) {
					t.Errorf("%s 未翻译完整: %q -> %q", lang, s, out)
				}
			}
		}
	}
}

func hasHan(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }) >= 0
}
