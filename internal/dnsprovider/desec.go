package dnsprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// deSEC：以记录集（RRset）管理，同 subname 同类型的全部值在一个记录集里，
// TXT 值必须带引号。文档：https://desec.readthedocs.io/en/latest/dns/rrsets.html

func init() {
	register(Meta{
		Name:           "desec",
		Title:          "deSEC",
		SecretLabel:    "API Token",
		DocURL:         "https://desec.readthedocs.io/en/latest/auth/tokens.html",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &desec{token: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://desec.io/api/v1"}, nil
	})
}

// deSEC 的 TTL 下限由每个域名的 minimum_ttl 决定（服务端设置，常见为 3600，dedyn.io 等更低），上限为 86400。
// desecMinTTL 是读不到 minimum_ttl 时的保守下限，也是未指定 TTL 时新建记录集使用的值。
const (
	desecMinTTL = 3600
	desecMaxTTL = 86400
)

type desec struct {
	token    string
	hc       *http.Client
	endpoint string
}

type desecRRSet struct {
	Subname string   `json:"subname"`
	Type    string   `json:"type"`
	Records []string `json:"records"`
	TTL     int      `json:"ttl"`
}

// desecTTL 把 TTL 限制在 [floor, 86400] 内，floor 为域名的 minimum_ttl。
func desecTTL(ttl, floor int) int {
	if ttl < floor {
		return floor
	}
	if ttl > desecMaxTTL {
		return desecMaxTTL
	}
	return ttl
}

// desecSub 返回 deSEC 的 subname，根域名为空字符串。
func desecSub(sub string) string {
	if sub == "@" {
		return ""
	}
	return sub
}

// desecValue 按记录类型整理要写入的值：TXT 需要带引号。
func desecValue(typ, v string) string {
	if typ == "TXT" {
		return `"` + unquote(v) + `"`
	}
	return v
}

// rrsetURL 返回单个记录集的地址，根域名的 subname 在路径里写作 @。
func (d *desec) rrsetURL(zone, sub, typ string) string {
	name := sub
	if name == "" {
		name = "@"
	}
	return fmt.Sprintf("%s/domains/%s/rrsets/%s/%s/", d.endpoint, url.PathEscape(zone), url.PathEscape(name), url.PathEscape(typ))
}

func (d *desec) call(ctx context.Context, method, u string, body, out any) error {
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
	req.Header.Set("Authorization", "Token "+d.token)
	req.Header.Set("Content-Type", "application/json")
	if err := doJSON(d.hc, req, out); err != nil {
		return fmt.Errorf("deSEC: %w", err)
	}
	return nil
}

// minTTL 读取域名的 minimum_ttl；读取失败或未返回时使用保守下限 3600。
func (d *desec) minTTL(ctx context.Context, zone string) int {
	var dom struct {
		MinimumTTL int `json:"minimum_ttl"`
	}
	if err := d.call(ctx, http.MethodGet, fmt.Sprintf("%s/domains/%s/", d.endpoint, url.PathEscape(zone)), nil, &dom); err != nil || dom.MinimumTTL <= 0 {
		return desecMinTTL
	}
	return dom.MinimumTTL
}

// getRRSet 查询记录集，不存在（404）时返回 nil。域名本身不存在时同样是 404，会在后续写入时报错。
func (d *desec) getRRSet(ctx context.Context, zone, sub, typ string) (*desecRRSet, error) {
	var out desecRRSet
	err := d.call(ctx, http.MethodGet, d.rrsetURL(zone, sub, typ), nil, &out)
	var he *HTTPError
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// create 创建记录集。
func (d *desec) create(ctx context.Context, zone string, rs desecRRSet) error {
	return d.call(ctx, http.MethodPost, fmt.Sprintf("%s/domains/%s/rrsets/", d.endpoint, url.PathEscape(zone)), rs, nil)
}

// replace 整体替换已有记录集的值与 TTL。
func (d *desec) replace(ctx context.Context, zone string, rs desecRRSet) error {
	return d.call(ctx, http.MethodPut, d.rrsetURL(zone, rs.Subname, rs.Type), rs, nil)
}

func (d *desec) Upsert(ctx context.Context, r Record) (string, error) {
	sub := desecSub(r.RR())
	cur, err := d.getRRSet(ctx, r.Zone, sub, r.Type)
	if err != nil {
		return "", err
	}
	ttl := desecMinTTL
	if r.TTL > 0 {
		floor := desecMinTTL
		if r.TTL < desecMinTTL {
			// 指定的 TTL 低于保守下限时，读取域名实际允许的最小值，避免把 60、300 这类值一律抬到 3600
			floor = d.minTTL(ctx, r.Zone)
		}
		ttl = desecTTL(r.TTL, floor)
	} else if cur != nil && cur.TTL > 0 {
		// 未指定 TTL 时沿用记录集现有的 TTL
		ttl = cur.TTL
	}
	want := desecRRSet{Subname: sub, Type: r.Type, Records: []string{desecValue(r.Type, r.Value)}, TTL: ttl}
	if cur == nil {
		if err := d.create(ctx, r.Zone, want); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	if len(cur.Records) == 1 && sameValue(r.Type, cur.Records[0], r.Value) && (r.TTL <= 0 || cur.TTL == ttl) {
		return ActionUnchanged, nil
	}
	if err := d.replace(ctx, r.Zone, want); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (d *desec) AddTXT(ctx context.Context, zone, sub, value string) error {
	sub = desecSub(Record{Zone: zone, Sub: sub}.RR())
	cur, err := d.getRRSet(ctx, zone, sub, "TXT")
	if err != nil {
		return err
	}
	quoted := desecValue("TXT", value)
	if cur == nil {
		return d.create(ctx, zone, desecRRSet{Subname: sub, Type: "TXT", Records: []string{quoted}, TTL: desecMinTTL})
	}
	// 记录集已存在（例如同时申请泛域名与主域名）：合并新值，已有该值时不重复添加
	for _, v := range cur.Records {
		if unquote(v) == value {
			return nil
		}
	}
	return d.replace(ctx, zone, desecRRSet{Subname: sub, Type: "TXT", Records: append(cur.Records, quoted), TTL: cur.TTL})
}

func (d *desec) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	sub = desecSub(Record{Zone: zone, Sub: sub}.RR())
	cur, err := d.getRRSet(ctx, zone, sub, "TXT")
	if err != nil || cur == nil {
		return err
	}
	var keep []string
	for _, v := range cur.Records {
		if unquote(v) != value {
			keep = append(keep, v)
		}
	}
	if len(keep) == len(cur.Records) {
		return nil
	}
	if len(keep) == 0 {
		return d.call(ctx, http.MethodDelete, d.rrsetURL(zone, sub, "TXT"), nil, nil)
	}
	return d.replace(ctx, zone, desecRRSet{Subname: sub, Type: "TXT", Records: keep, TTL: cur.TTL})
}

func (d *desec) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	cur, err := d.getRRSet(ctx, r.Zone, desecSub(r.RR()), r.Type)
	if err != nil || cur == nil {
		return nil, err
	}
	return []remoteRecord{{values: cur.Records, ttl: cur.TTL}}, nil
}

func (d *desec) removeRecord(ctx context.Context, r Record, _ remoteRecord) error {
	return d.call(ctx, http.MethodDelete, d.rrsetURL(r.Zone, desecSub(r.RR()), r.Type), nil, nil)
}

var (
	_ TXTProvider   = (*desec)(nil)
	_ recordDeleter = (*desec)(nil)
)
