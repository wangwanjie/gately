package ssl

import (
	"io"
	"log"
	"testing"

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
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// 配置文件缺 ACME 参数时加载后补上默认值，否则等待生效时间 0 时 TXT 记录只检查一次就判超时、
// 密钥类型为空时签发报错、邮箱为空时以空邮箱注册账户。
func TestInitFillsMissingACMEDefaults(t *testing.T) {
	m := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true, "defaultEmail": "admin@example.com"},
		"certs": []any{map[string]any{
			"key": "c1", "enable": true, "source": "acme",
			"acme": map[string]any{"domains": []string{"example.com"}, "provider": "alidns"},
		}},
	})
	a := m.cfg.Certs[0].ACME
	if a.PropagationTimeout != 300 || a.KeyType != "P256" || a.CA != "letsencrypt" || a.Challenge != ChallengeDNS {
		t.Fatalf("未补默认值: %+v", a)
	}
	if a.Email != "admin@example.com" {
		t.Fatalf("邮箱未取全局默认邮箱: %q", a.Email)
	}
	if a.directoryURL() != caDirectories["letsencrypt"] {
		t.Fatalf("目录地址不符: %s", a.directoryURL())
	}
}

// 已有的值保持不变；非 ACME 来源的证书不补 ACME 参数。
func TestInitKeepsExistingACMEValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{
		"settings": map[string]any{"enable": true, "defaultEmail": "admin@example.com"},
		"certs": []any{
			map[string]any{
				"key": "c1", "enable": true, "source": "acme",
				"acme": map[string]any{
					"domains": []string{"example.com"}, "email": "me@example.org", "ca": "zerossl", "keyType": "RSA2048",
					"challenge": "http", "propagationTimeout": 60,
				},
			},
			map[string]any{"key": "c2", "enable": true, "source": "file", "certFile": "/etc/ssl/a.crt", "keyFile": "/etc/ssl/a.key"},
		},
	})
	a := m.cfg.Certs[0].ACME
	if a.Email != "me@example.org" || a.CA != "zerossl" || a.KeyType != "RSA2048" || a.Challenge != ChallengeHTTP || a.PropagationTimeout != 60 {
		t.Fatalf("已有值被改动: %+v", a)
	}
	if f := m.cfg.Certs[1].ACME; f.KeyType != "" || f.PropagationTimeout != 0 || f.Email != "" {
		t.Fatalf("非 ACME 证书不应补 ACME 参数: %+v", f)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameACMEDefaults(t *testing.T) {
	s := defaultSettings()
	s.DefaultEmail = "admin@example.com"
	c := Cert{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"example.com"}, Provider: "alidns"}}
	if err := c.Validate(s); err != nil {
		t.Fatal(err)
	}
	a := c.ACME
	if a.PropagationTimeout != 300 || a.KeyType != "P256" || a.CA != "letsencrypt" || a.Challenge != ChallengeDNS || a.Email != "admin@example.com" {
		t.Fatalf("保存时默认值不符: %+v", a)
	}
	// 没有邮箱也没有全局默认邮箱时仍然拒绝
	bad := Cert{Source: SourceACME, ACME: ACMEConfig{Domains: []string{"example.com"}, Provider: "alidns"}}
	if err := bad.Validate(defaultSettings()); err == nil {
		t.Fatal("缺少邮箱应被拒绝")
	}
}
