package dnsprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// remoteRecord 是服务商处与要删除的记录同名、同类型（同线路）的一条解析。
// 按记录集管理的服务商（华为云、Route 53、Gandi、NS1、GoDaddy）一个记录集算一条，可能有多个值。
type remoteRecord struct {
	// id 为服务商的记录 ID，记录集类服务商可为空
	id string
	// scope 为删除时需要的上下文，例如 Cloudflare / 华为云的 zone ID
	scope  string
	values []string
	ttl    int
}

// recordDeleter 由支持删除解析的服务商实现：列出同名同类型的记录，再按条删除。
type recordDeleter interface {
	findRecords(ctx context.Context, r Record) ([]remoteRecord, error)
	removeRecord(ctx context.Context, r Record, rec remoteRecord) error
}

// SupportsDelete 判断服务商是否支持删除解析记录（用于界面显示“同时从服务商删除”）。
func SupportsDelete(name string) bool {
	e, ok := registry[name]
	if !ok {
		return false
	}
	switch name {
	case "alidns", "cloudflare", "tencentcloud", "dnspod", "dnspodcom", "huaweicloud", "godaddy", "porkbun",
		"dynv6", "route53", "gandi", "ns1", "bunny", "ovh":
		return true
	}
	// 通过 Register 注册的扩展服务商在描述中声明能力
	return e.meta.SupportsDelete
}

// ErrDeleteUnsupported 表示服务商不支持删除解析记录。
var ErrDeleteUnsupported = errors.New("该 DNS 服务商不支持删除解析记录")

// Delete 删除服务商处由动态域名同步的这条记录，返回删除的条数，记录不存在时返回 0。
//
// r.Value 为最近一次同步写入的值，空表示未知。只删除值与 r.Value 相同的记录：同名同类型（同线路）
// 只有一条时也要核对值，避免删掉用户手动添加或在控制台改过的记录；值未知时无法确认记录由 Gately 写入，
// 一条都不删并返回错误。按记录集管理的服务商，记录集里还有其他值时同样不删，避免连带删掉别人的值。
func Delete(ctx context.Context, p Provider, r Record) (int, error) {
	d, ok := p.(recordDeleter)
	if !ok {
		return 0, ErrDeleteUnsupported
	}
	recs, err := d.findRecords(ctx, r)
	if err != nil {
		return 0, err
	}
	targets, err := pickTargets(recs, r)
	if err != nil {
		return 0, err
	}
	for i, rec := range targets {
		if err := d.removeRecord(ctx, r, rec); err != nil {
			return i, err
		}
	}
	return len(targets), nil
}

// pickTargets 按 Delete 的规则选出要删除的记录；没有同名记录时返回空切片，无法确认时返回错误。
func pickTargets(recs []remoteRecord, r Record) ([]remoteRecord, error) {
	if len(recs) == 0 {
		return []remoteRecord{}, nil
	}
	if r.Value == "" {
		return nil, fmt.Errorf("Gately 还没有同步过 %s %s，无法确认服务商处的记录由 Gately 写入，未删除；请先同步一次，或到服务商控制台删除", r.Type, r.FQDN())
	}
	var out []remoteRecord
	for _, rec := range recs {
		if !slices.ContainsFunc(rec.values, func(v string) bool { return sameValue(r.Type, v, r.Value) }) {
			continue
		}
		if len(rec.values) > 1 {
			return nil, fmt.Errorf("服务商处的 %s %s 记录集还有其他 %d 个值，删除会一并删掉它们，未删除", r.Type, r.FQDN(), len(rec.values)-1)
		}
		out = append(out, rec)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("服务商处的 %s %s 记录值与 Gately 最近同步的 %s 不一致，可能已被修改，未删除", r.Type, r.FQDN(), r.Value)
	}
	return out, nil
}

// sameValue 比较记录值：去掉 TXT 的引号与 CNAME 结尾的点，CNAME 不区分大小写。
func sameValue(typ, a, b string) bool {
	norm := func(s string) string {
		return strings.TrimSuffix(unquote(strings.TrimSpace(s)), ".")
	}
	if typ == "CNAME" {
		return strings.EqualFold(norm(a), norm(b))
	}
	return norm(a) == norm(b)
}

// ---- 阿里云 ----

func (a *aliDNS) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	var list struct {
		DomainRecords struct {
			Record []aliRecord `json:"Record"`
		} `json:"DomainRecords"`
	}
	if err := a.call(ctx, "DescribeSubDomainRecords", map[string]string{
		"SubDomain": r.FQDN(), "DomainName": r.Zone, "Type": r.Type, "PageSize": "100",
	}, &list); err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range list.DomainRecords.Record {
		if rec.Type != r.Type || !lineMatch(rec.Line, r.Line, "default") {
			continue
		}
		out = append(out, remoteRecord{id: rec.RecordID, values: []string{rec.Value}})
	}
	return out, nil
}

func (a *aliDNS) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return a.call(ctx, "DeleteDomainRecord", map[string]string{"RecordId": rec.id}, nil)
}

// lineMatch 判断服务商记录的线路是否与 Gately 记录一致；Gately 记录未填线路时对应默认线路。
func lineMatch(got, want, def string) bool {
	if want == "" {
		return got == "" || got == def
	}
	return got == want
}

// ---- Cloudflare ----

func (c *cloudflare) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	zid, err := c.zoneID(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	var recs []cfRecord
	q := "?type=" + url.QueryEscape(r.Type) + "&name=" + url.QueryEscape(r.FQDN())
	if err := c.call(ctx, http.MethodGet, "/zones/"+zid+"/dns_records"+q, nil, &recs); err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, remoteRecord{id: rec.ID, scope: zid, values: []string{rec.Content}})
	}
	return out, nil
}

func (c *cloudflare) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return c.call(ctx, http.MethodDelete, "/zones/"+rec.scope+"/dns_records/"+rec.id, nil, nil)
}

// ---- 腾讯云 ----

func (t *tencentCloud) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	var list struct {
		RecordList []tcRecord `json:"RecordList"`
	}
	apiErr, err := t.call(ctx, "DescribeRecordList", map[string]any{"Domain": r.Zone, "Subdomain": r.RR(), "RecordType": r.Type}, &list)
	if err != nil {
		return nil, err
	}
	if apiErr != nil {
		if apiErr.Code == "ResourceNotFound.NoDataOfRecord" {
			return nil, nil
		}
		return nil, fmt.Errorf("腾讯云 DescribeRecordList: %s (%s)", apiErr.Message, apiErr.Code)
	}
	line := r.Line
	if line == "" {
		line = "默认"
	}
	var out []remoteRecord
	for _, rec := range list.RecordList {
		// Subdomain 是模糊匹配，需再按名称与类型过滤
		if rec.Type != r.Type || rec.Line != line || !strings.EqualFold(rec.Name, r.RR()) {
			continue
		}
		out = append(out, remoteRecord{id: strconv.FormatUint(rec.RecordID, 10), values: []string{rec.Value}})
	}
	return out, nil
}

func (t *tencentCloud) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	id, _ := strconv.ParseUint(rec.id, 10, 64)
	apiErr, err := t.call(ctx, "DeleteRecord", map[string]any{"Domain": r.Zone, "RecordId": id}, nil)
	if err != nil {
		return err
	}
	if apiErr != nil {
		return fmt.Errorf("腾讯云 DeleteRecord: %s (%s)", apiErr.Message, apiErr.Code)
	}
	return nil
}

// ---- DNSPod Token ----

func (d *dnspod) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	var list dnspodList
	st, err := d.call(ctx, "Record.List", url.Values{"domain": {r.Zone}, "sub_domain": {r.RR()}, "record_type": {r.Type}}, &list)
	if err != nil {
		return nil, err
	}
	// 10 表示没有记录
	if st.Code == "10" {
		return nil, nil
	}
	if st.Code != "1" {
		return nil, fmt.Errorf("DNSPod Record.List: %s (%s)", st.Message, st.Code)
	}
	line := r.Line
	if line == "" {
		line = d.defaultLine
	}
	var out []remoteRecord
	for _, rec := range list.Records {
		if rec.Type != r.Type || rec.Line != line || !strings.EqualFold(rec.Name, r.RR()) {
			continue
		}
		out = append(out, remoteRecord{id: rec.ID, values: []string{rec.Value}})
	}
	return out, nil
}

func (d *dnspod) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	st, err := d.call(ctx, "Record.Remove", url.Values{"domain": {r.Zone}, "record_id": {rec.id}}, nil)
	if err != nil {
		return err
	}
	if st.Code != "1" {
		return fmt.Errorf("DNSPod Record.Remove: %s (%s)", st.Message, st.Code)
	}
	return nil
}

// ---- 华为云：一个记录集算一条 ----

func (h *huaweiCloud) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	var zones struct {
		Zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones", url.Values{"name": {r.Zone + "."}, "search_mode": {"equal"}}, nil, &zones); err != nil {
		return nil, err
	}
	zid := ""
	for _, z := range zones.Zones {
		if strings.EqualFold(strings.TrimSuffix(z.Name, "."), r.Zone) {
			zid = z.ID
		}
	}
	if zid == "" {
		return nil, fmt.Errorf("华为云账号下未找到域名 %s", r.Zone)
	}
	fqdn := r.FQDN() + "."
	var sets struct {
		Recordsets []hwRecordset `json:"recordsets"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones/"+zid+"/recordsets", url.Values{"name": {fqdn}, "type": {r.Type}, "search_mode": {"equal"}}, nil, &sets); err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, s := range sets.Recordsets {
		if strings.EqualFold(s.Name, fqdn) && s.Type == r.Type {
			out = append(out, remoteRecord{id: s.ID, scope: zid, values: s.Records})
		}
	}
	return out, nil
}

func (h *huaweiCloud) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return h.call(ctx, http.MethodDelete, "/v2/zones/"+rec.scope+"/recordsets/"+rec.id, nil, nil, nil)
}

// ---- GoDaddy：同名同类型的全部值一起删除 ----

func (g *godaddy) recordsPath(r Record) string {
	return fmt.Sprintf("%s/v1/domains/%s/records/%s/%s", g.endpoint, r.Zone, r.Type, url.PathEscape(r.RR()))
}

func (g *godaddy) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	var cur []gdRecord
	if err := g.do(ctx, http.MethodGet, g.recordsPath(r), nil, &cur); err != nil {
		return nil, err
	}
	if len(cur) == 0 {
		return nil, nil
	}
	rec := remoteRecord{}
	for _, c := range cur {
		rec.values = append(rec.values, c.Data)
	}
	return []remoteRecord{rec}, nil
}

func (g *godaddy) removeRecord(ctx context.Context, r Record, _ remoteRecord) error {
	return g.do(ctx, http.MethodDelete, g.recordsPath(r), nil, nil)
}

// ---- Porkbun ----

func (p *porkbun) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	suffix := "/" + r.Zone + "/" + r.Type
	if r.RR() != "@" {
		suffix += "/" + url.PathEscape(r.RR())
	}
	cur, err := p.post(ctx, "/dns/retrieveByNameType"+suffix, nil)
	if err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(cur.Records))
	for _, rec := range cur.Records {
		out = append(out, remoteRecord{id: rec.ID, values: []string{rec.Content}})
	}
	return out, nil
}

func (p *porkbun) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	_, err := p.post(ctx, "/dns/delete/"+r.Zone+"/"+rec.id, nil)
	return err
}

// ---- dynv6 ----

func (d *dynv6) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	zp, name, err := d.resolveZone(ctx, r.Zone, r.RR())
	if err != nil {
		return nil, err
	}
	var recs []dynv6Record
	if err := d.do(ctx, http.MethodGet, zp, nil, &recs); err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range recs {
		if rec.Type == r.Type && rec.Name == name {
			out = append(out, remoteRecord{id: strconv.FormatInt(rec.ID, 10), scope: zp, values: []string{rec.Data}})
		}
	}
	return out, nil
}

func (d *dynv6) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return d.do(ctx, http.MethodDelete, rec.scope+"/"+rec.id, nil, nil)
}

// ---- Route 53：DELETE 需要与现有记录集完全一致 ----

func (r *route53) findRecords(ctx context.Context, rec Record) ([]remoteRecord, error) {
	zid, err := r.hostedZone(ctx, rec.Zone)
	if err != nil {
		return nil, err
	}
	cur, err := r.getSet(ctx, zid, rec.FQDN(), rec.Type)
	if err != nil || cur == nil {
		return nil, err
	}
	return []remoteRecord{{scope: zid, values: cur.Values, ttl: cur.TTL}}, nil
}

func (r *route53) removeRecord(ctx context.Context, rec Record, rr remoteRecord) error {
	return r.change(ctx, rr.scope, "DELETE", rec.FQDN(), rec.Type, rr.ttl, rr.values)
}

// ---- Gandi ----

func (g *gandi) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	cur, err := g.rrset(ctx, http.MethodGet, r.Zone, r.RR(), r.Type, nil)
	if err != nil || cur == nil {
		return nil, err
	}
	return []remoteRecord{{values: cur.Values}}, nil
}

func (g *gandi) removeRecord(ctx context.Context, r Record, _ remoteRecord) error {
	_, err := g.rrset(ctx, http.MethodDelete, r.Zone, r.RR(), r.Type, nil)
	return err
}

// ---- NS1 ----

func (n *ns1) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	cur, err := n.call(ctx, http.MethodGet, r.Zone, r.FQDN(), r.Type, nil)
	if err != nil || cur == nil {
		return nil, err
	}
	rec := remoteRecord{}
	for _, a := range cur.Answers {
		rec.values = append(rec.values, a.Answer...)
	}
	return []remoteRecord{rec}, nil
}

func (n *ns1) removeRecord(ctx context.Context, r Record, _ remoteRecord) error {
	_, err := n.call(ctx, http.MethodDelete, r.Zone, r.FQDN(), r.Type, nil)
	return err
}

// ---- Bunny ----

func (b *bunny) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	typ, err := bunnyType(r.Type)
	if err != nil {
		return nil, err
	}
	zid, recs, err := b.zone(ctx, r.Zone)
	if err != nil {
		return nil, err
	}
	name := bunnyName(r.RR())
	var out []remoteRecord
	for _, rec := range recs {
		if rec.Type == typ && strings.EqualFold(rec.Name, name) {
			out = append(out, remoteRecord{id: strconv.FormatInt(rec.ID, 10), scope: strconv.FormatInt(zid, 10), values: []string{rec.Value}})
		}
	}
	return out, nil
}

func (b *bunny) removeRecord(ctx context.Context, _ Record, rec remoteRecord) error {
	return b.call(ctx, http.MethodDelete, "/dnszone/"+rec.scope+"/records/"+rec.id, nil, nil)
}

// ---- OVH ----

func (o *ovh) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := o.find(ctx, r.Zone, r.RR(), r.Type)
	if err != nil {
		return nil, err
	}
	out := make([]remoteRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, remoteRecord{id: strconv.FormatInt(rec.ID, 10), values: []string{rec.Target}})
	}
	return out, nil
}

func (o *ovh) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	if err := o.call(ctx, http.MethodDelete, "/domain/zone/"+r.Zone+"/record/"+rec.id, nil, nil, nil); err != nil {
		return err
	}
	return o.refresh(ctx, r.Zone)
}

// 编译期确认实现了接口
var (
	_ recordDeleter = (*aliDNS)(nil)
	_ recordDeleter = (*cloudflare)(nil)
	_ recordDeleter = (*tencentCloud)(nil)
	_ recordDeleter = (*dnspod)(nil)
	_ recordDeleter = (*huaweiCloud)(nil)
	_ recordDeleter = (*godaddy)(nil)
	_ recordDeleter = (*porkbun)(nil)
	_ recordDeleter = (*dynv6)(nil)
	_ recordDeleter = (*route53)(nil)
	_ recordDeleter = (*gandi)(nil)
	_ recordDeleter = (*ns1)(nil)
	_ recordDeleter = (*bunny)(nil)
	_ recordDeleter = (*ovh)(nil)
)
