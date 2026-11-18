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
