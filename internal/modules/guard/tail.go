package guard

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/wangwanjie/gately/internal/demo"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/script"
)

// 日志跟踪参数，可在测试中调整
var (
	// tailPoll 为轮询兜底周期：Linux 下文件变化由 inotify 即时唤醒，此周期只用于 inotify 不可用或漏掉事件（例如网络文件系统）时
	tailPoll     = time.Second
	retryBackoff = 5 * time.Second
	maxBackoff   = time.Minute
	// rotateGrace 为轮转后继续读取旧文件的时长：logrotate create 模式下新文件建好后、写入方收到信号重新打开日志前，
	// 新内容仍写在旧文件里
	rotateGrace = time.Second
)

// mtimeSlack 是判断「等待期间才出现的文件」时对修改时间的容差：文件系统时间戳取自粗粒度时钟（可能落后一个时钟周期），
// 部分文件系统只精确到秒甚至两秒，文件刚好在开始等待后立即创建时，修改时间可能略早于开始等待的时间
const mtimeSlack = 2 * time.Second

// maxLine 是单行日志的长度上限，超出部分丢弃
const maxLine = 64 << 10

// lineSplitter 把字节流拆成行，过长的行截断。
type lineSplitter struct {
	buf    []byte
	onLine func(string)
}

func (s *lineSplitter) write(p []byte) {
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if len(s.buf) < maxLine {
				s.buf = append(s.buf, p[:min(len(p), maxLine-len(s.buf))]...)
			}
			return
		}
		if len(s.buf) < maxLine {
			s.buf = append(s.buf, p[:min(i, maxLine-len(s.buf))]...)
		}
		line := strings.TrimRight(string(s.buf), "\r")
		s.buf = s.buf[:0]
		if line != "" {
			s.onLine(line)
		}
		p = p[i+1:]
	}
}

// tailWatcher 等待被跟踪文件发生变化。Linux 下用 inotify 在写入、轮转、新建时立即唤醒，其他情况按超时轮询。
// 所有方法只在 tailFile 所在的协程里调用。
type tailWatcher interface {
	// watchFile 监视已打开的文件 f（路径为 path）的写入、截断、改名和删除，返回供 unwatchFile 使用的句柄，无法监视时返回 -1
	watchFile(f *os.File, path string) int
	// unwatchFile 取消 watchFile 建立的监视，h 为 -1 时什么也不做
	unwatchFile(h int)
	// wait 等到文件可能有变化或超过 d，ctx 取消时返回 false；期间到达的多个事件合并为一次唤醒
	wait(ctx context.Context, d time.Duration) bool
	// close 释放监视用的描述符并等待后台协程退出
	close()
}

// pollWatcher 只按超时轮询，用于非 Linux 平台和 inotify 不可用时。
type pollWatcher struct{}

func (pollWatcher) watchFile(*os.File, string) int {
	return -1
}

func (pollWatcher) unwatchFile(int) {
}

func (pollWatcher) wait(ctx context.Context, d time.Duration) bool {
	return sleepCtx(ctx, d, nil)
}

func (pollWatcher) close() {
}

// sleepCtx 等待 d、wake 有信号或 ctx 取消，ctx 取消时返回 false。wake 为 nil 时只等超时。
func sleepCtx(ctx context.Context, d time.Duration, wake <-chan struct{}) bool {
	t := time.NewTimer(max(d, 0))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	case <-wake:
		return true
	}
}

// rotatedFile 是轮转后仍在宽限期内读取的旧文件。
type rotatedFile struct {
	f     *os.File
	watch int
	sp    *lineSplitter
	until time.Time
}

// drainFile 读取 f 中剩余的内容直到末尾。
func drainFile(f *os.File, sp *lineSplitter, buf []byte) {
	for {
		n, err := f.Read(buf)
		if n > 0 {
			sp.write(buf[:n])
		}
		if n == 0 || err != nil {
			return
		}
	}
}

// tailFile 跟踪文件新增的内容，相当于 tail -F：启动时从文件末尾开始，文件被轮转（换成新文件）或截断时从头读取新文件。
// 文件不存在或无法读取时通过 setErr 报告并定期重试；只读取普通文件，FIFO、设备等会让打开或读取一直阻塞，直接报错。
// Linux 下由 inotify 在文件写入、轮转或出现时立即唤醒，每次唤醒读到文件末尾为止；轮询（tailPoll）始终作为兜底。
func tailFile(ctx context.Context, path string, onLine func(string), setErr func(string)) {
	var (
		f *os.File
		// fileWatch 为当前文件的监视句柄
		fileWatch = -1
		info      os.FileInfo
		offset    int64
		// fromStart 为 true 表示下次打开时从头读取（轮转后的新文件）
		fromStart bool
		// since 为开始等待文件出现的时间：之后才出现的文件从头读取，之前就已存在的（例如稍后才挂载的存储卷）从末尾读取，
		// 避免把历史记录当成刚发生的失败
		since time.Time
		// old 为轮转后在宽限期内继续读取的旧文件
		old *rotatedFile
	)
	w := newTailWatcher(path)
	defer w.close()
	sp := &lineSplitter{onLine: onLine}
	buf := make([]byte, 32<<10)
	closeFile := func() {
		if f != nil {
			w.unwatchFile(fileWatch)
			fileWatch = -1
			f.Close()
			f = nil
		}
	}
	// closeOld 关闭轮转前的旧文件，drain 为 true 时先读完剩余内容
	closeOld := func(drain bool) {
		if old == nil {
			return
		}
		if drain {
			drainFile(old.f, old.sp, buf)
		}
		w.unwatchFile(old.watch)
		old.f.Close()
		old = nil
	}
	defer closeOld(false)
	defer closeFile()
	// wait 等待文件变化或超时；有旧文件时顺带读取旧文件的新内容，宽限期满后关闭
	wait := func(d time.Duration) bool {
		if old != nil {
			d = min(d, time.Until(old.until))
		}
		if !w.wait(ctx, d) {
			return false
		}
		if old != nil {
			if time.Now().Before(old.until) {
				drainFile(old.f, old.sp, buf)
			} else {
				closeOld(true)
			}
		}
		return true
	}
	for {
		if f == nil {
			st, err := os.Stat(path)
			if err == nil && !st.Mode().IsRegular() {
				err = errors.New("不是普通文件")
			}
			if err == nil {
				f, err = os.Open(path)
			}
			if err != nil {
				// 刚轮转、旧文件还在宽限期内时新文件暂未出现属于正常现象，不报告错误
				if old == nil {
					if errors.Is(err, os.ErrNotExist) {
						setErr("文件不存在")
					} else {
						setErr(err.Error())
					}
				}
				if since.IsZero() {
					since = time.Now()
				}
				fromStart = true
				if !wait(retryBackoff) {
					return
				}
				continue
			}
			// 先建立监视再读取：之后的写入一定会产生事件，之前的写入由下面的读取读到
			fileWatch = w.watchFile(f, path)
			info, _ = f.Stat()
			offset = 0
			// 等待期间才创建（或轮转产生）的新文件从头读取，其余从末尾开始
			if info != nil && (!fromStart || !since.IsZero() && info.ModTime().Before(since.Add(-mtimeSlack))) {
				offset = info.Size()
			}
			since = time.Time{}
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				setErr(err.Error())
				closeFile()
				if !wait(retryBackoff) {
					return
				}
				continue
			}
			sp.buf = sp.buf[:0]
			setErr("")
		}
		n, err := f.Read(buf)
		if n > 0 {
			offset += int64(n)
			sp.write(buf[:n])
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			setErr(err.Error())
			closeFile()
			if !wait(retryBackoff) {
				return
			}
			continue
		}
		// 已读到末尾：等待下一次写入事件或轮询超时
		if !wait(tailPoll) {
			return
		}
		// 检查文件是否被轮转或截断
		cur, statErr := os.Stat(path)
		switch {
		case statErr != nil || info != nil && !os.SameFile(info, cur):
			// 已被轮转（改名后文件可能短暂不存在）：先读完旧文件里现有的内容，写入方可能还没切到新文件，
			// 宽限期内继续读取旧文件，同时从头读取新文件
			drainFile(f, sp, buf)
			closeOld(true)
			old = &rotatedFile{f: f, watch: fileWatch, sp: sp, until: time.Now().Add(rotateGrace)}
			f, fileWatch = nil, -1
			sp = &lineSplitter{onLine: onLine}
			fromStart = true
		case cur.Size() < offset:
			if _, err := f.Seek(0, io.SeekStart); err == nil {
				offset = 0
				sp.buf = sp.buf[:0]
			}
		}
	}
}

// runCommand 运行持续输出日志的命令，按行处理标准输出；命令退出后按退避时间重启，直到 ctx 取消。
func runCommand(ctx context.Context, name string, args []string, onLine func(string), setErr func(string), log *logbuf.Logger) {
	backoff := retryBackoff
	for {
		start := time.Now()
		err := runOnce(ctx, name, args, onLine)
		if ctx.Err() != nil {
			return
		}
		msg := "命令已退出"
		if err != nil {
			msg = err.Error()
		}
		setErr(msg)
		log.Warnf("%s，%s 后重试", msg, backoff)
		// 连续运行超过一分钟视为正常，重新计算退避时间
		if time.Since(start) > maxBackoff {
			backoff = retryBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// stderrTail 保留标准错误输出的最后一段，用于报告命令失败原因。
type stderrTail struct {
	buf []byte
}

func (s *stderrTail) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	if len(s.buf) > 4096 {
		s.buf = s.buf[len(s.buf)-4096:]
	}
	return len(p), nil
}

func (s *stderrTail) last() string {
	lines := splitLines(string(s.buf))
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func runOnce(ctx context.Context, name string, args []string, onLine func(string)) error {
	if err := demo.Check(); err != nil {
		return err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("找不到命令 %s", name)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	script.Prepare(cmd)
	stderr := &stderrTail{}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sp := &lineSplitter{onLine: onLine}
	rd := bufio.NewReaderSize(out, 32<<10)
	buf := make([]byte, 32<<10)
	for {
		n, err := rd.Read(buf)
		if n > 0 {
			sp.write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	err = cmd.Wait()
	if last := stderr.last(); last != "" {
		return errors.New(last)
	}
	return err
}

// followDocker 跟踪容器日志，连接中断（容器重启、被删除等）后按退避时间重连。
func followDocker(ctx context.Context, follow func(context.Context, string, func(string)) error, name string, onLine func(string), setErr func(string)) {
	backoff := retryBackoff
	for {
		start := time.Now()
		err := follow(ctx, name, func(line string) {
			setErr("")
			onLine(line)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			setErr(err.Error())
		}
		if time.Since(start) > maxBackoff {
			backoff = retryBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}
