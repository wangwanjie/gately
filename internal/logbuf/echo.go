package logbuf

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// 异步回显的参数
const (
	// echoMaxPending 限制尚未写出的回显数据：标准错误输出阻塞或太慢时（终端被暂停、docker 日志驱动卡住）
	// 丢弃新的回显行，不拖慢写日志的协程。日志页与日志文件不经过这里，不受影响
	echoMaxPending = 1 << 20
	// echoIdleKeep 为空闲时保留的缓冲区大小：洪峰期间两块缓冲区（正在写的与正在攒的）反复复用，
	// 最大各为 echoMaxPending；写完进入空闲后超过这个大小的缓冲区交还给运行时
	echoIdleKeep = 64 << 10
	// echoMinInterval 为两次写出之间的最短间隔：日志很密时把这段时间里到来的行攒成一次写入，
	// 每秒最多写这么多次；空闲后的第一行不受影响，立刻写出
	echoMinInterval = time.Millisecond
)

// echoFlushWait 为 Flush 与同步写入等待数据写出的最长时间：标准错误输出彻底卡死时不让进程退出一直等下去
var echoFlushWait = 3 * time.Second

// EchoTranslator 把一条日志翻译为回显使用的语言：source 为来源标签，msg 为内容，
// format、args 为生成 msg 的格式串与各参数的长度（没有时为零值）。返回翻译后的来源与内容。
type EchoTranslator func(source, msg, format string, args ArgLens) (string, string)

// Echo 把日志行异步写到标准错误输出（docker logs、journalctl 看到的内容）。
//
// 写日志的协程只把格式化好的一行追加到内存里的待写数据，由后台协程写出：空闲时来的第一行立刻写，
// 之后一毫秒内以及写的过程中到来的行攒在一起下次一并写出。高并发下每行不再各做一次系统调用，也不在全局锁里等前一行写完。
// 进程退出前调用 Flush；经 Write 写入的内容（标准库 log 的输出，如启动失败的 log.Fatalf）会等写出后才返回。
type Echo struct {
	w  io.Writer
	tr EchoTranslator

	mu sync.Mutex
	// buf 为待写出的数据，spare 为上一轮写完后留下的空缓冲
	buf, spare []byte
	// dropped 为因积压过多而丢弃的行数，下次写出时补一行说明
	dropped int
	// running 表示后台协程正在写（或即将开始写）
	running bool
	// queued、written 为累计进入待写数据与已写出（含写失败丢弃）的字节数，Flush 据此等待
	queued, written uint64
	waiters         []echoWaiter
	wake            chan struct{}
	started         bool
}

type echoWaiter struct {
	target uint64
	done   chan struct{}
}

// NewEcho 创建写到 w 的异步回显。tr 不为 nil 时每条日志先经它翻译，翻译在写日志的协程里进行，不持有任何锁。
func NewEcho(w io.Writer, tr EchoTranslator) *Echo {
	return &Echo{w: w, tr: tr, wake: make(chan struct{}, 1)}
}

// linePool 复用拼装回显行的缓冲区
var linePool = sync.Pool{New: func() any {
	b := make([]byte, 0, 256)
	return &b
}}

// entry 回显一条日志，格式与标准库 log 的默认输出相同：“2006/01/02 15:04:05 [级别] [来源] 内容”。
func (e *Echo) entry(en *Entry) {
	source, msg := en.Source, en.Message
	if e.tr != nil {
		// 参数长度按值传递：翻译函数是外部传入的，传切片会让整条日志逃逸到堆上
		source, msg = e.tr(source, msg, en.Format, en.Args)
	}
	bp := linePool.Get().(*[]byte)
	b := en.Time.AppendFormat((*bp)[:0], "2006/01/02 15:04:05 ")
	b = append(b, '[')
	b = append(b, en.Level...)
	b = append(b, "] "...)
	if source != "" {
		b = append(b, '[')
		b = append(b, source...)
		b = append(b, "] "...)
	}
	b = append(b, msg...)
	if len(msg) == 0 || msg[len(msg)-1] != '\n' {
		b = append(b, '\n')
	}
	e.enqueue(b)
	if cap(b) <= 4096 {
		*bp = b
		linePool.Put(bp)
	}
}

// enqueue 把一段数据追加到待写数据并唤醒后台协程，返回追加后的累计字节数；积压超过上限时丢弃并返回 0。
func (e *Echo) enqueue(p []byte) uint64 {
	e.mu.Lock()
	if len(e.buf)+len(p) > echoMaxPending && len(e.buf) > 0 {
		e.dropped++
		e.mu.Unlock()
		return 0
	}
	e.buf = append(e.buf, p...)
	e.queued += uint64(len(p))
	target := e.queued
	e.kickLocked()
	e.mu.Unlock()
	return target
}

// kickLocked 确保后台协程在运行，调用方持有 e.mu。
func (e *Echo) kickLocked() {
	if !e.started {
		e.started = true
		go e.loop()
	}
	if !e.running {
		e.running = true
		select {
		case e.wake <- struct{}{}:
		default:
		}
	}
}

// loop 是后台写出协程：被唤醒后把待写数据一批批写出，直到没有积压。
func (e *Echo) loop() {
	// last 为上一次开始写出的时间
	var last time.Time
	for range e.wake {
		for {
			// 距上次写出太近时稍等片刻，让这期间到来的行一起写出
			if d := echoMinInterval - time.Since(last); d > 0 {
				time.Sleep(d)
			}
			last = time.Now()
			e.mu.Lock()
			if len(e.buf) == 0 && e.dropped == 0 {
				e.running = false
				if cap(e.buf) > echoIdleKeep {
					e.buf = nil
				}
				if cap(e.spare) > echoIdleKeep {
					e.spare = nil
				}
				e.notifyLocked()
				e.mu.Unlock()
				break
			}
			data, dropped := e.buf, e.dropped
			e.buf, e.spare, e.dropped = e.spare[:0], nil, 0
			e.mu.Unlock()
			n := uint64(len(data))
			if dropped > 0 {
				data = append(data, e.droppedNotice(dropped)...)
			}
			// 写失败（标准错误输出已关闭等）时这批数据丢弃，日志页与日志文件里仍有
			_, _ = e.w.Write(data)
			e.mu.Lock()
			e.written += n
			e.spare = data[:0]
			e.notifyLocked()
			e.mu.Unlock()
		}
	}
}

// droppedNotice 生成一行“有若干行没有回显”的说明。
func (e *Echo) droppedNotice(n int) []byte {
	msg := fmt.Sprintf("标准错误输出写入过慢，有 %d 行日志没有回显（日志页与日志文件不受影响）", n)
	if e.tr != nil {
		_, msg = e.tr("", msg, "", ArgLens{})
	}
	b := time.Now().AppendFormat(nil, "2006/01/02 15:04:05 ")
	b = append(b, "[warn] "...)
	b = append(b, msg...)
	return append(b, '\n')
}

// notifyLocked 唤醒已经等到目标的 Flush，调用方持有 e.mu。
func (e *Echo) notifyLocked() {
	keep := e.waiters[:0]
	for _, w := range e.waiters {
		if e.written >= w.target {
			close(w.done)
		} else {
			keep = append(keep, w)
		}
	}
	e.waiters = keep
}

// waitFor 等待累计写出的字节数达到 target，最多等 echoFlushWait。
func (e *Echo) waitFor(target uint64) {
	e.mu.Lock()
	if e.written >= target {
		e.mu.Unlock()
		return
	}
	done := make(chan struct{})
	e.waiters = append(e.waiters, echoWaiter{target: target, done: done})
	e.kickLocked()
	e.mu.Unlock()
	t := time.NewTimer(echoFlushWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	}
}

// Flush 等待此前交给回显的内容全部写出，在进程退出、重启之前调用。可在 nil 上调用。
func (e *Echo) Flush() {
	if e == nil {
		return
	}
	e.mu.Lock()
	target := e.queued
	e.mu.Unlock()
	e.waitFor(target)
}

// Write 实现 io.Writer，供标准库 log 使用：内容排在已有的回显行之后，并等它写出后才返回，
// 这样 log.Fatalf 之类写完就退出进程的调用不会丢内容，顺序也与异步回显的行一致。
// 这类输出很少（启动信息、落盘失败等），同步等待不影响性能。
func (e *Echo) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if target := e.enqueue(p); target > 0 {
		e.waitFor(target)
	}
	return len(p), nil
}
