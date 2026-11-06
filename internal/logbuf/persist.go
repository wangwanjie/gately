package logbuf

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/fsperm"
)

// 落盘参数
const (
	// flushInterval 为批量写盘延迟：日志先攒在内存，有新日志后最多延迟这么久追加一次，减少小写入与闪存磨损
	flushInterval = time.Second
	// maxPending 限制未写盘数据的大小，磁盘卡住时丢弃新日志，避免撑大内存
	maxPending = 4 << 20
	// flushEarlyBytes 为提前写盘的阈值：待写数据攒到这么多就立即写入，不再等满 flushInterval。
	// 访问日志在高流量下每秒可达数 MB，只按时间写盘会顶到 maxPending 丢日志
	flushEarlyBytes = 256 << 10
	// DefaultFileBytes 为单个日志文件的默认上限，超过后轮转为 <文件>.1，即每个来源最多占用两倍空间
	DefaultFileBytes = 2 << 20
)

// fileSink 把日志以 JSON Lines 追加写入文件，超过上限后轮转一份历史文件。
type fileSink struct {
	path string

	mu       sync.Mutex
	pending  []byte
	maxBytes int64
	// scheduled 表示已安排了一次延迟写盘；没有新日志时不安排，空闲时没有任何定时唤醒
	scheduled bool
	// urgent 表示已因待写数据过多发起了一次立即写盘，写盘开始前不再重复发起
	urgent bool

	// ioMu 串行化文件操作（写入、轮转、清空）
	ioMu sync.Mutex
	size int64
}

// Persist 开启日志落盘：path 为日志文件，maxBytes 为单个文件上限（<=0 时使用默认值）。
//
// 首次开启且缓冲为空时，从文件尾部恢复最近的日志，并让序号接着文件中最大的序号继续递增，
// 前端按序号增量拉取不会因为重启而错乱。已开启时只更新文件上限。
func (b *Buffer) Persist(path string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultFileBytes
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sink != nil && b.sink.path == path {
		b.sink.mu.Lock()
		b.sink.maxBytes = maxBytes
		b.sink.mu.Unlock()
		return nil
	}
	if b.sink != nil {
		// 换了文件路径：先把旧文件写完
		b.sink.close()
		b.sink = nil
	}
	if err := fsperm.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var size int64
	if st, err := os.Stat(path); err == nil {
		size = st.Size()
	}
	restored := readTail(path, len(b.entries))
	var maxSeq uint64
	for _, e := range restored {
		maxSeq = max(maxSeq, e.Seq)
	}
	if b.count == 0 {
		for _, e := range restored {
			b.entries[(b.start+b.count)%len(b.entries)] = e
			b.count++
		}
	}
	// 序号只增不减：运行中开启落盘时，文件里可能有更大的旧序号
	if maxSeq >= b.nextSeq {
		b.nextSeq = maxSeq + 1
	}
	b.sink = &fileSink{path: path, maxBytes: maxBytes, size: size}
	return nil
}

// ClosePersist 把未写盘的日志写入文件并停止落盘，之后日志只保留在内存。未开启时什么也不做。
func (b *Buffer) ClosePersist() {
	b.mu.Lock()
	s := b.sink
	b.sink = nil
	b.mu.Unlock()
	if s != nil {
		s.close()
	}
}

// add 序列化一条日志追加到待写盘数据。调用方持有 Buffer.mu，保证文件中的顺序与序号一致。
func (s *fileSink) add(e Entry) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	s.mu.Lock()
	if len(s.pending)+len(line) < maxPending {
		s.pending = append(s.pending, line...)
		s.pending = append(s.pending, '\n')
	}
	if !s.scheduled {
		s.scheduled = true
		time.AfterFunc(flushInterval, s.flush)
	}
	if len(s.pending) >= flushEarlyBytes && !s.urgent {
		s.urgent = true
		go s.flush()
	}
	s.mu.Unlock()
}

// close 立即写入剩余日志。之后仍可能触发已安排的延迟写盘，那时没有待写数据，什么也不做。
func (s *fileSink) close() {
	s.flush()
}

// flush 把待写盘数据追加到文件，写入前按需轮转。写入失败只丢弃这批日志。
func (s *fileSink) flush() {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.flushLocked()
}

// flushLocked 是 flush 的实际实现，调用方需持有 ioMu。
func (s *fileSink) flushLocked() {
	s.mu.Lock()
	data := s.pending
	s.pending = nil
	s.scheduled = false
	s.urgent = false
	limit := s.maxBytes
	s.mu.Unlock()
	if len(data) == 0 {
		return
	}
	if s.size > 0 && s.size+int64(len(data)) > limit {
		// 只保留一份历史文件：<文件> -> <文件>.1，覆盖更旧的
		if err := os.Rename(s.path, s.path+".1"); err == nil || errors.Is(err, os.ErrNotExist) {
			s.size = 0
		}
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("写入日志文件 %s 失败: %v", s.path, err)
		return
	}
	if s.size == 0 {
		// 新建（或轮转、清空后重建）的文件显式设置权限，部分文件系统会忽略创建权限，见 fsperm
		_ = f.Chmod(0o600)
	}
	n, err := f.Write(data)
	s.size += int64(n)
	_ = f.Close()
	if err != nil {
		log.Printf("写入日志文件 %s 失败: %v", s.path, err)
	}
}

// clear 丢弃未写盘数据并删除日志文件。
func (s *fileSink) clear() {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
	_ = os.Remove(s.path + ".1")
	_ = os.Remove(s.path)
	s.size = 0
}

// readTail 按时间顺序返回日志文件（含 .1 历史文件）中最近的至多 n 条日志。
func readTail(path string, n int) []Entry {
	if n <= 0 {
		return nil
	}
	cur := tailEntries(path, n)
	if len(cur) >= n {
		return cur
	}
	old := tailEntries(path+".1", n-len(cur))
	return append(old, cur...)
}

// tailChunk 为从文件末尾向前读取的块大小
const tailChunk = 64 << 10

// tailEntries 从文件末尾向前读取，解析出最近的至多 n 条日志；无法解析的行（例如断电写了一半）跳过。
func tailEntries(path string, n int) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	// 从末尾按块向前读，直到凑够 n 行或读到文件开头
	var data []byte
	off := st.Size()
	for off > 0 && bytes.Count(data, []byte{'\n'}) <= n {
		step := min(int64(tailChunk), off)
		off -= step
		chunk := make([]byte, step)
		if _, err := f.ReadAt(chunk, off); err != nil && !errors.Is(err, io.EOF) {
			return nil
		}
		data = append(chunk, data...)
	}
	lines := bytes.Split(data, []byte{'\n'})
	if off > 0 && len(lines) > 0 {
		// 没读到文件开头时，第一行可能不完整
		lines = lines[1:]
	}
	out := make([]Entry, 0, min(n, len(lines)))
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		var e Entry
		if len(lines[i]) == 0 || json.Unmarshal(lines[i], &e) != nil || e.Seq == 0 {
			continue
		}
		out = append(out, e)
	}
	// 上面是倒序收集的，翻转为时间顺序
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
