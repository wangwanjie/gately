package webservice

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"sync/atomic"
	"time"
)

// maxProxyAttempts 是一个请求最多尝试的后端数（含第一次）。只在与后端建连失败时换下一个后端：
// 这时请求还没有发出，POST 等非幂等请求重发也是安全的；连接超时默认 10 秒，限制次数避免访客等待过久。
const maxProxyAttempts = 3

// proxyAttempt 记录一个代理请求当前使用的后端与已尝试过的后端，存放在请求上下文中（ctxKeyUpstream）。
// Rewrite、RoundTrip、ModifyResponse 与 ErrorHandler 都在处理请求的同一个协程中调用，按顺序读写。
type proxyAttempt struct {
	pool   *pool
	client netip.Addr
	// active 为实时连接列表中的登记项，换后端时同步更新，拓扑视图据此高亮到实际使用的后端
	active *activeReq
	// in 为交给反向代理的访客请求，重试时据此重新拼接后端地址
	in    *http.Request
	up    atomic.Pointer[upstream]
	tried []*upstream
}

// attemptFrom 取出请求的后端选择，可为 nil。
func attemptFrom(ctx context.Context) *proxyAttempt {
	a, _ := ctx.Value(ctxKeyUpstream{}).(*proxyAttempt)
	return a
}

// current 返回当前使用的后端。
func (a *proxyAttempt) current() *upstream {
	return a.up.Load()
}

// use 改用 up：并发计数（最少连接策略使用）从上一个后端移到 up，请求数与近期活动 IP 记到 up 上。
func (a *proxyAttempt) use(up *upstream) {
	if prev := a.up.Swap(up); prev != nil {
		prev.conns.Add(-1)
	}
	up.conns.Add(1)
	now := time.Now().Unix()
	up.stats.requests.Add(1)
	up.stats.rate.add(now, 1, 0)
	up.stats.recent.touch(a.client, now)
	if a.active != nil {
		a.active.upstream.Store(up)
	}
	a.tried = append(a.tried, up)
}

// release 在请求结束时释放当前后端的并发计数。
func (a *proxyAttempt) release() {
	if up := a.up.Load(); up != nil {
		up.conns.Add(-1)
	}
}

// next 返回下一个要尝试的后端：先按可用后端（含按策略回退的结果）的顺序，再尝试其余可用的备用后端；
// 已尝试过的跳过，达到次数上限或没有后端可换时返回 nil。
func (a *proxyAttempt) next() *upstream {
	if len(a.tried) >= maxProxyAttempts {
		return nil
	}
	for _, u := range a.pool.candidates() {
		if !slices.Contains(a.tried, u) {
			return u
		}
	}
	now := time.Now()
	for _, u := range a.pool.ups {
		if !slices.Contains(a.tried, u) && u.usable(now) {
			return u
		}
	}
	return nil
}

// dialFailed 判断错误是否发生在与后端建立 TCP 连接时（连接被拒绝、不可达、连接超时），此时请求一个字节都没有发出。
func dialFailed(err error) bool {
	var oe *net.OpError
	return errors.As(err, &oe) && oe.Op == "dial"
}

// retryBody 包装发往后端的请求体：Transport 出错时会关闭请求体，这里忽略关闭以便换后端重发；
// read 记录是否读过，读过的请求体无法重发。访客的请求体在请求结束时由 net/http 关闭。
type retryBody struct {
	io.ReadCloser
	read atomic.Bool
}

func (b *retryBody) Read(p []byte) (int, error) {
	b.read.Store(true)
	return b.ReadCloser.Read(p)
}

func (b *retryBody) Close() error {
	return nil
}

// shallowRequest 浅拷贝请求，RoundTrip 不能修改调用方传入的请求。
func shallowRequest(r *http.Request) *http.Request {
	r2 := new(http.Request)
	*r2 = *r
	return r2
}
