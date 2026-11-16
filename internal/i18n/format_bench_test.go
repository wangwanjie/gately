package i18n

import (
	"fmt"
	"testing"
)

// 登录失败的日志格式：参数里带内层中文错误。
const benchLoginFormat = "来自 %s 的登录失败: %v"

// 回显一行被拒绝的请求：改造前把“时间 [级别] [来源] 内容”整行交给 Tr，改造后按格式串查表。
// 来源地址每次不同，缓存不会命中。
func BenchmarkEchoTranslateDenyLine(b *testing.B) {
	catalogFor("en")
	msgs := make([]string, 1<<16)
	lens := make([][]uint16, len(msgs))
	for i := range msgs {
		ip := benchIP(i)
		msgs[i] = fmt.Sprintf(benchDenyFormat, ip, "s5.test.local")
		lens[i] = []uint16{uint16(len(ip)), uint16(len("s5.test.local"))}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := i % len(msgs)
		_ = TrLogSource("en", "bench")
		_ = TrFormatLines("en", benchDenyFormat, msgs[k], lens[k])
	}
}

// 日志页翻译一条登录失败的日志：参数里的内层错误是另一条文案（重复出现，走缓存或精确匹配）。
func BenchmarkLogsTranslateLoginLine(b *testing.B) {
	catalogFor("en")
	inner := "用户名或密码错误"
	msgs := make([]string, 1<<16)
	lens := make([][]uint16, len(msgs))
	for i := range msgs {
		ip := benchIP(i)
		msgs[i] = fmt.Sprintf(benchLoginFormat, ip, inner)
		lens[i] = []uint16{uint16(len(ip)), uint16(len(inner))}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := i % len(msgs)
		_ = TrFormat("en", benchLoginFormat, msgs[k], lens[k])
	}
}
