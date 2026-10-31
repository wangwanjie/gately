package protoroute

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/l4proxy"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/netpipe"
)

// StatsSnapshot 是规则统计的快照，用于 API 输出。
type StatsSnapshot struct {
	ActiveConns int64 `json:"activeConns"`
	TotalConns  int64 `json:"totalConns"`
	BytesIn     int64 `json:"bytesIn"`
	BytesOut    int64 `json:"bytesOut"`
	// RouteConns 与规则的 routes 一一对应，为各路由累计连接数
	RouteConns   []int64 `json:"routeConns"`
	DefaultConns int64   `json:"defaultConns"`
	// Rejected 是未命中路由且没有默认目标而被断开的连接数
	Rejected int64 `json:"rejected"`
}

// runtime 是规则运行时可热替换的部分：路由表、默认目标、识别超时、日志名与全局参数。
// 每个新连接开始时读取一次当前 runtime，替换后新连接立即使用新配置，已建立的连接不受影响。
type runtime struct {
	rule     Rule
	settings Settings
	log      *logbuf.Logger
	router   *router
}

func newRuntime(rule Rule, settings Settings, log *logbuf.Logger) *runtime {
	return &runtime{rule: rule, settings: settings, log: log, router: newRouter(rule)}
}

// sniffTimeout 返回规则的识别超时，未设置时使用默认值。
func (rt *runtime) sniffTimeout() time.Duration {
	if rt.rule.SniffTimeout > 0 {
		return time.Duration(rt.rule.SniffTimeout * float64(time.Second))
	}
	return defaultSniffTimeout
}

// runner 负责一条规则的全部监听器。
type runner struct {
	// rule、settings 为启动监听时的配置，决定监听的协议、端口映射与连接上限；运行中只读
	rule     Rule
	settings Settings
	stats    l4proxy.Stats
	rejected atomic.Int64
	cur      atomic.Pointer[runtime]
	srv      l4proxy.Server
	// listenErrors 记录启动失败的监听项，供界面展示
	listenErrors []string
}

func newRunner(rule Rule, settings Settings, log *logbuf.Logger) *runner {
	r := &runner{rule: rule, settings: settings}
	r.cur.Store(newRuntime(rule, settings, log))
	return r
}

// current 返回当前生效的 runtime。
func (r *runner) current() *runtime {
	return r.cur.Load()
}

// snapshot 输出规则统计与各路由的命中数。
func (r *runner) snapshot() StatsSnapshot {
	rt := r.current().router
	snap := StatsSnapshot{
		ActiveConns: r.stats.ActiveConns.Load(),
		TotalConns:  r.stats.TotalConns.Load(),
		BytesIn:     r.stats.BytesIn.Load(),
		BytesOut:    r.stats.BytesOut.Load(),
		RouteConns:  make([]int64, len(rt.targets)),
		Rejected:    r.rejected.Load(),
	}
	for i, t := range rt.targets {
		snap.RouteConns[i] = t.conns.Load()
	}
	if rt.fallback != nil {
		snap.DefaultConns = rt.fallback.conns.Load()
	}
	return snap
}

// effectiveMaxConns 返回规则实际使用的每端口最大连接数（规则未设置时取全局默认值）。
func effectiveMaxConns(rule Rule, settings Settings) int {
	if rule.MaxConnections > 0 {
		return rule.MaxConnections
	}
	return settings.DefaultMaxConnections
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
	return effectiveMaxConns(old, r.settings) == effectiveMaxConns(rule, settings)
}

// update 热替换路由表、默认目标与识别超时，监听器与已建立的连接保持不断。调用前需用 canUpdate 确认。
func (r *runner) update(rule Rule, settings Settings, log *logbuf.Logger) {
	r.cur.Store(newRuntime(rule, settings, log))
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
	maxConns := effectiveMaxConns(r.rule, r.settings)

	okCount := 0
	for _, proto := range r.rule.Protocols {
		for _, pair := range pairs {
			addr := net.JoinHostPort(r.rule.ListenAddress, strconv.Itoa(pair.Listen))
			err := r.srv.ServeTCP(ctx, proto, addr, maxConns, func(format string, args ...any) {
				r.current().log.Warnf(format, args...)
			}, func(conn net.Conn) {
				// 每个连接固定使用建立时的配置，热更新只影响之后的新连接
				r.handle(ctx, r.current(), conn, pair)
			})
			if err != nil {
				msg := fmt.Sprintf("%s %s 监听失败: %v", proto, addr, err)
				r.listenErrors = append(r.listenErrors, msg)
				log.Errorf("%s", msg)
				continue
			}
			okCount++
			log.Infof("%s %s 已启动（协议分流，%d 条路由）", proto, addr, len(r.rule.Routes))
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

// handle 预读客户端最先发送的数据，识别协议（TLS 再取 SNI 与 ALPN，HTTP 再取 Host）选择目标后透传整个连接，不解密、不终止任何协议。
// 与 haproxy 的 inspect-delay + req_ssl_sni 行为一致：无法识别、未命中路由时交给默认目标；
// 超时未收到任何数据的连接交给「等待超时」路由，没有该路由时同样交给默认目标。
func (r *runner) handle(ctx context.Context, rt *runtime, client net.Conn, pair l4proxy.PortPair) {
	defer client.Close()
	r.stats.ActiveConns.Add(1)
	r.stats.TotalConns.Add(1)
	defer r.stats.ActiveConns.Add(-1)

	peeked, res, err := sniffConn(client, rt.sniffTimeout())
	if err != nil {
		// 客户端在识别完成前断开
		return
	}

	t := rt.router.match(res)
	if t == nil {
		r.rejected.Add(1)
		rt.log.Debugf("%s %s 未命中路由且未设置默认目标，已断开", client.RemoteAddr(), describeSniff(res))
		return
	}
	t.conns.Add(1)
	port := t.port
	if t == rt.router.fallback {
		port = pair.Target
	} else if port == 0 {
		port = pair.Listen
	}
	conn, err := t.picker.Dial("tcp", port, time.Duration(rt.settings.DialTimeout)*time.Second)
	if err != nil {
		rt.log.Warnf("%s %s 连接目标失败: %v", client.RemoteAddr(), describeSniff(res), err)
		return
	}
	defer conn.Close()
	if err := l4proxy.WritePrefix(conn, client, t.proxyProtocol, peeked); err != nil {
		rt.log.Warnf("%s 写入目标失败: %v", client.RemoteAddr(), err)
		return
	}
	rt.log.Debugf("%s %s -> %s", client.RemoteAddr(), describeSniff(res), conn.RemoteAddr())

	// 预读的数据已直接写给目标，这里传入原始连接，Linux 上 io.Copy 可走 splice 零拷贝
	in, out := netpipe.Join(ctx, client, conn)
	r.stats.BytesIn.Add(in + int64(len(peeked)))
	r.stats.BytesOut.Add(out)
}

// describeSniff 生成识别结果的日志描述，例如 "tls SNI \"a.example.com\" ALPN [h2]"。
func describeSniff(res sniffResult) string {
	proto := res.Proto
	if proto == "" {
		proto = "unknown"
	}
	switch res.Proto {
	case ProtoTLS:
		return fmt.Sprintf("%s SNI %q ALPN %v", proto, res.Host, res.ALPN)
	case ProtoHTTP, ProtoMinecraft:
		return fmt.Sprintf("%s Host %q", proto, res.Host)
	}
	return proto
}
