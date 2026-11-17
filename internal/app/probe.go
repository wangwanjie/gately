package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// 后端连通性测试（快速建站向导的「测试连通」）。
// 只允许管理员登录会话调用（只读账号与 API 令牌在 roleGuard / 令牌权限里拒绝），
// 只做 TCP 连接或一次不跟随跳转的 HTTP HEAD，响应里只有可达与否、状态码与失败类别，
// 不返回响应头和响应体，避免被当作读取内网内容的跳板。
const (
	probeDialTimeout  = 3 * time.Second
	probeTotalTimeout = 5 * time.Second
	// probeMaxInflight 限制同时进行的测试数量
	probeMaxInflight = 4
)

// probeSlots 是同时进行的测试的令牌桶
var probeSlots = make(chan struct{}, probeMaxInflight)

type probeRequest struct {
	// Target 为后端地址：http(s)://主机:端口 做一次 HTTP HEAD，主机:端口 只测试 TCP 连接
	Target string `json:"target"`
}

type probeResult struct {
	Reachable bool `json:"reachable"`
	// Status 为 HTTP 状态码，只有 http(s) 地址且收到响应时有值
	Status int `json:"status,omitempty"`
	// Insecure 为 https 后端的证书无法通过校验（自签名等），跳过校验后可以连接
	Insecure bool `json:"insecure,omitempty"`
	// Reason 为不可达的原因类别：refused / timeout / dns / tls / protocol / unreachable / error
	Reason string `json:"reason,omitempty"`
	// ElapsedMs 为耗时（毫秒）
	ElapsedMs int64 `json:"elapsedMs"`
}

// parseProbeTarget 解析测试地址，返回协议（http / https / tcp）与 主机:端口。
// 路径、查询参数与用户信息一律忽略或拒绝：HEAD 请求只发往根路径。
func parseProbeTarget(s string) (scheme, hostport string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", errors.New("请填写后端地址")
	}
	if !strings.Contains(s, "://") {
		host, port, err := net.SplitHostPort(s)
		if err != nil || host == "" {
			return "", "", errors.New("地址格式应为 http(s)://主机:端口 或 主机:端口")
		}
		if err := checkProbePort(port); err != nil {
			return "", "", err
		}
		return "tcp", net.JoinHostPort(host, port), nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", "", errors.New("地址格式应为 http(s)://主机:端口 或 主机:端口")
	}
	scheme = strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", errors.New("只支持 http 与 https 地址")
	}
	if u.User != nil {
		return "", "", errors.New("地址中不能包含用户名或密码")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if scheme == "https" {
			port = "443"
		}
	}
	if err := checkProbePort(port); err != nil {
		return "", "", err
	}
	return scheme, net.JoinHostPort(u.Hostname(), port), nil
}

func checkProbePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("端口应为 1-65535")
	}
	return nil
}

// probeReason 把连接错误归为几类，不返回原始错误文本
func probeReason(err error) string {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var unknownCA x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var recErr tls.RecordHeaderError
	switch {
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "unreachable"
	case errors.As(err, &certErr), errors.As(err, &unknownCA), errors.As(err, &hostErr):
		return "tls"
	case errors.As(err, &recErr):
		// 用 https 访问了明文 HTTP 端口
		return "protocol"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, http.ErrHandlerTimeout):
		return "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	if strings.Contains(err.Error(), "malformed HTTP") || strings.Contains(err.Error(), "server gave HTTP response") {
		return "protocol"
	}
	return "error"
}

// isCertError 判断是否为证书校验失败（可通过跳过校验连接）
func isCertError(err error) bool {
	var certErr *tls.CertificateVerificationError
	var unknownCA x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	return errors.As(err, &certErr) || errors.As(err, &unknownCA) || errors.As(err, &hostErr) || errors.As(err, &invalid)
}

// probeHead 向 scheme://hostport/ 发一次 HEAD，不跟随跳转、不读取响应体，返回状态码
func probeHead(ctx context.Context, scheme, hostport string, insecure bool) (int, error) {
	dialer := &net.Dialer{Timeout: probeDialTimeout}
	tr := &http.Transport{
		// 不经过任何代理，直接连接后端
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec // 只在证书校验失败后用于判断能否跳过校验连接
		TLSHandshakeTimeout: probeDialTimeout,
		DisableKeepAlives:   true,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, scheme+"://"+hostport+"/", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Gately-Probe")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	// 只取状态码，不读取响应体
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// runProbe 执行一次测试
func runProbe(ctx context.Context, scheme, hostport string) probeResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, probeTotalTimeout)
	defer cancel()
	var res probeResult
	if scheme == "tcp" {
		conn, err := (&net.Dialer{Timeout: probeDialTimeout}).DialContext(ctx, "tcp", hostport)
		if err != nil {
			res.Reason = probeReason(err)
		} else {
			_ = conn.Close()
			res.Reachable = true
		}
		res.ElapsedMs = time.Since(start).Milliseconds()
		return res
	}
	status, err := probeHead(ctx, scheme, hostport, false)
	if err != nil && scheme == "https" && isCertError(err) {
		// 自签名证书等：跳过校验再试一次，能连上时提示需要「跳过后端证书校验」
		if st, err2 := probeHead(ctx, scheme, hostport, true); err2 == nil {
			status, err = st, nil
			res.Insecure = true
		}
	}
	if err != nil {
		res.Reason = probeReason(err)
	} else {
		res.Reachable = true
		res.Status = status
	}
	res.ElapsedMs = time.Since(start).Milliseconds()
	return res
}

// handleProbe 测试后端地址是否可达。
func (a *App) handleProbe(w http.ResponseWriter, r *http.Request) {
	var req probeRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	scheme, hostport, err := parseProbeTarget(req.Target)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	select {
	case probeSlots <- struct{}{}:
		defer func() { <-probeSlots }()
	default:
		httpx.Error(w, http.StatusTooManyRequests, "测试请求过多，请稍后再试")
		return
	}
	httpx.OK(w, runProbe(r.Context(), scheme, hostport))
}
