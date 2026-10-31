package webservice

import (
	"errors"
	"net/http"
	"slices"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 「认证用户」页面的接口：集中管理共享用户、单点登录、自助改密与 OIDC 身份提供方，
// 并批量设置各站点是否使用共享用户。只读写这些设置，全局设置中的其他项保持不变，
// 避免页面上较旧的全局设置覆盖别处刚保存的修改。

// UsersSettings 为共享用户与 OIDC 身份提供方相关的设置，字段含义同 Settings 中的同名字段。
type UsersSettings struct {
	AuthUsers           []AuthUser   `json:"authUsers"`
	SSODomain           string       `json:"ssoDomain"`
	SelfServicePassword bool         `json:"selfServicePassword"`
	OIDC                OIDCSettings `json:"oidc"`
}

// UserSite 是可以使用共享用户的站点（断开连接类型的站点不经过认证，不列出）。
type UserSite struct {
	Listener     string `json:"listener"`
	ListenerName string `json:"listenerName"`
	Port         int    `json:"port"`
	TLS          bool   `json:"tls"`
	// ListenerEnable 为监听器是否启用，Enable 为站点本身是否启用
	ListenerEnable bool `json:"listenerEnable"`
	// Site 为站点 key，默认站点为 default
	Site    string   `json:"site"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	// Group 为站点在 Web 服务列表中的分组，认证用户页按它归类显示
	Group   string `json:"group"`
	Enable  bool   `json:"enable"`
	Default bool   `json:"default"`
	Type    string `json:"type"`

	BasicAuth   bool `json:"basicAuth"`
	WebAuth     bool `json:"webAuth"`
	SharedUsers bool `json:"sharedUsers"`
	// OtherAuth 为站点除共享用户外还有本地用户或第三方登录身份：取消共享用户后站点仍需登录
	OtherAuth bool `json:"otherAuth"`
	// ForwardAuth 为站点使用转发认证，不能同时启用共享用户
	ForwardAuth bool `json:"forwardAuth"`
}

// UsersView 为 GET /users 的响应，密码、TOTP 与客户端密钥已脱敏。
type UsersView struct {
	UsersSettings
	Sites []UserSite `json:"sites"`
}

// SiteSharedUsers 把一个站点设为使用或不再使用共享用户。
type SiteSharedUsers struct {
	Listener    string `json:"listener"`
	Site        string `json:"site"`
	SharedUsers bool   `json:"sharedUsers"`
}

// UsersRequest 为 PUT /users 的请求体。Sites 只需列出要修改的站点，省略或为空表示不修改站点。
type UsersRequest struct {
	UsersSettings
	Sites []SiteSharedUsers `json:"sites"`
}

func (m *Module) registerUsers(r module.Router) {
	r.HandleFunc("GET /users", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, m.usersViewLocked())
	})
	r.HandleFunc("PUT /users", m.handlePutUsers)
	module.DocOf(r, "GET /users", module.Doc{Summary: "读取共享用户、OIDC 身份提供方与各站点的共享用户开关", Resp: &UsersView{}})
	module.DocOf(r, "PUT /users", module.Doc{Summary: "保存共享用户与 OIDC 身份提供方，并批量设置站点是否使用共享用户", Req: &UsersRequest{}, Resp: &UsersView{}})
}

func (m *Module) usersViewLocked() UsersView {
	s := m.cfg.Settings
	v := UsersView{
		UsersSettings: UsersSettings{
			AuthUsers:           maskAuthUsers(s.AuthUsers),
			SSODomain:           s.SSODomain,
			SelfServicePassword: s.SelfServicePassword,
			OIDC:                maskOIDC(s.OIDC),
		},
		Sites: []UserSite{},
	}
	for _, l := range m.cfg.Listeners {
		add := func(st Site, deflt bool) {
			// 断开连接的站点不经过认证；停用的默认站点通常只是占位，也不列出
			if st.Type == TypeClose || (deflt && !st.Enable) {
				return
			}
			v.Sites = append(v.Sites, UserSite{
				Listener:       l.Key,
				ListenerName:   l.Name,
				Port:           l.Port,
				TLS:            l.TLS,
				ListenerEnable: l.Enable,
				Site:           st.Key,
				Name:           st.Name,
				Domains:        st.Domains,
				Group:          st.Group,
				Enable:         st.Enable,
				Default:        deflt,
				Type:           st.Type,
				BasicAuth:      st.BasicAuth,
				WebAuth:        st.WebAuth,
				SharedUsers:    st.SharedUsers,
				OtherAuth:      hasOtherAuth(st),
				ForwardAuth:    st.ForwardAuth.URL != "",
			})
		}
		for _, st := range l.Sites {
			add(st, false)
		}
		add(l.Default, true)
	}
	return v
}

// hasOtherAuth 判断站点在不使用共享用户时是否仍然需要登录。
func hasOtherAuth(s Site) bool {
	return (s.BasicAuth && len(s.Users) > 0) || (s.WebAuth && (len(s.Users) > 0 || len(s.OAuthUsers) > 0))
}

// setSiteSharedUsers 修改站点的共享用户开关。启用时站点若未开启任何认证，同时开启网页认证；
// 取消时站点若再没有其他认证方式会变为无需登录即可访问，这种情况拒绝修改，需在站点设置中明确关闭认证。
func setSiteSharedUsers(s *Site, on bool) error {
	if s.SharedUsers == on {
		return nil
	}
	if on {
		if s.Type == TypeClose {
			return errors.New("断开连接的站点不经过认证")
		}
		if s.ForwardAuth.URL != "" {
			return errors.New("站点使用转发认证，不能同时使用共享用户")
		}
		if !s.BasicAuth && !s.WebAuth {
			s.WebAuth = true
		}
		s.SharedUsers = true
		return nil
	}
	if (s.BasicAuth || s.WebAuth) && !hasOtherAuth(*s) {
		return errors.New("站点只通过共享用户认证，取消后将无需登录即可访问，请在站点设置中关闭认证")
	}
	s.SharedUsers = false
	return nil
}

// normalizeUsersSettings 校验并整理共享用户、单点登录域与 OIDC 设置，被脱敏的密码与密钥沿用 old 中的值。
func normalizeUsersSettings(s *UsersSettings, old Settings) error {
	users, err := normalizeAuthUsers(s.AuthUsers, old.AuthUsers)
	if err != nil {
		return err
	}
	s.AuthUsers = users
	if s.SSODomain, err = validateSSODomain(s.SSODomain); err != nil {
		return err
	}
	return normalizeOIDC(&s.OIDC, old.OIDC)
}

// applyUsersLocked 让已保存的共享用户、单点登录、自助改密与 OIDC 设置生效。
func (m *Module) applyUsersLocked(s Settings) {
	m.auth.shared.set(s.AuthUsers, s.SSODomain)
	m.auth.selfAuth.Store(s.SelfServicePassword)
	if err := m.auth.oidc.set(s.OIDC); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "OIDC 身份提供方未启动: "+err.Error())
	}
}

func (m *Module) handlePutUsers(w http.ResponseWriter, r *http.Request) {
	var req UsersRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	if err := normalizeUsersSettings(&req.UsersSettings, old); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// 站点修改先作用在监听器副本上，全部校验通过后再与设置一起保存，任一失败都不做任何修改
	oldListeners := m.cfg.Listeners
	listeners := slices.Clone(oldListeners)
	changed := map[string]bool{}
	for _, c := range req.Sites {
		i := slices.IndexFunc(listeners, func(l Listener) bool { return l.Key == c.Listener })
		if i < 0 {
			httpx.Error(w, http.StatusBadRequest, "监听器 %s 不存在", c.Listener)
			return
		}
		l := &listeners[i]
		if !changed[l.Key] {
			// 首次修改该监听器时复制站点列表，避免改到当前配置
			l.Sites = slices.Clone(l.Sites)
		}
		var site *Site
		if c.Site == "default" {
			site = &l.Default
		} else if j := slices.IndexFunc(l.Sites, func(s Site) bool { return s.Key == c.Site }); j >= 0 {
			site = &l.Sites[j]
		} else {
			httpx.Error(w, http.StatusBadRequest, "站点 %s 不存在", c.Site)
			return
		}
		if err := setSiteSharedUsers(site, c.SharedUsers); err != nil {
			httpx.Error(w, http.StatusBadRequest, "站点 %s: %v", siteLabel(*site), err)
			return
		}
		changed[l.Key] = true
	}
	for i := range listeners {
		l := &listeners[i]
		if !changed[l.Key] {
			continue
		}
		if err := l.Validate(); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	s := old
	s.AuthUsers = req.AuthUsers
	s.SSODomain = req.SSODomain
	s.SelfServicePassword = req.SelfServicePassword
	s.OIDC = req.OIDC
	m.cfg.Settings = s
	m.cfg.Listeners = listeners
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		m.cfg.Listeners = oldListeners
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.applyUsersLocked(s)
	// 共享用户变化需要重建站点表；热替换不断开现有连接
	for _, l := range m.cfg.Listeners {
		m.restartLocked(l.Key)
	}
	httpx.OK(w, m.usersViewLocked())
}
