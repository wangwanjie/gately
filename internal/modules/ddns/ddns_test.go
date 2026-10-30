package ddns

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/webhook"
)

// fakeProvider 记录每次 Upsert 调用，并模拟服务商侧状态。
type fakeProvider struct {
	mu    sync.Mutex
	calls []dnsprovider.Record
	state map[string]string
	fail  error
}

func (f *fakeProvider) Upsert(_ context.Context, r dnsprovider.Record) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r)
	if f.fail != nil {
		return "", f.fail
	}
	k := r.Type + " " + r.FQDN()
	cur, ok := f.state[k]
	f.state[k] = r.Value
	switch {
	case !ok:
		return dnsprovider.ActionCreated, nil
	case cur == r.Value:
		return dnsprovider.ActionUnchanged, nil
	default:
		return dnsprovider.ActionUpdated, nil
	}
}

func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeIPs struct {
	mu     sync.Mutex
	v4, v6 string
	err    error
}

func (f *fakeIPs) get(_ context.Context, _ ipsource.Source, fam ipsource.Family) (netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return netip.Addr{}, f.err
	}
	if fam == ipsource.IPv6 {
		return netip.ParseAddr(f.v6)
	}
	return netip.ParseAddr(f.v4)
}

func (f *fakeIPs) set(v4, v6 string) {
	f.mu.Lock()
	f.v4, f.v6 = v4, v6
	f.mu.Unlock()
}

func setupFakes(t *testing.T) (*fakeProvider, *fakeIPs) {
	t.Helper()
	fp := &fakeProvider{state: map[string]string{}}
	ips := &fakeIPs{v4: "203.0.113.1", v6: "2001:db8::1"}
	oldP, oldG := providerFactory, ipGetter
	providerFactory = func(string, dnsprovider.Credentials, *http.Client) (dnsprovider.Provider, error) { return fp, nil }
	ipGetter = ips.get
	t.Cleanup(func() { providerFactory, ipGetter = oldP, oldG })
	return fp, ips
}

func sampleTask() Task {
	src := ipsource.Source{Enable: true, Type: ipsource.TypeURL, URLs: []string{"https://example.invalid"}}
	return Task{
		Key: "t1", Name: "home", Enable: true, Provider: "alidns",
		Credentials: dnsprovider.Credentials{ID: "ak", Secret: "sk"},
		IPv4:        src, IPv6: src,
		Records: []Record{
			{Key: "a", Enable: true, Zone: "example.com", Sub: "@", Type: "A", Value: VarIPv4},
			{Key: "b", Enable: true, Zone: "example.com", Sub: "nas", Type: "AAAA"},
			{Key: "c", Enable: false, Zone: "example.com", Sub: "off", Type: "A"},
		},
		Interval: 60, ForceInterval: 3600,
	}
}

func testLogger() *logbuf.Logger {
	return logbuf.New(200, log.New(io.Discard, "", 0)).Logger("t")
}

func TestValidate(t *testing.T) {
	task := sampleTask()
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	if task.Records[1].Value != VarIPv6 {
		t.Fatalf("AAAA 记录默认值应为 {ipv6Addr}，实际 %q", task.Records[1].Value)
	}
	bad := []func(*Task){
		func(t *Task) { t.Provider = "nope" },
		func(t *Task) { t.Interval = 5 },
		func(t *Task) { t.Records = nil },
		func(t *Task) { t.Records[0].Zone = "bad zone" },
		func(t *Task) { t.Records[0].Sub = "a b" },
		func(t *Task) { t.Records[0].Type = "MX" },
		func(t *Task) { t.Records[0].Value = "2001:db8::1" },
		func(t *Task) { t.IPv4.Enable = false },
		func(t *Task) { t.Records = append(t.Records, t.Records[0]) },
	}
	for i, mut := range bad {
		task := sampleTask()
		mut(&task)
		if err := task.Validate(); err == nil {
			t.Errorf("第 %d 个非法用例未报错", i)
		}
	}
}

// 分组名称去掉首尾空白，超过 64 个字符时拒绝保存。
func TestValidateGroup(t *testing.T) {
	task := sampleTask()
	task.Group = "  阿里云  "
	if err := task.Validate(); err != nil || task.Group != "阿里云" {
		t.Fatalf("分组应去掉首尾空白: %q %v", task.Group, err)
	}
	task.Group = strings.Repeat("组", crud.MaxGroupLen+1)
	if task.Validate() == nil {
		t.Fatal("超长分组名应被拒绝")
	}
}

func TestSyncFlow(t *testing.T) {
	fp, ips := setupFakes(t)
	var hookBodies []string
	var hookMu sync.Mutex
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hookMu.Lock()
		hookBodies = append(hookBodies, string(b))
		hookMu.Unlock()
	}))
	defer hook.Close()

	task := sampleTask()
	task.Webhook = webhook.Config{Enable: true, URL: hook.URL, Method: "POST", Body: `#{ipAddr}|{ipv6Addr}|#{successDomains}`}
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	r := newRunner(task, testLogger(), nil, nil)
	ctx := context.Background()

	// 首次：两条启用记录都被创建，禁用记录不处理
	r.syncOnce(ctx, false)
	if fp.callCount() != 2 {
		t.Fatalf("首次应调用 2 次，实际 %d", fp.callCount())
	}
	st := r.snapshot()
	if st.IPv4 != "203.0.113.1" || st.Records["a"].LastAction != dnsprovider.ActionCreated {
		t.Fatalf("状态错误: %+v", st)
	}
	hookMu.Lock()
	if len(hookBodies) != 1 || hookBodies[0] != "203.0.113.1|2001:db8::1|example.com,nas.example.com" {
		t.Fatalf("Webhook 内容错误: %v", hookBodies)
	}
	hookMu.Unlock()

	// IP 未变：不调用服务商，不触发 Webhook
	r.syncOnce(ctx, false)
	if fp.callCount() != 2 {
		t.Fatalf("IP 未变不应调用服务商，实际 %d", fp.callCount())
	}

	// IPv4 变化：只更新 A 记录
	ips.set("203.0.113.9", "2001:db8::1")
	r.syncOnce(ctx, false)
	if fp.callCount() != 3 || fp.calls[2].Type != "A" || fp.calls[2].Value != "203.0.113.9" {
		t.Fatalf("应只更新 A 记录: %+v", fp.calls)
	}

	// 强制同步：全部调用一次
	r.syncOnce(ctx, true)
	if fp.callCount() != 5 {
		t.Fatalf("强制同步应调用全部记录，实际 %d", fp.callCount())
	}

	// 超过强制周期：再次调用
	r.mu.Lock()
	for k, s := range r.state.Records {
		s.LastSync = time.Now().Add(-2 * time.Hour)
		r.state.Records[k] = s
	}
	r.mu.Unlock()
	r.syncOnce(ctx, false)
	if fp.callCount() != 7 {
		t.Fatalf("超过强制周期应重新同步，实际 %d", fp.callCount())
	}
}

func TestSyncErrors(t *testing.T) {
	fp, ips := setupFakes(t)
	task := sampleTask()
	_ = task.Validate()
	r := newRunner(task, testLogger(), nil, nil)

	// IP 获取失败：不调用服务商，记录错误
	ips.mu.Lock()
	ips.err = errors.New("network down")
	ips.mu.Unlock()
	r.syncOnce(context.Background(), false)
	if fp.callCount() != 0 || !strings.Contains(r.snapshot().IPError, "network down") {
		t.Fatalf("IP 失败处理错误: calls=%d err=%q", fp.callCount(), r.snapshot().IPError)
	}

	// 服务商失败：下一轮应重试
	ips.mu.Lock()
	ips.err = nil
	ips.mu.Unlock()
	fp.fail = errors.New("quota")
	r.syncOnce(context.Background(), false)
	if r.snapshot().Records["a"].LastError == "" {
		t.Fatal("应记录服务商错误")
	}
	fp.fail = nil
	before := fp.callCount()
	r.syncOnce(context.Background(), false)
	if fp.callCount() != before+2 || r.snapshot().Records["a"].LastError != "" {
		t.Fatal("失败后应在下一轮重试并清除错误")
	}
}

func TestRunnerLoopAndManualSync(t *testing.T) {
	fp, _ := setupFakes(t)
	task := sampleTask()
	_ = task.Validate()
	r := newRunner(task, testLogger(), nil, nil)
	r.start(context.Background(), time.Hour)
	defer r.stop()
	r.requestSync(false)
	deadline := time.Now().Add(3 * time.Second)
	for fp.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if fp.callCount() != 2 {
		t.Fatalf("手动触发未执行同步: %d", fp.callCount())
	}
}

func newTestModule(t *testing.T) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

type testRouter struct{ mux *http.ServeMux }

func (tr testRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	tr.mux.HandleFunc(method+" /api/ddns"+path, h)
}

func TestAPISecretMasking(t *testing.T) {
	setupFakes(t)
	m := newTestModule(t)
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	task := sampleTask()
	task.Key = ""
	task.Enable = false
	body, _ := json.Marshal(task)
	resp, err := http.Post(srv.URL+"/api/ddns/tasks", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	var created TaskView
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if resp.StatusCode != 200 || created.Credentials.Secret != crud.SecretMask {
		t.Fatalf("创建响应应脱敏: %d %+v", resp.StatusCode, created.Credentials)
	}

	// 原样提交脱敏值，服务端应保留真实密钥
	created.Name = "renamed"
	body, _ = json.Marshal(created.Task)
	req, _ := http.NewRequest("PUT", srv.URL+"/api/ddns/tasks/"+created.Key, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("更新失败 %d", resp.StatusCode)
	}
	m.mu.Lock()
	got := m.cfg.Tasks[0]
	m.mu.Unlock()
	if got.Credentials.Secret != "sk" || got.Name != "renamed" {
		t.Fatalf("密钥应被保留: %+v", got.Credentials)
	}

	// 查看密钥明文接口返回真实密钥，任务不存在时返回 404
	resp, err = http.Get(srv.URL + "/api/ddns/tasks/" + created.Key + "/secrets")
	if err != nil {
		t.Fatal(err)
	}
	var secrets map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&secrets)
	resp.Body.Close()
	if resp.StatusCode != 200 || secrets["secret"] != "sk" {
		t.Fatalf("应返回密钥明文: %d %+v", resp.StatusCode, secrets)
	}
	resp, err = http.Get(srv.URL + "/api/ddns/tasks/missing/secrets")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("任务不存在应返回 404，实际 %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/ddns/providers")
	if err != nil {
		t.Fatal(err)
	}
	var metas []dnsprovider.Meta
	_ = json.NewDecoder(resp.Body).Decode(&metas)
	resp.Body.Close()
	if len(metas) < 10 {
		t.Fatalf("服务商列表异常: %d", len(metas))
	}
}

func TestConvertLucky(t *testing.T) {
	raw := json.RawMessage(`[
	 {"TaskName":"v4","Enable":true,"TaskType":"IPv4","V4QueryIPEnable":true,"V4QueryIPType":"url",
	  "V4QueryUrl":"http://v4.666666.host:66/ip\nhttps://4.ipw.cn","Intervals":36,"HttpClientTimeout":15,
	  "Records":[{"Key":"k1","Disable":false,"SyncRecordData":{"DomainName":"example.com","SubDomainName":"www","ipv4Address":"{ipv4Addr}","type":"A","ttl":0,"line":""}}],
	  "DNS":{"Name":"alidns","ID":"ak","Secret":"sk","ForceInterval":3600},
	  "WebhookEnable":true,"WebhookURL":"https://hook.example/x?ip=#{ipAddr}","WebhookMethod":"get","WebhookSuccessContent":["ok"]},
	 {"TaskName":"old","Enable":true,"TaskType":"IPv6","V6QueryIPEnable":true,"V6QueryIPType":"netInterface","V6NetInterface":"eth0","V6NetInterfaceIPReg":"@1",
	  "Domains":["nas.example.com.cn","example.org"],"DNS":{"Name":"cloudflare","Secret":"t"},"Intervals":300},
	 {"TaskName":"unsupported","DNS":{"Name":"west"}}
	]`)
	tasks, warns, found, err := convertLuckyTasks(raw)
	if err != nil {
		t.Fatal(err)
	}
	if found != 3 || len(tasks) != 2 {
		t.Fatalf("found=%d imported=%d warns=%v", found, len(tasks), warns)
	}
	a := tasks[0]
	if !a.Enable || a.Interval != 36 || len(a.IPv4.URLs) != 1 || a.IPv4.URLs[0] != "https://4.ipw.cn" {
		t.Fatalf("任务 1 映射错误: %+v", a)
	}
	if a.Records[0].Sub != "www" || a.Records[0].Value != VarIPv4 || !a.Webhook.Enable || a.Webhook.SuccessContent[0] != "ok" {
		t.Fatalf("记录或 Webhook 映射错误: %+v %+v", a.Records, a.Webhook)
	}
	b := tasks[1]
	if len(b.Records) != 2 || b.Records[0].Zone != "example.com.cn" || b.Records[0].Sub != "nas" || b.Records[1].Sub != "@" || b.Records[1].Type != "AAAA" {
		t.Fatalf("旧版 Domains 拆分错误: %+v", b.Records)
	}
	if b.IPv6.Type != ipsource.TypeInterface || b.IPv6.Regex != "" {
		t.Fatalf("网卡来源映射错误: %+v", b.IPv6)
	}
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"Lucky 官方服务器", "按序号", `"west"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少告警 %q:\n%s", want, joined)
		}
	}
}

// TestImportRealSample 用真实 lucky_ddns.lkcf 做导入预演，仅在设置 GATELY_LUCKY_DDNS_SAMPLE 时运行。
func TestImportRealSample(t *testing.T) {
	path := os.Getenv("GATELY_LUCKY_DDNS_SAMPLE")
	if path == "" {
		t.Skip("未设置 GATELY_LUCKY_DDNS_SAMPLE")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := luckyimport.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModule(t)
	rep, err := m.ImportLucky(doc, luckyimport.Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found=%d imported=%d", rep.Found, rep.Imported)
	for _, w := range rep.Warnings {
		t.Log("warning:", w)
	}
	if rep.Imported != rep.Found {
		t.Fatal("有任务未能导入")
	}
}

// 修改任务时，改了域名的记录不能沿用旧状态，否则会被当作已同步而不创建新解析
func TestRenamedRecordResyncs(t *testing.T) {
	fp, _ := setupFakes(t)
	task := sampleTask()
	_ = task.Validate()
	r := newRunner(task, testLogger(), nil, nil)
	r.syncOnce(context.Background(), false)
	prev := r.snapshot()
	before := fp.callCount()

	renamed := sampleTask()
	_ = renamed.Validate()
	renamed.Records[1].Sub = "gately"
	r2 := newRunner(renamed, testLogger(), &prev, &task)
	if _, ok := r2.snapshot().Records["b"]; ok {
		t.Fatal("改名记录不应保留旧状态")
	}
	if _, ok := r2.snapshot().Records["a"]; !ok {
		t.Fatal("未改动的记录应保留状态")
	}
	r2.syncOnce(context.Background(), false)
	if fp.callCount() != before+1 {
		t.Fatalf("只应重新同步改名的记录，实际调用 %d 次", fp.callCount()-before)
	}
}

// 记录值与 Webhook 可以引用 {STUN_规则名_PORT} 等变量（#47）。
func TestSTUNVars(t *testing.T) {
	fp, _ := setupFakes(t)
	var hookBodies []string
	var hookMu sync.Mutex
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hookMu.Lock()
		hookBodies = append(hookBodies, string(b))
		hookMu.Unlock()
	}))
	defer hook.Close()

	task := sampleTask()
	task.IPv6.Enable = false
	task.Records = []Record{
		{Key: "txt", Enable: true, Zone: "example.com", Sub: "_ssh", Type: "TXT", Value: "port={STUN_ssh_PORT} addr={STUN_ssh_ADDR}"},
		{Key: "a", Enable: true, Zone: "example.com", Sub: "stun", Type: "A", Value: "{STUN_ssh_IP}"},
	}
	task.Webhook = webhook.Config{Enable: true, URL: hook.URL, Method: "POST", Body: `{STUN_ssh_PORT}|{successDomains}`}
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	r := newRunner(task, testLogger(), nil, nil)
	ctx := context.Background()

	// 规则还没有公网地址：记录跳过并给出原因，不调用服务商
	r.syncOnce(ctx, false)
	if fp.callCount() != 0 {
		t.Fatalf("规则无地址时不应调用服务商，实际 %d", fp.callCount())
	}
	if e := r.snapshot().Records["txt"].LastError; !strings.Contains(e, "ssh") {
		t.Fatalf("应提示 STUN 规则暂无公网地址: %q", e)
	}

	stunaddr.Set("k-ssh", "ssh", netip.MustParseAddrPort("203.0.113.7:40001"))
	defer stunaddr.Delete("k-ssh")
	r.syncOnce(ctx, false)
	fp.mu.Lock()
	txt, a := fp.state["TXT _ssh.example.com"], fp.state["A stun.example.com"]
	fp.mu.Unlock()
	if txt != "port=40001 addr=203.0.113.7:40001" || a != "203.0.113.7" {
		t.Fatalf("记录值渲染错误: txt=%q a=%q", txt, a)
	}
	hookMu.Lock()
	if len(hookBodies) != 1 || !strings.HasPrefix(hookBodies[0], "40001|") {
		t.Fatalf("Webhook 内容错误: %v", hookBodies)
	}
	hookMu.Unlock()

	// 端口变化后，模块收到 STUN 地址变化事件立即同步引用它的任务
	m := newTestModule(t)
	m.cfg.Tasks = []Task{task}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	before := fp.callCount()
	stunaddr.Set("k-ssh", "ssh", netip.MustParseAddrPort("203.0.113.7:40002"))
	m.onEvent(events.Event{Type: events.STUNChanged, Source: "other"})
	m.onEvent(events.Event{Type: events.STUNChanged, Source: "ssh"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fp.mu.Lock()
		v := fp.state["TXT _ssh.example.com"]
		fp.mu.Unlock()
		if v == "port=40002 addr=203.0.113.7:40002" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("STUN 地址变化后未立即同步，服务商调用 %d -> %d", before, fp.callCount())
}
