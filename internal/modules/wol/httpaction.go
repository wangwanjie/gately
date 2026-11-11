package wol

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

// HTTPAction 是 http 关机方式发送的请求，例如调用 Home Assistant、智能插座或设备上关机工具的接口。
type HTTPAction struct {
	// Method 为请求方法，留空时有请求体用 POST、否则用 GET
	Method string `json:"method"`
	// URL 为请求地址，只支持 http 与 https
	URL string `json:"url"`
	// Headers 为附加请求头，每项为“名称: 值”，凭据（如 Authorization）放在这里，只读账号看到的值会被脱敏
	Headers []string `json:"headers"`
	// Body 为请求体，可为空
	Body string `json:"body"`
	// Insecure 为 true 时不校验 HTTPS 证书，用于局域网内使用自签名证书的设备
	Insecure bool `json:"insecure"`
}

// 请求的大小限制：请求体与响应内容都不必很大，限制住避免误填大段内容或读取超大响应
const (
	httpBodyLimit   = 16 << 10
	httpOutputLimit = 16 << 10
	httpMaxHeaders  = 20
)

var httpMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// Validate 校验并规范化请求设置。
func (h *HTTPAction) Validate() error {
	h.URL = strings.TrimSpace(h.URL)
	h.Method = strings.ToUpper(strings.TrimSpace(h.Method))
	if h.URL == "" {
		return errors.New("请填写关机请求的地址")
	}
	u, err := url.Parse(h.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("关机请求的地址须以 http:// 或 https:// 开头")
	}
	if h.Method == "" {
		h.Method = "GET"
		if h.Body != "" {
			h.Method = "POST"
		}
	}
	if !httpMethods[h.Method] {
		return fmt.Errorf("不支持的请求方法 %s", h.Method)
	}
	if len(h.Body) > httpBodyLimit || !utf8.ValidString(h.Body) {
		return fmt.Errorf("请求体须为文本，且不超过 %d KB", httpBodyLimit>>10)
	}
	var headers []string
	for _, line := range h.Headers {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		// 名称与值按 HTTP 规范校验，换行等控制字符会被拒绝，不能借此注入额外的请求头
		if !ok || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return fmt.Errorf("请求头 %q 格式应为 名称: 值", line)
		}
		headers = append(headers, name+": "+value)
	}
	if len(headers) > httpMaxHeaders {
		return fmt.Errorf("请求头不能超过 %d 个", httpMaxHeaders)
	}
	h.Headers = headers
	return nil
}

// httpClientFor 返回发送关机请求的客户端，测试可替换。每次新建 Transport：请求很少，
// 不必复用连接，也避免设备重启后连接池里留下的死连接。
var httpClientFor = func(insecure bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 用户为局域网自签名设备显式开启
	}
	tr.DisableKeepAlives = true
	return &http.Client{
		Transport: tr,
		// 最多跟随 3 次跳转
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("跳转次数过多")
			}
			return nil
		},
	}
}

// httpShutdown 发送设备配置的关机请求，响应状态码为 2xx 视为成功。
func (m *Module) httpShutdown(ctx context.Context, d Device) (string, error) {
	h := d.ShutdownHTTP
	log := m.logs.Logger(d.Name)
	var body io.Reader
	if h.Body != "" {
		body = strings.NewReader(h.Body)
	}
	req, err := http.NewRequestWithContext(ctx, h.Method, h.URL, body)
	if err != nil {
		return "", err
	}
	for _, line := range h.Headers {
		name, value, _ := strings.Cut(line, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if strings.EqualFold(name, "Host") {
			req.Host = value
			continue
		}
		req.Header.Add(name, value)
	}
	// 有请求体且没有指定类型时，按内容猜测 JSON 或表单
	if h.Body != "" && req.Header.Get("Content-Type") == "" {
		ct := "text/plain; charset=utf-8"
		if b := strings.TrimSpace(h.Body); strings.HasPrefix(b, "{") || strings.HasPrefix(b, "[") {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Gately-WOL")
	}
	resp, err := httpClientFor(h.Insecure).Do(req)
	if err != nil {
		// url.Error 里带完整地址，只去掉了密码，查询参数中的凭据（?token=…）会进入日志与接口返回
		var ue *url.Error
		if errors.As(err, &ue) {
			ue.URL = redactURL(h.URL)
		}
		log.Errorf("关机请求失败: %v", err)
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, httpOutputLimit))
	out := strings.TrimSpace(string(raw))
	if !utf8.ValidString(out) {
		out = ""
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		log.Errorf("关机请求返回 %s", resp.Status)
		return out, fmt.Errorf("请求返回 %s", resp.Status)
	}
	log.Infof("已发送关机请求（%s %s，返回 %d）", h.Method, redactURL(h.URL), resp.StatusCode)
	return out, nil
}

// redactURL 去掉地址中的用户名密码与查询参数后写日志，避免凭据进入日志。
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	if u.RawQuery != "" {
		u.RawQuery = "…"
	}
	return u.String()
}
