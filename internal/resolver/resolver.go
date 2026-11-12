// Package resolver 让程序的出站域名解析使用自定义 DNS 服务器，支持普通 DNS 与 DNS over HTTPS。
//
// 服务器写法：
//
//	223.5.5.5              普通 DNS，默认 53 端口
//	223.5.5.5:5353         普通 DNS，指定端口
//	doh@223.5.5.5          DoH，地址为 https://223.5.5.5/dns-query
//	doh@dns.alidns.com/x   DoH，自定义路径
//	https://doh.pub/dns-query
package resolver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Server 是解析后的一个 DNS 服务器。
type Server struct {
	// Addr 为普通 DNS 的 IP:端口；DoH 时为空
	Addr string
	// URL 为 DoH 地址
	URL string
}

// Parse 解析一项服务器配置。
func Parse(s string) (Server, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return Server{}, errors.New("DNS 服务器为空")
	case strings.HasPrefix(strings.ToLower(s), "doh@"):
		host := s[4:]
		if !strings.Contains(host, "/") {
			host += "/dns-query"
		}
		return parseURL("https://" + host)
	case strings.HasPrefix(strings.ToLower(s), "https://"):
		return parseURL(s)
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return Server{Addr: ap.String()}, nil
	}
	if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return Server{Addr: netip.AddrPortFrom(a, 53).String()}, nil
	}
	return Server{}, fmt.Errorf("DNS 服务器 %q 格式错误，应为 IP、IP:端口、doh@主机 或 https:// 地址", s)
}

func parseURL(raw string) (Server, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Server{}, fmt.Errorf("DoH 地址 %q 无效", raw)
	}
	return Server{URL: u.String()}, nil
}

// Validate 校验一组服务器配置。
func Validate(list []string) error {
	for _, s := range list {
		if _, err := Parse(s); err != nil {
			return err
		}
	}
	return nil
}

// installMu 保护对 net.DefaultResolver 的写入与 Current 的读取，避免保存 DNS 设置时与并发的出站检查竞争。
var installMu sync.RWMutex

// Current 返回当前生效的全局解析器，供需要显式解析的代码使用（不要直接读 net.DefaultResolver）。
func Current() *net.Resolver {
	installMu.RLock()
	defer installMu.RUnlock()
	return net.DefaultResolver
}

// Install 设置全局解析器；list 为空时恢复系统默认。无效项被忽略并返回错误。
func Install(list []string) error {
	var (
		servers []Server
		errs    []error
	)
	for _, s := range list {
		srv, err := Parse(s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		servers = append(servers, srv)
	}
	installMu.Lock()
	defer installMu.Unlock()
	if len(servers) == 0 {
		net.DefaultResolver = &net.Resolver{}
		return errors.Join(errs...)
	}
	net.DefaultResolver = New(servers)
	return errors.Join(errs...)
}

// New 创建依次尝试各服务器的解析器。
func New(servers []Server) *net.Resolver {
	ex := &exchanger{servers: servers}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			// Go 按连接是否实现 net.PacketConn 决定报文格式；queryConn 不是 PacketConn，始终使用带长度前缀的流格式
			return &queryConn{ctx: ctx, stream: true, ex: ex}, nil
		},
	}
}

// bootstrap 用系统解析器解析 DoH 服务器自身的域名，避免递归。
var bootstrap = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, Resolver: &net.Resolver{}}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

// exchanger 把一次 DNS 查询依次发往各服务器，第一个成功的结果返回。
type exchanger struct {
	servers []Server
	mu      sync.Mutex
	// preferred 为最近一次成功的服务器下标，下次优先使用
	preferred int
}

func (e *exchanger) exchange(ctx context.Context, msg []byte) ([]byte, error) {
	e.mu.Lock()
	start := e.preferred
	e.mu.Unlock()
	var lastErr error
	for i := range e.servers {
		idx := (start + i) % len(e.servers)
		resp, err := e.query(ctx, e.servers[idx], msg)
		if err == nil {
			e.mu.Lock()
			e.preferred = idx
			e.mu.Unlock()
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

func (e *exchanger) query(ctx context.Context, s Server, msg []byte) ([]byte, error) {
	if s.URL != "" {
		return queryDoH(ctx, s.URL, msg)
	}
	return queryUDP(ctx, s.Addr, msg)
}

func queryDoH(ctx context.Context, endpoint string, msg []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := bootstrap.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH %s 返回 %s", endpoint, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

func queryUDP(ctx context.Context, addr string, msg []byte) ([]byte, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = c.SetDeadline(deadline)
	if _, err := c.Write(msg); err != nil {
		return nil, err
	}
	buf := make([]byte, 64<<10)
	n, err := c.Read(buf)
	if err != nil {
		return nil, err
	}
	// 截断的响应改用 TCP 重新查询
	if n > 3 && buf[2]&0x02 != 0 {
		return queryTCP(ctx, addr, msg)
	}
	return buf[:n], nil
}

func queryTCP(ctx context.Context, addr string, msg []byte) ([]byte, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	frame := binary.BigEndian.AppendUint16(nil, uint16(len(msg)))
	if _, err := c.Write(append(frame, msg...)); err != nil {
		return nil, err
	}
	var l [2]byte
	if _, err := io.ReadFull(c, l[:]); err != nil {
		return nil, err
	}
	resp := make([]byte, binary.BigEndian.Uint16(l[:]))
	_, err = io.ReadFull(c, resp)
	return resp, err
}

// queryConn 是交给 Go 解析器的虚拟连接：Write 收到完整的查询报文后完成一次查询，Read 返回响应。
// stream 为 true 时报文带 2 字节长度前缀。
type queryConn struct {
	ctx    context.Context
	stream bool
	ex     *exchanger

	mu       sync.Mutex
	pending  []byte
	resp     bytes.Buffer
	err      error
	deadline time.Time
}

func (c *queryConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = append(c.pending, b...)
	msg := c.pending
	if c.stream {
		if len(msg) < 2 || len(msg) < 2+int(binary.BigEndian.Uint16(msg)) {
			return len(b), nil
		}
		msg = msg[2 : 2+int(binary.BigEndian.Uint16(msg))]
	}
	c.pending = nil
	ctx := c.ctx
	if !c.deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.deadline)
		defer cancel()
	}
	resp, err := c.ex.exchange(ctx, append([]byte(nil), msg...))
	if err != nil {
		c.err = err
		return len(b), nil
	}
	if c.stream {
		c.resp.Write(binary.BigEndian.AppendUint16(nil, uint16(len(resp))))
	}
	c.resp.Write(resp)
	return len(b), nil
}

func (c *queryConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resp.Len() == 0 {
		if c.err != nil {
			return 0, c.err
		}
		return 0, io.EOF
	}
	return c.resp.Read(b)
}

func (c *queryConn) Close() error                     { return nil }
func (c *queryConn) LocalAddr() net.Addr              { return dummyAddr{} }
func (c *queryConn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (c *queryConn) SetReadDeadline(time.Time) error  { return nil }
func (c *queryConn) SetWriteDeadline(time.Time) error { return nil }
func (c *queryConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadline = t
	c.mu.Unlock()
	return nil
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "gately-dns" }
func (dummyAddr) String() string  { return "gately-dns" }
