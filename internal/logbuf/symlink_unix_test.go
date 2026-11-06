//go:build unix

package logbuf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNoFollowSymlinks 模拟日志目录可被其他用户写入时预先放好的符号链接：
// 当前日志文件、来源记录的临时文件、压缩的临时文件都不能把内容写进（或截断）链接指向的文件。
func TestNoFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	const orig = "重要内容"
	if err := os.WriteFile(victim, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "webservice.log")
	// 第一条写入失败后，当前文件从第二条（1 号 2 点）开始，归档按它命名
	ts := day(1, 2).UTC().Format(archiveTimeLayout)
	for _, name := range []string{"webservice.log", "webservice.sources.tmp", "webservice." + ts + ".log.gz.tmp"} {
		if err := os.Symlink(victim, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	b := New(10, nil)
	if err := b.PersistWith(path, PersistOptions{Archive: true}); err != nil {
		t.Fatal(err)
	}
	b.addAt(day(1, 1), LevelInfo, "HTTPS", "第一天", nil)
	b.sink.flush()
	// 当前文件是符号链接：写入失败，日志留在内存，不写进目标文件
	if data, _ := os.ReadFile(victim); string(data) != orig {
		t.Fatalf("不应跟随符号链接写入当前日志文件: %q", data)
	}
	// 删掉当前文件的链接后正常写入；跨天轮转时压缩用的临时文件名上也有链接
	_ = os.Remove(path)
	b.addAt(day(1, 2), LevelInfo, "HTTPS", "第一天 2", nil)
	b.sink.flush()
	b.addAt(day(2, 1), LevelInfo, "HTTPS", "第二天", nil)
	settle(b)
	s := b.sink
	b.ClosePersist()
	s.bg.Wait()
	if data, _ := os.ReadFile(victim); string(data) != orig {
		t.Fatalf("目标文件被改写: %q", data)
	}
	if st, err := os.Lstat(filepath.Join(dir, "webservice."+ts+".log.gz")); err != nil || !st.Mode().IsRegular() {
		t.Fatalf("归档应正常压缩为普通文件: %v", err)
	}
	if st, err := os.Lstat(filepath.Join(dir, "webservice.sources")); err != nil || !st.Mode().IsRegular() {
		t.Fatalf("来源记录应写成普通文件: %v", err)
	}
	// 归档名上的符号链接不会被当作归档读取或压缩
	if err := os.Symlink(victim, filepath.Join(dir, "webservice."+day(5, 1).UTC().Format(archiveTimeLayout)+".log")); err != nil {
		t.Fatal(err)
	}
	if err := compressFile(filepath.Join(dir, "webservice."+day(5, 1).UTC().Format(archiveTimeLayout)+".log")); err == nil {
		t.Fatal("压缩符号链接应失败")
	}
	if data, _ := os.ReadFile(victim); string(data) != orig {
		t.Fatalf("目标文件被改写: %q", data)
	}
}
