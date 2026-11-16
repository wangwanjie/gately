// Gately 新增：连接处理中的 panic 只断开当前连接，进程与监听继续工作。

//go:build linux || darwin

package parent

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	smb2c "github.com/cloudsoda/go-smb2"

	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/userdb"
)

// capturePanics 在测试期间记录被捕获的 panic 发生位置。
func capturePanics(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var got []string
	connPanicHook = func(where string, _ any) {
		mu.Lock()
		got = append(got, where)
		mu.Unlock()
	}
	t.Cleanup(func() { connPanicHook = nil })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

func TestConnPoolRecoversWorkerPanic(t *testing.T) {
	panics := capturePanics(t)
	p := newConnPool(1)
	p.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	dropped := make(chan struct{})
	p.onPanic = func() { close(dropped) }
	p.submit(func() { panic("boom") })
	select {
	case <-dropped:
	case <-time.After(5 * time.Second):
		t.Fatal("工作协程 panic 后应调用 onPanic")
	}
	p.drain()
	// 唯一的名额应已归还，后续帧照常执行
	ran := make(chan struct{})
	p.submit(func() { close(ran) })
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("panic 之后工作池应仍可使用")
	}
	p.drain()
	if got := panics(); len(got) != 1 || got[0] != "frame" {
		t.Fatalf("捕获记录 %v，期望 [frame]", got)
	}
}

// TestServeConnRecoversPanic 让登录成功后的钩子 panic：只有这条连接被断开，同一监听上的下一条连接照常登录。
func TestServeConnRecoversPanic(t *testing.T) {
	panics := capturePanics(t)
	users := []config.UserConfig{{Name: "alice", NTHash: userdb.NTHash("pw"), AllowShares: []string{"*"},
		Shares: []config.ShareConfig{{Name: "share", Path: t.TempDir()}}}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	var mu sync.Mutex
	l := &Listener{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxFrame: 1 << 20,
		Handler: func(ctx context.Context, c net.Conn, lg *slog.Logger, maxFrame uint32) {
			ServeConn(ctx, c, lg, maxFrame, ConnOptions{RequireSigning: true, MaxIOSize: 1 << 20, Users: users,
				OnAuthenticated: func(*Session) error {
					mu.Lock()
					calls++
					first := calls == 1
					mu.Unlock()
					if first {
						panic("post-auth hook failure")
					}
					return nil
				}})
		},
	}
	done := make(chan error, 1)
	go func() { done <- l.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	login := func() error {
		c, err := net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second)
		if err != nil {
			return err
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		d := &smb2c.Dialer{Initiator: &smb2c.NTLMInitiator{User: "alice", Password: "pw"}}
		s, err := d.DialConn(context.Background(), c, ln.Addr().String())
		if err != nil {
			return err
		}
		_, err = s.ListSharenames()
		_ = s.Logoff()
		return err
	}
	if err := login(); err == nil {
		t.Fatal("钩子 panic 的连接应被断开")
	}
	if err := login(); err != nil {
		t.Fatalf("panic 之后新连接应照常登录: %v", err)
	}
	if got := panics(); len(got) != 1 || got[0] != "connection" {
		t.Fatalf("捕获记录 %v，期望 [connection]", got)
	}
}

// TestListenerRecoversHandlerPanic：处理函数自身 panic 时，监听继续接受新连接。
func TestListenerRecoversHandlerPanic(t *testing.T) {
	panics := capturePanics(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{}, 2)
	l := &Listener{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxFrame: 1 << 20,
		Handler: func(_ context.Context, c net.Conn, _ *slog.Logger, _ uint32) {
			served <- struct{}{}
			panic("handler bug")
		},
	}
	done := make(chan error, 1)
	go func() { done <- l.Serve(ctx, ln) }()
	for i := 0; i < 2; i++ {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Fatalf("第 %d 条连接未被处理", i+1)
		}
		// 连接应被关闭：读到 EOF
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("panic 后连接应被关闭，读取结果 %v", err)
		}
		c.Close()
	}
	cancel()
	<-done
	if got := panics(); len(got) != 2 {
		t.Fatalf("捕获记录 %v，期望两次", got)
	}
}
