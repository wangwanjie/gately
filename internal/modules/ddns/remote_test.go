package ddns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 同步时把备注写给服务商：记录备注优先，留空用任务备注，超长时截断。
func TestRemarkSentToProvider(t *testing.T) {
	fp, _ := setupFakes(t)
	task := sampleTask()
	task.Remark = "家庭宽带"
	task.Records[0].Remark = "根域名"
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	r := newRunner(task, testLogger(), nil, nil)
	r.syncOnce(context.Background(), false)
	got := map[string]string{}
	for _, c := range fp.calls {
		got[c.FQDN()] = c.Remark
	}
	if got["example.com"] != "根域名" || got["nas.example.com"] != "家庭宽带" {
		t.Fatalf("备注错误: %v", got)
	}

	long := Task{Remark: strings.Repeat("长", maxRemarkLen+20)}
	if n := len([]rune(long.remarkOf(Record{}))); n != maxRemarkLen {
		t.Fatalf("超长备注应截断为 %d 个字符，实际 %d", maxRemarkLen, n)
	}
}

// 修改备注（含作为默认值的任务备注）后，相关记录重新同步，备注才会写到服务商处。
func TestRemarkChangeResyncs(t *testing.T) {
	fp, _ := setupFakes(t)
	task := sampleTask()
	task.Records[0].Remark = "固定"
	_ = task.Validate()
	r := newRunner(task, testLogger(), nil, nil)
	r.syncOnce(context.Background(), false)
	prev := r.snapshot()
	before := fp.callCount()

	changed := task
	changed.Records = append([]Record{}, task.Records...)
	changed.Remark = "新的任务备注"
	r2 := newRunner(changed, testLogger(), &prev, &task)
	if _, ok := r2.snapshot().Records["a"]; !ok {
		t.Fatal("有自己备注的记录不受任务备注影响，应保留状态")
	}
	if _, ok := r2.snapshot().Records["b"]; ok {
		t.Fatal("使用任务备注的记录应重新同步")
	}
	r2.syncOnce(context.Background(), false)
	if n := fp.callCount() - before; n != 1 {
		t.Fatalf("只应重新同步记录 b，实际 %d 次", n)
	}
	if last := fp.calls[len(fp.calls)-1]; last.Remark != "新的任务备注" {
		t.Fatalf("应带上新备注: %+v", last)
	}
}

// fakeDeleter 替换服务商删除，记录调用。
type fakeDeleter struct {
	mu    sync.Mutex
	calls []dnsprovider.Record
	fail  map[string]error
}

func (f *fakeDeleter) del(_ context.Context, _ dnsprovider.Provider, r dnsprovider.Record) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r)
	if err := f.fail[r.FQDN()]; err != nil {
		return 0, err
	}
	return 1, nil
}

func (f *fakeDeleter) domains() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.Type+" "+c.FQDN()+"="+c.Value)
	}
	return out
}

func TestAPIRemoteDelete(t *testing.T) {
	fp, _ := setupFakes(t)
	fd := &fakeDeleter{fail: map[string]error{}}
	oldDel, oldStart, oldResume := remoteDelete, startupDelay, resumeDelay
	remoteDelete, startupDelay, resumeDelay = fd.del, 0, time.Hour
	t.Cleanup(func() { remoteDelete, startupDelay, resumeDelay = oldDel, oldStart, oldResume })

	m := newTestModule(t)
	task := sampleTask()
	task.Records[2].Value = "198.51.100.7"
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Tasks = []Task{task}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	deadline := time.Now().Add(3 * time.Second)
	for fp.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fp.callCount() < 2 {
		t.Fatal("任务未完成首次同步")
	}

	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	post := func(key string, records []string) (int, string) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"records": records})
		resp, err := http.Post(srv.URL+"/api/ddns/tasks/"+key+"/remote-delete", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}

	// 删除一条：值取最近一次同步结果；成功后清掉该记录的同步状态，任务暂停到 resumeDelay 之后
	m.mu.Lock()
	runBefore := m.runners["t1"]
	m.mu.Unlock()
	calls := fp.callCount()
	if code, body := post("t1", []string{"a"}); code != 200 || !strings.Contains(body, `"deleted":1`) {
		t.Fatalf("删除失败 %d %s", code, body)
	}
	if got := fd.domains(); strings.Join(got, ",") != "A example.com=203.0.113.1" {
		t.Fatalf("删除调用错误: %v", got)
	}
	m.mu.Lock()
	run := m.runners["t1"]
	m.mu.Unlock()
	if run == nil || run == runBefore {
		t.Fatal("删除后任务应以新的运行实例恢复")
	}
	st := run.snapshot()
	if _, ok := st.Records["a"]; ok {
		t.Fatal("已删除记录的同步状态应清空")
	}
	if _, ok := st.Records["b"]; !ok {
		t.Fatal("其他记录的同步状态应保留")
	}
	time.Sleep(50 * time.Millisecond)
	if fp.callCount() != calls {
		t.Fatal("恢复等待期间任务不应同步，否则会把刚删除的记录建回来")
	}

	// 留空表示全部记录：停用记录也删除，固定值记录用配置中的值
	fd.calls = nil
	if code, body := post("t1", nil); code != 200 {
		t.Fatalf("删除全部失败 %d %s", code, body)
	}
	if got := strings.Join(fd.domains(), ","); got != "A example.com=,AAAA nas.example.com=2001:db8::1,A off.example.com=198.51.100.7" {
		t.Fatalf("删除全部调用错误: %s", got)
	}

	// 任一条失败返回 502 与原因
	fd.fail["nas.example.com"] = errors.New("服务商拒绝")
	if code, body := post("t1", nil); code != http.StatusBadGateway || !strings.Contains(body, "服务商拒绝") {
		t.Fatalf("失败时应返回 502 与原因: %d %s", code, body)
	}
	if code, _ := post("t1", []string{"zz"}); code != 400 {
		t.Fatalf("记录不存在应返回 400，实际 %d", code)
	}
	if code, _ := post("missing", nil); code != 404 {
		t.Fatalf("任务不存在应返回 404，实际 %d", code)
	}

	// 带上待保存的任务配置时先校验：校验不通过不删除；密钥为占位符时沿用已保存的值
	delete(fd.fail, "nas.example.com")
	postTask := func(tk Task) (int, string) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"records": []string{"b"}, "task": tk})
		resp, err := http.Post(srv.URL+"/api/ddns/tasks/t1/remote-delete", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}
	bad := sampleTask()
	bad.Provider = "no-such-provider"
	fd.calls = nil
	if code, body := postTask(bad); code != 400 {
		t.Fatalf("待保存配置无效时应返回 400: %d %s", code, body)
	}
	if len(fd.domains()) != 0 {
		t.Fatalf("校验不通过时不应删除服务商处的解析: %v", fd.domains())
	}
	good := sampleTask()
	good.Credentials.Secret = crud.SecretMask
	if code, body := postTask(good); code != 200 {
		t.Fatalf("待保存配置有效时应删除: %d %s", code, body)
	}
	if got := strings.Join(fd.domains(), ","); !strings.Contains(got, "nas.example.com") {
		t.Fatalf("应删除记录 b: %s", got)
	}

	// 不支持删除的服务商
	m.mu.Lock()
	m.cfg.Tasks[0].Provider = "duckdns"
	m.mu.Unlock()
	if code, body := post("t1", nil); code != 400 || !strings.Contains(body, "不支持删除") {
		t.Fatalf("不支持删除时应返回 400: %d %s", code, body)
	}
}

// 只改删除默认值时保存设置，不重启任务。
func TestSettingsDeleteRemote(t *testing.T) {
	setupFakes(t)
	oldStart := startupDelay
	startupDelay = time.Hour
	t.Cleanup(func() { startupDelay = oldStart })
	m := newTestModule(t)
	task := sampleTask()
	_ = task.Validate()
	m.cfg.Tasks = []Task{task}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = m.Start(ctx)
	defer m.Stop()
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m.mu.Lock()
	before := m.runners["t1"]
	m.mu.Unlock()
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/ddns/settings", strings.NewReader(`{"enable":true,"deleteRemote":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("保存设置失败: %v %d %s", err, resp.StatusCode, b)
	}
	resp.Body.Close()
	m.mu.Lock()
	after, saved := m.runners["t1"], m.cfg.Settings
	m.mu.Unlock()
	if after != before {
		t.Fatal("只改删除默认值不应重启任务")
	}
	if !saved.DeleteRemote {
		t.Fatal("删除默认值未保存")
	}
	m2 := New()
	_ = m2.Init(module.Env{Store: m.store, Logs: logbuf.New(10, log.New(io.Discard, "", 0))})
	if !m2.cfg.Settings.DeleteRemote || !m2.cfg.Settings.Enable {
		t.Fatalf("设置应已落盘: %+v", m2.cfg.Settings)
	}
}
