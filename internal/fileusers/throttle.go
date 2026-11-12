package fileusers

import (
	"net/netip"
	"sync"
	"time"
)

// 文件服务（WebDAV、FTP）面向公网，密码校验没有登录页的限流，攻击者可以无限次猜测密码。
// Throttle 按来源地址限制失败次数：窗口内失败达到上限后，在窗口结束前拒绝该来源的所有登录尝试。

const (
	// ThrottleMaxFailures 为窗口内允许的最多失败次数
	ThrottleMaxFailures = 10
	// ThrottleWindow 为失败次数的统计窗口，也是达到上限后的拒绝时长
	ThrottleWindow = 15 * time.Minute
	// throttleMaxKeys 为记录的来源数量上限，超过时先清理过期项
	throttleMaxKeys = 8192
)

type throttleEntry struct {
	first    time.Time
	n        int
	inflight int
}

// Throttle 是按来源地址的登录失败限流器，零值可用。
type Throttle struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[string]*throttleEntry
}

func (t *Throttle) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// throttleKey 返回限流键：IPv6 按 /64 聚合，避免攻击者在同一网段内轮换地址绕过限制。
func throttleKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.String()
}

// Reserve 在校验密码前调用，为一次尝试占位；失败次数加上进行中的尝试达到上限时返回 false，调用方应直接拒绝。
// 返回 true 后必须调用 Done。
func (t *Throttle) Reserve(ip string) bool {
	key := throttleKey(ip)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clock()
	if t.m == nil {
		t.m = map[string]*throttleEntry{}
	}
	if len(t.m) > throttleMaxKeys {
		for k, e := range t.m {
			if e.inflight == 0 && now.Sub(e.first) > ThrottleWindow {
				delete(t.m, k)
			}
		}
	}
	e := t.m[key]
	if e == nil || (e.inflight == 0 && now.Sub(e.first) > ThrottleWindow) {
		e = &throttleEntry{first: now}
		t.m[key] = e
	}
	if e.n+e.inflight >= ThrottleMaxFailures {
		return false
	}
	e.inflight++
	return true
}

// Done 结束一次 Reserve 占用的尝试并记录结果。成功不清零已有的失败次数，
// 避免攻击者用自己的有效账号交替登录来重置计数。
func (t *Throttle) Done(ip string, ok bool) {
	key := throttleKey(ip)
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.m[key]
	if e == nil {
		return
	}
	if e.inflight > 0 {
		e.inflight--
	}
	if !ok {
		e.n++
	}
}

// ClipName 截断登录时提交的用户名再写入日志：用户名来自未认证的请求，
// 不截断时每次失败的登录都能向日志缓冲写入接近请求头上限的内容。
func ClipName(s string) string {
	const max = 64
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
