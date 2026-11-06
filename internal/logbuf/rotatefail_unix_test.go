//go:build unix

package logbuf

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 轮转改名失败（目录只读）时，一次写盘只尝试一次轮转：剩余日志整批追加到当前文件，
// 不再每写一行就重试改名并打一行错误；目录恢复可写后下一次写盘正常轮转。
func TestRotateFailureDoesNotRetryPerLine(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 不受目录只读权限限制")
	}
	b, path := archiveBuf(t, 1000, PersistOptions{FileBytes: 300, KeepBytes: 1 << 20})
	dir := filepath.Dir(path)
	// 先写一行，让当前文件存在且有内容
	b.addAt(day(5, 10), LevelInfo, "", strings.Repeat("x", 80), nil)
	settle(b)

	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	for i := 0; i < 200; i++ {
		b.addAt(day(5, 10).Add(time.Duration(i+1)*time.Second), LevelInfo, "", strings.Repeat("y", 80), nil)
	}
	b.sink.flush()
	if n := strings.Count(out.String(), "归档日志文件"); n != 1 {
		t.Fatalf("一次写盘应只报一次轮转失败，得到 %d 次:\n%s", n, out.String())
	}
	// 日志没有丢：全部追加到了当前文件
	if got := readTail(path, 1000); len(got) != 201 {
		t.Fatalf("轮转失败时日志应追加到当前文件，得到 %d 条", len(got))
	}

	// 目录恢复可写后，下一次写盘正常轮转
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b.addAt(day(5, 11), LevelInfo, "", "恢复后", nil)
	settle(b)
	if len(b.sink.listArchives()) == 0 {
		t.Fatal("目录可写后应轮转出归档")
	}
}
