package protoroute

import (
	"bytes"
	"encoding/binary"
)

// probeTLS 检查 TLS 握手记录头：类型 0x16，主版本 3，次版本 0～4（SSL 3.0 到 TLS 1.3 的记录层版本）。
func probeTLS(b []byte) probeResult {
	if b[0] != 0x16 {
		return probeNo
	}
	if len(b) < 3 {
		if len(b) == 2 && b[1] != 3 {
			return probeNo
		}
		return probeMore
	}
	if b[1] == 3 && b[2] <= 4 {
		return probeYes
	}
	return probeNo
}

// probeSSH 检查 SSH 客户端的版本标识，客户端连接后立即发送 "SSH-2.0-..."。
func probeSSH(b []byte) probeResult {
	return hasPrefixPartial(b, "SSH-")
}

// probeRDP 检查 TPKT 头（版本 3、保留 0、长度）与 X.224 连接请求（CR，类型 0xE0）。
func probeRDP(b []byte) probeResult {
	if b[0] != 0x03 {
		return probeNo
	}
	if len(b) >= 2 && b[1] != 0x00 {
		return probeNo
	}
	if len(b) >= 4 {
		// TPKT 长度包含 4 字节头，X.224 连接请求至少 7 字节；连接请求只带 Cookie 与协商请求，不会超过 1 KB。
		// 限制上限也让长度高字节不会是 OpenVPN 硬重置的操作码字节，两者的特征不重叠
		if n := binary.BigEndian.Uint16(b[2:4]); n < 11 || n > 1024 {
			return probeNo
		}
	}
	if len(b) < 6 {
		return probeMore
	}
	if b[5]&0xF0 == 0xE0 {
		return probeYes
	}
	return probeNo
}

// probeSOCKS 检查 SOCKS5 的方法协商请求与 SOCKS4/4a 的 CONNECT/BIND 请求。
func probeSOCKS(b []byte) probeResult {
	switch b[0] {
	case 0x05:
		// 版本(1) + 方法数(1) + 方法列表
		if len(b) < 2 {
			return probeMore
		}
		n := int(b[1])
		if n == 0 {
			return probeNo
		}
		for i := 2; i < len(b) && i < 2+n; i++ {
			// 0x00～0x09 为已分配的认证方法，0x80～0xFE 为私有方法，0xFF 只出现在服务端回复中
			if m := b[i]; m > 0x09 && (m < 0x80 || m == 0xFF) {
				return probeNo
			}
		}
		if len(b) < 2+n {
			return probeMore
		}
		return probeYes
	case 0x04:
		// 版本(1) + 命令(1，1 为 CONNECT、2 为 BIND) + 端口(2) + IPv4(4) + 以 0 结尾的用户 ID
		if len(b) >= 2 && b[1] != 0x01 && b[1] != 0x02 {
			return probeNo
		}
		if len(b) < 9 {
			return probeMore
		}
		// 用户 ID 不会超过 255 字节，只在这个范围内找结尾的 0，保证结果与数据分几次到达无关
		id := b[8:min(len(b), 8+256)]
		if bytes.IndexByte(id, 0) >= 0 {
			return probeYes
		}
		if len(id) == 256 {
			return probeNo
		}
		return probeMore
	}
	return probeNo
}

// probeOpenVPN 检查 TCP 模式 OpenVPN 的首包：2 字节包长度，随后是操作码与密钥 ID 组成的字节。
// 客户端首包为 P_CONTROL_HARD_RESET_CLIENT_V2（操作码 7）或使用 tls-crypt-v2 时的 V3（操作码 10），密钥 ID 为 0。
func probeOpenVPN(b []byte) probeResult {
	// 04 01、04 02 开头的是 SOCKS4 请求（随后是目标端口），长度 1025、1026 的 OpenVPN 包让给 SOCKS4，保证两者的特征不重叠
	if len(b) >= 2 && b[0] == 0x04 && (b[1] == 0x01 || b[1] == 0x02) {
		return probeNo
	}
	if len(b) >= 2 {
		// 硬重置包至少含 1 字节操作码与 8 字节会话 ID，tls-crypt-v2 附带的客户端密钥也不超过 2 KB
		if n := binary.BigEndian.Uint16(b[:2]); n < 14 || n > 2048 {
			return probeNo
		}
		// 长度恰为 1792、2048 时前两字节是 07 00、08 00，与 Minecraft 握手包（长度 7、8 后跟包 ID 0）重叠，让给 Minecraft
		if b[1] == 0x00 && b[0] >= 0x07 {
			return probeNo
		}
	} else if b[0] > 0x08 {
		return probeNo
	}
	if len(b) < 3 {
		return probeMore
	}
	if b[2] == 7<<3 || b[2] == 10<<3 {
		return probeYes
	}
	return probeNo
}

// probeXMPP 检查 XMPP 客户端发送的 XML 声明或 stream 开始标签。
func probeXMPP(b []byte) probeResult {
	if b[0] != '<' {
		return probeNo
	}
	res := probeNo
	for _, p := range []string{"<?xml", "<stream:stream"} {
		switch hasPrefixPartial(b, p) {
		case probeYes:
			return probeYes
		case probeMore:
			res = probeMore
		}
	}
	return res
}

// probeMQTT 检查 MQTT CONNECT 报文：固定头 0x10、剩余长度（1～4 字节变长编码），随后是协议名 "MQTT"（3.1.1/5.0）或 "MQIsdp"（3.1）。
func probeMQTT(b []byte) probeResult {
	if b[0] != 0x10 {
		return probeNo
	}
	// 解析剩余长度，每字节最高位表示后面还有字节
	remaining, n, state := readVarint(b[1:], 4)
	if state != probeYes {
		return state
	}
	// CONNECT 的可变头与载荷至少 12 字节（协议名 6、版本 1、标志 1、保活 2、客户端 ID 长度 2），
	// 同时排除剩余长度为 0 时与 Minecraft 握手包（长度 16 后跟包 ID 0）的重叠
	if remaining < 12 {
		return probeNo
	}
	rest := b[1+n:]
	res := probeNo
	for _, name := range []string{"\x00\x04MQTT", "\x00\x06MQIsdp"} {
		switch hasPrefixPartial(rest, name) {
		case probeYes:
			return probeYes
		case probeMore:
			res = probeMore
		}
	}
	return res
}

// PostgreSQL 客户端首包中的协议号与请求码。
const (
	pgProtocol3     = 196608   // 3.0 启动消息
	pgSSLRequest    = 80877103 // 请求升级 TLS
	pgGSSENCRequest = 80877104 // 请求 GSSAPI 加密
	pgCancelRequest = 80877102 // 取消正在执行的查询
)

// probePostgres 检查 PostgreSQL 客户端首包：4 字节长度（含自身）与 4 字节协议号或请求码。
func probePostgres(b []byte) probeResult {
	// 启动消息不会超过 10000 字节，长度的前两个字节必为 0
	for i := 0; i < 2 && i < len(b); i++ {
		if b[i] != 0 {
			return probeNo
		}
	}
	if len(b) < 8 {
		return probeMore
	}
	n := binary.BigEndian.Uint32(b[:4])
	if n < 8 || n > 10000 {
		return probeNo
	}
	switch binary.BigEndian.Uint32(b[4:8]) {
	case pgProtocol3, pgSSLRequest, pgGSSENCRequest, pgCancelRequest:
		return probeYes
	}
	return probeNo
}

// readVarint 读取 Minecraft、MQTT 使用的变长整数（每字节低 7 位为数据，最高位表示后面还有字节），最多 maxBytes 字节。
// 返回值、占用的字节数与状态：数据不够时为 probeMore，超过 maxBytes 仍未结束时为 probeNo。
func readVarint(b []byte, maxBytes int) (value, n int, state probeResult) {
	for i := 0; i < maxBytes; i++ {
		if i >= len(b) {
			return 0, 0, probeMore
		}
		value |= int(b[i]&0x7F) << (7 * i)
		if b[i]&0x80 == 0 {
			return value, i + 1, probeYes
		}
	}
	return 0, 0, probeNo
}

// probeRedis 检查 Redis 客户端以 RESP 数组发送的命令：*<参数个数>\r\n$<长度>。
func probeRedis(b []byte) probeResult {
	if b[0] != '*' {
		return probeNo
	}
	i := 1
	for ; i < len(b) && b[i] >= '0' && b[i] <= '9'; i++ {
		// 参数个数不会超过 7 位数
		if i > 7 {
			return probeNo
		}
	}
	if i == 1 && i < len(b) {
		return probeNo
	}
	return hasPrefixPartial(b[i:], "\r\n$")
}

// probeAMQP 检查 AMQP 协议头：0-9-1 为 "AMQP\x00\x00\x09\x01"，1.0 为 "AMQP" 加协议 ID 0（AMQP）、2（TLS）或 3（SASL）。
func probeAMQP(b []byte) probeResult {
	if r := hasPrefixPartial(b, "AMQP"); r != probeYes {
		return r
	}
	if len(b) < 5 {
		return probeMore
	}
	switch b[4] {
	case 0x00, 0x02, 0x03:
		return probeYes
	}
	return probeNo
}

// probeSMB 检查 NetBIOS 会话报文中的 SMB 协商请求：类型 0、3 字节长度，随后是 SMB1（FF）、SMB2（FE）或加密（FD）的协议标识 "SMB"。
func probeSMB(b []byte) probeResult {
	// 协商请求不超过 64 KB，长度最高字节为 0
	for i, want := range []byte{0x00, 0x00} {
		if i < len(b) && b[i] != want {
			return probeNo
		}
	}
	if len(b) > 4 && b[4] != 0xFF && b[4] != 0xFE && b[4] != 0xFD {
		return probeNo
	}
	if len(b) <= 5 {
		return probeMore
	}
	return hasPrefixPartial(b[5:], "SMB")
}

// probeGit 检查 git:// 协议的首个 pkt-line：4 位十六进制长度，随后是要执行的服务名。
func probeGit(b []byte) probeResult {
	for i := 0; i < 4 && i < len(b); i++ {
		c := b[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return probeNo
		}
	}
	if len(b) <= 4 {
		return probeMore
	}
	res := probeNo
	for _, svc := range []string{"git-upload-pack ", "git-receive-pack ", "git-upload-archive "} {
		switch hasPrefixPartial(b[4:], svc) {
		case probeYes:
			return probeYes
		case probeMore:
			res = probeMore
		}
	}
	return res
}

// probeBitTorrent 检查 BitTorrent 对等连接的握手：长度 19 后跟 "BitTorrent protocol"。启用协议加密（MSE）的连接没有特征。
func probeBitTorrent(b []byte) probeResult {
	return hasPrefixPartial(b, "\x13BitTorrent protocol")
}

// probeADB 检查 Android 调试桥（adb connect）的首个消息 CNXN。
func probeADB(b []byte) probeResult {
	return hasPrefixPartial(b, "CNXN")
}
