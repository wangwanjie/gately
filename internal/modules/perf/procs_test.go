package perf

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"

	"github.com/wangwanjie/gately/internal/proc"
	"github.com/wangwanjie/gately/internal/sysstat"
)

func fp(v float64) *float64 { return &v }

func managedOf(module, name, program string, pid int, cpu *float64, rssMB uint64) sysstat.ManagedProcess {
	return sysstat.ManagedProcess{Managed: proc.Managed{Module: module, Name: name, Program: program, PID: pid, Since: t0}, CPU: cpu, RSS: rssMB << 20, Threads: 3}
}

// 托管进程合计只累加取得到的值；一个都取不到或没有托管进程时为 unknown
func TestToHostSampleChildTotals(t *testing.T) {
	s := toHostSample(sysstat.Snapshot{})
	if s.ChildCPU != unknown || s.ChildRSSMB != unknown {
		t.Fatalf("没有托管进程时合计应为 unknown: %+v", s)
	}
	s = toHostSample(sysstat.Snapshot{Managed: []sysstat.ManagedProcess{
		managedOf("frp", "a", "frpc", 10, fp(1.5), 30),
		managedOf("frp", "b", "frps", 11, nil, 20),
		managedOf("tailscale", "c", "tailscaled", 12, fp(0.5), 0),
	}})
	if s.ChildCPU != 2 || s.ChildRSSMB != 50 {
		t.Fatalf("合计不符: cpu=%v rss=%v", s.ChildCPU, s.ChildRSSMB)
	}
	s = toHostSample(sysstat.Snapshot{Managed: []sysstat.ManagedProcess{managedOf("frp", "a", "frpc", 10, nil, 0)}})
	if s.ChildCPU != unknown || s.ChildRSSMB != unknown {
		t.Fatalf("取不到任何值时应为 unknown: %+v", s)
	}
}

// 分钟聚合与 24 小时合并带上托管进程合计，峰值取最大值
func TestHistoryChildFields(t *testing.T) {
	var a minuteAgg
	a.add(hostSample{CPU: 1, ChildCPU: 2, ChildRSSMB: 100})
	a.add(hostSample{CPU: 1, ChildCPU: 6, ChildRSSMB: 200})
	a.add(hostSample{CPU: 1, ChildCPU: unknown, ChildRSSMB: unknown})
	p := a.point()
	if p.ChildCPU != 4 || p.ChildCPUMax != 6 || p.ChildRSSMB != 150 {
		t.Fatalf("分钟聚合不符: %+v", p)
	}
	var empty minuteAgg
	empty.add(hostSample{CPU: 1, ChildCPU: unknown, ChildRSSMB: unknown})
	if q := empty.point(); q.ChildCPU != unknown || q.ChildCPUMax != unknown || q.ChildRSSMB != unknown {
		t.Fatalf("没有托管进程的分钟应为 unknown: %+v", q)
	}
	m := merge([]hostPoint{
		{T: 0, ChildCPU: 2, ChildCPUMax: 3, ChildRSSMB: 100},
		{T: 60, ChildCPU: 4, ChildCPUMax: 9, ChildRSSMB: 300},
		{T: 120, ChildCPU: unknown, ChildCPUMax: unknown, ChildRSSMB: unknown},
	}, 5)
	if len(m) != 1 || m[0].ChildCPU != 3 || m[0].ChildCPUMax != 9 || m[0].ChildRSSMB != 200 {
		t.Fatalf("合并不符: %+v", m)
	}
}

// 按实例跟踪：重启（PID 变化）沿用采样，消失的进程移除，最多保留 60 秒
func TestProcTracker(t *testing.T) {
	var tr procTracker
	if v := tr.views(); v == nil || len(v) != 0 {
		t.Fatalf("尚未采样时应为空数组: %#v", v)
	}
	now := t0
	for i := 0; i < shortSlots+3; i++ {
		tr.add(now, []sysstat.ManagedProcess{
			managedOf("frp", "a", "frpc", 10, fp(float64(i)), 30),
			managedOf("tailscale", "c", "tailscaled", 12, nil, 0),
		})
		now = now.Add(SampleInterval)
	}
	v := tr.views()
	if len(v) != 2 || v[0].Program != "frpc" || len(v[0].Samples) != shortSlots {
		t.Fatalf("视图不符: %+v", v)
	}
	if *v[0].Samples[0].CPU != 3 || *v[0].CPU != float64(shortSlots+2) || v[0].RSS != 30<<20 || v[0].Threads != 3 {
		t.Fatalf("应只保留最近的采样: first=%v cur=%v", *v[0].Samples[0].CPU, *v[0].CPU)
	}
	if v[1].CPU != nil || v[1].Samples[0].CPU != nil || v[1].Samples[0].RSS != nil {
		t.Fatalf("取不到的值应为 null: %+v", v[1])
	}
	// frpc 重启后 PID 变化，采样延续；tailscaled 停止后移除
	tr.add(now, []sysstat.ManagedProcess{managedOf("frp", "a", "frpc", 99, fp(1), 31)})
	v = tr.views()
	if len(v) != 1 || v[0].PID != 99 || len(v[0].Samples) != shortSlots {
		t.Fatalf("重启后应延续采样、移除已停止的进程: %+v", v)
	}
	// 视图是副本，外部修改不影响内部状态
	v[0].Samples[0].Time = -1
	if tr.views()[0].Samples[0].Time == -1 {
		t.Fatal("视图应为副本")
	}
	tr.add(now, nil)
	if len(tr.views()) != 0 {
		t.Fatal("没有托管进程时应清空")
	}
}

// 进程退出到重启之间恰好错过一次采样时，重启后的曲线仍然延续；错过太久则从头开始
func TestProcTrackerRestartGap(t *testing.T) {
	var tr procTracker
	now := t0
	for i := 0; i < 5; i++ {
		tr.add(now, []sysstat.ManagedProcess{managedOf("frp", "a", "frpc", 10, fp(1), 30)})
		now = now.Add(SampleInterval)
	}
	// 错过一次采样：视图里暂时没有该进程
	tr.add(now, nil)
	now = now.Add(SampleInterval)
	if len(tr.views()) != 0 {
		t.Fatal("消失的进程不应出现在视图中")
	}
	tr.add(now, []sysstat.ManagedProcess{managedOf("frp", "a", "frpc", 11, fp(2), 30)})
	if v := tr.views(); len(v) != 1 || v[0].PID != 11 || len(v[0].Samples) != 6 {
		t.Fatalf("错过一次采样后应延续原有 5 个点再加 1 个: %+v", v)
	}
	// 消失超过保留时间则重新开始
	tr.add(now.Add(SampleInterval), nil)
	tr.add(now.Add(goneGrace+SampleInterval*2), []sysstat.ManagedProcess{managedOf("frp", "a", "frpc", 12, fp(3), 30)})
	if v := tr.views(); len(v) != 1 || len(v[0].Samples) != 1 {
		t.Fatalf("消失过久应重新开始: %+v", v)
	}
	if len(tr.gone) != 0 {
		t.Fatalf("过期的暂存应被清除: %d", len(tr.gone))
	}
}

// asScopes 以指定权限的 API 令牌身份请求接口
func (h *harness) asScopes(t *testing.T, path string, allow func(string) bool) []ProcessView {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if allow != nil {
		req = req.WithContext(auth.WithScopeCheck(context.Background(), allow))
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("状态码 %d: %s", rec.Code, rec.Body.String())
	}
	var out []ProcessView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out == nil {
		t.Fatalf("应返回数组: %v %s", err, rec.Body.String())
	}
	return out
}

// 进程带有各模块的实例名称，只列出请求者有读权限的模块
func TestProcessesAPIScoped(t *testing.T) {
	h := newHarness(t)
	h.host.managed = []sysstat.ManagedProcess{
		managedOf("frp", "家里", "frpc", 10, fp(2), 30),
		managedOf("filebrowser", "文件", "filebrowser", 11, fp(1), 70),
	}
	h.step(1)
	all := func(string) bool { return true }
	if got := h.asScopes(t, "/api/perf/processes", all); len(got) != 2 {
		t.Fatalf("有全部权限时应列出全部: %+v", got)
	}
	only := func(need string) bool { return need == "perf:read" || need == "filebrowser:read" }
	got := h.asScopes(t, "/api/perf/processes", only)
	if len(got) != 1 || got[0].Module != "filebrowser" {
		t.Fatalf("只应列出有读权限的模块: %+v", got)
	}
	// 写权限包含读权限由令牌的判定函数负责；这里只要求读
	asked := map[string]bool{}
	h.asScopes(t, "/api/perf/processes", func(need string) bool { asked[need] = true; return false })
	if !asked["frp:read"] || !asked["filebrowser:read"] || len(asked) != 2 {
		t.Fatalf("应按模块读权限判定: %v", asked)
	}
	// 没有身份的上下文一律不列出
	if got := h.asScopes(t, "/api/perf/processes", nil); len(got) != 0 {
		t.Fatalf("没有身份时不应列出: %+v", got)
	}
}

// 接口：/processes 列出托管进程；/overview 带托管进程合计；输出不含路径与参数
func TestProcessesAPI(t *testing.T) {
	h := newHarness(t)
	allowAll := func(string) bool { return true }
	if got := h.asScopes(t, "/api/perf/processes", allowAll); len(got) != 0 {
		t.Fatalf("尚未采样时应返回空数组: %+v", got)
	}
	_, body := h.call(t, "GET", "/api/perf/processes", nil)
	if strings.TrimSpace(string(body)) == "" || !strings.Contains(string(body), "[]") {
		t.Fatalf("尚未采样时应返回空数组: %s", body)
	}
	h.host.managed = []sysstat.ManagedProcess{
		managedOf("frp", "家里", "frpc", 10, fp(2), 30),
		managedOf("filebrowser", "文件", "filebrowser", 11, fp(1), 70),
	}
	h.step(3)
	list := h.asScopes(t, "/api/perf/processes", allowAll)
	if len(list) != 2 {
		t.Fatalf("应列出两个进程: %+v", list)
	}
	raw, _ := json.Marshal(list)
	body = raw
	if list[0].Module != "frp" || list[0].Name != "家里" || list[0].PID != 10 || len(list[0].Samples) != 3 || *list[0].Samples[2].RSS != 30 {
		t.Fatalf("进程视图不符: %+v", list[0])
	}
	for _, bad := range []string{"/etc/", "-c", "args", "env", "path"} {
		if strings.Contains(string(body), bad) {
			t.Fatalf("输出不应含路径、参数或环境变量（%s）: %s", bad, body)
		}
	}
	_, body = h.call(t, "GET", "/api/perf/overview?range=60s", nil)
	var o Overview
	if err := json.Unmarshal(body, &o); err != nil || o.Latest == nil || o.Latest.ChildCPU == nil || *o.Latest.ChildCPU != 3 || *o.Latest.ChildRSS != 100 {
		t.Fatalf("最新点应带托管进程合计: %v %s", err, body)
	}
	h.host.managed = nil
	h.now = h.now.Add(2 * time.Minute)
	h.step(1)
	_, body = h.call(t, "GET", "/api/perf/overview?range=60s", nil)
	_ = json.Unmarshal(body, &o)
	if o.Latest.ChildCPU != nil || o.Latest.ChildRSS != nil {
		t.Fatalf("没有托管进程时合计应为 null: %s", body)
	}
}
