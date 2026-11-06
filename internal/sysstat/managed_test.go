package sysstat

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/proc"
)

func TestParseStat(t *testing.T) {
	// 进程名含空格与括号；ppid=42，utime=150，stime=50，num_threads=7，rss=300 页
	line := "1234 (my (odd) proc) S 42 1234 1234 0 -1 4194560 100 0 0 0 150 50 0 0 20 0 7 0 5000 123456789 300 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0\n"
	r, ppid, ok := parseStat(line, 4096)
	if !ok || ppid != 42 || r.ticks != 200 || r.threads != 7 || r.rss != 300*4096 {
		t.Fatalf("解析结果不符: %+v ppid=%d ok=%v", r, ppid, ok)
	}
	for _, bad := range []string{"", "1234 (x) S 1 2", "1234 no-paren S 42", "1 (x) S abc 1 1 0 -1 0 0 0 0 0 1 1 0 0 20 0 1 0 0 0 1"} {
		if _, _, ok := parseStat(bad, 4096); ok {
			t.Errorf("格式不完整应解析失败: %q", bad)
		}
	}
}

func TestParseCPUTime(t *testing.T) {
	cases := map[string]float64{
		"0:00.03":     0.03,
		"12:34.50":    12*60 + 34.5,
		"1:02:03":     3723,
		"2-01:00:00":  2*86400 + 3600,
		"123:45.67":   123*60 + 45.67,
		"00:00:01.50": 1.5,
	}
	for in, want := range cases {
		got, ok := parseCPUTime(in)
		if !ok || got < want-0.001 || got > want+0.001 {
			t.Errorf("parseCPUTime(%q) = %v %v，期望 %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "12", "a:b", "1:2:3:4", "x-1:00", "-1:00"} {
		if _, ok := parseCPUTime(bad); ok {
			t.Errorf("parseCPUTime(%q) 应失败", bad)
		}
	}
}

func TestParsePS(t *testing.T) {
	out := "  101   50  2048   0:01.50\n  102   99  4096   0:02.00\n  bad line\n  103   50  1024   1:00:00\n"
	got := parsePS(out, 50)
	if len(got) != 2 {
		t.Fatalf("只应保留父进程为自身的两行: %+v", got)
	}
	if r := got[101]; r.rss != 2048*1024 || r.ticks != 150 {
		t.Fatalf("101 解析不符: %+v", r)
	}
	if r := got[103]; r.ticks != 3600*clockTicks {
		t.Fatalf("103 解析不符: %+v", r)
	}
	if _, ok := got[102]; ok {
		t.Fatal("父进程不是自身的 PID 不应计入")
	}
}

func TestManagedStats(t *testing.T) {
	since := time.Unix(1000, 0)
	list := []proc.Managed{
		{Module: "frp", Name: "a", Program: "frpc", PID: 10, Since: since},
		{Module: "frp", Name: "b", Program: "frps", PID: 11, Since: since},
		{Module: "tailscale", Name: "c", Program: "tailscaled", PID: 12, Since: since},
	}
	t0 := time.Unix(2000, 0)
	prev := rawSample{at: t0, children: map[int]childRaw{
		10: {since: since.UnixNano(), ticks: 100, rss: 1 << 20},
		// 11 在上一次采样时是另一个进程（重启后 PID 复用）
		11: {since: since.Add(-time.Hour).UnixNano(), ticks: 0},
	}}
	cur := rawSample{at: t0.Add(2 * time.Second), children: map[int]childRaw{
		10: {since: since.UnixNano(), ticks: 180, rss: 8 << 20, threads: 5},
		11: {since: since.UnixNano(), ticks: 500, rss: 2 << 20},
	}}
	got := managedStats(list, cur, prev, 4)
	if len(got) != 3 {
		t.Fatalf("应列出全部托管进程: %+v", got)
	}
	// 80 个时钟周期 = 0.8 秒 CPU，2 秒内、4 核：0.8/2/4 = 10%
	if got[0].CPU == nil || *got[0].CPU < 9.99 || *got[0].CPU > 10.01 || got[0].RSS != 8<<20 || got[0].Threads != 5 {
		t.Fatalf("frpc 占用不符: %+v cpu=%v", got[0], got[0].CPU)
	}
	if got[1].CPU != nil || got[1].RSS != 2<<20 {
		t.Fatalf("启动时间变化后不应计算 CPU: %+v", got[1])
	}
	if got[2].CPU != nil || got[2].RSS != 0 || got[2].Program != "tailscaled" {
		t.Fatalf("读不到计数器时只列出进程: %+v", got[2])
	}
	// 计数器回退（异常）时不计算
	cur.children[10] = childRaw{since: since.UnixNano(), ticks: 50}
	if g := managedStats(list, cur, prev, 4); g[0].CPU != nil {
		t.Fatal("CPU 时间回退时不应计算")
	}
	// 没有托管进程时返回空数组（JSON 中为 []）
	if g := managedStats(nil, cur, prev, 4); g == nil || len(g) != 0 {
		t.Fatalf("应返回空数组: %#v", g)
	}
}

// 真实启动一个托管进程，采集结果应包含它；Linux 与 macOS 上能读到常驻内存
func TestSnapshotManaged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sleep 命令")
	}
	buf := logbuf.New(50, nil)
	s := proc.New(buf.Logger("t"))
	s.Start(proc.Spec{Path: "sleep", Args: []string{"60"}, Module: "frp", Name: "测试"})
	defer s.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for !s.State().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	pid := s.State().PID
	c := New(os.TempDir())
	c.Snapshot()
	time.Sleep(400 * time.Millisecond)
	snap := c.Snapshot()
	var found *ManagedProcess
	for i := range snap.Managed {
		if snap.Managed[i].PID == pid {
			found = &snap.Managed[i]
		}
	}
	if found == nil {
		t.Fatalf("快照应包含托管进程 %d: %+v", pid, snap.Managed)
	}
	if found.Module != "frp" || found.Name != "测试" || found.Program != "sleep" {
		t.Fatalf("归属信息不符: %+v", found)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if found.RSS == 0 {
			t.Fatalf("应读到常驻内存: %+v", found)
		}
		// macOS 读不到主机计数器，托管进程的 CPU 靠自己的基准计算，同样应有值
		if found.CPU == nil {
			t.Fatalf("两次采样后应有 CPU: %+v", found)
		}
	}
	s.Stop()
	for _, m := range c.Snapshot().Managed {
		if m.PID == pid {
			t.Fatal("停止后不应再列出")
		}
	}
}
