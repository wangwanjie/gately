package i18n

import (
	"strconv"
	"strings"
)

// 本文件保留改造前的翻译算法，作为对照实现：逐个尝试全部模板与片段，不用字符索引、不走缓存。
// 新实现（translateSlow 的候选筛选、TrFormat 的按格式串翻译）都与它逐句比较，见 TestSlowPathMatchesReference 等。

// refTranslate 是改造前的 catalog.translate（去掉缓存，结果只取决于输入）。
func (c *catalog) refTranslate(s string, depth int) string {
	if v, ok := c.exact[s]; ok {
		return v
	}
	if depth > 4 {
		return s
	}
	for i, t := range c.tmpls {
		if t.must != "" && !strings.Contains(s, t.must) {
			continue
		}
		if m := t.re.FindStringSubmatchIndex(s); m != nil && m[0] == 0 && m[1] == len(s) {
			if t.lead && c.refLongerInside(s, i) {
				break
			}
			return t.refFill(c.refCapture(s, m, depth))
		}
	}
	out := s
	for _, t := range c.tmpls {
		if !refHasHan(out) {
			break
		}
		if t.must != "" && !strings.Contains(out, t.must) {
			continue
		}
		cur := out
		out = replaceAllIndex(t, cur, func(m []int) string {
			return t.refFill(c.refCapture(cur, m, depth))
		})
	}
	for _, k := range c.segs {
		if !refHasHan(out) {
			break
		}
		if strings.Contains(out, k) {
			out = strings.ReplaceAll(out, k, c.exact[k])
		}
	}
	return out
}

func (c *catalog) refLongerInside(s string, i int) bool {
	for _, t := range c.tmpls[:i] {
		if t.nlit <= c.tmpls[i].nlit {
			break
		}
		if t.must != "" && !strings.Contains(s, t.must) {
			continue
		}
		if t.re.MatchString(s) {
			return true
		}
	}
	return false
}

func (c *catalog) refCapture(s string, m []int, depth int) []string {
	var args []string
	for i := 2; i+1 < len(m); i += 2 {
		args = append(args, c.refTranslate(s[m[i]:m[i+1]], depth+1))
	}
	return args
}

// refFill 是改造前的 tmpl.fill：每次调用时用正则解析译文模板。
func (t *tmpl) refFill(args []string) string {
	var sb strings.Builder
	out := t.out
	next := 0
	prev := 0
	for _, loc := range verbRe.FindAllStringIndex(out, -1) {
		sb.WriteString(out[prev:loc[0]])
		prev = loc[1]
		spec := out[loc[0]:loc[1]]
		if spec == "%%" {
			sb.WriteByte('%')
			continue
		}
		idx := next
		if strings.HasPrefix(spec, "%[") {
			if n, err := strconv.Atoi(spec[2:strings.Index(spec, "]")]); err == nil {
				idx = n - 1
			}
		} else {
			next++
		}
		if idx >= 0 && idx < len(args) {
			sb.WriteString(args[idx])
		}
	}
	sb.WriteString(out[prev:])
	return sb.String()
}

// refHasHan 是改造前的 hasHan：逐个字符解码。
func refHasHan(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// refTr 是改造前的 Tr（不含缓存）。
func refTr(lang, s string) string {
	if lang == "" || lang == SourceLang || s == "" || !refHasHan(s) {
		return s
	}
	c := catalogFor(lang)
	if c == nil {
		return s
	}
	return c.refTranslate(s, 0)
}

// refTrLines 是改造前 lineWriter.Write 的逐行翻译。
func refTrLines(lang, p string) string {
	var sb strings.Builder
	for _, ln := range strings.SplitAfter(p, "\n") {
		body := strings.TrimRight(ln, "\r\n")
		sb.WriteString(refTr(lang, body))
		sb.WriteString(ln[len(body):])
	}
	return sb.String()
}
