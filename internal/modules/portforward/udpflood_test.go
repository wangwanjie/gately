package portforward

import (
	"context"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// 会话数达到上限后，伪造来源的洪水里每个数据包都被丢弃：警告应按 10 秒节流，不能每个数据包写一条把日志刷满。
func TestUDPSessionLimitWarnThrottled(t *testing.T) {
	sink := udpSink(t)
	listenPort := freePort(t, "udp")
	rule := Rule{
		Protocols: []string{"udp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(sink), UDPSessionTimeout: 60,
	}
	s := defaultSettings()
	s.DialTimeout = 1
	s.MaxUDPSessions = 2
	buf := logbuf.New(1000, log.New(io.Discard, "", 0))
	r := newRunner(rule, s, buf.Logger("test"))
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	const clients = 50
	for range clients {
		c, err := net.Dial("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		for range 4 {
			_, _ = c.Write([]byte("x"))
		}
		// 不让瞬间涌入的数据报超出监听套接字的接收缓冲
		time.Sleep(time.Millisecond)
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.stats.ActiveConns.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	warns := 0
	for _, e := range buf.Since(0) {
		if strings.Contains(e.Message, "UDP 会话数达到上限") {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("%d 个来源被丢弃的数据包应只记录 1 条会话数上限警告，实际 %d 条", clients-2, warns)
	}
}
