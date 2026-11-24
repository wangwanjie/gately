package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func authOnlyPanel() Panel {
	return Panel{Key: "p1", Name: "家里", Enable: true, Links: []PanelLink{
		{Name: "公开站", URL: "https://public.example.com"},
		{Name: "内部站", Group: "内部", URL: "https://secret.example.com", Description: "仅登录可见", AuthOnly: true},
		{Name: "被隐藏的内部站", URL: "https://hidden.example.com", AuthOnly: true, Hide: true},
	}}
}

// 视图：未登录时“登录后可见”的链接不进入页面（名称、地址、描述都没有），只统计个数；已登录时显示并带锁标记；
// 手动隐藏的链接不计入。
func TestPanelViewAuthOnly(t *testing.T) {
	cp := &compiledPanel{Panel: authOnlyPanel()}
	r := httptest.NewRequest("GET", "http://home.nav.test/", nil)
	q := redirectRequest{r: r}

	v := cp.view(q, false, false, false, false)
	if v.Hidden != 1 || len(v.Groups) != 1 || len(v.Groups[0].Links) != 1 || v.Groups[0].Links[0].Name != "公开站" || v.Groups[0].Links[0].Locked {
		t.Fatalf("未登录: %+v", v)
	}
	v = cp.view(q, false, false, false, true)
	if v.Hidden != 0 || len(v.Groups) != 2 {
		t.Fatalf("已登录: %+v", v)
	}
	if l := v.Groups[1].Links[0]; l.Name != "内部站" || !l.Locked || l.Href != "https://secret.example.com" {
		t.Fatalf("已登录应看到带锁标记的内部站: %+v", l)
	}
	// 全部链接都要求登录时，未登录访客看到空页面与提示，没有空分组
	only := authOnlyPanel()
	only.Links = only.Links[1:2]
	v = (&compiledPanel{Panel: only}).view(q, false, false, false, false)
	if len(v.Groups) != 0 || v.Hidden != 1 {
		t.Fatalf("全部隐藏: %+v", v)
	}
}

func TestPanelAuthOnlyHTTP(t *testing.T) {
	share := []ShareLink{{Key: "k1", Name: "临时", Token: strings.Repeat("t", 24)}}
	l := Listener{Sites: []Site{
		// 面板对所有人开放，登录后显示更多链接
		{Enable: true, Key: "opt", Domains: []string{"opt.test"}, Type: TypePanel, Panel: "p1", WebAuth: true, Users: []string{"alice:pw"}, AuthOptional: true},
		// 面板整体需要登录
		{Enable: true, Key: "must", Domains: []string{"must.test"}, Type: TypePanel, Panel: "p1", WebAuth: true, Users: []string{"alice:pw"}, ShareLinks: share},
		// 没有认证：登录后可见的链接始终不显示
		{Enable: true, Key: "open", Domains: []string{"open.test"}, Type: TypePanel, Panel: "p1"},
		// 基本认证的可选模式无法识别访客身份：同样不显示
		{Enable: true, Key: "basic", Domains: []string{"basic.test"}, Type: TypePanel, Panel: "p1", BasicAuth: true, Users: []string{"alice:pw"}, AuthOptional: true},
		// 基本认证整体需要登录
		{Enable: true, Key: "bmust", Domains: []string{"bmust.test"}, Type: TypePanel, Panel: "p1", BasicAuth: true, Users: []string{"alice:pw"}},
	}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.panels.set([]Panel{authOnlyPanel()})
	loginURL := authPath + "/login?next=%2F"
	logoutURL := authPath + "/logout"

	t.Run("可选认证-未登录", func(t *testing.T) {
		resp, body := get(t, noRedirect(), addr, "opt.test", "/", htmlAccept)
		if resp.StatusCode != 200 || !strings.Contains(body, "public.example.com") {
			t.Fatalf("应看到公开链接: %d", resp.StatusCode)
		}
		for _, leak := range []string{"secret.example.com", "内部站", "仅登录可见", "hidden.example.com", "内部</h2>"} {
			if strings.Contains(body, leak) {
				t.Fatalf("未登录页面泄露了 %q", leak)
			}
		}
		if !strings.Contains(body, `href="`+loginURL+`"`) || strings.Contains(body, `id="user-btn"`) || !strings.Contains(body, `id="set-btn"`) || strings.Contains(body, `class="more"`) || strings.Contains(body, logoutURL) {
			t.Fatalf("应有登录入口、没有退出入口；页头已有登录按钮，链接下方不再提示")
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("页面随登录状态变化，不能缓存: %q", cc)
		}
	})
	t.Run("可选认证-已登录", func(t *testing.T) {
		cookie := loginCookie(t, addr, "opt.test", "alice", "pw")
		resp, body := get(t, noRedirect(), addr, "opt.test", "/", map[string]string{"Accept": "text/html", "Accept-Language": "zh-CN", "Cookie": cookie})
		if resp.StatusCode != 200 || !strings.Contains(body, "public.example.com") || !strings.Contains(body, "secret.example.com") {
			t.Fatalf("登录后应看到全部链接: %d", resp.StatusCode)
		}
		if strings.Contains(body, "hidden.example.com") {
			t.Fatal("手动隐藏的链接不应显示")
		}
		if !strings.Contains(body, `class="lock"`) || !strings.Contains(body, `"logout":"`+logoutURL+`"`) || !strings.Contains(body, `id="user-btn"`) || !strings.Contains(body, `<span class="av">A</span>`) || !strings.Contains(body, `title="alice"`) || strings.Contains(body, loginURL) || strings.Contains(body, `class="more"`) {
			t.Fatalf("应有锁标记与退出入口，没有登录入口与提示: lock=%v logout=%v user=%v av=%v title=%v login=%v more=%v", strings.Contains(body, `class="lock"`), strings.Contains(body, `"logout":"`+logoutURL+`"`), strings.Contains(body, `id="user-btn"`), strings.Contains(body, `<span class="av">A</span>`), strings.Contains(body, `title="alice"`), strings.Contains(body, loginURL), strings.Contains(body, `class="more"`))
		}
	})
	t.Run("可选认证-伪造与别站的会话", func(t *testing.T) {
		for _, c := range []string{"gately_site_x=forged", loginCookie(t, addr, "must.test", "alice", "pw")} {
			_, body := get(t, noRedirect(), addr, "opt.test", "/", map[string]string{"Accept": "text/html", "Cookie": c})
			if strings.Contains(body, "secret.example.com") {
				t.Fatalf("会话 %q 不应看到内部链接", c)
			}
		}
	})
	t.Run("整体认证", func(t *testing.T) {
		resp, body := get(t, noRedirect(), addr, "must.test", "/", map[string]string{"Accept": "text/html", "User-Agent": "Mozilla/5.0"})
		if resp.StatusCode != 302 || strings.Contains(body, "secret.example.com") {
			t.Fatalf("未登录应跳转登录页: %d", resp.StatusCode)
		}
		cookie := loginCookie(t, addr, "must.test", "alice", "pw")
		resp, body = get(t, noRedirect(), addr, "must.test", "/", map[string]string{"Accept": "text/html", "Cookie": cookie})
		if resp.StatusCode != 200 || !strings.Contains(body, "secret.example.com") || strings.Contains(body, loginURL) {
			t.Fatalf("登录后应看到内部链接且没有登录入口: %d", resp.StatusCode)
		}
	})
	t.Run("分享链接访客不算已登录", func(t *testing.T) {
		resp, _ := get(t, noRedirect(), addr, "must.test", "/?_gs="+share[0].Token, htmlAccept)
		var c string
		for _, ck := range resp.Cookies() {
			c = ck.Name + "=" + ck.Value
		}
		if c == "" {
			t.Fatalf("分享链接应写出 Cookie: %d", resp.StatusCode)
		}
		resp, body := get(t, noRedirect(), addr, "must.test", "/", map[string]string{"Accept": "text/html", "Cookie": c})
		if resp.StatusCode != 200 || !strings.Contains(body, "public.example.com") || strings.Contains(body, "secret.example.com") {
			t.Fatalf("分享链接访客只应看到公开链接: %d", resp.StatusCode)
		}
	})
	t.Run("无认证站点", func(t *testing.T) {
		_, body := get(t, http.DefaultClient, addr, "open.test", "/", htmlAccept)
		if !strings.Contains(body, "public.example.com") || strings.Contains(body, "secret.example.com") || strings.Contains(body, authPath) {
			t.Fatal("无认证站点不显示内部链接，也没有登录入口")
		}
	})
	t.Run("基本认证", func(t *testing.T) {
		// 可选模式识别不出身份：带有效凭据也只显示公开链接，没有登录入口（基本认证没有登录页）
		_, body := get(t, http.DefaultClient, addr, "basic.test", "/", map[string]string{"Accept": "text/html", "Authorization": "Basic YWxpY2U6cHc="})
		if !strings.Contains(body, "public.example.com") || strings.Contains(body, "secret.example.com") || strings.Contains(body, authPath) {
			t.Fatal("基本认证可选模式只显示公开链接")
		}
		// 整体需要基本认证：通过认证的访客看到全部链接
		resp, body := get(t, http.DefaultClient, addr, "bmust.test", "/", map[string]string{"Accept": "text/html", "Authorization": "Basic YWxpY2U6cHc="})
		if resp.StatusCode != 200 || !strings.Contains(body, "secret.example.com") {
			t.Fatalf("基本认证通过后应看到内部链接: %d", resp.StatusCode)
		}
		if resp, body = get(t, http.DefaultClient, addr, "bmust.test", "/", htmlAccept); resp.StatusCode != 401 || strings.Contains(body, "secret.example.com") {
			t.Fatalf("未认证应拒绝: %d", resp.StatusCode)
		}
	})
}

// 面板中全部链接都要求登录时，未登录访客看到提示而不是“还没有添加链接”。
func TestPanelAuthOnlyEmptyHint(t *testing.T) {
	p := authOnlyPanel()
	p.Links = p.Links[1:2]
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	if !strings.Contains(body, `class="empty">登录后可查看更多链接`) || strings.Contains(body, `class="empty">还没有添加链接`) || strings.Contains(body, "secret.example.com") {
		t.Fatal("应只给出登录提示")
	}
}

// 分组登录后可见：分组中的链接（包括之后加入的）都按登录后可见处理；未分组也可以设置；
// 保存时去掉重复项与已没有链接的分组。
func TestPanelAuthGroups(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true,
		AuthGroups: []string{" 内部 ", "内部", "已删除的分组", ""},
		Links: []PanelLink{
			{Name: "公开站", Group: "公开", URL: "https://public.example.com"},
			{Name: "内部站", Group: "内部", URL: "https://secret.example.com"},
			{Name: "新加入的内部站", Group: " 内部", URL: "https://new.example.com"},
			{Name: "未分组站", URL: "https://plain.example.com"},
		}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	if len(p.AuthGroups) != 2 || p.AuthGroups[0] != "内部" || p.AuthGroups[1] != "" {
		t.Fatalf("规范化后的分组: %q", p.AuthGroups)
	}
	cp := &compiledPanel{Panel: p}
	q := redirectRequest{r: httptest.NewRequest("GET", "http://home.nav.test/", nil)}
	v := cp.view(q, false, false, false, false)
	if v.Hidden != 3 || len(v.Groups) != 1 || v.Groups[0].Name != "公开" || len(v.Groups[0].Links) != 1 {
		t.Fatalf("未登录只应看到公开分组: %+v", v)
	}
	v = cp.view(q, false, false, false, true)
	if v.Hidden != 0 || len(v.Groups) != 3 {
		t.Fatalf("已登录: %+v", v)
	}
	if v.Groups[0].Links[0].Locked || !v.Groups[1].Links[0].Locked || !v.Groups[1].Links[1].Locked || !v.Groups[2].Links[0].Locked {
		t.Fatalf("分组内的链接应带锁标记: %+v", v.Groups)
	}
	// 分组里的链接全部移走后，旧分组名不再保留
	p.Links = p.Links[:1]
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	if len(p.AuthGroups) != 0 {
		t.Fatalf("没有链接的分组应去掉: %q", p.AuthGroups)
	}
}
