package visitors

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// 写盘为快照后流式编码：数据量较大时读回与内存中一致，写盘期间并发记录不出错。
func TestSaveSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.gz")
	s, now := newTestStore(t, path)
	for i := 0; i < 2000; i++ {
		s.Record(Hit{Addr: ip(fmt.Sprintf("10.%d.%d.%d", i/65536, i/256%256, i%256)), Time: *now, Site: "s", Path: fmt.Sprint("/", i%13), UA: "ua", Status: 200})
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s.Record(Hit{Addr: ip("10.0.0.1"), Time: *now, Site: "s", Path: "/x"})
		}
	}()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	close(stop)
	wg.Wait()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	want, _ := s.Get(ip("10.0.7.207"))
	s2, _ := newTestStore(t, path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if s2.Len() != 2000 {
		t.Fatalf("读回 %d 条", s2.Len())
	}
	got, _ := s2.Get(ip("10.0.7.207"))
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("读回的记录不一致:\n%+v\n%+v", got, want)
	}
	a, _ := s.Get(ip("10.0.0.1"))
	b, _ := s2.Get(ip("10.0.0.1"))
	if a.Requests != b.Requests {
		t.Fatalf("并发记录后读回请求数 %d != %d", b.Requests, a.Requests)
	}
}
