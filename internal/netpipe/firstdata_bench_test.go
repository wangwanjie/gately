package netpipe

import (
	"context"
	"testing"
	"time"
)

// BenchmarkWaitFirstDataReady 衡量客户端数据在等待开始前就已到达时（HTTP、TLS 等客户端先发言的协议，
// 连接目标需要一个往返，期间首包通常已经到了）WaitFirstData 的开销：每个转发的新连接都要经过一次。
func BenchmarkWaitFirstDataReady(b *testing.B) {
	a, aPeer := connPair(b)
	c, _ := connPair(b)
	if _, err := aPeer.Write([]byte("GET / HTTP/1.1\r\n")); err != nil {
		b.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := WaitFirstData(ctx, a, c, time.Minute); err != nil {
			b.Fatal(err)
		}
	}
}
