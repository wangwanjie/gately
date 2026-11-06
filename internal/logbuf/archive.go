package logbuf

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 归档文件命名：<名称>.<开始时间>[-<序号>].log，压缩后为 .log.gz。
// 开始时间是归档中第一条日志的 UTC 时间，按名称排序即为时间顺序；同一秒轮转多次时用序号区分。
const (
	archiveTimeLayout = "20060102T150405Z"
	gzSuffix          = ".gz"
	tmpSuffix         = ".tmp"
)

// archiveFile 是一个归档，plain、gz 为未压缩与压缩文件的路径（不存在时为空）。
// 压缩完成到删除未压缩文件之间两者同时存在，内容相同，读取时优先用未压缩的。
type archiveFile struct {
	start time.Time
	n     int
	plain string
	gz    string
}

// paths 返回归档现有的文件路径。
func (a archiveFile) paths() []string {
	var out []string
	for _, p := range []string{a.plain, a.gz} {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// before 判断 a 是否比 b 更早。
func (a archiveFile) before(b archiveFile) bool {
	if !a.start.Equal(b.start) {
		return a.start.Before(b.start)
	}
	return a.n < b.n
}

// archivePattern 返回匹配本日志归档文件名的正则：1 为开始时间，2 为序号，3 为 .gz 后缀。
func (s *fileSink) archivePattern() *regexp.Regexp {
	base := regexp.QuoteMeta(filepath.Base(s.stem()))
	return regexp.MustCompile(`^` + base + `\.(\d{8}T\d{6}Z)(?:-(\d+))?\.log(\.gz)?$`)
}

// archiveName 返回开始时间为 start 的新归档路径（未压缩），与已有归档重名时追加序号。
func (s *fileSink) archiveName(start time.Time) string {
	base := s.stem() + "." + start.UTC().Format(archiveTimeLayout)
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		name += ".log"
		if !exists(name) && !exists(name+gzSuffix) {
			return name
		}
	}
}

// exists 判断文件是否存在。
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// listArchives 按时间从旧到新列出本日志的全部归档。只认符合命名规则的普通文件，符号链接与目录忽略。
func (s *fileSink) listArchives() []archiveFile {
	dir := filepath.Dir(s.path)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	re := s.archivePattern()
	byKey := map[string]*archiveFile{}
	for _, ent := range ents {
		if !ent.Type().IsRegular() {
			continue
		}
		m := re.FindStringSubmatch(ent.Name())
		if m == nil {
			continue
		}
		start, err := time.Parse(archiveTimeLayout, m[1])
		if err != nil {
			continue
		}
		n := 0
		if m[2] != "" {
			if n, err = strconv.Atoi(m[2]); err != nil {
				continue
			}
		}
		key := m[1] + "-" + m[2]
		a := byKey[key]
		if a == nil {
			a = &archiveFile{start: start, n: n}
			byKey[key] = a
		}
		p := filepath.Join(dir, ent.Name())
		if m[3] != "" {
			a.gz = p
		} else {
			a.plain = p
		}
	}
	out := make([]archiveFile, 0, len(byKey))
	for _, a := range byKey {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].before(out[j]) })
	return out
}

// removeTempFiles 删除压缩中途退出留下的临时文件。
func (s *fileSink) removeTempFiles() {
	dir := filepath.Dir(s.path)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := filepath.Base(s.stem()) + "."
	for _, ent := range ents {
		name := ent.Name()
		if ent.Type().IsRegular() && strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".log"+gzSuffix+tmpSuffix) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// maintain 压缩尚未压缩的归档，再按保留策略删除最旧的归档。轮转后与开启落盘时在后台调用，多次调用依次执行。
func (s *fileSink) maintain() {
	s.archMu.Lock()
	defer s.archMu.Unlock()
	s.removeTempFiles()
	for _, a := range s.listArchives() {
		if a.plain == "" {
			continue
		}
		if a.gz != "" {
			// 上次压缩完成后没来得及删除未压缩文件，压缩文件是完整的（压缩完才改名）
			_ = os.Remove(a.plain)
			continue
		}
		if err := compressFile(a.plain); err != nil {
			log.Printf("压缩日志归档 %s 失败: %v", a.plain, err)
		}
	}
	s.prune()
}

// compressFile 把 path 压缩为 path.gz：先写临时文件，完成后改名并删除原文件，修改时间沿用原文件。
func compressFile(path string) (err error) {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return err
	}
	tmp := path + gzSuffix + tmpSuffix
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = dst.Close()
			_ = os.Remove(tmp)
		}
	}()
	_ = dst.Chmod(0o600)
	zw := gzip.NewWriter(dst)
	if _, err = io.Copy(zw, src); err != nil {
		return err
	}
	if err = zw.Close(); err != nil {
		return err
	}
	if err = dst.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path+gzSuffix); err != nil {
		return err
	}
	_ = os.Chtimes(path+gzSuffix, st.ModTime(), st.ModTime())
	return os.Remove(path)
}

// prune 按保留天数与总大小删除最旧的归档（以及旧版本留下的 .1 文件），当前文件不删。调用方持有 archMu。
//
// 归档的结束时间取文件修改时间（最后一次写入，压缩时沿用原文件的修改时间）与下一个归档（或当前文件）开始时间中较早的：
// 只看下一个文件的开始时间，中间停过机或长时间没有日志时会把旧归档算得太新；只看修改时间，复制过的文件又会显得太新。
// 结束时间早于保留期限时整个删除；总大小超过上限时从最旧的开始删除。
func (s *fileSink) prune() {
	s.mu.Lock()
	opt := s.opt
	s.mu.Unlock()
	if !opt.Archive {
		return
	}
	s.ioMu.Lock()
	activeSize, activeStart := s.size, s.start
	s.ioMu.Unlock()

	type item struct {
		paths []string
		size  int64
		end   time.Time
	}
	var items []item
	archives := s.listArchives()
	// 旧版本留下的 .1 比所有归档都早，结束时间用它的修改时间
	if st, err := os.Stat(s.path + ".1"); err == nil {
		items = append(items, item{paths: []string{s.path + ".1"}, size: st.Size(), end: st.ModTime()})
	}
	for i, a := range archives {
		var size int64
		var mtime time.Time
		for _, p := range a.paths() {
			if st, err := os.Stat(p); err == nil {
				size += st.Size()
				if mtime.IsZero() || st.ModTime().Before(mtime) {
					mtime = st.ModTime()
				}
			}
		}
		end := activeStart
		if i+1 < len(archives) {
			end = archives[i+1].start
		}
		if end.IsZero() {
			// 当前文件为空时，最新的归档一直延续到现在
			end = time.Now()
		}
		if !mtime.IsZero() && mtime.Before(end) {
			end = mtime
		}
		items = append(items, item{paths: a.paths(), size: size, end: end})
	}
	total := activeSize
	for _, it := range items {
		total += it.size
	}
	var cutoff time.Time
	if opt.KeepDays > 0 {
		cutoff = time.Now().Add(-time.Duration(opt.KeepDays) * 24 * time.Hour)
	}
	for _, it := range items {
		expired := !cutoff.IsZero() && it.end.Before(cutoff)
		if !expired && total <= opt.KeepBytes {
			// 从旧到新排列，这一个不用删，更新的也不用删
			break
		}
		for _, p := range it.paths {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("删除日志归档 %s 失败: %v", p, err)
			}
		}
		total -= it.size
	}
}
