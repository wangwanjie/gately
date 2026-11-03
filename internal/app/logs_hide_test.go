package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestLogsHideBannedIPv6AndNets 验证 hideBanned 按当前封禁列表隐藏来源时，IPv6 按 /64 封禁键匹配、
// 网段封禁覆盖段内地址、IPv4 映射地址按 IPv4 处理；解封后重新显示。
func TestLogsHideBannedIPv6AndNets(t *testing.T) {
	a, srv, c := loginApp(t)
	l := a.modules[0].logs.Logger("隐藏v6")
	add := func(ip, msg string) {
		l.With(logbuf.Fields{logbuf.FieldIP: ip, logbuf.FieldStatus: "444"}).Infof("%s", msg)
	}
	add("2001:db8:1:2::abcd", "v6-same64")
	add("2001:db8:1:3::1", "v6-other64")
	add("198.51.100.9", "v4-in-net")
	add("::ffff:198.51.100.10", "v4mapped-in-net")
	add("198.51.101.1", "v4-out")
	add("not-an-ip", "bad-ip")
	l.Infof("no-ip")

	keys := []string{}
	for _, s := range []string{"2001:db8:1:2::1", "198.51.100.0/24"} {
		k, err := banlist.ParseKey(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := banlist.Default.Ban(k, banlist.Options{Duration: time.Minute, Force: true}); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			banlist.Default.Unban(k)
		}
	})

	query := func(q string) string {
		t.Helper()
		resp, err := c.Get(srv.URL + "/api/portforward/logs?source=" + url.QueryEscape("隐藏v6") + "&" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%q: 状态码 %d", q, resp.StatusCode)
		}
		var es []logbuf.Entry
		_ = json.NewDecoder(resp.Body).Decode(&es)
		var msgs []string
		for _, e := range es {
			msgs = append(msgs, e.Message)
		}
		return strings.Join(msgs, ",")
	}
	if got, want := query(""), "v6-same64,v6-other64,v4-in-net,v4mapped-in-net,v4-out,bad-ip,no-ip"; got != want {
		t.Fatalf("不隐藏时: %q，期望 %q", got, want)
	}
	if got, want := query("hideBanned=1"), "v6-other64,v4-out,bad-ip,no-ip"; got != want {
		t.Fatalf("隐藏封禁来源: %q，期望 %q", got, want)
	}
	// 解封后重新显示
	for _, k := range keys {
		banlist.Default.Unban(k)
	}
	if got, want := query("hideBanned=1"), "v6-same64,v6-other64,v4-in-net,v4mapped-in-net,v4-out,bad-ip,no-ip"; got != want {
		t.Fatalf("解封后: %q，期望 %q", got, want)
	}
}
