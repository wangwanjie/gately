// Package dnsprovider 封装各 DNS 服务商的记录更新接口。
//
// 每个服务商实现 Provider.Upsert：查询记录，值相同则不动，不同则更新，不存在则创建。
// 接口地址保存在实例字段中，测试时可替换为 httptest 服务。
package dnsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// 记录同步结果
const (
	ActionUnchanged = "unchanged"
	ActionUpdated   = "updated"
	ActionCreated   = "created"
)

// Record 是要同步的一条解析记录。
type Record struct {
	// Zone 为主域名，例如 example.com
	Zone string
	// Sub 为主机记录，根域名用 "@"
	Sub   string
	Type  string
	Value string
	// TTL 为 0 时使用服务商默认值
	TTL int
	// Line 为解析线路，仅部分国内服务商支持，空表示默认
	Line string
	// Remark 为写到服务商处的记录备注，仅 Meta.SupportsRemark 的服务商使用；
	// 空表示不修改服务商处已有的备注
	Remark string
}

// FQDN 返回完整域名（不带结尾点）。
func (r Record) FQDN() string {
	if r.Sub == "" || r.Sub == "@" {
		return r.Zone
	}
	return r.Sub + "." + r.Zone
}

// RR 返回主机记录，根域名为 "@"。
func (r Record) RR() string {
	if r.Sub == "" {
		return "@"
	}
	return r.Sub
}

// Credentials 是服务商凭据。不同服务商对 ID/Secret 的含义不同，见 Meta 中的标签。
type Credentials struct {
	ID     string            `json:"id"`
	Secret string            `json:"secret"`
	Extra  map[string]string `json:"extra,omitempty"`
}

// Provider 是一个已配置凭据的服务商客户端。
type Provider interface {
	Upsert(ctx context.Context, r Record) (string, error)
}

// Field 描述服务商的额外配置项，供界面渲染表单。
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Multiline   bool   `json:"multiline,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Meta 描述一个服务商。
type Meta struct {
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	IDLabel     string  `json:"idLabel,omitempty"`
	SecretLabel string  `json:"secretLabel,omitempty"`
	Extra       []Field `json:"extra,omitempty"`
	// SupportsLine 表示支持解析线路
	SupportsLine bool `json:"supportsLine,omitempty"`
	// DocURL 指向获取凭据的官方文档
	DocURL string `json:"docUrl,omitempty"`
	// SupportsACME 表示可用于证书申请的 DNS 验证（支持 TXT 记录增删）
	SupportsACME bool `json:"supportsAcme,omitempty"`
	// SupportsRemark 表示同步记录时会把备注写到服务商处
	SupportsRemark bool `json:"supportsRemark,omitempty"`
	// SupportsDelete 表示删除动态域名记录时可以同时删除服务商处的解析
	SupportsDelete bool `json:"supportsDelete,omitempty"`
}

// Factory 根据凭据创建 Provider。
type Factory func(c Credentials, hc *http.Client) (Provider, error)

type entry struct {
	meta    Meta
	factory Factory
}

var registry = map[string]entry{}

// Register 注册额外的服务商（扩展或测试使用）。名称重复时 panic。
func Register(m Meta, f Factory) {
	register(m, f)
}

// register 在各服务商文件的 init 中调用。
func register(m Meta, f Factory) {
	if _, dup := registry[m.Name]; dup {
		panic("重复注册 DNS 服务商: " + m.Name)
	}
	registry[m.Name] = entry{meta: m, factory: f}
}

// List 返回所有服务商描述，按名称排序。
func List() []Meta {
	out := make([]Meta, 0, len(registry))
	for _, e := range registry {
		m := e.meta
		m.SupportsACME = SupportsTXT(m.Name)
		m.SupportsDelete = SupportsDelete(m.Name)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup 返回服务商描述。
func Lookup(name string) (Meta, bool) {
	e, ok := registry[name]
	if ok {
		e.meta.SupportsDelete = SupportsDelete(name)
	}
	return e.meta, ok
}

// New 创建服务商客户端。
func New(name string, c Credentials, hc *http.Client) (Provider, error) {
	e, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("不支持的 DNS 服务商 %q", name)
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	return e.factory(c, hc)
}

// requireCreds 校验必填凭据。
func requireCreds(c Credentials, needID, needSecret bool) error {
	if needID && strings.TrimSpace(c.ID) == "" {
		return errors.New("缺少凭据 ID")
	}
	if needSecret && strings.TrimSpace(c.Secret) == "" {
		return errors.New("缺少凭据 Secret")
	}
	return nil
}

// doJSON 发送请求并把 2xx 响应解析到 out；非 2xx 时返回包含响应体的错误。
func doJSON(hc *http.Client, req *http.Request, out any) error {
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode, Body: string(body)}
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析响应失败: %w: %s", err, truncate(string(body), 300))
	}
	return nil
}

// HTTPError 表示服务商返回了非 2xx 状态码。
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, truncate(e.Body, 300))
}

// truncate 折叠空白并截断，避免把整页 HTML 写进日志。
func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// ctxReq 是创建带 ctx 请求的简写。
func ctxReq(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, url, body)
}

func jsonUnmarshalString(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
