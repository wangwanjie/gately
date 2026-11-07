// Package crashlog 把进程崩溃时 Go 运行时输出的 panic / fatal error 与堆栈保存到配置目录，
// 下次启动时整理进 crash.log，并判断上次运行是否异常结束。
//
// 运行期间进程写着一个“运行标记”文件 crash-run.log：启动时写入一行版本与进程信息，
// 并通过 debug.SetCrashOutput 让运行时在崩溃时把输出同时写进这个文件；正常退出或重启前删除它。
// 下次启动时如果标记文件还在，说明上次运行没有正常结束：
//   - 文件里除了首行还有内容：运行时崩溃（或记录了致命错误），内容就是崩溃信息；
//   - 只有首行：进程被强制结束（如 OOM Killer、kill -9）或机器断电，没有机会留下任何输出。
//
// Docker、LXC、飞牛等环境里进程的 stderr 往往在容器重启后就看不到了，落盘后才能随诊断包导出。
package crashlog

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const (
	// LogName 是整理后的崩溃记录文件名，位于配置目录
	LogName = "crash.log"
	// runName 是运行标记文件名，正常退出时删除
	runName = "crash-run.log"
	// lockName 是实例锁文件名：运行期间持有排他锁，同一配置目录的另一个实例据此知道标记文件属于活着的进程。
	// 锁文件本身不删除，进程结束时锁由系统释放
	lockName = "crash-run.lock"
	// maxLogBytes 是 crash.log 的大小上限，超出时从最早的记录开始丢弃
	maxLogBytes = 1 << 20
	// entryPrefix 是 crash.log 中每条记录的标题行前缀，裁剪时按它对齐记录边界
	entryPrefix = "===== "
)

// ErrLocked 表示同一配置目录已有另一个实例在运行并持有实例锁：本次不整理它的运行标记，也不开启崩溃记录。
var ErrLocked = errors.New("another instance using this config directory is running")

// heartbeat 是刷新运行标记文件修改时间的间隔：被强制结束时，用最后的修改时间估计结束时间
var heartbeat = time.Minute

// Kind 是上次运行的结束方式。
type Kind string

const (
	// None 表示上次正常退出，或这是首次启动
	None Kind = ""
	// Crashed 表示运行时崩溃（panic、fatal error）或记录了致命错误后退出
	Crashed Kind = "crash"
	// Killed 表示进程意外结束且没有留下任何输出
	Killed Kind = "killed"
)

// Previous 描述上次运行的结束情况。
type Previous struct {
	Kind Kind
	// Time 为崩溃时间；Killed 时为最后一次存活记录的时间，实际结束时间在其后一个心跳间隔内
	Time time.Time
	// Reason 为崩溃输出的第一行，例如 "panic: runtime error: invalid memory address ..."
	Reason string
}

// Recorder 负责本次运行的崩溃记录。
type Recorder struct {
	path string
	mu   sync.Mutex
	f    *os.File
	lock *os.File
	stop chan struct{}
}

// Start 整理上次运行留下的标记文件，再为本次运行开启崩溃记录。header 写在标记文件首行，
// 用于在 crash.log 中标明是哪个版本、哪次启动崩溃的。
// 开启失败时仍返回已整理出的上次运行情况，调用方只需记录错误，不影响启动。
func Start(dir, header string) (*Recorder, Previous, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, Previous{}, err
	}
	// 先拿实例锁：拿不到说明另一个实例正在使用这个配置目录（例如服务已在运行，又手动启动了一次），
	// 它的运行标记不能当成上次运行的遗留来整理或删除
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, Previous{}, err
	}
	if err := tryLock(lock); err != nil {
		lock.Close()
		return nil, Previous{}, err
	}
	prev, err := collect(dir)
	if err != nil {
		lock.Close()
		return nil, prev, err
	}
	path := filepath.Join(dir, runName)
	// O_APPEND：运行时写崩溃输出与 Fatal 追加内容都落在文件末尾，不会覆盖首行
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		lock.Close()
		return nil, prev, err
	}
	if _, err := f.WriteString(strings.TrimSpace(header) + "\n"); err != nil {
		f.Close()
		lock.Close()
		return nil, prev, err
	}
	// 运行时会复制一份文件描述符，崩溃时直接写入，不经过 Go 的任何缓冲
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		f.Close()
		lock.Close()
		return nil, prev, err
	}
	r := &Recorder{path: path, f: f, lock: lock, stop: make(chan struct{})}
	go r.beat(heartbeat)
	return r, prev, nil
}

// beat 定期刷新标记文件的修改时间，进程被强制结束后据此估计结束时间。
func (r *Recorder) beat(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			now := time.Now()
			_ = os.Chtimes(r.path, now, now)
		}
	}
}

// Fatal 记录导致进程退出的致命错误（例如管理端口被占用），之后进程退出、不调用 Close，
// 下次启动时这条错误与运行时崩溃一样整理进 crash.log。
func (r *Recorder) Fatal(msg string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	_, _ = r.f.WriteString("fatal: " + strings.TrimSpace(msg) + "\n")
	_ = r.f.Sync()
}

// Close 在正常退出或重启前调用：停止崩溃记录并删除标记文件，下次启动不会误报异常结束。
func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	close(r.stop)
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_ = r.f.Close()
	r.f = nil
	_ = os.Remove(r.path)
	// 删除标记后再释放实例锁，避免另一个实例在两者之间把标记当成遗留
	_ = r.lock.Close()
}

// Abandon 模拟进程被强制结束：停止心跳并关闭文件、释放实例锁，但保留运行标记与崩溃输出设置之外的一切痕迹，
// 下次 Start 会把它当成意外结束整理。只供测试在同一进程内模拟 kill -9 使用。
func (r *Recorder) Abandon() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	close(r.stop)
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_ = r.f.Close()
	r.f = nil
	_ = r.lock.Close()
}

// collect 读取上次运行留下的标记文件，追加到 crash.log 后删除。没有标记文件表示上次正常退出。
func collect(dir string) (Previous, error) {
	path := filepath.Join(dir, runName)
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return Previous{}, nil
	}
	if err != nil {
		return Previous{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Previous{}, err
	}
	header, body, _ := strings.Cut(string(data), "\n")
	body = strings.TrimRight(body, "\r\n\t ")
	prev := Previous{Kind: Killed, Time: st.ModTime()}
	if strings.TrimSpace(body) != "" {
		prev.Kind = Crashed
		prev.Reason = firstLine(body)
		if strings.HasPrefix(prev.Reason, "goroutine ") {
			// 运行时致命错误（concurrent map writes、sync: unlock of unlocked mutex 等）的 "fatal error: ..." 一行
			// 在进入崩溃状态前打印，Go 只写到 stderr、不写进崩溃输出文件，文件从堆栈开始；用出错的函数代替原因
			prev.Reason = "fatal error in " + fatalSite(body)
			body = prev.Reason + "\n(the Go runtime prints the \"fatal error: ...\" message only to stderr, e.g. docker logs; " +
				"the stack below starts where the error was raised)\n\n" + body
		}
	} else {
		body = "(no crash output: the process ended without a chance to write anything, " +
			"e.g. killed by the OOM killer, kill -9 / docker kill, or a power loss; " +
			"the time above is the last heartbeat, the process ended within a minute after it)"
	}
	entry := fmt.Sprintf("%s%s %s | %s =====\n%s\n\n", entryPrefix, prev.Time.UTC().Format(time.RFC3339), prev.Kind, strings.TrimSpace(header), body)
	if err := appendLog(filepath.Join(dir, LogName), []byte(entry)); err != nil {
		return prev, err
	}
	return prev, os.Remove(path)
}

// fatalSite 返回致命错误发生的函数：堆栈第一帧通常是运行时的 fatal / throw，取它之后的第一个函数。
func fatalSite(body string) string {
	lines := strings.Split(body, "\n")
	for i := 0; i+1 < len(lines); i++ {
		line := lines[i]
		// 函数行后紧跟以 tab 开头的源码位置行
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "goroutine ") || !strings.HasPrefix(lines[i+1], "\t") {
			continue
		}
		name := line
		if j := strings.LastIndexByte(name, '('); j > 0 && strings.HasSuffix(name, ")") {
			name = name[:j]
		}
		switch {
		case strings.HasSuffix(name, ".fatal"), strings.HasSuffix(name, ".throw"), strings.HasSuffix(name, ".fatalthrow"):
			continue
		}
		return name
	}
	return "unknown function"
}

// firstLine 返回第一行非空内容，过长时截断。
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 300 {
				line = line[:300] + "…"
			}
			return line
		}
	}
	return ""
}

// appendLog 把一条记录追加到 crash.log，超过上限时从最早的整条记录开始丢弃。
func appendLog(path string, entry []byte) error {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data := append(old, entry...)
	if len(data) > maxLogBytes {
		data = trimHead(data, maxLogBytes)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// trimHead 保留末尾不超过 limit 字节的完整记录。最后一条记录单独就超过上限时
// （例如持有大量连接时运行时致命错误会打印全部协程的堆栈），保留它的开头与结尾、截去中间。
func trimHead(data []byte, limit int) []byte {
	if len(data) <= limit {
		return data
	}
	cut := len(data) - limit
	sep := []byte("\n" + entryPrefix)
	// 从 cut-1 开始查找：cut 恰好落在某条记录标题行开头时，它前面的换行也要在查找范围内，这条记录才能保留
	if i := bytes.Index(data[cut-1:], sep); i >= 0 {
		return data[cut-1+i+1:]
	}
	start := 0
	if i := bytes.LastIndex(data[:cut], sep); i >= 0 {
		start = i + 1
	}
	return truncateMiddle(data[start:], limit)
}

// truncateMiddle 把超长的单条记录截到 limit 以内：保留开头约 3/4（标题行、崩溃原因与出错协程的堆栈）
// 和结尾约 1/4，中间换成一行截断说明；截断点对齐到整行，避免半行地址或文本影响脱敏与阅读。
func truncateMiddle(entry []byte, limit int) []byte {
	if len(entry) <= limit {
		return entry
	}
	const room = 64 // 截断说明一行预留的长度
	head := entry[:limit*3/4]
	if i := bytes.LastIndexByte(head, '\n'); i >= 0 {
		head = head[:i+1]
	}
	tailLen := limit - len(head) - room
	if tailLen < 0 {
		tailLen = 0
	}
	tail := entry[len(entry)-tailLen:]
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	} else {
		tail = nil
	}
	note := fmt.Sprintf("\n... (%d bytes truncated) ...\n\n", len(entry)-len(head)-len(tail))
	out := make([]byte, 0, len(head)+len(note)+len(tail))
	out = append(out, head...)
	out = append(out, note...)
	return append(out, tail...)
}
