package webservice

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// 转发认证的子请求同样不能带上访客伪造的来源类请求头变体，认证服务可能据此判断来源或原始地址。
func TestForwardAuthStripsForwardedVariants(t *testing.T) {
	got := make(chan string, 1)
	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var bad []string
		for k, v := range r.Header {
			if strings.Contains(strings.Join(v, ","), "9.9.9.9") {
				bad = append(bad, k)
			}
		}
		sort.Strings(bad)
		got <- strings.Join(bad, ",")
		w.WriteHeader(http.StatusOK)
	}))
	defer authz.Close()
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"fa.test"}, Type: TypeText, Text: "ok",
		ForwardAuth: ForwardAuth{URL: authz.URL},
	}}}
	addr, _ := startRunner(t, l, nil, []string{"127.0.0.1"})
	q, _ := http.NewRequest("GET", "http://fa.test/", nil)
	for _, k := range []string{"X_Forwarded_For", "X_Real_IP", "X_Forwarded_Host", "X_Original_URL", "X_Forwarded_Uri", "X-Forwarded-Ssl"} {
		q.Header[k] = []string{"9.9.9.9"}
	}
	sendDirect(t, addr, q)
	if v := <-got; v != "" {
		t.Fatalf("认证服务收到了访客伪造的头: %s", v)
	}
}
