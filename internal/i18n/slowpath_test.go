package i18n

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// compareLangs 返回对照测试覆盖的语言：默认为全部 9 种非简体中文语言；
// 开启竞态检测时每句话的耗时放大一个数量级，只抽英语、繁体中文与日语（译文含汉字，会触发二次匹配）。
func compareLangs() []string {
	if raceEnabled {
		return []string{"en", "zh-TW", "ja"}
	}
	var out []string
	for _, l := range Langs {
		if l != SourceLang {
			out = append(out, l)
		}
	}
	return out
}

// corpusLine 是对照测试里的一句日志。
type corpusLine struct {
	format, level, msg string
	// args 为各参数格式化后的文字，lens 为它们的长度；格式串不能逐个参数拆开时两者为空
	args []string
	lens []uint16
}

// renderParts 用 format 的各个动词分别格式化对应的参数，返回这些文字及其长度；
// “固定片段 + 参数”拼起来与 fmt.Sprintf 的结果不同时（显式序号等）返回 nil。
func renderParts(format, msg string, args []any) ([]string, []uint16) {
	locs := realVerbs(format)
	if len(locs) != len(args) || len(locs) == 0 {
		return nil, nil
	}
	var sb strings.Builder
	var parts []string
	var lens []uint16
	prev := 0
	for i, loc := range locs {
		sb.WriteString(strings.ReplaceAll(format[prev:loc[0]], "%%", "%"))
		p := fmt.Sprintf(format[loc[0]:loc[1]], args[i])
		sb.WriteString(p)
		parts = append(parts, p)
		lens = append(lens, uint16(len(p)))
		prev = loc[1]
	}
	sb.WriteString(strings.ReplaceAll(format[prev:], "%%", "%"))
	if sb.String() != msg {
		return nil, nil
	}
	return parts, lens
}

// corpus 用仓库里全部日志格式串各生成两句日志：一句参数不含中文，一句参数含中文（用户起的名称、内层错误）。
// 错误文案（fmt.Errorf / errors.New）同样可能被当作格式串传给日志，也各生成两句，级别记为 error。
func corpus(t testing.TB) []corpusLine {
	logs, inner := repoFormats(t)
	if len(logs) < 200 || len(inner) < 500 {
		t.Fatalf("扫描到的格式串过少（日志 %d，内层错误 %d），请检查扫描逻辑", len(logs), len(inner))
	}
	seen := map[string]bool{}
	for _, f := range logs {
		seen[f.format] = true
	}
	for _, s := range inner {
		if !seen[s] {
			logs = append(logs, logFormat{format: strings.ReplaceAll(s, "%w", "%v"), level: "error"})
		}
	}
	var out []corpusLine
	for i, f := range logs {
		for _, han := range []bool{false, true} {
			if han && len(Verbs(f.format)) == 0 {
				continue
			}
			args := sampleArgs(f.format, han, inner, i)
			ln := corpusLine{format: f.format, level: f.level, msg: fmt.Sprintf(f.format, args...)}
			ln.args, ln.lens = renderParts(f.format, ln.msg, args)
			out = append(out, ln)
		}
	}
	return out
}

// 字符索引只是跳过不可能匹配的模板与片段，翻译结果必须与逐个尝试全部模板的旧实现完全相同：
// 全部语言、全部日志格式串（参数含与不含中文）、带时间与级别前缀的回显行逐句比较。
func TestSlowPathMatchesReference(t *testing.T) {
	lines := corpus(t)
	for _, lang := range compareLangs() {
		c := catalogFor(lang)
		if c == nil {
			t.Fatalf("%s 译文表加载失败", lang)
		}
		diff := 0
		for _, ln := range lines {
			for _, s := range []string{ln.msg, "2026/11/16 22:23:28 [" + ln.level + "] [家里的NAS] " + ln.msg} {
				want := c.refTranslate(s, 0)
				if got := c.translateSlow(lang, s, 0); got != want {
					if diff++; diff <= 5 {
						t.Errorf("%s: %q\n  旧实现 %q\n  新实现 %q", lang, s, want, got)
					}
				}
			}
		}
		if diff > 0 {
			t.Errorf("%s: 共 %d 句与旧实现不同", lang, diff)
		}
	}
}

// hasHan 改为按字节查找后，对任意字节序列（含不合法的 UTF-8）的判断必须与逐字符解码相同。
func TestHasHanMatchesRuneScan(t *testing.T) {
	fixed := []string{"", "abc", "中", "a中b", "\xe4", "\xe4\xb8", "\xe4\xb8\x80", "\xe9\xbf\xbf", "\xe9\xbf\xc0", "\xea\x80\x80",
		"\xe3\xbf\xbf", "\xe4\xb7\xbf", "\xe4\xb8\x7f", "（）", "あ", "한", "\xed\xa0\x80", "\xe4\xe4\xb8\x80", "\xf0\x9f\x98\x80", "㐀", "鿿", "ꀀ"}
	for _, s := range fixed {
		if got, want := hasHan(s), refHasHan(s); got != want {
			t.Errorf("hasHan(%q) = %v，逐字符解码为 %v", s, got, want)
		}
		if got, want := hasHan([]byte(s)), refHasHan(s); got != want {
			t.Errorf("hasHan([]byte(%q)) = %v，逐字符解码为 %v", s, got, want)
		}
	}
	rnd := rand.New(rand.NewSource(1))
	alphabet := []byte{'a', ' ', 0x80, 0xb8, 0xbf, 0xc0, 0xe3, 0xe4, 0xe5, 0xe9, 0xea, 0xed, 0xf0, 0x7f, 0xa0}
	for i := 0; i < 200000; i++ {
		b := make([]byte, rnd.Intn(8))
		for j := range b {
			b[j] = alphabet[rnd.Intn(len(alphabet))]
		}
		if got, want := hasHan(b), refHasHan(string(b)); got != want {
			t.Fatalf("hasHan(%q) = %v，逐字符解码为 %v", b, got, want)
		}
	}
}
