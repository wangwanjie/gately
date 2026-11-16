package logbuf

import (
	"fmt"
	"path/filepath"
	"testing"
)

// 缓冲按写入量增长：新建时不占用空间，日志很少的模块不会各自预留上千条的内存；
// 写到上限后才按环形缓冲覆盖最旧的日志，行为与预先分配时相同。
func TestRingGrowsOnDemand(t *testing.T) {
	b := New(2000, nil)
	if cap(b.entries) != 0 {
		t.Fatalf("新建的缓冲不应预先分配，容量 %d", cap(b.entries))
	}
	for i := 1; i <= 10; i++ {
		b.Add(LevelInfo, "", fmt.Sprintf("第 %d 条", i))
	}
	if c := cap(b.entries); c > 16 {
		t.Fatalf("写入 10 条后容量为 %d，应按需增长", c)
	}
	if got := b.Since(0); len(got) != 10 || got[0].Message != "第 1 条" || got[9].Seq != 10 {
		t.Fatalf("内容不对: %+v", got)
	}
	for i := 11; i <= 5000; i++ {
		b.Add(LevelInfo, "", fmt.Sprintf("第 %d 条", i))
		if cap(b.entries) > 2000 {
			t.Fatalf("写入 %d 条后容量 %d 超过上限", i, cap(b.entries))
		}
	}
	got := b.Since(0)
	if len(got) != 2000 || got[0].Seq != 3001 || got[1999].Seq != 5000 || got[1999].Message != "第 5000 条" {
		t.Fatalf("写满后应保留最新的 2000 条: %d 条, %d ~ %d", len(got), got[0].Seq, got[len(got)-1].Seq)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("顺序错乱: %d 之后是 %d", got[i-1].Seq, got[i].Seq)
		}
	}
	if got := b.Range(4990, 4995, 0); len(got) != 4 || got[0].Seq != 4991 {
		t.Fatalf("Range 结果不对: %+v", got)
	}
	// 清空后释放内存，之后仍可继续写
	b.Clear()
	if cap(b.entries) != 0 || len(b.Since(0)) != 0 {
		t.Fatalf("清空后应释放缓冲，容量 %d", cap(b.entries))
	}
	b.Add(LevelInfo, "", "清空后")
	if got := b.Since(0); len(got) != 1 || got[0].Seq != 5001 {
		t.Fatalf("清空后序号应继续递增: %+v", got)
	}
}

// 调整容量：缩小时保留最新的日志，放大后可以继续写到新的上限；写满（已环绕）与未写满两种状态都要正确。
func TestRingResize(t *testing.T) {
	for _, n := range []int{3, 8, 25} {
		b := New(8, nil)
		for i := 1; i <= n; i++ {
			b.Add(LevelInfo, "", fmt.Sprintf("%d", i))
		}
		b.Resize(5)
		got := b.Since(0)
		want := min(n, 5)
		if len(got) != want || got[len(got)-1].Seq != uint64(n) || b.Cap() != 5 {
			t.Fatalf("n=%d 缩小后得到 %d 条（应为 %d），最后一条 %d", n, len(got), want, got[len(got)-1].Seq)
		}
		for i := n + 1; i <= n+4; i++ {
			b.Add(LevelInfo, "", fmt.Sprintf("%d", i))
		}
		if got := b.Since(0); len(got) != 5 || got[0].Seq != uint64(n) || got[4].Seq != uint64(n+4) {
			t.Fatalf("n=%d 缩小后继续写入结果不对: %+v", n, got)
		}
		b.Resize(12)
		for i := n + 5; i <= n+20; i++ {
			b.Add(LevelInfo, "", fmt.Sprintf("%d", i))
		}
		got = b.Since(0)
		if len(got) != 12 || got[0].Seq != uint64(n+9) || got[11].Seq != uint64(n+20) || cap(b.entries) > 12 {
			t.Fatalf("n=%d 放大后结果不对: %d 条，%d ~ %d，容量 %d", n, len(got), got[0].Seq, got[len(got)-1].Seq, cap(b.entries))
		}
		for i := 1; i < len(got); i++ {
			if got[i].Seq != got[i-1].Seq+1 || got[i].Message != fmt.Sprint(got[i].Seq) {
				t.Fatalf("n=%d 放大后顺序错乱: %+v", n, got)
			}
		}
	}
}

// 重启恢复：只为恢复出来的日志分配空间，之后照常增长到上限并环绕。
func TestRingRestoreThenGrow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.log")
	b := New(100, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 30; i++ {
		b.Add(LevelInfo, "s", fmt.Sprintf("%d", i))
	}
	b.ClosePersist()

	b2 := New(40, nil)
	if err := b2.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	if got := b2.Since(0); len(got) != 30 || cap(b2.entries) != 30 {
		t.Fatalf("恢复 %d 条，容量 %d", len(got), cap(b2.entries))
	}
	for i := 31; i <= 55; i++ {
		b2.Add(LevelInfo, "s", fmt.Sprintf("%d", i))
	}
	got := b2.Since(0)
	if len(got) != 40 || got[0].Seq != 16 || got[39].Seq != 55 || cap(b2.entries) > 40 {
		t.Fatalf("恢复后继续写入结果不对: %d 条，%d ~ %d，容量 %d", len(got), got[0].Seq, got[len(got)-1].Seq, cap(b2.entries))
	}
	b2.ClosePersist()
	// 容量比文件里的日志少时只恢复最新的那些
	b4 := New(7, nil)
	if err := b4.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	defer b4.ClosePersist()
	if got := b4.Since(0); len(got) != 7 || got[6].Message != "55" || got[0].Message != "49" {
		t.Fatalf("容量较小时恢复结果不对: %+v", got)
	}
}
