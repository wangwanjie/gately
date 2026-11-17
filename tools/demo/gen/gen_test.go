package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangwanjie/gately/internal/modules/cron"
	"github.com/wangwanjie/gately/internal/modules/ddns"
	"github.com/wangwanjie/gately/internal/modules/frp"
	"github.com/wangwanjie/gately/internal/modules/guard"
	"github.com/wangwanjie/gately/internal/modules/knock"
	"github.com/wangwanjie/gately/internal/modules/netbird"
	"github.com/wangwanjie/gately/internal/modules/notify"
	"github.com/wangwanjie/gately/internal/modules/perf"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/stun"
	"github.com/wangwanjie/gately/internal/modules/tailscale"
	"github.com/wangwanjie/gately/internal/modules/waf"
	"github.com/wangwanjie/gately/internal/modules/webservice"
	"github.com/wangwanjie/gately/internal/modules/wol"
)

// loadItems 读取生成的配置文件中的条目列表。
func loadItems[T any](t *testing.T, dir, name, field string) []T {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var out []T
	if err := json.Unmarshal(raw[field], &out); err != nil {
		t.Fatalf("%s.%s: %v", name, field, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s.%s 没有演示数据", name, field)
	}
	return out
}

// 演示数据要能通过各模块保存时的校验：否则界面上会显示保存时不可能出现的状态
// （例如设备选了 SSH 关机却没有连接，显示「连接已删除 ()」）。
func TestGeneratedItemsValidate(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	for _, d := range loadItems[wol.Device](t, dir, "wol", "devices") {
		if err := d.Validate(); err != nil {
			t.Errorf("网络唤醒设备 %s: %v", d.Key, err)
		}
	}
	for _, r := range loadItems[portforward.Rule](t, dir, "portforward", "rules") {
		if err := r.Validate(); err != nil {
			t.Errorf("端口转发规则 %s: %v", r.Key, err)
		}
	}
	for _, task := range loadItems[ddns.Task](t, dir, "ddns", "tasks") {
		if err := task.Validate(); err != nil {
			t.Errorf("动态域名任务 %s: %v", task.Key, err)
		}
	}
	for _, r := range loadItems[stun.Rule](t, dir, "stun", "rules") {
		if err := r.Validate(); err != nil {
			t.Errorf("STUN 规则 %s: %v", r.Key, err)
		}
	}
	for _, r := range loadItems[knock.Rule](t, dir, "knock", "rules") {
		if err := r.Validate(); err != nil {
			t.Errorf("敲门规则 %s: %v", r.Key, err)
		}
	}
	for _, task := range loadItems[cron.Task](t, dir, "cron", "tasks") {
		if err := task.Validate(); err != nil {
			t.Errorf("计划任务 %s: %v", task.Key, err)
		}
	}
	for _, c := range loadItems[notify.Channel](t, dir, "notify", "channels") {
		if err := c.Validate(); err != nil {
			t.Errorf("通知渠道 %s: %v", c.Key, err)
		}
	}
	for _, r := range loadItems[notify.Rule](t, dir, "notify", "rules") {
		if err := r.Validate(); err != nil {
			t.Errorf("通知规则 %s: %v", r.Key, err)
		}
	}
	for _, c := range loadItems[frp.Client](t, dir, "frp", "clients") {
		if err := c.Validate(); err != nil {
			t.Errorf("frpc 实例 %s: %v", c.Key, err)
		}
	}
	for _, s := range loadItems[frp.Server](t, dir, "frp", "servers") {
		if err := s.Validate(); err != nil {
			t.Errorf("frps 实例 %s: %v", s.Key, err)
		}
	}
	for _, p := range loadItems[frp.Proxy](t, dir, "frp", "proxies") {
		if err := p.Validate(); err != nil {
			t.Errorf("frp 代理 %s: %v", p.Key, err)
		}
	}
	for _, in := range loadItems[waf.Instance](t, dir, "waf", "instances") {
		if err := in.Validate(); err != nil {
			t.Errorf("WAF 实例 %s: %v", in.Key, err)
		}
	}
	for _, j := range loadItems[guard.Jail](t, dir, "guard", "jails") {
		if err := j.Validate(); err != nil {
			t.Errorf("入侵防护规则 %s: %v", j.Key, err)
		}
	}
	for _, in := range loadItems[tailscale.Instance](t, dir, "tailscale", "instances") {
		if err := in.Validate(); err != nil {
			t.Errorf("Tailscale 实例 %s: %v", in.Key, err)
		}
	}
	for _, in := range loadItems[netbird.Instance](t, dir, "netbird", "instances") {
		if err := in.Validate(); err != nil {
			t.Errorf("NetBird 实例 %s: %v", in.Key, err)
		}
	}
	for _, r := range loadItems[perf.Rule](t, dir, "perf", "rules") {
		if err := r.Validate(); err != nil {
			t.Errorf("性能告警规则 %s: %v", r.Key, err)
		}
	}
	for _, l := range loadItems[webservice.Listener](t, dir, "webservice", "listeners") {
		if err := l.Validate(); err != nil {
			t.Errorf("Web 监听 %s: %v", l.Key, err)
		}
	}
}
