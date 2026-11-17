package webpage

// 导航面板页面的交互：搜索与自绘的搜索引擎菜单、语言切换、页面设置（背景图片、模糊、遮罩）、分组折叠、常用链接。
// 访客的选择都保存在浏览器的 localStorage 中；语言同时写入 Cookie，由服务端按它输出页面文案。
// 页面不使用 innerHTML 插入文案，只用它插入下面程序内置的图标。

// 菜单与设置用到的图标（Lucide，ISC 许可）。
const (
	panelChevronIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>`
	panelGoIcon      = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>`
	panelLangIcon    = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m5 8 6 6M4 14l6-6 2-3M2 5h12M7 2h1M22 22l-5-10-5 10M14 18h6"/></svg>`
	panelGearIcon    = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>`
	panelLogoutIcon  = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/></svg>`
	panelCheckIcon   = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>`
)

// panelUICSS 为新增交互的样式。
const panelUICSS = `
.bg{overflow:hidden}
.bg img{position:absolute;inset:-24px;width:calc(100% + 48px);height:calc(100% + 48px);object-fit:cover;filter:blur(var(--bgblur,0px))}
.bg.has::after{opacity:var(--bgdim,.45)}
.sbox{position:relative}
.eng{display:inline-flex;align-items:center;gap:6px;height:38px;max-width:150px;padding:0 10px 0 13px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);color:var(--text);font:inherit;cursor:pointer;white-space:nowrap;transition:border-color .15s,box-shadow .15s}
.eng span{overflow:hidden;text-overflow:ellipsis}
.eng svg{width:15px;height:15px;flex:none;color:var(--faint);transition:transform .15s}
.eng:hover,.eng[aria-expanded=true]{border-color:var(--brand)}
.eng[aria-expanded=true] svg{transform:rotate(180deg)}
.eng:focus-visible{outline:none;border-color:var(--brand);box-shadow:0 0 0 4px var(--soft)}
.go{position:absolute;right:5px;display:grid;place-items:center;width:28px;height:28px;padding:0;border:0;border-radius:calc(var(--r-sm) - 2px);background:var(--soft);color:var(--brand);cursor:pointer}
.go svg{position:static;left:auto;width:17px;height:17px;color:inherit;stroke-width:2.6}
.go:hover{background:var(--brand);color:var(--bg)}
.go:focus-visible{outline:none;box-shadow:0 0 0 3px var(--soft)}
html.nosc{overflow:hidden}
.search.web input{padding-right:42px}
.search.web kbd{display:none}
.pw{position:relative}
.pop{position:absolute;top:calc(100% + 8px);right:0;z-index:30;min-width:200px;max-width:min(92vw,320px);max-height:min(70vh,520px);overflow:auto;overscroll-behavior:contain;padding:6px;border:1px solid var(--line);border-radius:var(--r);background:var(--card);color:var(--text);box-shadow:0 18px 50px -18px rgba(0,0,0,.45);text-align:left}
.pop.em{left:0;right:auto;min-width:160px}
.pop button{font:inherit;color:inherit}
.pi{display:flex;align-items:center;gap:8px;width:100%;min-height:34px;padding:0 10px;border:0;border-radius:calc(var(--r-sm) - 2px);background:transparent;text-align:left;cursor:pointer;white-space:nowrap}
.pi:hover,.pi:focus-visible{outline:none;background:var(--input)}
.pi .ck{width:14px;height:14px;flex:none;color:var(--brand);visibility:hidden}
.pi[aria-selected=true] .ck,.pi[aria-checked=true] .ck{visibility:visible}
.pi[aria-selected=true]{color:var(--brand);font-weight:600}
.pop.st{width:300px;padding:12px}
.st .row{display:flex;align-items:center;gap:8px;margin:0 0 12px}
.st h3{margin:0 0 8px;font-size:12px;font-weight:600;color:var(--muted)}
.st .sb{display:inline-flex;align-items:center;justify-content:center;height:32px;padding:0 12px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);cursor:pointer;font-size:13px}
.st .sb:hover{border-color:var(--brand);color:var(--brand)}
.st .sl{display:grid;grid-template-columns:44px 1fr 34px;align-items:center;gap:8px;margin:0 0 10px;font-size:13px}
.st .sl output{text-align:right;color:var(--muted);font-variant-numeric:tabular-nums}
.st input[type=range]{width:100%;margin:0;accent-color:var(--brand)}
.st input[type=range]:disabled{opacity:.45}
.st .ck2{display:flex;align-items:center;gap:8px;font-size:13px;cursor:pointer}
.st .ck2 input{accent-color:var(--brand);width:16px;height:16px;margin:0}
.st .err{margin:0 0 8px;color:#dc2626;font-size:12px}
.fold{display:inline-flex;align-items:center;gap:6px;padding:0;border:0;background:transparent;color:inherit;font:inherit;letter-spacing:inherit;cursor:pointer}
.fold svg{width:14px;height:14px;transition:transform .15s}
.fold:hover{color:var(--text)}
.fold:focus-visible{outline:none;box-shadow:0 0 0 4px var(--soft);border-radius:6px}
.grp.fold-on .fold svg{transform:rotate(-90deg)}
.grp.fold-on .grid{display:none}
.searching .grp.fold-on .grid{display:grid}
.grid{grid-auto-flow:dense}
.hasbg .card{background:color-mix(in srgb,var(--card) 62%,transparent);-webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px)}
[data-theme=dark].hasbg .card{background:color-mix(in srgb,var(--card) 46%,transparent)}
.hasbg .card.link:hover{background:color-mix(in srgb,var(--hover) 78%,transparent)}
.card{min-height:72px}
.card.sm{grid-column:span 1;flex-direction:column;justify-content:center;padding:12px 6px}
.card.sm .ic{--s:calc(var(--is,40px) + 4px)}
.card.sm .tx{display:none}
.card.xl{grid-row:span 2;flex-direction:column;align-items:center;justify-content:center;gap:10px;padding:16px 14px;text-align:center}
.card.xl .ic{--s:calc(var(--is,40px) * 1.6)}
.card.xl .tx{align-items:center;max-width:100%}
.card.xl .nm{max-width:100%;font-size:15px}
.card.xl .ds{max-width:100%;white-space:normal;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical}
.tx{justify-content:center}
.acts{display:flex;align-items:center;gap:10px}
.top .sbox{margin-left:auto}
.ubtn{display:inline-flex;align-items:center;gap:6px;height:38px;padding:0 9px 0 4px;border:1px solid var(--line);border-radius:999px;background:var(--card);color:var(--muted);cursor:pointer;transition:border-color .15s}
.ubtn:hover,.ubtn[aria-expanded=true]{border-color:var(--brand)}
.ubtn:focus-visible{outline:none;box-shadow:0 0 0 4px var(--soft)}
.ubtn svg{width:15px;height:15px;color:var(--faint)}
.av{display:grid;place-items:center;width:28px;height:28px;border-radius:50%;background:linear-gradient(135deg,var(--brand-2),var(--brand));color:#fff;font-size:13px;font-weight:650}
.uh{padding:8px 10px 6px;color:var(--muted);font-size:12.5px;overflow-wrap:anywhere}
.pi svg.mi{width:16px;height:16px;flex:none;color:var(--muted)}
.pi.danger,.pi.danger svg.mi{color:#dc2626}
button.btn{font:inherit;cursor:pointer}
.foot a{color:inherit;text-decoration:none}
.foot a:hover{color:var(--brand)}
.mask{position:fixed;inset:0;z-index:60;display:grid;place-items:center;padding:16px;background:rgba(0,0,0,.45)}
.dlg{width:min(92vw,340px);padding:20px;border:1px solid var(--line);border-radius:var(--r);background:var(--card);color:var(--text);box-shadow:0 24px 60px -20px rgba(0,0,0,.5)}
.dlg p{margin:0 0 18px;line-height:1.6}
.dlg .bt{display:flex;justify-content:flex-end;gap:8px}
.dlg button{height:34px;padding:0 14px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);color:var(--text);font:inherit;cursor:pointer}
.dlg button:hover{border-color:var(--brand);color:var(--brand)}
.dlg button.ok{border-color:#dc2626;background:#dc2626;color:#fff}
.dlg button.ok:hover{filter:brightness(1.08);color:#fff}
.dlg button:focus-visible{outline:none;box-shadow:0 0 0 4px var(--soft)}
@media (max-width:640px){
.top{flex-wrap:wrap;gap:12px 10px}
.brand{order:1;flex:1;min-width:0}
.acts{order:2;gap:8px;flex:none}
.sbox{order:3;flex:1 1 100%;margin-left:0}
.net{order:4}
.eng{max-width:132px;padding:0 8px 0 10px}
.pop{position:fixed;top:var(--pop-top,64px);bottom:auto;left:auto;right:16px;width:min(260px,calc(100vw - 32px));min-width:0;max-width:none;max-height:calc(100vh - var(--pop-top,64px) - 16px)}
.pop.st{width:min(320px,calc(100vw - 32px))}
.pop.em{position:absolute;top:calc(100% + 8px);bottom:auto;left:0;right:auto;width:auto;min-width:160px;max-height:60vh}
.pop.um{position:absolute;top:calc(100% + 8px);bottom:auto;left:auto;right:0;width:auto;min-width:200px;max-height:none}
.acts .skin .sp{position:fixed;top:var(--pop-top,64px);bottom:auto;left:auto;right:16px;width:min(372px,calc(100vw - 32px));max-height:calc(100vh - var(--pop-top,64px) - 16px)}
.card.sm{padding:10px 4px}
.card.sm .ic{--s:calc(var(--is,34px) + 6px)}
.card.xl .ic{--s:calc(var(--is,34px) * 1.6)}
.card{min-height:64px}
}
@media (prefers-reduced-motion:reduce){.eng svg,.fold svg{transition:none}}
`

// panelLayoutStyleCSS 为链接样式与布局，以及多地址链接的选择列表（.chm 遮罩、.chp 列表，手机上为底部抽屉）；
// .mb 为多地址角标，.icw 为图标容器（角标相对图标定位）。
// html.app 为 App 图标样式，html.lc 为设置了宽屏每行数量（按 --cols 算出每格最小宽度，窗口变窄时每行自动减少）。
// 手机每行数量（html.lm）由服务端按设置输出。图标边长用 --s（各样式的默认值不同），--is 为面板或访客设置的大小。
const panelLayoutStyleCSS = `
@media (min-width:641px){
html.lc .grid{grid-template-columns:repeat(auto-fill,minmax(max(80px,calc((100% - (var(--cols) * 2 - 1) * var(--gap,12px)) / (var(--cols) * 2))),1fr))}
}
html.app .grid{gap:var(--gap,18px) var(--gap,10px);grid-template-columns:repeat(auto-fill,minmax(calc(var(--is,64px) + 24px),1fr))}
@media (min-width:641px){
html.app.lc .grid{grid-template-columns:repeat(auto-fill,minmax(max(calc(var(--is,64px) + 8px),calc((100% - (var(--cols) - 1) * var(--gap,10px)) / var(--cols))),1fr))}
}
html.app .grid .card,html.app .grid .card.link:hover{grid-column:span 1;flex-direction:column;align-items:center;justify-content:flex-start;gap:7px;min-width:0;min-height:0;padding:6px 2px 4px;border:0;background:transparent;box-shadow:none;-webkit-backdrop-filter:none;backdrop-filter:none;transform:none;text-align:center}
html.app .card .icw{width:min(var(--is,64px),100%)}
html.app .card .ic{--s:var(--is,64px);width:100%;height:auto;aspect-ratio:1;border-radius:calc(var(--s) * .24);font-size:calc(var(--s) * .4);box-shadow:0 8px 18px -12px rgba(0,0,0,.55);transition:transform .18s,box-shadow .18s}
html.app .card .ic.img{background:var(--card)}
html.app .card .mb{transition:transform .18s}
html.app .card.link:hover .ic,html.app .card.multi:hover .ic,html.app .card.link:hover .mb,html.app .card.multi:hover .mb{transform:translateY(-3px)}
html.app .card.link:hover .ic,html.app .card.multi:hover .ic{box-shadow:0 14px 26px -14px rgba(0,0,0,.6)}
html.app .card .hit:focus-visible{box-shadow:0 0 0 3px var(--soft),inset 0 0 0 2px var(--brand)}
html.app .tx{width:100%;align-items:center}
html.app .nm{max-width:100%;font-size:12.5px;font-weight:500;line-height:1.35}
html.app .ds{display:none}
html.hasbg.app .nm{text-shadow:0 1px 3px var(--bg),0 0 1px var(--bg)}
html.app .card .lock{top:1px;right:auto;left:max(1px,calc(50% - var(--is,64px) / 2 - 6px));width:16px;height:16px;padding:2px;border-radius:50%;background:var(--card);color:var(--muted);box-shadow:0 1px 4px rgba(0,0,0,.25)}
html.app .grid .card.sm .tx{display:flex}
html.app.an .grid .card:not(.wd) .tx{display:none}
html.app .grid .card.wd,html.app .grid .card.wd.link:hover{grid-column:span 2;flex-direction:row;align-self:start;align-items:center;gap:10px;height:var(--is,64px);padding:0 12px 0 0;border-radius:calc(var(--is,64px) * .24);background:color-mix(in srgb,var(--card) 72%,transparent);box-shadow:0 8px 18px -12px rgba(0,0,0,.55);-webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px);text-align:left}
html.app .grid .card.wd.link:hover,html.app .grid .card.wd.multi:hover{transform:translateY(-3px);background:color-mix(in srgb,var(--hover) 84%,transparent);box-shadow:0 14px 26px -14px rgba(0,0,0,.6)}
html.app .card.wd .icw{width:var(--is,64px);height:100%}
html.app .grid .card.wd .ic,html.app .grid .card.wd:hover .ic,html.app .grid .card.wd:hover .mb{box-shadow:none;transform:none}
html.app .card.wd .tx{flex:1;width:auto;min-width:0;align-items:flex-start}
html.app .card.wd .nm{font-size:13.5px;font-weight:600}
html.app .card.wd .ds{display:block;max-width:100%}
html.hasbg.app .card.wd .nm{text-shadow:none}
html.app .card.wd .lock{top:6px;right:6px;left:auto}
html.app .grid .card.xl{grid-column:span 2;grid-row:span 2;justify-content:center}
html.app .card.xl .icw{width:min(calc(var(--is,64px) * 2),100%)}
html.app .card.xl .ic{--s:calc(var(--is,64px) * 2)}
html.app .card.xl .nm{font-size:13.5px;font-weight:600}
html.app .card.xl .ds{display:none}
html.app .card.xl .lock{left:max(1px,calc(50% - var(--is,64px) - 6px))}
.card.multi .hit{cursor:pointer}
.st .seg{gap:6px}
.st .seg .sb{flex:1}
.st .seg .sb.on{border-color:var(--brand);background:var(--soft);color:var(--brand);font-weight:600}
.chm{position:fixed;inset:0;z-index:70}
.chm::before{content:"";position:absolute;inset:0;background:transparent;transition:background .22s}
.chp{position:absolute;left:0;top:0;outline:none;width:288px;max-width:calc(100vw - 16px);max-height:min(70vh,460px);overflow:auto;overscroll-behavior:contain;padding:6px;border:1px solid var(--line);border-radius:calc(var(--r) + 2px);background:var(--card);color:var(--text);box-shadow:0 24px 60px -20px rgba(0,0,0,.5),0 2px 10px -4px rgba(0,0,0,.18);opacity:0;transform:translateY(-6px) scale(.96);transform-origin:var(--ox,50%) var(--oy,0);transition:opacity .16s ease,transform .2s cubic-bezier(.2,.8,.2,1)}
.chm.up .chp{transform:translateY(6px) scale(.96)}
.chm.in .chp{opacity:1;transform:none}
.chh{display:flex;align-items:center;gap:10px;padding:8px 8px 10px;margin-bottom:4px;border-bottom:1px solid var(--line)}
.chh .ic{--s:34px;width:34px;height:34px;border-radius:9px;font-size:14px;box-shadow:none}
.chh .ic.tile img{width:58%;height:58%}
.chh div{display:flex;flex-direction:column;min-width:0}
.chh b{font-size:14px;font-weight:650;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.chh small{color:var(--muted);font-size:12px}
.ci{display:flex;align-items:center;gap:10px;min-height:46px;padding:7px 8px;border-radius:calc(var(--r-sm) + 1px);color:var(--text);text-decoration:none;transition:background .12s}
.chm.in .ci{animation:chi .26s cubic-bezier(.2,.8,.2,1) both;animation-delay:calc(var(--i,0) * 30ms)}
@keyframes chi{from{opacity:0;transform:translateY(4px)}}
a.ci:hover,a.ci:focus-visible{outline:none;background:var(--input)}
a.ci:focus-visible{box-shadow:inset 0 0 0 2px var(--brand)}
.ci .cg{display:grid;place-items:center;width:30px;height:30px;flex:none;border-radius:8px;background:var(--input);color:var(--muted)}
.ci.pri .cg{background:var(--soft);color:var(--brand)}
.ci .cg svg{width:16px;height:16px}
.ci .ct{display:flex;flex-direction:column;flex:1;min-width:0}
.ci .cn{font-size:13.5px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ci .ca{color:var(--faint);font:12px/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ci .cx{width:15px;height:15px;flex:none;color:var(--faint);transition:transform .15s,color .15s}
a.ci:hover .cx,a.ci:focus-visible .cx{color:var(--brand);transform:translate(2px,-2px)}
.ci.off{opacity:.5;cursor:not-allowed}
.ci.off .cn{text-decoration:line-through}
@media (max-width:640px){
html.app .grid{grid-template-columns:repeat(auto-fill,minmax(calc(var(--is,56px) + 14px),1fr))}
html.app .card .icw{width:min(var(--is,56px),100%)}
html.app .card .ic{--s:var(--is,56px)}
html.app .card .lock{left:max(1px,calc(50% - var(--is,56px) / 2 - 6px))}
html.app .nm{font-size:12px}
html.app .grid .card.wd,html.app .grid .card.wd.link:hover{height:var(--is,56px);border-radius:calc(var(--is,56px) * .24)}
html.app .card.wd .icw{width:var(--is,56px)}
html.app .card.wd .nm{font-size:13px}
html.app .card.xl .icw{width:min(calc(var(--is,56px) * 2),100%)}
html.app .card.xl .ic{--s:calc(var(--is,56px) * 2)}
html.app .card.xl .lock{left:max(1px,calc(50% - var(--is,56px) - 6px))}
.chm.in::before{background:rgba(0,0,0,.42)}
.chp,.chm.up .chp{left:0;right:0;top:auto;bottom:0;width:auto;max-width:none;max-height:78vh;padding:6px 10px calc(12px + env(safe-area-inset-bottom));border:0;border-top:1px solid var(--line);border-radius:20px 20px 0 0;opacity:1;transform:translateY(100%);transition:transform .28s cubic-bezier(.2,.8,.2,1)}
.chp::before{content:"";display:block;width:38px;height:4px;margin:4px auto 6px;border-radius:2px;background:var(--line)}
.ci{min-height:54px;padding:9px 8px}
.chh{padding:6px 8px 12px}
}
@media (prefers-reduced-motion:reduce){html.app .card .ic,html.app .card .mb,.chp,.chm::before,.ci .cx{transition:none}html.app .grid .card:hover .ic,html.app .grid .card:hover .mb{transform:none}.chm.in .ci{animation:none}}
`

// panelLayoutHeadJS 在页面头部按访客保存的布局（链接样式、图标大小）设置 <html> 的类名与 --is，避免先按面板设置显示再跳变。
// 图标大小只接受 24-128 的数字，与面板设置的范围相同。
const panelLayoutHeadJS = `(function(){try{var s=JSON.parse(localStorage.getItem("gately-panel-layout")||"null");if(!s||typeof s!=="object"){return}var r=document.documentElement;
if(s.style==="app"||s.style==="card"){r.classList.toggle("app",s.style==="app")}
if(typeof s.icon==="number"&&s.icon>=24&&s.icon<=128){r.style.setProperty("--is",Math.round(s.icon)+"px")}}catch(e){}})();`

// panelJS 为页面脚本。数据来自 id 为 ui-data 的 JSON：{t 文案, engines [{n,u}], def, langs [{c,n}], lang, bg {has,dim,blur}}。
// 搜索框：输入时过滤本页链接；选了搜索引擎时回车（或点右侧按钮）在新标签页用该引擎搜索，引擎地址里的 {q} 换成编码后的关键词。
const panelJS = `(function(){var q=document.getElementById("q");if(!q){return}
var D={};try{D=JSON.parse(document.getElementById("ui-data").textContent)}catch(e){}
var T=D.t||{},engines=D.engines||[],KE="gately-panel-engine",KB="gately-panel-bg",KF="gately-panel-fold",KH="gately-panel-hits",KQ="gately-panel-freq",KL="gately-panel-layout";
function get(k){try{return localStorage.getItem(k)}catch(e){return null}}
function put(k,v){try{if(v===null){localStorage.removeItem(k)}else{localStorage.setItem(k,v)}return true}catch(e){return false}}
function jget(k,d){try{var v=JSON.parse(get(k));return v&&typeof v==="object"?v:d}catch(e){return d}}
function h(tag,cls,text){var e=document.createElement(tag);if(cls){e.className=cls}if(text!==undefined){e.textContent=text}return e}
function svg(s){var d=h("span");d.innerHTML=s;return d.firstChild}
var CK='` + panelCheckIcon + `';
var main=document.querySelector("main"),root=document.documentElement;
function cards(){return [].slice.call(document.querySelectorAll("[data-s]"))}
var groups=[].slice.call(document.querySelectorAll(".grp")),none=document.getElementById("none");
var eng=document.getElementById("eng"),go=document.getElementById("go"),box=q.parentNode,cur=-1,ph=q.getAttribute("placeholder");
function apply(){var terms=q.value.trim().toLowerCase().split(/\s+/).filter(Boolean),shown=0;
cards().forEach(function(c){var s=c.getAttribute("data-s"),ok=terms.every(function(t){return s.indexOf(t)>=0});c.hidden=!ok;if(ok){shown++}});
groups.forEach(function(g){g.hidden=!g.querySelector("[data-s]:not([hidden])")});
document.body.classList.toggle("searching",terms.length>0);
var fq=document.getElementById("fq");if(fq){fq.hidden=terms.length>0}
if(none){none.hidden=shown>0||cur>=0}}
function openWeb(){if(cur<0){return false}var kw=q.value.trim();if(!kw){return true}var u=engines[cur].u.split("{q}").join(encodeURIComponent(kw));if(/^https?:\/\//i.test(u)){window.open(u,"_blank","noopener,noreferrer")}return true}
// ---- popups: one at a time, closed by outside click or Esc
var openPop=null;
function closePop(back){if(!openPop){return}var p=openPop;openPop=null;p.el.remove();p.btn.setAttribute("aria-expanded","false");if(back){p.btn.focus()}}
function popup(btn,wrap,cls,build){if(openPop&&openPop.btn===btn){closePop(true);return}closePop();var el=h("div","pop "+cls);build(el);wrap.appendChild(el);btn.setAttribute("aria-expanded","true");openPop={el:el,btn:btn};
var f=el.querySelector("[aria-selected=true]")||el.querySelector("button,input");if(f&&f.focus){f.focus()}}
document.addEventListener("mousedown",function(e){if(openPop&&!openPop.el.contains(e.target)&&!openPop.btn.contains(e.target)){closePop(false)}});
document.addEventListener("keydown",function(e){if(e.key==="Escape"&&openPop){e.preventDefault();closePop(true)}});
function listNav(el){el.addEventListener("keydown",function(e){var k=e.key;if(k!=="ArrowDown"&&k!=="ArrowUp"&&k!=="Home"&&k!=="End"){return}var its=[].slice.call(el.querySelectorAll(".pi")),i=its.indexOf(document.activeElement);e.preventDefault();
i=k==="Home"?0:k==="End"?its.length-1:k==="ArrowDown"?(i+1)%its.length:(i-1+its.length)%its.length;its[i].focus()})}
function item(label,selected,role,onPick){var b=h("button","pi");b.type="button";b.setAttribute("role",role);b.setAttribute(role==="option"?"aria-selected":"aria-checked",selected?"true":"false");b.appendChild(svg(CK)).setAttribute("class","ck");b.appendChild(h("span","",label));b.addEventListener("click",onPick);return b}
// ---- search engines
function setEngine(i,save){cur=i;var web=i>=0;document.getElementById("eng-name").textContent=web?engines[i].n:T.local;
q.setAttribute("placeholder",web?T.webSearch:ph);box.classList.toggle("web",web);if(go){go.hidden=!web}
if(save){put(KE,web?engines[i].n:"\u0001")}apply()}
if(eng){var saved=get(KE),idx=(D.def||0)-1;if(saved==="\u0001"){idx=-1}else if(saved){engines.forEach(function(e,i){if(e.n===saved){idx=i}})}
setEngine(idx,false);
eng.addEventListener("click",function(){popup(eng,eng.parentNode,"em",function(el){el.setAttribute("role","listbox");el.setAttribute("aria-label",eng.getAttribute("aria-label"));
el.appendChild(item(T.local,cur<0,"option",function(){setEngine(-1,true);closePop(true)}));
engines.forEach(function(e,i){el.appendChild(item(e.n,cur===i,"option",function(){setEngine(i,true);closePop(true);q.focus()}))});listNav(el)})});
if(go){go.addEventListener("click",function(){openWeb();q.focus()})}}
q.addEventListener("input",apply);
document.addEventListener("keydown",function(e){if(e.key==="/"&&document.activeElement!==q&&!e.metaKey&&!e.ctrlKey&&!e.altKey&&!/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)){e.preventDefault();q.focus()}});
q.addEventListener("keydown",function(e){if(e.key==="Escape"&&!openPop){q.value="";apply();q.blur()}else if(e.key==="Enter"){if(e.isComposing){return}
if(cur>=0){e.preventDefault();openWeb();return}
var f=cards().filter(function(c){return !c.hidden&&c.querySelector(".hit")})[0];if(f){e.preventDefault();f.querySelector(".hit").click()}}});
// ---- small screens: popups drop down from just under the header buttons (like the admin UI) and the page does not scroll behind them
var actsEl=document.getElementById("acts");
if(actsEl){actsEl.addEventListener("click",function(){root.style.setProperty("--pop-top",Math.round(actsEl.getBoundingClientRect().bottom+8)+"px")},true)}
new MutationObserver(function(){root.classList.toggle("nosc",!!document.querySelector(".pop:not(.em):not(.um),.sp")||(!!document.querySelector(".chm")&&phone.matches))}).observe(document.body,{childList:true,subtree:true});
// ---- group folding, remembered by group name
var folded=jget(KF,[]);if(!Array.isArray(folded)){folded=[]}
document.querySelectorAll(".fold").forEach(function(b){var g=b.closest(".grp"),n=b.getAttribute("data-g");
function set(on){g.classList.toggle("fold-on",on);b.setAttribute("aria-expanded",on?"false":"true")}
set(folded.indexOf(n)>=0);b.addEventListener("click",function(){var on=!g.classList.contains("fold-on");set(on);folded=folded.filter(function(x){return x!==n});if(on){folded.push(n)}put(KF,JSON.stringify(folded))})});
// ---- frequent links: counted by clicks, shown first once clicked twice
var hits=jget(KH,{});
function freq(){var old=document.getElementById("fq");if(old){old.remove()}if(get(KQ)==="0"){return}
var top=Object.keys(hits).filter(function(k){return hits[k]>=2}).sort(function(a,b){return hits[b]-hits[a]}).slice(0,6),list=[];
top.forEach(function(k){var c=document.querySelector('[data-k="'+k+'"]');if(c&&c.querySelector(".hit")){list.push(c)}});if(!list.length){return}
var s=h("section","grp");s.id="fq";var t=h("h2","",T.frequent);s.appendChild(t);var g=h("div","grid");
list.forEach(function(c){var n=c.cloneNode(true);n.removeAttribute("data-s");n.removeAttribute("data-k");n.setAttribute("data-fk",c.getAttribute("data-k"));g.appendChild(n)});s.appendChild(g);main.insertBefore(s,main.firstChild)}
document.addEventListener("click",function(e){var a=e.target.closest&&e.target.closest(".hit");if(!a){return}var c=a.closest("[data-k],[data-fk]");if(!c||(c.querySelector(".alts")&&e.button===0&&!e.metaKey&&!e.ctrlKey&&!e.shiftKey&&!e.altKey)){return}var k=c.getAttribute("data-k")||c.getAttribute("data-fk");hits[k]=(hits[k]||0)+1;put(KH,JSON.stringify(hits))});
freq();
// ---- language: written to a cookie, the server renders texts by it; also stored under the admin UI key
var lb=document.getElementById("lang-btn");
if(lb&&D.langs){lb.addEventListener("click",function(){popup(lb,lb.parentNode,"",function(el){el.setAttribute("role","listbox");el.setAttribute("aria-label",T.language);
D.langs.forEach(function(l){el.appendChild(item(l.n,l.c===D.lang,"option",function(){document.cookie="gately_panel_lang="+l.c+"; path=/; max-age=31536000; samesite=lax"+(location.protocol==="https:"?"; secure":"");put("gately.locale",l.c);location.reload()}))});
listNav(el)})})}
// ---- page settings: background image, blur, mask, frequent links
var bgEl=document.querySelector(".bg"),origImg=bgEl.querySelector("img"),origSrc=origImg?origImg.getAttribute("src"):null,B=D.bg||{dim:45,blur:0};
function bgState(){return jget(KB,{})}
function applyBg(){var s=bgState(),dim=typeof s.dim==="number"?s.dim:B.dim,blur=typeof s.blur==="number"?s.blur:B.blur,src=s.img||origSrc,img=bgEl.querySelector("img");
if(src){if(!img){img=h("img");img.alt="";img.decoding="async";img.referrerPolicy="no-referrer";bgEl.appendChild(img)}if(img.getAttribute("src")!==src){img.setAttribute("src",src)}bgEl.classList.add("has")}else{if(img){img.remove()}bgEl.classList.remove("has")}
root.classList.toggle("hasbg",bgEl.classList.contains("has"));root.style.setProperty("--bgdim",String(dim/100));root.style.setProperty("--bgblur",blur+"px")}
applyBg();
function shrink(f,done,fail){var r=new FileReader();r.onerror=fail;r.onload=function(){var im=new Image();im.onload=function(){var tries=[[1920,.8],[1600,.7],[1280,.6],[960,.5]];
for(var i=0;i<tries.length;i++){var sc=Math.min(1,tries[i][0]/Math.max(im.naturalWidth,im.naturalHeight,1)),c=document.createElement("canvas");c.width=Math.max(1,Math.round(im.naturalWidth*sc));c.height=Math.max(1,Math.round(im.naturalHeight*sc));
var x=c.getContext("2d");x.fillStyle="#fff";x.fillRect(0,0,c.width,c.height);x.drawImage(im,0,0,c.width,c.height);var d=c.toDataURL("image/jpeg",tries[i][1]);if(d.length<=1200000){done(d);return}}fail()};im.onerror=fail;im.src=String(r.result)};r.readAsDataURL(f)}
var sb=document.getElementById("set-btn"),setBtn=sb;
function buildSettings(el){el.setAttribute("role","dialog");el.setAttribute("aria-label",T.settings);
el.appendChild(h("h3","",T.background));var err=h("p","err");err.hidden=true;
var row=h("div","row"),pick=h("button","sb",T.pickImage);pick.type="button";var file=h("input");file.type="file";file.accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml";file.hidden=true;
var rs=h("button","sb",T.reset);rs.type="button";row.appendChild(pick);row.appendChild(rs);row.appendChild(file);el.appendChild(row);el.appendChild(err);
function slider(label,max,key,def){var s=bgState(),v=typeof s[key]==="number"?s[key]:def,r=h("label","sl"),n=h("span","",label),i=h("input");i.type="range";i.min="0";i.max=String(max);i.step=key==="dim"?"5":"1";i.value=String(v);var o=h("output","",String(v));
i.addEventListener("input",function(){o.textContent=i.value;var st=bgState();st[key]=parseInt(i.value,10);put(KB,JSON.stringify(st));applyBg()});r.appendChild(n);r.appendChild(i);r.appendChild(o);el.appendChild(r);return i}
var s1=slider(T.blur,20,"blur",B.blur),s2=slider(T.mask,90,"dim",B.dim);
function sync(){var on=bgEl.classList.contains("has");s1.disabled=!on;s2.disabled=!on}sync();
pick.addEventListener("click",function(){file.click()});
file.addEventListener("change",function(){var f=file.files&&file.files[0];file.value="";if(!f){return}err.hidden=true;
function bad(){err.textContent=T.imageFail;err.hidden=false}
function use(d){var st=bgState();st.img=d;if(!put(KB,JSON.stringify(st))){bad();return}if(typeof st.dim!=="number"&&!B.has){st.dim=45;put(KB,JSON.stringify(st));s2.value="45";s2.nextSibling.textContent="45"}applyBg();sync()}
if(f.type==="image/svg+xml"){var r=new FileReader();r.onload=function(){use(String(r.result))};r.onerror=bad;r.readAsDataURL(f)}else{shrink(f,use,bad)}});
rs.addEventListener("click",function(){put(KB,null);put(KL,null);applyBg();applyLayout();closePop(false);openSettings(setBtn)});
// layout: the visitor picks the link style and icon size; the panel settings apply until then
el.appendChild(h("h3","",T.layout));var seg=h("div","row seg"),sl2=h("label","sl"),si=h("input"),so=h("output");
function styleBtn(v,label){var b=h("button","sb",label);b.type="button";b.setAttribute("data-v",v);b.addEventListener("click",function(){var st=layoutState();st.style=v;put(KL,JSON.stringify(st));applyLayout();syncLayout()});seg.appendChild(b)}
styleBtn("card",T.styleCard);styleBtn("app",T.styleApp);el.appendChild(seg);
si.type="range";si.min="24";si.max="128";si.step="2";sl2.appendChild(h("span","",T.iconSize));sl2.appendChild(si);sl2.appendChild(so);el.appendChild(sl2);
si.addEventListener("input",function(){so.textContent=si.value;var st=layoutState();st.icon=parseInt(si.value,10);put(KL,JSON.stringify(st));applyLayout()});
function syncLayout(){var cur=curStyle();[].forEach.call(seg.children,function(b){var on=b.getAttribute("data-v")===cur;b.classList.toggle("on",on);b.setAttribute("aria-pressed",on?"true":"false")});si.value=String(curIcon());so.textContent=si.value}
syncLayout();
var fr=h("label","ck2"),cb=h("input");cb.type="checkbox";cb.checked=get(KQ)!=="0";fr.appendChild(cb);fr.appendChild(h("span","",T.showFreq));
cb.addEventListener("change",function(){put(KQ,cb.checked?null:"0");freq()});el.appendChild(h("h3","",""));el.appendChild(fr)}
// ---- layout chosen by the visitor (link style, icon size); the head script applies it before first paint
var L=D.layout||{style:"card",icon:0};
function layoutState(){return jget(KL,{})}
function curStyle(){var s=layoutState().style;return s==="app"||s==="card"?s:(L.style==="app"?"app":"card")}
function okIcon(n){return typeof n==="number"&&n>=24&&n<=128}
// curIcon: the icon size in effect - the visitor's, the panel's, or the style default (smaller on phones)
function curIcon(){var n=layoutState().icon;if(okIcon(n)){return Math.round(n)}if(okIcon(L.icon)){return L.icon}var m=window.matchMedia&&matchMedia("(max-width:640px)").matches;return curStyle()==="app"?(m?56:64):(m?34:40)}
function applyLayout(){root.classList.toggle("app",curStyle()==="app");var n=layoutState().icon;if(okIcon(n)){root.style.setProperty("--is",Math.round(n)+"px")}else{root.style.removeProperty("--is")}}
function openSettings(btn){setBtn=btn;popup(btn,btn.parentNode,"st",buildSettings)}
if(sb){sb.addEventListener("click",function(){openSettings(sb)})}
// ---- user menu (signed-in visitors): page settings and sign-out, sign-out asks for confirmation
var GEAR='` + panelGearIcon + `',OUT='` + panelLogoutIcon + `';
function menuItem(label,icon,cls,onPick){var b=h("button","pi"+(cls?" "+cls:""));b.type="button";b.setAttribute("role","menuitem");var i=svg(icon);i.setAttribute("class","mi");b.appendChild(i);b.appendChild(h("span","",label));b.addEventListener("click",onPick);return b}
function confirmLogout(){var url=D.logout;if(!url||url.charAt(0)!=="/"){return}
var prev=document.activeElement,m=h("div","mask"),d=h("div","dlg");d.setAttribute("role","alertdialog");d.setAttribute("aria-modal","true");d.setAttribute("aria-label",T.logout);
d.appendChild(h("p","",T.confirmLogout));var bt=h("div","bt"),no=h("button","",T.cancel),ok=h("button","ok",T.logout);no.type="button";ok.type="button";bt.appendChild(no);bt.appendChild(ok);d.appendChild(bt);m.appendChild(d);document.body.appendChild(m);no.focus();
function close(){m.remove();document.removeEventListener("keydown",key,true);if(prev&&prev.focus){prev.focus()}}
function key(e){if(e.key==="Escape"){e.preventDefault();e.stopPropagation();close()}else if(e.key==="Tab"){e.preventDefault();(document.activeElement===no?ok:no).focus()}}
document.addEventListener("keydown",key,true);
m.addEventListener("mousedown",function(e){if(e.target===m){close()}});no.addEventListener("click",close);ok.addEventListener("click",function(){location.href=url})}
var ub=document.getElementById("user-btn"),lo=document.getElementById("logout-btn");
if(ub){ub.addEventListener("click",function(){popup(ub,ub.parentNode,"um",function(el){el.setAttribute("role","menu");el.appendChild(h("div","uh",ub.getAttribute("title")));
el.appendChild(menuItem(T.settings,GEAR,"",function(){closePop(false);openSettings(ub)}));
el.appendChild(menuItem(T.logout,OUT,"danger",function(){closePop(false);confirmLogout()}));listNav(el)})})}
if(lo){lo.addEventListener("click",confirmLogout)}
// ---- links with several addresses: a click opens a chooser (a popover next to the link, a bottom sheet on phones);
// clicks with a modifier key or the middle button keep the browser default on the main address
var LINK='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>',
STAR='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></svg>',
EXT='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 17 17 7M8 7h9v9"/></svg>';
var ch=null,phone=window.matchMedia?matchMedia("(max-width:640px)"):{matches:false},still=window.matchMedia?matchMedia("(prefers-reduced-motion:reduce)"):{matches:false};
function hostOf(u){try{var x=new URL(u,location.href);return x.host||u}catch(e){return u}}
function count(k){if(k){hits[k]=(hits[k]||0)+1;put(KH,JSON.stringify(hits))}}
function closeCh(back){if(!ch){return}var c=ch;ch=null;document.removeEventListener("keydown",chKey,true);window.removeEventListener("resize",onMove);window.removeEventListener("scroll",onMove,true);
c.m.classList.remove("in");if(still.matches){c.m.remove()}else{setTimeout(function(){c.m.remove()},300)}if(back&&c.from&&c.from.focus){c.from.focus()}}
function onMove(e){if(ch&&!phone.matches&&!(e&&e.target&&e.target.nodeType===1&&ch.m.contains(e.target))){closeCh(false)}}
function chKey(e){if(!ch){return}var its=[].slice.call(ch.m.querySelectorAll("a.ci")),i=its.indexOf(document.activeElement),k=e.key;
if(k==="Escape"){e.preventDefault();e.stopPropagation();closeCh(true)}else if(k==="ArrowDown"||k==="ArrowUp"||k==="Home"||k==="End"||k==="Tab"){if(!its.length){return}e.preventDefault();
i=k==="Home"?0:k==="End"?its.length-1:(k==="ArrowDown"||(k==="Tab"&&!e.shiftKey))?(i+1)%its.length:(i-1+its.length)%its.length;its[i].focus()}}
function openCh(card,hit,byKey){closePop(false);closeCh(false);
var k=card.getAttribute("data-k")||card.getAttribute("data-fk"),items=[];
if(hit.tagName==="A"){items.push({n:T.primary,h:hit.getAttribute("href"),nt:hit.getAttribute("target")==="_blank",p:true})}else{items.push({n:T.primary,p:true})}
[].forEach.call(card.querySelectorAll(".alts .alt"),function(a){items.push(a.tagName==="A"?{n:a.textContent,h:a.getAttribute("href"),nt:true}:{n:a.textContent})});
var m=h("div","chm"),p=h("div","chp"),name=hit.getAttribute("aria-label")||"";p.setAttribute("role","dialog");p.setAttribute("aria-modal","true");p.setAttribute("aria-label",name);
var hd=h("div","chh"),ic=card.querySelector(".ic");if(ic){hd.appendChild(ic.cloneNode(true))}var tt=h("div");tt.appendChild(h("b","",name));tt.appendChild(h("small","",T.pickAddr));hd.appendChild(tt);p.appendChild(hd);
items.forEach(function(it,i){var r;if(it.h){r=h("a","ci");r.href=it.h;if(it.nt){r.target="_blank";r.rel="noopener noreferrer"}r.addEventListener("click",function(){count(k);closeCh(false)})}else{r=h("span","ci off");r.setAttribute("aria-disabled","true")}
if(it.p){r.classList.add("pri")}r.style.setProperty("--i",String(i));var g=h("span","cg");g.appendChild(svg(it.p?STAR:LINK));r.appendChild(g);
var t=h("span","ct");t.appendChild(h("span","cn",it.n));t.appendChild(h("span","ca",it.h?hostOf(it.h):T.off));r.appendChild(t);if(it.h){var x=svg(EXT);x.setAttribute("class","cx");r.appendChild(x)}p.appendChild(r)});
m.appendChild(p);document.body.appendChild(m);
if(!phone.matches){var rc=card.getBoundingClientRect(),pw=p.offsetWidth,ph=p.offsetHeight,vw=root.clientWidth,vh=window.innerHeight,
left=Math.min(Math.max(8,rc.left+rc.width/2-pw/2),vw-pw-8),top=rc.bottom+6;if(top+ph>vh-8&&rc.top-ph-6>=8){top=rc.top-ph-6;m.classList.add("up");p.style.setProperty("--oy","100%")}else if(top+ph>vh-8){top=Math.max(8,vh-ph-8)}
p.style.left=Math.round(left)+"px";p.style.top=Math.round(top)+"px";p.style.setProperty("--ox",Math.round(rc.left+rc.width/2-left)+"px")}
m.addEventListener("mousedown",function(e){if(e.target===m){closeCh(false)}});m.addEventListener("click",function(e){if(e.target===m){closeCh(false)}});
document.addEventListener("keydown",chKey,true);window.addEventListener("resize",onMove);window.addEventListener("scroll",onMove,true);
ch={m:m,from:hit};void p.offsetWidth;m.classList.add("in");p.tabIndex=-1;var f=byKey&&p.querySelector("a.ci");(f||p).focus({preventScroll:true})}
document.addEventListener("click",function(e){var hit=e.target.closest&&e.target.closest(".hit");if(!hit){return}var card=hit.closest(".card");
if(!card||!card.querySelector(".alts")||e.button!==0||e.metaKey||e.ctrlKey||e.shiftKey||e.altKey){return}e.preventDefault();openCh(card,hit,e.detail===0)});
apply()})();`
