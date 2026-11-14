package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// 后台登录桥接凭据：管理员登录后台后，Web 服务可以向单点登录域签发一个只表示“该管理员已登录后台”的凭据，
// 其他开启“后台登录免认证”的站点凭此放行。凭据与后台会话令牌是两个值：它只在站点认证处使用、不会转发给后端应用，
// 不能用来调用管理接口；后台会话令牌本身仍只发给后台所在的域名。
//
// 凭据绑定签发它的那一个后台会话令牌：登出（吊销）、退出所有设备（纪元变化）、修改密码（签名密钥轮换）、
// 令牌过期，凭据随之立即失效；只读账号的会话不会产生凭据。

// bridgePayload 是桥接凭据的内容。
type bridgePayload struct {
	User string `json:"u"`
	Exp  int64  `json:"e"`
	// Ver、Epoch 与后台会话令牌的含义相同
	Ver   string `json:"v"`
	Epoch int64  `json:"n,omitempty"`
	// Token 为所依附的后台会话令牌的吊销编号
	Token string `json:"t"`
}

// bridgeMAC 计算桥接凭据的签名。签名内容带固定前缀：后台会话令牌与桥接凭据用同一个密钥签名，
// 前缀保证两者的签名互不通用，凭据不能冒充会话令牌，反之亦然。
func bridgeMAC(secret string, p string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("bridge|" + p))
	return mac.Sum(nil)
}

// IssueBridge 为有效的管理员会话令牌签发桥接凭据，返回凭据与过期时间（与会话令牌相同）。
// 令牌无效、已吊销或属于只读账号时 ok 为 false。
func (m *Manager) IssueBridge(token string) (value string, exp time.Time, ok bool) {
	id, found := m.Identify(token)
	if !found || id.Role != RoleAdmin {
		return "", time.Time{}, false
	}
	p, sig, _ := strings.Cut(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return "", time.Time{}, false
	}
	var pl payload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return "", time.Time{}, false
	}
	m.mu.RLock()
	secret := m.acc.SessionSecret
	m.mu.RUnlock()
	body, _ := json.Marshal(bridgePayload{User: pl.User, Exp: pl.Exp, Ver: pl.Ver, Epoch: pl.Epoch, Token: tokenID(sig)})
	b := base64.RawURLEncoding.EncodeToString(body)
	return b + "." + base64.RawURLEncoding.EncodeToString(bridgeMAC(secret, b)), time.Unix(pl.Exp, 0), true
}

// VerifyBridge 校验桥接凭据，有效时返回管理员用户名。
func (m *Manager) VerifyBridge(value string) (string, bool) {
	p, sig, found := strings.Cut(value, ".")
	if !found {
		return "", false
	}
	// 先解码签名并检查长度，其他应用的同名 Cookie 等格式不对的值不必计算 HMAC
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || len(got) != sha256.Size {
		return "", false
	}
	m.mu.RLock()
	acc := m.acc
	m.mu.RUnlock()
	if !hmac.Equal(got, bridgeMAC(acc.SessionSecret, p)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return "", false
	}
	var bp bridgePayload
	if err := json.Unmarshal(raw, &bp); err != nil {
		return "", false
	}
	if m.now().Unix() > bp.Exp || bp.Epoch != acc.SessionEpoch || bp.User != acc.Username || bp.Ver != hashVersion(acc.PasswordHash) {
		return "", false
	}
	m.mu.RLock()
	_, revoked := m.revoked[bp.Token]
	m.mu.RUnlock()
	if revoked {
		return "", false
	}
	return bp.User, true
}
