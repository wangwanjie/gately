package dnsprovider

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 阿里云 ESA（边缘安全加速）DNS 记录管理，RPC 风格接口 2024-09-10，签名算法 HMAC-SHA1（Signature Version 1.0）。
// ESA 把同名的 A 与 AAAA 合并为一条类型为 "A/AAAA" 的记录，Data.Value 是逗号分隔的多个 IP。
// 文档：https://help.aliyun.com/zh/edge-security-acceleration/esa/api-esa-2024-09-10-listrecords
func init() {
	register(Meta{
		Name:        "aliesa",
		Title:       "阿里云 ESA（边缘安全加速）",
		IDLabel:     "AccessKey ID",
		SecretLabel: "AccessKey Secret",
		Extra: []Field{
			{Key: "endpoint", Label: "接口地址", Placeholder: "默认 https://esa.cn-hangzhou.aliyuncs.com；国际站为 https://esa.ap-southeast-1.aliyuncs.com"},
		},
		SupportsACME:   true,
		SupportsDelete: true,
		DocURL:         "https://ram.console.aliyun.com/manage/ak",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		endpoint := strings.TrimSpace(c.Extra["endpoint"])
		if endpoint == "" {
			endpoint = "https://esa.cn-hangzhou.aliyuncs.com"
		}
		if u, err := url.Parse(endpoint); err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("接口地址无效，需要 https:// 开头的完整地址")
		}
		return &aliESA{id: c.ID, secret: c.Secret, hc: hc, endpoint: endpoint, now: time.Now}, nil
	})
}

const (
	aliESAVersion = "2024-09-10"
	// ESA 中 A 与 AAAA 合并后的记录类型
	aliESAAddrType = "A/AAAA"
	// 新建记录未指定 TTL 时的默认值；接口允许 30~86400，1 表示自动
	aliESADefaultTTL = 600
	aliESAMinTTL     = 30
	aliESAMaxTTL     = 86400
	// ListSites / ListRecords 单页最大 500 条
	aliESAPageSize = 500
	aliESAMaxPages = 50
)

type aliESA struct {
	id, secret string
	hc         *http.Client
	endpoint   string
	now        func() time.Time
}

type aliESASite struct {
	SiteID     int64  `json:"SiteId"`
	SiteName   string `json:"SiteName"`
	AccessType string `json:"AccessType"`
}

type aliESARecord struct {
	RecordID   int64  `json:"RecordId"`
	RecordName string `json:"RecordName"`
	RecordType string `json:"RecordType"`
	Ttl        int    `json:"Ttl"`
	Data       struct {
		Value string `json:"Value"`
	} `json:"Data"`
}

// aliESASign 计算 RPC 签名并返回完整查询串。与云解析不同，ESA 的写操作使用 POST，参与签名的 HTTP 方法随之变化。
func aliESASign(method string, params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = aliPercentEncode(k) + "=" + aliPercentEncode(params[k])
	}
	canonical := strings.Join(parts, "&")
	stringToSign := method + "&%2F&" + aliPercentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return canonical + "&Signature=" + aliPercentEncode(sig)
}

// call 发送一次接口调用，参数全部放在查询串中。
func (a *aliESA) call(ctx context.Context, method, action string, params map[string]string, out any) error {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	all := map[string]string{
		"Format":           "JSON",
		"Version":          aliESAVersion,
		"AccessKeyId":      a.id,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   hex.EncodeToString(nonce),
		"Timestamp":        a.now().UTC().Format("2006-01-02T15:04:05Z"),
		"Action":           action,
	}
	for k, v := range params {
		all[k] = v
	}
	u := strings.TrimRight(a.endpoint, "/") + "/?" + aliESASign(method, all, a.secret)
	req, err := ctxReq(ctx, method, u, nil)
	if err != nil {
		return err
	}
	err = doJSON(a.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		var ae aliError
		if jerr := jsonUnmarshalString(he.Body, &ae); jerr == nil && ae.Code != "" {
			return fmt.Errorf("阿里云 ESA %s: %s (%s)", action, ae.Message, ae.Code)
		}
	}
	if err != nil {
		return fmt.Errorf("阿里云 ESA %s: %w", action, err)
	}
	return nil
}

// site 按站点名称查询站点，名称忽略大小写。
func (a *aliESA) site(ctx context.Context, zone string) (aliESASite, error) {
	name := strings.TrimSuffix(strings.TrimSpace(zone), ".")
	for page := 1; page <= aliESAMaxPages; page++ {
		var resp struct {
			TotalCount int          `json:"TotalCount"`
			Sites      []aliESASite `json:"Sites"`
		}
		err := a.call(ctx, http.MethodGet, "ListSites", map[string]string{
			"SiteName": name, "SiteSearchType": "exact",
			"PageNumber": strconv.Itoa(page), "PageSize": strconv.Itoa(aliESAPageSize),
		}, &resp)
		if err != nil {
			return aliESASite{}, err
		}
		for _, s := range resp.Sites {
			if strings.EqualFold(s.SiteName, name) {
				return s, nil
			}
		}
		if len(resp.Sites) < aliESAPageSize || page*aliESAPageSize >= resp.TotalCount {
			break
		}
	}
	return aliESASite{}, fmt.Errorf("阿里云 ESA 账号下未找到站点 %s", zone)
}

// listRecords 查询站点下指定名称与类型的记录（名称精确匹配）。
func (a *aliESA) listRecords(ctx context.Context, siteID int64, fqdn, typ string) ([]aliESARecord, error) {
	var out []aliESARecord
	for page := 1; page <= aliESAMaxPages; page++ {
		var resp struct {
			TotalCount int            `json:"TotalCount"`
			Records    []aliESARecord `json:"Records"`
		}
		err := a.call(ctx, http.MethodGet, "ListRecords", map[string]string{
			"SiteId": strconv.FormatInt(siteID, 10), "RecordName": fqdn, "RecordMatchType": "exact", "Type": typ,
			"PageNumber": strconv.Itoa(page), "PageSize": strconv.Itoa(aliESAPageSize),
		}, &resp)
		if err != nil {
			return nil, err
		}
		for _, rec := range resp.Records {
			if strings.EqualFold(rec.RecordName, fqdn) && (rec.RecordType == typ || (typ == aliESAAddrType && (rec.RecordType == "A" || rec.RecordType == "AAAA"))) {
				out = append(out, rec)
			}
		}
		if len(resp.Records) < aliESAPageSize || page*aliESAPageSize >= resp.TotalCount {
			break
		}
	}
	return out, nil
}

// aliESAType 返回 ESA 使用的记录类型：A 与 AAAA 合并为 A/AAAA。
func aliESAType(typ string) string {
	if typ == "A" || typ == "AAAA" {
		return aliESAAddrType
	}
	return typ
}

// aliESATTL 把 TTL 限制在接口允许的范围内；0 表示未指定，1 表示自动。
func aliESATTL(ttl int) int {
	if ttl <= 1 {
		return 0
	}
	if ttl < aliESAMinTTL {
		return aliESAMinTTL
	}
	if ttl > aliESAMaxTTL {
		return aliESAMaxTTL
	}
	return ttl
}

// aliESASplit 拆分逗号分隔的记录值。
func aliESASplit(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// aliESAIsV6 判断是否为 IPv6 地址：IPv6 地址含冒号。
func aliESAIsV6(v string) bool {
	return strings.Contains(v, ":")
}

// aliESASameIP 比较两个 IP，IPv6 先规范化写法再比较。
func aliESASameIP(a, b string) bool {
	pa, errA := netip.ParseAddr(strings.TrimSpace(a))
	pb, errB := netip.ParseAddr(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	return pa == pb
}

// aliESAJoin 把 IPv4 与 IPv6 值合并成接口需要的逗号分隔字符串，IPv4 在前。
func aliESAJoin(vals []string) string {
	var v4, v6 []string
	for _, v := range vals {
		if aliESAIsV6(v) {
			v6 = append(v6, v)
		} else {
			v4 = append(v4, v)
		}
	}
	return strings.Join(append(v4, v6...), ",")
}

func aliESAData(value string) string {
	b, _ := json.Marshal(map[string]string{"Value": value})
	return string(b)
}

func (a *aliESA) createRecord(ctx context.Context, site aliESASite, fqdn, typ, value string, ttl int) error {
	params := map[string]string{
		"SiteId": strconv.FormatInt(site.SiteID, 10), "RecordName": fqdn, "Type": typ,
		"Data": aliESAData(value), "Ttl": strconv.Itoa(ttl),
	}
	// CNAME 接入的站点只能创建开启代理加速的地址记录
	if typ == aliESAAddrType && strings.EqualFold(site.AccessType, "CNAME") {
		params["Proxied"] = "true"
		params["BizName"] = "web"
	}
	return a.call(ctx, http.MethodPost, "CreateRecord", params, nil)
}

func (a *aliESA) updateRecord(ctx context.Context, id int64, typ, value string, ttl int) error {
	params := map[string]string{
		"RecordId": strconv.FormatInt(id, 10), "Type": typ, "Data": aliESAData(value), "Ttl": strconv.Itoa(ttl),
	}
	return a.call(ctx, http.MethodPost, "UpdateRecord", params, nil)
}

func (a *aliESA) deleteRecord(ctx context.Context, id int64) error {
	return a.call(ctx, http.MethodPost, "DeleteRecord", map[string]string{"RecordId": strconv.FormatInt(id, 10)}, nil)
}

func (a *aliESA) Upsert(ctx context.Context, r Record) (string, error) {
	site, err := a.site(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	typ := aliESAType(r.Type)
	recs, err := a.listRecords(ctx, site.SiteID, r.FQDN(), typ)
	if err != nil {
		return "", err
	}
	ttl := aliESATTL(r.TTL)
	if len(recs) == 0 {
		if ttl == 0 {
			ttl = aliESADefaultTTL
		}
		if err := a.createRecord(ctx, site, r.FQDN(), typ, r.Value, ttl); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	rec := recs[0]
	value := r.Value
	same := false
	if typ == aliESAAddrType {
		// 同名 A 与 AAAA 共用一条记录：只替换本次同步的地址族，保留另一族的值
		wantV6 := r.Type == "AAAA"
		keep := []string{}
		var cur []string
		for _, v := range aliESASplit(rec.Data.Value) {
			if aliESAIsV6(v) == wantV6 {
				cur = append(cur, v)
			} else {
				keep = append(keep, v)
			}
		}
		same = len(cur) == 1 && aliESASameIP(cur[0], r.Value)
		value = aliESAJoin(append(keep, r.Value))
	} else {
		same = sameValue(r.Type, rec.Data.Value, r.Value)
	}
	if same && (ttl == 0 || rec.Ttl == ttl) {
		return ActionUnchanged, nil
	}
	if ttl == 0 {
		ttl = rec.Ttl
		if ttl <= 0 {
			ttl = aliESADefaultTTL
		}
	}
	if err := a.updateRecord(ctx, rec.RecordID, typ, value, ttl); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

// AddTXT 为 ACME DNS-01 验证新增 TXT 记录。
func (a *aliESA) AddTXT(ctx context.Context, zone, sub, value string) error {
	site, err := a.site(ctx, zone)
	if err != nil {
		return err
	}
	return a.createRecord(ctx, site, Record{Zone: zone, Sub: sub}.FQDN(), "TXT", value, txtTTL)
}

// DeleteTXT 按值删除 TXT 记录，不存在时视为成功。
func (a *aliESA) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	site, err := a.site(ctx, zone)
	if err != nil {
		return err
	}
	recs, err := a.listRecords(ctx, site.SiteID, Record{Zone: zone, Sub: sub}.FQDN(), "TXT")
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Data.Value) == value {
			return a.deleteRecord(ctx, rec.RecordID)
		}
	}
	return nil
}

func (a *aliESA) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	site, err := a.site(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	recs, err := a.listRecords(ctx, site.SiteID, r.FQDN(), aliESAType(r.Type))
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range recs {
		vals := aliESASplit(rec.Data.Value)
		if aliESAType(r.Type) == aliESAAddrType {
			// 只取与本记录同地址族的值，另一族的值不属于这条动态域名
			var fam []string
			for _, v := range vals {
				if aliESAIsV6(v) == (r.Type == "AAAA") {
					fam = append(fam, v)
				}
			}
			vals = fam
		} else {
			vals = []string{rec.Data.Value}
		}
		if len(vals) == 0 {
			continue
		}
		out = append(out, remoteRecord{id: strconv.FormatInt(rec.RecordID, 10), scope: strconv.FormatInt(site.SiteID, 10), values: vals, ttl: rec.Ttl})
	}
	return out, nil
}

func (a *aliESA) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	id, _ := strconv.ParseInt(rec.id, 10, 64)
	if aliESAType(r.Type) != aliESAAddrType {
		return a.deleteRecord(ctx, id)
	}
	// 同名记录还保留另一地址族的值时，只去掉本记录的地址，其余值写回；全部移除后才删除整条记录
	siteID, _ := strconv.ParseInt(rec.scope, 10, 64)
	recs, err := a.listRecords(ctx, siteID, r.FQDN(), aliESAAddrType)
	if err != nil {
		return err
	}
	for _, cur := range recs {
		if cur.RecordID != id {
			continue
		}
		var remain []string
		for _, v := range aliESASplit(cur.Data.Value) {
			if aliESAIsV6(v) == (r.Type == "AAAA") && aliESASameIP(v, r.Value) {
				continue
			}
			remain = append(remain, v)
		}
		if len(remain) == 0 {
			return a.deleteRecord(ctx, id)
		}
		ttl := cur.Ttl
		if ttl <= 0 {
			ttl = aliESADefaultTTL
		}
		return a.updateRecord(ctx, id, aliESAAddrType, aliESAJoin(remain), ttl)
	}
	return nil
}

var (
	_ TXTProvider   = (*aliESA)(nil)
	_ recordDeleter = (*aliESA)(nil)
)
