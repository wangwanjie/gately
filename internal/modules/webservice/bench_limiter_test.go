package webservice

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkLimiterManySources 衡量限流器记录了 3 万个近期来源后，每个请求的 acquire 开销
// （分布式扫描或攻击时来源很多，旧实现每次请求都会遍历整张表清理）。
func BenchmarkLimiterManySources(b *testing.B) {
	l := newLimiter(RateLimit{Enable: true, Rate: 1000, Burst: 1000, MaxConns: 100})
	keys := make([]string, 30000)
	for i := range keys {
		keys[i] = fmt.Sprintf("10.%d.%d.%d", i>>16, i>>8&255, i&255)
		if rel, _, ok := l.acquire(keys[i]); ok {
			rel()
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rel, _, ok := l.acquire(keys[i%len(keys)])
		if !ok {
			b.Fatal("不应被限流")
		}
		rel()
	}
}

// TestLimiterEvictsStale 验证来源很多时过期桶仍会被清理，活跃与有进行中请求的桶保留。
func TestLimiterEvictsStale(t *testing.T) {
	l := newLimiter(RateLimit{Enable: true, Rate: 1, Burst: 1, MaxConns: 10})
	now := time.Now()
	l.now = func() time.Time { return now }
	for i := 0; i < 12000; i++ {
		rel, _, ok := l.acquire(fmt.Sprintf("old-%d", i))
		if !ok {
			t.Fatal("首次请求应通过")
		}
		if i != 0 {
			rel()
		}
	}
	now = now.Add(11 * time.Minute)
	if _, _, ok := l.acquire("fresh"); !ok {
		t.Fatal("新来源应通过")
	}
	l.mu.Lock()
	n := len(l.buckets)
	_, busy := l.buckets["old-0"]
	l.mu.Unlock()
	if !busy {
		t.Fatal("有进行中请求的桶不应被清理")
	}
	if n > 10 {
		t.Fatalf("过期的桶应被清理, 剩余 %d", n)
	}
}
