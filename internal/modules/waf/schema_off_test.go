//go:build coraza.disabled_operators.validateSchema

package waf

import (
	"errors"
	"strings"
	"testing"
)

// 发布构建去掉了 @validateSchema：自定义规则用到它时保存失败，并说明原因；CRS 照常可用。
// 运行方式：go test -tags coraza.disabled_operators.validateSchema ./internal/modules/waf/
func TestValidateSchemaUnavailable(t *testing.T) {
	in := Instance{Name: "s", Enable: true, RuleSets: []RuleSet{
		{Enable: true, Type: RuleCRS},
		{Enable: true, Type: RuleCustom, Param: `SecRule REQUEST_BODY "@validateSchema /etc/gately/schema.json" "id:10010,phase:2,deny,status:403"`},
	}}
	err := in.Validate()
	if !errors.Is(err, errValidateSchemaUnavailable) || !strings.Contains(err.Error(), "@validateSchema") {
		t.Fatalf("应提示当前版本不含 @validateSchema: %v", err)
	}
	// 已保存的配置在启动时编译失败，错误同样清楚（界面显示该错误，引用的站点放行）
	m := newModule(t)
	in.Key = "s"
	m.cfg.Instances = append(m.cfg.Instances, in)
	m.buildLocked("s")
	if e := m.errors["s"]; !strings.Contains(e, "@validateSchema") {
		t.Fatalf("启动时的编译错误应说明原因: %q", e)
	}
	// 只用 CRS 的实例不受影响
	crs := Instance{Name: "crs", Enable: true, RuleSets: []RuleSet{{Enable: true, Type: RuleCRS}}}
	if err := crs.Validate(); err != nil {
		t.Fatalf("CRS 不使用 @validateSchema，应能编译: %v", err)
	}
	// 其他自定义规则的编译错误原样返回
	bad := Instance{Name: "b", Enable: true, RuleSets: []RuleSet{{Enable: true, Type: RuleCustom, Param: `SecRule ARGS "@nosuchop x" "id:10011,phase:2,deny"`}}}
	if err := bad.Validate(); err == nil || errors.Is(err, errValidateSchemaUnavailable) {
		t.Fatalf("其他操作符错误不应被改写: %v", err)
	}
}
