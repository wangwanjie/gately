package webservice

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// ngDirective 是 nginx 配置中的一条指令；Block 非 nil 表示块指令（如 server、location）。
type ngDirective struct {
	Name  string
	Args  []string
	Block []*ngDirective
	// File 与 Line 为指令所在的文件与行号，用于在导入报告中指出出处
	File string
	Line int
}

// isBlock 判断是否为块指令。
func (d *ngDirective) isBlock() bool { return d.Block != nil }

// where 返回“文件:行号”形式的出处。
func (d *ngDirective) where() string {
	if d.File == "" {
		return fmt.Sprintf("第 %d 行", d.Line)
	}
	return fmt.Sprintf("%s:%d", d.File, d.Line)
}

// arg 返回第 i 个参数，不存在时返回空串。
func (d *ngDirective) arg(i int) string {
	if i < len(d.Args) {
		return d.Args[i]
	}
	return ""
}

// ngToken 是词法单元：type 为 word、{、}、;。
type ngToken struct {
	typ  string
	text string
	line int
	// quoted 表示来自引号字符串，引号内的 { } ; 不作为语法符号
	quoted bool
}

// nginxFileMarker 是 nginx -T 输出中每个文件开头的注释。
const nginxFileMarker = "# configuration file "

// parseNginx 解析 nginx 配置文本。支持 nginx -T 的输出：按“# configuration file 路径:”标记拆成多个文件分别解析，
// 再把 include 指令替换为输出中对应文件的内容（不会读取磁盘上的文件）。
// 多个文件时，单个文件的语法错误只跳过该文件并在 warns 中说明（nginx -T 里可能混有被当作配置的其他文件）。
func parseNginx(text string) (out []*ngDirective, warns []string, err error) {
	type segment struct {
		file  string
		start int // 该段第一行在原文中的行号
		body  string
	}
	var segs []segment
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	cur := segment{start: 1}
	var b strings.Builder
	for i, line := range lines {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), nginxFileMarker); ok && strings.HasSuffix(rest, ":") {
			// 第一个标记之前通常只有 nginx -T 的状态行，没有内容时不算一个文件
			if strings.TrimSpace(b.String()) != "" {
				cur.body = b.String()
				segs = append(segs, cur)
			}
			b.Reset()
			cur = segment{file: strings.TrimSuffix(rest, ":"), start: i + 2}
			continue
		}
		// nginx -t / -T 的状态行（如 nginx: the configuration file ... syntax is ok）不是配置内容
		if strings.HasPrefix(line, "nginx: ") {
			line = ""
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	cur.body = b.String()
	segs = append(segs, cur)

	// 先分别解析每个文件
	parsed := map[string][]*ngDirective{}
	var files []string
	for _, s := range segs {
		toks, err := tokenizeNginx(s.body, s.start)
		var block []*ngDirective
		if err == nil {
			p := &ngParser{toks: toks, file: s.file}
			block, err = p.parseBlock(false)
		}
		if err != nil {
			if len(segs) == 1 {
				return nil, nil, err
			}
			name := s.file
			if name == "" {
				name = "开头部分"
			}
			warns = append(warns, fmt.Sprintf("%s 解析失败，已跳过：%v", name, err))
			continue
		}
		parsed[s.file] = block
		files = append(files, s.file)
	}
	// 再像 nginx 一样把 include 替换为对应文件的内容：从第一个文件（主配置）开始展开，
	// 没有被任何 include 引用的文件（例如只粘贴了部分站点文件）按顶层配置处理
	prefix := ""
	if len(segs) > 0 && segs[0].file != "" {
		prefix = path.Dir(segs[0].file)
	}
	included := map[string]bool{}
	var expand func(list []*ngDirective, depth int) []*ngDirective
	expand = func(list []*ngDirective, depth int) []*ngDirective {
		var res []*ngDirective
		for _, d := range list {
			if d.Name == "include" && !d.isBlock() && depth < 16 {
				pattern := d.arg(0)
				if !path.IsAbs(pattern) && prefix != "" {
					pattern = path.Join(prefix, pattern)
				}
				var matched []string
				for _, f := range files {
					if ok, _ := path.Match(pattern, f); ok && f != "" {
						matched = append(matched, f)
					}
				}
				// nginx 按文件名顺序处理通配符匹配到的文件
				sort.Strings(matched)
				for _, f := range matched {
					included[f] = true
					res = append(res, expand(parsed[f], depth+1)...)
				}
				if len(matched) > 0 {
					continue
				}
			}
			if d.isBlock() {
				cp := *d
				cp.Block = expand(d.Block, depth)
				d = &cp
			}
			res = append(res, d)
		}
		return res
	}
	if len(files) > 0 {
		out = append(out, expand(parsed[files[0]], 0)...)
		included[files[0]] = true
		for _, f := range files[1:] {
			if !included[f] {
				out = append(out, expand(parsed[f], 0)...)
			}
		}
	}
	return out, warns, nil
}

// tokenizeNginx 把配置文本切分为词法单元：处理 # 注释、单双引号字符串与转义。
func tokenizeNginx(text string, firstLine int) ([]ngToken, error) {
	var toks []ngToken
	line := firstLine
	i := 0
	for i < len(text) {
		c := text[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '{' || c == '}' || c == ';':
			toks = append(toks, ngToken{typ: string(c), line: line})
			i++
		case c == '"' || c == '\'':
			start := line
			var sb strings.Builder
			i++
			closed := false
			for i < len(text) {
				ch := text[i]
				if ch == '\\' && i+1 < len(text) {
					// 只对引号与反斜杠转义，其余反斜杠原样保留（正则中常见）
					if next := text[i+1]; next == c || next == '\\' {
						sb.WriteByte(next)
						i += 2
						continue
					}
				}
				if ch == c {
					closed = true
					i++
					break
				}
				if ch == '\n' {
					line++
				}
				sb.WriteByte(ch)
				i++
			}
			if !closed {
				return nil, fmt.Errorf("第 %d 行的引号没有闭合", start)
			}
			toks = append(toks, ngToken{typ: "word", text: sb.String(), line: start, quoted: true})
		default:
			start := i
			for i < len(text) {
				ch := text[i]
				if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ';' || ch == '{' || ch == '}' {
					// ${var} 中的大括号属于变量，不是块
					if ch == '{' && i > start && text[i-1] == '$' {
						if j := strings.IndexByte(text[i:], '}'); j > 0 {
							i += j + 1
							continue
						}
					}
					break
				}
				if ch == '\\' && i+1 < len(text) {
					i += 2
					continue
				}
				i++
			}
			toks = append(toks, ngToken{typ: "word", text: text[start:i], line: line})
		}
	}
	return toks, nil
}

type ngParser struct {
	toks []ngToken
	pos  int
	file string
	// depth 为当前块的嵌套层数
	depth int
}

// maxNginxDepth 是 nginx 配置块的最大嵌套层数，正常配置不超过十层。
// 解析是递归的，不限制时数 MB 的 "a{a{a{…" 会耗尽栈导致整个进程崩溃（Go 的栈溢出无法恢复）。
const maxNginxDepth = 64

// parseBlock 解析指令序列，inBlock 为 true 时遇到 } 结束。
func (p *ngParser) parseBlock(inBlock bool) ([]*ngDirective, error) {
	out := []*ngDirective{}
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		switch t.typ {
		case "}":
			if !inBlock {
				return nil, fmt.Errorf("第 %d 行有多余的 }", t.line)
			}
			p.pos++
			return out, nil
		case ";":
			// 空语句
			p.pos++
			continue
		case "{":
			return nil, fmt.Errorf("第 %d 行的 { 前缺少指令名", t.line)
		}
		d := &ngDirective{Name: t.text, File: p.file, Line: t.line}
		p.pos++
		for {
			if p.pos >= len(p.toks) {
				return nil, fmt.Errorf("第 %d 行的指令 %s 没有以 ; 或 {} 结束", d.Line, d.Name)
			}
			nt := p.toks[p.pos]
			if nt.typ == "word" {
				d.Args = append(d.Args, nt.text)
				p.pos++
				continue
			}
			if nt.typ == ";" {
				p.pos++
				break
			}
			if nt.typ == "{" {
				p.pos++
				if p.depth >= maxNginxDepth {
					return nil, fmt.Errorf("第 %d 行的配置块嵌套超过 %d 层", nt.line, maxNginxDepth)
				}
				p.depth++
				block, err := p.parseBlock(true)
				p.depth--
				if err != nil {
					return nil, err
				}
				d.Block = block
				break
			}
			return nil, fmt.Errorf("第 %d 行的指令 %s 缺少 ;", d.Line, d.Name)
		}
		out = append(out, d)
	}
	if inBlock {
		return nil, fmt.Errorf("缺少 }，配置不完整")
	}
	return out, nil
}
