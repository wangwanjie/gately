package portforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
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
}

// snapshot 输出规则统计。
func (r *runner) snapshot() StatsSnapshot {
	return StatsSnapshot{
		ActiveConns: r.stats.ActiveConns.Load(),
		TotalConns:  r.stats.TotalConns.Load(),
		BytesIn:     r.stats.BytesIn.Load(),
		BytesOut:    r.stats.BytesOut.Load(),
	}
}

// route 是规则运行时可热替换的部分：目标地址、PROXY 协议、日志名与全局参数。
// 每个新连接开始时读取一次当前 route，替换后新连接立即使用新配置，已建立的连接不受影响。
type route struct {
	rule     Rule
	settings Settings
	log      *logbuf.Logger
	picker   *l4proxy.Picker
}

func newRoute(rule Rule, settings Settings, log *logbuf.Logger) *route {
	return &route{
		rule:     rule,
		settings: settings,
		log:      log,
		picker:   l4proxy.NewPicker(rule.TargetAddresses),
	}
}

// runner 负责一条规则的全部监听器。
type runner struct {
	// rule、settings 为启动监听时的配置，决定监听的协议、端口映射与连接上限；运行中只读
	rule     Rule
	settings Settings
	stats    *l4proxy.Stats
	routes   atomic.Pointer[route]
	srv      l4proxy.Server
	// listenErrors 记录启动失败的监听项，供界面展示
	listenErrors []string
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

// effectiveLimits 返回规则实际使用的每端口最大连接数与 UDP 会话超时（规则未设置时取全局默认值）。
func effectiveLimits(rule Rule, settings Settings) (maxConns int, udpTimeout time.Duration) {
	maxConns = rule.MaxConnections
	if maxConns <= 0 {
		maxConns = settings.DefaultMaxConnections
	}
	udpTimeout = time.Duration(rule.UDPSessionTimeout) * time.Second
	if udpTimeout <= 0 {
		udpTimeout = time.Duration(settings.DefaultUDPSessionTimeout) * time.Second
	}
	return maxConns, udpTimeout
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

	maxConns, udpTimeout := effectiveLimits(r.rule, r.settings)

	okCount := 0
	for _, proto := range r.rule.Protocols {
		for _, pair := range pairs {
			addr := net.JoinHostPort(r.rule.ListenAddress, strconv.Itoa(pair.Listen))
			var startErr error
			if strings.HasPrefix(proto, "tcp") {
				targetPort := pair.Target
				startErr = r.srv.ServeTCP(ctx, proto, addr, maxConns, func(format string, args ...any) {
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

func (r *runner) startUDP(ctx context.Context, network, addr string, targetPort int, idle time.Duration) error {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, network, addr)
	if err != nil {
		return err
	}
	r.srv.Track(pc)

	var (
		mu       sync.Mutex
		sessions = map[string]*udpSession{}
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
		buf := make([]byte, 64*1024)
		for {
			n, clientAddr, err := pc.ReadFrom(buf)
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return
				}
				r.current().log.Warnf("UDP 读取失败: %v", err)
				continue
			}
			// 丢弃被封禁地址的数据包（已有会话随空闲超时自然结束）
			if banlist.Default.BannedAddr(clientAddr) {
				continue
			}
			key := clientAddr.String()
			// 新会话使用当前配置的目标与上限，已有会话保持原目标
			rt := r.current()
			mu.Lock()
			sess := sessions[key]
			if sess == nil {
				if len(sessions) >= rt.settings.MaxUDPSessions {
					mu.Unlock()
					rt.log.Warnf("%s UDP 会话数达到上限 %d，丢弃 %s 的数据包", addr, rt.settings.MaxUDPSessions, key)
					continue
				}
				// 目标为域名时拨号需要解析，放到锁外，避免阻塞会话清理
				mu.Unlock()
				conn, err := rt.dialTarget("udp", targetPort)
				if err != nil {
					rt.log.Warnf("%s 建立 UDP 会话失败: %v", key, err)
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
				sessions[key] = sess
				r.stats.ActiveConns.Add(1)
				r.stats.TotalConns.Add(1)
				r.srv.Go(func() {
					r.pumpUDP(pc, clientAddr, sess, idle, func() {
						mu.Lock()
						if sessions[key] == sess {
							delete(sessions, key)
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
func (r *runner) pumpUDP(pc net.PacketConn, client net.Addr, sess *udpSession, idle time.Duration, remove func()) {
	defer r.stats.ActiveConns.Add(-1)
	defer remove()
	defer sess.conn.Close()
	buf := make([]byte, 64*1024)
	for {
		_ = sess.conn.SetReadDeadline(time.Now().Add(idle))
		n, err := sess.conn.Read(buf)
		if err != nil {
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
		if _, err := pc.WriteTo(buf[:n], client); err != nil {
			return
		}
		r.stats.BytesOut.Add(int64(n))
	}
}
