package webservice

import (
	"io"
	"log"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// initFromRawWS 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的配置文件。
func initFromRawWS(t *testing.T, raw map[string]any) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// 配置文件缺这些字段时加载后补上默认值，否则会以零值运行：网络类型为空监听失败、站点类型为空没有处理器、
// 状态码 0 时 WriteHeader(0) panic、健康检查间隔 0 忙循环、限流突发 0 全部 429、缓存容量 0 不缓存。
func TestInitFillsMissingSiteDefaults(t *testing.T) {
	m := initFromRawWS(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"listeners": []any{map[string]any{
			"key": "l1", "enable": true, "port": 18443, "tls": true, "httpPort": 18080,
			"sites": []any{
				map[string]any{
					"key": "p", "enable": true, "domains": []string{"p.example.com"}, "targets": []string{"http://127.0.0.1:8080"},
					"health":    map[string]any{"enable": true},
					"rateLimit": map[string]any{"enable": true, "rate": 5},
					"cache":     map[string]any{"enable": true},
				},
				map[string]any{"key": "r", "enable": true, "domains": []string{"r.example.com"}, "type": "redirect", "redirectTo": "https://example.org"},
				map[string]any{"key": "t", "enable": true, "domains": []string{"t.example.com"}, "type": "text", "text": "ok"},
			},
			"default": map[string]any{"enable": true, "type": "text", "text": "default"},
		}},
	})
	l := m.cfg.Listeners[0]
	if l.Network != "tcp" || l.HTTPPortMode != PlainHTTPRedirect {
		t.Fatalf("监听器未补默认值: network=%q httpPortMode=%q", l.Network, l.HTTPPortMode)
	}
	p := l.Sites[0]
	if p.Type != TypeProxy {
		t.Fatalf("站点类型未补默认值: %q", p.Type)
	}
	h := p.Health
	if h.Mode != "http" || h.Path != "/" || h.Interval != 10 || h.Timeout != 3 || h.Fails != 3 || h.Passes != 2 {
		t.Fatalf("健康检查未补默认值: %+v", h)
	}
	if p.RateLimit.Burst != 6 {
		t.Fatalf("限流突发未补默认值: %d", p.RateLimit.Burst)
	}
	if p.Cache.MaxMB != 64 {
		t.Fatalf("缓存容量未补默认值: %d", p.Cache.MaxMB)
	}
	if l.Sites[1].RedirectCode != 302 {
		t.Fatalf("重定向状态码未补默认值: %d", l.Sites[1].RedirectCode)
	}
	if ts := l.Sites[2]; ts.StatusCode != 200 || ts.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("固定文本未补默认值: %d %q", ts.StatusCode, ts.ContentType)
	}
	if l.Default.StatusCode != 200 {
		t.Fatalf("默认站点未补默认值: %d", l.Default.StatusCode)
	}
}

// 已有的值保持不变；未启用的功能不补值，加载时也不做校验（不合法的值留给保存时报错）。
func TestInitKeepsExistingSiteValues(t *testing.T) {
	m := initFromRawWS(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"listeners": []any{map[string]any{
			"key": "l1", "enable": true, "network": "tcp6", "port": 18081, "httpPort": 18082,
			"sites": []any{
				map[string]any{
					"key": "p", "enable": true, "domains": []string{"p.example.com"}, "targets": []string{"http://127.0.0.1:8080"},
					"health":    map[string]any{"enable": true, "mode": "tcp", "interval": 30, "timeout": 5, "fails": 1, "passes": 4},
					"rateLimit": map[string]any{"enable": true, "rate": 5, "burst": 50},
					"cache":     map[string]any{"enable": false},
				},
				map[string]any{"key": "r", "enable": true, "domains": []string{"r.example.com"}, "type": "redirect", "redirectTo": "https://example.org", "redirectCode": 308},
				map[string]any{"key": "t", "enable": true, "domains": []string{"t.example.com"}, "type": "text", "statusCode": 503, "contentType": "text/html"},
			},
		}},
	})
	l := m.cfg.Listeners[0]
	// HTTP 监听的额外端口只能直接提供服务
	if l.Network != "tcp6" || l.HTTPPortMode != PlainHTTPServe {
		t.Fatalf("监听器值不符: network=%q httpPortMode=%q", l.Network, l.HTTPPortMode)
	}
	p := l.Sites[0]
	h := p.Health
	if h.Mode != "tcp" || h.Path != "" || h.Interval != 30 || h.Timeout != 5 || h.Fails != 1 || h.Passes != 4 {
		t.Fatalf("健康检查已有值被改动: %+v", h)
	}
	if p.RateLimit.Burst != 50 || p.Cache.MaxMB != 0 {
		t.Fatalf("限流或缓存被改动: burst=%d maxMB=%d", p.RateLimit.Burst, p.Cache.MaxMB)
	}
	if l.Sites[1].RedirectCode != 308 {
		t.Fatalf("重定向状态码被改动: %d", l.Sites[1].RedirectCode)
	}
	if ts := l.Sites[2]; ts.StatusCode != 503 || ts.ContentType != "text/html" {
		t.Fatalf("固定文本被改动: %d %q", ts.StatusCode, ts.ContentType)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameSiteDefaults(t *testing.T) {
	s := Site{
		Domains: []string{"p.example.com"}, Targets: []string{"http://127.0.0.1:8080"},
		Health:    HealthCheck{Enable: true},
		RateLimit: RateLimit{Enable: true, Rate: 2},
		Cache:     CacheConfig{Enable: true},
	}
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	if s.Type != TypeProxy || s.Health.Interval != 10 || s.Health.Path != "/" || s.RateLimit.Burst != 3 || s.Cache.MaxMB != 64 {
		t.Fatalf("保存时默认值不符: %+v", s)
	}
	text := Site{Domains: []string{"t.example.com"}, Type: TypeText}
	if err := text.validate(false); err != nil {
		t.Fatal(err)
	}
	if text.StatusCode != 200 || text.ContentType == "" {
		t.Fatalf("固定文本保存时默认值不符: %+v", text)
	}
}
