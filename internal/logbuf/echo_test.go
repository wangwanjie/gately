package logbuf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syncBuffer 是可并发读写的 bytes.Buffer。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

// gateWriter 在 release 关闭之前阻塞所有写入，模拟卡住的标准错误输出。
type gateWriter struct {
	syncBuffer
	release chan struct{}
}

func (g *gateWriter) Write(p []byte) (int, error) {
	<-g.release
	return g.syncBuffer.Write(p)
}

var stampRe = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)

// 异步回显每一行的内容与原来经标准库 log 同步输出的完全相同：时间格式、级别与来源的方括号、行尾换行的处理。
func TestEchoLineMatchesStdLog(t *testing.T) {
	var std, async syncBuffer
	b := New(100, log.New(&std, "", log.LstdFlags))
	echo := NewEcho(&async, nil)
	b.SetEcho(echo)
	msgs := []string{"普通一行", "", "行尾已有换行\n", "两个换行\n\n", "多行\n第二行", "多行带结尾\n第二行\n", "回车换行\r\n", "含 % 百分号与 [方括号]", strings.Repeat("长", 3000)}
	for i, m := range msgs {
		src := []string{"", "bench", "家里的NAS"}[i%3]
		b.AddFields([]string{LevelDebug, LevelInfo, LevelWarn, LevelError, "notice"}[i%5], src, m, nil)
	}
	b.Logger("fmt").Warnf("拒绝 %s 访问 %s（访问控制）", "1.2.3.4", "a.example.com")
	echo.Flush()
	want := strings.SplitAfter(std.String(), "\n")
	got := strings.SplitAfter(async.String(), "\n")
	if len(got) != len(want) {
		t.Fatalf("行数不同：异步 %d，标准库 %d", len(got), len(want))
	}
	entries := b.Since(0)
	n := 0
	for i := range want {
		if want[i] == "" {
			continue
		}
		if stampRe.MatchString(want[i]) != stampRe.MatchString(got[i]) {
			t.Fatalf("第 %d 行的时间前缀不一致:\n异步 %q\n标准 %q", i, got[i], want[i])
		}
		w, g := stampRe.ReplaceAllString(want[i], ""), stampRe.ReplaceAllString(got[i], "")
		if w != g {
			t.Errorf("第 %d 行不同:\n异步 %q\n标准 %q", i, g, w)
		}
		if stampRe.MatchString(got[i]) {
			// 时间取日志自身的时间
			if stamp := entries[n].Time.Format("2006/01/02 15:04:05 "); !strings.HasPrefix(got[i], stamp) {
				t.Errorf("第 %d 行的时间应为 %q: %q", i, stamp, got[i])
			}
			n++
		}
	}
}

// 单个协程写入的行按原顺序完整写出；Flush 返回时内容已全部到达输出。
func TestEchoOrderAndFlush(t *testing.T) {
	var out syncBuffer
	echo := NewEcho(&out, nil)
	b := New(10, nil)
	b.SetEcho(echo)
	l := b.Logger("")
	const n = 20000
	for i := 0; i < n; i++ {
		l.Infof("第 %d 行", i)
	}
	echo.Flush()
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("写出 %d 行，应为 %d", len(lines), n)
	}
	for i, ln := range lines {
		if !strings.HasSuffix(ln, fmt.Sprintf(" [info] 第 %d 行", i)) {
			t.Fatalf("第 %d 行内容或顺序不对: %q", i, ln)
		}
	}
	// 没有待写内容时 Flush 立即返回；nil 上可调用
	echo.Flush()
	var none *Echo
	none.Flush()
}

// 多个协程同时写：每一行都完整（不与别的行交错），每个协程自己的行保持先后顺序，一行不丢。
func TestEchoConcurrent(t *testing.T) {
	var out syncBuffer
	echo := NewEcho(&out, nil)
	b := New(100, nil)
	b.SetEcho(echo)
	const workers, per = 16, 2000
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			l := b.Logger("w" + strconv.Itoa(w))
			for i := 0; i < per; i++ {
				l.Infof("序号 %d", i)
			}
		}(w)
	}
	wg.Wait()
	echo.Flush()
	next := make([]int, workers)
	lineRe := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \[info\] \[w(\d+)\] 序号 (\d+)$`)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != workers*per {
		t.Fatalf("写出 %d 行，应为 %d", len(lines), workers*per)
	}
	for _, ln := range lines {
		m := lineRe.FindStringSubmatch(ln)
		if m == nil {
			t.Fatalf("行不完整或被交错: %q", ln)
		}
		w, _ := strconv.Atoi(m[1])
		i, _ := strconv.Atoi(m[2])
		if i != next[w] {
			t.Fatalf("协程 %d 的行顺序错乱：期望 %d，得到 %d", w, next[w], i)
		}
		next[w]++
	}
}

// 输出卡住时写日志的协程不被阻塞：积压超过上限后丢弃新的回显行，恢复后补一行说明丢了多少行。
func TestEchoDropsWhenOutputBlocked(t *testing.T) {
	g := &gateWriter{release: make(chan struct{})}
	echo := NewEcho(g, nil)
	b := New(10, nil)
	b.SetEcho(echo)
	l := b.Logger("")
	line := strings.Repeat("x", 1000)
	const n = 5000 // 约 5 MB，远超积压上限
	start := time.Now()
	for i := 0; i < n; i++ {
		l.Infof("%d %s", i, line)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("输出卡住时写日志被阻塞了 %v", d)
	}
	close(g.release)
	echo.Flush()
	// 丢弃说明在后台协程下一轮写出，再等它写完
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(g.String(), "没有回显") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	out := g.String()
	m := regexp.MustCompile(`\[warn\] 标准错误输出写入过慢，有 (\d+) 行日志没有回显`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("没有丢弃说明，输出共 %d 字节", len(out))
	}
	dropped, _ := strconv.Atoi(m[1])
	written := strings.Count(out, "\n") - 1
	if dropped == 0 || written == 0 || dropped+written != n {
		t.Fatalf("写出 %d 行、丢弃 %d 行，合计应为 %d", written, dropped, n)
	}
	if len(out) > 2*echoMaxPending+(2<<10) {
		t.Fatalf("积压了 %d 字节，超过上限", len(out))
	}
	// 恢复后新的行照常回显
	l.Infof("恢复之后")
	echo.Flush()
	if !strings.HasSuffix(g.String(), "恢复之后\n") {
		t.Fatal("恢复后的行没有写出")
	}
}

// 输出彻底卡死时 Flush 与同步写入最多等待 echoFlushWait，不会让进程退出一直卡住。
func TestEchoFlushGivesUp(t *testing.T) {
	old := echoFlushWait
	echoFlushWait = 100 * time.Millisecond
	defer func() { echoFlushWait = old }()
	g := &gateWriter{release: make(chan struct{})}
	defer close(g.release)
	echo := NewEcho(g, nil)
	start := time.Now()
	if n, err := echo.Write([]byte("卡住的一行\n")); n != len("卡住的一行\n") || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	echo.Flush()
	if d := time.Since(start); d < 150*time.Millisecond || d > 3*time.Second {
		t.Fatalf("等待了 %v，应约为两次 %v", d, echoFlushWait)
	}
}

// 经 Write 写入的内容（标准库 log 的输出）返回时已经写出，并排在此前异步回显的行之后：
// log.Fatalf 写完就退出进程，不能丢内容，顺序也不能乱。
func TestEchoWriteIsSynchronous(t *testing.T) {
	var out syncBuffer
	echo := NewEcho(&out, nil)
	b := New(10, nil)
	b.SetEcho(echo)
	std := log.New(echo, "", 0)
	for i := 0; i < 200; i++ {
		b.Add(LevelInfo, "", "异步 "+strconv.Itoa(i))
		std.Printf("同步 %d", i)
		if s := out.String(); !strings.HasSuffix(s, fmt.Sprintf("异步 %d\n同步 %d\n", i, i)) {
			t.Fatalf("第 %d 轮：Write 返回时内容未写出或顺序不对，末尾为 %q", i, s[max(0, len(s)-60):])
		}
	}
	if n, err := echo.Write(nil); n != 0 || err != nil {
		t.Fatalf("空写入 = %d, %v", n, err)
	}
}

// 翻译在写日志的协程里进行、不持有缓冲的锁（翻译函数里可以再读缓冲），并拿到格式串与参数长度。
func TestEchoTranslator(t *testing.T) {
	var out syncBuffer
	b := New(10, nil)
	type call struct {
		source, msg, format string
		lens                []uint16
	}
	var mu sync.Mutex
	var calls []call
	echo := NewEcho(&out, func(source, msg, format string, args ArgLens) (string, string) {
		// 持有 Buffer.mu 时调用翻译会在这里死锁
		_ = b.Since(0)
		mu.Lock()
		calls = append(calls, call{source, msg, format, append([]uint16(nil), args.Slice()...)})
		mu.Unlock()
		return strings.ToUpper(source), "<" + msg + ">"
	})
	b.SetEcho(echo)
	b.Logger("src").Warnf("拒绝 %s 访问 %s", "1.2.3.4", "h")
	b.Add(LevelInfo, "", "整句")
	echo.Flush()
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], " [warn] [SRC] <拒绝 1.2.3.4 访问 h>") || !strings.HasSuffix(lines[1], " [info] <整句>") {
		t.Fatalf("输出不对: %q", lines)
	}
	if len(calls) != 2 || calls[0].format != "拒绝 %s 访问 %s" || len(calls[0].lens) != 2 || calls[0].lens[0] != 7 || calls[1].format != "" || len(calls[1].lens) != 0 {
		t.Fatalf("翻译函数收到的参数不对: %+v", calls)
	}
	// 缓冲与文件里保存的仍是原文
	if got := b.Since(0); got[0].Message != "拒绝 1.2.3.4 访问 h" || got[0].Source != "src" {
		t.Fatalf("缓冲里的日志被改写: %+v", got[0])
	}
}

// TestEchoHelperProcess 不是独立的测试：TestEchoFlushedOnExit 以子进程方式运行它，模拟服务进程。
// 子进程把日志回显到自己的标准错误输出并落盘；exit 模式写完后正常退出，signal 模式持续写日志直到收到 SIGTERM，
// panic 模式写完后直接 panic（Flush 像主程序那样放在 defer 里）。由父进程检查标准错误输出是否完整。
func TestEchoHelperProcess(t *testing.T) {
	mode := os.Getenv("GATELY_ECHO_HELPER")
	if mode == "" {
		t.Skip("仅作为子进程运行")
	}
	echo := NewEcho(os.Stderr, nil)
	b := New(100, nil)
	b.SetEcho(echo)
	if err := b.Persist(os.Getenv("GATELY_ECHO_LOG"), 64<<20); err != nil {
		fmt.Fprintln(os.Stdout, "persist:", err)
		os.Exit(3)
	}
	std := log.New(echo, "", log.LstdFlags)
	l := b.Logger("helper")
	std.Printf("started")
	i := 0
	burst := func(n int) {
		for end := i + n; i < end; i++ {
			l.Infof("line %d %s", i, strings.Repeat("y", i%120))
		}
	}
	switch mode {
	case "exit":
		burst(30000)
	case "panic":
		defer echo.Flush()
		burst(30000)
		b.ClosePersist()
		panic("helper panic")
	case "signal":
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM)
		fmt.Fprintln(os.Stdout, "ready")
		for running := true; running; {
			select {
			case <-sig:
				running = false
			default:
				burst(500)
			}
		}
		// 收到信号后的收尾日志也要写出
		burst(1000)
		l.Warnf("shutting down after %d lines", i)
	}
	b.ClosePersist()
	echo.Flush()
	os.Exit(0)
}

// 进程正常退出、收到 SIGTERM 退出或主协程 panic 时，退出前的 Flush 把异步回显的内容全部写出：
// 子进程标准错误输出里的日志行与它落盘的日志文件逐条对应，一行不少、顺序相同、最后一行完整；
// panic 时运行时打印的崩溃信息排在全部日志之后。
func TestEchoFlushedOnExit(t *testing.T) {
	if testing.Short() {
		t.Skip("需要启动子进程")
	}
	for _, mode := range []string{"exit", "signal", "panic"} {
		t.Run(mode, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "helper.log")
			cmd := exec.Command(os.Args[0], "-test.run=^TestEchoHelperProcess$")
			cmd.Env = append(os.Environ(), "GATELY_ECHO_HELPER="+mode, "GATELY_ECHO_LOG="+logPath)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if mode == "signal" {
				// 等子进程装好信号处理、开始写日志后再发信号
				buf := make([]byte, 64)
				if n, _ := stdout.Read(buf); !strings.Contains(string(buf[:n]), "ready") {
					t.Fatalf("子进程没有就绪: %q", buf[:n])
				}
				time.Sleep(200 * time.Millisecond)
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
			}
			err = cmd.Wait()
			if mode == "panic" {
				if err == nil {
					t.Fatal("panic 模式的子进程应以非零状态退出")
				}
			} else if err != nil {
				t.Fatalf("子进程退出异常: %v\n%s", err, tail(stderr.String(), 500))
			}
			out := stderr.String()
			if mode == "panic" {
				// 崩溃信息由测试框架与运行时直接写到标准错误输出，必须排在回显的全部日志之后
				i := strings.Index(out, "panic: helper panic")
				if i < 0 {
					t.Fatalf("没有崩溃信息: %q", tail(out, 300))
				}
				if j := strings.LastIndex(out[:i], " [info] [helper] line "); j < 0 || strings.Contains(out[i:], " [info] [helper] line ") {
					t.Fatalf("崩溃信息之后还有日志行，说明 panic 前没有写完")
				}
				out = out[:strings.LastIndex(out[:i], "\n")+1]
				// 测试框架在崩溃信息前打印的“--- FAIL”一行不属于回显
				if k := strings.Index(out, "--- FAIL"); k >= 0 {
					out = out[:k]
				}
			}
			if !strings.HasSuffix(out, "\n") {
				t.Fatalf("标准错误输出的最后一行不完整: %q", tail(out, 200))
			}
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) == 0 || !strings.HasSuffix(lines[0], " started") {
				t.Fatalf("第一行应为启动信息: %q", lines[0])
			}
			lines = lines[1:]
			// 日志文件是落盘的权威记录，回显应与它逐条对应
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			var entries []Entry
			for _, ln := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
				var e Entry
				if err := json.Unmarshal(ln, &e); err != nil {
					t.Fatalf("日志文件里有无法解析的行: %q", ln)
				}
				entries = append(entries, e)
			}
			if mode != "signal" && len(entries) != 30000 {
				t.Fatalf("日志文件有 %d 条，应为 30000", len(entries))
			}
			if len(entries) < 1000 || len(lines) != len(entries) {
				t.Fatalf("标准错误输出有 %d 行日志，日志文件有 %d 条", len(lines), len(entries))
			}
			for i, e := range entries {
				want := e.Time.Format("2006/01/02 15:04:05 ") + "[" + e.Level + "] [helper] " + e.Message
				if lines[i] != want {
					t.Fatalf("第 %d 行与日志文件不一致:\n回显 %q\n文件 %q", i, lines[i], want)
				}
				if e.Seq != uint64(i+1) {
					t.Fatalf("日志文件第 %d 条的序号为 %d", i, e.Seq)
				}
			}
			if mode == "signal" && !strings.Contains(lines[len(lines)-1], "shutting down after") {
				t.Fatalf("收到信号后的最后一条日志没有回显: %q", lines[len(lines)-1])
			}
		})
	}
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
