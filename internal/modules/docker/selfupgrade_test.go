package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

const selfTestID = "5e1f" + "0000000000000000000000000000000000000000000000000000000000aa"

// addSelfContainer 在假 Docker 中加入 Gately 自身所在的容器，mountSock 为真时挂载 Docker 套接字。
func addSelfContainer(t *testing.T, f *fakeDocker, host string, mountSock bool) {
	t.Helper()
	sock := strings.TrimPrefix(host, "unix://")
	mounts := []map[string]any{{"Type": "bind", "Source": "/opt/gately/data", "Destination": "/etc/gately", "RW": true}}
	if mountSock {
		mounts = append(mounts, map[string]any{"Type": "bind", "Source": "/var/run/docker.sock", "Destination": sock, "RW": true})
	}
	f.mu.Lock()
	f.containers[selfTestID] = map[string]any{
		"Id": selfTestID, "Name": "/gately", "Image": "sha256:gold",
		"Config":     map[string]any{"Image": "ghcr.io/wangwanjie/gately:latest", "Labels": map[string]string{"com.docker.compose.project": "gately"}},
		"HostConfig": map[string]any{"Binds": []string{"/opt/gately/data:/etc/gately"}, "NetworkMode": "host"},
		"Mounts":     mounts,
		"State":      map[string]any{"Running": true},
	}
	f.images["ghcr.io/wangwanjie/gately:latest"] = "sha256:gold"
	f.mu.Unlock()
	fakeSelf(t, selfTestID)
	old := selfExecutable
	selfExecutable = func() (string, error) { return "/usr/local/bin/gately", nil }
	t.Cleanup(func() { selfExecutable = old })
}

// 升级自身：拉取新镜像后用当前镜像创建辅助容器，Gately 自己不执行停止与重建
func TestSelfUpgradeStartsHelper(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, true)
	_, api := ctrAPI(t, host)

	rec := api("POST", "/containers/"+selfTestID+"/upgrade", "", nil)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["self"] != true || out["upToDate"] != false {
		t.Fatalf("升级自身应交给辅助容器: %d %s", rec.Code, rec.Body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.created
	cmd, _ := json.Marshal(b["Cmd"])
	if b["Image"] != "sha256:gold" || fmt.Sprint(b["Entrypoint"]) != "[/usr/local/bin/gately]" ||
		string(cmd) != fmt.Sprintf(`["%s","-host","%s","-container","%s"]`, SelfUpgradeCommand, host, selfTestID) {
		t.Fatalf("辅助容器应使用当前镜像执行辅助子命令: %+v", b)
	}
	if b["Labels"].(map[string]any)[selfUpgradeLabel] != "gately" {
		t.Fatalf("辅助容器应带标记: %+v", b["Labels"])
	}
	hc := b["HostConfig"].(map[string]any)
	sock := strings.TrimPrefix(host, "unix://")
	if fmt.Sprint(hc["Binds"]) != "[/var/run/docker.sock:"+sock+"]" || hc["NetworkMode"] != "host" {
		t.Fatalf("辅助容器应挂载宿主机的 Docker 套接字并沿用网络: %+v", hc)
	}
	helper := fmt.Sprintf("new%061d", 1)
	var started bool
	for _, c := range f.calls {
		if c == "POST /containers/"+selfTestID+"/stop" || strings.HasPrefix(c, "POST /containers/"+selfTestID+"/rename") {
			t.Fatalf("Gately 不应自己停止或改名自身容器: %v", f.calls)
		}
		if c == "POST /containers/"+helper+"/start" {
			started = true
		}
	}
	if !started {
		t.Fatalf("应启动辅助容器: %v", f.calls)
	}
}

// 已有升级自身的辅助容器尚未结束时拒绝再次升级，避免两个辅助容器争抢同一个容器名
func TestSelfUpgradeRejectsConcurrent(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, true)
	f.extraList = []map[string]any{
		{"Id": "h1", "Names": []string{"/gately-upgrade-1"}, "State": "running", "Labels": map[string]string{selfUpgradeLabel: "gately"}},
	}
	_, api := ctrAPI(t, host)
	rec := api("POST", "/containers/"+selfTestID+"/upgrade", "", nil)
	if rec.Code == 200 || !strings.Contains(rec.Body.String(), "已有升级正在进行") {
		t.Fatalf("应拒绝并发升级: %d %s", rec.Code, rec.Body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != nil {
		t.Fatalf("不应再创建辅助容器: %+v", f.created)
	}
}

// 镜像没有更新时不创建辅助容器，避免无谓的重启
func TestSelfUpgradeUpToDate(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, true)
	f.pullTo = "sha256:gold"
	_, api := ctrAPI(t, host)
	rec := api("POST", "/containers/"+selfTestID+"/upgrade", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"upToDate":true`) {
		t.Fatalf("已是最新应直接返回: %d %s", rec.Code, rec.Body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != nil {
		t.Fatalf("已是最新不应创建辅助容器: %+v", f.created)
	}
}

// 找不到 Docker 套接字的挂载时辅助容器无法连上 Docker，应在拉取镜像前拒绝
func TestSelfUpgradeNeedsSocketMount(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, false)
	_, api := ctrAPI(t, host)
	rec := api("POST", "/containers/"+selfTestID+"/upgrade", "", nil)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "套接字") {
		t.Fatalf("缺少套接字挂载应拒绝: %d %s", rec.Code, rec.Body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == "POST /images/create" || c == "POST /containers/create" {
			t.Fatalf("拒绝前不应拉取镜像或创建容器: %v", f.calls)
		}
	}
}

func TestHelperDockerAccess(t *testing.T) {
	m := New()
	c, _ := NewClient("unix:///var/run/docker.sock")
	mk := func(src, dst string) selfInspect {
		var info selfInspect
		info.Mounts = append(info.Mounts, struct {
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
		}{src, dst})
		return info
	}
	// 挂载套接字所在目录
	if _, binds, err := m.helperDockerAccess(c, mk("/run", "/var/run")); err != nil || fmt.Sprint(binds) != "[/run:/var/run]" {
		t.Fatalf("目录挂载应可用: %v %v", binds, err)
	}
	// 同名前缀的其它目录不算
	if _, _, err := m.helperDockerAccess(c, mk("/x", "/var/run/docker")); err == nil {
		t.Fatal("/var/run/docker 不包含 /var/run/docker.sock")
	}
	// TCP 地址不需要挂载
	m.settings.Host = "tcp://10.0.0.2:2375"
	tc, _ := NewClient(m.settings.Host)
	if host, binds, err := m.helperDockerAccess(tc, selfInspect{}); err != nil || host != m.settings.Host || len(binds) != 0 {
		t.Fatalf("TCP 地址应原样传给辅助容器: %s %v %v", host, binds, err)
	}
}

// 批量升级时自身放到最后，其余容器升级完成后再交给辅助容器
func TestBatchUpgradeSelfLast(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, true)
	var other string
	for id := range f.containers {
		if id != selfTestID {
			other = id
		}
	}
	_, api := ctrAPI(t, host)
	rec := api("POST", "/containers/upgrade-batch", "application/json", jsonBody(map[string]any{"ids": []string{selfTestID, other}}))
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	task := ctrWaitTask(t, out["task"])
	joined := strings.Join(task.Lines, "\n")
	done := strings.Index(joined, "nginx: 升级完成")
	self := strings.Index(joined, "gately: 开始升级 Gately 自身")
	if task.Status != "done" || done < 0 || self < done || !strings.Contains(joined, "辅助容器") {
		t.Fatalf("自身应在其余容器之后交给辅助容器: %+v", task)
	}
}

type memLogger struct{ lines []string }

func (l *memLogger) Infof(format string, args ...any) {
	l.lines = append(l.lines, "info "+fmt.Sprintf(format, args...))
}
func (l *memLogger) Warnf(format string, args ...any) {
	l.lines = append(l.lines, "warn "+fmt.Sprintf(format, args...))
}
func (l *memLogger) Errorf(format string, args ...any) {
	l.lines = append(l.lines, "error "+fmt.Sprintf(format, args...))
}

// 辅助容器按原配置重建自身：新容器沿用原名并启动，旧容器删除
func TestSelfRebuilds(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, true)
	old := selfUpgradeDelay
	selfUpgradeDelay = 0
	t.Cleanup(func() { selfUpgradeDelay = old })
	log := &memLogger{}
	if err := runSelfUpgrade(host, selfTestID, log); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.containers[selfTestID]; ok {
		t.Fatal("旧容器应被删除")
	}
	newID := fmt.Sprintf("new%061d", 1)
	if f.containers[newID]["Name"] != "/gately" || f.created["Image"] != "ghcr.io/wangwanjie/gately:latest" {
		t.Fatalf("新容器应沿用原名与镜像引用: %+v", f.containers[newID])
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "POST /containers/"+newID+"/start") {
		t.Fatalf("应启动新容器: %v", f.calls)
	}
}

// 新容器启动失败时恢复旧容器
func TestSelfRestoreOnFailure(t *testing.T) {
	f, host := startFake(t)
	addSelfContainer(t, f, host, true)
	f.failStart = fmt.Sprintf("new%061d", 1)
	old := selfUpgradeDelay
	selfUpgradeDelay = 0
	t.Cleanup(func() { selfUpgradeDelay = old })
	var out strings.Builder
	// RunSelfUpgrade 的输出带级别前缀，由新启动的 Gately 读取
	if code := RunSelfUpgrade([]string{"-host", host, "-container", selfTestID}, &out); code != 1 {
		t.Fatalf("失败时退出码应为 1: %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), "[error] 升级失败") || !strings.Contains(out.String(), "port is already allocated") {
		t.Fatalf("应输出失败原因: %s", out.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.containers[selfTestID]["Name"] != "/gately" {
		t.Fatalf("旧容器应恢复原名: %v", f.containers[selfTestID]["Name"])
	}
	if last := f.calls[len(f.calls)-1]; last != "POST /containers/"+selfTestID+"/start" {
		t.Fatalf("应重新启动旧容器，最后一次调用 %s", last)
	}
}

// 启动后读取已结束的辅助容器输出写入日志并删除，运行中的辅助容器留到下一轮
func TestCollectSelfUpgrades(t *testing.T) {
	f, host := startFake(t)
	m := newModule(t, host)
	addSelfContainer(t, f, host, true)
	f.extraList = []map[string]any{
		{"Id": "h1", "Names": []string{"/gately-upgrade-1"}, "State": "exited", "Labels": map[string]string{selfUpgradeLabel: "gately"}},
		{"Id": "h2", "Names": []string{"/gately-upgrade-2"}, "State": "running", "Labels": map[string]string{selfUpgradeLabel: "gately"}},
		// 同一 Docker 上另一个 Gately 实例的辅助容器，不归这里处理
		{"Id": "h3", "Names": []string{"/other-upgrade-1"}, "State": "exited", "Labels": map[string]string{selfUpgradeLabel: "other"}},
	}
	f.logText = "[info] 开始按新镜像重建容器 gately\n[warn] 删除旧容器 x 失败，请手动清理: busy\n[error] 升级失败: boom\n"
	c, _ := NewClient(host)
	pending, err := m.collectSelfUpgradesOnce(context.Background(), c)
	if err != nil || !pending {
		t.Fatalf("运行中的辅助容器应留待下一轮: %v %v", pending, err)
	}
	var got []string
	for _, e := range m.logs.Since(0) {
		got = append(got, e.Level+" "+e.Source+" "+e.Message)
	}
	want := []string{
		logbuf.LevelInfo + " gately 开始按新镜像重建容器 gately",
		logbuf.LevelWarn + " gately 删除旧容器 x 失败，请手动清理: busy",
		logbuf.LevelError + " gately 升级失败: boom",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("日志不符:\n%s", strings.Join(got, "\n"))
	}
	f.mu.Lock()
	calls := strings.Join(f.calls, "\n")
	f.mu.Unlock()
	if !strings.Contains(calls, "DELETE /containers/h1") || strings.Contains(calls, "DELETE /containers/h2") || strings.Contains(calls, "/containers/h2/logs") {
		t.Fatalf("只处理已结束的辅助容器: %s", calls)
	}
	if strings.Contains(calls, "/containers/h3") {
		t.Fatalf("不应处理其他 Gately 实例的辅助容器: %s", calls)
	}

	// 轮询到没有运行中的辅助容器为止
	oldInt := helperCollectInterval
	helperCollectInterval = 10 * time.Millisecond
	t.Cleanup(func() { helperCollectInterval = oldInt })
	go func() {
		time.Sleep(30 * time.Millisecond)
		f.mu.Lock()
		f.extraList[1]["State"] = "exited"
		f.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.collectSelfUpgrades(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(strings.Join(f.calls, "\n"), "DELETE /containers/h2") {
		t.Fatalf("辅助容器结束后应被清理: %v", f.calls)
	}
}
