package banlist

import (
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fillStore 向封禁表写入 n 个单地址封禁，另有 nets 个手动网段（用 Import 一次写入，避免准备数据本身很慢）。
func fillStore(b testing.TB, n, nets int) *Store {
	s := New()
	now := time.Now()
	entries := make([]Entry, 0, n+nets)
	for i := 0; i < n; i++ {
		a := netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})
		entries = append(entries, Entry{Key: KeyFor(a), Until: now.Add(time.Hour), Created: now, Source: SourceScan, Count: 1})
	}
	for i := 0; i < nets; i++ {
		entries = append(entries, Entry{Key: fmt.Sprintf("172.%d.0.0/16", 16+i), Until: now.Add(time.Hour), Created: now, Source: SourceManual, Count: 1})
	}
	s.Import(entries, nil)
	return s
}

// BenchmarkBan50k 衡量封禁表已有 5 万条记录时再封禁一个新地址的开销（扫描器洪水下每秒会发生很多次）。
func BenchmarkBan50k(b *testing.B) {
	s := fillStore(b, 50000, 5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := netip.AddrFrom4([4]byte{192, byte(i >> 16), byte(i >> 8), byte(i)})
		if _, _, err := s.Ban(KeyFor(a), Options{Duration: time.Hour, Source: SourceScan, Force: true}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBannedDuringBans 衡量持续有新封禁写入时，其它协程查询 Banned（每个入站连接都会查）的耗时。
func BenchmarkBannedDuringBans(b *testing.B) {
	s := fillStore(b, 50000, 5)
	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load(); i++ {
			a := netip.AddrFrom4([4]byte{192, byte(i >> 16), byte(i >> 8), byte(i)})
			_, _, _ = s.Ban(KeyFor(a), Options{Duration: time.Hour, Source: SourceScan, Force: true})
		}
	}()
	probe := netip.MustParseAddr("203.0.113.9")
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Banned(probe)
		}
	})
	b.StopTimer()
	stop.Store(true)
	wg.Wait()
}

// TestNetsTracking 验证网段封禁在增删、到期清除与导入后仍然生效，且单地址封禁不影响网段列表。
func TestNetsTracking(t *testing.T) {
	s := fillStore(t, 1000, 2)
	in := netip.MustParseAddr("172.17.5.5")
	if !s.Banned(in) {
		t.Fatal("导入的网段应当生效")
	}
	if _, _, err := s.Ban("198.51.100.0/24", Options{Duration: time.Hour, Force: true}); err != nil {
		t.Fatal(err)
	}
	if !s.Banned(netip.MustParseAddr("198.51.100.77")) {
		t.Fatal("新封禁的网段应当生效")
	}
	if _, _, err := s.Ban("203.0.113.1", Options{Duration: time.Hour, Force: true}); err != nil {
		t.Fatal(err)
	}
	if len(s.nets) != 3 {
		t.Fatalf("单地址封禁不应改变网段列表: %d", len(s.nets))
	}
	if !s.Unban("198.51.100.0/24") || s.Banned(netip.MustParseAddr("198.51.100.77")) {
		t.Fatal("解封后网段应当失效")
	}
	// 短时封禁的网段到期后被清除
	if _, _, err := s.Ban("192.0.2.0/24", Options{Duration: time.Millisecond, Force: true}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	s.Prune()
	if s.Banned(netip.MustParseAddr("192.0.2.9")) || len(s.nets) != 2 {
		t.Fatalf("到期的网段应被清除: %d", len(s.nets))
	}
}
