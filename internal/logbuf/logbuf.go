// Package logbuf 提供带序号的内存环形日志缓冲，供前端增量拉取。
//
// 开启落盘（Persist）后，日志同时以 JSON Lines 批量追加到文件，重启后从文件恢复最近的日志。
package logbuf

import (
	"fmt"
	"log"
	"sort"
	"sync"
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
	mu      sync.Mutex
	entries []Entry
	start   int // 最旧一条在 entries 中的下标
	count   int
	nextSeq uint64
	// echo 不为 nil 时同时输出到标准日志，便于 docker logs 查看
	echo *log.Logger
	// sink 不为 nil 时日志同时写入文件，见 Persist
	sink *fileSink
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
	return &Buffer{entries: make([]Entry, size), nextSeq: 1, echo: echo, sources: map[string]struct{}{}}
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
	b.mu.Lock()
	e := Entry{Seq: b.nextSeq, Time: t, Level: level, Source: source, Message: msg, Fields: fields}
	b.nextSeq++
	idx := (b.start + b.count) % len(b.entries)
	b.entries[idx] = e
	if b.count < len(b.entries) {
		b.count++
	} else {
		b.start = (b.start + 1) % len(b.entries)
	}
	// 第一次出现的来源记下来；map 查找很快，不影响写入性能
	if source != "" {
		if _, ok := b.sources[source]; !ok && len(b.sources) < maxSources {
			b.sources[source] = struct{}{}
			if b.sink != nil {
				b.sink.addSource(source)
			}
		}
	}
	// 在锁内交给落盘，保证文件中的顺序与序号一致
	if b.sink != nil {
		b.sink.add(e)
	}
	b.mu.Unlock()
	if b.echo != nil {
		if source != "" {
			b.echo.Printf("[%s] [%s] %s", level, source, msg)
		} else {
			b.echo.Printf("[%s] %s", level, msg)
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

// Clear 清空缓冲（开启落盘时同时删除日志文件与来源记录），序号继续递增，保证前端增量拉取不会错乱。
// 记录的来源一并清空：日志已经删除，再列出这些来源也查不到内容。
func (b *Buffer) Clear() {
	b.mu.Lock()
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
	if size == len(b.entries) {
		return
	}
	keep := min(b.count, size)
	next := make([]Entry, size)
	for i := 0; i < keep; i++ {
		next[i] = b.entries[(b.start+b.count-keep+i)%len(b.entries)]
	}
	b.entries, b.start, b.count = next, 0, keep
}
