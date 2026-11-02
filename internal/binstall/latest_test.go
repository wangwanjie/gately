package binstall

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

func TestVersionNewer(t *testing.T) {
	cases := []struct {
		latest, installed string
		want              bool
	}{
		{"v0.62.0", "v0.61.2", true},
		{"v0.61.2", "v0.61.2", false},
		{"v0.61.10", "v0.61.9", true},
		{"1.90.2", "1.88.4", true},
		{"1.90.2", "v1.90.2", false},
		{"2025.10.0", "2025.9.1", true},
		{"v1.1.0-stable", "v1.1.0", false},
		{"v1.1.0", "v1.1.0-beta", false},
		{"v1.2", "v1.1.9", true},
		{"v1.2.0.1", "v1.2", true},
		{"v1.0.0", "", false},
		{"", "v1.0.0", false},
		{"latest", "v1.0.0", false},
		{"v1.0.0", "unknown", false},
		{"1.2.3.4.5.6.7.8.9", "1.0.0", false},
	}
	for _, c := range cases {
		if got := VersionNewer(c.latest, c.installed); got != c.want {
			t.Errorf("VersionNewer(%q, %q) = %v，应为 %v", c.latest, c.installed, got, c.want)
		}
	}
}

// TestCheckLatest 覆盖检查更新：只查询不下载、按已安装版本计算是否有更新、失败时保留上次结果、并发检查互斥。
func TestCheckLatest(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo-raw")
	up.set("1.0.0", script("1.0.0"))
	in := newInstaller(t, up.spec("demo"), nil)
	if l := in.LatestInfo(); !l.CheckedAt.IsZero() || in.View().Latest != nil {
		t.Fatalf("从未检查过时不应有结果: %+v", l)
	}
	l, err := in.CheckLatest(context.Background())
	if err != nil || l.Version != "1.0.0" || l.HasUpdate || l.CheckedAt.IsZero() {
		t.Fatalf("未安装时只记录最新版本: %+v %v", l, err)
	}
	if up.requests.Load() != 0 {
		t.Fatal("检查更新不应下载程序")
	}
	install(t, in, "")
	up.set("1.1.0", script("1.1.0"))
	if l, err = in.CheckLatest(context.Background()); err != nil || !l.HasUpdate || l.Version != "1.1.0" {
		t.Fatalf("应提示有新版本: %+v %v", l, err)
	}
	if v := in.View(); v.Latest == nil || !v.Latest.HasUpdate {
		t.Fatalf("安装信息应带上最新版本: %+v", v.Latest)
	}
	// 安装最新版后不再提示
	install(t, in, "")
	if l := in.LatestInfo(); l.HasUpdate {
		t.Fatalf("安装最新版后不应提示: %+v", l)
	}

	// 查询失败：返回错误，保留上次查到的版本
	failing := up.spec("demo")
	failing.Resolve = func(context.Context, *http.Client, Platform, string) (*Release, error) {
		return nil, errors.New("上游不可用")
	}
	in.spec = failing
	l, err = in.CheckLatest(context.Background())
	if err == nil || l.Error != "上游不可用" || l.Version != "1.1.0" {
		t.Fatalf("失败时应记录错误并保留版本: %+v %v", l, err)
	}

	// 并发检查：后到的直接返回 ErrChecking
	release := make(chan struct{})
	started := make(chan struct{})
	slow := up.spec("demo")
	slow.Resolve = func(ctx context.Context, _ *http.Client, _ Platform, _ string) (*Release, error) {
		close(started)
		<-release
		return &Release{Version: "1.2.0"}, nil
	}
	in.spec = slow
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = in.CheckLatest(context.Background())
	}()
	<-started
	if _, err := in.CheckLatest(context.Background()); !errors.Is(err, ErrChecking) {
		t.Fatalf("并发检查应返回 ErrChecking: %v", err)
	}
	close(release)
	wg.Wait()
	if l := in.LatestInfo(); l.Version != "1.2.0" || l.Error != "" {
		t.Fatalf("检查完成后应更新结果: %+v", l)
	}
}

func TestCheckLatestUsesProgramProxy(t *testing.T) {
	hits := make(chan string, 2)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.URL.String()
		_, _ = io.WriteString(w, "9.9.9")
	}))
	defer proxy.Close()
	spec := Spec{
		Key: "demo", Name: "demo", Files: []string{"demo"},
		Resolve: func(ctx context.Context, client *http.Client, _ Platform, _ string) (*Release, error) {
			v, err := FetchText(ctx, client, "http://upstream.invalid/latest")
			if err != nil {
				return nil, err
			}
			return &Release{Version: strings.TrimSpace(v)}, nil
		},
		Probe: func(string) (string, error) { return "", nil },
	}
	in := New(spec, Options{
		BinDir:   filepath.Join(t.TempDir(), "bin"),
		Logs:     logbuf.New(10, log.New(io.Discard, "", 0)),
		GetProxy: func() string { return proxy.URL },
	})
	if l, err := in.CheckLatest(context.Background()); err != nil || l.Version != "9.9.9" {
		t.Fatalf("应经程序的下载代理查询: %+v %v", l, err)
	}
	select {
	case u := <-hits:
		if u != "http://upstream.invalid/latest" {
			t.Fatalf("代理收到 %s", u)
		}
	case <-time.After(time.Second):
		t.Fatal("请求没有经过代理")
	}
}

func TestCheckRoute(t *testing.T) {
	skipWindows(t)
	up := newUpstream(t, "demo-raw")
	up.set("1.0.0", script("1.0.0"))
	spec := up.spec("demo")
	unsupported := false
	spec.Check = func(p Platform) error {
		if unsupported {
			return Unsupported("demo 没有提供 %s 的程序", p)
		}
		return nil
	}
	in := newInstaller(t, spec, nil)
	install(t, in, "")
	up.set("2.0.0", script("2.0.0"))
	mux := http.NewServeMux()
	in.Register(routerFunc(func(pattern string, h http.HandlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.HandleFunc(method+" /api/demo"+path, h)
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	post := func() (int, View) {
		resp, err := http.Post(srv.URL+"/api/demo/install/check", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v View
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return resp.StatusCode, v
	}
	if code, v := post(); code != http.StatusOK || v.Latest == nil || v.Latest.Version != "2.0.0" || !v.Latest.HasUpdate || v.Installed.Version != "1.0.0" {
		t.Fatalf("检查更新接口: %d %+v", code, v.Latest)
	}
	if st := in.State(); st.Stage != StageDone {
		t.Fatalf("检查更新不应改变安装进度: %+v", st)
	}
	unsupported = true
	if code, _ := post(); code != http.StatusBadRequest {
		t.Fatalf("不支持的平台应返回 400，实际 %d", code)
	}
	unsupported = false
	failing := up.spec("demo")
	failing.Resolve = func(context.Context, *http.Client, Platform, string) (*Release, error) {
		return nil, errors.New("上游不可用")
	}
	in.spec = failing
	if code, _ := post(); code != http.StatusBadGateway {
		t.Fatalf("上游失败应返回 502，实际 %d", code)
	}
}
