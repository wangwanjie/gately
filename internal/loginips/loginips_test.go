package loginips

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

func TestFirstLoginOnlyBuildsBaseline(t *testing.T) {
	tr := New(nil, nil)
	if tr.Observe("1.2.3.4", t0) {
		t.Fatal("首次登录只建立基线，不应告警")
	}
	if tr.Observe("1.2.3.4", t0.Add(time.Hour)) {
		t.Fatal("已知地址不应告警")
	}
	if !tr.Observe("5.6.7.8", t0.Add(2*time.Hour)) {
		t.Fatal("新地址应告警")
	}
	if tr.Observe("5.6.7.8", t0.Add(3*time.Hour)) {
		t.Fatal("同一新地址只告警一次")
	}
}

func TestIPv6GroupedBy64AndMappedV4(t *testing.T) {
	tr := New(nil, nil)
	tr.Observe("2001:db8:1:2::1", t0)
	if tr.Observe("2001:db8:1:2:aaaa:bbbb:cccc:dddd", t0) {
		t.Fatal("同一 /64 的临时地址不应告警")
	}
	if !tr.Observe("2001:db8:1:3::1", t0) {
		t.Fatal("不同 /64 应告警")
	}
	tr.Observe("9.9.9.9", t0)
	if tr.Observe("::ffff:9.9.9.9", t0) {
		t.Fatal("IPv4 映射地址应与 IPv4 视为同一来源")
	}
}

func TestInvalidIPIgnored(t *testing.T) {
	tr := New(nil, nil)
	for _, s := range []string{"", "unknown", "1.2.3", "1.2.3.4:80"} {
		if tr.Observe(s, t0) {
			t.Fatalf("%q 不应告警", s)
		}
	}
	if len(tr.Known()) != 0 {
		t.Fatal("无效地址不应记录")
	}
}

func TestPersistOnlyOnNewAndRestore(t *testing.T) {
	var saved []Entry
	saves := 0
	tr := New(nil, func(e []Entry) error { saves++; saved = e; return nil })
	tr.Observe("1.1.1.1", t0)
	tr.Observe("1.1.1.1", t0.Add(time.Minute))
	if saves != 1 {
		t.Fatalf("已知地址不应写盘: %d", saves)
	}
	tr2 := New(saved, nil)
	if tr2.Observe("1.1.1.1", t0) {
		t.Fatal("重启后已知地址不应告警")
	}
	if !tr2.Observe("2.2.2.2", t0) {
		t.Fatal("重启后新地址应告警")
	}
}

func TestSaveErrorKeepsMemory(t *testing.T) {
	tr := New(nil, func([]Entry) error { return fmt.Errorf("disk full") })
	tr.Observe("1.1.1.1", t0)
	if tr.Observe("1.1.1.1", t0) || len(tr.Known()) != 1 {
		t.Fatal("写盘失败不应影响内存记录")
	}
}

func TestCapEvictsOldest(t *testing.T) {
	tr := New(nil, nil)
	for i := 0; i < MaxEntries+50; i++ {
		tr.Observe(fmt.Sprintf("10.%d.%d.1", i/250, i%250), t0.Add(time.Duration(i)*time.Second))
	}
	got := tr.Known()
	if len(got) != MaxEntries {
		t.Fatalf("应封顶 %d: %d", MaxEntries, len(got))
	}
	if !tr.Observe("10.0.0.1", t0.Add(time.Hour)) {
		t.Fatal("最旧的地址应已被淘汰")
	}
}

func TestLoadTruncatesOversized(t *testing.T) {
	var in []Entry
	for i := 0; i < MaxEntries*3; i++ {
		in = append(in, Entry{Key: fmt.Sprintf("10.1.%d.%d", i/250, i%250), Last: t0})
	}
	in = append(in, Entry{})
	if n := len(New(in, nil).Known()); n != MaxEntries {
		t.Fatalf("加载时应截断: %d", n)
	}
}

func TestConcurrent(t *testing.T) {
	tr := New(nil, func([]Entry) error { return nil })
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.Observe(fmt.Sprintf("8.8.%d.%d", i, i), t0)
			tr.Known()
		}(i)
	}
	wg.Wait()
}
