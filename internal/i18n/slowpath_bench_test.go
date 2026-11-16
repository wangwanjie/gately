package i18n

import (
	"fmt"
	"testing"
)

// 基准用的日志格式：访问控制拒绝，来源地址各不相同。
const benchDenyFormat = "拒绝 %s 访问 %s（访问控制）"

func benchIP(i int) string {
	return fmt.Sprintf("%d.%d.%d.%d", 11+i>>24&0x3f, i>>16&0xff, i>>8&0xff, i&0xff)
}

// 没有格式串可用时的整句翻译（从文件读回的日志、标准库 log 的输出），内容每次不同：遍历模板与片段的慢路径。
func BenchmarkTrUniqueLine(b *testing.B) {
	catalogFor("en")
	lines := make([]string, 1<<16)
	for i := range lines {
		lines[i] = "2026/11/16 22:23:28 [warn] [bench] " + fmt.Sprintf(benchDenyFormat, benchIP(i), "s5.test.local")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Tr("en", lines[i%len(lines)])
	}
}

// 同一句话反复翻译：命中缓存。
func BenchmarkTrCachedLine(b *testing.B) {
	line := "2026/11/16 22:23:28 [warn] [bench] " + fmt.Sprintf(benchDenyFormat, "11.0.0.1", "s5.test.local")
	_ = Tr("en", line)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Tr("en", line)
	}
}

// 多个协程同时翻译各不相同的日志行（洪峰下每个请求协程各翻译一行）。
func BenchmarkTrUniqueLineParallel(b *testing.B) {
	catalogFor("en")
	lines := make([]string, 1<<16)
	for i := range lines {
		lines[i] = "2026/11/16 22:23:28 [warn] [bench] " + fmt.Sprintf(benchDenyFormat, benchIP(i), "s5.test.local")
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = Tr("en", lines[i%len(lines)])
			i += 7
		}
	})
}
