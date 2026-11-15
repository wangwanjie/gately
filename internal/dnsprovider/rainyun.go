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

// 雨云：接口基地址 https://api.v2.rainyun.com/product，请求头 x-api-key 携带 API 密钥。
// 记录按域名产品 ID 管理，域名 ID 通过域名列表按名称查得，不需要用户填写。
// 响应统一为 {code, message, data}，code 为 200 表示成功。

func init() {
	register(Meta{
		Name:           "rainyun",
		Title:          "雨云",
		SecretLabel:    "API 密钥",
		DocURL:         "https://app.rainyun.com/account/settings/api-key",
		SupportsACME:   true,
		SupportsDelete: true,
		SupportsLine:   true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &rainyun{key: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://api.v2.rainyun.com/product"}, nil
	})
}

type rainyun struct {
	key      string
	hc       *http.Client
	endpoint string
}

type rainyunRecord struct {
	ID    int64  `json:"record_id"`
	Host  string `json:"host"`
	Type  string `json:"type"`
	Value string `json:"value"`
	Line  string `json:"line"`
	TTL   int    `json:"ttl"`
	Level int    `json:"level"`
}

// rainyunPage 是列表接口 data 字段的结构。
type rainyunPage struct {
	TotalRecords int             `json:"TotalRecords"`
	Records      json.RawMessage `json:"Records"`
}

// call 发送请求并校验业务状态码，data 原文写入 out（可为 nil）。
func (y *rainyun) call(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	u := y.endpoint + path
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
	req.Header.Set("x-api-key", y.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := doJSON(y.hc, req, &env); err != nil {
		return fmt.Errorf("雨云: %w", err)
	}
	if env.Code != 200 {
		if env.Message != "" {
			return fmt.Errorf("雨云: %s (code %d)", env.Message, env.Code)
		}
		return fmt.Errorf("雨云: 接口返回错误 code=%d", env.Code)
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("雨云: 解析响应失败: %w", err)
	}
	return nil
}

// domainID 按域名名称查询雨云的域名产品 ID。
func (y *rainyun) domainID(ctx context.Context, zone string) (int64, error) {
	const perPage = 100
	total := 0
	for page := 1; page <= 50; page++ {
		opts, _ := json.Marshal(map[string]any{
			"columnFilters": map[string]string{"domains.Domain": zone},
			"sort":          []any{},
			"page":          page,
			"perPage":       perPage,
		})
		var res struct {
			TotalRecords int `json:"TotalRecords"`
			Records      []struct {
				ID     int64  `json:"id"`
				Domain string `json:"domain"`
			} `json:"Records"`
		}
		if err := y.call(ctx, http.MethodGet, "/domain", url.Values{"options": {string(opts)}}, nil, &res); err != nil {
			return 0, err
		}
		for _, d := range res.Records {
			if strings.EqualFold(d.Domain, zone) {
				return d.ID, nil
			}
		}
		total += len(res.Records)
		if len(res.Records) < perPage || (res.TotalRecords > 0 && total >= res.TotalRecords) {
			break
		}
	}
	return 0, fmt.Errorf("雨云账号下未找到域名 %s", zone)
}

func (y *rainyun) dnsPath(domainID int64) string {
	return "/domain/" + strconv.FormatInt(domainID, 10) + "/dns"
}

// list 列出域名下的全部解析记录，自动翻页。
func (y *rainyun) list(ctx context.Context, domainID int64) ([]rainyunRecord, error) {
	const limit = 100
	var out []rainyunRecord
	for page := 1; page <= 50; page++ {
		var res rainyunPage
		q := url.Values{"limit": {strconv.Itoa(limit)}, "page_no": {strconv.Itoa(page)}}
		if err := y.call(ctx, http.MethodGet, y.dnsPath(domainID)+"/", q, nil, &res); err != nil {
			return nil, err
		}
		var recs []rainyunRecord
		if len(res.Records) > 0 && string(res.Records) != "null" {
			if err := json.Unmarshal(res.Records, &recs); err != nil {
				return nil, fmt.Errorf("雨云: 解析记录列表失败: %w", err)
			}
		}
		out = append(out, recs...)
		if len(recs) < limit || (res.TotalRecords > 0 && len(out) >= res.TotalRecords) {
			break
		}
	}
	return out, nil
}

// rainyunLine 返回线路代码，未指定时为默认线路 DEFAULT。
func rainyunLine(line string) string {
	if line == "" {
		return "DEFAULT"
	}
	return strings.ToUpper(line)
}

// match 选出主机记录、类型与线路都相同的记录。
func (y *rainyun) match(recs []rainyunRecord, host, typ, line string) []rainyunRecord {
	var out []rainyunRecord
	for _, rec := range recs {
		if !dnslaSameHost(rec.Host, host) || !strings.EqualFold(rec.Type, typ) {
			continue
		}
		if rainyunLine(rec.Line) != line {
			continue
		}
		out = append(out, rec)
	}
	return out
}

func (y *rainyun) Upsert(ctx context.Context, r Record) (string, error) {
	did, err := y.domainID(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	recs, err := y.list(ctx, did)
	if err != nil {
		return "", err
	}
	host := r.RR()
	line := rainyunLine(r.Line)
	found := y.match(recs, host, r.Type, line)
	ttl := r.TTL
	if len(found) > 0 {
		cur := found[0]
		if ttl <= 0 {
			ttl = cur.TTL
		}
		if ttl <= 0 {
			ttl = 600
		}
		if cur.Value == r.Value && (r.TTL <= 0 || cur.TTL == ttl) {
			return ActionUnchanged, nil
		}
		level := cur.Level
		if level <= 0 {
			level = 10
		}
		err := y.call(ctx, http.MethodPatch, y.dnsPath(did), nil, map[string]any{
			"record_id": cur.ID, "host": host, "line": line, "level": level,
			"ttl": ttl, "type": r.Type, "value": r.Value,
		}, nil)
		if err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if ttl <= 0 {
		ttl = 600
	}
	err = y.call(ctx, http.MethodPost, y.dnsPath(did), nil, map[string]any{
		"host": host, "line": line, "level": 10, "ttl": ttl, "type": r.Type, "value": r.Value,
	}, nil)
	if err != nil {
		return "", err
	}
	return ActionCreated, nil
}

func (y *rainyun) AddTXT(ctx context.Context, zone, sub, value string) error {
	did, err := y.domainID(ctx, zone)
	if err != nil {
		return err
	}
	return y.call(ctx, http.MethodPost, y.dnsPath(did), nil, map[string]any{
		"host": Record{Zone: zone, Sub: sub}.RR(), "line": "DEFAULT", "level": 10, "ttl": 600, "type": "TXT", "value": value,
	}, nil)
}

func (y *rainyun) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	did, err := y.domainID(ctx, zone)
	if err != nil {
		return err
	}
	recs, err := y.list(ctx, did)
	if err != nil {
		return err
	}
	host := Record{Zone: zone, Sub: sub}.RR()
	for _, rec := range recs {
		if dnslaSameHost(rec.Host, host) && strings.EqualFold(rec.Type, "TXT") && unquote(rec.Value) == value {
			return y.remove(ctx, did, rec.ID)
		}
	}
	return nil
}

func (y *rainyun) remove(ctx context.Context, domainID, recordID int64) error {
	return y.call(ctx, http.MethodDelete, y.dnsPath(domainID), url.Values{"record_id": {strconv.FormatInt(recordID, 10)}}, nil, nil)
}

func (y *rainyun) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	did, err := y.domainID(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	recs, err := y.list(ctx, did)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range y.match(recs, r.RR(), r.Type, rainyunLine(r.Line)) {
		out = append(out, remoteRecord{
			id: strconv.FormatInt(rec.ID, 10), scope: strconv.FormatInt(did, 10), values: []string{rec.Value}, ttl: rec.TTL,
		})
	}
	return out, nil
}

func (y *rainyun) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	did, err := strconv.ParseInt(rec.scope, 10, 64)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(rec.id, 10, 64)
	if err != nil {
		return err
	}
	return y.remove(ctx, did, id)
}

// 编译期确认实现了接口
var (
	_ TXTProvider   = (*rainyun)(nil)
	_ recordDeleter = (*rainyun)(nil)
)
