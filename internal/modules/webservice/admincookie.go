package webservice

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
)

// 管理后台的 Cookie 不带 Domain、也不区分端口：后台与某个站点主机名相同（同 IP 或同域名的不同端口）时，
// 浏览器访问该站点也会带上它们。会话 Cookie 等同管理员身份（可直接当 Bearer 令牌使用），
// 因此只交给本实例的管理后台（站点反向代理到后台端口的用法），其他后端与转发认证服务一律收不到。

// adminCookies 为管理后台的 Cookie：会话、安全入口标记与第三方登录随机数，名称见 internal/auth 与 internal/app。
var adminCookies = []string{auth.CookieName, "gately_entry", "gately_login_nonce"}

// hasAdminCookie 粗略判断请求是否带有后台 Cookie（按名称前缀），没有时不必解析与判定后端。
func hasAdminCookie(h http.Header) bool {
	for _, v := range h.Values("Cookie") {
		for _, name := range adminCookies {
			if strings.Contains(v, name+"=") {
				return true
			}
		}
	}
	return false
}

// stripAdminCookies 在请求发往后端 up 前去掉后台 Cookie，除非后端就是本实例的管理后台。
func stripAdminCookies(r *http.Request, up *url.URL) {
	if !hasAdminCookie(r.Header) || upstreamIsAdmin(r.Context(), up) {
		return
	}
	stripCookie(r, adminCookies...)
}

// upstreamIsAdmin 判断后端地址是否为本实例的管理后台：主机名解析出的全部地址都必须落在后台的监听上
// （端口相同；后台监听全部地址时为本机地址，监听具体地址时与之相同，见 httpx.IsAdminAddr）。
// 解析失败、后台没有登记监听地址（例如只经飞牛网关套接字提供服务）时一律不算。
func upstreamIsAdmin(ctx context.Context, u *url.URL) bool {
	if u == nil {
		return false
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https", "wss":
			port = "443"
		default:
			port = "80"
		}
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil || p == 0 {
		return false
	}
	ips := resolveUpstreamIPs(ctx, u.Hostname())
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !httpx.IsAdminAddr(ip, uint16(p)) {
			return false
		}
	}
	return true
}

// upstreamIPTTL 为后端主机名解析结果的缓存时间；upstreamIPMax 为缓存条目上限，超过时整体清空。
const (
	upstreamIPTTL     = 30 * time.Second
	upstreamIPMax     = 1024
	upstreamIPTimeout = 3 * time.Second
)

type upstreamIPEntry struct {
	ips []netip.Addr
	exp time.Time
}

var (
	upstreamIPMu    sync.Mutex
	upstreamIPCache = map[string]upstreamIPEntry{}
	// lookupUpstreamIPs 解析后端主机名，测试可替换
	lookupUpstreamIPs = func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
)

// resetUpstreamIPCache 清空解析缓存（测试用）。
func resetUpstreamIPCache() {
	upstreamIPMu.Lock()
	upstreamIPCache = map[string]upstreamIPEntry{}
	upstreamIPMu.Unlock()
}

// resolveUpstreamIPs 返回后端主机的地址：IP 字面量直接返回，主机名解析后缓存一段时间；解析失败返回 nil。
func resolveUpstreamIPs(ctx context.Context, host string) []netip.Addr {
	if host == "" {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}
	}
	host = strings.ToLower(host)
	now := time.Now()
	upstreamIPMu.Lock()
	if e, ok := upstreamIPCache[host]; ok && now.Before(e.exp) {
		upstreamIPMu.Unlock()
		return e.ips
	}
	upstreamIPMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, upstreamIPTimeout)
	defer cancel()
	ips, err := lookupUpstreamIPs(ctx, host)
	if err != nil {
		// 解析失败按非后台处理，不缓存，下次重新解析
		return nil
	}
	upstreamIPMu.Lock()
	if len(upstreamIPCache) >= upstreamIPMax {
		upstreamIPCache = map[string]upstreamIPEntry{}
	}
	upstreamIPCache[host] = upstreamIPEntry{ips: ips, exp: now.Add(upstreamIPTTL)}
	upstreamIPMu.Unlock()
	return ips
}
