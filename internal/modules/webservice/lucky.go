package webservice

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/luckyimport"
)

// flexStrings 兼容 Lucky 中同一字段时而是字符串、时而是字符串数组的情况。
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*f = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s != "" {
		*f = []string{s}
	}
	return nil
}

// luckySite 对应 Lucky 反向代理子规则（ProxyList 元素）与默认规则。
type luckySite struct {
	Key                        string
	Enable                     bool
	Remark                     string
	Domains                    []string
	Locations                  []string
	WebServiceType             string
	LocationInsecureSkipVerify bool
	UseTargetHost              bool
	DisableKeepAlives          bool
	AddRemoteIPToHeader        bool
	AddProtoToHeader           bool
	EnableBasicAuth            bool
	BasicAuthUser              string
	BasicAuthPasswd            string
	BasicAuthUserList          flexStrings
	SafeIPMode                 string
	EnableAccessLog            bool
	EnableCrossDomain          bool
	CorazaWAFInstance          string
	CustomOutputText           string
	OtherParams                map[string]any
}

type luckyListener struct {
	RuleName      string
	Enable        bool
	ListenIP      string
	ListenPort    int
	Network       string
	EnableTLS     bool
	Http3         bool
	TLSMinVersion int
	DefaultProxy  luckySite
	ProxyList     []luckySite
	// CorazaWAFInstance 为监听规则的 WAF 实例；子规则填 "main" 表示沿用它
	CorazaWAFInstance string
}

// luckyInheritWAF 是 Lucky 子规则中表示"沿用监听规则 WAF 实例"的取值。
const luckyInheritWAF = "main"

// resolveWAF 把子规则的 WAF 引用解析为实例 key。
func resolveWAF(site, listener string) string {
	if site == luckyInheritWAF {
		return listener
	}
	return site
}

// luckyUsers 解析用户列表。Lucky 某些版本会把整段 "user:pass" 文本按字符存成数组，这里先还原再拆分。
func luckyUsers(ls luckySite) []string {
	list := ls.BasicAuthUserList
	allChars := len(list) > 1
	for _, s := range list {
		if len([]rune(s)) != 1 {
			allChars = false
			break
		}
	}
	if allChars {
		list = []string{strings.Join(list, "")}
	}
	var out []string
	for _, item := range list {
		for _, u := range strings.FieldsFunc(item, func(r rune) bool { return r == '\n' || r == ',' }) {
			if u = strings.TrimSpace(u); strings.Contains(u, ":") {
				out = append(out, u)
			}
		}
	}
	if ls.BasicAuthUser != "" && ls.BasicAuthPasswd != "" {
		out = append(out, ls.BasicAuthUser+":"+ls.BasicAuthPasswd)
	}
	return out
}

// convertSite 把 Lucky 子规则转换为站点，返回不支持特性的说明。
func convertSite(ls luckySite, opts luckyimport.Options) (Site, []string) {
	var notes []string
	s := Site{
		Name:               ls.Remark,
		Enable:             ls.Enable,
		Domains:            ls.Domains,
		Targets:            ls.Locations,
		InsecureSkipVerify: ls.LocationInsecureSkipVerify,
		UseTargetHost:      ls.UseTargetHost,
		DisableKeepAlives:  ls.DisableKeepAlives,
		ForwardedHeaders:   ls.AddRemoteIPToHeader || ls.AddProtoToHeader,
		AccessLog:          ls.EnableAccessLog,
		CORS:               ls.EnableCrossDomain,
		// 保留 Lucky 的 WAF 实例 key，WAF 模块导入时沿用相同 key
		WAF: ls.CorazaWAFInstance,
	}
	// OtherParams.ProxyProtocolV2 只对 Lucky 的 SNI 分流类型生效，且新建站点默认即为 true；
	// 反向代理站点不会向后端发送 PROXY 头，迁移时不能据此开启，否则不支持该协议的后端会返回 400
	if v, ok := ls.OtherParams["ProxyProtocolV2"].(bool); ok && ls.WebServiceType == "SNIRouting" {
		s.SendProxyProtocol = v
	}
	webAuth, _ := ls.OtherParams["WebAuth"].(bool)
	s.WebAuth = webAuth
	s.BasicAuth = ls.EnableBasicAuth
	// 第三方登录：Lucky 中引用身份表里的 Key，按身份表解析为 提供方:标识
	allThird, _ := ls.OtherParams["AllowAllThirdUsers"].(bool)
	thirdKeys := stringList(ls.OtherParams["AllowThirdUserList"])
	if webAuth && (allThird || len(thirdKeys) > 0) {
		notes = append(notes, "第三方登录")
		if opts.ResolveThirdParty != nil {
			var missing int
			s.OAuthUsers, missing = opts.ResolveThirdParty(thirdKeys, allThird)
			if missing > 0 {
				notes = append(notes, "第三方身份缺失")
			}
		} else {
			notes = append(notes, "第三方身份缺失")
		}
	}
	if s.WebAuth || s.BasicAuth {
		s.Users = luckyUsers(ls)
		// 没有站点用户、也没有可迁移的第三方身份时无法等价迁移；为避免去掉认证后暴露站点，停用该站点
		if (s.BasicAuth && len(s.Users) == 0) || (s.WebAuth && len(s.Users) == 0 && len(s.OAuthUsers) == 0) {
			s.WebAuth, s.BasicAuth, s.Enable = false, false, false
			s.OAuthUsers = nil
			notes = append(notes, "认证停用")
		}
	}
	switch ls.WebServiceType {
	case "reverseproxy", "":
		s.Type = TypeProxy
	case "redirect", "url":
		s.Type = TypeRedirect
		if len(ls.Locations) > 0 {
			s.RedirectTo = ls.Locations[0]
		}
		s.KeepPath = true
	case "textoutput":
		s.Type = TypeText
		s.Text = ls.CustomOutputText
	default:
		notes = append(notes, fmt.Sprintf("站点类型 %q 暂不支持", ls.WebServiceType))
	}
	if ls.SafeIPMode == "whitelist" {
		notes = append(notes, "原规则使用全局 IP 白名单，迁移后未启用访问控制，请注意暴露面")
	}
	return s, notes
}

func convertLuckyListeners(raw json.RawMessage, opts luckyimport.Options) ([]Listener, []string, int, error) {
	var src []luckyListener
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, nil, 0, err
	}
	var (
		out   []Listener
		warns []string
	)
	minTLS := map[int]string{0: "1.0", 1: "1.1", 2: "1.2", 3: "1.3"}
	for i, ll := range src {
		label := ll.RuleName
		if label == "" {
			label = fmt.Sprintf("第 %d 个监听规则", i+1)
		}
		l := Listener{
			Name:     ll.RuleName,
			Enable:   ll.Enable,
			Network:  ll.Network,
			ListenIP: ll.ListenIP,
			Port:     ll.ListenPort,
			TLS:      ll.EnableTLS,
			MinTLS:   minTLS[ll.TLSMinVersion],
		}
		if ll.Http3 {
			warns = append(warns, label+": HTTP/3 暂不支持，已忽略")
		}
		// 汇总各站点的不支持特性，避免逐条重复告警
		featureSites := map[string][]string{}
		for j, ls := range ll.ProxyList {
			site, notes := convertSite(ls, opts)
			site.WAF = resolveWAF(site.WAF, ll.CorazaWAFInstance)
			name := site.Name
			if name == "" && len(site.Domains) > 0 {
				name = site.Domains[0]
			}
			if name == "" {
				name = fmt.Sprintf("站点 %d", j+1)
			}
			skip := false
			var bad []string
			site.Domains, bad = splitDomains(site.Domains)
			if len(bad) > 0 {
				warns = append(warns, fmt.Sprintf("%s/%s: 域名 %s 格式不正确，已忽略", label, name, strings.Join(bad, "、")))
				if len(site.Domains) == 0 {
					warns = append(warns, fmt.Sprintf("%s/%s: 没有可用的域名，已跳过", label, name))
					skip = true
				}
			}
			for _, n := range notes {
				if strings.HasPrefix(n, "站点类型") {
					warns = append(warns, label+"/"+name+": "+n+"，已跳过")
					skip = true
				} else {
					featureSites[n] = append(featureSites[n], name)
				}
			}
			if !skip {
				l.Sites = append(l.Sites, site)
			}
		}
		if def, notes := convertSite(ll.DefaultProxy, opts); len(def.Targets) > 0 || def.Type != TypeProxy {
			// 默认站点没有单独的启用开关，认证无法迁移时保持停用
			def.Enable = !slices.Contains(notes, "认证停用")
			def.WAF = resolveWAF(def.WAF, ll.CorazaWAFInstance)
			def.Domains = nil
			l.Default = def
			for _, n := range notes {
				featureSites[n] = append(featureSites[n], "默认站点")
			}
		}
		keys := make([]string, 0, len(featureSites))
		for k := range featureSites {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			msg := k
			switch k {
			case "第三方登录":
				msg = "第三方登录原本经过 Lucky 官方中转服务器，允许的身份已迁移；需在「Web 服务 → 全局设置」填写你自己的 OAuth 应用（登录方式标识用 github / google / qq）和回调地址后才能使用，在此之前仅能使用站点用户登录"
			case "第三方身份缺失":
				msg = "引用的第三方身份在身份表中找不到，请同时导入 lucky_thirdPartyAuthManager.lkcf（或先导入它再导入反向代理）"
			case "认证停用":
				msg = "这些站点没有可迁移的认证方式（无站点用户，也没有可解析的第三方身份），为避免去掉认证后暴露，已停用（导入第三方身份表或添加用户后再启用）"
			}
			warns = append(warns, fmt.Sprintf("%s: %s：%s", label, msg, strings.Join(featureSites[k], "、")))
		}
		if err := l.Validate(); err != nil {
			warns = append(warns, label+": 跳过，"+err.Error())
			continue
		}
		out = append(out, l)
	}
	if len(out) > 0 {
		warns = append(warns, "站点引用的 WAF 实例需同时导入 lucky_coraza.lkcf；子规则的 WAF 为 main 时已改为监听规则使用的实例")
	}
	return out, warns, len(src), nil
}

// ImportLucky 实现 luckyimport.Importer，同时识别 lucky_base 中的 PROXY 协议可信来源。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	if raw, ok := doc["ProxyProtocolTrustedCIDRs"]; ok && !opts.DryRun {
		var cidrs any
		_ = json.Unmarshal(raw, &cidrs)
		var list []string
		switch v := cidrs.(type) {
		case string:
			list = strings.FieldsFunc(v, func(r rune) bool { return r == '\n' || r == ',' })
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok {
					list = append(list, s)
				}
			}
		}
		if list = cleanList(list); len(list) > 0 {
			if _, err := parsePrefixes(list); err == nil {
				m.mu.Lock()
				m.cfg.Settings.TrustedProxies = list
				_ = m.saveLocked()
				m.mu.Unlock()
			}
		}
	}
	raw, ok := doc["ReverseProxyRuleList"]
	if !ok {
		return nil, nil
	}
	listeners, warns, found, err := convertLuckyListeners(raw, opts)
	if err != nil {
		return nil, err
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: found, Warnings: warns}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := map[string]bool{}
	for _, l := range m.cfg.Listeners {
		existing[l.Name] = true
	}
	var fresh []Listener
	for _, l := range listeners {
		if existing[l.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped(l.Name))
			continue
		}
		if !opts.KeepEnabled {
			l.Enable = false
		}
		fresh = append(fresh, l)
	}
	rep.Imported = len(fresh)
	if !opts.KeepEnabled && rep.Imported > 0 {
		rep.Warnings = append(rep.Warnings, "导入的监听规则默认停用：请先停止 Lucky 中对应的规则，再在 Gately 中启用；HTTPS 站点需要先在证书模块中启用对应证书")
	}
	if opts.DryRun {
		return rep, nil
	}
	old := m.cfg.Listeners
	for _, l := range fresh {
		l.Key = crud.NewKey()
		m.cfg.Listeners = append(m.cfg.Listeners, l)
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Listeners = old
		return nil, err
	}
	for _, l := range m.cfg.Listeners[len(old):] {
		m.restartLocked(l.Key)
	}
	return rep, nil
}

// stringList 把 JSON 解码出的任意值转换为字符串列表。
func stringList(v any) []string {
	arr, _ := v.([]any)
	var out []string
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
