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
)

// ClouDNS：所有接口都是带认证参数的 GET/POST 表单，失败时仍返回 HTTP 200，
// 通过响应里的 status=Failed 与 statusDescription 说明原因。
// 文档：https://www.cloudns.net/wiki/article/42/

func init() {
	register(Meta{
		Name:        "cloudns",
		Title:       "ClouDNS",
		IDLabel:     "Auth ID / Sub Auth ID / Sub Auth User",
		SecretLabel: "Auth Password",
		Extra: []Field{
			{Key: "authType", Label: "认证类型", Placeholder: "auth-id（默认）、sub-auth-id 或 sub-auth-user，与上面的 ID 对应"},
		},
		DocURL:         "https://www.cloudns.net/wiki/article/42/",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		authType := strings.TrimSpace(c.Extra["authType"])
		if authType == "" {
			authType = "auth-id"
		}
		switch authType {
		case "auth-id", "sub-auth-id", "sub-auth-user":
		default:
			return nil, errors.New("ClouDNS 认证类型只能是 auth-id、sub-auth-id 或 sub-auth-user")
		}
		return &cloudns{authType: authType, id: strings.TrimSpace(c.ID), password: c.Secret, hc: hc, endpoint: "https://api.cloudns.net/dns/"}, nil
	})
}

type cloudns struct {
	authType string
	id       string
	password string
	hc       *http.Client
	endpoint string
}

type cloudnsRecord struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"`
	Host  string      `json:"host"`
	Value string      `json:"record"`
	TTL   json.Number `json:"ttl"`
}

// cloudnsTTLs 是 ClouDNS 允许的 TTL，其他值会被拒绝。
var cloudnsTTLs = []int{60, 300, 900, 1800, 3600, 21600, 43200, 86400, 172800, 259200, 604800, 1209600, 2592000}

// cloudnsTTL 把 TTL 取整到不小于它的最近的允许值；0 表示默认 3600。
func cloudnsTTL(ttl int) int {
	if ttl <= 0 {
		return 3600
	}
	for _, v := range cloudnsTTLs {
		if ttl <= v {
			return v
		}
	}
	return cloudnsTTLs[len(cloudnsTTLs)-1]
}

// cloudnsHost 返回发给 ClouDNS 的主机记录。根域名直接写 "@"。
func cloudnsHost(sub string) string {
	if sub == "" {
		return "@"
	}
	return sub
}

// cloudnsHostEqual 判断响应里的 host 是否对应目标主机；根域名兼容 "@" 与空字符串两种返回。
func cloudnsHostEqual(got, want string) bool {
	if got == "" {
		got = "@"
	}
	return got == want
}

// call 发送请求并返回原始响应；失败状态（status=Failed）转成错误。
func (c *cloudns) call(ctx context.Context, action string, params url.Values) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form[k] = v
	}
	form.Set(c.authType, c.id)
	form.Set("auth-password", c.password)
	req, err := ctxReq(ctx, http.MethodPost, c.endpoint+action, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var raw json.RawMessage
	if err := doJSON(c.hc, req, &raw); err != nil {
		return nil, fmt.Errorf("ClouDNS: %w", err)
	}
	body := bytes.TrimSpace(raw)
	if len(body) > 0 && body[0] == '{' {
		var st struct {
			Status      json.RawMessage `json:"status"`
			Description string          `json:"statusDescription"`
		}
		if json.Unmarshal(body, &st) == nil && len(st.Status) > 0 {
			var s string
			if json.Unmarshal(st.Status, &s) == nil && s == "Failed" {
				return nil, fmt.Errorf("ClouDNS %s: %s", action, st.Description)
			}
		}
	}
	return body, nil
}

// list 返回指定主机与类型的记录（records.json 的 host、type 过滤参数），并在本地再过滤一次。
func (c *cloudns) list(ctx context.Context, zone, host, typ string) ([]cloudnsRecord, error) {
	params := url.Values{"domain-name": {zone}, "type": {typ}}
	params.Set("host", host)
	body, err := c.call(ctx, "records.json", params)
	if err != nil {
		return nil, err
	}
	// 没有记录时可能返回空数组
	if len(body) == 0 || body[0] != '{' {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ClouDNS: 解析记录列表失败: %w", err)
	}
	var out []cloudnsRecord
	for key, v := range m {
		var rec cloudnsRecord
		if err := json.Unmarshal(v, &rec); err != nil {
			continue
		}
		if rec.ID == "" {
			rec.ID = key
		}
		if rec.Type == typ && cloudnsHostEqual(rec.Host, host) {
			out = append(out, rec)
		}
	}
	return out, nil
}

// add 新增记录。官方文档参数表写 type，但示例 URL 与错误说明写 record-type，
// lego、acme.sh、libdns 等实际可用的客户端都发 record-type，两者同时发送以兼容。
func (c *cloudns) add(ctx context.Context, zone, host, typ, value string, ttl int) error {
	_, err := c.call(ctx, "add-record.json", url.Values{
		"domain-name": {zone}, "record-type": {typ}, "type": {typ}, "host": {host},
		"record": {value}, "ttl": {strconv.Itoa(cloudnsTTL(ttl))},
	})
	return err
}

// modify 修改记录。接口名为 mod-record.json（见 https://www.cloudns.net/wiki/article/60/ 示例地址），
// 记录 ID 参数为 record-id。
func (c *cloudns) modify(ctx context.Context, zone, id, host, value string, ttl int) error {
	_, err := c.call(ctx, "mod-record.json", url.Values{
		"domain-name": {zone}, "record-id": {id}, "host": {host},
		"record": {value}, "ttl": {strconv.Itoa(cloudnsTTL(ttl))},
	})
	return err
}

func (c *cloudns) Upsert(ctx context.Context, r Record) (string, error) {
	host := cloudnsHost(r.RR())
	recs, err := c.list(ctx, r.Zone, host, r.Type)
	if err != nil {
		return "", err
	}
	if len(recs) == 0 {
		if err := c.add(ctx, r.Zone, host, r.Type, r.Value, r.TTL); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	cur := recs[0]
	ttlSame := r.TTL <= 0 || cur.TTL.String() == strconv.Itoa(cloudnsTTL(r.TTL))
	if sameValue(r.Type, cur.Value, r.Value) && ttlSame {
		return ActionUnchanged, nil
	}
	if err := c.modify(ctx, r.Zone, cur.ID, host, r.Value, r.TTL); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (c *cloudns) AddTXT(ctx context.Context, zone, sub, value string) error {
	return c.add(ctx, zone, cloudnsHost(Record{Zone: zone, Sub: sub}.RR()), "TXT", value, 60)
}

func (c *cloudns) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	host := cloudnsHost(Record{Zone: zone, Sub: sub}.RR())
	recs, err := c.list(ctx, zone, host, "TXT")
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Value) == value {
			return c.remove(ctx, zone, rec.ID)
		}
	}
	return nil
}

func (c *cloudns) remove(ctx context.Context, zone, id string) error {
	_, err := c.call(ctx, "delete-record.json", url.Values{"domain-name": {zone}, "record-id": {id}})
	return err
}

func (c *cloudns) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := c.list(ctx, r.Zone, cloudnsHost(r.RR()), r.Type)
	if err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, remoteRecord{id: rec.ID, values: []string{rec.Value}})
	}
	return out, nil
}

func (c *cloudns) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	return c.remove(ctx, r.Zone, rec.id)
}

var (
	_ TXTProvider   = (*cloudns)(nil)
	_ recordDeleter = (*cloudns)(nil)
)
