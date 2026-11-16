// Package logbuf 提供带序号的内存环形日志缓冲，供前端增量拉取。
//
// 开启落盘（Persist）后，日志同时以 JSON Lines 批量追加到文件，重启后从文件恢复最近的日志。
package logbuf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// 日志级别
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Entry 是一条日志。Seq 单调递增，前端用它做增量拉取。
type Entry struct {
	Seq     uint64    `json:"seq"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Source  string    `json:"source,omitempty"`
	Message string    `json:"message"`
	// Fields 为可选的结构化字段（客户端 IP、域名、状态码等），用于按字段过滤
	Fields Fields `json:"fields,omitempty"`
}

// Buffer 是固定容量的环形缓冲，写满后覆盖最旧的日志。
type Buffer struct {
	mu sync.Mutex
	// entries 按写入量增长到 size 条，之后作为环形缓冲覆盖最旧的日志：大多数模块的日志远少于上限，不必一开始就占满
	entries []Entry
	size    int
	start   int // 最旧一条在 entries 中的下标；未写满时为 0
	count   int // 已有的日志条数，与 len(entries) 相同
	nextSeq uint64
	// echo 不为 nil 时同时输出到标准日志，便于 docker logs 查看
	echo *log.Logger
	// sink 不为 nil 时日志同时写入文件，见 Persist；persisting 与它同步变化，写日志时不加锁就能知道是否需要序列化
	sink       *fileSink
	persisting atomic.Bool
	// sources 为出现过的来源标签（不含空串），供日志页的来源下拉列出全部来源，见 Sources。
	// 开启落盘时同时保存到文件，重启后保留；数量受 maxSources 限制
	sources map[string]struct{}
}

// maxSources 限制记录的来源数量。来源来自监听器、规则等配置名称，正常远小于该值，
// 上限只是防止异常情况下无限增长
const maxSources = 1000

// New 创建容量为 size 的缓冲。
func New(size int, echo *log.Logger) *Buffer {
	if size <= 0 {
		size = 500
	}
	return &Buffer{size: size, nextSeq: 1, echo: echo, sources: map[string]struct{}{}}
}

// Add 追加一条日志。
func (b *Buffer) Add(level, source, msg string) {
	b.AddFields(level, source, msg, nil)
}

// AddFields 追加一条带结构化字段的日志，fields 写入后不应再修改。
func (b *Buffer) AddFields(level, source, msg string, fields Fields) {
	b.addAt(time.Now(), level, source, msg, fields)
}

// addAt 以指定时间追加一条日志，测试用它模拟跨天写入。
func (b *Buffer) addAt(t time.Time, level, source, msg string, fields Fields) {
	b.add(Entry{Time: t, Level: level, Source: source, Message: msg, Fields: fields})
}

// lineEncoder 把一条日志序列化为一行 JSON，缓冲区随对象池复用。
type lineEncoder struct {
	buf bytes.Buffer
	enc *json.Encoder
	// e 为正在序列化的日志：放在可复用的对象里，不必为每条日志单独分配
	e Entry
}

var lineEncoders = sync.Pool{New: func() any {
	le := &lineEncoder{}
	le.enc = json.NewEncoder(&le.buf)
	return le
}}

// lineEncoderKeep 以上的缓冲区不放回对象池，避免个别超长日志让池里长期留着大块内存
const lineEncoderKeep = 16 << 10

// encodeLine 把 e 序列化为一行 JSON（序号写成 0），失败时返回 nil。
func encodeLine(e Entry) *lineEncoder {
	le := lineEncoders.Get().(*lineEncoder)
	le.buf.Reset()
	le.e = e
	le.e.Seq = 0
	err := le.enc.Encode(&le.e)
	// 不留着对消息与字段表的引用
	le.e = Entry{}
	if err != nil {
		lineEncoders.Put(le)
		return nil
	}
	return le
}

// add 追加一条日志，序号由缓冲分配（e.Seq 被忽略）。
func (b *Buffer) add(e Entry) {
	// 落盘的 JSON 在加锁之前序列化（序号先写 0，加锁后再填入真正的序号）：
	// 序列化是写一条日志里最耗时的一步，放在锁内会让高并发下所有写日志的协程排队
	var le *lineEncoder
	if b.persisting.Load() {
		le = encodeLine(e)
	}
	b.mu.Lock()
	e.Seq = b.nextSeq
	b.nextSeq++
	if len(b.entries) < b.size {
		if len(b.entries) == cap(b.entries) {
			// 按需扩容，但不超过容量上限
			next := make([]Entry, len(b.entries), min(max(16, 2*len(b.entries)), b.size))
			copy(next, b.entries)
			b.entries = next
		}
		b.entries = append(b.entries, e)
		b.count++
	} else {
		b.entries[b.start] = e
		b.start = (b.start + 1) % len(b.entries)
	}
	// 第一次出现的来源记下来；map 查找很快，不影响写入性能
	if e.Source != "" {
		if _, ok := b.sources[e.Source]; !ok && len(b.sources) < maxSources {
			b.sources[e.Source] = struct{}{}
			if b.sink != nil {
				b.sink.addSource(e.Source)
			}
		}
	}
	// 在锁内交给落盘，保证文件中的顺序与序号一致
	if b.sink != nil {
		if le == nil || !b.sink.addEncoded(e.Seq, e.Time, le.buf.Bytes()) {
			// 写入前一刻才开启落盘，或预先序列化的内容不是预期的形状：在锁内序列化
			b.sink.add(e)
		}
	}
	b.mu.Unlock()
	if le != nil && le.buf.Cap() <= lineEncoderKeep {
		lineEncoders.Put(le)
	}
	if b.echo != nil {
		if e.Source != "" {
			b.echo.Printf("[%s] [%s] %s", e.Level, e.Source, e.Message)
		} else {
			b.echo.Printf("[%s] %s", e.Level, e.Message)
		}
	}
}

// Since 返回序号大于 seq 的全部日志（按时间顺序）。seq 为 0 时返回全部。
func (b *Buffer) Since(seq uint64) []Entry {
	return b.Range(seq, 0, 0)
}

// Range 返回序号在 (after, before) 之间的日志，before 为 0 表示不限；limit 大于 0 时只保留其中最新的 limit 条。
// 缓冲内日志按序号递增排列，从最新一条往前扫描，只为结果分配内存，增量轮询时不会按整个缓冲大小分配。
func (b *Buffer) Range(after, before uint64, limit int) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	// 先从新到旧找出结果区间 [lo, hi)（相对 start 的偏移）
	hi := b.count
	for hi > 0 && before > 0 && b.at(hi-1).Seq >= before {
		hi--
	}
	lo := hi
	for lo > 0 && b.at(lo-1).Seq > after {
		if limit > 0 && hi-lo >= limit {
			break
		}
		lo--
	}
	out := make([]Entry, 0, hi-lo)
	for i := lo; i < hi; i++ {
		out = append(out, b.at(i))
	}
	return out
}

// Cap 返回缓冲的容量上限（条数）。
func (b *Buffer) Cap() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size
}

// at 返回从最旧一条起第 i 条日志，调用方需持有锁。
func (b *Buffer) at(i int) Entry {
	return b.entries[(b.start+i)%len(b.entries)]
}

// Sources 按名称排序返回出现过的来源标签：本次运行中写过日志的来源，
// 以及开启落盘后从来源记录文件与恢复的日志中读到的来源。
func (b *Buffer) Sources() []string {
	b.mu.Lock()
	out := make([]string, 0, len(b.sources))
	for s := range b.sources {
		out = append(out, s)
	}
	b.mu.Unlock()
	sort.Strings(out)
	return out
}

// Clear 清空缓冲（开启落盘时同时删除日志文件与归档），序号继续递增，保证前端增量拉取不会错乱。
// 记录的来源一并清空：日志已经删除，再列出这些来源也查不到内容。
func (b *Buffer) Clear() {
	b.mu.Lock()
	// 连同底层数组一起释放，清空后不再占着旧日志的内存
	b.entries = nil
	b.start = 0
	b.count = 0
	b.sources = map[string]struct{}{}
	sink := b.sink
	b.mu.Unlock()
	if sink != nil {
		sink.clear()
	}
}

// Logger 是绑定了来源标签的便捷写入器。
type Logger struct {
	buf    *Buffer
	source string
	// fields 为 With 附加的结构化字段，随每条日志写入
	fields Fields
}

// Logger 返回带来源标签的写入器，例如每条转发规则一个。
func (b *Buffer) Logger(source string) *Logger {
	return &Logger{buf: b, source: source}
}

// With 返回附带结构化字段的写入器，用于单条日志，例如 log.With(logbuf.Fields{"ip": ip}).Infof(...)。
// 空值的字段会被忽略。
func (l *Logger) With(fields Fields) *Logger {
	merged := make(Fields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		if v != "" {
			merged[k] = v
		}
	}
	return &Logger{buf: l.buf, source: l.source, fields: merged}
}

// Source 返回日志来源标签。
func (l *Logger) Source() string {
	return l.source
}

// Log 以指定级别写入一条已格式化的日志，供托管外部进程的输出使用。
func (l *Logger) Log(level, msg string) {
	l.buf.AddFields(level, l.source, msg, l.fields)
}

// Debugf 写入调试日志。
func (l *Logger) Debugf(format string, args ...any) {
	l.buf.AddFields(LevelDebug, l.source, fmt.Sprintf(format, args...), l.fields)
}

// Infof 写入普通日志。
func (l *Logger) Infof(format string, args ...any) {
	l.buf.AddFields(LevelInfo, l.source, fmt.Sprintf(format, args...), l.fields)
}

// Warnf 写入警告日志。
func (l *Logger) Warnf(format string, args ...any) {
	l.buf.AddFields(LevelWarn, l.source, fmt.Sprintf(format, args...), l.fields)
}

// Errorf 写入错误日志。
func (l *Logger) Errorf(format string, args ...any) {
	l.buf.AddFields(LevelError, l.source, fmt.Sprintf(format, args...), l.fields)
}

// Resize 修改缓冲容量，保留最新的日志。
func (b *Buffer) Resize(size int) {
	if size <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if size == b.size {
		return
	}
	keep := min(b.count, size)
	next := make([]Entry, keep)
	for i := 0; i < keep; i++ {
		next[i] = b.entries[(b.start+b.count-keep+i)%len(b.entries)]
	}
	b.entries, b.size, b.start, b.count = next, size, 0, keep
}
