package knock

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/spa"
)

type testRouter struct{ mux *http.ServeMux }

func (tr testRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	tr.mux.HandleFunc(method+" /api/knock"+path, h)
}

// serve 注册模块接口，返回测试服务器。
func (e *testEnv) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	e.m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func put(t *testing.T, url, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// 启用规则（单个与批量）也检查冲突：拦截敲门端口本身的停用规则不能被启用。
func TestEnableChecksConflict(t *testing.T) {
	r := sshRule("bad")
	r.Ports = []string{"tcp/22", "udp/62201"}
	r.Enable = false
	env := newEnv(t, r)
	env.m.mu.Lock()
	env.m.cfg.Settings.Port = 62201
	env.m.mu.Unlock()
	srv := env.serve(t)
	if code := put(t, srv.URL+"/api/knock/rules/r-bad/enable", `{"enable":true}`); code != http.StatusBadRequest {
		t.Fatalf("单个启用应被拒绝: %d", code)
	}
	if code := put(t, srv.URL+"/api/knock/rules-enable", `{"keys":["r-bad"],"enable":true}`); code != http.StatusBadRequest {
		t.Fatalf("批量启用应被拒绝: %d", code)
	}
	if env.rule("r-bad").Enable {
		t.Fatal("被拒绝后规则应保持停用")
	}
}

// 生成防火墙规则时始终排除 UDP 敲门端口与协议分流接收敲门报文的 TCP 端口（兜底：手改配置等情况）。
func TestPlanExcludesKnockPorts(t *testing.T) {
	r := sshRule("ssh")
	r.Ports = []string{"tcp/22", "tcp/443", "udp/62201"}
	env := newEnv(t, r)
	knockgate.Default.SetSPAPorts("protoroute", []int{443})
	t.Cleanup(func() { knockgate.Default.SetSPAPorts("protoroute", nil) })
	env.m.mu.Lock()
	env.m.cfg.Settings.Port = 62201
	env.m.applyLocked()
	env.m.mu.Unlock()
	got := env.be.lastPlan().Ports
	if !slices.Equal(got, []spa.Access{{Proto: "tcp", Port: 22}}) {
		t.Fatalf("应排除敲门端口: %v", got)
	}
	if !knockgate.Default.Protected("tcp", 22) || knockgate.Default.Protected("tcp", 443) {
		t.Fatal("发布的受保护端口不对")
	}
	// 协议分流的敲门端口变化后，定期检查时重新计算
	knockgate.Default.SetSPAPorts("protoroute", nil)
	env.m.refreshSPAPorts()
	if got := env.be.lastPlan().Ports; len(got) != 2 {
		t.Fatalf("协议分流不再使用 443 后应恢复拦截: %v", got)
	}
	// 拦截协议分流敲门端口的规则不能保存
	env.m.mu.Lock()
	defer env.m.mu.Unlock()
	knockgate.Default.SetSPAPorts("protoroute", []int{8443})
	x := sshRule("x")
	x.Ports = []string{"tcp/8443"}
	if err := env.m.checkRuleLocked(&x); err == nil || !strings.Contains(err.Error(), "8443") {
		t.Fatalf("应拒绝拦截协议分流的敲门端口: %v", err)
	}
}
