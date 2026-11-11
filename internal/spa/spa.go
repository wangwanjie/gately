// Package spa 实现与 fwknop 兼容的单包授权（Single Packet Authorization，SPA）报文的解析与生成。
//
// 报文格式（fwknop 2.5 起的 Rijndael + HMAC 模式，FKO 协议 3.0.0）：
//
//	明文 = 随机数:base64(用户名):时间戳:版本:消息类型:base64(消息)[:base64(NAT)][:base64(服务端认证)][:客户端超时]:摘要
//	密文 = "Salted__" + 8 字节盐 + AES-256-CBC(明文, PKCS#7 填充)，密钥与 IV 按 OpenSSL EVP_BytesToKey（MD5、1 轮）由密钥与盐导出
//	报文 = base64(密文) 去掉固定前缀 "U2FsdGVkX1" 与末尾的 "="，再直接拼接 base64(HMAC(带前缀的 base64 密文)) 去掉 "="
//
// 只支持 Rijndael（AES）CBC 模式与 HMAC 认证，不支持 GnuPG、旧版 IV 模式与不带 HMAC 的报文：
// 不带 HMAC 时解密前无法认证，存在填充预言等攻击面。
package spa

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/netip"
	"strconv"
	"strings"
)

// 消息类型（fwknop 的 FKO_*_MSG）。只有 AccessMsg 与 ClientTimeoutAccessMsg 被接受，其余类型解析后由调用方拒绝。
const (
	CommandMsg             = 0
	AccessMsg              = 1
	NATAccessMsg           = 2
	ClientTimeoutAccessMsg = 3
	ClientTimeoutNATMsg    = 4
	LocalNATAccessMsg      = 5
	ClientTimeoutLocalNAT  = 6
	maxMsgType             = 6
)

// Version 是生成报文时写入的协议版本（fwknop 2.6.x 为 3.0.0）。
const Version = "3.0.0"

// 报文长度范围：fwknop 客户端生成的报文通常为 200～400 字节，上限与 fwknopd 的 MAX_SPA_PACKET_LEN 一致
const (
	MinPacketLen = 64
	MaxPacketLen = 1500
)

// 密钥长度范围（字节）：加密密钥是 EVP_BytesToKey 的口令，fwknop 限制为最多 32 字节；
// HMAC 密钥 fwknop 限制为最多 128 字节。下限是为了拒绝过短、可被暴力猜测的密钥
const (
	MinEncKeyLen  = 16
	MaxEncKeyLen  = 32
	MinHMACKeyLen = 16
	MaxHMACKeyLen = 128
)

// maxAccess 为一个报文最多请求的端口数
const maxAccess = 16

const (
	saltedMagic = "Salted__"
	saltedB64   = "U2FsdGVkX1"
	saltLen     = 8
	keyLen      = 32
	blockLen    = aes.BlockSize
	randLen     = 16
	maxUserLen  = 64
	// maxFields 为明文中摘要之前的最多字段数：随机数、用户名、时间戳、版本、类型、消息、NAT、服务端认证、客户端超时
	maxFields = 9
)

// 解析失败的类别，调用方据此记录日志与统计；均可用 errors.Is 判断。
var (
	// ErrFormat 表示报文长度、字符集或结构不符合 SPA 格式（多为端口扫描或其他协议的探测包）
	ErrFormat = errors.New("not an SPA packet")
	// ErrHMAC 表示 HMAC 校验失败：密钥不匹配或报文被篡改
	ErrHMAC = errors.New("HMAC mismatch")
	// ErrDecrypt 表示 HMAC 正确但解密或填充校验失败：两端的加密密钥不一致
	ErrDecrypt = errors.New("decryption failed")
	// ErrMessage 表示解密成功但内容格式错误或摘要不符
	ErrMessage = errors.New("malformed SPA message")
)

// HMAC 摘要算法
const (
	HMACSHA256 = "sha256"
	HMACSHA384 = "sha384"
	HMACSHA512 = "sha512"
)

// hmacAlgos 为支持的 HMAC 算法及其 base64（去掉 "="）长度。不提供 MD5 与 SHA-1。
var hmacAlgos = map[string]struct {
	fn    func() hash.Hash
	b64ln int
}{
	HMACSHA256: {sha256.New, 43},
	HMACSHA384: {sha512.New384, 64},
	HMACSHA512: {sha512.New, 86},
}

// ValidHMACType 判断 HMAC 算法名是否受支持。
func ValidHMACType(s string) bool {
	_, ok := hmacAlgos[s]
	return ok
}

// Key 是一组共享密钥。
type Key struct {
	// Enc 为加密密钥（fwknop 的 KEY_BASE64 解码后的字节）
	Enc []byte
	// HMAC 为 HMAC 密钥（HMAC_KEY_BASE64 解码后的字节）
	HMAC []byte
	// HMACType 为 HMAC 摘要算法，空表示 sha256
	HMACType string
}

func (k Key) hmacType() string {
	if k.HMACType == "" {
		return HMACSHA256
	}
	return k.HMACType
}

// Validate 校验密钥长度与算法。
func (k Key) Validate() error {
	if n := len(k.Enc); n < MinEncKeyLen || n > MaxEncKeyLen {
		return fmt.Errorf("加密密钥长度应为 %d～%d 字节，当前 %d 字节", MinEncKeyLen, MaxEncKeyLen, n)
	}
	if n := len(k.HMAC); n < MinHMACKeyLen || n > MaxHMACKeyLen {
		return fmt.Errorf("HMAC 密钥长度应为 %d～%d 字节，当前 %d 字节", MinHMACKeyLen, MaxHMACKeyLen, n)
	}
	if !ValidHMACType(k.hmacType()) {
		return fmt.Errorf("不支持的 HMAC 算法 %q", k.HMACType)
	}
	return nil
}

// Access 是报文请求放行的一个端口。
type Access struct {
	// Proto 为 tcp 或 udp
	Proto string `json:"proto"`
	Port  int    `json:"port"`
}

func (a Access) String() string {
	return a.Proto + "/" + strconv.Itoa(a.Port)
}

// Message 是解密后的 SPA 消息。
type Message struct {
	// Random 为客户端生成的 16 位随机数
	Random    string
	User      string
	Timestamp int64
	Version   string
	Type      int
	// Addr 为客户端请求放行的地址；客户端使用 -s 时为 0.0.0.0，表示放行报文的来源地址
	Addr   netip.Addr
	Access []Access
	// Command 为命令消息（类型 0）中的命令，不会被执行，只用于日志
	Command string
	// NAT、ServerAuth 为可选字段，原样保留
	NAT        string
	ServerAuth string
	// ClientTimeout 为客户端请求的放行时长（秒），0 表示由服务端决定
	ClientTimeout int
	// Digest 为明文摘要（base64），每个报文唯一，用于防重放
	Digest string
}

// IsAccess 判断消息是否为普通放行请求（可带客户端超时）。
func (m *Message) IsAccess() bool {
	return m.Type == AccessMsg || m.Type == ClientTimeoutAccessMsg
}

// Prefilter 只做不需要密钥的快速检查：长度与字符集。不满足时返回 ErrFormat，调用方可直接丢弃而不必逐个尝试密钥。
func Prefilter(packet []byte) error {
	if len(packet) < MinPacketLen || len(packet) > MaxPacketLen {
		return ErrFormat
	}
	for _, c := range packet {
		if !isB64Char(c) {
			return ErrFormat
		}
	}
	// 客户端会去掉固定的 Salted__ 前缀；带着前缀的报文不是 fwknop 生成的（fwknopd 同样拒绝）
	if bytes.HasPrefix(packet, []byte(saltedB64)) {
		return ErrFormat
	}
	return nil
}

func isB64(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isB64Char(s[i]) {
			return false
		}
	}
	return true
}

func isB64Char(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/'
}

// Decode 用一组密钥认证并解密报文。先校验 HMAC（常数时间比较）再解密，密钥不对的报文不会进入解密流程。
func Decode(packet []byte, k Key) (*Message, error) {
	if err := Prefilter(packet); err != nil {
		return nil, err
	}
	algo, ok := hmacAlgos[k.hmacType()]
	if !ok || len(k.Enc) == 0 || len(k.Enc) > MaxEncKeyLen || len(k.HMAC) == 0 {
		return nil, ErrHMAC
	}
	if len(packet) <= algo.b64ln+MinPacketLen/2 {
		return nil, ErrFormat
	}
	body := packet[:len(packet)-algo.b64ln]
	tag := packet[len(packet)-algo.b64ln:]
	// HMAC 按加上 Salted__ 前缀的 base64 密文计算
	full := make([]byte, 0, len(saltedB64)+len(body))
	full = append(full, saltedB64...)
	full = append(full, body...)
	mac := hmac.New(algo.fn, k.HMAC)
	mac.Write(full)
	want := base64.RawStdEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), tag) != 1 {
		return nil, ErrHMAC
	}
	raw, err := base64.RawStdEncoding.DecodeString(string(full))
	if err != nil || len(raw) < len(saltedMagic)+saltLen+blockLen || !bytes.HasPrefix(raw, []byte(saltedMagic)) {
		return nil, ErrDecrypt
	}
	salt := raw[len(saltedMagic) : len(saltedMagic)+saltLen]
	ct := raw[len(saltedMagic)+saltLen:]
	if len(ct)%blockLen != 0 {
		return nil, ErrDecrypt
	}
	key, iv := deriveKeyIV(k.Enc, salt)
	block, _ := aes.NewCipher(key)
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	pt, ok = unpad(pt)
	if !ok {
		return nil, ErrDecrypt
	}
	return parsePlain(pt)
}

// deriveKeyIV 按 OpenSSL EVP_BytesToKey（MD5、1 轮）由口令与盐导出 32 字节密钥与 16 字节 IV，与 fwknop 的 rij_salt_and_iv 相同。
func deriveKeyIV(pass, salt []byte) (key, iv []byte) {
	var out, prev []byte
	for len(out) < keyLen+blockLen {
		h := md5.New()
		h.Write(prev)
		h.Write(pass)
		h.Write(salt)
		prev = h.Sum(nil)
		out = append(out, prev...)
	}
	return out[:keyLen], out[keyLen : keyLen+blockLen]
}

// unpad 去掉 PKCS#7 填充，填充不合法时返回 false。
func unpad(b []byte) ([]byte, bool) {
	if len(b) == 0 || len(b)%blockLen != 0 {
		return nil, false
	}
	n := int(b[len(b)-1])
	if n == 0 || n > blockLen {
		return nil, false
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, false
		}
	}
	return b[:len(b)-n], true
}

// digestAlgos 按摘要的 base64 长度区分算法。43 与 86 字符分别对应 SHA-256 / SHA3-256 与 SHA-512 / SHA3-512，依次尝试。
// 27 字符（SHA-1）不校验：fwknop 2.6.11 自带的 SHA-1 实现在 arm64 macOS 上算出的摘要与标准 SHA-1 不同，
// 而整个报文已经过 HMAC 认证，摘要只是内层校验和，跳过不影响安全性。
var digestAlgos = map[int][]func() hash.Hash{
	22: {md5.New},
	27: nil,
	43: {sha256.New, func() hash.Hash { return sha3.New256() }},
	64: {sha512.New384},
	86: {sha512.New, func() hash.Hash { return sha3.New512() }},
}

// parsePlain 解析解密后的明文并校验摘要。
func parsePlain(pt []byte) (*Message, error) {
	for _, c := range pt {
		if c < 0x20 || c > 0x7e {
			return nil, ErrMessage
		}
	}
	s := string(pt)
	cut := strings.LastIndexByte(s, ':')
	if cut <= 0 {
		return nil, ErrMessage
	}
	encoded, digest := s[:cut], s[cut+1:]
	algos, known := digestAlgos[len(digest)]
	if !known || !isB64(digest) {
		return nil, ErrMessage
	}
	matched := algos == nil
	for _, fn := range algos {
		h := fn()
		h.Write([]byte(encoded))
		if base64.RawStdEncoding.EncodeToString(h.Sum(nil)) == digest {
			matched = true
			break
		}
	}
	if !matched {
		return nil, ErrMessage
	}
	f := strings.Split(encoded, ":")
	if len(f) < 6 || len(f) > maxFields {
		return nil, ErrMessage
	}
	m := &Message{Random: f[0], Version: f[3], Digest: digest}
	if len(m.Random) != randLen || !allDigits(m.Random) {
		return nil, ErrMessage
	}
	user, err := b64Field(f[1])
	if err != nil || len(user) > maxUserLen {
		return nil, ErrMessage
	}
	m.User = user
	if !allDigits(f[2]) || len(f[2]) > 12 {
		return nil, ErrMessage
	}
	m.Timestamp, _ = strconv.ParseInt(f[2], 10, 64)
	if m.Version == "" || len(m.Version) > 16 {
		return nil, ErrMessage
	}
	if len(f[4]) != 1 || !allDigits(f[4]) {
		return nil, ErrMessage
	}
	m.Type = int(f[4][0] - '0')
	if m.Type > maxMsgType {
		return nil, ErrMessage
	}
	body, err := b64Field(f[5])
	if err != nil {
		return nil, ErrMessage
	}
	rest := f[6:]
	// 带客户端超时的类型，超时总是最后一个字段
	if m.Type == ClientTimeoutAccessMsg || m.Type == ClientTimeoutNATMsg || m.Type == ClientTimeoutLocalNAT {
		if len(rest) == 0 || !allDigits(rest[len(rest)-1]) || len(rest[len(rest)-1]) > 9 {
			return nil, ErrMessage
		}
		m.ClientTimeout, _ = strconv.Atoi(rest[len(rest)-1])
		rest = rest[:len(rest)-1]
	}
	// NAT 类型的下一个字段为 NAT 目标，剩下的一个字段为服务端认证
	if m.Type == NATAccessMsg || m.Type == ClientTimeoutNATMsg || m.Type == LocalNATAccessMsg || m.Type == ClientTimeoutLocalNAT {
		if len(rest) == 0 {
			return nil, ErrMessage
		}
		if m.NAT, err = b64Field(rest[0]); err != nil {
			return nil, ErrMessage
		}
		rest = rest[1:]
	}
	if len(rest) > 1 {
		return nil, ErrMessage
	}
	if len(rest) == 1 {
		if m.ServerAuth, err = b64Field(rest[0]); err != nil {
			return nil, ErrMessage
		}
	}
	if err := m.parseBody(body); err != nil {
		return nil, ErrMessage
	}
	return m, nil
}

// parseBody 解析消息正文：放行请求为 “地址,协议/端口[,协议/端口...]”，命令消息为 “地址,命令”。
func (m *Message) parseBody(body string) error {
	addr, rest, ok := strings.Cut(body, ",")
	if !ok {
		return ErrMessage
	}
	a, err := netip.ParseAddr(addr)
	if err != nil || a.Zone() != "" {
		return ErrMessage
	}
	m.Addr = a.Unmap()
	if m.Type == CommandMsg {
		m.Command = rest
		return nil
	}
	// NAT 请求不会被接受，正文格式与放行请求相同，按同样规则解析便于日志显示
	parts := strings.Split(rest, ",")
	if len(parts) > maxAccess {
		return ErrMessage
	}
	for _, p := range parts {
		acc, err := ParseAccess(p)
		if err != nil {
			return err
		}
		m.Access = append(m.Access, acc)
	}
	return nil
}

// ParseAccess 解析 “tcp/22” 形式的端口。
func ParseAccess(s string) (Access, error) {
	proto, port, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		return Access{}, ErrMessage
	}
	proto = strings.ToLower(proto)
	if proto != "tcp" && proto != "udp" {
		return Access{}, ErrMessage
	}
	if !allDigits(port) || len(port) > 5 {
		return Access{}, ErrMessage
	}
	n, _ := strconv.Atoi(port)
	if n < 1 || n > 65535 {
		return Access{}, ErrMessage
	}
	return Access{Proto: proto, Port: n}, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// b64Field 解码可能去掉了 "=" 的 base64 字段。
func b64Field(s string) (string, error) {
	b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return "", err
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return "", ErrMessage
		}
	}
	return string(b), nil
}

// Encode 按 fwknop 客户端的格式生成报文（摘要固定用 SHA-256），用于测试与自检。rnd 为随机源。
func Encode(m Message, k Key, rnd io.Reader) ([]byte, error) {
	if err := k.Validate(); err != nil {
		return nil, err
	}
	if m.Random == "" {
		var b [randLen]byte
		if _, err := io.ReadFull(rnd, b[:]); err != nil {
			return nil, err
		}
		for i := range b {
			b[i] = '0' + b[i]%10
		}
		m.Random = string(b[:])
	}
	if m.Version == "" {
		m.Version = Version
	}
	acc := make([]string, 0, len(m.Access))
	for _, a := range m.Access {
		acc = append(acc, a.String())
	}
	addr := "0.0.0.0"
	if m.Addr.IsValid() {
		addr = m.Addr.String()
	}
	body := addr + "," + strings.Join(acc, ",")
	if m.Type == CommandMsg {
		body = addr + "," + m.Command
	}
	b64 := base64.RawStdEncoding.EncodeToString
	fields := []string{m.Random, b64([]byte(m.User)), strconv.FormatInt(m.Timestamp, 10), m.Version, strconv.Itoa(m.Type), b64([]byte(body))}
	if m.ServerAuth != "" {
		fields = append(fields, b64([]byte(m.ServerAuth)))
	}
	if m.Type == ClientTimeoutAccessMsg {
		fields = append(fields, strconv.Itoa(m.ClientTimeout))
	}
	encoded := strings.Join(fields, ":")
	sum := sha256.Sum256([]byte(encoded))
	pt := []byte(encoded + ":" + b64(sum[:]))
	pad := blockLen - len(pt)%blockLen
	pt = append(pt, bytes.Repeat([]byte{byte(pad)}, pad)...)
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rnd, salt); err != nil {
		return nil, err
	}
	key, iv := deriveKeyIV(k.Enc, salt)
	block, _ := aes.NewCipher(key)
	ct := make([]byte, len(pt))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, pt)
	raw := append(append([]byte(saltedMagic), salt...), ct...)
	full := base64.RawStdEncoding.EncodeToString(raw)
	mac := hmac.New(hmacAlgos[k.hmacType()].fn, k.HMAC)
	mac.Write([]byte(full))
	out := strings.TrimPrefix(full, saltedB64) + b64(mac.Sum(nil))
	return []byte(out), nil
}
