// Gately 新增：认证前的资源限制。
//
// 未登录的连接不需要任何凭据就能建立，上游对它们没有截止时间、并发上限，单帧可达 16 MiB，
// 空连接会被永久持有，大量连接还会耗尽文件描述符与内存，连带同一进程中的管理后台与其他服务。
// 这里给认证完成前的连接加上：每帧读超时与总时限、较小的单帧上限、全局与单个来源的并发上限。
// 认证成功后恢复原有的超时与帧大小，大文件传输不受影响。

//go:build linux || darwin

package parent

import (
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

// PreAuthLimits 是认证前的限制，字段为零值时该项不限制。
type PreAuthLimits struct {
	// IdleTimeout 为认证前等待下一帧（含连接后的第一帧）的最长时间
	IdleTimeout time.Duration
	// Deadline 为从连接建立到认证完成的最长时间
	Deadline time.Duration
	// MaxFrame 为认证前单帧的上限；SESSION_SETUP 的安全缓冲区长度字段只有 16 位，正常客户端的帧不超过约 64 KiB
	MaxFrame uint32
	// Slots 为认证前连接的并发计数，nil 表示不限
	Slots *PreAuthSlots
}

// frameLimit 返回认证前读帧使用的上限：不超过整体上限 maxFrame。
func (l *PreAuthLimits) frameLimit(maxFrame uint32) uint32 {
	if l == nil || l.MaxFrame == 0 || l.MaxFrame > maxFrame {
		return maxFrame
	}
	return l.MaxFrame
}

// readDeadline 返回认证前下一次读取的截止时间：每帧超时与总时限取较早者；两者都不限时返回零值（不设截止时间）。
func (l *PreAuthLimits) readDeadline(start, now time.Time) time.Time {
	var d time.Time
	if l.IdleTimeout > 0 {
		d = now.Add(l.IdleTimeout)
	}
	if l.Deadline > 0 {
		end := start.Add(l.Deadline)
		if d.IsZero() || end.Before(d) {
			d = end
		}
	}
	return d
}

// PreAuthSlots 统计尚未完成认证的连接数，限制全局与单个来源的并发数。零值不限制，可并发使用。
type PreAuthSlots struct {
	// MaxTotal 为全部来源合计的上限，MaxPerIP 为单个来源的上限（IPv6 按 /64 合并计数），0 表示不限
	MaxTotal int
	MaxPerIP int

	mu    sync.Mutex
	total int
	perIP map[string]int
	// lastLog 为上一次记录拒绝日志的时间（UnixNano），用于限制日志频率
	lastLog atomic.Int64
}

// slotKey 返回来源的计数键：IPv6 按 /64 聚合，同一网段轮换地址不能绕过单来源上限。
func slotKey(ip string) string {
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

// acquire 为来源 ip 的一条新连接占用名额，达到上限时返回 false。成功后必须调用 release。
func (s *PreAuthSlots) acquire(ip string) bool {
	if s == nil {
		return true
	}
	key := slotKey(ip)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.MaxTotal > 0 && s.total >= s.MaxTotal {
		return false
	}
	if s.MaxPerIP > 0 && s.perIP[key] >= s.MaxPerIP {
		return false
	}
	if s.perIP == nil {
		s.perIP = map[string]int{}
	}
	s.total++
	s.perIP[key]++
	return true
}

// release 归还 acquire 占用的名额。
func (s *PreAuthSlots) release(ip string) {
	if s == nil {
		return
	}
	key := slotKey(ip)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.total > 0 {
		s.total--
	}
	if n := s.perIP[key]; n > 1 {
		s.perIP[key] = n - 1
	} else {
		delete(s.perIP, key)
	}
}

// InUse 返回当前占用的名额总数。
func (s *PreAuthSlots) InUse() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// slotLogInterval 为拒绝日志的最小间隔：持续被拒绝时每个间隔只记录一条，避免日志被刷屏。
const slotLogInterval = 30 * time.Second

// shouldLog 判断这次拒绝是否需要记录日志。
func (s *PreAuthSlots) shouldLog(now time.Time) bool {
	last := s.lastLog.Load()
	if now.UnixNano()-last < int64(slotLogInterval) {
		return false
	}
	return s.lastLog.CompareAndSwap(last, now.UnixNano())
}

// isTimeout 判断错误是否为读写超时。
func isTimeout(err error) bool {
	var ne interface{ Timeout() bool }
	return errors.As(err, &ne) && ne.Timeout()
}
