// Package hops 记录 Gately 自身转发到本机的连接，用于识别管理界面的访问路径。
//
// 通过域名远程管理时，请求可能依次经过协议分流、端口转发、Web 服务才到达管理界面。四层转发（端口转发、
// 协议分流）连接目标后，如果目标在本机，就以拨出连接的本地地址登记这一跳；下一跳（Web 服务监听或管理界面）
// 看到的 TCP 对端正是这个地址，据此逐跳向外追溯，直到访客或本机的其他程序。
//
// Web 服务反向代理到管理界面时复用后端连接池，同一条连接会承载不同访客的请求，不能按连接登记；
// 改为在发往管理界面的请求上附带进程内签名的请求头（Header），说明经过的监听与站点和访客连接的对端。
// 签名密钥每次启动随机生成且不出进程，外部伪造的请求头无法通过校验。
package hops

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"sync"
)

// Header 为 Web 服务发往本实例管理界面的请求上附带的访问路径信息。
const Header = "X-Gately-Hop"

// Hop 是访问路径上由 Gately 自身转发的一跳。
type Hop struct {
	// Module 为 portforward、protoroute 或 webservice
	Module string `json:"module"`
	// Rule 为端口转发或协议分流的规则 key，Web 服务的监听 key
	Rule string `json:"rule"`
	// Route 为协议分流命中的路由名称（默认目标为空）
	Route string `json:"route,omitempty"`
	// Site 为 Web 服务的站点 key
	Site string `json:"site,omitempty"`
	// Listen 为这一跳接入连接的本地地址
	Listen string `json:"listen"`
	// Peer 为这一跳接入连接的 TCP 对端，据此继续向外追溯
	Peer string `json:"peer"`
	// Client 为 Web 服务识别出的访客地址（经可信代理时为真实地址），可为空
	Client string `json:"client,omitempty"`
}

var registry sync.Map // 规范化的地址 → Hop

// Normalize 把 ip:port 规范为统一形式（IPv4 映射地址还原为 IPv4），无法解析时原样返回。
func Normalize(addr string) string {
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return addr
	}
	return netip.AddrPortFrom(ap.Addr().Unmap().WithZone(""), ap.Port()).String()
}

// isLocalConn 判断拨出的连接是否连到本机：本地与对端是同一个地址（回环地址或本机网卡地址）。
func isLocalConn(c net.Conn) bool {
	l, ok1 := c.LocalAddr().(*net.TCPAddr)
	r, ok2 := c.RemoteAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return false
	}
	la, _ := netip.AddrFromSlice(l.IP)
	ra, _ := netip.AddrFromSlice(r.IP)
	la, ra = la.Unmap(), ra.Unmap()
	return la == ra || (la.IsLoopback() && ra.IsLoopback())
}

// Track 登记一条四层转发：in 为接入的连接，out 为连到目标的连接。目标不在本机时不登记。
// 返回的函数在连接结束时调用，撤销登记。
func Track(in, out net.Conn, h Hop) func() {
	if out == nil || in == nil || !isLocalConn(out) {
		return func() {}
	}
	h.Listen = Normalize(in.LocalAddr().String())
	h.Peer = RawPeer(in)
	key := Normalize(out.LocalAddr().String())
	registry.Store(key, h)
	return func() { registry.CompareAndDelete(key, h) }
}

// Lookup 按下一跳看到的 TCP 对端查找登记的转发。
func Lookup(peer string) (Hop, bool) {
	v, ok := registry.Load(Normalize(peer))
	if !ok {
		return Hop{}, false
	}
	return v.(Hop), true
}

// RawPeer 返回连接最底层的 TCP 对端：依次拆开 TLS、PROXY 协议等包装（NetConn、Raw 方法），
// PROXY 协议改写过的对端地址不是真正发起连接的一方，不能用于追溯。
func RawPeer(c net.Conn) string {
	for i := 0; i < 8 && c != nil; i++ {
		var inner net.Conn
		switch w := c.(type) {
		case interface{ NetConn() net.Conn }:
			inner = w.NetConn()
		case interface{ Raw() net.Conn }:
			inner = w.Raw()
		}
		if inner == nil {
			break
		}
		c = inner
	}
	if c == nil || c.RemoteAddr() == nil {
		return ""
	}
	return Normalize(c.RemoteAddr().String())
}

// 进程内签名密钥
var signKey = func() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}()

func sign(payload string) string {
	m := hmac.New(sha256.New, signKey)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Encode 把 Web 服务这一跳编码为带签名的请求头值。
func Encode(h Hop) string {
	data, _ := json.Marshal(h)
	payload := base64.RawURLEncoding.EncodeToString(data)
	return payload + "." + sign(payload)
}

// Decode 校验并解出请求头中的一跳，签名不符时返回 false。
func Decode(v string) (Hop, bool) {
	payload, mac, ok := strings.Cut(v, ".")
	if !ok || !hmac.Equal([]byte(mac), []byte(sign(payload))) {
		return Hop{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Hop{}, false
	}
	var h Hop
	if json.Unmarshal(data, &h) != nil {
		return Hop{}, false
	}
	return h, true
}
