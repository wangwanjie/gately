package ssl

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

// selfSigned 生成自签名证书，用于不依赖 CA 的测试。
func selfSigned(t *testing.T, domains []string, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: domains[0]},
		DNSNames:     domains,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}))
}

func newTestModule(t *testing.T) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(500, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestValidate(t *testing.T) {
	s := defaultSettings()
	ok := Cert{Source: SourceACME, ACME: ACMEConfig{Domains: []string{" Example.com ", "*.example.com", "example.com"}, Email: "a@b.c", Provider: "alidns"}}
	if err := ok.Validate(s); err != nil {
		t.Fatal(err)
	}
	if len(ok.ACME.Domains) != 2 || ok.ACME.KeyType != "P256" || ok.ACME.CA != "letsencrypt" || ok.ACME.PropagationTimeout != 300 {
		t.Fatalf("默认值补全或去重错误: %+v", ok.ACME)
	}
	certPEM, keyPEM := selfSigned(t, []string{"a.com"}, time.Now(), time.Now().Add(time.Hour))
	_, otherKey := selfSigned(t, []string{"b.com"}, time.Now(), time.Now().Add(time.Hour))
	bad := []Cert{
		{Source: "x"},
		{Source: SourceACME, ACME: ACMEConfig{Email: "a@b.c", Provider: "alidns"}},
		{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"a.*.com"}, Email: "a@b.c", Provider: "alidns"}},
		{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"a.com"}, Provider: "alidns"}},
		{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"a.com"}, Email: "a@b.c", Provider: "duckdns"}},
		{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"a.com"}, Email: "a@b.c", Provider: "alidns", CA: "zerossl"}},
		{Source: SourceUpload, CertPEM: certPEM, KeyPEM: otherKey},
		{Source: SourceUpload, CertPEM: certPEM, KeyPEM: keyPEM, Mapping: Mapping{Enable: true, Dir: "/tmp"}},
		{Source: SourceUpload, CertPEM: certPEM, KeyPEM: keyPEM, Remark: "../x"},
		{Source: SourceFile, CertFile: "rel.pem", KeyFile: "/k"},
	}
	for i, c := range bad {
		if err := c.Validate(s); err == nil {
			t.Errorf("第 %d 个非法配置未报错", i)
		}
	}
	up := Cert{Source: SourceUpload, CertPEM: certPEM, KeyPEM: keyPEM, Remark: "a.com", Mapping: Mapping{Enable: true, Dir: "/tmp/x"}}
	if err := up.Validate(s); err != nil {
		t.Fatal(err)
	}
}

func TestNeedsRenewal(t *testing.T) {
	m := newTestModule(t)
	now := time.Now()
	mk := func(life, left time.Duration) *Cert {
		c, k := selfSigned(t, []string{"a.com"}, now.Add(left-life), now.Add(left))
		return &Cert{Source: SourceACME, CertPEM: c, KeyPEM: k, ACME: ACMEConfig{Domains: []string{"a.com"}}}
	}
	cases := []struct {
		name string
		c    *Cert
		want bool
	}{
		{"未签发", &Cert{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"a.com"}}}, true},
		{"90 天证书剩 60 天", mk(90*24*time.Hour, 60*24*time.Hour), false},
		{"90 天证书剩 20 天", mk(90*24*time.Hour, 20*24*time.Hour), true},
		{"6 天短期证书剩 2 天", mk(6*24*time.Hour, 48*time.Hour), false},
		{"6 天短期证书剩 10 小时", mk(6*24*time.Hour, 10*time.Hour), true},
	}
	for _, c := range cases {
		if got, _ := m.needsRenewal(c.c, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	changed := mk(90*24*time.Hour, 60*24*time.Hour)
	changed.ACME.Domains = []string{"a.com", "b.com"}
	if got, _ := m.needsRenewal(changed, now); !got {
		t.Error("域名变化后应重新签发")
	}
}

func TestGetCertificate(t *testing.T) {
	m := newTestModule(t)
	now := time.Now()
	c1, k1 := selfSigned(t, []string{"example.com", "www.example.com"}, now, now.Add(24*time.Hour))
	c2, k2 := selfSigned(t, []string{"*.example.com"}, now, now.Add(24*time.Hour))
	c3, k3 := selfSigned(t, []string{"old.example.org"}, now.Add(-48*time.Hour), now.Add(-time.Hour))
	m.cfg.Certs = []Cert{
		{Key: "1", Enable: true, Source: SourceUpload, CertPEM: c1, KeyPEM: k1},
		{Key: "2", Enable: true, Source: SourceUpload, CertPEM: c2, KeyPEM: k2},
		{Key: "3", Enable: true, Source: SourceUpload, CertPEM: c3, KeyPEM: k3},
	}
	m.rebuildTLSLocked()
	leafName := func(host string) string {
		c, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: host})
		if err != nil {
			return ""
		}
		leaf, _ := x509.ParseCertificate(c.Certificate[0])
		return leaf.DNSNames[0]
	}
	if leafName("www.example.com") != "example.com" {
		t.Error("精确匹配应优先")
	}
	if leafName("nas.example.com") != "*.example.com" {
		t.Error("应匹配通配符证书")
	}
	if leafName("a.b.example.com") != "" {
		t.Error("通配符只匹配一级")
	}
	if leafName("old.example.org") != "" {
		t.Error("过期证书不应被选中")
	}
	m.cfg.Certs[1].Enable = false
	m.rebuildTLSLocked()
	if m.HasCertificate("nas.example.com") {
		t.Error("停用的证书不应被选中")
	}
}

// 多张证书覆盖同一域名时选用到期最晚的一张，与列表顺序无关（排序、分组不改变实际使用的证书）
func TestGetCertificateIgnoresOrder(t *testing.T) {
	now := time.Now()
	shortC, shortK := selfSigned(t, []string{"*.example.com", "nas.example.com"}, now, now.Add(24*time.Hour))
	longC, longK := selfSigned(t, []string{"*.example.com", "nas.example.com"}, now, now.Add(72*time.Hour))
	short := Cert{Key: "short", Enable: true, Source: SourceUpload, CertPEM: shortC, KeyPEM: shortK}
	long := Cert{Key: "long", Enable: true, Source: SourceUpload, CertPEM: longC, KeyPEM: longK}
	for _, order := range [][]Cert{{short, long}, {long, short}} {
		m := newTestModule(t)
		m.cfg.Certs = order
		m.rebuildTLSLocked()
		for _, host := range []string{"nas.example.com", "a.example.com"} {
			c, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: host})
			if err != nil {
				t.Fatalf("%s 没有选到证书: %v", host, err)
			}
			leaf, _ := x509.ParseCertificate(c.Certificate[0])
			if !leaf.NotAfter.After(now.Add(48 * time.Hour)) {
				t.Errorf("顺序 %s,%s 下 %s 应选用到期最晚的证书，实际到期 %v", order[0].Key, order[1].Key, host, leaf.NotAfter)
			}
		}
	}
}

func TestMappingAndScript(t *testing.T) {
	m := newTestModule(t)
	m.ctx = context.Background()
	dir := filepath.Join(t.TempDir(), "certs")
	out := filepath.Join(t.TempDir(), "script.out")
	now := time.Now()
	c, k := selfSigned(t, []string{"nas.example.com"}, now, now.Add(24*time.Hour))
	cert := Cert{
		Key: "a", Remark: "nas.example.com", Enable: true, Source: SourceUpload, CertPEM: c, KeyPEM: k, IssuerPEM: c,
		Mapping: Mapping{Enable: true, Dir: dir, Script: `echo "$certName|$certFile|$keyFile" > ` + out},
	}
	if err := cert.Validate(m.cfg.Settings); err != nil {
		t.Fatal(err)
	}
	m.cfg.Certs = []Cert{cert}
	m.changedLocked("a")
	for _, name := range []string{"nas.example.com.crt", "nas.example.com.pem", "nas.example.com.key", "nas.example.com_issuerCertificate.crt"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(data) == 0 {
			t.Fatalf("缺少映射文件 %s: %v", name, err)
		}
	}
	if fi, _ := os.Stat(filepath.Join(dir, "nas.example.com.key")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("私钥权限应为 0600，实际 %v", fi.Mode().Perm())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(out); len(b) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(out)
	want := "nas.example.com|" + filepath.Join(dir, "nas.example.com.crt") + "|" + filepath.Join(dir, "nas.example.com.key")
	if strings.TrimSpace(string(b)) != want {
		t.Fatalf("脚本变量错误: %q want %q", b, want)
	}
	// 内容未变化时不应再次执行脚本
	_ = os.Remove(out)
	m.applyMappingLocked(&m.cfg.Certs[0], true)
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(out); err == nil {
		t.Fatal("内容未变化不应执行脚本")
	}
}

func TestFileSourceReload(t *testing.T) {
	m := newTestModule(t)
	dir := t.TempDir()
	now := time.Now()
	c1, k1 := selfSigned(t, []string{"f.example.com"}, now, now.Add(24*time.Hour))
	certFile, keyFile := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	_ = os.WriteFile(certFile, []byte(c1), 0o600)
	_ = os.WriteFile(keyFile, []byte(k1), 0o600)
	m.cfg.Certs = []Cert{{Key: "f", Enable: true, Source: SourceFile, CertFile: certFile, KeyFile: keyFile}}
	m.changedLocked("f")
	if m.cfg.Certs[0].CertPEM != c1 || !m.HasCertificate("f.example.com") {
		t.Fatal("文件证书未加载")
	}
	c2, k2 := selfSigned(t, []string{"f2.example.com"}, now, now.Add(24*time.Hour))
	_ = os.WriteFile(certFile, []byte(c2), 0o600)
	_ = os.WriteFile(keyFile, []byte(k2), 0o600)
	m.checkAll(false)
	if !m.HasCertificate("f2.example.com") {
		t.Fatal("文件更新后应重新加载")
	}
	_ = os.WriteFile(keyFile, []byte("broken"), 0o600)
	m.checkAll(false)
	if m.stateLocked("f").LastError == "" || !m.HasCertificate("f2.example.com") {
		t.Fatal("文件损坏时应报错并保留旧证书")
	}
}

func TestConvertLucky(t *testing.T) {
	now := time.Now()
	c, k := selfSigned(t, []string{"ddns.example.com"}, now, now.Add(60*24*time.Hour))
	b64 := func(s string) string { return encodeB64(s) }
	raw, _ := json.Marshal([]map[string]any{
		{
			"Enable": true, "AddFrom": "acme", "Remark": "ddns.example.com", "CertBase64": b64(c), "KeyBase64": b64(k), "IssuerCertificate": b64(c),
			"MappingToPath": true, "MappingPath": "/data/nginx/ssl/ddns", "MappingChangeScript": "echo hi",
			"ExtParams": map[string]any{
				"acmeCADirURL": "https://acme.zerossl.com/v2/DV90", "acmeDomains": []string{"ddns.example.com"}, "acmeEmail": "a@b.c",
				"acmeDNSServer": "alidns", "acmeDNSID": "id", "acmeDNSSecret": "sec", "acmeKeyType": "2048", "acmeKid": "kid", "acmeHMAC": "hmac",
				"acmePropagationTimeout": 600, "acmeIgnorePropagationChecErrors": true,
			},
		},
		{"Enable": true, "AddFrom": "acme", "Remark": "bad", "ExtParams": map[string]any{"acmeDNSServer": "west", "acmeDomains": []string{"a.com"}, "acmeEmail": "a@b.c"}},
	})
	certs, warns, found, err := convertLuckyCerts(raw, defaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if found != 2 || len(certs) != 1 {
		t.Fatalf("found=%d imported=%d %v", found, len(certs), warns)
	}
	got := certs[0]
	if !got.Enable || got.ACME.CA != "zerossl" || got.ACME.EABKid != "kid" || got.ACME.KeyType != "2048" || got.ACME.PropagationTimeout != 600 ||
		!got.ACME.SkipPropagationCheck || got.CertPEM != c || got.KeyPEM != k || !got.Mapping.Enable {
		t.Fatalf("映射错误: %+v", got)
	}
	// 导入的证书仍在有效期内，不需要重新签发
	m := newTestModule(t)
	if need, reason := m.needsRenewal(&got, time.Now()); need {
		t.Fatalf("导入的有效证书不应触发重签: %s", reason)
	}
	joined := strings.Join(warns, "\n")
	for _, w := range []string{"bad: 跳过", "/data/nginx/ssl/ddns"} {
		if !strings.Contains(joined, w) {
			t.Errorf("缺少告警 %q:\n%s", w, joined)
		}
	}
}

// TestImportRealSample 仅在设置 GATELY_LUCKY_SSL_SAMPLE 时运行。
func TestImportRealSample(t *testing.T) {
	path := os.Getenv("GATELY_LUCKY_SSL_SAMPLE")
	if path == "" {
		t.Skip("未设置 GATELY_LUCKY_SSL_SAMPLE")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := luckyimport.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModule(t)
	rep, err := m.ImportLucky(doc, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found=%d imported=%d", rep.Found, rep.Imported)
	for _, w := range rep.Warnings {
		t.Log("warning:", w)
	}
	for _, c := range m.cfg.Certs {
		info, err := certInfo(c.CertPEM)
		if err != nil {
			t.Fatalf("证书无法解析: %v", err)
		}
		need, reason := m.needsRenewal(&c, time.Now())
		t.Logf("%s 有效期至 %s，需要重签=%v %s", c.Remark, info.NotAfter.Format("2006-01-02"), need, reason)
	}
	if rep.Imported != rep.Found {
		t.Fatal("有证书未能导入")
	}
}

// ---------- Pebble 端到端测试 ----------

// challtestsrvProvider 把 TXT 记录写入 pebble-challtestsrv，供 Pebble 验证。
type challtestsrvProvider struct{ mgmt string }

func (p *challtestsrvProvider) Upsert(context.Context, dnsprovider.Record) (string, error) {
	return "", fmt.Errorf("不支持")
}

func (p *challtestsrvProvider) post(path string, body map[string]string) error {
	b, _ := json.Marshal(body)
	resp, err := http.Post(p.mgmt+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("challtestsrv %s 返回 %d", path, resp.StatusCode)
	}
	return nil
}

func (p *challtestsrvProvider) AddTXT(_ context.Context, zone, sub, value string) error {
	return p.post("/set-txt", map[string]string{"host": sub + "." + zone + ".", "value": value})
}

func (p *challtestsrvProvider) DeleteTXT(_ context.Context, zone, sub, _ string) error {
	return p.post("/clear-txt", map[string]string{"host": sub + "." + zone + "."})
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitTCP(t *testing.T, addr string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("服务未启动: %s", addr)
}

// TestPebbleEndToEnd 用 Let's Encrypt 的测试 CA Pebble 完整走一遍 DNS-01 签发与续签。
// 需要设置 GATELY_PEBBLE_BIN（包含 pebble 与 pebble-challtestsrv 的目录）与 GATELY_PEBBLE_SRC（pebble 源码目录）。
func TestPebbleEndToEnd(t *testing.T) {
	bin, src := os.Getenv("GATELY_PEBBLE_BIN"), os.Getenv("GATELY_PEBBLE_SRC")
	if bin == "" || src == "" {
		t.Skip("未设置 GATELY_PEBBLE_BIN / GATELY_PEBBLE_SRC")
	}
	dnsPort, mgmtPort, acmePort, pebbleMgmt := freePort(t), freePort(t), freePort(t), freePort(t)
	chall := exec.Command(filepath.Join(bin, "pebble-challtestsrv"),
		"-dnsserver", fmt.Sprintf("127.0.0.1:%d", dnsPort), "-management", fmt.Sprintf("127.0.0.1:%d", mgmtPort),
		"-http01", "", "-https01", "", "-tlsalpn01", "", "-doh", "")
	if err := chall.Start(); err != nil {
		t.Fatal(err)
	}
	defer chall.Process.Kill()

	cfgPath := filepath.Join(t.TempDir(), "pebble.json")
	cfg := fmt.Sprintf(`{"pebble":{"listenAddress":"127.0.0.1:%d","managementListenAddress":"127.0.0.1:%d",
		"certificate":"%s/test/certs/localhost/cert.pem","privateKey":"%s/test/certs/localhost/key.pem",
		"httpPort":5002,"tlsPort":5001,"ocspResponderURL":"","externalAccountBindingRequired":false}}`, acmePort, pebbleMgmt, src, src)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o600)
	pebble := exec.Command(filepath.Join(bin, "pebble"), "-config", cfgPath, "-dnsserver", fmt.Sprintf("127.0.0.1:%d", dnsPort))
	pebble.Env = append(os.Environ(), "PEBBLE_VA_NOSLEEP=1", "PEBBLE_WFE_NONCEREJECT=0", "PEBBLE_AUTHZREUSE=0")
	if err := pebble.Start(); err != nil {
		t.Fatal(err)
	}
	defer pebble.Process.Kill()
	waitTCP(t, fmt.Sprintf("127.0.0.1:%d", mgmtPort))
	waitTCP(t, fmt.Sprintf("127.0.0.1:%d", acmePort))

	// 信任 Pebble 的 HTTPS 证书
	ca, err := os.ReadFile(filepath.Join(src, "test/certs/pebble.minica.pem"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	acmeHTTPClient = func() *http.Client {
		return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	}
	defer func() { acmeHTTPClient = nil }()

	dnsprovider.Register(dnsprovider.Meta{Name: "challtestsrv-test", SupportsACME: true}, func(dnsprovider.Credentials, *http.Client) (dnsprovider.Provider, error) {
		return &challtestsrvProvider{mgmt: fmt.Sprintf("http://127.0.0.1:%d", mgmtPort)}, nil
	})

	m := newTestModule(t)
	m.ctx = context.Background()
	mapDir := filepath.Join(t.TempDir(), "mapped")
	c := Cert{
		Key: "p", Remark: "gately.test", Enable: true, Source: SourceACME,
		ACME: ACMEConfig{
			Domains: []string{"gately.test", "*.gately.test"}, Email: "test@gately.test", CA: "custom",
			DirectoryURL: fmt.Sprintf("https://127.0.0.1:%d/dir", acmePort), Provider: "challtestsrv-test", Zone: "gately.test",
			Resolvers: []string{fmt.Sprintf("127.0.0.1:%d", dnsPort)}, PropagationTimeout: 60, DisableAuthoritativeCheck: true,
		},
		Mapping: Mapping{Enable: true, Dir: mapDir},
	}
	if err := c.Validate(m.cfg.Settings); err != nil {
		t.Fatal(err)
	}
	m.cfg.Certs = []Cert{c}
	m.obtainNow("p")
	st := m.stateLocked("p")
	if st.LastError != "" {
		t.Fatalf("签发失败: %s", st.LastError)
	}
	info, err := certInfo(m.cfg.Certs[0].CertPEM)
	if err != nil {
		t.Fatal(err)
	}
	if !sameDomains(info.Domains, []string{"gately.test", "*.gately.test"}) {
		t.Fatalf("证书域名错误: %v", info.Domains)
	}
	if !m.HasCertificate("nas.gately.test") || !m.HasCertificate("gately.test") {
		t.Fatal("签发后应能按 SNI 选择证书")
	}
	if _, err := os.Stat(filepath.Join(mapDir, "gately.test.key")); err != nil {
		t.Fatalf("签发后应写入映射目录: %v", err)
	}
	if len(m.cfg.Accounts) != 1 {
		t.Fatalf("应缓存 1 个 ACME 账户，实际 %d", len(m.cfg.Accounts))
	}
	firstFP := info.Fingerprint

	// 提高续签阈值触发续签，应复用已缓存账户
	m.cfg.Settings.RenewBeforeDays = 3650
	m.checkAll(true)
	info2, _ := certInfo(m.cfg.Certs[0].CertPEM)
	if info2.Fingerprint == firstFP {
		t.Fatalf("续签后证书应变化，错误: %s", m.stateLocked("p").LastError)
	}
	if len(m.cfg.Accounts) != 1 {
		t.Fatal("续签不应重复注册账户")
	}

	// 限定 IPv4：ACME 接口、DNS 服务商与 TXT 生效检查都只走 IPv4，续签照常成功
	m.cfg.Certs[0].ACME.Network = NetworkIPv4
	m.checkAll(true)
	info3, _ := certInfo(m.cfg.Certs[0].CertPEM)
	if info3.Fingerprint == info2.Fingerprint {
		t.Fatalf("限定 IPv4 后续签应成功，错误: %s", m.stateLocked("p").LastError)
	}
	// 限定 IPv6：测试 CA 只监听 127.0.0.1，连接 ACME 服务就应失败，不会回退到 IPv4
	m.cfg.Certs[0].ACME.Network = NetworkIPv6
	m.obtainNow("p")
	if e := m.stateLocked("p").LastError; !strings.Contains(e, "tcp6") {
		t.Fatalf("限定 IPv6 时不应回退到 IPv4: %s", e)
	}
}

func encodeB64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestValidateHTTPChallenge(t *testing.T) {
	ok := ACMEConfig{Domains: []string{"a.example.com"}, Email: "a@example.com", Challenge: ChallengeHTTP}
	if err := ok.validate(Settings{}); err != nil {
		t.Fatalf("HTTP-01 不应要求 DNS 服务商: %v", err)
	}
	wild := ACMEConfig{Domains: []string{"*.example.com"}, Email: "a@example.com", Challenge: ChallengeHTTP}
	if err := wild.validate(Settings{}); err == nil {
		t.Fatal("HTTP-01 应拒绝通配符域名")
	}
	bad := ACMEConfig{Domains: []string{"a.example.com"}, Email: "a@example.com", Challenge: "tls"}
	if err := bad.validate(Settings{}); err == nil {
		t.Fatal("应拒绝未知验证方式")
	}
	dns := ACMEConfig{Domains: []string{"a.example.com"}, Email: "a@example.com"}
	if err := dns.validate(Settings{}); err == nil {
		t.Fatal("默认 DNS 验证仍需要受支持的服务商")
	}
}

// TestCertSecrets 验证查看证书密钥明文接口返回 DNS 凭据密钥与 EAB HMAC，证书不存在时返回 404。
func TestCertSecrets(t *testing.T) {
	m := newTestModule(t)
	c := Cert{Key: "k", Source: SourceACME}
	c.ACME.Credentials.Secret = "dns-secret"
	c.ACME.EABHMAC = "hmac"
	m.cfg.Certs = []Cert{c}
	mux := http.NewServeMux()
	m.Routes(secretsRouter{mux})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/certs/k/secrets", nil))
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got["secret"] != "dns-secret" || got["eabHmac"] != "hmac" {
		t.Fatalf("应返回密钥明文: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/certs/missing/secrets", nil))
	if rec.Code != 404 {
		t.Fatalf("证书不存在应返回 404，实际 %d", rec.Code)
	}
}

type secretsRouter struct{ mux *http.ServeMux }

func (r secretsRouter) HandleFunc(pattern string, h http.HandlerFunc) { r.mux.HandleFunc(pattern, h) }
