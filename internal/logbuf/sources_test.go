package logbuf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSourcesTrackedPersistedAndCleared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frp.log")
	b := New(2, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	b.Add(LevelInfo, "b", "x")
	b.Add(LevelInfo, "", "无来源")
	b.Add(LevelInfo, "a", "y")
	b.Add(LevelInfo, "c", "z")
	b.Add(LevelInfo, "a", "w")
	if got := fmt.Sprint(b.Sources()); got != "[a b c]" {
		t.Fatalf("来源应排序去重且不含空串，得到 %s", got)
	}
	b.ClosePersist()

	// 缓冲只有 2 条，恢复的日志里没有 b，但来源记录文件里有
	b2 := New(2, nil)
	if err := b2.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(b2.Sources()); got != "[a b c]" {
		t.Fatalf("重启后应恢复全部来源，得到 %s", got)
	}
	b2.Clear()
	if got := len(b2.Sources()); got != 0 {
		t.Fatalf("清空后来源应清空，得到 %d", got)
	}
	b2.ClosePersist()
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "frp.sources")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("清空后应删除来源记录文件: %v", err)
	}
	// 损坏的来源记录文件被忽略
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "frp.sources"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readSources(filepath.Join(filepath.Dir(path), "frp.sources")); got != nil {
		t.Fatalf("损坏的记录应忽略，得到 %v", got)
	}
}

func TestSourcesLimit(t *testing.T) {
	b := New(10, nil)
	for i := 0; i < maxSources+50; i++ {
		b.Add(LevelInfo, fmt.Sprintf("s%04d", i), "x")
	}
	if n := len(b.Sources()); n != maxSources {
		t.Fatalf("来源数量应受上限限制，得到 %d", n)
	}
}
