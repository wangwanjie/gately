package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

// 腾讯云 EdgeOne（TEO）DNS 记录管理，API 3.0，签名算法 TC3-HMAC-SHA256（服务名 teo）。
// 仅适用于以 NS 方式接入 EdgeOne 的站点。
// 文档：https://cloud.tencent.com/document/api/1552/80716
func init() {
	register(Meta{
		Name:        "edgeone",
		Title:       "腾讯云 EdgeOne",
		IDLabel:     "SecretId",
		SecretLabel: "SecretKey",
		Extra: []Field{
			{Key: "endpoint", Label: "接口地址", Placeholder: "默认 https://teo.tencentcloudapi.com；国际站为 https://teo.intl.tencentcloudapi.com"},
		},
		SupportsLine:   true,
		SupportsACME:   true,
		SupportsDelete: true,
		DocURL:         "https://console.cloud.tencent.com/cam/capi",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		endpoint := strings.TrimSpace(c.Extra["endpoint"])
		if endpoint == "" {
			endpoint = "https://teo.tencentcloudapi.com"
		}
		if u, err := url.Parse(endpoint); err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("接口地址无效，需要 https:// 开头的完整地址")
		}
		return &edgeOne{id: c.ID, secret: c.Secret, hc: hc, endpoint: endpoint, now: time.Now}, nil
	})
}

const (
	// EdgeOne 接口版本
	edgeOneVersion = "2022-09-01"
	// 新建记录未指定 TTL 时使用的默认值（接口允许 60~86400）
	edgeOneDefaultTTL = 600
	edgeOneMinTTL     = 60
	edgeOneMaxTTL     = 86400
	// DescribeZones 单页最大 100 条，DescribeDnsRecords 单页最大 1000 条
	edgeOneZonePage   = 100
	edgeOneRecordPage = 1000
	// 翻页上限，避免服务端异常时死循环
	edgeOneMaxPages = 50
)

type edgeOne struct {
	id, secret string
	hc         *http.Client
	endpoint   string
	now        func() time.Time
}

type edgeOneRecord struct {
	RecordID string `json:"RecordId"`
	Name     string `json:"Name"`
	Type     string `json:"Type"`
	Content  string `json:"Content"`
	Location string `json:"Location"`
	TTL      int    `json:"TTL"`
	Status   string `json:"Status"`
}

// edgeOneFilter 是 DescribeZones / DescribeDnsRecords 的过滤条件，Fuzzy 固定为精确匹配。
type edgeOneFilter struct {
	Name   string   `json:"Name"`
	Values []string `json:"Values"`
	Fuzzy  bool     `json:"Fuzzy"`
}

// call 发送一次 API 调用；业务错误（Response.Error）转换为 error 返回。
func (e *edgeOne) call(ctx context.Context, action string, params any, out any) error {
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	u, err := url.Parse(e.endpoint)
	if err != nil {
		return err
	}
	ts := e.now()
	req, err := ctxReq(ctx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", edgeOneVersion)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts.Unix(), 10))
	req.Header.Set("Authorization", tc3Authorization(e.id, e.secret, "teo", u.Host, action, payload, ts))
	var env struct {
		Response json.RawMessage `json:"Response"`
	}
	if err := doJSON(e.hc, req, &env); err != nil {
		return fmt.Errorf("腾讯云 EdgeOne %s: %w", action, err)
	}
	var apiErr struct {
		Error *tcError `json:"Error"`
	}
	_ = json.Unmarshal(env.Response, &apiErr)
	if apiErr.Error != nil {
		return fmt.Errorf("腾讯云 EdgeOne %s: %s (%s)", action, apiErr.Error.Message, apiErr.Error.Code)
	}
	if out != nil {
		return json.Unmarshal(env.Response, out)
	}
	return nil
}

// edgeOneASCII 把域名转成 punycode，接口要求中日韩域名使用 punycode。
func edgeOneASCII(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if a, err := idna.ToASCII(s); err == nil {
		return a
	}
	return s
}

// zoneID 按站点名称查询 ZoneId，分页遍历并忽略大小写比较。
func (e *edgeOne) zoneID(ctx context.Context, zone string) (string, error) {
	name := edgeOneASCII(zone)
	offset := 0
	for page := 0; page < edgeOneMaxPages; page++ {
		var resp struct {
			TotalCount int `json:"TotalCount"`
			Zones      []struct {
				ZoneID   string `json:"ZoneId"`
				ZoneName string `json:"ZoneName"`
			} `json:"Zones"`
		}
		err := e.call(ctx, "DescribeZones", map[string]any{
			"Offset":  offset,
			"Limit":   edgeOneZonePage,
			"Filters": []edgeOneFilter{{Name: "zone-name", Values: []string{name}}},
		}, &resp)
		if err != nil {
			return "", err
		}
		for _, z := range resp.Zones {
			if strings.EqualFold(z.ZoneName, name) {
				return z.ZoneID, nil
			}
		}
		offset += len(resp.Zones)
		if len(resp.Zones) == 0 || offset >= resp.TotalCount {
			break
		}
	}
	return "", fmt.Errorf("腾讯云 EdgeOne 账号下未找到站点 %s：该域名需以 NS 方式接入 EdgeOne 后才能管理解析记录", zone)
}

// listRecords 查询站点下指定名称与类型的记录。名称过滤支持模糊匹配，因此再按名称精确过滤。
func (e *edgeOne) listRecords(ctx context.Context, zoneID, fqdn, typ string) ([]edgeOneRecord, error) {
	name := edgeOneASCII(fqdn)
	var out []edgeOneRecord
	offset := 0
	for page := 0; page < edgeOneMaxPages; page++ {
		var resp struct {
			TotalCount int             `json:"TotalCount"`
			DNSRecords []edgeOneRecord `json:"DnsRecords"`
		}
		err := e.call(ctx, "DescribeDnsRecords", map[string]any{
			"ZoneId": zoneID,
			"Offset": offset,
			"Limit":  edgeOneRecordPage,
			"Filters": []edgeOneFilter{
				{Name: "name", Values: []string{name}},
				{Name: "type", Values: []string{typ}},
			},
		}, &resp)
		if err != nil {
			return nil, err
		}
		for _, rec := range resp.DNSRecords {
			if rec.Type == typ && strings.EqualFold(rec.Name, name) {
				out = append(out, rec)
			}
		}
		offset += len(resp.DNSRecords)
		if len(resp.DNSRecords) == 0 || offset >= resp.TotalCount {
			break
		}
	}
	return out, nil
}

// edgeOneTTL 把 TTL 限制在接口允许的 60~86400 秒内；0 表示未指定。
func edgeOneTTL(ttl int) int {
	if ttl <= 0 {
		return 0
	}
	if ttl < edgeOneMinTTL {
		return edgeOneMinTTL
	}
	if ttl > edgeOneMaxTTL {
		return edgeOneMaxTTL
	}
	return ttl
}

// edgeOneLocation 返回记录的解析线路，空表示默认线路 Default。
func edgeOneLocation(line string) string {
	if line == "" {
		return "Default"
	}
	return line
}

func (e *edgeOne) Upsert(ctx context.Context, r Record) (string, error) {
	zid, err := e.zoneID(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	recs, err := e.listRecords(ctx, zid, r.FQDN(), r.Type)
	if err != nil {
		return "", err
	}
	ttl := edgeOneTTL(r.TTL)
	var picked *edgeOneRecord
	for i := range recs {
		rec := &recs[i]
		if !lineMatch(rec.Location, r.Line, "Default") {
			continue
		}
		// 优先使用已启用的记录
		if picked == nil || (picked.Status != "enable" && rec.Status == "enable") {
			picked = rec
		}
	}
	if picked != nil {
		if sameValue(r.Type, picked.Content, r.Value) && (ttl == 0 || picked.TTL == ttl) {
			return ActionUnchanged, nil
		}
		if ttl == 0 {
			ttl = picked.TTL
		}
		item := map[string]any{
			"RecordId": picked.RecordID, "Name": edgeOneASCII(r.FQDN()), "Type": r.Type, "Content": r.Value,
			"Location": edgeOneLocation(picked.Location),
		}
		if ttl > 0 {
			item["TTL"] = ttl
		}
		err := e.call(ctx, "ModifyDnsRecords", map[string]any{"ZoneId": zid, "DnsRecords": []any{item}}, nil)
		if err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if ttl == 0 {
		ttl = edgeOneDefaultTTL
	}
	if err := e.createRecord(ctx, zid, r.FQDN(), r.Type, r.Value, r.Line, ttl); err != nil {
		return "", err
	}
	return ActionCreated, nil
}

// createRecord 新建一条记录。默认线路不传 Location，避免套餐不支持线路时报错。
func (e *edgeOne) createRecord(ctx context.Context, zid, fqdn, typ, value, line string, ttl int) error {
	params := map[string]any{
		"ZoneId": zid, "Name": edgeOneASCII(fqdn), "Type": typ, "Content": value, "TTL": ttl,
	}
	if line != "" && !strings.EqualFold(line, "Default") {
		params["Location"] = line
	}
	return e.call(ctx, "CreateDnsRecord", params, nil)
}

// AddTXT 为 ACME DNS-01 验证新增 TXT 记录。
func (e *edgeOne) AddTXT(ctx context.Context, zone, sub, value string) error {
	zid, err := e.zoneID(ctx, zone)
	if err != nil {
		return err
	}
	return e.createRecord(ctx, zid, Record{Zone: zone, Sub: sub}.FQDN(), "TXT", value, "", txtTTL)
}

// DeleteTXT 按值删除 TXT 记录，不存在时视为成功。
func (e *edgeOne) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	zid, err := e.zoneID(ctx, zone)
	if err != nil {
		return err
	}
	recs, err := e.listRecords(ctx, zid, Record{Zone: zone, Sub: sub}.FQDN(), "TXT")
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Content) == value {
			return e.deleteRecords(ctx, zid, []string{rec.RecordID})
		}
	}
	return nil
}

func (e *edgeOne) deleteRecords(ctx context.Context, zid string, ids []string) error {
	return e.call(ctx, "DeleteDnsRecords", map[string]any{"ZoneId": zid, "RecordIds": ids}, nil)
}

func (e *edgeOne) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	zid, err := e.zoneID(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	recs, err := e.listRecords(ctx, zid, r.FQDN(), r.Type)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range recs {
		if !lineMatch(rec.Location, r.Line, "Default") {
			continue
		}
		out = append(out, remoteRecord{id: rec.RecordID, scope: zid, values: []string{rec.Content}, ttl: rec.TTL})
	}
	return out, nil
}

func (e *edgeOne) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return e.deleteRecords(ctx, rec.scope, []string{rec.id})
}

var (
	_ TXTProvider   = (*edgeOne)(nil)
	_ recordDeleter = (*edgeOne)(nil)
)
