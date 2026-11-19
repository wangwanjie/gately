package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// doConfirmJSON 与 doJSON 相同，但带上管理界面的确认请求头。
func doConfirmJSON(t *testing.T, c *http.Client, method, url string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(confirmHeader, "1")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// confirmViaSocket 经 WebSocket 确认，返回结果状态。
func confirmViaSocket(t *testing.T, c *http.Client, srvURL string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srvURL, "http")+"/api/system/confirm/connect", &websocket.DialOptions{HTTPClient: c})
	if err != nil {
		t.Fatalf("建立确认连接失败: %v", err)
	}
	defer ws.CloseNow()
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("读取确认结果失败: %v", err)
	}
	var msg struct{ Status string }
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("确认结果格式错误: %s", data)
	}
	return msg.Status
}

func confirmStatus(t *testing.T, c *http.Client, srvURL string) confirmView {
	t.Helper()
	code, body := doJSON(t, c, "GET", srvURL+"/api/system/confirm", nil)
	if code != 200 {
		t.Fatalf("读取确认状态失败 %d %s", code, body)
	}
	var v confirmView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func testRule(t *testing.T, name string) map[string]any {
	return map[string]any{
		"name": name, "enable": false, "protocols": []string{"tcp4"},
		"listenAddress": "127.0.0.1", "listenPorts": strconv.Itoa(freeTCPPort(t)),
		"targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80",
	}
}

// 带确认请求头保存端口转发后登记待确认变更，经 WebSocket 确认后清除；不带请求头或保存失败不登记
func TestConfirmSaveAndConfirm(t *testing.T) {
	a, srv, c := loginApp(t)
	statePath := filepath.Join(a.store.Dir(), confirmStateName)

	if code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "plain")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("不带确认请求头的保存不应登记待确认变更")
	}
	// 校验失败的保存没有改动配置文件
	if code, _ := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", map[string]any{"name": "bad"}); code == 200 {
		t.Fatal("无效规则应保存失败")
	}
	if v := confirmStatus(t, c, srv.URL); v.Pending != nil || !v.Enabled || v.Timeout != defaultConfirmTimeout {
		t.Fatalf("保存失败不应登记待确认变更: %+v", v)
	}

	before, _ := a.store.ReadRaw("portforward")
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "rule-a")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "rule-b")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	v := confirmStatus(t, c, srv.URL)
	if v.Pending == nil || len(v.Pending.Configs) != 1 || v.Pending.Configs[0] != "portforward" || v.Pending.Remaining <= 0 {
		t.Fatalf("应登记端口转发的待确认变更: %+v", v.Pending)
	}
	p, err := loadPendingConfirm(a.store.Dir())
	if err != nil || p == nil || p.Files["portforward"] == nil || *p.Files["portforward"] != string(before) {
		t.Fatalf("两次保存应合并，并保留第一次保存前的内容: %+v %v", p, err)
	}

	if got := confirmViaSocket(t, c, srv.URL); got != confirmConfirmed {
		t.Fatalf("确认结果应为 confirmed，实际 %s", got)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("确认后应删除待确认状态")
	}
	if got := confirmViaSocket(t, c, srv.URL); got != confirmNone {
		t.Fatalf("没有待确认变更时应返回 none，实际 %s", got)
	}
	// 普通请求不能确认：确认接口只接受 WebSocket
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/system/confirm/connect", nil); code == 200 {
		t.Fatal("普通 GET 不应完成确认")
	}
}

// 关闭确认后不登记；确认接口不允许令牌访问
func TestConfirmDisabledAndTokens(t *testing.T) {
	a, srv, c := loginApp(t)
	s := a.settingsSnapshot()
	s.ConfirmTimeout = -5
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if got := a.settingsSnapshot().ConfirmTimeout; got != -1 {
		t.Fatalf("负数应规范为 -1，实际 %d", got)
	}
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "off")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if v := confirmStatus(t, c, srv.URL); v.Enabled || v.Pending != nil {
		t.Fatalf("关闭后不应登记: %+v", v)
	}
	s = a.settingsSnapshot()
	s.ConfirmTimeout = 10
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 400 {
		t.Fatal("小于下限的确认时限应被拒绝")
	}
	for _, p := range []string{"/api/system/confirm", "/api/system/confirm/connect", "/api/system/confirm/rollback"} {
		if apitoken.Required("GET", p) != apitoken.Denied || apitoken.Required("POST", p) != apitoken.Denied {
			t.Fatalf("%s 不应允许令牌访问", p)
		}
	}
}

// 超时后标记回滚并请求重启；下次启动在加载配置之前恢复改动前的内容
func TestConfirmTimeoutRollsBackOnRestart(t *testing.T) {
	a, srv, c := loginApp(t)
	dir := a.store.Dir()
	before, _ := a.store.ReadRaw("portforward")
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "broken")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	// 模拟时限已到
	a.confirm.mu.Lock()
	a.confirm.tx.Deadline = time.Now().Add(-time.Second)
	id := a.confirm.tx.ID
	a.confirm.mu.Unlock()
	a.confirmExpired(id)
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("超时后应请求重启")
	}
	if got := confirmViaSocket(t, c, srv.URL); got != confirmRollingBack {
		t.Fatalf("已决定回滚时不能再确认，实际 %s", got)
	}

	b, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	b.logger = log.New(io.Discard, "", 0)
	after, _ := b.store.ReadRaw("portforward")
	if string(after) != string(before) {
		t.Fatalf("重启后应恢复改动前的内容:\n%s\n---\n%s", after, before)
	}
	if b.confirm.recovered.reason != "timeout" || len(b.confirm.recovered.rolledBack) != 1 {
		t.Fatalf("应记录超时回滚: %+v", b.confirm.recovered)
	}
	if _, err := os.Stat(filepath.Join(dir, confirmStateName)); !os.IsNotExist(err) {
		t.Fatal("回滚后应删除待确认状态")
	}
	// 被回滚掉的新内容保存在配置历史中
	vs, _ := b.store.Versions("portforward")
	found := false
	for _, v := range vs {
		if data, _ := b.store.ReadVersion("portforward", v.ID); bytes.Contains(data, []byte("broken")) {
			found = true
		}
	}
	if !found {
		t.Fatal("回滚前的新内容应保存为历史版本")
	}
}

// 手动回滚：立即标记并重启；没有待确认变更时返回 409
func TestConfirmManualRollback(t *testing.T) {
	a, srv, c := loginApp(t)
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/confirm/rollback", nil); code != http.StatusConflict {
		t.Fatalf("没有待确认变更时应返回 409，实际 %d", code)
	}
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "x")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/confirm/rollback", nil); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("手动回滚后应请求重启")
	}
	rec := recoverPendingConfirm(a.store, time.Now())
	if rec.reason != "manual" || len(rec.rolledBack) != 1 {
		t.Fatalf("启动时应按手动回滚处理: %+v", rec)
	}
}

// 启动时的处理：未超时继续计时（至少保留宽限时间）、启动次数过多回滚、等待重启的从本次启动开始计时、
// 改动前不存在的配置在回滚时删除
func TestRecoverPendingConfirm(t *testing.T) {
	store, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig := "{\"a\":1}\n"
	write := func(p *pendingConfirm) {
		t.Helper()
		if err := store.WriteRaw("portforward", []byte("{\"a\":2}\n")); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteRaw("webservice", []byte("{\"new\":true}\n")); err != nil {
			t.Fatal(err)
		}
		if err := savePendingConfirm(store.Dir(), p); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	files := func() map[string]*string {
		o := orig
		return map[string]*string{"portforward": &o, "webservice": nil}
	}

	// 未超时：继续计时，截止时间至少延到宽限时间之后
	write(&pendingConfirm{ID: "1", Deadline: now.Add(5 * time.Second), Timeout: 90, Files: files()})
	rec := recoverPendingConfirm(store, now)
	if rec.resume == nil || rec.resume.Boots != 1 || rec.resume.Deadline.Before(now.Add(confirmBootGrace)) || len(rec.rolledBack) != 0 {
		t.Fatalf("未超时应继续计时: %+v", rec)
	}
	// 连续启动次数达到上限：回滚
	p := rec.resume
	p.Boots = confirmMaxBoots
	write(p)
	rec = recoverPendingConfirm(store, now)
	if rec.reason != "boots" || rec.resume != nil {
		t.Fatalf("启动次数过多应回滚: %+v", rec)
	}
	if data, _ := store.ReadRaw("portforward"); string(data) != orig {
		t.Fatalf("应恢复改动前的内容: %s", data)
	}
	if data, _ := store.ReadRaw("webservice"); data != nil {
		t.Fatalf("改动前不存在的配置应删除: %s", data)
	}

	// 等待重启：从本次启动开始按登记的时限计时
	write(&pendingConfirm{ID: "2", WaitRestart: true, Timeout: 120, Boots: 2, Files: files()})
	rec = recoverPendingConfirm(store, now)
	if rec.resume == nil || rec.resume.WaitRestart || !rec.resume.Deadline.Equal(now.Add(120*time.Second)) {
		t.Fatalf("等待重启的变更应从本次启动开始计时: %+v", rec.resume)
	}

	// 状态文件损坏：忽略并删除
	if err := os.WriteFile(filepath.Join(store.Dir(), confirmStateName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec = recoverPendingConfirm(store, now)
	if len(rec.errs) == 0 || rec.resume != nil {
		t.Fatalf("损坏的状态应报告错误: %+v", rec)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), confirmStateName)); !os.IsNotExist(err) {
		t.Fatal("损坏的状态文件应删除")
	}
}

// 需要重启才生效的设置：经管理界面重启时登记，回滚内容只恢复这几项的启动时取值，重启后开始计时
func TestConfirmRestartRequiredSettings(t *testing.T) {
	a, srv, c := loginApp(t)
	s := a.settingsSnapshot()
	bootListen := s.Listen
	s.Listen = "127.0.0.1:" + strconv.Itoa(freeTCPPort(t))
	s.LogLines = 3000
	if code, body := doConfirmJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if got := confirmViaSocket(t, c, srv.URL); got != confirmConfirmed {
		t.Fatalf("保存后的确认应成功，实际 %s", got)
	}
	// 不带请求头的重启不登记
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/restart", nil); code != 200 {
		t.Fatal("重启请求失败")
	}
	<-a.restartCh
	if p, _ := loadPendingConfirm(a.store.Dir()); p != nil {
		t.Fatal("不带确认请求头的重启不应登记")
	}
	if code, _ := doConfirmJSON(t, c, "POST", srv.URL+"/api/system/restart", nil); code != 200 {
		t.Fatal("重启请求失败")
	}
	<-a.restartCh
	p, _ := loadPendingConfirm(a.store.Dir())
	if p == nil || !p.WaitRestart || p.Files[settingsName] == nil {
		t.Fatalf("应登记需要重启后确认的设置变更: %+v", p)
	}
	var rb Settings
	if err := json.Unmarshal([]byte(*p.Files[settingsName]), &rb); err != nil {
		t.Fatal(err)
	}
	if rb.Listen != bootListen || rb.LogLines != 3000 {
		t.Fatalf("回滚内容应只恢复监听地址，其他设置保持当前值: listen=%s logLines=%d", rb.Listen, rb.LogLines)
	}
	if got := confirmViaSocket(t, c, srv.URL); got != confirmWaitRestart {
		t.Fatalf("重启前不能确认，实际 %s", got)
	}
}
