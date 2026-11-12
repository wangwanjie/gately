package app

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// 管理端口只设置了 ReadHeaderTimeout，请求头读完后客户端可以任意缓慢地发送请求体（慢速上传攻击）。
// 这里按请求限制请求体的读取：普通接口的请求体都是小型 JSON，要求在较短时间内读完；
// 备份恢复、升级包、文件上传等大文件接口不限总时长（慢速链路上的大文件也能传完），但仍有读取间隔与最低平均速率。
// 不使用 http.Server.ReadTimeout：它是整条连接的绝对期限，无法按路径区分。
var (
	// 普通接口：两次读取之间的最长间隔，与请求体读取的总时长上限
	bodyIdleTimeout  = 15 * time.Second
	bodyTotalTimeout = 60 * time.Second
	// 大文件接口：读取间隔、总时长上限（0 表示不限）、宽限期过后的最低平均速率（字节/秒）
	bulkIdleTimeout  = 30 * time.Second
	bulkTotalTimeout = time.Duration(0)
	bulkRateGrace    = 30 * time.Second
	bulkMinRate      = int64(4 << 10)
	// 处理函数返回后，服务器丢弃未读完请求体的最长时间
	bodyDrainTimeout = 5 * time.Second
)

// errBodyTooSlow 表示请求体发送过慢或总时长超限。
var errBodyTooSlow = errors.New("请求体发送过慢或超时")

// bulkUploadPaths 是接收大文件的接口（精确路径）。
var bulkUploadPaths = map[string]bool{
	"/api/system/restore":        true,
	"/api/system/upgrade/upload": true,
	"/api/import/lucky":          true,
	"/api/docker/images/load":    true,
	"/api/docker/images/build":   true,
}

// isBulkUpload 判断请求是否属于大文件上传：精确路径，或各模块按约定命名的 upload、write 与数据卷 import 接口。
func isBulkUpload(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		return false
	}
	p := r.URL.Path
	if bulkUploadPaths[p] {
		return true
	}
	if !strings.HasPrefix(p, "/api/") {
		return false
	}
	if strings.HasSuffix(p, "/upload") || strings.HasSuffix(p, "/files/write") || strings.HasSuffix(p, "/geoip/upload") {
		return true
	}
	// SFTP 写入文件与 Docker 数据卷导入
	if strings.Contains(p, "/sftp/") && strings.HasSuffix(p, "/write") {
		return true
	}
	return strings.HasPrefix(p, "/api/docker/volumes/") && strings.HasSuffix(p, "/import")
}

// limitBodyRead 为带请求体的请求设置读取期限。没有请求体的请求（含 WebSocket 升级与长连接流式响应）不受影响。
func limitBodyRead(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
			next.ServeHTTP(w, r)
			return
		}
		rc := http.NewResponseController(w)
		gb := &guardedBody{ReadCloser: r.Body, rc: rc, start: time.Now(), idle: bodyIdleTimeout, total: bodyTotalTimeout, length: r.ContentLength}
		if isBulkUpload(r) {
			gb.idle, gb.total, gb.grace, gb.minRate = bulkIdleTimeout, bulkTotalTimeout, bulkRateGrace, bulkMinRate
		}
		r.Body = gb
		// 处理函数返回后限定服务器丢弃剩余请求体的时间；下一个请求开始时会重新设置读取期限
		defer func() {
			if !gb.done {
				_ = rc.SetReadDeadline(time.Now().Add(bodyDrainTimeout))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// guardedBody 在每次读取前设置连接读取期限，并检查总时长与最低平均速率。
type guardedBody struct {
	io.ReadCloser
	rc          *http.ResponseController
	start       time.Time
	idle, total time.Duration
	// grace 之后平均速率低于 minRate（字节/秒）视为过慢，minRate 为 0 表示不检查
	grace   time.Duration
	minRate int64
	n       int64
	// length 为声明的请求体长度，-1 表示未知（分块传输）
	length int64
	// done 表示请求体已读完或已关闭，连接的读取期限已清除
	done bool
}

func (b *guardedBody) Read(p []byte) (int, error) {
	if b.done {
		// 已读完并清除了读取期限：再读一次（处理函数确认 EOF）不能重新设置期限，否则到点会取消请求上下文
		return b.ReadCloser.Read(p)
	}
	now := time.Now()
	elapsed := now.Sub(b.start)
	if b.total > 0 && elapsed > b.total {
		return 0, errBodyTooSlow
	}
	// 按整秒比较平均速率；不能写成 n*1e9，已读超过约 8.6 GiB 时会溢出成负数而被误判为过慢
	if b.minRate > 0 && elapsed > b.grace && b.n < b.minRate*int64(elapsed/time.Second) {
		return 0, errBodyTooSlow
	}
	deadline := now.Add(b.idle)
	if b.total > 0 {
		if limit := b.start.Add(b.total); deadline.After(limit) {
			deadline = limit
		}
	}
	_ = b.rc.SetReadDeadline(deadline)
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	if err != nil {
		b.finish(err)
	} else if b.length > 0 && b.n >= b.length {
		// 已读满声明的长度：处理函数（如 JSON 解码器）可能不再多读一次来取得 EOF
		b.finish(io.EOF)
	}
	return n, err
}

// finish 在读完（EOF）后清除读取期限：之后服务器会在后台监视连接，残留的期限到点会取消请求上下文，
// 打断流式响应与长时间运行的处理。读取出错时保留期限，由处理函数返回后的丢弃时限收尾。
func (b *guardedBody) finish(err error) {
	if errors.Is(err, io.EOF) && !b.done {
		b.done = true
		_ = b.rc.SetReadDeadline(time.Time{})
	}
}
