package visitors

import (
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// saveDelay 为有变化后延迟写盘的时间。访客数据是统计性质，异常退出最多丢失这段时间内的计数；
// 间隔不宜太短：路由器闪存上频繁整份重写会加速磨损。
var saveDelay = 10 * time.Minute

// 读取落盘文件的上限：解压后总大小与单行长度，防止损坏或被替换的文件耗尽内存。
// maxFileBytes 为变量便于测试调小。
var maxFileBytes int64 = 512 << 20

const maxLineBytes = 1 << 20

// Info 是存储状态。
type Info struct {
	Settings
	Count     int    `json:"count"`
	FileBytes int64  `json:"fileBytes"`
	SavedAt   int64  `json:"savedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Info 返回存储状态。
func (s *Store) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	in := Info{Settings: s.set, Count: len(s.m), FileBytes: s.fileSize, Error: s.saveErr}
	if !s.saved.IsZero() {
		in.SavedAt = s.saved.Unix()
	}
	return in
}

// markDirtyLocked 标记有未写盘的变化，并在没有已安排的写盘时安排一次延迟写盘。
// 只在有变化时安排定时器，空闲时没有任何后台唤醒。
func (s *Store) markDirtyLocked() {
	s.dirty = true
	if s.path == "" || s.timer != nil {
		return
	}
	var t *time.Timer
	t = time.AfterFunc(saveDelay, func() {
		s.mu.Lock()
		// 定时器已被 Save 或 Flush 取消并替换时不再处理
		if s.timer != t {
			s.mu.Unlock()
			return
		}
		s.timer = nil
		s.mu.Unlock()
		if err := s.Save(); err != nil && s.warn != nil {
			s.warn("访客审计数据写盘失败: %v", err)
		}
	})
	s.timer = t
}

// Flush 取消已安排的延迟写盘，有未写盘的变化时立即写盘。程序退出与模块停止时调用。
func (s *Store) Flush() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	dirty := s.dirty
	s.mu.Unlock()
	if !dirty {
		return nil
	}
	return s.Save()
}

// Save 清理过期数据后把全部记录写入文件：先写临时文件再改名，写到一半断电也不会损坏已有文件。
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	// 锁内只复制记录，排序、JSON 编码、压缩与写文件都在锁外进行，不阻塞请求的记录；
	// 编码直接流式写入 gzip，不在内存里攒出整份未压缩的数据
	s.mu.Lock()
	s.pruneLocked(s.now())
	snap := make([]Visitor, 0, len(s.m))
	for _, v := range s.m {
		snap = append(snap, v.clone())
	}
	s.dirty = false
	s.mu.Unlock()

	// 按地址排序，文件内容稳定，便于比对
	slices.SortFunc(snap, func(x, y Visitor) int {
		return cmp.Compare(x.IP, y.IP)
	})
	size, err := writeFile(s.path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		for i := range snap {
			if err := enc.Encode(&snap[i]); err != nil {
				return err
			}
		}
		return nil
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		// 写盘失败时保留脏标记，下次有变化或退出时重试
		s.dirty = true
		s.saveErr = err.Error()
		return err
	}
	s.saveErr = ""
	s.saved = s.now()
	s.fileSize = size
	return nil
}

// writeFile 把 write 写出的内容压缩后原子地写入 path，返回文件大小。
func writeFile(path string, write func(io.Writer) error) (int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	tmp := path + ".tmp"
	// 先删除残留的临时文件，再以 O_EXCL 创建：目录里预先放好的同名符号链接不会被跟随
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	bw := bufio.NewWriterSize(f, 64<<10)
	zw, _ := gzip.NewWriterLevel(bw, gzip.BestSpeed)
	werr := write(zw)
	if cerr := zw.Close(); werr == nil {
		werr = cerr
	}
	if ferr := bw.Flush(); werr == nil {
		werr = ferr
	}
	if werr == nil {
		werr = f.Sync()
	}
	st, serr := f.Stat()
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = serr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return 0, werr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return st.Size(), nil
}

// errTooLarge 表示落盘文件解压后超过 maxFileBytes。
var errTooLarge = errors.New("访客审计数据解压后超过大小上限，其余部分未读取")

// limitReader 读取到 n 字节后返回 errTooLarge。与 io.LimitReader 不同，超限时报错而不是像正常结束一样静默截断。
type limitReader struct {
	r io.Reader
	n int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		// 恰好读到上限且后面已没有数据时不算超限
		var one [1]byte
		if n, _ := l.r.Read(one[:]); n == 0 {
			return 0, io.EOF
		}
		return 0, errTooLarge
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

// Load 读取落盘文件，无效的行跳过；文件不存在时不报错。读取时跳过已过期的记录，
// 记录数超出上限时边读边淘汰，读取后按保留天数与上限清理。
func (s *Store) Load() error {
	if s.path == "" {
		return nil
	}
	st, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// 只认普通文件，不跟随符号链接
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s 不是普通文件", s.path)
	}
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	// Lstat 与 Open 之间文件可能被换成符号链接或其他文件，打开后再确认是同一个普通文件
	fst, err := f.Stat()
	if err != nil {
		return err
	}
	if !fst.Mode().IsRegular() || !os.SameFile(st, fst) {
		return fmt.Errorf("%s 不是普通文件", s.path)
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()

	s.mu.Lock()
	set := s.set
	now := s.now()
	s.mu.Unlock()
	nowSec, today := now.Unix(), dayOf(now)
	cutSec := nowSec - int64(set.Days)*86400
	// 超出上限一定比例后才淘汰一次，避免每读一条就排序一遍
	evictAt := set.Max + max(1, set.Max/20)

	sc := bufio.NewScanner(&limitReader{r: zr, n: maxFileBytes})
	sc.Buffer(make([]byte, 64<<10), maxLineBytes)
	loaded := map[netip.Addr]*Visitor{}
	bad, dropped := 0, false
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var v Visitor
		if json.Unmarshal(line, &v) != nil {
			bad++
			continue
		}
		a, ok := v.sanitize(nowSec, today)
		if !ok {
			bad++
			continue
		}
		// 已超过保留天数的记录直接跳过，不占用内存
		if v.Last < cutSec {
			dropped = true
			continue
		}
		if old := loaded[a]; old != nil && old.Last >= v.Last {
			continue
		}
		loaded[a] = &v
		if len(loaded) >= evictAt {
			evictFrom(loaded, len(loaded)-set.Max)
			dropped = true
		}
	}
	// 文件被截断（异常退出时的旧版本等）只影响末尾，已读出的记录照常使用
	scanErr := sc.Err()
	if errors.Is(scanErr, errTooLarge) && s.warn != nil {
		s.warn("%v", scanErr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = loaded
	s.fileSize = st.Size()
	s.saved = st.ModTime()
	changed := s.pruneLocked(s.now())
	if len(s.m) > s.set.Max {
		s.evictLocked(len(s.m) - s.set.Max)
		changed = true
	}
	if changed || dropped || bad > 0 {
		s.markDirtyLocked()
	}
	if bad > 0 && s.warn != nil {
		s.warn("访客审计数据中有 %d 条无效记录，已跳过", bad)
	}
	if scanErr != nil {
		return fmt.Errorf("读取访客审计数据不完整: %w", scanErr)
	}
	return nil
}

// sanitize 校验并规整从文件读出的记录（文件可能被手工编辑）：地址必须有效，字符串截断，列表限制条数，按天计数排序去重；
// 晚于 now 的时间截到 now、晚于 today 的按天计数丢弃（写盘时时钟曾前跳到未来）。
func (v *Visitor) sanitize(now int64, today int32) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(v.IP))
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, false
	}
	a = a.Unmap()
	v.IP = a.String()
	if v.Last < v.First {
		v.Last = v.First
	}
	if v.Noise > v.Requests {
		v.Noise = v.Requests
	}
	if v.Denied > v.Requests {
		v.Denied = v.Requests
	}
	v.Deny = capList(v.Deny, maxDeny)
	for i := range v.Deny {
		v.Deny[i].Name = clip(v.Deny[i].Name, 32)
	}
	slices.SortFunc(v.Days, func(x, y Day) int { return int(x.D) - int(y.D) })
	v.Days = slices.CompactFunc(v.Days, func(x, y Day) bool { return x.D == y.D })
	resetFuture(v, now, today)
	v.Sites = capList(v.Sites, maxSites)
	for i := range v.Sites {
		st := &v.Sites[i]
		st.Listener, st.Key, st.Name = clip(st.Listener, maxNameLen), clip(st.Key, maxNameLen), clip(st.Name, maxNameLen)
	}
	v.Paths = capList(v.Paths, maxPaths)
	for i := range v.Paths {
		v.Paths[i].Path = clip(v.Paths[i].Path, maxPathLen)
	}
	v.Identities = capList(v.Identities, maxIdents)
	for i := range v.Identities {
		id := &v.Identities[i]
		id.Name = clip(id.Name, maxNameLen)
		if id.Kind != "admin" {
			id.Kind = "user"
		}
	}
	v.Agents = capList(v.Agents, maxAgents)
	for i := range v.Agents {
		v.Agents[i].UA = clip(v.Agents[i].UA, maxUALen)
	}
	r := &v.LastReq
	r.Method, r.Host, r.Path = clip(r.Method, maxMethod), clip(r.Host, maxHostLen), clip(r.Path, maxPathLen)
	r.Deny, r.SiteName = clip(r.Deny, 32), clip(r.SiteName, maxNameLen)
	return a, true
}

func capList[T any](l []T, n int) []T {
	if len(l) > n {
		return l[:n:n]
	}
	return l
}
