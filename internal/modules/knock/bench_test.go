package knock

import (
	"crypto/rand"
	"fmt"
	"net/netip"
	"testing"
	"time"
)

// BenchmarkForgedPacket 测量一个格式正确、HMAC 不匹配的报文（伪造洪水的典型内容）的处理成本：
// 每条启用的规则各做一次 HMAC 校验后丢弃。限速器放开，只计校验本身，用于确定全局令牌桶的速率。
func BenchmarkForgedPacket(b *testing.B) {
	for _, rules := range []int{1, 8} {
		for _, size := range []int{200, 1500} {
			b.Run(fmt.Sprintf("rules=%d/len=%d", rules, size), func(b *testing.B) {
				list := make([]Rule, rules)
				for i := range list {
					list[i] = sshRule(fmt.Sprintf("r%d", i))
				}
				env := newEnv(b, list...)
				env.m.stateMu.Lock()
				env.m.bucket = tokenBucket{rate: 1e12, burst: 1e12}
				env.m.stateMu.Unlock()
				pkt := forgedPacket(size)
				b.SetBytes(int64(size))
				b.ResetTimer()
				src := netip.MustParseAddr("198.51.100.9")
				for range b.N {
					// 每个报文前把时钟拨快 1 秒，让按来源的令牌桶回满，只测校验本身
					env.clock.set(env.clock.now().Add(time.Second))
					_ = env.m.handle(pkt, src, transportUDP, "UDP")
				}
			})
		}
	}
}

// forgedPacket 返回长度为 n、只含 base64 字符的随机报文。
func forgedPacket(n int) []byte {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	raw := make([]byte, n)
	_, _ = rand.Read(raw)
	for i := range raw {
		raw[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return raw
}
