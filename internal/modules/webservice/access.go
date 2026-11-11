package webservice

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/geoip"
)

// 国家过滤模式
const (
	CountryOff   = ""
	CountryAllow = "allow"
	CountryDeny  = "deny"
)

// 路径规则的认证覆盖
const (
	RuleAuthInherit = ""
	RuleAuthOff     = "off"
	RuleAuthOn      = "on"
)

const (
	// shareParam 是分享链接携带令牌的查询参数
	shareParam = "_gs"
	// shareMaxSession 是通过分享链接建立的会话的最长有效期
	shareMaxSession = 24 * time.Hour
	maxBanDuration  = 30 * 24 * time.Hour
)

// ShareLink 是站点的临时分享链接：持有令牌的访客无需账号即可在有效期内访问。
type ShareLink struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Token 是访问令牌，访问 https://域名/任意路径?_gs=令牌 即可兑换会话
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
	// MaxUses 为最多可被兑换的次数，0 表示不限；重启后计数清零
	MaxUses int `json:"maxUses"`
	// PathPrefix 非空时，链接只能访问该前缀下的路径
	PathPrefix string `json:"pathPrefix"`
}

// PathRule 为某个路径前缀覆盖站点的认证与 IP 访问控制。最长前缀优先。
type PathRule struct {
	Prefix string `json:"prefix"`
	// Auth 为 ""（沿用站点）、off（无需认证）、on（必须认证）
	Auth   string   `json:"auth"`
	IPMode string   `json:"ipMode"`
	IPList []string `json:"ipList"`
	// IPSets 为引用的订阅 IP 列表 key
	IPSets []string `json:"ipSets"`
}

func (l *ShareLink) expired(now time.Time) bool {
	return !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt)
}

func (l *ShareLink) covers(p string) bool {
	return l.PathPrefix == "" || prefixMatch(l.PathPrefix, cleanReqPath(p))
}

// cleanReqPath 规范化请求路径（消除 . / .. / 重复斜杠），避免用 /public/../admin 绕过按前缀的规则。
func cleanReqPath(p string) string {
	if p == "" {
		return "/"
	}
	return path.Clean("/" + p)
}

// prefixMatch 按路径段边界匹配：/admin 匹配 /admin 与 /admin/x，不匹配 /administrator。
func prefixMatch(prefix, p string) bool {
	if prefix == "/" {
		return true
	}
	prefix = strings.TrimRight(prefix, "/")
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validateAccess 校验并规范化站点的国家过滤、路径规则与分享链接。
func (s *Site) validateAccess() error {
	switch s.CountryMode {
	case CountryOff, CountryAllow, CountryDeny:
	default:
		return fmt.Errorf("未知的国家过滤模式 %q", s.CountryMode)
	}
	var codes []string
	seen := map[string]bool{}
	for _, c := range s.Countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || seen[c] {
			continue
		}
		if len(c) != 2 || c[0] < 'A' || c[0] > 'Z' || c[1] < 'A' || c[1] > 'Z' {
			return fmt.Errorf("国家代码 %q 无效，需为两位 ISO 代码（如 CN、US）", c)
		}
		seen[c] = true
		codes = append(codes, c)
	}
	s.Countries = codes
	if s.CountryMode != CountryOff && len(codes) == 0 {
		return errors.New("启用国家过滤时至少填写一个国家代码")
	}

	rules := map[string]bool{}
	for i := range s.Rules {
		r := &s.Rules[i]
		r.Prefix = strings.TrimSpace(r.Prefix)
		if !strings.HasPrefix(r.Prefix, "/") {
			return fmt.Errorf("路径规则 %q 需以 / 开头", r.Prefix)
		}
		if r.Prefix != "/" {
			r.Prefix = strings.TrimRight(r.Prefix, "/")
		}
		if rules[r.Prefix] {
			return fmt.Errorf("路径规则 %s 重复", r.Prefix)
		}
		rules[r.Prefix] = true
		switch r.Auth {
		case RuleAuthInherit, RuleAuthOff, RuleAuthOn:
		default:
			return fmt.Errorf("路径规则 %s 的认证设置无效", r.Prefix)
		}
		switch r.IPMode {
		case IPModeOff, IPModeWhitelist, IPModeBlacklist:
		default:
			return fmt.Errorf("路径规则 %s 的访问控制模式无效", r.Prefix)
		}
		r.IPList = cleanList(r.IPList)
		r.IPSets = uniqueList(r.IPSets)
		if _, err := parsePrefixes(r.IPList); err != nil {
			return fmt.Errorf("路径规则 %s: %w", r.Prefix, err)
		}
	}
	hasAuth := s.BasicAuth || s.WebAuth || strings.TrimSpace(s.ForwardAuth.URL) != ""
	// 转发登录 Cookie 时认证交给后端 Gately，本站点再开认证会把本机签发的凭据交给后端
	if s.PassAuthCookies && hasAuth {
		return errors.New("转发 Gately 登录 Cookie 时本站点不能启用认证，认证由后端 Gately 负责")
	}
	// 信任已认证用户只认 Gately 自己的认证，没有启用基本认证或网页认证时不生效，直接关闭，
	// 避免关闭认证（包括批量操作）后残留一个看不到的开关
	if s.TrustAuthUsers && !s.BasicAuth && !s.WebAuth {
		s.TrustAuthUsers = false
	}
	if s.AuthOptional && !hasAuth {
		return errors.New("仅对指定路径认证需要先启用基本认证、网页认证或转发认证")
	}

	if len(s.ShareLinks) > 0 && !hasAuth {
		return errors.New("分享链接用于绕过认证，请先启用基本认证、网页认证或转发认证")
	}
	keys, tokens := map[string]bool{}, map[string]bool{}
	for i := range s.ShareLinks {
		l := &s.ShareLinks[i]
		l.Name = strings.TrimSpace(l.Name)
		if l.Key == "" || keys[l.Key] {
			l.Key = randomToken()[:8]
		}
		keys[l.Key] = true
		if len(l.Token) < 16 {
			l.Token = randomToken()
		}
		if tokens[l.Token] {
			return fmt.Errorf("分享链接 %s 的令牌重复", l.Name)
		}
		tokens[l.Token] = true
		if l.MaxUses < 0 {
			return errors.New("可用次数不能为负数")
		}
		l.PathPrefix = strings.TrimSpace(l.PathPrefix)
		if l.PathPrefix != "" && !strings.HasPrefix(l.PathPrefix, "/") {
			return fmt.Errorf("分享链接路径 %q 需以 / 开头", l.PathPrefix)
		}
	}
	return nil
}

// compiledRule 是预编译的路径规则。
type compiledRule struct {
	prefix string
	auth   string
	ipMode string
	ipList []netip.Prefix
	ipSets []string
}

func compileRules(rules []PathRule) ([]compiledRule, error) {
	out := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		list, err := parsePrefixes(r.IPList)
		if err != nil {
			return nil, err
		}
		out = append(out, compiledRule{prefix: r.Prefix, auth: r.Auth, ipMode: r.IPMode, ipList: list, ipSets: r.IPSets})
	}
	// 最长前缀优先
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].prefix) > len(out[j].prefix) })
	return out, nil
}

// matchRule 返回与请求路径匹配的最长前缀规则。
func (h *siteHandler) matchRule(reqPath string) *compiledRule {
	if len(h.rules) == 0 {
		return nil
	}
	p := cleanReqPath(reqPath)
	for i := range h.rules {
		if prefixMatch(h.rules[i].prefix, p) {
			return &h.rules[i]
		}
	}
	return nil
}

// needAuth 判断该请求是否需要认证。
func (h *siteHandler) needAuth(rule *compiledRule) bool {
	if !h.site.BasicAuth && !h.site.WebAuth && h.fwd == nil {
		return false
	}
	if rule != nil {
		switch rule.auth {
		case RuleAuthOff:
			return false
		case RuleAuthOn:
			return true
		}
	}
	return !h.site.AuthOptional
}

// countryAllowed 按国家过滤设置判断客户端是否允许访问。
// 内网地址始终放行；数据库未加载时放行（避免误锁全部访客），界面会提示。
func (h *siteHandler) countryAllowed(a netip.Addr) bool {
	if h.site.CountryMode == CountryOff || geoip.IsLocal(a) {
		return true
	}
	if !geoip.Default.Info().Loaded {
		return true
	}
	code := geoip.Default.Country(a)
	in := h.countries[code]
	if h.site.CountryMode == CountryAllow {
		return in
	}
	// 黑名单模式下，库中查不到的地址放行
	return code == "" || !in
}

// shareSession 是通过分享链接建立的会话 Cookie 内容。
type shareSession struct {
	Link  string `json:"l"`
	Host  string `json:"h"`
	Scope string `json:"s"`
}

func shareCookieName(s Site) string {
	return cookieName(s) + "_sh"
}

func (h *siteHandler) findShareByToken(token string) *ShareLink {
	var found *ShareLink
	// 遍历全部链接并使用常量时间比较，避免通过耗时推断令牌
	for i := range h.site.ShareLinks {
		l := &h.site.ShareLinks[i]
		if subtle.ConstantTimeCompare([]byte(l.Token), []byte(token)) == 1 {
			found = l
		}
	}
	return found
}

func (h *siteHandler) findShareByKey(key string) *ShareLink {
	for i := range h.site.ShareLinks {
		if h.site.ShareLinks[i].Key == key {
			return &h.site.ShareLinks[i]
		}
	}
	return nil
}

// tryShare 处理分享链接。handled 为 true 表示已写出响应（兑换后重定向）；allowed 为 true 表示 Cookie 有效，可直接放行。
func (h *siteHandler) tryShare(w http.ResponseWriter, r *http.Request) (handled, allowed bool) {
	if len(h.site.ShareLinks) == 0 {
		return false, false
	}
	now := time.Now()
	if token := r.URL.Query().Get(shareParam); token != "" {
		key := limitKey(clientAddr(r).Addr().String())
		if !h.auth.reserve(key) {
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return true, false
		}
		link := h.findShareByToken(token)
		ok := link != nil && !link.expired(now) && link.covers(r.URL.Path) && h.auth.takeShareUse(h.site.scope+"|"+link.Key, link.MaxUses)
		h.auth.release(key, ok)
		if !ok {
			// 令牌无效时按普通访问处理（通常会跳转到登录页），不透露失败原因
			return false, false
		}
		ttl := shareMaxSession
		if !link.ExpiresAt.IsZero() && time.Until(link.ExpiresAt) < ttl {
			ttl = time.Until(link.ExpiresAt)
		}
		sess := shareSession{Link: link.Key, Host: hostOnly(r.Host), Scope: h.site.scope}
		http.SetCookie(w, &http.Cookie{
			Name: shareCookieName(h.site), Value: h.auth.signBlob("share", sess, ttl),
			Path: "/", Expires: now.Add(ttl), HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
		})
		h.reqLog(r).Infof("分享链接「%s」被兑换（%s）", link.Name, clientAddr(r).Addr())
		q := r.URL.Query()
		q.Del(shareParam)
		target := *r.URL
		target.RawQuery = q.Encode()
		http.Redirect(w, r, (&url.URL{Path: target.Path, RawQuery: target.RawQuery}).String(), http.StatusFound)
		return true, false
	}
	c, err := r.Cookie(shareCookieName(h.site))
	if err != nil {
		return false, false
	}
	var sess shareSession
	if h.auth.verifyBlob("share", c.Value, &sess) != nil || sess.Host != hostOnly(r.Host) || sess.Scope != h.site.scope {
		return false, false
	}
	link := h.findShareByKey(sess.Link)
	if link == nil || link.expired(now) || !link.covers(r.URL.Path) {
		return false, false
	}
	return false, true
}

// takeShareUse 登记一次兑换；达到上限时返回 false。max 为 0 表示不限。
func (a *authenticator) takeShareUse(key string, max int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if max > 0 && a.shareUses[key] >= max {
		return false
	}
	a.shareUses[key]++
	return true
}

// moveShareUses 把分享链接的已兑换次数从旧的站点作用域迁到新的作用域，renamed 的键与值为“监听key/站点key”。
func (a *authenticator) moveShareUses(renamed map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 先收集再改写，避免遍历中新增的键被再次处理
	moved := map[string]int{}
	for k, n := range a.shareUses {
		scope, link, ok := strings.Cut(k, "|")
		if !ok {
			continue
		}
		if to, hit := renamed[scope]; hit {
			delete(a.shareUses, k)
			moved[to+"|"+link] += n
		}
	}
	for k, n := range moved {
		a.shareUses[k] += n
	}
}

// ---- IP 封禁 ----
// 封禁记录保存在进程共享的封禁表（internal/banlist）中：同一张表也对端口转发、STUN 等服务生效，
// 入侵防护模块开启时负责持久化并同步到系统防火墙。

// BanInfo 是封禁列表中的一项。
type BanInfo struct {
	// Key 为被封禁的地址（IPv6 按 /64 聚合）或网段
	Key   string    `json:"key"`
	Until time.Time `json:"until,omitzero"`
	// Permanent 为 true 表示永久封禁（Until 为空）
	Permanent bool   `json:"permanent,omitempty"`
	Reason    string `json:"reason"`
	Manual    bool   `json:"manual"`
	// Source 为封禁来源，例如 webservice、manual、trap、scan 或 jail:<规则>
	Source string `json:"source"`
}

// setBanSeconds 设置认证失败达到上限后的自动封禁时长，0 表示不自动封禁。
func (a *authenticator) setBanSeconds(sec int) {
	a.banMu.Lock()
	a.autoBan = time.Duration(sec) * time.Second
	a.banMu.Unlock()
}

// banned 判断地址当前是否被封禁。
func (a *authenticator) banned(addr netip.Addr) bool {
	return a.bans.Banned(addr)
}

// ban 封禁地址 d 时长（d 小于 0 为永久），返回规范化后的键；manual 为 true 时不受入侵防护白名单限制。
// 地址在白名单中而未被封禁时返回空字符串。
func (a *authenticator) ban(ip string, d time.Duration, reason string, manual bool) string {
	key := limitKey(ip)
	source := banlist.SourceWeb
	if manual {
		source = banlist.SourceManual
	}
	// d 小于 0 表示永久封禁（手动封禁接口的 -1）
	if _, _, err := a.bans.Ban(key, banlist.Options{Duration: d, Permanent: d < 0, Reason: reason, Source: source, Force: manual}); err != nil {
		return ""
	}
	return key
}

// unban 解除封禁。
func (a *authenticator) unban(key string) {
	a.bans.Unban(key)
}

// listBans 返回尚未过期的封禁，按到期时间排序（永久封禁在最后）。
func (a *authenticator) listBans() []BanInfo {
	out := []BanInfo{}
	for _, e := range a.bans.List() {
		out = append(out, BanInfo{Key: e.Key, Until: e.Until, Permanent: e.Permanent(), Reason: e.Reason,
			Manual: e.Source == banlist.SourceManual, Source: e.Source})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Permanent != out[j].Permanent {
			return !out[i].Permanent
		}
		return out[i].Until.Before(out[j].Until)
	})
	return out
}

// autoBanFor 在失败次数达到上限时自动封禁，并发布事件。
func (a *authenticator) autoBanFor(key string) {
	a.banMu.Lock()
	d := a.autoBan
	a.banMu.Unlock()
	if d <= 0 {
		return
	}
	if a.ban(key, d, "认证连续失败", false) == "" {
		return
	}
	events.Publish(events.Event{
		Type:    events.AuthBanned,
		Source:  "Web 服务",
		Level:   events.LevelWarn,
		Title:   "IP 因认证连续失败被封禁：" + key,
		Message: fmt.Sprintf("%s 认证失败达到 %d 次，已封禁 %s", key, maxFailures, d),
		Fields:  map[string]string{"ip": key},
	})
}
