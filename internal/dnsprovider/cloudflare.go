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
			{Key: "proxied", Label: "代理状态", Placeholder: "true 或 false；留空不管理已有记录的代理状态，新建记录默认 false"},
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

// cfHTTPError 是带 Cloudflare 错误信息的 HTTP 失败，Unwrap 后可得到 *HTTPError。
type cfHTTPError struct {
	*HTTPError
	msg string
}

func (e *cfHTTPError) Error() string {
	return fmt.Sprintf("Cloudflare: HTTP %d: %s", e.Status, e.msg)
}

func (e *cfHTTPError) Unwrap() error {
	return e.HTTPError
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
	isHTTP := errors.As(err, &he)
	if isHTTP {
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
		if isHTTP {
			// 带上 HTTP 状态码便于判断是否为瞬时 5xx，并保留 *HTTPError 供上层判断是否可重试
			return &cfHTTPError{HTTPError: he, msg: strings.Join(msgs, "; ")}
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

// proxiedSetting 解析额外字段“代理状态”：managed 表示填写了 true/false 并需要同步已有记录，
// desired 为期望的代理状态；留空（或无法识别的值）时不管理已有记录，新建记录按 false 处理。
func (c *cloudflare) proxiedSetting() (managed, desired bool) {
	switch strings.ToLower(strings.TrimSpace(c.creds.Extra["proxied"])) {
	case "true":
		return true, true
	case "false":
		return true, false
	}
	return false, false
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
	if len(recs) > 0 {
		// 同名同类型的多条记录（如轮询解析）最终都解析到同一个目标值。Cloudflare 不允许同名同类型同值的
		// 重复记录（81058 identical record already exists），所以只保留一条改成目标值：优先已经是目标值的那条，
		// 否则取第一条；其余旧值记录删除，解析结果与逐条改成目标值相同
		keep := 0
		for i, x := range recs {
			if x.Content == r.Value {
				keep = i
				break
			}
		}
		recs[0], recs[keep] = recs[keep], recs[0]
		managed, desired := c.proxiedSetting()
		updated := false
		for i, cur := range recs {
			if i > 0 {
				if cur.Content == r.Value {
					// 已是目标值的其他记录（理论上 Cloudflare 不会出现）不动
					continue
				}
				if err := c.call(ctx, http.MethodDelete, "/zones/"+zid+"/dns_records/"+cur.ID, nil, nil); err != nil {
					return "", err
				}
				updated = true
				continue
			}
			// 生效后的代理状态：配置了代理状态则以配置为准，否则沿用记录现状
			proxied := cur.Proxied
			if managed {
				proxied = desired
			}
			ttl := r.TTL
			if ttl <= 0 || proxied {
				// 未配置 TTL 时用 1（自动）；开启代理的记录 TTL 只能是自动（1），否则 Cloudflare 会拒绝
				ttl = 1
			}
			proxiedChanged := managed && cur.Proxied != desired
			// 未配置 TTL 时只按记录值判断是否变化；代理记录读回的 TTL 恒为 1，不参与比较，避免每轮重复更新
			ttlSame := r.TTL <= 0 || proxied || cur.TTL == ttl
			if cur.Content == r.Value && ttlSame && !proxiedChanged && (r.Remark == "" || cur.Comment == r.Remark) {
				continue
			}
			// PATCH 只改给出的字段，标签和未由 Gately 管理的备注保持不变
			upd := map[string]any{"content": r.Value, "ttl": ttl}
			if managed {
				upd["proxied"] = desired
			}
			if r.Remark != "" {
				upd["comment"] = r.Remark
			}
			if err := c.call(ctx, http.MethodPatch, "/zones/"+zid+"/dns_records/"+cur.ID, upd, nil); err != nil {
				return "", err
			}
			updated = true
		}
		if updated {
			return ActionUpdated, nil
		}
		return ActionUnchanged, nil
	}
	_, proxied := c.proxiedSetting()
	ttl := r.TTL
	if ttl <= 0 || proxied {
		// 1 表示自动；开启代理时只能用自动
		ttl = 1
	}
	rec := cfRecord{Type: r.Type, Name: r.FQDN(), Content: r.Value, TTL: ttl, Proxied: proxied, Comment: r.Remark}
	if err := c.call(ctx, http.MethodPost, "/zones/"+zid+"/dns_records", rec, nil); err != nil {
		return "", err
	}
	return ActionCreated, nil
}
