//go:build linux

package netpipe

// spliceSupported 表示该系统上 TCP 之间的 io.Copy 走 splice 零拷贝。
const spliceSupported = true
