package webservice

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/i18n"
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
	// maxPanelAlts 为每个链接的其他访问方式数量上限，maxPanelAltName 为其名称的最多字符数
	maxPanelAlts    = 8
	maxPanelAltName = 20
	// maxPanelEngines 为每个面板的搜索引擎数量上限，maxPanelEngineName 为其名称的最多字符数
	maxPanelEngines    = 12
	maxPanelEngineName = 20
	// maxPanelBackground 为背景图片 data:image 内联图片的字节上限；内联图片由 panelBgPath 路径按需输出并可被浏览器缓存，
	// 不随每次页面一起下发，所以可以比图标大
	maxPanelBackground = 1 << 20
	// maxPanelBgDim 为背景遮罩浓度上限（百分比），maxPanelBgBlur 为背景模糊半径上限（像素）
	maxPanelBgDim  = 90
	maxPanelBgBlur = 20
)

// 页面布局的取值范围；各项为 0 时使用默认值（随链接样式而定）。
const (
	minPanelIconSize, maxPanelIconSize = 24, 128
	maxPanelColumns                    = 12
	maxPanelMobileColumns              = 8
	minPanelGap, maxPanelGap           = 4, 48
	minPanelMaxWidth, maxPanelMaxWidth = 640, 3840
)

// 链接样式：默认卡片（图标在左，右侧标题与描述），app 为桌面 App 图标（大图标在上、名称在下，不显示描述）。
const (
	PanelStyleCard = ""
	PanelStyleApp  = "app"
)

// 链接的尺寸（行×列，与 Sun-Panel 的 1x1、1x2、2x2 对应）：
//   - PanelSizeLarge 为默认尺寸：卡片样式下为 1×2（图标加标题、描述），App 图标样式下为 1×1（图标加名称）；
//   - PanelSizeSmall 为 1×1：卡片样式下只显示图标，App 图标样式下与默认相同；
//   - PanelSizeWide 为 1×2：卡片样式下与默认相同，App 图标样式下为横向的宽块（图标在左，名称与描述在右）；
//   - PanelSizeBig 为 2×2：两行两列的大块，图标放大，名称（卡片样式下还有描述）在图标下方。
//
// 默认尺寸随样式变化，是为了让升级前的面板（链接都是默认尺寸）在两种样式下保持原样。
const (
	PanelSizeLarge = ""
	PanelSizeSmall = "small"
	PanelSizeWide  = "wide"
	PanelSizeBig   = "big"
)

// panelLangCookie 记录访客在页面上选择的语言，没有时按浏览器的 Accept-Language。
const panelLangCookie = "gately_panel_lang"

// panelBgPath 为面板站点输出内联背景图片的路径，页面中的地址带 ?v=图片指纹，图片可长期缓存。
const panelBgPath = "/__gately_panel/bg"

// panelBuiltinPrefix 为内置图标的写法前缀，例如 builtin:jellyfin，图标列表见 webpage.BuiltinIcons
const panelBuiltinPrefix = "builtin:"

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
	// TabTitle 为浏览器标签页标题，为空时用页面标题；Favicon 为标签页图标（写法同 Icon，内置图标会画在底色方块上），
	// 为空时用页面图标；Description 为页面描述（meta description，搜索与分享预览时显示），不显示在页面上
	TabTitle    string `json:"tabTitle"`
	Favicon     string `json:"favicon"`
	Description string `json:"description"`
	// Footer 为页脚文字，为空时显示 Gately 标志与名称；FooterURL 为页脚文字的链接（http/https），为空时不加链接；
	// HideFooter 为 true 时不显示页脚
	Footer     string `json:"footer"`
	FooterURL  string `json:"footerUrl"`
	HideFooter bool   `json:"hideFooter"`
	// Theme 为 ""（跟随访客系统）、light、dark
	Theme string `json:"theme"`
	// Skin 为皮肤（色系 ID，与管理界面的主题配色相同），为空时用默认色系。访客可在页面上换肤，选择保存在浏览器中
	Skin string `json:"skin"`
	// Engines 为页面搜索框可选的搜索引擎，按列表顺序显示在“本站”之后；DefaultEngine 为默认选项：0 为本站（过滤本页链接），n 为 Engines[n-1]
	Engines       []PanelEngine `json:"engines"`
	DefaultEngine int           `json:"defaultEngine"`
	// Background 为页面背景图片：http(s) 图片地址，或 data:image 内联图片，为空时用默认渐变背景；
	// BgDim 为盖在图片上的底色遮罩浓度（0-90），避免文字被图片淹没
	Background string `json:"background"`
	BgDim      int    `json:"bgDim"`
	// BgBlur 为背景图片的模糊半径（0-20 像素）
	BgBlur int `json:"bgBlur"`
	// CardStyle 为链接样式：空为卡片，app 为桌面 App 图标（名称在图标下方，不显示描述）；两种样式下有其他访问方式的链接点击后都弹出地址选择
	CardStyle string `json:"cardStyle"`
	// IconSize 为图标边长（24-128 像素），0 为默认（卡片 40、App 图标 64，手机上分别为 34、56）
	IconSize int `json:"iconSize"`
	// Columns 为宽屏下每行最多显示的链接数（1-12），窗口变窄时自动减少；0 为按宽度自动排列。
	// 卡片样式下按大卡片计，小卡片占半格
	Columns int `json:"columns"`
	// MobileColumns 为手机（宽度不超过 640 像素）上每行显示的链接数（1-8），0 为默认（卡片 2 个，App 图标按宽度自动排列）
	MobileColumns int `json:"mobileColumns"`
	// Gap 为链接之间的间距（4-48 像素），0 为默认（卡片 12、App 图标 18）
	Gap int `json:"gap"`
	// MaxWidth 为页面内容的最大宽度（640-3840 像素），0 为默认 1200
	MaxWidth int `json:"maxWidth"`
	// GroupCount 为在分组标题旁显示链接数，默认不显示
	GroupCount bool `json:"groupCount"`
	// AltBadge 为在有多个访问地址（主地址加其他访问方式）的链接图标右上角显示地址数，默认不显示。
	// 无论是否显示角标，点击这类链接都会弹出地址选择列表
	AltBadge bool `json:"altBadge"`
	// HideAppName 为 App 图标样式下不在图标下方显示名称（1×2 宽块中的名称照常显示），默认显示
	HideAppName bool `json:"hideAppName"`
	// HideLock 为已登录访客不显示“登录后可见”链接上的锁标记，默认显示
	HideLock bool `json:"hideLock"`
	// ShowLang 为在页面右上角显示语言切换，默认不显示（访客按浏览器语言看到页面）
	ShowLang bool `json:"showLang"`
	// HideSkin 为不显示右上角的换肤按钮，默认显示；不显示时页面固定用面板设置的皮肤与亮暗模式，访客此前保存的选择不再生效
	HideSkin bool `json:"hideSkin"`
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
	// Alts 为其他访问方式（如 IPv6、frp、STUN、Cloudflare 隧道）：点击链接时与主地址一起列在选择列表中，内网与外网访客都显示
	Alts []PanelAlt `json:"alts"`
	// Icon 为内置图标（builtin:编号）、http(s) 图片地址、data:image 内联图片，或不超过 8 个字符的文字（如 emoji），留空时显示名称首字
	Icon string `json:"icon"`
	// Color 为文字图标与内置图标的底色（#rrggbb），留空时文字图标按名称挑选、内置图标用图标自带的颜色
	Color string `json:"color"`
	// SameTab 为 true 时在当前页打开，默认在新标签页打开
	SameTab bool `json:"sameTab"`
	// Size 为尺寸：空为默认（卡片样式 1×2、App 图标样式 1×1），small 为 1×1，wide 为 1×2，big 为 2×2，见 PanelSizeLarge 等
	Size string `json:"size"`
	// HideName、HideDesc 对 1×1 以外的尺寸生效：隐藏标题、隐藏描述（没有描述时的地址也一并隐藏）
	HideName bool `json:"hideName"`
	HideDesc bool `json:"hideDesc"`
	// AuthOnly 为 true 时只对已登录的访客显示：未登录时页面中没有该链接（地址、名称都不下发）。
	// 站点需启用网页认证并设为“仅对指定路径认证”，让未登录访客也能看到面板；没有启用认证的站点上这类链接始终不显示
	AuthOnly bool `json:"authOnly"`
	// Hide 为 true 时不在页面中显示
	Hide bool `json:"hide"`
}

// PanelAlt 是链接的一个其他访问方式。
type PanelAlt struct {
	// Name 为显示的名称，如 IPv6、frp
	Name string `json:"name"`
	// URL 的写法与可用变量同 PanelLink.URL
	URL string `json:"url"`
}

// PanelEngine 是一个搜索引擎。
type PanelEngine struct {
	// Name 为下拉框中显示的名称，如 Google、GitHub
	Name string `json:"name"`
	// URL 为搜索地址，用 {q} 表示关键词，例如 https://www.google.com/search?q={q}。关键词由访客在浏览器中输入并编码
	URL string `json:"url"`
}

// clone 深拷贝面板：链接与其他访问方式的底层数组都复制一份。validate 会就地规范化字段，
// 载入面板库时不能写到配置或正在服务的面板共用的数组上，否则与访客请求并发读写
func (p Panel) clone() Panel {
	p.LANCIDRs = slices.Clone(p.LANCIDRs)
	p.Engines = slices.Clone(p.Engines)
	p.Links = slices.Clone(p.Links)
	for i := range p.Links {
		p.Links[i].Alts = slices.Clone(p.Links[i].Alts)
	}
	return p
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
	if scheme != "mailto" && !validPanelHost(u.Host) {
		return errors.New("地址需为完整 URL，例如 https://nas.example.com")
	}
	if _, has := u.User.Password(); has {
		// 面板对所有能打开它的访客可见，地址里的密码会随页面泄露
		return errors.New("地址不能包含密码")
	}
	return nil
}

// validPanelHost 检查地址的主机部分：主机名不能为空、不能有空标签（如 .nas.example.com、a..b），
// 未加方括号的主机名不能含冒号（IPv6 地址须写成 [2001:db8::1]），端口须为 1-65535 的数字。
// 变量展开为空（如非通配站点的 {1}、越界的 {domainPartN}）时得到的地址会在这里被拦下
func validPanelHost(host string) bool {
	if strings.HasPrefix(host, "[") {
		end := strings.IndexByte(host, ']')
		if end < 0 {
			return false
		}
		if _, err := netip.ParseAddr(host[1:end]); err != nil {
			return false
		}
		rest := host[end+1:]
		if rest == "" {
			return true
		}
		port, ok := strings.CutPrefix(rest, ":")
		return ok && validPanelPort(port)
	}
	name := host
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if !validPanelPort(host[i+1:]) {
			return false
		}
		name = host[:i]
	}
	if name == "" || strings.Contains(name, ":") {
		return false
	}
	// 允许完全限定写法末尾的一个点
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			return false
		}
	}
	return true
}

// validPanelPort 检查端口为 1-65535 的十进制数字。
func validPanelPort(port string) bool {
	if port == "" || len(port) > 5 || strings.TrimLeft(port, "0123456789") != "" {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
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
	case strings.HasPrefix(icon, panelBuiltinPrefix):
		if _, ok := webpage.FindBuiltinIcon(strings.TrimPrefix(icon, panelBuiltinPrefix)); !ok {
			return 0, errors.New("内置图标不存在")
		}
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

// validatePanelEngineURL 校验搜索引擎地址：http(s) 协议，必须含 {q}，且第一个 {q} 之前已写完主机并进入路径或查询串
// （https://example.com/s?q={q}）。关键词只能落在路径、查询串或片段中：写在主机或用户信息位置时，
// 访客输入的文字会决定打开哪个网站。不允许其他变量与花括号。
func validatePanelEngineURL(raw string) error {
	if len(raw) > maxPanelURLLen {
		return fmt.Errorf("地址不能超过 %d 个字符", maxPanelURLLen)
	}
	if !strings.Contains(raw, "{q}") {
		return errors.New("搜索地址需包含 {q} 代表关键词，例如 https://www.google.com/search?q={q}")
	}
	if strings.ContainsAny(strings.ReplaceAll(raw, "{q}", ""), "{}") {
		return errors.New("搜索地址只支持 {q} 一个变量")
	}
	if !hasHTTPPrefix(raw) {
		return errors.New("搜索地址需以 http:// 或 https:// 开头")
	}
	head, _, _ := strings.Cut(raw, "{q}")
	hu, err := url.Parse(head)
	if err != nil || (hu.Path == "" && hu.RawQuery == "" && !hu.ForceQuery && !strings.Contains(head, "#")) {
		return errors.New("{q} 需写在路径或查询参数中，例如 https://www.google.com/search?q={q}")
	}
	// 用示例关键词展开后整体校验：完整 URL、主机合法、没有空白与密码
	return validatePanelURL(strings.ReplaceAll(raw, "{q}", "test"))
}

// validatePanelBackground 校验背景图片写法，返回其占用的字节数（内联图片计入面板的图片总量）。
func validatePanelBackground(bg string) (int, error) {
	switch {
	case bg == "":
		return 0, nil
	case strings.HasPrefix(bg, "data:"):
		if len(bg) > maxPanelBackground {
			return 0, fmt.Errorf("背景图片不能超过 %d KB，请压缩后再上传，或改用图片地址", maxPanelBackground>>10)
		}
		m := panelDataImageRe.FindStringSubmatch(bg)
		if m == nil {
			return 0, errors.New("背景图片只支持 PNG、JPEG、GIF、WebP、SVG 图片")
		}
		if _, err := base64.StdEncoding.DecodeString(m[2]); err != nil {
			return 0, errors.New("背景图片的数据无效")
		}
		return len(bg), nil
	case hasHTTPPrefix(bg):
		if len(bg) > maxPanelURLLen {
			return 0, fmt.Errorf("背景图片地址不能超过 %d 个字符", maxPanelURLLen)
		}
		u, err := url.Parse(bg)
		if err != nil || u.Host == "" || strings.ContainsFunc(bg, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return 0, errors.New("背景图片地址无效")
		}
		if u.User != nil {
			return 0, errors.New("背景图片地址不能包含用户名或密码")
		}
		return 0, nil
	}
	return 0, errors.New("背景图片需为 http(s) 图片地址或上传的图片")
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
	p.TabTitle = strings.TrimSpace(p.TabTitle)
	p.Favicon = strings.TrimSpace(p.Favicon)
	p.Description = strings.TrimSpace(p.Description)
	p.Footer = strings.TrimSpace(p.Footer)
	p.FooterURL = strings.TrimSpace(p.FooterURL)
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
	if err := runeLimit(p.TabTitle, "标签页标题", maxPanelTitleLen); err != nil {
		return err
	}
	if err := runeLimit(p.Description, "页面描述", 2*maxPanelTextLen); err != nil {
		return err
	}
	if err := runeLimit(p.Footer, "页脚文字", maxPanelTextLen); err != nil {
		return err
	}
	if p.FooterURL != "" {
		// 页脚链接只允许网页地址，且不能带密码
		if !hasHTTPPrefix(p.FooterURL) {
			return errors.New("页脚链接需以 http:// 或 https:// 开头")
		}
		if err := validatePanelURL(p.FooterURL); err != nil {
			return fmt.Errorf("页脚链接: %w", err)
		}
	}
	switch p.Theme {
	case "", "light", "dark":
	default:
		return fmt.Errorf("不支持的主题 %q", p.Theme)
	}
	p.Skin = strings.TrimSpace(p.Skin)
	if !webpage.ValidSkin(p.Skin) {
		return fmt.Errorf("不支持的皮肤 %q", p.Skin)
	}
	total, err := validatePanelIcon(p.Icon)
	if err != nil {
		return fmt.Errorf("页面图标: %w", err)
	}
	favSize, err := validatePanelIcon(p.Favicon)
	if err != nil {
		return fmt.Errorf("标签页图标: %w", err)
	}
	total += favSize
	p.Background = strings.TrimSpace(p.Background)
	bgSize, err := validatePanelBackground(p.Background)
	if err != nil {
		return err
	}
	total += bgSize
	if p.BgDim < 0 || p.BgDim > maxPanelBgDim {
		return fmt.Errorf("背景遮罩需在 0 到 %d 之间", maxPanelBgDim)
	}
	if p.BgBlur < 0 || p.BgBlur > maxPanelBgBlur {
		return fmt.Errorf("背景模糊需在 0 到 %d 之间", maxPanelBgBlur)
	}
	if err := p.validateLayout(); err != nil {
		return err
	}
	if err := p.validateEngines(); err != nil {
		return err
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

// validateLayout 校验链接样式与页面布局：数值为 0 表示默认，其余需在范围内。
// 这些值以整数写进页面样式，范围校验同时保证页面不会被设置成无法使用的尺寸。
func (p *Panel) validateLayout() error {
	switch p.CardStyle {
	case PanelStyleCard, PanelStyleApp:
	default:
		return fmt.Errorf("不支持的链接样式 %q", p.CardStyle)
	}
	inRange := func(v, lo, hi int) bool {
		return v == 0 || (v >= lo && v <= hi)
	}
	if !inRange(p.IconSize, minPanelIconSize, maxPanelIconSize) {
		return fmt.Errorf("图标大小需在 %d 到 %d 之间", minPanelIconSize, maxPanelIconSize)
	}
	if !inRange(p.Columns, 1, maxPanelColumns) {
		return fmt.Errorf("每行数量需在 1 到 %d 之间", maxPanelColumns)
	}
	if !inRange(p.MobileColumns, 1, maxPanelMobileColumns) {
		return fmt.Errorf("手机每行数量需在 1 到 %d 之间", maxPanelMobileColumns)
	}
	if !inRange(p.Gap, minPanelGap, maxPanelGap) {
		return fmt.Errorf("间距需在 %d 到 %d 之间", minPanelGap, maxPanelGap)
	}
	if !inRange(p.MaxWidth, minPanelMaxWidth, maxPanelMaxWidth) {
		return fmt.Errorf("内容宽度需在 %d 到 %d 之间", minPanelMaxWidth, maxPanelMaxWidth)
	}
	return nil
}

// validateEngines 校验搜索引擎列表：数量、名称（不重复）、地址，默认选项在范围内。
func (p *Panel) validateEngines() error {
	if len(p.Engines) > maxPanelEngines {
		return fmt.Errorf("搜索引擎最多 %d 个", maxPanelEngines)
	}
	seen := map[string]bool{}
	for i := range p.Engines {
		e := &p.Engines[i]
		e.Name = strings.TrimSpace(e.Name)
		e.URL = strings.TrimSpace(e.URL)
		if e.Name == "" {
			return errors.New("请填写搜索引擎的名称")
		}
		if err := runeLimit(e.Name, "搜索引擎名称", maxPanelEngineName); err != nil {
			return err
		}
		key := strings.ToLower(e.Name)
		if seen[key] {
			return fmt.Errorf("搜索引擎 %s 重复", e.Name)
		}
		seen[key] = true
		if err := validatePanelEngineURL(e.URL); err != nil {
			return fmt.Errorf("搜索引擎 %s: %w", e.Name, err)
		}
	}
	if p.DefaultEngine < 0 || p.DefaultEngine > len(p.Engines) {
		return errors.New("默认搜索引擎无效")
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
	if len(l.Alts) > maxPanelAlts {
		return 0, fmt.Errorf("其他访问方式最多 %d 个", maxPanelAlts)
	}
	for i := range l.Alts {
		a := &l.Alts[i]
		a.Name = strings.TrimSpace(a.Name)
		a.URL = strings.TrimSpace(a.URL)
		if a.Name == "" {
			return 0, errors.New("请填写其他访问方式的名称")
		}
		if err := runeLimit(a.Name, "访问方式名称", maxPanelAltName); err != nil {
			return 0, err
		}
		if a.URL == "" {
			return 0, fmt.Errorf("请填写访问方式 %s 的地址", a.Name)
		}
		if err := validatePanelTemplate(a.URL); err != nil {
			return 0, fmt.Errorf("访问方式 %s: %w", a.Name, err)
		}
	}
	switch l.Size {
	case PanelSizeLarge, PanelSizeSmall, PanelSizeWide, PanelSizeBig:
	default:
		return 0, errors.New("卡片尺寸无效")
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
	// bg 为内联背景图片解码后的内容，bgMime 为其类型，bgTag 为内容指纹（用作 ETag 与页面中图片地址的版本）；背景为图片地址或没有背景时为空
	bg     []byte
	bgMime string
	bgTag  string
}

// decodeBackground 解码内联背景图片（已通过 validatePanelBackground 校验）。
func decodeBackground(bg string) (data []byte, mime, tag string) {
	m := panelDataImageRe.FindStringSubmatch(bg)
	if m == nil {
		return nil, "", ""
	}
	data, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		return nil, "", ""
	}
	sum := sha256.Sum256(data)
	return data, strings.TrimPrefix(bg[:strings.IndexByte(bg, ';')], "data:"), hex.EncodeToString(sum[:8])
}

// panelStore 为面板库，全部监听的站点共用；面板修改后整体替换，站点无需重建。
type panelStore struct {
	mu     sync.RWMutex
	panels map[string]*compiledPanel
	// warn 记录载入时因校验失败被忽略的面板
	warn func(format string, args ...any)
}

func newPanelStore() *panelStore {
	return &panelStore{panels: map[string]*compiledPanel{}, warn: func(string, ...any) {}}
}

// set 替换面板库。配置文件可能被手工编辑，逐个重新校验，无效的面板视为不存在并记录警告。
// 内置图标不存在（例如降级到图标较少的版本）时只清空该图标，不因此丢弃整个面板
func (s *panelStore) set(list []Panel) {
	panels := make(map[string]*compiledPanel, len(list))
	for _, src := range list {
		p := src.clone()
		p.Icon = dropUnknownBuiltin(p.Icon)
		p.Favicon = dropUnknownBuiltin(p.Favicon)
		for i := range p.Links {
			p.Links[i].Icon = dropUnknownBuiltin(p.Links[i].Icon)
		}
		if err := p.validate(); err != nil {
			s.warn("导航面板 %s 无效，已忽略: %v", p.Name, err)
			continue
		}
		lan, _ := parsePrefixes(p.LANCIDRs)
		cp := &compiledPanel{Panel: p, lan: lan}
		cp.bg, cp.bgMime, cp.bgTag = decodeBackground(p.Background)
		panels[p.Key] = cp
	}
	s.mu.Lock()
	s.panels = panels
	s.mu.Unlock()
}

// dropUnknownBuiltin 把不存在的内置图标换成空图标（页面显示名称首字），其他写法原样返回。
func dropUnknownBuiltin(icon string) string {
	id, ok := strings.CutPrefix(strings.TrimSpace(icon), panelBuiltinPrefix)
	if !ok {
		return icon
	}
	if _, found := webpage.FindBuiltinIcon(id); !found {
		return ""
	}
	return icon
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

// view 生成页面内容。q 用于展开链接地址中的变量；lan 为使用内网地址；authed 为访客已登录，
// 未登录时“登录后可见”的链接不进入页面，只统计被隐藏的个数。
func (p *compiledPanel) view(q redirectRequest, lan, forced, preview, authed bool) webpage.PanelView {
	v := webpage.PanelView{Title: p.Title, Subtitle: p.Subtitle, Theme: p.Theme, Skin: p.Skin, LAN: lan, Forced: forced, Preview: preview, BgDim: p.BgDim, BgBlur: p.BgBlur,
		Layout:     webpage.PanelLayout{Style: p.CardStyle, IconSize: p.IconSize, Columns: p.Columns, MobileColumns: p.MobileColumns, Gap: p.Gap, MaxWidth: p.MaxWidth, HideAppName: p.HideAppName},
		GroupCount: p.GroupCount, AltBadge: p.AltBadge, ShowLang: p.ShowLang, HideSkin: p.HideSkin}
	for _, e := range p.Engines {
		v.Engines = append(v.Engines, webpage.PanelEngine{Name: e.Name, URL: e.URL})
	}
	v.DefaultEngine = p.DefaultEngine
	// 内联背景图片由站点的专用路径输出（可缓存，页面不带图片数据）；预览放在 srcdoc iframe 中取不到站点路径，
	// 预览用的面板没有解码结果，直接内联
	if p.bg != nil {
		v.Background = panelBgPath + "?v=" + p.bgTag
	} else {
		v.Background = p.Background
	}
	if v.Title == "" {
		v.Title = p.Name
	}
	v.Icon = panelIcon(p.Icon, "")
	v.TabTitle, v.Description = p.TabTitle, p.Description
	v.Footer, v.FooterURL, v.HideFooter = p.Footer, p.FooterURL, p.HideFooter
	if p.Favicon != "" {
		v.Favicon = panelIcon(p.Favicon, "")
	}
	order := map[string]int{}
	for _, l := range p.Links {
		if l.Hide {
			continue
		}
		if l.AuthOnly && !authed {
			v.Hidden++
			continue
		}
		if l.LANURL != "" {
			v.Switchable = true
		}
		raw := l.URL
		if lan && l.LANURL != "" {
			raw = l.LANURL
		}
		// 锁标记只出现在已登录访客的页面上（未登录时这类链接不下发），面板可设置不显示
		link := webpage.PanelLink{Name: l.Name, Description: l.Description, NewTab: !l.SameTab, Icon: panelIcon(l.Icon, l.Color), Locked: l.AuthOnly && !p.HideLock,
			Small: l.Size == PanelSizeSmall, Wide: l.Size == PanelSizeWide, Big: l.Size == PanelSizeBig, HideName: l.HideName, HideDesc: l.HideDesc}
		// 变量暂无值（例如 STUN 规则还没有公网地址）或展开后不再是允许的地址时，卡片显示为暂不可用
		if href, ok := expandPanelURL(raw, q); ok {
			if u, err := url.Parse(href); err == nil {
				link.Href = href
				link.Address = linkAddress(u)
			}
		}
		for _, a := range l.Alts {
			href, _ := expandPanelURL(a.URL, q)
			link.Alts = append(link.Alts, webpage.PanelAlt{Name: a.Name, Href: href})
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

// expandPanelURL 展开链接地址中的变量并校验，不可用时返回 false。
// {1} 没有值（面板站点不是通配域名，或预览时）时链接视为暂不可用：空值代入后地址虽可能仍能解析，
// 指向的却是另一个主机，例如 https://{1}.nas.example.com 变成 https://.nas.example.com
func expandPanelURL(raw string, q redirectRequest) (string, bool) {
	empty := false
	lookup := panelLookup(q.lookup)
	href, err := expandVars(raw, func(name string) (string, bool, error) {
		v, found, lookupErr := lookup(name)
		if lookupErr == nil && found && name == "1" && v == "" {
			empty = true
		}
		return v, found, lookupErr
	})
	if err != nil || empty || validatePanelURL(href) != nil {
		return "", false
	}
	return href, true
}

// panelIcon 把配置中的图标写法转换为页面图标。图片地址已在保存与载入时校验。
// 内置图标画在底色方块上：底色为链接设置的颜色，未设置时用图标自带的颜色；底色较浅时图形用深色
func panelIcon(icon, color string) webpage.PanelIcon {
	if id, ok := strings.CutPrefix(icon, panelBuiltinPrefix); ok {
		if ic, found := webpage.FindBuiltinIcon(id); found {
			if color == "" {
				color = ic.Color
			}
			return webpage.PanelIcon{Image: webpage.BuiltinIconURL(ic, webpage.LightColor(color)), Color: color, Tile: true}
		}
		return webpage.PanelIcon{Color: color}
	}
	if strings.HasPrefix(icon, "data:") || hasHTTPPrefix(icon) {
		return webpage.PanelIcon{Image: icon}
	}
	return webpage.PanelIcon{Text: icon, Color: color}
}

// newPanel 返回导航面板站点的处理器：只在根路径（与 /index.html）输出页面，?net= 切换内外网后跳回原路径。
func (h *siteHandler) newPanel(s Site) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == panelBgPath {
			h.servePanelBackground(w, r, s)
			return
		}
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
		authed := panelAuthed(r)
		v := p.view(q, lan, forced, false, authed)
		if authed {
			v.User, _ = r.Context().Value(ctxKeyPanelUser{}).(string)
		}
		// 不显示语言切换时访客无法改回，此前在页面上选过的语言不再生效，按浏览器语言显示
		if p.ShowLang {
			v.Lang = panelLang(r)
		}
		// 面板对未登录访客开放且站点启用了网页认证时，页面提供登录入口；登录后回到面板，退出后回到登录页
		if h.site.WebAuth && !h.needAuth(h.matchRule(r.URL.Path)) {
			base := h.site.authPrefix()
			if authed {
				v.LogoutURL = base + "/logout"
			} else {
				v.LoginURL = base + "/login?next=" + url.QueryEscape("/")
			}
		}
		body, csp := webpage.RenderPanel(r, v)
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

// panelLang 返回访客在页面上选择的语言（Cookie），没有选择或语言不受支持时返回空，由页面按浏览器语言决定。
func panelLang(r *http.Request) string {
	c, err := r.Cookie(panelLangCookie)
	if err != nil {
		return ""
	}
	for _, l := range i18n.AllLangs {
		if c.Value == l {
			return i18n.Normalize(l)
		}
	}
	return ""
}

// ctxKeyPanelAuthed 标记请求已通过站点认证（见 panelAuthed），由 ServeHTTP 在认证后写入。
type ctxKeyPanelAuthed struct{}

// ctxKeyPanelUser 记录已登录访客的名称，用于页头的用户菜单。
type ctxKeyPanelUser struct{}

// panelAuthed 判断导航面板的访客是否已登录：通过了站点的认证，或在无需认证的路径上带有有效的网页登录会话。
// 经分享链接放行的访客不算已登录。
func panelAuthed(r *http.Request) bool {
	v, _ := r.Context().Value(ctxKeyPanelAuthed{}).(bool)
	return v
}

// servePanelBackground 输出面板的内联背景图片。图片按内容指纹版本化，浏览器可缓存；
// 内容来自管理员保存的 data:image，用 CSP sandbox 与 nosniff 保证即使被直接打开，SVG 中的脚本也不会运行。
func (h *siteHandler) servePanelBackground(w http.ResponseWriter, r *http.Request, s Site) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeErrorPage(h.errPage, w, r, http.StatusMethodNotAllowed, "不支持的请求方法", "导航面板只支持 GET 请求。")
		return
	}
	var p *compiledPanel
	if h.auth != nil {
		p, _ = h.auth.panels.get(s.Panel)
	}
	if p == nil || !p.Enable || p.bg == nil {
		writeNotFound(h.errPage, w, r, "页面不存在", "请求的页面不存在。")
		return
	}
	etag := `"` + p.bgTag + `"`
	hd := w.Header()
	hd.Set("Content-Type", p.bgMime)
	hd.Set("ETag", etag)
	hd.Set("Cache-Control", "private, max-age=86400")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hd.Set("Content-Length", strconv.Itoa(len(p.bg)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(p.bg)
	}
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
