package i18n

import (
	"bytes"
	"encoding/json"
)

// mayHaveHanJSON 判断一段 JSON 里是否可能有汉字：直接以 UTF-8 写出的汉字，或 \u4e00 ~ \u9fff 形式的转义。
// 都没有时整个响应不需要翻译。
func mayHaveHanJSON(b []byte) bool {
	if hasHan(b) {
		return true
	}
	for i := 0; ; {
		j := bytes.Index(b[i:], []byte(`\u`))
		if j < 0 {
			return false
		}
		i += j + 2
		if i < len(b) && b[i] >= '4' && b[i] <= '9' {
			return true
		}
	}
}

// jsonMaxDepth 为改写时允许的嵌套层数，超过时放弃改写（与 encoding/json 的上限同量级）
const jsonMaxDepth = 1000

// rewriteJSON 翻译 JSON 文本 src 中需要翻译的字符串值，其余内容逐字节照抄，返回改写后的文本与是否有改动。
//
// deep 为 true 时翻译全部字符串值（错误响应与完全由程序生成的接口）；否则只翻译 messageKeys 字段下的字符串，
// 以及 stringMessageKeys 字段的字符串值，用户填写的名称、备注等保持原样——与 TrDeep、TrFields 的范围相同。
// 对象的键从不翻译。src 不是合法 JSON 时不做任何改动。
//
// 相比“解析成 any 再整个重新编码”，没有改动的部分保持处理器写出的原样：字段顺序不变，
// 超过 2^53 的整数不会被转成浮点数，也不必为整棵树分配内存。
func rewriteJSON(lang string, src []byte, deep bool) ([]byte, bool) {
	r := jsonRewriter{src: src, lang: lang}
	if !r.value(deep, false, 0) {
		return nil, false
	}
	r.space()
	if r.pos != len(src) || r.out == nil {
		// 末尾还有多余内容（不是单个 JSON 值），或没有任何字符串被改写
		return nil, false
	}
	return append(r.out, src[r.copied:]...), true
}

type jsonRewriter struct {
	src  []byte
	pos  int
	lang string
	// out 为改写后的内容，第一次改写时才分配；copied 为 src 中已经抄到 out 的位置
	out    []byte
	copied int
}

func (r *jsonRewriter) space() {
	for r.pos < len(r.src) {
		switch r.src[r.pos] {
		case ' ', '\t', '\n', '\r':
			r.pos++
		default:
			return
		}
	}
}

// value 扫描一个 JSON 值。deep 表示其中的字符串都要翻译；text 表示这个值本身是字符串时要翻译。
func (r *jsonRewriter) value(deep, text bool, depth int) bool {
	r.space()
	if r.pos >= len(r.src) || depth > jsonMaxDepth {
		return false
	}
	switch c := r.src[r.pos]; {
	case c == '{':
		return r.object(deep, depth)
	case c == '[':
		return r.array(deep, depth)
	case c == '"':
		start := r.pos
		plain, han, ok := r.str()
		if !ok {
			return false
		}
		if (deep || text) && han {
			return r.translate(start, r.pos, plain)
		}
		return true
	case c == 't':
		return r.literal("true")
	case c == 'f':
		return r.literal("false")
	case c == 'n':
		return r.literal("null")
	case c == '-' || c >= '0' && c <= '9':
		for r.pos < len(r.src) {
			switch c := r.src[r.pos]; {
			case c >= '0' && c <= '9', c == '-', c == '+', c == '.', c == 'e', c == 'E':
				r.pos++
				continue
			}
			break
		}
		return true
	}
	return false
}

func (r *jsonRewriter) literal(word string) bool {
	if !bytes.HasPrefix(r.src[r.pos:], []byte(word)) {
		return false
	}
	r.pos += len(word)
	return true
}

// str 扫描一个字符串（r.pos 指向开头的引号），结束后 r.pos 指向结尾引号之后。
// plain 表示没有转义，引号之间的字节就是字符串内容；han 表示其中可能有汉字。
func (r *jsonRewriter) str() (plain, han, ok bool) {
	plain = true
	for i := r.pos + 1; i < len(r.src); i++ {
		switch c := r.src[i]; {
		case c == '"':
			r.pos = i + 1
			return plain, han, true
		case c == '\\':
			plain = false
			i++
			if i < len(r.src) && r.src[i] == 'u' {
				// \uXXXX 转义可能是汉字，交给解码后再判断
				han = true
			}
		case c < 0x20:
			return false, false, false
		case c >= 0xe4 && c <= 0xe9:
			han = true
		}
	}
	return false, false, false
}

// translate 翻译 src[start:end] 这个字符串（含两端引号），译文与原文不同时写入 out。
func (r *jsonRewriter) translate(start, end int, plain bool) bool {
	var s string
	if plain {
		s = string(r.src[start+1 : end-1])
	} else if json.Unmarshal(r.src[start:end], &s) != nil {
		return false
	}
	tr := Tr(r.lang, s)
	if tr == s {
		return true
	}
	enc, err := json.Marshal(tr)
	if err != nil {
		return false
	}
	if r.out == nil {
		// 第一次改写前确认整段是合法 JSON：扫描时只检查了结构，不合法的内容（如错误的转义）一个字节也不改
		if !json.Valid(r.src) {
			return false
		}
		r.out = make([]byte, 0, len(r.src)+len(r.src)/8+64)
	}
	r.out = append(r.out, r.src[r.copied:start]...)
	r.out = append(r.out, enc...)
	r.copied = end
	return true
}

func (r *jsonRewriter) object(deep bool, depth int) bool {
	r.pos++
	r.space()
	if r.pos < len(r.src) && r.src[r.pos] == '}' {
		r.pos++
		return true
	}
	for {
		r.space()
		if r.pos >= len(r.src) || r.src[r.pos] != '"' {
			return false
		}
		start := r.pos
		plain, _, ok := r.str()
		if !ok {
			return false
		}
		childDeep, childText := deep, false
		if !deep {
			key := r.src[start+1 : r.pos-1]
			if !plain {
				// 键里带转义的情况很少见，解码后再查表
				var k string
				if json.Unmarshal(r.src[start:r.pos], &k) != nil {
					return false
				}
				key = []byte(k)
			}
			childDeep = messageKeys[string(key)]
			childText = !childDeep && stringMessageKeys[string(key)]
		}
		r.space()
		if r.pos >= len(r.src) || r.src[r.pos] != ':' {
			return false
		}
		r.pos++
		if !r.value(childDeep, childText, depth+1) {
			return false
		}
		r.space()
		if r.pos >= len(r.src) {
			return false
		}
		switch r.src[r.pos] {
		case ',':
			r.pos++
		case '}':
			r.pos++
			return true
		default:
			return false
		}
	}
}

func (r *jsonRewriter) array(deep bool, depth int) bool {
	r.pos++
	r.space()
	if r.pos < len(r.src) && r.src[r.pos] == ']' {
		r.pos++
		return true
	}
	for {
		if !r.value(deep, false, depth+1) {
			return false
		}
		r.space()
		if r.pos >= len(r.src) {
			return false
		}
		switch r.src[r.pos] {
		case ',':
			r.pos++
		case ']':
			r.pos++
			return true
		default:
			return false
		}
	}
}
