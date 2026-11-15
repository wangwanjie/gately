package dnsprovider

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// 本文件包含面向海外用户常用的服务商：dyndns2 协议（No-IP、Dynu 等）、Namecheap、Gandi、NS1、Bunny。
// Route53 与 OVH 因需要请求签名，分别位于 route53.go 与 ovh.go。

func init() {
	register(Meta{
		Name:        "dyndns2",
		Title:       "DynDNS2 (No-IP / Dynu / DynDNS …)",
		IDLabel:     "用户名",
		SecretLabel: "密码",
		Extra: []Field{
			{Key: "server", Label: "更新地址", Placeholder: "默认 https://dynupdate.no-ip.com/nic/update；Dynu 为 https://api.dynu.com/nic/update"},
		},
		DocURL: "https://www.noip.com/integrate/request",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, true, true); err != nil {
			return nil, err
		}
		server := strings.TrimSpace(c.Extra["server"])
		if server == "" {
			server = "https://dynupdate.no-ip.com/nic/update"
		}
		if u, err := url.Parse(server); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("更新地址无效，需要 http(s):// 开头的完整地址")
		}
		return &dyndns2{user: c.ID, pass: c.Secret, hc: hc, endpoint: server}, nil
	})
	register(Meta{
		Name:        "namecheap",
		Title:       "Namecheap",
		SecretLabel: "Dynamic DNS Password",
		DocURL:      "https://www.namecheap.com/support/knowledgebase/article.aspx/29/11/how-to-dynamically-update-the-hosts-ip-with-an-http-request/",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &namecheap{password: c.Secret, hc: hc, endpoint: "https://dynamicdns.park-your-domain.com/update"}, nil
	})
	register(Meta{
		Name:         "gandi",
		Title:        "Gandi",
		SecretLabel:  "Personal Access Token",
		DocURL:       "https://admin.gandi.net/organizations/account/pat",
		SupportsACME: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &gandi{token: c.Secret, hc: hc, endpoint: "https://api.gandi.net/v5/livedns"}, nil
	})
	register(Meta{
		Name:         "ns1",
		Title:        "NS1",
		SecretLabel:  "API Key",
		DocURL:       "https://my.nsone.net/#/account/settings",
		SupportsACME: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &ns1{key: c.Secret, hc: hc, endpoint: "https://api.nsone.net/v1"}, nil
	})
	register(Meta{
		Name:         "bunny",
		Title:        "Bunny DNS",
		SecretLabel:  "Account API Key",
		DocURL:       "https://dash.bunny.net/account/settings",
		SupportsACME: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &bunny{key: c.Secret, hc: hc, endpoint: "https://api.bunny.net"}, nil
	})
}

// ---- DynDNS2 协议 ----

type dyndns2 struct {
	user, pass string
	hc         *http.Client
	endpoint   string
}

func (d *dyndns2) Upsert(ctx context.Context, r Record) (string, error) {
	q := url.Values{"hostname": {r.FQDN()}, "myip": {r.Value}}
	req, err := ctxReq(ctx, http.MethodGet, d.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(d.user, d.pass)
	req.Header.Set("User-Agent", "Gately")
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	body := strings.TrimSpace(string(raw))
	if resp.StatusCode == http.StatusUnauthorized {
		return "", errors.New("DynDNS2: 用户名或密码错误")
	}
	switch {
	case strings.HasPrefix(body, "good"):
		return ActionUpdated, nil
	case strings.HasPrefix(body, "nochg"):
		return ActionUnchanged, nil
	}
	return "", fmt.Errorf("DynDNS2: %s", truncate(body, 200))
}

// ---- Namecheap Dynamic DNS ----

type namecheap struct {
	password string
	hc       *http.Client
	endpoint string
}

func (n *namecheap) Upsert(ctx context.Context, r Record) (string, error) {
	if r.Type != "A" {
		return "", errors.New("Namecheap 动态 DNS 仅支持 A 记录（IPv4）")
	}
	q := url.Values{"host": {r.RR()}, "domain": {r.Zone}, "password": {n.password}, "ip": {r.Value}}
	req, err := ctxReq(ctx, http.MethodGet, n.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := n.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	var out struct {
		ErrCount int    `xml:"ErrCount"`
		Err1     string `xml:"errors>Err1"`
	}
	if err := xml.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("Namecheap: 无法解析响应: %s", truncate(string(raw), 200))
	}
	if out.ErrCount > 0 {
		return "", fmt.Errorf("Namecheap: %s", out.Err1)
	}
	// 接口不返回旧值，无法区分是否变化
	return ActionUpdated, nil
}

// ---- Gandi LiveDNS ----

type gandi struct {
	token    string
	hc       *http.Client
	endpoint string
}

type gandiRRSet struct {
	Values []string `json:"rrset_values"`
	TTL    int      `json:"rrset_ttl,omitempty"`
}

// call 发送请求；404 返回 (nil, nil) 表示记录不存在。
func (g *gandi) rrset(ctx context.Context, method, zone, rr, typ string, body *gandiRRSet) (*gandiRRSet, error) {
	var rd io.Reader
	if body != nil {
		rd = jsonBody(body)
	}
	u := fmt.Sprintf("%s/domains/%s/records/%s/%s", g.endpoint, zone, url.PathEscape(rr), typ)
	req, err := ctxReq(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	var out gandiRRSet
	err = doJSON(g.hc, req, &out)
	var he *HTTPError
	if errors.As(err, &he) {
		if he.Status == http.StatusNotFound && method == http.MethodGet {
			return nil, nil
		}
		return nil, fmt.Errorf("Gandi: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (g *gandi) Upsert(ctx context.Context, r Record) (string, error) {
	cur, err := g.rrset(ctx, http.MethodGet, r.Zone, r.RR(), r.Type, nil)
	if err != nil {
		return "", err
	}
	ttl := r.TTL
	if ttl <= 0 && cur != nil {
		// 未指定 TTL 时沿用记录现有值
		ttl = cur.TTL
	}
	// Gandi 最小 TTL 为 300
	if ttl < 300 {
		ttl = 300
	}
	if cur != nil && len(cur.Values) == 1 && cur.Values[0] == r.Value && (r.TTL == 0 || cur.TTL == ttl) {
		return ActionUnchanged, nil
	}
	if _, err := g.rrset(ctx, http.MethodPut, r.Zone, r.RR(), r.Type, &gandiRRSet{Values: []string{r.Value}, TTL: ttl}); err != nil {
		return "", err
	}
	if cur == nil {
		return ActionCreated, nil
	}
	return ActionUpdated, nil
}

func (g *gandi) AddTXT(ctx context.Context, zone, sub, value string) error {
	rr := Record{Zone: zone, Sub: sub}.RR()
	cur, err := g.rrset(ctx, http.MethodGet, zone, rr, "TXT", nil)
	if err != nil {
		return err
	}
	vals := []string{`"` + value + `"`}
	if cur != nil {
		vals = append(cur.Values, vals...)
	}
	_, err = g.rrset(ctx, http.MethodPut, zone, rr, "TXT", &gandiRRSet{Values: vals, TTL: 300})
	return err
}

func (g *gandi) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	rr := Record{Zone: zone, Sub: sub}.RR()
	cur, err := g.rrset(ctx, http.MethodGet, zone, rr, "TXT", nil)
	if err != nil || cur == nil {
		return err
	}
	var keep []string
	for _, v := range cur.Values {
		if unquote(v) != value {
			keep = append(keep, v)
		}
	}
	if len(keep) == len(cur.Values) {
		return nil
	}
	if len(keep) == 0 {
		_, err = g.rrset(ctx, http.MethodDelete, zone, rr, "TXT", nil)
		return err
	}
	_, err = g.rrset(ctx, http.MethodPut, zone, rr, "TXT", &gandiRRSet{Values: keep, TTL: cur.TTL})
	return err
}

// ---- NS1 ----

type ns1 struct {
	key      string
	hc       *http.Client
	endpoint string
}

type ns1Answer struct {
	Answer []string `json:"answer"`
}

type ns1Record struct {
	Zone    string      `json:"zone,omitempty"`
	Domain  string      `json:"domain,omitempty"`
	Type    string      `json:"type,omitempty"`
	TTL     int         `json:"ttl,omitempty"`
	Answers []ns1Answer `json:"answers"`
}

func (n *ns1) call(ctx context.Context, method, zone, fqdn, typ string, body *ns1Record) (*ns1Record, error) {
	var rd io.Reader
	if body != nil {
		rd = jsonBody(body)
	}
	u := fmt.Sprintf("%s/zones/%s/%s/%s", n.endpoint, zone, fqdn, typ)
	req, err := ctxReq(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-NSONE-Key", n.key)
	req.Header.Set("Content-Type", "application/json")
	var out ns1Record
	err = doJSON(n.hc, req, &out)
	var he *HTTPError
	if errors.As(err, &he) {
		if he.Status == http.StatusNotFound && method == http.MethodGet {
			return nil, nil
		}
		return nil, fmt.Errorf("NS1: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (n *ns1) Upsert(ctx context.Context, r Record) (string, error) {
	fqdn := r.FQDN()
	cur, err := n.call(ctx, http.MethodGet, r.Zone, fqdn, r.Type, nil)
	if err != nil {
		return "", err
	}
	ttl := r.TTL
	if ttl <= 0 && cur != nil && cur.TTL > 0 {
		// 未指定 TTL 时沿用记录现有值
		ttl = cur.TTL
	}
	if ttl <= 0 {
		ttl = 3600
	}
	want := &ns1Record{Zone: r.Zone, Domain: fqdn, Type: r.Type, TTL: ttl, Answers: []ns1Answer{{Answer: []string{r.Value}}}}
	if cur == nil {
		if _, err := n.call(ctx, http.MethodPut, r.Zone, fqdn, r.Type, want); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	if len(cur.Answers) == 1 && len(cur.Answers[0].Answer) == 1 && cur.Answers[0].Answer[0] == r.Value && (r.TTL <= 0 || cur.TTL == ttl) {
		return ActionUnchanged, nil
	}
	if _, err := n.call(ctx, http.MethodPost, r.Zone, fqdn, r.Type, &ns1Record{TTL: ttl, Answers: want.Answers}); err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (n *ns1) AddTXT(ctx context.Context, zone, sub, value string) error {
	fqdn := Record{Zone: zone, Sub: sub}.FQDN()
	cur, err := n.call(ctx, http.MethodGet, zone, fqdn, "TXT", nil)
	if err != nil {
		return err
	}
	add := ns1Answer{Answer: []string{value}}
	if cur == nil {
		_, err = n.call(ctx, http.MethodPut, zone, fqdn, "TXT", &ns1Record{Zone: zone, Domain: fqdn, Type: "TXT", TTL: 60, Answers: []ns1Answer{add}})
		return err
	}
	_, err = n.call(ctx, http.MethodPost, zone, fqdn, "TXT", &ns1Record{TTL: cur.TTL, Answers: append(cur.Answers, add)})
	return err
}

func (n *ns1) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	fqdn := Record{Zone: zone, Sub: sub}.FQDN()
	cur, err := n.call(ctx, http.MethodGet, zone, fqdn, "TXT", nil)
	if err != nil || cur == nil {
		return err
	}
	var keep []ns1Answer
	for _, a := range cur.Answers {
		if len(a.Answer) == 1 && unquote(a.Answer[0]) == value {
			continue
		}
		keep = append(keep, a)
	}
	if len(keep) == len(cur.Answers) {
		return nil
	}
	if len(keep) == 0 {
		_, err = n.call(ctx, http.MethodDelete, zone, fqdn, "TXT", nil)
		return err
	}
	_, err = n.call(ctx, http.MethodPost, zone, fqdn, "TXT", &ns1Record{TTL: cur.TTL, Answers: keep})
	return err
}

// ---- Bunny DNS ----

type bunny struct {
	key      string
	hc       *http.Client
	endpoint string
}

// Bunny 记录类型编号
const (
	bunnyA    = 0
	bunnyAAAA = 1
	bunnyTXT  = 3
)

type bunnyRecord struct {
	ID    int64  `json:"Id,omitempty"`
	Type  int    `json:"Type"`
	TTL   int    `json:"Ttl"`
	Value string `json:"Value"`
	Name  string `json:"Name"`
}

func (b *bunny) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		rd = jsonBody(body)
	}
	req, err := ctxReq(ctx, method, b.endpoint+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("AccessKey", b.key)
	req.Header.Set("Content-Type", "application/json")
	if err := doJSON(b.hc, req, out); err != nil {
		return fmt.Errorf("Bunny: %w", err)
	}
	return nil
}

func (b *bunny) zone(ctx context.Context, zone string) (int64, []bunnyRecord, error) {
	var list struct {
		Items []struct {
			ID     int64  `json:"Id"`
			Domain string `json:"Domain"`
		} `json:"Items"`
	}
	if err := b.call(ctx, http.MethodGet, "/dnszone?search="+url.QueryEscape(zone), nil, &list); err != nil {
		return 0, nil, err
	}
	for _, z := range list.Items {
		if strings.EqualFold(z.Domain, zone) {
			var detail struct {
				Records []bunnyRecord `json:"Records"`
			}
			if err := b.call(ctx, http.MethodGet, "/dnszone/"+strconv.FormatInt(z.ID, 10), nil, &detail); err != nil {
				return 0, nil, err
			}
			return z.ID, detail.Records, nil
		}
	}
	return 0, nil, fmt.Errorf("Bunny 账号下未找到域名 %s", zone)
}

func bunnyType(t string) (int, error) {
	switch t {
	case "A":
		return bunnyA, nil
	case "AAAA":
		return bunnyAAAA, nil
	case "TXT":
		return bunnyTXT, nil
	}
	return 0, fmt.Errorf("Bunny 不支持记录类型 %s", t)
}

// bunnyName 返回主机记录名，根域名为空字符串。
func bunnyName(sub string) string {
	if sub == "@" {
		return ""
	}
	return sub
}

func (b *bunny) Upsert(ctx context.Context, r Record) (string, error) {
	typ, err := bunnyType(r.Type)
	if err != nil {
		return "", err
	}
	zid, recs, err := b.zone(ctx, r.Zone)
	if err != nil {
		return "", err
	}
	name := bunnyName(r.RR())
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 300
	}
	base := "/dnszone/" + strconv.FormatInt(zid, 10) + "/records"
	for _, rec := range recs {
		if rec.Type != typ || !strings.EqualFold(rec.Name, name) {
			continue
		}
		if r.TTL <= 0 && rec.TTL > 0 {
			// 未指定 TTL 时沿用记录现有值
			ttl = rec.TTL
		}
		if rec.Value == r.Value && (r.TTL <= 0 || rec.TTL == ttl) {
			return ActionUnchanged, nil
		}
		upd := bunnyRecord{ID: rec.ID, Type: typ, TTL: ttl, Value: r.Value, Name: name}
		if err := b.call(ctx, http.MethodPost, base+"/"+strconv.FormatInt(rec.ID, 10), upd, nil); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if err := b.call(ctx, http.MethodPut, base, bunnyRecord{Type: typ, TTL: ttl, Value: r.Value, Name: name}, nil); err != nil {
		return "", err
	}
	return ActionCreated, nil
}

func (b *bunny) AddTXT(ctx context.Context, zone, sub, value string) error {
	zid, _, err := b.zone(ctx, zone)
	if err != nil {
		return err
	}
	return b.call(ctx, http.MethodPut, "/dnszone/"+strconv.FormatInt(zid, 10)+"/records",
		bunnyRecord{Type: bunnyTXT, TTL: 60, Value: value, Name: bunnyName(Record{Zone: zone, Sub: sub}.RR())}, nil)
}

func (b *bunny) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	zid, recs, err := b.zone(ctx, zone)
	if err != nil {
		return err
	}
	name := bunnyName(Record{Zone: zone, Sub: sub}.RR())
	for _, rec := range recs {
		if rec.Type == bunnyTXT && strings.EqualFold(rec.Name, name) && unquote(rec.Value) == value {
			return b.call(ctx, http.MethodDelete, "/dnszone/"+strconv.FormatInt(zid, 10)+"/records/"+strconv.FormatInt(rec.ID, 10), nil, nil)
		}
	}
	return nil
}

// 编译期确认实现了接口
var (
	_ TXTProvider = (*gandi)(nil)
	_ TXTProvider = (*ns1)(nil)
	_ TXTProvider = (*bunny)(nil)
)
