package bounds

import (
	"math"
	"testing"
)

// 用例覆盖 32 位平台上 int(uint32) 会变成负数的取值：判断在 uint64 中完成，
// 64 位平台上运行这些用例也能证明逻辑与字长无关。
func TestSpan(t *testing.T) {
	cases := []struct {
		name       string
		off, n     uint64
		size       int
		start, end int
		ok         bool
	}{
		{"整个缓冲区", 0, 10, 10, 0, 10, true},
		{"末尾的空区间", 10, 0, 10, 10, 10, true},
		{"中间区间", 3, 4, 10, 3, 7, true},
		{"越过末尾一字节", 3, 8, 10, 0, 0, false},
		{"起点越过末尾", 11, 0, 10, 0, 0, false},
		{"偏移最高位为 1", 0x80000000, 8, 100, 0, 0, false},
		{"偏移最高位为 1 且长度为 0", 0x80000000, 0, 100, 0, 0, false},
		{"偏移为 uint32 最大值", math.MaxUint32, 1, 100, 0, 0, false},
		{"长度为 uint32 最大值", 4, math.MaxUint32, 100, 0, 0, false},
		{"偏移与长度相加回绕 uint32", 0xFFFFFFF0, 0x20, 100, 0, 0, false},
		{"偏移与长度相加回绕 uint64", math.MaxUint64, 2, 100, 0, 0, false},
		{"长度回绕 uint64", 2, math.MaxUint64, 100, 0, 0, false},
		{"负的缓冲区长度", 0, 0, -1, 0, 0, false},
	}
	for _, c := range cases {
		start, end, ok := Span(c.off, c.n, c.size)
		if ok != c.ok || start != c.start || end != c.end {
			t.Errorf("%s: Span(%#x, %#x, %d) = %d, %d, %v；期望 %d, %d, %v", c.name, c.off, c.n, c.size, start, end, ok, c.start, c.end, c.ok)
		}
	}
}

func TestRelSpan(t *testing.T) {
	const header = 64
	cases := []struct {
		name       string
		abs, n     uint64
		size       int
		start, end int
		ok         bool
	}{
		{"紧跟报头", 64, 8, 100, 0, 8, true},
		{"报文体中间", 100, 8, 100, 36, 44, true},
		{"指向报头内", 60, 8, 100, 0, 0, false},
		// NEGOTIATE 的 NegotiateContextOffset=0x80000040：32 位上 int(x)-64 为负数，曾越过检查后 panic
		{"协商上下文偏移 0x80000040", 0x80000040, 8, 100, 0, 0, false},
		{"偏移 0xFFFFFFFF", 0xFFFFFFFF, 1, 100, 0, 0, false},
	}
	for _, c := range cases {
		start, end, ok := RelSpan(c.abs, header, c.n, c.size)
		if ok != c.ok || start != c.start || end != c.end {
			t.Errorf("%s: RelSpan(%#x, %d, %#x, %d) = %d, %d, %v；期望 %d, %d, %v", c.name, c.abs, header, c.n, c.size, start, end, ok, c.start, c.end, c.ok)
		}
	}
}

func TestClamp(t *testing.T) {
	cases := []struct {
		v    uint64
		max  int
		want int
	}{
		{0, 10, 0},
		{5, 10, 5},
		{10, 10, 10},
		{0x80000000, 10, 10},
		{math.MaxUint32, 1 << 20, 1 << 20},
		{math.MaxUint64, 1 << 20, 1 << 20},
		{5, 0, 0},
		{5, -3, 0},
	}
	for _, c := range cases {
		if got := Clamp(c.v, c.max); got != c.want {
			t.Errorf("Clamp(%#x, %d) = %d；期望 %d", c.v, c.max, got, c.want)
		}
	}
}
