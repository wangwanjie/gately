// Package i18n 为后端运行时文案提供多语言：源码中的中文字面量即键，译文放在 locales/<语言>.json。
// 二进制只嵌入由 i18nsync 生成的 locales/<语言>.json.gz（体积约为 JSON 的三成），首次用到某种语言时才解压并建表。
//
// 翻译发生在输出边界（HTTP 错误与数据、日志、通知、API 文档），业务代码仍然产生中文原文，
// 因此不需要改动调用点；带 %s / %d 的格式串按模板匹配，译文可用 %[n]s 调整参数顺序。
package i18n

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// SourceLang 是源码文案使用的语言，翻译为该语言时原样返回。
const SourceLang = "zh-CN"

// AllLangs 是 Gately 支持的全部语言，顺序同官网与管理界面。
var AllLangs = []string{"zh-CN", "en", "zh-TW", "ja", "ko", "es", "fr", "de", "ru", "pt-BR"}

// Langs 是本次编译包含的语言，由 locales_*.go 按语言档位的 build tag 决定：
// 默认包含全部语言，lang_cjken 只含简体中文、繁体中文与英语，lang_zh 只含简体中文。
var Langs = builtLangs

// FallbackLang 是无法识别语言偏好时使用的语言：编译了英语时为英语，否则为简体中文。
var FallbackLang = fallbackLang()

func fallbackLang() string {
	for _, l := range Langs {
		if l == "en" {
			return l
		}
	}
	return SourceLang
}

var (
	catalogMu sync.Mutex
	// catalogs 按语言缓存已加载的译文表；值为 nil 表示该语言加载失败，不再重试
	catalogs = map[string]*catalog{}
)

// Normalize 把任意语言标识（如 zh_CN.UTF-8、en-US、pt）规整为受支持的语言代码，无法识别或该语言没有编译进来时返回空串。
func Normalize(tag string) string {
	l := normalizeTag(tag)
	for _, b := range Langs {
		if b == l {
			return l
		}
	}
	return ""
}

// normalizeTag 把语言标识规整为 AllLangs 中的语言代码，不考虑本次编译包含哪些语言。
func normalizeTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	switch {
	case tag == "":
		return ""
	case tag == "c" || tag == "posix":
		return ""
	case strings.HasPrefix(tag, "zh-tw"), strings.HasPrefix(tag, "zh-hk"), strings.HasPrefix(tag, "zh-mo"), strings.HasPrefix(tag, "zh-hant"):
		return "zh-TW"
	case tag == "zh" || strings.HasPrefix(tag, "zh-"):
		return "zh-CN"
	case strings.HasPrefix(tag, "pt"):
		return "pt-BR"
	}
	base := tag
	if i := strings.Index(base, "-"); i >= 0 {
		base = base[:i]
	}
	for _, l := range AllLangs {
		if l == base {
			return l
		}
	}
	return ""
}

// FromAcceptLanguage 解析 Accept-Language 头，返回第一个受支持的语言；没有时返回空串。
func FromAcceptLanguage(h string) string {
	for _, part := range strings.Split(h, ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if l := Normalize(tag); l != "" {
			return l
		}
	}
	return ""
}

// FromEnv 按 GATELY_LANG、LC_ALL、LC_MESSAGES、LANG 的顺序读取系统语言，识别不了时返回 FallbackLang。
func FromEnv() string {
	if l := envLang(); l != "" {
		return l
	}
	return FallbackLang
}

// envLang 返回环境变量指定的语言，没有指定或识别不了（如 Docker 镜像常见的未设置、C.UTF-8）时返回空串。
func envLang() string {
	for _, k := range []string{"GATELY_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := Normalize(os.Getenv(k)); l != "" {
			return l
		}
	}
	return ""
}

var (
	serverMu   sync.RWMutex
	serverLang = ""
	// uiLang 是管理界面最近一次登录时使用的语言，作为「跟随系统」且环境变量未指定语言时的选择
	uiLang = ""
)

// SetServerLang 设置服务端语言（日志、通知等无请求上下文的输出）；传入空串表示按系统环境自动选择。
func SetServerLang(l string) {
	serverMu.Lock()
	serverLang = Normalize(l)
	serverMu.Unlock()
}

// SetUILang 设置管理界面使用的语言，服务端语言为自动且环境变量未指定语言时按它输出。
func SetUILang(l string) {
	serverMu.Lock()
	uiLang = Normalize(l)
	serverMu.Unlock()
}

// ServerLang 返回服务端语言：优先使用设置中的语言，其次是环境变量，再次是管理界面的语言，都没有时为 FallbackLang。
// Docker、飞牛 fpk、OpenWrt 等环境的服务进程通常没有 LANG，若直接回落到英文，界面是中文的用户会收到英文通知。
func ServerLang() string {
	serverMu.RLock()
	l, ui := serverLang, uiLang
	serverMu.RUnlock()
	if l != "" {
		return l
	}
	if l = envLang(); l != "" {
		return l
	}
	if ui != "" {
		return ui
	}
	return FallbackLang
}

type catalog struct {
	exact map[string]string
	tmpls []*tmpl
	// segs 是可用于句中替换的固定片段（按长度降序），覆盖“前缀 + 动态内容”这类拼接出来的文案
	segs []string
	// tmplIdx、segIdx 把模板与片段挂在各自“最少见的字符”下：一句话只需要尝试它包含的字符对应的那几个，
	// 不必把上千个模板和片段逐个做子串查找；tmplAny 为没有固定片段、任何句子都要尝试的模板
	tmplIdx map[rune][]int32
	tmplAny []int32
	segIdx  map[rune][]int32
	// cache 缓存已翻译结果，日志轮询会反复翻译同一批文案
	cache genCache
}

type tmpl struct {
	key  string
	re   *regexp.Regexp
	out  string
	nlit int
	// lead 为模板以动词开头（如“%s失败: %v”）：整句匹配时前缀会落进第一个参数
	lead bool
	// must 是模板中最长的固定片段，用于在跑正则前快速排除不相关的句子
	must string
	// parts 为预先解析好的译文：固定文本与参数序号交替，见 fill
	parts []fillPart
}

// fillPart 是译文模板的一段：lit 为固定文本，arg >= 0 时其后接第 arg 个参数。
type fillPart struct {
	lit string
	arg int
}

// verbRe 匹配格式动词；%% 先于动词被消费，因此“%% packet”不会被误判为动词。
var verbRe = regexp.MustCompile(`%%|%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[a-zA-Z]`)

// realVerbs 返回字符串中的真实动词位置（排除 %%）。
func realVerbs(k string) [][]int {
	var out [][]int
	for _, loc := range verbRe.FindAllStringIndex(k, -1) {
		if k[loc[0]:loc[1]] != "%%" {
			out = append(out, loc)
		}
	}
	return out
}

// catalogFor 返回目标语言的译文表，首次调用时才解压并解析该语言，未用到的语言不占内存。
func catalogFor(lang string) *catalog {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if c, ok := catalogs[lang]; ok {
		return c
	}
	var c *catalog
	if m, err := readLocale(lang); err == nil {
		c = buildCatalog(m)
	}
	catalogs[lang] = c
	return c
}

// readLocale 读取并解压内嵌的 locales/<语言>.json.gz。
func readLocale(lang string) (map[string]string, error) {
	raw, err := localeFS.ReadFile("locales/" + lang + ".json.gz")
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// CompressLocale 把译文 JSON 压缩为内嵌用的 .json.gz；输出只取决于输入，便于提交与校验。
func CompressLocale(data []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(data)
	_ = zw.Close()
	return buf.Bytes()
}

func buildCatalog(m map[string]string) *catalog {
	c := &catalog{exact: map[string]string{}}
	for k, v := range m {
		if v == "" {
			continue
		}
		locs := realVerbs(k)
		if len(locs) == 0 {
			c.exact[k] = v
			continue
		}
		var sb strings.Builder
		nlit := 0
		must := ""
		prev := 0
		t := &tmpl{key: k, out: v}
		for _, loc := range locs {
			lit := strings.ReplaceAll(k[prev:loc[0]], "%%", "%")
			nlit += len([]rune(lit))
			if len(lit) > len(must) {
				must = lit
			}
			sb.WriteString(regexp.QuoteMeta(lit))
			verb := k[loc[1]-1]
			switch {
			case verb == 'd':
				sb.WriteString(`(-?\d+)`)
			case loc[1] == len(k):
				// 以动词结尾时取到句末，常见于“失败: %w”这类包装错误
				sb.WriteString(`(.+)`)
			default:
				sb.WriteString(`(.+?)`)
			}
			prev = loc[1]
		}
		tail := strings.ReplaceAll(k[prev:], "%%", "%")
		nlit += len([]rune(tail))
		if len(tail) > len(must) {
			must = tail
		}
		sb.WriteString(regexp.QuoteMeta(tail))
		re, err := regexp.Compile("(?s)" + sb.String())
		if err != nil {
			continue
		}
		t.re, t.nlit, t.must, t.lead = re, nlit, must, locs[0][0] == 0
		t.parts = parseFill(v)
		c.tmpls = append(c.tmpls, t)
		// 模板同时保留一份精确匹配，避免格式串本身被当作普通文本时漏翻
		c.exact[k] = v
	}
	for k := range c.exact {
		if segmentOK(k) && len(realVerbs(k)) == 0 {
			c.segs = append(c.segs, k)
		}
	}
	// 长度相同时按内容排序：译文表来自 map，不加这一条每次启动的顺序都不同，同一句话的翻译结果可能随之变化
	sort.Slice(c.segs, func(i, j int) bool {
		if len(c.segs[i]) != len(c.segs[j]) {
			return len(c.segs[i]) > len(c.segs[j])
		}
		return c.segs[i] < c.segs[j]
	})
	// 字面量更长的模板优先，避免短模板抢先匹配
	sort.Slice(c.tmpls, func(i, j int) bool {
		if c.tmpls[i].nlit != c.tmpls[j].nlit {
			return c.tmpls[i].nlit > c.tmpls[j].nlit
		}
		return c.tmpls[i].key < c.tmpls[j].key
	})
	c.buildIndex()
	return c
}

// buildIndex 建立模板与片段的字符索引：每个模式挂在它包含的、在全部模式中出现最少的那个字符下。
// 句子里没有这个字符，就一定不包含这个模式；常见字（如“失败”）下只挂很少的模式，候选集合因此很小。
func (c *catalog) buildIndex() {
	freq := map[rune]int{}
	count := func(p string) {
		seen := map[rune]bool{}
		for _, r := range p {
			if !seen[r] {
				seen[r] = true
				freq[r]++
			}
		}
	}
	for _, t := range c.tmpls {
		count(t.must)
	}
	for _, k := range c.segs {
		count(k)
	}
	rarest := func(p string) rune {
		best, n := rune(-1), 0
		for _, r := range p {
			if best < 0 || freq[r] < n {
				best, n = r, freq[r]
			}
		}
		return best
	}
	c.tmplIdx, c.segIdx = map[rune][]int32{}, map[rune][]int32{}
	for i, t := range c.tmpls {
		if t.must == "" {
			c.tmplAny = append(c.tmplAny, int32(i))
			continue
		}
		r := rarest(t.must)
		c.tmplIdx[r] = append(c.tmplIdx[r], int32(i))
	}
	for i, k := range c.segs {
		r := rarest(k)
		c.segIdx[r] = append(c.segIdx[r], int32(i))
	}
}

// candidates 返回下标不小于 from、且可能出现在 s 中的模式（升序）：s 含有该模式的索引字符，或模式属于 always。
// 结果追加到 out[:0]。每个模式只挂在一个字符下，各字符的列表互不相交，合并后无需去重。
func candidates(idx map[rune][]int32, always []int32, s string, from int32, out []int32) []int32 {
	out = out[:0]
	for _, i := range always {
		if i >= from {
			out = append(out, i)
		}
	}
	// 同一个字符只查一次：ASCII 与常用汉字用位图去重，其余字符（全角标点等）很少，线性查找即可
	var ascii [2]uint64
	var han [(hanHi - hanLo + 64) / 64]uint64
	var otherBuf [16]rune
	other := otherBuf[:0]
	for _, r := range s {
		switch {
		case r < 128:
			if ascii[r>>6]&(1<<(r&63)) != 0 {
				continue
			}
			ascii[r>>6] |= 1 << (r & 63)
		case r >= hanLo && r <= hanHi:
			o := r - hanLo
			if han[o>>6]&(1<<(o&63)) != 0 {
				continue
			}
			han[o>>6] |= 1 << (o & 63)
		default:
			if slices.Contains(other, r) {
				continue
			}
			other = append(other, r)
		}
		for _, i := range idx[r] {
			if i >= from {
				out = append(out, i)
			}
		}
	}
	slices.Sort(out)
	return out
}

// parseFill 把译文模板解析为固定文本与参数序号，支持 %% 与 %[n]x 形式的显式序号。
func parseFill(out string) []fillPart {
	var parts []fillPart
	var lit strings.Builder
	next := 0
	prev := 0
	for _, loc := range verbRe.FindAllStringIndex(out, -1) {
		lit.WriteString(out[prev:loc[0]])
		prev = loc[1]
		spec := out[loc[0]:loc[1]]
		if spec == "%%" {
			lit.WriteByte('%')
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
		// 序号无效的动词不输出任何内容，但仍要断开，保证固定文本的顺序
		parts = append(parts, fillPart{lit: lit.String(), arg: max(idx, -1)})
		lit.Reset()
	}
	lit.WriteString(out[prev:])
	return append(parts, fillPart{lit: lit.String(), arg: -1})
}

// fill 把参数代入译文模板。
func (t *tmpl) fill(args []string) string {
	n := 0
	for _, p := range t.parts {
		n += len(p.lit)
		if p.arg >= 0 && p.arg < len(args) {
			n += len(args[p.arg])
		}
	}
	var sb strings.Builder
	sb.Grow(n)
	for _, p := range t.parts {
		sb.WriteString(p.lit)
		if p.arg >= 0 && p.arg < len(args) {
			sb.WriteString(args[p.arg])
		}
	}
	return sb.String()
}

// Tr 把中文原文翻译为目标语言；找不到译文时原样返回。
func Tr(lang, s string) string {
	if lang == "" || lang == SourceLang || s == "" || !hasHan(s) {
		return s
	}
	c := catalogFor(lang)
	if c == nil {
		return s
	}
	return c.translate(lang, s, 0)
}

// TrExact 只按整句精确匹配翻译，用于可能是程序文案也可能是用户填写名称的字段（如事件来源）：
// 程序文案（“管理后台”）能翻译，用户起的名称不会被句中片段替换改写。
func TrExact(lang, s string) string {
	if lang == "" || lang == SourceLang || s == "" || !hasHan(s) {
		return s
	}
	c := catalogFor(lang)
	if c == nil {
		return s
	}
	if v, ok := c.exact[s]; ok {
		return v
	}
	return s
}

// 源码文案里的汉字范围（CJK 统一表意文字基本区），UTF-8 编码为 E4 B8 80 ~ E9 BF BF
const (
	hanLo = 0x4e00
	hanHi = 0x9fff
)

// hasHan 判断 s 是否含有汉字。汉字的 UTF-8 首字节只可能是 0xE4~0xE9，先按字节找到候选位置再解码确认，
// 纯 ASCII 的内容（访问日志、配置数据）不必逐个字符解码。
func hasHan[T string | []byte](s T) bool {
	for i := 0; i+2 < len(s); i++ {
		if b := s[i]; b < 0xe4 || b > 0xe9 {
			continue
		}
		// 首字节之后必须是两个后续字节（10xxxxxx），否则不是合法的三字节编码
		if s[i+1]&0xc0 != 0x80 || s[i+2]&0xc0 != 0x80 {
			continue
		}
		r := rune(s[i]&0x0f)<<12 | rune(s[i+1]&0x3f)<<6 | rune(s[i+2]&0x3f)
		if r >= hanLo && r <= hanHi {
			return true
		}
	}
	return false
}

// segmentOK 判断固定片段能否用于句中替换：太短的词（如“删除”）容易误伤，除非以冒号结尾（拼接前缀）。
func segmentOK(k string) bool {
	if len([]rune(k)) >= 4 {
		return true
	}
	t := strings.TrimRight(k, " ")
	return strings.HasSuffix(t, ":") || strings.HasSuffix(t, "：")
}

func (c *catalog) translate(lang, s string, depth int) string {
	if v, ok := c.exact[s]; ok {
		return v
	}
	// 整句与第一层参数（如“失败: <内层错误>”里的内层错误）走缓存；更深的嵌套很少见，直接计算
	if depth > 1 {
		return c.translateSlow(lang, s, depth)
	}
	if v, ok := c.cache.get(s); ok {
		return v
	}
	out := c.translateSlow(lang, s, depth)
	c.cache.put(s, out)
	return out
}

func (c *catalog) translateSlow(lang, s string, depth int) string {
	if v, ok := c.exact[s]; ok {
		return v
	}
	if depth > 4 {
		return s
	}
	var buf [48]int32
	// 先尝试整句匹配，再对句中出现的已知模板逐段替换（如“加载失败: <内层错误>”）。
	// 只尝试固定片段可能出现在句中的模板，顺序与逐个尝试全部模板相同
	cands := candidates(c.tmplIdx, c.tmplAny, s, 0, buf[:0])
	for ci, i := range cands {
		t := c.tmpls[i]
		if t.must != "" && !strings.Contains(s, t.must) {
			continue
		}
		if m := t.re.FindStringSubmatchIndex(s); m != nil && m[0] == 0 && m[1] == len(s) {
			// 句中还出现了更具体的模板时改走逐段替换：带前缀的日志行（“[warn] [auth] 来自 x 的登录失败: ...”）
			// 会被“%s失败: %v”这类通用模板整句匹配，前缀连同具体句子落进参数里而无法翻译。
			// 只有以动词开头的模板会把前缀吞进参数；以字面量开头的模板已锚定句首，整句匹配更准确
			if t.lead && c.longerInside(s, cands[:ci], t.nlit) {
				break
			}
			return t.fill(c.capture(lang, s, m, depth))
		}
	}
	out := s
	for ci := 0; ci < len(cands); ci++ {
		if !hasHan(out) {
			break
		}
		t := c.tmpls[cands[ci]]
		if t.must != "" && !strings.Contains(out, t.must) {
			continue
		}
		cur := out
		out = replaceAllIndex(t, cur, func(m []int) string {
			return t.fill(c.capture(lang, cur, m, depth))
		})
		if out != cur {
			// 替换进来的译文可能带来新的字符，后面的模板按新内容重新筛选
			cands = candidates(c.tmplIdx, c.tmplAny, out, cands[ci]+1, cands[:0])
			ci = -1
		}
	}
	// 最后替换拼接出来的固定前缀/片段
	segs := candidates(c.segIdx, nil, out, 0, cands[:0])
	for ci := 0; ci < len(segs); ci++ {
		if !hasHan(out) {
			break
		}
		k := c.segs[segs[ci]]
		if strings.Contains(out, k) {
			out = strings.ReplaceAll(out, k, c.exact[k])
			segs = candidates(c.segIdx, nil, out, segs[ci]+1, segs[:0])
			ci = -1
		}
	}
	return out
}

// longerInside 判断字面量比 nlit 更多的模板是否出现在句中；before 为排在当前模板之前的候选（按字面量长度降序）。
func (c *catalog) longerInside(s string, before []int32, nlit int) bool {
	for _, i := range before {
		t := c.tmpls[i]
		if t.nlit <= nlit {
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

func (c *catalog) capture(lang, s string, m []int, depth int) []string {
	var args []string
	for i := 2; i+1 < len(m); i += 2 {
		part := s[m[i]:m[i+1]]
		args = append(args, c.translate(lang, part, depth+1))
	}
	return args
}

func replaceAllIndex(t *tmpl, s string, repl func(m []int) string) string {
	var sb strings.Builder
	last := 0
	for _, m := range t.re.FindAllStringSubmatchIndex(s, -1) {
		sb.WriteString(s[last:m[0]])
		sb.WriteString(repl(m))
		last = m[1]
	}
	sb.WriteString(s[last:])
	return sb.String()
}

// Verbs 返回格式串中的动词序列（不含 %%），供校验译文与原文参数一致。
func Verbs(s string) []string {
	var out []string
	for _, loc := range realVerbs(s) {
		out = append(out, s[loc[0]:loc[1]])
	}
	return out
}
