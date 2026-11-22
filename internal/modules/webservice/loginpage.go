package webservice

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/webpage"
)

// 网页认证页面的外观：登录页、修改密码页与 Passkey 管理页（含 OIDC 身份提供方的登录页）共用。
// 全局设置一种内置样式与品牌项（主色、背景图、Logo、标题、说明、隐藏页脚标识）和自定义 CSS，
// 站点可以单独换一种样式（Site.LoginStyle），品牌项与自定义 CSS 对全部站点生效。

// 站点的登录页样式：空为跟随全局设置，LoginStyleDefault 为默认样式，其余为内置样式 ID。
const LoginStyleDefault = "default"

// 外观设置的取值范围。背景图与 Logo 的内联图片由认证路径下的 lookBgSuffix、lookLogoSuffix 按需输出并可被浏览器缓存。
const (
	maxLoginLogo     = 256 << 10
	maxLoginTitle    = 60
	maxLoginSubtitle = 160
	maxLoginCSS      = 32 << 10
	maxLoginBgDim    = 90
	lookBgSuffix     = "/look/bg"
	lookLogoSuffix   = "/look/logo"
)

// LoginPage 是网页认证页面的外观设置。
type LoginPage struct {
	// Style 为内置样式 ID，空为默认样式
	Style string `json:"style"`
	// Color 为主色（#rrggbb），空为样式自带的颜色
	Color string `json:"color"`
	// Background 为背景图片：http(s) 图片地址，或上传的 data:image 内联图片（不超过 1 MB），空为样式自带的背景
	Background string `json:"background"`
	// BgDim 为盖在背景图片上的暗色遮罩浓度（0-90，百分比）
	BgDim int `json:"bgDim"`
	// Logo 替换登录页的锁形图标：http(s) 图片地址，或上传的 data:image 内联图片（不超过 256 KB）
	Logo string `json:"logo"`
	// Title、Subtitle 替换登录页的标题与说明（不随访客语言翻译），空为默认文案
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	// HideBrand 隐藏页脚的 Gately 标识
	HideBrand bool `json:"hideBrand"`
	// CSS 为追加在样式之后的自定义 CSS（不超过 32 KB）
	CSS string `json:"css"`
}

// validLoginStyle 判断全局设置中的样式是否有效（空为默认样式）。
func validLoginStyle(id string) bool {
	return id == "" || webpage.HasLoginSkin(id)
}

// validate 校验并规范化外观设置。
func (p *LoginPage) validate() error {
	p.Style = strings.TrimSpace(p.Style)
	if p.Style == LoginStyleDefault {
		p.Style = ""
	}
	if !validLoginStyle(p.Style) {
		return fmt.Errorf("未知的登录页样式 %q", p.Style)
	}
	p.Color = strings.ToLower(strings.TrimSpace(p.Color))
	if p.Color != "" && !webpage.ValidColor(p.Color) {
		return errors.New("主色需为 #rrggbb 形式的颜色")
	}
	p.Background = strings.TrimSpace(p.Background)
	if _, err := validatePanelBackground(p.Background); err != nil {
		return err
	}
	if p.BgDim < 0 || p.BgDim > maxLoginBgDim {
		return fmt.Errorf("背景遮罩浓度需在 0-%d 之间", maxLoginBgDim)
	}
	p.Logo = strings.TrimSpace(p.Logo)
	if err := validateLoginLogo(p.Logo); err != nil {
		return err
	}
	p.Title = strings.TrimSpace(p.Title)
	p.Subtitle = strings.TrimSpace(p.Subtitle)
	if utf8.RuneCountInString(p.Title) > maxLoginTitle {
		return fmt.Errorf("标题不能超过 %d 个字符", maxLoginTitle)
	}
	if utf8.RuneCountInString(p.Subtitle) > maxLoginSubtitle {
		return fmt.Errorf("说明不能超过 %d 个字符", maxLoginSubtitle)
	}
	if strings.ContainsFunc(p.Title+p.Subtitle, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errors.New("标题与说明不能包含换行或控制字符")
	}
	p.CSS = strings.TrimSpace(p.CSS)
	if len(p.CSS) > maxLoginCSS {
		return fmt.Errorf("自定义 CSS 不能超过 %d KB", maxLoginCSS>>10)
	}
	if !utf8.ValidString(p.CSS) {
		return errors.New("自定义 CSS 不是有效的 UTF-8 文本")
	}
	return nil
}

// validateLoginLogo 校验 Logo 写法：http(s) 图片地址，或上传的 data:image 内联图片。
func validateLoginLogo(logo string) error {
	switch {
	case logo == "":
		return nil
	case strings.HasPrefix(logo, "data:"):
		if len(logo) > maxLoginLogo {
			return fmt.Errorf("Logo 不能超过 %d KB，请压缩后再上传，或改用图片地址", maxLoginLogo>>10)
		}
		m := panelDataImageRe.FindStringSubmatch(logo)
		if m == nil {
			return errors.New("Logo 只支持 PNG、JPEG、GIF、WebP、SVG 图片")
		}
		if _, err := base64.StdEncoding.DecodeString(m[2]); err != nil {
			return errors.New("Logo 的数据无效")
		}
		return nil
	case hasHTTPPrefix(logo):
		u, err := url.Parse(logo)
		if err != nil || u.Host == "" || len(logo) > maxPanelURLLen || strings.ContainsFunc(logo, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return errors.New("Logo 地址无效")
		}
		if u.User != nil {
			return errors.New("Logo 地址不能包含用户名或密码")
		}
		return nil
	}
	return errors.New("Logo 需为 http(s) 图片地址或上传的图片")
}

// validateSiteLoginStyle 校验站点的登录页样式（空为跟随全局设置）。
func validateSiteLoginStyle(id string) error {
	if id == "" || id == LoginStyleDefault || webpage.HasLoginSkin(id) {
		return nil
	}
	return fmt.Errorf("未知的登录页样式 %q", id)
}

// compiledLook 为生效中的外观设置，内联图片已解码。
type compiledLook struct {
	LoginPage
	bg, logo             []byte
	bgMime, logoMime     string
	bgTag, logoTag       string
	bgInline, logoInline bool
}

// lookHolder 保存生效中的外观设置，设置变更时整体替换。
type lookHolder struct{ p atomic.Pointer[compiledLook] }

// set 替换外观设置。配置文件可能被手工编辑，无效的设置按默认外观处理并返回错误供记录日志。
func (h *lookHolder) set(p LoginPage) error {
	if err := p.validate(); err != nil {
		h.p.Store(nil)
		return err
	}
	c := &compiledLook{LoginPage: p}
	if strings.HasPrefix(p.Background, "data:") {
		c.bg, c.bgMime, c.bgTag = decodeBackground(p.Background)
		c.bgInline = c.bg != nil
	}
	if strings.HasPrefix(p.Logo, "data:") {
		c.logo, c.logoMime, c.logoTag = decodeBackground(p.Logo)
		c.logoInline = c.logo != nil
	}
	h.p.Store(c)
	return nil
}

func (h *lookHolder) get() *compiledLook {
	return h.p.Load()
}

// lookOptions 返回站点（base 为其认证路径前缀）使用的外观：站点单独设置的样式优先于全局设置；
// 内联图片换成认证路径下带内容指纹的地址。preview 为 true 时内联图片直接以 data URL 写入页面（管理界面预览）。
func lookOptions(c *compiledLook, siteStyle, base, host, lang string, preview bool) webpage.LookOptions {
	o := webpage.LookOptions{Host: host, Lang: lang}
	if c != nil {
		o.Skin, o.Color, o.BgDim = c.Style, c.Color, c.BgDim
		o.Title, o.Subtitle, o.HideBrand, o.CSS = c.Title, c.Subtitle, c.HideBrand, c.CSS
		o.Background, o.Logo = c.Background, c.Logo
		if c.bgInline && !preview {
			o.Background = base + lookBgSuffix + "?v=" + c.bgTag
		}
		if c.logoInline && !preview {
			o.Logo = base + lookLogoSuffix + "?v=" + c.logoTag
		}
	}
	switch {
	case siteStyle == LoginStyleDefault:
		o.Skin = ""
	case webpage.HasLoginSkin(siteStyle):
		o.Skin = siteStyle
	}
	return o
}

// pageLook 返回站点认证页面使用的外观数据。
func (a *authenticator) pageLook(s Site, host, lang string) *webpage.Look {
	return webpage.BuildLook(lookOptions(a.look.get(), s.LoginStyle, s.authPrefix(), host, lang, false))
}

// serveLookAsset 输出外观设置中上传的背景图或 Logo；路径不是外观图片时返回 false。
// 图片按内容指纹版本化，浏览器可缓存；用 CSP sandbox 与 nosniff 保证即使被直接打开，SVG 中的脚本也不会运行。
func (a *authenticator) serveLookAsset(w http.ResponseWriter, r *http.Request, base string) bool {
	var logo bool
	switch r.URL.Path {
	case base + lookBgSuffix:
	case base + lookLogoSuffix:
		logo = true
	default:
		return false
	}
	c := a.look.get()
	var data []byte
	var mime, tag string
	if c != nil {
		if logo {
			data, mime, tag = c.logo, c.logoMime, c.logoTag
		} else {
			data, mime, tag = c.bg, c.bgMime, c.bgTag
		}
	}
	if data == nil {
		http.NotFound(w, r)
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return true
	}
	etag := `"` + tag + `"`
	hd := w.Header()
	hd.Set("Content-Type", mime)
	hd.Set("ETag", etag)
	hd.Set("Cache-Control", "public, max-age=86400")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	hd.Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
	return true
}
