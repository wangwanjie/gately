package fileusers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 日志中的用户名被截断，短用户名与多字节字符保持完整。
func TestClipName(t *testing.T) {
	if got := ClipName("alice"); got != "alice" {
		t.Fatalf("短用户名不应改变: %q", got)
	}
	long := strings.Repeat("用", 1000)
	got := ClipName(long)
	if utf8.RuneCountInString(got) != 65 || !utf8.ValidString(got) {
		t.Fatalf("应截断为 64 个字符加省略号: %d", utf8.RuneCountInString(got))
	}
	if got := ClipName(strings.Repeat("用", 30)); utf8.RuneCountInString(got) != 30 {
		t.Fatal("64 个字符以内的多字节用户名不应被截断")
	}
}
