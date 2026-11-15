package dnsprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Gcore DNS：以记录集（rrset）管理，同名同类型的全部值在一个记录集里。
// 文档：https://api.gcore.com/docs/dns

func init() {
	register(Meta{
		Name:           "gcore",
		Title:          "Gcore",
		SecretLabel:    "Permanent API Token",
		DocURL:         "https://accounts.gcore.com/profile/api-tokens",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &gcore{token: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://api.gcore.com/dns/v2"}, nil
	})
}

type gcore struct {
	token    string
	hc       *http.Client
	endpoint string
}

// gcoreRR 是记录集里的一个值；content 对 A/AAAA/TXT 为只含一个字符串的数组。
type gcoreRR struct {
	Content []any `json:"content"`
	Enabled bool  `json:"enabled"`
}

// gcoreRRSet 是 Gcore 的记录集，创建与更新的请求体只需要 ttl 与 resource_records。
type gcoreRRSet struct {
	TTL     int       `json:"ttl"`
	Records []gcoreRR `json:"resource_records"`
}

// first 返回记录值（content 的第一项）。
func (r gcoreRR) first() string {
	if len(r.Content) == 0 {
		return ""
	}
	if s, ok := r.Content[0].(string); ok {
		return s
	}
	return fmt.Sprint(r.Content[0])
}

func gcoreName(r Record) string {
	return r.FQDN()
}

func (g *gcore) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		rd = jsonBody(body)
	}
	req, err := ctxReq(ctx, method, g.endpoint+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "APIKey "+g.token)
	req.Header.Set("Content-Type", "application/json")
	if err := doJSON(g.hc, req, out); err != nil {
		return fmt.Errorf("Gcore: %w", err)
	}
	return nil
}

// rrsetPath 返回单个记录集的接口路径：/zones/{zone}/{rrsetName}/{type}。
func (g *gcore) rrsetPath(zone, fqdn, typ string) string {
	return "/zones/" + url.PathEscape(zone) + "/" + url.PathEscape(fqdn) + "/" + url.PathEscape(typ)
}

// getRRSet 查询记录集，不存在（404）时返回 nil。
func (g *gcore) getRRSet(ctx context.Context, zone, fqdn, typ string) (*gcoreRRSet, error) {
	var out gcoreRRSet
	err := g.call(ctx, http.MethodGet, g.rrsetPath(zone, fqdn, typ), nil, &out)
	var he *HTTPError
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (g *gcore) Upsert(ctx context.Context, r Record) (string, error) {
	fqdn := gcoreName(r)
	cur, err := g.getRRSet(ctx, r.Zone, fqdn, r.Type)
	if err != nil {
		return "", err
	}
	ttl := r.TTL
	if ttl <= 0 {
		// 免费版允许的最小 TTL
		ttl = 120
	}
	want := gcoreRRSet{TTL: ttl, Records: []gcoreRR{{Content: []any{r.Value}, Enabled: true}}}
	if cur == nil {
		if err := g.call(ctx, http.MethodPost, g.rrsetPath(r.Zone, fqdn, r.Type), want, nil); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	if len(cur.Records) == 1 && cur.Records[0].first() == r.Value && (r.TTL <= 0 || cur.TTL == ttl) {
		return ActionUnchanged, nil
	}
	if err := g.call(ctx, http.MethodPut, g.rrsetPath(r.Zone, fqdn, r.Type), want, nil); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (g *gcore) AddTXT(ctx context.Context, zone, sub, value string) error {
	fqdn := gcoreName(Record{Zone: zone, Sub: sub})
	cur, err := g.getRRSet(ctx, zone, fqdn, "TXT")
	if err != nil {
		return err
	}
	add := gcoreRR{Content: []any{value}, Enabled: true}
	if cur == nil {
		return g.call(ctx, http.MethodPost, g.rrsetPath(zone, fqdn, "TXT"), gcoreRRSet{TTL: txtTTL, Records: []gcoreRR{add}}, nil)
	}
	// 同名已有验证值时保留，追加新值
	return g.call(ctx, http.MethodPut, g.rrsetPath(zone, fqdn, "TXT"), gcoreRRSet{TTL: cur.TTL, Records: append(gcoreKeep(cur.Records), add)}, nil)
}

func (g *gcore) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	fqdn := gcoreName(Record{Zone: zone, Sub: sub})
	cur, err := g.getRRSet(ctx, zone, fqdn, "TXT")
	if err != nil || cur == nil {
		return err
	}
	var keep []gcoreRR
	for _, rr := range cur.Records {
		if unquote(rr.first()) != value {
			keep = append(keep, rr)
		}
	}
	if len(keep) == len(cur.Records) {
		return nil
	}
	if len(keep) == 0 {
		return g.call(ctx, http.MethodDelete, g.rrsetPath(zone, fqdn, "TXT"), nil, nil)
	}
	return g.call(ctx, http.MethodPut, g.rrsetPath(zone, fqdn, "TXT"), gcoreRRSet{TTL: cur.TTL, Records: gcoreKeep(keep)}, nil)
}

// gcoreKeep 只保留写回时需要的字段（content、enabled），丢弃服务商附带的 id 与 meta。
func gcoreKeep(in []gcoreRR) []gcoreRR {
	out := make([]gcoreRR, 0, len(in)+1)
	for _, rr := range in {
		out = append(out, gcoreRR{Content: rr.Content, Enabled: rr.Enabled})
	}
	return out
}

func (g *gcore) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	cur, err := g.getRRSet(ctx, r.Zone, gcoreName(r), r.Type)
	if err != nil || cur == nil {
		return nil, err
	}
	rec := remoteRecord{ttl: cur.TTL}
	for _, rr := range cur.Records {
		rec.values = append(rec.values, rr.first())
	}
	return []remoteRecord{rec}, nil
}

func (g *gcore) removeRecord(ctx context.Context, r Record, _ remoteRecord) error {
	return g.call(ctx, http.MethodDelete, g.rrsetPath(r.Zone, gcoreName(r), r.Type), nil, nil)
}

var (
	_ TXTProvider   = (*gcore)(nil)
	_ recordDeleter = (*gcore)(nil)
)
