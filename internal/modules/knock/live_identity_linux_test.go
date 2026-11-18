//go:build linux

package knock

// 身份的真实防火墙测试：与 TestLive 相同，只在设置 GATELY_KNOCK_LIVE=1 时运行，必须在独立的网络命名空间中运行。
// 模块整体运行在真实后端（nftables 与 iptables + ipset 各一遍）上，客户端在 knockcli 命名空间中用不同的源地址建连，
// 验证身份敲门、单地址放行、编辑规则时只撤销失效身份，以及停止模块后恢复访问。
//
//	GATELY_KNOCK_LIVE=1 ./knock.test -test.run TestLiveIdentities -test.v

import (
	"context"
	"crypto/rand"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/spa"
)

// 客户端命名空间中的第二个 IPv4 地址，模拟另一个人
const cli4b = "10.99.0.3"

// dialFromNSSrc 在客户端命名空间中以 src 为源地址建立连接。
func dialFromNSSrc(addr, src string) (net.Conn, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	orig, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		return nil, err
	}
	defer orig.Close()
	target, err := os.Open("/var/run/netns/" + liveNS)
	if err != nil {
		return nil, err
	}
	defer target.Close()
	if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
		return nil, err
	}
	defer unix.Setns(int(orig.Fd()), unix.CLONE_NEWNET)
	d := net.Dialer{Timeout: 700 * time.Millisecond, LocalAddr: &net.TCPAddr{IP: net.ParseIP(src)}}
	return d.Dial("tcp", addr)
}

func TestLiveIdentities(t *testing.T) {
	if os.Getenv("GATELY_KNOCK_LIVE") != "1" {
		t.Skip("设置 GATELY_KNOCK_LIVE=1 在独立网络命名空间中运行")
	}
	setupNetns(t)
	sh(t, "ip netns exec "+liveNS+" ip addr add "+cli4b+"/24 dev kn1")
	echoServer(t, srv4)
	echoServer(t, srv6)
	ctx := context.Background()
	var backends []backend
	if b, _, _ := detectBackend(ctx); b != nil && b.kind() == BackendNFT {
		backends = append(backends, b)
	}
	if b := findIPT(ctx); b != nil {
		backends = append(backends, b)
	}
	if len(backends) < 2 {
		t.Logf("只找到 %d 个后端", len(backends))
	}
	for _, be := range backends {
		t.Run(be.kind(), func(t *testing.T) { liveIdentities(t, be) })
	}
}

func liveIdentities(t *testing.T, be backend) {
	env := newEnv(t)
	env.m.fw.detect = func(context.Context) (backend, string, string) { return be, "", "" }
	// 测试时钟固定，防火墙写入的到期时间按真实时间计算：改回系统时钟
	env.m.now = time.Now
	env.m.fw.now = time.Now
	env.clock.set(time.Now())
	knockgate.Default.SetClock(time.Now)
	env.setStartedAt(time.Now().Add(-time.Minute))
	srv := env.serve(t)
	base := srv.URL + "/api/knock"
	// 客户端地址都是私有地址：关闭“内网免敲门”
	if code, resp := doJSON(t, http.MethodPut, base+"/settings", Settings{Port: 0, AllowList: []string{}, MaxAge: 120}); code != http.StatusOK {
		t.Fatalf("保存设置: %d %s", code, resp.Error)
	}
	rule := Rule{Name: "live", Enable: true, Ports: []string{"tcp/2222"}, Firewall: true, Timeout: 120, SingleAddr: true,
		Identities: []Identity{{Name: "alice", Enable: true}, {Name: "bob", Enable: true}}}
	if code, resp := doJSON(t, http.MethodPost, base+"/rules", rule); code != http.StatusOK {
		t.Fatalf("新增规则: %d %s", code, resp.Error)
	}
	var list []RuleView
	getJSON(t, base+"/rules", &list)
	key := list[0].Key
	waitFor(t, "防火墙拦截生效", func() bool { return env.m.fw.snapshot().Active })
	if got := env.m.fw.snapshot().Backend; got != be.kind() {
		t.Fatalf("后端应为 %s: %s", be.kind(), got)
	}

	addr4 := net.JoinHostPort(srv4, "2222")
	addr6 := net.JoinHostPort(srv6, "2222")
	can := func(addr, src string) bool {
		c, err := dialFromNSSrc(addr, src)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}
	expect := func(what string, want map[string]bool) {
		t.Helper()
		// 防火墙由后台协程写入：等待最多 3 秒
		deadline := time.Now().Add(3 * time.Second)
		for {
			ok := true
			got := map[string]bool{}
			for src, w := range want {
				addr := addr4
				if netip.MustParseAddr(src).Is6() {
					addr = addr6
				}
				got[src] = can(addr, src)
				if got[src] != w {
					ok = false
				}
			}
			if ok {
				t.Logf("%s：%v", what, got)
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s：期望 %v，实际 %v", what, want, got)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	knock := func(name, addr string) {
		t.Helper()
		r := env.rule(key)
		var k spa.Key
		for _, id := range r.Identities {
			if id.Name == name {
				k, _ = id.spaKey(r.HMACType)
			}
		}
		a := netip.MustParseAddr(addr)
		pkt, err := spa.Encode(spa.Message{User: "u", Timestamp: time.Now().Unix(), Type: spa.AccessMsg, Addr: a,
			Access: []spa.Access{{Proto: "tcp", Port: livePort}}}, k, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.m.handle(pkt, a, transportUDP, "UDP"); err != nil {
			t.Fatalf("%s 从 %s 敲门: %v", name, addr, err)
		}
	}

	expect("敲门前全部拦截", map[string]bool{cli4: false, cli4b: false, cli6: false})
	knock("alice", cli4)
	expect("alice 敲门后只放行 alice 的地址", map[string]bool{cli4: true, cli4b: false, cli6: false})
	held, err := dialFromNSSrc(addr4, cli4)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	knock("bob", cli4b)
	expect("bob 敲门后两人都放行", map[string]bool{cli4: true, cli4b: true, cli6: false})
	knock("alice", cli6)
	expect("单地址放行：alice 换到 IPv6 后旧地址撤销，bob 不受影响", map[string]bool{cli4: false, cli4b: true, cli6: true})
	if !echoOK(held) {
		t.Fatal("撤销前已建立的连接应保持")
	}

	// 停用 bob：只撤销 bob，alice 的放行在防火墙重建后仍在
	upd := env.rule(key)
	upd.Identities[1].Enable = false
	if code, resp := doJSON(t, http.MethodPut, base+"/rules/"+key, upd); code != http.StatusOK {
		t.Fatalf("停用 bob: %d %s", code, resp.Error)
	}
	expect("停用 bob 后", map[string]bool{cli4: false, cli4b: false, cli6: true})

	// 更换 alice 的密钥：alice 的放行撤销
	upd = env.rule(key)
	upd.Identities[0].EncKey, upd.Identities[0].HMACKey = "", ""
	if code, resp := doJSON(t, http.MethodPut, base+"/rules/"+key, upd); code != http.StatusOK {
		t.Fatalf("更换 alice 密钥: %d %s", code, resp.Error)
	}
	expect("更换 alice 密钥后", map[string]bool{cli4: false, cli4b: false, cli6: false})

	// 新密钥敲门，再从管理端撤销
	knock("alice", cli4b)
	expect("alice 用新密钥敲门", map[string]bool{cli4: false, cli4b: true, cli6: false})
	if code, resp := doJSON(t, http.MethodDelete, base+"/grants", map[string]string{"addr": cli4b, "rule": key}); code != http.StatusOK {
		t.Fatalf("撤销: %d %s", code, resp.Error)
	}
	expect("管理端撤销后", map[string]bool{cli4b: false})

	// 停止模块后删除拦截规则，恢复访问
	if err := env.m.Stop(); err != nil {
		t.Fatal(err)
	}
	expect("停止模块后恢复访问", map[string]bool{cli4: true, cli4b: true, cli6: true})
}
