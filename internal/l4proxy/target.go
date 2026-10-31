package l4proxy

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

// Picker 在多个目标地址间轮询，连接失败时依次尝试下一个。
type Picker struct {
	hosts []string
	next  atomic.Uint64
}

// NewPicker 创建目标选择器，hosts 为目标地址列表（不含端口）。
func NewPicker(hosts []string) *Picker {
	return &Picker{hosts: hosts}
}

// Hosts 返回目标地址列表。
func (p *Picker) Hosts() []string {
	return p.hosts
}

// Order 返回本次尝试的目标顺序：从轮询位置开始的完整一圈。
func (p *Picker) Order() []string {
	n := len(p.hosts)
	if n == 0 {
		return nil
	}
	start := int(p.next.Add(1)-1) % n
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = p.hosts[(start+i)%n]
	}
	return out
}

// Dial 按轮询顺序依次连接各目标地址的 port 端口，返回第一个连接成功的连接。
func (p *Picker) Dial(network string, port int, timeout time.Duration) (net.Conn, error) {
	return p.DialContext(context.Background(), network, port, timeout)
}

// DialContext 与 Dial 相同，ctx 取消（如规则停止）时立即放弃，不再尝试后面的目标。
func (p *Picker) DialContext(ctx context.Context, network string, port int, timeout time.Duration) (net.Conn, error) {
	lastErr := fmt.Errorf("没有目标地址")
	d := net.Dialer{Timeout: timeout}
	for _, host := range p.Order() {
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

// CleanTargets 去掉目标地址两侧的空白与 IPv6 方括号，并丢弃空项。
func CleanTargets(in []string) []string {
	var out []string
	for _, t := range in {
		t = strings.Trim(strings.TrimSpace(t), "[]")
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ValidateProxyProtocol 校验发给目标的 PROXY 协议版本：空为不发送，v1 或 v2。
func ValidateProxyProtocol(v string) error {
	switch v {
	case "", "v1", "v2":
		return nil
	}
	return fmt.Errorf("不支持的 PROXY 协议版本 %q", v)
}

// WritePrefix 在双向转发开始前写入 PROXY 协议头（version 非空时）与已预读的客户端数据，合并为一次写入以减少小包。
func WritePrefix(target, client net.Conn, version string, peeked []byte) error {
	var buf []byte
	if version != "" {
		// 源地址为真实客户端，目的地址为本规则的监听地址
		v := byte(2)
		if version == "v1" {
			v = 1
		}
		header, err := proxyproto.HeaderProxyFromAddrs(v, client.RemoteAddr(), client.LocalAddr()).Format()
		if err != nil {
			return err
		}
		buf = header
	}
	buf = append(buf, peeked...)
	if len(buf) == 0 {
		return nil
	}
	_, err := target.Write(buf)
	return err
}
