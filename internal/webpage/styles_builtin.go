package webpage

import (
	"fmt"
	"strings"
)

// 内置的十种错误页风格。每个模板通过 {{define "page"}} 给出完整页面，可用的数据见 problemData 与 RenderError：
// Code、Status（英文状态短语）、Title、Detail、Hint、Host、Path、Time、Server（5xx）、NotFound（404）、
// Action/ActionText/Retry/Home 与已翻译的 .T 文案。所有风格都需适配 360px 宽的手机屏幕，动画在“减少动态效果”时关闭。

// fontSans 与 fontMono 为各风格共用的系统字体栈，不加载任何网络字体。
const (
	fontSans = `-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Hiragino Sans GB","Microsoft YaHei","Noto Sans CJK SC",sans-serif`
	fontMono = `ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace`
)

// baseReset 为各风格共用的基础样式。
const baseReset = `*{box-sizing:border-box}html,body{min-height:100%}body{margin:0;-webkit-font-smoothing:antialiased;overflow-wrap:anywhere}
a{-webkit-tap-highlight-color:transparent}.acts{display:flex;flex-wrap:wrap;gap:12px}
@media (prefers-reduced-motion:reduce){*,*::before,*::after{animation:none!important;transition:none!important}}`

// minimal：极简，留白与超大细体数字。
const styleMinimal = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="light dark"><style>` + baseReset + `
:root{--bg:#fff;--fg:#111;--mut:#6b6b70;--line:#e7e7ea}
@media (prefers-color-scheme:dark){:root{--bg:#0b0b0c;--fg:#f3f3f4;--mut:#93939a;--line:#25252a}}
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:40px 24px;background:var(--bg);color:var(--fg);font:16px/1.65 ` + fontSans + `}
main{width:100%;max-width:560px}
.code{margin:0;font-size:clamp(96px,26vw,176px);font-weight:200;line-height:.9;letter-spacing:-.06em}
h1{margin:30px 0 8px;font-size:clamp(21px,5vw,27px);font-weight:600;line-height:1.35}
p{margin:0;color:var(--mut)}.hint{margin-top:14px}
.acts{gap:22px;margin-top:34px}
.b{color:var(--fg);text-decoration:none;font-weight:500;border-bottom:1.5px solid currentColor;padding-bottom:2px}
.b.sec{color:var(--mut)}.b:hover{opacity:.7}
.meta{display:flex;flex-wrap:wrap;gap:4px 18px;margin-top:64px;padding-top:16px;border-top:1px solid var(--line);color:var(--mut);font:12px/1.6 ` + fontMono + `}
</style></head><body><main>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<div class="meta"><span>{{.Code}} {{.Status}}</span>{{if .Host}}<span>{{.Host}}</span>{{end}}<span>{{.Time}}</span></div>
</main></body></html>{{end}}`

// terminal：终端窗口，以命令行输出的形式展示响应。
const styleTerminal = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="dark"><style>` + baseReset + `
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:28px 14px;background:#0d1117;color:#c9d1d9;font:14px/1.75 ` + fontMono + `}
.win{width:100%;max-width:740px;border:1px solid #30363d;border-radius:12px;background:#010409;box-shadow:0 30px 80px -24px rgba(0,0,0,.85);overflow:hidden}
.bar{display:flex;align-items:center;gap:8px;padding:11px 14px;background:#161b22;border-bottom:1px solid #30363d}
.bar i{flex:none;width:12px;height:12px;border-radius:50%;background:#ff5f56}.bar i:nth-child(2){background:#ffbd2e}.bar i:nth-child(3){background:#27c93f}
.bar span{min-width:0;margin-left:6px;overflow:hidden;color:#8b949e;font-size:12px;text-overflow:ellipsis;white-space:nowrap}
.out{padding:20px 20px 24px}.out p{margin:0}.gap{height:12px}
.p{color:#7ee787;user-select:none}.c{color:#e6edf3}.e{color:#ff7b72;font-weight:600}.w{color:#e3b341;font-weight:600}.cm{color:#8b949e}
.cur{display:inline-block;width:.6em;height:1.15em;vertical-align:-.22em;background:#c9d1d9;animation:blink 1.1s steps(1) infinite}
@keyframes blink{50%{opacity:0}}
.acts{gap:6px 20px;margin:16px 0 10px}
.b{color:#79c0ff;text-decoration:none}.b::before{content:"[ ";color:#8b949e}.b::after{content:" ]";color:#8b949e}.b:hover{color:#a5d6ff;text-decoration:underline}
@media (max-width:480px){body{font-size:13px}.out{padding:16px 14px 20px}}
</style></head><body><main class="win"><div class="bar" aria-hidden="true"><i></i><i></i><i></i><span>{{.Host}}</span></div><div class="out">
<p><span class="p">$ </span><span class="c">curl -I {{.Host}}{{.Path}}</span></p>
<p class="{{if .Server}}e{{else}}w{{end}}">HTTP/1.1 {{.Code}} {{.Status}}</p>
<p class="cm">date: {{.Time}}</p>
<div class="gap"></div>
<h1 class="c" style="margin:0;font-size:inherit;font-weight:600"># {{.Title}}</h1>
<p class="cm"># {{.Detail}}</p>
{{if .Hint}}<p class="cm"># {{.Hint}}</p>{{end}}
{{template "actions" .}}
<p><span class="p">$ </span><span class="cur" aria-hidden="true"></span></p>
</div></main></body></html>{{end}}`

// neon：霓虹灯牌，暗色背景与透视网格地面。
const styleNeon = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="dark"><style>` + baseReset + `
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:40px 20px 30vh;overflow-x:hidden;background:radial-gradient(circle at 50% 28%,#2a0f47 0%,#0b0418 62%,#05020c 100%);color:#e9e3ff;font:16px/1.65 ` + fontSans + `;text-align:center}
.floor{position:fixed;left:-50%;right:-50%;bottom:0;z-index:-1;height:42vh;background-image:linear-gradient(rgba(255,43,214,.4) 1px,transparent 1px),linear-gradient(90deg,rgba(255,43,214,.4) 1px,transparent 1px);background-size:56px 56px;transform:perspective(320px) rotateX(62deg);transform-origin:bottom;-webkit-mask-image:linear-gradient(to top,#000 10%,transparent);mask-image:linear-gradient(to top,#000 10%,transparent)}
main{max-width:560px}
.code{margin:0;font-size:clamp(100px,28vw,200px);font-weight:800;line-height:1;color:#fff5fd;text-shadow:0 0 4px #fff,0 0 16px #ff2bd6,0 0 40px #ff2bd6,0 0 90px #ff2bd6;animation:flick 5s infinite}
.c4 .code{text-shadow:0 0 4px #fff,0 0 16px #20e3ff,0 0 40px #20e3ff,0 0 90px #20e3ff}
@keyframes flick{0%,18%,22%,25%,53%,57%,100%{opacity:1}20%,24%,55%{opacity:.45}}
h1{margin:18px 0 10px;font-size:clamp(21px,5.4vw,29px);letter-spacing:.04em;color:#20e3ff;text-shadow:0 0 12px rgba(32,227,255,.55)}
.c4 h1{color:#ff7be6;text-shadow:0 0 12px rgba(255,43,214,.55)}
p{margin:0 auto;max-width:470px;color:#bdb4dc}.hint{margin-top:12px}
.acts{justify-content:center;gap:14px;margin-top:32px}
.b{padding:10px 24px;border:1.5px solid #ff2bd6;border-radius:999px;color:#ffe0f8;text-decoration:none;box-shadow:0 0 14px rgba(255,43,214,.45),inset 0 0 14px rgba(255,43,214,.22)}
.b.sec{border-color:#20e3ff;color:#d6f8ff;box-shadow:0 0 14px rgba(32,227,255,.4),inset 0 0 14px rgba(32,227,255,.2)}
.b:hover{background:rgba(255,255,255,.06)}
.meta{margin-top:34px;color:#7d72a6;font:12px/1.7 ` + fontMono + `}
</style></head><body class="{{if not .Server}}c4{{end}}"><div class="floor" aria-hidden="true"></div><main>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<p class="meta">{{.Code}} {{.Status}}{{if .Host}} · {{.Host}}{{end}} · {{.Time}}</p>
</main></body></html>{{end}}`

// space：星空与行星，星点由多层径向渐变平铺而成。
const styleSpace = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="dark"><style>` + baseReset + `
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:40px 20px;overflow-x:hidden;background:radial-gradient(ellipse at 72% 0%,#33378a 0%,#161a57 36%,#070b24 78%);color:#eef0ff;font:16px/1.65 ` + fontSans + `;text-align:center}
.sky{position:fixed;inset:0;z-index:-1;background-image:radial-gradient(1.5px 1.5px at 24px 36px,#fff,transparent),radial-gradient(1px 1px at 90px 120px,#dfe3ff,transparent),radial-gradient(1.5px 1.5px at 160px 60px,#fff,transparent),radial-gradient(1px 1px at 210px 170px,#cfd5ff,transparent),radial-gradient(1px 1px at 60px 200px,#fff,transparent),radial-gradient(2px 2px at 250px 20px,#fff8e1,transparent);background-size:280px 230px;animation:twinkle 6s ease-in-out infinite alternate}
.sky.far{background-size:170px 150px;opacity:.5;animation-delay:-3s}
@keyframes twinkle{from{opacity:.55}to{opacity:1}}
.planet{position:relative;width:clamp(110px,30vw,168px);aspect-ratio:1;margin:0 auto 34px;border-radius:50%;background:radial-gradient(circle at 32% 30%,#ffd9a8 0%,#f39a63 42%,#7a2f60 100%);box-shadow:inset -20px -16px 34px rgba(0,0,0,.45),0 0 70px rgba(243,154,99,.35);animation:float 7s ease-in-out infinite}
.planet::before{content:"";position:absolute;left:-34%;right:-34%;top:42%;height:20%;border:3px solid rgba(255,224,190,.6);border-top-color:transparent;border-radius:50%;transform:rotate(-16deg)}
.planet::after{content:"";position:absolute;width:18%;height:18%;top:24%;left:56%;border-radius:50%;background:rgba(122,47,96,.35)}
@keyframes float{50%{transform:translateY(-10px)}}
main{max-width:540px}
.code{margin:0;font-size:clamp(68px,18vw,124px);font-weight:800;line-height:1;letter-spacing:.06em;background:linear-gradient(180deg,#fff 20%,#a9b2ff);-webkit-background-clip:text;background-clip:text;color:transparent}
h1{margin:14px 0 10px;font-size:clamp(21px,5vw,27px)}
p{margin:0 auto;max-width:460px;color:#b9c0ee}.hint{margin-top:12px}
.acts{justify-content:center;margin-top:30px}
.b{padding:10px 22px;border-radius:12px;color:#fff;text-decoration:none;background:rgba(255,255,255,.1);border:1px solid rgba(255,255,255,.22)}
.b.pri{background:linear-gradient(135deg,#7b83ff,#b06ab3);border-color:transparent}.b:hover{filter:brightness(1.12)}
.meta{margin-top:30px;color:#7d84b8;font:12px/1.7 ` + fontMono + `}
</style></head><body><div class="sky" aria-hidden="true"></div><div class="sky far" aria-hidden="true"></div><main>
<div class="planet" aria-hidden="true"></div>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<p class="meta">{{.Code}} {{.Status}}{{if .Host}} · {{.Host}}{{end}} · {{.Time}}</p>
</main></body></html>{{end}}`

// paper：报刊版式，衬线字体、双线分隔与首字下沉。
const stylePaper = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="light dark"><style>` + baseReset + `
:root{--bg:#f4efe4;--fg:#231f1a;--mut:#6f665a}
@media (prefers-color-scheme:dark){:root{--bg:#1b1916;--fg:#ece4d4;--mut:#a39886}}
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:clamp(24px,7vw,72px) 18px;background:var(--bg);color:var(--fg);font:17px/1.7 Georgia,"Times New Roman","Songti SC","STSong","SimSun",serif}
article{width:100%;max-width:640px;padding:16px 0 20px;border-top:5px double currentColor;border-bottom:5px double currentColor}
.mast{display:flex;flex-wrap:wrap;justify-content:space-between;gap:4px 14px;padding-bottom:10px;border-bottom:1px solid currentColor;color:var(--mut);font:11.5px/1.5 ` + fontMono + `;letter-spacing:.12em;text-transform:uppercase}
.code{margin:24px 0 2px;font-size:clamp(76px,21vw,140px);font-style:italic;font-weight:700;line-height:.95}
h1{margin:8px 0 16px;font-size:clamp(25px,6vw,38px);font-weight:700;line-height:1.2}
.lead{margin:0}.lead::first-letter{float:left;padding:6px 10px 0 0;font-size:3.3em;font-weight:700;line-height:.8}
.hint{clear:both;margin:16px 0 0;padding-left:14px;border-left:3px solid currentColor;color:var(--mut);font-style:italic}
.acts{clear:both;gap:24px;margin-top:28px}
.b{color:inherit;font-style:italic;text-decoration:underline;text-underline-offset:5px;text-decoration-thickness:1px}.b:hover{color:var(--mut)}
.foot{clear:both;margin-top:26px;color:var(--mut);font:11.5px/1.5 ` + fontMono + `;letter-spacing:.08em;text-transform:uppercase}
</style></head><body><article>
<div class="mast"><span>{{if .Host}}{{.Host}}{{else}}—{{end}}</span><span>{{.Time}}</span></div>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p class="lead">{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<p class="foot">{{.T.status}} {{.Code}} · {{.Status}}</p>
</article></body></html>{{end}}`

// brutalist：新粗野主义，高饱和底色、粗黑边框与硬投影。
const styleBrutalist = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="light"><style>` + baseReset + `
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:28px 18px 36px;background:#ffe14d;color:#000;font:16px/1.55 ` + fontSans + `}
.box{width:100%;max-width:640px;padding:clamp(22px,5.5vw,42px);background:#fff;border:4px solid #000;box-shadow:12px 12px 0 #000}
.tag{display:inline-block;padding:4px 10px;background:#000;color:#ffe14d;font:700 12px/1.4 ` + fontMono + `;letter-spacing:.1em;text-transform:uppercase}
.code{margin:16px 0 6px;font-size:clamp(84px,25vw,172px);font-weight:900;line-height:.86;letter-spacing:-.05em}
h1{margin:0 0 10px;font-size:clamp(23px,5.6vw,33px);font-weight:900;line-height:1.15;text-transform:uppercase}
p{margin:0;font-weight:500}.hint{margin-top:12px;padding:10px 12px;background:#ffe14d;border:3px solid #000}
.acts{gap:14px;margin-top:28px}
.b{padding:11px 20px;background:#fff;border:3px solid #000;color:#000;font-weight:800;text-decoration:none;box-shadow:5px 5px 0 #000;transition:transform .08s,box-shadow .08s}
.b.pri{background:#4dd2ff}.b:hover,.b:active{transform:translate(3px,3px);box-shadow:2px 2px 0 #000}
.meta{display:flex;flex-wrap:wrap;gap:4px 18px;margin-top:28px;padding-top:12px;border-top:3px solid #000;font:12px/1.6 ` + fontMono + `}
@media (max-width:480px){.box{box-shadow:7px 7px 0 #000;border-width:3px}}
</style></head><body><main class="box">
<span class="tag">{{.T.status}} {{.Code}}</span>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<div class="meta"><span>{{.Status}}</span>{{if .Host}}<span>{{.Host}}</span>{{end}}<span>{{.Time}}</span></div>
</main></body></html>{{end}}`

// aurora：极光渐变与毛玻璃卡片。
const styleAurora = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="light dark"><style>` + baseReset + `
:root{--bg:#f3f5ff;--fg:#111633;--mut:#545b7e;--glass:rgba(255,255,255,.58);--edge:rgba(255,255,255,.85);--o:.45}
@media (prefers-color-scheme:dark){:root{--bg:#0a0f1f;--fg:#f2f5ff;--mut:#aab2d6;--glass:rgba(255,255,255,.07);--edge:rgba(255,255,255,.16);--o:.55}}
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:32px 18px;overflow-x:hidden;background:var(--bg);color:var(--fg);font:16px/1.65 ` + fontSans + `}
.blob{position:fixed;z-index:-1;width:62vmax;height:62vmax;border-radius:50%;filter:blur(80px);opacity:var(--o);animation:drift 20s ease-in-out infinite alternate}
.b1{top:-24vmax;left:-18vmax;background:#22d3ee}.b2{right:-18vmax;bottom:-28vmax;background:#a855f7;animation-delay:-7s}.b3{top:34%;left:42%;width:40vmax;height:40vmax;background:#10b981;animation-delay:-13s}
@keyframes drift{to{transform:translate(9vmax,7vmax) scale(1.15)}}
.glass{width:100%;max-width:470px;padding:clamp(26px,6.5vw,42px);border-radius:26px;background:var(--glass);border:1px solid var(--edge);-webkit-backdrop-filter:blur(20px) saturate(150%);backdrop-filter:blur(20px) saturate(150%);box-shadow:0 24px 70px -26px rgba(10,15,50,.45)}
.code{margin:0;font-size:clamp(66px,19vw,100px);font-weight:750;line-height:1;letter-spacing:-.04em;background:linear-gradient(120deg,#06b6d4,#a855f7 55%,#10b981);-webkit-background-clip:text;background-clip:text;color:transparent}
h1{margin:16px 0 8px;font-size:clamp(20px,5vw,25px);line-height:1.35}
p{margin:0;color:var(--mut)}.hint{margin-top:12px}
.acts{gap:12px;margin-top:28px}.acts .b{flex:1;min-width:130px}
.b{display:inline-flex;align-items:center;justify-content:center;height:44px;padding:0 18px;border-radius:14px;color:var(--fg);font-weight:600;text-decoration:none;background:var(--glass);border:1px solid var(--edge)}
.b.pri{color:#fff;background:linear-gradient(120deg,#0891b2,#9333ea);border-color:transparent}.b:hover{filter:brightness(1.08)}
.meta{display:flex;flex-wrap:wrap;gap:4px 14px;margin-top:26px;color:var(--mut);font:12px/1.6 ` + fontMono + `;opacity:.85}
</style></head><body><div class="blob b1" aria-hidden="true"></div><div class="blob b2" aria-hidden="true"></div><div class="blob b3" aria-hidden="true"></div>
<main class="glass"><p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<div class="meta"><span>{{.Code}} {{.Status}}</span>{{if .Host}}<span>{{.Host}}</span>{{end}}<span>{{.Time}}</span></div>
</main></body></html>{{end}}`

// pixelSprite 为像素风格中的小怪物，每行一个字符串，X 为实心像素。
var pixelSprite = []string{
	"..X.....X..",
	"...X...X...",
	"..XXXXXXX..",
	".XX.XXX.XX.",
	"XXXXXXXXXXX",
	"X.XXXXXXX.X",
	"X.X.....X.X",
	"...XX.XX...",
}

// pixelShadow 把像素图转换成 box-shadow 列表，每个像素是 1 个 --px 单位的方块。
func pixelShadow(rows []string) string {
	var parts []string
	for y, row := range rows {
		for x, c := range row {
			if c == 'X' {
				parts = append(parts, fmt.Sprintf("calc(var(--px)*%d) calc(var(--px)*%d) 0 #53d8fb", x, y))
			}
		}
	}
	return strings.Join(parts, ",")
}

// pixel：八位机像素风，CRT 扫描线与像素小怪物。
var stylePixel = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="dark"><style>` + baseReset + `
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:32px 16px;background:#1b1b2f;color:#f8f8f2;font:15px/1.75 "Courier New",` + fontMono + `;text-align:center}
body::after{content:"";position:fixed;inset:0;pointer-events:none;background:repeating-linear-gradient(0deg,rgba(0,0,0,.28) 0 1px,transparent 1px 3px)}
main{max-width:560px}
.sprite{--px:7px;position:relative;width:calc(var(--px)*11);height:calc(var(--px)*8);margin:0 auto 26px;animation:bob 1s steps(2) infinite}
.sprite::before{content:"";position:absolute;top:0;left:0;width:var(--px);height:var(--px);box-shadow:` + pixelShadow(pixelSprite) + `}
@keyframes bob{50%{transform:translateY(calc(var(--px)*-1))}}
.code{margin:0 0 10px;font-size:clamp(58px,17vw,108px);font-weight:700;line-height:1.05;letter-spacing:.08em;color:#ffcc00;text-shadow:4px 4px 0 #e94560,8px 8px 0 #0f3460}
h1{margin:0 0 8px;font-size:clamp(18px,4.8vw,24px);line-height:1.4;letter-spacing:.06em;text-transform:uppercase;color:#53d8fb}
p{margin:0 auto;max-width:460px;color:#c7c7d9}.hint{margin-top:12px;color:#ffcc00}
.acts{justify-content:center;gap:16px;margin-top:30px}
.b{padding:10px 18px;background:#e94560;color:#fff;text-decoration:none;text-transform:uppercase;letter-spacing:.06em;box-shadow:inset -4px -4px 0 rgba(0,0,0,.35),inset 4px 4px 0 rgba(255,255,255,.25),0 0 0 4px #000}
.b.sec{background:#0f3460}.b:hover,.b:active{transform:translateY(2px)}
.b.pri::before{content:"▶ ";animation:blink 1s steps(1) infinite}@keyframes blink{50%{opacity:0}}
.meta{margin-top:30px;color:#8585a8;font-size:12px;letter-spacing:.04em}
@media (max-width:480px){.sprite{--px:6px}}
</style></head><body><main>
<div class="sprite" aria-hidden="true"></div>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<p class="meta">{{.Code}} {{.Status}}{{if .Host}} · {{.Host}}{{end}} · {{.Time}}</p>
</main></body></html>{{end}}`

// blueprint：工程蓝图，网格底纹、描边数字与图纸标题栏。
const styleBlueprint = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="dark"><style>` + baseReset + `
body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:30px 16px;background-color:#0b3d91;background-image:linear-gradient(rgba(255,255,255,.07) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.07) 1px,transparent 1px),linear-gradient(rgba(255,255,255,.16) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.16) 1px,transparent 1px);background-size:20px 20px,20px 20px,100px 100px,100px 100px;color:#e8f0ff;font:15px/1.65 ` + fontMono + `}
.sheet{position:relative;width:100%;max-width:680px;padding:clamp(22px,5vw,38px);border:2px solid rgba(232,240,255,.9);background:rgba(11,61,145,.55)}
.sheet::before{content:"";position:absolute;inset:6px;border:1px solid rgba(232,240,255,.35);pointer-events:none}
.dim{display:flex;align-items:center;gap:10px;color:#bcd2ff;font-size:12px}.dim::before,.dim::after{content:"";flex:1;height:1px;background:currentColor}
.code{margin:6px 0 14px;font-size:clamp(76px,21vw,152px);font-weight:300;line-height:1;letter-spacing:.06em;text-align:center;color:transparent;-webkit-text-stroke:1.5px #e8f0ff}
h1{margin:0 0 6px;font-size:clamp(18px,4.6vw,24px);font-weight:600;line-height:1.4}
p{margin:0;color:#c9dbff}.hint{margin-top:10px;padding:8px 12px;border:1px dashed rgba(232,240,255,.6)}
.acts{gap:12px;margin-top:24px}
.b{padding:8px 16px;border:1px dashed #e8f0ff;color:#e8f0ff;text-decoration:none}.b.pri{border-style:solid;background:rgba(232,240,255,.14)}.b:hover{background:rgba(232,240,255,.22)}
table{width:100%;margin-top:26px;border-collapse:collapse;font-size:12px;table-layout:fixed}
td{padding:6px 10px;border:1px solid rgba(232,240,255,.6);overflow-wrap:anywhere}td:first-child{width:30%;color:#bcd2ff;text-transform:uppercase}
</style></head><body><main class="sheet">
<div class="dim" aria-hidden="true">{{.Code}}</div>
<p class="code">{{.Code}}</p><h1>{{.Title}}</h1><p>{{.Detail}}</p>{{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
{{template "actions" .}}
<table><tr><td>{{.T.status}}</td><td>{{.Code}} {{.Status}}</td></tr>{{if .Host}}<tr><td>{{.T.host}}</td><td>{{.Host}}</td></tr>{{end}}<tr><td>{{.T.time}}</td><td>{{.Time}}</td></tr></table>
</main></body></html>{{end}}`

// status：状态链路，按浏览器 → 网关 → 站点三段标出哪一段出了问题。
const styleStatus = `{{define "page"}}<!doctype html><html lang="{{.Lang}}"><head>{{template "head" .}}<meta name="color-scheme" content="light dark"><style>` + baseReset + `
:root{--bg:#f5f6f8;--card:#fff;--fg:#1c1f24;--mut:#626a75;--line:#e1e4e8;--ok:#16a34a;--okbg:rgba(22,163,74,.1);--bad:#dc2626;--badbg:rgba(220,38,38,.1);--warn:#d97706;--warnbg:rgba(217,119,6,.12)}
@media (prefers-color-scheme:dark){:root{--bg:#0f1115;--card:#171a20;--fg:#e8ebf0;--mut:#9aa3ae;--line:#2a2f37;--ok:#4ade80;--okbg:rgba(74,222,128,.12);--bad:#f87171;--badbg:rgba(248,113,113,.12);--warn:#fbbf24;--warnbg:rgba(251,191,36,.12)}}
body{min-height:100vh;padding:clamp(28px,7vw,72px) 18px;background:var(--bg);color:var(--fg);font:15px/1.65 ` + fontSans + `}
main{max-width:880px;margin:0 auto}
.pill{display:inline-block;padding:2px 10px;border-radius:999px;background:var(--badbg);color:var(--bad);font:600 12.5px/1.7 ` + fontMono + `}.c4 .pill{background:var(--warnbg);color:var(--warn)}
h1{margin:12px 0 6px;font-size:clamp(24px,5.6vw,34px);line-height:1.25}
.lead{margin:0;color:var(--mut)}
.chain{display:grid;grid-template-columns:repeat(3,1fr);gap:22px;margin:34px 0}
.node{position:relative;padding:20px 14px;border:1px solid var(--line);border-radius:16px;background:var(--card);text-align:center}
.node:not(:last-child)::after{content:"";position:absolute;top:50%;right:-23px;width:24px;height:2px;background:var(--line)}
.ic{display:grid;place-items:center;width:46px;height:46px;margin:0 auto 10px;border-radius:50%}.ic svg{width:24px;height:24px}
.ok .ic{background:var(--okbg);color:var(--ok)}.bad .ic{background:var(--badbg);color:var(--bad)}.warn .ic{background:var(--warnbg);color:var(--warn)}
.node b{display:block;font-size:15px}.node small{display:block;margin-top:2px;color:var(--mut);font-size:12.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.st{display:block;margin-top:6px;font-weight:600;font-size:13px}.ok .st{color:var(--ok)}.bad .st{color:var(--bad)}.warn .st{color:var(--warn)}
.cols{display:grid;grid-template-columns:1fr 1fr;gap:22px}
.cols h2{margin:0 0 6px;font-size:16px}.cols p{margin:0;color:var(--mut)}
.acts{margin-top:14px}
.b{display:inline-flex;align-items:center;height:40px;padding:0 16px;border-radius:10px;border:1px solid var(--line);background:var(--card);color:var(--fg);font-weight:600;text-decoration:none}
.b.pri{background:var(--fg);border-color:var(--fg);color:var(--bg)}.b:hover{filter:brightness(1.1)}
.meta{display:flex;flex-wrap:wrap;gap:4px 18px;margin-top:40px;padding-top:14px;border-top:1px solid var(--line);color:var(--mut);font:12px/1.6 ` + fontMono + `}
@media (max-width:640px){.chain{grid-template-columns:1fr;gap:14px}.node{display:grid;grid-template-columns:auto 1fr auto;align-items:center;gap:12px;padding:14px 16px;text-align:left}.ic{margin:0;width:40px;height:40px}.st{margin:0}
.node:not(:last-child)::after{top:auto;right:auto;bottom:-15px;left:35px;width:2px;height:16px}.cols{grid-template-columns:1fr}}
</style></head><body class="{{if not .Server}}c4{{end}}"><main>
<span class="pill">{{.T.status}} {{.Code}}</span><h1>{{.Title}}</h1><p class="lead">{{.Detail}}</p>
<div class="chain">
<div class="node ok"><div class="ic"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg></div><div><b>{{.T.browser}}</b><small>{{.T.you}}</small></div><span class="st">{{.T.ok}}</span></div>
<div class="node ok"><div class="ic"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg></div><div><b>{{.T.gateway}}</b><small>{{.Time}}</small></div><span class="st">{{.T.ok}}</span></div>
{{if .Server}}<div class="node bad"><div class="ic"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></div><div><b>{{.T.site}}</b><small>{{.Host}}</small></div><span class="st">{{.T.failed}}</span></div>
{{else}}<div class="node warn"><div class="ic"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg></div><div><b>{{.T.site}}</b><small>{{.Host}}</small></div><span class="st">{{if .NotFound}}{{.T.missing}}{{else}}{{.Status}}{{end}}</span></div>{{end}}
</div>
<div class="cols"><section><h2>{{.T.what}}</h2><p>{{.Detail}}</p>{{if .Hint}}<p>{{.Hint}}</p>{{end}}</section>
<section><h2>{{.T.todo}}</h2><p>{{if .Server}}{{.T.todo5xx}}{{else if .NotFound}}{{.T.todo404}}{{else}}{{.T.todoOther}}{{end}}</p>{{template "actions" .}}</section></div>
<div class="meta"><span>{{.Code}} {{.Status}}</span>{{if .Host}}<span>{{.Host}}</span>{{end}}<span>{{.Time}}</span></div>
</main></body></html>{{end}}`

func init() {
	// 顺序即管理界面中的展示顺序
	registerStyle("minimal", styleMinimal)
	registerStyle("terminal", styleTerminal)
	registerStyle("neon", styleNeon)
	registerStyle("space", styleSpace)
	registerStyle("paper", stylePaper)
	registerStyle("brutalist", styleBrutalist)
	registerStyle("aurora", styleAurora)
	registerStyle("pixel", stylePixel)
	registerStyle("blueprint", styleBlueprint)
	registerStyle("status", styleStatus)
}
