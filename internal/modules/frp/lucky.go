package frp

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/luckyimport"
)

// luckyInstance 是 Lucky lucky_frp.lkcf 中的一个实例。Params 为 frpc / frps 参数（字段名为 frp 结构体名），
// proxies 与 visitors 为 frp v1 原生 JSON（camelCase）。
type luckyInstance struct {
	Key      string
	Remark   string
	Enable   bool
	Type     string
	Params   json.RawMessage
	Proxies  []map[string]any `json:"proxies"`
	Visitors []map[string]any `json:"visitors"`
}

type luckyClientParams struct {
	ServerAddr string
	ServerPort int
	User       string
	AuthMethod string
	Token      string
	Protocol   string
	TLSEnable  *bool
	ProxyURL   string
	AdminPort  int
}

type luckyServerParams struct {
	BindAddr       string
	BindPort       int
	KCPBindPort    int
	QUICBindPort   int
	VhostHTTPPort  int
	VhostHTTPSPort int
	SubDomainHost  string
	AllowPorts     string
	TLSOnly        bool
	Token          string
	DashboardPort  int
}

// luckyProxyKeys 是 frp 代理 JSON 中已映射到 Proxy 的字段。
var luckyProxyKeys = map[string]bool{
	"name": true, "type": true, "localIP": true, "localPort": true, "remotePort": true,
	"customDomains": true, "subdomain": true, "secretKey": true, "disabled": true, "enabled": true,
}

// ImportLucky 实现 luckyimport.Importer。Lucky 中的每个客户端、服务端实例分别导入为 Gately 的 frpc、frps 实例。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	// cloudflared 与存储管理同样使用 List 字段，按实例类型识别
	if !luckyimport.AllTypes(luckyimport.ListTypes(doc), RoleClient, RoleServer) {
		return nil, nil
	}
	var src []luckyInstance
	if err := json.Unmarshal(doc["List"], &src); err != nil {
		return nil, err
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: len(src)}

	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.Clients = append([]Client(nil), m.cfg.Clients...)
	cfg.Servers = append([]Server(nil), m.cfg.Servers...)
	cfg.Proxies = append([]Proxy(nil), m.cfg.Proxies...)
	changed := false
	for i := range src {
		li := &src[i]
		var ok bool
		var err error
		switch li.Type {
		case RoleClient:
			ok, err = importLuckyClient(&cfg, li, opts, rep)
		case RoleServer:
			ok, err = importLuckyServer(&cfg, li, opts, rep)
		}
		if err != nil {
			return nil, err
		}
		if ok {
			changed = true
			rep.Imported++
		}
	}
	if opts.DryRun || !changed {
		return rep, nil
	}
	old := m.cfg
	m.cfg = cfg
	if err := m.saveLocked(); err != nil {
		m.cfg = old
		return nil, err
	}
	// 只启动新建的实例、重启追加了代理的实例
	m.syncLocked(false)
	return rep, nil
}

func instanceName(li *luckyInstance) string {
	if li.Remark != "" {
		return li.Remark
	}
	return li.Key
}

// importLuckyClient 把客户端实例导入 cfg：Gately 中已有连接同一服务端（地址、端口与用户相同）的 frpc 实例时，
// 保留该实例的设置只追加代理，否则新建实例。
func importLuckyClient(cfg *fileConfig, li *luckyInstance, opts luckyimport.Options, rep *luckyimport.Report) (bool, error) {
	name := instanceName(li)
	var p luckyClientParams
	if len(li.Params) > 0 {
		if err := json.Unmarshal(li.Params, &p); err != nil {
			return false, fmt.Errorf("%s: %w", name, err)
		}
	}
	c := Client{
		Name:   name,
		Enable: li.Enable && opts.KeepEnabled,
		ClientSettings: ClientSettings{
			ServerAddr: p.ServerAddr,
			ServerPort: p.ServerPort,
			User:       p.User,
			Token:      p.Token,
			Protocol:   p.Protocol,
		},
	}
	if c.Protocol == "tcp" {
		c.Protocol = ""
	}
	if !protocols[c.Protocol] {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 不支持的传输协议 %q，已改用 tcp", name, c.Protocol))
		c.Protocol = ""
	}
	if p.AuthMethod != "" && p.AuthMethod != "token" {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 认证方式 %s 暂不支持，仅导入令牌", name, p.AuthMethod))
	}
	if p.TLSEnable != nil && !*p.TLSEnable {
		rep.Warnings = append(rep.Warnings, name+": 原配置关闭了 TLS，Gately 使用 frp 默认的 TLS，需确认 frps 端兼容")
	}
	if p.ProxyURL != "" {
		rep.Warnings = append(rep.Warnings, name+": 经代理连接服务端暂不支持，已忽略")
	}
	if p.AdminPort != 0 {
		rep.Warnings = append(rep.Warnings, name+": frpc 管理端口暂不支持，已忽略")
	}
	if err := c.Validate(); err != nil {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 跳过，%v", name, err))
		return false, nil
	}
	target := ""
	for _, cur := range cfg.Clients {
		if cur.ServerAddr != "" && cur.ServerAddr == c.ServerAddr && cur.ServerPort == c.ServerPort && cur.User == c.User {
			// 服务端相同：保留现有实例的设置与启用状态，只追加代理
			target = cur.Key
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: Gately 中的 frpc 实例 %s 已连接 %s:%d，代理追加到该实例", name, cur.Name, c.ServerAddr, c.ServerPort))
			break
		}
	}
	if target == "" {
		c.Key = crud.NewKey()
		cfg.Clients = append(cfg.Clients, c)
		target = c.Key
	}

	existing := map[string]bool{}
	for _, px := range cfg.Proxies {
		if px.Client == target {
			existing[px.Name] = true
		}
	}
	unsupported := map[string]int{}
	added := 0
	for _, raw := range li.Proxies {
		px, why, bad := convertLuckyProxy(raw, unsupported)
		if len(bad) > 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("代理 %s 的自定义域名 %s 格式不正确，已忽略", px.Name, strings.Join(bad, "、")))
		}
		if why != "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("代理 %s: 跳过，%s", px.Name, why))
			continue
		}
		if existing[px.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped("代理 "+px.Name))
			continue
		}
		existing[px.Name] = true
		px.Key, px.Client = crud.NewKey(), target
		cfg.Proxies = append(cfg.Proxies, px)
		added++
	}
	if len(unsupported) > 0 {
		keys := make([]string, 0, len(unsupported))
		for k := range unsupported {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			keys[i] = fmt.Sprintf("%s（%d 个代理）", k, unsupported[k])
		}
		rep.Warnings = append(rep.Warnings, "代理字段暂不支持，已忽略："+strings.Join(keys, "、"))
	}
	if len(li.Visitors) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: %d 个访问者（visitor）暂不支持，已跳过", name, len(li.Visitors)))
	}
	rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: frpc %s:%d，导入 %d 个代理", name, c.ServerAddr, c.ServerPort, added))
	return true, nil
}

// convertLuckyProxy 转换一个 frp 代理，why 非空时表示无法迁移。unsupported 统计未映射且有值的字段。
func convertLuckyProxy(raw map[string]any, unsupported map[string]int) (Proxy, string, []string) {
	str := func(k string) string {
		s, _ := raw[k].(string)
		return strings.TrimSpace(s)
	}
	num := func(k string) int {
		f, _ := raw[k].(float64)
		return int(f)
	}
	px := Proxy{
		Name:       str("name"),
		Type:       str("type"),
		Enable:     true,
		LocalIP:    str("localIP"),
		LocalPort:  num("localPort"),
		RemotePort: num("remotePort"),
		Subdomain:  str("subdomain"),
		SecretKey:  str("secretKey"),
	}
	if d, _ := raw["disabled"].(bool); d {
		px.Enable = false
	}
	if e, ok := raw["enabled"].(bool); ok && !e {
		px.Enable = false
	}
	if list, ok := raw["customDomains"].([]any); ok {
		for _, it := range list {
			if s, ok := it.(string); ok {
				px.CustomDomains = append(px.CustomDomains, s)
			}
		}
	}
	// 只去掉写错的自定义域名，返回给调用方提示
	var bad []string
	px.CustomDomains, bad = splitCustomDomains(px.CustomDomains)
	if plugin := str("plugin"); plugin != "" {
		return px, fmt.Sprintf("插件 %s 暂不支持", plugin), bad
	}
	if err := px.Validate(); err != nil {
		return px, err.Error(), bad
	}
	for k, v := range raw {
		if !luckyProxyKeys[k] && k != "plugin" && !isZero(v) {
			unsupported[k]++
		}
	}
	return px, "", bad
}

// isZero 判断 JSON 值是否为空值；对象与数组递归判断，Lucky 默认带出的空结构不产生提示。
func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case float64:
		return x == 0
	case string:
		return x == ""
	case []any:
		for _, it := range x {
			if !isZero(it) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, it := range x {
			if !isZero(it) {
				return false
			}
		}
		return true
	}
	return false
}

// importLuckyServer 把服务端实例作为新的 frps 实例写入 cfg。Gately 中已有相同连接端口的 frps 实例时跳过，避免端口冲突。
func importLuckyServer(cfg *fileConfig, li *luckyInstance, opts luckyimport.Options, rep *luckyimport.Report) (bool, error) {
	name := instanceName(li)
	var p luckyServerParams
	if len(li.Params) > 0 {
		if err := json.Unmarshal(li.Params, &p); err != nil {
			return false, fmt.Errorf("%s: %w", name, err)
		}
	}
	s := Server{
		Name:   name,
		Enable: li.Enable && opts.KeepEnabled,
		ServerSettings: ServerSettings{
			BindPort:       p.BindPort,
			Token:          p.Token,
			VhostHTTPPort:  p.VhostHTTPPort,
			VhostHTTPSPort: p.VhostHTTPSPort,
			SubdomainHost:  p.SubDomainHost,
			AllowPorts:     strings.ReplaceAll(strings.TrimSpace(p.AllowPorts), " ", ""),
		},
	}
	if _, err := ParseAllowPorts(s.AllowPorts); err != nil {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: %v，已改为不限制", name, err))
		s.AllowPorts = ""
	}
	// Lucky 的 frps 代理端口与连接端口监听同一地址；0.0.0.0 与 :: 即所有地址，与 Gately 留空一致。
	// 不是 IP 的地址（如主机名）frps 校验不通过，只提示并改为监听所有地址，避免整个实例被跳过
	switch addr := strings.Trim(strings.TrimSpace(p.BindAddr), "[]"); {
	case addr == "" || addr == "0.0.0.0" || addr == "::":
	case net.ParseIP(addr) == nil:
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 监听地址 %s 不是 IP 地址，frps 将监听所有地址", name, p.BindAddr))
	default:
		s.BindAddr, s.ProxyBindAddr = addr, addr
	}
	if p.KCPBindPort != 0 || p.QUICBindPort != 0 {
		rep.Warnings = append(rep.Warnings, name+": KCP / QUIC 监听端口暂不支持，已忽略")
	}
	if p.TLSOnly {
		rep.Warnings = append(rep.Warnings, name+": 仅允许 TLS 连接暂不支持，已忽略")
	}
	if p.DashboardPort != 0 {
		// Lucky 的 frps 控制台账号由 Lucky 管理，配置中没有可迁移的密码
		rep.Warnings = append(rep.Warnings, name+": 控制台未迁移，需要时请在 Gately 中设置端口与账号密码")
	}
	if err := s.Validate(); err != nil {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 跳过，%v", name, err))
		return false, nil
	}
	for _, cur := range cfg.Servers {
		if cur.BindPort == s.BindPort {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 跳过，Gately 中的 frps 实例 %s 已使用端口 %d", name, cur.Name, s.BindPort))
			return false, nil
		}
	}
	s.Key = crud.NewKey()
	cfg.Servers = append(cfg.Servers, s)
	rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: frps 监听端口 %d", name, s.BindPort))
	return true, nil
}
