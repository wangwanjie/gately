package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

func TestTightenConfigDir(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, mode os.FileMode) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// 模拟旧版本在忽略创建权限的文件系统上留下的文件
	hist := write("history/frp/20261019T164942.550610455Z.json", 0o755)
	frpc := write("frp/frpc.toml", 0o755)
	audit := write("audit.log.1", 0o644)
	log := write("logs/ddns.log", 0o755)
	// bin/ 里是用户放的程序，必须保持可执行
	bin := write("bin/frpc", 0o755)
	if n := tightenConfigDir(dir); n == 0 {
		t.Fatal("应有改动")
	}
	for _, p := range []string{hist, frpc, audit, log} {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("%s 权限 %o", p, st.Mode().Perm())
		}
	}
	for _, d := range []string{"history", "history/frp", "frp", "logs"} {
		st, _ := os.Stat(filepath.Join(dir, d))
		if st.Mode().Perm() != 0o700 {
			t.Fatalf("%s 权限 %o", d, st.Mode().Perm())
		}
	}
	if st, _ := os.Stat(bin); st.Mode().Perm() != 0o755 {
		t.Fatalf("bin/ 不应被改动，实际 %o", st.Mode().Perm())
	}
	if n := tightenConfigDir(dir); n != 0 {
		t.Fatalf("再次执行不应改动，实际 %d", n)
	}
}

// TestLogsFilterAPI 验证日志接口的过滤参数：级别、来源、关键词、IP、站点、状态码，以及按请求语言匹配译文。
func TestLogsFilterAPI(t *testing.T) {
	a, srv, c := loginApp(t)
	pf := a.modules[0].logs
	l := pf.Logger("规则A")
	l.With(logbuf.Fields{logbuf.FieldIP: "10.0.0.5", logbuf.FieldHost: "a.example.com", logbuf.FieldSite: "博客", logbuf.FieldStatus: "404"}).Infof("10.0.0.5 GET a.example.com/x 404 1ms")
	l.With(logbuf.Fields{logbuf.FieldIP: "10.0.0.6", logbuf.FieldHost: "b.example.com", logbuf.FieldStatus: "502"}).Warnf("10.0.0.6 GET b.example.com/y 502 9ms")
	pf.Logger("规则B").Errorf("连接 192.168.1.9:80 失败")

	get := func(query, lang string) (int, []logbuf.Entry) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/api/portforward/logs?"+query, nil)
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out []logbuf.Entry
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
		}
		return resp.StatusCode, out
	}
	// 只关心本测试写入的日志：按来源前缀区分
	msgs := func(es []logbuf.Entry) []string {
		var out []string
		for _, e := range es {
			if strings.HasPrefix(e.Source, "规则") {
				out = append(out, e.Message)
			}
		}
		return out
	}

	cases := []struct {
		query string
		want  string
	}{
		{"level=warn", "[10.0.0.6 GET b.example.com/y 502 9ms 连接 192.168.1.9:80 失败]"},
		{"source=" + url.QueryEscape("规则B"), "[连接 192.168.1.9:80 失败]"},
		{"q=" + url.QueryEscape("GET -b.example"), "[10.0.0.5 GET a.example.com/x 404 1ms]"},
		// exclude 与 q 同时生效；exclude 中的词带不带 - 都是排除，包含任意一个即排除
		{"q=GET&exclude=" + url.QueryEscape("b.example"), "[10.0.0.5 GET a.example.com/x 404 1ms]"},
		{"exclude=" + url.QueryEscape("-404 \"192.168.1.9:80 失败\""), "[10.0.0.6 GET b.example.com/y 502 9ms]"},
		{"ip=10.0.0.0/24", "[10.0.0.5 GET a.example.com/x 404 1ms 10.0.0.6 GET b.example.com/y 502 9ms]"},
		{"ip=192.168.1.9", "[连接 192.168.1.9:80 失败]"},
		{"host=b.example", "[10.0.0.6 GET b.example.com/y 502 9ms]"},
		// site 完整匹配站点名称，只返回带该站点字段的访问日志；与 ip 同时生效
		{"site=" + url.QueryEscape("博客"), "[10.0.0.5 GET a.example.com/x 404 1ms]"},
		{"site=" + url.QueryEscape("博"), "[]"},
		{"site=" + url.QueryEscape("博客") + "&ip=10.0.0.6", "[]"},
		{"status=4xx", "[10.0.0.5 GET a.example.com/x 404 1ms]"},
		{"status=500-599&limit=1", "[10.0.0.6 GET b.example.com/y 502 9ms]"},
		{"from=" + url.QueryEscape(time.Now().Add(time.Hour).Format(time.RFC3339)), "[]"},
	}
	for _, cs := range cases {
		code, es := get(cs.query, "zh-CN")
		if code != http.StatusOK {
			t.Fatalf("%s: 状态码 %d", cs.query, code)
		}
		if got := fmt.Sprint(msgs(es)); got != cs.want {
			t.Errorf("%s = %s，期望 %s", cs.query, got, cs.want)
		}
	}
	// 毫秒时间戳可以直接作为 from/to
	if code, es := get(fmt.Sprintf("from=%d", time.Now().Add(-time.Hour).UnixMilli()), "zh-CN"); code != http.StatusOK || len(msgs(es)) != 3 {
		t.Errorf("毫秒时间戳 from：状态码 %d，%d 条", code, len(msgs(es)))
	}
	now := time.Now()
	reversed := "from=" + url.QueryEscape(now.Format(time.RFC3339)) + "&to=" + url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339))
	// 位数不足的纯数字（只填了年份）、from 晚于 to 都是无效参数
	for _, q := range []string{"status=abc", "level=fatal", "from=yesterday", "to=x", "from=2026", "to=17000000000", reversed} {
		if code, _ := get(q, ""); code != http.StatusBadRequest {
			t.Errorf("%s 应返回 400，得到 %d", q, code)
		}
	}

	// 系统日志里有登录记录，英文界面用译文关键词也能搜到
	req, _ := http.NewRequest("GET", srv.URL+"/api/logs?q="+url.QueryEscape("signed in from"), nil)
	req.Header.Set("Accept-Language", "en")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sys []logbuf.Entry
	_ = json.NewDecoder(resp.Body).Decode(&sys)
	if len(sys) == 0 || !strings.Contains(sys[len(sys)-1].Message, "signed in from") {
		t.Errorf("英文关键词应匹配登录日志的译文: %+v", sys)
	}
}

// TestLogsHideParams 验证 hideBanned、hideAdmin 与 IP 排除参数：只隐藏对应的访问日志，日志本身仍在。
func TestLogsHideParams(t *testing.T) {
	a, srv, c := loginApp(t)
	pf := a.modules[0].logs
	l := pf.Logger("隐藏测试")
	l.With(logbuf.Fields{logbuf.FieldIP: "1.2.3.4", logbuf.FieldStatus: "403", logbuf.FieldDeny: logbuf.DenyBan}).Infof("banned")
	l.With(logbuf.Fields{logbuf.FieldIP: "1.2.3.5", logbuf.FieldStatus: "403", logbuf.FieldDeny: "ip"}).Infof("ip-denied")
	l.With(logbuf.Fields{logbuf.FieldIP: "10.0.0.8", logbuf.FieldStatus: "200", logbuf.FieldAdmin: "admin"}).Infof("console")
	l.With(logbuf.Fields{logbuf.FieldIP: "10.0.0.9", logbuf.FieldStatus: "200"}).Infof("normal")

	query := func(q string) (int, string) {
		t.Helper()
		resp, err := c.Get(srv.URL + "/api/portforward/logs?source=" + url.QueryEscape("隐藏测试") + "&" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var es []logbuf.Entry
		_ = json.NewDecoder(resp.Body).Decode(&es)
		var msgs []string
		for _, e := range es {
			msgs = append(msgs, e.Message)
		}
		return resp.StatusCode, strings.Join(msgs, ",")
	}
	cases := map[string]string{
		"":                                     "banned,ip-denied,console,normal",
		"hideBanned=1":                         "ip-denied,console,normal",
		"hideAdmin=true":                       "banned,ip-denied,normal",
		"hideBanned=1&hideAdmin=1":             "ip-denied,normal",
		"hideBanned=0&hideAdmin=no":            "banned,ip-denied,console,normal",
		"ip=" + url.QueryEscape("!1.2.3.0/24"): "console,normal",
		"ip=" + url.QueryEscape("10.0.0.0/8,-10.0.0.8"):    "normal",
		"hideBanned=1&ip=" + url.QueryEscape("1.2.3.0/24"): "ip-denied",
	}
	for q, want := range cases {
		code, got := query(q)
		if code != http.StatusOK || got != want {
			t.Errorf("%q: 状态码 %d，结果 %q，期望 %q", q, code, got, want)
		}
	}
}

// TestSessionCheckHook 验证管理后台注册的登录态校验：有效 Cookie 返回用户名，伪造、篡改或缺失时返回空串，
// Authorization 头中的令牌不参与（多是发给后端应用的）。
func TestSessionCheckHook(t *testing.T) {
	_, srv, c := loginApp(t)
	u, _ := url.Parse(srv.URL)
	var token string
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == auth.CookieName {
			token = ck.Value
		}
	}
	if token == "" {
		t.Fatal("登录后应有后台 Cookie")
	}
	req := func(cookie, bearer string) *http.Request {
		r := httptest.NewRequest("GET", "http://site.test/api/webservice/logs", nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		return r
	}
	if got := httpx.SessionUser(req(token, "")); got != "admin" {
		t.Errorf("有效登录态应返回用户名，得到 %q", got)
	}
	p, sig, _ := strings.Cut(token, ".")
	tampered := p + "x." + sig
	for name, r := range map[string]*http.Request{
		"无 Cookie":         req("", ""),
		"篡改载荷":             req(tampered, ""),
		"签名长度不对":           req(p+".AAAA", ""),
		"不是令牌格式":           req("abc", ""),
		"只在 Authorization": req("", token),
	} {
		if got := httpx.SessionUser(r); got != "" {
			t.Errorf("%s: 应返回空串，得到 %q", name, got)
		}
	}
}
