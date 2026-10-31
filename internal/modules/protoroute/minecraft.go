package protoroute

import "strings"

// Minecraft Java 版客户端连接后先发送握手包：
//
//	VarInt 包长度 | VarInt 包 ID（0x00）| VarInt 协议版本 | String 服务器地址 | UShort 端口 | VarInt 下一状态（1 状态查询、2 登录、3 转移）
//
// 服务器地址即客户端填写的域名，可以像 SNI 一样按域名分流到不同的服务器。
// 1.6 及更早版本的服务器列表查询以 0xFE 0x01 开头，没有地址。

const (
	// minecraftMinPacket 是握手包的最小长度：包 ID、协议版本、地址长度、至少 1 字节地址、端口与下一状态各占的最少字节
	minecraftMinPacket = 7
	// minecraftMaxPacket 是握手包的最大长度。地址最多 255 个字符，Forge、BungeeCord 等会在地址后追加信息，留足余量
	minecraftMaxPacket = 2048
)

// probeMinecraft 检查 Minecraft Java 版的握手包，要求整个包到齐且各字段长度与包长度吻合。
func probeMinecraft(b []byte) probeResult {
	_, res := parseMinecraftHandshake(b)
	return res
}

// parseMinecraftHandshake 解析握手包，返回客户端填写的服务器地址（已规范化，旧版查询为空）。
func parseMinecraftHandshake(b []byte) (string, probeResult) {
	if b[0] == 0xFE {
		if len(b) < 2 {
			return "", probeMore
		}
		if b[1] == 0x01 {
			return "", probeYes
		}
		return "", probeNo
	}
	length, n, state := readVarint(b, 2)
	if state != probeYes {
		return "", state
	}
	if length < minecraftMinPacket || length > minecraftMaxPacket {
		return "", probeNo
	}
	// 包 ID 必须为 0
	if len(b) <= n {
		return "", probeMore
	}
	if b[n] != 0x00 {
		return "", probeNo
	}
	end := n + length
	p := n + 1
	// 协议版本
	_, m, st := readVarint(b[min(p, len(b)):], 5)
	if st != probeYes {
		return "", st
	}
	p += m
	hostLen, m, st := readVarint(b[min(p, len(b)):], 2)
	if st != probeYes {
		return "", st
	}
	p += m
	if hostLen == 0 || p+hostLen+3 > end {
		return "", probeNo
	}
	// 地址只含可打印字符；Forge 用 \0 分隔附加信息
	for i := p; i < p+hostLen && i < len(b); i++ {
		if c := b[i]; c != 0 && c < 0x20 || c == 0x7F {
			return "", probeNo
		}
	}
	if len(b) < end {
		return "", probeMore
	}
	host := string(b[p : p+hostLen])
	p += hostLen + 2
	next, m, st := readVarint(b[p:end], 1)
	if st != probeYes || next < 1 || next > 3 || p+m != end {
		return "", probeNo
	}
	return normalizeMinecraftHost(host), probeYes
}

// normalizeMinecraftHost 去掉 Forge（\0FML\0 等）与 TCPShield、BungeeCord（///、\0 之后的转发信息）追加的内容，转为小写并去掉末尾的点。
func normalizeMinecraftHost(h string) string {
	if i := strings.IndexByte(h, 0); i >= 0 {
		h = h[:i]
	}
	if i := strings.Index(h, "///"); i >= 0 {
		h = h[:i]
	}
	return normalizeSNI(h)
}
