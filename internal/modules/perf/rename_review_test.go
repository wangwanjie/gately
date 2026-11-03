package perf

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// 保存失败时告警规则回滚到改写前，未命中任何规则时不写盘。
func TestRenameSitesSaveFailureAndNoop(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要非 root 的类 Unix 环境才能用只读目录模拟保存失败")
	}
	h := newHarness(t)
	r := Rule{Name: "慢", Enable: true, Metric: MetricSiteP95, Op: OpGT, Threshold: 500, Duration: 1, Sites: []string{"l1/a", "l2/b"}}
	if code, body := h.call(t, "POST", "/api/perf/rules", r); code != 200 {
		t.Fatalf("新增规则失败: %d %s", code, body)
	}
	dir := h.m.store.Dir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	// 未命中：不应尝试保存（目录只读也不会出错），规则不变
	h.m.renameSites(map[string]string{"l9/z": "l8/z"})
	h.m.renameSites(map[string]string{"l1/a": "l3/a"})
	h.m.mu.Lock()
	got := strings.Join(h.m.cfg.Rules[0].Sites, ",")
	h.m.mu.Unlock()
	if got != "l1/a,l2/b" {
		t.Fatalf("保存失败时规则应回滚: %s", got)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	h.m.renameSites(map[string]string{"l1/a": "l3/a"})
	h.m.mu.Lock()
	got = strings.Join(h.m.cfg.Rules[0].Sites, ",")
	h.m.mu.Unlock()
	if got != "l3/a,l2/b" {
		t.Fatalf("恢复可写后应改写: %s", got)
	}
}
