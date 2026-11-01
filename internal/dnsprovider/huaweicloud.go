package dnsprovider

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 华为云 DNS，AK/SK 签名（SDK-HMAC-SHA256）。
// 文档：https://support.huaweicloud.com/api-dns/dns_api_64001.html
func init() {
	register(Meta{
		Name:           "huaweicloud",
		Title:          "华为云 DNS",
		IDLabel:        "Access Key ID",
		SecretLabel:    "Secret Access Key",
		DocURL:         "https://console.huaweicloud.com/iam/#/mine/accessKey",
		SupportsRemark: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &huaweiCloud{ak: c.ID, sk: c.Secret, hc: hc, endpoint: "https://dns.myhuaweicloud.com", now: time.Now}, nil
	})
}

type huaweiCloud struct {
	ak, sk   string
	hc       *http.Client
	endpoint string
	now      func() time.Time
}

// huaweiEscape 按 RFC3986 编码，与华为云 APIG SDK 保持一致。
func huaweiEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// huaweiSign 对请求签名并设置 X-Sdk-Date 与 Authorization 头。
func huaweiSign(req *http.Request, body []byte, ak, sk string, ts time.Time) {
	date := ts.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Sdk-Date", date)
	// 规范 URI 必须以 / 结尾
	path := req.URL.EscapedPath()
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	q := req.URL.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var qs []string
	for _, k := range keys {
		vals := append([]string{}, q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			qs = append(qs, huaweiEscape(k)+"="+huaweiEscape(v))
		}
	}
	canonicalHeaders := "host:" + req.URL.Host + "\nx-sdk-date:" + date + "\n"
	signedHeaders := "host;x-sdk-date"
	canonicalRequest := req.Method + "\n" + path + "\n" + strings.Join(qs, "&") + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + sha256Hex(body)
	stringToSign := "SDK-HMAC-SHA256\n" + date + "\n" + sha256Hex([]byte(canonicalRequest))
	sig := hex.EncodeToString(hmacSHA256([]byte(sk), stringToSign))
	req.Header.Set("Authorization", "SDK-HMAC-SHA256 Access="+ak+", SignedHeaders="+signedHeaders+", Signature="+sig)
}

func (h *huaweiCloud) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	u := h.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := ctxReq(ctx, method, u, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	huaweiSign(req, payload, h.ak, h.sk, h.now())
	err = doJSON(h.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(he.Body), &e) == nil && e.Code != "" {
			return fmt.Errorf("华为云: %s (%s)", e.Message, e.Code)
		}
	}
	return err
}

type hwRecordset struct {
	ID      string   `json:"id,omitempty"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl,omitempty"`
	Records []string `json:"records"`
	// Description 为记录集备注，空时不发送
	Description string `json:"description,omitempty"`
}

func (h *huaweiCloud) Upsert(ctx context.Context, r Record) (string, error) {
	var zones struct {
		Zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones", url.Values{"name": {r.Zone + "."}, "search_mode": {"equal"}}, nil, &zones); err != nil {
		return "", err
	}
	zid := ""
	for _, z := range zones.Zones {
		if strings.EqualFold(strings.TrimSuffix(z.Name, "."), r.Zone) {
			zid = z.ID
		}
	}
	if zid == "" {
		return "", fmt.Errorf("华为云账号下未找到域名 %s", r.Zone)
	}
	fqdn := r.FQDN() + "."
	var sets struct {
		Recordsets []hwRecordset `json:"recordsets"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones/"+zid+"/recordsets", url.Values{"name": {fqdn}, "type": {r.Type}, "search_mode": {"equal"}}, nil, &sets); err != nil {
		return "", err
	}
	body := hwRecordset{Name: fqdn, Type: r.Type, TTL: r.TTL, Records: []string{r.Value}, Description: r.Remark}
	for _, s := range sets.Recordsets {
		if !strings.EqualFold(s.Name, fqdn) || s.Type != r.Type {
			continue
		}
		valueSame := len(s.Records) == 1 && s.Records[0] == r.Value && (r.TTL == 0 || s.TTL == r.TTL)
		if valueSame && (r.Remark == "" || s.Description == r.Remark) {
			return ActionUnchanged, nil
		}
		if err := h.call(ctx, http.MethodPut, "/v2/zones/"+zid+"/recordsets/"+s.ID, nil, body, nil); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if err := h.call(ctx, http.MethodPost, "/v2/zones/"+zid+"/recordsets", nil, body, nil); err != nil {
		return "", err
	}
	return ActionCreated, nil
}
