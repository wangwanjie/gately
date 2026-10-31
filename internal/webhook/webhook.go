// Package webhook 实现带变量替换的 HTTP 回调，供 DDNS、STUN 等模块在状态变化时通知外部系统。
package webhook

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	return nil
}

// Render 把 s 中的 {name}（或 #{name}）替换为 vars[name]；未知变量保持原样。
// escape 不为 nil 时对替换值做转义（例如 URL 中使用 url.QueryEscape）。
func Render(s string, vars map[string]string, escape func(string) string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '{')
		if start < 0 {
			b.WriteString(s)
			break
		}
		end := strings.IndexByte(s[start:], '}')
		if end < 0 {
			b.WriteString(s)
			break
		}
		end += start
		// JSON 等内容本身含有花括号，取离 } 最近的 { 作为变量起点
		if inner := strings.LastIndexByte(s[start:end], '{'); inner > 0 {
			b.WriteString(s[:start+inner])
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
		b.WriteString(prefix)
		if ok {
			if escape != nil {
				v = escape(v)
			}
			b.WriteString(v)
		} else {
			b.WriteString(s[start : end+1])
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
		body = strings.NewReader(Render(c.Body, vars, nil))
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
		req.Header.Set(strings.TrimSpace(k), Render(strings.TrimSpace(v), vars, nil))
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{}
	if c.InsecureSkipVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	resp, err := client.Do(req)
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

// truncate 折叠空白并截断，避免把整页 HTML 写进日志。
func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
