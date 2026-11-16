package portforward

import (
	"context"
	"errors"
	"fmt"
	"github.com/wangwanjie/gately/internal/fwopen"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/l4proxy"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/netpipe"
)

// StatsSnapshot 是统计的快照，用于 API 输出。
type StatsSnapshot struct {
	ActiveConns int64 `json:"activeConns"`
	TotalConns  int64 `json:"totalConns"`
	BytesIn     int64 `json:"bytesIn"`
	BytesOut    int64 `json:"bytesOut"`
	// Denied 为不满足来源 IP 访问控制而被拒绝的次数：TCP 按连接、UDP 按数据包计数
	Denied int64 `json:"denied"`
}

// snapshot 输出规则统计。
func (r *runner) snapshot() StatsSnapshot {
	return StatsSnapshot{
		ActiveConns: r.stats.ActiveConns.Load(),
		TotalConns:  r.stats.TotalConns.Load(),
		BytesIn:     r.stats.BytesIn.Load(),
		BytesOut:    r.stats.BytesOut.Load(),
		Denied:      r.denied.Load(),
	}
}

// route 是规则运行时可热替换的部分：目标地址、PROXY 协议、来源 IP 访问控制、日志名与全局参数。
// 每个新连接开始时读取一次当前 route，替换后新连接立即使用新配置，已建立的连接不受影响。
type route struct {
	rule     Rule
	settings Settings
	log      *logbuf.Logger
	picker   *l4proxy.Picker
	// filter 为编译后的来源 IP 访问控制，nil 表示不限制
	filter *iplist.Filter
}

func newRoute(rule Rule, settings Settings, log *logbuf.Logger) *route {
	return &route{
		rule:     rule,
		settings: settings,
		log:      log,
		picker:   l4proxy.NewPicker(rule.TargetAddresses),
		filter:   rule.Access.Compile(),
	}
}

// runner 负责一条规则的全部监听器。
type runner struct {
	// rule、settings 为启动监听时的配置，决定监听的协议、端口映射与连接上限；运行中只读
	rule     Rule
	settings Settings
	stats    *l4proxy.Stats
	// denied 为不满足来源 IP 访问控制而被拒绝的次数
	denied atomic.Int64
	routes atomic.Pointer[route]
	srv    l4proxy.Server
	// listenErrors 记录启动失败的监听项，供界面展示
	listenErrors []string
	// listening 为监听成功的端口（协议 -> 端口），自动放行防火墙只放行这些端口
	listening map[string][]int
}

func newRunner(rule Rule, settings Settings, log *logbuf.Logger) *runner {
	r := &runner{rule: rule, settings: settings, stats: &l4proxy.Stats{}}
	r.routes.Store(newRoute(rule, settings, log))
	return r
}

// current 返回当前生效的 route。
func (r *runner) current() *route {
	return r.routes.Load()
}

// effectiveLimits 返回规则实际使用的每端口连接上限与 UDP 会话超时（规则未设置时取全局默认值）。
func effectiveLimits(rule Rule, settings Settings) (limits l4proxy.Limits, udpTimeout time.Duration) {
	limits.Max = rule.MaxConnections
	if limits.Max <= 0 {
		limits.Max = settings.DefaultMaxConnections
	}
	limits.PerSource = l4proxy.PerSourceLimit(rule.MaxConnsPerSource, settings.DefaultMaxConnsPerSource, limits.Max)
	udpTimeout = time.Duration(rule.UDPSessionTimeout) * time.Second
	if udpTimeout <= 0 {
		udpTimeout = time.Duration(settings.DefaultUDPSessionTimeout) * time.Second
	}
	return limits, udpTimeout
}

// canUpdate 判断新配置能否在不重新监听的情况下生效：监听协议、地址、端口映射与连接上限都不变，
// 且当前所有端口都在正常监听（有端口监听失败时需要重启以便重试）。
func (r *runner) canUpdate(rule Rule, settings Settings) bool {
	if len(r.listenErrors) > 0 {
		return false
	}
	old := r.rule
	if !slices.Equal(old.Protocols, rule.Protocols) || old.ListenAddress != rule.ListenAddress ||
		old.ListenPorts != rule.ListenPorts || old.TargetPorts != rule.TargetPorts {
		return false
	}
	oldMax, oldUDP := effectiveLimits(old, r.settings)
	newMax, newUDP := effectiveLimits(rule, settings)
	return oldMax == newMax && oldUDP == newUDP
}

// update 热替换目标地址等配置，监听器与已建立的连接保持不断。调用前需用 canUpdate 确认。
func (r *runner) update(rule Rule, settings Settings, log *logbuf.Logger) {
	r.routes.Store(newRoute(rule, settings, log))
	log.Infof("配置已更新，监听未中断")
}

// start 为每个 协议×端口 建立监听。部分端口失败时其余端口照常工作。
func (r *runner) start(parent context.Context) error {
	pairs, err := l4proxy.MapPorts(r.rule.ListenPorts, r.rule.TargetPorts)
	if err != nil {
		return err
	}
	ctx := r.srv.Context(parent)
	log := r.current().log

	limits, udpTimeout := effectiveLimits(r.rule, r.settings)

	okCount := 0
	for _, proto := range r.rule.Protocols {
		for _, pair := range pairs {
			addr := net.JoinHostPort(r.rule.ListenAddress, strconv.Itoa(pair.Listen))
			var startErr error
			if strings.HasPrefix(proto, "tcp") {
				targetPort := pair.Target
				startErr = r.srv.ServeTCP(ctx, proto, addr, limits, func(format string, args ...any) {
					r.current().log.Warnf(format, args...)
				}, func(conn net.Conn) {
					// 每个连接固定使用建立时的配置，热更新只影响之后的新连接
					r.handleTCP(ctx, r.current(), conn, targetPort)
				})
			} else {
				startErr = r.startUDP(ctx, proto, addr, pair.Target, udpTimeout)
			}
			if startErr != nil {
				msg := fmt.Sprintf("%s %s 监听失败: %v", proto, addr, startErr)
				r.listenErrors = append(r.listenErrors, msg)
				log.Errorf("%s", msg)
				continue
			}
			okCount++
			if r.listening == nil {
				r.listening = map[string][]int{}
			}
			r.listening[proto] = append(r.listening[proto], pair.Listen)
			log.Infof("%s %s -> %s:%d 已启动", proto, addr, strings.Join(r.rule.TargetAddresses, "|"), pair.Target)
		}
	}
	if okCount == 0 {
		r.stop()
		return errors.New("所有端口监听失败")
	}
	return nil
}

// stop 关闭全部监听器与连接，并等待协程退出。
func (r *runner) stop() {
	r.srv.Stop()
}

func (rt *route) dialTarget(network string, port int) (net.Conn, error) {
	return rt.picker.Dial(network, port, time.Duration(rt.settings.DialTimeout)*time.Second)
}

func (r *runner) handleTCP(ctx context.Context, rt *route, client net.Conn, targetPort int) {
	defer client.Close()
	// 封禁地址已在 Accept 时拒绝，这里再按名单判断，不命中时不连接目标
	if !rt.filter.Allows(client.RemoteAddr()) {
		r.denied.Add(1)
		rt.log.Debugf("%s 不满足来源 IP 访问控制，已断开", client.RemoteAddr())
		return
	}
	r.stats.ActiveConns.Add(1)
	r.stats.TotalConns.Add(1)
	defer r.stats.ActiveConns.Add(-1)

	target, err := rt.dialTarget("tcp", targetPort)
	if err != nil {
		rt.log.Warnf("%s 连接目标失败: %v", client.RemoteAddr(), err)
		return
	}
	defer target.Close()
	if err := l4proxy.WritePrefix(target, client, rt.rule.ProxyProtocol, nil); err != nil {
		rt.log.Warnf("%s 发送 PROXY 协议头失败: %v", client.RemoteAddr(), err)
		return
	}
	rt.log.Debugf("%s -> %s 已连接", client.RemoteAddr(), target.RemoteAddr())

	in, out := netpipe.Join(ctx, client, target)
	r.stats.BytesIn.Add(in)
	r.stats.BytesOut.Add(out)
}

// udpSession 是一个客户端地址对应的目标连接。
type udpSession struct {
	conn       net.Conn
	lastActive atomic.Int64 // UnixNano
}

// udpBufSize 为 UDP 数据报读缓冲的大小（能容纳任何数据报）
const udpBufSize = 64 * 1024

// udpBufPool 为会话读回包用的缓冲池：会话在等待回包期间不占用缓冲，只在目标有数据可读时借出，转发完立即归还。
// 公网 UDP 端口遇到伪造来源扫描时会建立大量只有一个包的会话，常驻缓冲会让每个会话多占 64 KB 直到空闲超时
var udpBufPool = sync.Pool{New: func() any {
	b := make([]byte, udpBufSize)
	return &b
}}

func (r *runner) startUDP(ctx context.Context, network, addr string, targetPort int, idle time.Duration) error {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, network, addr)
	if err != nil {
		return err
	}
	r.srv.Track(pc)
	uc, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return fmt.Errorf("%s 不是 UDP 监听", addr)
	}

	var (
		mu sync.Mutex
		// 会话键用 netip.AddrPort：可比较、不分配，省掉每个数据包的 String() 与 UDPAddr
		sessions = map[netip.AddrPort]*udpSession{}
	)
	closeAll := func() {
		mu.Lock()
		for k, s := range sessions {
			s.conn.Close()
			delete(sessions, k)
		}
		mu.Unlock()
	}

	r.srv.Go(func() {
		defer closeAll()
		buf := make([]byte, udpBufSize)
		for {
			n, client, err := uc.ReadFromUDPAddrPort(buf)
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return
				}
				r.current().log.Warnf("UDP 读取失败: %v", err)
				continue
			}
			ip := client.Addr().Unmap()
			// 丢弃被封禁地址的数据包（已有会话随空闲超时自然结束）
			if banlist.Default.Banned(ip) {
				continue
			}
			// 新会话使用当前配置的目标与上限，已有会话保持原目标
			rt := r.current()
			// 封禁优先，再按来源 IP 名单判断；名单每个数据包都判断，名单变化后已有会话的上行也立即被拦下。
			// 敲门只在新建会话时检查：与防火墙拦截保持已建立连接一致，授权到期后 WireGuard 等已有会话不被切断
			reason := rt.filter.ReasonIP(ip)
			if reason == iplist.DenyList {
				r.denied.Add(1)
				continue
			}
			mu.Lock()
			sess := sessions[client]
			if sess == nil {
				if reason == iplist.DenyKnock {
					mu.Unlock()
					r.denied.Add(1)
					continue
				}
				if len(sessions) >= rt.settings.MaxUDPSessions {
					mu.Unlock()
					rt.log.Warnf("%s UDP 会话数达到上限 %d，丢弃 %s 的数据包", addr, rt.settings.MaxUDPSessions, client)
					continue
				}
				// 目标为域名时拨号需要解析，放到锁外，避免阻塞会话清理
				mu.Unlock()
				conn, err := rt.dialTarget("udp", targetPort)
				if err != nil {
					rt.log.Warnf("%s 建立 UDP 会话失败: %v", client, err)
					continue
				}
				mu.Lock()
				if ctx.Err() != nil {
					mu.Unlock()
					conn.Close()
					return
				}
				sess = &udpSession{conn: conn}
				sess.lastActive.Store(time.Now().UnixNano())
				sessions[client] = sess
				r.stats.ActiveConns.Add(1)
				r.stats.TotalConns.Add(1)
				r.srv.Go(func() {
					r.pumpUDP(uc, client, sess, idle, func() {
						mu.Lock()
						if sessions[client] == sess {
							delete(sessions, client)
						}
						mu.Unlock()
					})
				})
			}
			mu.Unlock()

			sess.lastActive.Store(time.Now().UnixNano())
			if _, err := sess.conn.Write(buf[:n]); err == nil {
				r.stats.BytesIn.Add(int64(n))
			}
		}
	})
	return nil
}

// pumpUDP 把目标的回包发回客户端，空闲超时后关闭会话。
// 先不带缓冲地等到目标连接可读（见 waitReadable），再从池中借缓冲读取并转发：等待回包的会话只占一个协程，不占读缓冲。
func (r *runner) pumpUDP(pc *net.UDPConn, client netip.AddrPort, sess *udpSession, idle time.Duration, remove func()) {
	defer r.stats.ActiveConns.Add(-1)
	defer remove()
	defer sess.conn.Close()
	var raw syscall.RawConn
	if sc, ok := sess.conn.(syscall.Conn); ok {
		raw, _ = sc.SyscallConn()
	}
	for {
		_ = sess.conn.SetReadDeadline(time.Now().Add(idle))
		var err error
		if raw != nil {
			err = waitReadable(raw)
		}
		n := 0
		var bp *[]byte
		if err == nil {
			bp = udpBufPool.Get().(*[]byte)
			n, err = sess.conn.Read(*bp)
		}
		if err != nil {
			if bp != nil {
				udpBufPool.Put(bp)
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				// 读超时期间客户端仍有发送则继续等待
				last := time.Unix(0, sess.lastActive.Load())
				if time.Since(last) < idle {
					continue
				}
			}
			return
		}
		sess.lastActive.Store(time.Now().UnixNano())
		_, err = pc.WriteToUDPAddrPort((*bp)[:n], client)
		udpBufPool.Put(bp)
		if err != nil {
			return
		}
		r.stats.BytesOut.Add(int64(n))
	}
}

// firewallPorts 返回自动放行防火墙需要放行的端口：只包含监听成功的端口，连续端口合并为区间。
func (r *runner) firewallPorts() []fwopen.Port {
	var out []fwopen.Port
	for _, proto := range r.rule.Protocols {
		out = append(out, fwopen.ForListen(proto, r.rule.ListenAddress, r.listening[proto])...)
	}
	return out
}
