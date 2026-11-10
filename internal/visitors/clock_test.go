package visitors

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// 时钟前跳：记录时最近时间比当前晚超过 1 天就拉回，未来日期的按天计数删除。
func TestRecordResetsFutureTimes(t *testing.T) {
	s, now := newTestStore(t, "")
	a := ip("203.0.113.7")
	future := now.AddDate(0, 0, 10)
	s.Record(Hit{Addr: a, Time: future, Site: "s", Path: "/future", Status: 200})
	// 未超过 1 天的超前保持原样（时钟轻微抖动）
	s.Record(Hit{Addr: a, Time: future.Add(-time.Hour), Site: "s", Path: "/jitter", Status: 200})
	if v, _ := s.Get(a); v.Last != future.Unix() || v.LastReq.Path != "/future" {
		t.Fatalf("1 天内的超前不应重置: %+v", v.LastReq)
	}
	s.Record(Hit{Addr: a, Time: *now, Site: "s", Path: "/now", Status: 200})
	v, _ := s.Get(a)
	if v.Last != now.Unix() || v.LastReq.Time != now.Unix() || v.LastReq.Path != "/now" {
		t.Fatalf("时钟前跳后应拉回当前时间: last=%d lastReq=%+v", v.Last, v.LastReq)
	}
	if v.First > now.Unix() {
		t.Fatalf("首次时间不应晚于当前: %d", v.First)
	}
	if len(v.Days) != 1 || v.Days[0].D != dayOf(*now) {
		t.Fatalf("未来日期的按天计数应删除: %+v", v.Days)
	}
	for _, p := range v.Paths {
		if p.Last > now.Unix() {
			t.Fatalf("路径时间应截到当前: %+v", p)
		}
	}
}

// 读取时未来时间截到当前、未来日期的按天计数丢弃。
func TestLoadClampsFutureTimes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.gz")
	ts := base.Unix()
	fut := ts + 30*86400
	d := dayOf(base)
	writeGz(t, path, fmt.Sprintf(`{"ip":"1.2.3.4","first":%d,"last":%d,"requests":3,"days":[{"d":%d,"req":1},{"d":%d,"req":2}],"sites":[{"key":"s","req":3,"last":%d}],"lastReq":{"time":%d,"path":"/"}}`+"\n",
		fut, fut, d, d+30, fut, fut))
	s, _ := newTestStore(t, path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	v, ok := s.Get(ip("1.2.3.4"))
	if !ok {
		t.Fatal("记录应读回")
	}
	if v.First != ts || v.Last != ts || v.LastReq.Time != ts || v.Sites[0].Last != ts {
		t.Fatalf("未来时间应截到当前: %+v", v)
	}
	if len(v.Days) != 1 || v.Days[0].D != d {
		t.Fatalf("未来日期应丢弃: %+v", v.Days)
	}
}
