package visitors

import (
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 查询的国家回调阻塞时，请求的记录不应被挡住（回调在锁外调用）。
func TestQueryCallbacksDoNotBlockRecord(t *testing.T) {
	s, now := newTestStore(t, "")
	seed(s, *now)
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Query(Query{Country: func(netip.Addr) string {
			entered <- struct{}{}
			<-release
			return ""
		}})
	}()
	<-entered
	recorded := make(chan struct{})
	go func() {
		s.Record(Hit{Addr: ip("198.51.100.1"), Time: *now, Site: "s", Path: "/"})
		s.Detail(ip("203.0.113.1"), nil, nil)
		close(recorded)
	}()
	select {
	case <-recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("查询回调执行期间记录被阻塞")
	}
	close(release)
	<-done
}

// 查询串行执行：同一时刻只有一个查询在调用回调。
func TestQuerySerialized(t *testing.T) {
	s, now := newTestStore(t, "")
	seed(s, *now)
	var cur, peak atomic.Int32
	country := func(netip.Addr) string {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		cur.Add(-1)
		return ""
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Query(Query{Country: country})
		}()
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("并发查询数 = %d，应为 1", peak.Load())
	}
}

// 分批持锁复制时，记录数超过一批也能查全，且结果与筛选一致。
func TestQueryAcrossChunks(t *testing.T) {
	s, now := newTestStore(t, "")
	n := queryChunk*2 + 7
	for i := 0; i < n; i++ {
		h := Hit{Addr: ip(fmt.Sprintf("10.%d.%d.%d", i/65536, i/256%256, i%256)), Time: *now, Listener: "l", Site: "s", Path: "/"}
		if i%3 == 0 {
			h.User = "u"
		}
		s.Record(h)
	}
	r := s.Query(Query{Limit: MaxLimit})
	if r.Total != n || r.Summary.Visitors != n || len(r.Sites) != 1 || r.Sites[0].Req != uint64(n) {
		t.Fatalf("分批查询不完整: total=%d sites=%+v", r.Total, r.Sites)
	}
	if r := s.Query(Query{Filter: FilterAuthed}); r.Total != (n+2)/3 {
		t.Fatalf("认证筛选 = %d", r.Total)
	}
}
