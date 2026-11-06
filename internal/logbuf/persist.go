package logbuf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
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
	// 访问日志在高流量下每秒可达数 MB，只按时间写盘会顶到 maxPending 丢日志，单次写入也会远超文件的切分大小
	flushEarlyBytes = 256 << 10
	// DefaultFileBytes 为单个日志文件的默认上限，超过后轮转为 <文件>.1，即每个来源最多占用两倍空间
	DefaultFileBytes = 2 << 20
	// DefaultKeepBytes 为归档模式下当前文件与全部归档的默认总大小上限
	DefaultKeepBytes = 32 << 20
	// 归档模式下当前文件的轮转大小取总上限的四分之一，并限制在这个区间内：
	// 太小会产生大量小归档，太大则单个归档解压查找太慢、清理粒度太粗
	minArchiveFileBytes = 1 << 20
	maxArchiveFileBytes = 16 << 20
)

// PersistOptions 是日志落盘参数。
type PersistOptions struct {
	// FileBytes 为当前日志文件的大小上限，超过后轮转；<=0 时使用默认值
	// （普通模式为 DefaultFileBytes，归档模式按 KeepBytes 计算）
	FileBytes int64
	// Archive 为 true 时开启归档：当前文件跨天或超过上限时改名为 <名称>.<开始时间>.log 并在后台压缩为 .log.gz，
	// 按 KeepDays 与 KeepBytes 清理最旧的归档，检索与导出会查找全部归档。
	// 为 false 时只保留一份 <文件>.1 历史文件
	Archive bool
	// KeepDays 为归档的保留天数，0 表示不按天数清理
	KeepDays int
	// KeepBytes 为当前文件与全部归档（压缩后）的总大小上限，<=0 时使用 DefaultKeepBytes
	KeepBytes int64
}

// normalized 返回补齐默认值后的参数。
func (o PersistOptions) normalized() PersistOptions {
	if !o.Archive {
		o.KeepDays, o.KeepBytes = 0, 0
		if o.FileBytes <= 0 {
			o.FileBytes = DefaultFileBytes
		}
		return o
	}
	if o.KeepBytes <= 0 {
		o.KeepBytes = DefaultKeepBytes
	}
	if o.KeepDays < 0 {
		o.KeepDays = 0
	}
	if o.FileBytes <= 0 {
		o.FileBytes = min(max(o.KeepBytes/4, minArchiveFileBytes), maxArchiveFileBytes)
	}
	return o
}

// fileSink 把日志以 JSON Lines 追加写入文件，超过上限后轮转：普通模式只保留一份历史文件，
// 归档模式按时间命名、压缩并按保留策略清理，见 PersistOptions。
type fileSink struct {
	path string

	mu      sync.Mutex
	pending []byte
	// pendingFirst 为待写盘数据中第一条日志的时间，归档模式据此判断是否跨天
	pendingFirst time.Time
	opt          PersistOptions
	// scheduled 表示已安排了一次延迟写盘；没有新日志时不安排，空闲时没有任何定时唤醒
	scheduled bool
	// urgent 表示已因待写数据过多发起了一次立即写盘，写盘开始前不再重复发起
	urgent bool
	// sources 为出现过的来源，sourcesDirty 表示有新来源尚未写入来源记录文件
	sources      map[string]struct{}
	sourcesDirty bool

	// ioMu 串行化当前文件的操作（写入、轮转、清空）
	ioMu sync.Mutex
	size int64
	// start 为当前文件第一条日志的时间，归档时用作归档名称；未知时为零值
	start time.Time

	// archMu 串行化归档的压缩、清理与清空。加锁顺序为 archMu -> ioMu，持有 ioMu 时不能再取 archMu
	archMu sync.Mutex
	// bg 跟踪后台的压缩与清理，测试据此等待其完成
	bg sync.WaitGroup
}

// goMaintain 在后台执行一次 maintain。
func (s *fileSink) goMaintain() {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		s.maintain()
	}()
}

// Persist 开启日志落盘：path 为日志文件，maxBytes 为单个文件上限（<=0 时使用默认值），只保留一份历史文件。
func (b *Buffer) Persist(path string, maxBytes int64) error {
	return b.PersistWith(path, PersistOptions{FileBytes: maxBytes})
}

// PersistWith 按 opt 开启日志落盘，path 为当前日志文件。
//
// 首次开启且缓冲为空时，从文件（归档模式下包括最近的归档）恢复最近的日志，并让序号接着文件中最大的序号继续递增，
// 前端按序号增量拉取不会因为重启而错乱。同一路径已开启时只更新参数。
func (b *Buffer) PersistWith(path string, opt PersistOptions) error {
	opt = opt.normalized()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sink != nil && b.sink.path == path {
		b.sink.mu.Lock()
		changed := b.sink.opt != opt
		b.sink.opt = opt
		b.sink.mu.Unlock()
		if changed && opt.Archive {
			// 保留策略变了，按新策略清理一次
			b.sink.goMaintain()
		}
		return nil
	}
	if b.sink != nil {
		// 换了文件路径：先把旧文件写完
		b.sink.close()
		b.sink = nil
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	s := &fileSink{path: path, opt: opt, sources: map[string]struct{}{}}
	// 只认普通文件：同名的符号链接不会被写入（见 oNoFollow），也不计入大小
	if st, err := os.Lstat(path); err == nil && st.Mode().IsRegular() {
		s.size = st.Size()
		s.start = firstEntryTime(path)
	}
	// 缓冲已有日志（运行中换了日志目录）时只需要文件里最大的序号，读一条即可
	want := 1
	if b.count == 0 {
		want = len(b.entries)
	}
	var restored []Entry
	if opt.Archive {
		// 先按保留策略删掉过期的归档再恢复，已经过期的日志不会再进入内存缓冲。
		// 这时 sink 还没有交给其他协程，同步执行即可（只删文件，耗时的压缩仍放在后台）
		s.archMu.Lock()
		s.prune()
		s.archMu.Unlock()
		restored = s.restore(want)
	} else {
		restored = readTail(path, want)
	}
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
	// 来源：已记录的来源文件、恢复的日志与本次运行已出现的来源合在一起
	for _, src := range readSources(s.sourcesPath()) {
		if len(b.sources) < maxSources {
			b.sources[src] = struct{}{}
		}
	}
	for _, e := range restored {
		if e.Source != "" && len(b.sources) < maxSources {
			b.sources[e.Source] = struct{}{}
		}
	}
	for src := range b.sources {
		s.sources[src] = struct{}{}
	}
	s.sourcesDirty = len(s.sources) > 0
	b.sink = s
	if opt.Archive {
		// 压缩上次退出时没压缩完的归档，并按保留策略清理
		s.goMaintain()
	}
	return nil
}

// ensureDir 确保日志目录存在：不存在时创建并设为 0700；已存在的目录（可能是用户指定的共享目录）不改权限，
// 日志文件本身总是 0600。配置目录下默认的 logs/ 由启动时的权限收紧处理。
func ensureDir(dir string) error {
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return &os.PathError{Op: "mkdir", Path: dir, Err: syscall.ENOTDIR}
		}
		return nil
	}
	return fsperm.MkdirAll(dir, 0o700)
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

// stem 返回日志文件去掉 .log 后缀的路径，归档与来源记录文件都以它为前缀。
func (s *fileSink) stem() string {
	return strings.TrimSuffix(s.path, ".log")
}

// sourcesPath 返回来源记录文件的路径。
func (s *fileSink) sourcesPath() string {
	return s.stem() + ".sources"
}

// add 序列化一条日志追加到待写盘数据。调用方持有 Buffer.mu，保证文件中的顺序与序号一致。
func (s *fileSink) add(e Entry) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	s.mu.Lock()
	if len(s.pending)+len(line) < maxPending {
		if len(s.pending) == 0 {
			s.pendingFirst = e.Time
		}
		s.pending = append(s.pending, line...)
		s.pending = append(s.pending, '\n')
	}
	s.scheduleLocked()
	if len(s.pending) >= flushEarlyBytes && !s.urgent {
		s.urgent = true
		go s.flush()
	}
	s.mu.Unlock()
}

// addSource 记下新出现的来源，随下次写盘保存。调用方持有 Buffer.mu。
func (s *fileSink) addSource(src string) {
	s.mu.Lock()
	if _, ok := s.sources[src]; !ok {
		s.sources[src] = struct{}{}
		s.sourcesDirty = true
		s.scheduleLocked()
	}
	s.mu.Unlock()
}

// scheduleLocked 安排一次延迟写盘，调用方持有 s.mu。
func (s *fileSink) scheduleLocked() {
	if !s.scheduled {
		s.scheduled = true
		time.AfterFunc(flushInterval, s.flush)
	}
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
	first := s.pendingFirst
	s.pending = nil
	s.pendingFirst = time.Time{}
	s.scheduled = false
	s.urgent = false
	opt := s.opt
	var sources []string
	if s.sourcesDirty {
		s.sourcesDirty = false
		for src := range s.sources {
			sources = append(sources, src)
		}
	}
	s.mu.Unlock()
	if sources != nil {
		s.writeSources(sources)
	}
	if len(data) == 0 {
		return
	}
	if opt.Archive && s.size > 0 && s.start.IsZero() {
		// 当前文件的开始时间未知（开启落盘时第一行无法解析）：重新读一次，仍读不到就从这批日志算起，
		// 否则 sameDay 一直视为同一天，再也不会按天切分
		if s.start = firstEntryTime(s.path); s.start.IsZero() {
			s.start = first
		}
	}
	// 一批数据可能比当前文件剩余的空间大（甚至跨过多次切分大小）：在行边界拆开，写满一个文件就轮转再写下一段
	for len(data) > 0 {
		if s.size > 0 && (s.size+int64(len(data)) > opt.FileBytes || (opt.Archive && !sameDay(s.start, first))) {
			s.rotateLocked(opt)
		}
		part := data
		if room := opt.FileBytes - s.size; int64(len(part)) > room {
			// 写到文件满为止的最后一个整行；放不下一整行时至少写一行，避免单行超过上限时卡住
			cut := 0
			if room > 0 {
				cut = bytes.LastIndexByte(part[:room], '\n') + 1
			}
			if cut <= 0 {
				if cut = bytes.IndexByte(part, '\n') + 1; cut <= 0 {
					cut = len(part)
				}
			}
			part = part[:cut]
		}
		if !s.appendLocked(part, first) {
			return
		}
		data = data[len(part):]
		if len(data) > 0 {
			// 下一段的开始时间（用于跨天判断与归档命名）取它第一行的时间
			first = lineTime(data, first)
		}
	}
}

// appendLocked 把 data 追加到当前文件，调用方持有 ioMu；first 为其中第一条日志的时间，新文件以它为开始时间。
// 写入失败时记录日志并返回 false（这批日志丢弃）。
func (s *fileSink) appendLocked(data []byte, first time.Time) bool {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY|oNoFollow, 0o600)
	if err != nil {
		log.Printf("写入日志文件 %s 失败: %v", s.path, err)
		return false
	}
	if s.size == 0 {
		// 新建（或轮转、清空后重建）的文件显式设置权限，部分文件系统会忽略创建权限，见 fsperm
		_ = f.Chmod(0o600)
		s.start = first
	}
	n, err := f.Write(data)
	s.size += int64(n)
	_ = f.Close()
	if err != nil {
		log.Printf("写入日志文件 %s 失败: %v", s.path, err)
		return false
	}
	return true
}

// lineTime 解析 data 第一行日志的时间，解析失败时返回 fallback。
func lineTime(data []byte, fallback time.Time) time.Time {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	var e struct {
		Time time.Time `json:"time"`
	}
	if json.Unmarshal(line, &e) != nil || e.Time.IsZero() {
		return fallback
	}
	return e.Time
}

// rotateLocked 轮转当前文件，调用方持有 ioMu。普通模式改名为 <文件>.1（覆盖更旧的）；
// 归档模式改名为按开始时间命名的归档，再在后台压缩与清理。
func (s *fileSink) rotateLocked(opt PersistOptions) {
	if !opt.Archive {
		if err := os.Rename(s.path, s.path+".1"); err == nil || errors.Is(err, os.ErrNotExist) {
			s.size = 0
		}
		return
	}
	start := s.start
	if start.IsZero() {
		// 第一行无法解析（例如旧版本写坏的文件）时，用修改时间代替，至少能排在正确的位置附近
		start = time.Now()
		if st, err := os.Stat(s.path); err == nil {
			start = st.ModTime()
		}
	}
	name := s.archiveName(start)
	if err := os.Rename(s.path, name); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("归档日志文件 %s 失败: %v", s.path, err)
		return
	}
	s.size = 0
	s.start = time.Time{}
	s.goMaintain()
}

// sameDay 判断两个时间在 b 的时区里是否为同一天；a 为零值（当前文件开始时间未知）时视为同一天，不因此轮转。
func sameDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return true
	}
	y1, m1, d1 := a.In(b.Location()).Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// writeSources 把来源列表写入来源记录文件（先写临时文件再改名，断电不会留下半个文件）。
func (s *fileSink) writeSources(list []string) {
	sort.Strings(list)
	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	path := s.sourcesPath()
	tmp := path + ".tmp"
	if err := writeNewFile(tmp, data); err != nil {
		log.Printf("写入日志来源记录 %s 失败: %v", path, err)
		return
	}
	// 改名替换的是目录项本身，目标是符号链接时替换链接，不会写到链接指向的文件
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		log.Printf("写入日志来源记录 %s 失败: %v", path, err)
	}
}

// createNew 新建文件用于写入：先删除同名的残留（包括符号链接本身），再以 O_EXCL 创建，
// 不会跟随别人放好的符号链接截断或写入其他文件。权限显式设为 0600（部分文件系统忽略创建权限）。
func createNew(path string) (*os.File, error) {
	_ = os.Remove(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|oNoFollow, 0o600)
	if err != nil {
		return nil, err
	}
	_ = f.Chmod(0o600)
	return f, nil
}

// writeNewFile 用 createNew 新建文件并写入 data。
func writeNewFile(path string, data []byte) error {
	f, err := createNew(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// readSources 读取来源记录文件，文件不存在或内容无效时返回 nil。
func readSources(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var list []string
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	out := list[:0]
	for _, src := range list {
		if src != "" {
			out = append(out, src)
		}
	}
	return out
}

// clear 丢弃未写盘数据并删除日志文件、历史文件、归档与来源记录。
func (s *fileSink) clear() {
	s.archMu.Lock()
	defer s.archMu.Unlock()
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.mu.Lock()
	s.pending = nil
	s.pendingFirst = time.Time{}
	s.sources = map[string]struct{}{}
	s.sourcesDirty = false
	s.mu.Unlock()
	for _, a := range s.listArchives() {
		for _, p := range a.paths() {
			_ = os.Remove(p)
		}
	}
	s.removeTempFiles()
	_ = os.Remove(s.sourcesPath())
	_ = os.Remove(s.path + ".1")
	_ = os.Remove(s.path)
	s.size = 0
	s.start = time.Time{}
}

// restore 在归档模式下从当前文件、归档与旧版本留下的 .1 文件中读取最近的至多 n 条日志，按时间顺序返回。
func (s *fileSink) restore(n int) []Entry {
	files := s.openFiles()
	defer closeFiles(files)
	rev := searchFiles(context.Background(), files, 0, math.MaxUint64, n, nil)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// firstEntryTime 读取日志文件第一条日志的时间，读不到时返回零值。
func firstEntryTime(path string) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer f.Close()
	var t time.Time
	_ = scanForward(context.Background(), f, func(line []byte) bool {
		var e Entry
		if json.Unmarshal(line, &e) == nil && e.Seq > 0 {
			t = e.Time
			return false
		}
		// 损坏的行跳过，最多看前几行即可，见 maxHeadLines
		return true
	}, maxHeadLines)
	return t
}

// maxHeadLines 为读取文件开始时间时最多查看的行数
const maxHeadLines = 16

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
