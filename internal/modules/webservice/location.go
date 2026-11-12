package webservice

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Location 把站点下某个路径前缀的请求分到另一组后端，相当于 nginx 的 location /api/ { proxy_pass ...; }。
// 默认路径原样转发；StripPrefix 开启时去掉前缀再转发（nginx 的 proxy_pass 带 URI 的写法）。其余反向代理设置（Host、转发头、TLS 校验等）沿用站点。
type Location struct {
	// Prefix 为路径前缀，按路径段边界匹配：/api 与 /api/ 都匹配 /api 及 /api/x，不匹配 /apix
	Prefix string `json:"prefix"`
	// Targets 为该路径的后端，格式同站点后端；留空时沿用站点后端，只追加请求头
	Targets []string `json:"targets"`
	// StripPrefix 开启时转发前去掉路径前缀：/api/zyqd/abc 发往后端为 /abc，后端地址自带的路径会接在前面
	StripPrefix bool `json:"stripPrefix"`
	// RequestHeaders 追加在站点请求头之后，同名时覆盖站点的设置
	RequestHeaders []string `json:"requestHeaders"`
}

// locationHandler 是编译后的分路径规则。
type locationHandler struct {
	prefix string
	// stripPrefix 为转发前是否去掉前缀，按用户分流为沿用站点后端的路径重建处理器时需要
	stripPrefix bool
	// headers 为该路径追加的请求头，按用户分流时为沿用站点后端的路径重建处理器
	headers []string
	// pool 为该路径自己的后端，沿用站点后端时为 nil
	pool    *pool
	handler http.Handler
}

// validateLocations 校验分路径规则。
func (s *Site) validateLocations() error {
	if len(s.Locations) == 0 {
		return nil
	}
	if s.Type != TypeProxy {
		return errors.New("分路径后端仅适用于反向代理站点")
	}
	seen := map[string]bool{}
	for i := range s.Locations {
		l := &s.Locations[i]
		l.Prefix = strings.TrimSpace(l.Prefix)
		if !strings.HasPrefix(l.Prefix, "/") || strings.ContainsAny(l.Prefix, " ?#") {
			return fmt.Errorf("路径前缀 %q 需以 / 开头且不含空格、? 或 #", l.Prefix)
		}
		if seen[l.Prefix] {
			return fmt.Errorf("路径前缀 %s 重复", l.Prefix)
		}
		seen[l.Prefix] = true
		l.Targets = cleanList(l.Targets)
		for _, t := range l.Targets {
			if _, err := parseUpstream(t); err != nil {
				return err
			}
		}
		l.RequestHeaders = cleanList(l.RequestHeaders)
		if err := validateHeaderLines(l.RequestHeaders); err != nil {
			return err
		}
		if len(l.Targets) == 0 && len(l.RequestHeaders) == 0 && !l.StripPrefix {
			return fmt.Errorf("路径 %s 需要填写后端、请求头或开启去掉前缀", l.Prefix)
		}
	}
	return nil
}

// buildLocations 为每条分路径规则创建独立的反向代理。
func (h *siteHandler) buildLocations() error {
	for _, loc := range h.site.Locations {
		s := h.site
		// 请求头先应用站点的，再应用路径的，同名时以路径为准
		s.RequestHeaders = append(append([]string{}, h.site.RequestHeaders...), loc.RequestHeaders...)
		p := h.pool
		if len(loc.Targets) > 0 {
			s.Targets = loc.Targets
			var err error
			if p, err = newPool(s, h.log); err != nil {
				return err
			}
			p.site = siteLabel(h.site) + " " + loc.Prefix
			p.scope, p.match = scopeLocation, loc.Prefix
		}
		lh := &locationHandler{prefix: loc.Prefix, headers: loc.RequestHeaders, stripPrefix: loc.StripPrefix, handler: h.newProxy(s, p, stripPrefixOf(loc))}
		if p != h.pool {
			lh.pool = p
		}
		h.locations = append(h.locations, lh)
	}
	// 最长前缀优先
	sort.SliceStable(h.locations, func(i, j int) bool { return len(h.locations[i].prefix) > len(h.locations[j].prefix) })
	return nil
}

// dispatchLocations 按路径前缀在 locs 中选择后端，未命中时交给 fallback。
func (h *siteHandler) dispatchLocations(locs []*locationHandler, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := cleanReqPath(r.URL.Path)
		for _, l := range locs {
			if prefixMatch(l.prefix, p) {
				l.handler.ServeHTTP(w, r)
				return
			}
		}
		fallback.ServeHTTP(w, r)
	})
}

// stripPrefixOf 返回需要在转发前去掉的路径前缀，未开启时为空。
func stripPrefixOf(l Location) string {
	if !l.StripPrefix {
		return ""
	}
	return l.Prefix
}

// stripLocationPrefix 去掉请求路径的前缀并保留查询串，结果始终以 / 开头。
// 先按与分流匹配相同的方式清理路径（合并 //、解析 . 与 ..），避免用 /api/../x 这类写法越过前缀；
// 去掉前缀后原路径若以 / 结尾则保留，后端对 /dir 与 /dir/ 的处理可能不同。
func stripLocationPrefix(u *url.URL, prefix string) {
	if prefix == "" || prefix == "/" {
		return
	}
	p := cleanReqPath(u.Path)
	if !prefixMatch(prefix, p) {
		return
	}
	rest := strings.TrimPrefix(p, strings.TrimRight(prefix, "/"))
	if rest == "" {
		rest = "/"
	}
	if strings.HasSuffix(u.Path, "/") && !strings.HasSuffix(rest, "/") {
		rest += "/"
	}
	u.Path = rest
	// 路径已按解码后的值重建，原始编码需让标准库重新生成
	u.RawPath = ""
}
