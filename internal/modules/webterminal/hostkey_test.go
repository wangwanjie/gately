package webterminal

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

// multiKeyServer 是同时持有多把主机密钥的 SSH 服务端（与常见 OpenSSH 一样有 ECDSA、RSA、Ed25519），
// 统计 TCP 连接数与收到的密码认证次数，用于确认主机密钥校验不过时不会发送凭据。
type multiKeyServer struct {
	addr   string
	keys   map[string]ssh.PublicKey
	conns  atomic.Int32
	auths  atomic.Int32
	accept string
}

func newHostSigner(t *testing.T, typ string) ssh.Signer {
	t.Helper()
	var key any
	switch typ {
	case ssh.KeyAlgoED25519:
		_, k, _ := ed25519.GenerateKey(rand.Reader)
		key = k
	case ssh.KeyAlgoECDSA256:
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		key = k
	case ssh.KeyAlgoRSA:
		k, _ := rsa.GenerateKey(rand.Reader, 2048)
		key = k
	default:
		t.Fatalf("未知类型 %s", typ)
	}
	s, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// startMultiKeyServer 启动持有指定类型主机密钥的服务端，接受密码 "pw"。
func startMultiKeyServer(t *testing.T, types ...string) *multiKeyServer {
	t.Helper()
	s := &multiKeyServer{keys: map[string]ssh.PublicKey{}, accept: "pw"}
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
		s.auths.Add(1)
		if string(pw) == s.accept {
			return nil, nil
		}
		return nil, errors.New("密码错误")
	}}
	for _, typ := range types {
		signer := newHostSigner(t, typ)
		cfg.AddHostKey(signer)
		s.keys[typ] = signer.PublicKey()
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s.addr = ln.Addr().String()
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			s.conns.Add(1)
			go func() {
				defer nc.Close()
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					_ = nch.Reject(ssh.Prohibited, "测试服务端不提供通道")
				}
				_ = sc.Close()
			}()
		}
	}()
	return s
}

func (s *multiKeyServer) fp(typ string) string {
	return Fingerprint(s.keys[typ])
}

func (s *multiKeyServer) conn(t *testing.T, hostKey, hostKeyType string) *Connection {
	t.Helper()
	host, port, _ := net.SplitHostPort(s.addr)
	p, _ := strconv.Atoi(port)
	return &Connection{Key: "hk", Name: "multi", Type: TypeSSH, Enable: true, SSH: SSHConfig{
		Host: host, Port: p, Username: "root", AuthType: "password", Password: "pw",
		HostKey: hostKey, HostKeyType: hostKeyType, Timeout: 5,
	}}
}

func dialTest(t *testing.T, c *Connection) (hostKeyInfo, error) {
	t.Helper()
	client, hk, err := connectSSH(context.Background(), c)
	if client != nil {
		client.Close()
	}
	return hk, err
}

// 只记录了指纹（旧版本或用 ssh-keyscan 取的 Ed25519）时，x/crypto 默认协商 ECDSA 也能找到记录的那把并连上。
func TestHostKeyLegacyFingerprintOtherType(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSA, ssh.KeyAlgoED25519)
	for _, typ := range []string{ssh.KeyAlgoED25519, ssh.KeyAlgoRSA, ssh.KeyAlgoECDSA256} {
		hk, err := dialTest(t, srv.conn(t, srv.fp(typ), ""))
		if err != nil {
			t.Fatalf("记录 %s 指纹应能连接: %v", typ, err)
		}
		if hk.Fingerprint != srv.fp(typ) || hk.Type != typ || hk.New {
			t.Fatalf("应返回记录的那把密钥 %s: %+v", typ, hk)
		}
	}
}

// 记录了类型后只协商这种密钥，一次握手即可。
func TestHostKeyTypeRecordedSingleHandshake(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSA, ssh.KeyAlgoED25519)
	for _, typ := range []string{ssh.KeyAlgoED25519, ssh.KeyAlgoRSA, ssh.KeyAlgoECDSA256} {
		before := srv.conns.Load()
		if _, err := dialTest(t, srv.conn(t, srv.fp(typ), typ)); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if n := srv.conns.Load() - before; n != 1 {
			t.Fatalf("记录类型 %s 后应只握手一次，实际 %d 次", typ, n)
		}
	}
}

// 指纹与服务器所有主机密钥都不符时报错，且整个过程中不发送密码。
func TestHostKeyMismatchNoCredentialsSent(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSA, ssh.KeyAlgoED25519)
	forged := Fingerprint(newHostSigner(t, ssh.KeyAlgoED25519).PublicKey())
	for _, typ := range []string{"", ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256} {
		before := srv.conns.Load()
		_, err := dialTest(t, srv.conn(t, forged, typ))
		var mm *hostKeyMismatch
		if !errors.As(err, &mm) || !strings.Contains(err.Error(), "主机密钥已变化") || !strings.Contains(err.Error(), forged) {
			t.Fatalf("类型 %q：应报告主机密钥已变化: %v", typ, err)
		}
		// 报告的是第一次协商到的密钥（类型已记录时就是该类型），与 ssh-keygen -l 的类型写法一致
		if typ == ssh.KeyAlgoED25519 && !strings.Contains(err.Error(), "ED25519 "+srv.fp(ssh.KeyAlgoED25519)) {
			t.Fatalf("应带类型报告当前指纹: %v", err)
		}
		// 每种类型最多握手一次，不会无限重试
		if n := srv.conns.Load() - before; n > int32(len(hostKeyFormats)) {
			t.Fatalf("握手次数过多: %d", n)
		}
	}
	if n := srv.auths.Load(); n != 0 {
		t.Fatalf("主机密钥校验不过时不应发送密码，实际收到 %d 次", n)
	}
}

// 记录的类型服务器已不再提供（换了密钥）：报告指纹不符，而不是晦涩的算法协商失败。
func TestHostKeyRecordedTypeGone(t *testing.T) {
	old := startMultiKeyServer(t, ssh.KeyAlgoED25519)
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256)
	_, err := dialTest(t, srv.conn(t, old.fp(ssh.KeyAlgoED25519), ssh.KeyAlgoED25519))
	var mm *hostKeyMismatch
	if !errors.As(err, &mm) || mm.got != srv.fp(ssh.KeyAlgoECDSA256) {
		t.Fatalf("应报告当前为 ECDSA 密钥: %v", err)
	}
	if srv.auths.Load() != 0 {
		t.Fatal("不应发送密码")
	}
}

// 记录的类型与指纹不对应（手工改过）时仍以指纹为准找到正确的密钥，连接后补正类型。
func TestHostKeyStaleTypeCorrected(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoED25519)
	m, _ := newModule(t)
	c := srv.conn(t, srv.fp(ssh.KeyAlgoECDSA256), ssh.KeyAlgoED25519)
	m.cfg.Connections = []Connection{*c}
	client, hk, err := m.dialSSH(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	m.recordHostKey(c.Key, hk)
	if got := m.cfg.Connections[0].SSH; got.HostKeyType != ssh.KeyAlgoECDSA256 || got.HostKey != srv.fp(ssh.KeyAlgoECDSA256) {
		t.Fatalf("应补正类型: %+v", got)
	}
}

// 认证失败发生在主机密钥校验通过之后：直接返回认证错误，不再换类型重试。
func TestHostKeyAuthFailureNotRetried(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSA, ssh.KeyAlgoED25519)
	srv.accept = "other"
	before := srv.conns.Load()
	_, err := dialTest(t, srv.conn(t, srv.fp(ssh.KeyAlgoED25519), ""))
	if err == nil || strings.Contains(err.Error(), "主机密钥") || !strings.Contains(err.Error(), "unable to authenticate") {
		t.Fatalf("应返回认证错误: %v", err)
	}
	// ECDSA 不符、RSA 不符、Ed25519 匹配后认证失败，共 3 次
	if n := srv.conns.Load() - before; n != 3 {
		t.Fatalf("握手次数应为 3，实际 %d", n)
	}
}

// 经终端连接（连接池）后：首次连接记录指纹与类型；旧配置只有指纹时补记类型，下次只握手一次。
func TestHostKeyRecordAndBackfillViaPool(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSA, ssh.KeyAlgoED25519)
	m, _ := newModule(t)

	// 首次信任
	m.cfg.Connections = []Connection{*srv.conn(t, "", "")}
	_, release, err := m.acquireSSH(context.Background(), "hk")
	if err != nil {
		t.Fatal(err)
	}
	release()
	m.pool.drop("hk")
	got := m.cfg.Connections[0].SSH
	if got.HostKey != srv.fp(ssh.KeyAlgoECDSA256) || got.HostKeyType != ssh.KeyAlgoECDSA256 {
		t.Fatalf("首次连接应记录协商到的密钥及类型: %+v", got)
	}

	// 旧配置：只有 Ed25519 指纹
	m.mu.Lock()
	m.cfg.Connections[0].SSH.HostKey, m.cfg.Connections[0].SSH.HostKeyType = srv.fp(ssh.KeyAlgoED25519), ""
	m.mu.Unlock()
	_, release, err = m.acquireSSH(context.Background(), "hk")
	if err != nil {
		t.Fatal(err)
	}
	release()
	m.pool.drop("hk")
	got = m.cfg.Connections[0].SSH
	if got.HostKey != srv.fp(ssh.KeyAlgoED25519) || got.HostKeyType != ssh.KeyAlgoED25519 {
		t.Fatalf("应补记类型且不改指纹: %+v", got)
	}
	before := srv.conns.Load()
	_, release, err = m.acquireSSH(context.Background(), "hk")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if n := srv.conns.Load() - before; n != 1 {
		t.Fatalf("补记类型后应只握手一次，实际 %d", n)
	}
}

// recordHostKey 只在记录与本次校验通过的指纹一致时改动；连接期间被重置或改成别的指纹不受影响。
func TestRecordHostKeyGuards(t *testing.T) {
	m, _ := newModule(t)
	m.cfg.Connections = []Connection{{Key: "a", Type: TypeSSH, SSH: SSHConfig{HostKey: "SHA256:other"}}}
	m.recordHostKey("a", hostKeyInfo{Fingerprint: "SHA256:x", Type: ssh.KeyAlgoED25519})
	if s := m.cfg.Connections[0].SSH; s.HostKey != "SHA256:other" || s.HostKeyType != "" {
		t.Fatalf("指纹不同不应改动: %+v", s)
	}
	// 连接时已有记录（New=false），期间被重置为空：不应借机把这次的指纹记回去
	m.cfg.Connections[0].SSH.HostKey = ""
	m.recordHostKey("a", hostKeyInfo{Fingerprint: "SHA256:x", Type: ssh.KeyAlgoED25519})
	if s := m.cfg.Connections[0].SSH; s.HostKey != "" {
		t.Fatalf("重置后不应写回: %+v", s)
	}
}

// 编辑连接时主机密钥字段的合并规则。
func TestMergeHostKeyType(t *testing.T) {
	old := Connection{SSH: SSHConfig{Host: "h", Port: 22, HostKey: "SHA256:a", HostKeyType: ssh.KeyAlgoED25519}}
	cases := []struct {
		name         string
		upd          SSHConfig
		wantFP, want string
	}{
		{"旧前端不带类型", SSHConfig{Host: "h", Port: 22, HostKey: "SHA256:a"}, "SHA256:a", ssh.KeyAlgoED25519},
		{"未带指纹", SSHConfig{Host: "h", Port: 22}, "SHA256:a", ssh.KeyAlgoED25519},
		{"换主机", SSHConfig{Host: "h2", Port: 22, HostKey: "SHA256:a", HostKeyType: ssh.KeyAlgoED25519}, "", ""},
		{"换端口", SSHConfig{Host: "h", Port: 2222}, "", ""},
		{"改指纹沿用旧类型", SSHConfig{Host: "h", Port: 22, HostKey: "SHA256:b", HostKeyType: ssh.KeyAlgoED25519}, "SHA256:b", ""},
		{"改指纹并给出类型", SSHConfig{Host: "h", Port: 22, HostKey: "SHA256:b", HostKeyType: ssh.KeyAlgoRSA}, "SHA256:b", ssh.KeyAlgoRSA},
	}
	for _, tc := range cases {
		upd := Connection{SSH: tc.upd}
		mergeSecrets(&old, &upd)
		if upd.SSH.HostKey != tc.wantFP || upd.SSH.HostKeyType != tc.want {
			t.Fatalf("%s: 得到 %q/%q", tc.name, upd.SSH.HostKey, upd.SSH.HostKeyType)
		}
	}
}

// 重置主机密钥同时清空类型；测试连接不记录也不补记。
func TestResetAndTestDoNotKeepType(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoED25519)
	m, ts := newModule(t)
	m.cfg.Connections = []Connection{*srv.conn(t, srv.fp(ssh.KeyAlgoED25519), "")}

	resp, err := http.Post(ts.URL+"/api/webterminal/connections/hk/test", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s := m.cfg.Connections[0].SSH; s.HostKeyType != "" {
		t.Fatalf("测试连接不应补记类型: %+v", s)
	}

	m.cfg.Connections[0].SSH.HostKeyType = ssh.KeyAlgoED25519
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/webterminal/connections/hk/hostkey", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s := m.cfg.Connections[0].SSH; s.HostKey != "" || s.HostKeyType != "" {
		t.Fatalf("重置应清空指纹与类型: %+v", s)
	}
}

// 经接口写入的未知类型不用于限定协商，按未记录类型探测后照常连接。
func TestHostKeyUnknownTypeIgnored(t *testing.T) {
	srv := startMultiKeyServer(t, ssh.KeyAlgoECDSA256, ssh.KeyAlgoED25519)
	hk, err := dialTest(t, srv.conn(t, srv.fp(ssh.KeyAlgoED25519), "bogus-algo"))
	if err != nil || hk.Type != ssh.KeyAlgoED25519 {
		t.Fatalf("未知类型应忽略并探测到 Ed25519: %+v %v", hk, err)
	}
}

// TestHostKeyTypeLabel 验证主机密钥类型的短名：普通密钥与 ssh-keygen -l 一致，主机证书加 -CERT 后缀，未知类型原样显示。
func TestHostKeyTypeLabel(t *testing.T) {
	cases := map[string]string{
		ssh.KeyAlgoED25519:         "ED25519",
		ssh.KeyAlgoRSA:             "RSA",
		ssh.KeyAlgoECDSA256:        "ECDSA",
		ssh.KeyAlgoECDSA521:        "ECDSA",
		ssh.CertAlgoED25519v01:     "ED25519-CERT",
		ssh.CertAlgoRSAv01:         "RSA-CERT",
		ssh.CertAlgoECDSA384v01:    "ECDSA-CERT",
		"foo-cert-v01@openssh.com": "foo-cert-v01@openssh.com",
		"ssh-unknown":              "ssh-unknown",
	}
	for typ, want := range cases {
		if got := hostKeyTypeLabel(typ); got != want {
			t.Errorf("hostKeyTypeLabel(%q) = %q，期望 %q", typ, got, want)
		}
	}
}
