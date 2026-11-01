package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DNSPod 传统 Token 接口（dnsapi.cn / api.dnspod.com），ID 与 Token 拼接为 login_token。
// 文档：https://docs.dnspod.cn/api/
func init() {
	register(Meta{
		Name:           "dnspod",
		Title:          "DNSPod (国内站 Token)",
		IDLabel:        "ID",
		SecretLabel:    "Token",
		SupportsLine:   true,
		SupportsRemark: true,
		DocURL:         "https://console.dnspod.cn/account/token/token",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &dnspod{token: c.ID + "," + c.Secret, hc: hc, endpoint: "https://dnsapi.cn", defaultLine: "默认"}, nil
	})
	register(Meta{
		Name:           "dnspodcom",
		Title:          "DNSPod (国际站 Token)",
		IDLabel:        "ID",
		SecretLabel:    "Token",
		DocURL:         "https://www.dnspod.com/",
		SupportsRemark: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		return &dnspod{token: c.ID + "," + c.Secret, hc: hc, endpoint: "https://api.dnspod.com", defaultLine: "default"}, nil
	})
}

type dnspod struct {
	token       string
	hc          *http.Client
	endpoint    string
	defaultLine string
}

type dnspodStatus struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (d *dnspod) call(ctx context.Context, action string, form url.Values, out any) (dnspodStatus, error) {
	form.Set("login_token", d.token)
	form.Set("format", "json")
	form.Set("lang", "cn")
	req, err := ctxReq(ctx, http.MethodPost, d.endpoint+"/"+action, strings.NewReader(form.Encode()))
	if err != nil {
		return dnspodStatus{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// 官方要求设置可识别的 UA
	req.Header.Set("User-Agent", "Gately DDNS Client/1.0 (https://github.com/wangwanjie/gately)")
	var env struct {
		Status dnspodStatus `json:"status"`
	}
	if out == nil {
		out = &env
	}
	if err := doJSON(d.hc, req, out); err != nil {
		return dnspodStatus{}, err
	}
	if o, ok := out.(interface{ status() dnspodStatus }); ok {
		return o.status(), nil
	}
	return env.Status, nil
}

type dnspodList struct {
	Status  dnspodStatus `json:"status"`
	Records []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Value  string `json:"value"`
		Line   string `json:"line"`
		TTL    string `json:"ttl"`
		Remark string `json:"remark"`
	} `json:"records"`
}

func (l *dnspodList) status() dnspodStatus { return l.Status }

// dnspodCreated 是 Record.Create 的响应，新记录 ID 用于设置备注
type dnspodCreated struct {
	Status dnspodStatus `json:"status"`
	Record struct {
		ID json.Number `json:"id"`
	} `json:"record"`
}

func (c *dnspodCreated) status() dnspodStatus { return c.Status }

func (d *dnspod) Upsert(ctx context.Context, r Record) (string, error) {
	var list dnspodList
	st, err := d.call(ctx, "Record.List", url.Values{
		"domain": {r.Zone}, "sub_domain": {r.RR()}, "record_type": {r.Type},
	}, &list)
	if err != nil {
		return "", err
	}
	// 10 表示没有记录
	if st.Code != "1" && st.Code != "10" {
		return "", fmt.Errorf("DNSPod Record.List: %s (%s)", st.Message, st.Code)
	}
	line := r.Line
	if line == "" {
		line = d.defaultLine
	}
	form := url.Values{
		"domain": {r.Zone}, "sub_domain": {r.RR()}, "record_type": {r.Type}, "record_line": {line}, "value": {r.Value},
	}
	if r.TTL > 0 {
		form.Set("ttl", strconv.Itoa(r.TTL))
	}
	for _, rec := range list.Records {
		if rec.Type != r.Type || rec.Line != line {
			continue
		}
		valueSame := rec.Value == r.Value && (r.TTL == 0 || rec.TTL == strconv.Itoa(r.TTL))
		remarkSame := r.Remark == "" || rec.Remark == r.Remark
		if valueSame && remarkSame {
			return ActionUnchanged, nil
		}
		if !valueSame {
			form.Set("record_id", rec.ID)
			st, err := d.call(ctx, "Record.Modify", form, nil)
			if err != nil {
				return "", err
			}
			if st.Code != "1" {
				return "", fmt.Errorf("DNSPod Record.Modify: %s (%s)", st.Message, st.Code)
			}
		}
		if !remarkSame {
			if err := d.setRemark(ctx, r.Zone, rec.ID, r.Remark); err != nil {
				return "", err
			}
		}
		return ActionUpdated, nil
	}
	var created dnspodCreated
	st, err = d.call(ctx, "Record.Create", form, &created)
	if err != nil {
		return "", err
	}
	if st.Code != "1" {
		return "", fmt.Errorf("DNSPod Record.Create: %s (%s)", st.Message, st.Code)
	}
	// 新增接口不接受备注，创建后再单独设置
	if r.Remark != "" && created.Record.ID != "" {
		if err := d.setRemark(ctx, r.Zone, created.Record.ID.String(), r.Remark); err != nil {
			return "", fmt.Errorf("记录已创建，设置备注失败: %w", err)
		}
	}
	return ActionCreated, nil
}

// setRemark 修改解析记录的备注。
func (d *dnspod) setRemark(ctx context.Context, zone, id, remark string) error {
	st, err := d.call(ctx, "Record.Remark", url.Values{"domain": {zone}, "record_id": {id}, "remark": {remark}}, nil)
	if err != nil {
		return err
	}
	if st.Code != "1" {
		return fmt.Errorf("DNSPod Record.Remark: %s (%s)", st.Message, st.Code)
	}
	return nil
}
