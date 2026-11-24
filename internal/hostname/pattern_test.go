package hostname

import (
	"strings"
	"testing"
)

func TestNormalizePattern(t *testing.T) {
	for raw, want := range map[string]string{
		" API-*.Example.COM. ":    "api-*.example.com",
		"app-*-dev.example.com":   "app-*-dev.example.com",
		"*api.example.com":        "*api.example.com",
		"app-*.中文.com":            "app-*.xn--fiq228c.com",
		"Audiobook.*":             "audiobook.*",
		"audiobook.*.example.com": "audiobook.*.example.com",
		"a.api-*.example.com":     "a.api-*.example.com",
		"audiobook.*.中文.com":      "audiobook.*.xn--fiq228c.com",
		"中文.*":                    "xn--fiq228c.*",
		"api.ex*":                 "api.ex*",
		"*.example.com":           "*.example.com", "*": "*", "[::1]": "::1", "": "",
	} {
		if got, ok := NormalizePattern(raw); !ok || got != want {
			t.Errorf("NormalizePattern(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{
		"api-*", "api-**.example.com", "api-*.x*.com", "*.*", "a.*.*", "-api*.example.com", "a.-*.com",
		"api*-.example.com", "api-*.1.2.3.4", "192.168.1.*", "10.*.0.1", "a.*.example.com:443", "a..*", "api-*.example.com:443", "api-*.example.com/path",
		"api-*.example.com?x", "api-*.example..com", "中*.example.com", strings.Repeat("x", 63) + "*.example.com",
		"api-*." + strings.Repeat("a.", 125) + "com",
	} {
		if got, ok := NormalizePattern(raw); ok {
			t.Errorf("NormalizePattern(%q) accepted %q", raw, got)
		}
	}
	// 其他模块仍拒绝首段内通配，不改变它们的匹配语义。
	for _, raw := range []string{"api-*.example.com", "audiobook.*", "audiobook.*.example.com"} {
		if _, ok := Normalize(raw, true); ok {
			t.Fatalf("Normalize(%q) must retain its existing wildcard syntax", raw)
		}
	}
}

func TestCapturePattern(t *testing.T) {
	for _, tc := range []struct {
		pattern, host, capture string
		match                  bool
	}{
		{"api-*.example.com", "api-one.example.com", "one", true},
		{"app-*-dev.example.com", "app-one-dev.example.com", "one", true},
		{"app-*-dev.example.com", "app--dev.example.com", "", true},
		{"*api.example.com", "myapi.example.com", "my", true},
		{"api-*.example.com", "api-one.other.example.com", "", false},
		{"api-*.example.com", "api-one.example.com.evil.test", "", false},
		{"api-*.example.com", "other-one.example.com", "", false},
		{"api-*.example.com", "example.com", "", false},
		{"ab*bc.example.com", "abc.example.com", "", false},
		{"api-*.example.com", "api-.example.com", "", false},
		{"*.example.com", "a.b.example.com", "a.b", true},
		{"*.example.com", "example.com", "", false},
		{"*", "any.example.com", "any.example.com", true},
		{"exact.example.com", "exact.example.com", "", true},
		{"exact.example.com", "other.example.com", "", false},
		// 末尾整段 * 匹配一段或多段
		{"audiobook.*", "audiobook.example.com", "example.com", true},
		{"audiobook.*", "audiobook.home.lan", "home.lan", true},
		{"audiobook.*", "audiobook.com", "com", true},
		{"audiobook.*", "audiobook", "", false},
		{"audiobook.*", "xaudiobook.example.com", "", false},
		{"audiobook.*", "api.audiobook.example.com", "", false},
		{"audiobook.*", "audiobook..com", "", false},
		// 中间整段 * 匹配恰好一段
		{"audiobook.*.example.com", "audiobook.home.example.com", "home", true},
		{"audiobook.*.example.com", "audiobook.a.b.example.com", "", false},
		{"audiobook.*.example.com", "audiobook.example.com", "", false},
		{"audiobook.*.example.com", "audiobook..example.com", "", false},
		{"audiobook.*.example.com", "audiobook.-x.example.com", "", false},
		{"audiobook.*.example.com", "audiobook.home.example.com.evil.test", "", false},
		{"audiobook.*.example.com", "x.audiobook.home.example.com", "", false},
		// 非首段内的段内通配
		{"a.api-*.example.com", "a.api-one.example.com", "one", true},
		{"a.api-*.example.com", "a.api-one.x.example.com", "", false},
		{"a.api-*.example.com", "a.api-.example.com", "", false},
		{"api.ex*", "api.example", "ample", true},
		{"api.ex*", "api.example.com", "", false},
	} {
		got, ok := CapturePattern(tc.pattern, tc.host)
		if ok != tc.match || ok && got != tc.capture {
			t.Errorf("CapturePattern(%q, %q) = %q, %v; want %q, %v", tc.pattern, tc.host, got, ok, tc.capture, tc.match)
		}
	}
}
