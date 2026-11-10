package visitors

import (
	"bufio"
	"bytes"
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
const (
	maxFileBytes = 512 << 20
	maxLineBytes = 1 << 20
)

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

	// 在锁内只做 JSON 编码，压缩与写文件在锁外进行，不阻塞请求的记录
	s.mu.Lock()
	s.pruneLocked(s.now())
	addrs := make([]netip.Addr, 0, len(s.m))
	for a := range s.m {
		addrs = append(addrs, a)
	}
	slices.SortFunc(addrs, func(x, y netip.Addr) int { return x.Compare(y) })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, a := range addrs {
		if err := enc.Encode(s.m[a]); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.dirty = false
	s.mu.Unlock()

	size, err := writeFile(s.path, buf.Bytes())
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

// writeFile 把内容压缩后原子地写入 path，返回文件大小。
func writeFile(path string, data []byte) (int64, error) {
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
	zw, _ := gzip.NewWriterLevel(f, gzip.BestSpeed)
	_, werr := zw.Write(data)
	if cerr := zw.Close(); werr == nil {
		werr = cerr
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

// Load 读取落盘文件，无效的行跳过；文件不存在时不报错。读取后按保留天数与上限清理。
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
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	sc := bufio.NewScanner(io.LimitReader(zr, maxFileBytes))
	sc.Buffer(make([]byte, 64<<10), maxLineBytes)
	loaded := map[netip.Addr]*Visitor{}
	bad := 0
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
		a, ok := v.sanitize()
		if !ok {
			bad++
			continue
		}
		if old := loaded[a]; old != nil && old.Last >= v.Last {
			continue
		}
		loaded[a] = &v
	}
	// 文件被截断（异常退出时的旧版本等）只影响末尾，已读出的记录照常使用
	scanErr := sc.Err()
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
	if changed || bad > 0 {
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

// sanitize 校验并规整从文件读出的记录（文件可能被手工编辑）：地址必须有效，字符串截断，列表限制条数，按天计数排序去重。
func (v *Visitor) sanitize() (netip.Addr, bool) {
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
