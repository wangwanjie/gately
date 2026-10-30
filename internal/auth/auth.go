// Package auth 实现管理员账号、会话令牌与登录限流。
//
// 令牌格式：base64url(payload) + "." + base64url(HMAC-SHA256(payload))。
// 修改密码时轮换签名密钥，使所有旧会话立即失效。
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/webauthn"
)

// CookieName 是会话 Cookie 名称。
const CookieName = "gately_token"

// DefaultPassword 是管理员账号的默认密码：全新安装（未指定 GATELY_INITIAL_PASSWORD）与命令行重置密码时使用。
// 固定默认密码便于不查看日志的用户首次登录，仍在使用时管理界面会持续提醒修改。
const DefaultPassword = "gately.cn"

// 登录限流默认参数：窗口期内连续失败达到上限后锁定该 IP。上限与锁定时长可在设置中调整。
const (
	maxFailures  = 5
	failWindow   = 15 * time.Minute
	lockDuration = 15 * time.Minute
)

var (
	// ErrInvalidCredentials 表示用户名或密码错误。
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	// ErrLocked 表示该 IP 因多次失败被临时锁定。
	ErrLocked = errors.New("登录失败次数过多，请稍后再试")
	// ErrTOTPRequired 表示需要输入两步验证码。
	ErrTOTPRequired = errors.New("请输入两步验证码")
)

// Account 是持久化的管理员账号信息。
type Account struct {
	Username      string `json:"username"`
	PasswordHash  string `json:"passwordHash"`
	SessionSecret string `json:"sessionSecret"`
	TokenTTLHours int    `json:"tokenTTLHours"`
	// TOTPSecret 非空表示已启用两步验证
	TOTPSecret string `json:"totpSecret,omitempty"`
	// Users 是管理员创建的附加账号（目前只有只读角色），主账号之外使用
	Users []User `json:"users,omitempty"`
	// Passkeys 是管理员注册的 Passkey（仅公钥）
	Passkeys []webauthn.Credential `json:"passkeys,omitempty"`
	// OnboardingPending 在全新安装（首次创建管理员账号）时置位，完成或跳过首次引导后清除
	OnboardingPending bool `json:"onboardingPending,omitempty"`
	// UsingDefaultPassword 表示主账号仍在使用默认密码 DefaultPassword，修改密码后清除
	UsingDefaultPassword bool `json:"usingDefaultPassword,omitempty"`
}

// 角色
const (
	// RoleAdmin 拥有全部权限（主账号与第三方登录）
	RoleAdmin = "admin"
	// RoleReadonly 只能查看，不能修改配置、读取密钥或进行任何写操作
	RoleReadonly = "readonly"
)

// User 是附加账号。
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	Role         string `json:"role"`
}

// Identity 是令牌验证通过后的身份。
type Identity struct {
	User string
	Role string
}

type failState struct {
	count int
	// inflight 为正在校验中的请求数，校验前预占，防止并发请求绕过次数限制
	inflight    int
	first       time.Time
	lockedUntil time.Time
}

// accountKey 用于账号设置类操作（修改密码、两步验证）的失败计数。
const accountKey = "__account__"

// bcryptSem 限制同时进行的 bcrypt 校验数量，避免未认证请求打满 CPU。
var bcryptSem = make(chan struct{}, 4)

// limitKey 返回限流键：IPv6 按 /64 聚合，避免攻击者在一个网段内轮换地址。
func limitKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().String()
	}
	return a.String()
}

// Manager 管理账号校验和会话。
type Manager struct {
	mu    sync.RWMutex
	acc   Account
	save  func(Account) error
	fails map[string]*failState
	now   func() time.Time
	// lastTOTPStep 为最近一次使用的验证码时间步，防止验证码在有效期内被重放
	lastTOTPStep int64
	// maxFail / lockDur 为当前生效的限流参数
	maxFail int
	lockDur time.Duration
	// bootstrapPassword 非空时，首次启动用它作为初始密码而不是默认密码
	bootstrapPassword string
}

// SetBootstrapPassword 指定首次启动时的初始密码（用于自动化部署）。账号已存在时不起作用。
func (m *Manager) SetBootstrapPassword(p string) error {
	if p != "" && len(p) < 8 {
		return errors.New("初始密码至少 8 位")
	}
	m.mu.Lock()
	m.bootstrapPassword = p
	m.mu.Unlock()
	return nil
}

// SetLimits 设置连续失败上限与锁定时长，非正数表示使用默认值。
func (m *Manager) SetLimits(maxFail int, lock time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if maxFail <= 0 {
		maxFail = maxFailures
	}
	if lock <= 0 {
		lock = lockDuration
	}
	m.maxFail, m.lockDur = maxFail, lock
}

// TokenTTLHours 返回会话有效期（小时）。
func (m *Manager) TokenTTLHours() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.TokenTTLHours
}

// SetTokenTTL 修改会话有效期并持久化，已签发的会话不受影响。
func (m *Manager) SetTokenTTL(hours int) error {
	if hours < 1 || hours > 24*365 {
		return errors.New("会话有效期应在 1 小时到 365 天之间")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.acc.TokenTTLHours == hours {
		return nil
	}
	acc := m.acc
	acc.TokenTTLHours = hours
	if err := m.save(acc); err != nil {
		return err
	}
	m.acc = acc
	return nil
}

// NewManager 用已加载的账号创建管理器，save 用于持久化账号变更。
func NewManager(acc Account, save func(Account) error) *Manager {
	return &Manager{acc: acc, save: save, fails: map[string]*failState{}, now: time.Now, maxFail: maxFailures, lockDur: lockDuration}
}

// EnsureInitialized 在首次启动时创建 admin 账号：未指定初始密码时使用默认密码 DefaultPassword。
// 返回值 initial 非空表示本次使用了默认密码，调用方需要提示用户登录后修改；指定的初始密码不返回，避免写入日志。
func (m *Manager) EnsureInitialized() (initial string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	if m.acc.SessionSecret == "" {
		m.acc.SessionSecret = randomString(32)
		changed = true
	}
	if m.acc.TokenTTLHours <= 0 {
		m.acc.TokenTTLHours = 72
		changed = true
	}
	if m.acc.Username == "" || m.acc.PasswordHash == "" {
		password := m.bootstrapPassword
		if password == "" {
			initial = DefaultPassword
			password = DefaultPassword
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		m.acc.Username = "admin"
		m.acc.PasswordHash = string(hash)
		m.acc.OnboardingPending = true
		m.acc.UsingDefaultPassword = password == DefaultPassword
		changed = true
	}
	if changed {
		if err := m.save(m.acc); err != nil {
			return "", err
		}
	}
	return initial, nil
}

// Username 返回当前管理员用户名。
func (m *Manager) Username() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.Username
}

// Login 校验凭据并签发令牌。启用两步验证时 code 必填。
func (m *Manager) Login(ip, username, password string, code ...string) (string, time.Time, error) {
	key := limitKey(ip)
	if !m.reserve(key) {
		return "", time.Time{}, ErrLocked
	}
	ok := false
	// pending 表示密码正确但还缺验证码：既不算失败也不算成功，只释放预占名额，不清除已累计的失败次数
	pending := false
	defer func() {
		if pending {
			m.cancel(key)
			return
		}
		m.release(key, ok)
	}()

	m.mu.RLock()
	acc := m.acc
	m.mu.RUnlock()

	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(acc.Username)) == 1
	hash, role := acc.PasswordHash, RoleAdmin
	if !userOK {
		// 不是主账号时查找附加账号；找不到则仍用主账号哈希执行一次 bcrypt，避免通过耗时判断用户名是否存在
		for _, u := range acc.Users {
			if subtle.ConstantTimeCompare([]byte(username), []byte(u.Username)) == 1 {
				hash, role, userOK = u.PasswordHash, u.Role, true
				break
			}
		}
	}
	// 无论用户名是否正确都执行一次 bcrypt，避免通过耗时判断用户名
	passErr := checkPassword(hash, password)
	if !userOK || passErr != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}
	// 两步验证只作用于主账号
	if role != RoleAdmin {
		ok = true
		exp := m.now().Add(time.Duration(acc.TokenTTLHours) * time.Hour)
		return m.sign(acc, exp, username, role, hash), exp, nil
	}
	if acc.TOTPSecret != "" {
		c := ""
		if len(code) > 0 {
			c = code[0]
		}
		if c == "" {
			// 密码正确但缺验证码，不计为失败；也不能算成功，否则攻击者可在每次猜错验证码之间
			// 插入一次不带验证码的请求来清零失败计数，从而无限次爆破验证码
			pending = true
			return "", time.Time{}, ErrTOTPRequired
		}
		if !m.consumeTOTP(acc.TOTPSecret, c) {
			return "", time.Time{}, errors.New("两步验证码错误")
		}
	}
	ok = true
	exp := m.now().Add(time.Duration(acc.TokenTTLHours) * time.Hour)
	return m.sign(acc, exp, acc.Username, RoleAdmin, acc.PasswordHash), exp, nil
}

// Issue 为已通过其他方式（如第三方登录）验证的管理员直接签发会话令牌。
func (m *Manager) Issue() (string, time.Time) {
	m.mu.RLock()
	acc := m.acc
	m.mu.RUnlock()
	exp := m.now().Add(time.Duration(acc.TokenTTLHours) * time.Hour)
	return m.sign(acc, exp, acc.Username, RoleAdmin, acc.PasswordHash), exp
}

// checkPassword 在全局并发上限内执行 bcrypt 比较。
func checkPassword(hash, password string) error {
	bcryptSem <- struct{}{}
	defer func() { <-bcryptSem }()
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// consumeTOTP 校验验证码，每个时间步只能使用一次。
func (m *Manager) consumeTOTP(secret, code string) bool {
	step, ok := matchTOTP(secret, code, m.now())
	if !ok {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if step <= m.lastTOTPStep {
		return false
	}
	m.lastTOTPStep = step
	return true
}

// ChangeCredentials 修改用户名和密码，需要提供旧密码；成功后旧令牌全部失效。
func (m *Manager) ChangeCredentials(oldPassword, newUsername, newPassword string) error {
	newUsername = strings.TrimSpace(newUsername)
	if newUsername == "" {
		return errors.New("用户名不能为空")
	}
	if len(newPassword) < 8 {
		return errors.New("新密码至少 8 位")
	}
	if err := m.verifyAccountPassword(oldPassword); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.acc.Users {
		if u.Username == newUsername {
			return errors.New("用户名与附加账号重复")
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	next := m.acc
	next.Username = newUsername
	next.PasswordHash = string(hash)
	next.SessionSecret = randomString(32)
	// 改回默认密码时继续提醒
	next.UsingDefaultPassword = newPassword == DefaultPassword
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

type payload struct {
	User string `json:"u"`
	Exp  int64  `json:"e"`
	// Role 缺省表示管理员，兼容旧令牌
	Role string `json:"r,omitempty"`
	// Ver 是账号密码哈希的摘要，修改密码或删除账号后旧令牌随之失效
	Ver string `json:"v,omitempty"`
}

// hashVersion 返回密码哈希的短摘要，用于把令牌绑定到账号当前的密码。
func hashVersion(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}

func (m *Manager) sign(acc Account, exp time.Time, user, role, hash string) string {
	pl := payload{User: user, Exp: exp.Unix(), Ver: hashVersion(hash)}
	if role != RoleAdmin {
		pl.Role = role
	}
	body, _ := json.Marshal(pl)
	p := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(acc.SessionSecret))
	mac.Write([]byte(p))
	return p + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify 校验令牌，返回用户名。
func (m *Manager) Verify(token string) (string, bool) {
	id, ok := m.Identify(token)
	return id.User, ok
}

// Identify 校验令牌并返回身份（用户名与角色）。
func (m *Manager) Identify(token string) (Identity, bool) {
	p, sig, found := strings.Cut(token, ".")
	if !found {
		return Identity{}, false
	}
	m.mu.RLock()
	acc := m.acc
	m.mu.RUnlock()
	mac := hmac.New(sha256.New, []byte(acc.SessionSecret))
	mac.Write([]byte(p))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, want) {
		return Identity{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return Identity{}, false
	}
	var pl payload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return Identity{}, false
	}
	if m.now().Unix() > pl.Exp {
		return Identity{}, false
	}
	if pl.Role == "" {
		// 管理员令牌：用户名须与主账号一致；带版本号的令牌还须与当前密码匹配
		if pl.User != acc.Username || (pl.Ver != "" && pl.Ver != hashVersion(acc.PasswordHash)) {
			return Identity{}, false
		}
		return Identity{User: pl.User, Role: RoleAdmin}, true
	}
	for _, u := range acc.Users {
		if u.Username == pl.User && u.Role == pl.Role && pl.Ver == hashVersion(u.PasswordHash) {
			return Identity{User: u.Username, Role: u.Role}, true
		}
	}
	return Identity{}, false
}

// TokenFromRequest 从 Authorization 头或 Cookie 中取令牌。
func TokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return ""
}

type ctxKey struct{}

type roleKey struct{}

// RoleFromContext 返回中间件写入的角色；未写入（如 API 令牌，自带权限范围）时为空。
func RoleFromContext(ctx context.Context) string {
	r, _ := ctx.Value(roleKey{}).(string)
	return r
}

type scopeKey struct{}

// WithScopeCheck 把 API 令牌的权限判定函数写入上下文，供 Allowed 使用。
func WithScopeCheck(ctx context.Context, check func(need string) bool) context.Context {
	return context.WithValue(ctx, scopeKey{}, check)
}

// Allowed 判断当前请求的身份是否具备权限 need（格式同 API 令牌权限，如 "docker:write"），
// 用于在同一接口内按权限决定是否返回敏感字段。API 令牌按自身权限判定；
// 登录会话中管理员具备全部权限、只读账号只具备读权限；上下文中没有身份时一律视为无权限。
func Allowed(ctx context.Context, need string) bool {
	if check, ok := ctx.Value(scopeKey{}).(func(string) bool); ok {
		return check(need)
	}
	switch RoleFromContext(ctx) {
	case RoleAdmin:
		return true
	case RoleReadonly:
		return strings.HasSuffix(need, ":read")
	}
	return false
}

// WithUser 把用户名写入上下文（API 令牌等其他认证方式使用）。
func WithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, ctxKey{}, user)
}

// EntrySecret 返回用于签名安全入口 Cookie 的密钥。
func (m *Manager) EntrySecret() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.SessionSecret
}

// UserFromContext 返回中间件写入的用户名。
func UserFromContext(ctx context.Context) string {
	u, _ := ctx.Value(ctxKey{}).(string)
	return u
}

// Require 是要求登录的中间件。
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := m.Identify(TokenFromRequest(r))
		user := id.User
		if !ok {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "未登录或登录已过期"})
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, user)
		ctx = context.WithValue(ctx, roleKey{}, id.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// reserve 检查是否锁定并预占一次尝试；返回 false 表示应拒绝。
func (m *Manager) reserve(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	st := m.fails[key]
	if st == nil || (now.Sub(st.first) > failWindow && st.inflight == 0) {
		if st != nil && now.Before(st.lockedUntil) {
			return false
		}
		st = &failState{first: now}
		m.fails[key] = st
	}
	if now.Before(st.lockedUntil) || st.count+st.inflight >= m.maxFail {
		return false
	}
	st.inflight++
	return true
}

// cancel 结束一次既非成功也非失败的尝试：只释放预占名额，保留已累计的失败次数。
func (m *Manager) cancel(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.fails[key]; st != nil && st.inflight > 0 {
		st.inflight--
	}
}

// release 结束一次尝试：成功时清除记录，失败时累计并在达到上限后锁定。
func (m *Manager) release(key string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.fails[key]
	if st == nil {
		return
	}
	st.inflight--
	now := m.now()
	if success {
		if st.inflight == 0 {
			delete(m.fails, key)
		}
		return
	}
	st.count++
	if st.count >= m.maxFail {
		st.lockedUntil = now.Add(m.lockDur)
		st.count = 0
		st.first = now
	}
	// 顺带清理过期记录，防止 map 无限增长
	for k, v := range m.fails {
		if v.inflight == 0 && now.Sub(v.first) > failWindow && now.After(v.lockedUntil) {
			delete(m.fails, k)
		}
	}
}

// verifyAccountPassword 校验账号设置操作中的当前密码，失败次数与登录一样受限。
func (m *Manager) verifyAccountPassword(password string) error {
	if !m.reserve(accountKey) {
		return ErrLocked
	}
	m.mu.RLock()
	hash := m.acc.PasswordHash
	m.mu.RUnlock()
	err := checkPassword(hash, password)
	m.release(accountKey, err == nil)
	if err != nil {
		return errors.New("原密码错误")
	}
	return nil
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

// TOTPEnabled 返回是否已启用两步验证。
func (m *Manager) TOTPEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.TOTPSecret != ""
}

// SetTOTP 启用或关闭两步验证。secret 为空表示关闭，关闭时还需提供当前验证码。
// 成功后轮换会话密钥，使此前签发的令牌全部失效。
func (m *Manager) SetTOTP(password, code, secret string) error {
	if err := m.verifyAccountPassword(password); err != nil {
		return err
	}
	m.mu.RLock()
	current := m.acc.TOTPSecret
	m.mu.RUnlock()
	if secret == "" && current != "" && !m.consumeTOTP(current, code) {
		return errors.New("两步验证码错误")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.acc
	next.TOTPSecret = secret
	next.SessionSecret = randomString(32)
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

// ---- 附加账号 ----

// ErrUserNotFound 表示附加账号不存在。
var ErrUserNotFound = errors.New("账号不存在")

// maxUsers 限制附加账号数量。
const maxUsers = 20

// UserView 是附加账号的对外信息（不含密码哈希）。
type UserView struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// Users 返回全部附加账号。
func (m *Manager) Users() []UserView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]UserView, 0, len(m.acc.Users))
	for _, u := range m.acc.Users {
		out = append(out, UserView{Username: u.Username, Role: u.Role})
	}
	return out
}

// AddUser 创建只读账号。
func (m *Manager) AddUser(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 64 || strings.ContainsAny(username, "\r\n\t") {
		return errors.New("用户名无效")
	}
	if len(password) < 8 {
		return errors.New("密码至少 8 位")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if username == m.acc.Username {
		return errors.New("用户名与管理员重复")
	}
	for _, u := range m.acc.Users {
		if u.Username == username {
			return errors.New("用户名已存在")
		}
	}
	if len(m.acc.Users) >= maxUsers {
		return errors.New("附加账号数量已达上限")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	next := m.acc
	next.Users = append(append([]User{}, m.acc.Users...), User{Username: username, PasswordHash: string(hash), Role: RoleReadonly})
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

// SetUserPassword 重置附加账号密码，该账号已有的会话随之失效。
func (m *Manager) SetUserPassword(username, password string) error {
	if len(password) < 8 {
		return errors.New("密码至少 8 位")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	next := m.acc
	next.Users = append([]User{}, m.acc.Users...)
	for i := range next.Users {
		if next.Users[i].Username == username {
			next.Users[i].PasswordHash = string(hash)
			if err := m.save(next); err != nil {
				return err
			}
			m.acc = next
			return nil
		}
	}
	return ErrUserNotFound
}

// DeleteUser 删除附加账号，其会话随之失效。
func (m *Manager) DeleteUser(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.acc
	next.Users = nil
	found := false
	for _, u := range m.acc.Users {
		if u.Username == username {
			found = true
			continue
		}
		next.Users = append(next.Users, u)
	}
	if !found {
		return ErrUserNotFound
	}
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

// ---- Passkey ----

// ErrPasskeyNotFound 表示 Passkey 不存在。
var ErrPasskeyNotFound = errors.New("Passkey 不存在")

// maxPasskeys 限制注册的 Passkey 数量。
const maxPasskeys = 10

// PasskeyView 是 Passkey 的对外信息。
type PasskeyView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

// Passkeys 返回已注册的 Passkey。
func (m *Manager) Passkeys() []PasskeyView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]PasskeyView, 0, len(m.acc.Passkeys))
	for _, c := range m.acc.Passkeys {
		out = append(out, PasskeyView{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt, LastUsed: c.LastUsed})
	}
	return out
}

// PasskeyIDs 返回全部凭据 ID，注册时用于排除已有的验证器。
func (m *Manager) PasskeyIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.acc.Passkeys))
	for _, c := range m.acc.Passkeys {
		ids = append(ids, c.ID)
	}
	return ids
}

// AddPasskey 校验当前密码后保存新的 Passkey。
func (m *Manager) AddPasskey(password string, c *webauthn.Credential) error {
	if err := m.verifyAccountPassword(password); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.acc.Passkeys) >= maxPasskeys {
		return errors.New("Passkey 数量已达上限")
	}
	for _, e := range m.acc.Passkeys {
		if e.ID == c.ID {
			return errors.New("该 Passkey 已注册")
		}
	}
	next := m.acc
	next.Passkeys = append(append([]webauthn.Credential{}, m.acc.Passkeys...), *c)
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

// RemovePasskey 校验当前密码后删除 Passkey。
func (m *Manager) RemovePasskey(password, id string) error {
	if err := m.verifyAccountPassword(password); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.acc
	next.Passkeys = nil
	found := false
	for _, c := range m.acc.Passkeys {
		if c.ID == id {
			found = true
			continue
		}
		next.Passkeys = append(next.Passkeys, c)
	}
	if !found {
		return ErrPasskeyNotFound
	}
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

// LoginPasskey 使用 Passkey 登录。verify 校验断言并返回新的签名计数；
// 失败与密码登录一样计入 IP 限流。Passkey 登录强制用户验证，因此无需再输入两步验证码。
func (m *Manager) LoginPasskey(ip, credID string, verify func(webauthn.Credential) (uint32, error)) (string, time.Time, error) {
	key := limitKey(ip)
	if !m.reserve(key) {
		return "", time.Time{}, ErrLocked
	}
	ok := false
	defer func() { m.release(key, ok) }()

	m.mu.RLock()
	var cred *webauthn.Credential
	for i := range m.acc.Passkeys {
		if m.acc.Passkeys[i].ID == credID {
			c := m.acc.Passkeys[i]
			cred = &c
			break
		}
	}
	m.mu.RUnlock()
	if cred == nil {
		return "", time.Time{}, ErrInvalidCredentials
	}
	count, err := verify(*cred)
	if err != nil {
		return "", time.Time{}, err
	}

	m.mu.Lock()
	next := m.acc
	next.Passkeys = append([]webauthn.Credential{}, m.acc.Passkeys...)
	for i := range next.Passkeys {
		if next.Passkeys[i].ID == credID {
			next.Passkeys[i].SignCount = count
			next.Passkeys[i].LastUsed = m.now()
		}
	}
	if err := m.save(next); err != nil {
		m.mu.Unlock()
		return "", time.Time{}, err
	}
	m.acc = next
	acc := m.acc
	m.mu.Unlock()

	ok = true
	exp := m.now().Add(time.Duration(acc.TokenTTLHours) * time.Hour)
	return m.sign(acc, exp, acc.Username, RoleAdmin, acc.PasswordHash), exp, nil
}

// UsingDefaultPassword 返回主账号是否仍在使用默认密码。
func (m *Manager) UsingDefaultPassword() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.UsingDefaultPassword
}

// OnboardingPending 返回是否需要展示首次启动引导。
func (m *Manager) OnboardingPending() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.OnboardingPending
}

// DismissOnboarding 标记首次启动引导已完成或被跳过。
func (m *Manager) DismissOnboarding() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.acc.OnboardingPending {
		return nil
	}
	next := m.acc
	next.OnboardingPending = false
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}
