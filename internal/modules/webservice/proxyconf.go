package webservice

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 负载均衡策略
const (
	LBRoundRobin = ""
	LBRandom     = "random"
	LBIPHash     = "ip_hash"
	LBLeastConn  = "least_conn"
)

// HealthCheck 是后端健康检查设置。未启用时仍会做被动检查（连续连接失败后暂时摘除）。
type HealthCheck struct {
	Enable bool `json:"enable"`
	// Mode 为 http（请求 Path，状态码 < 400 视为健康）或 tcp（只测试连接）
	Mode string `json:"mode"`
	Path string `json:"path"`
	// Interval / Timeout 单位为秒
	Interval int `json:"interval"`
	Timeout  int `json:"timeout"`
	// Fails 连续失败多少次判为不健康，Passes 连续成功多少次恢复
	Fails  int `json:"fails"`
	Passes int `json:"passes"`
}

// BackendTimeouts 是反向代理连接后端的超时设置，单位为秒，0 表示使用默认值。
type BackendTimeouts struct {
	// Connect 为与后端建立 TCP 连接的超时，默认 10 秒
	Connect int `json:"connect"`
	// TLSHandshake 为与 HTTPS 后端完成 TLS 握手的超时，默认 10 秒
	TLSHandshake int `json:"tlsHandshake"`
	// ResponseHeader 为请求（含请求体）发完后等待后端返回响应头的超时，默认不限；
	// 只限制响应头到达前的等待，不影响之后的响应体传输与 WebSocket、SSE 等长连接
	ResponseHeader int `json:"responseHeader"`
}

// 后端超时的默认值与上限
const (
	defaultConnectTimeout      = 10 * time.Second
	defaultTLSHandshakeTimeout = 10 * time.Second
	maxConnectTimeout          = 300
	maxTLSHandshakeTimeout     = 300
	maxResponseHeaderTimeout   = 86400
)

// validate 校验超时范围。0 为默认值，不做改写，以便日后调整默认值时旧配置自动跟随。
func (t BackendTimeouts) validate() error {
	if t.Connect < 0 || t.Connect > maxConnectTimeout {
		return fmt.Errorf("后端连接超时需在 1 到 %d 秒之间（0 为默认 10 秒）", maxConnectTimeout)
	}
	if t.TLSHandshake < 0 || t.TLSHandshake > maxTLSHandshakeTimeout {
		return fmt.Errorf("后端 TLS 握手超时需在 1 到 %d 秒之间（0 为默认 10 秒）", maxTLSHandshakeTimeout)
	}
	if t.ResponseHeader < 0 || t.ResponseHeader > maxResponseHeaderTimeout {
		return fmt.Errorf("等待后端响应头超时需在 1 到 %d 秒之间（0 为不限）", maxResponseHeaderTimeout)
	}
	return nil
}

// connect 返回生效的建连超时。
func (t BackendTimeouts) connect() time.Duration {
	if t.Connect > 0 {
		return time.Duration(t.Connect) * time.Second
	}
	return defaultConnectTimeout
}

// tlsHandshake 返回生效的 TLS 握手超时。
func (t BackendTimeouts) tlsHandshake() time.Duration {
	if t.TLSHandshake > 0 {
		return time.Duration(t.TLSHandshake) * time.Second
	}
	return defaultTLSHandshakeTimeout
}

// responseHeader 返回生效的等待响应头超时，0 表示不限。
func (t BackendTimeouts) responseHeader() time.Duration {
	return time.Duration(t.ResponseHeader) * time.Second
}

// RateLimit 按客户端 IP 限流。
type RateLimit struct {
	Enable bool `json:"enable"`
	// Rate 为每秒允许的请求数，Burst 为突发容量
	Rate  float64 `json:"rate"`
	Burst int     `json:"burst"`
	// MaxConns 为单个 IP 的并发请求上限，0 表示不限
	MaxConns int `json:"maxConns"`
}

// CacheConfig 是反向代理的响应缓存（内存）。
type CacheConfig struct {
	Enable bool `json:"enable"`
	// TTL 为上游未给出缓存指令时 200 响应的缓存秒数，0 表示只遵循上游的 Cache-Control
	TTL int `json:"ttl"`
	// MaxMB 为缓存总容量，默认 64
	MaxMB int `json:"maxMb"`
}

// Rewrite 是路径重写或重定向规则，按顺序匹配，命中第一条即停止。
type Rewrite struct {
	// Pattern 为匹配请求路径的正则表达式
	Pattern string `json:"pattern"`
	// To 为替换结果，可使用 $1 引用分组；Redirect 时为跳转目标，可用 {host} 代表访客请求的域名
	To       string `json:"to"`
	Redirect bool   `json:"redirect"`
	// Code 为重定向状态码，默认 302
	Code int `json:"code"`
}

// upstreamSpec 是解析后的后端地址：URL 后可跟 weight=N 与 backup。
type upstreamSpec struct {
	URL    *url.URL
	Weight int
	Backup bool
}

// parseUpstream 解析形如 "http://10.0.0.1:8080 weight=3 backup" 的后端。
func parseUpstream(s string) (upstreamSpec, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return upstreamSpec{}, errors.New("后端地址为空")
	}
	u, err := url.Parse(fields[0])
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return upstreamSpec{}, fmt.Errorf("后端地址 %q 需为 http(s)://主机:端口", fields[0])
	}
	spec := upstreamSpec{URL: u, Weight: 1}
	for _, f := range fields[1:] {
		switch {
		case f == "backup":
			spec.Backup = true
		case strings.HasPrefix(f, "weight="):
			w, err := strconv.Atoi(strings.TrimPrefix(f, "weight="))
			if err != nil || w < 1 || w > 100 {
				return upstreamSpec{}, fmt.Errorf("权重 %q 需为 1-100 的整数", f)
			}
			spec.Weight = w
		default:
			return upstreamSpec{}, fmt.Errorf("无法识别的后端参数 %q（支持 weight=N、backup）", f)
		}
	}
	return spec, nil
}

// validateProxyExtras 校验并规范化负载均衡、健康检查、限流、缓存与重写设置。
func (s *Site) validateProxyExtras() error {
	switch s.LBPolicy {
	case LBRoundRobin, LBRandom, LBIPHash, LBLeastConn:
	default:
		return fmt.Errorf("未知的负载均衡策略 %q", s.LBPolicy)
	}
	if err := s.Timeouts.validate(); err != nil {
		return err
	}
	h := &s.Health
	if h.Enable {
		switch h.Mode {
		case "":
			h.Mode = "http"
		case "http", "tcp":
		default:
			return fmt.Errorf("未知的健康检查方式 %q", h.Mode)
		}
		if h.Mode == "http" {
			if h.Path == "" {
				h.Path = "/"
			}
			if !strings.HasPrefix(h.Path, "/") {
				return errors.New("健康检查路径需以 / 开头")
			}
		}
		if h.Interval == 0 {
			h.Interval = 10
		}
		if h.Timeout == 0 {
			h.Timeout = 3
		}
		if h.Fails == 0 {
			h.Fails = 3
		}
		if h.Passes == 0 {
			h.Passes = 2
		}
		if h.Interval < 1 || h.Interval > 3600 || h.Timeout < 1 || h.Timeout > 60 || h.Fails < 1 || h.Fails > 20 || h.Passes < 1 || h.Passes > 20 {
			return errors.New("健康检查参数超出范围（间隔 1-3600 秒、超时 1-60 秒、次数 1-20）")
		}
	}
	rl := &s.RateLimit
	if rl.Enable {
		if rl.Rate <= 0 && rl.MaxConns <= 0 {
			return errors.New("限流需要设置每秒请求数或并发上限")
		}
		if rl.Rate < 0 || rl.MaxConns < 0 || rl.Burst < 0 {
			return errors.New("限流参数不能为负数")
		}
		if rl.Rate > 0 && rl.Burst == 0 {
			rl.Burst = int(rl.Rate) + 1
		}
	}
	c := &s.Cache
	if c.Enable {
		if c.TTL < 0 || c.TTL > 86400*30 {
			return errors.New("缓存时间需在 0 到 30 天之间")
		}
		if c.MaxMB == 0 {
			c.MaxMB = 64
		}
		if c.MaxMB < 1 || c.MaxMB > 4096 {
			return errors.New("缓存容量需在 1 到 4096 MB 之间")
		}
	}
	for i := range s.Rewrites {
		r := &s.Rewrites[i]
		r.Pattern = strings.TrimSpace(r.Pattern)
		if _, err := regexp.Compile(r.Pattern); err != nil || r.Pattern == "" {
			return fmt.Errorf("重写规则 %q 不是有效的正则表达式", r.Pattern)
		}
		if r.To == "" {
			return fmt.Errorf("重写规则 %q 缺少目标", r.Pattern)
		}
		if r.Redirect {
			switch r.Code {
			case 0:
				r.Code = 302
			case 301, 302, 307, 308:
			default:
				return errors.New("重定向状态码只能是 301/302/307/308")
			}
		} else if !strings.HasPrefix(r.To, "/") {
			return fmt.Errorf("重写目标 %q 需以 / 开头", r.To)
		}
	}
	return nil
}
