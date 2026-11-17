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
	panelMoreIcon    = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/></svg>`
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
.card{min-height:64px}
}
@media (prefers-reduced-motion:reduce){.eng svg,.fold svg{transition:none}}
`

// panelLayoutStyleCSS 为链接样式与布局：.am 为其他访问方式菜单（只在 App 图标样式下显示，卡片样式下访问方式直接显示在卡片上）；
// html.app 为 App 图标样式，html.lc 为设置了宽屏每行数量（按 --cols 算出每格最小宽度，窗口变窄时每行自动减少）。
// 手机每行数量（html.lm）由服务端按设置输出。图标边长用 --s（各样式的默认值不同），--is 为面板或访客设置的大小。
const panelLayoutStyleCSS = `
.am{display:none}
@media (min-width:641px){
html.lc .grid{grid-template-columns:repeat(auto-fill,minmax(max(80px,calc((100% - (var(--cols) * 2 - 1) * var(--gap,12px)) / (var(--cols) * 2))),1fr))}
}
html.app .grid{gap:var(--gap,18px) var(--gap,10px);grid-template-columns:repeat(auto-fill,minmax(calc(var(--is,64px) + 24px),1fr))}
@media (min-width:641px){
html.app.lc .grid{grid-template-columns:repeat(auto-fill,minmax(max(calc(var(--is,64px) + 8px),calc((100% - (var(--cols) - 1) * var(--gap,10px)) / var(--cols))),1fr))}
}
html.app .card{grid-column:span 1;flex-direction:column;align-items:center;justify-content:flex-start;gap:7px;min-width:0;min-height:0;padding:6px 2px 4px;border:0;background:transparent;box-shadow:none;-webkit-backdrop-filter:none;backdrop-filter:none;text-align:center}
html.app .card.link:hover{background:transparent;transform:none}
html.app .card .ic{--s:var(--is,64px);width:min(var(--s),100%);height:auto;aspect-ratio:1;border-radius:calc(var(--s) * .24);font-size:calc(var(--s) * .4);box-shadow:0 8px 18px -12px rgba(0,0,0,.55);transition:transform .18s,box-shadow .18s}
html.app .card .ic.img{background:var(--card)}
html.app .card.link:hover .ic{transform:translateY(-3px);box-shadow:0 14px 26px -14px rgba(0,0,0,.6)}
html.app .card .hit:focus-visible{box-shadow:0 0 0 3px var(--soft),inset 0 0 0 2px var(--brand)}
html.app .tx{width:100%;align-items:center}
html.app .nm{max-width:100%;font-size:12.5px;font-weight:500;line-height:1.35}
html.app .ds,html.app .card .alts{display:none}
html.hasbg.app .nm{text-shadow:0 1px 3px var(--bg),0 0 1px var(--bg)}
html.app .card .lock{top:1px;right:max(1px,calc(50% - var(--is,64px) / 2 - 6px));width:16px;height:16px;padding:2px;border-radius:50%;background:var(--card);color:var(--muted);box-shadow:0 1px 4px rgba(0,0,0,.25)}
html.app .card .am{display:block;position:absolute;z-index:3;top:calc(6px + min(var(--is,64px),100%) - 16px);right:max(0px,calc(50% - var(--is,64px) / 2 - 7px))}
html.app .card:has(.am[open]){z-index:6}
.am summary{display:grid;place-items:center;width:22px;height:22px;border:1px solid var(--line);border-radius:50%;background:var(--card);color:var(--muted);box-shadow:0 2px 6px -2px rgba(0,0,0,.35);cursor:pointer;list-style:none}
.am summary::-webkit-details-marker{display:none}
.am summary svg{width:13px;height:13px}
.am summary:hover,.am[open] summary{border-color:var(--brand);color:var(--brand)}
.am summary:focus-visible{outline:none;box-shadow:0 0 0 3px var(--soft)}
.amp{position:absolute;top:calc(100% + 6px);left:50%;transform:translateX(calc(-50% + var(--dx,0px)));display:flex;flex-direction:column;gap:1px;min-width:150px;max-width:min(240px,calc(100vw - 16px));padding:6px;border:1px solid var(--line);border-radius:var(--r);background:var(--card);box-shadow:0 18px 50px -18px rgba(0,0,0,.45);text-align:left}
.amp .t{padding:2px 10px 6px;color:var(--muted);font-size:11.5px;white-space:nowrap}
.amp a,.amp .off{display:block;padding:6px 10px;border-radius:calc(var(--r-sm) - 2px);color:var(--text);font-size:13px;text-decoration:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.amp a:hover,.amp a:focus-visible{outline:none;background:var(--input);color:var(--brand)}
.amp .off{opacity:.5;text-decoration:line-through;cursor:not-allowed}
.st .seg{gap:6px}
.st .seg .sb{flex:1}
.st .seg .sb.on{border-color:var(--brand);background:var(--soft);color:var(--brand);font-weight:600}
@media (max-width:640px){
html.app .grid{grid-template-columns:repeat(auto-fill,minmax(calc(var(--is,56px) + 14px),1fr))}
html.app .card .ic{--s:var(--is,56px)}
html.app .card .lock{right:max(1px,calc(50% - var(--is,56px) / 2 - 6px))}
html.app .card .am{top:calc(6px + min(var(--is,56px),100%) - 16px);right:max(0px,calc(50% - var(--is,56px) / 2 - 7px))}
html.app .nm{font-size:12px}
}
@media (prefers-reduced-motion:reduce){html.app .card .ic{transition:none}html.app .card.link:hover .ic{transform:none}}
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
new MutationObserver(function(){root.classList.toggle("nosc",!!document.querySelector(".pop:not(.em):not(.um),.sp"))}).observe(document.body,{childList:true,subtree:true});
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
document.addEventListener("click",function(e){var a=e.target.closest&&e.target.closest(".hit");if(!a){return}var c=a.closest("[data-k],[data-fk]");if(!c){return}var k=c.getAttribute("data-k")||c.getAttribute("data-fk");hits[k]=(hits[k]||0)+1;put(KH,JSON.stringify(hits))});
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
// ---- other access methods menu (App icon style): one open at a time, kept inside the window, closed by outside click or Esc
function closeAm(except){document.querySelectorAll("details.am[open]").forEach(function(d){if(d!==except){d.open=false}})}
document.addEventListener("toggle",function(e){var d=e.target;if(!d||!d.classList||!d.classList.contains("am")||!d.open){return}closeAm(d);
var p=d.querySelector(".amp");p.style.removeProperty("--dx");var r=p.getBoundingClientRect(),vw=root.clientWidth,dx=0;if(r.left<8){dx=8-r.left}else if(r.right>vw-8){dx=vw-8-r.right}if(dx){p.style.setProperty("--dx",Math.round(dx)+"px")}},true);
document.addEventListener("mousedown",function(e){if(!(e.target.closest&&e.target.closest("details.am"))){closeAm(null)}});
document.addEventListener("keydown",function(e){if(e.key!=="Escape"){return}var d=document.querySelector("details.am[open]");if(d){e.preventDefault();d.open=false;d.querySelector("summary").focus()}});
apply()})();`
