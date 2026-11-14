package webservice

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/hostname"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/thirdauth"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 敲门放行（登录后放行来源 IP）
//
// 站点准入（Site.Knock）：来源地址必须持有所选敲门规则的有效授权才能访问站点，其余访客按访问控制拒绝（403，或开启
// “拒绝时断开连接”时直接断开），扫描器看不到站点。授权可以来自端口敲门报文，也可以来自放行入口站点的登录。
// 准入只决定能否连到站点，不代替站点自己的认证：同一出口 IP 后面的其他人（公司、学校、运营商级 NAT）也能连上，
// 但仍要通过应用自己的登录，所以只对准入站点关闭 Gately 认证、由应用自行登录（如飞牛客户端），不会让任何人免认证。
//
// 放行入口（Site.KnockGrant）：开启了 Gately 认证的站点上，访客通过认证后为其来源地址登记授权（续期节流为每分钟最多
// 一次），IPv6 按设置放行整个网段（默认 /64，覆盖临时地址轮换）。入口页面 knockPagePath 显示已放行的地址与到期时间，
// 可撤销；配置了只解析 IPv4 / 只解析 IPv6 的探测域名时，页面用一次性令牌分别请求两个域名，把另一地址族也登记上
// （手机浏览器经 IPv6 登录、App 经 IPv4 连接的情况）。授权不带身份，不写入身份请求头。

const (
	// knockPagePath 为放行入口页面；不放在 authPath 下：未登录时要经站点认证跳回这里
	knockPagePath = "/__gately_knock"
	// knockProbePath 为另一地址族的探测地址（一次性令牌，不需要登录）
	knockProbePath = knockPagePath + "/probe"

	defaultKnockMinutes  = 360
	minKnockMinutes      = 5
	maxKnockMinutes      = 7 * 24 * 60
	defaultKnockV6Prefix = 64
	minKnockV6Prefix     = 48
	// knockProbeTTL 为探测令牌的有效期：页面加载后立即使用
	knockProbeTTL = 2 * time.Minute
	// knockRenewEvery 为同一地址两次续期之间的最短间隔：已登录访客的每个请求都会经过判断，只在剩余时长
	// 比放行时长少了这么多时才写登记表，常规请求只多一次读锁内的 map 查找
	knockRenewEvery = time.Minute
	// maxProbeHost 为探测域名的最大长度
	maxProbeHost = 253
)

// KnockGrant 是放行入口的设置：访客通过本站点的 Gately 认证后，为其来源地址登记敲门规则的授权。
type KnockGrant struct {
	// Rule 为登记授权的敲门规则 key，留空表示不放行
	Rule string `json:"rule"`
	// Minutes 为放行时长（分钟），默认 360，范围 5～10080（7 天）
	Minutes int `json:"minutes"`
	// V6Prefix 为 IPv6 来源放行的前缀长度，默认 64，范围 48～128（128 为只放行单个地址）
	V6Prefix int `json:"v6Prefix"`
	// ProbeV4、ProbeV6 为只解析 IPv4、只解析 IPv6 的探测域名，须是本站点的域名之一（不能是通配写法）；
	// 留空不探测该地址族
	ProbeV4 string `json:"probeV4"`
	ProbeV6 string `json:"probeV6"`
}

// validateKnock 校验站点的准入与放行入口设置；引用的敲门规则是否存在由 checkKnockRefsLocked 在保存时检查。
func (s *Site) validateKnock(isDefault bool) error {
	s.Knock = strings.TrimSpace(s.Knock)
	if s.Knock == "" {
		s.KnockLAN = false
	}
	g := &s.KnockGrant
	g.Rule = strings.TrimSpace(g.Rule)
	if g.Rule == "" {
		*g = KnockGrant{}
		return nil
	}
	if isDefault {
		return errors.New("默认站点不能作为放行入口")
	}
	if !s.BasicAuth && !s.WebAuth {
		return errors.New("放行入口需要启用基本认证或网页认证")
	}
	if g.Rule == s.Knock {
		return errors.New("放行入口不能要求同一条敲门规则的准入，否则未放行的访客无法登录")
	}
	if g.Minutes == 0 {
		g.Minutes = defaultKnockMinutes
	}
	if g.Minutes < minKnockMinutes || g.Minutes > maxKnockMinutes {
		return fmt.Errorf("放行时长应在 %d～%d 分钟之间", minKnockMinutes, maxKnockMinutes)
	}
	if g.V6Prefix == 0 {
		g.V6Prefix = defaultKnockV6Prefix
	}
	if g.V6Prefix < minKnockV6Prefix || g.V6Prefix > 128 {
		return fmt.Errorf("IPv6 放行的前缀长度应在 %d～128 之间", minKnockV6Prefix)
	}
	for _, p := range []*string{&g.ProbeV4, &g.ProbeV6} {
		h, err := s.probeHost(*p)
		if err != nil {
			return err
		}
		*p = h
	}
	if g.ProbeV4 != "" && g.ProbeV4 == g.ProbeV6 {
		return errors.New("IPv4 与 IPv6 探测域名不能相同")
	}
	return nil
}

// probeHost 规范化探测域名，并检查它是本站点的域名之一：探测请求要落到同一站点才能校验令牌。
func (s *Site) probeHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	h, ok := hostname.NormalizePattern(raw)
	if !ok || h == "" || len(h) > maxProbeHost || strings.ContainsAny(h, "*/") {
		return "", fmt.Errorf("探测域名 %q 格式错误", raw)
	}
	for _, d := range s.Domains {
		if d != "*" && matchDomain(d, h) > 0 {
			return h, nil
		}
	}
	return "", fmt.Errorf("探测域名 %s 需要添加到本站点的域名中", h)
}

// knockGrantable 判断地址能否登记放行：回环、内网、链路本地等地址不放行。经隧道或未开启 PROXY 协议的转发进来时，
// 所有访客都显示为这类地址，放行它等于放行所有人；内网访客用准入站点的“局域网地址无需放行”。
// 运营商级 NAT 段 100.64.0.0/10 同样不放行：Tailscale、NetBird 等组网用这一段，经它们转发进来的访客也都显示为这类地址。
func knockGrantable(a netip.Addr) bool {
	return a.IsValid() && !a.IsUnspecified() && !a.IsLoopback() && !a.IsMulticast() && !a.IsPrivate() &&
		!a.IsLinkLocalUnicast() && !a.IsLinkLocalMulticast() && !a.IsInterfaceLocalMulticast() && !cgnatPrefix.Contains(a)
}

// cgnatPrefix 为运营商级 NAT 共享地址段（RFC 6598）。
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// knockLAN 判断地址是否属于局域网（准入站点开启“局域网地址无需放行”时免放行）。回环地址不算：
// 同机的 frpc、cloudflared 等转发进来的访客都显示为回环地址。
func knockLAN(a netip.Addr) bool {
	return a.IsPrivate() || a.IsLinkLocalUnicast()
}

// prefix 返回地址放行的网段：IPv6 按设置的前缀长度，IPv4 为单个地址。地址不能放行时返回 false。
func (g KnockGrant) prefix(a netip.Addr) (netip.Prefix, bool) {
	a = a.Unmap()
	if !knockGrantable(a) {
		return netip.Prefix{}, false
	}
	bits := a.BitLen()
	if a.Is6() && g.V6Prefix >= minKnockV6Prefix && g.V6Prefix < 128 {
		bits = g.V6Prefix
	}
	p, err := a.Prefix(bits)
	return p, err == nil
}

// duration 返回放行时长。
func (g KnockGrant) duration() time.Duration {
	m := g.Minutes
	if m <= 0 {
		m = defaultKnockMinutes
	}
	return time.Duration(m) * time.Minute
}

// knockAdmitted 判断来源地址能否进入要求准入的站点。
func (h *siteHandler) knockAdmitted(a netip.Addr) bool {
	if h.site.KnockLAN && knockLAN(a) {
		return true
	}
	return knockgate.Default.Allowed(a, h.site.Knock)
}

// knockSiteID 返回授权记录的入口站点标识：站点 key 只在监听内唯一，加上监听 key。
func (h *siteHandler) knockSiteID() string {
	return h.listener + "/" + h.site.Key
}

// knockCred 返回用户当前凭据的指纹，登记在授权上：之后修改密码、删除用户时指纹变化，授权被撤销。
// 站点用户与共享用户按凭据计算；后台管理员与第三方登录没有可比较的凭据，只记录类别。
func (a *authenticator) knockCred(s Site, shared map[string]AuthUser, name string) string {
	if strings.HasPrefix(name, adminVisitorPrefix) {
		// 站点关闭“后台登录免认证”后，管理员身份登记的放行随之失效
		if !s.AdminPass {
			return ""
		}
		return "a"
	}
	if v := a.credVersion(s.Users, name); v != "" {
		return "s:" + v
	}
	if s.SharedUsers {
		if u, ok := shared[name]; ok {
			return "u:" + a.sharedCred(u)
		}
	}
	return "o"
}

// knockHolderValid 判断授权的持有者在当前配置下仍然有效：凭据指纹一致；第三方登录身份仍在允许列表中。
func (a *authenticator) knockHolderValid(s Site, shared map[string]AuthUser, o knockgate.Owner) bool {
	if a.knockCred(s, shared, o.User) != o.Cred {
		return false
	}
	if o.Cred == "o" {
		provider, id, ok := strings.Cut(o.User, ":")
		return ok && identityAllowed(s, thirdauth.Identity{Provider: provider, Primary: id, Aliases: o.Aliases})
	}
	return true
}

// grantKnock 为通过本站点认证的访客登记授权。返回放行的网段与是否为新登记；地址不能放行、规则停用时返回错误。
// 已有授权且剩余时长足够时不重复写入（见 knockRenewEvery）。
func (h *siteHandler) grantKnock(r *http.Request, id visitor) (netip.Prefix, bool, error) {
	g := h.site.KnockGrant
	if g.Rule == "" || len(id.names) == 0 {
		return netip.Prefix{}, false, nil
	}
	a := clientAddr(r).Addr()
	p, ok := g.prefix(a)
	if !ok {
		return netip.Prefix{}, false, errKnockAddr
	}
	return h.grantKnockPrefix(r, p, id.names)
}

// knockAliases 返回第三方登录身份的其他标识（去掉“提供方:”前缀），允许列表可能按它们（如 GitHub 数字 ID）放行。
func knockAliases(names []string) []string {
	if len(names) < 2 {
		return nil
	}
	provider, _, ok := strings.Cut(names[0], ":")
	if !ok {
		return nil
	}
	var out []string
	for _, n := range names[1:] {
		if v, ok := strings.CutPrefix(n, provider+":"); ok {
			out = append(out, v)
		}
	}
	return out
}

// errKnockAddr 表示访客地址不能放行（内网、回环等），见 knockGrantable。
var errKnockAddr = errors.New("当前地址是内网或本机地址，不能放行；经隧道或转发访问时请检查可信代理与真实 IP 设置")

// grantKnockPrefix 为访客身份 names（第一个为用户，其余为第三方登录的别名）登记网段 p 的授权。
func (h *siteHandler) grantKnockPrefix(r *http.Request, p netip.Prefix, names []string) (netip.Prefix, bool, error) {
	user := names[0]
	g := h.site.KnockGrant
	dur := g.duration()
	// 剩余时长足够、且这个地址已记录在授权上时不重复写入；同一 IPv6 网段换了临时地址时要立即记录，
	// 系统防火墙按记录的单个地址放行
	seen := clientAddr(r).Addr()
	rem, known := knockgate.Default.Lookup(p, g.Rule, seen)
	if known && rem > dur-knockRenewEvery {
		return p, false, nil
	}
	o := knockgate.Owner{User: user, Site: h.knockSiteID(), SiteName: siteLabel(h.site),
		Cred: h.auth.knockCred(h.site, h.auth.shared.get().users, user), Aliases: knockAliases(names)}
	if err := knockgate.Default.AllowWeb(p, g.Rule, time.Now().Add(dur), o, seen); err != nil {
		h.reqLog(r).Warnf("为 %s 登记放行失败: %v", p, err)
		return p, false, err
	}
	if rem > 0 {
		return p, false, nil
	}
	rule, _ := knockgate.Default.RuleName(g.Rule)
	addr := knockgate.Grant{Prefix: p}.String()
	h.reqLog(r).Infof("放行 %s（敲门规则 %s，用户 %s，%d 分钟）", addr, rule, user, int(dur/time.Minute))
	events.Publish(events.Event{
		Type:    events.KnockOpened,
		Source:  rule,
		Level:   events.LevelInfo,
		Title:   fmt.Sprintf("网页登录放行了 %s", addr),
		Message: fmt.Sprintf("%s：用户 %s 在 %s 登录，放行 %s %d 分钟", rule, user, siteLabel(h.site), addr, int(dur/time.Minute)),
		Fields:  map[string]string{"ip": addr, "rule": rule, "user": user, "site": siteLabel(h.site)},
	})
	return p, true, nil
}

// knockProbe 是探测令牌的内容：令牌绑定站点、规则、用户与目标地址族，只能使用一次。
type knockProbe struct {
	Site string `json:"s"`
	Rule string `json:"r"`
	User string `json:"u"`
	// A 为第三方登录身份的别名（见 knockAliases），随授权登记
	A    []string `json:"a,omitempty"`
	Cred string   `json:"c"`
	// Fam 为 "4" 或 "6"，探测请求的来源地址必须属于该地址族
	Fam string `json:"f"`
	// N 为随机数，使每个令牌都不同（一次性使用按令牌整体记录）
	N string `json:"n"`
}

// probeURL 返回探测地址；host 为探测域名，沿用当前请求的协议与端口。
func (h *siteHandler) probeURL(r *http.Request, host, fam string, names []string) string {
	user := names[0]
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	tok := h.auth.signBlob("knock-probe", knockProbe{Site: h.knockSiteID(), Rule: h.site.KnockGrant.Rule, User: user, A: knockAliases(names),
		Cred: h.auth.knockCred(h.site, h.auth.shared.get().users, user), Fam: fam, N: hex.EncodeToString(nonce)}, knockProbeTTL)
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	hostport := host
	if strings.Contains(host, ":") {
		hostport = "[" + host + "]"
	}
	if _, port, err := splitHostPortLoose(r.Host); err == nil && port != "" {
		hostport += ":" + port
	}
	return scheme + "://" + hostport + knockProbePath + "?t=" + url.QueryEscape(tok)
}

// splitHostPortLoose 拆出 Host 头中的端口（没有端口时返回空串）。
func splitHostPortLoose(h string) (string, string, error) {
	i := strings.LastIndexByte(h, ':')
	if i < 0 || strings.HasSuffix(h, "]") {
		return h, "", nil
	}
	port := h[i+1:]
	if _, err := strconv.Atoi(port); err != nil {
		return h, "", errors.New("端口无效")
	}
	return h[:i], port, nil
}

// handleKnockProbe 处理探测请求：校验一次性令牌后为请求的来源地址登记授权，返回放行的地址（JSON）。
// 跨域读取结果只开放给本站点的域名（放行入口页面所在的源）；令牌无效时返回 404，与普通的不存在路径相同。
func (h *siteHandler) handleKnockProbe(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); origin != "" && h.ownOrigin(origin) {
		hd.Set("Access-Control-Allow-Origin", origin)
		hd.Set("Vary", "Origin")
	}
	lang := i18n.RequestLang(r)
	fail := func(status int, msg string) {
		writeKnockJSON(w, status, map[string]string{"error": i18n.Tr(lang, msg)})
	}
	if r.Method != http.MethodGet {
		fail(http.StatusMethodNotAllowed, "请求方法无效")
		return
	}
	tok := r.URL.Query().Get("t")
	var pr knockProbe
	if tok == "" || len(tok) > 2048 || h.auth.verifyBlob("knock-probe", tok, &pr) != nil ||
		pr.Site != h.knockSiteID() || pr.Rule != h.site.KnockGrant.Rule || h.site.KnockGrant.Rule == "" {
		http.NotFound(w, r)
		return
	}
	a := clientAddr(r).Addr()
	fam := "4"
	if a.Is6() {
		fam = "6"
	}
	if fam != pr.Fam {
		fail(http.StatusConflict, "探测域名同时解析到了另一地址族，请让 IPv4 探测域名只有 A 记录、IPv6 探测域名只有 AAAA 记录")
		return
	}
	if pr.Cred != h.auth.knockCred(h.site, h.auth.shared.get().users, pr.User) {
		fail(http.StatusForbidden, "登录状态已失效，请重新登录")
		return
	}
	// 令牌只能使用一次：页面泄露的令牌不能被他人拿去放行自己的地址
	if !h.auth.consumeOnce("knock-probe|"+tok, knockProbeTTL) {
		fail(http.StatusGone, "探测链接已使用过，请刷新放行页面")
		return
	}
	p, ok := h.site.KnockGrant.prefix(a)
	if !ok {
		fail(http.StatusForbidden, errKnockAddr.Error())
		return
	}
	if _, _, err := h.grantKnockPrefix(r, p, append([]string{pr.User}, prefixAliases(pr.User, pr.A)...)); err != nil {
		fail(http.StatusServiceUnavailable, "放行失败，请联系管理员")
		return
	}
	writeKnockJSON(w, http.StatusOK, map[string]any{"addr": knockgate.Grant{Prefix: p}.String(),
		"until": time.Now().Add(knockgate.Default.Remaining(p, h.site.KnockGrant.Rule)).Unix()})
}

// prefixAliases 把别名还原为“提供方:别名”的访客身份写法（knockAliases 的逆过程）。
func prefixAliases(user string, aliases []string) []string {
	provider, _, ok := strings.Cut(user, ":")
	if !ok || len(aliases) == 0 {
		return nil
	}
	out := make([]string, len(aliases))
	for i, a := range aliases {
		out[i] = provider + ":" + a
	}
	return out
}

// ownOrigin 判断来源是否为本站点的域名（与 corsAllowed 的认证站点规则相同）。
func (h *siteHandler) ownOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := hostOnly(u.Host)
	for _, d := range h.site.Domains {
		if d != "*" && matchDomain(d, host) > 0 {
			return true
		}
	}
	return false
}

func writeKnockJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// knockTexts 为放行入口页面的文案，中文为翻译键。
var knockTexts = map[string]string{
	"pageTitle":    "访问放行",
	"title":        "已放行当前网络",
	"titleFail":    "未能放行当前网络",
	"titleRevoked": "已撤销放行",
	"subtitle":     "在有效期内，当前网络可以访问需要放行的站点；这些站点仍可能要求应用自己的登录",
	"rule":         "敲门规则",
	"addr":         "放行地址",
	"until":        "有效期至",
	"other":        "另一地址族",
	"probing":      "正在检测…",
	"probeFail":    "检测失败",
	"probeHint":    "手机切换网络或出口地址变化后，重新打开本页即可再次放行。",
	"revokeHere":   "撤销当前网络",
	"revokeAll":    "撤销我的全部放行",
	"back":         "返回站点首页",
	"retry":        "重新放行",
}

const knockSrc = `{{define "body"}}<section class="card knock">
<div class="icon{{if not .OK}} warn{{end}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">{{if .OK}}<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/>{{else}}<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>{{end}}</svg></div>
<h1>{{if .Revoked}}{{.T.titleRevoked}}{{else if .OK}}{{.T.title}}{{else}}{{.T.titleFail}}{{end}}</h1>
<p class="sub">{{if .Error}}{{.Error}}{{else if .Revoked}}{{.Revoked}}{{else}}{{.T.subtitle}}{{end}}</p>
<dl class="kv">
<div><dt>{{.T.rule}}</dt><dd>{{.Rule}}</dd></div>
<div><dt>{{.T.addr}}</dt><dd class="mono" id="kn-addr">{{.Addr}}</dd></div>
{{if .OK}}<div><dt>{{.T.until}}</dt><dd class="mono" data-ts="{{.Until}}">{{.UntilText}}</dd></div>{{end}}
{{range .Probes}}<div class="probe" data-url="{{.URL}}" data-fail="{{$.T.probeFail}}"><dt>{{$.T.other}} IPv{{.Fam}}</dt><dd class="mono">{{$.T.probing}}</dd></div>{{end}}
</dl>
{{if .OK}}<p class="hint">{{.T.probeHint}}</p>{{end}}
<div class="actions">
{{if .OK}}<form method="post" action="{{.Action}}"><input type="hidden" name="op" value="here"><button class="btn ghost" type="submit">{{.T.revokeHere}}</button></form>
<form method="post" action="{{.Action}}"><input type="hidden" name="op" value="all"><button class="btn ghost" type="submit">{{.T.revokeAll}}</button></form>
{{else}}<a class="btn ghost" href="{{.Action}}">{{.T.retry}}</a>{{end}}
<a class="btn primary" href="/">{{.T.back}}</a>
</div>
</section>
<style>
.knock{max-width:460px}
.kv{margin:22px 0 0;display:grid;gap:10px}
.kv>div{display:flex;justify-content:space-between;gap:16px;padding-bottom:10px;border-bottom:1px solid var(--line)}
.kv dt{color:var(--muted);flex:0 1 auto}
.kv dd{margin:0;min-width:0;text-align:right;overflow-wrap:anywhere}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px}
.knock .actions form{flex:1;min-width:130px;display:flex}
.knock .actions form .btn{flex:1;height:auto;min-height:44px;padding:10px 14px;line-height:1.3;text-align:center}
.knock .actions .btn.primary{margin-top:0}
@media (max-width:480px){.knock .actions form{flex-basis:100%}}
.probe dd.ok{color:var(--brand)}
.probe dd.err{color:var(--danger)}
</style>
<script>
(function(){
document.querySelectorAll('[data-ts]').forEach(function(el){var d=new Date(Number(el.getAttribute('data-ts'))*1000);if(!isNaN(d)){el.textContent=d.toLocaleString()}});
document.querySelectorAll('.probe').forEach(function(row){
var dd=row.querySelector('dd');var fail=row.getAttribute('data-fail');
fetch(row.getAttribute('data-url'),{credentials:'omit',cache:'no-store'}).then(function(r){return r.json().then(function(j){return{ok:r.ok,j:j}})}).then(function(x){
if(x.ok&&x.j.addr){dd.textContent=x.j.addr;dd.className='mono ok'}else{dd.textContent=fail+(x.j&&x.j.error?'：'+x.j.error:'');dd.className='mono err'}
}).catch(function(){dd.textContent=fail;dd.className='mono err'});
});
})();
</script>{{end}}`

func init() {
	webpage.MustRegister("ws-knock", knockSrc)
}

// knockProbeView 是页面上的一个探测项。
type knockProbeView struct {
	Fam string
	URL string
}

// handleKnockPage 显示放行入口页面（访客已通过认证），并处理撤销。
func (h *siteHandler) handleKnockPage(w http.ResponseWriter, r *http.Request, id visitor) {
	g := h.site.KnockGrant
	lang := i18n.RequestLang(r)
	user := id.names[0]
	a := clientAddr(r).Addr()
	ruleName, _ := knockgate.Default.RuleName(g.Rule)
	data := map[string]any{"Action": knockPagePath, "Rule": ruleName, "Addr": a.String(), "Host": hostOnly(r.Host),
		"PageTitle": i18n.Tr(lang, knockTexts["pageTitle"]) + " · " + hostOnly(r.Host)}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if !sameOriginPost(r) {
			data["Error"] = i18n.Tr(lang, "请求来源无效，请刷新页面后重试")
			webpage.Render(w, http.StatusForbidden, "ws-knock", lang, knockTexts, data)
			return
		}
		n := h.revokeKnock(a, user, r.PostForm.Get("op") == "all")
		h.reqLog(r).Infof("用户 %s 撤销了 %d 个放行", user, n)
		data["Revoked"] = fmt.Sprintf(i18n.Tr(lang, "已撤销 %d 个放行，需要时重新打开本页即可再次放行"), n)
		webpage.Render(w, http.StatusOK, "ws-knock", lang, knockTexts, data)
		return
	}
	p, _, err := h.grantKnock(r, id)
	if err != nil {
		data["Error"] = i18n.Tr(lang, err.Error())
		if errors.Is(err, knockgate.ErrRuleDisabled) {
			data["Error"] = i18n.Tr(lang, "敲门规则不存在或已停用，请联系管理员")
		}
		status = http.StatusForbidden
	} else {
		until := time.Now().Add(knockgate.Default.Remaining(p, g.Rule))
		data["OK"] = true
		data["Addr"] = knockgate.Grant{Prefix: p}.String()
		data["Until"] = until.Unix()
		data["UntilText"] = until.Format("2006-01-02 15:04 MST")
		var probes []knockProbeView
		if g.ProbeV4 != "" && !a.Is4() {
			probes = append(probes, knockProbeView{Fam: "4", URL: h.probeURL(r, g.ProbeV4, "4", id.names)})
		}
		if g.ProbeV6 != "" && !a.Is6() {
			probes = append(probes, knockProbeView{Fam: "6", URL: h.probeURL(r, g.ProbeV6, "6", id.names)})
		}
		data["Probes"] = probes
	}
	webpage.Render(w, status, "ws-knock", lang, knockTexts, data)
}

// revokeKnock 撤销用户在本入口登记的放行：all 为 false 时只撤销当前地址所在的网段，为 true 时撤销该用户在本入口的全部放行。
func (h *siteHandler) revokeKnock(a netip.Addr, user string, all bool) int {
	g := h.site.KnockGrant
	site := h.knockSiteID()
	p, ok := g.prefix(a)
	return knockgate.Default.RevokeFunc(func(x knockgate.Grant) bool {
		if x.Source != knockgate.SourceWeb || x.Rule != g.Rule || x.Site != site || x.User != user {
			return false
		}
		return all || (ok && x.Prefix == p)
	})
}

// revokeKnockOnLogout 撤销用户在入口站点 s 为地址 a 所在网段登记的放行（退出登录时调用）。
// 登录页处理时不知道站点所在的监听，按用户、规则、网段与站点 key 匹配（站点标识为 监听key/站点key），
// 其他入口站点上同名用户（可能是不同的用户库）的放行不受影响。
func (a *authenticator) revokeKnockOnLogout(s Site, addr netip.Addr, user string) {
	g := s.KnockGrant
	p, ok := g.prefix(addr)
	if !ok {
		return
	}
	knockgate.Default.RevokeFunc(func(x knockgate.Grant) bool {
		return x.Source == knockgate.SourceWeb && x.Rule == g.Rule && x.User == user && x.Prefix == p &&
			strings.HasSuffix(x.Site, "/"+s.Key)
	})
}

// handleKnock 处理放行入口的保留路径，返回是否已处理。页面需要通过站点认证（与路径规则无关），探测地址凭令牌访问。
func (h *siteHandler) handleKnock(w http.ResponseWriter, r *http.Request) bool {
	if h.site.KnockGrant.Rule == "" {
		return false
	}
	switch r.URL.Path {
	case knockProbePath:
		h.handleKnockProbe(w, r)
		return true
	case knockPagePath:
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return true
		}
		id, ok := h.auth.checkUser(w, r, h.site)
		if !ok {
			return true
		}
		if len(id.names) == 0 {
			http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
			return true
		}
		h.handleKnockPage(w, r, id)
		return true
	}
	return false
}

// syncKnockLocked 登记站点对敲门规则的引用（敲门页面据此显示引用方、阻止删除），并撤销已失效的网页登录授权：
// 入口站点已删除或不再放行该规则、用户被删除或修改了密码、第三方登录身份不再被允许。保存配置后调用，调用方持有 m.mu。
func (m *Module) syncKnockLocked() {
	refs := map[string][]string{}
	sites := map[string]Site{}
	for _, l := range m.cfg.Listeners {
		all := append([]Site{l.Default}, l.Sites...)
		for i, s := range all {
			// 与端口转发等模块的写法一致：模块 ID / 名称
			label := ID + " / " + siteLabel(s)
			if i == 0 {
				label = ID + " / " + defaultSiteLabel(&l, "默认站点")
			} else {
				sites[l.Key+"/"+s.Key] = s
			}
			if s.Knock != "" {
				refs[s.Knock] = append(refs[s.Knock], label)
			}
			if s.KnockGrant.Rule != "" && s.KnockGrant.Rule != s.Knock {
				refs[s.KnockGrant.Rule] = append(refs[s.KnockGrant.Rule], label)
			}
		}
	}
	knockgate.Default.SetUsers(ID, refs)
	if m.auth == nil {
		return
	}
	shared := make(map[string]AuthUser, len(m.cfg.Settings.AuthUsers))
	for _, u := range m.cfg.Settings.AuthUsers {
		shared[u.Name] = u
	}
	n := knockgate.Default.RevokeFunc(func(g knockgate.Grant) bool {
		if g.Source != knockgate.SourceWeb {
			return false
		}
		s, ok := sites[g.Site]
		if !ok || !s.Enable || s.KnockGrant.Rule != g.Rule || (!s.BasicAuth && !s.WebAuth) {
			return true
		}
		return !m.auth.knockHolderValid(s, shared, g.Owner)
	})
	if n > 0 {
		m.logs.Logger("").Infof("撤销了 %d 个已失效的网页登录放行（入口站点或用户已变化）", n)
	}
}

// checkKnockRefsLocked 检查监听器中站点引用的敲门规则都存在。
func checkKnockRefsLocked(l *Listener) error {
	for _, s := range append([]Site{l.Default}, l.Sites...) {
		for _, k := range []string{s.Knock, s.KnockGrant.Rule} {
			if k == "" {
				continue
			}
			if _, ok := knockgate.Default.RuleName(k); !ok {
				return fmt.Errorf("站点 %s 引用的敲门规则 %q 不存在（敲门模块未启用或规则已删除）", siteLabel(s), k)
			}
		}
	}
	return nil
}
