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
	"slices"
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
	// HostKeyType 为该指纹对应的密钥类型（如 ssh-ed25519、ecdsa-sha2-nistp256），连接时只协商这种主机密钥。
	// 服务器通常同时有多把不同类型的主机密钥，只记指纹时无法知道该让服务器出示哪一把；为空（旧配置或手工填写的指纹）时连接会自动探测并补记
	HostKeyType string `json:"hostKeyType"`
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

// hostKeyInfo 是本次连接中服务器出示的主机密钥。
type hostKeyInfo struct {
	Fingerprint string
	Type        string
	// New 表示连接前尚未记录主机密钥（首次信任）
	New bool
}

// hostKeyMismatch 表示服务器出示的主机密钥与记录的指纹不符。
type hostKeyMismatch struct {
	recorded, recordedType string
	got, gotType           string
}

func (e *hostKeyMismatch) Error() string {
	return fmt.Sprintf("主机密钥已变化（记录 %s，当前 %s），可能存在中间人攻击；确认无误后在连接设置中重置主机密钥",
		describeHostKey(e.recordedType, e.recorded), describeHostKey(e.gotType, e.got))
}

// describeHostKey 把密钥类型与指纹拼成「ED25519 SHA256:…」的形式，与 ssh-keygen -l 的类型写法一致，便于对照。
func describeHostKey(typ, fp string) string {
	if typ == "" {
		return fp
	}
	return hostKeyTypeLabel(typ) + " " + fp
}

func hostKeyTypeLabel(typ string) string {
	// 主机证书（如 ssh-ed25519-cert-v01@openssh.com）显示为 ED25519-CERT，与 ssh-keygen -l 的写法一致
	if base, ok := strings.CutSuffix(typ, "-cert-v01@openssh.com"); ok {
		if label := plainHostKeyLabel(base); label != base {
			return label + "-CERT"
		}
		return typ
	}
	return plainHostKeyLabel(typ)
}

// plainHostKeyLabel 返回普通主机密钥类型的短名，未知类型原样返回。
func plainHostKeyLabel(typ string) string {
	switch {
	case typ == ssh.KeyAlgoED25519:
		return "ED25519"
	case typ == ssh.KeyAlgoRSA:
		return "RSA"
	case strings.HasPrefix(typ, "ecdsa-sha2-"):
		return "ECDSA"
	}
	return typ
}

// hostKeyFormats 是按 x/crypto 默认偏好排列的普通主机密钥类型，记录的指纹与协商到的密钥不符时按此逐一换类型重试。
var hostKeyFormats = []string{ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoRSA, ssh.KeyAlgoED25519}

// knownHostKeyTypes 是可以记录并用于限定协商的主机密钥类型（含主机证书）。
var knownHostKeyTypes = append(slices.Clone(hostKeyFormats),
	ssh.CertAlgoECDSA256v01, ssh.CertAlgoECDSA384v01, ssh.CertAlgoECDSA521v01, ssh.CertAlgoRSAv01, ssh.CertAlgoED25519v01)

// hostKeyAlgorithms 返回协商某种主机密钥类型时可用的签名算法（RSA 密钥对应多种签名算法）。
func hostKeyAlgorithms(format string) []string {
	switch format {
	case ssh.KeyAlgoRSA:
		return []string{ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSA}
	case ssh.CertAlgoRSAv01:
		return []string{ssh.CertAlgoRSASHA256v01, ssh.CertAlgoRSASHA512v01, ssh.CertAlgoRSAv01}
	}
	return []string{format}
}

// isNoCommonHostKeyAlgo 判断握手是否因双方没有共同的主机密钥算法而失败（限定的类型服务器已不再提供）。
func isNoCommonHostKeyAlgo(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no common algorithm for host key")
}

// connectSSH 建立并认证 SSH 连接（可经代理），返回服务器出示的主机密钥。
// 已记录指纹时，只接受指纹一致的主机密钥；服务器通常有多把不同类型的主机密钥，
// 记录了类型时只协商这种类型，未记录类型或协商到的不是记录的那把时，逐一限定其余类型重新握手查找。
// 主机密钥校验在认证之前进行，校验不过的握手不会发送密码或签名。
// 调用前需先用 resolveCredential 把认证预设解析为实际凭据。
func connectSSH(ctx context.Context, c *Connection) (*ssh.Client, hostKeyInfo, error) {
	s := c.SSH
	// 类型可经接口写入，只认识的类型才用于限定协商，其他值按未记录类型处理（自动探测后补正）
	if !slices.Contains(knownHostKeyTypes, s.HostKeyType) {
		s.HostKeyType = ""
	}
	var algos []string
	if s.HostKey != "" && s.HostKeyType != "" {
		algos = hostKeyAlgorithms(s.HostKeyType)
	}
	client, info, err := sshHandshake(ctx, &s, algos)
	if err == nil || s.HostKey == "" {
		return client, info, err
	}
	var mismatch *hostKeyMismatch
	if !errors.As(err, &mismatch) && !(algos != nil && isNoCommonHostKeyAlgo(err)) {
		return nil, hostKeyInfo{}, err
	}
	// 记录的指纹可能属于服务器另一种类型的主机密钥：旧版本只记指纹、手工填写或从其他工具复制的指纹
	// （例如 ssh-keyscan 取的 Ed25519，而 x/crypto 默认优先协商 ECDSA）。只接受与记录指纹一致的密钥，安全性不变
	firstErr := err
	tried := map[string]bool{}
	if s.HostKeyType != "" {
		tried[s.HostKeyType] = true
	}
	if mismatch != nil {
		tried[mismatch.gotType] = true
	}
	for ctx.Err() == nil {
		var cand []string
		for _, f := range hostKeyFormats {
			if !tried[f] {
				cand = append(cand, hostKeyAlgorithms(f)...)
			}
		}
		if len(cand) == 0 {
			break
		}
		client, info, err = sshHandshake(ctx, &s, cand)
		if err == nil {
			return client, info, nil
		}
		var m *hostKeyMismatch
		if !errors.As(err, &m) {
			if isNoCommonHostKeyAlgo(err) {
				// 其余类型服务器都没有
				break
			}
			// 认证失败等错误发生在主机密钥校验通过之后，网络错误则无从继续，都如实返回
			return nil, hostKeyInfo{}, err
		}
		if tried[m.gotType] {
			break
		}
		tried[m.gotType] = true
		if mismatch == nil {
			mismatch = m
		}
	}
	// 服务器的所有主机密钥都与记录不符：优先报告指纹不符，让用户看到当前指纹
	if mismatch != nil {
		return nil, hostKeyInfo{}, mismatch
	}
	return nil, hostKeyInfo{}, firstErr
}

// sshHandshake 完成一次 TCP 连接、SSH 握手与认证；algos 非空时只接受这些主机密钥算法。
func sshHandshake(ctx context.Context, s *SSHConfig, algos []string) (client *ssh.Client, info hostKeyInfo, err error) {
	var auth []ssh.AuthMethod
	if s.AuthType == "key" {
		signer, err := loadSigner(s)
		if err != nil {
			return nil, hostKeyInfo{}, err
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
		User:              s.Username,
		Auth:              auth,
		Timeout:           timeout,
		HostKeyAlgorithms: algos,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp := Fingerprint(key)
			info = hostKeyInfo{Fingerprint: fp, Type: key.Type(), New: s.HostKey == ""}
			if s.HostKey == "" {
				return nil
			}
			if fp != s.HostKey {
				return &hostKeyMismatch{recorded: s.HostKey, recordedType: s.HostKeyType, got: fp, gotType: key.Type()}
			}
			return nil
		},
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	nc, err := dialTarget(ctx, s.Proxy, addr, timeout)
	if err != nil {
		return nil, hostKeyInfo{}, err
	}
	// 认证阶段同样受超时约束；握手完成后清除期限
	_ = nc.SetDeadline(time.Now().Add(timeout))
	cc, chans, reqs, err := ssh.NewClientConn(nc, addr, cfg)
	if err != nil {
		nc.Close()
		return nil, hostKeyInfo{}, err
	}
	_ = nc.SetDeadline(time.Time{})
	return ssh.NewClient(cc, chans, reqs), info, nil
}

// shellQuote 用单引号包裹字符串，供拼接到 Shell 命令中。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// dialSSH 解析认证预设后建立 SSH 连接，不修改传入的连接配置。
func (m *Module) dialSSH(ctx context.Context, c *Connection) (*ssh.Client, hostKeyInfo, error) {
	conn := *c
	if err := m.resolveCredential(&conn.SSH); err != nil {
		return nil, hostKeyInfo{}, err
	}
	return connectSSH(ctx, &conn)
}

func (m *Module) startSSH(ctx context.Context, c *Connection, cols, rows int, notice func(string)) (session, error) {
	client, hk, err := m.dialSSH(ctx, c)
	if err != nil {
		return nil, err
	}
	m.recordHostKey(c.Key, hk)
	if hk.New {
		notice("首次连接，已信任主机密钥 " + describeHostKey(hk.Type, hk.Fingerprint))
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

// recordHostKey 在连接成功后更新记录：首次连接时信任并记录指纹与类型；
// 指纹已记录但缺少类型（或类型不对应）时补记类型，以后连接直接协商这种密钥，不必逐一探测。
// 只在记录的指纹与本次实际校验通过的指纹一致时补记，期间被重置或改动过的记录不受影响。
func (m *Module) recordHostKey(key string, hk hostKeyInfo) {
	if hk.Fingerprint == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Connections {
		s := &m.cfg.Connections[i].SSH
		if m.cfg.Connections[i].Key != key {
			continue
		}
		switch {
		case s.HostKey == "" && hk.New:
			s.HostKey, s.HostKeyType = hk.Fingerprint, hk.Type
		case s.HostKey == hk.Fingerprint && s.HostKeyType != hk.Type:
			s.HostKeyType = hk.Type
		default:
			continue
		}
		_ = m.saveLocked()
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
		client, hk, err := m.dialSSH(ctx, conn)
		if err != nil {
			httpx.OK(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		version := string(client.ServerVersion())
		client.Close()
		newHostKey, newHostKeyType := "", ""
		if hk.New {
			newHostKey, newHostKeyType = hk.Fingerprint, hk.Type
		}
		httpx.OK(w, map[string]any{"ok": true, "serverVersion": version, "newHostKey": newHostKey, "newHostKeyType": newHostKeyType, "latencyMs": time.Since(start).Milliseconds()})
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
