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

// NameSilo DNS：https://www.namesilo.com/api-reference
// 所有操作都是带 key 的 GET 请求，业务结果在响应 XML 的 reply.code 中（300 表示成功）。

func init() {
	register(Meta{
		Name:           "namesilo",
		Title:          "NameSilo",
		SecretLabel:    "API Key",
		DocURL:         "https://www.namesilo.com/account/api-manager",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &nameSilo{key: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://www.namesilo.com/api"}, nil
	})
}

type nameSilo struct {
	key      string
	hc       *http.Client
	endpoint string
}

type nameSiloRecord struct {
	ID    string `xml:"record_id"`
	Type  string `xml:"type"`
	Host  string `xml:"host"`
	Value string `xml:"value"`
	TTL   int    `xml:"ttl"`
}

type nameSiloReply struct {
	Reply struct {
		Code    int              `xml:"code"`
		Detail  string           `xml:"detail"`
		Records []nameSiloRecord `xml:"resource_record"`
	} `xml:"reply"`
}

// nameSiloTTL 返回提交的 TTL：接口要求 3600 到 2592000 秒。
func nameSiloTTL(ttl int) int {
	if ttl < 3600 {
		return 3600
	}
	if ttl > 2592000 {
		return 2592000
	}
	return ttl
}

// nameSiloHost 返回 add/update 使用的 rrhost：只写子域名部分，根域名写 "@"。
func nameSiloHost(sub string) string {
	if sub == "" {
		return "@"
	}
	return sub
}

// call 调用一个操作并检查 reply.code。key 在查询串里，错误信息要先抹掉它。
func (n *nameSilo) call(ctx context.Context, op string, q url.Values) (*nameSiloReply, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("version", "1")
	q.Set("type", "xml")
	q.Set("key", n.key)
	req, err := ctxReq(ctx, http.MethodGet, n.endpoint+"/"+op+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := n.hc.Do(req)
	if err != nil {
		return nil, errors.New("NameSilo: " + redactSecret(err.Error(), n.key))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NameSilo: %w", &HTTPError{Status: resp.StatusCode, Body: string(raw)})
	}
	var out nameSiloReply
	if err := xml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("NameSilo: 无法解析响应: %s", truncate(string(raw), 200))
	}
	if out.Reply.Code != 300 {
		return nil, fmt.Errorf("NameSilo %s: %s (%d)", op, out.Reply.Detail, out.Reply.Code)
	}
	return &out, nil
}

// hostMatches 判断列表中的 host 是否对应目标主机记录。
// 文档示例写完整域名，也有实现按相对名返回，两种都接受。
func (n *nameSilo) hostMatches(host, zone, sub string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	zone = strings.ToLower(zone)
	if sub == "@" || sub == "" {
		return host == "" || host == "@" || host == zone
	}
	sub = strings.ToLower(sub)
	return host == sub || host == sub+"."+zone
}

func (n *nameSilo) list(ctx context.Context, zone, sub, typ string) ([]nameSiloRecord, error) {
	resp, err := n.call(ctx, "dnsListRecords", url.Values{"domain": {zone}})
	if err != nil {
		return nil, err
	}
	var out []nameSiloRecord
	for _, rec := range resp.Reply.Records {
		if rec.Type == typ && n.hostMatches(rec.Host, zone, sub) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (n *nameSilo) add(ctx context.Context, zone, sub, typ, value string, ttl int) error {
	_, err := n.call(ctx, "dnsAddRecord", url.Values{
		"domain": {zone}, "rrtype": {typ}, "rrhost": {nameSiloHost(sub)}, "rrvalue": {value}, "rrttl": {strconv.Itoa(nameSiloTTL(ttl))},
	})
	return err
}

func (n *nameSilo) remove(ctx context.Context, zone, id string) error {
	_, err := n.call(ctx, "dnsDeleteRecord", url.Values{"domain": {zone}, "rrid": {id}})
	return err
}

func (n *nameSilo) Upsert(ctx context.Context, r Record) (string, error) {
	cur, err := n.list(ctx, r.Zone, r.RR(), r.Type)
	if err != nil {
		return "", err
	}
	if len(cur) == 0 {
		if err := n.add(ctx, r.Zone, r.RR(), r.Type, r.Value, r.TTL); err != nil {
			return "", err
		}
		return ActionCreated, nil
	}
	target := cur[0]
	for _, rec := range cur {
		if strings.EqualFold(rec.Value, r.Value) {
			target = rec
			break
		}
	}
	ttl := nameSiloTTL(r.TTL)
	if strings.EqualFold(target.Value, r.Value) && (r.TTL == 0 || target.TTL == ttl) {
		return ActionUnchanged, nil
	}
	// 未指定 TTL 时沿用记录原有的 TTL
	if r.TTL == 0 && target.TTL > 0 {
		ttl = target.TTL
	}
	_, err = n.call(ctx, "dnsUpdateRecord", url.Values{
		"domain": {r.Zone}, "rrid": {target.ID}, "rrhost": {nameSiloHost(r.RR())}, "rrvalue": {r.Value}, "rrttl": {strconv.Itoa(ttl)},
	})
	if err != nil {
		return "", err
	}
	return ActionUpdated, nil
}

func (n *nameSilo) AddTXT(ctx context.Context, zone, sub, value string) error {
	return n.add(ctx, zone, sub, "TXT", value, txtTTL)
}

func (n *nameSilo) DeleteTXT(ctx context.Context, zone, sub, value string) error {
	recs, err := n.list(ctx, zone, sub, "TXT")
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if unquote(rec.Value) == value {
			if err := n.remove(ctx, zone, rec.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *nameSilo) findRecords(ctx context.Context, r Record) ([]remoteRecord, error) {
	recs, err := n.list(ctx, r.Zone, r.RR(), r.Type)
	if err != nil {
		return nil, err
	}
	var out []remoteRecord
	for _, rec := range recs {
		out = append(out, remoteRecord{id: rec.ID, values: []string{rec.Value}, ttl: rec.TTL})
	}
	return out, nil
}

func (n *nameSilo) removeRecord(ctx context.Context, r Record, rec remoteRecord) error {
	return n.remove(ctx, r.Zone, rec.id)
}

var (
	_ TXTProvider   = (*nameSilo)(nil)
	_ recordDeleter = (*nameSilo)(nil)
)
