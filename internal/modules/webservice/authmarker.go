package webservice

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 认证标记（issue #81 的双层 Gately 场景）：外层 Gately（如 VPS）只做 WAF 并反向代理到内层 Gately，
// 由内层负责登录。外层无法校验内层签发的会话，信任已登录用户在外层不起作用。
//
// 两端在全局设置中填写相同的认证标记密钥后：
//   - 内层：开启信任已登录用户的站点，对带受信任凭据的请求在响应头 X-Gately-Auth 中返回签名标记，
//     标记绑定该请求所用凭据（会话 Cookie 值或 Authorization 头）的哈希与过期时间；后端应用自带的同名头先被去掉。
//   - 外层：开启信任后端 Gately 的认证标记的反向代理站点，在响应头写出前校验标记的签名、有效期，
//     并确认它绑定的正是本次请求携带的凭据，通过后记住该凭据：本次请求不计入路径探测，
//     同一凭据之后的请求不经过 WAF。标记头始终在返回访客前去掉。
//
// WAF 在请求转给后端之前检查，所以同一凭据的第一个请求仍经过 WAF，拿到标记后的请求才放行。
// 标记被截获或被缓存重放给别人也无法利用：外层只信任与标记绑定的凭据，拿到凭据本身才能通过。

const (
	// authMarkerHeader 为认证标记的响应头
	authMarkerHeader = "X-Gately-Auth"
	// authMarkerTTL 为标记的有效期，每次响应都会续签；内层的会话失效后外层最多在这段时间内仍放行该凭据（只影响 WAF 与路径探测，
	// 请求本身仍由内层认证拦下）
	authMarkerTTL = 5 * time.Minute
	// authMarkerMaxAhead 为外层接受的最远过期时间，两端时钟偏差较大或伪造超长有效期的标记一律拒绝
	authMarkerMaxAhead = 10 * time.Minute
	// backendTrustMax 为外层记住的凭据数上限。只有签名有效且与请求凭据一致的标记才能写入
	backendTrustMax = 4096
	// maxCandidateCreds 为外层从一个请求中取出比对的凭据数上限，避免为大量伪造的 Cookie 逐个计算哈希
	maxCandidateCreds = 8
	// minAuthMarkerKey 为认证标记密钥的最短长度
	minAuthMarkerKey = 16
)

// credSession 与 credBasic 把凭据转成签发标记时绑定的字符串：会话 Cookie 只取值（名称在两层之间相同），
// 基本认证取整个 Authorization 头
func credSession(v string) string { return "c\x00" + v }
func credBasic(v string) string   { return "b\x00" + v }

// credHash 返回凭据的哈希（十六进制 32 位），标记中只出现哈希，不暴露凭据本身
func credHash(cred string) string {
	sum := sha256.Sum256([]byte("gately-auth-marker-cred\x00" + cred))
	return hex.EncodeToString(sum[:16])
}

// markerKey 保存认证标记密钥，设置变更时整体替换；nil 表示未启用
type markerKey struct {
	p atomic.Pointer[[]byte]
}

func (k *markerKey) set(s string) {
	if s == "" {
		k.p.Store(nil)
		return
	}
	b := []byte(s)
	k.p.Store(&b)
}

func (k *markerKey) get() []byte {
	if p := k.p.Load(); p != nil {
		return *p
	}
	return nil
}

// markerMAC 计算标记的签名
func markerMAC(key []byte, exp int64, hash string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("gately-auth-marker|v1|" + strconv.FormatInt(exp, 10) + "|" + hash))
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// signAuthMarker 为凭据签发标记：v1.过期时间.凭据哈希.签名
func signAuthMarker(key []byte, cred string, now time.Time) string {
	exp := now.Add(authMarkerTTL).Unix()
	h := credHash(cred)
	return "v1." + strconv.FormatInt(exp, 10) + "." + h + "." + markerMAC(key, exp, h)
}

// verifyAuthMarker 校验标记，返回其中的凭据哈希与过期时间
func verifyAuthMarker(key []byte, v string, now time.Time) (hash string, exp int64, ok bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 4 || parts[0] != "v1" || len(parts[2]) != 32 || len(parts[3]) != 32 {
		return "", 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	n := now.Unix()
	if exp <= n || exp > n+int64(authMarkerMaxAhead/time.Second) {
		return "", 0, false
	}
	if !hmac.Equal([]byte(parts[3]), []byte(markerMAC(key, exp, parts[2]))) {
		return "", 0, false
	}
	return parts[2], exp, true
}

// requestCredHashes 返回请求携带的、可能由后端 Gately 校验的凭据哈希：Gately 会话 Cookie（站点会话与单点登录）
// 与 Authorization 头，最多 maxCandidateCreds 个
func requestCredHashes(r *http.Request) []string {
	var out []string
	if v := r.Header.Get("Authorization"); v != "" {
		out = append(out, credHash(credBasic(v)))
	}
	for _, c := range r.Cookies() {
		if len(out) >= maxCandidateCreds {
			break
		}
		if c.Name == ssoCookie || strings.HasPrefix(c.Name, "gately_site_") {
			out = append(out, credHash(credSession(c.Value)))
		}
	}
	return out
}

// backendTrust 记录外层站点上得到后端认证标记的凭据：键为 站点作用域 + 凭据哈希，值为过期时间（Unix 秒）。
// 按站点作用域隔离：一个站点的后端认可的凭据，不能在后端不同的另一个站点上跳过 WAF
type backendTrust struct {
	mu sync.Mutex
	m  map[string]int64
}

func (b *backendTrust) remember(key string, exp int64, now int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = map[string]int64{}
	}
	if _, ok := b.m[key]; !ok && len(b.m) >= backendTrustMax {
		for k, e := range b.m {
			if e <= now {
				delete(b.m, k)
			}
		}
		if len(b.m) >= backendTrustMax {
			clear(b.m)
		}
	}
	b.m[key] = exp
}

// reset 清空记住的凭据：认证标记密钥修改或清空后，按旧密钥得到的放行立即失效
func (b *backendTrust) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	clear(b.m)
}

func (b *backendTrust) valid(key string, now int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	exp, ok := b.m[key]
	return ok && exp > now
}

// trustedBackend 判断请求的凭据近期是否得到后端 Gately 的认证标记（站点开启了信任后端 Gately 的认证标记时）。
func (h *siteHandler) trustedBackend(r *http.Request) bool {
	if !h.site.TrustBackendAuth || h.auth.marker.get() == nil {
		return false
	}
	now := time.Now().Unix()
	for _, hash := range requestCredHashes(r) {
		if h.auth.backends.valid(h.site.scope+"\x00"+hash, now) {
			return true
		}
	}
	return false
}

// markerHook 返回在响应头写出前处理认证标记的函数；未配置密钥时返回 nil。
//   - 信任已登录用户的站点（内层）：去掉后端应用自带的标记头，本次请求有受信任凭据时签发新标记；
//   - 信任后端认证标记的站点（外层）：校验并去掉后端返回的标记头，有效时记住凭据、本次请求不计入路径探测；
//   - 其他站点：只去掉标记头。配置了密钥即说明部署了双层 Gately，后端的标记不应原样返回给访客（外层关闭选项时也一样）。
func (h *siteHandler) markerHook(r *http.Request, ar *activeReq) func(http.Header) {
	key := h.auth.marker.get()
	if key == nil || ar == nil {
		return nil
	}
	switch {
	case h.site.TrustAuthUsers:
		return func(hd http.Header) {
			hd.Del(authMarkerHeader)
			if cred := ar.markCred.Load(); cred != nil && *cred != "" {
				hd.Set(authMarkerHeader, signAuthMarker(key, *cred, time.Now()))
			}
		}
	case !h.site.TrustBackendAuth:
		return func(hd http.Header) {
			hd.Del(authMarkerHeader)
		}
	default:
		return func(hd http.Header) {
			vals := hd.Values(authMarkerHeader)
			hd.Del(authMarkerHeader)
			if len(vals) != 1 {
				return
			}
			now := time.Now()
			hash, exp, ok := verifyAuthMarker(key, vals[0], now)
			if !ok {
				return
			}
			for _, c := range requestCredHashes(r) {
				if hmac.Equal([]byte(c), []byte(hash)) {
					h.auth.backends.remember(h.site.scope+"\x00"+hash, exp, now.Unix())
					ar.authed.Store(true)
					return
				}
			}
		}
	}
}
