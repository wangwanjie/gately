package ssl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
)

// 复制证书：凭据与 EAB 占位符沿用模板；ACME/文件来源清空证书内容与签发记录；上传来源沿用私钥；部署记录清空。
func TestCopyCert(t *testing.T) {
	src := Cert{
		Key: "a", Source: SourceACME, CertPEM: "cert", KeyPEM: "key", IssuerPEM: "issuer",
		UpdatedAt: time.Now(), IssuedProfile: "shortlived",
		Deploys: []Deploy{{DeployedHash: "h", DeployedAt: time.Now()}},
	}
	src.ACME.Credentials.Secret = "dns-secret"
	src.ACME.EABHMAC = "hmac"

	dst := src
	dst.Deploys = []Deploy{{Enable: true, DeployedHash: "h", DeployedAt: time.Now()}}
	dst.KeyPEM = crud.SecretMask
	dst.ACME.Credentials.Secret = crud.SecretMask
	dst.ACME.EABHMAC = crud.SecretMask
	copyCert(&src, &dst)
	if dst.ACME.Credentials.Secret != "dns-secret" || dst.ACME.EABHMAC != "hmac" {
		t.Fatalf("占位符应沿用模板的密钥: %+v", dst.ACME)
	}
	if dst.CertPEM != "" || dst.KeyPEM != "" || dst.IssuerPEM != "" || dst.IssuedProfile != "" || !dst.UpdatedAt.IsZero() {
		t.Fatalf("ACME 副本应清空已签发的证书: %+v", dst)
	}
	if dst.Deploys[0].DeployedHash != "" || !dst.Deploys[0].DeployedAt.IsZero() {
		t.Fatalf("部署记录应清空: %+v", dst.Deploys[0])
	}
	// 直接调用复制接口时请求体里的部署目标即使启用，副本中也应停用，避免覆盖原证书在目标上的文件
	if dst.Deploys[0].Enable {
		t.Fatalf("副本的部署目标应停用: %+v", dst.Deploys[0])
	}
	if src.Deploys[0].DeployedHash != "h" || src.KeyPEM != "key" {
		t.Fatalf("模板不应被修改: %+v", src)
	}

	// 新填写的密钥原样保留
	dst = Cert{Source: SourceACME}
	dst.ACME.Credentials.Secret = "new"
	copyCert(&src, &dst)
	if dst.ACME.Credentials.Secret != "new" {
		t.Fatalf("新密钥应原样保留: %q", dst.ACME.Credentials.Secret)
	}

	// 上传来源：证书内容是配置本身，私钥占位符沿用模板
	up := Cert{Source: SourceUpload, CertPEM: "c", KeyPEM: "k"}
	dst = Cert{Source: SourceUpload, CertPEM: "c", KeyPEM: crud.SecretMask}
	copyCert(&up, &dst)
	if dst.CertPEM != "c" || dst.KeyPEM != "k" {
		t.Fatalf("上传证书应沿用证书与私钥: %+v", dst)
	}

	// 文件来源：内容由系统重新读取
	dst = Cert{Source: SourceFile, CertFile: "/a.crt", KeyFile: "/a.key", CertPEM: "c", KeyPEM: crud.SecretMask}
	copyCert(&up, &dst)
	if dst.CertPEM != "" || dst.KeyPEM != "" || dst.CertFile != "/a.crt" {
		t.Fatalf("文件来源副本应清空内容、保留路径: %+v", dst)
	}
}

// 通过接口复制上传的证书：私钥占位符沿用模板，响应中的私钥脱敏；部署记录清空；副本排在模板之后，模板不变。
func TestCopyCertHTTP(t *testing.T) {
	m := newTestModule(t)
	s := newTestServer(t, m)
	certPEM, keyPEM := selfSigned(t, []string{"a.example.com"}, time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))
	deployed := time.Now().Add(-time.Hour)
	m.mu.Lock()
	m.cfg.Certs = []Cert{
		{Key: "a", Remark: "a", Source: SourceUpload, CertPEM: certPEM, KeyPEM: keyPEM,
			Deploys: []Deploy{{Type: "local", Dir: "/tmp/gately-copy-test", DeployedHash: "h", DeployedAt: deployed}}},
		{Key: "b", Remark: "b", Source: SourceUpload, CertPEM: certPEM, KeyPEM: keyPEM},
	}
	m.mu.Unlock()

	body, _ := json.Marshal(Cert{Remark: "a-copy", Source: SourceUpload, CertPEM: certPEM, KeyPEM: crud.SecretMask,
		Deploys: []Deploy{{Type: "local", Dir: "/tmp/gately-copy-test", DeployedHash: "h", DeployedAt: deployed}}})
	code, resp := s.do("POST", "/certs/a/copy", string(body))
	if code != 200 {
		t.Fatalf("复制证书失败: %d %s", code, resp)
	}
	if strings.Contains(resp, "PRIVATE KEY") {
		t.Fatalf("响应不应含私钥明文: %s", resp)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Certs) != 3 || m.cfg.Certs[1].Remark != "a-copy" || m.cfg.Certs[2].Key != "b" {
		t.Fatalf("副本应排在模板之后: %+v", m.cfg.Certs)
	}
	cp := m.cfg.Certs[1]
	if cp.KeyPEM != keyPEM {
		t.Fatal("上传证书的私钥占位符应沿用模板")
	}
	if cp.Deploys[0].DeployedHash != "" || !cp.Deploys[0].DeployedAt.IsZero() {
		t.Fatalf("副本的部署记录应清空: %+v", cp.Deploys[0])
	}
	if m.cfg.Certs[0].Deploys[0].DeployedHash != "h" {
		t.Fatalf("模板的部署记录不应改变: %+v", m.cfg.Certs[0].Deploys[0])
	}
}
