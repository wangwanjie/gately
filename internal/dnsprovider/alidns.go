package dnsprovider

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 阿里云云解析 DNS，RPC 风格接口，签名算法 HMAC-SHA1（Signature Version 1.0）。
// 文档：https://help.aliyun.com/document_detail/29745.html
func init() {
	register(Meta{
		Name:           "alidns",
		Title:          "阿里云 DNS",
		IDLabel:        "AccessKey ID",
		SecretLabel:    "AccessKey Secret",
		SupportsLine:   true,
		SupportsRemark: true,
		DocURL:         "https://ram.console.aliyun.com/manage/ak",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &aliDNS{id: c.ID, secret: c.Secret, hc: hc, endpoint: "https://alidns.aliyuncs.com/", now: time.Now}, nil
	})
}

type aliDNS struct {
	id, secret string
	hc         *http.Client
	endpoint   string
	now        func() time.Time
}

type aliRecord struct {
	RecordID string `json:"RecordId"`
	RR       string `json:"RR"`
	Type     string `json:"Type"`
	Value    string `json:"Value"`
	TTL      int    `json:"TTL"`
	Line     string `json:"Line"`
	Remark   string `json:"Remark"`
}

type aliError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// aliPercentEncode 按阿里云规范编码：RFC3986，空格为 %20，保留 ~。
func aliPercentEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	return strings.ReplaceAll(e, "%7E", "~")
}

// signedQuery 生成带公共参数与签名的查询串。
func (a *aliDNS) signedQuery(action string, params map[string]string) string {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	all := map[string]string{
		"Format":           "JSON",
		"Version":          "2015-01-09",
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
	return aliSign(all, a.secret)
}

// aliSign 计算签名并返回完整查询串（独立出来便于用官方示例验证）。
func aliSign(params map[string]string, secret string) string {
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
	stringToSign := "GET&%2F&" + aliPercentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return canonical + "&Signature=" + aliPercentEncode(sig)
}

func (a *aliDNS) call(ctx context.Context, action string, params map[string]string, out any) error {
	req, err := ctxReq(ctx, http.MethodGet, a.endpoint+"?"+a.signedQuery(action, params), nil)
	if err != nil {
		return err
	}
	err = doJSON(a.hc, req, out)
	if he, ok := err.(*HTTPError); ok {
		var ae aliError
		if jerr := jsonUnmarshalString(he.Body, &ae); jerr == nil && ae.Code != "" {
			return fmt.Errorf("阿里云 %s: %s (%s)", action, ae.Message, ae.Code)
		}
	}
	return err
}

func (a *aliDNS) Upsert(ctx context.Context, r Record) (string, error) {
	var list struct {
		TotalCount    int `json:"TotalCount"`
		DomainRecords struct {
			Record []aliRecord `json:"Record"`
		} `json:"DomainRecords"`
	}
	err := a.call(ctx, "DescribeSubDomainRecords", map[string]string{
		"SubDomain":  r.FQDN(),
		"DomainName": r.Zone,
		"Type":       r.Type,
		"PageSize":   "100",
	}, &list)
	if err != nil {
		return "", err
	}
	r.Remark = aliRemark(r.Remark)
	params := map[string]string{"RR": r.RR(), "Type": r.Type, "Value": r.Value}
	if r.TTL > 0 {
		params["TTL"] = strconv.Itoa(r.TTL)
	}
	if r.Line != "" {
		params["Line"] = r.Line
	}
	for _, rec := range list.DomainRecords.Record {
		if rec.Type != r.Type || (r.Line != "" && rec.Line != r.Line) {
			continue
		}
		valueSame := rec.Value == r.Value && (r.TTL == 0 || rec.TTL == r.TTL)
		remarkSame := r.Remark == "" || rec.Remark == r.Remark
		if valueSame && remarkSame {
			return ActionUnchanged, nil
		}
		// 值与 TTL 都没变时 UpdateDomainRecord 会报 DomainRecordDuplicate，只改备注时跳过
		if !valueSame {
			params["RecordId"] = rec.RecordID
			if err := a.call(ctx, "UpdateDomainRecord", params, nil); err != nil {
				return "", err
			}
		}
		if !remarkSame {
			if err := a.setRemark(ctx, rec.RecordID, r.Remark); err != nil {
				return "", err
			}
		}
		return ActionUpdated, nil
	}
	params["DomainName"] = r.Zone
	var created struct {
		RecordID string `json:"RecordId"`
	}
	if err := a.call(ctx, "AddDomainRecord", params, &created); err != nil {
		return "", err
	}
	// 新增接口不接受备注，创建后再单独设置
	if r.Remark != "" && created.RecordID != "" {
		if err := a.setRemark(ctx, created.RecordID, r.Remark); err != nil {
			return "", fmt.Errorf("记录已创建，设置备注失败: %w", err)
		}
	}
	return ActionCreated, nil
}

// aliRemarkMax 为阿里云解析记录备注的最大字符数
const aliRemarkMax = 50

// aliRemark 把备注转换为阿里云接受的格式：只能含数字、中文、英文字母与 _ - , . ，。（空格也不行），
// 最多 50 个字符。其他字符换成下划线并合并连续的下划线，避免因备注格式导致解析更新失败。
func aliRemark(s string) string {
	var b strings.Builder
	n := 0
	lastUnderscore := false
	for _, c := range s {
		ok := c < utf8.RuneSelf && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_-,.", c)) ||
			unicode.Is(unicode.Han, c) || c == '，' || c == '。'
		if !ok {
			c = '_'
		}
		if c == '_' && lastUnderscore {
			continue
		}
		if n == aliRemarkMax {
			break
		}
		lastUnderscore = c == '_'
		b.WriteRune(c)
		n++
	}
	return strings.Trim(b.String(), "_")
}

// setRemark 修改解析记录的备注。
func (a *aliDNS) setRemark(ctx context.Context, id, remark string) error {
	return a.call(ctx, "UpdateDomainRecordRemark", map[string]string{"RecordId": id, "Remark": remark}, nil)
}
