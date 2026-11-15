package dnsprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// TXTProvider 支持 ACME DNS-01 验证所需的 TXT 记录增删。
// 同一主机名可能同时存在多个验证值（例如同时申请 example.com 与 *.example.com），
// 因此 AddTXT 总是新增，DeleteTXT 按值删除。
type TXTProvider interface {
	AddTXT(ctx context.Context, zone, sub, value string) error
	DeleteTXT(ctx context.Context, zone, sub, value string) error
}

// ACME 验证记录使用较短的 TTL，加快传播
const txtTTL = 600

// unquote 去掉部分服务商在 TXT 值两侧加的引号。
func unquote(s string) string {
	return strings.Trim(s, `"`)
}

// ---- 阿里云 ----

func (a *aliDNS) AddTXT(ctx context.Context, zone, sub, value string) error {
	return a.call(ctx, "AddDomainRecord", map[string]string{
		"DomainName": zone, "RR": sub, "Type": "TXT", "Value": value, "TTL": strconv.Itoa(txtTTL),
	}, nil)
}

func (a *aliDNS) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	var list struct {
		DomainRecords struct {
			Record []aliRecord `json:"Record"`
		} `json:"DomainRecords"`
	}
	r := Record{Zone: zone, Sub: sub}
	if err := a.call(ctx, "DescribeSubDomainRecords", map[string]string{
		"SubDomain": r.FQDN(), "DomainName": zone, "Type": "TXT", "PageSize": "100",
	}, &list); err != nil {
		return err
	}
	for _, rec := range list.DomainRecords.Record {
		if unquote(rec.Value) == value {
			return a.call(ctx, "DeleteDomainRecord", map[string]string{"RecordId": rec.RecordID}, nil)
		}
	}
	return nil
}

// ---- Cloudflare ----

func (c *cloudflare) AddTXT(ctx context.Context, zone, sub, value string) error {
	zid, err := c.zoneID(ctx, zone)
	if err != nil {
		return err
	}
	rec := cfRecord{Type: "TXT", Name: Record{Zone: zone, Sub: sub}.FQDN(), Content: value, TTL: 120}
	return c.call(ctx, http.MethodPost, "/zones/"+zid+"/dns_records", rec, nil)
}

func (c *cloudflare) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	zid, err := c.zoneID(ctx, zone)
	if err != nil {
		return err
	}
	var recs []cfRecord
	q := "?type=TXT&name=" + url.QueryEscape(Record{Zone: zone, Sub: sub}.FQDN())
	if err := c.call(ctx, http.MethodGet, "/zones/"+zid+"/dns_records"+q, nil, &recs); err != nil {
		return err
	}
	for _, r := range recs {
		if unquote(r.Content) == value {
			return c.call(ctx, http.MethodDelete, "/zones/"+zid+"/dns_records/"+r.ID, nil, nil)
		}
	}
	return nil
}

// ---- 腾讯云 ----

func (t *tencentCloud) AddTXT(ctx context.Context, zone, sub, value string) error {
	apiErr, err := t.call(ctx, "CreateRecord", map[string]any{
		"Domain": zone, "SubDomain": sub, "RecordType": "TXT", "RecordLine": "默认", "Value": value, "TTL": txtTTL,
	}, nil)
	if err != nil {
		return err
	}
	if apiErr != nil {
		return fmt.Errorf("腾讯云 CreateRecord: %s (%s)", apiErr.Message, apiErr.Code)
	}
	return nil
}

func (t *tencentCloud) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	var list struct {
		RecordList []tcRecord `json:"RecordList"`
	}
	apiErr, err := t.call(ctx, "DescribeRecordList", map[string]any{"Domain": zone, "Subdomain": sub, "RecordType": "TXT"}, &list)
	if err != nil {
		return err
	}
	if apiErr != nil {
		if apiErr.Code == "ResourceNotFound.NoDataOfRecord" {
			return nil
		}
		return fmt.Errorf("腾讯云 DescribeRecordList: %s (%s)", apiErr.Message, apiErr.Code)
	}
	for _, r := range list.RecordList {
		if unquote(r.Value) == value {
			apiErr, err := t.call(ctx, "DeleteRecord", map[string]any{"Domain": zone, "RecordId": r.RecordID}, nil)
			if err != nil {
				return err
			}
			if apiErr != nil {
				return fmt.Errorf("腾讯云 DeleteRecord: %s (%s)", apiErr.Message, apiErr.Code)
			}
			return nil
		}
	}
	return nil
}

// ---- DNSPod Token ----

func (d *dnspod) AddTXT(ctx context.Context, zone, sub, value string) error {
	st, err := d.call(ctx, "Record.Create", url.Values{
		"domain": {zone}, "sub_domain": {sub}, "record_type": {"TXT"}, "record_line": {d.defaultLine}, "value": {value}, "ttl": {strconv.Itoa(txtTTL)},
	}, nil)
	if err != nil {
		return err
	}
	if st.Code != "1" {
		return fmt.Errorf("DNSPod Record.Create: %s (%s)", st.Message, st.Code)
	}
	return nil
}

func (d *dnspod) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	var list dnspodList
	st, err := d.call(ctx, "Record.List", url.Values{"domain": {zone}, "sub_domain": {sub}, "record_type": {"TXT"}}, &list)
	if err != nil {
		return err
	}
	if st.Code == "10" {
		return nil
	}
	if st.Code != "1" {
		return fmt.Errorf("DNSPod Record.List: %s (%s)", st.Message, st.Code)
	}
	for _, r := range list.Records {
		if unquote(r.Value) == value {
			st, err := d.call(ctx, "Record.Remove", url.Values{"domain": {zone}, "record_id": {r.ID}}, nil)
			if err != nil {
				return err
			}
			if st.Code != "1" {
				return fmt.Errorf("DNSPod Record.Remove: %s (%s)", st.Message, st.Code)
			}
			return nil
		}
	}
	return nil
}

// ---- 华为云：同名同类型记录在一个记录集中，TXT 值需带引号 ----

func (h *huaweiCloud) txtRecordset(ctx context.Context, zone, sub string) (string, *hwRecordset, error) {
	var zones struct {
		Zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones", url.Values{"name": {zone + "."}, "search_mode": {"equal"}}, nil, &zones); err != nil {
		return "", nil, err
	}
	zid := ""
	for _, z := range zones.Zones {
		if strings.EqualFold(strings.TrimSuffix(z.Name, "."), zone) {
			zid = z.ID
		}
	}
	if zid == "" {
		return "", nil, fmt.Errorf("华为云账号下未找到域名 %s", zone)
	}
	fqdn := Record{Zone: zone, Sub: sub}.FQDN() + "."
	var sets struct {
		Recordsets []hwRecordset `json:"recordsets"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones/"+zid+"/recordsets", url.Values{"name": {fqdn}, "type": {"TXT"}, "search_mode": {"equal"}}, nil, &sets); err != nil {
		return "", nil, err
	}
	for i := range sets.Recordsets {
		if strings.EqualFold(sets.Recordsets[i].Name, fqdn) {
			return zid, &sets.Recordsets[i], nil
		}
	}
	return zid, nil, nil
}

func (h *huaweiCloud) AddTXT(ctx context.Context, zone, sub, value string) error {
	zid, rs, err := h.txtRecordset(ctx, zone, sub)
	if err != nil {
		return err
	}
	quoted := `"` + value + `"`
	if rs == nil {
		body := hwRecordset{Name: Record{Zone: zone, Sub: sub}.FQDN() + ".", Type: "TXT", TTL: txtTTL, Records: []string{quoted}}
		return h.call(ctx, http.MethodPost, "/v2/zones/"+zid+"/recordsets", nil, body, nil)
	}
	body := hwRecordset{Name: rs.Name, Type: "TXT", TTL: txtTTL, Records: append(rs.Records, quoted)}
	return h.call(ctx, http.MethodPut, "/v2/zones/"+zid+"/recordsets/"+rs.ID, nil, body, nil)
}

func (h *huaweiCloud) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	zid, rs, err := h.txtRecordset(ctx, zone, sub)
	if err != nil || rs == nil {
		return err
	}
	var remain []string
	for _, v := range rs.Records {
		if unquote(v) != value {
			remain = append(remain, v)
		}
	}
	if len(remain) == 0 {
		return h.call(ctx, http.MethodDelete, "/v2/zones/"+zid+"/recordsets/"+rs.ID, nil, nil, nil)
	}
	body := hwRecordset{Name: rs.Name, Type: "TXT", TTL: rs.TTL, Records: remain}
	return h.call(ctx, http.MethodPut, "/v2/zones/"+zid+"/recordsets/"+rs.ID, nil, body, nil)
}

// ---- GoDaddy：按名称整体替换 ----

type gdRecord struct {
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
}

func (g *godaddy) txtPath(zone, sub string) string {
	return fmt.Sprintf("%s/v1/domains/%s/records/TXT/%s", g.endpoint, zone, url.PathEscape(sub))
}

func (g *godaddy) do(ctx context.Context, method, path string, body any, out any) error {
	var req *http.Request
	var err error
	if body != nil {
		req, err = ctxReq(ctx, method, path, jsonBody(body))
	} else {
		req, err = ctxReq(ctx, method, path, nil)
	}
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "sso-key "+g.key+":"+g.secret)
	req.Header.Set("Content-Type", "application/json")
	if err := doJSON(g.hc, req, out); err != nil {
		return fmt.Errorf("GoDaddy: %w", err)
	}
	return nil
}

func (g *godaddy) AddTXT(ctx context.Context, zone, sub, value string) error {
	var cur []gdRecord
	if err := g.do(ctx, http.MethodGet, g.txtPath(zone, sub), nil, &cur); err != nil {
		return err
	}
	return g.do(ctx, http.MethodPut, g.txtPath(zone, sub), append(cur, gdRecord{Data: value, TTL: txtTTL}), nil)
}

func (g *godaddy) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	var cur []gdRecord
	if err := g.do(ctx, http.MethodGet, g.txtPath(zone, sub), nil, &cur); err != nil {
		return err
	}
	var remain []gdRecord
	for _, r := range cur {
		if unquote(r.Data) != value {
			remain = append(remain, r)
		}
	}
	if len(remain) == len(cur) {
		return nil
	}
	if len(remain) == 0 {
		return g.do(ctx, http.MethodDelete, g.txtPath(zone, sub), nil, nil)
	}
	return g.do(ctx, http.MethodPut, g.txtPath(zone, sub), remain, nil)
}

// ---- Porkbun ----

func (p *porkbun) AddTXT(ctx context.Context, zone, sub, value string) error {
	_, err := p.post(ctx, "/dns/create/"+zone, map[string]any{"name": sub, "type": "TXT", "content": value, "ttl": strconv.Itoa(txtTTL)})
	return err
}

func (p *porkbun) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	cur, err := p.post(ctx, "/dns/retrieveByNameType/"+zone+"/TXT/"+url.PathEscape(sub), nil)
	if err != nil {
		return err
	}
	for _, r := range cur.Records {
		if unquote(r.Content) == value {
			_, err := p.post(ctx, "/dns/delete/"+zone+"/"+r.ID, nil)
			return err
		}
	}
	return nil
}

// ---- dynv6 ----

type dynv6Record struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data"`
}

// dynv6Zone 为 zone 本身的信息；根域名的地址直接保存在 zone 上，不走 records 接口。
type dynv6Zone struct {
	ID   int64  `json:"id"`
	IPv4 string `json:"ipv4address"`
	IPv6 string `json:"ipv6prefix"`
}

func (d *dynv6) zoneInfo(ctx context.Context, zone string) (dynv6Zone, error) {
	var z dynv6Zone
	if err := d.do(ctx, http.MethodGet, "/zones/by-name/"+zone, nil, &z); err != nil {
		return dynv6Zone{}, err
	}
	return z, nil
}

func (d *dynv6) zonePath(ctx context.Context, zone string) (string, error) {
	z, err := d.zoneInfo(ctx, zone)
	if err != nil {
		return "", err
	}
	return "/zones/" + strconv.FormatInt(z.ID, 10) + "/records", nil
}

func (d *dynv6) AddTXT(ctx context.Context, zone, sub, value string) error {
	zp, err := d.zonePath(ctx, zone)
	if err != nil {
		return err
	}
	return d.do(ctx, http.MethodPost, zp, map[string]string{"name": sub, "type": "TXT", "data": value}, nil)
}

func (d *dynv6) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	zp, err := d.zonePath(ctx, zone)
	if err != nil {
		return err
	}
	var recs []dynv6Record
	if err := d.do(ctx, http.MethodGet, zp, nil, &recs); err != nil {
		return err
	}
	for _, r := range recs {
		if r.Type == "TXT" && r.Name == sub && unquote(r.Data) == value {
			return d.do(ctx, http.MethodDelete, zp+"/"+strconv.FormatInt(r.ID, 10), nil, nil)
		}
	}
	return nil
}

// AsTXT 返回服务商的 TXT 能力；不支持时返回错误。
func AsTXT(p Provider) (TXTProvider, error) {
	t, ok := p.(TXTProvider)
	if !ok {
		return nil, errors.New("该 DNS 服务商不支持 ACME DNS 验证")
	}
	return t, nil
}

// SupportsTXT 判断服务商是否支持 TXT 增删（用于界面过滤）。
func SupportsTXT(name string) bool {
	switch name {
	case "alidns", "cloudflare", "tencentcloud", "dnspod", "dnspodcom", "huaweicloud", "godaddy", "porkbun", "dynv6":
		return true
	}
	// 通过 Register 注册的扩展服务商在描述中声明能力
	m, ok := registry[name]
	return ok && m.meta.SupportsACME
}

// 编译期确认实现了接口
var (
	_ TXTProvider = (*aliDNS)(nil)
	_ TXTProvider = (*cloudflare)(nil)
	_ TXTProvider = (*tencentCloud)(nil)
	_ TXTProvider = (*dnspod)(nil)
	_ TXTProvider = (*huaweiCloud)(nil)
	_ TXTProvider = (*godaddy)(nil)
	_ TXTProvider = (*porkbun)(nil)
	_ TXTProvider = (*dynv6)(nil)
)
