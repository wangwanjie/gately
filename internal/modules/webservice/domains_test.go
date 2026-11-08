package webservice

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/luckyimport"
)

// TestSiteDomainsNormalize 校验站点域名的规范化与拒绝：永远匹配不上 Host 的写法在保存时报错。
func TestSiteDomainsNormalize(t *testing.T) {
	l := Listener{Port: 8080, Sites: []Site{{
		Domains: []string{" A.Example.com. ", "中文.com", "[::1]", "*.Wild.com", "a.example.com"},
		Targets: []string{"http://127.0.0.1:1"},
	}}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(l.Sites[0].Domains, ",")
	if got != "a.example.com,xn--fiq228c.com,::1,*.wild.com" {
		t.Fatalf("规范化结果错误: %s", got)
	}
	for _, d := range []string{"a.com:8080", "a.com/x", "evil.com/@x", "javascript:alert(1)", "user@a.com", "a..com", "[a.com]"} {
		bad := Listener{Port: 8080, Sites: []Site{{Domains: []string{d}, Targets: []string{"http://127.0.0.1:1"}}}}
		if err := bad.Validate(); err == nil {
			t.Errorf("域名 %q 应报错", d)
		}
	}
}

// TestIDNDomainMatchesPunycodeHost 中文域名保存为 punycode 后，浏览器发来的 punycode Host 能命中站点。
func TestIDNDomainMatchesPunycodeHost(t *testing.T) {
	addr, _ := startRunner(t, Listener{Sites: []Site{
		{Enable: true, Domains: []string{"中文.com"}, Type: TypeText, Text: "idn"},
		{Enable: true, Domains: []string{"[::1]"}, Type: TypeText, Text: "v6"},
	}}, nil, nil)
	for host, want := range map[string]string{"xn--fiq228c.com": "idn", "XN--FIQ228C.COM:8080": "idn", "[::1]:8080": "v6"} {
		resp, body := get(t, http.DefaultClient, addr, host, "/", nil)
		if resp.StatusCode != 200 || body != want {
			t.Errorf("Host %s: %d %q，期望 %q", host, resp.StatusCode, body, want)
		}
	}
}

// TestLabelWildcardRouting 覆盖精确域名、首段通配、后缀通配与兜底的路由优先级。
func TestLabelWildcardRouting(t *testing.T) {
	addr, _ := startRunner(t, Listener{Sites: []Site{
		{Enable: true, Domains: []string{"*"}, Type: TypeText, Text: "fallback"},
		{Enable: true, Domains: []string{"*.example.com"}, Type: TypeText, Text: "suffix"},
		{Enable: true, Domains: []string{"ping-*.example.com"}, Type: TypeText, Text: "label"},
		{Enable: true, Domains: []string{"ping-one.example.com"}, Type: TypeText, Text: "exact"},
	}}, nil, nil)
	for host, want := range map[string]string{
		"ping-one.example.com": "exact", "PING-TWO.EXAMPLE.COM:8080": "label",
		"ping-two.nested.example.com": "suffix", "other.example.com": "suffix",
		"example.com": "fallback", "ping-two.example.com.evil.test": "fallback",
	} {
		resp, body := get(t, http.DefaultClient, addr, host, "/", nil)
		if resp.StatusCode != 200 || body != want {
			t.Errorf("Host %s: %d %q; want %q", host, resp.StatusCode, body, want)
		}
	}
}

func TestLabelWildcardAuth(t *testing.T) {
	addr, _ := startRunner(t, Listener{Sites: []Site{
		{Enable: true, Domains: []string{"ping-*.example.com"}, Type: TypeText, Text: "private", BasicAuth: true, Users: []string{"alice:test"}},
		{Enable: true, Domains: []string{"api-ping-*.example.com"}, Type: TypeText, Text: "public"},
	}}, nil, nil)
	for host, want := range map[string]int{
		"ping-one.example.com": 401, "api-ping-one.example.com": 200,
		"ping-one.nested.example.com": 404, "other.example.com": 404,
	} {
		resp, _ := get(t, http.DefaultClient, addr, host, "/", nil)
		if resp.StatusCode != want {
			t.Errorf("Host %s: %d; want %d", host, resp.StatusCode, want)
		}
	}
	req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
	req.Host = "ping-one.example.com"
	req.SetBasicAuth("alice", "test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("authenticated request: %d", resp.StatusCode)
	}
}

func TestImportLabelWildcardDomains(t *testing.T) {
	raw := json.RawMessage(`[{"Enable":true,"ListenPort":8080,"Network":"tcp","ProxyList":[{"Enable":true,"Domains":["ping-*.example.com","api-ping-*.example.com"],"Locations":["http://127.0.0.1:1"],"WebServiceType":"reverseproxy","EnableBasicAuth":true,"BasicAuthUserList":["alice:test"]}]}]`)
	ls, warns, _, err := convertLuckyListeners(raw, luckyimport.Options{})
	if err != nil || len(ls) != 1 || len(ls[0].Sites) != 1 {
		t.Fatalf("import: %v, %v, %+v", err, warns, ls)
	}
	s := ls[0].Sites[0]
	if strings.Join(s.Domains, ",") != "ping-*.example.com,api-ping-*.example.com" || !s.BasicAuth || len(s.Users) != 1 || s.Targets[0] != "http://127.0.0.1:1" {
		t.Fatalf("import changed site settings: %+v", s)
	}
	if _, err := normalizeBatchDomains(s.Domains); err != nil {
		t.Fatal(err)
	}
}

// TestLabelWildcardSameLength 两条长度相同的首段通配同时命中时，以先出现的站点为准，与书写顺序一致且结果确定。
func TestLabelWildcardSameLength(t *testing.T) {
	first := Site{Enable: true, Domains: []string{"a*.example.com"}, Type: TypeText, Text: "a-prefix"}
	second := Site{Enable: true, Domains: []string{"*b.example.com"}, Type: TypeText, Text: "b-suffix"}
	for _, order := range [][]Site{{first, second}, {second, first}} {
		addr, _ := startRunner(t, Listener{Sites: order}, nil, nil)
		want := order[0].Text
		// 多次请求结果保持一致，不随 map 遍历等因素变化
		for range 5 {
			if resp, body := get(t, http.DefaultClient, addr, "ab.example.com", "/", nil); resp.StatusCode != 200 || body != want {
				t.Fatalf("Host ab.example.com: %d %q; want %q", resp.StatusCode, body, want)
			}
		}
		// 只命中其中一条时不受顺序影响
		if _, body := get(t, http.DefaultClient, addr, "ax.example.com", "/", nil); body != "a-prefix" {
			t.Fatalf("Host ax.example.com: %q", body)
		}
		if _, body := get(t, http.DefaultClient, addr, "xb.example.com", "/", nil); body != "b-suffix" {
			t.Fatalf("Host xb.example.com: %q", body)
		}
	}
}

// TestLabelWildcardCORSAndSNI 开启认证的站点用首段通配时，跨域来源与 TLS SNI 只认真正匹配该段的域名，
// 多出一级、换了后缀或在前面拼接字符的来源都不能借用登录态。
func TestLabelWildcardCORSAndSNI(t *testing.T) {
	h := &siteHandler{site: Site{CORS: true, WebAuth: true, Domains: []string{"api-*.app.test"}}}
	for origin, want := range map[string]bool{
		"https://api-one.app.test":          true,
		"https://API-ONE.app.test:8443":     true,
		"https://api-one.app.test.evil.com": false,
		"https://api-one.x.app.test":        false,
		"https://x.api-one.app.test":        false,
		"https://evilapi-one.app.test":      false,
		"https://api-.app.test":             false,
		"https://api-one.evil.test":         false,
		"null":                              false,
	} {
		if got := h.corsAllowed(origin); got != want {
			t.Errorf("corsAllowed(%q) = %v; want %v", origin, got, want)
		}
	}

	tbl := &siteTable{exact: map[string]*siteHandler{}, sites: []*siteHandler{h}}
	for name, want := range map[string]bool{
		"api-one.app.test": true, "API-ONE.APP.TEST.": true,
		"api-one.x.app.test": false, "other.app.test": false, "api-one.app.test.evil.com": false,
	} {
		if got := tbl.servesSNI(name); got != want {
			t.Errorf("servesSNI(%q) = %v; want %v", name, got, want)
		}
	}
}

// TestLabelWildcardRedirectCapture 首段通配捕获的 {1} 只含该段内的字符，不会带出 . / @ 等改变跳转主机的内容。
func TestLabelWildcardRedirectCapture(t *testing.T) {
	domains := []string{"*.b.test", "app-*-dev.b.test"}
	for host, want := range map[string]string{
		"app-one-dev.b.test": "one",
		"app--dev.b.test":    "",
		// 多出一级时首段通配不匹配，由 *.b.test 捕获完整的前缀
		"app-one.two-dev.b.test": "app-one.two-dev",
	} {
		if got := wildcardPart(domains, host); got != want {
			t.Errorf("wildcardPart(%q) = %q; want %q", host, got, want)
		}
	}
	// 只配置首段通配时，任何能命中的访客域名捕获值都不含主机分隔符
	for _, host := range []string{"app-a_b-dev.b.test", "app-x-y-dev.b.test", "app-1-dev.b.test"} {
		got := wildcardPart([]string{"app-*-dev.b.test"}, host)
		if got == host || strings.ContainsAny(got, "./@:") {
			t.Errorf("wildcardPart(%q) = %q", host, got)
		}
	}
}

// TestImportSkipsBadDomains 导入外部配置时只跳过写错的域名，同一站点、同一监听的其他内容照常导入。
func TestImportSkipsBadDomains(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{{
		"RuleName": "main", "Enable": true, "ListenPort": 8080, "Network": "tcp",
		"ProxyList": []map[string]any{
			{"Enable": true, "Domains": []string{"ok.example.com", "bad.com/x"}, "Locations": []string{"http://127.0.0.1:1"}, "WebServiceType": "reverseproxy"},
			{"Enable": true, "Domains": []string{"a.com:8080"}, "Locations": []string{"http://127.0.0.1:2"}, "WebServiceType": "reverseproxy"},
			{"Enable": true, "Domains": []string{"other.example.com"}, "Locations": []string{"http://127.0.0.1:3"}, "WebServiceType": "reverseproxy"},
		},
	}})
	ls, warns, _, err := convertLuckyListeners(raw, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || len(ls[0].Sites) != 2 {
		t.Fatalf("监听应保留并只跳过没有可用域名的站点: %+v %v", ls, warns)
	}
	if d := ls[0].Sites[0].Domains; len(d) != 1 || d[0] != "ok.example.com" {
		t.Fatalf("应只去掉写错的域名: %v", d)
	}
	w := strings.Join(warns, "\n")
	if !strings.Contains(w, "bad.com/x") || !strings.Contains(w, "a.com:8080") || !strings.Contains(w, "没有可用的域名") {
		t.Fatalf("应提示跳过的域名: %s", w)
	}

	conf := `server {
	listen 8443 ssl;
	server_name ok.example.com $hostname;
	return 200 "ok";
}`
	rep, err := importNginx(conf, nil, nameCerts{})
	if err != nil {
		t.Fatal(err)
	}
	p, pl, _ := findSite(rep, "ok.example.com")
	if p == nil || len(pl.Domains) != 1 || !strings.Contains(strings.Join(pl.Warnings, "\n"), "$hostname") {
		t.Fatalf("nginx 导入应跳过变量域名并提示: %+v", pl)
	}
}
