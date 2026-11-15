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
	"sort"
	"strconv"
	"strings"
	"time"
)

// 百度智能云 DNS（云解析 BCD）记录管理，认证方式 bce-auth-v1。
// 文档：https://cloud.baidu.com/doc/BCD/s/4jwvymhs7
func init() {
	register(Meta{
		Name:           "baiducloud",
		Title:          "百度智能云 DNS",
		IDLabel:        "AccessKey ID",
		SecretLabel:    "SecretAccessKey",
		SupportsACME:   true,
		SupportsDelete: true,
		DocURL:         "https://console.bce.baidu.com/iam/#/iam/accesslist",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &baiduCloud{id: c.ID, secret: c.Secret, hc: hc, endpoint: "https://bcd.baidubce.com", now: time.Now}, nil
	})
}

const (
	// 解析 TTL 允许 60~3600 秒，默认 600 秒
	baiduCloudMinTTL     = 60
	baiduCloudMaxTTL     = 3600
	baiduCloudDefaultTTL = 600
	// 列表接口 pageSize 的默认值，翻页以不足一页作为结束
	baiduCloudPageSize = 100
	baiduCloudMaxPages = 100
	// 认证字符串有效期（秒）
	baiduCloudExpire = 1800
)

type baiduCloud struct {
	id, secret string
	hc         *http.Client
	endpoint   string
	now        func() time.Time
}

// baiduCloudRecord 是列表接口返回的一条解析记录。注意列表返回 rdtype，而增改接口的请求字段为 rdType。
type baiduCloudRecord struct {
	RecordID int64  `json:"recordId"`
	Domain   string `json:"domain"`
	View     string `json:"view"`
	RdType   string `json:"rdtype"`
	TTL      int    `json:"ttl"`
	RData    string `json:"rdata"`
	ZoneName string `json:"zoneName"`
	Status   string `json:"status"`
}

// bceEncode 按 BCE 规范编码：保留 A-Z a-z 0-9 - . _ ~，其余字节百分号编码（大写十六进制）。
// keepSlash 为 true 时不编码 "/"，用于路径。
func bceEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// bceAuthorization 生成 bce-auth-v1 认证字符串。
// headers 是参与签名的请求头（名称小写）；signedHeaders 写入认证字符串，可为空表示只签名推荐头。
func bceAuthorization(ak, sk, method, path string, query url.Values, headers map[string]string, signedHeaders string, ts time.Time, expire int) string {
	prefix := "bce-auth-v1/" + ak + "/" + ts.UTC().Format("2006-01-02T15:04:05Z") + "/" + strconv.Itoa(expire)
	// 规范化查询串：忽略 authorization，键值编码后按字典序排序
	var qs []string
	for k, vs := range query {
		if strings.EqualFold(k, "authorization") {
			continue
		}
		for _, v := range vs {
			qs = append(qs, bceEncode(k, false)+"="+bceEncode(v, false))
		}
	}
	sort.Strings(qs)
	// 规范化请求头：名称转小写、值去首尾空白，空值忽略，整串排序
	var hs []string
	for k, v := range headers {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		hs = append(hs, bceEncode(strings.ToLower(k), false)+":"+bceEncode(v, false))
	}
	sort.Strings(hs)
	if path == "" {
		path = "/"
	}
	canonical := method + "\n" + bceEncode(path, true) + "\n" + strings.Join(qs, "&") + "\n" + strings.Join(hs, "\n")
	signingKey := bceHMACHex(sk, prefix)
	return prefix + "/" + signedHeaders + "/" + bceHMACHex(signingKey, canonical)
}

func bceHMACHex(key, msg string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

// call 以 POST JSON 调用接口；成功时增改删接口返回 200 且无响应体。
func (b *baiduCloud) call(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	u, err := url.Parse(b.endpoint)
	if err != nil {
		return err
	}
	req, err := ctxReq(ctx, http.MethodPost, strings.TrimRight(b.endpoint, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	ts := b.now()
	date := ts.UTC().Format("2006-01-02T15:04:05Z")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("x-bce-date", date)
	req.Header.Set("Authorization", bceAuthorization(b.id, b.secret, http.MethodPost, path, nil,
		map[string]string{"host": u.Host, "x-bce-date": date}, "host;x-bce-date", ts, baiduCloudExpire))
	err = doJSON(b.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		var be struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if jerr := jsonUnmarshalString(he.Body, &be); jerr == nil && be.Code != "" {
			return fmt.Errorf("百度智能云 DNS %s: %s (%s)", path, be.Message, be.Code)
		}
	}
	if err != nil {
		return fmt.Errorf("百度智能云 DNS %s: %w", path, err)
	}
	return nil
}

// listRecords 分页取出域名下全部解析记录。
func (b *baiduCloud) listRecords(ctx context.Context, zone string) ([]baiduCloudRecord, error) {
	var all []baiduCloudRecord
	var prevFirst int64
	for page := 1; page <= baiduCloudMaxPages; page++ {
		var resp struct {
			Result []baiduCloudRecord `json:"result"`
		}
		err := b.call(ctx, "/v1/domain/resolve/list", map[string]any{
			"domain": zone, "pageNo": page, "pageSize": baiduCloudPageSize,
		}, &resp)
		if err != nil {
			return nil, err
		}
		// 服务端若忽略页码会重复返回同一页，遇到重复即停止
		if page > 1 && len(resp.Result) > 0 && resp.Result[0].RecordID == prevFirst {
			break
		}
		all = append(all, resp.Result...)
		if len(resp.Result) < baiduCloudPageSize {
			break
		}
		prevFirst = resp.Result[0].RecordID
	}
	return all, nil
}

// baiduCloudTTL 把 TTL 限制在 60~3600 秒；0 表示未指定。
func baiduCloudTTL(ttl int) int {
	if ttl <= 0 {
		return 0
	}
	if ttl < baiduCloudMinTTL {
		return baiduCloudMinTTL
	}
	if ttl > baiduCloudMaxTTL {
		return baiduCloudMaxTTL
	}
	return ttl
}

// baiduCloudMatch 在记录中找出主机记录与类型都相同、位于默认线路的记录。
func baiduCloudMatch(recs []baiduCloudRecord, r Record) []baiduCloudRecord {
	var out []baiduCloudRecord
	for _, rec := range recs {
		if !strings.EqualFold(rec.Domain, r.RR()) || !strings.EqualFold(rec.RdType, r.Type) {
			continue
		}
		// 只处理默认线路，避免改动运营商线路等其他线路的记录
		if rec.View != "" && !strings.EqualFold(rec.View, "DEFAULT") {
			continue
		}
		out = append(out, rec)
	}
	return out
}

func (b *baiduCloud) Upsert(ctx context.Context, r Record) (string, error) {
	recs, err := b.listRecords(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	ttl := baiduCloudTTL(r.TTL)
	if matched := baiduCloudMatch(recs, r); len(matched) > 0 {
		rec := matched[0]
		if sameValue(r.Type, rec.RData, r.Value) && (ttl == 0 || rec.TTL == ttl) {
			return ActionUnchanged, nil
		}
		if ttl == 0 {
			ttl = rec.TTL
		}
		// 编辑接口只允许改 ttl、rdType、rdata，其余字段需与原记录一致
		err := b.call(ctx, "/v1/domain/resolve/edit", map[string]any{
			"recordId": rec.RecordID, "domain": rec.Domain, "view": rec.View, "rdType": r.Type,
			"ttl": ttl, "rdata": r.Value, "zoneName": r.Zone,
		}, nil)
		if err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if ttl == 0 {
		ttl = baiduCloudDefaultTTL
	}
	if err := b.addRecord(ctx, r.Zone, r.RR(), r.Type, r.Value, ttl); err != nil {
		return "", err
	}
	return ActionCreated, nil
}

func (b *baiduCloud) addRecord(ctx context.Context, zone, host, typ, value string, ttl int) error {
	return b.call(ctx, "/v1/domain/resolve/add", map[string]any{
		"domain": host, "rdType": typ, "ttl": ttl, "rdata": value, "zoneName": zone,
	}, nil)
}

func (b *baiduCloud) deleteRecord(ctx context.Context, zone string, id int64) error {
	return b.call(ctx, "/v1/domain/resolve/delete", map[string]any{"zoneName": zone, "recordId": id}, nil)
}

// AddTXT 为 ACME DNS-01 验证新增 TXT 记录。
func (b *baiduCloud) AddTXT(ctx context.Context, zone, sub, value string) error {
	return b.addRecord(ctx, zone, Record{Zone: zone, Sub: sub}.RR(), "TXT", value, txtTTL)
}

// DeleteTXT 按值删除 TXT 记录，不存在时视为成功。
func (b *baiduCloud) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	recs, err := b.listRecords(ctx, zone)
	if err != nil {
		return err
	}
	for _, rec := range baiduCloudMatch(recs, Record{Zone: zone, Sub: sub, Type: "TXT"}) {
		if unquote(rec.RData) == value {
			return b.deleteRecord(ctx, zone, rec.RecordID)
		}
	}
	return nil
}

func (b *baiduCloud) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := b.listRecords(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range baiduCloudMatch(recs, r) {
		out = append(out, remoteRecord{id: strconv.FormatInt(rec.RecordID, 10), values: []string{rec.RData}, ttl: rec.TTL})
	}
	return out, nil
}

func (b *baiduCloud) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	id, _ := strconv.ParseInt(rec.id, 10, 64)
	return b.deleteRecord(ctx, r.Zone, id)
}

var (
	_ TXTProvider   = (*baiduCloud)(nil)
	_ recordDeleter = (*baiduCloud)(nil)
)
