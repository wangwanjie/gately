package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/applyhold"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// bannerServer 启动一个接受连接后写出 tag 的 TCP 服务，返回端口。
func bannerServer(t *testing.T, tag string) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte(tag))
			c.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// readVia 经转发端口连接，返回读到的标记。
func readVia(t *testing.T, port int) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp4", "127.0.0.1:"+strconv.Itoa(port), time.Second)
		if err == nil {
			_ = c.SetReadDeadline(time.Now().Add(time.Second))
			b, _ := io.ReadAll(c)
			c.Close()
			if len(b) > 0 {
				return string(b)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ""
}

func startModules(t *testing.T, a *App) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for _, e := range a.modules {
		if err := e.mod.Start(ctx); err != nil {
			t.Fatal(err)
		}
		mod := e.mod
		t.Cleanup(func() { _ = mod.Stop() })
	}
}

// 批量修改：期间保存的端口转发只写配置、运行中仍转发到旧目标；全部生效后切到新目标并等待确认
func TestChangesetHoldAndApply(t *testing.T) {
	t.Cleanup(applyhold.Drop)
	a, srv, c := loginApp(t)
	startModules(t, a)
	oldT, newT := bannerServer(t, "old"), bannerServer(t, "new")
	listen := freeTCPPort(t)
	rule := testRule(t, "entry")
	rule["enable"], rule["listenPorts"], rule["targetPorts"] = true, strconv.Itoa(listen), strconv.Itoa(oldT)
	code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	key := created["key"].(string)
	if got := readVia(t, listen); got != "old" {
		t.Fatalf("应转发到旧目标: %q", got)
	}

	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/begin", nil); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	created["targetPorts"] = strconv.Itoa(newT)
	if code, body := doConfirmJSON(t, c, "PUT", srv.URL+"/api/portforward/rules/"+key, created); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if got := readVia(t, listen); got != "old" {
		t.Fatalf("批量修改期间运行中的规则不应切换: %q", got)
	}
	v := confirmStatus(t, c, srv.URL)
	if v.Hold == nil || len(v.Hold.Items) != 1 || v.Hold.Items[0].Key != key || v.Hold.Items[0].Name != "entry" || v.Pending == nil || !v.Pending.Hold {
		t.Fatalf("应显示暂缓生效的规则: %+v %+v", v.Hold, v.Pending)
	}
	if got := confirmViaSocket(t, c, srv.URL); got != confirmHold {
		t.Fatalf("批量修改期间确认应返回 hold，实际 %s", got)
	}

	code, body = doJSON(t, c, "POST", srv.URL+"/api/system/changeset/apply", nil)
	var res holdApplyResult
	if code != 200 || json.Unmarshal(body, &res) != nil || res.Applied != 1 || !res.Confirm {
		t.Fatalf("全部生效应返回生效数量并需要确认: %d %s", code, body)
	}
	// 生效在响应之后进行：确认接口会等到生效完成
	a.confirm.writeMu.Lock()
	a.confirm.writeMu.Unlock()
	if applyhold.Active() {
		t.Fatal("生效完成后应结束暂缓")
	}
	if got := readVia(t, listen); got != "new" {
		t.Fatalf("全部生效后应转发到新目标: %q", got)
	}
	if v := confirmStatus(t, c, srv.URL); v.Hold != nil || v.Pending == nil || v.Pending.Hold || v.Pending.Remaining <= 0 {
		t.Fatalf("全部生效后应开始计时: %+v", v.Pending)
	}
	if got := confirmViaSocket(t, c, srv.URL); got != confirmConfirmed {
		t.Fatalf("应能确认: %s", got)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/apply", nil); code != 409 {
		t.Fatal("没有批量修改时应返回 409")
	}
}

// 批量修改中途重启（进程内存清空）：按新配置启动，从启动时开始计时
func TestChangesetRestartMidway(t *testing.T) {
	t.Cleanup(applyhold.Drop)
	a, srv, c := loginApp(t)
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/begin", nil); code != 200 {
		t.Fatal("开始批量修改失败")
	}
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "staged")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	applyhold.Drop()
	rec := recoverPendingConfirm(a.store, time.Now())
	if rec.resume == nil || rec.resume.Hold || rec.resume.Deadline.IsZero() || len(rec.rolledBack) != 0 {
		t.Fatalf("批量修改中途重启应继续计时: %+v", rec)
	}
}

// 放弃批量修改：标记回滚并重启，启动时恢复开始前的配置
func TestChangesetDiscard(t *testing.T) {
	t.Cleanup(applyhold.Drop)
	a, srv, c := loginApp(t)
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "kept")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	before, _ := a.store.ReadRaw("portforward")
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/begin", nil); code != 200 {
		t.Fatal("开始批量修改失败")
	}
	if code, body := doConfirmJSON(t, c, "POST", srv.URL+"/api/portforward/rules", testRule(t, "staged")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/discard", nil); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("放弃后应重启")
	}
	if applyhold.Active() {
		t.Fatal("放弃后应结束暂缓")
	}
	n, err := New(Options{ConfigDir: a.store.Dir(), Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	n.logger = log.New(io.Discard, "", 0)
	if cur, _ := n.store.ReadRaw("portforward"); string(cur) != string(before) || n.confirm.recovered.reason != "manual" {
		t.Fatalf("重启后应恢复开始批量修改前的配置: %+v", n.confirm.recovered)
	}
}

// 没有任何改动时放弃：直接结束，不重启
func TestChangesetDiscardEmpty(t *testing.T) {
	t.Cleanup(applyhold.Drop)
	a, srv, c := loginApp(t)
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/begin", nil); code != 200 {
		t.Fatal("开始批量修改失败")
	}
	code, body := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/discard", nil)
	var res struct{ Restart bool }
	if code != 200 || json.Unmarshal(body, &res) != nil || res.Restart || applyhold.Active() {
		t.Fatalf("没有改动时应直接结束: %d %s", code, body)
	}
	if p, _ := loadPendingConfirm(a.store.Dir()); p != nil {
		t.Fatal("不应留下待确认状态")
	}
}

// 监听地址在本机不可用：全部生效前的检查列出来；生效后模块报告的启动失败记入待确认变更，回滚后据此说明原因
func TestChangesetListenProblemAndErrors(t *testing.T) {
	t.Cleanup(applyhold.Drop)
	a, srv, c := loginApp(t)
	startModules(t, a)
	rule := testRule(t, "entry")
	rule["enable"] = true
	code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	key := created["key"].(string)

	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/begin", nil); code != 200 {
		t.Fatal("开始批量修改失败")
	}
	// 203.0.113.0/24 为文档专用地址，不会是本机地址
	created["listenAddress"] = "203.0.113.5"
	if code, body := doConfirmJSON(t, c, "PUT", srv.URL+"/api/portforward/rules/"+key, created); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	code, body = doJSON(t, c, "POST", srv.URL+"/api/system/changeset/check", nil)
	var chk struct{ Problems []holdProblem }
	if code != 200 || json.Unmarshal(body, &chk) != nil || len(chk.Problems) != 1 || chk.Problems[0].Addr != "203.0.113.5" || chk.Problems[0].Error == "" {
		t.Fatalf("应列出不可用的监听地址: %d %s", code, body)
	}

	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/changeset/apply", nil); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	a.confirm.writeMu.Lock()
	a.confirm.writeMu.Unlock()
	v := confirmStatus(t, c, srv.URL)
	if v.Pending == nil || len(v.Pending.Errors) == 0 || !strings.Contains(v.Pending.Errors[0], "203.0.113.5") {
		t.Fatalf("待确认状态应带上监听启动失败的错误: %+v", v.Pending)
	}
	// 到期回滚：错误随状态保存，启动时交给回滚提示
	a.confirm.mu.Lock()
	a.confirm.tx.Deadline = time.Now().Add(-time.Second)
	id := a.confirm.tx.ID
	a.confirm.mu.Unlock()
	a.confirmExpired(id)
	<-a.restartCh
	rec := recoverPendingConfirm(a.store, time.Now())
	if rec.reason != "timeout" || len(rec.applyErrs) == 0 || !strings.Contains(rec.applyErrs[0], "203.0.113.5") {
		t.Fatalf("回滚结果应带上生效期间的错误: %+v", rec)
	}
}

func TestListenProblem(t *testing.T) {
	for _, ip := range []string{"", "0.0.0.0", "::", "127.0.0.1", "not-an-ip"} {
		if err := listenProblem(ip); err != nil {
			t.Errorf("%q 应可用: %v", ip, err)
		}
	}
	if listenProblem("203.0.113.5") == nil {
		t.Error("非本机地址应不可用")
	}
}
