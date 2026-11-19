package ssl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFnOSIndex 按飞牛的目录结构 ssls/<域名>/<时间戳>/ 写入证书，并生成登记文件。
func writeFnOSIndex(t *testing.T, root, index, domain, stamp, certPEM, keyPEM string, fullchain bool) {
	t.Helper()
	dir := filepath.Join(root, domain, stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	crt, key := filepath.Join(dir, domain+".crt"), filepath.Join(dir, domain+".key")
	entry := map[string]any{"domain": domain, "san": []string{domain}, "privateKey": key, "validTo": time.Now().Add(24 * time.Hour).UnixMilli(), "used": true}
	if fullchain {
		// 真实情况下 certificate 只有叶证书、fullchain 带中间证书；这里让 certificate 无效，确认优先读 fullchain
		_ = os.WriteFile(crt, []byte("leaf only"), 0o640)
		fc := filepath.Join(dir, "fullchain.crt")
		_ = os.WriteFile(fc, []byte(certPEM), 0o640)
		entry["certificate"], entry["fullchain"] = crt, fc
	} else {
		_ = os.WriteFile(crt, []byte(certPEM), 0o640)
		entry["certificate"], entry["fullchain"] = crt, ""
	}
	_ = os.WriteFile(key, []byte(keyPEM), 0o640)
	other := map[string]any{"domain": "fnOS", "san": []string{"fnOS"}, "certificate": "/nonexistent/fnOS.crt", "fullchain": "", "privateKey": "/nonexistent/fnOS.key"}
	raw, _ := json.Marshal([]any{other, entry})
	if err := os.WriteFile(index, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func useFnOSIndex(t *testing.T) (root, index string) {
	t.Helper()
	dir := t.TempDir()
	old := fnosCertIndex
	fnosCertIndex = filepath.Join(dir, "network_cert_all.conf")
	t.Cleanup(func() { fnosCertIndex = old })
	return filepath.Join(dir, "ssls"), fnosCertIndex
}

// 飞牛续期会把证书放到新的时间戳目录并更新登记文件，证书应随之重新加载。
func TestFnOSSourceFollowsRenewal(t *testing.T) {
	root, index := useFnOSIndex(t)
	m := newTestModule(t)
	now := time.Now()
	c1, k1 := selfSigned(t, []string{"nas.example.com"}, now, now.Add(24*time.Hour))
	writeFnOSIndex(t, root, index, "nas.example.com", "1000", c1, k1, false)

	m.cfg.Certs = []Cert{{Key: "n", Enable: true, Source: SourceFnOS, FnOSDomain: "NAS.example.com"}}
	m.changedLocked("n")
	if m.cfg.Certs[0].CertPEM != c1 || !m.HasCertificate("nas.example.com") {
		t.Fatalf("飞牛证书未加载: %s", m.stateLocked("n").LastError)
	}

	c2, k2 := selfSigned(t, []string{"nas.example.com", "renewed.example.com"}, now, now.Add(48*time.Hour))
	writeFnOSIndex(t, root, index, "nas.example.com", "2000", c2, k2, true)
	m.checkAll(false)
	if m.cfg.Certs[0].CertPEM != c2 || !m.HasCertificate("renewed.example.com") {
		t.Fatalf("续期后应读取新目录的 fullchain: %s", m.stateLocked("n").LastError)
	}

	// 证书从飞牛中删除：报错并保留已加载的证书
	_ = os.WriteFile(index, []byte(`[]`), 0o644)
	m.checkAll(false)
	if e := m.stateLocked("n").LastError; !strings.Contains(e, "NAS.example.com") || !m.HasCertificate("renewed.example.com") {
		t.Fatalf("证书不存在时应报错并保留旧证书: %q", e)
	}

	// 登记文件不存在（非飞牛系统或容器未挂载）
	_ = os.Remove(index)
	m.checkAll(false)
	if e := m.stateLocked("n").LastError; !strings.Contains(e, "network_cert_all.conf") {
		t.Fatalf("登记文件缺失时应提示挂载: %q", e)
	}
}

func TestFnOSValidate(t *testing.T) {
	s := defaultSettings()
	if err := (&Cert{Source: SourceFnOS, FnOSDomain: "  "}).Validate(s); err == nil {
		t.Fatal("未选择证书应报错")
	}
	c := Cert{Source: SourceFnOS, FnOSDomain: " nas.example.com "}
	if err := c.Validate(s); err != nil || c.FnOSDomain != "nas.example.com" {
		t.Fatalf("err=%v domain=%q", err, c.FnOSDomain)
	}
}

func TestFnOSCertsAPI(t *testing.T) {
	root, index := useFnOSIndex(t)
	m := newTestModule(t)
	s := newTestServer(t, m)

	code, body := s.do("GET", "/fnos-certs", "")
	if code != 200 || !strings.Contains(body, `"available":false`) {
		t.Fatalf("非飞牛系统: %d %s", code, body)
	}

	now := time.Now()
	c, k := selfSigned(t, []string{"nas.example.com"}, now, now.Add(24*time.Hour))
	writeFnOSIndex(t, root, index, "nas.example.com", "1000", c, k, false)
	code, body = s.do("GET", "/fnos-certs", "")
	if code != 200 || !strings.Contains(body, `"available":true`) || !strings.Contains(body, `"domain":"nas.example.com"`) || !strings.Contains(body, `"notAfter"`) {
		t.Fatalf("应列出飞牛证书: %d %s", code, body)
	}
	// 只返回域名与有效期，不暴露文件路径
	if strings.Contains(body, root) {
		t.Fatalf("不应返回证书路径: %s", body)
	}

	_ = os.WriteFile(index, []byte("{bad"), 0o644)
	code, body = s.do("GET", "/fnos-certs", "")
	if code != 200 || !strings.Contains(body, `"error"`) {
		t.Fatalf("登记文件损坏应返回错误说明: %d %s", code, body)
	}
}
