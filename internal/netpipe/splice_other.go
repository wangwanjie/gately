//go:build !linux

package netpipe

// spliceSupported 表示该系统上 TCP 之间的 io.Copy 走 splice 零拷贝；其它系统没有，统一使用池化缓冲区。
const spliceSupported = false
