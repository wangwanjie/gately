package dnsprovider

import (
	"testing"
)

func TestSplitFQDNPublicSuffix(t *testing.T) {
	cases := map[string][2]string{
		// 不在内置表里的 ICANN 多级后缀
		"www.example.com.ar": {"www", "example.com.ar"},
		"a.b.example.org.br": {"a.b", "example.org.br"},
		"example.com.ar":     {"@", "example.com.ar"},
		"nas.example.gov.br": {"nas", "example.gov.br"},
		"*.example.com.ar":   {"*", "example.com.ar"},
		// 私有后缀保持取最后两段，由各服务商自行处理
		"x.my.dynv6.net": {"x.my", "dynv6.net"},
		// 原有行为不变
		"www.example.com.cn": {"www", "example.com.cn"},
		"a.b.example.com":    {"a.b", "example.com"},
		"host.lan":           {"host", "host.lan"},
	}
	for in, want := range cases {
		s, z := SplitFQDN(in)
		if in == "host.lan" {
			// 未收录的 TLD：沿用最后两段
			want = [2]string{"@", "host.lan"}
		}
		if s != want[0] || z != want[1] {
			t.Errorf("SplitFQDN(%q)=%q,%q want %v", in, s, z, want)
		}
	}
}

// 开启代理的记录 TTL 只能是自动（1）；同名多条记录时优先认已是目标值的那条；未指定 TTL 的更新不带 ttl。
