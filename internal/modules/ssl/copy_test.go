package ssl

import (
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
	dst.Deploys = []Deploy{{DeployedHash: "h", DeployedAt: time.Now()}}
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
