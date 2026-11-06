package app

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/proc"
	"github.com/wangwanjie/gately/internal/sysstat"
)

// 概览接口只要求 status:read，托管进程带有各模块的实例名称，按请求者的模块读权限过滤
func TestHostForRequestFiltersManaged(t *testing.T) {
	snap := sysstat.Snapshot{Managed: []sysstat.ManagedProcess{
		{Managed: proc.Managed{Module: "frp", Name: "家里", Program: "frpc", PID: 10}},
		{Managed: proc.Managed{Module: "tailscale", Name: "办公室", Program: "tailscaled", PID: 11}},
	}}
	req := func(ctx context.Context) []sysstat.ManagedProcess {
		r := httptest.NewRequest("GET", "/api/status", nil).WithContext(ctx)
		return hostForRequest(r, snap).Managed
	}
	// 只有 status:read 的令牌看不到任何托管进程
	if got := req(auth.WithScopeCheck(context.Background(), func(need string) bool { return need == "status:read" })); len(got) != 0 {
		t.Fatalf("没有模块读权限时不应列出: %+v", got)
	}
	// 有 frp:read 的令牌只看到 frp
	got := req(auth.WithScopeCheck(context.Background(), func(need string) bool { return need == "status:read" || need == "frp:read" }))
	if len(got) != 1 || got[0].Module != "frp" {
		t.Fatalf("应只列出 frp: %+v", got)
	}
	// 没有身份的上下文一律不列出，且结果是空数组（JSON 中为 []）
	if got := req(context.Background()); got == nil || len(got) != 0 {
		t.Fatalf("没有身份时应为空数组: %#v", got)
	}
	// 原快照不被修改
	if len(snap.Managed) != 2 {
		t.Fatal("不应修改原快照")
	}
}
