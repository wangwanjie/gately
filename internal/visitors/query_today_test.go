package visitors

import (
	"testing"
	"time"
)

// “今天”范围的新访客按今天本地 0 点算：昨天 23 点首次出现的不算今天的新访客。
func TestQueryNewFromLocalMidnight(t *testing.T) {
	s, now := newTestStore(t, "")
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	s.Record(Hit{Addr: ip("192.0.2.1"), Time: midnight.Add(-time.Hour), Site: "s", Path: "/"})
	s.Record(Hit{Addr: ip("192.0.2.1"), Time: *now, Site: "s", Path: "/"})
	s.Record(Hit{Addr: ip("192.0.2.2"), Time: midnight.Add(time.Hour), Site: "s", Path: "/"})
	r := s.Query(Query{Days: 1})
	if r.Summary.Visitors != 2 || r.Summary.New != 1 {
		t.Fatalf("今天的新访客 = %+v", r.Summary)
	}
	if r := s.Query(Query{Days: 2}); r.Summary.New != 2 {
		t.Fatalf("两天内的新访客 = %+v", r.Summary)
	}
}
