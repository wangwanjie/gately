package webpage

import (
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/i18n"
)

// 网页认证页面（登录、修改密码、Passkey 管理）的外观：内置样式加上管理员填写的品牌项与自定义 CSS。
// 样式与品牌项都只追加 CSS，不改变页面结构与脚本，登录逻辑对所有样式一致。

// loginSkin 是一种内置样式。
type loginSkin struct {
	id  string
	css string
}

var loginSkinByID = func() map[string]*loginSkin {
	m := make(map[string]*loginSkin, len(loginSkinList))
	for i := range loginSkinList {
		m[loginSkinList[i].id] = &loginSkinList[i]
	}
	return m
}()

// LoginSkinIDs 返回全部内置样式的 ID（不含默认样式），顺序即管理界面中的展示顺序。
func LoginSkinIDs() []string {
	out := make([]string, 0, len(loginSkinList))
	for _, s := range loginSkinList {
		out = append(out, s.id)
	}
	return out
}

// HasLoginSkin 判断 id 是否为内置样式（不含默认样式的空 ID）。
func HasLoginSkin(id string) bool {
	return loginSkinByID[id] != nil
}

// LookOptions 描述一次渲染使用的外观，各项已由调用方校验。
type LookOptions struct {
	// Skin 为内置样式 ID，空为默认样式
	Skin string
	// Color 为主色（#rrggbb），空为样式自带的颜色
	Color string
	// Background 为背景图片地址（http(s) 或同站路径），BgDim 为盖在图片上的暗色遮罩浓度（0-90）
	Background string
	BgDim      int
	// Logo 为替换登录页锁形图标的图片地址（http(s)、同站路径，或预览时的 data:image）
	Logo string
	// Title、Subtitle 替换登录页的标题与说明，空为默认文案
	Title    string
	Subtitle string
	// HideBrand 隐藏页脚的 Gately 标识
	HideBrand bool
	// CSS 为追加在最后的自定义样式
	CSS string
	// Host 为访问的域名（分屏样式的品牌区显示），Lang 为访客语言
	Host string
	Lang string
}

// Look 为布局模板使用的外观数据：Style 输出在页面主体之后，覆盖默认样式与页面自带的样式。
type Look struct {
	BodyClass string
	Decor     template.HTML
	Style     template.CSS
	Title     string
	Subtitle  string
	Logo      template.URL
	HideBrand bool
}

// lookTexts 为背景装饰层中的文案（中文原文）。
var lookTexts = map[string]string{
	"protected": "受保护的站点",
}

// BuildLook 生成外观数据。
func BuildLook(o LookOptions) *Look {
	lk := &Look{Title: o.Title, Subtitle: o.Subtitle, HideBrand: o.HideBrand}
	if o.Logo != "" {
		// 地址已校验：http(s)、同站路径或 data:image（img 中的 SVG 不会执行脚本）
		lk.Logo = template.URL(o.Logo)
	}
	cls := []string{"skin-" + skinOrDefault(o.Skin)}
	var css strings.Builder
	if s := loginSkinByID[o.Skin]; s != nil {
		css.WriteString(s.css)
	}
	css.WriteString(logoCSS)
	if o.Color != "" {
		css.WriteString(brandColorCSS(o.Color))
	}
	if o.Background != "" {
		cls = append(cls, "has-bg")
		css.WriteString(backgroundCSS(o.Background, o.BgDim))
	}
	if o.HideBrand {
		css.WriteString(".foot .brand,.foot .sep{display:none}")
	}
	if c := strings.TrimSpace(o.CSS); c != "" {
		css.WriteString("\n/* custom */\n")
		css.WriteString(SafeStyleText(c))
	}
	lk.BodyClass = strings.Join(cls, " ")
	lk.Style = template.CSS(css.String())
	lk.Decor = lookDecor(o)
	return lk
}

func skinOrDefault(id string) string {
	if id == "" {
		return "default"
	}
	return id
}

// logoCSS 让自定义 Logo 原样显示，不套用图标的底色；分屏样式的 Logo 显示在左侧品牌区。
const logoCSS = `
.icon.logo{display:grid!important;background:none!important;box-shadow:none!important;border:0!important;border-radius:0!important;transform:none!important}
.icon.logo img{width:100%;height:100%;object-fit:contain}
.skin-split .card .icon.logo{display:none!important}
.mark img{width:100%;height:100%;object-fit:contain}.mark.logo{background:none;box-shadow:none}`

// SafeStyleText 让一段 CSS 可以安全地放进 <style> 元素：把 "<" 换成 CSS 转义，内容无法提前结束 <style>
// 或插入标签；"<" 在 CSS 中只会出现在字符串与注释里，转义后显示效果不变。同时去掉 NUL。
func SafeStyleText(css string) string {
	css = strings.ReplaceAll(css, "\x00", "")
	return strings.ReplaceAll(css, "<", `\3c `)
}

// cssURL 把地址写成 CSS 的 url("...")。地址已校验不含空白与控制字符，这里再转义引号、反斜杠与 "<"。
func cssURL(u string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "<", `\3c `, "\n", "", "\r", "")
	return `url("` + r.Replace(u) + `")`
}

// brandColorCSS 用主色覆盖样式的强调色：主按钮、焦点框、图标与浅色底纹。
func brandColorCSS(hex string) string {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return ""
	}
	// 渐变的另一端取主色与白色 18% 混合，浅色底纹取主色 12% 透明度
	mix := func(c int) int { return c + (255-c)*18/100 }
	// 锁屏与蓝图样式的主按钮默认是白色，设置主色后改用主色；极简样式的按钮文字在主色上用白色
	return fmt.Sprintf("\n:root{--brand:%s!important;--brand-2:#%02x%02x%02x!important;--brand-soft:rgba(%d,%d,%d,.12)!important;--ink:%s!important;--on-brand:#fff!important}"+
		"\n.skin-lockscreen .btn.primary,.skin-blueprint .btn.primary{background:var(--brand)!important;color:#fff!important}",
		hex, mix(r), mix(g), mix(b), r, g, b, hex)
}

// parseHexColor 解析 #rrggbb。
func parseHexColor(s string) (r, g, b int, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff), true
}

// ValidColor 判断 s 是否为 #rrggbb 形式的颜色。
func ValidColor(s string) bool {
	_, _, _, ok := parseHexColor(s)
	return ok
}

// backgroundCSS 用背景图片替换样式自带的背景（分屏样式只替换左侧品牌区），并按 dim 盖一层暗色遮罩。
func backgroundCSS(u string, dim int) string {
	dim = max(0, min(dim, 90))
	return fmt.Sprintf(`
.bg{display:block!important;background:#111 %s center/cover no-repeat!important}
.bg::before,.bg>i{display:none!important}
.bg::after{content:""!important;display:block!important;position:absolute;inset:0;background:rgba(0,0,0,%s)!important;-webkit-mask-image:none!important;mask-image:none!important;opacity:1!important}
body.has-bg:not(.skin-split) .foot{color:rgba(255,255,255,.82);text-shadow:0 1px 2px rgba(0,0,0,.5)}
body.has-bg:not(.skin-split) .foot .brand{color:#fff}`, cssURL(u), strconv.FormatFloat(float64(dim)/100, 'f', 2, 64))
}

// lookDecor 返回背景层中的装饰：锁屏样式的时钟（由脚本按访客的时区与语言显示）、分屏样式的品牌区、
// 极光与像素样式的图层。其余样式没有装饰。
func lookDecor(o LookOptions) template.HTML {
	switch o.Skin {
	case "lockscreen":
		// 先填入服务器的当前时间（预览与禁用脚本时显示），脚本再按访客的时区与语言每 10 秒刷新
		return template.HTML(`<div class="clock"><b id="lk-t">` + time.Now().Format("15:04") + `</b><span id="lk-d"></span></div><script>(function(){var t=document.getElementById('lk-t'),d=document.getElementById('lk-d'),l=document.documentElement.lang||undefined;function f(){var n=new Date();try{t.textContent=n.toLocaleTimeString(l,{hour:'2-digit',minute:'2-digit'});d.textContent=n.toLocaleDateString(l,{month:'long',day:'numeric',weekday:'long'});}catch(e){}}f();setInterval(f,10000);})();</script>`)
	case "split":
		mark := `<div class="mark"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="16" r="1"/><rect x="3" y="10" width="18" height="12" rx="2"/><path d="M7 10V7a5 5 0 0 1 10 0v3"/></svg></div>`
		if o.Logo != "" {
			mark = `<div class="mark logo"><img src="` + html.EscapeString(o.Logo) + `" alt=""></div>`
		}
		return template.HTML(`<div class="hero">` + mark + `<div class="host">` + html.EscapeString(o.Host) + `</div><div class="line">` +
			html.EscapeString(i18n.Tr(o.Lang, lookTexts["protected"])) + `</div></div>`)
	case "aurora":
		return `<i class="a1"></i><i class="a2"></i><i class="a3"></i>`
	case "pixel":
		return `<i class="st"></i>`
	}
	return ""
}
