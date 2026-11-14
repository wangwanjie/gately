package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckPasswordStrength(t *testing.T) {
	cases := []struct {
		name, user, pass string
		code             string // 为空表示应通过
	}{
		{"过短", "admin", "Ab1!xyz", WeakTooShort},
		{"九位仍过短", "admin", "Ab1!xyz23", WeakTooShort},
		{"十位通过", "admin", "xK9#mPq2vL", ""},
		{"超长", "admin", strings.Repeat("aB3$", 20), WeakTooLong},
		{"含换行", "admin", "abc\ndef-ghi-123", WeakControl},
		{"默认密码", "admin", "gately.cn", WeakTooShort},
		{"含产品名", "admin", "MyGately-2026-xyz", WeakProduct},
		{"含产品名大写", "admin", "GATELY-Router-77", WeakProduct},
		{"含用户名", "alice", "alice-Router-77x", WeakUsername},
		{"用户名过短不检查", "ab", "ab-Router-77xyz", ""},
		{"常见口令加数字", "admin", "password123", WeakCommon},
		{"常见口令加年份符号", "admin", "Password2024!", WeakCommon},
		{"替换字符变形", "admin", "P@ssw0rd2024", WeakCommon},
		{"常见数字串", "admin", "1234567890", WeakCommon},
		{"拼音口令", "admin", "woaini1314520", WeakCommon},
		{"键盘行", "admin", "qwertyuiopasdf", WeakSequence},
		{"字母表顺序", "admin", "abcdefghijkl", WeakSequence},
		{"数字顺序", "admin", "1234567890123", WeakSequence},
		{"倒序", "admin", "9876543210987", WeakSequence},
		{"全相同", "admin", "aaaaaaaaaaaa", WeakRepeat},
		{"重复片段", "admin", "k9Xk9Xk9Xk9X", WeakRepeat},
		{"字符种类过少", "admin", "kjxhzpqwmvnb", WeakSimple},
		{"十六位纯小写口令短语放宽", "admin", "kjxhzpqwmvnbtrgd", ""},
		{"随机强密码", "admin", "Zm3$kq9Lr!vT", ""},
		{"口令短语", "admin", "correct horse battery staple", ""},
		{"中文口令", "admin", "我的密码很长很安全2026", ""},
		{"常见词仅作子串不误伤", "admin", "dockerfly-77-Zq", ""},
	}
	for _, c := range cases {
		err := CheckPasswordStrength(c.user, c.pass)
		if c.code == "" {
			if err != nil {
				t.Errorf("%s: 应通过，得到 %v", c.name, err)
			}
			continue
		}
		var we *WeakError
		if !errors.As(err, &we) || we.Code != c.code {
			t.Errorf("%s: 期望 %s，得到 %v", c.name, c.code, err)
		}
	}
}

// 默认密码必须被识别为弱口令，否则新装用户用默认密码登录后不会被要求修改。
func TestDefaultPasswordIsWeak(t *testing.T) {
	if CheckPasswordStrength("admin", DefaultPassword) == nil {
		t.Fatal("默认密码应判定为弱口令")
	}
}

func TestRepeatedPattern(t *testing.T) {
	for _, s := range []string{"aa", "abab", "abcabc", "121212"} {
		if !isRepeatedPattern([]rune(s)) {
			t.Errorf("%q 应判定为重复", s)
		}
	}
	for _, s := range []string{"a", "ab", "abca", "abcabd"} {
		if isRepeatedPattern([]rune(s)) {
			t.Errorf("%q 不应判定为重复", s)
		}
	}
}

func BenchmarkCheckPasswordStrength(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = CheckPasswordStrength("admin", "Zm3$kq9Lr!vT-xyz")
	}
}
