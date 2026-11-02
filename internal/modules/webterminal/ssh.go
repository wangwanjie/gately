package webterminal

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/wangwanjie/gately/internal/httpx"
)

// SSHConfig 是 SSH 连接参数。
type SSHConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	// AuthType 为 password、key 或 credential（引用认证预设）
	AuthType string `json:"authType"`
	// CredentialKey 为 AuthType=credential 时引用的认证预设 key
	CredentialKey string `json:"credentialKey"`
	Password      string `json:"password"`
	PrivateKey    string `json:"privateKey"`
	// KeyFile 为服务器本机上的私钥文件路径，PrivateKey 为空时使用
	KeyFile    string `json:"keyFile"`
	Passphrase string `json:"passphrase"`
	// HostKey 为首次连接时记录的主机密钥指纹（SHA256），为空表示下次连接时重新信任
	HostKey string `json:"hostKey"`
	// KeepAlive 为保活间隔（秒）
	KeepAlive int `json:"keepAlive"`
	// WorkDir 为登录后切换到的目录
	WorkDir string `json:"workDir"`
	// Timeout 为连接超时（秒）
	Timeout int   `json:"timeout"`
	Proxy   Proxy `json:"proxy"`
}

func parseKey(key, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
	}
	return ssh.ParsePrivateKey([]byte(key))
}

// loadSigner 解析认证私钥：优先使用内联私钥，其次读取服务器上的密钥文件。
func loadSigner(s *SSHConfig) (ssh.Signer, error) {
	key := s.PrivateKey
	if key == "" {
		if s.KeyFile == "" {
			return nil, errors.New("未配置私钥")
		}
		path := s.KeyFile
		if path == "~" || strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取私钥文件失败: %w", err)
		}
		key = string(data)
	}
	return parseKey(key, s.Passphrase)
}

// Fingerprint 返回主机密钥的 SHA256 指纹（与 ssh-keygen -lf 输出格式一致）。
func Fingerprint(k ssh.PublicKey) string {
	h := sha256.Sum256(k.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(h[:])
}

// sshSession 包装 SSH 客户端与会话。
type sshSession struct {
	client *ssh.Client
	sess   *ssh.Session
	stdin  io.WriteCloser
	out    io.Reader
	stop   chan struct{}
	once   sync.Once
}

func (s *sshSession) Read(p []byte) (int, error)  { return s.out.Read(p) }
func (s *sshSession) Write(p []byte) (int, error) { return s.stdin.Write(p) }
func (s *sshSession) Resize(c, r int) error       { return s.sess.WindowChange(r, c) }
func (s *sshSession) Close() error {
	var err error
	s.once.Do(func() {
		close(s.stop)
		s.sess.Close()
		err = s.client.Close()
	})
	return err
}

// connectSSH 建立并认证 SSH 连接（可经代理）。newFingerprint 非空表示首次连接（尚未记录主机密钥）。
// 调用前需先用 resolveCredential 把认证预设解析为实际凭据。
func connectSSH(ctx context.Context, c *Connection) (client *ssh.Client, newFingerprint string, err error) {
	s := c.SSH
	var auth []ssh.AuthMethod
	if s.AuthType == "key" {
		signer, err := loadSigner(&s)
		if err != nil {
			return nil, "", err
		}
		auth = append(auth, ssh.PublicKeys(signer))
	} else {
		auth = append(auth, ssh.Password(s.Password), ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i := range ans {
				ans[i] = s.Password
			}
			return ans, nil
		}))
	}
	timeout := time.Duration(clampTimeout(s.Timeout)) * time.Second
	cfg := &ssh.ClientConfig{
		User:    s.Username,
		Auth:    auth,
		Timeout: timeout,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp := Fingerprint(key)
			if s.HostKey == "" {
				newFingerprint = fp
				return nil
			}
			if fp != s.HostKey {
				return fmt.Errorf("主机密钥已变化（记录 %s，当前 %s），可能存在中间人攻击；确认无误后在连接设置中重置主机密钥", s.HostKey, fp)
			}
			return nil
		},
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	nc, err := dialTarget(ctx, s.Proxy, addr, timeout)
	if err != nil {
		return nil, "", err
	}
	// 认证阶段同样受超时约束；握手完成后清除期限
	_ = nc.SetDeadline(time.Now().Add(timeout))
	cc, chans, reqs, err := ssh.NewClientConn(nc, addr, cfg)
	if err != nil {
		nc.Close()
		return nil, "", err
	}
	_ = nc.SetDeadline(time.Time{})
	return ssh.NewClient(cc, chans, reqs), newFingerprint, nil
}

// shellQuote 用单引号包裹字符串，供拼接到 Shell 命令中。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// dialSSH 解析认证预设后建立 SSH 连接，不修改传入的连接配置。
func (m *Module) dialSSH(ctx context.Context, c *Connection) (*ssh.Client, string, error) {
	conn := *c
	if err := m.resolveCredential(&conn.SSH); err != nil {
		return nil, "", err
	}
	return connectSSH(ctx, &conn)
}

func (m *Module) startSSH(ctx context.Context, c *Connection, cols, rows int, notice func(string)) (session, error) {
	client, newFingerprint, err := m.dialSSH(ctx, c)
	if err != nil {
		return nil, err
	}
	if newFingerprint != "" {
		m.recordHostKey(c.Key, newFingerprint)
		notice("首次连接，已信任主机密钥 " + newFingerprint)
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	stdin, _ := sess.StdinPipe()
	pr, pw := io.Pipe()
	sess.Stdout, sess.Stderr = pw, pw
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		client.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		client.Close()
		return nil, err
	}
	ss := &sshSession{client: client, sess: sess, stdin: stdin, out: pr, stop: make(chan struct{})}
	go func() {
		_ = sess.Wait()
		pw.Close()
	}()
	// 登录后切换工作目录
	if dir := strings.TrimSpace(c.SSH.WorkDir); dir != "" {
		_, _ = stdin.Write([]byte("cd " + shellQuote(dir) + "\n"))
	}
	// 定期发送保活请求，避免空闲连接被中间设备断开
	if ka := c.SSH.KeepAlive; ka > 0 {
		go func() {
			t := time.NewTicker(time.Duration(ka) * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ss.stop:
					return
				case <-t.C:
					if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
						return
					}
				}
			}
		}()
	}
	return ss, nil
}

func (m *Module) recordHostKey(key, fp string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Connections {
		if m.cfg.Connections[i].Key == key && m.cfg.Connections[i].SSH.HostKey == "" {
			m.cfg.Connections[i].SSH.HostKey = fp
			_ = m.saveLocked()
		}
	}
}

// handleTestSaved 测试已保存的连接。已保存的密码或认证预设会发往连接中的主机，而主机可以先经修改接口改掉，
// 因此与打开终端一样先过门禁（协议与二次验证）。
func (m *Module) handleTestSaved(w http.ResponseWriter, r *http.Request) {
	if !m.requireAccess(w, r) {
		return
	}
	conn, ok := m.findConn(r.PathValue("key"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "连接不存在")
		return
	}
	m.runTest(w, r, &conn)
}

// handleTestDraft 测试未保存的连接；body 带 key 时，被脱敏的密文字段从该已保存连接合并。
// 合并后的密码、引用的认证预设都会发往请求指定的主机，等同于取得明文，与查看明文、复制连接一样先过门禁（协议与二次验证）；
// 只用请求中自带密码的草稿不涉及已保存的密钥，不受门禁限制。
func (m *Module) handleTestDraft(w http.ResponseWriter, r *http.Request) {
	var c Connection
	if err := httpx.ReadJSON(r, &c); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if (c.Key != "" || (c.Type == TypeSSH && c.SSH.AuthType == AuthCredential)) && !m.requireAccess(w, r) {
		return
	}
	if c.Key != "" {
		if old, ok := m.findConn(c.Key); ok {
			mergeSecrets(&old, &c)
		}
	}
	if err := c.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.runTest(w, r, &c)
}

// runTest 测试 SSH 或 Telnet 连通性：SSH 完成认证后立即断开，不记录主机密钥；Telnet 完成 TCP 连接（含代理）即成功。
func (m *Module) runTest(w http.ResponseWriter, r *http.Request, conn *Connection) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	start := time.Now()
	switch conn.Type {
	case TypeSSH:
		client, fp, err := m.dialSSH(ctx, conn)
		if err != nil {
			httpx.OK(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		version := string(client.ServerVersion())
		client.Close()
		httpx.OK(w, map[string]any{"ok": true, "serverVersion": version, "newHostKey": fp, "latencyMs": time.Since(start).Milliseconds()})
	case TypeTelnet:
		t := conn.Telnet
		nc, err := dialTelnet(ctx, &t)
		if err != nil {
			httpx.OK(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		nc.Close()
		httpx.OK(w, map[string]any{"ok": true, "latencyMs": time.Since(start).Milliseconds()})
	default:
		httpx.Error(w, http.StatusBadRequest, "只能测试 SSH 或 Telnet 连接")
	}
}
