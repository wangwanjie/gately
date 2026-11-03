//go:build lang_zh || lang_cjken

package i18n

import (
	"slices"
	"testing"
)

// 精简语言档位只编译所选语言：lang_zh 只有简体中文且回退为简体中文，lang_cjken 为简繁英且回退为英语；
// 已编译语言的译文可用，未编译的语言无法识别、按回退语言显示。CI 用对应 tag 单独运行。
func TestLangTier(t *testing.T) {
	zhOnly := slices.Equal(Langs, []string{"zh-CN"})
	if zhOnly {
		if FallbackLang != "zh-CN" {
			t.Fatalf("lang_zh 应回退到简体中文，实际 %s", FallbackLang)
		}
	} else {
		if !slices.Equal(Langs, []string{"zh-CN", "en", "zh-TW"}) || FallbackLang != "en" {
			t.Fatalf("lang_cjken 应为简繁英并回退英语: %v %s", Langs, FallbackLang)
		}
		if got := Tr("en", "请求无效"); got == "请求无效" || got == "" {
			t.Fatalf("已编译的英语译文应可用，得到 %q", got)
		}
	}
	// 未编译的语言无法识别，界面与日志按回退语言显示
	if got := Normalize("ja_JP.UTF-8"); got != "" {
		t.Fatalf("未编译的语言不应被识别，得到 %q", got)
	}
	t.Setenv("GATELY_LANG", "ja")
	if got := FromEnv(); got != FallbackLang {
		t.Fatalf("环境变量指定未编译的语言时应回退到 %s，得到 %s", FallbackLang, got)
	}
}
