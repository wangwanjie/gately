// Package proc 托管外部可执行文件（frpc、cloudflared、tailscaled 等）：
// 查找二进制、后台运行、异常退出自动重启、把输出写入模块日志。
package proc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/wangwanjie/gately/internal/events"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

const (
	// stopGrace 是发送终止信号后等待进程自行退出的时间，超时强制结束
	stopGrace = 5 * time.Second
	// maxBackoff 是异常退出后重启等待时间的上限
	maxBackoff = 60 * time.Second
	// stableRun 表示进程连续运行超过该时长即视为稳定，重启等待时间归零
	stableRun = 30 * time.Second
)

// Resolve 查找可执行文件：优先使用配置的路径，其次是配置目录下的 bin 子目录，最后是 PATH。
// bin 放在 PATH 之前：界面中在线安装、更新或回滚的程序都在 bin 下，应当优先生效。
// 返回绝对路径：子进程可能在其他工作目录启动（如 File Browser），相对路径会按该目录解析而找不到。
func Resolve(configured, configDir string, names ...string) (string, error) {
	p, err := resolve(configured, configDir, names...)
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func resolve(configured, configDir string, names ...string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if strings.ContainsRune(configured, filepath.Separator) {
			if st, err := os.Stat(configured); err != nil || st.IsDir() {
				return "", fmt.Errorf("指定的可执行文件不存在: %s", configured)
			}
			return configured, nil
		}
		p, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("在 PATH 中找不到 %s", configured)
		}
		return p, nil
	}
	for _, n := range names {
		local := filepath.Join(configDir, "bin", n)
		if runtime.GOOS == "windows" {
			local += ".exe"
		}
		if st, err := os.Stat(local); err == nil && !st.IsDir() {
			return local, nil
		}
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 %s，请点击“下载安装”，或在设置中填写可执行文件路径", names[0])
}

// Version 返回 `<path> <args...>` 输出的第一行，失败时返回空串。
func Version(path string, args ...string) string {
	out, err := Output(3*time.Second, path, args, nil)
	if err != nil && len(out) == 0 {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 120 {
				line = line[:120]
			}
			return line
		}
	}
	return ""
}

// Output 运行命令并返回合并后的输出，超时后强制结束。
func Output(timeout time.Duration, path string, args, env []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if ctx.Err() != nil {
		err = fmt.Errorf("执行超时（%s）", timeout)
	}
	return buf.Bytes(), err
}

// Spec 描述要运行的进程。
type Spec struct {
	Path string
	Args []string
	// Env 追加到当前进程环境变量之后，用于传递令牌等不宜出现在命令行的参数
	Env []string
	Dir string
	// OnLine 在每行输出（已去除颜色码与时间戳）到达时调用，可为 nil
	OnLine func(line string)
	// OnStart 在进程每次成功启动后调用，可为 nil
	OnStart func()
	// LogLine 在写入日志前改写一行输出（如隐藏登录链接），可为 nil；OnLine 收到的仍是原始内容
	LogLine func(line string) string
}

// State 是进程运行状态。
type State struct {
	Running  bool      `json:"running"`
	PID      int       `json:"pid,omitempty"`
	Since    time.Time `json:"since,omitempty"`
	Restarts int       `json:"restarts"`
	// LastExit 记录最近一次退出原因
	LastExit string `json:"lastExit,omitempty"`
	// Error 非空表示无法启动（例如二进制不存在），不会自动重试
	Error string `json:"error,omitempty"`
}

// Supervisor 托管一个长期运行的进程。
type Supervisor struct {
	log *logbuf.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	state  State
}

// New 创建托管器，log 接收进程输出与托管事件。
func New(log *logbuf.Logger) *Supervisor {
	return &Supervisor{log: log}
}

// State 返回当前状态快照。
func (s *Supervisor) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Start 启动进程；已在运行时先停止旧进程。
func (s *Supervisor) Start(spec Spec) {
	s.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	s.cancel, s.done = cancel, done
	s.state = State{}
	s.mu.Unlock()
	go s.loop(ctx, spec, done)
}

// Stop 停止进程并等待其退出，未运行时无操作。
func (s *Supervisor) Stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (s *Supervisor) loop(ctx context.Context, spec Spec, done chan struct{}) {
	defer close(done)
	backoff := time.Second
	for {
		started := time.Now()
		startErr, exitErr := s.runOnce(ctx, spec)
		if ctx.Err() != nil {
			break
		}
		if startErr != nil {
			// 无法启动（二进制缺失、无执行权限）重试没有意义，等待用户修正配置
			s.mu.Lock()
			s.state.Error = startErr.Error()
			s.mu.Unlock()
			s.log.Errorf("启动失败: %v", startErr)
			return
		}
		reason := "正常退出"
		if exitErr != nil {
			reason = exitErr.Error()
		}
		s.mu.Lock()
		s.state.LastExit = reason
		s.state.Restarts++
		s.mu.Unlock()
		if time.Since(started) > stableRun {
			backoff = time.Second
		}
		s.log.Warnf("进程已退出（%s），%s 后重启", reason, backoff)
		events.Publish(events.Event{
			Type:    events.ProcessExited,
			Source:  s.log.Source(),
			Level:   events.LevelError,
			Title:   s.log.Source() + " 进程异常退出",
			Message: "退出原因：" + reason + "，将自动重启。",
		})
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		if ctx.Err() != nil {
			break
		}
		backoff = min(backoff*2, maxBackoff)
	}
	s.mu.Lock()
	s.state.Running, s.state.PID = false, 0
	s.mu.Unlock()
}

// runOnce 运行一次进程。startErr 表示未能启动，exitErr 表示进程退出时的错误。
func (s *Supervisor) runOnce(ctx context.Context, spec Spec) (startErr, exitErr error) {
	return execOnce(ctx, spec, s.log, func(pid int) {
		s.mu.Lock()
		s.state.Running, s.state.PID, s.state.Since = true, pid, time.Now()
		s.state.Error = ""
		s.mu.Unlock()
	}, func() {
		s.mu.Lock()
		s.state.Running, s.state.PID = false, 0
		s.mu.Unlock()
	})
}

// Run 运行一次命令并把输出写入日志，进程退出后返回。ctx 取消时先发送终止信号。
func Run(ctx context.Context, spec Spec, log *logbuf.Logger) error {
	startErr, exitErr := execOnce(ctx, spec, log, nil, nil)
	if startErr != nil {
		return startErr
	}
	return exitErr
}

// execOnce 启动进程并等待其退出，输出按行写入日志。
// onStarted 与 onExited 分别在启动成功后与退出后调用，可为 nil。
func execOnce(ctx context.Context, spec Spec, log *logbuf.Logger, onStarted func(pid int), onExited func()) (startErr, exitErr error) {
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Dir = spec.Dir
	// 先请求进程优雅退出，stopGrace 后由 WaitDelay 强制结束
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = stopGrace
	w := &lineWriter{handle: func(line string) {
		logged := line
		if spec.LogLine != nil {
			logged = spec.LogLine(line)
		}
		log.Log(classify(logged), logged)
		if spec.OnLine != nil {
			spec.OnLine(line)
		}
	}}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		// 二进制缺失、无执行权限等启动错误交给调用方，不进入重启循环
		return err, nil
	}
	if onStarted != nil {
		onStarted(cmd.Process.Pid)
	}
	log.Infof("已启动 %s（PID %d）", filepath.Base(spec.Path), cmd.Process.Pid)
	if spec.OnStart != nil {
		spec.OnStart()
	}
	err := cmd.Wait()
	w.flush()
	if onExited != nil {
		onExited()
	}
	return nil, err
}

// lineWriter 按行拆分输出。
type lineWriter struct {
	mu     sync.Mutex
	buf    []byte
	handle func(line string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	// 防御没有换行的超长输出
	if len(w.buf) > 64*1024 {
		w.emit(string(w.buf))
		w.buf = nil
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (w *lineWriter) emit(line string) {
	line = Clean(line)
	if line != "" {
		w.handle(line)
	}
}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	// 行首时间戳：2006-01-02T15:04:05Z / 2006/01/02 15:04:05.000 / 2006-01-02 15:04:05
	tsRe = regexp.MustCompile(`^\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?\s*`)
)

// Clean 去除颜色码、回车与行首时间戳。
func Clean(line string) string {
	line = ansiRe.ReplaceAllString(line, "")
	line = strings.TrimSpace(strings.ReplaceAll(line, "\r", ""))
	return strings.TrimSpace(tsRe.ReplaceAllString(line, ""))
}

var (
	// ERRO、FATA、PANI、DEBU、TRAC 是 logrus 文本格式的四字母级别（NetBird 等使用）
	errRe   = regexp.MustCompile(`\[E\]|\bERR\b|\bERRO\b|\bERROR\b|\blevel=error\b|\bFATAL?\b|\bPANI\b`)
	warnRe  = regexp.MustCompile(`\[W\]|\bWRN\b|\bWARN(ING)?\b|\blevel=warn`)
	debugRe = regexp.MustCompile(`\[D\]|\bDBG\b|\bDEBUG?\b|\bTRAC\b`)
)

// classify 根据 frp（[I] [W] [E]）与 cloudflared（INF WRN ERR）的日志格式判断级别。
func classify(line string) (level string) {
	switch {
	case errRe.MatchString(line):
		level = logbuf.LevelError
	case warnRe.MatchString(line):
		level = logbuf.LevelWarn
	case debugRe.MatchString(line):
		level = logbuf.LevelDebug
	default:
		level = logbuf.LevelInfo
	}
	return level
}

// loginURLRe 匹配输出中的网址
var loginURLRe = regexp.MustCompile(`(https?://[^/\s]+)(/\S*)?`)

// MaskURLPaths 把一行输出中网址的路径与查询部分换成 ********，只保留协议与主机。
// 用于 tailscale up、netbird up 等只会输出登录链接的命令：日志对只读身份可见，
// 登录链接（含设备码）可用于把本机登记到他人的网络，完整链接只通过需要写权限的状态接口提供。
func MaskURLPaths(line string) string {
	return loginURLRe.ReplaceAllStringFunc(line, func(u string) string {
		m := loginURLRe.FindStringSubmatch(u)
		if m[2] == "" || m[2] == "/" {
			return u
		}
		return m[1] + "/********"
	})
}
