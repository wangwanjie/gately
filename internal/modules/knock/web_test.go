package knock

import (
	"net"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/spa"
)

// webRule 返回只经网页登录放行的规则：没有端口、不拦截防火墙。
func webRule(name string) Rule {
	return Rule{Key: "r-" + name, Name: name, Enable: true}
}

// 没有端口的规则：可以保存；不打开 UDP 监听；敲门报文被拒绝并说明原因。
func TestPortlessRule(t *testing.T) {
	r := webRule("web")
	if err := r.Validate(); err != nil {
		t.Fatalf("不拦截防火墙时端口可以留空: %v", err)
	}
	env := newEnv(t, r)
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()
	env.m.mu.Lock()
	env.m.cfg.Settings.Port = port
	env.m.applyLocked()
	listening := env.m.conn != nil
	env.m.mu.Unlock()
	if listening {
		t.Fatal("只有没有端口的规则时不应监听 UDP")
	}
	if p := env.be.lastPlan(); len(p.Ports) != 0 {
		t.Fatalf("不应拦截任何端口: %+v", p.Ports)
	}
	pkt := env.packet(t, env.rule("r-web"), spa.Message{Access: []spa.Access{{Proto: "tcp", Port: 22}}})
	if err := env.m.handle(pkt, src, transportUDP, "UDP"); err == nil || !strings.Contains(err.Error(), "没有填写端口") {
		t.Fatalf("没有端口的规则应拒绝敲门报文: %v", err)
	}
	if knockgate.Default.Allowed(src, "r-web") {
		t.Fatal("报文不应放行")
	}
}
