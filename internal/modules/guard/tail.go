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
	tailPoll     = time.Second
	retryBackoff = 5 * time.Second
	maxBackoff   = time.Minute
)

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

// tailFile 跟踪文件新增的内容，相当于 tail -F：启动时从文件末尾开始，文件被轮转（换成新文件）或截断时从头读取新文件。
// 文件不存在或无法读取时通过 setErr 报告并定期重试；只读取普通文件，FIFO、设备等会让打开或读取一直阻塞，直接报错。
func tailFile(ctx context.Context, path string, onLine func(string), setErr func(string)) {
	var (
		f      *os.File
		info   os.FileInfo
		offset int64
		// fromStart 为 true 表示下次打开时从头读取（轮转后的新文件）
		fromStart bool
		// since 为开始等待文件出现的时间：之后才出现的文件从头读取，之前就已存在的（例如稍后才挂载的存储卷）从末尾读取，
		// 避免把历史记录当成刚发生的失败
		since time.Time
	)
	sp := &lineSplitter{onLine: onLine}
	buf := make([]byte, 32<<10)
	closeFile := func() {
		if f != nil {
			f.Close()
			f = nil
		}
	}
	defer closeFile()
	wait := func(d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
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
				if errors.Is(err, os.ErrNotExist) {
					setErr("文件不存在")
				} else {
					setErr(err.Error())
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
			info, _ = f.Stat()
			offset = 0
			// 等待期间才创建（或轮转产生）的新文件从头读取，其余从末尾开始
			if info != nil && (!fromStart || !since.IsZero() && info.ModTime().Before(since)) {
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
		if !wait(tailPoll) {
			return
		}
		// 读到末尾后检查文件是否被轮转或截断
		cur, statErr := os.Stat(path)
		switch {
		case statErr != nil || info != nil && !os.SameFile(info, cur):
			// 已被轮转（改名后文件可能短暂不存在）：先读完旧文件里剩下的内容，再从头读取新文件
			for {
				n, err := f.Read(buf)
				if n > 0 {
					sp.write(buf[:n])
				}
				if n == 0 || err != nil {
					break
				}
			}
			fromStart = true
			closeFile()
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
