package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Cloudflare API v4。Secret 为 API Token；若 ID 填写账号邮箱，则 Secret 视为 Global API Key。
// ID 不是邮箱时忽略：Lucky 的 Cloudflare 只用 API Token，导入的配置里 ID 常残留其他服务商的 AccessKey，
// 按邮箱发送会被 Cloudflare 以 6003 Invalid request headers 拒绝。
// 文档：https://developers.cloudflare.com/api/resources/dns/subresources/records/
func init() {
	register(Meta{
		Name:        "cloudflare",
		Title:       "Cloudflare",
		IDLabel:     "账号邮箱",
		SecretLabel: "API Token / Global API Key",
		Extra: []Field{
			{Key: "proxied", Label: "新建时开启代理", Placeholder: "true 或 false，默认 false"},
		},
		DocURL:         "https://dash.cloudflare.com/profile/api-tokens",
		SupportsRemark: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &cloudflare{creds: c, hc: hc, endpoint: "https://api.cloudflare.com/client/v4"}, nil
	})
}

type cloudflare struct {
	creds    Credentials
	hc       *http.Client
	endpoint string
}

type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

type cfRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	// Comment 为记录备注，空时不发送
	Comment string `json:"comment,omitempty"`
}

func (c *cloudflare) call(ctx context.Context, method, path string, body any, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := ctxReq(ctx, method, c.endpoint+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.globalKey() {
		req.Header.Set("X-Auth-Email", c.creds.ID)
		req.Header.Set("X-Auth-Key", c.creds.Secret)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.creds.Secret)
	}
	var env cfEnvelope
	err = doJSON(c.hc, req, &env)
	var he *HTTPError
	if errors.As(err, &he) {
		_ = json.Unmarshal([]byte(he.Body), &env)
	} else if err != nil {
		return err
	}
	if !env.Success {
		var msgs []string
		for _, e := range env.Errors {
			msgs = append(msgs, fmt.Sprintf("%d %s", e.Code, e.Message))
		}
		if len(msgs) == 0 && err != nil {
			return err
		}
		return fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// globalKey 判断是否使用 Global API Key 认证：只有 ID 是邮箱时才是。
func (c *cloudflare) globalKey() bool {
	return strings.Contains(c.creds.ID, "@")
}

func (c *cloudflare) zoneID(ctx context.Context, zone string) (string, error) {
	var zones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.call(ctx, http.MethodGet, "/zones?name="+url.QueryEscape(zone), nil, &zones); err != nil {
		return "", err
	}
	for _, z := range zones {
		if strings.EqualFold(z.Name, zone) {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("Cloudflare 账号下未找到域名 %s", zone)
}

func (c *cloudflare) Upsert(ctx context.Context, r Record) (string, error) {
	zid, err := c.zoneID(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	var recs []cfRecord
	q := "?type=" + url.QueryEscape(r.Type) + "&name=" + url.QueryEscape(r.FQDN())
	if err := c.call(ctx, http.MethodGet, "/zones/"+zid+"/dns_records"+q, nil, &recs); err != nil {
		return "", err
	}
	ttl := r.TTL
	if ttl <= 0 {
		// 1 表示自动
		ttl = 1
	}
	if len(recs) > 0 {
		cur := recs[0]
		valueSame := cur.Content == r.Value && (r.TTL <= 0 || cur.TTL == ttl)
		if valueSame && (r.Remark == "" || cur.Comment == r.Remark) {
			return ActionUnchanged, nil
		}
		// PATCH 只改给出的字段，代理状态、标签和未由 Gately 管理的备注保持不变
		upd := map[string]any{"content": r.Value, "ttl": ttl}
		if r.Remark != "" {
			upd["comment"] = r.Remark
		}
		if err := c.call(ctx, http.MethodPatch, "/zones/"+zid+"/dns_records/"+cur.ID, upd, nil); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	rec := cfRecord{Type: r.Type, Name: r.FQDN(), Content: r.Value, TTL: ttl, Proxied: c.creds.Extra["proxied"] == "true", Comment: r.Remark}
	if err := c.call(ctx, http.MethodPost, "/zones/"+zid+"/dns_records", rec, nil); err != nil {
		return "", err
	}
	return ActionCreated, nil
}
