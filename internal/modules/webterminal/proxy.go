package webterminal

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/wangwanjie/gately/internal/demo"
	"net"
	"net/http"
	"strconv"
	"time"
)

// 代理类型
const (
	ProxySOCKS5 = "socks5"
	ProxyHTTP   = "http"
)

// Proxy 是 SSH / Telnet 连接使用的出口代理。
type Proxy struct {
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// fillDefaults 为已启用代理中留空（零值）的类型与端口补默认值，不做校验、不会失败；未启用时不改动。
func (p *Proxy) fillDefaults() {
	if !p.Enabled {
		return
	}
	if p.Type == "" {
		p.Type = ProxySOCKS5
	}
	if p.Port == 0 {
		if p.Type == ProxyHTTP {
			p.Port = 8080
		} else {
			p.Port = 1080
		}
	}
}

// validate 校验并补全默认值；未启用时不做任何检查，保留用户已填内容。
func (p *Proxy) validate() error {
	if !p.Enabled {
		return nil
	}
	p.fillDefaults()
	switch p.Type {
	case ProxySOCKS5, ProxyHTTP:
	default:
		return fmt.Errorf("未知的代理类型 %q", p.Type)
	}
	if p.Host == "" {
		return errors.New("请填写代理地址")
	}
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("代理端口无效")
	}
	return nil
}

// dialDirect 是直连拨号，测试可替换。
var dialDirect = func(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	// 演示模式下不执行任何外部命令
	if err := demo.Check(); err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", addr)
}

// dialTarget 按代理配置拨号到目标地址 addr（host:port）。
func dialTarget(ctx context.Context, p Proxy, addr string, timeout time.Duration) (net.Conn, error) {
	if !p.Enabled {
		return dialDirect(ctx, addr, timeout)
	}
	proxyAddr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	nc, err := dialDirect(ctx, proxyAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接代理失败: %w", err)
	}
	// 握手期间受超时与 ctx 取消共同约束，完成后清除期限
	_ = nc.SetDeadline(time.Now().Add(timeout))
	stop := context.AfterFunc(ctx, func() {
		_ = nc.SetDeadline(time.Unix(1, 0))
	})
	defer stop()
	var out net.Conn
	switch p.Type {
	case ProxyHTTP:
		out, err = httpConnect(nc, p, addr)
	default:
		out, err = socks5Connect(nc, p, addr)
	}
	if err != nil {
		_ = nc.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	_ = nc.SetDeadline(time.Time{})
	return out, nil
}

// socks5Connect 完成 SOCKS5 握手（RFC 1928，支持无认证与用户名密码认证），让代理连接到 addr。
func socks5Connect(c net.Conn, p Proxy, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	methods := []byte{0x00}
	if p.Username != "" {
		methods = []byte{0x00, 0x02}
	}
	if _, err := c.Write(append([]byte{0x05, byte(len(methods))}, methods...)); err != nil {
		return nil, err
	}
	reply := make([]byte, 2)
	if _, err := readFull(c, reply); err != nil {
		return nil, fmt.Errorf("SOCKS5 握手失败: %w", err)
	}
	if reply[0] != 0x05 {
		return nil, errors.New("代理不是 SOCKS5 服务")
	}
	switch reply[1] {
	case 0x00:
	case 0x02:
		if len(p.Username) > 255 || len(p.Password) > 255 {
			return nil, errors.New("代理用户名或密码过长")
		}
		req := []byte{0x01, byte(len(p.Username))}
		req = append(req, p.Username...)
		req = append(req, byte(len(p.Password)))
		req = append(req, p.Password...)
		if _, err := c.Write(req); err != nil {
			return nil, err
		}
		if _, err := readFull(c, reply); err != nil {
			return nil, fmt.Errorf("SOCKS5 认证失败: %w", err)
		}
		if reply[1] != 0x00 {
			return nil, errors.New("代理用户名或密码错误")
		}
	default:
		return nil, errors.New("代理要求不支持的认证方式")
	}
	// CONNECT 请求：IP 字面量用地址类型 1/4，其余按域名交给代理解析
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			req = append(req, ip4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return nil, errors.New("目标主机名过长")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	}
	req = append(req, byte(port>>8), byte(port))
	if _, err := c.Write(req); err != nil {
		return nil, err
	}
	head := make([]byte, 4)
	if _, err := readFull(c, head); err != nil {
		return nil, fmt.Errorf("SOCKS5 连接失败: %w", err)
	}
	if head[1] != 0x00 {
		return nil, fmt.Errorf("代理拒绝连接（错误码 %d）", head[1])
	}
	// 丢弃绑定地址
	skip := 0
	switch head[3] {
	case 0x01:
		skip = 4 + 2
	case 0x04:
		skip = 16 + 2
	case 0x03:
		l := make([]byte, 1)
		if _, err := readFull(c, l); err != nil {
			return nil, err
		}
		skip = int(l[0]) + 2
	default:
		return nil, errors.New("代理返回了未知的地址类型")
	}
	if _, err := readFull(c, make([]byte, skip)); err != nil {
		return nil, err
	}
	return c, nil
}

func readFull(c net.Conn, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := c.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// bufConn 让握手时 bufio 多读的数据不丢失。
type bufConn struct {
	net.Conn
	br *bufio.Reader
}

func (b *bufConn) Read(p []byte) (int, error) { return b.br.Read(p) }

// httpConnect 通过 HTTP CONNECT 隧道连接到 addr。
func httpConnect(c net.Conn, p Proxy, addr string) (net.Conn, error) {
	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if p.Username != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(p.Username + ":" + p.Password))
		req += "Proxy-Authorization: Basic " + cred + "\r\n"
	}
	req += "\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		return nil, err
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, fmt.Errorf("读取代理响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("代理拒绝连接: %s", resp.Status)
	}
	if br.Buffered() > 0 {
		return &bufConn{Conn: c, br: br}, nil
	}
	return c, nil
}
