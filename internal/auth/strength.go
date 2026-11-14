package auth

import (
	"strings"
	"unicode"
)

// 密码长度范围：下限抵御短口令爆破；上限 72 字节是 bcrypt 能处理的最大长度，超出部分会被静默截断。
const (
	MinPasswordLen = 10
	MaxPasswordLen = 72
)

// 弱口令原因代码，WeakError.Code 取值；前端据此展示对应提示。
const (
	WeakTooShort = "tooShort"
	WeakTooLong  = "tooLong"
	WeakCommon   = "common"
	WeakUsername = "username"
	WeakProduct  = "product"
	WeakSequence = "sequence"
	WeakRepeat   = "repeat"
	WeakSimple   = "simple"
	WeakControl  = "control"
)

// WeakError 表示密码强度不足，Code 为机器可读的原因，Error() 为面向用户的中文提示（经 i18n 翻译）。
type WeakError struct {
	Code string
	msg  string
}

func (e *WeakError) Error() string {
	return e.msg
}

func weak(code, msg string) error {
	return &WeakError{Code: code, msg: msg}
}

// CheckPasswordStrength 校验密码强度：通过返回 nil，否则返回 *WeakError。
// username 用于拒绝与用户名相同或包含用户名的密码，可为空。
//
// 规则以抵御在线爆破为目标，不要求“必须含大小写数字符号”这类让用户写出 Passw0rd! 的形式规则：
// 长度不足、常见弱口令（含常见变形）、含产品名或用户名、键盘/字母/数字连续、重复片段、字符种类过少都会被拒绝；
// 长度达到 16 位的口令放宽字符种类要求，鼓励使用口令短语。
func CheckPasswordStrength(username, password string) error {
	if strings.ContainsAny(password, "\x00\r\n") {
		return weak(WeakControl, "密码不能包含换行或控制字符")
	}
	runes := []rune(password)
	if len(runes) < MinPasswordLen {
		return weak(WeakTooShort, "密码至少 10 位")
	}
	if len(password) > MaxPasswordLen {
		return weak(WeakTooLong, "密码最长 72 字节")
	}
	lower := strings.ToLower(password)
	if strings.Contains(lower, "gately") {
		return weak(WeakProduct, "密码不能包含 gately 等产品名称")
	}
	if u := strings.ToLower(strings.TrimSpace(username)); len(u) >= 3 && strings.Contains(lower, u) {
		return weak(WeakUsername, "密码不能包含用户名")
	}
	if isCommonPassword(lower) {
		return weak(WeakCommon, "这是常见的弱口令，请换一个更难猜的密码")
	}
	if distinctRunes(runes) < 5 {
		return weak(WeakRepeat, "密码中重复的字符太多")
	}
	if isRepeatedPattern(runes) {
		return weak(WeakRepeat, "密码由重复的片段组成，请换一个更难猜的密码")
	}
	if sequentialRatio(runes) >= 0.7 {
		return weak(WeakSequence, "密码是连续的字符（如 123456、abcdef、qwerty），请换一个更难猜的密码")
	}
	if len(runes) < 16 && charClasses(runes) < 2 {
		return weak(WeakSimple, "密码只含一种字符，请混合字母、数字或符号，或使用 16 位以上的口令短语")
	}
	return nil
}

// distinctRunes 返回不同字符的个数。
func distinctRunes(rs []rune) int {
	seen := map[rune]struct{}{}
	for _, r := range rs {
		seen[unicode.ToLower(r)] = struct{}{}
	}
	return len(seen)
}

// charClasses 返回出现的字符类别数：小写、大写、数字、其他。
func charClasses(rs []rune) int {
	var lower, upper, digit, other bool
	for _, r := range rs {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			other = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			n++
		}
	}
	return n
}

// isRepeatedPattern 判断密码是否由某个短片段重复构成，例如 abcabcabcabc、1212121212。
func isRepeatedPattern(rs []rune) bool {
	n := len(rs)
	for size := 1; size <= n/2; size++ {
		if n%size != 0 {
			continue
		}
		ok := true
		for i := size; i < n; i++ {
			if rs[i] != rs[i-size] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// keyboardRows 是常见的键盘相邻序列，正向与反向都按连续处理。
var keyboardRows = []string{"qwertyuiop", "asdfghjkl", "zxcvbnm", "1234567890", "!@#$%^&*()"}

// sequentialRatio 返回密码中落在“连续序列”里的字符占比：字母表顺序、数字顺序、键盘行，正向或反向。
// 占比高表示密码基本由 12345678、abcdefgh、qwertyuiop 这类序列拼成。
func sequentialRatio(rs []rune) float64 {
	if len(rs) < 2 {
		return 0
	}
	low := []rune(strings.ToLower(string(rs)))
	covered := make([]bool, len(low))
	// 按码点相邻（abc、123）找出长度不小于 3 的递增或递减段
	markRuns := func(step func(a, b rune) bool) {
		start := 0
		for i := 1; i <= len(low); i++ {
			if i < len(low) && step(low[i-1], low[i]) {
				continue
			}
			if i-start >= 3 {
				for j := start; j < i; j++ {
					covered[j] = true
				}
			}
			start = i
		}
	}
	markRuns(func(a, b rune) bool { return b-a == 1 && isAlnum(a) && isAlnum(b) })
	markRuns(func(a, b rune) bool { return a-b == 1 && isAlnum(a) && isAlnum(b) })
	// 键盘行：在行内位置相邻
	for _, row := range keyboardRows {
		rr := []rune(row)
		pos := map[rune]int{}
		for i, r := range rr {
			pos[r] = i
		}
		for _, dir := range []int{1, -1} {
			start := 0
			for i := 1; i <= len(low); i++ {
				if i < len(low) {
					a, aok := pos[low[i-1]]
					b, bok := pos[low[i]]
					if aok && bok && b-a == dir {
						continue
					}
				}
				if i-start >= 3 {
					if _, ok := pos[low[start]]; ok {
						for j := start; j < i; j++ {
							covered[j] = true
						}
					}
				}
				start = i
			}
		}
	}
	n := 0
	for _, c := range covered {
		if c {
			n++
		}
	}
	return float64(n) / float64(len(low))
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// leet 把常见的字符替换还原为字母，识别 p@ssw0rd 这类变形。
var leet = strings.NewReplacer("0", "o", "1", "l", "3", "e", "4", "a", "5", "s", "7", "t", "8", "b", "@", "a", "$", "s", "!", "i", "|", "l", "+", "t")

// isCommonPassword 判断小写后的密码是否为常见弱口令，或是“常见口令 + 少量数字/符号”的变形。
func isCommonPassword(lower string) bool {
	if _, ok := commonPasswords[lower]; ok {
		return true
	}
	// 去掉首尾的数字与符号后再比，识别 password2024!、123password 等
	core := strings.TrimFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) })
	if len([]rune(core)) >= 4 {
		if _, ok := commonPasswords[core]; ok {
			return true
		}
	}
	// 还原常见替换字符后再比，同样识别带数字后缀的变形
	l := leet.Replace(lower)
	if _, ok := commonPasswords[l]; ok {
		return true
	}
	lcore := strings.TrimFunc(l, func(r rune) bool { return !unicode.IsLetter(r) })
	if len([]rune(lcore)) >= 4 {
		if _, ok := commonPasswords[lcore]; ok {
			return true
		}
	}
	// 常见口令作为前缀或后缀，其余部分只有数字与符号（年份、123 之类，按原文判断）。
	// 还原替换字符后的文本与原文逐字节对齐（替换都是单字节 ASCII），可以用同一偏移取原文的剩余部分。
	for _, c := range []string{lower, l} {
		for w := range commonWords {
			if len(lower) <= len(w) {
				continue
			}
			if strings.HasPrefix(c, w) && trivialRest(lower[len(w):]) {
				return true
			}
			if strings.HasSuffix(c, w) && trivialRest(lower[:len(lower)-len(w)]) {
				return true
			}
		}
	}
	return false
}

// trivialRest 判断常见词前后附加的部分是否只是数字与符号（年份、123 之类），不构成额外的随机性。
func trivialRest(s string) bool {
	if len(s) > 8 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
