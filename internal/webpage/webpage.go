// Package webpage 渲染访客直接看到的独立页面：Web 服务的登录页、404/502 等错误页、管理后台的安全入口提示。
//
// 所有页面共用同一套布局与样式（浅色/深色自动切换）；模板只描述结构，文案以中文字面量写在 Go 代码里，
// 渲染时按访客语言经 i18n 翻译后放进 .T，这样调整样式不需要改动各语言的译文。
package webpage

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/i18n"
)

// layoutSrc 是所有页面共用的外壳；页面通过 {{define "body"}} 提供主体内容。
const layoutSrc = `{{define "layout"}}<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow"><meta name="color-scheme" content="light dark">
<title>{{.PageTitle}}</title>
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='9' fill='%230e7c66'/%3E%3Cpath d='M21.7 10.3A8 8 0 1 0 24 16h-7' stroke='%23fff' stroke-width='3' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3Ccircle cx='23.6' cy='8.6' r='2.2' fill='%239ff3dc'/%3E%3C/svg%3E">
<style>` + baseCSS + `</style></head>
<body><div class="bg" aria-hidden="true"></div>
<main class="wrap">{{template "body" .}}</main>
<footer class="foot">{{if .Host}}<span>{{.Host}}</span><span class="sep"></span>{{end}}<span class="brand"><svg viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="9" fill="currentColor"/><path d="M21.7 10.3A8 8 0 1 0 24 16h-7" stroke="#fff" stroke-width="3" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>Gately</span></footer>
</body></html>{{end}}`

// baseCSS 为共用样式：主色与管理后台默认的翠绿主题一致。
const baseCSS = `
:root{--brand:#0e7c66;--brand-2:#18a888;--brand-soft:rgba(14,124,102,.10);--bg:#f4f7f6;--card:#fff;--text:#1d2624;--muted:#6b7775;--faint:#9aa5a3;--line:#e3e9e7;--input:#f8faf9;--danger:#d14343;--danger-soft:rgba(209,67,67,.08);--shadow:0 1px 2px rgba(16,40,34,.04),0 12px 40px -12px rgba(16,40,34,.18)}
@media (prefers-color-scheme:dark){:root{--brand:#2bbf9c;--brand-2:#18a888;--brand-soft:rgba(43,191,156,.14);--bg:#0f1413;--card:#171d1c;--text:#e4ebe9;--muted:#93a09d;--faint:#66726f;--line:#26302e;--input:#121817;--danger:#f07070;--danger-soft:rgba(240,112,112,.10);--shadow:0 1px 2px rgba(0,0,0,.3),0 16px 48px -16px rgba(0,0,0,.6)}}
*{box-sizing:border-box}
html,body{height:100%}
body{margin:0;min-height:100vh;display:flex;flex-direction:column;background:var(--bg);color:var(--text);font:14px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif;-webkit-font-smoothing:antialiased}
.bg{position:fixed;inset:0;z-index:-1;background:radial-gradient(60rem 36rem at 12% -10%,var(--brand-soft),transparent 70%),radial-gradient(48rem 30rem at 100% 110%,var(--brand-soft),transparent 70%)}
.bg::after{content:"";position:absolute;inset:0;background-image:linear-gradient(var(--line) 1px,transparent 1px),linear-gradient(90deg,var(--line) 1px,transparent 1px);background-size:44px 44px;opacity:.45;-webkit-mask-image:radial-gradient(ellipse at center,#000 10%,transparent 70%);mask-image:radial-gradient(ellipse at center,#000 10%,transparent 70%)}
.wrap{flex:1;display:flex;align-items:center;justify-content:center;padding:32px 16px 8px}
.card{width:100%;max-width:400px;background:var(--card);border:1px solid var(--line);border-radius:20px;box-shadow:var(--shadow);padding:36px 32px 30px;animation:rise .35s ease-out both}
@keyframes rise{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
.icon{width:52px;height:52px;border-radius:15px;display:grid;place-items:center;color:#fff;background:linear-gradient(135deg,var(--brand-2),var(--brand));box-shadow:0 8px 20px -8px var(--brand)}
.icon svg{width:26px;height:26px}
.icon.soft{background:var(--brand-soft);color:var(--brand);box-shadow:none}
.icon.warn{background:var(--danger-soft);color:var(--danger);box-shadow:none}
h1{font-size:21px;line-height:1.35;font-weight:650;margin:18px 0 6px;letter-spacing:-.01em}
.sub{margin:0;color:var(--muted);text-wrap:pretty}
.chip{display:inline-flex;align-items:center;gap:6px;max-width:100%;margin-top:10px;padding:3px 10px;border-radius:999px;background:var(--brand-soft);color:var(--brand);font-size:12.5px;font-weight:500}
.chip span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chip svg{width:13px;height:13px;flex:none}
.alert{display:flex;gap:8px;align-items:flex-start;margin:20px 0 0;padding:10px 12px;border-radius:10px;background:var(--danger-soft);color:var(--danger);font-size:13px}
.alert svg{width:16px;height:16px;flex:none;margin-top:2px}
form{margin:0}
.fields{margin-top:24px;display:grid;gap:14px}
label.f{display:block}
label.f>span{display:block;font-size:13px;font-weight:500;margin-bottom:6px}
.ctl{position:relative;display:flex;align-items:center}
.ctl>svg{position:absolute;left:12px;width:17px;height:17px;color:var(--faint);pointer-events:none}
.ctl input{width:100%;height:44px;padding:0 42px 0 38px;border:1px solid var(--line);border-radius:11px;background:var(--input);color:var(--text);font:inherit;font-size:15px;outline:none;transition:border-color .15s,box-shadow .15s,background .15s}
.ctl input::placeholder{color:var(--faint)}
.ctl input:focus{border-color:var(--brand);background:var(--card);box-shadow:0 0 0 4px var(--brand-soft)}
.eye{position:absolute;right:6px;width:32px;height:32px;display:grid;place-items:center;border:0;border-radius:8px;background:none;color:var(--faint);cursor:pointer}
.eye:hover{color:var(--text);background:var(--brand-soft)}
.eye svg{width:17px;height:17px}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:8px;height:44px;padding:0 18px;border-radius:11px;border:1px solid transparent;font:inherit;font-size:15px;font-weight:600;text-decoration:none;cursor:pointer;transition:filter .15s,transform .05s,background .15s}
.btn svg{width:17px;height:17px}
.btn:active{transform:translateY(1px)}
.btn.primary{width:100%;margin-top:22px;color:#fff;background:linear-gradient(135deg,var(--brand-2),var(--brand));box-shadow:0 8px 18px -10px var(--brand)}
.btn.primary:hover{filter:brightness(1.06)}
.btn.ghost{color:var(--text);background:var(--card);border-color:var(--line)}
.btn.ghost:hover{background:var(--brand-soft);border-color:transparent}
.btn[disabled]{opacity:.7;cursor:default}
.or{display:flex;align-items:center;gap:12px;margin:24px 0 14px;color:var(--faint);font-size:12.5px}
.or::before,.or::after{content:"";flex:1;height:1px;background:var(--line)}
.oauth{display:grid;gap:10px}
.oauth .btn{width:100%;font-weight:500;font-size:14px}
.actions{display:flex;gap:10px;margin-top:26px;flex-wrap:wrap}
.actions .btn{flex:1;min-width:130px}
.hint{margin-top:18px;padding:12px 14px;border-radius:12px;background:var(--input);border:1px dashed var(--line);color:var(--muted);font-size:13px}
.meta{margin-top:20px;padding-top:16px;border-top:1px solid var(--line);display:flex;flex-wrap:wrap;gap:4px 14px;color:var(--faint);font:12px/1.6 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.foot{display:flex;align-items:center;justify-content:center;gap:10px;padding:18px 16px 22px;color:var(--faint);font-size:12px;word-break:break-all}
.foot .sep{width:3px;height:3px;border-radius:50%;background:currentColor;opacity:.6}
.foot .brand{display:inline-flex;align-items:center;gap:6px;font-weight:600;color:var(--muted)}
.foot .brand svg{width:14px;height:14px;color:var(--brand)}
@media (max-width:480px){.card{padding:30px 22px 24px;border-radius:18px}}
`

var (
	mu    sync.RWMutex
	pages = map[string]*template.Template{}
)

// MustRegister 注册一个页面模板，src 需以 {{define "body"}} 定义主体；在包初始化时调用，模板错误直接 panic。
func MustRegister(name, src string) {
	t := template.Must(template.New(name).Parse(layoutSrc))
	template.Must(t.Parse(src))
	mu.Lock()
	pages[name] = t
	mu.Unlock()
}

// Localize 把以中文为键的文案表翻译成目标语言，返回新的表。
func Localize(lang string, texts map[string]string) map[string]string {
	out := make(map[string]string, len(texts))
	for k, v := range texts {
		out[k] = i18n.Tr(lang, v)
	}
	return out
}

// Render 以 status 状态码输出已注册的页面。data 会补上 Lang（访客语言）与 T（已翻译的 texts）；
// PageTitle 为浏览器标签页标题，Host 非空时显示在页脚。
func Render(w http.ResponseWriter, status int, name, lang string, texts map[string]string, data map[string]any) {
	mu.RLock()
	t := pages[name]
	mu.RUnlock()
	if t == nil {
		http.Error(w, fmt.Sprintf("%d %s", status, http.StatusText(status)), status)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["Lang"] = lang
	data["T"] = Localize(lang, texts)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = t.ExecuteTemplate(w, "layout", data)
}

// WantsHTML 判断请求方是否接受 HTML（浏览器导航请求）。
func WantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// Problem 描述一个错误页。
type Problem struct {
	Code   int
	Title  string // 标题，中文原文
	Detail string // 说明，中文原文
	Hint   string // 可选：额外的提示框内容，中文原文
	// Home 显示“返回首页”按钮；Retry 显示“重试”按钮（默认 5xx 显示）
	Home  bool
	Retry bool
	// Lock 用锁形图标（访问受限类提示），默认按状态码选择图标
	Lock bool
	// Action 非空时显示一个主按钮，跳转到该站内地址；ActionText 为按钮文字（中文原文）
	Action     string
	ActionText string
}

// icon 按错误类型选择图标：search（找不到）、lock（受限）、unplug（后端不可达）、alert（其他）。
func (p Problem) icon() string {
	switch {
	case p.Lock || p.Code == http.StatusUnauthorized || p.Code == http.StatusForbidden:
		return "lock"
	case p.Code == http.StatusNotFound:
		return "search"
	case p.Code == http.StatusBadGateway || p.Code == http.StatusServiceUnavailable || p.Code == http.StatusGatewayTimeout:
		return "unplug"
	}
	return "alert"
}

const errorSrc = `{{define "body"}}<section class="card err">
<div class="code-row"><div class="icon {{if ge .Code 500}}warn{{else}}soft{{end}}">{{if eq .Icon "search"}}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/><path d="M8.5 8.5l5 5M13.5 8.5l-5 5"/></svg>{{else if eq .Icon "lock"}}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>{{else if eq .Icon "unplug"}}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m19 5 3-3"/><path d="m2 22 3-3"/><path d="M6.3 20.3a2.4 2.4 0 0 0 3.4 0L12 18l-6-6-2.3 2.3a2.4 2.4 0 0 0 0 3.4Z"/><path d="M7.5 13.5 10 11"/><path d="M10.5 16.5 13 14"/><path d="m12 6 6 6 2.3-2.3a2.4 2.4 0 0 0 0-3.4l-2.6-2.6a2.4 2.4 0 0 0-3.4 0Z"/></svg>{{else}}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>{{end}}</div>
<div class="code {{if ge .Code 500}}warn{{end}}">{{.Code}}</div></div>
<h1>{{.Title}}</h1>
<p class="sub">{{.Detail}}</p>
{{if .Hint}}<div class="hint">{{.Hint}}</div>{{end}}
{{if or .Retry .Home .Action}}<div class="actions">
{{if .Action}}<a class="btn primary" href="{{.Action}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m16 3 4 4-4 4"/><path d="M20 7H4"/><path d="m8 21-4-4 4-4"/><path d="M4 17h16"/></svg>{{.ActionText}}</a>{{end}}
{{if .Retry}}<a class="btn primary" href=""><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>{{.T.retry}}</a>{{end}}
{{if .Home}}<a class="btn {{if or .Retry .Action}}ghost{{else}}primary{{end}}" href="/"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8"/><path d="M3 10a2 2 0 0 1 .709-1.528l7-5.999a2 2 0 0 1 2.582 0l7 5.999A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>{{.T.home}}</a>{{end}}
</div>{{end}}
<div class="meta"><span>{{.T.status}} {{.Code}} {{.Status}}</span><span>{{.Time}}</span></div>
</section>
<style>
.err .code-row{display:flex;align-items:center;justify-content:space-between}
.err .code{font-size:46px;line-height:1;font-weight:750;letter-spacing:-.04em;background:linear-gradient(135deg,var(--brand-2),var(--brand));-webkit-background-clip:text;background-clip:text;color:transparent;opacity:.9}
.err .code.warn{background:none;color:var(--danger);opacity:.85}
.err .actions .btn.primary{margin-top:0}
</style>{{end}}`

var errorTexts = map[string]string{
	"retry":  "重新加载",
	"home":   "返回首页",
	"status": "状态码",
}

func init() {
	MustRegister("error", errorSrc)
}

// Error 输出错误页：浏览器得到样式化的 HTML 页面，其他客户端得到纯文本，与 http.Error 行为一致。
func Error(w http.ResponseWriter, r *http.Request, p Problem) {
	if !WantsHTML(r) {
		http.Error(w, fmt.Sprintf("%d %s", p.Code, http.StatusText(p.Code)), p.Code)
		return
	}
	lang, data := problemData(r, p)
	Render(w, p.Code, "error", lang, errorTexts, data)
}

// problemData 返回渲染错误页所需的访客语言与模板数据（文案已按访客语言翻译）。
func problemData(r *http.Request, p Problem) (string, map[string]any) {
	lang := i18n.RequestLang(r)
	title := i18n.Tr(lang, p.Title)
	return lang, map[string]any{
		"PageTitle":  fmt.Sprintf("%d · %s", p.Code, title),
		"Code":       p.Code,
		"Icon":       p.icon(),
		"Status":     http.StatusText(p.Code),
		"Title":      title,
		"Detail":     i18n.Tr(lang, p.Detail),
		"Hint":       i18n.Tr(lang, p.Hint),
		"Home":       p.Home,
		"Action":     p.Action,
		"ActionText": i18n.Tr(lang, p.ActionText),
		"Retry":      p.Retry || p.Code >= 500,
		"Host":       r.Host,
		"Time":       time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	}
}
