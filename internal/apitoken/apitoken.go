// Package apitoken 实现可多个、可限定权限的 API 令牌。
//
// 令牌明文只在创建（或重新生成）时返回一次，磁盘上只保存 SHA-256 哈希。
// 令牌格式为 "gtl_" + 随机串；升级自旧版的单个全局令牌保留原值，标记为遗留令牌。
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
)

// StoreName 是令牌在配置目录中的文件名（tokens.json）。
const StoreName = "tokens"

// Prefix 是新令牌的固定前缀，便于在代码仓库扫描中识别泄露。
const Prefix = "gtl_"

const (
	maxTokens   = 100
	maxNameLen  = 64
	flushPeriod = time.Minute
)

var (
	// ErrInvalid 表示令牌不存在、已停用或已过期。
	ErrInvalid = errors.New("API 令牌无效或已过期")
	// ErrNotFound 表示令牌记录不存在。
	ErrNotFound = errors.New("令牌不存在")
)

// Token 是持久化的令牌记录。
type Token struct {
	ID string `json:"id"`
	// Name 是用户给令牌起的用途名称
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group string `json:"group,omitempty"`
	// Hash 为令牌明文的 SHA-256（十六进制）
	Hash string `json:"hash"`
	// Hint 为明文的前几位与末尾几位，只用于在列表中辨认
	Hint   string   `json:"hint"`
	Scopes []string `json:"scopes"`
	// Legacy 标记由旧版全局令牌迁移而来
	Legacy    bool       `json:"legacy,omitempty"`
	Disabled  bool       `json:"disabled,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// LastUsedAt / LastUsedIP 记录最近一次成功认证，磁盘写入有节流，可能滞后至多一分钟
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	LastUsedIP string     `json:"lastUsedIP,omitempty"`
}

// View 是返回给界面的令牌信息，不含哈希。
type View struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Group      string     `json:"group"`
	Hint       string     `json:"hint"`
	Scopes     []string   `json:"scopes"`
	Legacy     bool       `json:"legacy"`
	Disabled   bool       `json:"disabled"`
	Expired    bool       `json:"expired"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	LastUsedIP string     `json:"lastUsedIP"`
}

type fileData struct {
	Tokens []Token `json:"tokens"`
	// Ordered 为 true 表示用户手动调整过顺序，列表按保存顺序返回；否则按创建时间从新到旧
	Ordered bool `json:"ordered,omitempty"`
}

// Manager 管理令牌的增删改查与认证。
type Manager struct {
	mu      sync.Mutex
	store   *config.Store
	data    fileData
	domains map[string]bool
	now     func() time.Time
	// dirty 表示最近使用信息尚未落盘
	dirty     bool
	lastFlush time.Time
}

// Open 加载令牌文件。domains 是允许出现在权限中的权限域（模块 ID 与内置域）。
func Open(store *config.Store, domains []string) (*Manager, error) {
	m := &Manager{store: store, now: time.Now, domains: map[string]bool{}}
	m.SetDomains(domains)
	if _, err := store.Load(StoreName, &m.data); err != nil {
		return nil, err
	}
	m.lastFlush = m.now()
	return m, nil
}

// SetDomains 设置有效的权限域。
func (m *Manager) SetDomains(domains []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domains = map[string]bool{}
	for _, d := range domains {
		m.domains[d] = true
	}
}

func hashOf(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generate() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return Prefix + hex.EncodeToString(b)
}

func hintOf(raw string) string {
	if len(raw) <= 10 {
		return raw[:min(len(raw), 4)] + "…"
	}
	return raw[:len(Prefix)+4] + "…" + raw[len(raw)-4:]
}

func (m *Manager) saveLocked() error {
	return m.store.Save(StoreName, m.data)
}

func (m *Manager) view(t Token) View {
	return View{
		ID: t.ID, Name: t.Name, Group: t.Group, Hint: t.Hint, Scopes: slices.Clone(t.Scopes), Legacy: t.Legacy, Disabled: t.Disabled,
		Expired:   t.ExpiresAt != nil && !m.now().Before(*t.ExpiresAt),
		CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, LastUsedIP: t.LastUsedIP,
	}
}

// List 返回全部令牌：手动调整过顺序时按保存顺序，否则按创建时间从新到旧。
func (m *Manager) List() []View {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]View, 0, len(m.data.Tokens))
	for _, t := range m.data.Tokens {
		out = append(out, m.view(t))
	}
	if !m.data.Ordered {
		slices.SortStableFunc(out, func(a, b View) int { return b.CreatedAt.Compare(a.CreatedAt) })
	}
	return out
}

// Input 是创建或修改令牌的参数。
type Input struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	// ExpiresAt 为空表示永不过期
	ExpiresAt *time.Time `json:"expiresAt"`
	Disabled  bool       `json:"disabled"`
	// Group 为分组名称；修改时不提供表示保持原分组
	Group *string `json:"group,omitempty"`
}

// normalize 校验并规范化输入；checkExpiry 为 false 时不要求过期时间晚于当前（保留已过期令牌的原过期时间）。
func (m *Manager) normalize(in *Input, checkExpiry bool) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("请填写令牌名称")
	}
	if len([]rune(in.Name)) > maxNameLen {
		return fmt.Errorf("令牌名称不能超过 %d 个字符", maxNameLen)
	}
	if len(in.Scopes) == 0 {
		return errors.New("至少选择一项权限")
	}
	seen := map[string]bool{}
	var scopes []string
	for _, s := range in.Scopes {
		s = strings.TrimSpace(s)
		if !ValidScope(s, m.domains) {
			return fmt.Errorf("权限 %q 无效", s)
		}
		if !seen[s] {
			seen[s] = true
			scopes = append(scopes, s)
		}
	}
	slices.Sort(scopes)
	in.Scopes = scopes
	if in.Group != nil {
		if err := crud.NormalizeGroup(in.Group); err != nil {
			return err
		}
	}
	if checkExpiry && in.ExpiresAt != nil && !in.ExpiresAt.After(m.now()) {
		return errors.New("过期时间必须晚于当前时间")
	}
	return nil
}

// Create 新建令牌，返回记录与仅此一次可见的明文。
func (m *Manager) Create(in Input) (View, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.normalize(&in, true); err != nil {
		return View{}, "", err
	}
	if len(m.data.Tokens) >= maxTokens {
		return View{}, "", fmt.Errorf("令牌数量已达上限 %d", maxTokens)
	}
	raw := generate()
	t := Token{ID: newID(), Name: in.Name, Hash: hashOf(raw), Hint: hintOf(raw), Scopes: in.Scopes,
		Disabled: in.Disabled, CreatedAt: m.now(), ExpiresAt: in.ExpiresAt}
	if in.Group != nil {
		t.Group = *in.Group
	}
	old := m.data.Tokens
	if m.data.Ordered {
		// 手动排序后新令牌放在最前，与按创建时间排序时的位置一致
		m.data.Tokens = append([]Token{t}, old...)
	} else {
		m.data.Tokens = append(slices.Clone(old), t)
	}
	if err := m.saveLocked(); err != nil {
		m.data.Tokens = old
		return View{}, "", err
	}
	return m.view(t), raw, nil
}

func (m *Manager) index(id string) int {
	return slices.IndexFunc(m.data.Tokens, func(t Token) bool { return t.ID == id })
}

// Update 修改名称、权限、过期时间与启停，不改变令牌明文。
func (m *Manager) Update(id string, in Input) (View, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.index(id)
	if i < 0 {
		return View{}, ErrNotFound
	}
	old := m.data.Tokens[i]
	// 已过期的令牌可以保持原过期时间而只修改其他字段
	unchanged := in.ExpiresAt != nil && old.ExpiresAt != nil && in.ExpiresAt.Equal(*old.ExpiresAt)
	if err := m.normalize(&in, !unchanged); err != nil {
		return View{}, err
	}
	next := old
	next.Name, next.Scopes, next.ExpiresAt, next.Disabled = in.Name, in.Scopes, in.ExpiresAt, in.Disabled
	if in.Group != nil {
		next.Group = *in.Group
	}
	m.data.Tokens[i] = next
	if err := m.saveLocked(); err != nil {
		m.data.Tokens[i] = old
		return View{}, err
	}
	return m.view(next), nil
}

// Order 按 ids 的顺序重排全部令牌，之后列表按此顺序返回；ids 必须恰好包含全部令牌。
func (m *Manager) Order(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(ids) != len(m.data.Tokens) {
		return errors.New("排序列表与条目数量不一致")
	}
	next := make([]Token, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		i := m.index(id)
		if i < 0 || seen[id] {
			return fmt.Errorf("排序列表包含未知或重复条目 %s", id)
		}
		seen[id] = true
		next = append(next, m.data.Tokens[i])
	}
	old, oldOrdered := m.data.Tokens, m.data.Ordered
	m.data.Tokens, m.data.Ordered = next, true
	if err := m.saveLocked(); err != nil {
		m.data.Tokens, m.data.Ordered = old, oldOrdered
		return err
	}
	return nil
}

// SetGroup 把多个令牌移到同一分组，留空表示移出分组；顺序不变。
func (m *Manager) SetGroup(ids []string, group string) error {
	if len(ids) == 0 {
		return errors.New("请选择要设置分组的条目")
	}
	if err := crud.NormalizeGroup(&group); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := make([]int, 0, len(ids))
	for _, id := range ids {
		i := m.index(id)
		if i < 0 {
			return ErrNotFound
		}
		idx = append(idx, i)
	}
	old := slices.Clone(m.data.Tokens)
	for _, i := range idx {
		m.data.Tokens[i].Group = group
	}
	if err := m.saveLocked(); err != nil {
		m.data.Tokens = old
		return err
	}
	return nil
}

// Regenerate 为令牌生成新的明文，旧明文立即失效。
func (m *Manager) Regenerate(id string) (View, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.index(id)
	if i < 0 {
		return View{}, "", ErrNotFound
	}
	old := m.data.Tokens[i]
	raw := generate()
	next := old
	next.Hash, next.Hint, next.Legacy = hashOf(raw), hintOf(raw), false
	next.LastUsedAt, next.LastUsedIP = nil, ""
	m.data.Tokens[i] = next
	if err := m.saveLocked(); err != nil {
		m.data.Tokens[i] = old
		return View{}, "", err
	}
	return m.view(next), raw, nil
}

// Delete 删除令牌，返回被删除令牌的名称用于审计日志。
func (m *Manager) Delete(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.index(id)
	if i < 0 {
		return "", ErrNotFound
	}
	old := slices.Clone(m.data.Tokens)
	name := m.data.Tokens[i].Name
	m.data.Tokens = slices.Delete(m.data.Tokens, i, i+1)
	if err := m.saveLocked(); err != nil {
		m.data.Tokens = old
		return "", err
	}
	return name, nil
}

// ImportLegacy 把旧版全局令牌明文迁移为一条拥有全部权限的遗留令牌；已迁移过（哈希相同）则忽略。
func (m *Manager) ImportLegacy(raw string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := hashOf(raw)
	if slices.ContainsFunc(m.data.Tokens, func(t Token) bool { return t.Hash == h }) {
		return nil
	}
	t := Token{ID: newID(), Name: "遗留令牌（由旧版迁移）", Hash: h, Hint: hintOf(raw), Scopes: []string{"*"},
		Legacy: true, CreatedAt: m.now()}
	m.data.Tokens = append(m.data.Tokens, t)
	if err := m.saveLocked(); err != nil {
		m.data.Tokens = m.data.Tokens[:len(m.data.Tokens)-1]
		return err
	}
	return nil
}

// Authenticate 校验令牌明文，成功时返回令牌记录副本并记录最近使用。
func (m *Manager) Authenticate(raw, ip string) (Token, error) {
	if raw == "" {
		return Token{}, ErrInvalid
	}
	h := hashOf(raw)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for i := range m.data.Tokens {
		t := &m.data.Tokens[i]
		// 令牌是高熵随机串，对哈希做比较即可，无需常量时间
		if t.Hash != h {
			continue
		}
		if t.Disabled || (t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)) {
			return Token{}, ErrInvalid
		}
		t.LastUsedAt, t.LastUsedIP = &now, ip
		m.dirty = true
		// 最近使用信息每分钟最多落盘一次，避免高频调用时频繁写文件
		if now.Sub(m.lastFlush) >= flushPeriod {
			m.flushLocked()
		}
		return *t, nil
	}
	return Token{}, ErrInvalid
}

func (m *Manager) flushLocked() {
	if !m.dirty {
		return
	}
	if err := m.saveLocked(); err == nil {
		m.dirty = false
		m.lastFlush = m.now()
	}
}

// Flush 立即写入尚未落盘的最近使用信息，程序退出前调用。
func (m *Manager) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushLocked()
}

// Peek 判断令牌明文当前是否有效，不记录最近使用。
func (m *Manager) Peek(raw string) bool {
	if raw == "" {
		return false
	}
	h := hashOf(raw)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.data.Tokens {
		if t.Hash == h {
			return !t.Disabled && (t.ExpiresAt == nil || m.now().Before(*t.ExpiresAt))
		}
	}
	return false
}
