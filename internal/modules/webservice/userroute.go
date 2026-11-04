package webservice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

// UserRoute 把指定认证用户的请求分到另一组后端，相当于 nginx 的 map $remote_user $backend。
// 未匹配任何规则的用户（以及无需认证路径上的未登录访客）使用站点后端。
// 分路径规则中填写了后端的路径仍按分路径规则转发；只追加请求头、沿用站点后端的路径改用该用户的后端。
type UserRoute struct {
	// Users 为身份标识：站点用户或共享用户的用户名，第三方登录身份写 提供方key:标识，转发认证为认证服务返回的 Remote-User
	Users []string `json:"users"`
	// Targets 为这些用户的后端，格式同站点后端
	Targets []string `json:"targets"`
}

// userRouteHandler 是编译后的按用户分流规则。
type userRouteHandler struct {
	users   map[string]bool
	pool    *pool
	handler http.Handler
}

// requestIdentity 记录认证后识别出的访客身份与选中的分流规则，经请求上下文传给后续处理器。
type requestIdentity struct {
	// route 为命中的分流规则序号，-1 表示使用站点后端
	route int
	// cacheTag 附加在响应缓存键中，避免不同后端（或传递了用户名时不同用户）的响应相互串用
	cacheTag string
}

type ctxKeyIdentity struct{}

// validateUserRoutes 校验按用户分流规则与身份请求头。
func (s *Site) validateUserRoutes() error {
	s.UserHeader = strings.TrimSpace(s.UserHeader)
	if len(s.UserRoutes) == 0 && len(s.identityHeaders()) == 0 {
		s.IdentityHeaders = nil
		return nil
	}
	if s.Type != TypeProxy {
		return errors.New("按用户分流与传递身份信息仅适用于反向代理站点")
	}
	if !s.BasicAuth && !s.WebAuth && s.ForwardAuth.URL == "" {
		return errors.New("按用户分流与传递身份信息需要先启用基本认证、网页认证或转发认证")
	}
	if err := s.validateIdentityHeaders(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := range s.UserRoutes {
		u := &s.UserRoutes[i]
		u.Users = cleanList(u.Users)
		u.Targets = cleanList(u.Targets)
		if len(u.Users) == 0 {
			return fmt.Errorf("第 %d 条按用户分流规则需要填写用户", i+1)
		}
		if len(u.Targets) == 0 {
			return fmt.Errorf("第 %d 条按用户分流规则需要填写后端", i+1)
		}
		for _, name := range u.Users {
			if seen[routeUserKey(name)] {
				return fmt.Errorf("用户 %s 出现在多条按用户分流规则中", name)
			}
			seen[routeUserKey(name)] = true
		}
		for _, t := range u.Targets {
			if _, err := parseUpstream(t); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildUserRoutes 为每条按用户分流规则创建一套反向代理：站点后端换成规则的后端，
// 自带后端的分路径规则沿用已建好的处理器，只追加请求头的分路径规则改用规则的后端。
func (h *siteHandler) buildUserRoutes() error {
	for _, ur := range h.site.UserRoutes {
		s := h.site
		s.Targets = ur.Targets
		p, err := newPool(s, h.log)
		if err != nil {
			return err
		}
		p.site = siteLabel(h.site) + " @" + strings.Join(ur.Users, ",")
		p.scope, p.match = scopeUser, strings.Join(ur.Users, ",")
		handler := h.newProxy(s, p)
		if len(h.locations) > 0 {
			locs := make([]*locationHandler, 0, len(h.locations))
			for _, l := range h.locations {
				if l.pool != nil {
					locs = append(locs, l)
					continue
				}
				ls := s
				ls.RequestHeaders = append(append([]string{}, h.site.RequestHeaders...), l.headers...)
				locs = append(locs, &locationHandler{prefix: l.prefix, headers: l.headers, handler: h.newProxy(ls, p)})
			}
			handler = h.dispatchLocations(locs, handler)
		}
		users := map[string]bool{}
		for _, u := range ur.Users {
			users[routeUserKey(u)] = true
		}
		h.userRoutes = append(h.userRoutes, &userRouteHandler{users: users, pool: p, handler: handler})
	}
	return nil
}

// dispatchUserRoutes 按认证阶段选中的规则转发，未命中时交给站点默认的处理器。
func (h *siteHandler) dispatchUserRoutes(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := r.Context().Value(ctxKeyIdentity{}).(requestIdentity); ok && id.route >= 0 && id.route < len(h.userRoutes) {
			h.userRoutes[id.route].handler.ServeHTTP(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}

// usesIdentity 判断站点是否需要在认证后识别访客身份。
func (h *siteHandler) usesIdentity() bool {
	return len(h.userRoutes) > 0 || len(h.site.identityHeaders()) > 0
}

// withIdentity 按访客身份选择分流规则、写入身份请求头，并把结果放入请求上下文。
// v.names 为空表示未识别出身份（未登录或经分享链接访问），使用站点后端且不传身份信息。
func (h *siteHandler) withIdentity(r *http.Request, v visitor) *http.Request {
	names := v.names
	id := requestIdentity{route: -1}
	for i, ur := range h.userRoutes {
		if matchUserRoute(ur.users, names) {
			id.route = i
			break
		}
	}
	id.cacheTag = "r" + strconv.Itoa(id.route) + h.setIdentityHeaders(r, v)
	return r.WithContext(context.WithValue(r.Context(), ctxKeyIdentity{}, id))
}

// routeUserKey 返回分流规则中用户的比较键：第三方登录标识（提供方:ID）与允许名单一样不区分大小写，
// 站点用户与共享用户名区分大小写，保持原样。
func routeUserKey(name string) string {
	if strings.Contains(name, ":") {
		return strings.ToLower(name)
	}
	return name
}

func matchUserRoute(users map[string]bool, names []string) bool {
	for _, n := range names {
		if users[routeUserKey(n)] {
			return true
		}
	}
	return false
}

// forwardIdentity 返回转发认证通过后认证服务给出的身份：Remote-User、Remote-Email、Remote-Name、Remote-Groups
// 仅当在复制的响应头中时可信（访客自带的已被删除，未复制的头可能是访客伪造的）；没有用户名时其他信息也不认
func (f *forwardAuther) forwardIdentity(r *http.Request) visitor {
	var v visitor
	for _, h := range f.headers {
		val := strings.TrimSpace(r.Header.Get(h))
		switch textproto.CanonicalMIMEHeaderKey(h) {
		case "Remote-User":
			if val != "" {
				v.names = []string{val}
			}
		case "Remote-Email":
			v.email = val
		case "Remote-Name":
			v.name = val
		case "Remote-Groups":
			v.groups = val
		}
	}
	if len(v.names) == 0 {
		return visitor{}
	}
	return v
}
