package dnsprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Name.com Core API：https://docs.name.com/
// 使用用户名与 API Token 做 HTTP Basic 认证；记录以整数 ID 标识。

func init() {
	register(Meta{
		Name:           "namecom",
		Title:          "Name.com",
		IDLabel:        "Username",
		SecretLabel:    "API Token",
		DocURL:         "https://www.name.com/account/settings/api",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &nameCom{user: strings.TrimSpace(c.ID), token: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://api.name.com"}, nil
	})
}

type nameCom struct {
	user, token string
	hc          *http.Client
	endpoint    string
}

type nameComRecord struct {
	ID     int    `json:"id,omitempty"`
	Host   string `json:"host"`
	Type   string `json:"type"`
	Answer string `json:"answer"`
	TTL    int    `json:"ttl,omitempty"`
}

// nameComTTL 返回提交的 TTL：0 表示不指定，最小 300。
func nameComTTL(ttl int) int {
	if ttl > 0 && ttl < 300 {
		return 300
	}
	return ttl
}

// nameComHost 返回主机记录：根域名用空字符串。
func nameComHost(sub string) string {
	if sub == "@" {
		return ""
	}
	return sub
}

func (n *nameCom) call(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := n.endpoint + path
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
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(n.user+":"+n.token)))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	err = doJSON(n.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		// 错误体形如 {"message":"...","details":"..."}
		var e struct {
			Message string `json:"message"`
			Details string `json:"details"`
		}
		if json.Unmarshal([]byte(he.Body), &e) == nil && e.Message != "" {
			msg := e.Message
			if e.Details != "" {
				msg += " (" + e.Details + ")"
			}
			return fmt.Errorf("Name.com: HTTP %d: %s", he.Status, msg)
		}
	}
	if err != nil {
		return fmt.Errorf("Name.com: %w", err)
	}
	return nil
}

func (n *nameCom) recordsPath(zone string) string {
	return "/v4/domains/" + url.PathEscape(zone) + "/records"
}

// list 返回域名下的全部记录，按 nextPage 翻页。
func (n *nameCom) list(ctx context.Context, zone string) ([]nameComRecord, error) {
	var all []nameComRecord
	page := 1
	for i := 0; i < 200; i++ {
		var resp struct {
			Records  []nameComRecord `json:"records"`
			NextPage int             `json:"nextPage"`
		}
		q := url.Values{"perPage": {"1000"}, "page": {strconv.Itoa(page)}}
		if err := n.call(ctx, http.MethodGet, n.recordsPath(zone), q, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Records...)
		if resp.NextPage <= page {
			break
		}
		page = resp.NextPage
	}
	return all, nil
}

func (n *nameCom) match(recs []nameComRecord, sub, typ string) []nameComRecord {
	want := nameComHost(sub)
	var out []nameComRecord
	for _, rec := range recs {
		if rec.Type == typ && strings.EqualFold(nameComHost(rec.Host), want) {
			out = append(out, rec)
		}
	}
	return out
}

func (n *nameCom) create(ctx context.Context, zone, sub, typ, value string, ttl int) error {
	body := nameComRecord{Host: nameComHost(sub), Type: typ, Answer: value, TTL: nameComTTL(ttl)}
	return n.call(ctx, http.MethodPost, n.recordsPath(zone), nil, body, nil)
}

func (n *nameCom) remove(ctx context.Context, zone string, id int) error {
	return n.call(ctx, http.MethodDelete, n.recordsPath(zone)+"/"+strconv.Itoa(id), nil, nil, nil)
}

func (n *nameCom) Upsert(ctx context.Context, r Record) (string, error) {
	recs, err := n.list(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	cur := n.match(recs, r.RR(), r.Type)
	if len(cur) == 0 {
		if err := n.create(ctx, r.Zone, r.RR(), r.Type, r.Value, r.TTL); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	target := cur[0]
	for _, rec := range cur {
		if strings.EqualFold(rec.Answer, r.Value) {
			target = rec
			break
		}
	}
	ttl := nameComTTL(r.TTL)
	if strings.EqualFold(target.Answer, r.Value) && (ttl == 0 || target.TTL == ttl) {
		return ActionUnchanged, nil
	}
	// 未指定 TTL 时沿用记录原有的 TTL
	if ttl == 0 {
		ttl = target.TTL
	}
	body := nameComRecord{Host: nameComHost(r.RR()), Type: r.Type, Answer: r.Value, TTL: ttl}
	if err := n.call(ctx, http.MethodPut, n.recordsPath(r.Zone)+"/"+strconv.Itoa(target.ID), nil, body, nil); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (n *nameCom) AddTXT(ctx context.Context, zone, sub, value string) error {
	return n.create(ctx, zone, sub, "TXT", value, txtTTL)
}

func (n *nameCom) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	recs, err := n.list(ctx, zone)
	if err != nil {
		return err
	}
	for _, rec := range n.match(recs, Record{Sub: sub}.RR(), "TXT") {
		if unquote(rec.Answer) == value {
			if err := n.remove(ctx, zone, rec.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *nameCom) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := n.list(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range n.match(recs, r.RR(), r.Type) {
		out = append(out, remoteRecord{id: strconv.Itoa(rec.ID), values: []string{rec.Answer}, ttl: rec.TTL})
	}
	return out, nil
}

func (n *nameCom) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	id, err := strconv.Atoi(rec.id)
	if err != nil {
		return err
	}
	return n.remove(ctx, r.Zone, id)
}

var (
	_ TXTProvider   = (*nameCom)(nil)
	_ recordDeleter = (*nameCom)(nil)
)
