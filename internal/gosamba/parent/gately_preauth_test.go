// Gately 新增：认证前的超时、帧大小与并发上限。

//go:build linux || darwin

package parent

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	smb2c "github.com/cloudsoda/go-smb2"

	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
	"github.com/wangwanjie/gately/internal/gosamba/userdb"
)

func TestPreAuthSlots(t *testing.T) {
	s := &PreAuthSlots{MaxTotal: 3, MaxPerIP: 2}
	if !s.acquire("192.0.2.1") || !s.acquire("192.0.2.1") {
		t.Fatal("同一来源前两条应获得名额")
	}
	if s.acquire("192.0.2.1") {
		t.Fatal("同一来源第三条应被拒绝")
	}
	// IPv4 映射的 IPv6 地址与 IPv4 地址按同一来源计数
	if s.acquire("::ffff:192.0.2.1") {
		t.Fatal("IPv4 映射地址应计入同一来源")
	}
	if !s.acquire("2001:db8::1") {
		t.Fatal("其他来源应获得名额")
	}
	if s.acquire("2001:db8:1::1") {
		t.Fatal("达到全局上限后应拒绝")
	}
	s.release("192.0.2.1")
	if !s.acquire("2001:db8::2") {
		t.Fatal("归还后应可再获得名额")
	}
	// 同一 /64 内的 IPv6 地址按同一来源计数
	if s.acquire("2001:db8::3") {
		t.Fatal("全局已满，应拒绝")
	}
	s.release("2001:db8::1")
	s.release("2001:db8::2")
	s.release("192.0.2.1")
	if n := s.InUse(); n != 0 {
		t.Fatalf("全部归还后占用 %d，期望 0", n)
	}
	if len(s.perIP) != 0 {
		t.Fatalf("全部归还后不应保留来源记录: %v", s.perIP)
	}
	v6 := &PreAuthSlots{MaxPerIP: 1}
	if !v6.acquire("2001:db8::1") || v6.acquire("2001:db8::ffff") {
		t.Fatal("同一 /64 内的地址应共用单来源上限")
	}
	if !v6.acquire("2001:db8:0:1::1") {
		t.Fatal("不同 /64 应分别计数")
	}
}

func TestPreAuthReadDeadline(t *testing.T) {
	start := time.Unix(1000, 0)
	l := &PreAuthLimits{IdleTimeout: 30 * time.Second, Deadline: 60 * time.Second}
	if got := l.readDeadline(start, start); !got.Equal(start.Add(30 * time.Second)) {
		t.Fatalf("开始时应按每帧超时，得到 %v", got)
	}
	if got := l.readDeadline(start, start.Add(45*time.Second)); !got.Equal(start.Add(60 * time.Second)) {
		t.Fatalf("接近总时限时应按总时限，得到 %v", got)
	}
	if got := (&PreAuthLimits{}).readDeadline(start, start); !got.IsZero() {
		t.Fatalf("都不限时不应设截止时间，得到 %v", got)
	}
	if got := (&PreAuthLimits{MaxFrame: 1 << 17}).frameLimit(1 << 24); got != 1<<17 {
		t.Fatalf("认证前帧上限 %d", got)
	}
	if got := (&PreAuthLimits{MaxFrame: 1 << 24}).frameLimit(1 << 20); got != 1<<20 {
		t.Fatalf("认证前帧上限不应超过整体上限，得到 %d", got)
	}
}

// preAuthServer 在随机端口启动服务，返回地址与已结束连接数的读取函数。
func preAuthServer(t *testing.T, limits *PreAuthLimits) (string, func() int) {
	t.Helper()
	users := []config.UserConfig{{Name: "alice", NTHash: userdb.NTHash("pw"), AllowShares: []string{"*"},
		Shares: []config.ShareConfig{{Name: "share", Path: t.TempDir()}}}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	ended := 0
	l := &Listener{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxFrame: transport.MaxFrameSize,
		Handler: func(ctx context.Context, c net.Conn, lg *slog.Logger, maxFrame uint32) {
			ServeConn(ctx, c, lg, maxFrame, ConnOptions{RequireSigning: true, MaxIOSize: 8 << 20, Users: users, PreAuth: limits})
			mu.Lock()
			ended++
			mu.Unlock()
		},
	}
	done := make(chan error, 1)
	go func() { done <- l.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return ln.Addr().String(), func() int {
		mu.Lock()
		defer mu.Unlock()
		return ended
	}
}

// waitClosed 等待服务端关闭连接（读到 EOF 或连接重置），返回耗时；超过 limit 视为失败。
func waitClosed(t *testing.T, c net.Conn, limit time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	_ = c.SetReadDeadline(start.Add(limit))
	buf := make([]byte, 4096)
	for {
		if _, err := c.Read(buf); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatalf("%v 内连接未被服务端关闭", limit)
			}
			return time.Since(start)
		}
	}
}

func writeFrame(t *testing.T, c net.Conn, payload []byte) {
	t.Helper()
	var b bytes.Buffer
	if err := transport.WriteFrame(&b, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(b.Bytes()); err != nil {
		t.Fatal(err)
	}
}

func TestServeConnPreAuthIdleTimeout(t *testing.T) {
	addr, _ := preAuthServer(t, &PreAuthLimits{IdleTimeout: 300 * time.Millisecond, Deadline: 5 * time.Second})
	// 连上后什么也不发：协商阶段也要受截止时间限制
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if d := waitClosed(t, c, 3*time.Second); d < 200*time.Millisecond {
		t.Fatalf("连接过早被关闭: %v", d)
	}
	// 协商后不再发送：同样超时断开
	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	writeFrame(t, c2, buildClientNegotiate311())
	waitClosed(t, c2, 3*time.Second)
}

func TestServeConnPreAuthTotalDeadline(t *testing.T) {
	addr, _ := preAuthServer(t, &PreAuthLimits{IdleTimeout: 400 * time.Millisecond, Deadline: 800 * time.Millisecond})
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	writeFrame(t, c, buildClientNegotiate311())
	// 每 200 毫秒发一个 NTLM 协商消息（SESSION_SETUP 第一步），始终不完成登录：
	// 每帧超时不会触发，总时限到达后连接被关闭
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		type1 := append([]byte("NTLMSSP\x00\x01\x00\x00\x00"), make([]byte, 28)...)
		var b bytes.Buffer
		_ = transport.WriteFrame(&b, fuzzSessionSetup(type1, 0))
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
				if _, err := c.Write(b.Bytes()); err != nil {
					return
				}
			}
		}
	}()
	start := time.Now()
	waitClosed(t, c, 3*time.Second)
	if d := time.Since(start); d < 500*time.Millisecond {
		t.Fatalf("连接过早被关闭: %v", d)
	}
}

func TestServeConnPreAuthFrameLimit(t *testing.T) {
	addr, _ := preAuthServer(t, &PreAuthLimits{IdleTimeout: 5 * time.Second, MaxFrame: 128 << 10})
	// 第一个帧（协商前）声明 1 MiB：读完帧头即断开，不等待也不分配帧内容
	for _, negotiated := range []bool{false, true} {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		if negotiated {
			writeFrame(t, c, buildClientNegotiate311())
		}
		if _, err := c.Write([]byte{0, 0x10, 0, 0}); err != nil {
			t.Fatal(err)
		}
		if d := waitClosed(t, c, 2*time.Second); d > time.Second {
			t.Fatalf("超大帧应立即断开，耗时 %v", d)
		}
		c.Close()
	}
}

func TestServeConnPreAuthSlotsLimit(t *testing.T) {
	slots := &PreAuthSlots{MaxTotal: 10, MaxPerIP: 2}
	addr, ended := preAuthServer(t, &PreAuthLimits{IdleTimeout: 10 * time.Second, Slots: slots})
	var held []net.Conn
	defer func() {
		for _, c := range held {
			c.Close()
		}
	}()
	for i := 0; i < 2; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	deadline := time.Now().Add(2 * time.Second)
	for slots.InUse() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := slots.InUse(); n != 2 {
		t.Fatalf("两条未登录连接应占用两个名额，实际 %d", n)
	}
	// 第三条立即被拒绝
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if d := waitClosed(t, c, 2*time.Second); d > time.Second {
		t.Fatalf("超过上限的连接应立即关闭，耗时 %v", d)
	}
	// 关闭一条后名额归还，新连接可以登录；登录后名额释放，不占用认证前的上限
	held[0].Close()
	for slots.InUse() != 1 && time.Now().Before(deadline.Add(time.Second)) {
		time.Sleep(10 * time.Millisecond)
	}
	nc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	_ = nc.SetDeadline(time.Now().Add(10 * time.Second))
	d := &smb2c.Dialer{Initiator: &smb2c.NTLMInitiator{User: "alice", Password: "pw"}}
	sess, err := d.DialConn(context.Background(), nc, addr)
	if err != nil {
		t.Fatalf("名额归还后应能登录: %v", err)
	}
	defer sess.Logoff()
	if n := slots.InUse(); n != 1 {
		t.Fatalf("登录后应只剩一条未登录连接占用名额，实际 %d", n)
	}
	_ = ended
}

// TestServeConnPreAuthLimitsLiftAfterLogin：登录后恢复原有的空闲超时与帧大小，空闲片刻后照常读写大文件。
func TestServeConnPreAuthLimitsLiftAfterLogin(t *testing.T) {
	addr, _ := preAuthServer(t, &PreAuthLimits{IdleTimeout: 300 * time.Millisecond, Deadline: 600 * time.Millisecond, MaxFrame: 128 << 10,
		Slots: &PreAuthSlots{MaxTotal: 4, MaxPerIP: 4}})
	nc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	_ = nc.SetDeadline(time.Now().Add(20 * time.Second))
	d := &smb2c.Dialer{Initiator: &smb2c.NTLMInitiator{User: "alice", Password: "pw"}}
	sess, err := d.DialConn(context.Background(), nc, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Logoff()
	// 超过认证前的总时限再操作：登录后不再受它限制
	time.Sleep(900 * time.Millisecond)
	fs, err := sess.Mount("share")
	if err != nil {
		t.Fatalf("登录后空闲片刻应仍可使用: %v", err)
	}
	defer fs.Umount()
	data := bytes.Repeat([]byte("0123456789abcdef"), 256<<10) // 4 MiB，单个写请求远超认证前的帧上限
	if err := fs.WriteFile("big.bin", data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile("big.bin")
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("读回内容不一致: %v", err)
	}
}
