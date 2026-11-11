//go:build linux

package knock

// 真实防火墙测试：只在设置 GATELY_KNOCK_LIVE=1 时运行，需要 root、nft、iptables、ipset 与 iproute2。
// 必须在独立的网络命名空间中运行（例如特权容器内），测试会创建 veth 与名为 knockcli 的命名空间，
// 并写入 nftables / iptables 规则，不要在宿主机网络中运行。
//
//	GATELY_KNOCK_LIVE=1 ./knock.test -test.run TestLive -test.v

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/wangwanjie/gately/internal/spa"
)

const (
	liveNS   = "knockcli"
	srv4     = "10.99.0.1"
	cli4     = "10.99.0.2"
	srv6     = "fd99::1"
	cli6     = "fd99::2"
	livePort = 2222
)

func sh(t *testing.T, cmd string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", cmd, err, out)
	}
	return string(out)
}

func setupNetns(t *testing.T) {
	t.Helper()
	exec.Command("ip", "netns", "del", liveNS).Run()
	exec.Command("ip", "link", "del", "kn0").Run()
	sh(t, "ip netns add "+liveNS)
	sh(t, "ip link add kn0 type veth peer name kn1")
	sh(t, "ip link set kn1 netns "+liveNS)
	sh(t, "ip addr add "+srv4+"/24 dev kn0 && ip -6 addr add "+srv6+"/64 dev kn0 nodad && ip link set kn0 up")
	sh(t, "ip netns exec "+liveNS+" sh -c 'ip addr add "+cli4+"/24 dev kn1 && ip -6 addr add "+cli6+"/64 dev kn1 nodad && ip link set kn1 up && ip link set lo up'")
	t.Cleanup(func() {
		exec.Command("ip", "netns", "del", liveNS).Run()
		exec.Command("ip", "link", "del", "kn0").Run()
	})
}

// dialFromNS 在客户端命名空间中建立连接：锁定线程切换命名空间，连接建立后切回。
func dialFromNS(addr string) (net.Conn, error) {
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
	return net.DialTimeout("tcp", addr, 700*time.Millisecond)
}

// echoServer 在服务端地址上监听，回显收到的数据。
func echoServer(t *testing.T, host string) {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "2222"))
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
			go func() {
				buf := make([]byte, 64)
				for {
					n, err := c.Read(buf)
					if err != nil {
						c.Close()
						return
					}
					c.Write(buf[:n])
				}
			}()
		}
	}()
}

func echoOK(c net.Conn) bool {
	c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte("ping")); err != nil {
		return false
	}
	buf := make([]byte, 4)
	n, err := c.Read(buf)
	return err == nil && string(buf[:n]) == "ping"
}

func TestLive(t *testing.T) {
	if os.Getenv("GATELY_KNOCK_LIVE") != "1" {
		t.Skip("设置 GATELY_KNOCK_LIVE=1 在独立网络命名空间中运行")
	}
	setupNetns(t)
	echoServer(t, srv4)
	echoServer(t, srv6)
	ctx := context.Background()
	backends := map[string]backend{}
	if bin := lookCmd("nft"); bin != "" {
		b, reason, msg := detectBackend(ctx)
		if b == nil || b.kind() != BackendNFT {
			t.Fatalf("应使用 nftables: %s %s", reason, msg)
		}
		backends[BackendNFT] = b
	}
	if b := findIPT(ctx); b != nil {
		backends[BackendIPT] = b
	}
	if len(backends) == 0 {
		t.Fatal("没有可用的后端")
	}
	for name, be := range backends {
		t.Run(name, func(t *testing.T) { liveBackend(t, be) })
	}
}

func liveBackend(t *testing.T, be backend) {
	ctx := context.Background()
	t.Cleanup(func() { be.remove(ctx) })
	addr4 := net.JoinHostPort(srv4, "2222")
	addr6 := net.JoinHostPort(srv6, "2222")
	// 规则写入前可以直接连接
	pre, err := dialFromNS(addr4)
	if err != nil {
		t.Fatalf("写规则前应能连接: %v", err)
	}
	defer pre.Close()
	acc := spa.Access{Proto: "tcp", Port: livePort}
	plan := fwPlan{Ports: []spa.Access{acc, {Proto: "udp", Port: 51820}}, Exempt: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}}
	if err := be.apply(ctx, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	if ok, _, err := be.check(ctx); !ok || err != nil {
		t.Fatalf("写入后检查应通过: %v %v", ok, err)
	}
	// 新连接被拦截，已建立的连接不受影响
	if c, err := dialFromNS(addr4); err == nil {
		c.Close()
		t.Fatal("IPv4 新连接应被拦截")
	}
	if c, err := dialFromNS(addr6); err == nil {
		c.Close()
		t.Fatal("IPv6 新连接应被拦截")
	}
	if !echoOK(pre) {
		t.Fatal("写规则前建立的连接应保持")
	}
	// 本机回环不拦截
	if c, err := net.DialTimeout("tcp", addr4, time.Second); err != nil {
		t.Fatalf("本机访问不应被拦截: %v", err)
	} else {
		c.Close()
	}
	// 敲门放行 3 秒
	now := time.Now()
	grants := []fwGrant{
		{Addr: netip.MustParseAddr(cli4), Access: acc, Until: now.Add(3 * time.Second)},
		{Addr: netip.MustParseAddr(cli6), Access: acc, Until: now.Add(3 * time.Second)},
	}
	if err := be.add(ctx, grants, now); err != nil {
		t.Fatal(err)
	}
	// 再写一次（刷新超时）不应出错
	if err := be.add(ctx, grants, now); err != nil {
		t.Fatalf("重复写入放行: %v", err)
	}
	c4, err := dialFromNS(addr4)
	if err != nil {
		t.Fatalf("放行后 IPv4 应能连接: %v", err)
	}
	defer c4.Close()
	c6, err := dialFromNS(addr6)
	if err != nil {
		t.Fatalf("放行后 IPv6 应能连接: %v", err)
	}
	c6.Close()
	text, err := be.text(ctx)
	if err != nil || !strings.Contains(text, cli4) {
		t.Fatalf("规则文本应包含放行地址: %v\n%s", err, text)
	}
	// 到期后新连接再次被拦截，已建立的连接照常
	time.Sleep(3500 * time.Millisecond)
	if c, err := dialFromNS(addr4); err == nil {
		c.Close()
		t.Fatal("放行到期后新连接应被拦截")
	}
	if !echoOK(c4) {
		t.Fatal("到期前建立的连接应保持")
	}
	_, counts, _ := be.check(ctx)
	if counts.dropped == 0 || counts.allowed == 0 {
		t.Fatalf("计数应有拦截与放行: %+v", counts)
	}
	t.Logf("计数 %+v", counts)
	// 撤销
	if err := be.add(ctx, grants[:1], time.Now().Add(-time.Second+0)); err != nil {
		t.Fatal(err)
	}
	g := fwGrant{Addr: netip.MustParseAddr(cli4), Access: acc, Until: time.Now().Add(time.Minute)}
	if err := be.add(ctx, []fwGrant{g}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if c, err := dialFromNS(addr4); err != nil {
		t.Fatalf("重新放行后应能连接: %v", err)
	} else {
		c.Close()
	}
	if err := be.del(ctx, []fwGrant{g}); err != nil {
		t.Fatal(err)
	}
	if err := be.del(ctx, []fwGrant{g}); err != nil {
		t.Fatalf("删除不存在的放行不应出错: %v", err)
	}
	if c, err := dialFromNS(addr4); err == nil {
		c.Close()
		t.Fatal("撤销后应被拦截")
	}
	// 重建时带上放行
	plan.Grants = []fwGrant{g}
	if err := be.apply(ctx, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	if c, err := dialFromNS(addr4); err != nil {
		t.Fatalf("重建后放行应保留: %v", err)
	} else {
		c.Close()
	}
	// 外部删除后检查失败
	if be.kind() == BackendNFT {
		sh(t, "nft delete table inet "+nftTable)
	} else {
		sh(t, "iptables-legacy -w -t mangle -D PREROUTING -j "+iptChain+" 2>/dev/null || iptables -w -t mangle -D PREROUTING -j "+iptChain)
	}
	if ok, _, _ := be.check(ctx); ok {
		t.Fatal("规则被外部删除后检查应失败")
	}
	if err := be.apply(ctx, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := be.remove(ctx); err != nil {
		t.Fatal(err)
	}
	if c, err := dialFromNS(addr4); err != nil {
		t.Fatalf("删除规则后应恢复访问: %v", err)
	} else {
		c.Close()
	}
	if ok, _, _ := be.check(ctx); ok {
		t.Fatal("删除后检查应失败")
	}
}
