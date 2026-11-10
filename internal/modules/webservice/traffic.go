package webservice

import (
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---- 限流 ----

// bucket 是单个客户端的令牌桶与并发计数。
type bucket struct {
	tokens float64
	last   time.Time
	conns  int
}

// limiter 按客户端 IP 限制请求速率与并发数。
type limiter struct {
	mu       sync.Mutex
	rate     float64
	burst    float64
	maxConns int
	buckets  map[string]*bucket
	now      func() time.Time
}

func newLimiter(c RateLimit) *limiter {
	return &limiter{rate: c.Rate, burst: float64(c.Burst), maxConns: c.MaxConns, buckets: map[string]*bucket{}, now: time.Now}
}

// acquire 申请处理一个请求。成功时返回 release，调用方必须在请求结束后调用；
// 失败时返回建议的重试等待时间。
func (l *limiter) acquire(key string) (release func(), retry time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.buckets) > 10000 {
		for k, b := range l.buckets {
			if b.conns == 0 && now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	if l.rate > 0 {
		b.tokens += now.Sub(b.last).Seconds() * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
		if b.tokens < 1 {
			return nil, time.Duration((1 - b.tokens) / l.rate * float64(time.Second)), false
		}
	}
	if l.maxConns > 0 && b.conns >= l.maxConns {
		return nil, time.Second, false
	}
	if l.rate > 0 {
		b.tokens--
	}
	b.conns++
	return func() {
		l.mu.Lock()
		b.conns--
		l.mu.Unlock()
	}, 0, true
}

// ---- 重写 ----

type compiledRewrite struct {
	re       *regexp.Regexp
	to       string
	redirect bool
	code     int
}

func compileRewrites(rs []Rewrite) ([]compiledRewrite, error) {
	out := make([]compiledRewrite, 0, len(rs))
	for _, r := range rs {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, err
		}
		out = append(out, compiledRewrite{re: re, to: r.To, redirect: r.Redirect, code: r.Code})
	}
	return out, nil
}

// redirectBeforeAuth 找到第一条匹配的重写规则，是跳转时直接写出跳转并返回 true；
// 是路径重写时不处理，留给认证之后的 applyRewrite，保持“命中第一条即停止”的顺序语义。
func (h *siteHandler) redirectBeforeAuth(w http.ResponseWriter, r *http.Request) bool {
	for _, rw := range h.rewrites {
		if rw.re.MatchString(r.URL.Path) {
			if rw.redirect {
				return h.applyRewrite(w, r)
			}
			return false
		}
	}
	return false
}

// applyRewrite 按顺序匹配重写规则。命中重定向时直接写出响应并返回 true；
// 命中重写时就地修改请求路径。
func (h *siteHandler) applyRewrite(w http.ResponseWriter, r *http.Request) bool {
	for _, rw := range h.rewrites {
		if !rw.re.MatchString(r.URL.Path) {
			continue
		}
		out := rw.re.ReplaceAllString(r.URL.Path, rw.to)
		if rw.redirect {
			// 跳转目标中的 {host} 替换为访客请求的域名，便于多个域名共用一条规则跳到各自的 HTTPS。
			// 与重定向变量同样处理：IPv6 字面地址补回方括号，含域名以外字符的 Host 按非法请求拒绝
			if strings.Contains(out, hostPlaceholder) {
				host, ok := rewriteHost(r.Host)
				if !ok {
					http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
					return true
				}
				out = strings.ReplaceAll(out, hostPlaceholder, host)
			}
			if !strings.Contains(out, "?") && r.URL.RawQuery != "" {
				out += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, out, rw.code)
			return true
		}
		r.URL.Path = out
		r.URL.RawPath = ""
		return false
	}
	return false
}

// rewriteHost 返回可代入跳转地址的访客域名：IPv6 字面地址带方括号，空值或含非法字符时返回 false。
func rewriteHost(hostport string) (string, bool) {
	h := hostOnly(hostport)
	if h == "" {
		return "", false
	}
	if strings.Contains(h, ":") {
		h = "[" + h + "]"
	}
	v, ok, err := safeHost(h)
	return v, ok && err == nil
}

// retryAfterSeconds 把等待时间向上取整为秒。
func retryAfterSeconds(d time.Duration) string {
	s := int((d + time.Second - 1) / time.Second)
	if s < 1 {
		s = 1
	}
	return fmt.Sprint(s)
}

// limitKeyAddr 返回限流键。
func limitKeyAddr(a netip.Addr) string { return limitKey(a.String()) }
