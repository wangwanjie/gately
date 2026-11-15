package dnsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Dynadot 动态 DNS：https://www.dynadot.com/help/question/enable-DDNS
//
// 使用域名专属的 Dynamic DNS 密码调用 set_ddns，只能写 A / AAAA 记录，
// 没有查询、删除和 TXT 接口，因此不实现 TXT 与删除。每个域名的密码不同，一个域名对应一份凭据。

func init() {
	register(Meta{
		Name:        "dynadot",
		Title:       "Dynadot",
		SecretLabel: "Dynamic DNS 密码",
		DocURL:      "https://www.dynadot.com/help/question/enable-DDNS",
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		if err := requireCreds(c, false, true); err != nil {
			return nil, err
		}
		return &dynadot{pwd: strings.TrimSpace(c.Secret), hc: hc, endpoint: "https://www.dynadot.com/set_ddns"}, nil
	})
}

// dynadotTTLs 是 set_ddns 文档列出的 TTL 可选值（5 分钟到 8 小时）。
var dynadotTTLs = []int{300, 600, 1800, 3600, 7200, 14400, 28800}

// dynadotTTL 把 TTL 取整到不小于它的最近可选值：0 取 600，超过上限取 28800。
func dynadotTTL(ttl int) int {
	if ttl <= 0 {
		return 600
	}
	for _, v := range dynadotTTLs {
		if ttl <= v {
			return v
		}
	}
	return dynadotTTLs[len(dynadotTTLs)-1]
}

type dynadot struct {
	pwd      string
	hc       *http.Client
	endpoint string
}

// Upsert 调用 set_ddns 更新目标主机记录。接口不提供查询，无法区分新增、更新与未变化，成功时一律返回 updated，
// 重复提交由调用方的缓存避免。
func (d *dynadot) Upsert(ctx context.Context, r Record) (string, error) {
	if r.Type != "A" && r.Type != "AAAA" {
		return "", errors.New("Dynadot 动态 DNS 仅支持 A 与 AAAA 记录")
	}
	q := url.Values{
		"domain": {r.Zone},
		"type":   {r.Type},
		"ip":     {r.Value},
		"pwd":    {d.pwd},
		"ttl":    {strconv.Itoa(dynadotTTL(r.TTL))},
	}
	// subDomain 始终携带（根域名写 "@"），containRoot 仅在根域名时为 true
	q.Set("subDomain", r.RR())
	if r.RR() == "@" {
		q.Set("containRoot", "true")
	} else {
		q.Set("containRoot", "false")
	}
	req, err := ctxReq(ctx, http.MethodGet, d.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		// 密码在查询串里，错误信息要先抹掉
		return "", errors.New("Dynadot: " + redactSecret(err.Error(), d.pwd))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Dynadot: %w", &HTTPError{Status: resp.StatusCode, Body: string(raw)})
	}
	var out struct {
		Status    string   `json:"status"`
		ErrorCode *int     `json:"error_code"`
		Content   []string `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("Dynadot: 无法解析响应: %s", truncate(string(raw), 200))
	}
	if (out.ErrorCode != nil && *out.ErrorCode == -1) || strings.EqualFold(out.Status, "error") || strings.EqualFold(out.Status, "fail") {
		msg := strings.Join(out.Content, ", ")
		if msg == "" {
			msg = truncate(string(raw), 200)
		}
		return "", fmt.Errorf("Dynadot: %s", msg)
	}
	return ActionUpdated, nil
}
