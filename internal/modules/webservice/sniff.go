package webservice

import (
	"crypto/tls"
	"io"
	"net"
	"sync"
	"time"
)

// sniffTimeout 是等待客户端发出第一个字节的时长，与 http.Server 的 ReadHeaderTimeout 一致；测试中会调小
var sniffTimeout = 30 * time.Second

// tlsRecordHandshake 是 TLS 握手记录的类型字节，客户端发起 TLS 连接时第一个字节总是它；
// 明文 HTTP 请求以方法名开头，第一个字节是 ASCII 字母，两者不会混淆
const tlsRecordHandshake = 0x16

// sniffListener 让开启 HTTPS 的监听同时接受明文 HTTP：按连接的第一个字节区分协议，
// TLS 连接包装为 *tls.Conn 交给 http.Server（由它完成握手并设置 r.TLS），其余连接按明文 HTTP 处理。
// 读取第一个字节在独立的 goroutine 中进行，迟迟不发数据的客户端不会阻塞其他连接的接入。
type sniffListener struct {
	net.Listener
	tlsCfg *tls.Config
	conns  chan net.Conn
	errs   chan error
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	// pending 为还在等待第一个字节的连接，关闭监听时一并关闭
	pending map[net.Conn]struct{}
}

func newSniffListener(inner net.Listener, cfg *tls.Config) *sniffListener {
	l := &sniffListener{
		Listener: inner,
		tlsCfg:   cfg,
		conns:    make(chan net.Conn),
		errs:     make(chan error),
		done:     make(chan struct{}),
		pending:  map[net.Conn]struct{}{},
	}
	go l.acceptLoop()
	return l
}

// acceptLoop 持续接入原始连接，交给 classify 识别协议；Accept 出错时把错误转交给调用方，
// 由 http.Server 决定重试（临时错误）还是退出。
func (l *sniffListener) acceptLoop() {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			select {
			case l.errs <- err:
			case <-l.done:
				return
			}
			continue
		}
		if !l.track(c) {
			c.Close()
			return
		}
		go l.classify(c)
	}
}

// track 登记等待识别的连接；监听已关闭时返回 false。
func (l *sniffListener) track(c net.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending == nil {
		return false
	}
	l.pending[c] = struct{}{}
	return true
}

// untrack 移除登记；监听已关闭（连接已被关闭）时返回 false。
func (l *sniffListener) untrack(c net.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending == nil {
		return false
	}
	delete(l.pending, c)
	return true
}

// classify 读取第一个字节判断协议，再把连接交给 Accept。
func (l *sniffListener) classify(c net.Conn) {
	var first [1]byte
	_ = c.SetReadDeadline(time.Now().Add(sniffTimeout))
	_, err := io.ReadFull(c, first[:])
	if !l.untrack(c) {
		return
	}
	if err != nil {
		// 超时或客户端未发数据就断开，直接丢弃
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	var out net.Conn = &prefixConn{Conn: c, prefix: first[:]}
	if first[0] == tlsRecordHandshake {
		out = tls.Server(out, l.tlsCfg)
	}
	select {
	case l.conns <- out:
	case <-l.done:
		c.Close()
	}
}

func (l *sniffListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case err := <-l.errs:
		return nil, err
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *sniffListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() {
		close(l.done)
		l.mu.Lock()
		for c := range l.pending {
			c.Close()
		}
		l.pending = nil
		l.mu.Unlock()
	})
	return err
}

// prefixConn 先返回已预读的字节，再继续读取原连接。
type prefixConn struct {
	net.Conn
	prefix []byte
}

// NetConn 返回被包装的连接，访问路径追溯（hops.RawPeer）据此取得最底层的 TCP 对端。
func (c *prefixConn) NetConn() net.Conn { return c.Conn }

func (c *prefixConn) Read(b []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(b, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}
