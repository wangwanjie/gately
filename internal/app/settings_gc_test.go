package app

import "testing"

// 可用内存不足 1 GiB 时保持 Go 默认的 100，内存充足或未知时用 200。
func TestDefaultGCPercent(t *testing.T) {
	cases := []struct {
		mem  uint64
		want int
	}{
		{0, 200},
		{256 << 20, 100},
		{1000628 << 10, 100},
		{1 << 30, 200},
		{8 << 30, 200},
	}
	for _, c := range cases {
		if got := defaultGCPercent(c.mem); got != c.want {
			t.Fatalf("内存 %d 字节时应为 %d，得到 %d", c.mem, c.want, got)
		}
	}
}

// 设置为 0 时，环境变量 GOGC 有效则沿用，无效或为空时按内存取默认值。
func TestUnsetGCPercent(t *testing.T) {
	cases := []struct {
		env  string
		mem  uint64
		want int
	}{
		{"", 8 << 30, 200},
		{"", 256 << 20, 100},
		{"50", 8 << 30, 50},
		{" 300 ", 256 << 20, 300},
		{"off", 8 << 30, -1},
		{"OFF", 8 << 30, -1},
		{"abc", 8 << 30, 200},
	}
	for _, c := range cases {
		if got := unsetGCPercent(c.env, c.mem); got != c.want {
			t.Fatalf("GOGC=%q 内存 %d 时应为 %d，得到 %d", c.env, c.mem, c.want, got)
		}
	}
}
