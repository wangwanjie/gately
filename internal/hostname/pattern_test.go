package hostname

import (
	"strings"
	"testing"
)

func TestNormalizePattern(t *testing.T) {
	for raw, want := range map[string]string{
		" API-*.Example.COM. ":  "api-*.example.com",
		"app-*-dev.example.com": "app-*-dev.example.com",
		"*api.example.com":      "*api.example.com",
		"app-*.中文.com":          "app-*.xn--fiq228c.com",
		"*.example.com":         "*.example.com", "*": "*", "[::1]": "::1", "": "",
	} {
		if got, ok := NormalizePattern(raw); !ok || got != want {
			t.Errorf("NormalizePattern(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{
		"api-*", "api-**.example.com", "api-*.x*.com", "api.*.com", "-api*.example.com",
		"api*-.example.com", "api-*.1.2.3.4", "api-*.example.com:443", "api-*.example.com/path",
		"api-*.example.com?x", "api-*.example..com", "中*.example.com", strings.Repeat("x", 63) + "*.example.com",
		"api-*." + strings.Repeat("a.", 125) + "com",
	} {
		if got, ok := NormalizePattern(raw); ok {
			t.Errorf("NormalizePattern(%q) accepted %q", raw, got)
		}
	}
	// 其他模块仍拒绝首段内通配，不改变它们的匹配语义。
	if _, ok := Normalize("api-*.example.com", true); ok {
		t.Fatal("Normalize must retain its existing wildcard syntax")
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
	} {
		got, ok := CapturePattern(tc.pattern, tc.host)
		if ok != tc.match || ok && got != tc.capture {
			t.Errorf("CapturePattern(%q, %q) = %q, %v; want %q, %v", tc.pattern, tc.host, got, ok, tc.capture, tc.match)
		}
	}
}
