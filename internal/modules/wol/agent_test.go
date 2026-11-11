package wol

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// publicTestRouter 把免登录接口挂到 /api/wol/public 下，与管理后台一致。
type publicTestRouter struct{ mux *http.ServeMux }

func (r publicTestRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	r.mux.HandleFunc(method+" /api/wol/public"+path, h)
}

// fastAgentTimes 调小客户端相关的时间参数。
func fastAgentTimes(t *testing.T) {
	t.Helper()
	pw, og, aw := agentPollWait, agentOnlineGrace, agentAckWait
	agentPollWait, agentOnlineGrace, agentAckWait = 2*time.Second, 300*time.Millisecond, 600*time.Millisecond
	t.Cleanup(func() { agentPollWait, agentOnlineGrace, agentAckWait = pw, og, aw })
}

// newAgentModule 创建同时挂载管理接口与免登录接口的模块。
func newAgentModule(t *testing.T) (*Module, *httptest.Server, *logbuf.Buffer) {
	t.Helper()
	fastAgentTimes(t)
	st, _ := config.Open(t.TempDir())
	m := New()
	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	m.PublicRoutes(publicTestRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, srv, logs
}

// agentPost 以客户端身份发送表单请求。
func agentPost(t *testing.T, srv *httptest.Server, path string, form url.Values) (int, string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/wol/public/agent/"+path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

// agentPostAsync 供 goroutine 中使用：出错时返回状态码 0，不调用 t.Fatal。
func agentPostAsync(srv *httptest.Server, path string, form url.Values) (int, string) {
	resp, err := http.Post(srv.URL+"/api/wol/public/agent/"+path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func pollForm(token, instance string) url.Values {
	return url.Values{"token": {token}, "instance": {instance}, "host": {"pc-1"}, "os": {"Windows-10.0-AMD64"}, "version": {"1"}}
}

func ackForm(token, instance, id, result string) url.Values {
	return url.Values{"token": {token}, "instance": {instance}, "id": {id}, "result": {result}}
}

// revealToken 经 secrets 接口取设备的客户端凭据。
func revealToken(t *testing.T, srv *httptest.Server, key string) string {
	t.Helper()
	code, body := call(t, srv, "GET", "/devices/"+key+"/secrets", nil)
	if code != 200 {
		t.Fatalf("查看凭据失败: %d %s", code, body)
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(body), &out)
	return out["agentToken"]
}

func addAgentDevice(t *testing.T, srv *httptest.Server, name string) (string, string) {
	t.Helper()
	key := addDevice(t, srv, map[string]any{"name": name, "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownMode": "agent"})
	return key, revealToken(t, srv, key)
}

// waitPolling 等到客户端的轮询已登记（指令可以立即投递）。
func waitPolling(t *testing.T, m *Module, key string) {
	t.Helper()
	waitFor(t, "轮询登记", func() bool {
		m.agents.mu.Lock()
		defer m.agents.mu.Unlock()
		c := m.agents.conns[key]
		return c != nil && c.poll != nil
	})
}

func TestAgentValidate(t *testing.T) {
	base := func() Device { return Device{Name: "pc", MACs: []string{"aa:bb:cc:dd:ee:ff"}} }
	d := base()
	d.ShutdownMode = ModeAgent
	if err := d.Validate(); err != nil || !validAgentToken(d.AgentToken) || len(d.AgentToken) != 64 {
		t.Fatalf("应生成凭据: %+v %v", d, err)
	}
	// 新增时提交占位符也生成新凭据
	d = base()
	d.ShutdownMode, d.AgentToken = ModeAgent, crud.SecretMask
	if err := d.Validate(); err != nil || !validAgentToken(d.AgentToken) {
		t.Fatalf("占位符应换成新凭据: %+v %v", d, err)
	}
	d = base()
	d.ShutdownMode, d.AgentToken = ModeAgent, "not-hex-token-not-hex-token-not-hex"
	if err := d.Validate(); err == nil {
		t.Fatal("无效凭据应被拒绝")
	}
	// 换成其他方式时清空无关字段
	d = base()
	d.ShutdownMode, d.AgentToken, d.ShutdownConn, d.ShutdownCommand = ModeSSH, newAgentToken(), "c1", "poweroff"
	d.ShutdownHTTP = HTTPAction{URL: "http://x"}
	if err := d.Validate(); err != nil || d.AgentToken != "" || d.ShutdownHTTP.URL != "" {
		t.Fatalf("ssh 方式应清空凭据与请求: %+v %v", d, err)
	}
	d = base()
	d.ShutdownMode, d.ShutdownConn, d.ShutdownCommand = ModeAgent, "c1", "poweroff"
	if err := d.Validate(); err != nil || d.ShutdownConn != "" || d.ShutdownCommand != "" {
		t.Fatalf("客户端方式应清空 SSH 设置: %+v %v", d, err)
	}
	// 旧配置只有 shutdownConn：视为 ssh
	d = base()
	d.ShutdownConn, d.ShutdownCommand = "c1", "poweroff"
	if err := d.Validate(); err != nil || d.ShutdownMode != ModeSSH {
		t.Fatalf("旧配置应视为 ssh: %+v %v", d, err)
	}
	d = base()
	d.ShutdownMode = "telnet"
	if err := d.Validate(); err == nil {
		t.Fatal("未知方式应被拒绝")
	}
	d = base()
	d.ShutdownMode = ModeSSH
	if err := d.Validate(); err == nil {
		t.Fatal("ssh 方式未选连接应报错")
	}
}

func TestHTTPActionValidate(t *testing.T) {
	ok := []HTTPAction{
		{URL: "http://192.168.1.10:8000/shutdown"},
		{URL: "https://ha.local/api/services/button/press", Body: `{"entity_id":"button.pc"}`, Headers: []string{" Authorization: Bearer abc ", ""}},
		{Method: "put", URL: "http://x/y"},
	}
	for _, h := range ok {
		if err := h.Validate(); err != nil {
			t.Fatalf("%+v 应通过: %v", h, err)
		}
	}
	h := HTTPAction{URL: "http://x", Body: "a=1"}
	_ = h.Validate()
	if h.Method != "POST" {
		t.Fatalf("有请求体时默认 POST: %s", h.Method)
	}
	h = HTTPAction{URL: "http://x", Headers: []string{" X-A :  1 "}}
	_ = h.Validate()
	if h.Headers[0] != "X-A: 1" {
		t.Fatalf("请求头应规范化: %q", h.Headers)
	}
	bad := []HTTPAction{
		{},
		{URL: "ftp://x/y"},
		{URL: "http://"},
		{URL: "file:///etc/passwd"},
		{URL: "http://x", Method: "CONNECT"},
		{URL: "http://x", Headers: []string{"NoColon"}},
		{URL: "http://x", Headers: []string{"X-A: 1\r\nX-B: 2"}},
		{URL: "http://x", Headers: []string{"Bad Name: 1"}},
		{URL: "http://x", Body: strings.Repeat("a", httpBodyLimit+1)},
		{URL: "http://x", Body: "\xff\xfe"},
	}
	for _, h := range bad {
		if err := h.Validate(); err == nil {
			t.Fatalf("%+v 应被拒绝", h)
		}
	}
	many := HTTPAction{URL: "http://x"}
	for i := 0; i <= httpMaxHeaders; i++ {
		many.Headers = append(many.Headers, fmt.Sprintf("X-%d: v", i))
	}
	if err := many.Validate(); err == nil {
		t.Fatal("请求头过多应被拒绝")
	}
}

// 凭据在列表中脱敏、可经 secrets 查看；修改时提交占位符保留原值；复制的设备生成新凭据；重新生成后旧凭据失效。
func TestAgentTokenLifecycle(t *testing.T) {
	_, srv, _ := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	if !validAgentToken(tok) {
		t.Fatalf("凭据无效: %q", tok)
	}
	_, list := call(t, srv, "GET", "/devices", nil)
	if strings.Contains(list, tok) || !strings.Contains(list, crud.SecretMask) {
		t.Fatalf("列表中凭据应脱敏: %s", list)
	}
	if code, body := call(t, srv, "PUT", "/devices/"+key, map[string]any{"name": "pc2", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownMode": "agent", "agentToken": crud.SecretMask}); code != 200 {
		t.Fatalf("修改失败: %d %s", code, body)
	}
	if got := revealToken(t, srv, key); got != tok {
		t.Fatalf("修改后凭据应保持不变: %s != %s", got, tok)
	}
	code, body := call(t, srv, "POST", "/devices/"+key+"/copy", map[string]any{"name": "pc3", "macs": []string{"aa:bb:cc:dd:ee:01"}, "shutdownMode": "agent", "agentToken": crud.SecretMask})
	if code != 200 {
		t.Fatalf("复制失败: %d %s", code, body)
	}
	var cp struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal([]byte(body), &cp)
	if ct := revealToken(t, srv, cp.Key); ct == tok || !validAgentToken(ct) {
		t.Fatalf("复制的设备应有自己的凭据: %s", ct)
	}
	code, body = call(t, srv, "POST", "/devices/"+key+"/agent-token", nil)
	var nt map[string]string
	_ = json.Unmarshal([]byte(body), &nt)
	if code != 200 || nt["agentToken"] == tok || !validAgentToken(nt["agentToken"]) {
		t.Fatalf("重新生成失败: %d %s", code, body)
	}
	if code, _ := agentPost(t, srv, "poll", pollForm(tok, "inst-aaaaaaaa")); code != http.StatusUnauthorized {
		t.Fatalf("旧凭据应失效: %d", code)
	}
	// 换成其他方式：凭据清空，重新生成接口拒绝
	call(t, srv, "PUT", "/devices/"+key, map[string]any{"name": "pc2", "macs": []string{"aa:bb:cc:dd:ee:ff"}})
	if got := revealToken(t, srv, key); got != "" {
		t.Fatalf("不再使用客户端时凭据应清空: %q", got)
	}
	if code, _ := call(t, srv, "POST", "/devices/"+key+"/agent-token", nil); code != http.StatusBadRequest {
		t.Fatalf("非客户端方式不能重新生成: %d", code)
	}
}

// 完整流程：客户端轮询 → 管理界面关机 → 轮询返回指令 → 客户端确认 → 接口返回成功，设备随即显示离线。
func TestAgentShutdownFlow(t *testing.T) {
	m, srv, logs := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	const inst = "inst-0001aaaa"

	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != http.StatusConflict || !strings.Contains(body, "未连接") {
		t.Fatalf("客户端未连接时应返回 409: %d %s", code, body)
	}
	_, st := call(t, srv, "GET", "/status", nil)
	if !strings.Contains(st, `"`+key+`":"offline"`) {
		t.Fatalf("未连接时应为离线: %s", st)
	}

	polled := make(chan string, 1)
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok, inst))
		polled <- body
	}()
	waitPolling(t, m, key)
	_, st = call(t, srv, "GET", "/status", nil)
	if !strings.Contains(st, `"`+key+`":"online"`) {
		t.Fatalf("轮询中应为在线: %s", st)
	}
	_, ag := call(t, srv, "GET", "/agents", nil)
	if !strings.Contains(ag, "pc-1") || !strings.Contains(ag, "Windows-10.0-AMD64") || !strings.Contains(ag, `"online":true`) {
		t.Fatalf("连接信息错误: %s", ag)
	}

	done := make(chan [2]any, 1)
	go func() {
		code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", map[string]string{"action": "reboot"})
		done <- [2]any{code, body}
	}()
	var cmd string
	select {
	case cmd = <-polled:
	case <-time.After(3 * time.Second):
		t.Fatal("轮询没有收到指令")
	}
	parts := strings.Fields(cmd)
	if len(parts) != 3 || parts[0] != "cmd" || parts[2] != "reboot" {
		t.Fatalf("指令格式错误: %q", cmd)
	}
	// 其他客户端运行标识不能确认
	if code, _ := agentPost(t, srv, "ack", ackForm(tok, "inst-other00", parts[1], "ok")); code != http.StatusConflict {
		t.Fatalf("其他客户端确认应被拒绝: %d", code)
	}
	if code, body := agentPost(t, srv, "ack", ackForm(tok, inst, parts[1], "ok")); code != 200 || body != "ok" {
		t.Fatalf("确认失败: %d %s", code, body)
	}
	r := <-done
	if r[0].(int) != 200 {
		t.Fatalf("重启接口应成功: %v", r)
	}
	if m.agents.online(key) {
		t.Fatal("确认后应立即显示离线")
	}
	if info := m.agents.info()[key]; info.LastSeen != 0 || info.Online {
		t.Fatalf("确认后最近通信时间应为 0: %+v", info)
	}
	// 执行失败的报告写入日志
	if code, _ := agentPost(t, srv, "ack", url.Values{"token": {tok}, "instance": {inst}, "id": {parts[1]}, "result": {"fail"}, "msg": {"access\ndenied"}}); code != 200 {
		t.Fatalf("失败报告应被接受: %d", code)
	}
	text := fmt.Sprint(logs.Since(0))
	if !strings.Contains(text, "客户端已连接") || !strings.Contains(text, "重启") || !strings.Contains(text, "accessdenied") {
		t.Fatalf("日志缺少内容: %s", text)
	}
	if code, _ := agentPost(t, srv, "ack", url.Values{"token": {tok}, "instance": {inst}, "id": {"ffff"}, "result": {"fail"}}); code != http.StatusNotFound {
		t.Fatalf("未知指令的失败报告应被拒绝: %d", code)
	}
}

// 客户端收到指令却没有确认：接口超时报错，之后迟到的确认被拒绝（客户端据此不执行），指令不会留到下次。
func TestAgentAckTimeout(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	const inst = "inst-0002aaaa"
	polled := make(chan string, 1)
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok, inst))
		polled <- body
	}()
	waitPolling(t, m, key)
	code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil)
	if code != http.StatusBadGateway || !strings.Contains(body, "未确认") {
		t.Fatalf("未确认应返回 502: %d %s", code, body)
	}
	parts := strings.Fields(<-polled)
	if code, body := agentPost(t, srv, "ack", ackForm(tok, inst, parts[1], "ok")); code != http.StatusGone || body != "expired" {
		t.Fatalf("迟到的确认应被拒绝: %d %s", code, body)
	}
	// 下一次轮询不会再收到这条指令
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok, inst))
		polled <- body
	}()
	select {
	case b := <-polled:
		if b != "noop" {
			t.Fatalf("过期指令不应再次下发: %q", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("轮询没有按时返回")
	}
}

// 指令在客户端两次轮询之间发出（仍在在线宽限期内）：下次轮询立即取走。
func TestAgentQueuedBetweenPolls(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	const inst = "inst-0003aaaa"
	// 制造“刚结束一次轮询”的状态
	m.agents.mu.Lock()
	m.agents.conns[key] = &agentConn{tokenSum: sha256.Sum256([]byte(tok)), instance: inst, lastSeen: time.Now()}
	m.agents.mu.Unlock()
	done := make(chan int, 1)
	go func() {
		code, _ := call(t, srv, "POST", "/devices/"+key+"/shutdown", map[string]string{"action": "sleep"})
		done <- code
	}()
	waitFor(t, "指令排队", func() bool {
		m.agents.mu.Lock()
		defer m.agents.mu.Unlock()
		return m.agents.conns[key].queued != nil
	})
	code, body := agentPost(t, srv, "poll", pollForm(tok, inst))
	parts := strings.Fields(body)
	if code != 200 || len(parts) != 3 || parts[2] != "sleep" {
		t.Fatalf("排队的指令应立即下发: %d %q", code, body)
	}
	if code, _ := agentPost(t, srv, "ack", ackForm(tok, inst, parts[1], "ok")); code != 200 {
		t.Fatalf("确认失败: %d", code)
	}
	if c := <-done; c != 200 {
		t.Fatalf("睡眠接口应成功: %d", c)
	}
}

// 同一凭据被另一台机器使用：原客户端在线期间拒绝新客户端；同一客户端重连替换旧轮询，旧轮询立即返回 noop。
func TestAgentConflictAndReplace(t *testing.T) {
	m, srv, logs := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	first := make(chan string, 1)
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok, "inst-first00"))
		first <- body
	}()
	waitPolling(t, m, key)
	if code, body := agentPost(t, srv, "poll", pollForm(tok, "inst-second0")); code != http.StatusConflict || body != "conflict" {
		t.Fatalf("另一客户端应被拒绝: %d %s", code, body)
	}
	if !strings.Contains(fmt.Sprint(logs.Since(0)), "多个客户端") {
		t.Fatal("应记录凭据被多个客户端使用")
	}
	second := make(chan string, 1)
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok, "inst-first00"))
		second <- body
	}()
	select {
	case b := <-first:
		if b != "noop" {
			t.Fatalf("被替换的轮询应返回 noop: %q", b)
		}
	case <-time.After(time.Second):
		t.Fatal("被替换的轮询没有立即返回")
	}
	waitPolling(t, m, key)
	// 原客户端离线超过宽限期后，新客户端可以接入
	m.agents.drop(key)
	<-second
	time.Sleep(agentOnlineGrace)
	go agentPostAsync(srv, "poll", pollForm(tok, "inst-second0"))
	waitPolling(t, m, key)
	m.agents.mu.Lock()
	inst := m.agents.conns[key].instance
	m.agents.mu.Unlock()
	if inst != "inst-second0" {
		t.Fatalf("应换成新客户端: %s", inst)
	}
	m.agents.drop(key)
}

// 设备换成其他关机方式或被删除时，挂起中的轮询立即结束，之后凭据失效。
func TestAgentDroppedOnChange(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	polled := make(chan string, 1)
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok, "inst-0004aaaa"))
		polled <- body
	}()
	waitPolling(t, m, key)
	call(t, srv, "PUT", "/devices/"+key, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownMode": "http", "shutdownHttp": map[string]any{"url": "http://127.0.0.1:1/x"}})
	select {
	case <-polled:
	case <-time.After(time.Second):
		t.Fatal("换方式后轮询应立即结束")
	}
	if m.agents.online(key) {
		t.Fatal("换方式后不应在线")
	}
	if code, _ := agentPost(t, srv, "poll", pollForm(tok, "inst-0004aaaa")); code != http.StatusUnauthorized {
		t.Fatalf("换方式后凭据应失效: %d", code)
	}
	key2, tok2 := addAgentDevice(t, srv, "pc2")
	go func() {
		_, body := agentPostAsync(srv, "poll", pollForm(tok2, "inst-0005aaaa"))
		polled <- body
	}()
	waitPolling(t, m, key2)
	call(t, srv, "DELETE", "/devices/"+key2, nil)
	select {
	case <-polled:
	case <-time.After(time.Second):
		t.Fatal("删除设备后轮询应立即结束")
	}
}

// 轮询已通过凭据校验、尚未登记连接时凭据被重新生成：旧凭据的轮询不能留在连接表里接收之后的指令。
func TestAgentPollTokenRegeneratedBeforeRegister(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	var once sync.Once
	agentPollHook = func() {
		once.Do(func() {
			if code, body := call(t, srv, "POST", "/devices/"+key+"/agent-token", nil); code != 200 {
				t.Errorf("重新生成凭据失败: %d %s", code, body)
			}
		})
	}
	t.Cleanup(func() { agentPollHook = nil })
	code, _ := agentPost(t, srv, "poll", pollForm(tok, "inst-0006aaaa"))
	if code != http.StatusUnauthorized {
		t.Fatalf("凭据已重新生成时轮询应返回 401: %d", code)
	}
	m.agents.mu.Lock()
	c := m.agents.conns[key]
	m.agents.mu.Unlock()
	if c != nil {
		t.Fatal("旧凭据的轮询不应留在连接表中")
	}
	if m.agents.online(key) {
		t.Fatal("旧凭据的客户端不应显示在线")
	}
}

// 请求格式与凭据校验：错误凭据、缺少运行标识、超大请求体、未知操作、非客户端方式的重启。
func TestAgentRequestChecks(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	key, tok := addAgentDevice(t, srv, "pc")
	if code, _ := agentPost(t, srv, "poll", pollForm(strings.Repeat("0", 64), "inst-0006aaaa")); code != http.StatusUnauthorized {
		t.Fatalf("错误凭据应返回 401: %d", code)
	}
	if code, _ := agentPost(t, srv, "poll", url.Values{"token": {tok}}); code != http.StatusBadRequest {
		t.Fatalf("缺少运行标识应返回 400: %d", code)
	}
	if code, _ := agentPost(t, srv, "poll", pollForm(tok, "bad id!")); code != http.StatusBadRequest {
		t.Fatalf("运行标识含非法字符应返回 400: %d", code)
	}
	// 超过上限的请求体不解析，凭据取不到
	big := pollForm(tok, "inst-0006aaaa")
	big.Set("pad", strings.Repeat("x", agentFormLimit))
	if code, _ := agentPost(t, srv, "poll", big); code != http.StatusUnauthorized {
		t.Fatalf("超大请求体应被拒绝: %d", code)
	}
	// Authorization 头同样可用；上报的字段去掉控制字符
	req, _ := http.NewRequest("POST", srv.URL+"/api/wol/public/agent/poll", strings.NewReader(url.Values{"instance": {"inst-0007aaaa"}, "host": {"evil\x1b[31m\nhost"}}.Encode()))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	go func() {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	waitPolling(t, m, key)
	if info := m.agents.info()[key]; info.Host != "evil31mhost" {
		t.Fatalf("上报字段应去掉控制字符: %q", info.Host)
	}
	if code, _ := call(t, srv, "POST", "/devices/"+key+"/shutdown", map[string]string{"action": "format"}); code != http.StatusBadRequest {
		t.Fatalf("未知操作应返回 400: %d", code)
	}
	if code, _ := call(t, srv, "POST", "/devices/"+key+"/shutdown", map[string]string{"action": "reboot", "x": "1"}); code != http.StatusBadRequest {
		t.Fatalf("未知字段应返回 400: %d", code)
	}
	plain := addDevice(t, srv, map[string]any{"name": "nas", "macs": []string{"aa:bb:cc:dd:ee:02"}, "shutdownMode": "http", "shutdownHttp": map[string]any{"url": "http://127.0.0.1:1/x"}})
	if code, body := call(t, srv, "POST", "/devices/"+plain+"/shutdown", map[string]string{"action": "reboot"}); code != http.StatusBadRequest || !strings.Contains(body, "客户端") {
		t.Fatalf("HTTP 方式不支持重启: %d %s", code, body)
	}
	m.agents.drop(key)
}

// 脚本下载：免登录接口要求凭据，管理接口直接下载；未知文件名返回 404。
func TestAgentScripts(t *testing.T) {
	_, srv, _ := newAgentModule(t)
	_, tok := addAgentDevice(t, srv, "pc")
	resp, err := http.Get(srv.URL + "/api/wol/public/agent/gately-agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("没有凭据不能下载: %d", resp.StatusCode)
	}
	code, body := agentPost(t, srv, "gately-agent.sh", url.Values{"token": {tok}})
	if code != 200 || !strings.HasPrefix(body, "#!/bin/sh") {
		t.Fatalf("下载 sh 失败: %d %.40s", code, body)
	}
	code, body = agentPost(t, srv, "gately-agent.ps1", url.Values{"token": {tok}})
	if code != 200 || !strings.HasPrefix(body, "\ufeff# Gately") {
		t.Fatalf("ps1 应带 UTF-8 BOM: %d %q", code, body[:min(len(body), 20)])
	}
	if code, _ := agentPost(t, srv, "../wol.go", url.Values{"token": {tok}}); code != http.StatusNotFound {
		t.Fatalf("未知文件应返回 404: %d", code)
	}
	code, body = call(t, srv, "GET", "/agent-script/gately-agent.ps1", nil)
	if code != 200 || !strings.Contains(body, "Register-ScheduledTask") {
		t.Fatalf("管理接口下载失败: %d", code)
	}
}

// PublicAuth 只认有效凭据，读取请求体后处理函数仍能读到。
func TestAgentPublicAuth(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	_, tok := addAgentDevice(t, srv, "pc")
	body := pollForm(tok, "inst-0008aaaa").Encode()
	req := httptest.NewRequest("POST", "/api/wol/public/agent/poll", strings.NewReader(body))
	if !m.PublicAuth(req) {
		t.Fatal("有效凭据应通过")
	}
	rest, _ := io.ReadAll(req.Body)
	if string(rest) != body {
		t.Fatal("请求体应被还原")
	}
	if m.PublicAuth(httptest.NewRequest("POST", "/api/wol/public/agent/poll", strings.NewReader("token=abc"))) {
		t.Fatal("无效凭据不应通过")
	}
	if m.PublicAuth(httptest.NewRequest("GET", "/api/wol/public/agent/poll", nil)) {
		t.Fatal("没有凭据不应通过")
	}
}

// HTTP 关机方式：按设置发送方法、请求头与请求体；非 2xx 视为失败；日志不含查询参数中的凭据。
func TestHTTPShutdown(t *testing.T) {
	var mu sync.Mutex
	var got []string
	status := 200
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Authorization")+" "+r.Header.Get("Content-Type")+" "+string(b))
		code := status
		mu.Unlock()
		w.WriteHeader(code)
		_, _ = io.WriteString(w, "bye")
	}))
	defer backend.Close()
	_, srv, logs := newAgentModule(t)
	key := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownMode": "http",
		"shutdownHttp": map[string]any{"url": backend.URL + "/off?key=s3cret", "headers": []string{"Authorization: Bearer xyz"}, "body": `{"a":1}`}})
	code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil)
	if code != 200 || !strings.Contains(body, "bye") {
		t.Fatalf("关机请求失败: %d %s", code, body)
	}
	mu.Lock()
	first := got[0]
	status = 500
	mu.Unlock()
	if first != `POST /off?key=s3cret Bearer xyz application/json {"a":1}` {
		t.Fatalf("请求内容错误: %q", first)
	}
	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != http.StatusBadGateway || !strings.Contains(body, "500") {
		t.Fatalf("非 2xx 应返回 502: %d %s", code, body)
	}
	if text := fmt.Sprint(logs.Since(0)); strings.Contains(text, "s3cret") || !strings.Contains(text, "已发送关机请求") {
		t.Fatalf("日志错误: %s", text)
	}
	// 连接失败时 url.Error 带完整地址：日志与接口返回都不能含查询参数里的凭据
	dead := addDevice(t, srv, map[string]any{"name": "dead", "macs": []string{"aa:bb:cc:dd:ee:01"}, "shutdownMode": "http",
		"shutdownHttp": map[string]any{"url": "http://127.0.0.1:1/off?token=deadS3cret"}})
	code, body = call(t, srv, "POST", "/devices/"+dead+"/shutdown", nil)
	if code == 200 || strings.Contains(body, "deadS3cret") {
		t.Fatalf("连接失败应报错且不含凭据: %d %s", code, body)
	}
	if text := fmt.Sprint(logs.Since(0)); strings.Contains(text, "deadS3cret") || !strings.Contains(text, "关机请求失败") {
		t.Fatalf("连接失败的日志不应含凭据: %s", text)
	}
}

// 客户端断开连接（进程退出、断网、关机）后立即显示离线，换一台机器的客户端不必等宽限期即可接入。
func TestAgentDisconnectOffline(t *testing.T) {
	m, srv, _ := newAgentModule(t)
	agentOnlineGrace = time.Minute
	key, tok := addAgentDevice(t, srv, "pc")
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/wol/public/agent/poll", strings.NewReader(pollForm(tok, "inst-gone0000").Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	done := make(chan struct{})
	go func() {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
		close(done)
	}()
	waitPolling(t, m, key)
	cancel()
	<-done
	waitFor(t, "断开后离线", func() bool { return !m.agents.online(key) })
	go agentPostAsync(srv, "poll", pollForm(tok, "inst-new00000"))
	waitPolling(t, m, key)
	m.agents.mu.Lock()
	inst := m.agents.conns[key].instance
	m.agents.mu.Unlock()
	if inst != "inst-new00000" {
		t.Fatalf("新客户端应立即接入: %s", inst)
	}
	m.agents.drop(key)
}
