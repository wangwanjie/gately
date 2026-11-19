package hops

import (
	"net"
	"strings"
	"testing"
)

// 连到本机的转发按拨出连接的本地地址登记，下一跳按对端地址查到；结束后撤销
func TestTrackLookup(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 2)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()
	in, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	inServer := <-accepted
	out, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	outServer := <-accepted

	done := Track(inServer, out, Hop{Module: "portforward", Rule: "r1"})
	h, ok := Lookup(outServer.RemoteAddr().String())
	if !ok || h.Rule != "r1" || h.Peer != Normalize(in.LocalAddr().String()) || h.Listen != Normalize(inServer.LocalAddr().String()) {
		t.Fatalf("应按下一跳看到的对端查到登记: %+v %v", h, ok)
	}
	done()
	if _, ok := Lookup(outServer.RemoteAddr().String()); ok {
		t.Fatal("撤销后不应再查到")
	}
}

// 签名的请求头只能由本进程生成，篡改或伪造都校验失败
func TestEncodeDecode(t *testing.T) {
	v := Encode(Hop{Module: "webservice", Rule: "l1", Site: "s1", Peer: "1.2.3.4:5"})
	h, ok := Decode(v)
	if !ok || h.Site != "s1" || h.Peer != "1.2.3.4:5" {
		t.Fatalf("应解出原内容: %+v %v", h, ok)
	}
	payload, mac, _ := strings.Cut(v, ".")
	if _, ok := Decode(payload + "x." + mac); ok {
		t.Fatal("篡改的内容不应通过校验")
	}
	if _, ok := Decode("e30.forged"); ok {
		t.Fatal("伪造的签名不应通过校验")
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("[::ffff:127.0.0.1]:80"); got != "127.0.0.1:80" {
		t.Fatalf("IPv4 映射地址应还原: %s", got)
	}
}

// Chain 由内向外列出本跳与 Prev，返回的本跳不再带 Prev；编码后 Prev 原样保留
func TestChain(t *testing.T) {
	h := Hop{Module: "webservice", Rule: "in", Prev: []Hop{{Module: "webservice", Rule: "out"}}}
	d, ok := Decode(Encode(h))
	c := d.Chain()
	if !ok || len(c) != 2 || c[0].Rule != "in" || c[0].Prev != nil || c[1].Rule != "out" {
		t.Fatalf("应由内向外列出: %+v %v", c, ok)
	}
}
