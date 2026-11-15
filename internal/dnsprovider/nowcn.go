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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 时代互联 Now.cn、新网互联 Eranet、Tnet.hk 共用同一套“域名独立解析 API”：
// 所有接口使用 GET，参数放在查询串里，按阿里云 RPC 风格用 HMAC-SHA1 签名。
// 本文件实现共用部分（nowcnAPI），eranet.go 与 tnethk.go 只提供各自的接口地址与说明。
//
// 接口：/api/dns/DescribeRecordIndex、AddDomainRecord、UpdateDomainRecord、DeleteDomainRecord。
// 凭据：ID 为 AccessInstanceID，Secret 为 AccessKeySecret。

func init() {
	register(Meta{
		Name:           "nowcn",
		Title:          "时代互联 (Now.cn)",
		IDLabel:        "AccessInstanceID",
		SecretLabel:    "AccessKeySecret",
		DocURL:         "https://help.now.cn/domain/resolve/20260310102214/",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		api, err := newNowcnAPI("时代互联", "https://www.now.cn", c, hc)
		if err != nil {
			return nil, err
		}
		return &nowcn{nowcnAPI: *api}, nil
	})
}

type nowcn struct {
	nowcnAPI
}

// nowcnAPI 是三家服务商共用的接口客户端。
type nowcnAPI struct {
	// name 用于错误信息中的服务商名称
	name     string
	id       string
	secret   string
	hc       *http.Client
	endpoint string
}

func newNowcnAPI(name, endpoint string, c Credentials, hc *http.Client) (*nowcnAPI, error) {
	if err := requireCreds(c, true, true); err != nil {
		return nil, err
	}
	return &nowcnAPI{name: name, id: strings.TrimSpace(c.ID), secret: strings.TrimSpace(c.Secret), hc: hc, endpoint: endpoint}, nil
}

// nowcnNum 兼容数字与字符串两种写法的数值：记录列表里的 id 是数字，新增/修改接口返回的 Id 是字符串。
type nowcnNum string

func (n *nowcnNum) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*n = ""
		return nil
	}
	*n = nowcnNum(strings.Trim(s, `"`))
	return nil
}

func (n nowcnNum) int() int {
	v, _ := strconv.Atoi(string(n))
	return v
}

type nowcnRecord struct {
	ID    nowcnNum `json:"id"`
	Host  string   `json:"Host"`
	Type  string   `json:"Type"`
	Value string   `json:"Value"`
	TTL   nowcnNum `json:"Ttl"`
}

type nowcnResult struct {
	Error string        `json:"error"`
	Data  []nowcnRecord `json:"Data"`
}

// nowcnPercentEncode 按签名规范编码：RFC3986，空格为 %20，保留 ~。
func nowcnPercentEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	return strings.ReplaceAll(e, "%7E", "~")
}

// nowcnSign 返回 params 的签名。params 需已包含公共参数（不含 Signature）。
func nowcnSign(secret string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, nowcnPercentEncode(k)+"="+nowcnPercentEncode(params[k]))
	}
	toSign := "GET&" + nowcnPercentEncode("/") + "&" + nowcnPercentEncode(strings.Join(parts, "&"))
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(toSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// signedQuery 加上公共参数与签名，生成查询串。
func (a *nowcnAPI) signedQuery(params map[string]string) string {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	all := map[string]string{
		"AccessInstanceID": a.id,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   hex.EncodeToString(nonce),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	for k, v := range params {
		all[k] = v
	}
	all["Signature"] = nowcnSign(a.secret, all)
	// 签名后再统一编码，保证服务端按同样规则还原
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, nowcnPercentEncode(k)+"="+nowcnPercentEncode(all[k]))
	}
	return strings.Join(parts, "&")
}

// call 调用接口。鉴权失败时服务端可能返回 200 与空响应体，这里按错误处理。
func (a *nowcnAPI) call(ctx context.Context, action string, params map[string]string) (*nowcnResult, error) {
	req, err := ctxReq(ctx, http.MethodGet, a.endpoint+"/api/dns/"+action+"?"+a.signedQuery(params), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var raw json.RawMessage
	if err := doJSON(a.hc, req, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", a.name, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("%s: 响应为空，请检查 AccessInstanceID 与 AccessKeySecret", a.name)
	}
	var out nowcnResult
	if err := json.Unmarshal(raw, &out); err != nil {
		// 增删改接口可能返回对象以外的合法 JSON（如 true），视为成功；列表接口必须是对象
		if action == "DescribeRecordIndex" {
			return nil, fmt.Errorf("%s: 解析响应失败: %w: %s", a.name, err, truncate(string(raw), 200))
		}
		return &nowcnResult{}, nil
	}
	if out.Error != "" {
		return nil, errors.New(a.name + ": " + out.Error)
	}
	return &out, nil
}

// list 查询域名下指定主机记录与类型的记录。
func (a *nowcnAPI) list(ctx context.Context, zone, host, typ string) ([]nowcnRecord, error) {
	res, err := a.call(ctx, "DescribeRecordIndex", map[string]string{"Domain": zone, "Host": host, "Type": typ})
	if err != nil {
		return nil, err
	}
	// 服务端的主机/类型过滤是否精确未在文档中说明，这里再精确过滤一遍
	var out []nowcnRecord
	for _, rec := range res.Data {
		if dnslaSameHost(rec.Host, host) && strings.EqualFold(rec.Type, typ) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (a *nowcnAPI) Upsert(ctx context.Context, r Record) (string, error) {
	host := r.RR()
	recs, err := a.list(ctx, r.Zone, host, r.Type)
	if err != nil {
		return "", err
	}
	ttl := r.TTL
	if len(recs) > 0 {
		cur := recs[0]
		if ttl <= 0 {
			ttl = cur.TTL.int()
		}
		if ttl <= 0 {
			ttl = 600
		}
		// 接口未返回 TTL 时无法比较，只按值判断
		if cur.Value == r.Value && (r.TTL <= 0 || cur.TTL.int() == 0 || cur.TTL.int() == ttl) {
			return ActionUnchanged, nil
		}
		_, err := a.call(ctx, "UpdateDomainRecord", map[string]string{
			"Id": string(cur.ID), "Domain": r.Zone, "Host": host, "Type": r.Type, "Value": r.Value, "Ttl": strconv.Itoa(ttl),
		})
		if err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if ttl <= 0 {
		ttl = 600
	}
	_, err = a.call(ctx, "AddDomainRecord", map[string]string{
		"Domain": r.Zone, "Host": host, "Type": r.Type, "Value": r.Value, "Ttl": strconv.Itoa(ttl),
	})
	if err != nil {
		return "", err
	}
	return ActionCreated, nil
}

func (a *nowcnAPI) AddTXT(ctx context.Context, zone, sub, value string) error {
	_, err := a.call(ctx, "AddDomainRecord", map[string]string{
		"Domain": zone, "Host": Record{Zone: zone, Sub: sub}.RR(), "Type": "TXT", "Value": value, "Ttl": strconv.Itoa(txtTTL),
	})
	return err
}

func (a *nowcnAPI) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	recs, err := a.list(ctx, zone, Record{Zone: zone, Sub: sub}.RR(), "TXT")
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Value) == value {
			return a.remove(ctx, zone, string(rec.ID))
		}
	}
	return nil
}

func (a *nowcnAPI) remove(ctx context.Context, zone, id string) error {
	_, err := a.call(ctx, "DeleteDomainRecord", map[string]string{"Domain": zone, "Id": id})
	return err
}

func (a *nowcnAPI) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := a.list(ctx, r.Zone, r.RR(), r.Type)
	if err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, remoteRecord{id: string(rec.ID), values: []string{rec.Value}, ttl: rec.TTL.int()})
	}
	return out, nil
}

func (a *nowcnAPI) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	return a.remove(ctx, r.Zone, rec.id)
}

// 编译期确认实现了接口
var (
	_ TXTProvider   = (*nowcn)(nil)
	_ recordDeleter = (*nowcn)(nil)
)
