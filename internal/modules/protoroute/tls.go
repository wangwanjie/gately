package protoroute

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
)

const (
	// maxHelloSize 限制 ClientHello 握手消息的长度。后量子密钥交换的 ClientHello 约 2 KB，64 KB 足够且能防止恶意大包占内存
	maxHelloSize = 64 * 1024
	// tlsRecordHeaderLen 是 TLS 记录头长度：类型(1) + 版本(2) + 长度(2)
	tlsRecordHeaderLen = 5
	// tlsMaxRecordLen 是明文记录的最大长度（2^14），额外留出 256 字节容错
	tlsMaxRecordLen = 16384 + 256
)

// errNotTLS 表示连接的首个数据不是 TLS 握手记录。
var errNotTLS = errors.New("不是 TLS 握手")

// clientHello 是从 ClientHello 中解析出的分流依据。
type clientHello struct {
	// ServerName 为 SNI，已转小写并去掉末尾的点；客户端未携带时为空
	ServerName string
	// ALPN 为客户端声明的应用层协议列表，按客户端的偏好排序
	ALPN []string
}

// peekClientHello 读取客户端发来的 TLS ClientHello 并解析 SNI。
// 返回值 raw 为已从连接读出的全部字节，无论解析成功与否都必须原样转发给目标。
// 读取严格按记录长度进行，不会多读 ClientHello 之后的数据。
func peekClientHello(r io.Reader) (raw []byte, hello clientHello, err error) {
	header := make([]byte, tlsRecordHeaderLen)
	// 握手消息可能跨多条记录，按消息长度把各记录的载荷拼接起来
	var msg []byte
	need := -1
	for need < 0 || len(msg) < need {
		if _, err := io.ReadFull(r, header[:1]); err != nil {
			return raw, hello, err
		}
		raw = append(raw, header[0])
		// 0x16 为握手记录；首个字节不是握手时不再继续读，避免卡住非 TLS 协议
		if header[0] != 0x16 {
			return raw, hello, errNotTLS
		}
		// 读取中途出错（如识别超时）时只返回实际读到的字节，不能把缓冲区里未填充的 0 当作数据转发
		if m, err := io.ReadFull(r, header[1:]); err != nil {
			return append(raw, header[1:1+m]...), hello, err
		}
		raw = append(raw, header[1:]...)
		// 记录层版本主版本号固定为 3（SSL 3.0 及之后的各 TLS 版本）
		if header[1] != 3 {
			return raw, hello, errNotTLS
		}
		n := int(binary.BigEndian.Uint16(header[3:5]))
		if n == 0 || n > tlsMaxRecordLen {
			return raw, hello, errors.New("TLS 记录长度无效")
		}
		start := len(raw)
		raw = append(raw, make([]byte, n)...)
		if m, err := io.ReadFull(r, raw[start:]); err != nil {
			return raw[:start+m], hello, err
		}
		msg = append(msg, raw[start:]...)
		if need < 0 && len(msg) >= 4 {
			// 握手消息头：类型(1) + 长度(3)，类型 1 为 ClientHello
			if msg[0] != 1 {
				return raw, hello, errors.New("首个握手消息不是 ClientHello")
			}
			need = 4 + (int(msg[1])<<16 | int(msg[2])<<8 | int(msg[3]))
			if need > maxHelloSize {
				return raw, hello, errors.New("ClientHello 过大")
			}
		}
	}
	hello, err = parseClientHello(msg[4:need])
	return raw, hello, err
}

// parseClientHello 解析 ClientHello 消息体（不含 4 字节握手消息头）。
func parseClientHello(b []byte) (clientHello, error) {
	var hello clientHello
	errBad := errors.New("ClientHello 格式错误")
	// 客户端版本(2) + 随机数(32)
	if len(b) < 34 {
		return hello, errBad
	}
	b = b[34:]
	// 会话 ID、密码套件、压缩方法依次为 1、2、1 字节长度前缀的变长字段
	var ok bool
	if b, ok = skipVector(b, 1); !ok {
		return hello, errBad
	}
	if b, ok = skipVector(b, 2); !ok {
		return hello, errBad
	}
	if b, ok = skipVector(b, 1); !ok {
		return hello, errBad
	}
	// 没有扩展的 ClientHello（极老的客户端）合法，此时无 SNI
	if len(b) == 0 {
		return hello, nil
	}
	if len(b) < 2 {
		return hello, errBad
	}
	extLen := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if extLen > len(b) {
		return hello, errBad
	}
	b = b[:extLen]
	for len(b) >= 4 {
		typ := binary.BigEndian.Uint16(b)
		n := int(binary.BigEndian.Uint16(b[2:]))
		b = b[4:]
		if n > len(b) {
			return hello, errBad
		}
		data := b[:n]
		b = b[n:]
		switch typ {
		case 0: // server_name
			name, err := parseServerName(data)
			if err != nil {
				return hello, err
			}
			hello.ServerName = name
		case 16: // application_layer_protocol_negotiation
			hello.ALPN = parseALPN(data)
		}
	}
	return hello, nil
}

// parseServerName 解析 server_name 扩展，取第一个 host_name 类型的名称。
func parseServerName(b []byte) (string, error) {
	errBad := errors.New("SNI 扩展格式错误")
	if len(b) < 2 {
		return "", errBad
	}
	n := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if n > len(b) {
		return "", errBad
	}
	b = b[:n]
	for len(b) >= 3 {
		nameType := b[0]
		l := int(binary.BigEndian.Uint16(b[1:]))
		b = b[3:]
		if l > len(b) {
			return "", errBad
		}
		if nameType == 0 {
			return normalizeSNI(string(b[:l])), nil
		}
		b = b[l:]
	}
	return "", nil
}

// parseALPN 解析 ALPN 扩展中的协议列表，格式错误时返回已解析的部分。
func parseALPN(b []byte) []string {
	if len(b) < 2 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if n < len(b) {
		b = b[:n]
	}
	var out []string
	for len(b) > 0 {
		l := int(b[0])
		if l == 0 || l+1 > len(b) {
			break
		}
		out = append(out, string(b[1:1+l]))
		b = b[1+l:]
	}
	return out
}

// skipVector 跳过一个以 prefix 字节长度开头的变长字段。
func skipVector(b []byte, prefix int) ([]byte, bool) {
	if len(b) < prefix {
		return nil, false
	}
	n := 0
	for i := 0; i < prefix; i++ {
		n = n<<8 | int(b[i])
	}
	b = b[prefix:]
	if n > len(b) {
		return nil, false
	}
	return b[n:], true
}

// normalizeSNI 统一域名大小写并去掉末尾的点，与 haproxy 的 req_ssl_sni -i 一样不区分大小写。
func normalizeSNI(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}
