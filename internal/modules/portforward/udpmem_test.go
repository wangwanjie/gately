package portforward

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

// udpSink 是只收不回的 UDP 目标，模拟伪造来源扫描时目标不回包的情况。
func udpSink(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

// heapInuse 返回两次 GC 之后的堆占用。
func heapInuse() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}

// 1024 个不同源端口各发一个包、目标不回包：每个会话在等待回包期间不应常驻一块 64 KB 读缓冲
// （改动前 1024 个会话约多占 64 MB 堆），缓冲只在目标有回包可读时才从池中借出。
func TestUDPIdleSessionsHoldNoBuffer(t *testing.T) {
	const clients = 1024
	sink := udpSink(t)
	listenPort := freePort(t, "udp")
	rule := Rule{
		Protocols: []string{"udp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(sink), UDPSessionTimeout: 60,
	}
	r := testRunner(rule)
	base, baseG := heapInuse(), runtime.NumGoroutine()
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	conns := make([]net.Conn, 0, clients)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for range clients {
		c, err := net.Dial("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		if _, err := c.Write([]byte("0123456789abcdef0123456789abcdef")); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for r.stats.ActiveConns.Load() < clients && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := r.stats.ActiveConns.Load(); n != clients {
		t.Fatalf("应建立 %d 个会话，实际 %d", clients, n)
	}
	grown := int64(heapInuse()) - int64(base)
	t.Logf("%d 个空闲会话：堆增长 %.1f MB，协程 %d -> %d", clients, float64(grown)/(1<<20), baseG, runtime.NumGoroutine())
	if grown > 16<<20 {
		t.Fatalf("%d 个等待回包的会话使堆增长 %.1f MB，不应为每个会话常驻读缓冲", clients, float64(grown)/(1<<20))
	}
}

// 多个客户端并发收发：每个客户端的回包只回到自己，超时后会话全部回收。配合 -race 运行。
func TestUDPConcurrentSessions(t *testing.T) {
	echoPort, closeEcho := udpEcho(t)
	defer closeEcho()
	listenPort := freePort(t, "udp")
	rule := Rule{
		Protocols: []string{"udp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(echoPort), UDPSessionTimeout: 1,
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	const clients, rounds = 32, 20
	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.Dial("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
			if err != nil {
				errs <- err
				return
			}
			defer c.Close()
			buf := make([]byte, 128)
			for j := range rounds {
				msg := fmt.Appendf(nil, "client-%d-round-%d", i, j)
				if _, err := c.Write(msg); err != nil {
					errs <- err
					return
				}
				_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
				n, err := c.Read(buf)
				if err != nil {
					errs <- fmt.Errorf("客户端 %d 第 %d 轮: %w", i, j, err)
					return
				}
				if !bytes.Equal(buf[:n], msg) {
					errs <- fmt.Errorf("客户端 %d 收到别人的回包: %q", i, buf[:n])
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if n := r.stats.ActiveConns.Load(); n != clients {
		t.Fatalf("应有 %d 个会话，实际 %d", clients, n)
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.stats.ActiveConns.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := r.stats.ActiveConns.Load(); n != 0 {
		t.Fatalf("会话未按空闲超时回收: %d", n)
	}
}
