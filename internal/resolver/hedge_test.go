package resolver

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// dohServer 返回一个计数的 DoH 测试服务器：fail 为真时返回 500，否则返回 A 记录。
func dohServer(t *testing.T, fail bool, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		q, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(buildAnswer(q, net.IPv4(203, 0, 113, 9)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// useInsecureBootstrap 让 DoH 查询接受多个测试服务器各自的自签证书。
func useInsecureBootstrap(t *testing.T, srv *httptest.Server) {
	t.Helper()
	old := bootstrap
	tr := srv.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig.InsecureSkipVerify = true
	bootstrap = &http.Client{Transport: tr}
	t.Cleanup(func() { bootstrap = old })
}

func setHedgeDelay(t *testing.T, d time.Duration) {
	t.Helper()
	old := hedgeDelay
	hedgeDelay = d
	t.Cleanup(func() { hedgeDelay = old })
}

// testQuery 构造查询 example.test A 记录的报文。
func testQuery() []byte {
	q := []byte{0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	q = append(q, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 4, 't', 'e', 's', 't', 0, 0, 1, 0, 1)
	return q
}

// 服务器明确失败时立即换下一个，不等 hedgeDelay；成功的服务器成为下次查询的首选。
func TestFailedServerSwitchesImmediately(t *testing.T) {
	var badHits, goodHits atomic.Int32
	bad := dohServer(t, true, &badHits)
	good := dohServer(t, false, &goodHits)
	useInsecureBootstrap(t, good)
	setHedgeDelay(t, 10*time.Second)

	e := &exchanger{servers: []Server{{URL: bad.URL}, {URL: good.URL}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	begin := time.Now()
	if _, err := e.exchange(ctx, testQuery()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(begin); d > 3*time.Second {
		t.Fatalf("失败后应立即换服务器，实际等了 %v", d)
	}
	if e.preferred != 1 || badHits.Load() != 1 || goodHits.Load() != 1 {
		t.Fatalf("首选应变为第二个服务器: preferred=%d bad=%d good=%d", e.preferred, badHits.Load(), goodHits.Load())
	}
	// 下一次查询直接从首选开始，不再先问失败的服务器
	if _, err := e.exchange(ctx, testQuery()); err != nil {
		t.Fatal(err)
	}
	if badHits.Load() != 1 || goodHits.Load() != 2 {
		t.Fatalf("应直接查询首选服务器: bad=%d good=%d", badHits.Load(), goodHits.Load())
	}
}

// 所有服务器都失败时返回最后的错误，不会一直等待。
func TestAllServersFail(t *testing.T) {
	var hits atomic.Int32
	a := dohServer(t, true, &hits)
	b := dohServer(t, true, &hits)
	useInsecureBootstrap(t, a)
	setHedgeDelay(t, 10*time.Second)
	e := &exchanger{servers: []Server{{URL: a.URL}, {URL: b.URL}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := e.exchange(ctx, testQuery())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("应返回服务器的错误: %v", err)
	}
	if hits.Load() != 2 || ctx.Err() != nil {
		t.Fatalf("两个服务器各应查询一次且不耗尽期限: hits=%d ctx=%v", hits.Load(), ctx.Err())
	}
}

// 唯一的服务器卡住时，期限到达后返回错误。
func TestHungOnlyServerHonoursDeadline(t *testing.T) {
	release := make(chan struct{})
	hung := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer hung.Close()
	defer close(release)
	useInsecureBootstrap(t, hung)
	setHedgeDelay(t, 50*time.Millisecond)
	e := &exchanger{servers: []Server{{URL: hung.URL}}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	begin := time.Now()
	if _, err := e.exchange(ctx, testQuery()); err == nil {
		t.Fatal("期限到达后应返回错误")
	}
	if d := time.Since(begin); d > 3*time.Second {
		t.Fatalf("应在期限附近返回，实际 %v", d)
	}
}

// UDP 响应被截断（TC 位）时改用 TCP 向同一地址重新查询。
func TestTruncatedUDPFallsBackToTCP(t *testing.T) {
	var tl net.Listener
	var uc net.PacketConn
	for i := 0; ; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		u, err := net.ListenPacket("udp", l.Addr().String())
		if err == nil {
			tl, uc = l, u
			break
		}
		l.Close()
		if i > 20 {
			t.Skip("找不到 TCP 与 UDP 同时空闲的端口")
		}
	}
	defer tl.Close()
	defer uc.Close()
	go func() {
		buf := make([]byte, 512)
		n, from, err := uc.ReadFrom(buf)
		if err != nil {
			return
		}
		resp := append([]byte(nil), buf[:n]...)
		// 置 QR 与 TC 位
		resp[2] |= 0x82
		_, _ = uc.WriteTo(resp, from)
	}()
	go func() {
		c, err := tl.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var l [2]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return
		}
		q := make([]byte, binary.BigEndian.Uint16(l[:]))
		if _, err := io.ReadFull(c, q); err != nil {
			return
		}
		ans := buildAnswer(q, net.IPv4(203, 0, 113, 53))
		_, _ = c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(ans))), ans...))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := queryUDP(ctx, tl.Addr().String(), testQuery())
	if err != nil {
		t.Fatal(err)
	}
	if resp[2]&0x02 != 0 || !net.IP(resp[len(resp)-4:]).Equal(net.IPv4(203, 0, 113, 53)) {
		t.Fatalf("应得到 TCP 的完整响应: %x", resp)
	}
}
