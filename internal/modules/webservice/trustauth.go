package webservice

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/thirdauth"
)

// 信任已认证用户（Site.TrustAuthUsers，issue #81）：通过本站点 Gately 认证的访客不经过 WAF，
// 请求也不交给入侵防护的路径探测规则计数，避免 WAF 或 404、诱饵路径规则误报把真实用户拦在外面或封禁。
//
// 只认 Gately 自己签发、校验的凭据：站点会话 Cookie（网页认证与第三方登录）、共享用户的单点登录 Cookie、
// 近期验证通过的基本认证。转发认证（外部服务）、分享链接、管理后台登录失败与封禁表都不受影响：
// 已被封禁的地址仍在访问控制阶段被拒绝，端口扫描、诱饵端口、未知域名等也照常处理。

const (
	// trustBasicTTL 为基本认证凭据验证通过后的信任时长。基本认证每个请求都带凭据，
	// 在 WAF 之前再做一次 bcrypt 会让每个请求的 CPU 开销翻倍，所以只信任认证器近期验证过的凭据
	trustBasicTTL = 10 * time.Minute
	// trustBasicMax 为缓存的凭据数上限。只有验证通过的凭据才会写入，攻击者无法用随机凭据撑大缓存
	trustBasicMax = 1024
)

// basicTrustEntry 是一条验证通过的基本认证凭据：exp 为过期时间（Unix 秒），
// cred 为验证时用户的凭据指纹，用户被删除或修改密码后指纹变化，缓存立即失效
type basicTrustEntry struct {
	exp  int64
	cred string
}

// basicTrust 缓存验证通过的基本认证凭据。键为 HMAC(站点作用域、用户名、密码)，不保存明文密码。
type basicTrust struct {
	mu sync.Mutex
	m  map[[32]byte]basicTrustEntry
}

// basicTrustKey 计算凭据的缓存键，按站点作用域隔离：同一用户名密码在别的站点验证通过不算本站点的认证
func (a *authenticator) basicTrustKey(s Site, name, pass string) [32]byte {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte("trust-basic\x00" + s.scope + "\x00" + name + "\x00" + pass))
	var k [32]byte
	copy(k[:], m.Sum(nil))
	return k
}

// sharedBasicCred 返回共享用户当前用于基本认证的凭据指纹；站点未开启共享用户、用户不存在或启用了 TOTP
// （基本认证无法输入验证码）时返回空
func (a *authenticator) sharedBasicCred(s Site, name string) string {
	if !s.SharedUsers {
		return ""
	}
	u, ok := a.shared.get().users[name]
	if !ok || u.TOTPSecret != "" {
		return ""
	}
	return "sh|" + a.sharedCred(u)
}

// rememberBasic 在基本认证验证通过后记下凭据（仅开启了信任已认证用户的站点）。shared 表示按共享用户通过。
func (a *authenticator) rememberBasic(s Site, name, pass string, shared bool) {
	if !s.TrustAuthUsers {
		return
	}
	cred := a.credVersion(s.Users, name)
	if shared {
		cred = a.sharedBasicCred(s, name)
	}
	if cred == "" {
		return
	}
	k := a.basicTrustKey(s, name, pass)
	now := time.Now().Unix()
	a.trustBasic.mu.Lock()
	defer a.trustBasic.mu.Unlock()
	if a.trustBasic.m == nil {
		a.trustBasic.m = map[[32]byte]basicTrustEntry{}
	}
	if _, ok := a.trustBasic.m[k]; !ok && len(a.trustBasic.m) >= trustBasicMax {
		// 满了先清掉过期的，仍然满（正常用户不会有这么多不同凭据）就整体清空，之后重新验证即可
		for key, e := range a.trustBasic.m {
			if e.exp <= now {
				delete(a.trustBasic.m, key)
			}
		}
		if len(a.trustBasic.m) >= trustBasicMax {
			clear(a.trustBasic.m)
		}
	}
	a.trustBasic.m[k] = basicTrustEntry{exp: now + int64(trustBasicTTL/time.Second), cred: cred}
}

// trustedBasic 判断请求携带的基本认证凭据是否在近期验证通过，且用户凭据此后没有变化。只做一次 HMAC 与查表。
func (a *authenticator) trustedBasic(r *http.Request, s Site) bool {
	name, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	k := a.basicTrustKey(s, name, pass)
	a.trustBasic.mu.Lock()
	e, ok := a.trustBasic.m[k]
	a.trustBasic.mu.Unlock()
	if !ok || e.exp <= time.Now().Unix() {
		return false
	}
	// 站点用户优先（与 checkUser 一致），其次是共享用户
	if c := a.credVersion(s.Users, name); c != "" && hmac.Equal([]byte(c), []byte(e.cred)) {
		return true
	}
	c := a.sharedBasicCred(s, name)
	return c != "" && hmac.Equal([]byte(c), []byte(e.cred))
}

// trustedAuth 判断请求是否带有本站点 Gately 认证的有效凭据（站点开启了信任已认证用户时）。
// 在 WAF 之前调用：只校验签名与凭据指纹，不写响应、不计登录失败，也不处理登录页等认证保留路径。
func (h *siteHandler) trustedAuth(r *http.Request) bool {
	s := h.site
	if !s.TrustAuthUsers || (!s.WebAuth && !s.BasicAuth) {
		return false
	}
	if s.WebAuth {
		if sess, ok := h.auth.cookieSession(r, s); ok {
			// 第三方登录会话只在身份被允许列表逐个列出时信任：提供方key:* 通配让任何人注册个账号就能登录，
			// 信任它等于对所有人关闭 WAF
			provider, id, third := strings.Cut(sess.User, ":")
			if !third || sess.Shared || sess.Cred != "" {
				return true
			}
			if thirdauth.AllowedExplicit(s.OAuthUsers, thirdauth.Identity{Provider: provider, Primary: id, Aliases: sess.Aliases}) {
				return true
			}
		}
		if s.SharedUsers {
			if _, ok := h.auth.ssoSession(r, s); ok {
				return true
			}
		}
	}
	// 网页认证对非浏览器客户端同样回退到基本认证
	return h.auth.trustedBasic(r, s)
}
