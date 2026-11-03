package webterminal

import (
	"testing"

	"github.com/wangwanjie/gately/internal/crud"
)

// 复制连接：占位符沿用模板的密码与私钥；主机不变时沿用主机密钥，改了则清空；快速访问目录不与模板共用。
func TestCopyConnection(t *testing.T) {
	src := Connection{QuickAccessDirs: []string{"/a"}}
	src.SSH.Host, src.SSH.Port = "10.0.0.1", 22
	src.SSH.Password, src.SSH.PrivateKey, src.SSH.HostKey = "pw", "pk", "SHA256:x"

	dst := Connection{}
	dst.SSH.Host, dst.SSH.Port = "10.0.0.1", 22
	dst.SSH.Password, dst.SSH.PrivateKey = crud.SecretMask, crud.SecretMask
	copyConnection(&src, &dst)
	if dst.SSH.Password != "pw" || dst.SSH.PrivateKey != "pk" || dst.SSH.HostKey != "SHA256:x" {
		t.Fatalf("应沿用模板的密钥与主机密钥: %+v", dst.SSH)
	}
	dst.QuickAccessDirs[0] = "/changed"
	if src.QuickAccessDirs[0] != "/a" {
		t.Fatal("副本的快速访问目录不应与模板共用")
	}

	dst = Connection{}
	dst.SSH.Host, dst.SSH.Port, dst.SSH.HostKey = "10.0.0.2", 22, "SHA256:x"
	dst.SSH.Password = crud.SecretMask
	copyConnection(&src, &dst)
	if dst.SSH.HostKey != "" {
		t.Fatalf("换了主机应清空主机密钥: %q", dst.SSH.HostKey)
	}
}

// 复制连接与用已保存连接的密钥测试草稿都会把密码发往请求指定的主机，须先通过门禁；
// 通过后副本沿用模板的密码，模板本身不变。
func TestCopyConnectionGate(t *testing.T) {
	m, srv := newModule(t)
	telnetConn(t, m, "t1", "127.0.0.1:2323", "root", "secret")
	draft := Connection{Name: "t1 副本", Type: TypeTelnet, Telnet: TelnetConfig{Host: "127.0.0.2", Port: 2323, Username: "root", Password: crud.SecretMask}}

	m.mu.Lock()
	m.cfg.Security.AgreementAccepted = false
	m.mu.Unlock()
	if code, body := call(t, srv, "POST", "/connections/t1/copy", draft); code != 403 || body["needAgreement"] != true {
		t.Fatalf("未通过门禁时应拒绝复制: %d %v", code, body)
	}
	withKey := draft
	withKey.Key = "t1"
	if code, body := call(t, srv, "POST", "/connections/test", withKey); code != 403 || body["needAgreement"] != true {
		t.Fatalf("未通过门禁时不应用已保存的密码测试草稿: %d %v", code, body)
	}
	// 不带 key 的草稿不涉及已保存的密钥，不受门禁限制（地址不通，只要求不是 403）
	noKey := draft
	noKey.Telnet.Password = "typed"
	noKey.Telnet.Port, noKey.Telnet.Timeout = 1, 1
	if code, _ := call(t, srv, "POST", "/connections/test", noKey); code == 403 {
		t.Fatal("不带 key 的草稿不应受门禁限制")
	}
	m.mu.Lock()
	n := len(m.cfg.Connections)
	m.cfg.Security.AgreementAccepted = true
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("被拒绝的复制不应新增连接: %d", n)
	}

	code, body := call(t, srv, "POST", "/connections/t1/copy", draft)
	if code != 200 {
		t.Fatalf("复制失败: %d %v", code, body)
	}
	if body["telnet"].(map[string]any)["password"] != crud.SecretMask {
		t.Fatalf("响应中的密码应脱敏: %v", body["telnet"])
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Connections) != 2 {
		t.Fatalf("应新增一个连接: %d", len(m.cfg.Connections))
	}
	cp := m.cfg.Connections[1]
	if cp.Key == "t1" || cp.Telnet.Password != "secret" || cp.Telnet.Host != "127.0.0.2" {
		t.Fatalf("副本应沿用模板的密码并使用新地址: %+v", cp.Telnet)
	}
	if m.cfg.Connections[0].Telnet.Host != "127.0.0.1" {
		t.Fatalf("模板不应被修改: %+v", m.cfg.Connections[0].Telnet)
	}
}

// 测试已保存的连接、引用认证预设的草稿都会用到已保存的密钥，须先通过门禁。
func TestTestEndpointsGate(t *testing.T) {
	m, srv := newModule(t)
	telnetConn(t, m, "t1", "127.0.0.1:1", "root", "secret")
	m.mu.Lock()
	m.cfg.Security.AgreementAccepted = false
	m.mu.Unlock()
	if code, body := call(t, srv, "POST", "/connections/t1/test", nil); code != 403 || body["needAgreement"] != true {
		t.Fatalf("测试已保存的连接应受门禁: %d %v", code, body)
	}
	draft := Connection{Name: "x", Type: TypeSSH, SSH: SSHConfig{Host: "127.0.0.2", Port: 22, Username: "root", AuthType: AuthCredential, CredentialKey: "k1"}}
	if code, body := call(t, srv, "POST", "/connections/test", draft); code != 403 || body["needAgreement"] != true {
		t.Fatalf("引用认证预设的草稿应受门禁: %d %v", code, body)
	}
}

// 复制认证预设同样须先通过门禁（协议与二次验证）；通过后占位符沿用模板的密码与口令，新填写的值原样保存。
func TestCopyCredentialGate(t *testing.T) {
	m, srv := newModule(t)
	m.mu.Lock()
	m.cfg.Credentials = []Credential{{Key: "k1", Name: "root", AuthType: "password", Username: "root", Password: "pw", Enable: true}}
	m.cfg.Security.TwoFAEnable, m.cfg.Security.TwoFAKey = true, "JBSWY3DPEHPK3PXP"
	m.mu.Unlock()
	draft := Credential{Name: "root 副本", AuthType: "password", Username: "admin", Password: crud.SecretMask, Passphrase: "new"}
	if code, body := call(t, srv, "POST", "/credentials/k1/copy", draft); code != 403 || body["need2fa"] != true {
		t.Fatalf("未通过二次验证时应拒绝复制: %d %v", code, body)
	}
	m.mu.Lock()
	n := len(m.cfg.Credentials)
	m.cfg.Security.TwoFAEnable = false
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("被拒绝的复制不应新增: %d", n)
	}
	code, body := call(t, srv, "POST", "/credentials/k1/copy", draft)
	if code != 200 || body["password"] != crud.SecretMask {
		t.Fatalf("复制失败或响应未脱敏: %d %v", code, body)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := m.cfg.Credentials[1]
	if cp.Password != "pw" || cp.Passphrase != "new" || cp.Username != "admin" || cp.Key == "k1" {
		t.Fatalf("副本应沿用模板密码、保存新口令: %+v", cp)
	}
}

// 复制连接时 SSH 代理与 Telnet 代理的密码占位符同样沿用模板；新填写的代理密码原样保存。
func TestCopyConnectionProxySecrets(t *testing.T) {
	src := Connection{}
	src.SSH.Proxy.Password, src.Telnet.Proxy.Password, src.SSH.Passphrase = "sp", "tp", "pass"
	dst := Connection{}
	dst.SSH.Proxy.Password, dst.Telnet.Proxy.Password, dst.SSH.Passphrase = crud.SecretMask, "typed", crud.SecretMask
	copyConnection(&src, &dst)
	if dst.SSH.Proxy.Password != "sp" || dst.Telnet.Proxy.Password != "typed" || dst.SSH.Passphrase != "pass" {
		t.Fatalf("代理密码处理不符: ssh=%q telnet=%q passphrase=%q", dst.SSH.Proxy.Password, dst.Telnet.Proxy.Password, dst.SSH.Passphrase)
	}
}
