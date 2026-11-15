package dnsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// HiPM DNSMgr（HiDNS）：自建的多服务商 DNS 管理面板，Gately 通过它的 REST 接口写入解析。
// 凭据：ID 为面板地址（例如 https://dnsmgr.example.com），Secret 为面板里创建的 API 令牌，
// 以 Authorization: Bearer 发送。响应统一为 {code, msg, data}，code 为 0 表示成功。
// 记录的 name 是相对主机记录（根域名为 @），线路沿用面板中记录已有的线路。

func init() {
	register(Meta{
		Name:           "hipmdnsmgr",
		Title:          "HiPM DNSMgr",
		IDLabel:        "面板地址 (Base URL)",
		SecretLabel:    "API 令牌",
		DocURL:         "https://github.com/HiPM-Tech/HiDNS",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		base, err := hipmBaseURL(c.ID)
		if err != nil {
			return nil, err
		}
		return &hipmDnsMgr{token: strings.TrimSpace(c.Secret), hc: hc, endpoint: base}, nil
	})
}

type hipmDnsMgr struct {
	token string
	hc    *http.Client
	// endpoint 为面板地址（不含 /api 后缀）
	endpoint string
}

// hipmBaseURL 校验并规范化面板地址，去掉结尾的斜杠与 /api。
func hipmBaseURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("面板地址无效，需要 http(s):// 开头的完整地址")
	}
	s = strings.TrimSuffix(strings.TrimRight(s, "/"), "/api")
	return strings.TrimRight(s, "/"), nil
}

// hipmID 兼容数字与字符串两种写法的 ID。
type hipmID string

func (h *hipmID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*h = ""
		return nil
	}
	*h = hipmID(strings.Trim(s, `"`))
	return nil
}

type hipmRecord struct {
	ID    hipmID `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

// call 发送请求并校验业务状态码，data 原文写入 out（可为 nil）。
func (h *hipmDnsMgr) call(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	u := h.endpoint + "/api" + path
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
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := doJSON(h.hc, req, &env); err != nil {
		return fmt.Errorf("HiPM DNSMgr: %w", err)
	}
	if env.Code != 0 {
		return fmt.Errorf("HiPM DNSMgr: %s (code %d)", env.Msg, env.Code)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("HiPM DNSMgr: 解析响应失败: %w", err)
	}
	return nil
}

// hipmList 解析列表类 data：API 令牌访问域名列表时返回裸数组，其余情况返回 {list, total}。
func hipmList(data json.RawMessage, out any) (int, error) {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		return 0, json.Unmarshal(data, out)
	}
	var wrap struct {
		List  json.RawMessage `json:"list"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return 0, err
	}
	if len(wrap.List) == 0 || string(wrap.List) == "null" {
		return wrap.Total, nil
	}
	return wrap.Total, json.Unmarshal(wrap.List, out)
}

// domainID 按名称查询面板中的域名 ID。
func (h *hipmDnsMgr) domainID(ctx context.Context, zone string) (string, error) {
	const pageSize = 100
	for page := 1; page <= 10; page++ {
		var raw json.RawMessage
		q := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(pageSize)}, "keyword": {zone}}
		if err := h.call(ctx, http.MethodGet, "/domains", q, nil, &raw); err != nil {
			return "", err
		}
		var domains []struct {
			ID   hipmID `json:"id"`
			Name string `json:"name"`
		}
		if len(raw) > 0 {
			if _, err := hipmList(raw, &domains); err != nil {
				return "", fmt.Errorf("HiPM DNSMgr: 解析域名列表失败: %w", err)
			}
		}
		for _, d := range domains {
			if strings.EqualFold(d.Name, zone) && d.ID != "" {
				return string(d.ID), nil
			}
		}
		if len(domains) < pageSize {
			break
		}
	}
	return "", fmt.Errorf("HiPM DNSMgr 中未找到域名 %s", zone)
}

// list 列出域名下指定主机记录与类型的记录，自动翻页（最多 1000 条）。
func (h *hipmDnsMgr) list(ctx context.Context, domainID, host, typ string) ([]hipmRecord, error) {
	const pageSize = 100
	var all []hipmRecord
	for page := 1; page <= 10; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(pageSize)}, "type": {typ}}
		// 根域名的 @ 是否被服务端过滤识别没有说明，不传主机过滤，下面按名称精确筛选
		if host != "@" {
			q.Set("subdomain", host)
		}
		var raw json.RawMessage
		if err := h.call(ctx, http.MethodGet, "/domains/"+url.PathEscape(domainID)+"/records", q, nil, &raw); err != nil {
			return nil, err
		}
		var recs []hipmRecord
		total := 0
		if len(raw) > 0 {
			var err error
			total, err = hipmList(raw, &recs)
			if err != nil {
				return nil, fmt.Errorf("HiPM DNSMgr: 解析记录列表失败: %w", err)
			}
		}
		all = append(all, recs...)
		if len(recs) < pageSize || (total > 0 && len(all) >= total) {
			break
		}
	}
	var out []hipmRecord
	for _, rec := range all {
		if dnslaSameHost(rec.Name, host) && strings.EqualFold(rec.Type, typ) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (h *hipmDnsMgr) recordsPath(domainID string) string {
	return "/domains/" + url.PathEscape(domainID) + "/records"
}

func (h *hipmDnsMgr) Upsert(ctx context.Context, r Record) (string, error) {
	did, err := h.domainID(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	host := r.RR()
	recs, err := h.list(ctx, did, host, r.Type)
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
		if cur.Value == r.Value && (r.TTL <= 0 || cur.TTL == 0 || cur.TTL == ttl) {
			return ActionUnchanged, nil
		}
		// 更新不传线路，面板会保留记录原有线路
		err := h.call(ctx, http.MethodPut, h.recordsPath(did)+"/"+url.PathEscape(string(cur.ID)), nil, map[string]any{
			"name": host, "type": r.Type, "value": r.Value, "ttl": ttl,
		}, nil)
		if err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if ttl <= 0 {
		ttl = 600
	}
	err = h.call(ctx, http.MethodPost, h.recordsPath(did), nil, map[string]any{
		"name": host, "type": r.Type, "value": r.Value, "ttl": ttl,
	}, nil)
	if err != nil {
		return "", err
	}
	return ActionCreated, nil
}

func (h *hipmDnsMgr) AddTXT(ctx context.Context, zone, sub, value string) error {
	did, err := h.domainID(ctx, zone)
	if err != nil {
		return err
	}
	return h.call(ctx, http.MethodPost, h.recordsPath(did), nil, map[string]any{
		"name": Record{Zone: zone, Sub: sub}.RR(), "type": "TXT", "value": value, "ttl": txtTTL,
	}, nil)
}

func (h *hipmDnsMgr) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	did, err := h.domainID(ctx, zone)
	if err != nil {
		return err
	}
	recs, err := h.list(ctx, did, Record{Zone: zone, Sub: sub}.RR(), "TXT")
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Value) == value {
			return h.remove(ctx, did, string(rec.ID))
		}
	}
	return nil
}

func (h *hipmDnsMgr) remove(ctx context.Context, domainID, recordID string) error {
	return h.call(ctx, http.MethodDelete, h.recordsPath(domainID)+"/"+url.PathEscape(recordID), nil, nil, nil)
}

func (h *hipmDnsMgr) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	did, err := h.domainID(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	recs, err := h.list(ctx, did, r.RR(), r.Type)
	if err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, remoteRecord{id: string(rec.ID), scope: did, values: []string{rec.Value}, ttl: rec.TTL})
	}
	return out, nil
}

func (h *hipmDnsMgr) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return h.remove(ctx, rec.scope, rec.id)
}

// 编译期确认实现了接口
var (
	_ TXTProvider   = (*hipmDnsMgr)(nil)
	_ recordDeleter = (*hipmDnsMgr)(nil)
)
