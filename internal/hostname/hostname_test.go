package hostname

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	ok := map[string]string{
		"":                  "",
		"  A.Example.COM. ": "a.example.com",
		".example.com":      "example.com",
		"my_host.lan":       "my_host.lan",
		"localhost":         "localhost",
		"192.168.1.10":      "192.168.1.10",
		"::1":               "::1",
		"[::1]":             "::1",
		"2001:DB8:0:0::1":   "2001:db8::1",
		"中文.com":            "xn--fiq228c.com",
		"Bücher.example":    "xn--bcher-kva.example",
		"xn--fiq228c.com":   "xn--fiq228c.com",
		"*":                 "*",
		"*.":                "*",
		"*.Example.com":     "*.example.com",
		"*.中文.com":          "*.xn--fiq228c.com",
		"a-b.c1":            "a-b.c1",
	}
	for in, want := range ok {
		got, valid := Normalize(in, true)
		if !valid || got != want {
			t.Errorf("Normalize(%q) = %q, %v，期望 %q", in, got, valid, want)
		}
	}
	bad := []string{
		"a.com:8080", "a.com/x", "a.com?x", "a.com#x", "user@a.com", "evil.com/@x", "javascript:alert(1)",
		"a com", "a..com", "-a.com", "a-.com", "a.*.com", "**", "*a.com", "*.1.2.3.4", "[1.2.3.4]",
		"[::1]:8443", "fe80::1%eth0", "[a.com]", "a.com\\b", "%61.com", "a\x00.com", "a\t.com",
		strings.Repeat("a", 64) + ".com", strings.Repeat("a.", 127) + "com",
	}
	for _, in := range bad {
		if got, valid := Normalize(in, true); valid {
			t.Errorf("Normalize(%q) 应判为非法，得到 %q", in, got)
		}
	}
	// 不允许通配时 * 与 *.x 都非法
	for _, in := range []string{"*", "*.a.com"} {
		if _, valid := Normalize(in, false); valid {
			t.Errorf("wildcard=false 时 %q 应非法", in)
		}
	}
}

func BenchmarkNormalize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Normalize("www.example.com", true)
	}
}
