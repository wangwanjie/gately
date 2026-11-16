package logbuf

import "testing"

// 调试日志（端口转发每条连接一行）：默认的 info 级别下在格式化之前返回。
func BenchmarkDebugfConnLine(b *testing.B) {
	buf := New(2000, nil)
	buf.SetMinLevel(LevelInfo)
	l := buf.Logger("fwd")
	var client, target any = "10.0.0.1:51234", "10.0.0.2:80"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Debugf("%s -> %s 已连接", client, target)
	}
}
