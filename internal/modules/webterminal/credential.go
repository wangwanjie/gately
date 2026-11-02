package webterminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/module"
)

// AuthCredential 表示 SSH 连接引用认证预设，凭据在连接时从预设中解析。
const AuthCredential = "credential"

// Credential 是可被多个 SSH 连接复用的认证预设（密码或私钥）。
type Credential struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// AuthType 为 password 或 key
	AuthType string `json:"authType"`
	// Username 为可选的默认用户名，连接未填写用户名时使用
	Username   string `json:"username"`
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"`
	// KeyFile 为服务器本机上的私钥文件路径，PrivateKey 为空时使用
	KeyFile    string `json:"keyFile"`
	Passphrase string `json:"passphrase"`
	Remark     string `json:"remark"`
	// Enable 为复用通用接口保留，预设始终可用
	Enable bool `json:"enable"`
}

// Validate 校验认证预设并补全默认值。
func (c *Credential) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("名称不能为空")
	}
	c.Username = strings.TrimSpace(c.Username)
	c.KeyFile = strings.TrimSpace(c.KeyFile)
	switch c.AuthType {
	case "", "password":
		c.AuthType = "password"
	case "key":
		if c.PrivateKey == "" && c.KeyFile == "" {
			return errors.New("请填写私钥或私钥文件路径")
		}
		if c.PrivateKey != "" && c.PrivateKey != crud.SecretMask {
			if _, err := parseKey(c.PrivateKey, c.Passphrase); err != nil {
				return fmt.Errorf("私钥无效: %w", err)
			}
		}
	default:
		return fmt.Errorf("未知的认证方式 %q", c.AuthType)
	}
	c.Enable = true
	return nil
}

// mergeCredentialSecrets 把被脱敏的密文字段恢复为旧值。
func mergeCredentialSecrets(old, updated *Credential) {
	crud.KeepSecret(old.Password, &updated.Password)
	crud.KeepSecret(old.PrivateKey, &updated.PrivateKey)
	crud.KeepSecret(old.Passphrase, &updated.Passphrase)
}

// viewCredential 返回脱敏后的认证预设。
func viewCredential(c *Credential) any {
	v := *c
	v.Password = crud.Mask(c.Password)
	v.PrivateKey = crud.Mask(c.PrivateKey)
	v.Passphrase = crud.Mask(c.Passphrase)
	return v
}

// registerCredentials 注册认证预设的增删改查接口。
func (m *Module) registerCredentials(r module.Router) {
	creds := &crud.Collection[Credential]{
		Mu:       &m.mu,
		Items:    &m.cfg.Credentials,
		Save:     m.saveLocked,
		Key:      func(c *Credential) *string { return &c.Key },
		Enabled:  func(c *Credential) *bool { return &c.Enable },
		Validate: func(c *Credential) error { return c.Validate() },
		Merge:    mergeCredentialSecrets,
		Copy:     mergeCredentialSecrets,
		View:     viewCredential,
		// 查看被脱敏的密码与私钥口令明文；私钥在界面上没有显示按钮，不经此接口返回
		Secrets: func(c *Credential) map[string]string {
			return map[string]string{"password": c.Password, "passphrase": c.Passphrase}
		},
		// 与连接的密钥一样，需要先接受协议并通过二次验证
		SecretsGuard: m.requireAccess,
		// 预设变更后，丢弃引用该预设的连接在池中的 SSH 连接，下次使用新凭据重新认证
		Changed: m.dropCredentialUsersLocked,
		// 仍被连接引用的预设不允许删除，避免连接失去凭据
		BeforeRemove: func(c *Credential) error {
			if names := m.credentialUsersLocked(c.Key); len(names) > 0 {
				return fmt.Errorf("认证预设正被以下连接使用，请先修改这些连接：%s", strings.Join(names, "、"))
			}
			return nil
		},
	}
	creds.Register(r, "/credentials")
}

// credentialUsersLocked 返回引用指定预设的连接名称，调用方需持有 m.mu。
func (m *Module) credentialUsersLocked(key string) []string {
	var names []string
	for i := range m.cfg.Connections {
		c := &m.cfg.Connections[i]
		if c.Type == TypeSSH && c.SSH.AuthType == AuthCredential && c.SSH.CredentialKey == key {
			names = append(names, c.Name)
		}
	}
	return names
}

// dropCredentialUsersLocked 丢弃引用指定预设的连接的池化 SSH 连接，调用方需持有 m.mu。
func (m *Module) dropCredentialUsersLocked(key string) {
	for i := range m.cfg.Connections {
		c := &m.cfg.Connections[i]
		if c.SSH.AuthType == AuthCredential && c.SSH.CredentialKey == key {
			m.pool.drop(c.Key)
		}
	}
}

// resolveCredential 把引用预设的 SSH 配置替换为预设中的实际凭据；未使用预设时原样返回。
// 连接未填写用户名时使用预设的默认用户名。
func (m *Module) resolveCredential(s *SSHConfig) error {
	if s.AuthType != AuthCredential {
		return nil
	}
	m.mu.Lock()
	var cred *Credential
	for i := range m.cfg.Credentials {
		if m.cfg.Credentials[i].Key == s.CredentialKey {
			cred = &m.cfg.Credentials[i]
			break
		}
	}
	if cred == nil {
		m.mu.Unlock()
		return errors.New("所选认证预设不存在，请重新选择")
	}
	c := *cred
	m.mu.Unlock()
	s.AuthType = c.AuthType
	s.Password, s.PrivateKey, s.KeyFile, s.Passphrase = c.Password, c.PrivateKey, c.KeyFile, c.Passphrase
	if s.Username == "" {
		s.Username = c.Username
	}
	if s.Username == "" {
		return errors.New("请填写用户名，或在认证预设中设置默认用户名")
	}
	return nil
}
