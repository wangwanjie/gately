package ddns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 调整记录顺序与分组只改配置，不重启任务（issue #48）
func TestRecordsLayout(t *testing.T) {
	setupFakes(t)
	m := newTestModule(t)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	task := sampleTask()
	task.Key = ""
	body, _ := json.Marshal(task)
	resp, err := http.Post(srv.URL+"/api/ddns/tasks", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	var created TaskView
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("创建任务失败 %d", resp.StatusCode)
	}
	m.mu.Lock()
	runBefore := m.runners[created.Key]
	m.mu.Unlock()
	if runBefore == nil {
		t.Fatal("任务应已启动")
	}

	put := func(payload string) int {
		req, _ := http.NewRequest("PUT", srv.URL+"/api/ddns/tasks/"+created.Key+"/records-layout", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := put(`{"records":[{"key":"c","group":" 内网 "},{"key":"a","group":"公网"},{"key":"b","group":"内网"}]}`); code != 200 {
		t.Fatalf("调整记录布局失败 %d", code)
	}
	m.mu.Lock()
	got := m.cfg.Tasks[0].Records
	runAfter := m.runners[created.Key]
	m.mu.Unlock()
	order := got[0].Key + got[1].Key + got[2].Key
	if order != "cab" || got[0].Group != "内网" || got[1].Group != "公网" || got[2].Group != "内网" {
		t.Fatalf("记录顺序或分组有误: %+v", got)
	}
	if got[0].Sub != "off" || got[1].Value != VarIPv4 {
		t.Fatalf("记录其他字段应保持不变: %+v", got)
	}
	if runAfter != runBefore {
		t.Fatal("调整记录布局不应重启任务")
	}

	// 缺少、重复、未知的记录与过长的分组名称都被拒绝，配置不变
	for _, bad := range []string{
		`{"records":[{"key":"a"},{"key":"b"}]}`,
		`{"records":[{"key":"a"},{"key":"a"},{"key":"b"}]}`,
		`{"records":[{"key":"a"},{"key":"b"},{"key":"x"}]}`,
		`{"records":[{"key":"a","group":"` + strings.Repeat("组", 65) + `"},{"key":"b"},{"key":"c"}]}`,
	} {
		if code := put(bad); code != 400 {
			t.Fatalf("非法请求应返回 400，实际 %d: %s", code, bad)
		}
	}
	m.mu.Lock()
	order = m.cfg.Tasks[0].Records[0].Key + m.cfg.Tasks[0].Records[1].Key + m.cfg.Tasks[0].Records[2].Key
	m.mu.Unlock()
	if order != "cab" {
		t.Fatalf("被拒绝的请求不应修改配置: %s", order)
	}

	req, _ := http.NewRequest("PUT", srv.URL+"/api/ddns/tasks/missing/records-layout", strings.NewReader(`{"records":[]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("任务不存在应返回 404，实际 %d", resp.StatusCode)
	}

	// 记录分组随任务保存校验：首尾空白被去掉，过长被拒绝
	tk := sampleTask()
	tk.Records[0].Group = "  家里  "
	if err := tk.Validate(); err != nil || tk.Records[0].Group != "家里" {
		t.Fatalf("记录分组应去掉首尾空白: %v %q", err, tk.Records[0].Group)
	}
	tk.Records[0].Group = strings.Repeat("组", 65)
	if err := tk.Validate(); err == nil {
		t.Fatal("过长的记录分组应被拒绝")
	}
}
