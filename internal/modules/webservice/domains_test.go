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
