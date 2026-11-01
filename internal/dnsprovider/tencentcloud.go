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

// 腾讯云 DNSPod API 3.0，签名算法 TC3-HMAC-SHA256。
// 文档：https://cloud.tencent.com/document/api/1427/56193
func init() {
	register(Meta{
		Name:           "tencentcloud",
		Title:          "腾讯云 DNSPod (API 3.0)",
		IDLabel:        "SecretId",
		SecretLabel:    "SecretKey",
		SupportsLine:   true,
		SupportsRemark: true,
		DocURL:         "https://console.cloud.tencent.com/cam/capi",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &tencentCloud{id: c.ID, secret: c.Secret, hc: hc, endpoint: "https://dnspod.tencentcloudapi.com", now: time.Now}, nil
	})
}

type tencentCloud struct {
	id, secret string
	hc         *http.Client
	endpoint   string
	now        func() time.Time
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// tc3Authorization 计算 TC3-HMAC-SHA256 签名头。
func tc3Authorization(secretID, secretKey, service, host, action string, payload []byte, ts time.Time) string {
	date := ts.UTC().Format("2006-01-02")
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + host + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + sha256Hex(payload)
	scope := date + "/" + service + "/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + strconv.FormatInt(ts.Unix(), 10) + "\n" + scope + "\n" + sha256Hex([]byte(canonicalRequest))
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	sig := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + sig
}

type tcError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func (t *tencentCloud) call(ctx context.Context, action string, params any, out any) (*tcError, error) {
	payload, _ := json.Marshal(params)
	u, err := url.Parse(t.endpoint)
	if err != nil {
		return nil, err
	}
	ts := t.now()
	req, err := ctxReq(ctx, http.MethodPost, t.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", "2021-03-23")
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts.Unix(), 10))
	req.Header.Set("Authorization", tc3Authorization(t.id, t.secret, "dnspod", u.Host, action, payload, ts))
	var env struct {
		Response json.RawMessage `json:"Response"`
	}
	if err := doJSON(t.hc, req, &env); err != nil {
		return nil, err
	}
	var e struct {
		Error *tcError `json:"Error"`
	}
	_ = json.Unmarshal(env.Response, &e)
	if e.Error != nil {
		return e.Error, nil
	}
	if out != nil {
		return nil, json.Unmarshal(env.Response, out)
	}
	return nil, nil
}

type tcRecord struct {
	RecordID uint64 `json:"RecordId"`
	Value    string `json:"Value"`
	Type     string `json:"Type"`
	Name     string `json:"Name"`
	Line     string `json:"Line"`
	TTL      int    `json:"TTL"`
	Remark   string `json:"Remark"`
}

func (t *tencentCloud) Upsert(ctx context.Context, r Record) (string, error) {
	var list struct {
		RecordList []tcRecord `json:"RecordList"`
	}
	apiErr, err := t.call(ctx, "DescribeRecordList", map[string]any{
		"Domain": r.Zone, "Subdomain": r.RR(), "RecordType": r.Type,
	}, &list)
	if err != nil {
		return "", err
	}
	// 没有记录时接口返回错误码 ResourceNotFound.NoDataOfRecord
	if apiErr != nil && apiErr.Code != "ResourceNotFound.NoDataOfRecord" {
		return "", fmt.Errorf("腾讯云 DescribeRecordList: %s (%s)", apiErr.Message, apiErr.Code)
	}
	line := r.Line
	if line == "" {
		line = "默认"
	}
	params := map[string]any{
		"Domain": r.Zone, "SubDomain": r.RR(), "RecordType": r.Type, "RecordLine": line, "Value": r.Value,
	}
	if r.TTL > 0 {
		params["TTL"] = r.TTL
	}
	if r.Remark != "" {
		params["Remark"] = r.Remark
	}
	for _, rec := range list.RecordList {
		if rec.Type != r.Type || rec.Line != line {
			continue
		}
		valueSame := rec.Value == r.Value && (r.TTL == 0 || rec.TTL == r.TTL)
		remarkSame := r.Remark == "" || rec.Remark == r.Remark
		if valueSame && remarkSame {
			return ActionUnchanged, nil
		}
		action, params := "ModifyRecord", params
		if valueSame {
			// 只有备注不同时调用专门的备注接口，不重复提交记录值
			action, params = "ModifyRecordRemark", map[string]any{"Domain": r.Zone, "Remark": r.Remark}
		}
		params["RecordId"] = rec.RecordID
		apiErr, err := t.call(ctx, action, params, nil)
		if err != nil {
			return "", err
		}
		if apiErr != nil {
			return "", fmt.Errorf("腾讯云 %s: %s (%s)", action, apiErr.Message, apiErr.Code)
		}
		return ActionUpdated, nil
	}
	apiErr, err = t.call(ctx, "CreateRecord", params, nil)
	if err != nil {
		return "", err
	}
	if apiErr != nil {
		return "", fmt.Errorf("腾讯云 CreateRecord: %s (%s)", apiErr.Message, apiErr.Code)
	}
	return ActionCreated, nil
}
