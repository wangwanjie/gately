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
