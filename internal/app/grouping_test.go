package app

import (
	"testing"

	"github.com/wangwanjie/gately/internal/apitoken"
)

// 支持分组排序的列表都要注册排序与批量分组接口（issue #48），漏掉时管理界面的排序模式会报错。
func TestGroupedListRoutes(t *testing.T) {
	a := newDocApp(t)
	have := map[string]bool{}
	for _, e := range a.buildDoc().Endpoints {
		have[e.Method+" "+e.Path] = true
	}
	lists := []string{
		"/api/ddns/tasks",
		"/api/portforward/rules",
		"/api/stun/rules",
		"/api/ssl/certs",
		"/api/frp/clients",
		"/api/frp/servers",
		"/api/frp/proxies",
		"/api/cloudflared/tunnels",
		"/api/filebrowser/instances",
		"/api/waf/instances",
		"/api/cron/tasks",
		"/api/wol/devices",
		"/api/notify/channels",
		"/api/notify/rules",
		"/api/webterminal/connections",
		"/api/storage/mounts",
		"/api/guard/jails",
	}
	for _, p := range lists {
		for _, ep := range []string{"PUT " + p + "-order", "PUT " + p + "-group"} {
			if !have[ep] {
				t.Errorf("缺少接口 %s", ep)
			}
		}
	}
	for _, ep := range []string{"PUT /api/ddns/tasks/{key}/records-layout", "PUT /api/tokens/order", "PUT /api/tokens/group"} {
		if !have[ep] {
			t.Errorf("缺少接口 %s", ep)
		}
	}
	// 令牌管理接口与其他令牌接口一样不允许用令牌调用
	for _, p := range []string{"/api/tokens/order", "/api/tokens/group"} {
		if got := apitoken.Required("PUT", p); got != apitoken.Denied {
			t.Errorf("%s 应拒绝令牌访问，实际 %q", p, got)
		}
	}
}
