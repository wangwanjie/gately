package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/module"
)

func TestPortCheck(t *testing.T) {
	// 端口关闭后可能被并行运行的其他测试进程立即占用，换端口重试几次，避免偶发失败
	for attempt := 0; attempt < 5; attempt++ {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		r := portCheck(port)
		if r.Status != module.CheckInfo || r.Params["state"] != "inuse" || r.ID != "port."+strconv.Itoa(port) {
			ln.Close()
			t.Fatalf("占用的端口应为 info/inuse: %+v", r)
		}
		ln.Close()
		r = portCheck(port)
		if r.Status == module.CheckOK {
			return
		}
		t.Logf("端口 %d 关闭后仍被占用（可能被其他进程抢占），重试: %+v", port, r)
	}
	t.Fatal("空闲端口应为 ok：连续 5 次关闭后仍显示占用")
}

func TestNatChecks(t *testing.T) {
	old := listIfaceAddrs
	defer func() { listIfaceAddrs = old }()
	mk := func(name string, ips ...string) ifaceAddr {
		ia := ifaceAddr{name: name}
		for _, s := range ips {
			ia.addrs = append(ia.addrs, netip.MustParseAddr(s))
		}
		return ia
	}
	ids := func(rs []module.CheckResult) string {
		var out []string
		for _, r := range rs {
			out = append(out, r.ID)
		}
		return strings.Join(out, ",")
	}
	// 真实场景：内网 IPv4 + 直连公网 IPv6，另有 Tailscale 网卡（100.64.x 不应判为运营商 NAT）
	listIfaceAddrs = func() []ifaceAddr {
		return []ifaceAddr{
			mk("eth0", "192.168.1.8", "2001:db8:ac7:1::f93"),
			mk("tailscale0", "100.64.0.4", "fd7a:115c:a1e0::4"),
			mk("veth-odd", "100.64.0.9", "fd7a:115c:a1e0::9"),
		}
	}
	if got := ids(natChecks("198.51.100.3", "2001:db8:ac7:1::f93")); got != "net.direct6,net.behindNat" {
		t.Fatalf("Tailscale 网卡不应触发 CGNAT，且应识别直连 IPv6: %s", got)
	}
	// 真正的运营商 NAT：普通网卡拿到 100.64.x
	listIfaceAddrs = func() []ifaceAddr { return []ifaceAddr{mk("eth0", "100.72.1.2")} }
	if got := ids(natChecks("203.0.113.7", "")); got != "net.behindNat,net.cgnat" {
		t.Fatalf("应判为运营商 NAT: %s", got)
	}
	// 直接持有公网 IPv4
	listIfaceAddrs = func() []ifaceAddr { return []ifaceAddr{mk("ppp0", "203.0.113.7")} }
	if got := ids(natChecks("203.0.113.7", "")); got != "net.direct" {
		t.Fatalf("应判为直连公网: %s", got)
	}
	if got := natChecks("", ""); len(got) != 0 {
		t.Fatalf("没有公网地址时不应给出结论: %+v", got)
	}
}

func TestClockSkewFromDateHeader(t *testing.T) {
	// 返回 5 分钟前的 Date 头，模拟本机时钟偏快
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().Add(-5*time.Minute).UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	old := ipsource.DefaultIPv4URLs
	ipsource.DefaultIPv4URLs = []string{"http://127.0.0.1:1/unreachable", srv.URL}
	defer func() { ipsource.DefaultIPv4URLs = old }()
	r := clockCheck(context.Background())
	if sec, _ := strconv.Atoi(r.Params["seconds"]); r.Status != module.CheckWarn || sec < 290 || sec > 310 {
		t.Fatalf("应检测到约 300 秒偏差（即使状态码非 2xx，且前一个地址不可达）: %+v", r)
	}
}
