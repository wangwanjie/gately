// Package acmehttp 实现 ACME HTTP-01 验证的应答：登记 token 对应的密钥授权，
// 并在 /.well-known/acme-challenge/<token> 上返回。
//
// 应答有两条路径：Web 服务模块在 80 端口监听时直接调用 Serve 应答；
// 80 端口空闲时 Provider 会临时启动一个只应答验证请求的服务，验证结束后关闭。
package acmehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wangwanjie/gately/internal/openports"
)

// PathPrefix 是 HTTP-01 验证请求的路径前缀。
const PathPrefix = "/.well-known/acme-challenge/"

var (
	mu     sync.RWMutex
	tokens = map[string]string{}
)

// Put 登记 token 对应的密钥授权。
func Put(token, keyAuth string) {
	mu.Lock()
	tokens[token] = keyAuth
	mu.Unlock()
}

// Delete 移除登记。
func Delete(token string) {
	mu.Lock()
	delete(tokens, token)
	mu.Unlock()
}

// Serve 处理验证请求：路径匹配且 token 已登记时写出密钥授权并返回 true，其余请求返回 false。
func Serve(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, PathPrefix) {
		return false
	}
	token := strings.TrimPrefix(r.URL.Path, PathPrefix)
	mu.RLock()
	keyAuth, ok := tokens[token]
	mu.RUnlock()
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(keyAuth))
	return true
}

// Provider 实现 lego 的 challenge.Provider（HTTP-01）。
type Provider struct {
	// Port 为临时服务监听的端口，默认 80
	Port int
	// Logf 记录过程，可为 nil
	Logf func(format string, args ...any)

	mu  sync.Mutex
	srv *http.Server
	// ln 为临时服务的监听，关闭时同步关闭，避免下次签发紧接着监听同一端口时报端口已被占用
	ln net.Listener
	// release 撤销临时监听在 openports 中的登记
	release func()
	// active 为尚未清理的 token 数量，归零时关闭临时服务
	active int
}

func (p *Provider) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
	}
}

// Present 登记密钥授权，并确保有服务在监听验证端口。
func (p *Provider) Present(domain, token, keyAuth string) error {
	Put(token, keyAuth)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active++
	if p.srv != nil {
		return nil
	}
	port := p.Port
	if port <= 0 {
		port = 80
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			// 端口已被占用，通常是 Web 服务模块在监听，由它调用 Serve 应答
			p.logf("端口 %d 已被占用，假定由 Web 服务模块应答 HTTP-01 验证（域名 %s）", port, domain)
			return nil
		}
		Delete(token)
		p.active--
		return fmt.Errorf("监听 %d 端口失败: %w（HTTP-01 验证需要 80 端口可达；可在 Web 服务中监听 80 端口，或以有权限的用户运行）", port, err)
	}
	p.srv = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !Serve(w, r) {
				http.NotFound(w, r)
			}
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	srv := p.srv
	p.ln = ln
	// 立即登记为开放端口：诱饵端口范围包含验证端口时，不必等入侵防护下一次轮询才把它移出诱饵集合
	p.release = openports.Hold(ln.Addr())
	go func() { _ = srv.Serve(ln) }()
	p.logf("已在 %d 端口临时监听，用于 HTTP-01 验证", port)
	return nil
}

// CleanUp 移除登记；最后一个 token 清理后关闭临时服务。
func (p *Provider) CleanUp(domain, token, keyAuth string) error {
	Delete(token)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active > 0 {
		p.active--
	}
	if p.active == 0 && p.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = p.srv.Shutdown(ctx)
		if p.ln != nil {
			_ = p.ln.Close()
		}
		if p.release != nil {
			p.release()
		}
		p.srv, p.ln, p.release = nil, nil, nil
	}
	return nil
}
