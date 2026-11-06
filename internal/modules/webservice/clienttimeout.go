package webservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// ClientTimeouts 是对访客的空闲超时，单位为秒，0 表示使用默认值 60 秒（与 nginx 的 client_body_timeout、send_timeout 默认值相同）。
// 只在一次读取或写出卡住时计时：数据持续流动的大文件上传下载、等待后端处理的时间、WebSocket 等长连接都不受影响，
// 用于防止访客极慢地发送请求体或极慢地读取响应（slowloris、slow read）长期占住连接。
type ClientTimeouts struct {
	// ReadIdle 为读取请求体时一次读取最多等待的秒数
	ReadIdle int `json:"readIdle"`
	// WriteIdle 为写出响应时一次写出最多等待的秒数
	WriteIdle int `json:"writeIdle"`
}

const (
	defaultClientIdle = 60 * time.Second
	maxClientIdle     = 3600
)

// validate 校验范围。0 为默认值，不改写，以便日后调整默认值时旧配置自动跟随。
func (t ClientTimeouts) validate() error {
	if t.ReadIdle < 0 || t.ReadIdle > maxClientIdle {
		return fmt.Errorf("读取请求体超时需在 1 到 %d 秒之间（0 为默认 60 秒）", maxClientIdle)
	}
	if t.WriteIdle < 0 || t.WriteIdle > maxClientIdle {
		return fmt.Errorf("写出响应超时需在 1 到 %d 秒之间（0 为默认 60 秒）", maxClientIdle)
	}
	return nil
}

func (t ClientTimeouts) readIdle() time.Duration {
	if t.ReadIdle > 0 {
		return time.Duration(t.ReadIdle) * time.Second
	}
	return defaultClientIdle
}

func (t ClientTimeouts) writeIdle() time.Duration {
	if t.WriteIdle > 0 {
		return time.Duration(t.WriteIdle) * time.Second
	}
	return defaultClientIdle
}

// clientDeadlines 在每次读取请求体、写出响应前设置截止时间，结束后立即清除。
// 清除很重要：HTTP/1 读完请求体后 net/http 会在同一连接上做后台读取以发现访客断开，残留的读截止时间会让它误判断开并取消请求；
// HTTP/2 的截止时间是请求流上的定时器，残留时即使没有读写也会重置请求流。HTTP/1、HTTP/2 与 HTTP/3 都支持；不支持截止时间的连接不做限制。
type clientDeadlines struct {
	rc        *http.ResponseController
	read      time.Duration
	write     time.Duration
	noRead    atomic.Bool
	noWrite   atomic.Bool
	readTimed atomic.Bool
}

func newClientDeadlines(w http.ResponseWriter, t ClientTimeouts) *clientDeadlines {
	return &clientDeadlines{rc: http.NewResponseController(w), read: t.readIdle(), write: t.writeIdle()}
}

// armRead 设置读截止时间，返回是否设置成功。
func (d *clientDeadlines) armRead() bool {
	if d.noRead.Load() || d.readTimed.Load() {
		return false
	}
	if err := d.rc.SetReadDeadline(time.Now().Add(d.read)); err != nil {
		d.noRead.Store(true)
		return false
	}
	return true
}

func (d *clientDeadlines) clearRead() {
	_ = d.rc.SetReadDeadline(time.Time{})
}

// armWrite 设置写截止时间，返回是否设置成功。
func (d *clientDeadlines) armWrite() bool {
	if d.noWrite.Load() {
		return false
	}
	if err := d.rc.SetWriteDeadline(time.Now().Add(d.write)); err != nil {
		d.noWrite.Store(true)
		return false
	}
	return true
}

func (d *clientDeadlines) clearWrite() {
	_ = d.rc.SetWriteDeadline(time.Time{})
}

// readTimedOut 表示读取请求体因访客空闲超时而失败，此时后端没有过错，不计入被动检查，返回 408。
func (d *clientDeadlines) readTimedOut() bool {
	return d != nil && d.readTimed.Load()
}

type ctxKeyClientDeadlines struct{}

// clientDeadlinesFrom 取出请求的访客超时状态，可为 nil。
func clientDeadlinesFrom(ctx context.Context) *clientDeadlines {
	d, _ := ctx.Value(ctxKeyClientDeadlines{}).(*clientDeadlines)
	return d
}

// idleBody 为请求体的每次读取设置空闲超时。
type idleBody struct {
	io.ReadCloser
	d *clientDeadlines
}

func (b *idleBody) Read(p []byte) (int, error) {
	armed := b.d.armRead()
	n, err := b.ReadCloser.Read(p)
	var ne net.Error
	if armed && err != nil && errors.As(err, &ne) && ne.Timeout() {
		// 超时后保持截止时间已过：net/http 回复前会尝试读掉剩余的请求体，没有截止时间就会一直等待停住的访客；
		// 读取失败后服务器回复完即关闭连接
		b.d.readTimed.Store(true)
		_ = b.d.rc.SetReadDeadline(time.Now())
		return n, err
	}
	if armed {
		b.d.clearRead()
	}
	return n, err
}
