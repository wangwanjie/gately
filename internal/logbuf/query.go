package logbuf

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Query 返回序号在 (after, before) 之间、满足过滤条件的日志（按时间顺序），before 为 0 表示不限。
// limit 大于 0 时只保留其中最新的 limit 条；这时如果内存缓冲里凑不够，且开启了落盘，
// 会继续从日志文件（含 .1 历史文件与归档）中查找比缓冲更早的日志，让过滤与“加载更早的日志”能覆盖文件里保留的全部历史。
// limit 为 0 时只查内存缓冲，避免一次返回整份日志文件。
func (b *Buffer) Query(after, before uint64, limit int, f *Filter) []Entry {
	return b.QueryContext(context.Background(), after, before, limit, f)
}

// QueryContext 与 Query 相同，但查找日志文件时会定期检查 ctx：请求被取消（例如前端已换了过滤条件）后
// 立即停止扫描，返回已找到的部分结果。
func (b *Buffer) QueryContext(ctx context.Context, after, before uint64, limit int, f *Filter) []Entry {
	b.mu.Lock()
	// 从新到旧扫描缓冲，倒序收集候选日志。每次写日志都要拿 b.mu，
	// 有过滤条件时只在锁内复制序号范围内的日志，匹配放到锁外做，避免拖慢写入
	var cand []Entry
	filtered := !f.Empty()
	for i := b.count - 1; i >= 0; i-- {
		e := b.at(i)
		if before > 0 && e.Seq >= before {
			continue
		}
		if e.Seq <= after {
			break
		}
		cand = append(cand, e)
		if !filtered && limit > 0 && len(cand) >= limit {
			// 没有过滤条件时每条都命中，凑够即可停止
			break
		}
	}
	// oldest 为缓冲中最早一条的序号，文件里只取比它更早的日志，避免与缓冲重复
	oldest := b.nextSeq
	if b.count > 0 {
		oldest = b.at(0).Seq
	}
	sink := b.sink
	b.mu.Unlock()

	rev := cand
	if filtered {
		// 原地过滤：命中的日志依次前移，仍保持从新到旧的顺序
		rev = cand[:0]
		for i := range cand {
			if f.Match(&cand[i]) {
				rev = append(rev, cand[i])
				if limit > 0 && len(rev) >= limit {
					break
				}
			}
		}
	}

	if limit > 0 && len(rev) < limit && sink != nil && after+1 < oldest {
		upper := oldest
		if before > 0 && before < upper {
			upper = before
		}
		rev = append(rev, sink.search(ctx, after, upper, limit-len(rev), f)...)
	}
	// 翻转为时间顺序
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// snapFile 是查找开始时记下的一个日志文件，按从新到旧排列。
//
// 当前文件与普通模式的 .1 历史文件在快照时就打开并记下大小（之后可能被轮转改名或继续追加）；
// 归档内容不再变化，只记下路径，读到时再打开，避免归档很多时一次占用大量文件句柄。
type snapFile struct {
	file *os.File
	size int64
	// paths 为尚未打开的归档的候选路径（未压缩的优先），打开后按扩展名判断是否压缩
	paths []string
	gz    bool
	// start、end 为文件中日志的时间范围（end 为下一个更新文件的开始时间），零值表示未知或不限；
	// 按时间范围查找时据此跳过整个文件
	start, end time.Time
}

// open 打开尚未打开的归档：第一个候选文件不存在时（刚好压缩完成并删除了未压缩文件）尝试下一个。
func (sf *snapFile) open() error {
	if sf.file != nil {
		return nil
	}
	err := error(os.ErrNotExist)
	for _, p := range sf.paths {
		var f *os.File
		if f, err = os.Open(p); err != nil {
			continue
		}
		st, serr := f.Stat()
		if serr != nil {
			_ = f.Close()
			return serr
		}
		sf.file, sf.size, sf.gz = f, st.Size(), strings.HasSuffix(p, gzSuffix)
		return nil
	}
	return err
}

// closeFiles 关闭快照中已打开的文件。
func closeFiles(files []snapFile) {
	for _, sf := range files {
		if sf.file != nil {
			_ = sf.file.Close()
		}
	}
}

// snapshot 在 ioMu 内先把待写盘的日志同步写入文件，再打开当前文件与历史文件并记下大小，按从新到旧排列。
//
// 先写盘是为了让刚被挤出内存缓冲、还在 pending 里等待批量写盘的日志也能被查到，否则“加载更早的日志”会漏掉这一段。
// 之后的扫描不再持有 ioMu，不阻塞写盘：轮转是把当前文件重命名为 .1 或归档、清空是删除文件，
// 在 Unix 上已打开的句柄仍指向原来的内容；新追加的内容在记下的大小之后，不会被读到半行。
// Windows 上文件被打开时重命名或删除可能失败，写盘逻辑会容忍（留到下次轮转），扫描很快结束后即恢复。
// 文件不存在时跳过；其他打开失败时不再继续打开更早的文件，避免结果中间缺一段。调用方负责用 closeFiles 关闭。
func (s *fileSink) snapshot() []snapFile {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.flushLocked()
	return s.openFilesLocked()
}

// openFiles 与 snapshot 相同但不先写盘，开启落盘时恢复日志用（此时还没有待写数据，也没有其他协程访问）。
func (s *fileSink) openFiles() []snapFile {
	return s.openFilesLocked()
}

// openFilesLocked 打开当前文件，并列出历史文件与归档，调用方持有 ioMu（轮转需要 ioMu，列出期间不会发生）。
func (s *fileSink) openFilesLocked() []snapFile {
	s.mu.Lock()
	archive := s.opt.Archive
	s.mu.Unlock()
	var out []snapFile
	// openNow 立即打开文件，返回 false 表示应停止（打开失败且不是文件不存在）
	openNow := func(p string, start, end time.Time) bool {
		file, err := os.Open(p)
		if err != nil {
			return errors.Is(err, os.ErrNotExist)
		}
		st, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return false
		}
		out = append(out, snapFile{file: file, size: st.Size(), start: start, end: end})
		return true
	}
	activeStart := s.start
	if s.size == 0 {
		activeStart = time.Time{}
	}
	if !openNow(s.path, activeStart, time.Time{}) {
		return out
	}
	if !archive {
		// 普通模式：.1 可能随时被下一次轮转覆盖，必须现在打开
		openNow(s.path+".1", time.Time{}, activeStart)
		return out
	}
	archives := s.listArchives()
	end := activeStart
	for i := len(archives) - 1; i >= 0; i-- {
		a := archives[i]
		// 结束时间为零（当前文件为空）时最新的归档不限结束时间
		out = append(out, snapFile{paths: a.paths(), start: a.start, end: end})
		end = a.start
	}
	// 旧版本留下的 .1 比所有归档都早；归档模式下不会再被覆盖，读到时再打开
	if exists(s.path + ".1") {
		out = append(out, snapFile{paths: []string{s.path + ".1"}, end: end})
	}
	return out
}

// search 从新到旧在日志文件中查找序号在 (after, before) 之间且满足条件的日志，最多 n 条，按倒序返回。
// ctx 被取消或读取出错时停止，返回已找到的部分。
func (s *fileSink) search(ctx context.Context, after, before uint64, n int, f *Filter) []Entry {
	files := s.snapshot()
	defer closeFiles(files)
	return searchFiles(ctx, files, after, before, n, f)
}

// skipByTime 按时间范围判断文件的处理方式：skip 表示整个文件都晚于范围（跳过它，继续看更早的），
// stop 表示整个文件都早于范围（更早的文件也不用看了）。
func skipByTime(sf *snapFile, f *Filter) (skip, stop bool) {
	if f == nil {
		return false, false
	}
	if !f.To.IsZero() && !sf.start.IsZero() && sf.start.After(f.To) {
		return true, false
	}
	if !f.From.IsZero() && !sf.end.IsZero() && sf.end.Before(f.From) {
		return false, true
	}
	return false, false
}

// searchFiles 依次（从新到旧）扫描日志文件，不涉及任何锁。未压缩的文件从末尾向前读，
// 压缩的归档只能从头解压，保留其中最新的若干条命中结果。
func searchFiles(ctx context.Context, files []snapFile, after, before uint64, n int, f *Filter) []Entry {
	var out []Entry
	pre := newPrefilter(f)
	// match 解析并判断一行，返回命中的日志
	match := func(line []byte) (Entry, bool) {
		if !pre.ok(line) {
			return Entry{}, false
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil || e.Seq == 0 || !f.Match(&e) {
			return Entry{}, false
		}
		return e, true
	}
	visit := func(line []byte) bool {
		seq, ok := lineSeq(line)
		if !ok || seq >= before {
			return true
		}
		if seq <= after {
			// 文件按序号递增写入，再往前只会更小
			return false
		}
		if e, ok := match(line); ok {
			out = append(out, e)
		}
		return len(out) < n
	}
	for i := range files {
		sf := &files[i]
		skip, stop := skipByTime(sf, f)
		if stop {
			break
		}
		if skip {
			continue
		}
		if err := sf.open(); err != nil {
			// 归档刚被清理删除，或读取出错：不再继续读更早的文件，否则结果中间会缺一段
			break
		}
		if !sf.gz {
			done, err := scanBackward(ctx, sf.file, sf.size, visit)
			if err != nil || !done {
				break
			}
			continue
		}
		found, more, err := searchGzip(ctx, sf.file, after, before, n-len(out), match)
		out = append(out, found...)
		if err != nil || !more || len(out) >= n {
			break
		}
	}
	return out
}

// searchGzip 解压扫描一个归档，返回其中序号在 (after, before) 之间的最新至多 n 条命中日志（从新到旧）。
// more 表示还应继续查更早的文件：归档里没有遇到序号不大于 after 的日志。
func searchGzip(ctx context.Context, file *os.File, after, before uint64, n int, match func([]byte) (Entry, bool)) (found []Entry, more bool, err error) {
	zr, err := gzip.NewReader(file)
	if err != nil {
		return nil, false, err
	}
	defer zr.Close()
	var ring []Entry
	reachedAfter := false
	err = scanForward(ctx, zr, func(line []byte) bool {
		seq, ok := lineSeq(line)
		if !ok {
			return true
		}
		if seq >= before {
			// 序号递增，后面的都更大
			return false
		}
		if seq <= after {
			reachedAfter = true
			return true
		}
		if e, ok := match(line); ok {
			ring = append(ring, e)
			if len(ring) > n {
				ring = ring[1:]
			}
		}
		return true
	}, 0)
	if err != nil {
		return nil, false, err
	}
	for i := len(ring) - 1; i >= 0; i-- {
		found = append(found, ring[i])
	}
	return found, !reachedAfter, nil
}

// maxLineBytes 限制单行日志的长度，超过的行（文件损坏）跳过，避免一次读入过大的数据
const maxLineBytes = 4 << 20

// scanForward 从 r 逐行读取，对每个非空行调用 fn，fn 返回 false 时停止；maxLines 大于 0 时最多读这么多行。
// 读取出错或 ctx 被取消时返回错误，正常读完或 fn 要求停止时返回 nil。
func scanForward(ctx context.Context, r io.Reader, fn func(line []byte) bool, maxLines int) error {
	br := bufio.NewReaderSize(r, tailChunk)
	var long []byte
	tooLong := false
	for count := 0; maxLines <= 0 || count < maxLines; {
		chunk, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// 行比缓冲长：先攒起来，超过上限后丢弃这一行
			if !tooLong {
				long = append(long, chunk...)
				if len(long) > maxLineBytes {
					long, tooLong = nil, true
				}
			}
			continue
		}
		line := chunk
		if long != nil || tooLong {
			line = append(long, chunk...)
			if tooLong || len(line) > maxLineBytes+1 {
				line = nil
			}
			long, tooLong = nil, false
		}
		line = bytes.TrimSuffix(line, []byte{'\n'})
		if len(line) > 0 {
			count++
			// 第一行与之后每读一批行检查一次是否已取消
			if count%1024 == 1 {
				if cerr := ctx.Err(); cerr != nil {
					return cerr
				}
			}
			if !fn(line) {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
}

// lineSeq 从一行 JSON 开头快速读出序号（Entry 序列化时 seq 是第一个字段），不必完整解析。
func lineSeq(line []byte) (uint64, bool) {
	const prefix = `{"seq":`
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return 0, false
	}
	rest := line[len(prefix):]
	end := bytes.IndexByte(rest, ',')
	if end <= 0 {
		return 0, false
	}
	v, err := strconv.ParseUint(string(rest[:end]), 10, 64)
	return v, err == nil
}

// prefilter 在完整解析 JSON 之前，用原始字节排除明显不含关键词的行。
// JSON 序列化会转义引号、反斜杠与 <>&，含这些字符的关键词不做预过滤；需要与译文比较时也不做。
type prefilter struct {
	terms [][]byte
}

func newPrefilter(f *Filter) prefilter {
	var p prefilter
	if f == nil || f.Translate != nil {
		// 需要与译文比较时，原文里不一定包含关键词，不能预过滤
		return p
	}
	for _, t := range f.Terms {
		if !bytes.ContainsAny([]byte(t), "\"\\<>&") {
			p.terms = append(p.terms, []byte(t))
		}
	}
	return p
}

func (p prefilter) ok(line []byte) bool {
	if len(p.terms) == 0 {
		return true
	}
	lower := bytes.ToLower(line)
	for _, t := range p.terms {
		if !bytes.Contains(lower, t) {
			return false
		}
	}
	return true
}

// scanBackward 从 file 的 size 处逐行向前读取，对每个非空行调用 fn，fn 返回 false 时停止。
// 返回值表示是否读完了整个文件，调用方据此决定是否继续读更早的文件；
// 读取出错或 ctx 被取消时返回错误，调用方应停止，避免跳过一段继续读更早的文件。
func scanBackward(ctx context.Context, file *os.File, size int64, fn func(line []byte) bool) (bool, error) {
	off := size
	// carry 为上一块开头尚未凑成整行的部分
	var carry []byte
	for off > 0 {
		// 每读一块检查一次是否已取消
		if err := ctx.Err(); err != nil {
			return false, err
		}
		step := min(int64(tailChunk), off)
		off -= step
		chunk := make([]byte, step, int(step)+len(carry))
		// ReadAt 读不满时一定返回错误（包括 io.EOF，说明文件比记下的大小短），都按读取失败处理
		if n, err := file.ReadAt(chunk, off); n < len(chunk) {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return false, err
		}
		data := append(chunk, carry...)
		// 从后往前切出完整的行；块开头的残行留到下一轮
		for {
			i := bytes.LastIndexByte(data, '\n')
			if i < 0 {
				break
			}
			if line := data[i+1:]; len(line) > 0 && !fn(line) {
				return false, nil
			}
			data = data[:i]
		}
		carry = data
	}
	if len(carry) > 0 && !fn(carry) {
		return false, nil
	}
	return true, nil
}
