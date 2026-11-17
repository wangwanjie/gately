package webpage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/i18n"
)

// 导航面板：Web 服务“导航面板”类型站点输出的页面，把多个服务的链接按分组汇总在一页。
// 链接地址、图标都由 Web 服务按访客请求选好（内网或外网地址、展开变量）并校验过，这里只负责排版。

// PanelView 是一个面板页面的内容。
type PanelView struct {
	Title    string
	Subtitle string
	// Icon 为页面标题旁的图标，也是浏览器标签页图标，为空时使用默认图标
	Icon PanelIcon
	// TabTitle 为浏览器标签页标题（为空时用 Title），Favicon 为标签页图标（为空时用 Icon），Description 为 meta description
	TabTitle    string
	Favicon     PanelIcon
	Description string
	// Footer 为页脚文字（为空时显示 Gately 标志与名称），FooterURL 为文字的链接，HideFooter 为不显示页脚
	Footer     string
	FooterURL  string
	HideFooter bool
	// User 为已登录访客的名称，页头显示头像菜单（含页面设置与退出登录）；LogoutURL 为空时只显示设置按钮
	User string
	// Theme 为 ""（跟随系统）、light、dark
	Theme string
	// Skin 为默认皮肤（色系 ID，见 panelthemes.go），为空时用默认色系；访客可在页面上换肤，选择保存在浏览器中
	Skin   string
	Groups []PanelGroup
	// Engines 为搜索引擎：页面搜索框旁的下拉框选择“本站”（过滤本页链接）或某个引擎，引擎地址里的 {q} 换成关键词后在新标签页打开。
	// DefaultEngine 为默认选项：0 为本站，n 为 Engines[n-1]，访客的选择保存在浏览器中
	Engines       []PanelEngine
	DefaultEngine int
	// Background 为背景图片地址（http/https，或同站的图片路径），为空时用默认渐变；BgDim 为盖在图片上的底色遮罩浓度（0-90）
	Background string
	BgDim      int
	// BgBlur 为背景图片的模糊半径（像素，0-20）
	BgBlur int
	// Lang 为访客在页面上选择的语言，为空时按浏览器语言
	Lang string
	// LoginURL 非空时页面提供“登录”入口（面板对未登录访客开放且站点启用了网页认证）；LogoutURL 为已登录访客的退出入口
	LoginURL  string
	LogoutURL string
	// Hidden 为因访客未登录而隐藏的链接数，只用于提示“登录后可查看更多链接”，不暴露链接内容
	Hidden int
	// LAN 为当前使用内网地址；Switchable 表示有链接配置了内网地址，显示内外网切换；
	// Forced 表示访客手动选择了内网或外网，显示“自动识别”
	LAN        bool
	Switchable bool
	Forced     bool
	// Preview 为管理界面中的预览：放在禁用脚本的 iframe 中，不输出脚本与搜索框
	Preview bool
	// Layout 为链接样式与页面布局
	Layout PanelLayout
}

// PanelLayout 是链接样式与页面布局，数值为 0 时用默认值（随样式而定）。取值已由 Web 服务校验，渲染时再按范围收紧一次。
type PanelLayout struct {
	// Style 为 ""（卡片）或 app（桌面 App 图标：大图标在上、名称在下，不显示描述）
	Style string
	// IconSize 为图标边长（像素），Columns 为宽屏每行最多的链接数，MobileColumns 为手机每行的链接数，
	// Gap 为链接间距（像素），MaxWidth 为内容最大宽度（像素）
	IconSize, Columns, MobileColumns, Gap, MaxWidth int
}

// layoutCSS 把布局写成页面样式与 <html> 的类名：app 为 App 图标样式，lc 为设置了宽屏每行数量，lm 为设置了手机每行数量。
// 写入样式的只有收紧到范围内的整数；手机每行数量要写进 repeat() 的次数，只能由服务端按样式分别输出。
func layoutCSS(l PanelLayout) (css string, classes []string) {
	clamp := func(v, lo, hi int) int {
		if v <= 0 {
			return 0
		}
		return min(max(v, lo), hi)
	}
	icon, cols, mcols := clamp(l.IconSize, 24, 128), clamp(l.Columns, 1, 12), clamp(l.MobileColumns, 1, 8)
	gap, width := clamp(l.Gap, 4, 48), clamp(l.MaxWidth, 640, 3840)
	if l.Style == "app" {
		classes = append(classes, "app")
	}
	var vars []string
	if icon > 0 {
		vars = append(vars, "--is:"+strconv.Itoa(icon)+"px")
	}
	if cols > 0 {
		vars = append(vars, "--cols:"+strconv.Itoa(cols))
		classes = append(classes, "lc")
	}
	if gap > 0 {
		vars = append(vars, "--gap:"+strconv.Itoa(gap)+"px")
	}
	if width > 0 {
		vars = append(vars, "--maxw:"+strconv.Itoa(width)+"px")
	}
	if len(vars) > 0 {
		css = ":root{" + strings.Join(vars, ";") + "}"
	}
	if mcols > 0 {
		classes = append(classes, "lm")
		// 卡片样式的网格以半张大卡片为一格（小卡片占一格），所以是 2 倍
		css += "@media (max-width:640px){html.lm .grid{grid-template-columns:repeat(" + strconv.Itoa(mcols*2) + ",minmax(0,1fr))}" +
			"html.lm.app .grid{grid-template-columns:repeat(" + strconv.Itoa(mcols) + ",minmax(0,1fr))}}"
	}
	return css, classes
}

// PanelEngine 是一个搜索引擎：URL 已校验为 http(s) 地址，其中的 {q} 由页面脚本换成关键词。
type PanelEngine struct {
	Name string
	URL  string
}

// PanelGroup 是面板中的一组链接，Name 为空表示未分组（不显示组标题）。
type PanelGroup struct {
	Name  string
	Links []PanelLink
}

// PanelLink 是一个链接卡片。
type PanelLink struct {
	Name        string
	Description string
	// Address 为卡片上显示的地址（主机与端口），没有描述时显示
	Address string
	// Href 为已校验的链接地址，为空表示暂不可用（如引用的 STUN 规则还没有公网地址）
	Href   string
	NewTab bool
	Icon   PanelIcon
	// Small 为小卡片：只显示图标，名称在悬停时提示；HideName、HideDesc 只对大卡片生效
	Small, HideName, HideDesc bool
	// Locked 表示链接只对登录访客显示，已登录的访客在卡片上看到锁标记
	Locked bool
	// Alts 为其他访问方式（如 IPv6、frp、STUN、Cloudflare），显示为卡片上的小链接，始终在新标签页打开
	Alts []PanelAlt
}

// PanelAlt 是一个其他访问方式：Href 为空表示暂不可用。
type PanelAlt struct {
	Name string
	Href string
}

// PanelIcon 是链接或页面的图标：Image 为已校验的图片地址（http/https 或 data:image），
// 否则显示 Text（文字或 emoji，为空时取名称首字）；Color 为 #rrggbb 的背景色，为空时按名称挑选。
// Tile 为 true 时图片画在 Color 底色的方块上（内置图标），否则画在浅色框中
type PanelIcon struct {
	Image string
	Text  string
	Color string
	Tile  bool
}

// panelTexts 为页面文案（中文原文），按访客语言翻译。
var panelTexts = map[string]string{
	"search":        "搜索",
	"lan":           "内网",
	"wan":           "外网",
	"toLan":         "切换到内网",
	"toWan":         "切换到外网",
	"auto":          "自动识别",
	"noMatch":       "没有匹配的链接",
	"empty":         "还没有添加链接",
	"off":           "暂不可用",
	"netHint":       "当前使用的地址",
	"clearHint":     "按 Esc 清空",
	"login":         "登录",
	"logout":        "退出登录",
	"loginHint":     "登录后可查看更多链接",
	"locked":        "登录后可见",
	"local":         "本站",
	"engine":        "搜索范围",
	"webSearch":     "输入关键词，回车搜索",
	"go":            "搜索",
	"language":      "语言",
	"settings":      "页面设置",
	"background":    "背景图片",
	"pickImage":     "选择图片",
	"blur":          "模糊",
	"mask":          "遮罩",
	"reset":         "恢复默认",
	"frequent":      "常用",
	"showFreq":      "显示常用链接",
	"imageFail":     "无法使用这张图片",
	"foldHint":      "展开或收起分组",
	"confirmLogout": "确定要退出登录吗？",
	"cancel":        "取消",
	"layout":        "布局",
	"styleCard":     "卡片",
	"styleApp":      "图标",
	"iconSize":      "大小",
	"alts":          "其他访问方式",
}

// panelPalette 为未指定颜色时按名称挑选的图标底色，深浅主题下白字都清晰可读。
var panelPalette = []string{"#0e7c66", "#2563eb", "#7c3aed", "#db2777", "#ea580c", "#b45309", "#15803d", "#0e7490", "#4f46e5", "#dc2626"}

// 默认图标（Gately 标志）与面板图标（四宫格）。
const (
	panelDefaultFavicon = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='9' fill='%230e7c66'/%3E%3Cpath d='M21.7 10.3A8 8 0 1 0 24 16h-7' stroke='%23fff' stroke-width='3' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3Ccircle cx='23.6' cy='8.6' r='2.2' fill='%239ff3dc'/%3E%3C/svg%3E"
	panelLockIcon       = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>`
	panelGridIcon       = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>`
)

// panelCSS 为页面样式；颜色与圆角变量（--brand、--bg、--r 等）由 skinCSS 按皮肤输出，页面脚本换肤时覆盖。
const panelCSS = `
*{box-sizing:border-box}
[hidden]{display:none!important}
body{margin:0;min-height:100vh;display:flex;flex-direction:column;background:var(--bg);color:var(--text);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif;-webkit-font-smoothing:antialiased}
.bg{position:fixed;inset:0;z-index:-1;background:radial-gradient(60rem 30rem at 0% -10%,var(--soft),transparent 70%),radial-gradient(50rem 28rem at 100% 0%,var(--soft),transparent 70%)}
.bg.has::after{content:"";position:absolute;inset:0;background:var(--bg)}
.page{width:100%;max-width:var(--maxw,1200px);margin:0 auto;padding:32px 24px 12px;flex:1}
.top{display:flex;align-items:center;justify-content:space-between;gap:16px 24px;flex-wrap:wrap;margin-bottom:26px}
.brand{display:flex;align-items:center;gap:14px;min-width:0}
.logo{width:48px;height:48px;flex:none;border-radius:var(--r);display:grid;place-items:center;color:#fff;background:linear-gradient(135deg,var(--brand-2),var(--brand));box-shadow:0 8px 20px -10px var(--brand);font-size:22px;font-weight:650;overflow:hidden}
.logo svg{width:24px;height:24px}
.logo.img{background:var(--card);border:1px solid var(--line);box-shadow:none}
.logo img{width:34px;height:34px;object-fit:contain}
.logo.tile{background:var(--c)}
.logo.tile img{width:26px;height:26px}
.brand h1{margin:0;font-size:22px;line-height:1.3;font-weight:650;letter-spacing:-.01em;overflow-wrap:anywhere}
.brand p{margin:2px 0 0;color:var(--muted);overflow-wrap:anywhere}
.tools{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.search{position:relative;display:flex;align-items:center}
.search svg{position:absolute;left:11px;width:16px;height:16px;color:var(--faint);pointer-events:none}
.search input{width:260px;height:38px;padding:0 34px 0 34px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);color:var(--text);font:inherit;outline:none;transition:border-color .15s,box-shadow .15s}
.search input::placeholder{color:var(--faint)}
.search input:focus{border-color:var(--brand);box-shadow:0 0 0 4px var(--soft)}
.search input::-webkit-search-cancel-button{display:none}
.search kbd{position:absolute;right:9px;min-width:18px;height:20px;padding:0 5px;border:1px solid var(--line);border-radius:5px;color:var(--faint);font:11px/18px ui-monospace,Menlo,monospace;text-align:center;background:var(--input)}
.search input:focus~kbd{display:none}
.sbox{display:flex;align-items:center;gap:6px;min-width:0}
.sbox .search{min-width:0}
.btn{display:inline-flex;align-items:center;gap:6px;height:38px;padding:0 13px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);color:var(--text);font-size:13px;text-decoration:none;white-space:nowrap;transition:border-color .15s,color .15s}
.btn:hover{border-color:var(--brand);color:var(--brand)}
.btn svg{width:15px;height:15px}
.lock{position:absolute;top:7px;right:8px;z-index:2;width:14px;height:14px;color:var(--faint);pointer-events:none}
.more{margin:0 0 20px;text-align:center;color:var(--muted);font-size:12.5px}
.more a{color:var(--brand);text-decoration:none;margin-left:6px}
.net{display:inline-flex;align-items:center;gap:2px;height:38px;padding:3px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card)}
.net a,.net span{display:inline-flex;align-items:center;gap:6px;height:30px;padding:0 11px;border-radius:max(2px,calc(var(--r-sm) - 3px));color:var(--muted);text-decoration:none;font-size:13px;white-space:nowrap}
.net a:hover{color:var(--text);background:var(--input)}
.net .on{color:var(--brand);background:var(--soft);font-weight:600}
.net .dot{width:6px;height:6px;border-radius:50%;background:currentColor}
.grp{margin:0 0 26px}
.grp h2{display:flex;align-items:center;gap:8px;margin:0 0 12px;font-size:13px;font-weight:600;color:var(--muted);letter-spacing:.02em}
.grp h2 .n{min-width:20px;padding:0 6px;border-radius:999px;background:var(--input);border:1px solid var(--line);color:var(--faint);font-size:11px;font-weight:500;text-align:center}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(92px,1fr));gap:var(--gap,12px)}
.card{grid-column:span 2;position:relative;display:flex;align-items:center;gap:12px;min-width:0;padding:12px 14px;border:1px solid var(--line);border-radius:var(--r);background:var(--card);color:inherit;box-shadow:var(--shadow);transition:border-color .15s,transform .15s,background .15s}
.card .hit{position:absolute;inset:0;z-index:1;border-radius:inherit}
.card.link:hover{border-color:var(--brand);background:var(--hover);transform:translateY(-1px)}
.card .hit:focus-visible{outline:none;box-shadow:0 0 0 4px var(--soft)}
.card.off>.ic,.card.off .nm,.card.off .ds{opacity:.55}
.alts{position:relative;z-index:2;display:flex;flex-wrap:wrap;gap:4px;margin-top:5px}
.alt{display:inline-block;max-width:100%;padding:1px 7px;border:1px solid var(--line);border-radius:999px;background:var(--input);color:var(--muted);font-size:11.5px;line-height:17px;text-decoration:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
a.alt:hover{border-color:var(--brand);color:var(--brand)}
.alt.off{opacity:.5;text-decoration:line-through;cursor:not-allowed}
.ic{--s:var(--is,40px);width:var(--s);height:var(--s);flex:none;border-radius:calc(var(--r-sm) + 1px);display:grid;place-items:center;color:#fff;background:var(--c,var(--brand));font-size:calc(var(--s)*.42);font-weight:650;overflow:hidden}
.ic.img{background:var(--input);border:1px solid var(--line)}
.ic img{width:70%;height:70%;object-fit:contain}
.ic.tile img{width:55%;height:55%}
.tx{display:flex;flex-direction:column;min-width:0}
.nm{font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ds{margin-top:1px;color:var(--muted);font-size:12.5px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ds.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;color:var(--faint)}
.empty{margin:48px 0;text-align:center;color:var(--faint)}
.foot{display:flex;align-items:center;justify-content:center;gap:6px;padding:18px 16px 22px;color:var(--faint);font-size:12px}
.foot svg{width:14px;height:14px;color:var(--brand)}
@media (max-width:640px){
.page{padding:20px 14px 8px}
.top{margin-bottom:20px}
.brand h1{font-size:19px}
.logo{width:42px;height:42px;border-radius:calc(var(--r) - 2px);font-size:19px}
.tools{width:100%}
.search{flex:1 1 180px}
.search input{width:100%}
.sbox{flex:1 1 180px}
.sbox .search{flex:1 1 0}
.search kbd{display:none}
.grid{grid-template-columns:repeat(4,1fr);gap:var(--gap,10px)}
.card{grid-column:span 2}
.card{gap:10px;padding:10px 11px;border-radius:calc(var(--r) - 2px)}
.ic{--s:var(--is,34px);border-radius:var(--r-sm)}
.nm{font-size:13.5px}
.ds{font-size:12px}
}
`

const panelSrc = `<!doctype html>
<html lang="{{.Lang}}"{{if .Theme}} data-theme="{{.Theme}}"{{end}}{{with .RootClass}} class="{{.}}"{{end}}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>{{.TabTitle}}</title>
{{if .MetaDesc}}<meta name="description" content="{{.MetaDesc}}">{{end}}
<link rel="icon" href="{{.Favicon}}">
<style nonce="{{.Nonce}}">{{.SkinCSS}}` + panelCSS + panelUICSS + panelLayoutStyleCSS + panelSkinCSS + `{{range $i, $c := .Colors}}.c{{$i}}{--c:{{$c}}}{{end}}{{.BgCSS}}{{.LayoutCSS}}</style>
{{if not .Preview}}<script type="application/json" id="ui-data">{{.UIData}}</script><script type="application/json" id="skin-data">{{.Skins}}</script><script nonce="{{.Nonce}}">` + panelSkinJS + panelLayoutHeadJS + `</script>{{end}}</head>
<body><div class="bg{{if .Background}} has{{end}}" aria-hidden="true">{{if .Background}}<img src="{{.Background}}" alt="" decoding="async" referrerpolicy="no-referrer">{{end}}</div>
<div class="page">
<header class="top">
<div class="brand">{{with .Icon}}{{if and .Image .Class}}<span class="logo tile {{.Class}}"><img src="{{.Image}}" alt=""></span>{{else if .Image}}<span class="logo img"><img src="{{.Image}}" alt="" referrerpolicy="no-referrer"></span>{{else if .Text}}<span class="logo {{.Class}}">{{.Text}}</span>{{else}}<span class="logo">{{$.GridIcon}}</span>{{end}}{{end}}
<div><h1>{{.Title}}</h1>{{if .Subtitle}}<p>{{.Subtitle}}</p>{{end}}</div></div>
{{if not .Preview}}<div class="sbox">{{if .Engines}}<button id="eng" class="eng" type="button" aria-haspopup="listbox" aria-expanded="false" aria-label="{{.T.engine}}" title="{{.T.engine}}"><span id="eng-name">{{.EngineName}}</span>{{.ChevronIcon}}</button>{{end}}<label class="search"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg><input id="q" type="search" enterkeyhint="search" placeholder="{{.T.search}}" aria-label="{{.T.search}}" title="{{.T.clearHint}}" autocomplete="off" spellcheck="false"><kbd>/</kbd>{{if .Engines}}<button id="go" class="go" type="button" hidden aria-label="{{.T.go}}" title="{{.T.go}}">{{.GoIcon}}</button>{{end}}</label></div>{{end}}
{{if .Switchable}}<nav class="net" aria-label="{{.T.netHint}}">{{if .LAN}}<span class="on"><i class="dot"></i>{{.T.lan}}</span><a href="?net=wan" title="{{.T.toWan}}">{{.T.wan}}</a>{{else}}<a href="?net=lan" title="{{.T.toLan}}">{{.T.lan}}</a><span class="on"><i class="dot"></i>{{.T.wan}}</span>{{end}}{{if .Forced}}<a href="?net=auto">{{.T.auto}}</a>{{end}}</nav>{{end}}
{{if not .Preview}}<div class="acts" id="acts">
{{if .Langs}}<div class="pw"><button id="lang-btn" class="skin-btn" type="button" aria-haspopup="listbox" aria-expanded="false" title="{{.T.language}}" aria-label="{{.T.language}}">{{.LangIcon}}</button></div>{{end}}
<div class="skin"><button id="skin-btn" class="skin-btn" type="button" aria-haspopup="dialog" aria-controls="skin-pop" aria-expanded="false" title="{{.SkinT.switchTheme}}" aria-label="{{.SkinT.switchTheme}}">{{.SkinIcon}}</button></div>
{{if and .User .LogoutURL}}<div class="pw"><button id="user-btn" class="ubtn" type="button" aria-haspopup="menu" aria-expanded="false" title="{{.User}}" aria-label="{{.User}}"><span class="av">{{.UserInitial}}</span>{{.ChevronIcon}}</button></div>{{else}}<div class="pw"><button id="set-btn" class="skin-btn" type="button" aria-haspopup="dialog" aria-expanded="false" title="{{.T.settings}}" aria-label="{{.T.settings}}">{{.GearIcon}}</button></div>{{if .LoginURL}}<a class="btn" href="{{.LoginURL}}">{{.LockIcon}}{{.T.login}}</a>{{end}}{{if .LogoutURL}}<button id="logout-btn" class="btn" type="button">{{.T.logout}}</button>{{end}}{{end}}
</div>{{end}}
</header>
<main>
{{range .Groups}}<section class="grp">{{if .Name}}<h2><button class="fold" type="button" data-g="{{.Name}}" aria-expanded="true" title="{{$.T.foldHint}}">{{$.ChevronIcon}}{{.Name}}</button><span class="n">{{len .Links}}</span></h2>{{end}}<div class="grid">
{{range .Links}}<div class="card{{if .Small}} sm{{end}}{{if .Href}} link{{else}} off{{end}}" data-s="{{.Search}}" data-k="{{.Key}}"{{if not .Href}} title="{{$.T.off}}"{{end}}>
{{if .Locked}}<svg class="lock" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><title>{{$.T.locked}}</title><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>{{end}}
{{if .Href}}<a class="hit" href="{{.Href}}"{{if .NewTab}} target="_blank" rel="noopener noreferrer"{{end}} title="{{.Name}}" aria-label="{{.Name}}"></a>{{end}}
{{if and .Alts (not .Small)}}<details class="am"><summary title="{{$.T.alts}}" aria-label="{{$.T.alts}}">{{$.MoreIcon}}</summary><span class="amp"><span class="t">{{$.T.alts}}</span>{{range .Alts}}{{if .Href}}<a href="{{.Href}}" target="_blank" rel="noopener noreferrer" title="{{.Title}}">{{.Name}}</a>{{else}}<span class="off" title="{{$.T.off}}">{{.Name}}</span>{{end}}{{end}}</span></details>{{end}}
{{with .Icon}}{{if and .Image .Class}}<span class="ic tile {{.Class}}"><img src="{{.Image}}" alt="" loading="lazy"></span>{{else if .Image}}<span class="ic img"><img src="{{.Image}}" alt="" loading="lazy" referrerpolicy="no-referrer"></span>{{else}}<span class="ic {{.Class}}">{{.Text}}</span>{{end}}{{end}}
{{if not .Small}}<span class="tx">{{if not .HideName}}<span class="nm">{{.Name}}</span>{{end}}{{if not .Href}}<span class="ds">{{$.T.off}}</span>{{else if .HideDesc}}{{else if .Description}}<span class="ds">{{.Description}}</span>{{else if .Address}}<span class="ds mono">{{.Address}}</span>{{end}}{{if .Alts}}<span class="alts">{{range .Alts}}{{if .Href}}<a class="alt" href="{{.Href}}" target="_blank" rel="noopener noreferrer" title="{{.Title}}">{{.Name}}</a>{{else}}<span class="alt off" title="{{$.T.off}}">{{.Name}}</span>{{end}}{{end}}</span>{{end}}</span>{{end}}
</div>
{{end}}</div></section>
{{end}}
{{if .Empty}}<p class="empty">{{if .Hidden}}{{.T.loginHint}}{{else}}{{.T.empty}}{{end}}</p>{{else}}<p id="none" class="empty" hidden>{{.T.noMatch}}</p>{{end}}
{{if and .Hidden (not .Empty)}}<p class="more">{{.T.loginHint}}{{if .LoginURL}}<a href="{{.LoginURL}}">{{.T.login}}</a>{{end}}</p>{{end}}
</main>
</div>
{{if not .HideFooter}}<footer class="foot">{{if .Footer}}{{if .FooterLink}}<a href="{{.FooterLink}}" target="_blank" rel="noopener noreferrer">{{.Footer}}</a>{{else}}<span>{{.Footer}}</span>{{end}}{{else}}<svg viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="9" fill="currentColor"/><path d="M21.7 10.3A8 8 0 1 0 24 16h-7" stroke="#fff" stroke-width="3" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>Gately{{end}}</footer>{{end}}
{{if not .Preview}}<script nonce="{{.Nonce}}">` + panelJS + `</script>{{end}}
</body></html>`

// panelTpl 为导航面板的页面模板，首次渲染面板时解析
var panelTpl = newLazyTemplate("panel", panelSrc)

// panelIconData 是模板中的图标：图片地址与颜色已转为模板的安全类型，Class 引用页面中生成的颜色类。
type panelIconData struct {
	Image template.URL
	Text  string
	// Class 为底色类名；图片图标只有画在底色方块上（内置图标）时才有
	Class string
}

type panelAltData struct {
	Name  string
	Href  template.URL
	Title string
}

type panelLinkData struct {
	Name, Description, Address string
	Href                       template.URL
	NewTab, Locked             bool
	Small, HideName, HideDesc  bool
	Key                        string
	Search                     string
	Icon                       panelIconData
	Alts                       []panelAltData
}

type panelGroupData struct {
	Name  string
	Links []panelLinkData
}

// PanelCSP 返回面板页面的内容安全策略：只运行本页带 nonce 的样式与脚本，图片可来自任意 http/https 地址与 data:，
// 不能提交表单、改写 base，也只允许被同源页面嵌入。
func PanelCSP(nonce string) string {
	return "default-src 'none'; style-src 'nonce-" + nonce + "'; script-src 'nonce-" + nonce + "'; img-src data: https: http:; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
}

// panelPreviewCSP 用于管理界面预览：放在 srcdoc iframe 中，用 meta 标签声明，禁止脚本，允许页面内联样式与图标图片。
const panelPreviewCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data: https: http:; base-uri 'none'; form-action 'none'"

// newNonce 生成一次性的 CSP nonce。
func newNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// 用十六进制：base64 中的 + 在属性里会被转义成 &#43;，虽然浏览器能还原，但不便核对
	return hex.EncodeToString(b[:])
}

// panelLangNames 为语言切换菜单中各语言的自称，与管理界面的语言菜单一致。
var panelLangNames = map[string]string{
	"en": "English", "zh-CN": "简体中文", "zh-TW": "繁體中文", "ja": "日本語", "ko": "한국어",
	"es": "Español", "fr": "Français", "de": "Deutsch", "ru": "Русский", "pt-BR": "Português (Brasil)",
}

// linkKey 返回链接的稳定标识（分组加名称的哈希），浏览器按它统计常用链接；换了地址仍算同一个链接。
func linkKey(group, name string) string {
	var h uint32 = 2166136261
	for _, b := range []byte(group + "\x00" + name) {
		h ^= uint32(b)
		h *= 16777619
	}
	return strconv.FormatUint(uint64(h), 16)
}

// panelFavicon 返回标签页图标的地址：图片图标直接使用；内置图标（透明底的白色图形）画在底色方块上，
// 文字图标画成文字方块，都生成 SVG data URL；没有设置时用默认图标。颜色已由调用方校验为 #rrggbb，图片与文字经 XML 转义。
func panelFavicon(ic PanelIcon, name string) string {
	color := ic.Color
	if color == "" {
		color = paletteColor(name)
	}
	var inner string
	switch {
	case ic.Image != "" && !ic.Tile:
		return ic.Image
	case ic.Image != "":
		inner = `<image href="` + html.EscapeString(ic.Image) + `" x="6" y="6" width="20" height="20"/>`
	case ic.Text != "":
		inner = `<text x="16" y="16" font-size="17" font-family="sans-serif" font-weight="700" fill="#fff" text-anchor="middle" dominant-baseline="central">` + html.EscapeString(ic.Text) + `</text>`
	default:
		return panelDefaultFavicon
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="` + color + `"/>` + inner + `</svg>`
	return "data:image/svg+xml," + url.PathEscape(svg)
}

// iconInitial 返回名称的首个字母或汉字（大写），名称为空时为 ?。
func iconInitial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return strings.ToUpper(string(r))
		}
	}
	if r, _ := utf8.DecodeRuneInString(strings.TrimSpace(name)); r != utf8.RuneError {
		return string(r)
	}
	return "?"
}

// paletteColor 按名称稳定地挑选一个底色。
func paletteColor(name string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619
	}
	return panelPalette[h%uint32(len(panelPalette))]
}

// altTitle 返回其他访问方式的提示文字：地址中的主机与端口。
func altTitle(href string) string {
	if u, err := url.Parse(href); err == nil && u.Host != "" {
		return u.Host
	}
	return href
}

// RenderPanel 生成面板页面，返回页面内容与应放在响应头中的 CSP（预览时 CSP 已写在页面的 meta 标签中，返回空）。
func RenderPanel(r *http.Request, v PanelView) ([]byte, string) {
	lang := i18n.RequestLang(r)
	// 访客在页面上选过语言（Cookie）时以它为准
	if l := i18n.Normalize(v.Lang); l != "" {
		lang = l
	}
	nonce := newNonce()
	// colors 为页面用到的底色，按出现顺序编号为 .c0、.c1……；颜色已由调用方校验为 #rrggbb
	var colors []template.CSS
	colorIdx := map[string]int{}
	colorClass := func(c string) string {
		i, ok := colorIdx[c]
		if !ok {
			i = len(colors)
			colorIdx[c] = i
			colors = append(colors, template.CSS(c))
		}
		return "c" + strconv.Itoa(i)
	}
	icon := func(ic PanelIcon, name string) panelIconData {
		c := ic.Color
		if c == "" {
			c = paletteColor(name)
		}
		if ic.Image != "" {
			if ic.Tile {
				return panelIconData{Image: template.URL(ic.Image), Class: colorClass(c)}
			}
			return panelIconData{Image: template.URL(ic.Image)}
		}
		text := ic.Text
		if text == "" {
			text = iconInitial(name)
		}
		return panelIconData{Text: text, Class: colorClass(c)}
	}
	groups := make([]panelGroupData, 0, len(v.Groups))
	total := 0
	for _, g := range v.Groups {
		gd := panelGroupData{Name: g.Name}
		for _, l := range g.Links {
			words := []string{l.Name, l.Description, l.Address, g.Name}
			var alts []panelAltData
			for _, a := range l.Alts {
				words = append(words, a.Name)
				alts = append(alts, panelAltData{Name: a.Name, Href: template.URL(a.Href), Title: altTitle(a.Href)})
			}
			search := strings.ToLower(strings.Join(words, " "))
			gd.Links = append(gd.Links, panelLinkData{Name: l.Name, Description: l.Description, Address: l.Address,
				Href: template.URL(l.Href), NewTab: l.NewTab, Locked: l.Locked, Small: l.Small || (l.HideName && l.HideDesc && len(alts) == 0), HideName: l.HideName, HideDesc: l.HideDesc, Key: linkKey(g.Name, l.Name), Search: search, Icon: icon(l.Icon, l.Name), Alts: alts})
		}
		total += len(gd.Links)
		groups = append(groups, gd)
	}
	tabTitle := v.TabTitle
	if tabTitle == "" {
		tabTitle = v.Title
	}
	var head panelIconData
	switch {
	case v.Icon.Image != "":
		head = icon(v.Icon, v.Title)
	case v.Icon.Text != "":
		head = icon(v.Icon, v.Title)
	}
	// 标签页图标：单独设置的优先，没有时用页面图标
	favSrc := v.Favicon
	if favSrc == (PanelIcon{}) {
		favSrc = v.Icon
	}
	favicon := template.URL(panelFavicon(favSrc, v.Title))
	theme := ""
	if v.Theme == "light" || v.Theme == "dark" {
		theme = v.Theme
	}
	skin := v.Skin
	if !ValidSkin(skin) || skin == "" {
		skin = DefaultSkin()
	}
	// 搜索引擎地址放在 <option value> 里：html/template 对该属性只做 HTML 转义，页面脚本换入关键词前再核对 http(s) 协议
	type engineData struct{ Name, URL string }
	var engines []engineData
	for _, e := range v.Engines {
		engines = append(engines, engineData{Name: e.Name, URL: e.URL})
	}
	bg := template.URL("")
	if v.Background != "" {
		bg = template.URL(v.Background)
	}
	// 遮罩浓度与模糊半径是整数，直接写入样式；页面脚本按访客的设置覆盖这两个变量
	dim, blur := min(max(v.BgDim, 0), 90), min(max(v.BgBlur, 0), 20)
	bgCSS := template.CSS(":root{--bgdim:" + strconv.FormatFloat(float64(dim)/100, 'f', 2, 64) + ";--bgblur:" + strconv.Itoa(blur) + "px}")
	// 页面脚本用到的数据：文案、搜索引擎、语言列表与背景默认值；json.Marshal 会转义 < > &，放在 script 标签里安全
	type uiEngine struct {
		N string `json:"n"`
		U string `json:"u"`
	}
	type uiLang struct {
		C string `json:"c"`
		N string `json:"n"`
	}
	ui := struct {
		T       map[string]string `json:"t"`
		Engines []uiEngine        `json:"engines"`
		Def     int               `json:"def"`
		Langs   []uiLang          `json:"langs"`
		Lang    string            `json:"lang"`
		Bg      map[string]any    `json:"bg"`
		Logout  string            `json:"logout"`
		// Layout 为面板设置的链接样式（card 或 app）与图标大小（0 为默认），访客在页面设置中可以另选
		Layout struct {
			Style string `json:"style"`
			Icon  int    `json:"icon"`
		} `json:"layout"`
	}{Logout: v.LogoutURL, T: Localize(lang, panelTexts), Engines: []uiEngine{}, Def: v.DefaultEngine, Lang: lang, Bg: map[string]any{"has": v.Background != "", "dim": dim, "blur": blur}}
	engineName := Localize(lang, panelTexts)["local"]
	for i, e := range v.Engines {
		ui.Engines = append(ui.Engines, uiEngine{N: e.Name, U: e.URL})
		if i+1 == v.DefaultEngine {
			engineName = e.Name
		}
	}
	var langs []uiLang
	for _, c := range i18n.Langs {
		langs = append(langs, uiLang{C: c, N: panelLangNames[c]})
	}
	// 只有一种语言（定制构建）时不显示语言切换
	if len(langs) < 2 {
		langs = nil
	}
	ui.Langs = langs
	ui.Layout.Style, ui.Layout.Icon = "card", 0
	if v.Layout.Style == "app" {
		ui.Layout.Style = "app"
	}
	if v.Layout.IconSize >= 24 && v.Layout.IconSize <= 128 {
		ui.Layout.Icon = v.Layout.IconSize
	}
	layout, rootClass := layoutCSS(v.Layout)
	if v.Background != "" {
		rootClass = append([]string{"hasbg"}, rootClass...)
	}
	uiJSON, _ := json.Marshal(ui)
	data := map[string]any{
		"Engines": engines, "DefaultEngine": v.DefaultEngine, "Background": bg, "BgCSS": bgCSS, "LayoutCSS": template.CSS(layout), "RootClass": strings.Join(rootClass, " "), "MoreIcon": template.HTML(panelMoreIcon), "UIData": template.JS(uiJSON), "EngineName": engineName,
		"Langs": langs, "ChevronIcon": template.HTML(panelChevronIcon), "GoIcon": template.HTML(panelGoIcon), "LangIcon": template.HTML(panelLangIcon), "HideFooter": v.HideFooter, "Footer": v.Footer, "FooterLink": template.URL(v.FooterURL), "UserInitial": iconInitial(v.User), "User": v.User, "GearIcon": template.HTML(panelGearIcon),
		"LoginURL": v.LoginURL, "LogoutURL": v.LogoutURL, "Hidden": v.Hidden, "LockIcon": template.HTML(panelLockIcon),
		"Lang": lang, "T": Localize(lang, panelTexts), "Nonce": nonce,
		"Title": v.Title, "TabTitle": tabTitle, "MetaDesc": v.Description, "Subtitle": v.Subtitle, "Icon": head, "Favicon": favicon, "GridIcon": template.HTML(panelGridIcon),
		"Theme": theme, "Groups": groups,
		"SkinCSS": template.CSS(skinCSS(skin)), "Skins": skinScriptData(lang, skin, theme), "SkinT": skinLangTexts(lang).UI, "SkinIcon": template.HTML(skinIconPalette), "Empty": total == 0, "Colors": colors,
		"LAN": v.LAN, "Switchable": v.Switchable, "Forced": v.Forced, "Preview": v.Preview,
	}
	var buf bytes.Buffer
	_ = panelTpl.get().Execute(&buf, data)
	if v.Preview {
		return forPreview(buf.Bytes(), panelPreviewCSP), ""
	}
	return buf.Bytes(), PanelCSP(nonce)
}
