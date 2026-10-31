package stun

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/script"
	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/upnp"
	"github.com/wangwanjie/gately/internal/webhook"
)

// 规则状态取值
const (
	StatusStopped    = "stopped"
	StatusConnecting = "connecting"
	StatusOK         = "ok"
	StatusRetrying   = "retrying"
	StatusFailed     = "failed"
)

// AddrChange 记录一次公网地址变化。
type AddrChange struct {
	Time time.Time `json:"time"`
	Addr string    `json:"addr"`
}

// Stats 是转发统计。
type Stats struct {
	ActiveConns int64 `json:"activeConns"`
	TotalConns  int64 `json:"totalConns"`
	BytesIn     int64 `json:"bytesIn"`
	BytesOut    int64 `json:"bytesOut"`
}

// RuleState 是规则运行状态。
type RuleState struct {
	Status     string       `json:"status"`
	PublicAddr string       `json:"publicAddr,omitempty"`
	LocalAddr  string       `json:"localAddr,omitempty"`
	StunServer string       `json:"stunServer,omitempty"`
	Since      time.Time    `json:"since,omitzero"`
	Error      string       `json:"error,omitempty"`
	Mapping    string       `json:"mapping,omitempty"`
	History    []AddrChange `json:"history,omitempty"`
	Stats      Stats        `json:"stats"`
}

type counters struct {
	active, total, in, out atomic.Int64
}

// runner 管理一条规则的穿透会话，失败后按配置重试。
type runner struct {
	rule     Rule
	settings Settings
	log      *logbuf.Logger
	cancel   context.CancelFunc
	done     chan struct{}
	stats    counters

	mu    sync.Mutex
	state RuleState
	// port 为本次运行确定的本地端口，随机端口在重试间保持不变
	port int
	// lastPublished 用于判断地址是否变化
	lastPublished string
	// serverIdx 记住上次成功的 STUN 服务器，下次优先使用
	serverIdx int
}

func newRunner(rule Rule, settings Settings, log *logbuf.Logger) *runner {
	return &runner{rule: rule, settings: settings, log: log, port: rule.LocalPort, state: RuleState{Status: StatusStopped}}
}

func (r *runner) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.loop(ctx)
}

func (r *runner) stop() {
	if r.cancel != nil {
		r.cancel()
		<-r.done
	}
	// 规则停止后不再提供地址，Web 服务里引用它的重定向会提示暂不可用
	stunaddr.Delete(r.rule.Key)
	r.setStatus(StatusStopped, "")
}

func (r *runner) snapshot() RuleState {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.state
	s.History = append([]AddrChange{}, r.state.History...)
	s.Stats = Stats{
		ActiveConns: r.stats.active.Load(),
		TotalConns:  r.stats.total.Load(),
		BytesIn:     r.stats.in.Load(),
		BytesOut:    r.stats.out.Load(),
	}
	return s
}

func (r *runner) setStatus(status, errMsg string) {
	r.mu.Lock()
	r.state.Status = status
	r.state.Error = errMsg
	if status != StatusOK {
		r.state.PublicAddr = ""
	}
	r.mu.Unlock()
}

func (r *runner) loop(ctx context.Context) {
	defer close(r.done)
	for {
		r.setStatus(StatusConnecting, "")
		var err error
		if r.rule.Protocol == ProtoUDP4 {
			err = r.sessionUDP(ctx)
		} else {
			err = r.sessionTCP(ctx)
		}
		if ctx.Err() != nil {
			return
		}
		msg := "穿透会话结束"
		if err != nil {
			msg = err.Error()
		}
		r.log.Errorf("%s", msg)
		if !r.rule.AutoRetry {
			stunaddr.Delete(r.rule.Key)
			r.setStatus(StatusFailed, msg)
			return
		}
		// 重试期间界面上的公网地址已清空，旧映射多半已失效：同步清掉，引用它的重定向返回 503，
		// 而不是继续跳到可能打不开的旧端口；重新穿透成功后 publish 会再写入
		stunaddr.Delete(r.rule.Key)
		r.setStatus(StatusRetrying, msg)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(r.rule.RetryInterval) * time.Millisecond):
		}
	}
}

func (r *runner) stunServers() []string {
	if r.rule.UseGlobalServers || len(r.rule.StunServers) == 0 {
		out := make([]string, len(r.settings.StunServers))
		for i, s := range r.settings.StunServers {
			out[i] = withDefaultPort(s, "3478")
		}
		return out
	}
	return r.rule.StunServers
}

func (r *runner) keepAliveServers() []string {
	list := r.rule.KeepAliveServers
	if len(list) == 0 {
		list = r.settings.KeepAliveServers
	}
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = withDefaultPort(s, "80")
	}
	return out
}

// orderedServers 从上次成功的服务器开始轮转。
func (r *runner) orderedServers() []string {
	list := r.stunServers()
	if len(list) == 0 {
		return nil
	}
	r.mu.Lock()
	start := r.serverIdx % len(list)
	r.mu.Unlock()
	return append(append([]string{}, list[start:]...), list[:start]...)
}

func (r *runner) rememberServer(server string) {
	for i, s := range r.stunServers() {
		if s == server {
			r.mu.Lock()
			r.serverIdx = i
			r.state.StunServer = server
			r.mu.Unlock()
			return
		}
	}
}

// publish 更新公网地址，地址变化（或配置为每次都通知）时触发 Webhook 与脚本。
func (r *runner) publish(ctx context.Context, addr netip.AddrPort) {
	s := addr.String()
	r.mu.Lock()
	changed := s != r.lastPublished
	r.lastPublished = s
	r.state.Status = StatusOK
	r.state.Error = ""
	r.state.PublicAddr = s
	if changed {
		r.state.Since = time.Now()
		r.state.History = append(r.state.History, AddrChange{Time: time.Now(), Addr: s})
		if len(r.state.History) > 20 {
			r.state.History = r.state.History[len(r.state.History)-20:]
		}
	}
	r.mu.Unlock()
	// 供 Web 服务按规则名引用；重试期间不删除，重新打洞后映射多半不变，不必让引用它的重定向中断
	stunaddr.Set(r.rule.Key, r.rule.Name, addr)
	if changed {
		r.log.Infof("公网地址: %s", s)
		events.Publish(events.Event{Type: events.STUNChanged, Source: r.rule.Name, Level: events.LevelInfo,
			Title: "STUN 公网地址变化：" + r.rule.Name, Message: "当前公网地址 " + s,
			Fields: map[string]string{"ipAddr": s, "ip": addr.Addr().String(), "ruleKey": r.rule.Key}})
	}
	if !changed && r.rule.WebhookOnlyOnChange {
		return
	}
	vars := map[string]string{
		"ruleName": r.rule.Name,
		"time":     time.Now().Format("2006-01-02 15:04:05"),
		"ip":       addr.Addr().String(),
		"port":     strconv.Itoa(int(addr.Port())),
		"ipAddr":   s,
	}
	hook := r.rule.Webhook
	if r.rule.UseGlobalWebhook {
		hook = r.settings.Webhook
	}
	// 通知在后台执行，不阻塞穿透会话
	go func() {
		if hook.Enable {
			hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
			defer cancel()
			if _, err := webhook.Send(hctx, hook, vars); err != nil {
				r.log.Errorf("Webhook 调用失败: %v", err)
			} else {
				r.log.Infof("Webhook 调用成功")
			}
		}
		if r.rule.ScriptEnable {
			r.runScript(vars)
		}
	}()
}

// runScript 执行用户脚本，变量通过环境变量传入（脚本中可用 ${port} 等写法）。
func (r *runner) runScript(vars map[string]string) {
	out, err := script.Run(r.rule.Script, vars)
	text := script.Summarize(out, 2000)
	if err != nil {
		r.log.Errorf("脚本执行失败: %v %s", err, text)
		return
	}
	if text != "" {
		r.log.Infof("脚本输出: %s", text)
	} else {
		r.log.Infof("脚本执行完成")
	}
}

// publicMapping 维护 STUN 观察到的公网端口 → 本机通道端口 的映射。
// 部分路由器出站时不保持源端口（例如通道端口 37701 出站后变成 65453），
// 仅映射通道端口无法让外部访问公网端口，需要按公网端口再建一条映射；公网端口变化时随之更新。
// 网关外层还有一层 NAT（如运营商 CGNAT）时，公网端口属于外层 NAT，不在该网关上，不追加映射。
type publicMapping struct {
	r     *runner
	proto string
	port  int
	unmap func()
	// skipped 为已判定处于多层 NAT 而跳过追加映射的公网地址（IP 与端口），避免每次复查都重新查询网关；
	// 公网 IP 变化（如运营商改发公网地址）时重新判断
	skipped netip.AddrPort
}

func (p *publicMapping) update(ctx context.Context, pub netip.AddrPort) {
	want := int(pub.Port())
	if !p.r.rule.UPnP && !p.r.rule.NATPMP || want == p.r.port {
		p.close()
		p.skipped = netip.AddrPort{}
		return
	}
	if want == p.port || pub == p.skipped {
		return
	}
	p.close()
	p.skipped = netip.AddrPort{}
	gctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	gw, err := p.r.gatewayExternalIP(gctx)
	cancel()
	if err != nil {
		// 查不到网关外网地址时无法判断，按单层 NAT 处理
		p.r.log.Debugf("查询网关外网地址失败，按单层 NAT 处理: %v", err)
	} else if behindOuterNAT(gw, pub.Addr()) {
		p.r.log.Infof("网关外网地址 %s 与公网地址 %s 不同（外层还有 NAT），公网端口 %d 不在该网关上，跳过追加映射", gw, pub.Addr(), want)
		p.skipped = pub
		return
	}
	p.r.log.Infof("公网端口 %d 与通道端口 %d 不同，按公网端口追加映射", want, p.r.port)
	p.port, p.unmap = want, p.r.mapPort(ctx, p.proto, want)
}

func (p *publicMapping) close() {
	if p.unmap != nil {
		p.unmap()
	}
	p.port, p.unmap = 0, nil
}

// behindOuterNAT 判断网关外网地址与 STUN 观察到的公网地址是否不同，不同说明网关外层还有 NAT。
// 网关返回空地址或 0.0.0.0（例如 WAN 未连接）时无法判断，视为单层。
func behindOuterNAT(gateway, public netip.Addr) bool {
	if !gateway.IsValid() || gateway.IsUnspecified() || !public.IsValid() {
		return false
	}
	return gateway.Unmap() != public.Unmap()
}

// natpmpGateway 返回 NAT-PMP 网关地址，未单独填写时沿用 UPnP 网关。
func (r *runner) natpmpGateway() string {
	if r.rule.NATPMPGateway != "" {
		return r.rule.NATPMPGateway
	}
	return r.rule.UPnPGateway
}

// gatewayExternalIP 查询建立映射所用网关自身的外网地址，规则启用 UPnP 时优先用 UPnP，否则用 NAT-PMP。
func (r *runner) gatewayExternalIP(ctx context.Context) (netip.Addr, error) {
	var ip string
	var err error
	switch {
	case r.rule.UPnP:
		var client *upnp.Client
		if r.rule.UPnPControlURL != "" {
			client = upnp.New(r.rule.UPnPControlURL, "")
		} else if client, err = upnp.Discover(ctx, r.rule.UPnPGateway); err != nil {
			return netip.Addr{}, err
		}
		ip, err = client.ExternalIP(ctx)
	case r.rule.NATPMP:
		gw := r.natpmpGateway()
		if gw == "" {
			return netip.Addr{}, errors.New("未填写 NAT-PMP 网关地址")
		}
		ip, err = upnp.NATPMPExternalIP(ctx, gw)
	default:
		return netip.Addr{}, errors.New("未启用 UPnP / NAT-PMP")
	}
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.ParseAddr(strings.TrimSpace(ip))
}

// mapPort 在路由器上建立 外部端口 port → 本机通道端口 的 UPnP / NAT-PMP 映射，返回清理函数。
// 映射按租期定时续约，路由器重启后也能恢复。
func (r *runner) mapPort(ctx context.Context, proto string, port int) func() {
	if !r.rule.UPnP && !r.rule.NATPMP {
		return func() {}
	}
	intPort := r.port
	if r.rule.UPnPInternalPort > 0 {
		intPort = r.rule.UPnPInternalPort
	}
	desc := "Gately STUN " + r.rule.Name
	const lease = 3600
	mctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	// client 为当前使用的网关；mapped 为最近一次映射成功的网关，结束时用它删除映射
	var client, mapped *upnp.Client

	addUPnP := func() error {
		if client == nil {
			dctx, dcancel := context.WithTimeout(mctx, 6*time.Second)
			defer dcancel()
			var err error
			if r.rule.UPnPControlURL != "" {
				client = upnp.New(r.rule.UPnPControlURL, "")
			} else {
				client, err = upnp.Discover(dctx, r.rule.UPnPGateway)
			}
			if err != nil {
				client = nil
				return err
			}
		}
		actx, acancel := context.WithTimeout(mctx, 8*time.Second)
		defer acancel()
		_, err := client.AddPortMapping(actx, proto, port, r.rule.UPnPLocalHost, intPort, desc, lease)
		if err != nil {
			// 网关重启后控制地址可能变化，下次刷新时重新发现
			if r.rule.UPnPControlURL == "" {
				client = nil
			}
			return err
		}
		mapped = client
		return nil
	}
	addNATPMP := func() error {
		gw := r.natpmpGateway()
		if gw == "" {
			return errors.New("未填写 NAT-PMP 网关地址")
		}
		_, _, err := upnp.NATPMPMap(mctx, gw, proto, intPort, port, lease*time.Second)
		return err
	}
	refresh := func(first bool) {
		var msgs []string
		if r.rule.UPnP {
			if err := addUPnP(); err != nil {
				msgs = append(msgs, "UPnP 失败: "+err.Error())
				r.log.Warnf("UPnP 映射 %s %d 失败: %v", proto, port, err)
			} else {
				msgs = append(msgs, fmt.Sprintf("UPnP %s %d→%d", strings.ToUpper(proto), port, intPort))
				if first {
					r.log.Infof("UPnP 已映射 %s 外部端口 %d -> 内部端口 %d", strings.ToUpper(proto), port, intPort)
				}
			}
		}
		if r.rule.NATPMP {
			if err := addNATPMP(); err != nil {
				msgs = append(msgs, "NAT-PMP 失败: "+err.Error())
				r.log.Warnf("NAT-PMP 映射失败: %v", err)
			} else {
				msgs = append(msgs, fmt.Sprintf("NAT-PMP %s %d", strings.ToUpper(proto), port))
			}
		}
		r.mu.Lock()
		r.state.Mapping = strings.Join(msgs, "；")
		r.mu.Unlock()
	}
	refresh(true)
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(lease / 2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-mctx.Done():
				return
			case <-t.C:
				refresh(false)
			}
		}
	}()
	return func() {
		cancel()
		wg.Wait()
		// 会话结束时删除映射，使用独立超时避免阻塞
		dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dcancel()
		if r.rule.UPnP && mapped != nil {
			_ = mapped.DeletePortMapping(dctx, proto, port)
		}
		if r.rule.NATPMP {
			if gw := r.natpmpGateway(); gw != "" {
				_, _, _ = upnp.NATPMPMap(dctx, gw, proto, intPort, 0, 0)
			}
		}
		r.mu.Lock()
		r.state.Mapping = ""
		r.mu.Unlock()
	}
}

// localTCPAddr 构造通道本地地址。
func localTCPAddr(ip string, port int) *net.TCPAddr {
	a := &net.TCPAddr{Port: port}
	if ip != "" {
		a.IP = net.ParseIP(ip)
	}
	return a
}
