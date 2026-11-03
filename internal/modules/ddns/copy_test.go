package ddns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/crud"
)

// 复制 DDNS 任务：凭据密钥为占位符时沿用模板，副本排在模板之后；记录 key 原样保留，
// 新插入的记录（不带 key）分配不冲突的 key；模板本身不变，响应中的密钥脱敏。
func TestCopyTask(t *testing.T) {
	setupFakes(t)
	m := newTestModule(t)
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tpl := sampleTask()
	tpl.Enable = false
	other := sampleTask()
	other.Key, other.Name, other.Enable = "t2", "office", false
	m.mu.Lock()
	m.cfg.Tasks = []Task{tpl, other}
	m.mu.Unlock()

	draft := tpl
	draft.Key, draft.Name = "", "home 副本"
	draft.Credentials.Secret = crud.SecretMask
	draft.Records = []Record{tpl.Records[0], {Enable: true, Zone: "example.com", Sub: "new", Type: "A", Value: VarIPv4}, tpl.Records[1], tpl.Records[2]}
	data, _ := json.Marshal(draft)
	resp, err := http.Post(srv.URL+"/api/ddns/tasks/t1/copy", "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	var view TaskView
	_ = json.NewDecoder(resp.Body).Decode(&view)
	resp.Body.Close()
	if resp.StatusCode != 200 || view.Credentials.Secret != crud.SecretMask {
		t.Fatalf("复制失败或响应未脱敏: %d %+v", resp.StatusCode, view.Credentials)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Tasks) != 3 || m.cfg.Tasks[1].Key != view.Key || m.cfg.Tasks[2].Key != "t2" {
		t.Fatalf("副本应排在模板之后: %+v", m.cfg.Tasks)
	}
	cp := m.cfg.Tasks[1]
	if cp.Credentials.Secret != "sk" {
		t.Fatalf("占位符应沿用模板密钥: %q", cp.Credentials.Secret)
	}
	keys := []string{}
	for _, r := range cp.Records {
		keys = append(keys, r.Key)
	}
	if got := strings.Join(keys, ","); got != "a,r2,b,c" {
		t.Fatalf("已有记录 key 应保持不变、新记录分配新 key: %s", got)
	}
	if m.cfg.Tasks[0].Credentials.Secret != "sk" || len(m.cfg.Tasks[0].Records) != 3 {
		t.Fatalf("模板不应被修改: %+v", m.cfg.Tasks[0])
	}
}

// 复制时清空密钥或填写新密钥按提交的值保存；校验失败（记录重复）时不新增。
func TestCopyTaskSecretAndValidation(t *testing.T) {
	setupFakes(t)
	m := newTestModule(t)
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	tpl := sampleTask()
	tpl.Enable = false
	m.mu.Lock()
	m.cfg.Tasks = []Task{tpl}
	m.mu.Unlock()

	post := func(d Task) int {
		data, _ := json.Marshal(d)
		resp, err := http.Post(srv.URL+"/api/ddns/tasks/t1/copy", "application/json", strings.NewReader(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	draft := tpl
	draft.Key = ""
	draft.Credentials.Secret = "new-sk"
	if code := post(draft); code != 200 {
		t.Fatalf("复制失败: %d", code)
	}
	dup := tpl
	dup.Key = ""
	dup.Credentials.Secret = crud.SecretMask
	dup.Records = append(dup.Records, tpl.Records[0])
	dup.Records[3].Key = ""
	if code := post(dup); code != 400 {
		t.Fatalf("记录重复应拒绝: %d", code)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Tasks) != 2 || m.cfg.Tasks[1].Credentials.Secret != "new-sk" {
		t.Fatalf("新密钥应原样保存、被拒绝的复制不应新增: %+v", m.cfg.Tasks)
	}
}
