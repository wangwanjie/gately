package docker

import (
	"net/http"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
)

func TestSummaryCountsContainers(t *testing.T) {
	_, host := newCtrFake(t, map[string]http.HandlerFunc{
		"GET /containers/json": ctrJSON([]map[string]any{
			{"Id": "aaaaaaaaaaaa1", "Names": []string{"/a"}, "State": "running", "Status": "Up 2 days"},
			{"Id": "bbbbbbbbbbbb2", "Names": []string{"/b"}, "State": "running", "Status": "Up 1 hour (unhealthy)"},
			{"Id": "cccccccccccc3", "Names": []string{"/c"}, "State": "exited", "Status": "Exited (0) 3 days ago"},
			{"Id": "dddddddddddd4", "Names": []string{"/d"}, "State": "exited", "Status": "Exited (137) 1 hour ago"},
		}),
	})
	m := newModule(t, host)
	m.updates["aaaaaaaaaaaa1"] = UpdateInfo{HasUpdate: true}
	got := map[string]string{}
	for _, it := range m.Summary() {
		got[it.Label] = it.Value
	}
	if got["运行中"] != "2 / 4" || got["已停止"] != "1" || got["异常"] != "1" || got["健康检查失败"] != "1" || got["有更新"] != "1" {
		t.Errorf("摘要错误: %v", got)
	}
}

func TestSummaryDisconnected(t *testing.T) {
	m := newModule(t, "tcp://127.0.0.1:1")
	items := m.Summary()
	if len(items) != 1 || items[0].Value != "未连接" || items[0].Level != "info" {
		t.Errorf("未连接摘要错误: %v", items)
	}
}

// 健康检查失败的容器仍计入运行中，异常为 0 时不出现「异常 1」（issue #114）
func TestSummaryUnhealthyCountsAsRunning(t *testing.T) {
	_, host := newCtrFake(t, map[string]http.HandlerFunc{
		"GET /containers/json": ctrJSON([]map[string]any{
			{"Id": "aaaaaaaaaaaa1", "Names": []string{"/a"}, "State": "running", "Status": "Up 2 days (healthy)"},
			{"Id": "bbbbbbbbbbbb2", "Names": []string{"/b"}, "State": "running", "Status": "Up 5 minutes (unhealthy)"},
			{"Id": "cccccccccccc3", "Names": []string{"/c"}, "State": "running", "Status": "Up 3 seconds (health: starting)"},
		}),
	})
	m := newModule(t, host)
	got := map[string]module.SummaryItem{}
	for _, it := range m.Summary() {
		got[it.Label] = it
	}
	if got["运行中"].Value != "3 / 3" || got["运行中"].Level != "warn" {
		t.Errorf("运行中错误: %+v", got["运行中"])
	}
	if got["异常"].Value != "0" || got["异常"].Level != "ok" {
		t.Errorf("异常错误: %+v", got["异常"])
	}
	if got["健康检查失败"].Value != "1" || got["健康检查失败"].Level != "warn" {
		t.Errorf("健康检查失败错误: %+v", got["健康检查失败"])
	}
}
