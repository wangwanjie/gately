package fileusers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"
)

// bcrypt 验证通过的结果缓存。WebDAV 客户端（访达、资源管理器、rclone 等）每个请求都带 Basic 凭据，
// 一次目录浏览会发出几十个请求，密码为 bcrypt 哈希时逐个请求验证会占用大量 CPU（默认成本约 85 毫秒/次）。
// 缓存键是进程内随机密钥对「存储的哈希 + 提交的密码」做的 HMAC，不保存明文密码；
// 存储的哈希在键里，修改密码后立即不再命中。只缓存验证通过的结果，错误密码每次都完整验证。
const (
	verifiedTTL = 10 * time.Minute
	// verifiedMax 为缓存条目上限。只有验证通过的凭据才会写入，攻击者无法用随机密码撑大缓存
	verifiedMax = 1024
)

var verified struct {
	once sync.Once
	key  [32]byte
	mu   sync.Mutex
	m    map[[32]byte]time.Time
}

func verifiedKey(stored, given string) [32]byte {
	verified.once.Do(func() {
		_, _ = rand.Read(verified.key[:])
	})
	m := hmac.New(sha256.New, verified.key[:])
	m.Write([]byte(stored))
	m.Write([]byte{0})
	m.Write([]byte(given))
	var k [32]byte
	copy(k[:], m.Sum(nil))
	return k
}

// verifiedHit 判断凭据是否在近期验证通过。
func verifiedHit(k [32]byte, now time.Time) bool {
	verified.mu.Lock()
	defer verified.mu.Unlock()
	exp, ok := verified.m[k]
	return ok && now.Before(exp)
}

// verifiedStore 记录验证通过的凭据。
func verifiedStore(k [32]byte, now time.Time) {
	verified.mu.Lock()
	defer verified.mu.Unlock()
	if verified.m == nil {
		verified.m = map[[32]byte]time.Time{}
	}
	if _, ok := verified.m[k]; !ok && len(verified.m) >= verifiedMax {
		// 满了先清掉过期的，仍然满（正常使用不会有这么多不同凭据）就整体清空，之后重新验证即可
		for key, exp := range verified.m {
			if !now.Before(exp) {
				delete(verified.m, key)
			}
		}
		if len(verified.m) >= verifiedMax {
			clear(verified.m)
		}
	}
	verified.m[k] = now.Add(verifiedTTL)
}
