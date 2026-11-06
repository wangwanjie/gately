package logbuf

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
)

// Export 按时间顺序（从旧到新）把满足条件的全部日志依次交给 fn：开启落盘时包括历史文件与归档，
// 最后是内存缓冲里尚未出现在文件中的日志。fn 返回错误或 ctx 被取消时停止并返回该错误。
//
// 日志逐条读出、逐条交给 fn，内存占用与日志总量无关，调用方可以边读边写出响应。
func (b *Buffer) Export(ctx context.Context, f *Filter, fn func(*Entry) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	sink := b.sink
	mem := make([]Entry, 0, b.count)
	for i := 0; i < b.count; i++ {
		mem = append(mem, b.at(i))
	}
	b.mu.Unlock()

	// last 为已交出的最大序号：缓冲里恢复自文件的日志与文件内容重复，按序号去重
	var last uint64
	if sink != nil {
		files := sink.snapshot()
		defer closeFiles(files)
		var err error
		if last, err = exportFiles(ctx, files, f, fn); err != nil {
			return err
		}
	}
	for i := range mem {
		e := &mem[i]
		if e.Seq <= last || !f.Match(e) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

// exportFiles 从旧到新顺序读取快照中的文件（files 本身是从新到旧排列的），返回已交出的最大序号。
// 某个文件打不开或读取出错时跳过它继续导出其余文件：导出是尽量完整地取出数据，与查找不同，不要求结果连续。
func exportFiles(ctx context.Context, files []snapFile, f *Filter, fn func(*Entry) error) (uint64, error) {
	pre := newPrefilter(f)
	var last uint64
	var fnErr error
	visit := func(line []byte) bool {
		seq, ok := lineSeq(line)
		if !ok || seq <= last {
			return true
		}
		last = seq
		if !pre.ok(line) {
			return true
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil || e.Seq == 0 || !f.Match(&e) {
			return true
		}
		if fnErr = fn(&e); fnErr != nil {
			return false
		}
		return true
	}
	for i := len(files) - 1; i >= 0; i-- {
		sf := &files[i]
		// files 从新到旧排列，倒着遍历时：整个文件早于范围的跳过，晚于范围的说明之后的文件也都晚于范围
		skipNewer, skipOlder := skipByTime(sf, f)
		if skipOlder {
			continue
		}
		if skipNewer {
			break
		}
		if err := ctx.Err(); err != nil {
			return last, err
		}
		if err := sf.open(); err != nil {
			continue
		}
		var r io.Reader = io.NewSectionReader(sf.file, 0, sf.size)
		if sf.gz {
			zr, err := gzip.NewReader(r)
			if err != nil {
				continue
			}
			r = zr
		}
		if err := scanForward(ctx, r, visit, 0); err != nil && ctx.Err() != nil {
			return last, ctx.Err()
		}
		if fnErr != nil {
			return last, fnErr
		}
	}
	return last, nil
}
