// Gately 新增：报文偏移与长度的边界判断。
//
// SMB2、NTLMSSP 报文里的偏移与长度多为 uint32。直接 int(x) 换算在 32 位平台上会把
// 0x80000000 以上的值变成负数，越过 "off+n > len" 一类的检查后用负下标切片，导致 panic。
// 这里的函数全部在 uint64 中完成判断（uint32 相加不会溢出 uint64），确认落在缓冲区内后
// 才转换为 int，因此结果与平台的 int 位宽无关。

// Package bounds 提供与平台字长无关的报文区间边界判断。
package bounds

// Span 判断长度为 size 的缓冲区中，从 off 开始、长度为 n 的区间 [off, off+n) 是否完整落在缓冲区内。
// 成功时返回可直接用于切片的起止下标；off 等于 size 且 n 为 0 视为合法的空区间。
func Span(off, n uint64, size int) (start, end int, ok bool) {
	if size < 0 {
		return 0, 0, false
	}
	limit := uint64(size)
	if off > limit || n > limit-off {
		return 0, 0, false
	}
	return int(off), int(off + n), true
}

// RelSpan 与 Span 相同，但 abs 是从 base 之前某个起点算起的绝对偏移：先减去 base 再判断。
// SMB2 报文体中的偏移从 64 字节报头的开头算起，解析报文体时 base 为报头长度。
// abs 小于 base（指向报文体之前）时返回 false。
func RelSpan(abs, base, n uint64, size int) (start, end int, ok bool) {
	if abs < base {
		return 0, 0, false
	}
	return Span(abs-base, n, size)
}

// Clamp 把 v 限制到 [0, max] 后转换为 int；max 为负数时返回 0。
// 用于客户端声明的缓冲区长度等只作上限使用的数值，避免 32 位平台上转换为负数。
func Clamp(v uint64, max int) int {
	if max <= 0 {
		return 0
	}
	if v > uint64(max) {
		return max
	}
	return int(v)
}
