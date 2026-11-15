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

// DNS.LA：接口基地址 https://api.dns.la/api，使用 APIID/APISecret 做 Basic 认证。
// 记录类型使用枚举数值（A=1、CNAME=5、TXT=16、AAAA=28），域名与记录 ID 均为字符串。
// 响应统一为 {code, msg, data}，code 为 200 表示成功（HTTP 状态码另有 401 表示认证失败）。

func init() {
	register(Meta{
		Name:           "dnsla",
		Title:          "DNS.LA",
		IDLabel:        "APIID",
		SecretLabel:    "APISecret",
		DocURL:         "https://www.dns.la/docs/ApiDoc",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &dnsla{id: strings.TrimSpace(c.ID), secret: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://api.dns.la/api"}, nil
	})
}

type dnsla struct {
	id, secret string
	hc         *http.Client
	endpoint   string
}

// dnslaID 兼容数字与字符串两种写法的 ID。
type dnslaID string

func (d *dnslaID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*d = ""
		return nil
	}
	*d = dnslaID(strings.Trim(s, `"`))
	return nil
}

type dnslaResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type dnslaRecord struct {
	ID   dnslaID `json:"id"`
	Host string  `json:"host"`
	Type int     `json:"type"`
	Data string  `json:"data"`
	TTL  int     `json:"ttl"`
}

// dnslaType 返回记录类型对应的枚举值。
func dnslaType(t string) (int, error) {
	switch strings.ToUpper(t) {
	case "A":
		return 1, nil
	case "CNAME":
		return 5, nil
	case "TXT":
		return 16, nil
	case "AAAA":
		return 28, nil
	}
	return 0, fmt.Errorf("DNS.LA 不支持记录类型 %s", t)
}

// call 发送请求并校验业务状态码，返回 data 原文。
func (d *dnsla) call(ctx context.Context, method, path string, q url.Values, body any) (json.RawMessage, error) {
	u := d.endpoint + path
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
		return nil, err
	}
	req.SetBasicAuth(d.id, d.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json;charset=utf-8")
	}
	var out dnslaResp
	if err := doJSON(d.hc, req, &out); err != nil {
		return nil, fmt.Errorf("DNS.LA: %w", err)
	}
	if out.Code != 200 {
		return nil, fmt.Errorf("DNS.LA: %s (code %d)", out.Msg, out.Code)
	}
	return out.Data, nil
}

// domainID 按域名查询 DNS.LA 的域名 ID。
func (d *dnsla) domainID(ctx context.Context, zone string) (string, error) {
	data, err := d.call(ctx, http.MethodGet, "/domain", url.Values{"domain": {zone}}, nil)
	if err != nil {
		return "", err
	}
	var one struct {
		ID     dnslaID `json:"id"`
		Domain string  `json:"domain"`
	}
	if err := json.Unmarshal(data, &one); err == nil && one.ID != "" {
		return string(one.ID), nil
	}
	// 兼容返回列表的写法
	var list struct {
		Results []struct {
			ID     dnslaID `json:"id"`
			Domain string  `json:"domain"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &list); err == nil {
		for _, it := range list.Results {
			if strings.EqualFold(it.Domain, zone) && it.ID != "" {
				return string(it.ID), nil
			}
		}
	}
	return "", fmt.Errorf("DNS.LA 账号下未找到域名 %s", zone)
}

// list 列出域名下指定主机记录与类型的记录，自动翻页。
func (d *dnsla) list(ctx context.Context, domainID, host string, typ int) ([]dnslaRecord, error) {
	const pageSize = 100
	var out []dnslaRecord
	for page := 1; page <= 50; page++ {
		q := url.Values{
			"domainId":  {domainID},
			"host":      {host},
			"type":      {strconv.Itoa(typ)},
			"pageIndex": {strconv.Itoa(page)},
			"pageSize":  {strconv.Itoa(pageSize)},
		}
		data, err := d.call(ctx, http.MethodGet, "/recordList", q, nil)
		if err != nil {
			return nil, err
		}
		var res struct {
			Total   int           `json:"total"`
			Results []dnslaRecord `json:"results"`
		}
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, fmt.Errorf("DNS.LA: 解析记录列表失败: %w", err)
		}
		out = append(out, res.Results...)
		if len(res.Results) < pageSize || (res.Total > 0 && len(out) >= res.Total) {
			break
		}
	}
	// 服务端的主机/类型过滤可能是模糊匹配，这里再精确过滤一遍
	var exact []dnslaRecord
	for _, rec := range out {
		if rec.Type == typ && dnslaSameHost(rec.Host, host) {
			exact = append(exact, rec)
		}
	}
	return exact, nil
}

// dnslaSameHost 比较主机记录，空值视为根域名 "@"。
func dnslaSameHost(a, b string) bool {
	if a == "" {
		a = "@"
	}
	if b == "" {
		b = "@"
	}
	return strings.EqualFold(a, b)
}

func (d *dnsla) Upsert(ctx context.Context, r Record) (string, error) {
	typ, err := dnslaType(r.Type)
	if err != nil {
		return "", err
	}
	did, err := d.domainID(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	host := r.RR()
	recs, err := d.list(ctx, did, host, typ)
	if err != nil {
		return "", err
	}
	ttl := r.TTL
	if len(recs) > 0 {
		cur := recs[0]
		if ttl <= 0 {
			ttl = cur.TTL
		}
		if ttl <= 0 {
			ttl = 600
		}
		// 接口未返回 TTL（为 0）时无法比较，只按值判断
		if cur.Data == r.Value && (r.TTL <= 0 || cur.TTL == 0 || cur.TTL == ttl) {
			return ActionUnchanged, nil
		}
		_, err := d.call(ctx, http.MethodPut, "/record", nil, map[string]any{
			"id": string(cur.ID), "host": host, "type": typ, "data": r.Value, "ttl": ttl,
		})
		if err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if ttl <= 0 {
		ttl = 600
	}
	_, err = d.call(ctx, http.MethodPost, "/record", nil, map[string]any{
		"domainId": did, "host": host, "type": typ, "data": r.Value, "ttl": ttl,
	})
	if err != nil {
		return "", err
	}
	return ActionCreated, nil
}

func (d *dnsla) AddTXT(ctx context.Context, zone, sub, value string) error {
	did, err := d.domainID(ctx, zone)
	if err != nil {
		return err
	}
	_, err = d.call(ctx, http.MethodPost, "/record", nil, map[string]any{
		"domainId": did, "host": Record{Zone: zone, Sub: sub}.RR(), "type": 16, "data": value, "ttl": 600,
	})
	// 与已有记录冲突说明同样的验证值已存在，视为成功
	if err != nil && strings.Contains(err.Error(), "与已有记录冲突") {
		return nil
	}
	return err
}

func (d *dnsla) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	did, err := d.domainID(ctx, zone)
	if err != nil {
		return err
	}
	recs, err := d.list(ctx, did, Record{Zone: zone, Sub: sub}.RR(), 16)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Data) == value {
			return d.remove(ctx, string(rec.ID))
		}
	}
	return nil
}

func (d *dnsla) remove(ctx context.Context, id string) error {
	_, err := d.call(ctx, http.MethodDelete, "/record", url.Values{"id": {id}}, nil)
	return err
}

func (d *dnsla) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	typ, err := dnslaType(r.Type)
	if err != nil {
		return nil, err
	}
	did, err := d.domainID(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	recs, err := d.list(ctx, did, r.RR(), typ)
	if err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, remoteRecord{id: string(rec.ID), scope: did, values: []string{rec.Data}, ttl: rec.TTL})
	}
	return out, nil
}

func (d *dnsla) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return d.remove(ctx, rec.id)
}

// 编译期确认实现了接口
var (
	_ TXTProvider   = (*dnsla)(nil)
	_ recordDeleter = (*dnsla)(nil)
)
