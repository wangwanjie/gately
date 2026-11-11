package protoroute

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/knockgate"
)

// fakeKnock 代替端口敲门模块：收到 "OPEN" 开头的报文就为来源登记 rule 的授权。
type fakeKnock struct {
	mu   sync.Mutex
	got  [][]byte
	via  []string
	rule string
}

func installFakeKnock(t *testing.T, rule string) *fakeKnock {
	t.Helper()
	f := &fakeKnock{rule: rule}
	knockgate.Default.Clear()
	knockgate.Default.SetRules([]knockgate.Rule{{Key: rule, Name: "SSH", Enable: true}})
	knockgate.Default.SetSubmitter(func(p []byte, src netip.Addr, via string) error {
		f.mu.Lock()
		f.got = append(f.got, bytes.Clone(p))
		f.via = append(f.via, via)
		f.mu.Unlock()
		if bytes.HasPrefix(p, []byte("OPEN")) {
			return knockgate.Default.Allow(src, f.rule, time.Now().Add(time.Minute))
		}
		return nil
	})
	t.Cleanup(func() {
		knockgate.Default.SetSubmitter(nil)
		knockgate.Default.SetRules(nil)
		knockgate.Default.Clear()
	})
	return f
}

func (f *fakeKnock) packets() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte{}, f.got...)
}

// spaPacket 生成形如 fwknop 报文的 base64 字符串（OPEN 开头表示认证通过）。
func spaPacket(open bool) []byte {
	p := strings.Repeat("A1b2C3d4+/", 22)
	if open {
		p = "OPEN" + p
	}
	return []byte(p)
}

// sendSPA 像 fwknop -P tcp 一样连接、发送报文后关闭。
func sendSPA(t *testing.T, port int, pkt []byte) {
	t.Helper()
	c, err := net.Dial("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(pkt); err != nil {
		t.Fatal(err)
	}
	_ = c.(*net.TCPConn).CloseWrite()
	// 服务端不回复任何数据，读到 EOF 表示已处理完并关闭
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _ := io.Copy(io.Discard, c)
	c.Close()
	if n != 0 {
		t.Fatalf("敲门路由不应回复数据，收到 %d 字节", n)
	}
}

// waitAllowed 等待授权登记（报文在连接处理协程中提交）。
func waitAllowed(t *testing.T, rule string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if knockgate.Default.Allowed(netip.MustParseAddr("127.0.0.1"), rule) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("敲门后应登记授权")
}

// sshReaches 用 SSH 版本标识连接分流端口，返回后端回显的标签（tagEcho 先回名称），被断开时返回空串。
func sshReaches(t *testing.T, port int) string {
	t.Helper()
	c, err := net.Dial("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return ""
	}
	defer c.Close()
	_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9.9\r\n"))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, _ := c.Read(buf)
	name, _, _ := strings.Cut(string(buf[:n]), "\n")
	return name
}

// 一个端口同时承载：HTTP 对所有人开放，SSH 路由要求敲门，敲门报文经同一端口的 TCP 连接发来（fwknop -P tcp）。
func TestKnockRouteAndSPA(t *testing.T) {
	fk := installFakeKnock(t, "k1")
	web := tagEcho(t, "web")
	sshB := tagEcho(t, "ssh")
	port := freePort(t)
	rule := Rule{
		Name: "mux", Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(port),
		Routes: []Route{
			{Name: "web", Protocol: ProtoHTTP, TargetAddresses: []string{"127.0.0.1"}, TargetPort: web},
			{Name: "ssh", Protocol: ProtoSSH, TargetAddresses: []string{"127.0.0.1"}, TargetPort: sshB, Knock: "k1"},
			{Name: "knock", Protocol: ProtoSPA},
		},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := rule.checkKnocks(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)

	if got := httpReaches(t, port); got != "web" {
		t.Fatalf("HTTP 不要求敲门，应能访问: %q", got)
	}
	if got := sshReaches(t, port); got != "" {
		t.Fatalf("未敲门时 SSH 路由应断开: %q", got)
	}
	if r.snapshot().KnockDenied != 1 {
		t.Fatalf("应计入 1 次未敲门拒绝: %+v", r.snapshot())
	}
	// 认证失败的报文不产生授权
	sendSPA(t, port, spaPacket(false))
	if got := sshReaches(t, port); got != "" {
		t.Fatal("认证失败后仍应断开")
	}
	sendSPA(t, port, spaPacket(true))
	waitAllowed(t, "k1")
	if got := sshReaches(t, port); got != "ssh" {
		t.Fatalf("敲门后应能连接 SSH: %q", got)
	}
	pk := fk.packets()
	if len(pk) != 2 || !bytes.Equal(pk[1], spaPacket(true)) {
		t.Fatalf("应原样提交完整报文: %d", len(pk))
	}
	if !strings.HasPrefix(fk.via[0], "TCP ") {
		t.Fatalf("应注明报文通道: %q", fk.via[0])
	}
	s := r.snapshot()
	if s.RouteConns[2] != 2 || s.RouteConns[1] != 1 {
		t.Fatalf("路由统计不对: %v", s.RouteConns)
	}
}

// 整条规则要求敲门（访问控制中的敲门）时，未敲门的来源只能使用敲门路由，敲门后才能使用其他路由与默认目标。
func TestRuleKnockWithSPARoute(t *testing.T) {
	installFakeKnock(t, "k1")
	web := tagEcho(t, "web")
	port := freePort(t)
	rule := Rule{
		Name: "gated", Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(port),
		Routes:          []Route{{Name: "knock", Protocol: ProtoSPA}},
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(web),
		Access: iplist.Access{Knock: "k1"},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)
	if got := httpReaches(t, port); got != "" {
		t.Fatalf("未敲门应断开: %q", got)
	}
	sendSPA(t, port, spaPacket(true))
	waitAllowed(t, "k1")
	if got := httpReaches(t, port); got != "web" {
		t.Fatalf("敲门后应转发到默认目标: %q", got)
	}
}

// 规则要求敲门而没有敲门路由时，在识别协议之前直接断开。
func TestRuleKnockWithoutSPARoute(t *testing.T) {
	installFakeKnock(t, "k1")
	r, _, port := startAccessRule(t, iplist.Access{Knock: "k1"}, "127.0.0.1", []string{"tcp4"})
	if got := httpReaches(t, port); got != "" {
		t.Fatalf("未敲门应断开: %q", got)
	}
	if s := r.snapshot(); s.KnockDenied != 1 || s.Denied != 0 {
		t.Fatalf("应计入未敲门拒绝而不是名单拒绝: %+v", s)
	}
	_ = knockgate.Default.Allow(netip.MustParseAddr("127.0.0.1"), "k1", time.Now().Add(time.Minute))
	if got := httpReaches(t, port); got != "web" {
		t.Fatalf("持有授权后应能访问: %q", got)
	}
}

// 没有敲门路由时，看起来像敲门报文的数据仍按无法识别交给默认目标，不被延迟或吞掉。
func TestSPAProbeOnlyWithRoute(t *testing.T) {
	if p, done := detect(spaPacket(true), false); !done || p != "" {
		t.Fatalf("没有敲门路由时不识别敲门报文: %q %v", p, done)
	}
	if p, done := detect(spaPacket(true), true); !done || p != ProtoSPA {
		t.Fatalf("应识别为敲门报文: %q %v", p, done)
	}
	// 没有敲门路由时与原有识别完全一致
	for _, s := range []string{"abc", "Zz9", "0", strings.Repeat("Q", 70)} {
		p1, d1 := detect([]byte(s), false)
		p2, d2 := detectProtocol([]byte(s))
		if p1 != p2 || d1 != d2 {
			t.Fatalf("%q: 没有敲门路由时识别结果应不变", s)
		}
	}
	if _, done := detect([]byte("abc"), true); done {
		t.Fatal("有敲门路由时短的 base64 字符应继续等待")
	}
	for _, s := range []string{"SSH-2.0-x\r\n", "GET / HTTP/1.1\r\n", strings.Repeat("A", 30) + "-" + strings.Repeat("A", 40), "\x16\x03\x01"} {
		if p, _ := detect([]byte(s), true); p == ProtoSPA {
			t.Fatalf("%q 不应识别为敲门报文", s)
		}
	}
}

// 有敲门路由时识别结果仍满足单调性：前缀判定后，追加数据不改变结果。
func FuzzDetectWithSPA(f *testing.F) {
	f.Add(spaPacket(true))
	f.Add([]byte("SSH-2.0-OpenSSH\r\n"))
	f.Add([]byte("GET / HTTP/1.1\r\nHost: a\r\n\r\n"))
	f.Add([]byte("0032git-upload-pack /x\x00host=a\x00"))
	f.Fuzz(func(t *testing.T, b []byte) {
		proto, done := detect(b, true)
		for n := 1; n < len(b); n++ {
			if p, d := detect(b[:n], true); d && (!done || p != proto) {
				t.Fatalf("前缀 %q 判定为 %q，完整数据为 %q（done=%v）", b[:n], p, proto, done)
			}
		}
	})
}

// 敲门路由与路由敲门的校验。
func TestKnockRouteValidate(t *testing.T) {
	base := func(routes ...Route) Rule {
		return Rule{Protocols: []string{"tcp"}, ListenPorts: "443", Routes: routes}
	}
	ok := base(Route{Name: "k", Protocol: ProtoSPA, TargetAddresses: []string{" "}})
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Rule{
		"不能填写目标":    base(Route{Name: "k", Protocol: ProtoSPA, TargetAddresses: []string{"127.0.0.1"}}),
		"本身不能要求敲门":  base(Route{Name: "k", Protocol: ProtoSPA, Knock: "k1"}),
		"每种协议只能有一条": base(Route{Name: "a", Protocol: ProtoSPA}, Route{Name: "b", Protocol: ProtoSPA}),
	}
	for want, rule := range bad {
		if err := rule.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("期望包含 %q 的错误，得到 %v", want, err)
		}
	}
	knockgate.Default.SetRules([]knockgate.Rule{{Key: "k1", Name: "SSH"}})
	t.Cleanup(func() { knockgate.Default.SetRules(nil) })
	r := base(Route{Name: "s", Protocol: ProtoSSH, TargetAddresses: []string{"127.0.0.1"}, Knock: "nope"})
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := r.checkKnocks(); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("引用不存在的敲门规则应报错: %v", err)
	}
	r.Routes[0].Knock = "k1"
	r.Access.Knock = "nope"
	if err := r.checkKnocks(); err == nil {
		t.Fatal("规则访问控制引用不存在的敲门规则应报错")
	}
	r.Access.Knock = "k1"
	if err := r.checkKnocks(); err != nil {
		t.Fatal(err)
	}
}
