package webservice

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/stunaddr"
)

func TestRedirectTargetVars(t *testing.T) {
	stunaddr.Set("rule-wz", "wz", netip.MustParseAddrPort("203.0.113.7:51234"))
	stunaddr.Set("rule-x", "443_穿透", netip.MustParseAddrPort("198.51.100.2:40001"))
	defer stunaddr.Delete("rule-wz")
	defer stunaddr.Delete("rule-x")

	cases := []struct {
		name    string
		domains []string
		to      string
		keep    bool
		url     string
		want    string
	}{
		// 群里的用法：通配符子域名 + STUN 端口，路径与参数由变量带上
		{"Lucky 通配符加 STUN", []string{"*.7772888.xyz"}, "https://{1}.v4.7772888.xyz:{STUN_wz_PORT}{path}{args}", true,
			"http://zly.7772888.xyz:2053/a/b?x=1", "https://zly.v4.7772888.xyz:51234/a/b?x=1"},
		// 目标里写了 {path}{args} 时不再按「保留路径」重复追加
		{"写了路径变量不重复追加", []string{"a.test"}, "https://{host}{path}{args}", true,
			"http://a.test/p?q=1", "https://a.test/p?q=1"},
		{"无查询串时 args 为空", []string{"a.test"}, "https://{host}{path}{args}", false,
			"http://a.test/p", "https://a.test/p"},
		{"保留路径照旧", []string{"a.test"}, "https://{host}", true,
			"http://a.test/p?q=1", "https://a.test/p?q=1"},
		{"端口与 hostAndPort", []string{"a.test"}, "https://{host}:{port}/{PATH}?{Args}&h={hostAndPort}", false,
			"http://a.test:8080/x/y?k=v", "https://a.test:8080/x/y?k=v&h=a.test:8080"},
		// IPv6 字面地址访问时 {host} 带方括号，{hostAndPort} 原样保留
		{"IPv6 字面地址", []string{"*"}, "https://{host}:9443{path}?h={hostAndPort}", false,
			"http://[2001:DB8::1]:8443/x", "https://[2001:db8::1]:9443/x?h=[2001:db8::1]:8443"},
		{"IPv6 字面地址保留路径", []string{"*"}, "https://{host}", true,
			"http://[2001:db8::1]/p?q=1", "https://[2001:db8::1]/p?q=1"},
		{"规则名含下划线与中文", []string{"a.test"}, "https://{STUN_443_穿透_IP}:{STUN_443_穿透_PORT}/{STUN_wz_ADDR}", false,
			"http://a.test/", "https://198.51.100.2:40001/203.0.113.7:51234"},
		{"域名与路径分段", []string{"*.b.test"}, "https://{domainPart2}.c.test/{pathPart2}/{domainPart9}", false,
			"http://x.y.b.test/p1/p2/p3", "https://y.c.test/p2/"},
		{"多级子域名", []string{"*.b.test"}, "https://{1}.c.test", false,
			"http://x.y.b.test/", "https://x.y.c.test"},
		{"首段内通配优先于后缀通配", []string{"*.b.test", "app-*-dev.b.test"}, "https://{1}.c.test", false,
			"http://app-one-dev.b.test/", "https://one.c.test"},
		{"精确域名优先时 {1} 为空", []string{"*.b.test", "a.b.test"}, "https://n{1}.c.test", false,
			"http://a.b.test/", "https://n.c.test"},
		{"* 匹配完整域名", []string{"*"}, "https://{1}.c.test", false,
			"http://a.test/", "https://a.test.c.test"},
		{"客户端 IP", []string{"a.test"}, "https://a.test/?ip={ClientIP}&r={RemoteIP}", false,
			"http://a.test/", "https://a.test/?ip=192.0.2.1&r=192.0.2.1"},
		{"未知变量原样保留", []string{"a.test"}, "https://a.test/{unknown}", false,
			"http://a.test/", "https://a.test/{unknown}"},
		// 「保留路径」按路径与查询串分别判断：只写了其中一类变量时，另一部分照常追加
		{"只写路径变量时保留查询串", []string{"a.test"}, "https://new.test{path}", true,
			"http://a.test/p?q=1", "https://new.test/p?q=1"},
		{"只写查询变量时保留路径", []string{"a.test"}, "https://new.test/base?{Args}&from=a", true,
			"http://a.test/p?q=1", "https://new.test/base/p?q=1&from=a"},
		{"目标自带查询串时只追加路径", []string{"a.test"}, "https://new.test/base/?from=a", true,
			"http://a.test/p?q=1", "https://new.test/base/p?from=a"},
		{"保留路径不吞掉主机", []string{"a.test"}, "https://new.test/", true,
			"http://a.test//evil.com/x", "https://new.test//evil.com/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Site{Domains: c.domains, RedirectTo: c.to, KeepPath: c.keep}
			r := httptest.NewRequest("GET", c.url, nil)
			got, err := redirectTarget(&s, r)
			if err != nil || got != c.want {
				t.Fatalf("got %q %v, want %q", got, err, c.want)
			}
		})
	}

	// {time} 为当前时间，空格写成 %20，生成的地址是合法 URL，查询参数解码后为原时间
	s := Site{Domains: []string{"a.test"}, RedirectTo: "https://a.test/?t={time}"}
	got, _ := redirectTarget(&s, httptest.NewRequest("GET", "http://a.test/", nil))
	if strings.Contains(got, " ") {
		t.Fatalf("{time} 不应含未转义的空格: %q", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("{time} 生成的地址无效: %q %v", got, err)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", u.Query().Get("t")); err != nil {
		t.Fatalf("{time} 格式错误: %q", got)
	}

	// 变量展开后主机为空时拒绝跳转：否则拼上 //evil.com 这样的访客路径会把访客带到别的站点
	for _, c := range []struct {
		domains []string
		to, url string
	}{
		{[]string{"a.test", "*.a.test"}, "https://{1}", "http://a.test//evil.com/x"},
		{[]string{"*.b.test"}, "https://{domainPart4}", "http://x.b.test//evil.com/x"},
		{[]string{"a.test"}, "https://{pathPart3}", "http://a.test//evil.com/x"},
	} {
		s := Site{Domains: c.domains, RedirectTo: c.to, KeepPath: true}
		if got, err := redirectTarget(&s, httptest.NewRequest("GET", c.url, nil)); err == nil {
			t.Fatalf("%s 展开后主机为空，应拒绝跳转，却得到 %q", c.to, got)
		}
	}

	// Host 中含 & = / 等字符时不代入跳转地址，避免改变目标主机或注入查询参数
	for _, host := range []string{"a&admin=1.example.com", "evil.com/x.example.com", "a?b.example.com", "a@b.example.com"} {
		for _, to := range []string{"https://t.test/?sub={1}", "https://{host}/", "https://{hostAndPort}/"} {
			r := httptest.NewRequest("GET", "http://x.example.com/", nil)
			r.Host = host
			s := Site{Domains: []string{"*.example.com"}, RedirectTo: to}
			if got, err := redirectTarget(&s, r); !errors.Is(err, errBadHost) {
				t.Fatalf("Host %q 代入 %s 应被拒绝，得到 %q %v", host, to, got, err)
			}
		}
	}

	r := httptest.NewRequest("GET", "http://x.example.com/", nil)
	r.Host = "a&b.example.com"
	s = Site{Domains: []string{"*.example.com"}, RedirectTo: "https://{domainPart1}.t.test/"}
	if got, err := redirectTarget(&s, r); !errors.Is(err, errBadHost) {
		t.Fatalf("含非法字符的域名分段应被拒绝，得到 %q %v", got, err)
	}

	// 引用的 STUN 规则没有地址时报错
	s = Site{Domains: []string{"a.test"}, RedirectTo: "https://a.test:{STUN_none_PORT}"}
	if _, err := redirectTarget(&s, httptest.NewRequest("GET", "http://a.test/", nil)); err == nil || !strings.Contains(err.Error(), "none") {
		t.Fatalf("规则不存在时应报错: %v", err)
	}
}

func TestRedirectValidateVars(t *testing.T) {
	for _, to := range []string{
		"https://{host}",
		"https://{host}{path}{args}",
		"https://{host}:{port}{path}{args}",
		"https://{1}.v4.7772888.xyz:{STUN_wz_PORT}{path}{args}",
		"https://{STUN_web_ADDR}/{PATH}",
		"https://{STUN_web_IP}:8443",
		"https://{domainPart1}.example.com/{pathPart1}",
	} {
		s := Site{Domains: []string{"*.example.com"}, Type: TypeRedirect, RedirectTo: to}
		if err := s.validate(false); err != nil {
			t.Errorf("%s 应能保存: %v", to, err)
		}
	}
	for _, to := range []string{"/rel", "{path}", "https://{1}.{2}.example.com", "https://{unknown}"} {
		s := Site{Domains: []string{"*.example.com"}, Type: TypeRedirect, RedirectTo: to}
		if err := s.validate(false); err == nil {
			t.Errorf("%s 应被拒绝", to)
		}
	}
}

// 访问端到端：STUN 端口变化后无需改站点，跳转地址跟着变；规则停止后返回 503
func TestRedirectSTUNEndToEnd(t *testing.T) {
	stunaddr.Set("e2e", "wz", netip.MustParseAddrPort("203.0.113.7:51234"))
	defer stunaddr.Delete("e2e")
	l := Listener{
		Sites: []Site{{Enable: true, Domains: []string{"*.7772888.xyz"}, Type: TypeRedirect,
			RedirectTo: "https://{1}.v4.7772888.xyz:{STUN_wz_PORT}{path}{args}", KeepPath: true, RedirectCode: 302}},
		Default: Site{Enable: true, Type: TypeText, Text: "default", StatusCode: 404},
	}
	addr, _ := startRunner(t, l, nil, nil)
	c := noRedirect()

	resp, _ := get(t, c, addr, "zly.7772888.xyz", "/dav/a?x=1", nil)
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "https://zly.v4.7772888.xyz:51234/dav/a?x=1" {
		t.Fatalf("重定向异常: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	stunaddr.Set("e2e", "wz", netip.MustParseAddrPort("203.0.113.7:52000"))
	resp, _ = get(t, c, addr, "tv.7772888.xyz", "/", nil)
	if resp.Header.Get("Location") != "https://tv.v4.7772888.xyz:52000/" {
		t.Fatalf("端口变化后应跟着变: %s", resp.Header.Get("Location"))
	}
	stunaddr.Delete("e2e")
	resp, body := get(t, c, addr, "zly.7772888.xyz", "/", map[string]string{"Accept": "text/html"})
	// 原因只写日志，页面不向访客透露 STUN 规则名
	if resp.StatusCode != 503 || resp.Header.Get("Location") != "" || strings.Contains(body, "暂无公网地址") {
		t.Fatalf("STUN 无地址时应返回 503 且不暴露规则名: %d %q", resp.StatusCode, body)
	}

	// Host 含非法字符时返回 400
	resp, _ = get(t, c, addr, "a&b=1.7772888.xyz", "/", nil)
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
		t.Fatalf("非法 Host 应返回 400: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// Lucky 的重定向子规则常用 {host}{path}{args}、{port}、{1} 与 STUN 变量，此前导入时校验失败会跳过整条监听
func TestConvertLuckyRedirectVars(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{{
		"RuleName": "redir", "Enable": true, "ListenPort": 2053, "Network": "tcp6",
		"DefaultProxy": map[string]any{"Locations": []string{"https://{host}:{port}{path}{args}"}, "WebServiceType": "redirect"},
		"ProxyList": []map[string]any{
			{"Enable": true, "Domains": []string{"*.7772888.xyz"}, "WebServiceType": "redirect",
				"Locations": []string{"https://{1}.v4.7772888.xyz:{STUN_wz_PORT}{path}{args}"}},
			{"Enable": true, "Domains": []string{"a.7772888.xyz"}, "WebServiceType": "url",
				"Locations": []string{"https://{host}{path}{args}"}},
		},
	}})
	ls, warns, found, err := convertLuckyListeners(raw, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if found != 1 || len(ls) != 1 {
		t.Fatalf("监听应被导入: found=%d imported=%d %v", found, len(ls), warns)
	}
	l := ls[0]
	if l.Default.Type != TypeRedirect || l.Default.RedirectTo != "https://{host}:{port}{path}{args}" {
		t.Fatalf("默认子规则映射错误: %+v", l.Default)
	}
	if len(l.Sites) != 2 || l.Sites[0].RedirectTo != "https://{1}.v4.7772888.xyz:{STUN_wz_PORT}{path}{args}" || l.Sites[1].Type != TypeRedirect {
		t.Fatalf("子规则映射错误: %+v", l.Sites)
	}
	// 导入后实际跳转：路径只出现一次
	r := httptest.NewRequest("GET", "http://a.7772888.xyz/p?q=1", nil)
	if got, _ := redirectTarget(&l.Sites[1], r); got != "https://a.7772888.xyz/p?q=1" {
		t.Fatalf("导入后的跳转地址错误: %q", got)
	}
}
