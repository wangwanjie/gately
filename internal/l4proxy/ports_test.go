package l4proxy

import (
	"slices"
	"testing"
)

func TestParsePorts(t *testing.T) {
	cases := []struct {
		spec string
		want []int
		ok   bool
	}{
		{"80", []int{80}, true},
		{"80,443", []int{80, 443}, true},
		{"8000-8002, 9000", []int{8000, 8001, 8002, 9000}, true},
		{"0", nil, false},
		{"65536", nil, false},
		{"10-5", nil, false},
		{"", nil, false},
		{"1-2000", nil, false},
	}
	for _, c := range cases {
		got, err := ParsePorts(c.spec)
		if (err == nil) != c.ok {
			t.Fatalf("%q: err=%v, want ok=%v", c.spec, err, c.ok)
		}
		if c.ok && !slices.Equal(got, c.want) {
			t.Fatalf("%q: got %v want %v", c.spec, got, c.want)
		}
	}
}

func TestMapPorts(t *testing.T) {
	pairs, err := MapPorts("8000-8002", "9000-9002")
	if err != nil {
		t.Fatal(err)
	}
	if pairs[2] != (PortPair{8002, 9002}) {
		t.Fatalf("一一映射错误: %v", pairs)
	}
	pairs, err = MapPorts("80,443", "8080")
	if err != nil || pairs[1] != (PortPair{443, 8080}) {
		t.Fatalf("单目标端口映射错误: %v %v", pairs, err)
	}
	pairs, err = MapPorts("22", "")
	if err != nil || pairs[0] != (PortPair{22, 22}) {
		t.Fatalf("空目标端口应与监听端口相同: %v %v", pairs, err)
	}
	if _, err := MapPorts("80,81,82", "90,91"); err == nil {
		t.Fatal("数量不匹配应报错")
	}
}
