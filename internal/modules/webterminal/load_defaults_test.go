package webterminal

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的配置文件。
func initFromRaw(t *testing.T, raw map[string]any) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// 配置文件缺端口、类型、认证方式与代理参数时加载后补上默认值，否则会拨号到端口 0、
// 类型为空的连接不被连接池与 SFTP 当作 SSH 连接。
func TestInitFillsMissingConnectionDefaults(t *testing.T) {
	m := initFromRaw(t, map[string]any{"connections": []any{
		map[string]any{"key": "s", "name": "ssh", "enable": true,
			"ssh": map[string]any{"host": "10.0.0.2", "username": "root", "proxy": map[string]any{"enabled": true, "host": "10.0.0.9"}}},
		map[string]any{"key": "t", "name": "telnet", "enable": true, "type": "telnet",
			"telnet": map[string]any{"host": "10.0.0.3", "proxy": map[string]any{"enabled": true, "type": "http", "host": "10.0.0.9"}}},
	}})
	s := m.cfg.Connections[0]
	if s.Type != TypeSSH || s.SSH.Port != 22 || s.SSH.AuthType != "password" {
		t.Fatalf("SSH 连接未补默认值: type=%q port=%d auth=%q", s.Type, s.SSH.Port, s.SSH.AuthType)
	}
	if p := s.SSH.Proxy; p.Type != ProxySOCKS5 || p.Port != 1080 {
		t.Fatalf("SSH 代理未补默认值: %+v", p)
	}
	tl := m.cfg.Connections[1]
	if tl.Telnet.Port != 23 || tl.Telnet.Proxy.Port != 8080 {
		t.Fatalf("Telnet 连接未补默认值: port=%d proxy=%+v", tl.Telnet.Port, tl.Telnet.Proxy)
	}

	// 运行时：经代理拨号时先连代理的默认端口
	var mu sync.Mutex
	var dialed []string
	old := dialDirect
	dialDirect = func(_ context.Context, addr string, _ time.Duration) (net.Conn, error) {
		mu.Lock()
		dialed = append(dialed, addr)
		mu.Unlock()
		return nil, errors.New("测试中不真正拨号")
	}
	t.Cleanup(func() { dialDirect = old })
	_, _ = dialTarget(context.Background(), s.SSH.Proxy, net.JoinHostPort(s.SSH.Host, strconv.Itoa(s.SSH.Port)), time.Second)
	if len(dialed) != 1 || dialed[0] != "10.0.0.9:1080" {
		t.Fatalf("代理拨号地址不符: %v", dialed)
	}
}

// 已有的值保持不变；未启用的代理与其他类型的设置不补值。
func TestInitKeepsExistingConnectionValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{"connections": []any{
		map[string]any{"key": "s", "name": "ssh", "enable": true, "type": "ssh",
			"ssh": map[string]any{"host": "10.0.0.2", "port": 2222, "username": "root", "authType": "key", "proxy": map[string]any{"enabled": false}}},
		map[string]any{"key": "l", "name": "local", "enable": true, "type": "local"},
	}})
	s := m.cfg.Connections[0]
	if s.SSH.Port != 2222 || s.SSH.AuthType != "key" {
		t.Fatalf("已有值被改动: port=%d auth=%q", s.SSH.Port, s.SSH.AuthType)
	}
	if p := s.SSH.Proxy; p.Type != "" || p.Port != 0 {
		t.Fatalf("未启用的代理不应补值: %+v", p)
	}
	if l := m.cfg.Connections[1]; l.SSH.Port != 0 || l.Telnet.Port != 0 {
		t.Fatalf("本机 Shell 不应补 SSH/Telnet 端口: %+v", l)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameConnectionDefaults(t *testing.T) {
	c := Connection{Name: "ssh", SSH: SSHConfig{Host: "10.0.0.2", Username: "root", Password: "x", Proxy: Proxy{Enabled: true, Host: "10.0.0.9"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Type != TypeSSH || c.SSH.Port != 22 || c.SSH.AuthType != "password" || c.SSH.Proxy.Type != ProxySOCKS5 || c.SSH.Proxy.Port != 1080 {
		t.Fatalf("保存时默认值不符: %+v", c)
	}
	tl := Connection{Name: "t", Type: TypeTelnet, Telnet: TelnetConfig{Host: "10.0.0.3"}}
	if err := tl.Validate(); err != nil {
		t.Fatal(err)
	}
	if tl.Telnet.Port != 23 {
		t.Fatalf("Telnet 保存时默认端口不符: %d", tl.Telnet.Port)
	}
	if err := (&Connection{Name: "bad", SSH: SSHConfig{Host: "h", Username: "u", Port: 70000}}).Validate(); err == nil {
		t.Fatal("端口超出范围应被拒绝")
	}
}
