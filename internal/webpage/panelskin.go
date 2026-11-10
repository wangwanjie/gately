package webpage

// 导航面板的换肤面板：与管理界面右上角的“主题配色”相同的皮肤与交互——亮色 / 暗色 / 跟随系统三种模式，
// 原创与经典两组色系（悬停预览、点击时从点击处圆形展开新配色），自定义主色与圆角，底部显示色系说明。
// 选择保存在浏览器的 localStorage 中，键名与管理界面相同：面板与管理界面在同一域名下时两边的选择互通；
// 访客没有选择过时使用面板设置的皮肤与模式。页面不使用 innerHTML 插入文案，色系数据来自程序内置的 panelthemes.json。

// panelSkinCSS 为换肤按钮与面板的样式。
const panelSkinCSS = `
.skin{position:relative}
.skin-btn{display:inline-grid;place-items:center;width:38px;height:38px;padding:0;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);color:var(--muted);cursor:pointer;transition:color .15s,border-color .15s}
.skin-btn:hover,.skin-btn[aria-expanded=true]{color:var(--brand);border-color:var(--brand)}
.skin-btn:focus-visible{outline:none;box-shadow:0 0 0 4px var(--soft)}
.skin-btn svg{width:18px;height:18px}
.sp{position:absolute;top:calc(100% + 8px);right:0;z-index:30;width:392px;max-height:min(76vh,620px);overflow:auto;overscroll-behavior:contain;padding:14px;border:1px solid var(--line);border-radius:var(--r);background:var(--card);color:var(--text);box-shadow:0 18px 50px -18px rgba(0,0,0,.45);text-align:left}
.sp button{font:inherit;color:inherit}
.sp-head{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:12px}
.sp-title{font-weight:650}
.sp-modes{display:inline-flex;gap:2px;padding:3px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--input)}
.sp-mode{display:inline-flex;align-items:center;gap:5px;height:26px;padding:0 9px;border:0;border-radius:max(2px,calc(var(--r-sm) - 3px));background:transparent;color:var(--muted)!important;font-size:12px!important;cursor:pointer;white-space:nowrap}
.sp-mode svg{width:13px;height:13px}
.sp-mode:hover{color:var(--text)!important}
.sp-mode.on{background:var(--card);color:var(--brand)!important;font-weight:600;box-shadow:0 1px 2px rgba(0,0,0,.08)}
.sp-gt{margin:2px 0 8px;color:var(--muted);font-size:12px;font-weight:600}
.sp-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:8px;margin-bottom:12px}
.sw{display:flex;flex-direction:column;gap:5px;min-width:0;padding:5px;border:1px solid var(--line);border-radius:var(--r-sm);background:var(--card);cursor:pointer;transition:border-color .15s,transform .15s}
.sw:hover{border-color:var(--brand);transform:translateY(-1px)}
.sw.on{border-color:var(--brand);box-shadow:0 0 0 3px var(--soft)}
.sw:focus-visible{outline:none;border-color:var(--brand)}
.pv{position:relative;display:flex;height:46px;overflow:hidden;border-radius:max(2px,calc(var(--r-sm) - 3px));background:var(--sw-page);border:1px solid var(--sw-line)}
.pv-side{width:24%;background:var(--sw-side);border-right:1px solid var(--sw-line)}
.pv-main{flex:1;display:flex;flex-direction:column;gap:4px;padding:5px}
.pv-bar{height:5px;width:60%;border-radius:3px;background:var(--sw-sub);opacity:.45}
.pv-card{flex:1;display:flex;align-items:flex-end;gap:3px;padding:4px;border-radius:3px;background:var(--sw-card);border:1px solid var(--sw-line)}
.pv-card i{display:block;width:9px;height:9px;border-radius:2px}
.pv-card .a{background:var(--sw-pri)}
.pv-card .b{background:var(--sw-2)}
.pv-only,.pv-check{position:absolute;top:3px;display:grid;place-items:center;width:15px;height:15px;border-radius:50%}
.pv-only{left:3px;background:rgba(0,0,0,.45);color:#fff}
.pv-check{right:3px;background:var(--sw-pri);color:#fff}
.pv-only svg,.pv-check svg{width:9px;height:9px}
.sw-name{overflow:hidden;font-size:11.5px;line-height:1.3;text-align:center;white-space:nowrap;text-overflow:ellipsis}
.sp-custom{padding-top:10px;border-top:1px solid var(--line)}
.sp-row{display:flex;align-items:center;gap:10px;margin-top:8px}
.sp-label{flex:none;width:56px;color:var(--muted);font-size:12px}
.sp-dots{display:flex;flex-wrap:wrap;align-items:center;gap:6px}
.sp-dot{position:relative;width:20px;height:20px;padding:0;border:2px solid transparent;border-radius:50%;background:var(--dot);cursor:pointer;box-shadow:inset 0 0 0 1px rgba(0,0,0,.08)}
.sp-dot.on{border-color:var(--card);box-shadow:0 0 0 2px var(--dot)}
.sp-pick{overflow:hidden;background:conic-gradient(#f43f5e,#f59e0b,#84cc16,#10b981,#06b6d4,#6366f1,#d946ef,#f43f5e)}
.sp-pick.on{box-shadow:0 0 0 2px var(--text)}
.sp-pick input{position:absolute;inset:-6px;width:calc(100% + 12px);height:calc(100% + 12px);padding:0;border:0;opacity:0;cursor:pointer}
.sp-reset{display:inline-grid;place-items:center;width:24px;height:24px;padding:0;border:1px solid var(--line);border-radius:50%;background:var(--card);color:var(--muted);cursor:pointer}
.sp-reset:hover{color:var(--brand);border-color:var(--brand)}
.sp-reset svg{width:13px;height:13px}
.sp-ri{display:inline-block;width:11px;height:11px;border:1.5px solid currentColor;border-right:0;border-bottom:0}
.sp-foot{display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 8px;margin-top:12px;padding-top:10px;border-top:1px solid var(--line);font-size:12px}
.sp-fname{font-weight:600}
.sp-fdesc{color:var(--muted)}
.sp-fonly{padding:0 6px;border-radius:999px;background:var(--input);color:var(--faint);font-size:11px}
.theme-vt::view-transition-old(root),.theme-vt::view-transition-new(root){animation:none;mix-blend-mode:normal}
@media (max-width:640px){
.sp{position:fixed;top:auto;left:8px;right:8px;bottom:8px;width:auto;max-height:78vh}
.sp-head{flex-wrap:wrap}
.sp-grid{grid-template-columns:repeat(3,1fr)}
}
`

// 换肤用到的图标（Lucide，ISC 许可）。
const (
	skinIconPalette = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z"/><circle cx="13.5" cy="6.5" r=".5" fill="currentColor"/><circle cx="17.5" cy="10.5" r=".5" fill="currentColor"/><circle cx="6.5" cy="12.5" r=".5" fill="currentColor"/><circle cx="8.5" cy="7.5" r=".5" fill="currentColor"/></svg>`
)

// panelSkinJS 在页面头部运行：先按访客保存的选择（没有时用面板设置）应用皮肤，避免先显示默认配色再闪变；
// 页面载入后再接管换肤按钮。vars 与服务端的 skinVars 是同一套推导规则。reveal 用视图过渡从点击处圆形展开新配色
// （浏览器不支持或偏好减少动效时直接切换）；h 创建元素，文案一律经 textContent 写入，只有固定的图标用 innerHTML；
// 悬停预览要鼠标停留片刻才生效，快速划过不闪烁。选择立即写入状态，只把重绘放进视图过渡：
// 过渡回调是异步的，若在回调里改状态，过渡结束前的其他选择（如主色）会被覆盖。脚本中不写中文注释：i18nsync 会把含中文的字符串当作待翻译文案。
const panelSkinJS = `(function(){
var data=document.getElementById("skin-data");if(!data){return}
var D;try{D=JSON.parse(data.textContent)}catch(e){return}
var root=document.documentElement,KF="gately-theme-family",KM="gately-theme-mode",KA="gately-theme-accent",KR="gately-theme-radius";
function get(k){try{return localStorage.getItem(k)}catch(e){return null}}
function put(k,v){try{if(v===null){localStorage.removeItem(k)}else{localStorage.setItem(k,v)}}catch(e){}}
var byId={};D.families.forEach(function(f){byId[f.id]=f});
var HEX=/^#[0-9a-fA-F]{6}$/,sf=get(KF),sm=get(KM),sa=get(KA),sr=get(KR);
var st={family:byId[sf]?sf:D.skin,mode:/^(light|dark|auto)$/.test(sm||"")?sm:D.mode,accent:HEX.test(sa||"")?sa.toLowerCase():null,radius:/^(sharp|round)$/.test(sr||"")?sr:"",preview:null};
var mql=window.matchMedia?window.matchMedia("(prefers-color-scheme: dark)"):{matches:false};
function hex(s){return [1,3,5].map(function(i){return parseInt(s.substr(i,2),16)})}
function mix(a,b,t){var x=hex(a),y=hex(b);return "#"+x.map(function(v,i){var n=Math.round(v*(1-t)+y[i]*t).toString(16);return n.length<2?"0"+n:n}).join("")}
function rgba(h,a){return "rgba("+hex(h).join(",")+","+a+")"}
function resolve(f,wantDark){return (wantDark&&f.dark)||!f.light?[f.dark,true]:[f.light,false]}
function wantDark(){return st.mode==="auto"?mql.matches:st.mode==="dark"}
function radiusOf(f){return st.radius==="sharp"?3:st.radius==="round"?16:f.radius}
function vars(p,dark,radius,accent){var pri=accent||p.primary;return {"--brand":pri,"--brand-2":accent?mix(accent,"#ffffff",.25):p.logo[0],"--soft":rgba(pri,dark?.14:.1),"--bg":p.page,"--card":p.card,"--hover":mix(p.card,pri,dark?.05:.03),"--text":p.text,"--muted":p.textSub,"--faint":mix(p.textSub,p.card,.35),"--line":p.border,"--input":mix(p.card,p.text,dark?.035:.025),
"--shadow":dark?"0 1px 2px rgba(0,0,0,.3),0 10px 30px -16px rgba(0,0,0,.7)":"0 1px 2px rgba(16,24,40,.04),0 8px 24px -14px rgba(16,24,40,.22)","--r":(radius+4)+"px","--r-sm":(Math.round(radius*.8)+2)+"px"}}
var listeners=[];
function apply(){var f=byId[st.preview||st.family]||byId[D.skin],r=resolve(f,wantDark()),v=vars(r[0],r[1],radiusOf(f),st.preview?null:st.accent);
for(var k in v){root.style.setProperty(k,v[k])}root.style.colorScheme=r[1]?"dark":"light";root.setAttribute("data-theme",r[1]?"dark":"light");
listeners.forEach(function(fn){fn()})}
apply();
if(mql.addEventListener){mql.addEventListener("change",function(){if(st.mode==="auto"){apply()}})}
function reveal(update,x,y){var reduce=window.matchMedia&&window.matchMedia("(prefers-reduced-motion: reduce)").matches;
if(!document.startViewTransition||reduce||x===undefined){update();return}
root.classList.add("theme-vt");var vt=document.startViewTransition(update),rad=Math.hypot(Math.max(x,innerWidth-x),Math.max(y,innerHeight-y));
vt.ready.then(function(){root.animate({clipPath:["circle(0px at "+x+"px "+y+"px)","circle("+rad+"px at "+x+"px "+y+"px)"]},{duration:450,easing:"ease-in-out",pseudoElement:"::view-transition-new(root)"})}).catch(function(){});
vt.finished.finally(function(){root.classList.remove("theme-vt")})}
function setFamily(id,x,y){var shown=st.preview===id&&!st.accent;put(KF,id);put(KA,null);st.preview=null;st.family=id;st.accent=null;if(shown){apply()}else{reveal(apply,x,y)}}
function setMode(m,x,y){if(m===st.mode){return}put(KM,m);var before=wantDark();st.mode=m;if(before===wantDark()){apply()}else{reveal(apply,x,y)}}
function setAccent(c){put(KA,c);st.accent=c;apply()}
function setRadius(r){put(KR,r||null);st.radius=r;apply()}

var SVG={sun:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2m-7.07-14.93 1.41 1.41m11.32 11.32 1.41 1.41M2 12h2m16 0h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/></svg>',
moon:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>',
auto:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8V6a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h8M10 19v-3.96 3.15M7 19h5"/><rect width="6" height="10" x="16" y="12" rx="2"/></svg>',
check:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>',
reset:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/></svg>'};
function h(tag,cls,text){var e=document.createElement(tag);if(cls){e.className=cls}if(text!==undefined){e.textContent=text}return e}
function icon(name){var s=h("span");s.innerHTML=SVG[name];return s.firstChild}

function init(){var btn=document.getElementById("skin-btn");if(!btn){return}
var T=D.texts.ui,names=D.texts.names,descs=D.texts.descs,pop=null,hover=null,swatches=[],modeBtns=[],dots=[],pick=null,pickInput=null,reset=null,radBtns=[],fname,fdesc,fonly;
function only(f){return f.light&&f.dark?null:(f.dark?"dark":"light")}
function refresh(){if(!pop){return}var wd=wantDark();
swatches.forEach(function(s){var f=s.f,r=resolve(f,wd),p=r[0],on=st.family===f.id;s.el.classList.toggle("on",on);s.el.setAttribute("aria-pressed",on?"true":"false");s.check.hidden=!on;
var sv={"--sw-page":p.page,"--sw-side":p.sidebar,"--sw-card":p.card,"--sw-line":p.border,"--sw-sub":p.textSub,"--sw-pri":p.primary,"--sw-2":p.logo[0]};for(var k in sv){s.el.style.setProperty(k,sv[k])}});
modeBtns.forEach(function(b){var on=b.m===st.mode;b.el.classList.toggle("on",on);b.el.setAttribute("aria-checked",on?"true":"false")});
var cur=(byId[st.family]&&resolve(byId[st.family],wd)[0].primary)||"#000000";
dots.forEach(function(d){var on=st.accent===d.c;d.el.classList.toggle("on",on);d.el.setAttribute("aria-pressed",on?"true":"false")});
pick.classList.toggle("on",!!st.accent&&D.accents.indexOf(st.accent)<0);pickInput.value=st.accent||cur;reset.hidden=!st.accent;
radBtns.forEach(function(b){var on=b.r===st.radius;b.el.classList.toggle("on",on);b.el.setAttribute("aria-checked",on?"true":"false")});
var info=byId[hover||st.family]||byId[D.skin],o=only(info);fname.textContent=names[info.id]||info.id;fdesc.textContent=descs[info.id]||"";fonly.hidden=!o;fonly.textContent=o==="dark"?T.darkOnly:T.lightOnly}
listeners.push(refresh);
function build(){pop=h("div","sp");pop.id="skin-pop";pop.setAttribute("role","dialog");pop.setAttribute("aria-label",T.title);
var head=h("div","sp-head");head.appendChild(h("span","sp-title",T.title));var modes=h("div","sp-modes");modes.setAttribute("role","radiogroup");modes.setAttribute("aria-label",T.mode);
[["light","sun",T.modeLight],["dark","moon",T.modeDark],["auto","auto",T.modeAuto]].forEach(function(m){var b=h("button","sp-mode");b.type="button";b.setAttribute("role","radio");b.title=m[2];b.appendChild(icon(m[1]));b.appendChild(h("span","",m[2]));
b.addEventListener("click",function(e){setMode(m[0],e.clientX,e.clientY)});modes.appendChild(b);modeBtns.push({m:m[0],el:b})});
head.appendChild(modes);pop.appendChild(head);
var groups=h("div","sp-groups");groups.addEventListener("mouseleave",function(){endHover()});
[["original",T.groupOriginal],["classic",T.groupClassic]].forEach(function(g){var fs=D.families.filter(function(f){return f.group===g[0]});if(!fs.length){return}
var sec=h("section");sec.appendChild(h("div","sp-gt",g[1]));var grid=h("div","sp-grid");
fs.forEach(function(f){var b=h("button","sw");b.type="button";b.setAttribute("aria-label",names[f.id]||f.id);var pv=h("span","pv");pv.appendChild(h("span","pv-side"));var main=h("span","pv-main");main.appendChild(h("span","pv-bar"));
var card=h("span","pv-card");card.appendChild(h("i","a"));card.appendChild(h("i","b"));main.appendChild(card);pv.appendChild(main);var o=only(f);if(o){var ob=h("span","pv-only");ob.appendChild(icon(o==="dark"?"moon":"sun"));pv.appendChild(ob)}
var ck=h("span","pv-check");ck.appendChild(icon("check"));pv.appendChild(ck);b.appendChild(pv);b.appendChild(h("span","sw-name",names[f.id]||f.id));
b.addEventListener("pointerenter",function(e){if(e.pointerType==="mouse"){startHover(f.id)}});b.addEventListener("focus",function(){hover=f.id;refresh()});
b.addEventListener("click",function(e){hover=null;setFamily(f.id,e.clientX,e.clientY)});grid.appendChild(b);swatches.push({f:f,el:b,check:ck})});
sec.appendChild(grid);groups.appendChild(sec)});pop.appendChild(groups);
var cu=h("section","sp-custom");cu.appendChild(h("div","sp-gt",T.custom));var row=h("div","sp-row");row.appendChild(h("span","sp-label",T.accent));var ds=h("div","sp-dots");
D.accents.forEach(function(c){var d=h("button","sp-dot");d.type="button";d.style.setProperty("--dot",c);d.setAttribute("aria-label",c);d.addEventListener("click",function(){setAccent(c)});ds.appendChild(d);dots.push({c:c,el:d})});
pick=h("label","sp-dot sp-pick");pick.title=T.accentCustom;pickInput=h("input");pickInput.type="color";pickInput.setAttribute("aria-label",T.accentCustom);
pickInput.addEventListener("input",function(){if(HEX.test(pickInput.value)){setAccent(pickInput.value.toLowerCase())}});pick.appendChild(pickInput);ds.appendChild(pick);
reset=h("button","sp-reset");reset.type="button";reset.title=T.accentReset;reset.setAttribute("aria-label",T.accentReset);reset.appendChild(icon("reset"));reset.addEventListener("click",function(){setAccent(null)});ds.appendChild(reset);
row.appendChild(ds);cu.appendChild(row);
var row2=h("div","sp-row");row2.appendChild(h("span","sp-label",T.radius));var rg=h("div","sp-modes");rg.setAttribute("role","radiogroup");rg.setAttribute("aria-label",T.radius);
[["sharp",T.radiusSharp,"0"],["",T.radiusDefault,"3px"],["round",T.radiusRound,"7px"]].forEach(function(r){var b=h("button","sp-mode");b.type="button";b.setAttribute("role","radio");var ri=h("span","sp-ri");ri.style.borderTopLeftRadius=r[2];b.appendChild(ri);b.appendChild(h("span","",r[1]));
b.addEventListener("click",function(){setRadius(r[0])});rg.appendChild(b);radBtns.push({r:r[0],el:b})});row2.appendChild(rg);cu.appendChild(row2);pop.appendChild(cu);
var foot=h("div","sp-foot");fname=h("span","sp-fname");fdesc=h("span","sp-fdesc");fonly=h("span","sp-fonly");foot.appendChild(fname);foot.appendChild(fdesc);foot.appendChild(fonly);pop.appendChild(foot);
btn.parentNode.appendChild(pop);refresh()}
var hoverTimer=0;function startHover(id){clearTimeout(hoverTimer);hover=id;refresh();hoverTimer=setTimeout(function(){if(hover===id&&st.preview!==id){st.preview=id;apply()}},120)}
function endHover(){clearTimeout(hoverTimer);hover=null;if(st.preview!==null){st.preview=null;apply()}else{refresh()}}
function lock(on){if(!window.matchMedia("(max-width:640px)").matches){return}document.body.style.overflow=on?"hidden":""}
function open(){if(!pop){build()}pop.hidden=false;btn.setAttribute("aria-expanded","true");lock(true);refresh();var a=pop.querySelector(".sw.on");if(a&&window.matchMedia("(hover:hover)").matches){a.focus({preventScroll:true})}}
function close(focus){if(!pop||pop.hidden){return}endHover();pop.hidden=true;btn.setAttribute("aria-expanded","false");lock(false);if(focus){btn.focus()}}
btn.addEventListener("click",function(){if(pop&&!pop.hidden){close(false)}else{open()}});
document.addEventListener("pointerdown",function(e){if(pop&&!pop.hidden&&!pop.contains(e.target)&&!btn.contains(e.target)){close(false)}});
document.addEventListener("keydown",function(e){if(e.key==="Escape"&&pop&&!pop.hidden){e.stopPropagation();close(true)}},true)}
if(document.readyState==="loading"){document.addEventListener("DOMContentLoaded",init)}else{init()}
})();`
