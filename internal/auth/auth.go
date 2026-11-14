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
	// ErrBusy 表示陌生来源的登录失败总量超过全局预算，疑似分布式爆破：此时只拒绝从未成功登录过的来源，
	// 常用地址不受影响，窗口期过后自动恢复。
	ErrBusy = errors.New("检测到大量登录失败，来自陌生地址的登录暂时受限，请稍后再试，或改用常用网络登录")
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
	// WeakPassword 表示主账号当前密码不满足强度要求：密码只在登录时以明文出现，所以在密码登录成功时判定并记录，
	// 改成强密码后清除。仍使用默认密码时由 UsingDefaultPassword 表示
	WeakPassword bool `json:"weakPassword,omitempty"`
	// PasswordLoginOff 为 true 时主账号不能再用密码登录（只能用 Passkey 或第三方登录），
	// 从根本上取消管理员密码被爆破的可能；只能在已注册 Passkey 时开启，命令行重置密码会将其关闭
	PasswordLoginOff bool `json:"passwordLoginOff,omitempty"`
	// SessionEpoch 是会话纪元：令牌内记录签发时的纪元，“退出所有设备”使其加一，旧纪元的令牌全部失效
	SessionEpoch int64 `json:"sessionEpoch,omitempty"`
	// Revoked 是已吊销（已登出）的会话令牌，条目随令牌过期清理，数量有上限
	Revoked []RevokedToken `json:"revoked,omitempty"`
}

// RevokedToken 是一条已吊销的会话令牌记录：ID 取自令牌签名，Exp 为令牌过期时间（Unix 秒）。
type RevokedToken struct {
	ID  string `json:"id"`
	Exp int64  `json:"e"`
}

// maxRevoked 是吊销列表的容量上限。超出时提升会话纪元，让全部旧令牌一并失效，列表即可清空。
const maxRevoked = 1024

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
	// strikes 为 24 小时内已被锁定的次数：反复触发锁定的来源，每次锁定时长翻倍，直至 maxLockDuration
	strikes    int
	lastStrike time.Time
}

// maxLockDuration 是递增锁定的时长上限；strikeMemory 是锁定记录的保留时间，超过后从头计算。
const (
	maxLockDuration = 24 * time.Hour
	strikeMemory    = 24 * time.Hour
	// maxTracked 是限流表的条目上限：攻击者轮换大量地址时防止内存无限增长，超出时先清理过期条目
	maxTracked = 20000
	// globalWindow 是陌生来源全局失败预算的统计窗口
	globalWindow = 10 * time.Minute
)

// DefaultGlobalLimit 是陌生来源在一个窗口期内允许累计的登录失败次数（所有陌生地址合计）。
const DefaultGlobalLimit = 20

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
	// revoked 是 acc.Revoked 的集合形式，便于每次校验时快速查找
	revoked map[string]struct{}
	// 陌生来源的全局失败预算：known 判断来源是否曾成功登录过（为空表示不启用），globalLimit 为窗口内允许的失败总数
	known       func(ip string) bool
	globalLimit int
	onTrip      func(fails int)
	gStart      time.Time
	gFails      int
	gInflight   int
	gTripped    bool
	lastSweep   time.Time
}

// SetGlobalLimit 启用陌生来源的全局失败预算。known 返回来源地址是否曾成功登录过管理后台，
// limit 为 10 分钟窗口内所有陌生来源合计允许的失败次数，非正数表示关闭；
// onTrip 在预算首次耗尽时调用一次（窗口内不重复），用于记录日志与发出告警。
func (m *Manager) SetGlobalLimit(limit int, known func(ip string) bool, onTrip func(fails int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalLimit, m.known, m.onTrip = limit, known, onTrip
}

// rebuildRevoked 按账号中的吊销列表重建查找集合，调用时须持有写锁或尚未并发使用。
func (m *Manager) rebuildRevoked() {
	m.revoked = make(map[string]struct{}, len(m.acc.Revoked))
	for _, r := range m.acc.Revoked {
		m.revoked[r.ID] = struct{}{}
	}
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
	m := &Manager{acc: acc, save: save, fails: map[string]*failState{}, now: time.Now, maxFail: maxFailures, lockDur: lockDuration}
	m.rebuildRevoked()
	return m
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
		// 自动化部署指定的初始密码不够强时同样标记，首次登录后管理界面会要求修改
		m.acc.WeakPassword = password != DefaultPassword && CheckPasswordStrength(m.acc.Username, password) != nil
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
	global := m.isUnknown(ip)
	if err := m.reserve(key, global); err != nil {
		return "", time.Time{}, err
	}
	ok := false
	// pending 表示密码正确但还缺验证码：既不算失败也不算成功，只释放预占名额，不清除已累计的失败次数
	pending := false
	defer func() {
		if pending {
			m.cancel(key, global)
			return
		}
		m.release(key, ok, global)
	}()

	m.mu.RLock()
	acc := m.acc
	m.mu.RUnlock()

	// 关闭密码登录后主账号按“账号不存在”处理：仍执行一次 bcrypt 并返回统一的凭据错误，
	// 无法借响应差异得知该用户名存在或密码登录已关闭
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(acc.Username)) == 1 && !acc.PasswordLoginOff
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
		// 附加账号登录成功不清除失败计数：否则持有只读账号的人可以在每次猜测管理员密码之间登录一次来清零计数，
		// 从而无限次爆破
		pending = true
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
	// 密码正确：趁明文在手判定强度并记录，供管理界面提示或强制修改弱密码
	m.noteStrength(acc.Username, password)
	exp := m.now().Add(time.Duration(acc.TokenTTLHours) * time.Hour)
	return m.sign(acc, exp, acc.Username, RoleAdmin, acc.PasswordHash), exp, nil
}

// noteStrength 按刚验证通过的明文密码更新主账号的弱密码标记，状态变化时才写盘。
// 持久化失败不影响登录：标记留在内存里，下次登录再判定。
func (m *Manager) noteStrength(username, password string) {
	weakNow := CheckPasswordStrength(username, password) != nil
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.acc.WeakPassword == weakNow {
		return
	}
	next := m.acc
	next.WeakPassword = weakNow
	if err := m.save(next); err == nil {
		m.acc = next
	}
}

// isUnknown 判断来源是否需要计入陌生来源的全局失败预算。
func (m *Manager) isUnknown(ip string) bool {
	m.mu.RLock()
	known, limit := m.known, m.globalLimit
	m.mu.RUnlock()
	if known == nil || limit <= 0 {
		return false
	}
	return !known(ip)
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
	if newPassword == DefaultPassword {
		return errors.New("新密码不能使用默认密码")
	}
	if err := CheckPasswordStrength(newUsername, newPassword); err != nil {
		return err
	}
	if err := m.verifyAccountPassword(oldPassword); err != nil {
		return err
	}
	if newPassword == oldPassword {
		return errors.New("新密码不能与原密码相同")
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
	// 新密码已通过强度校验且不可能是默认密码，清除默认密码与弱密码标记
	next.UsingDefaultPassword = false
	next.WeakPassword = false
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
	// Epoch 是签发时的会话纪元，与账号当前纪元不同的令牌无效
	Epoch int64 `json:"n,omitempty"`
	// ID 是随机令牌编号，保证同一秒签发的令牌互不相同，吊销其一不会误伤其他
	ID string `json:"j,omitempty"`
}

// hashVersion 返回密码哈希的短摘要，用于把令牌绑定到账号当前的密码。
func hashVersion(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}

func (m *Manager) sign(acc Account, exp time.Time, user, role, hash string) string {
	pl := payload{User: user, Exp: exp.Unix(), Ver: hashVersion(hash), Epoch: acc.SessionEpoch, ID: randomString(12)}
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
	// 先解码签名并检查长度，格式不对的值（例如其他应用同名 Cookie）不必计算 HMAC。
	// Web 服务会对带后台 Cookie 的每个请求调用这里，用来标记管理员自己的访问日志
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || len(got) != sha256.Size {
		return Identity{}, false
	}
	m.mu.RLock()
	acc := m.acc
	m.mu.RUnlock()
	mac := hmac.New(sha256.New, []byte(acc.SessionSecret))
	mac.Write([]byte(p))
	if !hmac.Equal(got, mac.Sum(nil)) {
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
	// 纪元不符（已退出所有设备）或令牌已被吊销（已登出）
	if pl.Epoch != acc.SessionEpoch {
		return Identity{}, false
	}
	m.mu.RLock()
	_, gone := m.revoked[tokenID(sig)]
	m.mu.RUnlock()
	if gone {
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

// tokenID 返回令牌的吊销编号：取签名的前 22 个字符（约 132 位），签名由 HMAC 产生，无法被碰撞伪造。
func tokenID(sig string) string {
	if len(sig) > 22 {
		return sig[:22]
	}
	return sig
}

// Revoke 吊销一个会话令牌（登出），令牌无效或已过期时不做任何事。
// 吊销记录持久化，重启后仍然有效；记录在令牌过期后清理，数量达到上限时改为提升会话纪元。
func (m *Manager) Revoke(token string) error {
	ident, ok := m.Identify(token)
	if !ok {
		return nil
	}
	p, sig, _ := strings.Cut(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return nil
	}
	var pl payload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.acc
	now := m.now().Unix()
	kept := make([]RevokedToken, 0, len(m.acc.Revoked)+1)
	for _, r := range m.acc.Revoked {
		if r.Exp >= now {
			kept = append(kept, r)
		}
	}
	if ident.Role != RoleAdmin && len(kept) >= maxRevoked/2 {
		// 只读账号登出只占用一半容量：否则只读账号反复登录、登出即可填满列表，触发下面的提升纪元把管理员全部踢下线。
		// 超出时不记录，该只读令牌照常到期失效
		return nil
	}
	if len(kept) >= maxRevoked {
		// 列表已满：让全部旧令牌失效，比丢弃记录更安全
		next.SessionEpoch++
		kept = nil
	} else {
		kept = append(kept, RevokedToken{ID: tokenID(sig), Exp: pl.Exp})
	}
	next.Revoked = kept
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	m.rebuildRevoked()
	return nil
}

// RevokeAll 使此前签发的全部会话令牌失效（退出所有设备），API 令牌等其他凭据不受影响。
func (m *Manager) RevokeAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.acc
	next.SessionEpoch++
	next.Revoked = nil
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	m.rebuildRevoked()
	return nil
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

// reserve 检查是否锁定并预占一次尝试；返回非 nil 表示应拒绝：
// ErrLocked 为该来源被锁定，ErrBusy 为陌生来源的全局失败预算已耗尽（global 为 true 时才检查）。
func (m *Manager) reserve(key string, global bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	st := m.fails[key]
	if st == nil || (now.Sub(st.first) > failWindow && st.inflight == 0) {
		if st != nil && now.Before(st.lockedUntil) {
			return ErrLocked
		}
		if st == nil && len(m.fails) >= maxTracked {
			m.sweep(now, true)
			if len(m.fails) >= maxTracked {
				return ErrBusy
			}
		}
		ns := &failState{first: now}
		if st != nil {
			// 窗口过期重新计数时保留递增锁定的记录
			ns.strikes, ns.lastStrike = st.strikes, st.lastStrike
		}
		st = ns
		m.fails[key] = st
	}
	if now.Before(st.lockedUntil) || st.count+st.inflight >= m.maxFail {
		return ErrLocked
	}
	if global {
		if now.Sub(m.gStart) > globalWindow && m.gInflight == 0 {
			m.gStart, m.gFails, m.gTripped = now, 0, false
		}
		if m.gFails+m.gInflight >= m.globalLimit {
			if !m.gTripped {
				m.gTripped = true
				if m.onTrip != nil {
					go m.onTrip(m.gFails)
				}
			}
			return ErrBusy
		}
		m.gInflight++
	}
	st.inflight++
	return nil
}

// cancel 结束一次既非成功也非失败的尝试：只释放预占名额，保留已累计的失败次数。
func (m *Manager) cancel(key string, global bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.fails[key]; st != nil && st.inflight > 0 {
		st.inflight--
	}
	if global && m.gInflight > 0 {
		m.gInflight--
	}
}

// release 结束一次尝试：成功时清除记录，失败时累计并在达到上限后锁定。
// 反复被锁定的来源每次锁定时长翻倍（上限 24 小时），低频但持续的爆破因此越试越慢。
func (m *Manager) release(key string, success, global bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if global {
		if m.gInflight > 0 {
			m.gInflight--
		}
		if !success {
			m.gFails++
		}
	}
	st := m.fails[key]
	if st == nil {
		return
	}
	if st.inflight > 0 {
		st.inflight--
	}
	now := m.now()
	if success {
		// 成功登录清除失败计数；递增锁定的记录仍保留到过期，避免靠一次成功登录洗掉历史
		st.count = 0
		if st.inflight == 0 && st.strikes == 0 {
			delete(m.fails, key)
		}
		return
	}
	st.count++
	if st.count >= m.maxFail {
		if now.Sub(st.lastStrike) > strikeMemory {
			st.strikes = 0
		}
		st.strikes++
		st.lastStrike = now
		limit := maxLockDuration
		if m.lockDur > limit {
			limit = m.lockDur
		}
		d := m.lockDur
		for i := 1; i < st.strikes && d < limit; i++ {
			d *= 2
		}
		if d > limit {
			d = limit
		}
		st.lockedUntil = now.Add(d)
		st.count = 0
		st.first = now
	}
	m.sweep(now, false)
}

// sweep 清理过期的限流记录，防止 map 无限增长；平时一分钟最多扫描一次，表满时（force）立即扫描。
func (m *Manager) sweep(now time.Time, force bool) {
	if !force && now.Sub(m.lastSweep) < time.Minute {
		return
	}
	m.lastSweep = now
	for k, v := range m.fails {
		if v.inflight == 0 && now.Sub(v.first) > failWindow && now.After(v.lockedUntil) && (v.strikes == 0 || now.Sub(v.lastStrike) > strikeMemory) {
			delete(m.fails, k)
		}
	}
}

// verifyAccountPassword 校验账号设置操作中的当前密码，失败次数与登录一样受限。
func (m *Manager) verifyAccountPassword(password string) error {
	if err := m.reserve(accountKey, false); err != nil {
		return err
	}
	m.mu.RLock()
	hash := m.acc.PasswordHash
	m.mu.RUnlock()
	err := checkPassword(hash, password)
	m.release(accountKey, err == nil, false)
	if err != nil {
		return errors.New("原密码错误")
	}
	return nil
}

// NewSessionSecret 生成新的会话签名密钥。恢复备份时用它替换备份中的密钥，让恢复前签发的令牌全部失效。
func NewSessionSecret() string {
	return randomString(32)
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
	if err := CheckPasswordStrength(username, password); err != nil {
		return err
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
	if err := CheckPasswordStrength(username, password); err != nil {
		return err
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
	// 已关闭密码登录时 Passkey 是唯一的本地登录方式，不能删光
	if m.acc.PasswordLoginOff && len(next.Passkeys) == 0 {
		return errors.New("已关闭密码登录，不能删除最后一个 Passkey，请先重新开启密码登录")
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
	global := m.isUnknown(ip)
	if err := m.reserve(key, global); err != nil {
		return "", time.Time{}, err
	}
	ok := false
	defer func() { m.release(key, ok, global) }()

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

// PasswordIssue 返回主账号密码存在的问题：default 为仍在使用默认密码，weak 为不满足强度要求，空串为无问题。
// 有问题时管理界面强制修改密码，服务端同时拒绝除修改密码之外的写操作。
func (m *Manager) PasswordIssue() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.passwordIssueLocked()
}

func (m *Manager) passwordIssueLocked() string {
	// 关闭密码登录后密码不能再用来登录，弱密码不构成风险
	if m.acc.PasswordLoginOff {
		return ""
	}
	switch {
	case m.acc.UsingDefaultPassword:
		return "default"
	case m.acc.WeakPassword:
		return "weak"
	}
	return ""
}

// PasswordLoginOff 返回主账号是否已关闭密码登录。
func (m *Manager) PasswordLoginOff() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acc.PasswordLoginOff
}

// SetPasswordLoginOff 关闭或重新开启主账号的密码登录，需要当前密码；关闭前必须已注册 Passkey，否则会把自己锁在外面。
func (m *Manager) SetPasswordLoginOff(password string, off bool) error {
	if err := m.verifyAccountPassword(password); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.acc.PasswordLoginOff == off {
		return nil
	}
	if off && len(m.acc.Passkeys) == 0 {
		return errors.New("请先注册 Passkey，再关闭密码登录")
	}
	next := m.acc
	next.PasswordLoginOff = off
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	return nil
}

// Snapshot 是账号安全状态的只读快照，供安全评分使用（不含任何凭据）。
type Snapshot struct {
	Username         string
	PasswordIssue    string
	TOTP             bool
	Passkeys         int
	PasswordLoginOff bool
	MaxFail          int
	LockDuration     time.Duration
}

// Snapshot 返回当前账号安全状态。
func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Snapshot{Username: m.acc.Username, PasswordIssue: m.passwordIssueLocked(), TOTP: m.acc.TOTPSecret != "", Passkeys: len(m.acc.Passkeys),
		PasswordLoginOff: m.acc.PasswordLoginOff, MaxFail: m.maxFail, LockDuration: m.lockDur}
}
