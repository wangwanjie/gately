package logbuf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "ddns.log")
	b := New(10, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		b.Logger("r").Infof("第 %d 条", i)
	}
	b.Add(LevelDebug, "", "调试日志")
	b.ClosePersist()

	// 模拟重启：新缓冲从文件恢复，序号接着递增
	b2 := New(3, nil)
	if err := b2.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	defer b2.ClosePersist()
	got := b2.Since(0)
	if len(got) != 3 || got[0].Message != "第 3 条" || got[1].Source != "r" || got[2].Level != LevelDebug {
		t.Fatalf("恢复结果不对: %+v", got)
	}
	b2.Add(LevelInfo, "", "重启后")
	last := b2.Since(0)
	if n := last[len(last)-1].Seq; n != 7 {
		t.Fatalf("重启后序号应接着最大序号递增，得到 %d", n)
	}
}

func TestPersistRotateAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.log")
	b := New(100, nil)
	if err := b.Persist(path, 200); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		b.Add(LevelInfo, "", strings.Repeat("x", 50))
		// 每条单独写盘，触发多次轮转
		b.sink.flush()
	}
	for _, p := range []string{path, path + ".1"} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() > 200 {
			t.Fatalf("%s 超过上限: %d", p, st.Size())
		}
	}
	if got := readTail(path, 100); len(got) < 2 || got[len(got)-1].Seq != 20 {
		t.Fatalf("应从当前文件与历史文件按顺序恢复: %+v", got)
	}

	b.Clear()
	b.ClosePersist()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("清空后日志文件应删除: %v", err)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("清空后历史文件应删除: %v", err)
	}
}

func TestTailEntriesSkipsBrokenLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	data := `{"seq":1,"time":"2026-01-01T00:00:00Z","level":"info","message":"a"}
not json
{"seq":2,"time":"2026-01-01T00:00:01Z","level":"warn","message":"b"}
{"seq":3,"time":"2026-01-01T00:00:02Z","lev`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got := tailEntries(path, 10)
	if len(got) != 2 || got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("应跳过损坏的行: %+v", got)
	}
}

func TestTailEntriesLargeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	b := New(5000, nil)
	if err := b.Persist(path, 64<<20); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		b.Add(LevelInfo, "src", strings.Repeat("y", 100))
	}
	b.ClosePersist()
	got := tailEntries(path, 1000)
	if len(got) != 1000 || got[0].Seq != 4001 || got[999].Seq != 5000 {
		t.Fatalf("跨块读取结果不对: len=%d first=%d", len(got), got[0].Seq)
	}
}

// TestFlushEarlyWhenPendingLarge 待写数据超过阈值时立即写盘，不等满 flushInterval。
func TestFlushEarlyWhenPendingLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.log")
	b := New(10, nil)
	if err := b.Persist(path, 64<<20); err != nil {
		t.Fatal(err)
	}
	defer b.ClosePersist()
	line := strings.Repeat("q", 1000)
	for i := 0; i < flushEarlyBytes/1000+10; i++ {
		b.Add(LevelInfo, "", line)
	}
	deadline := time.Now().Add(flushInterval / 2)
	for time.Now().Before(deadline) {
		if st, err := os.Stat(path); err == nil && st.Size() >= flushEarlyBytes {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("待写数据超过 %d 字节后应立即写盘", flushEarlyBytes)
}
