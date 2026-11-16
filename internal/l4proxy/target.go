package l4proxy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
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

// SplitTargets 清洗目标地址列表，并识别写成「主机:端口」或「[IPv6]:端口」的目标。
// 目标地址只应是主机名或 IP，端口另填；过去把 127.0.0.1:8080 原样保存，拨号时再拼上端口成了 127.0.0.1:8080:8080，
// 规则显示运行中却连不上。这里把端口拆出来交给调用方合并到目标端口。
// 返回去掉端口与方括号的主机列表，以及地址里写的端口（多个目标写了端口时必须相同；都没写时为 0）。
// 无法识别的写法（带 http:// 或路径、多个冒号又不是 IPv6 地址、端口不是 1～65535）返回错误。
func SplitTargets(in []string) ([]string, int, error) {
	var out []string
	port := 0
	for _, raw := range in {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		host, p, err := splitTarget(t)
		if err != nil {
			return nil, 0, err
		}
		if host == "" {
			continue
		}
		if p != 0 {
			if port != 0 && port != p {
				return nil, 0, fmt.Errorf("目标地址里写的端口不一致（%d 与 %d），请把端口填在目标端口里", port, p)
			}
			port = p
		}
		out = append(out, host)
	}
	return out, port, nil
}

// splitTarget 解析单个目标地址，返回主机与端口（没写端口时为 0）。
func splitTarget(t string) (string, int, error) {
	if strings.Contains(t, "/") {
		return "", 0, fmt.Errorf("目标地址 %q 无效：只填主机名或 IP，不带 http:// 或路径", t)
	}
	// [IPv6] 或 [IPv6]:端口
	if strings.HasPrefix(t, "[") {
		if strings.HasSuffix(t, "]") {
			return strings.Trim(t, "[]"), 0, nil
		}
		host, p, err := net.SplitHostPort(t)
		if err != nil {
			return "", 0, fmt.Errorf("目标地址 %q 无效", t)
		}
		port, err := parseTargetPort(t, p)
		return host, port, err
	}
	switch strings.Count(t, ":") {
	case 0:
		return strings.Trim(t, "[]"), 0, nil
	case 1:
		host, p, err := net.SplitHostPort(t)
		if err != nil || host == "" {
			return "", 0, fmt.Errorf("目标地址 %q 无效", t)
		}
		port, err := parseTargetPort(t, p)
		return host, port, err
	}
	// 多个冒号只能是不带方括号的 IPv6 地址（可带 %网卡）
	if _, err := netip.ParseAddr(t); err != nil {
		return "", 0, fmt.Errorf("目标地址 %q 无效：IPv6 地址带端口时写成 [地址]:端口，或把端口填在目标端口里", t)
	}
	return t, 0, nil
}

func parseTargetPort(t, p string) (int, error) {
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("目标地址 %q 的端口无效", t)
	}
	return n, nil
}

// MergeTargetPorts 把目标地址里拆出的端口 port 合并到目标端口 spec（如 "8080"、"8080-8090"）：
// spec 为空时填入 port；spec 正好是同一个端口时保留；其他情况视为冲突返回错误。port 为 0 时原样返回 spec。
func MergeTargetPorts(spec string, port int) (string, error) {
	if port == 0 {
		return spec, nil
	}
	s := strings.TrimSpace(spec)
	if s == "" {
		return strconv.Itoa(port), nil
	}
	if ports, err := ParsePorts(s); err == nil && len(ports) == 1 && ports[0] == port {
		return s, nil
	}
	return "", fmt.Errorf("目标地址里写了端口 %d，与目标端口 %s 不一致；请只在一处填写端口", port, s)
}

// MergeTargetPort 与 MergeTargetPorts 相同，用于单个整数端口字段（0 表示未填）。
func MergeTargetPort(cur, port int) (int, error) {
	if port == 0 || cur == port {
		return cur, nil
	}
	if cur == 0 {
		return port, nil
	}
	return 0, fmt.Errorf("目标地址里写了端口 %d，与目标端口 %d 不一致；请只在一处填写端口", port, cur)
}

// TargetProblems 检查已保存的目标地址里无法连接的写法（旧版本保存的「主机:端口」等），返回每个问题的说明。
// 用于启动时告警并在规则状态上显示，不修改配置。
func TargetProblems(in []string) []string {
	var out []string
	for _, raw := range in {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		_, p, err := splitTarget(t)
		if err != nil {
			out = append(out, err.Error())
		} else if p != 0 {
			out = append(out, fmt.Sprintf("目标地址 %q 带有端口，连接会失败；请编辑规则，把端口填在目标端口里", t))
		}
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
