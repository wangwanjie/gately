package waf

import (
	"io"
	"log"
	"net/url"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
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
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(200, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

var sqlAttack = "/?id=" + url.QueryEscape("1' OR '1'='1' UNION SELECT password FROM users--")

// 配置文件缺阈值、偏执等级等参数时加载后补上默认值。阈值为 0 时 CRS 的阻断判定恒成立，
// 拦截模式下连正常请求都会被拦；规则组类型为空时不会按 CRS 加载初始化设置。
func TestInitFillsMissingInstanceDefaults(t *testing.T) {
	m := initFromRaw(t, map[string]any{"instances": []any{
		map[string]any{"key": "typed", "name": "typed", "enable": true, "ruleSets": []any{map[string]any{"enable": true, "type": "crs"}}},
		map[string]any{"key": "untyped", "name": "untyped", "enable": true, "ruleSets": []any{map[string]any{"enable": true}}},
	}})
	in := m.cfg.Instances[0]
	if in.Mode != ModeBlock || in.Paranoia != 1 || in.InboundThreshold != 5 || in.OutboundThreshold != 4 {
		t.Fatalf("未补默认值: %+v", in)
	}
	if in.RegexPrefilter == nil || !*in.RegexPrefilter || in.BodyLimitAction != BodyLimitReject {
		t.Fatalf("未补默认值: prefilter=%v bodyLimitAction=%q", in.RegexPrefilter, in.BodyLimitAction)
	}
	if in.SafeLine.TimeoutMs != defaultSafeLineTimeoutMs || in.SafeLine.BodyLimitKB != defaultSafeLineBodyKB {
		t.Fatalf("雷池参数未补默认值: %+v", in.SafeLine)
	}
	if m.cfg.Instances[1].RuleSets[0].Type != RuleCRS {
		t.Fatalf("规则组类型未补默认值: %q", m.cfg.Instances[1].RuleSets[0].Type)
	}
	d, err := in.directives()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "inbound_anomaly_score_threshold=5") || !strings.Contains(d, "blocking_paranoia_level=1") {
		t.Fatalf("生成的规则未使用默认阈值:\n%s", d)
	}
	// 运行时：正常请求放行、攻击请求拦截，两种写法的实例都是如此
	for _, key := range []string{"typed", "untyped"} {
		if e := m.errors[key]; e != "" {
			t.Fatalf("%s 编译失败: %s", key, e)
		}
		h := m.Middleware(key, ok)
		if c := status(t, h, "/index.html?page=2"); c != 200 {
			t.Fatalf("%s: 正常请求不应被拦截: %d", key, c)
		}
		if c := status(t, h, sqlAttack); c != 403 {
			t.Fatalf("%s: 攻击请求应被拦截: %d", key, c)
		}
	}
}

// 已有的值保持不变，包括显式关闭的正则预过滤。
func TestInitKeepsExistingInstanceValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{"instances": []any{map[string]any{
		"key": "w", "name": "w", "enable": true, "mode": "detect", "paranoia": 2,
		"inboundThreshold": 10, "outboundThreshold": 8, "regexPrefilter": false, "bodyLimitAction": "partial",
		"ruleSets": []any{map[string]any{"enable": true, "type": "custom", "param": `SecRule ARGS "@contains x" "id:1001,phase:2,deny"`}},
		"safeline": map[string]any{"timeoutMs": 300, "bodyLimitKB": 64},
	}}})
	in := m.cfg.Instances[0]
	if in.Mode != ModeDetect || in.Paranoia != 2 || in.InboundThreshold != 10 || in.OutboundThreshold != 8 {
		t.Fatalf("已有值被改动: %+v", in)
	}
	if in.RegexPrefilter == nil || *in.RegexPrefilter || in.BodyLimitAction != BodyLimitPartial || in.RuleSets[0].Type != RuleCustom {
		t.Fatalf("已有值被改动: prefilter=%v bodyLimitAction=%q type=%q", in.RegexPrefilter, in.BodyLimitAction, in.RuleSets[0].Type)
	}
	if in.SafeLine.TimeoutMs != 300 || in.SafeLine.BodyLimitKB != 64 {
		t.Fatalf("雷池参数被改动: %+v", in.SafeLine)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameInstanceDefaults(t *testing.T) {
	in := Instance{Name: "w", RuleSets: []RuleSet{{Enable: true}}}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	if in.Mode != ModeBlock || in.Paranoia != 1 || in.InboundThreshold != 5 || in.OutboundThreshold != 4 ||
		in.RegexPrefilter == nil || !*in.RegexPrefilter || in.BodyLimitAction != BodyLimitReject || in.RuleSets[0].Type != RuleCRS ||
		in.SafeLine.TimeoutMs != defaultSafeLineTimeoutMs || in.SafeLine.BodyLimitKB != defaultSafeLineBodyKB {
		t.Fatalf("保存时默认值不符: %+v", in)
	}
	if err := (&Instance{Name: "bad", Paranoia: 9}).Validate(); err == nil {
		t.Fatal("偏执等级超出范围应被拒绝")
	}
}
