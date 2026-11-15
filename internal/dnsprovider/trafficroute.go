package dnsprovider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 火山引擎 DNS（TrafficRoute）记录管理，接口版本 2018-08-01，签名算法 HMAC-SHA256（服务 dns，地域 cn-north-1）。
// 文档：https://www.volcengine.com/docs/6758/155088
func init() {
	register(Meta{
		Name:           "trafficroute",
		Title:          "火山引擎 DNS",
		IDLabel:        "Access Key ID",
		SecretLabel:    "Secret Access Key",
		SupportsLine:   true,
		SupportsACME:   true,
		SupportsDelete: true,
		DocURL:         "https://console.volcengine.com/iam/keymanage/",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &trafficRoute{id: c.ID, secret: c.Secret, hc: hc, endpoint: "https://open.volcengineapi.com", now: time.Now}, nil
	})
}

const (
	trafficRouteVersion = "2018-08-01"
	trafficRouteService = "dns"
	trafficRouteRegion  = "cn-north-1"
	// 列表接口单页最大 500 条
	trafficRoutePageSize = 500
	trafficRouteMaxPages = 50
)

type trafficRoute struct {
	id, secret string
	hc         *http.Client
	endpoint   string
	now        func() time.Time
}

type trafficRouteRecord struct {
	RecordID string `json:"RecordID"`
	Host     string `json:"Host"`
	Type     string `json:"Type"`
	Value    string `json:"Value"`
	TTL      int    `json:"TTL"`
	Line     string `json:"Line"`
}

type trafficRouteError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func trafficRouteHMAC(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// trafficRouteAuthorization 计算请求头 Authorization。query 为已按 RFC3986 编码并排序的查询串。
func trafficRouteAuthorization(ak, sk, method, host, query, contentType, bodyHash, xdate string) string {
	short := xdate[:8]
	signedHeaders := "content-type;host;x-content-sha256;x-date"
	canonical := strings.Join([]string{
		method,
		"/",
		query,
		"content-type:" + contentType + "\nhost:" + host + "\nx-content-sha256:" + bodyHash + "\nx-date:" + xdate,
		"",
		signedHeaders,
		bodyHash,
	}, "\n")
	scope := short + "/" + trafficRouteRegion + "/" + trafficRouteService + "/request"
	stringToSign := strings.Join([]string{"HMAC-SHA256", xdate, scope, sha256Hex([]byte(canonical))}, "\n")
	kDate := trafficRouteHMAC([]byte(sk), short)
	kRegion := trafficRouteHMAC(kDate, trafficRouteRegion)
	kService := trafficRouteHMAC(kRegion, trafficRouteService)
	kSigning := trafficRouteHMAC(kService, "request")
	sig := hex.EncodeToString(trafficRouteHMAC(kSigning, stringToSign))
	return "HMAC-SHA256 Credential=" + ak + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + sig
}

// call 调用接口。读取类动作用 GET 且参数在查询串中，写入类动作用 POST 且参数为 JSON 请求体。
func (t *trafficRoute) call(ctx context.Context, method, action string, query url.Values, body any, out any) error {
	u, err := url.Parse(t.endpoint)
	if err != nil {
		return err
	}
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("Action", action)
	q.Set("Version", trafficRouteVersion)
	// 规范查询串要求空格编码为 %20，url.Values.Encode 会按键排序但把空格写成 +
	rawQuery := strings.ReplaceAll(q.Encode(), "+", "%20")
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := ctxReq(ctx, method, strings.TrimRight(t.endpoint, "/")+"/?"+rawQuery, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	xdate := t.now().UTC().Format("20060102T150405Z")
	bodyHash := sha256Hex(payload)
	const contentType = "application/json"
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Date", xdate)
	req.Header.Set("X-Content-Sha256", bodyHash)
	req.Header.Set("Authorization", trafficRouteAuthorization(t.id, t.secret, method, u.Host, rawQuery, contentType, bodyHash, xdate))
	var env struct {
		ResponseMetadata struct {
			Error *trafficRouteError `json:"Error"`
		} `json:"ResponseMetadata"`
		Result json.RawMessage `json:"Result"`
	}
	err = doJSON(t.hc, req, &env)
	if he, ok := err.(*HTTPError); ok {
		var e struct {
			ResponseMetadata struct {
				Error *trafficRouteError `json:"Error"`
			} `json:"ResponseMetadata"`
		}
		if jerr := jsonUnmarshalString(he.Body, &e); jerr == nil && e.ResponseMetadata.Error != nil && e.ResponseMetadata.Error.Code != "" {
			return fmt.Errorf("火山引擎 DNS %s: %s (%s)", action, e.ResponseMetadata.Error.Message, e.ResponseMetadata.Error.Code)
		}
	}
	if err != nil {
		return fmt.Errorf("火山引擎 DNS %s: %w", action, err)
	}
	if e := env.ResponseMetadata.Error; e != nil && e.Code != "" {
		return fmt.Errorf("火山引擎 DNS %s: %s (%s)", action, e.Message, e.Code)
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// zoneID 按域名查询 ZID，名称忽略大小写。
func (t *trafficRoute) zoneID(ctx context.Context, zone string) (int64, error) {
	name := strings.TrimSuffix(strings.TrimSpace(zone), ".")
	for page := 1; page <= trafficRouteMaxPages; page++ {
		var resp struct {
			Total int `json:"Total"`
			Zones []struct {
				ZID      int64  `json:"ZID"`
				ZoneName string `json:"ZoneName"`
			} `json:"Zones"`
		}
		err := t.call(ctx, http.MethodGet, "ListZones", url.Values{
			"Key": {name}, "SearchMode": {"exact"},
			"PageNumber": {strconv.Itoa(page)}, "PageSize": {strconv.Itoa(trafficRoutePageSize)},
		}, nil, &resp)
		if err != nil {
			return 0, err
		}
		for _, z := range resp.Zones {
			if strings.EqualFold(z.ZoneName, name) {
				return z.ZID, nil
			}
		}
		if len(resp.Zones) < trafficRoutePageSize || page*trafficRoutePageSize >= resp.Total {
			break
		}
	}
	return 0, fmt.Errorf("火山引擎 DNS 账号下未找到域名 %s", zone)
}

// listRecords 查询主机记录与类型都匹配的解析记录，并按线路过滤（未指定线路时取默认线路）。
func (t *trafficRoute) listRecords(ctx context.Context, zid int64, r Record) ([]trafficRouteRecord, error) {
	var out []trafficRouteRecord
	for page := 1; page <= trafficRouteMaxPages; page++ {
		var resp struct {
			TotalCount int                  `json:"TotalCount"`
			Records    []trafficRouteRecord `json:"Records"`
		}
		err := t.call(ctx, http.MethodGet, "ListRecords", url.Values{
			"ZID": {strconv.FormatInt(zid, 10)}, "Host": {r.RR()}, "Type": {r.Type}, "SearchMode": {"exact"},
			"PageNumber": {strconv.Itoa(page)}, "PageSize": {strconv.Itoa(trafficRoutePageSize)},
		}, nil, &resp)
		if err != nil {
			return nil, err
		}
		for _, rec := range resp.Records {
			// 接口按前缀匹配的可能性不可排除，再按主机记录与类型精确过滤
			if strings.EqualFold(rec.Host, r.RR()) && rec.Type == r.Type && lineMatch(rec.Line, r.Line, "default") {
				out = append(out, rec)
			}
		}
		if len(resp.Records) < trafficRoutePageSize || page*trafficRoutePageSize >= resp.TotalCount {
			break
		}
	}
	return out, nil
}

func (t *trafficRoute) createRecord(ctx context.Context, zid int64, r Record, ttl int) error {
	line := r.Line
	if line == "" {
		line = "default"
	}
	body := map[string]any{"ZID": zid, "Host": r.RR(), "Type": r.Type, "Value": r.Value, "Line": line}
	if ttl > 0 {
		body["TTL"] = ttl
	}
	return t.call(ctx, http.MethodPost, "CreateRecord", nil, body, nil)
}

func (t *trafficRoute) Upsert(ctx context.Context, r Record) (string, error) {
	zid, err := t.zoneID(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	recs, err := t.listRecords(ctx, zid, r)
	if err != nil {
		return "", err
	}
	if len(recs) == 0 {
		if err := t.createRecord(ctx, zid, r, r.TTL); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	rec := recs[0]
	if sameValue(r.Type, rec.Value, r.Value) && (r.TTL == 0 || rec.TTL == r.TTL) {
		return ActionUnchanged, nil
	}
	// 更新接口必须带主机记录与线路，沿用现有记录的值
	body := map[string]any{"RecordID": rec.RecordID, "Host": rec.Host, "Line": rec.Line, "Type": r.Type, "Value": r.Value}
	if r.TTL > 0 {
		body["TTL"] = r.TTL
	}
	if err := t.call(ctx, http.MethodPost, "UpdateRecord", nil, body, nil); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

// AddTXT 为 ACME DNS-01 验证新增 TXT 记录。
func (t *trafficRoute) AddTXT(ctx context.Context, zone, sub, value string) error {
	zid, err := t.zoneID(ctx, zone)
	if err != nil {
		return err
	}
	return t.createRecord(ctx, zid, Record{Zone: zone, Sub: sub, Type: "TXT", Value: value}, txtTTL)
}

// DeleteTXT 按值删除 TXT 记录，不存在时视为成功。
func (t *trafficRoute) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	zid, err := t.zoneID(ctx, zone)
	if err != nil {
		return err
	}
	recs, err := t.listRecords(ctx, zid, Record{Zone: zone, Sub: sub, Type: "TXT"})
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Value) == value {
			return t.deleteRecord(ctx, rec.RecordID)
		}
	}
	return nil
}

func (t *trafficRoute) deleteRecord(ctx context.Context, id string) error {
	return t.call(ctx, http.MethodPost, "DeleteRecord", nil, map[string]any{"RecordID": id}, nil)
}

func (t *trafficRoute) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	zid, err := t.zoneID(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	recs, err := t.listRecords(ctx, zid, r)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range recs {
		out = append(out, remoteRecord{id: rec.RecordID, scope: strconv.FormatInt(zid, 10), values: []string{rec.Value}, ttl: rec.TTL})
	}
	return out, nil
}

func (t *trafficRoute) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return t.deleteRecord(ctx, rec.id)
}

var (
	_ TXTProvider   = (*trafficRoute)(nil)
	_ recordDeleter = (*trafficRoute)(nil)
)
