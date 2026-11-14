package frp

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// authKey 按 frpc 的令牌认证算法计算登录签名：md5(令牌 + 十进制时间戳)
func authKey(token string, ts int64) string {
	sum := md5.Sum([]byte(token + strconv.FormatInt(ts, 10)))
	return hex.EncodeToString(sum[:])
}

func loginBody(addr, privilegeKey string, ts int64) map[string]any {
	return map[string]any{"op": "Login", "content": map[string]any{
		"version": "0.71.1", "hostname": "fnOS-B760", "client_address": addr,
		"privilege_key": privilegeKey, "timestamp": ts,
	}}
}

// trustedNow 返回当前全部登记的可信来源，按地址索引（各测试使用的地址互不相同）。
func trustedNow() map[netip.Addr]trustsrc.Source {
	out := map[netip.Addr]trustsrc.Source{}
	for _, s := range trustsrc.All() {
		out[s.Addr] = s
	}
	return out
}

// 只有令牌签名正确、且通过访问控制的 frpc 登录才把来源地址登记为可信来源
func TestLoginTrustsClientWithValidToken(t *testing.T) {
	const token = "s3cret"
	m := newAccessModule(
		Server{Key: "t1", Name: "t1", ServerSettings: ServerSettings{BindPort: 17000, Token: token}},
		Server{Key: "wl", Name: "wl", ServerSettings: ServerSettings{BindPort: 17001, Token: token, IPMode: iplist.ModeWhitelist, IPList: []string{"203.0.113.0/24"}}},
		Server{Key: "open", Name: "open", ServerSettings: ServerSettings{BindPort: 17002}},
	)
	for _, k := range []string{"t1", "wl", "open"} {
		m.armLearn(k, m.findServer(k).ServerSettings)
		key := k
		t.Cleanup(func() { m.clearServerPorts(key) })
	}
	ts := time.Now().Unix()

	// 签名错误、缺少签名、未运行的实例都不登记
	postPlugin(t, m, "t1", loginBody("198.51.100.1:31541", authKey("wrong", ts), ts))
	postPlugin(t, m, "t1", loginBody("198.51.100.2:31541", "", ts))
	postPlugin(t, m, "ghost", loginBody("198.51.100.3:31541", authKey(token, ts), ts))
	// 实例没有设置令牌时任何人都能登录，不算确认过身份
	postPlugin(t, m, "open", loginBody("198.51.100.4:31541", authKey("", ts), ts))
	// 回环地址（frps 在反向代理之后）不登记
	postPlugin(t, m, "t1", loginBody("127.0.0.1:31541", authKey(token, ts), ts))
	// 被白名单拒绝的登录即使签名正确也不登记
	postPlugin(t, m, "wl", loginBody("198.51.100.5:31541", authKey(token, ts), ts))
	for a := range trustedNow() {
		for _, bad := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3", "198.51.100.4", "198.51.100.5", "127.0.0.1"} {
			if a == netip.MustParseAddr(bad) {
				t.Fatalf("%s 不应登记为可信来源", bad)
			}
		}
	}

	// 签名正确：登记来源地址，端口为 frps 的连接端口，登录后宽限一段时间
	before := time.Now()
	postPlugin(t, m, "t1", loginBody("[::ffff:14.19.55.154]:31541", authKey(token, ts), ts))
	got, ok := trustedNow()[netip.MustParseAddr("14.19.55.154")]
	if !ok || got.Port != 17000 || got.Until.Before(before.Add(trustGrace)) {
		t.Fatalf("签名正确的登录应登记为可信来源: %+v ok=%v", got, ok)
	}
	postPlugin(t, m, "wl", loginBody("203.0.113.9:40000", authKey(token, ts), ts))
	if s, ok := trustedNow()[netip.MustParseAddr("203.0.113.9")]; !ok || s.Port != 17001 {
		t.Fatalf("白名单内且签名正确的登录应登记: %+v ok=%v", s, ok)
	}

	// 封禁的地址照常拒绝登录，也不会登记
	if _, _, err := banlist.Default.Ban("192.0.2.77", banlist.Options{Duration: time.Hour, Force: true, Source: banlist.SourceManual}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban("192.0.2.77") })
	postPlugin(t, m, "t1", loginBody("192.0.2.77:1", authKey(token, ts), ts))
	if _, ok := trustedNow()[netip.MustParseAddr("192.0.2.77")]; ok {
		t.Fatal("被封禁的地址不应登记")
	}

	// 实例停止时撤销
	m.clearServerPorts("t1")
	if _, ok := trustedNow()[netip.MustParseAddr("14.19.55.154")]; ok {
		t.Fatal("实例停止后应撤销可信来源")
	}
}

// 更换连接端口或令牌后撤销已登记的来源；单个实例登记的来源有上限，超出时忘记最早登录的
func TestTrustedClientsResetAndCap(t *testing.T) {
	const token = "tok"
	m := newAccessModule(Server{Key: "t2", Name: "t2", ServerSettings: ServerSettings{BindPort: 17100, Token: token}})
	m.armLearn("t2", ServerSettings{BindPort: 17100, Token: token})
	t.Cleanup(func() { m.clearServerPorts("t2") })
	ts := time.Now().Unix()
	addr := func(i int) string { return fmt.Sprintf("198.18.%d.%d", i/250, i%250+1) }
	for i := 0; i < maxTrusted+3; i++ {
		postPlugin(t, m, "t2", loginBody(addr(i)+":1000", authKey(token, ts), ts))
		// 登录时间相同时无法分辨先后，稍作间隔
		time.Sleep(time.Millisecond)
	}
	got := trustedNow()
	n := 0
	for a := range got {
		if a.Is4() && a.As4()[0] == 198 && a.As4()[1] == 18 {
			n++
		}
	}
	if n != maxTrusted {
		t.Fatalf("登记了 %d 个来源，上限为 %d", n, maxTrusted)
	}
	if _, ok := got[netip.MustParseAddr(addr(0))]; ok {
		t.Fatal("超出上限时应忘记最早登录的来源")
	}
	if _, ok := got[netip.MustParseAddr(addr(maxTrusted+2))]; !ok {
		t.Fatal("最近登录的来源应保留")
	}

	// 只改 allowPorts 不影响已登记的来源
	m.armLearn("t2", ServerSettings{BindPort: 17100, Token: token, AllowPorts: "6000-6010"})
	if _, ok := trustedNow()[netip.MustParseAddr(addr(maxTrusted+2))]; !ok {
		t.Fatal("只改 allowPorts 时不应撤销可信来源")
	}
	m.armLearn("t2", ServerSettings{BindPort: 17100, Token: "new"})
	if _, ok := trustedNow()[netip.MustParseAddr(addr(maxTrusted+2))]; ok {
		t.Fatal("更换令牌后应撤销可信来源")
	}
}

// localNonLoopback 返回一个本机非回环 IPv4 地址，frpc 从它连接 frps 时来源不是回环地址。
func localNonLoopback(t *testing.T) netip.Addr {
	t.Helper()
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
				ip = ip.Unmap()
				if ip.Is4() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
					return ip
				}
			}
		}
	}
	t.Skip("没有可用的非回环 IPv4 地址")
	return netip.Addr{}
}

// 用真实的 frps / frpc 验证签名算法：令牌正确的 frpc 登录后来源地址登记为可信来源，令牌错误的不登记
func TestRealFrpsTrustsClient(t *testing.T) {
	frps, frpc := os.Getenv("GATELY_TEST_FRPS"), os.Getenv("GATELY_TEST_FRPC")
	if frps == "" || frpc == "" {
		t.Skip("未设置 GATELY_TEST_FRPS / GATELY_TEST_FRPC")
	}
	host := localNonLoopback(t)
	ln, err := net.Listen("tcp", net.JoinHostPort(host.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	bind := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	const token = "real-token"
	s := ServerSettings{BindAddr: host.String(), BindPort: bind, Token: token}
	m := newAccessModule(Server{Key: "real-trust", Name: "real-trust", ServerSettings: s})
	gate, err := startLoginGate(m)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.close()
	m.armLearn("real-trust", s)
	t.Cleanup(func() { m.clearServerPorts("real-trust") })
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	srv := write("s.toml", ServerTOML(s)+gate.pluginTOML("real-trust"))
	start := func(bin, cfg string) *exec.Cmd {
		cmd := exec.Command(bin, "-c", cfg)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	start(frps, srv)
	time.Sleep(500 * time.Millisecond)
	cli := func(name, tok string) string {
		return write(name, fmt.Sprintf("serverAddr = %q\nserverPort = %d\nloginFailExit = false\nauth.method = \"token\"\nauth.token = %q\n", host, bind, tok))
	}

	// 令牌错误：frps 拒绝登录，Gately 也不登记
	bad := start(frpc, cli("bad.toml", "wrong"))
	time.Sleep(2 * time.Second)
	if _, ok := trustedNow()[host]; ok {
		t.Fatal("令牌错误的 frpc 不应登记为可信来源")
	}
	_ = bad.Process.Kill()

	start(frpc, cli("good.toml", token))
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := trustedNow()[host]; ok && s.Port == bind {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("令牌正确的 frpc 登录后应把 %s 登记为可信来源", host)
}
