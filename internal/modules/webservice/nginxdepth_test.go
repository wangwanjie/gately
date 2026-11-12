package webservice

import (
	"strings"
	"testing"
)

// 深度嵌套的 nginx 配置不能让解析递归失控：一份 4 MB 以内的 "a{a{a{…" 曾使栈溢出，整个进程崩溃（无法恢复）。
func TestParseNginxNestingLimit(t *testing.T) {
	deep := strings.Repeat("a{", 2_000_000)
	if _, _, err := parseNginx(deep); err == nil {
		t.Fatal("嵌套过深的配置应报错")
	}
	ok := strings.Repeat("a {\n", 20) + strings.Repeat("}\n", 20)
	if _, _, err := parseNginx(ok); err != nil {
		t.Fatalf("正常嵌套不应报错: %v", err)
	}
}
