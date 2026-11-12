package webservice

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// 部分后端（PHP、WSGI、CGI 一类）把请求头名称中的 - 与 _ 映射为同一个变量，访客发送的
// X_Forwarded_For、X_Real_IP 等下划线写法会与 Gately 写入的真实头冲突，甚至覆盖它。
// 这些写法在转发给后端前必须删除，无论是否开启“添加 X-Forwarded-*”。
func TestForwardedHeaderUnderscoreVariantsStripped(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var names []string
		for k := range r.Header {
			if strings.Contains(k, "_") || strings.HasPrefix(strings.ToLower(k), "x-forwarded") || strings.HasPrefix(strings.ToLower(k), "x-real") || strings.EqualFold(k, "Forwarded") {
				names = append(names, k+"="+strings.Join(r.Header.Values(k), ","))
			}
		}
		sort.Strings(names)
		_, _ = io.WriteString(w, strings.Join(names, ";"))
	}))
	defer backend.Close()
	sites := []Site{
		{Enable: true, Key: "p", Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{backend.URL}, ForwardedHeaders: true},
		{Enable: true, Key: "n", Domains: []string{"plain.test"}, Type: TypeProxy, Targets: []string{backend.URL}},
	}
	addr, _ := startRunner(t, Listener{Sites: sites}, nil, []string{"127.0.0.1"})
	forged := []string{"X_Forwarded_For", "X_forwarded_host", "X_Forwarded_Proto", "X_Forwarded_Port", "X_Real_IP", "X_Forwarded_Ssl", "X-Forwarded-Ssl", "X-Forwarded-Port"}
	for _, host := range []string{"app.test", "plain.test"} {
		q, _ := http.NewRequest("GET", "http://"+host+"/", nil)
		for _, k := range forged {
			q.Header[k] = []string{"9.9.9.9"}
		}
		_, body := sendDirect(t, addr, q)
		for _, name := range strings.Split(body, ";") {
			if strings.Contains(name, "9.9.9.9") {
				t.Fatalf("%s：访客伪造的头 %q 不应到达后端: %s", host, name, body)
			}
		}
	}
}
