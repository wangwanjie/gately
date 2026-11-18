package webservice

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/webpage"
)

// panelView 是面板列表中的条目：面板本身加上引用它的站点（监听名 / 站点名）。
type panelView struct {
	Panel
	UsedBy []string `json:"usedBy,omitempty"`
}

// PanelPreviewRequest 是预览请求：编辑中的面板（不必已保存）与预览使用内网还是外网地址。
type PanelPreviewRequest struct {
	Panel Panel `json:"panel"`
	LAN   bool  `json:"lan"`
}

// PanelPreview 是预览结果：完整页面，管理界面放在禁用脚本的 sandbox iframe 中显示。
type PanelPreview struct {
	HTML string `json:"html"`
}

// panelUsersLocked 返回引用面板的站点（含默认站点）。
func (m *Module) panelUsersLocked(key string) []string {
	var out []string
	for i := range m.cfg.Listeners {
		l := &m.cfg.Listeners[i]
		for j := range l.Sites {
			if s := &l.Sites[j]; s.Type == TypePanel && s.Panel == key {
				out = append(out, listenerSiteLabel(l, s))
			}
		}
		if l.Default.Type == TypePanel && l.Default.Panel == key {
			out = append(out, defaultSiteLabel(l, defaultSiteMarker))
		}
	}
	return out
}

func (m *Module) findPanelLocked(key string) *Panel {
	for i := range m.cfg.Panels {
		if m.cfg.Panels[i].Key == key {
			return &m.cfg.Panels[i]
		}
	}
	return nil
}

// checkPanelRefsLocked 检查监听器中导航面板站点引用的面板都存在。
func (m *Module) checkPanelRefsLocked(l *Listener) error {
	for i := range l.Sites {
		if s := &l.Sites[i]; s.Type == TypePanel && m.findPanelLocked(s.Panel) == nil {
			return fmt.Errorf("站点 %s 引用的导航面板不存在", listenerSiteLabel(l, s))
		}
	}
	if l.Default.Type == TypePanel && l.Default.Enable && m.findPanelLocked(l.Default.Panel) == nil {
		return fmt.Errorf("站点 %s 引用的导航面板不存在", defaultSiteLabel(l, "默认站点"))
	}
	return nil
}

// registerPanels 注册面板库的增删改查与预览接口。
func (m *Module) registerPanels(r module.Router) {
	panels := &crud.Collection[Panel]{
		Mu:      &m.mu,
		Items:   &m.cfg.Panels,
		Save:    m.saveLocked,
		Key:     func(p *Panel) *string { return &p.Key },
		Enabled: func(p *Panel) *bool { return &p.Enable },
		// 面板分组只影响管理界面的面板列表，修改分组与顺序不影响访客页面
		Group:    func(p *Panel) *string { return &p.Group },
		Validate: func(p *Panel) error { return p.validate() },
		Check: func(p *Panel) error {
			for _, x := range m.cfg.Panels {
				if x.Key != p.Key && x.Name == p.Name {
					return fmt.Errorf("已有名为 %s 的导航面板", p.Name)
				}
			}
			return nil
		},
		View: func(p *Panel) any { return panelView{Panel: *p, UsedBy: m.panelUsersLocked(p.Key)} },
		// 面板在请求时从面板库读取，修改后替换面板库即可生效，不需要重启监听
		Changed: func(string) { m.auth.panels.set(m.cfg.Panels) },
		Removed: func(string) { m.auth.panels.set(m.cfg.Panels) },
		BeforeRemove: func(p *Panel) error {
			if users := m.panelUsersLocked(p.Key); len(users) > 0 {
				return fmt.Errorf("导航面板仍被 %s 引用，请先修改这些站点", strings.ReplaceAll(strings.Join(users, "、"), defaultSiteMarker, "默认站点"))
			}
			return nil
		},
		Max: maxPanels,
	}
	panels.Register(r, "/panels")
	r.HandleFunc("POST /panels/preview", m.handlePanelPreview)
	r.HandleFunc("GET /panels/icons", handlePanelIcons)
	r.HandleFunc("POST /panels/sunpanel", m.handlePanelSunPanel)
	r.HandleFunc("POST /panels/favicon", m.handlePanelFavicon)
	module.DocOf(r, "GET /panels/icons", module.Doc{Summary: "导航面板的内置图标列表", Resp: []PanelIconView{}})
	module.DocOf(r, "POST /panels/sunpanel", module.Doc{Summary: "转换 Sun-Panel 导出的配置（1.x 的 .sun-panel.json 或 2.x 的 .zip）为导航面板链接，不保存。multipart 表单：file 为导出文件，baseUrl 为可选的 Sun-Panel 地址", Resp: &SunPanelImportResult{}})
	module.DocOf(r, "POST /panels/favicon", module.Doc{Summary: "从链接地址获取网站图标：按顺序访问各地址，解析网页中的图标声明与 Web App Manifest，找不到时用 /favicon.ico，返回 data:image 图片，不保存。地址需为不含变量的 http/https 地址", Req: &PanelFaviconRequest{}, Resp: &PanelFaviconResult{}})
	module.DocOf(r, "POST /panels/preview", module.Doc{Summary: "按编辑中的内容生成导航面板预览，不保存", Req: &PanelPreviewRequest{}, Resp: &PanelPreview{}})
}

// PanelIconView 是内置图标列表中的一项：图标写为 builtin:编号；src 为白色图形、srcDark 为深色图形，
// 管理界面按底色深浅选用，与访客页面一致。
type PanelIconView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cat     string `json:"cat"`
	Color   string `json:"color"`
	Src     string `json:"src"`
	SrcDark string `json:"srcDark"`
}

func handlePanelIcons(w http.ResponseWriter, _ *http.Request) {
	list := webpage.BuiltinIcons()
	out := make([]PanelIconView, 0, len(list))
	for i := range list {
		ic := &list[i]
		out = append(out, PanelIconView{ID: ic.ID, Name: ic.Name, Cat: ic.Cat, Color: ic.Color, Src: webpage.BuiltinIconURL(ic, false), SrcDark: webpage.BuiltinIconURL(ic, true)})
	}
	// 图标随版本固定，允许浏览器缓存一段时间
	w.Header().Set("Cache-Control", "private, max-age=3600")
	httpx.OK(w, out)
}

// handlePanelPreview 校验编辑中的面板并生成预览页面。{host} 等变量按管理后台的访问地址展开，
// STUN 变量取当前的公网地址；预览不含脚本与搜索框，并用 meta 标签禁止脚本。
func (m *Module) handlePanelPreview(w http.ResponseWriter, r *http.Request) {
	var req PanelPreviewRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	p := req.Panel
	if err := p.validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	lan, _ := parsePrefixes(p.LANCIDRs)
	cp := &compiledPanel{Panel: p, lan: lan}
	body, _ := webpage.RenderPanel(r, cp.view(redirectRequest{r: r}, req.LAN, false, true, true))
	httpx.OK(w, PanelPreview{HTML: string(body)})
}
