// Package webhook 实现带变量替换的 HTTP 回调，供 DDNS、STUN 等模块在状态变化时通知外部系统。
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/proxyx"
)

// Config 是一个 Webhook 的配置。URL、Headers、Body 中的 {变量名} 会被替换。
type Config struct {
	Enable bool   `json:"enable"`
	URL    string `json:"url"`
	Method string `json:"method"`
	// Headers 每项格式为 "Key: Value"
	Headers []string `json:"headers"`
	Body    string   `json:"body"`
	// SuccessContent 非空时，响应体需包含其中任意一项才视为成功；为空时仅检查 2xx 状态码
	SuccessContent     []string `json:"successContent"`
	Timeout            int      `json:"timeout"`
	InsecureSkipVerify bool     `json:"insecureSkipVerify"`
	// Proxy 为请求使用的代理：留空遵循 HTTPS_PROXY 等环境变量，direct 为直连，update 为全局更新代理，
	// 也可以填写 http://、https://、socks5:// 开头的代理地址（见 proxyx 包）
	Proxy string `json:"proxy,omitempty"`
}

// Validate 校验配置。未启用时不做检查。
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("Webhook 地址无效")
	}
	switch strings.ToUpper(c.Method) {
	case "", "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return fmt.Errorf("不支持的 Webhook 方法 %q", c.Method)
	}
	for _, h := range c.Headers {
		if strings.TrimSpace(h) == "" {
			continue
		}
		if k, _, ok := strings.Cut(h, ":"); !ok || strings.TrimSpace(k) == "" {
			return fmt.Errorf("请求头格式应为 Key: Value，实际 %q", h)
		}
	}
	c.Proxy = proxyx.Normalize(c.Proxy)
	return proxyx.Validate(c.Proxy)
}

// Render 把 s 中的 {name}（或 #{name}）替换为 vars[name]；未知变量保持原样。
// escape 不为 nil 时对替换值做转义（例如 URL 中使用 url.QueryEscape）。
func Render(s string, vars map[string]string, escape func(string) string) string {
	if escape == nil {
		return render(s, vars, nil)
	}
	return render(s, vars, func(v string, _ bool) string { return escape(v) })
}

// RenderJSON 与 Render 相同，但模板是 JSON（对象或数组）时，位于 JSON 字符串内的变量按 JSON 字符串转义，
// 访客可控的文本（例如请求路径）不能借引号、换行改变 JSON 结构；字符串外的变量（通常是数字）原样替换。
// 模板不是 JSON 时与 Render 相同，保持原样。
func RenderJSON(s string, vars map[string]string) string {
	if !jsonTemplate(s, vars) {
		return render(s, vars, nil)
	}
	return render(s, vars, func(v string, inString bool) string {
		if inString {
			return jsonEscape(v)
		}
		return v
	})
}

// jsonTemplate 判断模板是否为 JSON 对象或数组：把已知变量都换成 0 后检查是否为合法 JSON。
func jsonTemplate(s string, vars map[string]string) bool {
	probe := strings.TrimSpace(render(s, vars, func(string, bool) string { return "0" }))
	if probe == "" || (probe[0] != '{' && probe[0] != '[') {
		return false
	}
	return json.Valid([]byte(probe))
}

// jsonEscape 返回 v 作为 JSON 字符串内容（不含两侧引号）的转义结果，不转义 <、>、&。
func jsonEscape(v string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return v
	}
	// Encode 输出带两侧引号与换行的字符串字面量，去掉它们只留内容
	out := strings.TrimSuffix(b.String(), "\n")
	return strings.TrimSuffix(strings.TrimPrefix(out, `"`), `"`)
}

// headerValue 去掉请求头值中的换行等控制字符（换成空格），变量中的访客文本不能借换行注入额外请求头，
// 也不会因含换行导致请求无法发出。
func headerValue(v string) string {
	return strings.Map(func(r rune) rune {
		if r != '\t' && (r < 0x20 || r == 0x7f) {
			return ' '
		}
		return r
	}, v)
}

// render 是 Render 的实现：escape 不为 nil 时对替换值转义，inString 表示变量位于模板中的 JSON 字符串内
// （按模板本身的引号与反斜杠判断，不考虑替换进去的值）。
func render(s string, vars map[string]string, escape func(v string, inString bool) string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	var b strings.Builder
	// inStr、esc 为已写出的模板文本所处的 JSON 字符串状态
	inStr, esc := false, false
	literal := func(t string) {
		b.WriteString(t)
		for i := 0; i < len(t); i++ {
			c := t[i]
			switch {
			case esc:
				esc = false
			case inStr && c == '\\':
				esc = true
			case c == '"':
				inStr = !inStr
			}
		}
	}
	for {
		start := strings.IndexByte(s, '{')
		if start < 0 {
			literal(s)
			break
		}
		end := strings.IndexByte(s[start:], '}')
		if end < 0 {
			literal(s)
			break
		}
		end += start
		// JSON 等内容本身含有花括号，取离 } 最近的 { 作为变量起点
		if inner := strings.LastIndexByte(s[start:end], '{'); inner > 0 {
			literal(s[:start+inner])
			s = s[start+inner:]
			continue
		}
		name := s[start+1 : end]
		v, ok := vars[name]
		prefix := s[:start]
		// 同时支持 #{name} 写法（导入的配置常用）
		if ok && strings.HasSuffix(prefix, "#") {
			prefix = prefix[:len(prefix)-1]
		}
		literal(prefix)
		if ok {
			if escape != nil {
				v = escape(v, inStr)
			}
			b.WriteString(v)
			// 变量前的反斜杠只作用于变量本身
			esc = false
		} else {
			literal(s[start : end+1])
		}
		s = s[end+1:]
	}
	return b.String()
}

// Result 是一次调用的结果，供日志展示。
type Result struct {
	StatusCode int
	Body       string
}

// Send 按配置发送请求。
func Send(ctx context.Context, c Config, vars map[string]string) (*Result, error) {
	method := strings.ToUpper(c.Method)
	if method == "" {
		method = http.MethodGet
	}
	target := Render(strings.TrimSpace(c.URL), vars, url.QueryEscape)
	var body io.Reader
	if method != http.MethodGet && c.Body != "" {
		// 默认按 JSON 发送；请求头中明确写了其他内容类型（如表单、纯文本）时原样替换
		if jsonContent(c.Headers) {
			body = strings.NewReader(RenderJSON(c.Body, vars))
		} else {
			body = strings.NewReader(Render(c.Body, vars, nil))
		}
	}
	timeout := time.Duration(c.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	for _, h := range c.Headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(strings.TrimSpace(k), headerValue(Render(strings.TrimSpace(v), vars, nil)))
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// 超时由 context 控制；客户端按代理设置复用连接
	resp, err := proxyx.Client(c.Proxy, 0, c.InsecureSkipVerify).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	res := &Result{StatusCode: resp.StatusCode, Body: string(data)}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return res, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(res.Body, 200))
	}
	if len(c.SuccessContent) > 0 {
		for _, s := range c.SuccessContent {
			if s != "" && strings.Contains(res.Body, s) {
				return res, nil
			}
		}
		return res, errors.New("响应中未包含成功标识: " + truncate(res.Body, 200))
	}
	return res, nil
}

// jsonContent 判断请求体是否按 JSON 发送：没有填写 Content-Type（默认 application/json）或填写的是 JSON 类型。
func jsonContent(headers []string) bool {
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "Content-Type") {
			continue
		}
		mt, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(v)), ";")
		mt = strings.TrimSpace(mt)
		return mt == "" || mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.Contains(mt, "{")
	}
	return true
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
