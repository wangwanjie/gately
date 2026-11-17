package sysstat

import (
	"math"
	"os"
	"time"

	"github.com/wangwanjie/gately/internal/proc"
)

// demoWave 返回在 [lo, hi] 之间随时间平滑起伏的值，phase 让不同指标错开。
func demoWave(t time.Time, period, phase, lo, hi float64) float64 {
	x := math.Sin(2*math.Pi*float64(t.Unix())/period + phase)
	return lo + (hi-lo)*(0.5+0.5*x)
}

// demoSnapshot 是演示模式使用的主机信息：与演示数据一致的虚构主机，数值随时间起伏。
func demoSnapshot(now time.Time) Snapshot {
	cpu := demoWave(now, 97, 0, 8, 31)
	load := demoWave(now, 311, 1, 0.35, 0.9)
	const gb = 1 << 30
	memUsed := uint64(demoWave(now, 601, 2, 5.6, 6.4) * gb)
	rx := demoWave(now, 53, 0.5, 40e3, 380e3)
	tx := demoWave(now, 71, 2.1, 25e3, 210e3)
	up := now.Unix() - 12*86400 - 7*3600
	// 累计字节随时间线性增长，起点按开机以来的平均流量估算
	rxTotal := uint64(184e9) + uint64(float64(now.Unix()%86400)*180e3)
	txTotal := uint64(96e9) + uint64(float64(now.Unix()%86400)*110e3)
	f := func(v float64) *float64 { return &v }
	managed := []ManagedProcess{
		{Managed: proc.Managed{Module: "frp", Name: "hk-relay", Program: "frpc", PID: 211, Since: now.Add(-9 * 24 * time.Hour)}, RSS: 31 << 20, CPU: f(demoWave(now, 83, 1, 0.1, 0.9)), Threads: 11},
		{Managed: proc.Managed{Module: "tailscale", Name: "home-net", Program: "tailscaled", PID: 233, Since: now.Add(-9 * 24 * time.Hour)}, RSS: 58 << 20, CPU: f(demoWave(now, 61, 2, 0.2, 1.4)), Threads: 14},
		{Managed: proc.Managed{Module: "netbird", Name: "remote-net", Program: "netbird", PID: 245, Since: now.Add(-6 * 24 * time.Hour), Restarts: 1}, RSS: 47 << 20, CPU: f(demoWave(now, 79, 3, 0.1, 0.7)), Threads: 12},
	}
	return Snapshot{
		Hostname:    "gately-demo",
		OS:          "linux",
		Platform:    "Alpine Linux v3.23",
		Kernel:      "6.12.4-demo",
		Arch:        "amd64",
		InContainer: true,
		BootTime:    up,
		CPU:         CPU{Model: "Intel(R) N100", Cores: 4, Usage: f(cpu), Load: []float64{load, load * 0.93, load * 0.88}},
		Memory:      &Memory{Total: 16 * gb, Used: memUsed, Available: 16*gb - memUsed, SwapTotal: 2 * gb, SwapUsed: 120 << 20},
		Disk:        &Disk{Path: "/etc/gately", Total: 256 * gb, Used: 74 * gb, Free: 182 * gb},
		Interfaces: []Interface{
			{Name: "eth0", MAC: "02:00:5e:00:53:10", MTU: 1500, IPv4: []string{"192.0.2.1/24"}, IPv6: []string{"2001:db8::1/64"}, RxBytes: rxTotal, TxBytes: txTotal, RxRate: f(rx), TxRate: f(tx)},
			{Name: "tailscale0", MTU: 1280, IPv4: []string{"198.51.100.4/32"}, IPv6: []string{"2001:db8:100::4/128"}, RxBytes: 12e9, TxBytes: 8e9, RxRate: f(rx / 9), TxRate: f(tx / 11)},
			{Name: "wt0", MTU: 1280, IPv4: []string{"203.0.113.5/16"}, IPv6: []string{}, RxBytes: 4e9, TxBytes: 3e9, RxRate: f(rx / 20), TxRate: f(tx / 25)},
		},
		Process:   Process{PID: os.Getpid(), RSS: uint64(demoWave(now, 401, 0, 44, 52) * (1 << 20)), CPU: f(demoWave(now, 59, 4, 0.2, 1.8)), Threads: 18, FDs: 36, Goroutines: 74, HeapAlloc: 21 << 20, HeapSys: 38 << 20, NumGC: uint32(now.Unix()/30) % 100000},
		Managed:   managed,
		SampledAt: now,
	}
}
