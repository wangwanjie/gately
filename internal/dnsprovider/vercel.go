package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Vercel DNS：https://vercel.com/docs/rest-api/reference/endpoints/dns
// 凭据 Secret 为 Access Token；团队账号下的域名需要在额外配置中填写团队 ID（teamId）。

func init() {
	register(Meta{
		Name:        "vercel",
		Title:       "Vercel",
		SecretLabel: "Access Token",
		Extra: []Field{
			{Key: "teamId", Label: "团队 ID（可选）", Placeholder: "域名在团队账号下时填写，形如 team_xxxx"},
		},
		DocURL:         "https://vercel.com/account/settings/tokens",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &vercel{token: strings.TrimSpace(c.Secret), team: strings.TrimSpace(c.Extra["teamId"]), hc: hc, endpoint: "https://api.vercel.com"}, nil
	})
}

type vercel struct {
	token string
	// team 为团队 ID，空表示个人账号
	team     string
	hc       *http.Client
	endpoint string
}

type vercelRecord struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

// vercelName 把主机记录转成 Vercel 的写法：根域名为空字符串。
func vercelName(sub string) string {
	if sub == "@" {
		return ""
	}
	return sub
}

// vercelTTL 返回提交给 Vercel 的 TTL：0 表示不指定，最小 60。
func vercelTTL(ttl int) int {
	if ttl > 0 && ttl < 60 {
		return 60
	}
	return ttl
}

func (v *vercel) call(ctx context.Context, method, path string, q url.Values, body, out any) error {
	if q == nil {
		q = url.Values{}
	}
	if v.team != "" {
		q.Set("teamId", v.team)
	}
	u := v.endpoint + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var req *http.Request
	var err error
	if body != nil {
		req, err = ctxReq(ctx, method, u, jsonBody(body))
	} else {
		req, err = ctxReq(ctx, method, u, nil)
	}
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	err = doJSON(v.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		// 错误体形如 {"error":{"code":"forbidden","message":"..."}}
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(he.Body), &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("Vercel: HTTP %d: %s (%s)", he.Status, e.Error.Message, e.Error.Code)
		}
	}
	if err != nil {
		return fmt.Errorf("Vercel: %w", err)
	}
	return nil
}

// list 返回域名下的全部记录。接口默认每页 20 条，这里每页取 100 条，按 pagination.next 时间戳翻页。
func (v *vercel) list(ctx context.Context, zone string) ([]vercelRecord, error) {
	var all []vercelRecord
	q := url.Values{"limit": {"100"}}
	lastUntil := ""
	for page := 0; page < 200; page++ {
		var resp struct {
			Records    []vercelRecord `json:"records"`
			Pagination *struct {
				Next *json.Number `json:"next"`
			} `json:"pagination"`
		}
		if err := v.call(ctx, http.MethodGet, "/v5/domains/"+url.PathEscape(zone)+"/records", q, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Records...)
		if len(resp.Records) == 0 || resp.Pagination == nil || resp.Pagination.Next == nil {
			break
		}
		next := resp.Pagination.Next.String()
		if next == "" || next == lastUntil {
			break
		}
		lastUntil = next
		q.Set("until", next)
	}
	return all, nil
}

// match 筛选出主机记录与类型都相同的记录。
func (v *vercel) match(recs []vercelRecord, sub, typ string) []vercelRecord {
	want := vercelName(sub)
	var out []vercelRecord
	for _, rec := range recs {
		if rec.Type == typ && strings.EqualFold(vercelName(rec.Name), want) {
			out = append(out, rec)
		}
	}
	return out
}

func (v *vercel) Upsert(ctx context.Context, r Record) (string, error) {
	recs, err := v.list(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	ttl := vercelTTL(r.TTL)
	cur := v.match(recs, r.RR(), r.Type)
	if len(cur) == 0 {
		body := map[string]any{"name": vercelName(r.RR()), "type": r.Type, "value": r.Value, "comment": "Created by Gately"}
		if ttl > 0 {
			body["ttl"] = ttl
		}
		if err := v.call(ctx, http.MethodPost, "/v2/domains/"+url.PathEscape(r.Zone)+"/records", nil, body, nil); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	// 优先使用值相同的记录，其次取第一条
	target := cur[0]
	for _, rec := range cur {
		if strings.EqualFold(rec.Value, r.Value) {
			target = rec
			break
		}
	}
	if strings.EqualFold(target.Value, r.Value) && (ttl == 0 || target.TTL == ttl) {
		return ActionUnchanged, nil
	}
	body := map[string]any{"type": r.Type, "value": r.Value}
	if ttl > 0 {
		body["ttl"] = ttl
	}
	if err := v.call(ctx, http.MethodPatch, "/v1/domains/records/"+url.PathEscape(target.ID), nil, body, nil); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (v *vercel) AddTXT(ctx context.Context, zone, sub, value string) error {
	body := map[string]any{"name": vercelName(sub), "type": "TXT", "value": value, "ttl": txtTTL}
	return v.call(ctx, http.MethodPost, "/v2/domains/"+url.PathEscape(zone)+"/records", nil, body, nil)
}

func (v *vercel) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	recs, err := v.list(ctx, zone)
	if err != nil {
		return err
	}
	for _, rec := range v.match(recs, sub, "TXT") {
		if unquote(rec.Value) == value {
			if err := v.deleteByID(ctx, zone, rec.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *vercel) deleteByID(ctx context.Context, zone, id string) error {
	return v.call(ctx, http.MethodDelete, "/v2/domains/"+url.PathEscape(zone)+"/records/"+url.PathEscape(id), nil, nil, nil)
}

func (v *vercel) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := v.list(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range v.match(recs, r.RR(), r.Type) {
		out = append(out, remoteRecord{id: rec.ID, values: []string{rec.Value}, ttl: rec.TTL})
	}
	return out, nil
}

func (v *vercel) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	return v.deleteByID(ctx, r.Zone, rec.id)
}

var (
	_ TXTProvider   = (*vercel)(nil)
	_ recordDeleter = (*vercel)(nil)
)
