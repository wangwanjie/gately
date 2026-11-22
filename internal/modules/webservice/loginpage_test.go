package webservice

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/webpage"
)

// tinyPNG 为 1×1 的 PNG 图片。
var tinyPNG = func() string {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGBgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}()

func TestLoginPageValidate(t *testing.T) {
	ok := LoginPage{Style: "terminal", Color: "#C2410C", Background: tinyPNG, BgDim: 30, Logo: "https://example.com/logo.svg", Title: "家庭影音库", CSS: ".card{border-radius:0}"}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	if ok.Color != "#c2410c" {
		t.Fatalf("主色应转小写: %s", ok.Color)
	}
	def := LoginPage{Style: "default"}
	if err := def.validate(); err != nil || def.Style != "" {
		t.Fatalf("default 应规范化为空: %v %q", err, def.Style)
	}
	for name, bad := range map[string]LoginPage{
		"未知样式":   {Style: "nope"},
		"主色格式":   {Color: "red"},
		"遮罩过大":   {BgDim: 95},
		"背景协议":   {Background: "javascript:alert(1)"},
		"Logo协议": {Logo: "ftp://x/logo.png"},
		"Logo过大": {Logo: "data:image/png;base64," + strings.Repeat("A", maxLoginLogo)},
		"Logo类型": {Logo: "data:text/html;base64,PGgxPg=="},
		"标题过长":   {Title: strings.Repeat("长", maxLoginTitle+1)},
		"标题换行":   {Title: "a\nb"},
		"CSS过大":  {CSS: strings.Repeat("a", maxLoginCSS+1)},
	} {
		if err := bad.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
	if validateSiteLoginStyle("default") != nil || validateSiteLoginStyle("") != nil || validateSiteLoginStyle("paper") != nil || validateSiteLoginStyle("x") == nil {
		t.Fatal("站点样式校验不符")
	}
}

// getLogin 以 GET 打开站点的登录页（或认证路径下的其他地址）。
func getLogin(a *authenticator, s Site, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://rl.test"+s.authPrefix()+path, nil)
	w := httptest.NewRecorder()
	a.handleLogin(w, req, s, "198.51.100.7")
	return w
}

// 全局外观生效到登录页：样式类名、主色、背景图（内联图片换成带指纹的地址）、Logo、标题与说明、隐藏页脚标识、
// 自定义 CSS 无法提前结束 <style>；站点单独设置的样式优先，default 回到默认样式。
func TestLoginPageLook(t *testing.T) {
	a := newAuthenticator("secret")
	s := loginSite(t)
	if err := a.look.set(LoginPage{Style: "paper", Color: "#c2410c", Background: tinyPNG, BgDim: 40, Logo: tinyPNG,
		Title: "家庭影音库", Subtitle: "仅限家庭成员", HideBrand: true, CSS: `.card{color:red}</style><script>alert(1)</script>`}); err != nil {
		t.Fatal(err)
	}
	body := getLogin(a, s, "/login").Body.String()
	for _, want := range []string{`class="skin-paper has-bg"`, "--brand:#c2410c", authPath + lookBgSuffix + "?v=", `<div class="icon logo"><img src="` + authPath + lookLogoSuffix + "?v=",
		"<h1>家庭影音库</h1>", "仅限家庭成员", ".foot .brand,.foot .sep{display:none}", `\3c /style>`} {
		if !strings.Contains(body, want) {
			t.Errorf("登录页缺少 %q", want)
		}
	}
	if strings.Contains(body, "<script>alert(1)") || strings.Contains(body, "base64,") {
		t.Errorf("自定义 CSS 不应插入标签，内联图片不应写进页面")
	}
	s.LoginStyle = "terminal"
	if body := getLogin(a, s, "/login").Body.String(); !strings.Contains(body, `class="skin-terminal has-bg"`) {
		t.Error("站点样式应优先于全局设置")
	}
	s.LoginStyle = LoginStyleDefault
	if body := getLogin(a, s, "/login").Body.String(); !strings.Contains(body, `class="skin-default has-bg"`) {
		t.Error("站点设为 default 应使用默认样式")
	}
	// 修改密码页同样使用外观，但不替换它的标题
	s.SharedUsers = true
	a.selfAuth.Store(true)
	if body := getLogin(a, s, passwordSuffix).Body.String(); !strings.Contains(body, "skin-default") || strings.Contains(body, "<h1>家庭影音库</h1>") {
		t.Error("修改密码页应使用外观、保留自己的标题")
	}
}

// 上传的背景图与 Logo 由认证路径输出：带 ETag、可缓存、sandbox；未上传时 404。
func TestLoginPageAssets(t *testing.T) {
	a := newAuthenticator("secret")
	s := loginSite(t)
	if w := getLogin(a, s, lookBgSuffix); w.Code != http.StatusNotFound {
		t.Fatalf("未设置背景时应 404: %d", w.Code)
	}
	_ = a.look.set(LoginPage{Background: tinyPNG, Logo: "https://example.com/l.png"})
	w := getLogin(a, s, lookBgSuffix)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("ETag") == "" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("背景图: %d %v", w.Code, w.Header())
	}
	req := httptest.NewRequest(http.MethodGet, "http://rl.test"+authPath+lookBgSuffix, nil)
	req.Header.Set("If-None-Match", w.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req, s, "198.51.100.7")
	if rec.Code != http.StatusNotModified {
		t.Fatalf("ETag 相同应 304: %d", rec.Code)
	}
	// Logo 为图片地址时不经认证路径输出，页面直接引用
	if w := getLogin(a, s, lookLogoSuffix); w.Code != http.StatusNotFound {
		t.Fatalf("Logo 为地址时应 404: %d", w.Code)
	}
	if body := getLogin(a, s, "/login").Body.String(); !strings.Contains(body, `<img src="https://example.com/l.png"`) {
		t.Fatal("页面应直接引用 Logo 地址")
	}
}

// 配置文件中的无效外观不生效，按默认外观显示。
func TestLoginPageInvalidConfigIgnored(t *testing.T) {
	var h lookHolder
	if err := h.set(LoginPage{Style: "nope"}); err == nil || h.get() != nil {
		t.Fatal("无效外观应被忽略")
	}
	a := newAuthenticator("secret")
	_ = a.look.set(LoginPage{Style: "nope"})
	if body := getLogin(a, loginSite(t), "/login").Body.String(); !strings.Contains(body, "skin-default") {
		t.Fatal("应使用默认样式")
	}
}

// 每种内置样式（含站点单独设置）都能生成登录页与动态验证码对话框，背景装饰正确转义。
func TestLoginSkinsRender(t *testing.T) {
	for _, id := range append([]string{LoginStyleDefault}, webpage.LoginSkinIDs()...) {
		for _, state := range []string{"login", "totp", "error"} {
			o := lookOptions(nil, id, authPath, `<b>x.example.com`, "en", true)
			page := string(renderLoginSample(o, state, "en"))
			if !strings.Contains(page, "skin-"+id) || !strings.Contains(page, `name="username"`) {
				t.Fatalf("%s/%s 渲染不完整", id, state)
			}
			if state == "totp" && !strings.Contains(page, `id="otp"`) {
				t.Fatalf("%s 缺少验证码输入框", id)
			}
			if strings.Contains(page, "<b>x.example.com") {
				t.Fatalf("%s 域名未转义", id)
			}
		}
	}
}

func TestLoginPageAPI(t *testing.T) {
	m, do := newCopyModule(t)
	code, out := do("GET", "/api/webservice/loginpage", nil)
	if code != 200 || len(out["styles"].([]any)) != len(webpage.LoginSkinIDs()) {
		t.Fatalf("读取: %d %v", code, out)
	}
	if code, out = do("PUT", "/api/webservice/loginpage", LoginPage{Style: "aurora", Color: "#112233"}); code != 200 {
		t.Fatalf("保存: %d %v", code, out)
	}
	if m.cfg.LoginPage.Style != "aurora" || m.auth.look.get().Color != "#112233" {
		t.Fatal("保存后应写入配置并生效")
	}
	if code, _ = do("PUT", "/api/webservice/loginpage", LoginPage{Style: "x"}); code != 400 || m.cfg.LoginPage.Style != "aurora" {
		t.Fatal("无效设置应拒绝且不改动配置")
	}
	// 预览返回 JSON 数组，单独解码
	mux := http.NewServeMux()
	m.Routes(prefixRouter{mux, "/api/webservice"})
	b, _ := json.Marshal(LoginPreviewRequest{LoginPage: LoginPage{Background: tinyPNG, Logo: tinyPNG}, State: "totp"})
	post := func(b []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/webservice/loginpage/preview", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	rec := post(b)
	var list []LoginStylePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != len(webpage.LoginSkinIDs())+1 {
		t.Fatalf("预览: %d %s", rec.Code, rec.Body.String())
	}
	if p := list[0].HTML; list[0].ID != LoginStyleDefault || !strings.Contains(p, "Content-Security-Policy") || !strings.Contains(p, "data:image/png") || strings.Contains(p, "<script") || strings.Contains(p, "autofocus") {
		t.Fatalf("预览应带 CSP 并内联图片: %.300s", p)
	}
	b, _ = json.Marshal(LoginPreviewRequest{Styles: []string{"x"}})
	rec = post(b)
	if rec.Code != 400 {
		t.Fatalf("未知样式应拒绝: %d", rec.Code)
	}
}
