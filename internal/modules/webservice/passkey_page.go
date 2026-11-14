package webservice

import "github.com/wangwanjie/gately/internal/webpage"

// 登录页与 Passkey 管理页共用的浏览器脚本片段：base64url 转换与 JSON 请求。模板内不写中文，文案通过 data 属性传入。
const passkeyHelpersJS = `var pk={
  d:function(s){s=s.replace(/-/g,'+').replace(/_/g,'/');while(s.length%4){s+='=';}var b=atob(s),u=new Uint8Array(b.length);for(var i=0;i<b.length;i++){u[i]=b.charCodeAt(i);}return u.buffer;},
  e:function(buf){var u=new Uint8Array(buf),s='';for(var i=0;i<u.length;i++){s+=String.fromCharCode(u[i]);}return btoa(s).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');},
  post:function(url,body){return fetch(url,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}).then(function(r){return r.json().catch(function(){return {};}).then(function(j){if(!r.ok){var e=new Error(j.error||'');e.server=true;throw e;}return j;});});},
  ok:function(){return !!(window.PublicKeyCredential&&navigator.credentials&&navigator.credentials.get&&navigator.credentials.create);}
};`

// passkeySrc 是 Passkey 管理页的主体：先验证用户名、密码（与动态验证码），再列出、添加、删除自己的 Passkey。
// 所有请求由脚本发往 {{.Base}} 下的接口，列表用 DOM 方法构建（名称按文本显示，不会被当作 HTML）。
const passkeySrc = `{{define "body"}}<section class="card" id="pk-app" data-base="{{.Base}}" data-lang="{{.Lang}}"
 data-unsupported="{{.T.unsupported}}" data-cancelled="{{.T.cancelled}}" data-failed="{{.T.failed}}" data-created="{{.T.createdOn}}" data-used="{{.T.usedOn}}" data-confirm="{{.T.confirmDelete}}" data-added="{{.T.added}}" data-del="{{.T.delete}}">
<div class="icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 10a2 2 0 0 0-2 2c0 1.02-.1 2.51-.26 4"/><path d="M14 13.12c0 2.38 0 6.38-1 8.88"/><path d="M17.29 21.02c.12-.6.43-2.3.5-3.02"/><path d="M2 12a10 10 0 0 1 18-6"/><path d="M2 16h.01"/><path d="M21.8 16c.2-2 .131-5.354 0-6"/><path d="M5 19.5C5.5 18 6 15 6 12a6 6 0 0 1 .34-2"/><path d="M8.65 22c.21-.66.45-1.32.57-2"/><path d="M9 6.8a6 6 0 0 1 9 5.2v2"/></svg></div>
<h1>{{.T.title}}</h1>
<p class="sub">{{.T.subtitle}}</p>
<div class="chip"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg><span>{{.Host}}</span></div>
<div class="alert" id="pk-alert" role="alert"{{if not .Error}} hidden{{end}}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg><span id="pk-msg">{{.Error}}</span></div>
{{if not .Error}}<form id="pk-auth" autocomplete="on">
<div class="fields">
<label class="f"><span>{{.T.username}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/></svg><input name="username" id="pk-user" placeholder="{{.T.usernamePh}}" autocomplete="username" autocapitalize="none" spellcheck="false" required autofocus></div></label>
<label class="f"><span>{{.T.password}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg><input name="password" id="pk-pass" type="password" placeholder="{{.T.passwordPh}}" autocomplete="current-password" required></div></label>
<label class="f"><span>{{.T.code}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/></svg><input name="code" id="pk-code" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" placeholder="{{.T.codePh}}" autocomplete="one-time-code"></div></label>
</div>
<button class="btn primary" type="submit" id="pk-verify">{{.T.verify}}</button>
</form>
<div id="pk-manage" hidden>
<div class="chip"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/></svg><span id="pk-who"></span></div>
<h2 class="pk-h">{{.T.listTitle}}</h2>
<ul class="pk-list" id="pk-list"></ul>
<p class="pk-empty" id="pk-empty" hidden>{{.T.empty}}</p>
<div class="pk-add"><input id="pk-name" maxlength="40" placeholder="{{.T.namePh}}" aria-label="{{.T.namePh}}"><button class="btn primary" type="button" id="pk-add">{{.T.add}}</button></div>
</div>{{end}}
<a class="back" href="{{.LoginURL}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>{{.T.backLogin}}</a>
</section>
<style>
[hidden]{display:none!important}
.back{display:inline-flex;align-items:center;gap:6px;margin-top:16px;padding:4px 8px;border-radius:8px;color:var(--muted);text-decoration:none;font-size:13px}
.back:hover{color:var(--text);background:var(--brand-soft)}
.back svg{width:15px;height:15px}
.pk-h{margin:22px 0 10px;font-size:14px;font-weight:600;color:var(--text);text-align:left}
.pk-list{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:8px}
.pk-list li{display:flex;align-items:center;gap:10px;padding:10px 12px;border:1px solid var(--line);border-radius:12px;background:var(--input);text-align:left}
.pk-list .pk-meta{min-width:0;flex:1}
.pk-list .pk-nm{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text);font-weight:600;font-size:14px}
.pk-list .pk-dt{display:block;color:var(--faint);font-size:12px;margin-top:2px}
.pk-list button{flex:none;padding:6px 10px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--danger);font:inherit;font-size:13px;cursor:pointer}
.pk-list button:hover{border-color:var(--danger)}
.pk-ok{background:var(--brand-soft);color:var(--brand);border-color:var(--brand-soft)}
.pk-empty{margin:4px 0 0;color:var(--muted);font-size:13.5px;text-align:left}
.pk-add{display:flex;gap:8px;margin-top:16px}
.pk-add input{flex:1;min-width:0;height:44px;padding:0 12px;border:1px solid var(--line);border-radius:12px;background:var(--input);color:var(--text);font:inherit;outline:none}
.pk-add input:focus{border-color:var(--brand);background:var(--card);box-shadow:0 0 0 4px var(--brand-soft)}
.pk-add .btn{flex:none;width:auto;margin:0;padding:0 16px;white-space:nowrap}
@media (max-width:480px){.pk-add{flex-direction:column}.pk-add input{flex:none;width:100%}.pk-add .btn{width:100%;height:44px}}
</style>
<script>
(function(){
` + passkeyHelpersJS + `
  var app=document.getElementById('pk-app'),form=document.getElementById('pk-auth');
  if(!form){return;}
  var base=app.dataset.base,token='',box=document.getElementById('pk-alert'),msg=document.getElementById('pk-msg');
  var list=document.getElementById('pk-list'),empty=document.getElementById('pk-empty');
  function show(t,good){msg.textContent=t;box.classList.toggle('pk-ok',!!good);box.hidden=false;}
  function hide(){box.hidden=true;box.classList.remove('pk-ok');}
  function fail(e){show(e&&e.server&&e.message?e.message:(e&&e.name==='NotAllowedError'?app.dataset.cancelled:app.dataset.failed));}
  if(!pk.ok()){show(app.dataset.unsupported);form.querySelector('button[type=submit]').disabled=true;return;}
  function fmt(tpl,t){var d=new Date(t);return tpl.replace('%s',isNaN(d)?'':d.toLocaleDateString(app.dataset.lang));}
  function render(items){
    list.textContent='';
    items.forEach(function(p){
      var li=document.createElement('li'),m=document.createElement('div'),n=document.createElement('span'),t=document.createElement('span'),b=document.createElement('button');
      m.className='pk-meta';n.className='pk-nm';n.textContent=p.name;t.className='pk-dt';
      t.textContent=fmt(app.dataset.created,p.createdAt)+(p.lastUsed?' · '+fmt(app.dataset.used,p.lastUsed):'');
      b.type='button';b.textContent=b.title=app.dataset.del||'';
      b.addEventListener('click',function(){
        if(!window.confirm(app.dataset.confirm)){return;}
        hide();b.disabled=true;
        pk.post(base+'/manage/delete',{token:token,id:p.id}).then(function(j){render(j.passkeys);}).catch(function(e){b.disabled=false;fail(e);});
      });
      m.appendChild(n);m.appendChild(t);li.appendChild(m);li.appendChild(b);list.appendChild(li);
    });
    empty.hidden=items.length>0;
  }
  form.addEventListener('submit',function(ev){
    ev.preventDefault();hide();
    var btn=document.getElementById('pk-verify');btn.disabled=true;
    pk.post(base+'/manage/auth',{username:document.getElementById('pk-user').value,password:document.getElementById('pk-pass').value,code:document.getElementById('pk-code').value}).then(function(j){
      token=j.token;document.getElementById('pk-pass').value='';document.getElementById('pk-code').value='';
      document.getElementById('pk-who').textContent=j.username;
      form.hidden=true;document.getElementById('pk-manage').hidden=false;render(j.passkeys);
    }).catch(function(e){btn.disabled=false;fail(e);});
  });
  document.getElementById('pk-add').addEventListener('click',function(){
    var add=this;hide();add.disabled=true;
    pk.post(base+'/manage/begin',{token:token}).then(function(o){
      return navigator.credentials.create({publicKey:{
        challenge:pk.d(o.challenge),rp:o.rp,user:{id:pk.d(o.user.id),name:o.user.name,displayName:o.user.displayName},
        pubKeyCredParams:o.pubKeyCredParams,timeout:o.timeout,attestation:o.attestation,authenticatorSelection:o.authenticatorSelection,
        excludeCredentials:(o.excludeCredentials||[]).map(function(c){return {type:c.type,id:pk.d(c.id)};})
      }}).then(function(c){
        var r=c.response;
        return pk.post(base+'/manage/finish',{token:token,name:document.getElementById('pk-name').value,challenge:o.challenge,credential:{id:c.id,type:c.type,response:{clientDataJSON:pk.e(r.clientDataJSON),attestationObject:pk.e(r.attestationObject)}}});
      });
    }).then(function(j){document.getElementById('pk-name').value='';render(j.passkeys);show(app.dataset.added,true);add.disabled=false;}).catch(function(e){add.disabled=false;fail(e);});
  });
})();
</script>{{end}}`

// passkeyTexts 为 Passkey 管理页的文案，中文为翻译键。
var passkeyTexts = map[string]string{
	"pageTitle":     "Passkey 管理",
	"title":         "管理 Passkey",
	"subtitle":      "验证身份后，添加或删除用于登录的 Passkey",
	"username":      "用户名",
	"usernamePh":    "请输入用户名",
	"password":      "密码",
	"passwordPh":    "请输入密码",
	"code":          "动态验证码",
	"codePh":        "已开启两步验证时填写",
	"verify":        "验证身份",
	"listTitle":     "已注册的 Passkey",
	"empty":         "尚未注册 Passkey",
	"namePh":        "给这个 Passkey 起个名字（可选）",
	"add":           "添加 Passkey",
	"delete":        "删除",
	"confirmDelete": "确定删除这个 Passkey 吗？",
	"createdOn":     "添加于 %s",
	"usedOn":        "最近使用 %s",
	"added":         "Passkey 已添加",
	"unsupported":   "此浏览器不支持 Passkey，请换用较新的浏览器",
	"cancelled":     "已取消 Passkey 验证",
	"failed":        "操作失败，请重试",
	"backLogin":     "返回登录",
}

func init() {
	webpage.MustRegister("ws-passkey", passkeySrc)
}
