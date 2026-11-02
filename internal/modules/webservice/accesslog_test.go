package webservice

import (
	"io"
	"log"
	"net/http"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestAccessLogFields 验证访问日志与访问控制拒绝日志带上结构化字段，日志页可按站点、域名、状态码与 IP 过滤。
func TestAccessLogFields(t *testing.T) {
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	l := Listener{
		Sites: []Site{
			{Enable: true, Name: "博客", Domains: []string{"blog.test"}, Type: TypeText, Text: "gone", StatusCode: 404, AccessLog: true},
			{Enable: true, Name: "内网", Domains: []string{"lan.test"}, Type: TypeText, Text: "x", AccessLog: true, IPMode: IPModeWhitelist, IPList: []string{"10.0.0.0/8"}},
		},
	}
	addr, _ := startRunnerLogs(t, l, nil, nil, logs)
	c := noRedirect()
	get(t, c, addr, "blog.test:8080", "/post?id=1", nil)
	get(t, c, addr, "lan.test", "/", nil)

	f := &logbuf.Filter{Status: []logbuf.StatusRange{{Min: 404, Max: 404}}}
	got := logs.Query(0, 0, 0, f)
	if len(got) != 1 {
		t.Fatalf("按状态码过滤应得到 1 条访问日志，得到 %+v", got)
	}
	want := logbuf.Fields{"ip": "127.0.0.1", "host": "blog.test", "site": "博客", "method": "GET", "path": "/post", "status": "404"}
	for k, v := range want {
		if got[0].Fields[k] != v {
			t.Errorf("字段 %s = %q，期望 %q（%+v）", k, got[0].Fields[k], v, got[0].Fields)
		}
	}
	denied := logs.Query(0, 0, 0, &logbuf.Filter{Host: "内网", MinLevel: logbuf.LevelWarn})
	if len(denied) != 1 || denied[0].Fields["ip"] != "127.0.0.1" {
		t.Fatalf("访问控制拒绝日志应带站点与 IP 字段: %+v", denied)
	}
}

// TestAccessLogDenyAndAdmin 验证访问日志记录拦截原因（封禁、IP 访问控制、限流）与后台登录态用户名，
// 日志页据此隐藏被封禁来源的请求与管理员自己的请求；关闭访问日志的站点不校验登录态。
func TestAccessLogDenyAndAdmin(t *testing.T) {
	var checks atomic.Int32
	httpx.SetSessionCheck(func(r *http.Request) string {
		checks.Add(1)
		if c, err := r.Cookie("gately_token"); err == nil && c.Value == "valid" {
			return "admin"
		}
		return ""
	})
	t.Cleanup(func() { httpx.SetSessionCheck(nil) })

	logs := logbuf.New(100, log.New(io.Discard, "", 0))
	l := Listener{
		Sites: []Site{
			{Enable: true, Name: "开放", Domains: []string{"open.test"}, Type: TypeText, Text: "ok", AccessLog: true},
			{Enable: true, Name: "断开", Domains: []string{"close.test"}, Type: TypeText, Text: "ok", AccessLog: true, DenyClose: true},
			{Enable: true, Name: "内网", Domains: []string{"lan.test"}, Type: TypeText, Text: "x", AccessLog: true, IPMode: IPModeWhitelist, IPList: []string{"10.0.0.0/8"}},
			{Enable: true, Name: "限流", Domains: []string{"rl.test"}, Type: TypeText, Text: "x", AccessLog: true, RateLimit: RateLimit{Enable: true, Rate: 0.001, Burst: 1}},
			{Enable: true, Name: "无日志", Domains: []string{"quiet.test"}, Type: TypeText, Text: "x"},
		},
	}
	addr, _ := startRunnerLogs(t, l, nil, nil, logs)
	c := noRedirect()
	last := func() logbuf.Entry {
		t.Helper()
		es := logs.Query(0, 0, 1, &logbuf.Filter{Status: []logbuf.StatusRange{{Min: 100, Max: 599}}})
		if len(es) != 1 {
			t.Fatalf("应有访问日志: %+v", es)
		}
		return es[0]
	}

	// 管理员的请求带用户名，伪造的 Cookie 不带
	get(t, c, addr, "open.test", "/api/webservice/logs?since=1", map[string]string{"Cookie": "gately_token=valid"})
	if e := last(); e.Fields[logbuf.FieldAdmin] != "admin" || e.Fields[logbuf.FieldDeny] != "" {
		t.Errorf("管理员请求应带 admin 字段且没有拦截原因: %+v", e.Fields)
	}
	get(t, c, addr, "open.test", "/", map[string]string{"Cookie": "gately_token=forged"})
	if e := last(); e.Fields[logbuf.FieldAdmin] != "" {
		t.Errorf("无效登录态不应带 admin 字段: %+v", e.Fields)
	}

	// 关闭访问日志的站点不必校验登录态
	before := checks.Load()
	get(t, c, addr, "quiet.test", "/", map[string]string{"Cookie": "gately_token=valid"})
	if checks.Load() != before {
		t.Error("未开启访问日志的站点不应校验登录态")
	}

	// IP 访问控制与限流
	get(t, c, addr, "lan.test", "/", nil)
	if e := last(); e.Fields[logbuf.FieldDeny] != "ip" || e.Fields[logbuf.FieldStatus] != "403" {
		t.Errorf("IP 访问控制拒绝应记 deny=ip: %+v", e.Fields)
	}
	get(t, c, addr, "rl.test", "/", nil)
	get(t, c, addr, "rl.test", "/", nil)
	if e := last(); e.Fields[logbuf.FieldDeny] != "rateLimit" || e.Fields[logbuf.FieldStatus] != "429" {
		t.Errorf("限流拒绝应记 deny=rateLimit: %+v", e.Fields)
	}

	// 封禁：返回 403 与断开连接（444）两种拒绝方式都记 deny=ban；管理员被封禁时同样带用户名
	key := banlist.KeyFor(netip.MustParseAddr("127.0.0.1"))
	if _, _, err := banlist.Default.Ban(key, banlist.Options{Duration: time.Minute, Force: true, Source: banlist.SourceManual}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban(key) })
	if resp, _ := get(t, c, addr, "open.test", "/", map[string]string{"Cookie": "gately_token=valid"}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("被封禁的来源应返回 403，得到 %d", resp.StatusCode)
	}
	if e := last(); e.Fields[logbuf.FieldDeny] != logbuf.DenyBan || e.Fields[logbuf.FieldStatus] != "403" || e.Fields[logbuf.FieldAdmin] != "admin" {
		t.Errorf("被封禁的请求应记 deny=ban: %+v", e.Fields)
	}
	req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
	req.Host = "close.test"
	// 新连接发出：复用的连接被断开时 http.Client 会自动重试一次，多记一条
	fresh := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	if resp, err := fresh.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("拒绝时断开连接的站点不应返回响应")
	}
	if e := last(); e.Fields[logbuf.FieldDeny] != logbuf.DenyBan || e.Fields[logbuf.FieldStatus] != "444" {
		t.Errorf("断开连接的封禁请求应记 deny=ban、444: %+v", e.Fields)
	}

	// 隐藏被封禁来源后，只剩其他请求；日志本身仍在
	all := logs.Query(0, 0, 0, &logbuf.Filter{Status: []logbuf.StatusRange{{Min: 100, Max: 599}}})
	shown := logs.Query(0, 0, 0, &logbuf.Filter{Status: []logbuf.StatusRange{{Min: 100, Max: 599}}, Hide: []logbuf.FieldCond{{Key: logbuf.FieldDeny, Value: logbuf.DenyBan}, {Key: logbuf.FieldAdmin}}})
	if len(all) != 7 || len(shown) != 4 {
		t.Errorf("全部访问日志 %d 条（期望 7），隐藏封禁与后台请求后 %d 条（期望 4）", len(all), len(shown))
	}
}
