package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// fakeDocker 模拟 Docker Engine API 的一个最小子集。
type fakeDocker struct {
	mu         sync.Mutex
	containers map[string]map[string]any // id -> inspect
	images     map[string]string         // ref -> image id
	pullTo     string                    // 拉取后镜像变为该 ID
	failPull   bool                      // 拉取镜像时返回错误（模拟镜像仓库限流）
	failStart  string                    // 启动该 ID 时失败
	calls      []string
	created    map[string]any
	connected  []string
	next       int
	// extraList 追加到容器列表中（如升级辅助容器），logText 非空时作为日志的 stdout 输出
	extraList []map[string]any
	logText   string
	// stopped 中的容器在列表中显示为已停止
	stopped map[string]bool
}

func (f *fakeDocker) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/v"+apiVersion)
	f.calls = append(f.calls, r.Method+" "+p)
	parts := strings.Split(strings.Trim(p, "/"), "/")
	switch {
	case p == "/_ping":
		_, _ = w.Write([]byte("OK"))
	case p == "/containers/json":
		var list []map[string]any
		for id, c := range f.containers {
			cfg := c["Config"].(map[string]any)
			list = append(list, map[string]any{"Id": id, "Names": []string{c["Name"].(string)}, "Image": cfg["Image"], "ImageID": c["Image"],
				"State": map[bool]string{false: "running", true: "exited"}[f.stopped[id]], "Status": "Up", "Labels": map[string]string{"com.docker.compose.project": "web"},
				"Ports": []map[string]any{{"PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}, {"PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}}})
		}
		list = append(list, f.extraList...)
		_ = json.NewEncoder(w).Encode(list)
	case len(parts) == 3 && parts[0] == "containers" && parts[2] == "json":
		c, ok := f.containers[parts[1]]
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"no such container"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	case len(parts) == 3 && parts[0] == "containers" && (parts[2] == "start" || parts[2] == "stop" || parts[2] == "restart"):
		if parts[2] == "start" && parts[1] == f.failStart {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"message":"port is already allocated"}`))
			return
		}
		w.WriteHeader(204)
	case len(parts) == 3 && parts[2] == "logs" && f.logText != "":
		text := f.logText
		if r.URL.Query().Get("timestamps") == "1" {
			// 与 Docker 一致，带时间戳时每行行首加上时间
			lines := strings.SplitAfter(strings.TrimSuffix(text, "\n"), "\n")
			for i := range lines {
				lines[i] = "2026-11-01T08:09:09.671620459Z " + lines[i]
			}
			text = strings.Join(lines, "") + "\n"
		}
		h := make([]byte, 8)
		h[0] = 1
		binary.BigEndian.PutUint32(h[4:], uint32(len(text)))
		_, _ = w.Write(append(h, text...))
	case len(parts) == 3 && parts[2] == "logs":
		// 多路复用格式：stdout 一帧、stderr 一帧
		for i, s := range []string{"hello stdout\n", "oops stderr\n"} {
			h := make([]byte, 8)
			h[0] = byte(i + 1)
			binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
			_, _ = w.Write(append(h, s...))
		}
	case p == "/images/create":
		if f.failPull {
			http.Error(w, `{"message":"toomanyrequests"}`, http.StatusInternalServerError)
			return
		}
		ref := r.URL.Query().Get("fromImage") + ":" + r.URL.Query().Get("tag")
		_, _ = w.Write([]byte(`{"status":"Pulling"}` + "\n" + `{"status":"Done"}` + "\n"))
		f.images[ref] = f.pullTo
	case len(parts) >= 3 && parts[0] == "images" && parts[len(parts)-1] == "json":
		ref := strings.Join(parts[1:len(parts)-1], "/")
		if id, ok := f.images[ref]; ok {
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
			return
		}
		w.WriteHeader(404)
	case len(parts) == 3 && parts[2] == "rename":
		f.containers[parts[1]]["Name"] = "/" + r.URL.Query().Get("name")
		w.WriteHeader(204)
	case p == "/containers/create":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.next++
		id := fmt.Sprintf("new%061d", f.next)
		f.created = body
		f.containers[id] = map[string]any{"Id": id, "Name": "/" + r.URL.Query().Get("name"), "Image": f.pullTo, "Config": body, "State": map[string]any{"Running": false}}
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	case len(parts) == 3 && parts[0] == "networks" && parts[2] == "connect":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.connected = append(f.connected, parts[1])
		w.WriteHeader(200)
	case r.Method == http.MethodDelete && len(parts) == 2:
		delete(f.containers, parts[1])
		w.WriteHeader(204)
	default:
		w.WriteHeader(404)
	}
}

func startFake(t *testing.T) (*fakeDocker, string) {
	t.Helper()
	oldID := "abc123def456" + strings.Repeat("0", 52)
	f := &fakeDocker{
		containers: map[string]map[string]any{oldID: {
			"Id": oldID, "Name": "/nginx", "Image": "sha256:old",
			"Config":     map[string]any{"Image": "nginx:1.27", "Hostname": oldID[:12], "Env": []string{"A=1"}, "Labels": map[string]string{"x": "y"}},
			"HostConfig": map[string]any{"Binds": []string{"/data:/data"}, "NetworkMode": "web_default", "RestartPolicy": map[string]any{"Name": "always"}},
			"NetworkSettings": map[string]any{"Networks": map[string]any{
				"web_default": map[string]any{"Aliases": []string{"nginx"}, "IPAMConfig": map[string]any{"IPv4Address": "172.20.0.5"}},
				"backend":     map[string]any{"Aliases": []string{"api"}},
			}},
			"Mounts": []map[string]any{
				{"Type": "bind", "Source": "/data", "Destination": "/data", "RW": true},
				{"Type": "volume", "Name": "f00dcafe", "Destination": "/var/cache/nginx", "RW": true},
			},
			"State": map[string]any{"Running": true},
		}},
		images: map[string]string{"nginx:1.27": "sha256:old"},
		pullTo: "sha256:new",
	}
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("当前环境不支持 unix socket:", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(f.handler)}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return f, "unix://" + sock
}

func newModule(t *testing.T, host string) *Module {
	st, _ := config.Open(t.TempDir())
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings.Host = host
	return m
}

func TestClientBasics(t *testing.T) {
	_, host := startFake(t)
	c, err := NewClient(host)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := c.Containers(ctx)
	if err != nil || len(list) != 1 || list[0].Name() != "nginx" {
		t.Fatalf("容器列表错误: %+v %v", list, err)
	}
	logs, err := c.Logs(ctx, list[0].ID, 10)
	if err != nil || logs != "hello stdout\noops stderr\n" {
		t.Fatalf("日志解析错误: %q %v", logs, err)
	}
	if err := c.Action(ctx, list[0].ID, "rm -rf"); err == nil {
		t.Fatal("非法操作应报错")
	}
}

func TestSplitRefAndDemux(t *testing.T) {
	cases := map[string][2]string{
		"nginx":                         {"nginx", "latest"},
		"nginx:1.27":                    {"nginx", "1.27"},
		"registry.local:5000/app":       {"registry.local:5000/app", "latest"},
		"registry.local:5000/app:v2":    {"registry.local:5000/app", "v2"},
		"ghcr.io/a/b@sha256:abcdef0123": {"ghcr.io/a/b@sha256:abcdef0123", ""},
	}
	for in, want := range cases {
		n, tag := splitRef(in)
		if n != want[0] || tag != want[1] {
			t.Errorf("splitRef(%q)=%q,%q", in, n, tag)
		}
	}
	if demux([]byte("plain tty output")) != "plain tty output" {
		t.Error("非多路复用输出应原样返回")
	}
}

func TestUpgradePreservesConfig(t *testing.T) {
	f, host := startFake(t)
	m := newModule(t, host)
	c, _ := NewClient(host)
	ctx := context.Background()
	var oldID string
	for id := range f.containers {
		oldID = id
	}
	has, err := m.checkUpdate(ctx, c, oldID)
	if err != nil || !has {
		t.Fatalf("应检测到更新: %v %v", has, err)
	}
	newID, err := m.upgrade(ctx, c, oldID)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.containers[oldID]; ok {
		t.Fatal("旧容器应被删除")
	}
	nc := f.containers[newID]
	if nc["Name"] != "/nginx" {
		t.Fatalf("新容器应沿用原名: %v", nc["Name"])
	}
	b := f.created
	if _, ok := b["Hostname"]; ok {
		t.Fatal("由旧容器 ID 生成的主机名不应沿用")
	}
	hc := b["HostConfig"].(map[string]any)
	if hc["Binds"].([]any)[0] != "/data:/data" || b["Image"] != "nginx:1.27" {
		t.Fatalf("配置应完整保留: %+v", b)
	}
	if binds := hc["Binds"].([]any); len(binds) != 2 || binds[1] != "f00dcafe:/var/cache/nginx" {
		t.Fatalf("匿名卷应沿用: %v", binds)
	}
	eps := b["NetworkingConfig"].(map[string]any)["EndpointsConfig"].(map[string]any)
	if len(eps) != 1 || len(f.connected) != 1 || f.connected[0] != "backend" {
		t.Fatalf("创建时只带主网络，其余网络创建后连接: %v %v", eps, f.connected)
	}
	ep := eps["web_default"].(map[string]any)
	if ep["Aliases"].([]any)[0] != "nginx" || ep["IPAMConfig"].(map[string]any)["IPv4Address"] != "172.20.0.5" {
		t.Fatalf("网络别名与固定 IP 应保留: %+v", ep)
	}
}

func TestUpgradeRollbackOnStartFailure(t *testing.T) {
	f, host := startFake(t)
	f.failStart = fmt.Sprintf("new%061d", 1)
	m := newModule(t, host)
	c, _ := NewClient(host)
	var oldID string
	for id := range f.containers {
		oldID = id
	}
	if _, err := m.upgrade(context.Background(), c, oldID); err == nil || !strings.Contains(err.Error(), "port is already allocated") {
		t.Fatalf("应返回启动失败原因: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.containers[oldID]["Name"] != "/nginx" {
		t.Fatalf("失败后旧容器应恢复原名: %v", f.containers[oldID]["Name"])
	}
	if len(f.containers) != 1 {
		t.Fatalf("失败的新容器应被删除: %d", len(f.containers))
	}
	last := f.calls[len(f.calls)-1]
	if last != "POST /containers/"+oldID+"/start" {
		t.Fatalf("应重新启动旧容器，最后一次调用 %s", last)
	}
}

func TestNegotiateVersion(t *testing.T) {
	cases := []struct{ max, min, want string }{
		{"1.52", "1.44", "1.44"}, // Docker 29 不再支持 1.41
		{"1.45", "1.24", "1.41"}, // 支持首选版本时保持不变
		{"1.40", "1.12", "1.40"}, // 旧版 Docker 用服务端版本
		{"", "", "1.41"},         // 读不到版本
	}
	for _, c := range cases {
		if got := negotiateVersion(c.max, c.min); got != c.want {
			t.Errorf("negotiateVersion(%q, %q) = %q，期望 %q", c.max, c.min, got, c.want)
		}
	}
}

// 定时检查发现镜像更新时通知一次；同一个新镜像不重复通知，出现更新的镜像时再次通知
func TestAutoCheckNotifiesImageUpdates(t *testing.T) {
	f, host := startFake(t)
	m := newModule(t, host)
	var mu sync.Mutex
	var got []events.Event
	unsub := events.Subscribe(func(e events.Event) {
		if e.Type == events.DockerUpdate {
			mu.Lock()
			got = append(got, e)
			mu.Unlock()
		}
	})
	defer unsub()
	take := func() []events.Event {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		out := got
		got = nil
		return out
	}
	m.autoCheckOnce(context.Background())
	evs := take()
	if len(evs) != 1 || evs[0].Fields["count"] != "1" || !strings.Contains(evs[0].Message, "nginx (nginx:1.27)") {
		t.Fatalf("应通知一次镜像更新: %+v", evs)
	}
	m.autoCheckOnce(context.Background())
	if evs := take(); len(evs) != 0 {
		t.Fatalf("同一个新镜像不应重复通知: %+v", evs)
	}
	f.mu.Lock()
	f.pullTo = "sha256:newer"
	f.mu.Unlock()
	m.autoCheckOnce(context.Background())
	if evs := take(); len(evs) != 1 {
		t.Fatalf("出现更新的镜像时应再次通知: %+v", evs)
	}
	// 手动检查不发通知
	c, _ := NewClient(host)
	if err := m.checkAll(context.Background(), c, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}
	if evs := take(); len(evs) != 0 {
		t.Fatalf("手动检查不应通知: %+v", evs)
	}
}

func TestPublishImageUpdatesLimit(t *testing.T) {
	ch := make(chan events.Event, 1)
	unsub := events.Subscribe(func(e events.Event) {
		if e.Type == events.DockerUpdate {
			ch <- e
		}
	})
	defer unsub()
	var names []string
	for i := range maxUpdateNames + 5 {
		names = append(names, fmt.Sprintf("c%d (img)", i))
	}
	publishImageUpdates(names)
	select {
	case e := <-ch:
		if strings.Contains(e.Message, fmt.Sprintf("c%d (", maxUpdateNames)) || !strings.Contains(e.Message, fmt.Sprintf("等 %d 个", len(names))) {
			t.Fatalf("超出上限的容器应以数量概括: %s", e.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("没有收到通知")
	}
}

// 定时检查中某个容器检查失败（如镜像仓库限流）时保留它的通知记录，下次检查成功不重复通知同一个新镜像
func TestAutoCheckKeepsRecordOnFailure(t *testing.T) {
	f, host := startFake(t)
	m := newModule(t, host)
	var mu sync.Mutex
	n := 0
	unsub := events.Subscribe(func(e events.Event) {
		if e.Type == events.DockerUpdate {
			mu.Lock()
			n++
			mu.Unlock()
		}
	})
	defer unsub()
	count := func() int {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		out := n
		n = 0
		return out
	}
	m.autoCheckOnce(context.Background())
	if got := count(); got != 1 {
		t.Fatalf("应通知一次镜像更新，实际 %d 次", got)
	}
	f.mu.Lock()
	f.failPull = true
	f.mu.Unlock()
	m.autoCheckOnce(context.Background())
	if got := count(); got != 0 {
		t.Fatalf("检查失败时不应通知，实际 %d 次", got)
	}
	f.mu.Lock()
	f.failPull = false
	f.mu.Unlock()
	m.autoCheckOnce(context.Background())
	if got := count(); got != 0 {
		t.Fatalf("检查恢复后同一个新镜像不应重复通知，实际 %d 次", got)
	}
}

// 定时检查只检查运行中的容器：容器停止一轮再启动后，同一个新镜像不重复通知
func TestAutoCheckStopped(t *testing.T) {
	f, host := startFake(t)
	m := newModule(t, host)
	var mu sync.Mutex
	n := 0
	unsub := events.Subscribe(func(e events.Event) {
		if e.Type == events.DockerUpdate {
			mu.Lock()
			n++
			mu.Unlock()
		}
	})
	defer unsub()
	count := func() int {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		out := n
		n = 0
		return out
	}
	m.autoCheckOnce(context.Background())
	if got := count(); got != 1 {
		t.Fatalf("应通知一次镜像更新，实际 %d 次", got)
	}
	f.mu.Lock()
	f.stopped = map[string]bool{}
	for id := range f.containers {
		f.stopped[id] = true
	}
	f.mu.Unlock()
	m.autoCheckOnce(context.Background())
	f.mu.Lock()
	f.stopped = nil
	f.mu.Unlock()
	m.autoCheckOnce(context.Background())
	if got := count(); got != 0 {
		t.Fatalf("容器重新启动后同一个新镜像不应重复通知，实际 %d 次", got)
	}
}
