package ddns

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

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的 ddns.json。
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
		t.Fatalf("单个任务无效不应导致模块初始化失败: %v", err)
	}
	return m, logs
}

func rawTask(key string, extra map[string]any) map[string]any {
	t := map[string]any{
		"key": key, "name": key, "enable": true, "provider": "alidns",
		"credentials": map[string]any{"id": "ak", "secret": "sk"},
		"ipv4":        map[string]any{"enable": true, "type": "url", "urls": []string{"https://example.invalid"}},
		"records": []map[string]any{
			{"key": "a", "enable": true, "zone": "Example.com", "sub": "NAS", "type": "a", "value": "{ipv4Addr}"},
		},
	}
	for k, v := range extra {
		t[k] = v
	}
	return t
}

// 配置文件缺 httpTimeout、interval 时加载后补上默认值，否则同步时会用 0 秒超时、0 秒周期运行。
func TestInitFillsMissingDefaults(t *testing.T) {
	m, _ := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"tasks":    []any{rawTask("t1", nil)},
	})
	got := m.cfg.Tasks[0]
	if got.HTTPTimeout != defaultTimeout {
		t.Fatalf("httpTimeout 未补默认值: %d", got.HTTPTimeout)
	}
	if got.Interval != defaultInterval {
		t.Fatalf("interval 未补默认值: %d", got.Interval)
	}
	if got.loadErr != "" {
		t.Fatalf("合法任务不应标记为无效: %s", got.loadErr)
	}
	// 与界面保存一致的规范化：域名、主机记录小写，记录类型大写
	r := got.Records[0]
	if r.Zone != "example.com" || r.Sub != "nas" || r.Type != "A" {
		t.Fatalf("记录未规范化: %+v", r)
	}
}

// 已有的合法值保持不变。
func TestInitKeepsExistingValues(t *testing.T) {
	m, _ := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"tasks":    []any{rawTask("t1", map[string]any{"httpTimeout": 7, "interval": 45, "forceInterval": 600})},
	})
	got := m.cfg.Tasks[0]
	if got.HTTPTimeout != 7 || got.Interval != 45 || got.ForceInterval != 600 {
		t.Fatalf("已有值被改动: timeout=%d interval=%d force=%d", got.HTTPTimeout, got.Interval, got.ForceInterval)
	}
}

// 校验失败的任务记日志、不启动，也不能不经修改直接启用；其他任务照常运行。
func TestInitSkipsInvalidTask(t *testing.T) {
	setupFakes(t)
	m, logs := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true},
		"tasks": []any{
			rawTask("good", nil),
			rawTask("bad", map[string]any{"interval": 5}),
		},
	})
	bad := m.cfg.Tasks[1]
	if bad.loadErr == "" {
		t.Fatal("周期过短的任务应标记为无效")
	}
	// 失败的任务保持文件中的原样，不被改成半成品
	if bad.Interval != 5 || bad.Records[0].Sub != "NAS" {
		t.Fatalf("无效任务被改动: interval=%d sub=%s", bad.Interval, bad.Records[0].Sub)
	}
	found := false
	for _, e := range logs.Since(0) {
		if e.Level == logbuf.LevelError && strings.Contains(e.Message, "bad") {
			found = true
		}
	}
	if !found {
		t.Fatal("无效任务应记错误日志")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	m.mu.Lock()
	_, goodRun := m.runners["good"]
	_, badRun := m.runners["bad"]
	m.mu.Unlock()
	if !goodRun || badRun {
		t.Fatalf("启动结果不符: good=%v bad=%v", goodRun, badRun)
	}

	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	put := func(path, body string) int {
		req, _ := http.NewRequest("PUT", srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := put("/api/ddns/tasks/bad/enable", `{"enable":true}`); code != http.StatusBadRequest {
		t.Fatalf("直接启用无效任务应被拒绝: %d", code)
	}

	// 修正后保存即恢复运行
	body := `{"key":"bad","name":"bad","enable":true,"provider":"alidns","credentials":{"id":"ak","secret":"sk"},` +
		`"ipv4":{"enable":true,"type":"url","urls":["https://example.invalid"]},"interval":60,` +
		`"records":[{"key":"a","enable":true,"zone":"example.com","sub":"nas","type":"A","value":"{ipv4Addr}"}]}`
	if code := put("/api/ddns/tasks/bad", body); code != http.StatusOK {
		t.Fatalf("修正后保存失败: %d", code)
	}
	m.mu.Lock()
	_, badRun = m.runners["bad"]
	saved := m.cfg.Tasks[1]
	m.mu.Unlock()
	if !badRun || saved.loadErr != "" || saved.HTTPTimeout != defaultTimeout {
		t.Fatalf("修正后应启动: run=%v loadErr=%q timeout=%d", badRun, saved.loadErr, saved.HTTPTimeout)
	}
}
