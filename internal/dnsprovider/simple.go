package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/webhook"
)

// 本文件包含接口较简单的服务商：GoDaddy、Porkbun、dynv6、DuckDNS 与自定义回调。

func init() {
	register(Meta{
		Name:        "godaddy",
		Title:       "GoDaddy",
		IDLabel:     "API Key",
		SecretLabel: "API Secret",
		DocURL:      "https://developer.godaddy.com/keys",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &godaddy{key: c.ID, secret: c.Secret, hc: hc, endpoint: "https://api.godaddy.com"}, nil
	})
	register(Meta{
		Name:        "porkbun",
		Title:       "Porkbun",
		IDLabel:     "API Key",
		SecretLabel: "Secret API Key",
		DocURL:      "https://porkbun.com/account/api",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &porkbun{key: c.ID, secret: c.Secret, hc: hc, endpoint: "https://api.porkbun.com/api/json/v3"}, nil
	})
	register(Meta{
		Name:        "dynv6",
		Title:       "dynv6",
		SecretLabel: "HTTP Token",
		DocURL:      "https://dynv6.com/keys",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &dynv6{token: c.Secret, hc: hc, endpoint: "https://dynv6.com/api/v2"}, nil
	})
	register(Meta{
		Name:        "duckdns",
		Title:       "DuckDNS",
		SecretLabel: "Token",
		DocURL:      "https://www.duckdns.org/",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &duckdns{token: c.Secret, hc: hc, endpoint: "https://www.duckdns.org/update"}, nil
	})
	register(Meta{
		Name:  "callback",
		Title: "自定义回调",
		Extra: []Field{
			{Key: "url", Label: "回调地址", Placeholder: "https://example.com/ddns?domain={domain}&ip={ip}", Required: true},
			{Key: "method", Label: "请求方法", Placeholder: "GET"},
			{Key: "headers", Label: "请求头", Placeholder: "每行一个，格式 Key: Value", Multiline: true},
			{Key: "body", Label: "请求体", Placeholder: "可用变量 {ip} {domain} {subDomain} {zone} {recordType} {ttl} {timestamp}", Multiline: true},
			{Key: "successContent", Label: "成功标识", Placeholder: "每行一个，响应包含任意一项即成功；留空只检查状态码", Multiline: true},
		},
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		cfg := webhook.Config{
			Enable:         true,
			URL:            c.Extra["url"],
			Method:         c.Extra["method"],
			Headers:        splitLines(c.Extra["headers"]),
			Body:           c.Extra["body"],
			SuccessContent: splitLines(c.Extra["successContent"]),
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return &callback{cfg: cfg, id: c.ID, secret: c.Secret}, nil
	})
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// ---- GoDaddy ----

type godaddy struct {
	key, secret string
	hc          *http.Client
	endpoint    string
}

func (g *godaddy) Upsert(ctx context.Context, r Record) (string, error) {
	path := fmt.Sprintf("%s/v1/domains/%s/records/%s/%s", g.endpoint, r.Zone, r.Type, url.PathEscape(r.RR()))
	req, err := ctxReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "sso-key "+g.key+":"+g.secret)
	var cur []struct {
		Data string `json:"data"`
		TTL  int    `json:"ttl"`
	}
	if err := doJSON(g.hc, req, &cur); err != nil {
		return "", fmt.Errorf("GoDaddy: %w", err)
	}
	if len(cur) == 1 && cur[0].Data == r.Value && (r.TTL == 0 || cur[0].TTL == r.TTL) {
		return ActionUnchanged, nil
	}
	ttl := r.TTL
	// GoDaddy 最小 TTL 为 600
	if ttl < 600 {
		ttl = 600
	}
	req, err = ctxReq(ctx, http.MethodPut, path, jsonBody([]map[string]any{{"data": r.Value, "ttl": ttl}}))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "sso-key "+g.key+":"+g.secret)
	req.Header.Set("Content-Type", "application/json")
	if err := doJSON(g.hc, req, nil); err != nil {
		return "", fmt.Errorf("GoDaddy: %w", err)
	}
	if len(cur) == 0 {
		return ActionCreated, nil
	}
	return ActionUpdated, nil
}

// ---- Porkbun ----

type porkbun struct {
	key, secret string
	hc          *http.Client
	endpoint    string
}

type porkbunResp struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Records []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		TTL     string `json:"ttl"`
	} `json:"records"`
}

func (p *porkbun) post(ctx context.Context, path string, extra map[string]any) (*porkbunResp, error) {
	body := map[string]any{"apikey": p.key, "secretapikey": p.secret}
	for k, v := range extra {
		body[k] = v
	}
	req, err := ctxReq(ctx, http.MethodPost, p.endpoint+path, jsonBody(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var out porkbunResp
	err = doJSON(p.hc, req, &out)
	if he, ok := err.(*HTTPError); ok {
		_ = json.Unmarshal([]byte(he.Body), &out)
		if out.Message != "" {
			return nil, fmt.Errorf("Porkbun: %s", out.Message)
		}
	}
	if err != nil {
		return nil, err
	}
	if out.Status != "SUCCESS" {
		return nil, fmt.Errorf("Porkbun: %s", out.Message)
	}
	return &out, nil
}

func (p *porkbun) Upsert(ctx context.Context, r Record) (string, error) {
	sub := r.Sub
	if sub == "@" {
		sub = ""
	}
	suffix := "/" + r.Zone + "/" + r.Type
	if sub != "" {
		suffix += "/" + url.PathEscape(sub)
	}
	cur, err := p.post(ctx, "/dns/retrieveByNameType"+suffix, nil)
	if err != nil {
		return "", err
	}
	ttl := r.TTL
	// Porkbun 最小 TTL 为 600
	if ttl < 600 {
		ttl = 600
	}
	if len(cur.Records) > 0 {
		if cur.Records[0].Content == r.Value && (r.TTL == 0 || cur.Records[0].TTL == strconv.Itoa(ttl)) {
			return ActionUnchanged, nil
		}
		if _, err := p.post(ctx, "/dns/editByNameType"+suffix, map[string]any{"content": r.Value, "ttl": strconv.Itoa(ttl)}); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if _, err := p.post(ctx, "/dns/create/"+r.Zone, map[string]any{"name": sub, "type": r.Type, "content": r.Value, "ttl": strconv.Itoa(ttl)}); err != nil {
		return "", err
	}
	return ActionCreated, nil
}

// ---- dynv6 ----

type dynv6 struct {
	token    string
	hc       *http.Client
	endpoint string
}

func (d *dynv6) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		rd = jsonBody(body)
	}
	req, err := ctxReq(ctx, method, d.endpoint+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := doJSON(d.hc, req, out); err != nil {
		return fmt.Errorf("dynv6: %w", err)
	}
	return nil
}

// resolveZoneInfo 返回 zone 信息与记录名（根记录为空串）。
// dynv6 的 zone 是用户自己的完整域名（如 my.dynv6.net）。主域名填成 dynv6.net、子域名填 my 时
// zone 查询会 404，此时把子域名最右侧的标签逐级并入 zone 重试。
func (d *dynv6) resolveZoneInfo(ctx context.Context, zoneName, sub string) (dynv6Zone, string, error) {
	name := sub
	if name == "@" {
		name = ""
	}
	var firstErr error
	for {
		z, err := d.zoneInfo(ctx, zoneName)
		if err == nil {
			return z, name, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		var he *HTTPError
		if !errors.As(err, &he) || he.Status != http.StatusNotFound || name == "" {
			return dynv6Zone{}, "", firstErr
		}
		if i := strings.LastIndex(name, "."); i >= 0 {
			zoneName, name = name[i+1:]+"."+zoneName, name[:i]
		} else {
			zoneName, name = name+"."+zoneName, ""
		}
	}
}

// resolveZone 返回 zone 的记录路径与记录名（根记录为空串）。
func (d *dynv6) resolveZone(ctx context.Context, zoneName, sub string) (string, string, error) {
	z, name, err := d.resolveZoneInfo(ctx, zoneName, sub)
	if err != nil {
		return "", "", err
	}
	return "/zones/" + strconv.FormatInt(z.ID, 10) + "/records", name, nil
}

func (d *dynv6) Upsert(ctx context.Context, r Record) (string, error) {
	zone, name, err := d.resolveZoneInfo(ctx, r.Zone, r.Sub)
	if err != nil {
		return "", err
	}
	zonePath := "/zones/" + strconv.FormatInt(zone.ID, 10)
	if name == "" {
		// 根域名通过 PATCH zone 更新（A 对应 ipv4address，AAAA 对应 ipv6prefix）
		cur, field := zone.IPv4, "ipv4address"
		if r.Type == "AAAA" {
			cur, field = zone.IPv6, "ipv6prefix"
		}
		if cur == r.Value {
			return ActionUnchanged, nil
		}
		if err := d.do(ctx, http.MethodPatch, zonePath, map[string]string{field: r.Value}, nil); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	zpath := zonePath + "/records"
	var recs []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if err := d.do(ctx, http.MethodGet, zpath, nil, &recs); err != nil {
		return "", err
	}
	for _, rec := range recs {
		if rec.Name != name || rec.Type != r.Type {
			continue
		}
		if rec.Data == r.Value {
			return ActionUnchanged, nil
		}
		if err := d.do(ctx, http.MethodPatch, zpath+"/"+strconv.FormatInt(rec.ID, 10), map[string]string{"data": r.Value}, nil); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if err := d.do(ctx, http.MethodPost, zpath, map[string]string{"name": name, "type": r.Type, "data": r.Value}, nil); err != nil {
		return "", err
	}
	return ActionCreated, nil
}

// ---- DuckDNS ----

type duckdns struct {
	token    string
	hc       *http.Client
	endpoint string
}

// Upsert 调用更新接口。DuckDNS 不提供查询，重复提交由调用方的缓存避免。
func (d *duckdns) Upsert(ctx context.Context, r Record) (string, error) {
	// DuckDNS 的主域名固定为 duckdns.org，子域名即账号下的域名
	domain := strings.TrimSuffix(r.FQDN(), ".duckdns.org")
	q := url.Values{"domains": {domain}, "token": {d.token}}
	if r.Type == "AAAA" {
		q.Set("ipv6", r.Value)
	} else {
		q.Set("ip", r.Value)
	}
	req, err := ctxReq(ctx, http.MethodGet, d.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if strings.TrimSpace(string(body)) != "OK" {
		return "", fmt.Errorf("DuckDNS 返回 %q", strings.TrimSpace(string(body)))
	}
	return ActionUpdated, nil
}

// ---- 自定义回调 ----

type callback struct {
	cfg        webhook.Config
	id, secret string
}

// Upsert 调用用户配置的回调地址，可用变量：{ip} {recordType} {domain} {subDomain} {zone} {ttl} {timestamp}（当前 Unix 秒）{id} {secret}。
func (c *callback) Upsert(ctx context.Context, r Record) (string, error) {
	// 未配置 TTL 时 {ttl} 取 600
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 600
	}
	vars := map[string]string{
		"ip":         r.Value,
		"recordType": r.Type,
		"domain":     r.FQDN(),
		"subDomain":  r.RR(),
		"zone":       r.Zone,
		"ttl":        strconv.Itoa(ttl),
		"timestamp":  strconv.FormatInt(time.Now().Unix(), 10),
		"id":         c.id,
		"secret":     c.secret,
	}
	if _, err := webhook.Send(ctx, c.cfg, vars); err != nil {
		return "", fmt.Errorf("回调失败: %w", err)
	}
	return ActionUpdated, nil
}
