package cloudflared

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/outbound"
)

// 令牌隧道的路由规则（ingress）保存在 Cloudflare 的远程配置中，cloudflared 运行时自动拉取，
// 修改后无需重启进程。这里通过 Cloudflare API 读写远程配置，并可为公网主机名创建指向隧道的 CNAME 记录。
// 文档：https://developers.cloudflare.com/api/resources/zero_trust/subresources/tunnels/subresources/cloudflared/subresources/configurations/

// cfEndpoint 为 Cloudflare API 地址，测试时替换为本地服务。
var cfEndpoint = "https://api.cloudflare.com/client/v4"

// defaultCatchAll 为未匹配任何规则时的默认服务。
const defaultCatchAll = "http_status:404"

// IngressRule 是一条路由规则。OriginRequest 为回源设置，界面只编辑常用项，其余字段原样保留。
type IngressRule struct {
	Hostname      string         `json:"hostname,omitempty"`
	Path          string         `json:"path,omitempty"`
	Service       string         `json:"service"`
	OriginRequest map[string]any `json:"originRequest,omitempty"`
}

// tokenInfo 是隧道令牌中的账号与隧道标识。
type tokenInfo struct {
	AccountTag string `json:"a"`
	TunnelID   string `json:"t"`
}

// parseToken 从隧道令牌解析账号与隧道 ID。令牌为 base64 编码的 JSON：{"a":账号,"t":隧道ID,"s":密钥}。
func parseToken(token string) (tokenInfo, error) {
	token = strings.TrimSpace(token)
	var raw []byte
	var err error
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if raw, err = enc.DecodeString(token); err == nil {
			break
		}
	}
	var info tokenInfo
	if err != nil || json.Unmarshal(raw, &info) != nil || info.AccountTag == "" || info.TunnelID == "" {
		return info, errors.New("无法从隧道令牌解析账号与隧道 ID，请确认令牌完整")
	}
	return info, nil
}

var (
	hostnameRe   = regexp.MustCompile(`^(\*\.)?([a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?\.)+[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])$`)
	httpStatusRe = regexp.MustCompile(`^http_status:[1-5][0-9]{2}$`)
)

// normalizeService 去掉 HTTP 类服务地址末尾单独的 /：cloudflared 要求 HTTP 类服务不带任何路径，
// 带 / 的地址能写入 Cloudflare，但运行中的 cloudflared 会拒绝整份远程配置。
func normalizeService(s string) string {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return s
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
		u.Path = ""
		return u.String()
	}
	return s
}

// validateService 校验回源服务地址，规则与 cloudflared 一致：HTTP 类服务不能带路径，TCP 类服务必须带端口。
func validateService(s string) error {
	switch {
	case s == "hello_world" || s == "bastion" || httpStatusRe.MatchString(s):
		return nil
	case strings.HasPrefix(s, "unix:") || strings.HasPrefix(s, "unix+tls:"):
		if strings.TrimPrefix(strings.TrimPrefix(s, "unix+tls:"), "unix:") == "" {
			return fmt.Errorf("服务 %s 缺少套接字路径", s)
		}
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return fmt.Errorf("服务 %s 格式不正确，示例：http://localhost:8080、ssh://localhost:22、http_status:404", s)
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
		if u.Path != "" {
			return fmt.Errorf("服务 %s 不能带路径，路由规则只按路径匹配，不改写回源路径", s)
		}
	case "tcp", "ssh", "rdp", "smb":
		if u.Port() == "" {
			return fmt.Errorf("服务 %s 需要填写端口", s)
		}
	default:
		return fmt.Errorf("服务 %s 的协议不受支持，可用 http、https、tcp、ssh、rdp、smb、unix", s)
	}
	return nil
}

// normalizeRules 校验并规范化路由规则（不含兜底规则）。每条规则至少填写主机名或路径，
// 否则会匹配全部请求，使后面的规则失效。
func normalizeRules(rules []IngressRule) error {
	for i := range rules {
		r := &rules[i]
		// 主机名去掉末尾的根域点，与 Cloudflare 的记录名写法一致
		r.Hostname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(r.Hostname)), ".")
		r.Path = strings.TrimSpace(r.Path)
		r.Service = normalizeService(r.Service)
		if r.Hostname == "" && r.Path == "" {
			return fmt.Errorf("第 %d 条规则需要填写主机名或路径", i+1)
		}
		if r.Hostname != "" && !hostnameRe.MatchString(r.Hostname) {
			return fmt.Errorf("第 %d 条规则的主机名 %s 格式不正确", i+1, r.Hostname)
		}
		if r.Path != "" {
			if _, err := regexp.Compile(r.Path); err != nil {
				return fmt.Errorf("第 %d 条规则的路径不是有效的正则表达式: %v", i+1, err)
			}
		}
		if r.Service == "" {
			return fmt.Errorf("第 %d 条规则需要填写服务", i+1)
		}
		if err := validateService(r.Service); err != nil {
			return fmt.Errorf("第 %d 条规则: %w", i+1, err)
		}
		if len(r.OriginRequest) == 0 {
			r.OriginRequest = nil
		}
	}
	return nil
}

// cfClient 是 Cloudflare API 客户端。
type cfClient struct {
	token string
	hc    *http.Client
}

func newCFClient(apiToken string) *cfClient {
	return &cfClient{token: apiToken, hc: outbound.NewClient(30 * time.Second)}
}

func (c *cfClient) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfEndpoint+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("访问 Cloudflare API 失败: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("访问 Cloudflare API 失败: %v", err)
	}
	var env struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("Cloudflare API 返回 HTTP %d", resp.StatusCode)
	}
	if !env.Success {
		var msgs []string
		for _, e := range env.Errors {
			msgs = append(msgs, strconv.Itoa(e.Code)+" "+e.Message)
		}
		if len(msgs) == 0 {
			msgs = append(msgs, "HTTP "+strconv.Itoa(resp.StatusCode))
		}
		return fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// remoteConfig 是隧道远程配置。Config 用通用结构保存，写回时只替换 ingress，保留 warp-routing、originRequest 等其他字段。
type remoteConfig struct {
	Version int            `json:"version"`
	Source  string         `json:"source"`
	Config  map[string]any `json:"config"`
}

func configPath(info tokenInfo) string {
	return "/accounts/" + url.PathEscape(info.AccountTag) + "/cfd_tunnel/" + url.PathEscape(info.TunnelID) + "/configurations"
}

func (c *cfClient) getConfig(ctx context.Context, info tokenInfo) (*remoteConfig, error) {
	var rc remoteConfig
	if err := c.call(ctx, http.MethodGet, configPath(info), nil, &rc); err != nil {
		return nil, err
	}
	if rc.Config == nil {
		rc.Config = map[string]any{}
	}
	return &rc, nil
}

// splitIngress 把远程配置的 ingress 拆成普通规则与兜底规则。兜底规则为最后一条既无主机名也无路径的规则。
func splitIngress(cfg map[string]any) (rules []IngressRule, catchAll IngressRule, err error) {
	catchAll = IngressRule{Service: defaultCatchAll}
	raw, ok := cfg["ingress"]
	if !ok || raw == nil {
		return nil, catchAll, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, catchAll, err
	}
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, catchAll, fmt.Errorf("无法解析隧道的路由规则: %v", err)
	}
	if n := len(rules); n > 0 && rules[n-1].Hostname == "" && rules[n-1].Path == "" {
		catchAll = rules[n-1]
		rules = rules[:n-1]
	}
	return rules, catchAll, nil
}

func (c *cfClient) putConfig(ctx context.Context, info tokenInfo, cfg map[string]any) (*remoteConfig, error) {
	var rc remoteConfig
	if err := c.call(ctx, http.MethodPut, configPath(info), map[string]any{"config": cfg}, &rc); err != nil {
		return nil, err
	}
	return &rc, nil
}

// DNS 记录同步结果。
const (
	dnsCreated   = "created"
	dnsUnchanged = "unchanged"
	dnsConflict  = "conflict"
	dnsFailed    = "failed"
)

// DNSResult 是一个主机名的 DNS 记录同步结果。Existing 为冲突时已有记录的类型与内容。
type DNSResult struct {
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	Existing string `json:"existing,omitempty"`
	Error    string `json:"error,omitempty"`
}

type cfDNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

// zoneFor 从最长的后缀开始查找主机名所属的域名（zone），返回 zone ID。
func (c *cfClient) zoneFor(ctx context.Context, account, host string, cache map[string]string) (string, error) {
	labels := strings.Split(strings.TrimPrefix(host, "*."), ".")
	for i := 0; i+2 <= len(labels); i++ {
		name := strings.Join(labels[i:], ".")
		if id, ok := cache[name]; ok {
			if id == "" {
				continue
			}
			return id, nil
		}
		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := c.call(ctx, http.MethodGet, "/zones?name="+url.QueryEscape(name)+"&account.id="+url.QueryEscape(account), nil, &zones); err != nil {
			return "", err
		}
		cache[name] = ""
		for _, z := range zones {
			if strings.EqualFold(z.Name, name) {
				cache[name] = z.ID
				return z.ID, nil
			}
		}
	}
	return "", fmt.Errorf("API 令牌可访问的域名中没有 %s，请确认域名托管在 Cloudflare 且令牌有该域名的 DNS 编辑权限", host)
}

// ensureCNAME 确保主机名有指向隧道的 CNAME 记录：没有记录时创建（开启代理），
// 已是指向本隧道的 CNAME 时不变，已有其他记录时不覆盖，返回冲突。
func (c *cfClient) ensureCNAME(ctx context.Context, account, host, target string, cache map[string]string) DNSResult {
	res := DNSResult{Hostname: host}
	// 只在隧道所属账号下查找域名，令牌能访问多个账号时不会找到别的账号的同名域名
	zid, err := c.zoneFor(ctx, account, host, cache)
	if err != nil {
		res.Status, res.Error = dnsFailed, err.Error()
		return res
	}
	var recs []cfDNSRecord
	if err := c.call(ctx, http.MethodGet, "/zones/"+url.PathEscape(zid)+"/dns_records?name="+url.QueryEscape(host), nil, &recs); err != nil {
		res.Status, res.Error = dnsFailed, err.Error()
		return res
	}
	for _, r := range recs {
		if !strings.EqualFold(r.Name, host) {
			continue
		}
		if r.Type == "CNAME" && strings.EqualFold(strings.TrimSuffix(r.Content, "."), target) {
			if r.Proxied {
				res.Status = dnsUnchanged
				return res
			}
			// 未开启代理的 CNAME 只解析到隧道域名，流量不会进入隧道，按冲突报告、不擅自修改
			res.Status, res.Existing = dnsConflict, r.Type+" "+r.Content+" (DNS only)"
			return res
		}
		res.Status, res.Existing = dnsConflict, r.Type+" "+r.Content
		return res
	}
	rec := cfDNSRecord{Type: "CNAME", Name: host, Content: target, TTL: 1, Proxied: true, Comment: "Gately cloudflared"}
	if err := c.call(ctx, http.MethodPost, "/zones/"+url.PathEscape(zid)+"/dns_records", rec, nil); err != nil {
		res.Status, res.Error = dnsFailed, err.Error()
		return res
	}
	res.Status = dnsCreated
	return res
}

// syncDNS 为规则中的全部主机名同步 CNAME 记录，同一主机名只处理一次。
func (c *cfClient) syncDNS(ctx context.Context, info tokenInfo, rules []IngressRule) []DNSResult {
	target := info.TunnelID + ".cfargotunnel.com"
	cache := map[string]string{}
	seen := map[string]bool{}
	out := []DNSResult{}
	for _, r := range rules {
		if r.Hostname == "" || seen[r.Hostname] {
			continue
		}
		seen[r.Hostname] = true
		out = append(out, c.ensureCNAME(ctx, info.AccountTag, r.Hostname, target, cache))
	}
	return out
}

// ingressView 是路由规则接口的返回值。Source 为 cloudflare 表示远程管理；为 local 时隧道使用本地配置文件，
// 远程配置不生效。
type ingressView struct {
	TunnelID string        `json:"tunnelId"`
	Version  int           `json:"version"`
	Source   string        `json:"source"`
	Rules    []IngressRule `json:"rules"`
	CatchAll string        `json:"catchAll"`
	DNS      []DNSResult   `json:"dns,omitempty"`
}

// ingressUpdate 是保存路由规则的请求体。Version 为读取时的配置版本，用于发现其他地方的修改；
// SyncDNS 为 true 时为主机名创建指向隧道的 CNAME 记录。Rules 必须提供（可以是空数组），
// 缺省时拒绝，避免只想改兜底规则的调用把全部规则清空。Force 为 true 时才允许写入
// 本地配置文件管理（source 为 local）的隧道。
type ingressUpdate struct {
	Version  int            `json:"version"`
	Rules    *[]IngressRule `json:"rules"`
	Force    bool           `json:"force"`
	CatchAll string         `json:"catchAll"`
	SyncDNS  bool           `json:"syncDns"`
}

// ingressTarget 取出隧道的 API 客户端与令牌信息；调用方之后访问外网时不持有锁。
func (m *Module) ingressTarget(key string) (*cfClient, tokenInfo, string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(key)
	if t == nil {
		return nil, tokenInfo{}, "", http.StatusNotFound, errors.New("隧道不存在")
	}
	if t.Mode != ModeToken {
		return nil, tokenInfo{}, "", http.StatusBadRequest, errors.New("只有令牌隧道可以编辑路由规则")
	}
	if t.APIToken == "" {
		return nil, tokenInfo{}, "", http.StatusBadRequest, errors.New("请先在隧道设置中填写 Cloudflare API 令牌")
	}
	info, err := parseToken(t.Token)
	if err != nil {
		return nil, tokenInfo{}, "", http.StatusBadRequest, err
	}
	return newCFClient(t.APIToken), info, t.Name, 0, nil
}

func newIngressView(info tokenInfo, rc *remoteConfig) (*ingressView, error) {
	rules, catchAll, err := splitIngress(rc.Config)
	if err != nil {
		return nil, err
	}
	if rules == nil {
		rules = []IngressRule{}
	}
	return &ingressView{TunnelID: info.TunnelID, Version: rc.Version, Source: rc.Source, Rules: rules, CatchAll: catchAll.Service}, nil
}

func (m *Module) ingressRoutes(r module.Router) {
	r.HandleFunc("GET /tunnels/{key}/ingress", func(w http.ResponseWriter, req *http.Request) {
		c, info, _, code, err := m.ingressTarget(req.PathValue("key"))
		if err != nil {
			httpx.Error(w, code, "%v", err)
			return
		}
		rc, err := c.getConfig(req.Context(), info)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "读取路由规则失败: %v", err)
			return
		}
		v, err := newIngressView(info, rc)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "%v", err)
			return
		}
		httpx.OK(w, v)
	})
	r.HandleFunc("PUT /tunnels/{key}/ingress", func(w http.ResponseWriter, req *http.Request) {
		var body ingressUpdate
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if body.Rules == nil {
			httpx.Error(w, http.StatusBadRequest, "缺少 rules 字段，清空全部规则请传空数组")
			return
		}
		rules := *body.Rules
		if err := normalizeRules(rules); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		body.CatchAll = normalizeService(body.CatchAll)
		if body.CatchAll == "" {
			body.CatchAll = defaultCatchAll
		}
		if err := validateService(body.CatchAll); err != nil {
			httpx.Error(w, http.StatusBadRequest, "兜底规则: %v", err)
			return
		}
		c, info, name, code, err := m.ingressTarget(req.PathValue("key"))
		if err != nil {
			httpx.Error(w, code, "%v", err)
			return
		}
		ctx := req.Context()
		// 读取、核对版本、写入串行执行，避免 Gately 内两次并发保存互相覆盖
		m.ingressMu.Lock()
		defer m.ingressMu.Unlock()
		// 先读取当前配置：核对版本，并保留 ingress 以外的字段与兜底规则的回源设置
		cur, err := c.getConfig(ctx, info)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "读取路由规则失败: %v", err)
			return
		}
		if body.Version != cur.Version {
			httpx.Error(w, http.StatusConflict, "路由规则已在其他地方修改（版本 %d，当前 %d），请刷新后重试", body.Version, cur.Version)
			return
		}
		if cur.Source == "local" && !body.Force {
			httpx.Error(w, http.StatusConflict, "这条隧道由本地配置文件管理，写入远程配置会改变它的管理方式，确认后请带 force 重试")
			return
		}
		_, oldCatchAll, err := splitIngress(cur.Config)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "%v", err)
			return
		}
		ingress := append(append([]IngressRule{}, rules...), IngressRule{Service: body.CatchAll, OriginRequest: oldCatchAll.OriginRequest})
		cur.Config["ingress"] = ingress
		saved, err := c.putConfig(ctx, info, cur.Config)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "保存路由规则失败: %v", err)
			return
		}
		m.logs.Logger(name).Infof("已更新路由规则：%d 条，配置版本 %d", len(rules), saved.Version)
		v, err := newIngressView(info, saved)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "%v", err)
			return
		}
		if body.SyncDNS {
			v.DNS = c.syncDNS(ctx, info, rules)
			for _, d := range v.DNS {
				switch d.Status {
				case dnsCreated:
					m.logs.Logger(name).Infof("已创建 DNS 记录 %s CNAME %s.cfargotunnel.com", d.Hostname, info.TunnelID)
				case dnsConflict:
					m.logs.Logger(name).Warnf("%s 已有 DNS 记录 %s，未修改", d.Hostname, d.Existing)
				case dnsFailed:
					m.logs.Logger(name).Warnf("%s 的 DNS 记录同步失败: %s", d.Hostname, d.Error)
				}
			}
		}
		httpx.OK(w, v)
	})
}
