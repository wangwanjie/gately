package apitoken

import "testing"

// TestRequiredIssue67 覆盖 #67 新增接口所需的令牌权限。
func TestRequiredIssue67(t *testing.T) {
	cases := []struct{ method, path, want string }{
		// 中断 Docker 任务属于写操作
		{"POST", "/api/docker/tasks/abc-1/cancel", "docker:write"},
		{"GET", "/api/docker/tasks/abc-1", "docker:read"},
		// 通知设置：读取需要读权限，保存需要写权限
		{"GET", "/api/notify/settings", "notify:read"},
		{"PUT", "/api/notify/settings", "notify:write"},
		// 接口描述任何有效令牌都可以读取
		{"GET", "/api/openapi.json", Any},
		// 日志按站点过滤沿用日志读权限
		{"GET", "/api/webservice/logs", "webservice:read"},
	}
	for _, c := range cases {
		if got := Required(c.method, c.path); got != c.want {
			t.Errorf("Required(%s %s) = %q，期望 %q", c.method, c.path, got, c.want)
		}
	}
}

// TestRequiredWOLShutdown 覆盖网络唤醒远程关机相关接口：能设置关机连接与命令或触发关机的接口还要求 Web 终端写权限，
// 否则只有 wol:write 的令牌就能借设备配置经 Web 终端的 SSH 连接执行任意命令。
func TestRequiredWOLShutdown(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"POST", "/api/wol/devices", "wol:write,webterminal:write"},
		{"PUT", "/api/wol/devices/abc", "wol:write,webterminal:write"},
		{"POST", "/api/wol/devices/abc/copy", "wol:write,webterminal:write"},
		{"POST", "/api/wol/devices/abc/shutdown", "wol:write,webterminal:write"},
		{"GET", "/api/wol/ssh-targets", "wol:read,webterminal:read"},
		// 不涉及 SSH 的接口不变
		{"POST", "/api/wol/devices/abc/wake", "wol:write"},
		{"PUT", "/api/wol/devices/abc/enable", "wol:write"},
		{"DELETE", "/api/wol/devices/abc", "wol:write"},
		{"PUT", "/api/wol/devices-order", "wol:write"},
		{"POST", "/api/wol/devices-delete", "wol:write"},
		{"GET", "/api/wol/devices", "wol:read"},
		{"GET", "/api/wol/devices/abc/secrets", "wol:write"},
		{"PUT", "/api/wol/bemfa", "wol:write"},
	}
	for _, c := range cases {
		if got := Required(c.method, c.path); got != c.want {
			t.Errorf("Required(%s %s) = %q，期望 %q", c.method, c.path, got, c.want)
		}
	}
	if Match([]string{"wol:write"}, Required("POST", "/api/wol/devices/abc/shutdown")) {
		t.Error("只有 wol:write 的令牌不应能触发远程关机")
	}
	if !Match([]string{"wol:write", "webterminal:write"}, Required("POST", "/api/wol/devices/abc/shutdown")) {
		t.Error("同时具备两项写权限的令牌应能触发远程关机")
	}
}
