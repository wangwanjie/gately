package webservice

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 导航面板：面板库中的每个面板是一组按分组排列的链接，“导航面板”类型的站点引用其中一个面板，
// 在站点根路径输出汇总页面。访客地址在内网时使用链接的内网地址（没有填写时仍用外网地址），访客也可手动切换。

// 面板与链接的数量、长度上限。图标可以是 data:image 内联图片，上限防止配置文件膨胀。
const (
	maxPanels          = 50
	maxPanelLinks      = 500
	maxPanelNameLen    = 64
	maxPanelTitleLen   = 100
	maxPanelTextLen    = 200
	maxPanelURLLen     = 2048
	maxPanelIconLen    = 96 << 10
	maxPanelIconsTotal = 3 << 20
	// maxPanelTextIcon 为文字图标的最多字符数（按字形簇不好算，按码点与字节同时限制，足够放下组合 emoji）
	maxPanelTextIconRunes = 8
	maxPanelTextIconBytes = 32
)

// panelNetCookie 记录访客手动选择的内网（lan）或外网（wan），没有时按访客地址自动判断。
const panelNetCookie = "gately_panel_net"

// panelSchemes 为链接允许的协议：网页与常见的文件共享、远程登录协议，挡住 javascript:、data: 等。
var panelSchemes = map[string]bool{
	"http": true, "https": true, "ftp": true, "ftps": true, "sftp": true, "ssh": true,
	"smb": true, "afp": true, "nfs": true, "vnc": true, "rdp": true, "mailto": true,
}

// panelColorRe 校验图标底色。
var panelColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// panelDataImageRe 校验内联图标：只接受常见图片格式的 base64 data URL。SVG 以 <img> 显示，其中的脚本不会执行。
var panelDataImageRe = regexp.MustCompile(`^data:image/(png|jpeg|gif|webp|svg\+xml|x-icon|vnd\.microsoft\.icon);base64,([A-Za-z0-9+/]+={0,2})$`)

// Panel 是面板库中的一个导航面板。
type Panel struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Title 与 Subtitle 显示在页面顶部，Title 为空时使用 Name
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	// Icon 为页面图标，写法与链接图标相同，为空时使用默认图标
	Icon string `json:"icon"`
	// Theme 为 ""（跟随访客系统）、light、dark
	Theme string `json:"theme"`
	// LANCIDRs 为额外视为内网的访客地址（IP 或 CIDR）。回环、私有地址（10/8、172.16/12、192.168/16、fc00::/7）
	// 与链路本地地址始终视为内网；经 Tailscale、ZeroTier 等组网访问时可在这里加上对应网段
	LANCIDRs []string `json:"lanCidrs"`
	// Links 为面板中的链接，按列表顺序显示；分组按首次出现的顺序排列
	Links  []PanelLink `json:"links"`
	Remark string      `json:"remark"`
}

// PanelLink 是面板中的一个链接。
type PanelLink struct {
	Name string `json:"name"`
	// Group 为分组名称，留空为未分组
	Group       string `json:"group"`
	Description string `json:"description"`
	// URL 为外网地址；LANURL 为内网地址，留空时内网访客也使用 URL。
	// 两者都支持 {host}、{port}、{hostAndPort}（访客访问面板使用的域名与端口）、{1}、{domainPartN} 与 {STUN_规则名_PORT/IP/ADDR}，
	// 变量暂无值时链接显示为暂不可用；取自请求路径、查询串的变量不支持，见 panelVarAllowed
	URL    string `json:"url"`
	LANURL string `json:"lanUrl"`
	// Icon 为 http(s) 图片地址、data:image 内联图片，或不超过 8 个字符的文字（如 emoji），留空时显示名称首字
	Icon string `json:"icon"`
	// Color 为文字图标的底色（#rrggbb），留空时按名称挑选
	Color string `json:"color"`
	// SameTab 为 true 时在当前页打开，默认在新标签页打开
	SameTab bool `json:"sameTab"`
	// Hide 为 true 时不在页面中显示
	Hide bool `json:"hide"`
}

// runeLimit 检查文本不超过 n 个字符且不含控制字符。
func runeLimit(s, what string, n int) error {
	if utf8.RuneCountInString(s) > n {
		return fmt.Errorf("%s不能超过 %d 个字符", what, n)
	}
	if strings.ContainsFunc(s, unicode.IsControl) {
		return fmt.Errorf("%s不能包含控制字符", what)
	}
	return nil
}

// validatePanelURL 校验链接地址（变量已展开）：完整地址、协议在允许范围内、不含空白与密码。
func validatePanelURL(raw string) error {
	if len(raw) > maxPanelURLLen {
		return fmt.Errorf("地址不能超过 %d 个字符", maxPanelURLLen)
	}
	if strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return errors.New("地址不能包含空白或控制字符")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return errors.New("地址需为完整 URL，例如 https://nas.example.com")
	}
	scheme := strings.ToLower(u.Scheme)
	if !panelSchemes[scheme] {
		return fmt.Errorf("不支持 %s: 协议的地址", scheme)
	}
	if scheme != "mailto" && u.Host == "" {
		return errors.New("地址需为完整 URL，例如 https://nas.example.com")
	}
	if _, has := u.User.Password(); has {
		// 面板对所有能打开它的访客可见，地址里的密码会随页面泄露
		return errors.New("地址不能包含密码")
	}
	return nil
}

// panelVarAllowed 判断链接地址中可用的变量：只允许访客访问面板所用的域名、端口与 STUN 地址。
// {path}、{args}、{ClientIP}、{time} 等取自访客请求的路径、查询串或与链接无关，不能用：
// 写在主机位置时，别人构造一个带查询串的面板地址就能把链接改指向任意网站
func panelVarAllowed(name string) bool {
	switch name {
	case "host", "port", "hostAndPort", "1":
		return true
	}
	if _, ok := partIndex(name, "domainPart"); ok {
		return true
	}
	_, _, ok := stunaddr.ParseVar(name)
	return ok
}

// panelLookup 包装变量查找：不允许的变量返回错误。
func panelLookup(lookup func(string) (string, bool, error)) func(string) (string, bool, error) {
	return func(name string) (string, bool, error) {
		if !panelVarAllowed(name) {
			return "", false, fmt.Errorf("链接地址不支持变量 {%s}", name)
		}
		return lookup(name)
	}
}

// validatePanelTemplate 用示例值展开变量后校验地址，保存时使用。
func validatePanelTemplate(raw string) error {
	sample, err := expandVars(raw, panelLookup(sampleVar))
	if err != nil {
		return err
	}
	return validatePanelURL(sample)
}

// validatePanelIcon 校验图标写法，返回其占用的字节数（用于统计内联图片总量）。
func validatePanelIcon(icon string) (int, error) {
	switch {
	case icon == "":
		return 0, nil
	case strings.HasPrefix(icon, "data:"):
		if len(icon) > maxPanelIconLen {
			return 0, fmt.Errorf("图标图片不能超过 %d KB", maxPanelIconLen>>10)
		}
		m := panelDataImageRe.FindStringSubmatch(icon)
		if m == nil {
			return 0, errors.New("内联图标只支持 PNG、JPEG、GIF、WebP、SVG、ICO 图片")
		}
		if _, err := base64.StdEncoding.DecodeString(m[2]); err != nil {
			return 0, errors.New("内联图标的图片数据无效")
		}
		return len(icon), nil
	case hasHTTPPrefix(icon):
		if len(icon) > maxPanelURLLen {
			return 0, fmt.Errorf("图标地址不能超过 %d 个字符", maxPanelURLLen)
		}
		u, err := url.Parse(icon)
		if err != nil || u.Host == "" || strings.ContainsFunc(icon, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return 0, errors.New("图标地址无效")
		}
		if u.User != nil {
			return 0, errors.New("图标地址不能包含用户名或密码")
		}
		return len(icon), nil
	}
	if utf8.RuneCountInString(icon) > maxPanelTextIconRunes || len(icon) > maxPanelTextIconBytes {
		return 0, fmt.Errorf("文字图标不能超过 %d 个字符", maxPanelTextIconRunes)
	}
	if strings.ContainsFunc(icon, func(r rune) bool { return unicode.IsControl(r) || r == '<' || r == '>' }) {
		return 0, errors.New("文字图标包含无效字符")
	}
	return len(icon), nil
}

// hasHTTPPrefix 判断字符串是否以 http:// 或 https:// 开头（不区分大小写）。
func hasHTTPPrefix(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// validate 校验并规范化面板。名称是否与其他面板重复由 checkPanelLocked 检查。
func (p *Panel) validate() error {
	p.Name = strings.TrimSpace(p.Name)
	p.Title = strings.TrimSpace(p.Title)
	p.Subtitle = strings.TrimSpace(p.Subtitle)
	p.Icon = strings.TrimSpace(p.Icon)
	p.Remark = strings.TrimSpace(p.Remark)
	if p.Name == "" {
		return errors.New("请填写面板名称")
	}
	if err := runeLimit(p.Name, "面板名称", maxPanelNameLen); err != nil {
		return err
	}
	if err := runeLimit(p.Title, "页面标题", maxPanelTitleLen); err != nil {
		return err
	}
	if err := runeLimit(p.Subtitle, "副标题", maxPanelTextLen); err != nil {
		return err
	}
	switch p.Theme {
	case "", "light", "dark":
	default:
		return fmt.Errorf("不支持的主题 %q", p.Theme)
	}
	total, err := validatePanelIcon(p.Icon)
	if err != nil {
		return fmt.Errorf("页面图标: %w", err)
	}
	p.LANCIDRs = uniqueList(p.LANCIDRs)
	if _, err := parsePrefixes(p.LANCIDRs); err != nil {
		return err
	}
	if len(p.Links) > maxPanelLinks {
		return fmt.Errorf("一个面板最多 %d 个链接", maxPanelLinks)
	}
	for i := range p.Links {
		l := &p.Links[i]
		n, err := l.validate()
		if err != nil {
			label := l.Name
			if label == "" {
				label = fmt.Sprintf("#%d", i+1)
			}
			return fmt.Errorf("链接 %s: %w", label, err)
		}
		total += n
	}
	if total > maxPanelIconsTotal {
		return fmt.Errorf("面板中的图标合计不能超过 %d MB，请改用图标地址或更小的图片", maxPanelIconsTotal>>20)
	}
	return nil
}

// validate 校验并规范化链接，返回图标占用的字节数。
func (l *PanelLink) validate() (int, error) {
	l.Name = strings.TrimSpace(l.Name)
	l.Description = strings.TrimSpace(l.Description)
	l.URL = strings.TrimSpace(l.URL)
	l.LANURL = strings.TrimSpace(l.LANURL)
	l.Icon = strings.TrimSpace(l.Icon)
	l.Color = strings.ToLower(strings.TrimSpace(l.Color))
	if l.Name == "" {
		return 0, errors.New("请填写名称")
	}
	if err := runeLimit(l.Name, "名称", maxPanelNameLen); err != nil {
		return 0, err
	}
	if err := crud.NormalizeGroup(&l.Group); err != nil {
		return 0, err
	}
	if err := runeLimit(l.Description, "描述", maxPanelTextLen); err != nil {
		return 0, err
	}
	if l.URL == "" {
		return 0, errors.New("请填写地址")
	}
	if err := validatePanelTemplate(l.URL); err != nil {
		return 0, err
	}
	if l.LANURL != "" {
		if err := validatePanelTemplate(l.LANURL); err != nil {
			return 0, fmt.Errorf("内网地址: %w", err)
		}
	}
	if l.Color != "" && !panelColorRe.MatchString(l.Color) {
		return 0, errors.New("颜色需为 #rrggbb 格式")
	}
	return validatePanelIcon(l.Icon)
}

// ---- 运行时 ----

// compiledPanel 是运行时使用的面板，额外视为内网的地址段已解析。
type compiledPanel struct {
	Panel
	lan []netip.Prefix
}

// panelStore 为面板库，全部监听的站点共用；面板修改后整体替换，站点无需重建。
type panelStore struct {
	mu     sync.RWMutex
	panels map[string]*compiledPanel
}

func newPanelStore() *panelStore {
	return &panelStore{panels: map[string]*compiledPanel{}}
}

// set 替换面板库。配置文件可能被手工编辑，逐个重新校验，无效的面板视为不存在。
func (s *panelStore) set(list []Panel) {
	panels := make(map[string]*compiledPanel, len(list))
	for _, p := range list {
		p.Links = slices.Clone(p.Links)
		if p.validate() != nil {
			continue
		}
		lan, _ := parsePrefixes(p.LANCIDRs)
		panels[p.Key] = &compiledPanel{Panel: p, lan: lan}
	}
	s.mu.Lock()
	s.panels = panels
	s.mu.Unlock()
}

func (s *panelStore) get(key string) (*compiledPanel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.panels[key]
	return p, ok
}

// isLAN 判断访客地址是否在内网：回环、私有、链路本地地址，或面板额外指定的地址段。
func (p *compiledPanel) isLAN(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.Unmap()
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() {
		return true
	}
	for _, pre := range p.lan {
		if pre.Contains(a) {
			return true
		}
	}
	return false
}

// network 返回本次请求使用内网地址与否，以及是否为访客手动选择。
func (p *compiledPanel) network(r *http.Request) (lan, forced bool) {
	if c, err := r.Cookie(panelNetCookie); err == nil {
		switch c.Value {
		case "lan":
			return true, true
		case "wan":
			return false, true
		}
	}
	return p.isLAN(clientAddr(r).Addr()), false
}

// linkAddress 返回卡片上显示的地址：主机与端口，mailto 显示收件人。
func linkAddress(u *url.URL) string {
	if u.Host != "" {
		return u.Host
	}
	return u.Opaque
}

// view 生成页面内容。q 用于展开链接地址中的变量；lan 为使用内网地址。
func (p *compiledPanel) view(q redirectRequest, lan, forced, preview bool) webpage.PanelView {
	v := webpage.PanelView{Title: p.Title, Subtitle: p.Subtitle, Theme: p.Theme, LAN: lan, Forced: forced, Preview: preview}
	if v.Title == "" {
		v.Title = p.Name
	}
	v.Icon = panelIcon(p.Icon, "")
	order := map[string]int{}
	for _, l := range p.Links {
		if l.Hide {
			continue
		}
		if l.LANURL != "" {
			v.Switchable = true
		}
		raw := l.URL
		if lan && l.LANURL != "" {
			raw = l.LANURL
		}
		link := webpage.PanelLink{Name: l.Name, Description: l.Description, NewTab: !l.SameTab, Icon: panelIcon(l.Icon, l.Color)}
		// 变量暂无值（例如 STUN 规则还没有公网地址）或展开后不再是允许的地址时，卡片显示为暂不可用
		if href, err := expandVars(raw, panelLookup(q.lookup)); err == nil && validatePanelURL(href) == nil {
			if u, err := url.Parse(href); err == nil {
				link.Href = href
				link.Address = linkAddress(u)
			}
		}
		i, ok := order[l.Group]
		if !ok {
			i = len(v.Groups)
			order[l.Group] = i
			v.Groups = append(v.Groups, webpage.PanelGroup{Name: l.Group})
		}
		v.Groups[i].Links = append(v.Groups[i].Links, link)
	}
	return v
}

// panelIcon 把配置中的图标写法转换为页面图标。图片地址已在保存与载入时校验。
func panelIcon(icon, color string) webpage.PanelIcon {
	if strings.HasPrefix(icon, "data:") || hasHTTPPrefix(icon) {
		return webpage.PanelIcon{Image: icon}
	}
	return webpage.PanelIcon{Text: icon, Color: color}
}

// newPanel 返回导航面板站点的处理器：只在根路径（与 /index.html）输出页面，?net= 切换内外网后跳回原路径。
func (h *siteHandler) newPanel(s Site) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			writeNotFound(h.errPage, w, r, "页面不存在", "请求的页面不存在。")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeErrorPage(h.errPage, w, r, http.StatusMethodNotAllowed, "不支持的请求方法", "导航面板只支持 GET 请求。")
			return
		}
		var p *compiledPanel
		if h.auth != nil {
			p, _ = h.auth.panels.get(s.Panel)
		}
		if p == nil || !p.Enable {
			writeNotFound(h.errPage, w, r, "导航面板不可用", "该站点引用的导航面板不存在或已停用。")
			return
		}
		if r.URL.Query().Has("net") {
			setPanelNet(w, r, r.URL.Query().Get("net"))
			// 跳回不带查询串的固定路径，刷新或收藏时不会再次切换；路径只可能是 / 或 /index.html，不会跳到别处
			http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
			return
		}
		lan, forced := p.network(r)
		q := redirectRequest{r: r, wildcard: wildcardPart(s.Domains, hostOnly(r.Host))}
		body, csp := webpage.RenderPanel(r, p.view(q, lan, forced, false))
		hd := w.Header()
		hd.Set("Content-Type", "text/html; charset=utf-8")
		// 页面内容随访客地址与 Cookie 变化，不能被缓存
		hd.Set("Cache-Control", "no-store")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("Content-Security-Policy", csp)
		applyHeaders(hd, s.ResponseHeaders)
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	})
}

// setPanelNet 记录访客选择的内网或外网，auto 或其他值清除选择、恢复自动判断。
func setPanelNet(w http.ResponseWriter, r *http.Request, v string) {
	c := &http.Cookie{Name: panelNetCookie, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode}
	switch v {
	case "lan", "wan":
		c.Value = v
		c.MaxAge = int((365 * 24 * time.Hour).Seconds())
	default:
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}
