// Package trustsrc 登记各模块确认过身份的来源地址（如用正确令牌登录 frps 的 frpc），
// 入侵防护的诱饵端口与扫描检测不统计这些来源。
//
// 家宽出口地址往往由一个局域网里的多台设备共用：其中一台设备的点对点组网程序做 TCP 打洞时，
// 会在一分钟内向服务器的大量未开放端口发起连接，被扫描检测当成端口扫描封禁，
// 同一出口地址上的 frpc 也随之断开。登记在这里的来源只是免于这两种按端口判断的检测，
// 日志规则、访问控制等其他封禁照常生效。
package trustsrc

import (
	"net/netip"
	"slices"
	"sync"
	"time"
)

// Source 是一个可信来源。
type Source struct {
	// Addr 为来源地址
	Addr netip.Addr
	// Port 不为 0 时，只要来源与本机 TCP 端口 Port 之间有已建立的连接就视为可信（如 frpc 到 frps 的控制连接）
	Port int
	// Until 之前无论是否有连接都视为可信，用于覆盖连接刚断开、客户端正在重连的空档
	Until time.Time
}

var (
	mu      sync.Mutex
	entries = map[string][]Source{}
	// wake 在登记内容变化时收到一个信号（容量为 1，多次变化合并为一次），入侵防护据此立即刷新
	wake = make(chan struct{}, 1)
)

// Notify 返回登记内容变化的通知通道。多次变化合并为一个信号，使用方收到后重新读取 All。
func Notify() <-chan struct{} {
	return wake
}

func signal() {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// Set 设置 owner 登记的来源，覆盖之前的登记；列表为空时等同 Clear。无效地址会被忽略。
func Set(owner string, list []Source) {
	next := make([]Source, 0, len(list))
	for _, s := range list {
		if !s.Addr.IsValid() {
			continue
		}
		s.Addr = s.Addr.Unmap()
		next = append(next, s)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(next) == 0 {
		if _, ok := entries[owner]; ok {
			delete(entries, owner)
			signal()
		}
		return
	}
	// 内容没有变化时不通知，避免重复登记引发无谓的防火墙刷新
	if old, ok := entries[owner]; ok && slices.Equal(old, next) {
		return
	}
	entries[owner] = next
	signal()
}

// Clear 删除 owner 的登记。
func Clear(owner string) {
	Set(owner, nil)
}

// All 返回全部登记的来源。
func All() []Source {
	mu.Lock()
	defer mu.Unlock()
	var out []Source
	for _, list := range entries {
		out = append(out, list...)
	}
	return out
}
