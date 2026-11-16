package l4proxy

import (
	"strings"
	"testing"
)

// 目标地址写成「主机:端口」时拆出端口，IPv6 地址不误拆。
func TestSplitTargets(t *testing.T) {
	cases := []struct {
		in    []string
		hosts string
		port  int
	}{
		{[]string{" 10.0.0.1 ", "", "nas.lan"}, "10.0.0.1,nas.lan", 0},
		{[]string{"127.0.0.1:19210"}, "127.0.0.1", 19210},
		{[]string{"nas.lan:22", "10.0.0.2:22"}, "nas.lan,10.0.0.2", 22},
		{[]string{"[::1]:8443"}, "::1", 8443},
		{[]string{"[::1]", "fe80::1%eth0", "2001:db8::1"}, "::1,fe80::1%eth0,2001:db8::1", 0},
		{[]string{"10.0.0.1:80", "10.0.0.2"}, "10.0.0.1,10.0.0.2", 80},
	}
	for _, c := range cases {
		hosts, port, err := SplitTargets(c.in)
		if err != nil {
			t.Fatalf("%v: %v", c.in, err)
		}
		if strings.Join(hosts, ",") != c.hosts || port != c.port {
			t.Fatalf("%v = %v, %d，期望 %s, %d", c.in, hosts, port, c.hosts, c.port)
		}
	}
}

func TestSplitTargetsRejects(t *testing.T) {
	for _, in := range [][]string{
		{"127.0.0.1:19210:19221"},
		{"http://10.0.0.1:8080"},
		{"10.0.0.1/24"},
		{"10.0.0.1:0"},
		{"10.0.0.1:http"},
		{"10.0.0.1:70000"},
		{":8080"},
		{"[::1]:x"},
		{"10.0.0.1:80", "10.0.0.2:81"},
	} {
		if _, _, err := SplitTargets(in); err == nil {
			t.Fatalf("%v 应被拒绝", in)
		}
	}
}

func TestMergeTargetPorts(t *testing.T) {
	cases := []struct {
		spec string
		port int
		want string
		ok   bool
	}{
		{"", 0, "", true},
		{"8080-8090", 0, "8080-8090", true},
		{"", 19210, "19210", true},
		{" 19210 ", 19210, "19210", true},
		{"19221", 19210, "", false},
		{"19210-19211", 19210, "", false},
	}
	for _, c := range cases {
		got, err := MergeTargetPorts(c.spec, c.port)
		if (err == nil) != c.ok || got != c.want {
			t.Fatalf("MergeTargetPorts(%q, %d) = %q, %v", c.spec, c.port, got, err)
		}
	}
	if p, err := MergeTargetPort(0, 22); err != nil || p != 22 {
		t.Fatalf("空端口应填入: %d %v", p, err)
	}
	if p, err := MergeTargetPort(22, 22); err != nil || p != 22 {
		t.Fatalf("相同端口应保留: %d %v", p, err)
	}
	if _, err := MergeTargetPort(2222, 22); err == nil {
		t.Fatal("不同端口应冲突")
	}
}

// 旧版本保存的「主机:端口」目标应被识别为问题，正常写法没有问题。
func TestTargetProblems(t *testing.T) {
	if p := TargetProblems([]string{"10.0.0.1", "::1", "nas.lan"}); len(p) != 0 {
		t.Fatalf("正常目标不应有问题: %v", p)
	}
	p := TargetProblems([]string{"10.0.0.1", "127.0.0.1:19210", "a:b:c"})
	if len(p) != 2 || !strings.Contains(p[0], "127.0.0.1:19210") {
		t.Fatalf("TargetProblems = %v", p)
	}
}
