// 本文件的 CSS 最初由登录页样式草稿生成，之后直接在这里维护。

package webpage

// 网页认证登录页的内置样式。每种样式只在默认样式之后追加 CSS（类名与登录页、修改密码页、Passkey 页的结构一致），
// 部分样式另有背景装饰层（见 loginlook.go 的 lookDecor）。新增样式需同时登记到 loginSkinList。

// loginSkinLockscreen 为「锁屏：大时钟、渐变壁纸与毛玻璃胶囊输入框，像解锁一台设备（仅深色）」。
const loginSkinLockscreen = `:root{color-scheme:dark}
body{color:#fff;background:#1a1c3a}
.bg{background:
 radial-gradient(42rem 32rem at 8% 12%,#3a6df0 0,transparent 60%),
 radial-gradient(40rem 34rem at 92% 18%,#b14bd8 0,transparent 62%),
 radial-gradient(46rem 36rem at 70% 108%,#ff8a4c 0,transparent 60%),
 radial-gradient(36rem 30rem at 6% 100%,#19b8a6 0,transparent 60%),
 linear-gradient(160deg,#20245a,#3b1f4f)}
.bg::after{background:linear-gradient(180deg,rgba(0,0,0,.18),rgba(0,0,0,.05) 40%,rgba(0,0,0,.35));-webkit-mask-image:none;mask-image:none;opacity:1}
.clock{position:absolute;left:0;right:0;top:max(6vh,28px);text-align:center;color:rgba(255,255,255,.92);text-shadow:0 2px 24px rgba(0,0,0,.25)}
.clock b{display:block;font-size:clamp(64px,11vw,104px);font-weight:600;line-height:1;letter-spacing:-.03em;font-variant-numeric:tabular-nums}
.clock span{display:block;margin-top:8px;font-size:17px;font-weight:500;opacity:.85}
.wrap{align-items:flex-end;padding:calc(max(6vh,28px) + 150px) 16px 4vh}
.card{max-width:340px;padding:0;background:none;border:0;box-shadow:none;text-align:center}
.icon{width:76px;height:76px;margin:0 auto;border-radius:50%;background:rgba(255,255,255,.18);box-shadow:inset 0 0 0 1px rgba(255,255,255,.28);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px)}
.icon svg{width:30px;height:30px}
h1{margin:14px 0 2px;font-size:20px;color:#fff}
.sub{color:rgba(255,255,255,.72);font-size:13.5px}
.chip{background:rgba(255,255,255,.16);color:#fff}
.alert{background:rgba(255,80,80,.22);color:#fff;-webkit-backdrop-filter:blur(16px);backdrop-filter:blur(16px)}
.fields{margin-top:20px;gap:10px}
label.f>span{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}
.ctl>svg{display:none}.ctl input{padding-left:14px}
.ctl input{height:46px;padding:0 44px 0 20px;border:0;border-radius:999px;background:rgba(255,255,255,.18);color:#fff;-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);box-shadow:inset 0 0 0 1px rgba(255,255,255,.22)}
.ctl input::placeholder{color:rgba(255,255,255,.62)}
.ctl input:focus{background:rgba(255,255,255,.26);box-shadow:inset 0 0 0 1px rgba(255,255,255,.55),0 0 0 4px rgba(255,255,255,.12)}
.eye{right:8px;color:rgba(255,255,255,.7)}.eye:hover{background:rgba(255,255,255,.14);color:#fff}
.btn{border-radius:999px}
.btn.primary{margin-top:14px;background:#fff;color:#1c1e3c;box-shadow:0 10px 30px -10px rgba(0,0,0,.5)}
.btn.ghost{background:rgba(255,255,255,.12);color:#fff;border-color:rgba(255,255,255,.2);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px)}
.btn.ghost:hover{background:rgba(255,255,255,.2)}
.or{color:rgba(255,255,255,.62)}.or::before,.or::after{background:rgba(255,255,255,.22)}
.foot{color:rgba(255,255,255,.6)}.foot .brand{color:rgba(255,255,255,.82)}.foot .brand svg{color:rgba(255,255,255,.3)}
.modal{background:rgba(10,8,30,.35);-webkit-backdrop-filter:blur(28px) saturate(1.2);backdrop-filter:blur(28px) saturate(1.2)}
.sheet{max-width:340px}
.otp{border:0;border-radius:18px;background:rgba(255,255,255,.16);color:#fff;box-shadow:inset 0 0 0 1px rgba(255,255,255,.25)}
.otp::placeholder{color:rgba(255,255,255,.3)}
.otp:focus{background:rgba(255,255,255,.22);box-shadow:inset 0 0 0 1px rgba(255,255,255,.6)}
.tip{color:rgba(255,255,255,.6)}.back{color:rgba(255,255,255,.78)}.back:hover{color:#fff;background:rgba(255,255,255,.12)}
@media (max-height:760px){.clock{display:none}.wrap{align-items:center;padding-top:32px}}`

// loginSkinSplit 为「分屏：左侧品牌区写站点名，右侧平铺表单；窄屏时品牌区收成顶部色带」。
const loginSkinSplit = `body{display:grid;grid-template-columns:minmax(0,1.15fr) minmax(420px,1fr);grid-template-rows:1fr auto;background:var(--card)}
.bg{position:relative;inset:auto;z-index:0;grid-row:1/3;grid-column:1;overflow:hidden;background:#0b5a4b}
.bg::before{content:"";position:absolute;inset:0;background:
 repeating-radial-gradient(circle at 78% 30%,transparent 0 22px,rgba(255,255,255,.07) 22px 23px),
 radial-gradient(60rem 40rem at 0% 100%,#0e7c66,transparent 70%),
 linear-gradient(150deg,#0a4f43,#0f6e5b 55%,#139478)}
.bg::after{display:none}
.hero{position:absolute;left:clamp(32px,6vw,72px);right:32px;bottom:clamp(40px,9vh,88px);color:#fff}
.mark{width:56px;height:56px;border-radius:16px;display:grid;place-items:center;background:rgba(255,255,255,.14);box-shadow:inset 0 0 0 1px rgba(255,255,255,.25)}
.mark svg{width:28px;height:28px}
.host{margin-top:28px;font-size:clamp(28px,3.6vw,46px);font-weight:700;line-height:1.1;letter-spacing:-.02em;overflow-wrap:anywhere}
.line{margin-top:12px;font-size:16px;color:rgba(255,255,255,.72)}
.wrap{grid-column:2;grid-row:1;padding:48px clamp(28px,5vw,64px) 8px}
.foot{grid-column:2;grid-row:2;justify-content:flex-start;padding-left:clamp(28px,5vw,64px)}
.card{max-width:380px;padding:0;background:none;border:0;box-shadow:none}
.icon,.chip{display:none}
h1{margin:0 0 8px;font-size:28px;font-weight:700}
.fields{margin-top:32px}
.ctl input{height:46px}
.btn.primary{height:46px;margin-top:24px;background:var(--brand);box-shadow:none}
.sheet{max-width:380px;padding:34px 30px 28px;background:var(--card);border:1px solid var(--line);border-radius:20px;box-shadow:var(--shadow)}.sheet .icon{display:grid}.sheet .chip{display:inline-flex}.sheet h1{margin:18px 0 6px;font-size:21px}
@media (max-width:860px){
 body{grid-template-columns:1fr;grid-template-rows:auto 1fr auto}
 .bg{grid-row:1;height:180px}.hero{bottom:24px}.host{margin-top:14px;font-size:26px}.mark{width:44px;height:44px;border-radius:12px}.mark svg{width:22px;height:22px}.line{margin-top:4px;font-size:14px}
 .wrap{grid-column:1;grid-row:2;align-items:flex-start;padding:32px 22px 8px}.foot{grid-column:1;grid-row:3;justify-content:center;padding-left:16px}
}
@media (prefers-color-scheme:dark){.bg{background:#06241e}.bg::before{background:repeating-radial-gradient(circle at 78% 30%,transparent 0 22px,rgba(255,255,255,.05) 22px 23px),radial-gradient(60rem 40rem at 0% 100%,#0e5c4d,transparent 70%),linear-gradient(150deg,#062a23,#0a3f35 55%,#0e5848)}.btn.primary{color:#04231c}}`

// loginSkinMinimal 为「极简：去掉卡片与图标，黑白文字与下划线输入框」。
const loginSkinMinimal = `:root{--bg:#fff;--card:#fff;--text:#111;--muted:#6b6b70;--faint:#a0a0a6;--line:#dcdce0;--input:transparent;--brand:#111;--on-brand:#fff;--brand-soft:rgba(0,0,0,.06)}
.bg{display:none}
.card{max-width:360px;padding:0;background:none;border:0;box-shadow:none;animation:none}
.icon{display:none}
h1{margin:0 0 10px;font-size:32px;font-weight:600;letter-spacing:-.025em;line-height:1.2}
.sub{font-size:15px}
.chip{margin-top:14px;padding:0;background:none;color:var(--faint);font:12.5px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace}.chip svg{display:none}
.fields{margin-top:40px;gap:22px}
label.f>span{font-size:12px;font-weight:500;color:var(--muted);margin-bottom:2px}
.ctl>svg{display:none}.ctl input{padding-left:14px}
.ctl input{height:42px;padding:0 40px 0 0;border:0;border-bottom:1.5px solid var(--line);border-radius:0;background:none;font-size:17px}
.ctl input:focus{border-color:var(--text);background:none;box-shadow:none}
.eye{right:0}
.btn{border-radius:6px}
.btn.primary{height:50px;margin-top:36px;background:var(--brand);color:var(--on-brand);box-shadow:none;justify-content:space-between;padding:0 20px}
.btn.ghost{background:none;border-color:var(--line);font-weight:500}
.btn.ghost:hover{border-color:var(--text);background:none}
.or{margin:30px 0 14px}
.foot{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace}.foot .brand svg{color:var(--text)}
.modal{background:var(--bg);-webkit-backdrop-filter:none;backdrop-filter:none}
.sheet{max-width:340px;text-align:left}.sheet .icon{display:none}
.otp{height:64px;padding-left:0;border:0;border-bottom:2px solid var(--text);border-radius:0;background:none;text-align:left;letter-spacing:.42em}
.otp:focus{background:none;box-shadow:none}
.has-bg .card{padding:32px 28px;background:var(--card);border-radius:14px;box-shadow:0 20px 60px -20px rgba(0,0,0,.5)}
@media (prefers-color-scheme:dark){:root{--bg:#0b0b0c;--card:#0b0b0c;--text:#f2f2f3;--muted:#9a9aa1;--faint:#66666d;--line:#2a2a2f;--brand:#f2f2f3;--on-brand:#0b0b0c}}`

// loginSkinTerminal 为「终端：SSH 登录窗口、提示符与方括号按钮（仅深色）」。
const loginSkinTerminal = `:root{color-scheme:dark;--brand:#7ee787;--brand-soft:rgba(126,231,135,.10);--text:#c9d1d9;--muted:#8b949e;--faint:#6e7681;--line:#30363d;--input:transparent;--card:#010409;--danger:#ff7b72;--danger-soft:rgba(255,123,114,.08)}
body{background:#0d1117;font:14px/1.7 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace}
.bg{display:none}
.card{max-width:560px;padding:56px 26px 24px;border-radius:12px;border-color:var(--line);box-shadow:0 30px 80px -24px rgba(0,0,0,.85);position:relative;overflow:hidden}
.card::before{content:"";position:absolute;left:0;right:0;top:0;height:38px;background:radial-gradient(circle at 20px 19px,#ff5f56 5.5px,transparent 6px),#161b22;border-bottom:1px solid var(--line)}
.card::after{content:"";position:absolute;left:38px;top:13px;width:12px;height:12px;border-radius:50%;background:#ffbd2e;box-shadow:18px 0 0 #27c93f}
.icon{display:none}
h1{margin:0;font-size:14px;font-weight:400;color:var(--text);letter-spacing:0}
h1::before{content:"$ ";color:var(--brand)}
.sub{color:var(--muted)}.sub::before{content:"# "}
.chip{margin-top:4px;padding:0;background:none;color:#79c0ff;font-size:14px;font-weight:400}.chip svg{display:none}.chip::before{content:"→ ";color:var(--faint)}
.alert{border-radius:0;background:none;padding:0;margin-top:14px}.alert svg{display:none}.alert span::before{content:"error: "}
.fields{margin-top:18px;gap:4px}
label.f{display:grid;grid-template-columns:auto 1fr;align-items:center}
label.f>span{margin:0;font-size:14px;font-weight:400;color:var(--brand)}
label.f>span::after{content:":"}
.ctl>svg{display:none}.ctl input{padding-left:14px}
.ctl input{height:30px;padding:0 36px 0 10px;border:0;border-radius:0;background:none;font:inherit;caret-color:var(--brand);color:#e6edf3}
.ctl input::placeholder{color:#484f58}
.ctl input:focus{background:rgba(126,231,135,.06);box-shadow:none}
.eye{width:26px;height:26px;right:0;border-radius:4px}
.btn{height:auto;padding:4px 0;border:0;border-radius:0;background:none!important;box-shadow:none!important;font:inherit;font-weight:400;color:#79c0ff}
.btn svg{display:none}
.btn::before{content:"[ ";color:var(--faint)}.btn::after{content:" ]";color:var(--faint)}
.btn:hover{text-decoration:underline;filter:none}
.btn.primary{width:auto;margin-top:16px;color:var(--brand)}
.or{justify-content:flex-start;margin:18px 0 6px;color:var(--faint);font-size:13px}.or::before{content:"#";flex:none;height:auto;background:none}.or::after{display:none}
.oauth{display:flex;flex-direction:column;align-items:flex-start;gap:0}.oauth .btn{width:auto;font-size:14px}
.pk{margin-top:0}.pk .btn.ghost{width:auto}
.foot{color:#484f58;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace}.foot .brand{color:var(--muted)}
.modal{background:rgba(1,4,9,.72);-webkit-backdrop-filter:blur(3px);backdrop-filter:blur(3px)}
.sheet{text-align:left;max-width:520px}
.otp{height:52px;margin-top:18px;padding-left:14px;border:1px dashed var(--line);border-radius:6px;background:none;color:var(--brand);text-align:left;font-size:26px;letter-spacing:.5em}
.otp::placeholder{color:#30363d}
.otp:focus{border-color:var(--brand);border-style:solid;background:none;box-shadow:none}
.sheet .btn.primary{margin-top:12px}
.tip{color:var(--faint)}.tip::before{content:"# "}
.back{margin-top:6px;padding:0;color:#79c0ff;font-size:14px}.back svg{display:none}.back::before{content:"cd ..";color:var(--faint);margin-right:8px}.back:hover{background:none;text-decoration:underline}`

// loginSkinPaper 为「信笺：暖色纸面、宋体标题、印章图标与横线输入框」。
const loginSkinPaper = `:root{--bg:#ece5d6;--card:#fbf8f0;--text:#2a2419;--muted:#7a6f5c;--faint:#a89c84;--line:#d6cab1;--input:transparent;--brand:#a8321f;--ink:#1f3a5f;--brand-soft:rgba(168,50,31,.08)}
body{background:var(--bg)}
.bg{background:radial-gradient(70rem 40rem at 50% 0%,rgba(255,255,255,.55),transparent 70%)}
.bg::after{background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .35 0 0 0 0 .28 0 0 0 0 .18 0 0 0 .5 0'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");background-size:160px;opacity:.35;-webkit-mask-image:none;mask-image:none}
.card{max-width:420px;padding:44px 40px 34px;border-radius:3px;border-color:var(--line);box-shadow:0 1px 0 rgba(255,255,255,.8) inset,0 2px 3px rgba(60,40,10,.06),0 24px 50px -24px rgba(60,40,10,.35);position:relative}
.card::before{content:"";position:absolute;left:40px;right:40px;top:18px;border-top:3px double var(--line)}
.icon{width:58px;height:58px;border-radius:50%;background:none;color:var(--brand);box-shadow:inset 0 0 0 2px var(--brand),inset 0 0 0 5px var(--card),inset 0 0 0 6px var(--brand);transform:rotate(-8deg)}
.icon svg{width:24px;height:24px}
h1{margin:22px 0 8px;font:600 27px/1.3 "Songti SC","STSong","Noto Serif CJK SC","Source Han Serif SC",Georgia,"Times New Roman",serif;letter-spacing:.02em}
.sub{font-size:14px}
.chip{padding:0;border-radius:0;background:none;color:var(--muted);border-bottom:1px dashed var(--faint);font-size:12.5px}
.alert{border-radius:2px;border-left:3px solid var(--brand);background:var(--brand-soft);color:var(--brand)}
.fields{margin-top:30px;gap:20px}
label.f>span{font:13px/1.4 "Songti SC","STSong","Noto Serif CJK SC","Source Han Serif SC",Georgia,"Times New Roman",serif;color:var(--muted);margin-bottom:0}
.ctl>svg{display:none}.ctl input{padding-left:14px}
.ctl input{height:40px;padding:0 40px 0 2px;border:0;border-bottom:1px solid #bdae8f;border-radius:0;background:none;font-size:16px}
.ctl input::placeholder{color:#c2b69c}
.ctl input:focus{background:none;border-color:var(--ink);box-shadow:0 1px 0 var(--ink)}
.eye{right:0}
.btn{border-radius:2px;font-family:"Songti SC","STSong","Noto Serif CJK SC","Source Han Serif SC",Georgia,"Times New Roman",serif;letter-spacing:.08em}
.btn.primary{height:48px;margin-top:30px;background:var(--ink);color:#f6f1e4;box-shadow:0 2px 0 #142941}
.btn.ghost{background:transparent;border-color:#cbbd9f;font-weight:500;letter-spacing:.02em}
.btn.ghost:hover{background:rgba(31,58,95,.05);border-color:var(--ink)}
.or{font-family:"Songti SC","STSong","Noto Serif CJK SC","Source Han Serif SC",Georgia,"Times New Roman",serif}.or::before,.or::after{background:none;border-top:1px dashed var(--line)}
.foot{color:var(--faint)}.foot .brand{color:var(--muted)}.foot .brand svg{color:var(--brand)}
.modal{background:rgba(60,45,20,.28);-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)}
.otp{height:62px;border:0;border-bottom:2px solid var(--ink);border-radius:0;background:none;font-family:"Songti SC","STSong","Noto Serif CJK SC","Source Han Serif SC",Georgia,"Times New Roman",serif;color:var(--ink)}
.otp::placeholder{color:#d8ccb2}
.otp:focus{background:none;box-shadow:none}
.back:hover{background:var(--brand-soft)}
@media (prefers-color-scheme:dark){:root{--bg:#1a1712;--card:#242019;--text:#ebe3d2;--muted:#a99d86;--faint:#7a6f5c;--line:#3a3328;--brand:#e0735d;--ink:#9cc0ea;--brand-soft:rgba(224,115,93,.12)}.bg{background:none}.bg::after{opacity:.18}.card{box-shadow:0 24px 50px -24px rgba(0,0,0,.7)}.ctl input{border-color:#5a4f3e}.ctl input::placeholder{color:#6a5f4c}.btn.primary{color:#1a1712;box-shadow:0 2px 0 #5f86b3}.btn.ghost{border-color:#4a4134}.otp::placeholder{color:#4a4134}}`

// loginSkinBrutalist 为「粗野：黄底、黑粗边与硬投影（仅浅色）」。
const loginSkinBrutalist = `:root{color-scheme:light;--brand:#3a86ff;--text:#111;--muted:#333;--faint:#555;--line:#111;--card:#fff;--input:#fff;--danger:#d40000;--danger-soft:#ffe1e1}
body{background:#ffd23f;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif}
.bg{background:repeating-linear-gradient(-45deg,transparent 0 22px,rgba(0,0,0,.05) 22px 24px)}
.bg::after{display:none}
.card{max-width:420px;padding:34px 30px 30px;border:3px solid #111;border-radius:0;box-shadow:10px 10px 0 #111}
.icon{width:56px;height:56px;border-radius:0;background:#ff5d8f;color:#111;border:3px solid #111;box-shadow:4px 4px 0 #111}
h1{margin:22px 0 6px;font-size:30px;font-weight:900;line-height:1.15;letter-spacing:-.02em}
.sub{color:#222;font-weight:500}
.chip{border-radius:0;background:#111;color:#ffd23f;font-weight:700;padding:4px 10px}
.alert{border:3px solid #111;border-radius:0;background:#ff5d8f;color:#111;font-weight:700}
label.f>span{font-weight:800;text-transform:uppercase;letter-spacing:.04em}
.ctl>svg{color:#111}
.ctl input{height:48px;border:3px solid #111;border-radius:0;background:#fff;font-weight:600}
.ctl input::placeholder{color:#888}
.ctl input:focus{background:#fff;border-color:#111;box-shadow:5px 5px 0 #111}
.eye{color:#111}.eye:hover{background:#ffd23f}
.btn{height:50px;border:3px solid #111;border-radius:0;font-weight:900;box-shadow:5px 5px 0 #111;transition:transform .06s,box-shadow .06s}
.btn:active{transform:translate(5px,5px);box-shadow:0 0 0 #111}
.btn.primary{margin-top:26px;background:var(--brand);color:#fff;font-size:17px}
.btn.primary:hover{filter:brightness(.94)}
.btn.ghost{background:#fff;color:#111;border-color:#111}
.btn.ghost:hover{background:#ffd23f;border-color:#111}
.oauth{gap:14px}.pk{margin-top:14px}
.or{color:#111;font-weight:800}.or::before,.or::after{height:3px;background:#111}
.foot{color:#111;font-weight:700}.foot .brand{color:#111}.foot .brand svg{color:#111}.foot .sep{opacity:1}
.modal{background:rgba(255,210,63,.94);-webkit-backdrop-filter:none;backdrop-filter:none}
.sheet{animation:none}
.otp{height:64px;border:3px solid #111;border-radius:0;background:#fff;color:#111;font-weight:900}
.otp::placeholder{color:#ddd}
.otp:focus{background:#fff;border-color:#111;box-shadow:6px 6px 0 #111}
.tip{color:#333;font-weight:600}.back{color:#111;font-weight:700;border-radius:0}.back:hover{background:#111;color:#ffd23f}`

// loginSkinAurora 为「极光：深色底上缓慢流动的极光与玻璃卡片（仅深色）」。
const loginSkinAurora = `:root{color-scheme:dark;--brand:#5eead4;--brand-2:#818cf8;--brand-soft:rgba(94,234,212,.12);--card:rgba(15,20,40,.55);--text:#eef2ff;--muted:#a3acc9;--faint:#6f7896;--line:rgba(255,255,255,.12);--input:rgba(255,255,255,.05)}
body{background:#05081a}
.bg{overflow:hidden;background:radial-gradient(80rem 50rem at 50% 120%,#0c1240,transparent 70%)}
.bg::after{background-image:radial-gradient(rgba(255,255,255,.55) 1px,transparent 1.5px);background-size:120px 120px;background-position:13px 29px;opacity:.25}
.bg i{position:absolute;border-radius:50%;filter:blur(70px);opacity:.75;mix-blend-mode:screen;animation:drift 18s ease-in-out infinite alternate}
.a1{width:46vw;height:30vw;left:-6vw;top:-4vw;background:#14b8a6}
.a2{width:40vw;height:28vw;right:-8vw;top:8vh;background:#6366f1;animation-duration:22s!important;animation-delay:-6s!important}
.a3{width:44vw;height:24vw;left:24vw;bottom:-10vw;background:#a855f7;opacity:.5!important;animation-duration:26s!important}
@keyframes drift{0%{transform:translate(0,0) scale(1)}50%{transform:translate(6vw,4vh) scale(1.15)}100%{transform:translate(-4vw,-3vh) scale(.95)}}
@media (prefers-reduced-motion:reduce){.bg i{animation:none}}
.card{position:relative;max-width:400px;border-radius:24px;border:1px solid transparent;background:linear-gradient(var(--card),var(--card)) padding-box,linear-gradient(140deg,rgba(94,234,212,.55),rgba(255,255,255,.06) 40%,rgba(129,140,248,.55)) border-box;-webkit-backdrop-filter:blur(24px) saturate(1.3);backdrop-filter:blur(24px) saturate(1.3);box-shadow:0 30px 80px -30px rgba(0,0,0,.8)}
.icon{background:linear-gradient(135deg,var(--brand),var(--brand-2));color:#05081a;box-shadow:0 10px 30px -8px rgba(94,234,212,.6)}
.chip{background:rgba(94,234,212,.1);color:var(--brand)}
.ctl input{border-color:var(--line)}
.ctl input:focus{background:rgba(255,255,255,.08)}
.btn.primary{color:#05081a;background:linear-gradient(110deg,var(--brand),var(--brand-2));box-shadow:0 12px 30px -12px rgba(129,140,248,.8)}
.btn.ghost{background:rgba(255,255,255,.04)}
.btn.ghost:hover{background:rgba(255,255,255,.09);border-color:var(--line)}
.modal{background:rgba(5,8,26,.45)}
.sheet{background:linear-gradient(rgba(15,20,40,.82),rgba(15,20,40,.82)) padding-box,linear-gradient(140deg,rgba(94,234,212,.55),rgba(255,255,255,.06) 40%,rgba(129,140,248,.55)) border-box}
.otp{background:rgba(255,255,255,.04)}
.otp::placeholder{color:rgba(255,255,255,.12)}
.otp:focus{background:rgba(255,255,255,.07)}`

// loginSkinPixel 为「像素：8 位游戏风格的阶梯描边与方块按钮（仅深色）」。
const loginSkinPixel = `:root{color-scheme:dark;--brand:#ffcc33;--text:#f4f1ff;--muted:#b7b2e0;--faint:#8c86c4;--line:#4b4f9a;--card:#2b2e66;--input:#151741;--danger:#ff6b6b;--danger-soft:rgba(255,107,107,.15)}
body{background:#181a46;font:15px/1.6 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace;-webkit-font-smoothing:none}
.bg{background:linear-gradient(#181a46 60%,#2a1f5e)}
.bg::after{background:none}
.st{position:absolute;inset:0;background-image:radial-gradient(#fff 1px,transparent 1.6px),radial-gradient(#ffcc33 1px,transparent 1.6px);background-size:90px 70px,150px 130px;background-position:10px 20px,60px 40px;opacity:.5}
.card{max-width:420px;padding:32px 28px 28px;border:0;border-radius:0;background:var(--card);animation:none;
 box-shadow:0 -6px 0 0 #f4f1ff,0 6px 0 0 #f4f1ff,-6px 0 0 0 #f4f1ff,6px 0 0 0 #f4f1ff,0 12px 0 6px #0e0f2e}
.icon{width:56px;height:56px;border-radius:0;background:var(--brand);color:#181a46;box-shadow:0 -4px 0 #181a46,0 4px 0 #181a46,-4px 0 0 #181a46,4px 0 0 #181a46}
.icon svg{stroke-width:2.6;shape-rendering:crispEdges}
h1{margin:24px 0 6px;font-size:22px;font-weight:800;text-transform:uppercase;letter-spacing:.04em;color:var(--brand);text-shadow:3px 3px 0 #0e0f2e}
.sub{font-size:13.5px}
.chip{border-radius:0;background:#151741;color:#7ef0ff;font-size:12.5px}
.alert{border-radius:0;box-shadow:0 0 0 3px var(--danger)}
label.f>span{font-weight:800;color:#7ef0ff;text-transform:uppercase}
label.f>span::before{content:"▶ "}
.ctl>svg{color:var(--faint)}
.ctl input{height:46px;border:0;border-radius:0;background:var(--input);color:var(--text);font:inherit;caret-color:var(--brand);box-shadow:inset 0 0 0 3px var(--line)}
.ctl input::placeholder{color:#5a5f9e}
.ctl input:focus{background:var(--input);box-shadow:inset 0 0 0 3px var(--brand)}
.eye{border-radius:0}.eye:hover{background:var(--line)}
.btn{height:50px;border:0;border-radius:0;font:800 15px/1 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace;text-transform:uppercase;letter-spacing:.06em;transition:none}
.btn.primary{margin-top:28px;background:var(--brand);color:#181a46;box-shadow:0 -4px 0 #181a46 inset,0 0 0 3px #0e0f2e,0 6px 0 3px #0e0f2e}
.btn.primary:hover{background:#ffe066;filter:none}
.btn:active{transform:translateY(4px)}
.btn.ghost{background:#3b3f86;color:var(--text);box-shadow:0 -4px 0 #24275e inset,0 0 0 3px #0e0f2e}
.btn.ghost:hover{background:#4a4fa0;border-color:transparent}
.oauth{gap:14px}.pk{margin-top:14px}
.or{color:var(--faint);text-transform:uppercase}.or::before,.or::after{height:3px;background:repeating-linear-gradient(90deg,var(--line) 0 6px,transparent 6px 12px)}
.foot{color:var(--faint)}.foot .brand{color:var(--muted)}.foot .brand svg{color:var(--brand)}
.modal{background:rgba(14,15,46,.94);-webkit-backdrop-filter:none;backdrop-filter:none}
.sheet{animation:none}
.otp{height:64px;border:0;border-radius:0;background:var(--input);color:var(--brand);font:800 30px/1 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace;box-shadow:inset 0 0 0 3px var(--line)}
.otp::placeholder{color:#2c2f6e}
.otp:focus{background:var(--input);box-shadow:inset 0 0 0 3px var(--brand)}
.back{border-radius:0;text-transform:uppercase}.back:hover{background:var(--line);color:#fff}`

// loginSkinBlueprint 为「蓝图：工程制图网格、白色细线与角标（仅深色）」。
const loginSkinBlueprint = `:root{color-scheme:dark;--brand:#fff;--text:#f2f7ff;--muted:#b9d0f2;--faint:#86a9d9;--line:rgba(255,255,255,.4);--card:transparent;--input:rgba(255,255,255,.04);--danger:#ffd0c8;--danger-soft:rgba(255,120,100,.18)}
body{background:#14448f;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif}
.bg{background:radial-gradient(70rem 40rem at 50% 50%,rgba(255,255,255,.06),transparent 70%)}
.bg::after{background-image:linear-gradient(rgba(255,255,255,.18) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.18) 1px,transparent 1px),linear-gradient(rgba(255,255,255,.07) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.07) 1px,transparent 1px);background-size:100px 100px,100px 100px,20px 20px,20px 20px;opacity:1;-webkit-mask-image:none;mask-image:none}
.card{position:relative;max-width:420px;border:1.5px solid rgba(255,255,255,.85);border-radius:0;background:rgba(20,68,143,.82);box-shadow:none;padding:40px 34px 32px}
.card::before{content:"A-01 · AUTH";position:absolute;left:-1.5px;top:-24px;padding:0 0 4px;color:var(--muted);font:11.5px/1 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace;letter-spacing:.12em}
.card::after{content:"";position:absolute;inset:-10px;border:1px dashed rgba(255,255,255,.3);pointer-events:none}
.icon{width:56px;height:56px;border-radius:50%;background:none;color:#fff;box-shadow:inset 0 0 0 1.5px #fff;position:relative}
.icon svg{stroke-width:1.5}
h1{font-weight:600}
.chip{border-radius:0;background:none;color:var(--muted);box-shadow:inset 0 0 0 1px var(--line);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace}
.alert{border-radius:0;box-shadow:inset 0 0 0 1px var(--danger)}
label.f>span{font:11.5px/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace;letter-spacing:.1em;color:var(--muted);text-transform:uppercase}
.ctl>svg{color:var(--muted)}
.ctl input{border:1px solid var(--line);border-radius:0;background:var(--input)}
.ctl input::placeholder{color:rgba(255,255,255,.38)}
.ctl input:focus{border-color:#fff;background:rgba(255,255,255,.08);box-shadow:0 0 0 3px rgba(255,255,255,.12)}
.eye{border-radius:0;color:var(--muted)}.eye:hover{background:rgba(255,255,255,.1);color:#fff}
.btn{border-radius:0}
.btn.primary{background:#fff;color:#14448f;box-shadow:none}
.btn.ghost{background:none;color:#fff;border-color:var(--line)}
.btn.ghost:hover{background:rgba(255,255,255,.08);border-color:#fff}
.or{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace;letter-spacing:.06em}.or::before,.or::after{background:none;border-top:1px dashed var(--line)}
.foot{color:var(--faint);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","PingFang SC","Microsoft YaHei",monospace}.foot .brand{color:var(--muted)}.foot .brand svg{color:rgba(255,255,255,.25)}
.modal{background:rgba(10,40,90,.6)}
.sheet{background:#14448f}
.sheet::before{content:"A-02 · TOTP"}
.otp{border:1px solid var(--line);border-radius:0;background:var(--input)}
.otp::placeholder{color:rgba(255,255,255,.2)}
.otp:focus{border-color:#fff;background:rgba(255,255,255,.06);box-shadow:none}
.back{border-radius:0}.back:hover{background:rgba(255,255,255,.08)}`

// loginSkinList 为内置样式，顺序即管理界面中的展示顺序。
var loginSkinList = []loginSkin{
	{id: "lockscreen", css: loginSkinLockscreen},
	{id: "split", css: loginSkinSplit},
	{id: "minimal", css: loginSkinMinimal},
	{id: "terminal", css: loginSkinTerminal},
	{id: "paper", css: loginSkinPaper},
	{id: "brutalist", css: loginSkinBrutalist},
	{id: "aurora", css: loginSkinAurora},
	{id: "pixel", css: loginSkinPixel},
	{id: "blueprint", css: loginSkinBlueprint},
}
