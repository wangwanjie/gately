package protoroute

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

// 分流路由可识别的协议。识别只看客户端最先发送的数据，不解密、不终止任何协议。
const (
	// ProtoTLS 按 ClientHello 中的 SNI 与 ALPN 分流，旧配置中协议为空的路由也按 TLS 处理
	ProtoTLS = "tls"
	// ProtoHTTP 为明文 HTTP/1.x 与 HTTP/2 明文直连（h2c prior knowledge），按 Host 头分流
	ProtoHTTP = "http"
	// ProtoSSH 客户端连接后先发送 "SSH-" 开头的版本标识
	ProtoSSH = "ssh"
	// ProtoRDP 为远程桌面，首包是 TPKT 头加 X.224 连接请求
	ProtoRDP = "rdp"
	// ProtoSOCKS 包括 SOCKS4/4a 与 SOCKS5
	ProtoSOCKS = "socks"
	// ProtoOpenVPN 为 TCP 模式的 OpenVPN，首包是客户端硬重置请求
	ProtoOpenVPN = "openvpn"
	// ProtoXMPP 客户端先发送 XML 声明或 stream 开始标签
	ProtoXMPP = "xmpp"
	// ProtoMQTT 首包是 CONNECT 报文
	ProtoMQTT = "mqtt"
	// ProtoPostgres 首包是启动消息、SSLRequest 或 GSSENCRequest
	ProtoPostgres = "postgresql"
	// ProtoRTSP 为摄像头、流媒体常用的 RTSP，请求行版本为 RTSP/x
	ProtoRTSP = "rtsp"
	// ProtoSIP 为 TCP 上的 SIP 信令，请求行版本为 SIP/2.0
	ProtoSIP = "sip"
	// ProtoMinecraft 为 Minecraft Java 版，按握手包中的服务器地址分流
	ProtoMinecraft = "minecraft"
	// ProtoRedis 为以 RESP 数组发送命令的 Redis 客户端
	ProtoRedis = "redis"
	// ProtoAMQP 为 AMQP 0-9-1（RabbitMQ）与 AMQP 1.0
	ProtoAMQP = "amqp"
	// ProtoSMB 为直接走 TCP 的 SMB1/2/3
	ProtoSMB = "smb"
	// ProtoGit 为 git:// 协议（git daemon）
	ProtoGit = "git"
	// ProtoBitTorrent 为未加密的 BitTorrent 对等连接
	ProtoBitTorrent = "bittorrent"
	// ProtoADB 为 adb connect 使用的 Android 调试桥
	ProtoADB = "adb"
	// ProtoIRC 为明文 IRC 客户端
	ProtoIRC = "irc"
	// ProtoTimeout 表示客户端在识别超时内没有发送任何数据，用于 VNC、MySQL、FTP、SMTP 等由服务端先发送数据的协议
	ProtoTimeout = "timeout"
)

// routeProtocols 是路由可以选择的协议。
var routeProtocols = map[string]bool{
	ProtoTLS: true, ProtoHTTP: true, ProtoSSH: true, ProtoRDP: true, ProtoSOCKS: true,
	ProtoOpenVPN: true, ProtoXMPP: true, ProtoMQTT: true, ProtoPostgres: true,
	ProtoRTSP: true, ProtoSIP: true, ProtoMinecraft: true, ProtoRedis: true, ProtoAMQP: true,
	ProtoSMB: true, ProtoGit: true, ProtoBitTorrent: true, ProtoADB: true, ProtoIRC: true,
	ProtoTimeout: true,
}

// protocolUsesDomains 判断该协议的路由是否按域名匹配：TLS 看 SNI，HTTP 看 Host，Minecraft 看握手包中的服务器地址。
func protocolUsesDomains(p string) bool {
	return p == ProtoTLS || p == ProtoHTTP || p == ProtoMinecraft
}

// defaultSniffTimeout 是规则未设置识别超时时的默认值：等待客户端发来可识别数据（包括完整 ClientHello）的时长，
// 与 haproxy 常用的 inspect-delay 5s 一致；测试中会调小
var defaultSniffTimeout = 5 * time.Second

const (
	// maxSniffSize 是识别协议阶段最多预读的字节数：请求行最长 8 KB，其余协议的特征都在开头几 KB 内
	maxSniffSize = 16 * 1024
	// maxHTTPHeaderSize 是为取得 Host 头最多预读的请求头长度
	maxHTTPHeaderSize = 32 * 1024
	// sniffReadChunk 是每次从连接读取的缓冲区大小
	sniffReadChunk = 4096
)

// sniffResult 是识别结果与分流依据。
type sniffResult struct {
	// Proto 为识别出的协议，空表示无法识别
	Proto string
	// Host 为 TLS 的 SNI、HTTP 的 Host 或 Minecraft 的服务器地址（已转小写、去掉端口与末尾的点），其他协议为空
	Host string
	// ALPN 为 TLS 客户端声明的应用层协议列表，按客户端的偏好排序
	ALPN []string
}

// probeResult 是单个协议特征检查的结果。
type probeResult int

const (
	// probeNo 表示数据不是该协议
	probeNo probeResult = iota
	// probeMore 表示已有数据与该协议一致，但还需要更多数据才能确定
	probeMore
	// probeYes 表示确定是该协议
	probeYes
)

type probe struct {
	proto string
	check func([]byte) probeResult
}

// probes 是按顺序检查的协议特征。各协议的特征互不重叠，顺序只影响检查开销。
var probes = []probe{
	{ProtoTLS, probeTLS},
	{ProtoHTTP, probeHTTP},
	{ProtoSSH, probeSSH},
	{ProtoRDP, probeRDP},
	{ProtoSOCKS, probeSOCKS},
	{ProtoOpenVPN, probeOpenVPN},
	{ProtoXMPP, probeXMPP},
	{ProtoMQTT, probeMQTT},
	{ProtoPostgres, probePostgres},
	{ProtoRTSP, probeRTSP},
	{ProtoSIP, probeSIP},
	{ProtoMinecraft, probeMinecraft},
	{ProtoRedis, probeRedis},
	{ProtoAMQP, probeAMQP},
	{ProtoSMB, probeSMB},
	{ProtoGit, probeGit},
	{ProtoBitTorrent, probeBitTorrent},
	{ProtoADB, probeADB},
	{ProtoIRC, probeIRC},
}

// detectProtocol 根据已收到的数据识别协议。done 为 false 时需要继续读取；done 为 true 且 proto 为空表示无法识别。
func detectProtocol(b []byte) (proto string, done bool) {
	if len(b) == 0 {
		return "", false
	}
	more := false
	for _, p := range probes {
		switch p.check(b) {
		case probeYes:
			return p.proto, true
		case probeMore:
			more = true
		}
	}
	return "", !more
}

// hasPrefixPartial 检查 b 与 prefix 在较短的长度内是否一致，返回 probeYes（b 已包含完整前缀）、probeMore 或 probeNo。
func hasPrefixPartial(b []byte, prefix string) probeResult {
	n := min(len(b), len(prefix))
	if string(b[:n]) != prefix[:n] {
		return probeNo
	}
	if n == len(prefix) {
		return probeYes
	}
	return probeMore
}

// sniffer 从连接中预读数据，记录已读出的全部字节。
type sniffer struct {
	conn net.Conn
	buf  []byte
}

// fill 从连接再读一次数据追加到缓冲区。
func (s *sniffer) fill() error {
	tmp := make([]byte, sniffReadChunk)
	n, err := s.conn.Read(tmp)
	s.buf = append(s.buf, tmp[:n]...)
	if n > 0 {
		return nil
	}
	if err == nil {
		err = io.ErrNoProgress
	}
	return err
}

// isTimeout 判断读取错误是否为读超时。
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// errClientGone 表示客户端在识别完成前断开或出错，连接应直接关闭。
var errClientGone = errors.New("客户端在识别完成前断开")

// sniffConn 预读客户端最初发送的数据，识别协议并取出分流依据（SNI、ALPN、Host）。
// 返回的 raw 为已从连接读出的全部字节，转发时必须原样先写给目标。
// 超时未收到任何数据时 Proto 为 ProtoTimeout；收到了数据但无法识别时 Proto 为空。
// 客户端在识别完成前断开时返回 errClientGone。
func sniffConn(conn net.Conn, timeout time.Duration) (raw []byte, res sniffResult, err error) {
	s := &sniffer{conn: conn}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	for {
		proto, done := detectProtocol(s.buf)
		if done {
			res.Proto = proto
			break
		}
		if len(s.buf) >= maxSniffSize {
			// 各协议的特征都在开头很短的范围内，读到这里仍未确定的按无法识别处理
			return s.buf, res, nil
		}
		if err := s.fill(); err != nil {
			switch {
			case isTimeout(err) && len(s.buf) == 0:
				res.Proto = ProtoTimeout
				return s.buf, res, nil
			case isTimeout(err), errors.Is(err, io.EOF) && len(s.buf) > 0:
				// 收到了一部分数据后超时，或客户端发完数据后半关闭，按无法识别交给默认目标
				return s.buf, res, nil
			}
			return s.buf, res, errClientGone
		}
	}

	switch res.Proto {
	case ProtoTLS:
		err = s.readClientHello(&res)
	case ProtoHTTP:
		s.readHTTPHost(&res)
	case ProtoMinecraft:
		// 识别时已要求整个握手包到齐，地址直接从缓冲区取
		res.Host, _ = parseMinecraftHandshake(s.buf)
	}
	return s.buf, res, err
}

// readClientHello 读完整个 ClientHello 并取出 SNI 与 ALPN。
// 与 haproxy 的 inspect-delay + req_ssl_sni 行为一致：超时或无法解析时按没有 SNI 的 TLS 连接分流，客户端断开时放弃连接。
func (s *sniffer) readClientHello(res *sniffResult) error {
	br := bytes.NewReader(s.buf)
	raw, hello, err := peekClientHello(io.MultiReader(br, s.conn))
	// raw 是 ClientHello 解析过程中读出的字节，缓冲区里剩余未用到的部分（例如紧随其后的早期数据）接在后面
	s.buf = append(raw, s.buf[len(s.buf)-br.Len():]...)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && !ne.Timeout() || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return errClientGone
		}
		return nil
	}
	res.Host = hello.ServerName
	res.ALPN = hello.ALPN
	return nil
}

// readHTTPHost 继续读取请求头直到取得 Host。超时、请求头过长或客户端半关闭时按没有 Host 处理。
func (s *sniffer) readHTTPHost(res *sniffResult) {
	for {
		host, done := httpHost(s.buf)
		if done {
			res.Host = host
			return
		}
		if len(s.buf) >= maxHTTPHeaderSize {
			return
		}
		if err := s.fill(); err != nil {
			return
		}
	}
}

// httpHost 从已收到的请求头中查找 Host。done 为 false 表示还没有读到 Host 行或请求头结尾。
// HTTP/2 明文直连（PRI 前言）没有 Host 头，直接返回空。
func httpHost(b []byte) (host string, done bool) {
	if bytes.HasPrefix(b, []byte("PRI ")) {
		return "", true
	}
	// 跳过请求行
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return "", false
	}
	b = b[i+1:]
	for {
		i = bytes.IndexByte(b, '\n')
		if i < 0 {
			return "", false
		}
		line := strings.TrimRight(string(b[:i]), "\r")
		b = b[i+1:]
		// 空行是请求头结尾，没有 Host 头（HTTP/1.0 客户端可以不发）
		if line == "" {
			return "", true
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "host") {
			return normalizeHost(value), true
		}
	}
}

// normalizeHost 去掉 Host 中的端口，统一为小写并去掉末尾的点。
func normalizeHost(v string) string {
	v = strings.TrimSpace(v)
	if h, _, err := net.SplitHostPort(v); err == nil {
		v = h
	}
	return normalizeSNI(strings.Trim(v, "[]"))
}
