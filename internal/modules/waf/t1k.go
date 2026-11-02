package waf

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// 本文件实现长亭雷池检测引擎（detector / snserver）使用的 T1K 协议客户端，
// 报文格式参考官方的 lua-resty-t1k 与 t1k-go（Apache-2.0）。
// 官方 Go 实现会把请求体整个读进内存，也没有读写超时，网关里不能直接使用，因此另行实现：
// 每次检测都有总超时，请求体只送检前面一部分，回包的大小也有上限。
//
// 报文由若干段组成，每段是 1 字节标签 + 4 字节小端长度 + 内容；
// 一次检测的第一段标签带 MASK_FIRST，最后一段带 MASK_LAST，检测引擎按同样的格式回包。

// T1K 段标签
const (
	t1kTagHeader      byte = 0x01
	t1kTagBody        byte = 0x02
	t1kTagExtra       byte = 0x03
	t1kTagVersion     byte = 0x20
	t1kTagExtraHeader byte = 0x23
	t1kTagExtraBody   byte = 0x24

	t1kMaskTag   byte = 0x3f
	t1kMaskFirst byte = 0x40
	t1kMaskLast  byte = 0x80
)

// t1kProto 是请求中声明的协议版本，与 lua-resty-t1k 一致；Extra 段使用 0x01 开头的新版二进制键
const t1kProto = "Proto:3\n"

// Extra 段的键（新版格式），每项为“键字节 + 值 + 换行”
const (
	t1kExtraNewVer       = "\x01"
	t1kExtraRemotePort   = "\x11"
	t1kExtraLocalPort    = "\x12"
	t1kExtraRemoteAddr   = "\x13"
	t1kExtraLocalAddr    = "\x14"
	t1kExtraServerName   = "\x15"
	t1kExtraScheme       = "\x16"
	t1kExtraProxyName    = "\x17"
	t1kExtraUUID         = "\x18"
	t1kExtraHasRspIfOK   = "\x19"
	t1kExtraHasRspIfBlk  = "\x1a"
	t1kExtraReqBeginTime = "\x1b"
)

// 检测结果的动作
const (
	t1kActionPass  = '.'
	t1kActionBlock = '?'
)

// 回包单段与整体的大小上限：拦截页、附加响应头通常只有几 KB，超过上限视为协议错误，防止异常回包占满内存
const (
	t1kMaxSection = 1 << 20
	t1kMaxReply   = 4 << 20
)

// t1kSection 是待发送的一段。
type t1kSection struct {
	tag  byte
	data []byte
}

// t1kResult 是一次检测的结果。
type t1kResult struct {
	// action 为 '.'（放行）或 '?'（拦截）
	action byte
	// status 为拦截时建议返回的状态码（检测引擎以文本给出，可能为空）
	status string
	// extraHeader 为需要加到响应上的附加响应头，每行“名称:值”
	extraHeader []byte
	// extraBody 为拦截时的附加内容，目前只有 <!-- event_id: xxx --> 形式的事件 ID 注释，不含完整拦截页
	extraBody []byte
}

// blocked 判断检测结果是否为拦截。
func (r *t1kResult) blocked() bool {
	return r.action == t1kActionBlock
}

// eventID 从拦截页中取出事件 ID，雷池控制台的攻击事件按它检索。
func (r *t1kResult) eventID() string {
	const mark = "event_id:"
	s := string(r.extraBody)
	i := strings.Index(s, mark)
	if i < 0 {
		return ""
	}
	s = strings.TrimLeft(s[i+len(mark):], " ")
	end := 0
	// 新版雷池的事件 ID 是带短横线的 UUID，旧版为纯字母数字
	for end < len(s) && (isAlnum(s[end]) || s[end] == '-') {
		end++
	}
	return s[:end]
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// headers 解析附加响应头，跳过格式不正确的行。
func (r *t1kResult) headers() [][2]string {
	var out [][2]string
	for _, line := range strings.Split(string(r.extraHeader), "\n") {
		line = strings.TrimRight(line, "\r")
		k, v, ok := strings.Cut(line, ":")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		out = append(out, [2]string{k, strings.TrimSpace(v)})
	}
	return out
}

// writeT1K 按顺序写出各段，自动给首段与末段加上标志位。
func writeT1K(w io.Writer, secs []t1kSection) error {
	var hdr [5]byte
	for i, s := range secs {
		tag := s.tag
		if i == 0 {
			tag |= t1kMaskFirst
		}
		if i == len(secs)-1 {
			tag |= t1kMaskLast
		}
		hdr[0] = tag
		binary.LittleEndian.PutUint32(hdr[1:], uint32(len(s.data)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
		if len(s.data) > 0 {
			if _, err := w.Write(s.data); err != nil {
				return err
			}
		}
	}
	return nil
}

// readT1K 读取一次完整的回包。
func readT1K(r io.Reader) (*t1kResult, error) {
	var res t1kResult
	var hdr [5]byte
	total := 0
	for first := true; ; first = false {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, err
		}
		if first && hdr[0]&t1kMaskFirst == 0 {
			return nil, errors.New("回包首段缺少起始标志，连接状态异常")
		}
		size := int(binary.LittleEndian.Uint32(hdr[1:]))
		total += size
		if size > t1kMaxSection || total > t1kMaxReply {
			return nil, fmt.Errorf("回包过大（%d 字节）", total)
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		switch hdr[0] & t1kMaskTag {
		case t1kTagHeader:
			if len(data) == 1 {
				res.action = data[0]
			}
		case t1kTagBody:
			res.status = string(data)
		case t1kTagExtraHeader:
			res.extraHeader = data
		case t1kTagExtraBody:
			res.extraBody = data
		}
		if hdr[0]&t1kMaskLast != 0 {
			break
		}
	}
	return &res, nil
}

// t1kUUID 生成 v4 UUID，作为检测引擎中这次请求的标识。
func t1kUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// t1kConn 是一条到检测引擎的连接。
type t1kConn struct {
	net.Conn
	// br 只在本次检测内使用，回包读完后缓冲区内不应还有数据
	br   *bufio.Reader
	used time.Time
}

// 连接池参数：空闲连接超过 t1kIdleTTL 不再复用（检测引擎可能已关闭它），最多保留 t1kMaxIdle 条
const (
	t1kIdleTTL = 30 * time.Second
	t1kMaxIdle = 64
)

// t1kClient 是检测引擎客户端，带简单的连接池，可并发使用。
type t1kClient struct {
	network, addr string
	timeout       time.Duration

	mu     sync.Mutex
	idle   []*t1kConn
	closed bool
}

// newT1KClient 创建客户端。addr 为 host:port，或 unix: 开头的 Unix 套接字路径。
func newT1KClient(addr string, timeout time.Duration) *t1kClient {
	c := &t1kClient{network: "tcp", addr: addr, timeout: timeout}
	if p, ok := strings.CutPrefix(addr, "unix:"); ok {
		c.network, c.addr = "unix", p
	}
	return c
}

// get 取一条空闲连接，没有时新建；reused 表示连接来自连接池。
func (c *t1kClient) get(deadline time.Time) (cn *t1kConn, reused bool, err error) {
	now := time.Now()
	c.mu.Lock()
	for len(c.idle) > 0 {
		cn = c.idle[len(c.idle)-1]
		c.idle = c.idle[:len(c.idle)-1]
		if now.Sub(cn.used) < t1kIdleTTL {
			c.mu.Unlock()
			return cn, true, nil
		}
		_ = cn.Close()
	}
	c.mu.Unlock()
	d := net.Dialer{Deadline: deadline}
	nc, err := d.Dial(c.network, c.addr)
	if err != nil {
		return nil, false, err
	}
	if tc, ok := nc.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return &t1kConn{Conn: nc, br: bufio.NewReader(nc)}, false, nil
}

// put 归还连接；客户端已关闭或连接池已满时直接关闭。
func (c *t1kClient) put(cn *t1kConn) {
	cn.used = time.Now()
	c.mu.Lock()
	if !c.closed && len(c.idle) < t1kMaxIdle {
		c.idle = append(c.idle, cn)
		cn = nil
	}
	c.mu.Unlock()
	if cn != nil {
		_ = cn.Close()
	}
}

// close 关闭全部空闲连接；进行中的检测结束后归还的连接也会被关闭。
func (c *t1kClient) close() {
	c.mu.Lock()
	c.closed = true
	idle := c.idle
	c.idle = nil
	c.mu.Unlock()
	for _, cn := range idle {
		_ = cn.Close()
	}
}

// roundTrip 发送一次报文并读取回包，整个过程不超过客户端超时。
// 复用的连接可能已被检测引擎关闭，失败时换一条新连接重试一次。
func (c *t1kClient) roundTrip(secs []t1kSection) (*t1kResult, error) {
	deadline := time.Now().Add(c.timeout)
	var buf bytes.Buffer
	if err := writeT1K(&buf, secs); err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		cn, reused, err := c.get(deadline)
		if err != nil {
			return nil, err
		}
		res, err := c.exchange(cn, buf.Bytes(), deadline)
		if err == nil {
			c.put(cn)
			return res, nil
		}
		// 出错的连接可能停在报文中间，不能再用
		_ = cn.Close()
		if !reused || attempt > 0 || !time.Now().Before(deadline) {
			return nil, err
		}
	}
}

func (c *t1kClient) exchange(cn *t1kConn, req []byte, deadline time.Time) (*t1kResult, error) {
	if err := cn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if _, err := cn.Write(req); err != nil {
		return nil, err
	}
	res, err := readT1K(cn.br)
	if err != nil {
		return nil, err
	}
	if cn.br.Buffered() > 0 {
		return nil, errors.New("回包后还有多余数据，连接状态异常")
	}
	return res, nil
}

// heartbeat 发送心跳（首末标志同时置位的空段），用于测试连接。
func (c *t1kClient) heartbeat() error {
	_, err := c.roundTrip([]t1kSection{{tag: 0}})
	return err
}
