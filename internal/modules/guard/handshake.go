package guard

import (
	"errors"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
)

// 诱饵端口与扫描检测的握手确认。
//
// 内核规则只凭一个 SYN 判断来源，而 SYN 的源地址可以伪造：攻击者伪造管理员、CDN 回源或组网对端的地址发几个 SYN，
// 就能让入侵防护把这些地址封进 prerouting。所以内核规则不再直接封禁，只把诱饵端口的新连接、
// 以及连接未开放端口超过阈值的可疑来源的后续新连接重定向（nat redirect）到 Gately 在随机端口上的监听，
// 由 Gately 完成 TCP 握手后再计入：伪造源地址的 SYN 收不到 SYN-ACK，握手无法完成，也就不会被计入。
// 代价是只发 SYN、不完成握手的扫描（masscan、nmap -sS 等）不再被封禁，它们会看到这些端口是开放的。

// 握手确认的连接类别
const (
	hsTrap = "trap"
	hsScan = "scan"
)

// redirPorts 为两个用户态监听的端口：内核把诱饵端口的新连接重定向到 trap，把可疑来源连接未开放端口的新连接重定向到 scan。
// 按端口区分类别，不依赖读取原目的端口。
type redirPorts struct {
	trap, scan int
}

func (r redirPorts) ok() bool {
	return r.trap > 0 && r.scan > 0
}

// hsListener 是接收重定向连接的监听：每个类别一个随机端口，IPv4 与 IPv6 各一个套接字。
type hsListener struct {
	ports redirPorts
	lns   []net.Listener
	wg    sync.WaitGroup
}

// hsListenTries 为挑选 IPv4、IPv6 都空闲的随机端口的尝试次数
const hsListenTries = 8

// startHandshake 开始监听，onConn 收到每个完成握手的连接：kind 为 hsTrap / hsScan，
// port 为连接原本的目的端口（读不到时为 0），只用于封禁原因。连接接受后立即以 RST 关闭，不读取任何数据。
func startHandshake(onConn func(kind string, src netip.Addr, port int)) (*hsListener, error) {
	h := &hsListener{}
	for _, kind := range []string{hsTrap, hsScan} {
		lns, port, err := listenPair()
		if err != nil {
			h.stop()
			return nil, err
		}
		if kind == hsTrap {
			h.ports.trap = port
		} else {
			h.ports.scan = port
		}
		for _, ln := range lns {
			h.lns = append(h.lns, ln)
			h.wg.Add(1)
			go h.serve(ln, kind, onConn)
		}
	}
	return h, nil
}

// listenPair 在同一个随机端口上监听 IPv4 与 IPv6（IPv6 套接字只收 IPv6，原目的地址按各自的地址族读取）。
// 系统没有 IPv6 时只监听 IPv4。
func listenPair() ([]net.Listener, int, error) {
	var lastErr error
	for range hsListenTries {
		ln4, err := net.Listen("tcp4", "0.0.0.0:0")
		if err != nil {
			return nil, 0, err
		}
		port := ln4.Addr().(*net.TCPAddr).Port
		ln6, err := net.Listen("tcp6", "[::]:"+strconv.Itoa(port))
		if err == nil {
			return []net.Listener{ln4, ln6}, port, nil
		}
		lastErr = err
		ln4.Close()
		if !errors.Is(err, errAddrInUse) {
			// 不是端口冲突（通常是系统禁用了 IPv6）：只监听 IPv4
			ln4, err = net.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(port))
			if err == nil {
				return []net.Listener{ln4}, port, nil
			}
			lastErr = err
		}
	}
	return nil, 0, lastErr
}

func (h *hsListener) serve(ln net.Listener, kind string, onConn func(string, netip.Addr, int)) {
	defer h.wg.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// 文件描述符耗尽等暂时性错误：稍后重试，避免空转
			time.Sleep(50 * time.Millisecond)
			continue
		}
		port := origDstPort(c)
		src, ok := netip.AddrFromSlice(c.RemoteAddr().(*net.TCPAddr).IP)
		if tc, isTCP := c.(*net.TCPConn); isTCP {
			// 立即以 RST 断开，不进入 TIME_WAIT
			_ = tc.SetLinger(0)
		}
		c.Close()
		if ok {
			onConn(kind, src.Unmap(), port)
		}
	}
}

// stop 关闭监听，不等待接受连接的协程退出（它们回调时会获取 firewall 的锁，调用方可能正持有它），见 wait。
func (h *hsListener) stop() {
	for _, ln := range h.lns {
		ln.Close()
	}
}

// wait 等待接受连接的协程退出，调用方不能持有 firewall 的锁。
func (h *hsListener) wait() {
	h.wg.Wait()
}

// scanCount 是一个可疑来源一分钟窗口内完成握手的未开放端口连接。
type scanCount struct {
	start time.Time
	n     int
	ports []int
}

// 握手确认的参数
const (
	// scanWindow 为扫描检测的计数窗口，与阈值「每分钟」一致
	scanWindow = time.Minute
	// scanTrackMax 为同时计数的来源数上限，满了以后先清理过期的窗口，仍然满时新来源暂不计数
	scanTrackMax = 16384
	// hsBatchDelay 为握手命中写入封禁表前的合并时间：扫描往往成批出现，每批只发一条通知
	hsBatchDelay = time.Second
)

// onHandshake 处理一个完成握手的重定向连接：诱饵端口直接计入封禁；扫描检测在一分钟内超过阈值时计入，
// IPv6 严格模式下第一次即计入。白名单（含内网、本机 IPv6 网段）与可信来源（含刚登录管理后台的地址）不处理。
func (f *firewall) onHandshake(kind string, a netip.Addr, port int) {
	a = a.Unmap()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.status.Active || !f.redirOK || a.IsLoopback() || a.IsUnspecified() {
		return
	}
	if prefixesContain(allowPrefixes(f.allowLocked()), a) {
		return
	}
	s := f.effectiveLocked()
	switch kind {
	case hsTrap:
		if !s.Trap.Enable {
			return
		}
		f.countHitLocked(hsTrap)
		f.queueBanLocked(banlist.SourceTrap, a, []int{port})
	case hsScan:
		if !s.Scan.Enable {
			return
		}
		f.countHitLocked(hsScan)
		need := s.Scan.Threshold + 1
		if a.Is6() && s.Scan.IPv6Strict {
			need = 1
		}
		key := banlist.KeyFor(a)
		now := time.Now()
		c := f.scans[key]
		if c == nil || now.Sub(c.start) >= scanWindow {
			if c == nil && len(f.scans) >= scanTrackMax {
				for k, v := range f.scans {
					if now.Sub(v.start) >= scanWindow {
						delete(f.scans, k)
					}
				}
				if len(f.scans) >= scanTrackMax {
					return
				}
			}
			c = &scanCount{start: now}
			if f.scans == nil {
				f.scans = map[string]*scanCount{}
			}
			f.scans[key] = c
		}
		c.n++
		if port > 0 && !slices.Contains(c.ports, port) && len(c.ports) < 64 {
			c.ports = append(c.ports, port)
		}
		if c.n >= need {
			delete(f.scans, key)
			f.queueBanLocked(banlist.SourceScan, a, c.ports)
		}
	}
}

// countHitLocked 累计握手确认的次数（界面上的「诱饵端口触发」「扫描检测触发」）。计入基数，不随规则重写清零。
func (f *firewall) countHitLocked(kind string) {
	f.baseHits[kind]++
	if f.status.Hits == nil {
		f.status.Hits = map[string]uint64{}
	}
	f.status.Hits[kind]++
}

// queueBanLocked 登记一个待封禁的来源，hsBatchDelay 后与同批的其他来源一起写入封禁表。调用方持有 f.mu。
func (f *firewall) queueBanLocked(source string, a netip.Addr, ports []int) {
	if f.pending == nil {
		f.pending = map[string][]netip.Addr{}
		f.pendingPorts = newPortHits()
	}
	if !slices.Contains(f.pending[source], a) {
		f.pending[source] = append(f.pending[source], a)
	}
	for _, p := range ports {
		if p > 0 {
			f.pendingPorts.add(a, p)
		}
	}
	if f.pendingTimer == nil {
		f.pendingTimer = time.AfterFunc(hsBatchDelay, f.flushPending)
	}
}

// flushPending 把待封禁的来源写入封禁表（随后由 flush 写入防火墙的封禁集合）。
func (f *firewall) flushPending() {
	f.mu.Lock()
	pending, ports, importFn := f.pending, f.pendingPorts, f.importFn
	f.pending, f.pendingPorts, f.pendingTimer = nil, nil, nil
	f.mu.Unlock()
	if importFn == nil {
		return
	}
	for _, source := range []string{banlist.SourceTrap, banlist.SourceScan} {
		if addrs := pending[source]; len(addrs) > 0 {
			importFn(source, addrs, ports)
		}
	}
}
