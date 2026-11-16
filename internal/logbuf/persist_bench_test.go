package logbuf

import (
	"path/filepath"
	"testing"
)

// 写一条带两个参数与结构化字段的警告日志并落盘：每条日志要序列化为一行 JSON。
func BenchmarkLoggerWarnfPersist(b *testing.B) {
	buf := New(2000, nil)
	if err := buf.Persist(filepath.Join(b.TempDir(), "bench.log"), 64<<20); err != nil {
		b.Fatal(err)
	}
	defer buf.ClosePersist()
	l := buf.Logger("bench").With(Fields{FieldIP: "11.0.0.1", FieldHost: "s5.test.local"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Warnf("拒绝 %s 访问 %s（访问控制）", "11.0.0.1", "s5.test.local")
	}
}
