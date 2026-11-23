package webservice

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProxyKeepsRawQuery 覆盖 #116：反向代理把访客的查询串原样交给后端，含分号、括号、空值、
// %2B 等编码与无效转义的参数都不能被丢弃或重新编码；去前缀的分路径规则与建连失败换后端重试同样原样转发。
func TestProxyKeepsRawQuery(t *testing.T) {
	main, sub, live := pathEcho(t, "main"), pathEcho(t, "sub"), pathEcho(t, "live")
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"app.test"}, Targets: []string{main.URL},
		Locations: []Location{
			{Prefix: "/v1/", Targets: []string{sub.URL + "/inner/"}, StripPrefix: true},
			{Prefix: "/retry/", Targets: []string{deadURL, live.URL}},
		},
	}}}
	addr, _ := startRunner(t, l, nil, nil)
	const q = "page=1&size=500&orders=&filters=and;(user_id;eq;123,and;(timestamp;gte;2026-11-23T00:00:00.000%2B00:00,timestamp;lte;2026-11-23T23:59:59.999%2B00:00))"
	cases := []struct{ path, want string }{
		{"/api/test?" + q, "main|/api/test?" + q},
		{"/api/test?a=1;b=2&c=%zz&d", "main|/api/test?a=1;b=2&c=%zz&d"},
		{"/v1/test?" + q, "sub|/inner/test?" + q},
	}
	for _, c := range cases {
		if _, body := get(t, http.DefaultClient, addr, "app.test", c.path, nil); body != c.want {
			t.Errorf("访问 %s 得到 %q，期望 %q", c.path, body, c.want)
		}
	}
	// 轮询会让死后端多次成为首选，重试后的请求同样带原始查询串
	for i := 0; i < 4; i++ {
		if _, body := get(t, http.DefaultClient, addr, "app.test", "/retry/x?"+q, nil); body != "live|/retry/x?"+q {
			t.Fatalf("第 %d 次请求得到 %q", i+1, body)
		}
	}
}
