package logbuf

import "testing"

// 写一条带两个参数与结构化字段的警告日志（访问控制拒绝）。只进内存缓冲。
func BenchmarkLoggerWarnf(b *testing.B) {
	buf := New(2000, nil)
	l := buf.Logger("bench").With(Fields{FieldIP: "11.0.0.1", FieldHost: "s5.test.local"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Warnf("拒绝 %s 访问 %s（访问控制）", "11.0.0.1", "s5.test.local")
	}
}

// 启动时为 28 个模块各建一个 2000 条的缓冲。
func BenchmarkNewBuffers(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for m := 0; m < 28; m++ {
			_ = New(2000, nil)
		}
	}
}
