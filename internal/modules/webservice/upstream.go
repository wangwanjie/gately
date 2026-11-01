package webservice

import (
	"context"
	"crypto/tls"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
)

const (
	// passiveFailLimit 是连续连接失败多少次后暂时摘除后端
	passiveFailLimit = 3
	// passiveCooldown 是被动摘除后允许再次试探的等待时间（未启用主动检查时）
	passiveCooldown = 30 * time.Second
)

// upstream 是反向代理的一个后端。
type upstream struct {
	url    *url.URL
	weight int
	backup bool

	// down 为对外状态；retryAt 非零时表示冷却结束后允许试探（被动摘除）
	down         atomic.Bool
	retryAt      atomic.Int64
	passiveFails atomic.Int32
	conns        atomic.Int64
	// 主动检查的连续计数，仅检查协程访问
	checkFails, checkPasses int
	// stats 为转发到该后端的请求与响应字节统计，站点热更新时同一后端沿用（见 siteHandler.inherit）
	stats *upstreamStats
}

// upstreamStats 是一个后端的转发统计。
type upstreamStats struct {
	requests atomic.Int64
	// bytes 为后端返回的响应体字节数（不含已升级的 WebSocket 连接上的数据）
	bytes atomic.Int64
	rate  rateRing
	// recent 为近期经该后端转发的访客地址，拓扑视图按地址筛选时据此精确到后端
	recent *recentIPs
}

// displayURL 返回用于展示、日志与事件的后端地址，去掉地址中的用户名与密码。
func (u *upstream) displayURL() string {
	if u.url.User == nil {
		return u.url.String()
	}
	c := *u.url
	c.User = nil
	return c.String()
}

// UpstreamStatus 是后端状态快照。
type UpstreamStatus struct {
	URL     string `json:"url"`
	Weight  int    `json:"weight"`
	Backup  bool   `json:"backup"`
	Healthy bool   `json:"healthy"`
	Conns   int64  `json:"conns"`
	// Scope 为后端所属的分流：空为站点后端，location 为分路径后端，user 为按用户分流的后端；Match 为对应的路径前缀或用户列表
	Scope string `json:"scope,omitempty"`
	Match string `json:"match,omitempty"`
	// Requests 为转发到该后端的累计请求数，Bytes 为后端返回的累计响应体字节数
	Requests int64 `json:"requests"`
	Bytes    int64 `json:"bytes"`
}

// 后端所属分流的取值。
const (
	scopeLocation = "location"
	scopeUser     = "user"
)

// pool 管理站点的后端集合、选择策略与健康状态。
type pool struct {
	ups     []*upstream
	policy  string
	rr      atomic.Uint64
	hc      HealthCheck
	site    string
	skipTLS bool
	// proxyProto 为 true 时后端要求 PROXY 头，HTTP 检查改用 TCP
	proxyProto bool
	log        *logbuf.Logger
	started    sync.Once
	// scope、match 为该组后端所属的分流（见 UpstreamStatus），站点后端为空
	scope, match string
}

func newPool(s Site, log *logbuf.Logger) (*pool, error) {
	p := &pool{policy: s.LBPolicy, hc: s.Health, site: siteLabel(s), skipTLS: s.InsecureSkipVerify, proxyProto: s.SendProxyProtocol, log: log}
	for _, t := range s.Targets {
		spec, err := parseUpstream(t)
		if err != nil {
			return nil, err
		}
		p.ups = append(p.ups, &upstream{url: spec.URL, weight: spec.Weight, backup: spec.Backup, stats: &upstreamStats{recent: newRecentIPs()}})
	}
	return p, nil
}

func siteLabel(s Site) string {
	if s.Name != "" {
		return s.Name
	}
	if len(s.Domains) > 0 {
		return s.Domains[0]
	}
	return s.Key
}

// usable 判断后端当前是否可接收请求。
func (u *upstream) usable(now time.Time) bool {
	if !u.down.Load() {
		return true
	}
	r := u.retryAt.Load()
	return r != 0 && now.UnixNano() >= r
}

// candidates 返回可用后端：优先非备用，其次备用，全部不可用时回退到所有非备用后端（宁可尝试也不直接报错）。
func (p *pool) candidates() []*upstream {
	now := time.Now()
	var primary, backup, all []*upstream
	for _, u := range p.ups {
		if !u.backup {
			all = append(all, u)
		}
		if !u.usable(now) {
			continue
		}
		if u.backup {
			backup = append(backup, u)
		} else {
			primary = append(primary, u)
		}
	}
	switch {
	case len(primary) > 0:
		return primary
	case len(backup) > 0:
		return backup
	case len(all) > 0:
		return all
	}
	return p.ups
}

// weightedIndex 把序号 n 映射到按权重展开后的后端。
func weightedIndex(c []*upstream, n uint64) *upstream {
	total := 0
	for _, u := range c {
		total += u.weight
	}
	pos := int(n % uint64(total))
	for _, u := range c {
		if pos < u.weight {
			return u
		}
		pos -= u.weight
	}
	return c[0]
}

// pick 按策略选择后端。ip 用于 ip_hash。
func (p *pool) pick(ip netip.Addr) *upstream {
	c := p.candidates()
	if len(c) == 1 {
		return c[0]
	}
	switch p.policy {
	case LBRandom:
		return weightedIndex(c, rand.Uint64())
	case LBIPHash:
		h := fnv.New64a()
		_, _ = h.Write([]byte(ip.Unmap().String()))
		return weightedIndex(c, h.Sum64())
	case LBLeastConn:
		best := c[0]
		for _, u := range c[1:] {
			// 比较 conns/weight，避免浮点：a/wa < b/wb 等价于 a*wb < b*wa
			if u.conns.Load()*int64(best.weight) < best.conns.Load()*int64(u.weight) {
				best = u
			}
		}
		return best
	}
	return weightedIndex(c, p.rr.Add(1)-1)
}

// setDown 更新对外状态并在变化时记录日志与事件。
func (p *pool) setDown(u *upstream, down bool, reason string) {
	if u.down.Swap(down) == down {
		return
	}
	if down {
		p.log.Warnf("后端 %s 不可用：%s", u.url.Host, reason)
		events.Publish(events.Event{Type: events.UpstreamDown, Source: p.site, Level: events.LevelError,
			Title: "上游不可用：" + u.url.Host, Message: p.site + " 的后端 " + u.displayURL() + " 不可用：" + reason,
			Fields: map[string]string{"upstream": u.displayURL()}})
		return
	}
	u.retryAt.Store(0)
	p.log.Infof("后端 %s 已恢复", u.url.Host)
	events.Publish(events.Event{Type: events.UpstreamUp, Source: p.site, Level: events.LevelInfo,
		Title: "上游已恢复：" + u.url.Host, Message: p.site + " 的后端 " + u.displayURL() + " 已恢复",
		Fields: map[string]string{"upstream": u.displayURL()}})
}

// fail 记录一次代理失败（被动检查）。
func (p *pool) fail(u *upstream, err error) {
	if len(p.ups) < 2 && !p.hc.Enable {
		// 只有一个后端时摘除没有意义
		return
	}
	n := u.passiveFails.Add(1)
	if u.down.Load() {
		// 试探失败，延长冷却
		if !p.hc.Enable {
			u.retryAt.Store(time.Now().Add(passiveCooldown).UnixNano())
		}
		return
	}
	if n >= passiveFailLimit {
		u.passiveFails.Store(0)
		if !p.hc.Enable {
			u.retryAt.Store(time.Now().Add(passiveCooldown).UnixNano())
		}
		p.setDown(u, true, "连续连接失败："+err.Error())
	}
}

// ok 记录一次成功的代理请求。
func (p *pool) ok(u *upstream) {
	u.passiveFails.Store(0)
	// 主动检查开启时，恢复只由检查决定
	if u.down.Load() && !p.hc.Enable {
		p.setDown(u, false, "")
	}
}

// status 返回后端状态快照。
func (p *pool) status() []UpstreamStatus {
	out := make([]UpstreamStatus, 0, len(p.ups))
	for _, u := range p.ups {
		out = append(out, UpstreamStatus{URL: u.displayURL(), Weight: u.weight, Backup: u.backup, Healthy: !u.down.Load(), Conns: u.conns.Load(),
			Scope: p.scope, Match: p.match, Requests: u.stats.requests.Load(), Bytes: u.stats.bytes.Load()})
	}
	return out
}

// startChecks 启动主动健康检查，ctx 取消后结束。重复调用无效。
func (p *pool) startChecks(ctx context.Context) {
	if !p.hc.Enable || len(p.ups) == 0 {
		return
	}
	p.started.Do(func() {
		for _, u := range p.ups {
			go p.checkLoop(ctx, u)
		}
	})
}

func (p *pool) checkLoop(ctx context.Context, u *upstream) {
	interval := time.Duration(p.hc.Interval) * time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		err := p.probe(ctx, u)
		if err == nil {
			u.checkFails = 0
			u.checkPasses++
			if u.checkPasses >= p.hc.Passes {
				p.setDown(u, false, "")
			}
		} else {
			u.checkPasses = 0
			u.checkFails++
			if u.checkFails >= p.hc.Fails {
				p.setDown(u, true, err.Error())
			}
		}
		timer.Reset(interval)
	}
}

// probe 对后端做一次检查。
func (p *pool) probe(ctx context.Context, u *upstream) error {
	timeout := time.Duration(p.hc.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if p.hc.Mode == "tcp" || p.proxyProto {
		host := u.url.Host
		if u.url.Port() == "" {
			port := "80"
			if u.url.Scheme == "https" {
				port = "443"
			}
			host = net.JoinHostPort(u.url.Hostname(), port)
		}
		c, err := (&net.Dialer{}).DialContext(ctx, "tcp", host)
		if err != nil {
			return err
		}
		return c.Close()
	}
	target := *u.url
	target.Path = strings.TrimRight(u.url.Path, "/") + p.hc.Path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Gately-HealthCheck")
	client := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: p.skipTLS}, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &statusError{resp.StatusCode}
	}
	return nil
}

type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("健康检查返回 HTTP %d", e.code) }
