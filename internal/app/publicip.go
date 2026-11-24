package app

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/ipsource"
)

// publicIPResult 是按需查询的本机公网地址。概览在动态域名模块没有获取到地址时展示它；
// 查询会访问公共的公网 IP 查询服务，因此只在管理员主动点击（或运行自检）时进行，结果只保存在内存中。
type publicIPResult struct {
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
	// Errors 按地址族记录查询失败的原因（键为 ipv4 / ipv6）；没有公网 IPv6 很常见，同样记在这里
	Errors    map[string]string `json:"errors,omitempty"`
	CheckedAt time.Time         `json:"checkedAt,omitzero"`
}

// publicIPCache 保存最近一次查询结果；同一时间只进行一次查询，并发的请求等待并共用这次结果。
type publicIPCache struct {
	mu      sync.Mutex
	last    publicIPResult
	running chan struct{}
}

// publicIPMinInterval 内重复点击直接返回上次结果，避免短时间内反复访问查询服务。
const publicIPMinInterval = 10 * time.Second

// publicIPLookup 查询公网地址，测试中替换。
var publicIPLookup = func(ctx context.Context, fam ipsource.Family) (string, error) {
	urls := ipsource.DefaultIPv4URLs
	if fam == ipsource.IPv6 {
		urls = ipsource.DefaultIPv6URLs
	}
	ip, err := ipsource.Get(ctx, ipsource.Source{Enable: true, Type: ipsource.TypeURL, URLs: urls}, fam)
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

func (c *publicIPCache) get() publicIPResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// store 记录一次查询结果（自检查到的地址也记在这里，概览随后即可显示）。
func (c *publicIPCache) store(res publicIPResult) {
	c.mu.Lock()
	c.last = res
	c.mu.Unlock()
}

// refresh 查询并保存公网地址：已有查询在进行时等它完成，刚查过时直接返回上次结果。
func (c *publicIPCache) refresh(ctx context.Context) publicIPResult {
	c.mu.Lock()
	if !c.last.CheckedAt.IsZero() && time.Since(c.last.CheckedAt) < publicIPMinInterval {
		res := c.last
		c.mu.Unlock()
		return res
	}
	if ch := c.running; ch != nil {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return c.get()
	}
	ch := make(chan struct{})
	c.running = ch
	c.mu.Unlock()

	// 查询不跟随请求取消：浏览器刷新页面时，正在进行的查询仍会完成并保存结果
	qctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var (
		wg     sync.WaitGroup
		v4, v6 string
		e4, e6 error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		v4, e4 = publicIPLookup(qctx, ipsource.IPv4)
	}()
	go func() {
		defer wg.Done()
		v6, e6 = publicIPLookup(qctx, ipsource.IPv6)
	}()
	wg.Wait()

	res := newPublicIPResult(v4, v6, e4, e6)
	c.mu.Lock()
	c.last = res
	c.running = nil
	c.mu.Unlock()
	close(ch)
	return res
}

// handlePublicIP 返回最近一次查询到的公网地址，不访问外部服务；从未查询过时各字段为空。
func (a *App) handlePublicIP(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.publicIP.get())
}

// handlePublicIPRefresh 立即查询本机公网地址（访问公共查询服务），只允许管理员登录会话调用。
func (a *App) handlePublicIPRefresh(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.publicIP.refresh(r.Context()))
}

// newPublicIPResult 汇总一次公网地址查询的结果，失败的地址族记下原因。
func newPublicIPResult(v4, v6 string, e4, e6 error) publicIPResult {
	res := publicIPResult{IPv4: v4, IPv6: v6, CheckedAt: time.Now()}
	if e4 != nil || e6 != nil {
		res.Errors = map[string]string{}
		if e4 != nil {
			res.Errors["ipv4"] = e4.Error()
		}
		if e6 != nil {
			res.Errors["ipv6"] = e6.Error()
		}
	}
	return res
}
