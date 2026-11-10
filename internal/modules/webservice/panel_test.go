package webservice

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/webpage"
)

// pngIcon 为 1x1 PNG 的 data URL。
const pngIcon = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func validPanel() Panel {
	return Panel{Name: "家里", Enable: true, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
}

// 链接地址：只允许白名单协议的完整地址，变量按示例值展开后检查，不能带密码、空白。
func TestPanelURLValidate(t *testing.T) {
	ok := []string{
		"https://nas.example.com", "http://192.168.1.2:5000/web", "https://[2001:db8::1]:8443",
		"ssh://root@192.168.1.2", "smb://nas.lan/share", "rdp://pc.lan", "mailto:admin@example.com",
		"https://{host}:8443", "http://{STUN_web_ADDR}/", "https://{1}.example.com", "https://{hostAndPort}/x", "https://{domainPart2}.example.com:{port}",
	}
	for _, u := range ok {
		if err := validatePanelTemplate(u); err != nil {
			t.Errorf("%s 应合法: %v", u, err)
		}
	}
	bad := []string{
		"javascript:alert(1)", "JavaScript:alert(1)", "data:text/html,<script>alert(1)</script>", "vbscript:x",
		"file:///etc/passwd", "nas.example.com", "/relative", "https://", "https://a.example.com/ x",
		"https://user:pass@a.example.com", "https://a.example.com/\x00", "{host}", "https://{2}.example.com",
		"https://a.example.com/" + strings.Repeat("a", maxPanelURLLen),
		// 取自访客请求的变量不允许：写在主机位置时可被构造的面板地址改指向任意网站
		"https://{Args}/", "https://{args}", "https://a.example.com{path}", "https://{pathPart1}.example.com", "https://a.example.com/?ip={ClientIP}", "https://a.example.com/{time}",
	}
	for _, u := range bad {
		if err := validatePanelTemplate(u); err == nil {
			t.Errorf("%q 应拒绝", u)
		}
	}
}

// 图标：data:image 只认常见图片格式与有效 base64，文字图标限长且不能带尖括号，图片地址只认 http(s)。
func TestPanelIconValidate(t *testing.T) {
	for _, icon := range []string{"", pngIcon, "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("<svg/>")),
		"https://cdn.example.com/icons/nas.svg", "📦", "👨‍👩‍👧", "N", "家"} {
		if _, err := validatePanelIcon(icon); err != nil {
			t.Errorf("%q 应合法: %v", icon, err)
		}
	}
	big := "data:image/png;base64," + strings.Repeat("A", maxPanelIconLen)
	for _, icon := range []string{"data:text/html;base64,PHNjcmlwdD4=", "data:image/png,rawdata", "data:image/png;base64,***",
		big, "javascript:alert(1)x", "<b>x</b>", "一二三四五六七八九", "https://u:p@cdn.example.com/x.png", "https:// x"} {
		if _, err := validatePanelIcon(icon); err == nil {
			t.Errorf("%q 应拒绝", icon)
		}
	}
}

func TestPanelValidate(t *testing.T) {
	p := validPanel()
	p.Name = "  家里 "
	p.Links[0].Color = "#ABCDEF"
	p.Links[0].Group = " 存储 "
	p.LANCIDRs = []string{"100.64.0.0/10", " ", "100.64.0.0/10"}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	if p.Name != "家里" || p.Links[0].Color != "#abcdef" || p.Links[0].Group != "存储" || len(p.LANCIDRs) != 1 {
		t.Fatalf("规范化: %+v", p)
	}
	cases := map[string]func(p *Panel){
		"缺少名称":   func(p *Panel) { p.Name = "" },
		"主题无效":   func(p *Panel) { p.Theme = "blue" },
		"皮肤无效":   func(p *Panel) { p.Skin = "unknown" },
		"地址段无效":  func(p *Panel) { p.LANCIDRs = []string{"x"} },
		"链接缺名称":  func(p *Panel) { p.Links[0].Name = "" },
		"链接缺地址":  func(p *Panel) { p.Links[0].URL = "" },
		"内网地址无效": func(p *Panel) { p.Links[0].LANURL = "javascript:1" },
		"颜色无效":   func(p *Panel) { p.Links[0].Color = "red" },
		"图标无效":   func(p *Panel) { p.Links[0].Icon = "data:text/html;base64,AA==" },
		"页面图标无效": func(p *Panel) { p.Icon = "<i>" },
		"名称过长":   func(p *Panel) { p.Name = strings.Repeat("长", maxPanelNameLen+1) },
		"控制字符":   func(p *Panel) { p.Links[0].Description = "a\x07b" },
		"链接过多": func(p *Panel) {
			p.Links = make([]PanelLink, maxPanelLinks+1)
			for i := range p.Links {
				p.Links[i] = PanelLink{Name: "x", URL: "https://x.example.com"}
			}
		},
		"图标合计过大": func(p *Panel) {
			icon := "data:image/png;base64," + strings.Repeat("A", maxPanelIconLen-64)
			p.Links = nil
			for range maxPanelIconsTotal/len(icon) + 1 {
				p.Links = append(p.Links, PanelLink{Name: "x", URL: "https://x.example.com", Icon: icon})
			}
		},
	}
	for name, mut := range cases {
		p := validPanel()
		mut(&p)
		if err := p.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
	// 导航面板站点必须选择面板
	s := Site{Enable: true, Domains: []string{"nav.test"}, Type: TypePanel, Panel: "  "}
	if err := s.validate(false); err == nil {
		t.Fatal("未选择面板应拒绝")
	}
	s.Panel = " p1 "
	if err := s.validate(false); err != nil || s.Panel != "p1" {
		t.Fatalf("选择面板: %v %q", err, s.Panel)
	}
}

// 内网判断：回环、私有、链路本地与额外地址段为内网，其余为外网；手动选择优先。
func TestPanelNetwork(t *testing.T) {
	p := validPanel()
	p.LANCIDRs = []string{"100.64.0.0/10"}
	s := newPanelStore()
	p.Key = "p1"
	s.set([]Panel{p})
	cp, _ := s.get("p1")
	for addr, want := range map[string]bool{
		"127.0.0.1": true, "192.168.1.9": true, "10.1.2.3": true, "172.20.0.1": true, "fd00::1": true, "fe80::1": true,
		"::ffff:192.168.1.9": true, "100.100.1.1": true, "::1": true,
		"203.0.113.9": false, "2001:db8::1": false, "172.32.0.1": false,
	} {
		if got := cp.isLAN(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s 内网=%v，期望 %v", addr, got, want)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	if lan, forced := cp.network(r); lan || forced {
		t.Fatal("公网访客应为外网")
	}
	r.AddCookie(&http.Cookie{Name: panelNetCookie, Value: "lan"})
	if lan, forced := cp.network(r); !lan || !forced {
		t.Fatal("手动选择内网应优先")
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.9:5000"
	r.AddCookie(&http.Cookie{Name: panelNetCookie, Value: "bogus"})
	if lan, forced := cp.network(r); !lan || forced {
		t.Fatal("无效的选择应忽略")
	}
	// 配置文件中手工写入的无效面板不进入运行时面板库
	bad := validPanel()
	bad.Key = "bad"
	bad.Links[0].URL = "javascript:alert(1)"
	s.set([]Panel{bad})
	if _, ok := s.get("bad"); ok {
		t.Fatal("无效面板不应载入")
	}
}

// panelHandler 构造只含导航面板站点的处理器。
func panelHandler(t *testing.T, panels []Panel) (*siteHandler, *authenticator) {
	t.Helper()
	auth := newAuthenticator("secret")
	auth.panels.set(panels)
	s := Site{Key: "s1", Enable: true, Domains: []string{"*.nav.test"}, Type: TypePanel, Panel: "p1", ResponseHeaders: []string{"X-Extra: 1"}}
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	h, err := newSiteHandler(s, testLogs().Logger("t"), auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h, auth
}

func panelGet(h http.Handler, path, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://home.nav.test"+path, nil)
	r.Host = "home.nav.test"
	r.RemoteAddr = remote
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Accept-Language", "zh-CN")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPanelServing(t *testing.T) {
	stunaddr.Set("rule-panel", "web", netip.MustParseAddrPort("203.0.113.7:51234"))
	defer stunaddr.Delete("rule-panel")
	p := Panel{Key: "p1", Name: "家里", Enable: true, Title: "我的导航", Subtitle: "常用服务", Theme: "dark", Links: []PanelLink{
		{Name: "NAS", Group: "存储", URL: "https://nas.example.com", LANURL: "http://192.168.1.2:5000", Description: "群晖"},
		{Name: "<script>alert(1)</script>", Group: "存储", URL: "https://{host}:8443/x?a=1&b=2", Icon: "📦", Color: "#123456"},
		{Name: "穿透", URL: "http://{STUN_web_ADDR}/", Icon: pngIcon, SameTab: true},
		{Name: "离线", URL: "http://{STUN_none_ADDR}/"},
		{Name: "通配", URL: "https://{1}.example.com"},
		{Name: "隐藏的", URL: "https://hidden.example.com", Hide: true},
		{Name: "下载", Group: "工具", URL: "https://dl.example.com"},
	}}
	h, auth := panelHandler(t, []Panel{p})

	// 内网访客：用内网地址，展开 {host}、{1}、STUN 变量，变量暂无值的链接不可点击
	w := panelGet(h, "/", "192.168.1.9:4000")
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Extra") != "1" {
		t.Fatalf("响应头: %d %v", w.Code, w.Header())
	}
	for _, want := range []string{`href="http://192.168.1.2:5000"`, `href="https://home.nav.test:8443/x?a=1&amp;b=2"`, `href="http://203.0.113.7:51234/"`,
		`href="https://home.example.com"`, "我的导航", "常用服务", `data-theme="dark"`, "&lt;script&gt;alert(1)&lt;/script&gt;", "📦",
		`src="` + strings.ReplaceAll(pngIcon, "+", "&#43;") + `"`, ">存储<", ">工具<", "暂不可用", `class="on"`, "?net=wan"} {
		if !strings.Contains(body, want) {
			t.Errorf("页面应包含 %s", want)
		}
	}
	for _, unwanted := range []string{"<script>alert(1)", "hidden.example.com", "https://nas.example.com", "{STUN", "?net=auto"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("页面不应包含 %s", unwanted)
		}
	}
	// 分组按首次出现排列；在当前页打开的链接不带 target
	if strings.Index(body, ">存储<") > strings.Index(body, ">工具<") {
		t.Error("分组顺序应为首次出现的顺序")
	}
	if !regexp.MustCompile(`class="hit" href="http://203.0.113.7:51234/" title=`).MatchString(body) {
		t.Error("SameTab 链接不应在新标签页打开")
	}
	// CSP 带 nonce，与页面中的样式、脚本一致；不允许内联脚本
	csp := w.Header().Get("Content-Security-Policy")
	m := regexp.MustCompile(`script-src 'nonce-([A-Za-z0-9+/]+)'`).FindStringSubmatch(csp)
	if m == nil || !strings.Contains(body, `<script nonce="`+m[1]+`">`) || !strings.Contains(body, `<style nonce="`+m[1]+`">`) ||
		strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("CSP: %s", csp)
	}
	if w2 := panelGet(h, "/", "192.168.1.9:4000"); strings.Contains(w2.Header().Get("Content-Security-Policy"), m[1]) {
		t.Fatal("每次响应的 nonce 应不同")
	}

	// 外网访客：用外网地址
	if body = panelGet(h, "/", "203.0.113.9:4000").Body.String(); !strings.Contains(body, `href="https://nas.example.com"`) || strings.Contains(body, "192.168.1.2") {
		t.Fatalf("外网访客应使用外网地址: %s", body)
	}
	// 手动切换：设置 Cookie 后跳回原路径；带 Cookie 时显示“自动识别”
	w = panelGet(h, "/?net=wan", "192.168.1.9:4000")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("切换: %d %v", w.Code, w.Header())
	}
	ck := w.Result().Cookies()
	if len(ck) != 1 || ck[0].Name != panelNetCookie || ck[0].Value != "wan" || !ck[0].HttpOnly || ck[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("Cookie: %+v", ck)
	}
	if body = panelGet(h, "/", "192.168.1.9:4000", ck[0]).Body.String(); !strings.Contains(body, `href="https://nas.example.com"`) || !strings.Contains(body, "?net=auto") {
		t.Fatal("选择外网后应使用外网地址并可恢复自动")
	}
	w = panelGet(h, "/index.html?net=auto", "192.168.1.9:4000")
	if ck = w.Result().Cookies(); w.Header().Get("Location") != "/index.html" || len(ck) != 1 || ck[0].MaxAge >= 0 {
		t.Fatalf("恢复自动应清除 Cookie: %v %+v", w.Header(), ck)
	}

	// 其他路径 404，非 GET 405，HEAD 只有响应头
	if w = panelGet(h, "/admin", "192.168.1.9:4000"); w.Code != 404 {
		t.Fatalf("其他路径: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "http://home.nav.test/", nil)
	r.RemoteAddr = "192.168.1.9:4000"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST: %d", w.Code)
	}
	r = httptest.NewRequest("HEAD", "http://home.nav.test/", nil)
	r.RemoteAddr = "192.168.1.9:4000"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatalf("HEAD: %d %d", w.Code, w.Body.Len())
	}

	// 面板停用或被删除：404，修改面板库后立即生效
	p.Enable = false
	auth.panels.set([]Panel{p})
	if w = panelGet(h, "/", "192.168.1.9:4000"); w.Code != 404 || !strings.Contains(w.Body.String(), "导航面板不可用") {
		t.Fatalf("停用: %d %s", w.Code, w.Body.String())
	}
	auth.panels.set(nil)
	if w = panelGet(h, "/", "192.168.1.9:4000"); w.Code != 404 {
		t.Fatalf("删除: %d", w.Code)
	}
}

// 内置图标：写法 builtin:编号，编号不存在时拒绝；页面中画在底色方块上，底色较浅时图形为深色。
func TestPanelBuiltinIcons(t *testing.T) {
	if _, err := validatePanelIcon("builtin:jellyfin"); err != nil {
		t.Fatal(err)
	}
	if _, err := validatePanelIcon("builtin:nope"); err == nil {
		t.Fatal("不存在的内置图标应拒绝")
	}
	if _, err := validatePanelIcon("builtin:"); err == nil {
		t.Fatal("空编号应拒绝")
	}
	ic := panelIcon("builtin:jellyfin", "")
	jf, _ := webpage.FindBuiltinIcon("jellyfin")
	if !ic.Tile || ic.Color != jf.Color || ic.Image != webpage.BuiltinIconURL(jf, false) {
		t.Fatalf("默认颜色: %+v", ic)
	}
	if ic = panelIcon("builtin:jellyfin", "#ffee00"); ic.Color != "#ffee00" || ic.Image != webpage.BuiltinIconURL(jf, true) {
		t.Fatal("浅色底色应使用深色图形")
	}
	h, _ := panelHandler(t, []Panel{{Key: "p1", Name: "家", Enable: true, Icon: "builtin:gately", Links: []PanelLink{{Name: "JF", URL: "https://jf.example.com", Icon: "builtin:jellyfin"}}}})
	body := panelGet(h, "/", "192.168.1.9:4000").Body.String()
	if !strings.Contains(body, `class="ic tile c`) || !strings.Contains(body, `class="logo tile c`) || !strings.Contains(body, "--c:"+jf.Color) {
		t.Fatalf("页面: %s", body)
	}
	// 每个内置图标都能生成有效的 SVG
	for _, x := range webpage.BuiltinIcons() {
		if x.ID == "" || x.Name == "" || !panelColorRe.MatchString(x.Color) || (x.Kind != "brand" && x.Kind != "line") || x.Body == "" {
			t.Fatalf("图标数据不完整: %+v", x)
		}
	}
}

// 其他访问方式：校验名称与地址，页面中显示为小链接；变量暂无值时显示为不可用，主地址不可用时仍可点。
func TestPanelAlts(t *testing.T) {
	p := validPanel()
	p.Links[0].Alts = []PanelAlt{{Name: " IPv6 ", URL: " https://[2001:db8::2]:8443 "}, {Name: "STUN", URL: "http://{STUN_web_ADDR}/"}}
	if err := p.validate(); err != nil || p.Links[0].Alts[0].Name != "IPv6" || p.Links[0].Alts[0].URL != "https://[2001:db8::2]:8443" {
		t.Fatalf("合法: %v %+v", err, p.Links[0].Alts)
	}
	for name, alts := range map[string][]PanelAlt{
		"缺名称":  {{URL: "https://a.example.com"}},
		"缺地址":  {{Name: "x"}},
		"危险地址": {{Name: "x", URL: "javascript:alert(1)"}},
		"请求变量": {{Name: "x", URL: "https://{args}"}},
		"名称过长": {{Name: strings.Repeat("长", maxPanelAltName+1), URL: "https://a.example.com"}},
		"数量过多": make([]PanelAlt, maxPanelAlts+1),
	} {
		p := validPanel()
		for i := range alts {
			if alts[i].Name == "" && name == "数量过多" {
				alts[i] = PanelAlt{Name: "x", URL: "https://a.example.com"}
			}
		}
		p.Links[0].Alts = alts
		if err := p.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
	stunaddr.Set("rule-alt", "frpweb", netip.MustParseAddrPort("203.0.113.8:40001"))
	defer stunaddr.Delete("rule-alt")
	h, _ := panelHandler(t, []Panel{{Key: "p1", Name: "家", Enable: true, Links: []PanelLink{
		{Name: "NAS", URL: "http://{STUN_none_ADDR}/", Alts: []PanelAlt{{Name: "IPv6", URL: "https://[2001:db8::2]:8443"}, {Name: "STUN", URL: "http://{STUN_frpweb_ADDR}/"}, {Name: "<b>坏</b>", URL: "http://{STUN_none_ADDR}/"}}},
	}}})
	body := panelGet(h, "/", "203.0.113.9:4000").Body.String()
	for _, want := range []string{`class="alt" href="https://[2001:db8::2]:8443"`, `href="http://203.0.113.8:40001/"`, `<span class="alt off"`, "&lt;b&gt;坏&lt;/b&gt;", `class="card off"`} {
		if !strings.Contains(body, want) {
			t.Errorf("页面应包含 %s", want)
		}
	}
	if strings.Contains(body, `class="hit"`) {
		t.Error("主地址不可用时不应有整卡链接")
	}
}

// 没有任何链接配置内网地址时不显示内外网切换；没有可见链接时显示空提示。
func TestPanelServingNoSwitch(t *testing.T) {
	h, _ := panelHandler(t, []Panel{{Key: "p1", Name: "空", Enable: true, Links: []PanelLink{{Name: "A", URL: "https://a.example.com", Hide: true}}}})
	body := panelGet(h, "/", "192.168.1.9:4000").Body.String()
	if strings.Contains(body, "?net=") || !strings.Contains(body, "还没有添加链接") || !strings.Contains(body, "<title>空</title>") {
		t.Fatalf("页面: %s", body)
	}
	h, _ = panelHandler(t, []Panel{{Key: "p1", Name: "外", Enable: true, Links: []PanelLink{{Name: "A", URL: "https://a.example.com"}}}})
	if body = panelGet(h, "/", "192.168.1.9:4000").Body.String(); strings.Contains(body, "?net=") || !strings.Contains(body, `href="https://a.example.com"`) {
		t.Fatalf("只有外网地址时内网访客也用外网地址: %s", body)
	}
}

// 页面文案按访客语言翻译。
func TestPanelServingLocalized(t *testing.T) {
	h, _ := panelHandler(t, []Panel{{Key: "p1", Name: "Home", Enable: true, Links: []PanelLink{{Name: "A", URL: "https://a.example.com", LANURL: "http://10.0.0.2"}}}})
	r := httptest.NewRequest("GET", "http://home.nav.test/", nil)
	r.RemoteAddr = "10.0.0.9:4000"
	r.Header.Set("Accept-Language", "en")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if body := w.Body.String(); !strings.Contains(body, `placeholder="Search"`) || !strings.Contains(body, `lang="en"`) {
		t.Fatalf("英文页面: %s", body)
	}
}

// 通过 HTTP 访问导航面板站点：站点的 IP 访问控制、认证照常生效。
func TestPanelSiteAccessControl(t *testing.T) {
	l := Listener{Sites: []Site{
		{Enable: true, Domains: []string{"nav.test"}, Type: TypePanel, Panel: "p1"},
		{Enable: true, Domains: []string{"deny.test"}, Type: TypePanel, Panel: "p1", IPMode: IPModeWhitelist, IPList: []string{"10.0.0.0/8"}},
		{Enable: true, Domains: []string{"auth.test"}, Type: TypePanel, Panel: "p1", WebAuth: true, Users: []string{"u:p"}},
	}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.panels.set([]Panel{{Key: "p1", Name: "家里", Enable: true, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}})
	if resp, body := get(t, http.DefaultClient, addr, "nav.test", "/", htmlAccept); resp.StatusCode != 200 || !strings.Contains(body, "nas.example.com") {
		t.Fatalf("面板: %d %s", resp.StatusCode, body)
	}
	if resp, body := get(t, http.DefaultClient, addr, "deny.test", "/", htmlAccept); resp.StatusCode != 403 || strings.Contains(body, "nas.example.com") {
		t.Fatalf("白名单外应拒绝: %d", resp.StatusCode)
	}
	if resp, body := get(t, noRedirect(), addr, "auth.test", "/", htmlAccept); resp.StatusCode == 200 || strings.Contains(body, "nas.example.com") {
		t.Fatalf("未登录不应看到面板: %d", resp.StatusCode)
	}
}

func TestPanelsAPI(t *testing.T) {
	m, do := newCopyModule(t)
	code, out := do("POST", "/api/webservice/panels", validPanel())
	if code != 200 {
		t.Fatalf("新增: %d %v", code, out)
	}
	key := out["key"].(string)
	if code, out = do("POST", "/api/webservice/panels", validPanel()); code != 400 || !strings.Contains(out["error"].(string), "已有名为") {
		t.Fatalf("重名应拒绝: %d %v", code, out)
	}
	bad := validPanel()
	bad.Name = "坏"
	bad.Links[0].URL = "javascript:alert(1)"
	if code, _ = do("POST", "/api/webservice/panels", bad); code != 400 {
		t.Fatalf("危险地址应拒绝: %d", code)
	}
	if _, ok := m.auth.panels.get(key); !ok {
		t.Fatal("新增后运行时面板库应同步")
	}
	// 站点引用不存在的面板时拒绝保存
	l := Listener{Name: "主", Port: 18091, Sites: []Site{{Enable: true, Domains: []string{"nav.test"}, Type: TypePanel, Panel: "missing"}}}
	if code, out = do("POST", "/api/webservice/listeners", l); code != 400 || !strings.Contains(out["error"].(string), "导航面板不存在") {
		t.Fatalf("引用不存在的面板应拒绝: %d %v", code, out)
	}
	l.Sites[0].Panel = key
	l.Default = Site{Enable: true, Type: TypePanel, Panel: "missing"}
	if code, _ = do("POST", "/api/webservice/listeners", l); code != 400 {
		t.Fatalf("默认站点引用不存在的面板应拒绝: %d", code)
	}
	l.Default.Panel = key
	if code, out = do("POST", "/api/webservice/listeners", l); code != 200 {
		t.Fatalf("引用存在的面板: %d %v", code, out)
	}
	// 内置图标列表
	rec0 := httptest.NewRecorder()
	mux0 := http.NewServeMux()
	m.Routes(prefixRouter{mux0, "/api/webservice"})
	mux0.ServeHTTP(rec0, httptest.NewRequest("GET", "/api/webservice/panels/icons", nil))
	if rec0.Code != 200 || !strings.Contains(rec0.Body.String(), `"id":"jellyfin"`) || !strings.Contains(rec0.Body.String(), `"srcDark":"data:image/svg+xml;base64,`) {
		t.Fatalf("图标列表: %d", rec0.Code)
	}
	// 列表带引用方；被引用时拒绝删除
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	m.Routes(prefixRouter{mux, "/api/webservice"})
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/webservice/panels", nil))
	if !strings.Contains(rec.Body.String(), `"usedBy":["主 / nav.test","主 / @default"]`) {
		t.Fatalf("列表: %s", rec.Body.String())
	}
	if code, out = do("DELETE", "/api/webservice/panels/"+key, nil); code != 409 || !strings.Contains(out["error"].(string), "主 / nav.test") || !strings.Contains(out["error"].(string), "默认站点") {
		t.Fatalf("被引用时应拒绝删除: %d %v", code, out)
	}
	// 修改后运行时面板库同步
	p := validPanel()
	p.Title = "新标题"
	if code, _ = do("PUT", "/api/webservice/panels/"+key, p); code != 200 {
		t.Fatalf("修改: %d", code)
	}
	if cp, _ := m.auth.panels.get(key); cp == nil || cp.Title != "新标题" {
		t.Fatal("修改后运行时面板库应同步")
	}
	m.mu.Lock()
	m.cfg.Listeners = nil
	m.mu.Unlock()
	if code, _ = do("DELETE", "/api/webservice/panels/"+key, nil); code != 200 || len(m.cfg.Panels) != 0 {
		t.Fatalf("删除: %d", code)
	}
	if _, ok := m.auth.panels.get(key); ok {
		t.Fatal("删除后运行时面板库应同步")
	}
	// 数量上限
	for i := range maxPanels {
		p := validPanel()
		p.Name = "p" + strings.Repeat("x", i)
		if code, out = do("POST", "/api/webservice/panels", p); code != 200 {
			t.Fatalf("第 %d 个: %d %v", i, code, out)
		}
	}
	p = validPanel()
	p.Name = "over"
	if code, _ = do("POST", "/api/webservice/panels", p); code != 400 {
		t.Fatalf("超过上限应拒绝: %d", code)
	}
}

// 预览：按编辑中的内容生成，不保存；不含脚本与搜索框，用 meta 标签禁止脚本。
func TestPanelPreview(t *testing.T) {
	m, do := newCopyModule(t)
	p := validPanel()
	p.Links = append(p.Links, PanelLink{Name: "内网", URL: "https://a.example.com", LANURL: "http://10.0.0.2"})
	code, out := do("POST", "/api/webservice/panels/preview", PanelPreviewRequest{Panel: p, LAN: true})
	if code != 200 {
		t.Fatalf("预览: %d %v", code, out)
	}
	html := out["html"].(string)
	if strings.Contains(html, "<script") || strings.Contains(html, `id="q"`) || !strings.Contains(html, `http-equiv="Content-Security-Policy" content="default-src &#39;none&#39;;`) ||
		strings.Contains(html, "script-src") || !strings.Contains(html, `href="http://10.0.0.2"`) {
		t.Fatalf("预览页面: %s", html)
	}
	if len(m.cfg.Panels) != 0 {
		t.Fatal("预览不应保存")
	}
	p.Links[0].URL = "javascript:1"
	if code, _ = do("POST", "/api/webservice/panels/preview", PanelPreviewRequest{Panel: p}); code != 400 {
		t.Fatalf("无效面板应拒绝预览: %d", code)
	}
}

// 载入面板库时深拷贝配置：validate 的规范化不写回配置，也不与正在服务的面板共用底层数组（go test -race 检查）。
func TestPanelStoreSetCopies(t *testing.T) {
	p := validPanel()
	p.Key = "p1"
	p.LANCIDRs = []string{" 100.64.0.0/10 "}
	p.Links[0].Alts = []PanelAlt{{Name: " IPv6 ", URL: " https://[2001:db8::2]:8443 "}}
	list := []Panel{p}
	s := newPanelStore()
	s.set(list)
	if a := list[0].Links[0].Alts[0]; a.Name != " IPv6 " || a.URL != " https://[2001:db8::2]:8443 " || list[0].LANCIDRs[0] != " 100.64.0.0/10 " {
		t.Fatalf("set 不应修改传入的配置: %+v %v", a, list[0].LANCIDRs)
	}
	cp, _ := s.get("p1")
	if cp.Links[0].Alts[0].Name != "IPv6" {
		t.Fatalf("运行时面板应已规范化: %+v", cp.Links[0].Alts)
	}
	// 面板库反复替换的同时访客在读取页面内容
	r := httptest.NewRequest("GET", "http://home.nav.test/", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			s.set(list)
		}
	}()
	for range 200 {
		if cp, ok := s.get("p1"); ok {
			if v := cp.view(redirectRequest{r: r}, false, false, false); len(v.Groups) != 1 || v.Groups[0].Links[0].Alts[0].Name != "IPv6" {
				t.Fatalf("页面内容: %+v", v)
			}
		}
	}
	<-done
	if c := p.clone(); &c.Links[0] == &p.Links[0] || &c.Links[0].Alts[0] == &p.Links[0].Alts[0] || &c.LANCIDRs[0] == &p.LANCIDRs[0] {
		t.Fatal("clone 应复制链接、访问方式与地址段")
	}
}

// 载入时内置图标不存在只清空该图标，面板照常提供；确实无效的面板被忽略并记录警告。
func TestPanelStoreLoadLenient(t *testing.T) {
	var warns []string
	s := newPanelStore()
	s.warn = func(format string, args ...any) {
		warns = append(warns, fmt.Sprintf(format, args...))
	}
	p := validPanel()
	p.Key = "p1"
	p.Icon = "builtin:no-such-icon"
	p.Links[0].Icon = " builtin:no-such-icon "
	p.Links = append(p.Links, PanelLink{Name: "JF", URL: "https://jf.example.com", Icon: "builtin:jellyfin"})
	bad := validPanel()
	bad.Key = "p2"
	bad.Name = "坏面板"
	bad.Links[0].URL = "javascript:alert(1)"
	s.set([]Panel{p, bad})
	cp, ok := s.get("p1")
	if !ok || cp.Icon != "" || cp.Links[0].Icon != "" || cp.Links[1].Icon != "builtin:jellyfin" {
		t.Fatalf("未知内置图标应降级为空图标: %v %+v", ok, cp)
	}
	if _, ok := s.get("p2"); ok {
		t.Fatal("无效面板不应载入")
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "坏面板") {
		t.Fatalf("应记录一条警告: %q", warns)
	}
	// 保存时仍严格校验
	if err := p.validate(); err == nil {
		t.Fatal("保存时不存在的内置图标应拒绝")
	}
}

// 用 IPv6 字面地址访问面板时 {host} 带方括号，链接可用。
func TestPanelIPv6Host(t *testing.T) {
	h, _ := panelHandler(t, []Panel{{Key: "p1", Name: "家", Enable: true, Links: []PanelLink{
		{Name: "NAS", URL: "https://{host}:5001/"},
		{Name: "原样", URL: "https://{hostAndPort}/x"},
	}}})
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "[2001:db8::1]:8443"
	r.RemoteAddr = "[2001:db8:78::9]:4000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	for _, want := range []string{`href="https://[2001:db8::1]:5001/"`, `href="https://[2001:db8::1]:8443/x"`} {
		if !strings.Contains(body, want) {
			t.Errorf("页面应包含 %s: %s", want, body)
		}
	}
	if strings.Contains(body, "暂不可用") {
		t.Error("IPv6 访问时链接不应显示为暂不可用")
	}
}

// {1} 没有值（非通配站点、预览）时链接暂不可用；展开后主机名为空标签或端口无效的地址也不可用。
func TestPanelEmptyWildcard(t *testing.T) {
	p := Panel{Name: "家", Links: []PanelLink{
		{Name: "通配", URL: "https://{1}.nas.example.com/"},
		{Name: "前缀", URL: "https://n{1}.example.com/"},
		{Name: "越界", URL: "https://{domainPart9}.example.com/"},
		{Name: "普通", URL: "https://a.example.com/"},
	}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	cp := &compiledPanel{Panel: p}
	r := httptest.NewRequest("GET", "http://nas.test/", nil)
	links := cp.view(redirectRequest{r: r}, false, false, true).Groups[0].Links
	for i, want := range []string{"", "", "", "https://a.example.com/"} {
		if links[i].Href != want {
			t.Errorf("%s: href=%q，期望 %q", links[i].Name, links[i].Href, want)
		}
	}
	links = cp.view(redirectRequest{r: r, wildcard: "home"}, false, false, false).Groups[0].Links
	if links[0].Href != "https://home.nas.example.com/" || links[1].Href != "https://nhome.example.com/" {
		t.Fatalf("{1} 有值时应可用: %+v", links)
	}
	for _, u := range []string{"https://.nas.example.com/", "https://a..b/", "https://a.example.com:0/", "https://a.example.com:65536/",
		"https://a.example.com:/", "https://a.example.com:+1/", "https://2001:db8::1:8443/", "https://[zz]/", "https://[::1]x/", "https://:443/"} {
		if err := validatePanelURL(u); err == nil {
			t.Errorf("%q 应拒绝", u)
		}
	}
	for _, u := range []string{"https://a.example.com./", "https://[::1]:65535/", "https://[fe80::1%25en0]/", "ssh://root@nas.lan:22", "smb://nas/share", "http://localhost:1"} {
		if err := validatePanelURL(u); err != nil {
			t.Errorf("%q 应合法: %v", u, err)
		}
	}
}
