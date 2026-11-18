package stun

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的 stun.json。
func initFromRaw(t *testing.T, raw map[string]any) (*Module, *logbuf.Buffer) {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(100, log.New(io.Discard, "", 0))
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatalf("单条规则无效不应导致模块初始化失败: %v", err)
	}
	return m, logs
}

func rawRule(key string, extra map[string]any) map[string]any {
	r := map[string]any{
		"key": key, "name": key, "enable": true,
		"useGlobalServers": true,
		"targetAddresses":  []string{"127.0.0.1:8080"},
	}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

// 配置文件缺心跳、超时与重试间隔时加载后补上默认值，否则心跳 time.NewTicker(0) 会 panic。
func TestInitFillsMissingRuleDefaults(t *testing.T) {
	m, _ := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"rules":    []any{rawRule("r1", nil)},
	})
	got := m.cfg.Rules[0]
	if got.HeartbeatInterval != 3000 || got.StunTimeout != 3000 || got.RetryInterval != 3000 {
		t.Fatalf("未补默认值: heartbeat=%d timeout=%d retry=%d", got.HeartbeatInterval, got.StunTimeout, got.RetryInterval)
	}
	if got.loadErr != "" {
		t.Fatalf("合法规则不应标记为无效: %s", got.loadErr)
	}
	// 与界面保存一致的规范化：协议、绑定方式补默认，目标地址中的端口拆到目标端口
	if got.Protocol != ProtoTCP4 || got.Bind.Mode != BindIP || got.TargetPort != 8080 || got.TargetAddresses[0] != "127.0.0.1" {
		t.Fatalf("未规范化: %+v", got)
	}
}

// 已有的合法值保持不变。
func TestInitKeepsExistingRuleValues(t *testing.T) {
	m, _ := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"rules": []any{rawRule("r1", map[string]any{
			"protocol": "udp4", "heartbeatInterval": 800, "stunTimeout": 1500, "retryInterval": 5000,
		})},
	})
	got := m.cfg.Rules[0]
	if got.Protocol != ProtoUDP4 || got.HeartbeatInterval != 800 || got.StunTimeout != 1500 || got.RetryInterval != 5000 {
		t.Fatalf("已有值被改动: %+v", got)
	}
}

// 校验失败的规则记日志、不启动，也不能不经修改直接启用。
func TestInitSkipsInvalidRule(t *testing.T) {
	m, logs := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"rules": []any{
			rawRule("good", nil),
			rawRule("bad", map[string]any{"heartbeatInterval": 100, "targetAddresses": []string{" 127.0.0.1:9 "}}),
		},
	})
	bad := m.cfg.Rules[1]
	if bad.loadErr == "" {
		t.Fatal("心跳间隔过短的规则应标记为无效")
	}
	// 失败的规则保持文件中的原样，不被改成半成品
	if bad.HeartbeatInterval != 100 || bad.StunTimeout != 0 || bad.TargetAddresses[0] != " 127.0.0.1:9 " {
		t.Fatalf("无效规则被改动: %+v", bad)
	}
	found := false
	for _, e := range logs.Since(0) {
		if e.Level == logbuf.LevelError && strings.Contains(e.Message, "bad") {
			found = true
		}
	}
	if !found {
		t.Fatal("无效规则应记错误日志")
	}

	// 只验证无效规则不启动；合法规则启动会真的去打洞，这里不启动它
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.mu.Lock()
	m.ctx = ctx
	m.startLocked(bad)
	_, running := m.runners["bad"]
	m.mu.Unlock()
	defer m.Stop()
	if running {
		t.Fatal("无效规则不应启动")
	}

	mux := http.NewServeMux()
	m.Routes(stunTestRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, _ := http.NewRequest("PUT", srv.URL+"/api/stun/rules/bad/enable", strings.NewReader(`{"enable":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("直接启用无效规则应被拒绝: %d", resp.StatusCode)
	}
}

type stunTestRouter struct{ mux *http.ServeMux }

func (tr stunTestRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	tr.mux.HandleFunc(method+" /api/stun"+path, h)
}
