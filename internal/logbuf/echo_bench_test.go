package logbuf

import (
	"os"
	"path/filepath"
	"testing"
)

// devNull 返回写到 /dev/null 的文件：回显每次写入都是一次真实的系统调用，与写标准错误输出的开销相当。
func devNull(b *testing.B) *os.File {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { f.Close() })
	return f
}

// 多个协程同时写日志并落盘、回显：衡量缓冲的锁与回显出口上的排队。
func BenchmarkLoggerParallelPersistEcho(b *testing.B) {
	buf := New(2000, nil)
	echo := NewEcho(devNull(b), nil)
	buf.SetEcho(echo)
	if err := buf.Persist(filepath.Join(b.TempDir(), "bench.log"), 64<<20); err != nil {
		b.Fatal(err)
	}
	defer buf.ClosePersist()
	l := buf.Logger("bench").With(Fields{FieldIP: "11.0.0.1", FieldHost: "s5.test.local", FieldStatus: "403"})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Warnf("拒绝 %s 访问 %s（访问控制）", "11.0.0.1", "s5.test.local")
		}
	})
	b.StopTimer()
	echo.Flush()
}

// 只回显（不落盘）：一条访问日志写到标准错误输出的开销。
func BenchmarkLoggerEcho(b *testing.B) {
	buf := New(2000, nil)
	echo := NewEcho(devNull(b), nil)
	buf.SetEcho(echo)
	l := buf.Logger("bench")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Infof("%s %s %s%s %d %dms %q %q", "11.0.0.1", "GET", "s0.test.local", "/", 200, 3, "-", "Mozilla/5.0")
	}
	b.StopTimer()
	echo.Flush()
}
