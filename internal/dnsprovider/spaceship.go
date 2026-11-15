package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Spaceship DNS：https://docs.spaceship.dev/
// 记录接口按域名整体操作：GET 列表（take/skip 分页）、PUT 保存（新增或更新）、DELETE 按内容删除。

func init() {
	register(Meta{
		Name:           "spaceship",
		Title:          "Spaceship",
		IDLabel:        "API Key",
		SecretLabel:    "API Secret",
		DocURL:         "https://www.spaceship.com/application/api-manager/",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &spaceship{key: strings.TrimSpace(c.ID), secret: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://spaceship.dev/api/v1"}, nil
	})
}

type spaceship struct {
	key, secret string
	hc          *http.Client
	endpoint    string
}

// spaceshipItem 是列表中的一条记录；A / AAAA 的内容在 address，TXT 的内容在 value。
type spaceshipItem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	Value   string `json:"value,omitempty"`
	TTL     int    `json:"ttl,omitempty"`
}

// content 返回记录内容。
func (i spaceshipItem) content() string {
	if i.Type == "TXT" {
		return i.Value
	}
	return i.Address
}

// spaceshipItemOf 按类型构造记录：TXT 用 value，其余用 address。
func spaceshipItemOf(typ, name, content string, ttl int) spaceshipItem {
	it := spaceshipItem{Type: typ, Name: name, TTL: ttl}
	if typ == "TXT" {
		it.Value = content
	} else {
		it.Address = content
	}
	return it
}

// spaceshipTTL 返回提交的 TTL：0 取 600，并限制在接口允许的 60 到 3600 秒内。
func spaceshipTTL(ttl int) int {
	if ttl <= 0 {
		return 600
	}
	if ttl < 60 {
		return 60
	}
	if ttl > 3600 {
		return 3600
	}
	return ttl
}

func (s *spaceship) call(ctx context.Context, method string, q url.Values, zone string, body, out any) error {
	u := s.endpoint + "/dns/records/" + url.PathEscape(zone)
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
	req.Header.Set("X-API-Key", s.key)
	req.Header.Set("X-API-Secret", s.secret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	err = doJSON(s.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		// 错误体为 problem+json：{"detail":"...","data":[{"field":"..","details":".."}]}
		var e struct {
			Detail string `json:"detail"`
			Data   []struct {
				Field   string `json:"field"`
				Details string `json:"details"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(he.Body), &e) == nil && e.Detail != "" {
			msg := e.Detail
			for _, d := range e.Data {
				msg += fmt.Sprintf("; %s: %s", d.Field, d.Details)
			}
			return fmt.Errorf("Spaceship: HTTP %d: %s", he.Status, msg)
		}
	}
	if err != nil {
		return fmt.Errorf("Spaceship: %w", err)
	}
	return nil
}

// list 返回域名下的全部记录，每页最多 500 条（接口上限），按 skip 翻页。
func (s *spaceship) list(ctx context.Context, zone string) ([]spaceshipItem, error) {
	const take = 500
	var all []spaceshipItem
	skip := 0
	for page := 0; page < 100; page++ {
		var resp struct {
			Items []spaceshipItem `json:"items"`
			Total int             `json:"total"`
		}
		q := url.Values{"take": {strconv.Itoa(take)}, "skip": {strconv.Itoa(skip)}}
		if err := s.call(ctx, http.MethodGet, q, zone, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if len(resp.Items) == 0 || len(all) >= resp.Total {
			break
		}
		// 服务端返回的条数可能少于 take，按实际条数推进偏移
		skip += len(resp.Items)
	}
	return all, nil
}

func (s *spaceship) match(items []spaceshipItem, sub, typ string) []spaceshipItem {
	var out []spaceshipItem
	for _, it := range items {
		if it.Type == typ && strings.EqualFold(it.Name, sub) {
			out = append(out, it)
		}
	}
	return out
}

func (s *spaceship) save(ctx context.Context, zone string, it spaceshipItem) error {
	return s.call(ctx, http.MethodPut, nil, zone, map[string]any{"items": []spaceshipItem{it}}, nil)
}

// del 按类型、名称和内容删除记录，不会影响其他记录。
func (s *spaceship) del(ctx context.Context, zone string, it spaceshipItem) error {
	it.TTL = 0
	return s.call(ctx, http.MethodDelete, nil, zone, []spaceshipItem{it}, nil)
}

func (s *spaceship) Upsert(ctx context.Context, r Record) (string, error) {
	items, err := s.list(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	name := r.RR()
	cur := s.match(items, name, r.Type)
	ttl := spaceshipTTL(r.TTL)
	var same *spaceshipItem
	var others []spaceshipItem
	for i := range cur {
		if strings.EqualFold(cur[i].content(), r.Value) && same == nil {
			same = &cur[i]
		} else {
			others = append(others, cur[i])
		}
	}
	if same != nil && len(others) == 0 && (r.TTL == 0 || same.TTL == ttl) {
		return ActionUnchanged, nil
	}
	if same != nil && r.TTL == 0 {
		ttl = same.TTL
	}
	// 先写入新值，再删除旧值，避免中途失败时记录消失
	if err := s.save(ctx, r.Zone, spaceshipItemOf(r.Type, name, r.Value, ttl)); err != nil {
		return "", err
	}
	for _, o := range others {
		if err := s.del(ctx, r.Zone, o); err != nil {
			return "", err
		}
	}
	if len(cur) == 0 {
		return ActionCreated, nil
	}
	return ActionUpdated, nil
}

func (s *spaceship) AddTXT(ctx context.Context, zone, sub, value string) error {
	return s.save(ctx, zone, spaceshipItemOf("TXT", Record{Sub: sub}.RR(), value, txtTTL))
}

func (s *spaceship) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	items, err := s.list(ctx, zone)
	if err != nil {
		return err
	}
	for _, it := range s.match(items, Record{Sub: sub}.RR(), "TXT") {
		if unquote(it.Value) == value {
			if err := s.del(ctx, zone, it); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *spaceship) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	items, err := s.list(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, it := range s.match(items, r.RR(), r.Type) {
		out = append(out, remoteRecord{values: []string{it.content()}, ttl: it.TTL})
	}
	return out, nil
}

func (s *spaceship) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	return s.del(ctx, r.Zone, spaceshipItemOf(r.Type, r.RR(), rec.values[0], 0))
}

var (
	_ TXTProvider   = (*spaceship)(nil)
	_ recordDeleter = (*spaceship)(nil)
)
