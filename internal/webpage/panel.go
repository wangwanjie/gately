package webpage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
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
	// Icon 为页面标题旁的图标与浏览器标签页图标，为空时使用默认图标
	Icon PanelIcon
	// Theme 为 ""（跟随系统）、light、dark
	Theme  string
	Groups []PanelGroup
	// LAN 为当前使用内网地址；Switchable 表示有链接配置了内网地址，显示内外网切换；
	// Forced 表示访客手动选择了内网或外网，显示“自动识别”
	LAN        bool
	Switchable bool
	Forced     bool
	// Preview 为管理界面中的预览：放在禁用脚本的 iframe 中，不输出脚本与搜索框
	Preview bool
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
	"search":    "搜索",
	"lan":       "内网",
	"wan":       "外网",
	"toLan":     "切换到内网",
	"toWan":     "切换到外网",
	"auto":      "自动识别",
	"noMatch":   "没有匹配的链接",
	"empty":     "还没有添加链接",
	"off":       "暂不可用",
	"netHint":   "当前使用的地址",
	"clearHint": "按 Esc 清空",
}

// panelPalette 为未指定颜色时按名称挑选的图标底色，深浅主题下白字都清晰可读。
var panelPalette = []string{"#0e7c66", "#2563eb", "#7c3aed", "#db2777", "#ea580c", "#b45309", "#15803d", "#0e7490", "#4f46e5", "#dc2626"}

// 默认图标（Gately 标志）与面板图标（四宫格）。
const (
	panelDefaultFavicon = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='9' fill='%230e7c66'/%3E%3Cpath d='M21.7 10.3A8 8 0 1 0 24 16h-7' stroke='%23fff' stroke-width='3' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3Ccircle cx='23.6' cy='8.6' r='2.2' fill='%239ff3dc'/%3E%3C/svg%3E"
	panelGridIcon       = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>`
)

// panelLight / panelDark 为浅色与深色主题的颜色变量。
const (
	panelLight = `--brand:#0e7c66;--brand-2:#18a888;--soft:rgba(14,124,102,.10);--bg:#f4f7f6;--card:#fff;--hover:#fbfdfc;--text:#1d2624;--muted:#6b7775;--faint:#9aa5a3;--line:#e3e9e7;--input:#f8faf9;--shadow:0 1px 2px rgba(16,40,34,.04),0 8px 24px -14px rgba(16,40,34,.22)`
	panelDark  = `--brand:#2bbf9c;--brand-2:#18a888;--soft:rgba(43,191,156,.14);--bg:#0f1413;--card:#171d1c;--hover:#1c2423;--text:#e4ebe9;--muted:#93a09d;--faint:#66726f;--line:#26302e;--input:#121817;--shadow:0 1px 2px rgba(0,0,0,.3),0 10px 30px -16px rgba(0,0,0,.7)`
)

const panelCSS = `
:root{` + panelLight + `}
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){` + panelDark + `}}
:root[data-theme=dark]{` + panelDark + `}
*{box-sizing:border-box}
[hidden]{display:none!important}
html{color-scheme:light dark}
:root[data-theme=light]{color-scheme:light}
:root[data-theme=dark]{color-scheme:dark}
body{margin:0;min-height:100vh;display:flex;flex-direction:column;background:var(--bg);color:var(--text);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif;-webkit-font-smoothing:antialiased}
.bg{position:fixed;inset:0;z-index:-1;background:radial-gradient(60rem 30rem at 0% -10%,var(--soft),transparent 70%),radial-gradient(50rem 28rem at 100% 0%,var(--soft),transparent 70%)}
.page{width:100%;max-width:1200px;margin:0 auto;padding:32px 24px 12px;flex:1}
.top{display:flex;align-items:center;justify-content:space-between;gap:16px 24px;flex-wrap:wrap;margin-bottom:26px}
.brand{display:flex;align-items:center;gap:14px;min-width:0}
.logo{width:48px;height:48px;flex:none;border-radius:14px;display:grid;place-items:center;color:#fff;background:linear-gradient(135deg,var(--brand-2),var(--brand));box-shadow:0 8px 20px -10px var(--brand);font-size:22px;font-weight:650;overflow:hidden}
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
.search input{width:260px;height:38px;padding:0 34px 0 34px;border:1px solid var(--line);border-radius:10px;background:var(--card);color:var(--text);font:inherit;outline:none;transition:border-color .15s,box-shadow .15s}
.search input::placeholder{color:var(--faint)}
.search input:focus{border-color:var(--brand);box-shadow:0 0 0 4px var(--soft)}
.search input::-webkit-search-cancel-button{display:none}
.search kbd{position:absolute;right:9px;min-width:18px;height:20px;padding:0 5px;border:1px solid var(--line);border-radius:5px;color:var(--faint);font:11px/18px ui-monospace,Menlo,monospace;text-align:center;background:var(--input)}
.search input:focus~kbd{display:none}
.net{display:inline-flex;align-items:center;gap:2px;height:38px;padding:3px;border:1px solid var(--line);border-radius:10px;background:var(--card)}
.net a,.net span{display:inline-flex;align-items:center;gap:6px;height:30px;padding:0 11px;border-radius:7px;color:var(--muted);text-decoration:none;font-size:13px;white-space:nowrap}
.net a:hover{color:var(--text);background:var(--input)}
.net .on{color:var(--brand);background:var(--soft);font-weight:600}
.net .dot{width:6px;height:6px;border-radius:50%;background:currentColor}
.grp{margin:0 0 26px}
.grp h2{display:flex;align-items:center;gap:8px;margin:0 0 12px;font-size:13px;font-weight:600;color:var(--muted);letter-spacing:.02em}
.grp h2 .n{min-width:20px;padding:0 6px;border-radius:999px;background:var(--input);border:1px solid var(--line);color:var(--faint);font-size:11px;font-weight:500;text-align:center}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));gap:12px}
.card{position:relative;display:flex;align-items:center;gap:12px;min-width:0;padding:12px 14px;border:1px solid var(--line);border-radius:14px;background:var(--card);color:inherit;box-shadow:var(--shadow);transition:border-color .15s,transform .15s,background .15s}
.card .hit{position:absolute;inset:0;z-index:1;border-radius:inherit}
.card.link:hover{border-color:var(--brand);background:var(--hover);transform:translateY(-1px)}
.card .hit:focus-visible{outline:none;box-shadow:0 0 0 4px var(--soft)}
.card.off>.ic,.card.off .nm,.card.off .ds{opacity:.55}
.alts{position:relative;z-index:2;display:flex;flex-wrap:wrap;gap:4px;margin-top:5px}
.alt{display:inline-block;max-width:100%;padding:1px 7px;border:1px solid var(--line);border-radius:999px;background:var(--input);color:var(--muted);font-size:11.5px;line-height:17px;text-decoration:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
a.alt:hover{border-color:var(--brand);color:var(--brand)}
.alt.off{opacity:.5;text-decoration:line-through;cursor:not-allowed}
.ic{width:40px;height:40px;flex:none;border-radius:11px;display:grid;place-items:center;color:#fff;background:var(--c,var(--brand));font-size:17px;font-weight:650;overflow:hidden}
.ic.img{background:var(--input);border:1px solid var(--line)}
.ic img{width:28px;height:28px;object-fit:contain}
.ic.tile img{width:22px;height:22px}
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
.logo{width:42px;height:42px;border-radius:12px;font-size:19px}
.tools{width:100%}
.search{flex:1 1 180px}
.search input{width:100%}
.search kbd{display:none}
.grid{grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:10px}
.card{gap:10px;padding:10px 11px;border-radius:12px}
.ic{width:34px;height:34px;border-radius:10px;font-size:15px}
.ic img{width:24px;height:24px}
.ic.tile img{width:19px;height:19px}
.nm{font-size:13.5px}
.ds{font-size:12px}
}
`

const panelSrc = `<!doctype html>
<html lang="{{.Lang}}"{{if .Theme}} data-theme="{{.Theme}}"{{end}}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>{{.Title}}</title>
<link rel="icon" href="{{.Favicon}}">
<style nonce="{{.Nonce}}">` + panelCSS + `{{range $i, $c := .Colors}}.c{{$i}}{--c:{{$c}}}{{end}}</style></head>
<body><div class="bg" aria-hidden="true"></div>
<div class="page">
<header class="top">
<div class="brand">{{with .Icon}}{{if and .Image .Class}}<span class="logo tile {{.Class}}"><img src="{{.Image}}" alt=""></span>{{else if .Image}}<span class="logo img"><img src="{{.Image}}" alt="" referrerpolicy="no-referrer"></span>{{else if .Text}}<span class="logo {{.Class}}">{{.Text}}</span>{{else}}<span class="logo">{{$.GridIcon}}</span>{{end}}{{end}}
<div><h1>{{.Title}}</h1>{{if .Subtitle}}<p>{{.Subtitle}}</p>{{end}}</div></div>
<div class="tools">
{{if not .Preview}}<label class="search"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg><input id="q" type="search" placeholder="{{.T.search}}" aria-label="{{.T.search}}" title="{{.T.clearHint}}" autocomplete="off" spellcheck="false"><kbd>/</kbd></label>{{end}}
{{if .Switchable}}<nav class="net" aria-label="{{.T.netHint}}">{{if .LAN}}<span class="on"><i class="dot"></i>{{.T.lan}}</span><a href="?net=wan" title="{{.T.toWan}}">{{.T.wan}}</a>{{else}}<a href="?net=lan" title="{{.T.toLan}}">{{.T.lan}}</a><span class="on"><i class="dot"></i>{{.T.wan}}</span>{{end}}{{if .Forced}}<a href="?net=auto">{{.T.auto}}</a>{{end}}</nav>{{end}}
</div>
</header>
<main>
{{range .Groups}}<section class="grp">{{if .Name}}<h2>{{.Name}}<span class="n">{{len .Links}}</span></h2>{{end}}<div class="grid">
{{range .Links}}<div class="card{{if .Href}} link{{else}} off{{end}}" data-s="{{.Search}}"{{if not .Href}} title="{{$.T.off}}"{{end}}>
{{if .Href}}<a class="hit" href="{{.Href}}"{{if .NewTab}} target="_blank" rel="noopener noreferrer"{{end}} title="{{.Name}}" aria-label="{{.Name}}"></a>{{end}}
{{with .Icon}}{{if and .Image .Class}}<span class="ic tile {{.Class}}"><img src="{{.Image}}" alt="" loading="lazy"></span>{{else if .Image}}<span class="ic img"><img src="{{.Image}}" alt="" loading="lazy" referrerpolicy="no-referrer"></span>{{else}}<span class="ic {{.Class}}">{{.Text}}</span>{{end}}{{end}}
<span class="tx"><span class="nm">{{.Name}}</span>{{if not .Href}}<span class="ds">{{$.T.off}}</span>{{else if .Description}}<span class="ds">{{.Description}}</span>{{else if .Address}}<span class="ds mono">{{.Address}}</span>{{end}}{{if .Alts}}<span class="alts">{{range .Alts}}{{if .Href}}<a class="alt" href="{{.Href}}" target="_blank" rel="noopener noreferrer" title="{{.Title}}">{{.Name}}</a>{{else}}<span class="alt off" title="{{$.T.off}}">{{.Name}}</span>{{end}}{{end}}</span>{{end}}</span>
</div>
{{end}}</div></section>
{{end}}
{{if .Empty}}<p class="empty">{{.T.empty}}</p>{{else}}<p id="none" class="empty" hidden>{{.T.noMatch}}</p>{{end}}
</main>
</div>
<footer class="foot"><svg viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="9" fill="currentColor"/><path d="M21.7 10.3A8 8 0 1 0 24 16h-7" stroke="#fff" stroke-width="3" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>Gately</footer>
{{if not .Preview}}<script nonce="{{.Nonce}}">` + panelJS + `</script>{{end}}
</body></html>`

// panelJS 为搜索：按空格分词，全部命中名称、描述、地址或分组才显示；/ 聚焦搜索框，Esc 清空，回车打开第一个结果。
const panelJS = `(function(){var q=document.getElementById("q");if(!q){return}
var cards=[].slice.call(document.querySelectorAll("[data-s]")),groups=[].slice.call(document.querySelectorAll(".grp")),none=document.getElementById("none");
function apply(){var terms=q.value.trim().toLowerCase().split(/\s+/).filter(Boolean),shown=0;
cards.forEach(function(c){var s=c.getAttribute("data-s"),ok=terms.every(function(t){return s.indexOf(t)>=0});c.hidden=!ok;if(ok){shown++}});
groups.forEach(function(g){g.hidden=!g.querySelector("[data-s]:not([hidden])")});if(none){none.hidden=shown>0}}
q.addEventListener("input",apply);
document.addEventListener("keydown",function(e){if(e.key==="/"&&document.activeElement!==q&&!e.metaKey&&!e.ctrlKey&&!e.altKey&&!/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)){e.preventDefault();q.focus()}});
q.addEventListener("keydown",function(e){if(e.key==="Escape"){q.value="";apply();q.blur()}else if(e.key==="Enter"){var f=cards.filter(function(c){return !c.hidden&&c.querySelector(".hit")})[0];if(f){e.preventDefault();f.querySelector(".hit").click()}}});
if(q.value){apply()}})();`

var panelTpl = template.Must(template.New("panel").Parse(panelSrc))

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
	NewTab                     bool
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
				Href: template.URL(l.Href), NewTab: l.NewTab, Search: search, Icon: icon(l.Icon, l.Name), Alts: alts})
		}
		total += len(gd.Links)
		groups = append(groups, gd)
	}
	var head panelIconData
	favicon := template.URL(panelDefaultFavicon)
	switch {
	case v.Icon.Image != "":
		head = icon(v.Icon, v.Title)
		if !v.Icon.Tile {
			// 内置图标是透明底的白色图形，不适合作标签页图标，沿用默认图标
			favicon = template.URL(v.Icon.Image)
		}
	case v.Icon.Text != "":
		head = icon(v.Icon, v.Title)
	}
	theme := ""
	if v.Theme == "light" || v.Theme == "dark" {
		theme = v.Theme
	}
	data := map[string]any{
		"Lang": lang, "T": Localize(lang, panelTexts), "Nonce": nonce,
		"Title": v.Title, "Subtitle": v.Subtitle, "Icon": head, "Favicon": favicon, "GridIcon": template.HTML(panelGridIcon),
		"Theme": theme, "Groups": groups, "Empty": total == 0, "Colors": colors,
		"LAN": v.LAN, "Switchable": v.Switchable, "Forced": v.Forced, "Preview": v.Preview,
	}
	var buf bytes.Buffer
	_ = panelTpl.Execute(&buf, data)
	if v.Preview {
		return forPreview(buf.Bytes(), panelPreviewCSP), ""
	}
	return buf.Bytes(), PanelCSP(nonce)
}
