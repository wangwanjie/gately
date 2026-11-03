package ipsource

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"runtime"
	"testing"
)

func TestExtract(t *testing.T) {
	text := `当前 IP：8.8.8.8  来自于：美国 ; also 999.1.1.1 and 1.1.1.1 dup 8.8.8.8`
	got := Extract(text, IPv4)
	if len(got) != 2 || got[0].String() != "8.8.8.8" || got[1].String() != "1.1.1.1" {
		t.Fatalf("IPv4 提取错误: %v", got)
	}
	v6 := Extract(`{"ip":"2408:8207:1234::1","x":"::ffff:1.2.3.4"} fe80::1`, IPv6)
	if len(v6) != 2 || v6[0].String() != "2408:8207:1234::1" || v6[1].String() != "fe80::1" {
		t.Fatalf("IPv6 提取错误: %v", v6)
	}
}

func TestIsPublic(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":       true,
		"192.168.1.1":   false,
		"10.0.0.1":      false,
		"100.64.1.1":    false,
		"127.0.0.1":     false,
		"2408:8207::1":  true,
		"fd00::1":       false,
		"fe80::1":       false,
		"::1":           false,
		"169.254.0.1":   false,
		"240e:3b0::abc": true,
	}
	for s, want := range cases {
		if got := IsPublic(netip.MustParseAddr(s)); got != want {
			t.Errorf("IsPublic(%s)=%v want %v", s, got, want)
		}
	}
}

func TestPick(t *testing.T) {
	cands := []netip.Addr{netip.MustParseAddr("fd00::1"), netip.MustParseAddr("2408::1"), netip.MustParseAddr("2408::2")}
	a, err := pick(cands, IPv6, "")
	if err != nil || a.String() != "2408::1" {
		t.Fatalf("默认应取第一个公网地址: %v %v", a, err)
	}
	a, err = pick(cands, IPv6, `::2$`)
	if err != nil || a.String() != "2408::2" {
		t.Fatalf("正则筛选错误: %v %v", a, err)
	}
	// 正则允许刻意选择内网地址
	a, err = pick(cands, IPv6, `^fd`)
	if err != nil || a.String() != "fd00::1" {
		t.Fatalf("正则应允许内网地址: %v %v", a, err)
	}
	if _, err := pick([]netip.Addr{netip.MustParseAddr("192.168.1.2")}, IPv4, ""); err == nil {
		t.Fatal("没有公网地址时应报错")
	}
}

func TestFromURLsFallback(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("no ip here"))
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.9\n"))
	}))
	defer good.Close()
	// 测试服务器在回环地址上，替换为不限制地址族的客户端
	clientForTest = func(Family) *http.Client { return http.DefaultClient }
	defer func() { clientForTest = nil }()

	a, err := Get(context.Background(), Source{Enable: true, Type: TypeURL, URLs: []string{bad.URL, good.URL}}, IPv4)
	if err != nil {
		t.Fatal(err)
	}
	if a.String() != "203.0.113.9" {
		t.Fatalf("got %v", a)
	}
}

func TestFromInterface(t *testing.T) {
	interfaceAddrs = func(string) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)},
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("2408:8207::abcd"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	defer func() {
		interfaceAddrs = func(name string) ([]net.Addr, error) {
			iface, err := net.InterfaceByName(name)
			if err != nil {
				return nil, err
			}
			return iface.Addrs()
		}
	}()
	a, err := Get(context.Background(), Source{Enable: true, Type: TypeInterface, Interface: "eth0"}, IPv6)
	if err != nil || a.String() != "2408:8207::abcd" {
		t.Fatalf("网卡取 IPv6 错误: %v %v", a, err)
	}
	if _, err := Get(context.Background(), Source{Enable: true, Type: TypeInterface, Interface: "eth0"}, IPv4); err == nil {
		t.Fatal("网卡只有内网 IPv4 时应报错")
	}
}

func TestFromCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("使用 sh")
	}
	a, err := Get(context.Background(), Source{Enable: true, Type: TypeCommand, Command: "echo inet 198.51.100.7/24"}, IPv4)
	if err != nil || a.String() != "198.51.100.7" {
		t.Fatalf("命令取 IP 错误: %v %v", a, err)
	}
}

func TestValidate(t *testing.T) {
	s := Source{Enable: true, Type: TypeURL, URLs: []string{" ", ""}}
	if err := s.Validate(); err == nil {
		t.Fatal("空 URL 列表应报错")
	}
	s = Source{Enable: true, Type: "x"}
	if err := s.Validate(); err == nil {
		t.Fatal("未知类型应报错")
	}
	s = Source{Enable: true, Type: TypeURL, URLs: []string{"https://a"}, Regex: "("}
	if err := s.Validate(); err == nil {
		t.Fatal("非法正则应报错")
	}
}

func TestReplaceRetiredURLs(t *testing.T) {
	got, changed := ReplaceRetiredURLs([]string{"https://4.ipw.cn/", "https://api.ipify.org", "http://4.ipw.cn"})
	if !changed || len(got) != 2 || got[0] != "https://ipv4.ddnspod.com" || got[1] != "https://api.ipify.org" {
		t.Fatalf("替换结果错误: %v %v", got, changed)
	}
	// 替代地址已存在时只去掉旧地址
	got, changed = ReplaceRetiredURLs([]string{"https://ipv6.ddnspod.com", "https://6.ipw.cn"})
	if !changed || len(got) != 1 || got[0] != "https://ipv6.ddnspod.com" {
		t.Fatalf("去重结果错误: %v %v", got, changed)
	}
	// 没有停用地址时保持原样，包括用户自己写的重复地址
	in := []string{"https://a.example/ip", "https://a.example/ip"}
	got, changed = ReplaceRetiredURLs(in)
	if changed || len(got) != 2 {
		t.Fatalf("不应改动: %v %v", got, changed)
	}
}
