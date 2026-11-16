package i18n

import "strings"

// TrFormat 翻译一条由 fmt.Sprintf(format, …) 生成的文案 msg：format 为源码里的中文格式串，
// argLens 为各个参数在 msg 中的字节长度（按出现顺序）。两者未知时传空值，结果与 Tr(lang, msg) 相同。
//
// 日志在写入时就知道自己的格式串和每个参数的位置：用格式串在译文表里一次查表得到译文模板，按位置切出参数代入，
// 不必像 Tr 那样在全部模板与片段里找出这句话用的是哪一个，也不会因为参数里恰好含有格式串的固定片段而切错。
// 参数里的中文（如内层错误文本）仍各自经 Tr 翻译。format 不在译文表里时（纯英文的访问日志格式、动态拼出来的文本）
// 同样退回 Tr。
func TrFormat(lang, format, msg string, argLens []uint16) string {
	if lang == "" || lang == SourceLang || msg == "" || !hasHan(msg) {
		return msg
	}
	c := catalogFor(lang)
	if c == nil {
		return msg
	}
	if out, ok := c.trFormat(lang, format, msg, argLens); ok {
		return out
	}
	return c.translate(lang, msg, 0)
}

// TrFormatLines 与 TrFormat 相同，但无法按格式串翻译时逐行翻译（见 TrLines），用于标准错误输出的日志回显。
func TrFormatLines(lang, format, msg string, argLens []uint16) string {
	if lang == "" || lang == SourceLang || msg == "" || !hasHan(msg) {
		return msg
	}
	c := catalogFor(lang)
	if c == nil {
		return msg
	}
	if out, ok := c.trFormat(lang, format, msg, argLens); ok {
		return out
	}
	return TrLines(lang, msg)
}

// trFormat 按格式串翻译 msg；格式串没有对应的模板，或 msg 与格式串、参数长度对不上时返回 false。
func (c *catalog) trFormat(lang, format, msg string, argLens []uint16) (string, bool) {
	// 与 Tr 一样，整句恰好是译文表里的文案时直接取译文（没有动词的格式串都走这里）
	if v, ok := c.exact[msg]; ok {
		return v, true
	}
	if format == "" || len(argLens) == 0 {
		return "", false
	}
	t := c.byKey[format]
	if t == nil {
		return "", false
	}
	var buf [8]string
	args, ok := t.cut(msg, argLens, buf[:0])
	if !ok {
		return "", false
	}
	for i, a := range args {
		if hasHan(a) {
			args[i] = c.translate(lang, a, 1)
		}
	}
	return t.fill(args), true
}

// cut 按格式串的固定片段与各参数的长度从 msg 中切出参数（追加到 args）。
// 片段与长度必须恰好拼成 msg，否则说明 msg 不是由这个格式串生成的，返回 false。
func (t *tmpl) cut(msg string, argLens []uint16, args []string) ([]string, bool) {
	if len(argLens) != len(t.lits)-1 {
		return nil, false
	}
	rest := msg
	for i, n := range argLens {
		lit := t.lits[i]
		if len(rest) < len(lit)+int(n) || rest[:len(lit)] != lit {
			return nil, false
		}
		args = append(args, rest[len(lit):len(lit)+int(n)])
		rest = rest[len(lit)+int(n):]
	}
	if rest != t.lits[len(argLens)] {
		return nil, false
	}
	return args, true
}

// TrLines 逐行翻译 s：每行去掉行尾的换行（\n 或 \r\n）后单独翻译，换行原样保留。
// 译文表里的文案不带行尾换行，连同换行一起翻译会查不到；多行输出（命令结果、堆栈）也应一行一行地处理。
func TrLines(lang, s string) string {
	if lang == "" || lang == SourceLang || !hasHan(s) {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for len(s) > 0 {
		ln := s
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			ln = s[:i+1]
		}
		s = s[len(ln):]
		body := strings.TrimRight(ln, "\r\n")
		sb.WriteString(Tr(lang, body))
		sb.WriteString(ln[len(body):])
	}
	return sb.String()
}

// TrLogSource 翻译日志回显行里方括号中的来源标签。标签多是用户起的规则、站点名称，只做句中片段替换、
// 不做整句匹配：与翻译整行（“[warn] [来源] 内容”）时对这一段的处理相同，恰好与某条文案同名的短名称保持原样。
func TrLogSource(lang, source string) string {
	if lang == "" || lang == SourceLang || !hasHan(source) {
		return source
	}
	out := Tr(lang, "["+source+"] ")
	if strings.HasPrefix(out, "[") && strings.HasSuffix(out, "] ") {
		return out[1 : len(out)-2]
	}
	return source
}
