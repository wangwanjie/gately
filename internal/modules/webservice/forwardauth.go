package webservice

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ForwardAuth 把站点的认证交给外部服务（Authelia、Authentik、oauth2-proxy 等），
// 相当于 nginx 的 auth_request 或 Traefik / Caddy 的 forward_auth。
type ForwardAuth struct {
	// URL 为认证接口，例如 Authelia 的 http://127.0.0.1:9091/api/authz/auth-request；留空不启用
	URL string `json:"url"`
	// LoginURL 为认证接口返回 401 时跳转的登录地址，可用 {url}（访客原始地址，已编码）与 {method} 占位，
	// 例如 https://auth.example.com/?rd={url}&rm={method}；留空时把认证接口的响应原样返回给访客
	LoginURL string `json:"loginUrl"`
	// LoginFromLocation 为 true 且未填 LoginURL 时，认证接口返回 401 并带有 http(s) 绝对地址的 Location 时，
	// 把访客 302 跳转到该地址（Casdoor 等服务在 401 中给出带 state 的动态登录地址）；默认关闭，401 原样返回
	LoginFromLocation bool `json:"loginFromLocation,omitempty"`
	// CopyHeaders 为认证通过后从认证响应复制到后端请求的头；留空时使用 Remote-User、Remote-Groups、Remote-Email、Remote-Name
	CopyHeaders []string `json:"copyHeaders"`
}

// defaultForwardAuthHeaders 是 Authelia 等服务返回的身份头。
var defaultForwardAuthHeaders = []string{"Remote-User", "Remote-Groups", "Remote-Email", "Remote-Name"}

// forwardAuthTimeout 是单次认证请求的超时。
const forwardAuthTimeout = 10 * time.Second

// maxForwardAuthBody 限制回传给访客的认证响应体大小。
const maxForwardAuthBody = 1 << 20

// forwardAuthClient 由所有站点共用，认证服务通常在本机，长连接复用可省去每次建连。
var forwardAuthClient = &http.Client{
	Timeout: forwardAuthTimeout,
	Transport: &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	},
	// 认证服务返回的跳转交给访客的浏览器处理
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// validate 校验转发认证设置。
func (f *ForwardAuth) validate() error {
	f.URL = strings.TrimSpace(f.URL)
	f.LoginURL = strings.TrimSpace(f.LoginURL)
	f.CopyHeaders = cleanList(f.CopyHeaders)
	if f.URL == "" {
		return nil
	}
	if u, err := url.Parse(f.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("转发认证地址需为完整的 http(s) 地址")
	}
	if f.LoginURL != "" {
		probe := strings.NewReplacer("{url}", "x", "{method}", "GET").Replace(f.LoginURL)
		if u, err := url.Parse(probe); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("登录跳转地址需为完整的 http(s) 地址")
		}
	}
	for _, h := range f.CopyHeaders {
		if strings.ContainsAny(h, " :") {
			return fmt.Errorf("头名称 %q 无效", h)
		}
	}
	return nil
}

// authLoginLocation 从认证接口 401 响应的 Location 中取出可用于浏览器跳转的登录地址。
// 只接受唯一一个、带主机的 http(s) 绝对地址，且不含控制字符与用户信息；相对地址的基准是认证接口自身，
// 浏览器无从得知，javascript:、data: 等其他形式更不能交给浏览器，这些情况都按未开启处理，401 原样返回。
func authLoginLocation(vs []string) (string, bool) {
	if len(vs) != 1 {
		return "", false
	}
	loc := strings.TrimSpace(vs[0])
	if loc == "" || len(loc) > 8192 || strings.ContainsFunc(loc, func(c rune) bool {
		return c < 0x20 || c == 0x7f
	}) {
		return "", false
	}
	u, err := url.Parse(loc)
	if err != nil || u.Host == "" || u.User != nil {
		return "", false
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return "", false
	}
	return loc, true
}

// forwardAuther 执行转发认证。
type forwardAuther struct {
	cfg     ForwardAuth
	headers []string
}

func newForwardAuther(cfg ForwardAuth) *forwardAuther {
	f := &forwardAuther{cfg: cfg, headers: cfg.CopyHeaders}
	if len(f.headers) == 0 {
		f.headers = defaultForwardAuthHeaders
	}
	return f
}

// stripIdentity 删除访客自带的身份头，防止伪造 Remote-User 等绕过后端的权限判断。
func (f *forwardAuther) stripIdentity(r *http.Request) {
	for _, h := range f.headers {
		delHeaderVariants(r.Header, h)
	}
}

// originalURL 返回访客请求的完整地址。
func originalURL(r *http.Request) string {
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

// check 向认证服务发起子请求。通过时把身份头写入 r 并返回 true；否则已向访客写出响应并返回 false。
func (f *forwardAuther) check(w http.ResponseWriter, r *http.Request, h *siteHandler) bool {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, f.cfg.URL, nil)
	if err != nil {
		writeErrorPage(h.errPage, w, r, http.StatusInternalServerError, "认证服务不可用", "无法连接到站点配置的认证服务，请稍后重试。")
		return false
	}
	// 与 nginx auth_request 一样带上访客的原始请求头（Cookie、Authorization 等），但不带请求体
	for k, vs := range r.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Content-Length", "Content-Type", "Connection", "Upgrade", "Te", "Trailer", "Transfer-Encoding", "Keep-Alive", "Proxy-Connection", "Expect":
			continue
		}
		req.Header[k] = vs
	}
	client := clientAddr(r).Addr().String()
	orig := originalURL(r)
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	// 同时提供 nginx（X-Original-*）与 Traefik / Caddy（X-Forwarded-*）两种约定，兼容各认证服务的不同接口
	req.Header.Set("X-Original-Method", r.Method)
	req.Header.Set("X-Original-URL", orig)
	req.Header.Set("X-Forwarded-Method", r.Method)
	req.Header.Set("X-Forwarded-Proto", scheme)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Uri", r.URL.RequestURI())
	req.Header.Set("X-Forwarded-For", client)
	req.Header.Set("X-Real-Ip", client)
	req.Host = ""

	resp, err := forwardAuthClient.Do(req)
	if err != nil {
		if r.Context().Err() == nil {
			h.reqLog(r).Warnf("%s 转发认证失败: %v", r.Host, err)
		}
		writeErrorPage(h.errPage, w, r, http.StatusBadGateway, "认证服务不可用", "无法连接到站点配置的认证服务，请稍后重试。")
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		for _, name := range f.headers {
			if vs := resp.Header.Values(name); len(vs) > 0 {
				r.Header[http.CanonicalHeaderKey(name)] = vs
			}
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxForwardAuthBody))
		return true
	}
	if resp.StatusCode == http.StatusUnauthorized && f.cfg.LoginURL != "" {
		to := strings.NewReplacer("{url}", url.QueryEscape(orig), "{method}", url.QueryEscape(r.Method)).Replace(f.cfg.LoginURL)
		http.Redirect(w, r, to, http.StatusFound)
		return false
	}
	// 静态登录地址优先；未填写且开启了「跟随 401 跳转」时，采用认证接口在 401 中给出的 Location
	if resp.StatusCode == http.StatusUnauthorized && f.cfg.LoginFromLocation {
		if to, ok := authLoginLocation(resp.Header.Values("Location")); ok {
			// 认证服务在未登录响应中设置的 Cookie（如 OAuth state）与缓存策略随跳转一并交给浏览器
			for _, k := range []string{"Set-Cookie", "Cache-Control"} {
				if vs := resp.Header.Values(k); len(vs) > 0 {
					w.Header()[k] = vs
				}
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxForwardAuthBody))
			w.Header().Set("Location", to)
			w.WriteHeader(http.StatusFound)
			return false
		}
	}
	// 其余情况（Authelia forward-auth 接口自身返回的 302、403 等）原样转给访客
	for _, k := range []string{"Location", "Set-Cookie", "Content-Type", "Www-Authenticate", "Cache-Control"} {
		if vs := resp.Header.Values(k); len(vs) > 0 {
			w.Header()[k] = vs
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxForwardAuthBody))
	return false
}
