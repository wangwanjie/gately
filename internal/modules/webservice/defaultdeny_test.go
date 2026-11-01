package webservice

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/webprobe"
)

// TestDefaultSiteListThenClose 验证 issue #53 的场景用“反向代理 + IP 访问控制 + 拒绝时断开连接”实现：
// 默认站点（以及普通站点）引用订阅 IP 列表，白名单命中时代理到后端、未命中时断开连接，黑名单反之；
// 订阅列表刷新后立即生效，无需重启监听。同时验证路径探测的上报口径：默认站点上被拒绝断开的请求按 444 上报，
// 与「断开连接」类型的默认站点一致；普通站点上按 403 上报，避免名单外的正常访客被路径探测误封。
func TestDefaultSiteListThenClose(t *testing.T) {
	var mu sync.Mutex
	var hits []webprobe.Hit
	webprobe.SetHandler(func(h webprobe.Hit) {
		mu.Lock()
		hits = append(hits, h)
		mu.Unlock()
	})
	t.Cleanup(func() { webprobe.SetHandler(nil) })
	// wait 等待第 n 条上报：上报在响应写出后的 defer 中执行，客户端收到响应时可能还没上报
	wait := func(n int) webprobe.Hit {
		t.Helper()
		for i := 0; i < 200; i++ {
			mu.Lock()
			if len(hits) >= n {
				h := hits[n-1]
				mu.Unlock()
				return h
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("没有收到第 %d 条上报", n)
		return webprobe.Hit{}
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "backend")
	}))
	t.Cleanup(backend.Close)

	// setList 模拟订阅列表刷新：直接替换注册表里的内容，站点按 key 引用，下一次请求即读到新内容
	const key = "test-i53"
	setList := func(body string) {
		t.Helper()
		set, _, err := iplist.Parse(strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		iplist.Default.Store(key, set)
	}
	t.Cleanup(func() { iplist.Default.Store(key, nil) })

	// try 访问一次：返回是否拿到响应及响应体；被断开时 Do 返回错误
	try := func(addr, host string) (bool, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", "http://"+addr+"/wp-login.php", nil)
		req.Host = host
		// 每次用新连接：复用的连接被断开时 net/http 会自动重试 GET，同一请求会多上报一次
		c := noRedirect()
		c.Transport = &http.Transport{DisableKeepAlives: true}
		c.Timeout = 5 * time.Second
		resp, err := c.Do(req)
		if err != nil {
			return false, ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return true, string(b)
	}

	white := Listener{
		Key:   "w",
		Sites: []Site{{Key: "app", Enable: true, Domains: []string{"app.test"}, Targets: []string{backend.URL}, IPMode: IPModeWhitelist, IPSets: []string{key}, DenyClose: true}},
		Default: Site{Key: "default", Enable: true, Type: TypeProxy, Targets: []string{backend.URL},
			IPMode: IPModeWhitelist, IPSets: []string{key}, DenyClose: true},
	}
	addr, _ := startRunner(t, white, nil, nil)
	n := 0

	// 列表下载成功前注册表里没有内容，按空列表处理，白名单拒绝所有访问
	iplist.Default.Store(key, nil)
	if ok, _ := try(addr, "scan.test"); ok {
		t.Fatal("列表为空时白名单应断开连接")
	}
	n++
	if h := wait(n); !h.Unknown || h.Site != "default" || h.Status != statusClosed {
		t.Fatalf("默认站点拒绝断开应按 444 上报: %+v", h)
	}

	// 列表刷新后包含本机地址：默认站点与普通站点都代理到后端
	setList("# 本机\n127.0.0.0/8\n")
	if ok, body := try(addr, "scan.test"); !ok || body != "backend" {
		t.Fatalf("白名单命中时默认站点应代理到后端: %v %q", ok, body)
	}
	n++
	if h := wait(n); h.Status != http.StatusOK || !h.Unknown {
		t.Fatalf("放行请求的上报不符: %+v", h)
	}
	if ok, body := try(addr, "app.test"); !ok || body != "backend" {
		t.Fatalf("白名单命中时普通站点应代理到后端: %v %q", ok, body)
	}
	n++
	wait(n)

	// 列表再次刷新、不含本机：普通站点断开，但按 403 上报
	setList("10.0.0.0/8\n")
	if ok, _ := try(addr, "app.test"); ok {
		t.Fatal("白名单未命中时普通站点应断开连接")
	}
	n++
	if h := wait(n); h.Unknown || h.Site != "app" || h.Status != http.StatusForbidden {
		t.Fatalf("普通站点拒绝断开应按 403 上报: %+v", h)
	}
	if ok, _ := try(addr, "scan.test"); ok {
		t.Fatal("白名单未命中时默认站点应断开连接")
	}
	n++
	if h := wait(n); !h.Unknown || h.Status != statusClosed {
		t.Fatalf("默认站点拒绝断开应按 444 上报: %+v", h)
	}

	// 黑名单：命中断开，未命中代理
	black := Listener{
		Key: "b",
		Default: Site{Key: "default", Enable: true, Type: TypeProxy, Targets: []string{backend.URL},
			IPMode: IPModeBlacklist, IPSets: []string{key}, DenyClose: true},
	}
	addrB, _ := startRunner(t, black, nil, nil)
	if ok, body := try(addrB, "scan.test"); !ok || body != "backend" {
		t.Fatalf("黑名单未命中时应代理到后端: %v %q", ok, body)
	}
	n++
	wait(n)
	setList("127.0.0.1\n")
	if ok, _ := try(addrB, "scan.test"); ok {
		t.Fatal("黑名单命中时应断开连接")
	}
	n++
	if h := wait(n); h.Listener != "b" || !h.Unknown || h.Status != statusClosed {
		t.Fatalf("默认站点黑名单断开应按 444 上报: %+v", h)
	}

	// 默认站点未开启“拒绝时断开连接”时仍返回 403，上报也是 403
	plain := Listener{
		Key: "p",
		Default: Site{Key: "default", Enable: true, Type: TypeProxy, Targets: []string{backend.URL},
			IPMode: IPModeBlacklist, IPSets: []string{key}},
	}
	addrP, _ := startRunner(t, plain, nil, nil)
	resp, _ := get(t, noRedirect(), addrP, "scan.test", "/", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("未开启断开时应返回 403，得到 %d", resp.StatusCode)
	}
	n++
	if h := wait(n); h.Status != http.StatusForbidden {
		t.Fatalf("返回 403 的请求应按 403 上报: %+v", h)
	}
}
