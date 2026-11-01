package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAPIDocsMarkdownSafe 检查生成的 API.md 中代码片段以外没有 <xxx 形式的文本：
// 官网用 VitePress 渲染这些文件，裸露的 <仓库地址> 会被当成 HTML 标签导致构建失败，应写在反引号里。
func TestAPIDocsMarkdownSafe(t *testing.T) {
	files, _ := filepath.Glob("../../docs/api/*/API.md")
	if root, err := filepath.Glob("../../docs/api/API.md"); err == nil {
		files = append(files, root...)
	}
	if len(files) == 0 {
		t.Skip("尚未生成 docs/api（make api-docs）")
	}
	code := regexp.MustCompile("`[^`]*`")
	tag := regexp.MustCompile(`<[^\s=<>]`)
	inFence := false
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			if m := tag.FindString(code.ReplaceAllString(line, "")); m != "" {
				t.Errorf("%s:%d 代码片段外出现 %q，请写在反引号中: %s", f, i+1, m, line)
			}
		}
	}
}

// 多段说明按段落翻译时，由多个字符串常量拼成的 apiIntro 也要完整翻译，不能留下中文段落。
func TestTrDocDeepTranslatesIntro(t *testing.T) {
	han := regexp.MustCompile(`\p{Han}`)
	for _, lang := range []string{"en", "de", "ja"} {
		out, _ := trDocDeep(lang, apiIntro).(string)
		if lang != "ja" && han.MatchString(out) {
			t.Errorf("%s 译文仍含中文: %s", lang, out)
		}
		if lang == "ja" && strings.Contains(out, "用于脚本与自动化") {
			t.Errorf("ja 译文仍含中文原文: %s", out)
		}
		if strings.Count(out, "\n\n") != strings.Count(apiIntro, "\n\n") {
			t.Errorf("%s 段落数改变: %q", lang, out)
		}
	}
}
