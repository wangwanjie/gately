package webservice

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

// benchSnapshot 构造 n 个进行中的请求，衡量实时连接列表接口（前端每隔数秒轮询）的生成开销。
func benchSnapshot(b *testing.B, n int, q string) {
	tr := newConnTracker()
	w := httptest.NewRecorder()
	for i := 0; i < n; i++ {
		r := httptest.NewRequest("GET", "http://a.test/some/path/index.html", nil)
		client := netip.AddrPortFrom(netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)}), uint16(1024+i%60000))
		tr.begin(w, r, client)
	}
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := tr.snapshot(q, now)
		if len(v.Active) == 0 || v.Total != n {
			b.Fatalf("结果不符: %d %d", len(v.Active), v.Total)
		}
	}
}

func BenchmarkConnSnapshot20k(b *testing.B)       { benchSnapshot(b, 20000, "") }
func BenchmarkConnSnapshot20kFilter(b *testing.B) { benchSnapshot(b, 20000, "10.0.1") }
