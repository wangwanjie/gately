// Package ssl 实现证书管理模块：ACME 自动签发与续期、上传证书、读取文件证书，
// 并可把证书写到目录供其他服务使用，同时为 Web 服务模块提供按 SNI 选择证书的能力。
package ssl

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/wangwanjie/gately/internal/events"
	stdlog "log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	legolog "github.com/go-acme/lego/v4/log"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/script"
)

// ID 是模块标识。
const ID = "ssl"

// 可在测试中调整的时间参数
var (
	checkInterval  = 30 * time.Minute
	firstCheck     = 30 * time.Second
	failureBackoff = 6 * time.Hour
	// shortLivedBackoff 是短期证书签发失败后的重试间隔：有效期只有几天，6 小时的退避会错过续签窗口
	shortLivedBackoff = time.Hour
)

// shortLivedLife 是短期证书的判断标准：总有效期不足 10 天
const shortLivedLife = 10 * 24 * time.Hour

// CertState 是证书的运行状态（不持久化）。
type CertState struct {
	Obtaining   bool      `json:"obtaining"`
	LastError   string    `json:"lastError,omitempty"`
	LastAttempt time.Time `json:"lastAttempt,omitzero"`
	NextRetry   time.Time `json:"nextRetry,omitzero"`
	MappingNote string    `json:"mappingNote,omitempty"`
	// Deploys 为各部署目标的运行状态，与 Cert.Deploys 按下标对应
	Deploys []DeployState `json:"deploys,omitempty"`

	// lastExpiryWarn 是上次发出到期事件的时间，用于每天最多提醒一次
	lastExpiryWarn time.Time
}

// Module 是证书模块。
type Module struct {
	mu     sync.Mutex
	store  *config.Store
	logs   *logbuf.Buffer
	cfg    fileConfig
	ctx    context.Context
	states map[string]*CertState
	// obtainSem 保证同一时间只有一个 ACME 签发流程
	obtainSem chan struct{}
	wg        sync.WaitGroup

	// tlsMu 保护握手时使用的证书索引，与 mu 分离避免握手被配置操作阻塞
	tlsMu    sync.RWMutex
	tlsIndex map[string]*tls.Certificate
	wildcard map[string]*tls.Certificate
	// ipDefault 为第一张 IP 地址证书，用于没有 SNI 且按本机地址找不到证书的连接
	ipDefault *tls.Certificate

	// deployers 由主程序注入的部署能力（SSH 推送、重启容器）
	deployers Deployers
}

// New 创建模块实例。
func New() *Module {
	return &Module{states: map[string]*CertState{}, obtainSem: make(chan struct{}, 1)}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "SSL 证书" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
	m.cfg = fileConfig{Settings: defaultSettings(), Accounts: map[string]Account{}}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	if m.cfg.Accounts == nil {
		m.cfg.Accounts = map[string]Account{}
	}
	m.cfg.Settings.normalize()
	m.rebuildTLSLocked()
	// lego 的过程日志写入本模块日志（调试级别），方便排查签发问题
	legolog.Logger = stdlog.New(logWriter{m.logs}, "", 0)
	return nil
}

// logWriter 把标准库日志输出转到模块日志缓冲。
type logWriter struct{ buf *logbuf.Buffer }

func (w logWriter) Write(p []byte) (int, error) {
	w.buf.Add(logbuf.LevelDebug, "acme", strings.TrimSpace(string(p)))
	return len(p), nil
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	// 启动时确保映射文件与当前证书一致
	for i := range m.cfg.Certs {
		if m.cfg.Certs[i].Enable {
			m.applyMappingLocked(&m.cfg.Certs[i], false)
			// 启动时补做尚未成功的部署（例如上次部署失败或新增了目标）
			m.scheduleDeploysLocked(&m.cfg.Certs[i])
		}
	}
	m.mu.Unlock()
	m.wg.Add(1)
	go m.scheduler(ctx)
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.wg.Wait()
	return nil
}

func (m *Module) saveLocked() error {
	return m.store.Save(ID, m.cfg)
}

func (m *Module) stateLocked(key string) *CertState {
	st := m.states[key]
	if st == nil {
		st = &CertState{}
		m.states[key] = st
	}
	return st
}

func (m *Module) findLocked(key string) *Cert {
	for i := range m.cfg.Certs {
		if m.cfg.Certs[i].Key == key {
			return &m.cfg.Certs[i]
		}
	}
	return nil
}

func label(c *Cert) string {
	if c.Remark != "" {
		return c.Remark
	}
	if len(c.ACME.Domains) > 0 {
		return c.ACME.Domains[0]
	}
	return c.Key
}

func (m *Module) scheduler(ctx context.Context) {
	defer m.wg.Done()
	timer := time.NewTimer(firstCheck)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		m.checkAll(false)
		timer.Reset(checkInterval)
	}
}

// needsRenewal 判断证书是否需要（重新）签发。
func (m *Module) needsRenewal(c *Cert, now time.Time) (bool, string) {
	if c.CertPEM == "" {
		return true, "尚未签发"
	}
	info, err := certInfo(c.CertPEM)
	if err != nil {
		return true, "证书无法解析"
	}
	if !sameDomains(info.Domains, c.ACME.Domains) {
		return true, "域名列表已变化"
	}
	if c.ACME.Profile != c.IssuedProfile {
		return true, "证书配置已变化"
	}
	life := info.NotAfter.Sub(info.NotBefore)
	left := info.NotAfter.Sub(now)
	if life < shortLivedLife {
		if left < time.Duration(m.cfg.Settings.ShortLivedRenewHours)*time.Hour {
			return true, fmt.Sprintf("短期证书剩余 %.0f 小时", left.Hours())
		}
		return false, ""
	}
	if left < time.Duration(m.cfg.Settings.RenewBeforeDays)*24*time.Hour {
		return true, fmt.Sprintf("剩余 %.0f 天", left.Hours()/24)
	}
	return false, ""
}

// isShortLived 判断证书是否按短期证书处理：配置为短期证书，或当前证书总有效期不足 10 天。
func isShortLived(c *Cert) bool {
	if c.ACME.Profile == ProfileShortLived {
		return true
	}
	if c.CertPEM == "" {
		return false
	}
	info, err := certInfo(c.CertPEM)
	return err == nil && info.NotAfter.Sub(info.NotBefore) < shortLivedLife
}

func sameDomains(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, d := range a {
		set[strings.ToLower(d)] = true
	}
	for _, d := range b {
		if !set[strings.ToLower(d)] {
			return false
		}
	}
	return true
}

// checkAll 检查全部证书：文件来源重新读取，ACME 来源按需续签。force 时忽略失败退避。
func (m *Module) checkAll(force bool) {
	m.mu.Lock()
	if !m.cfg.Settings.Enable {
		m.mu.Unlock()
		return
	}
	now := time.Now()
	var due []string
	for i := range m.cfg.Certs {
		c := &m.cfg.Certs[i]
		if !c.Enable {
			continue
		}
		// 失败的部署在退避结束后重试
		m.scheduleDeploysLocked(c)
		switch c.Source {
		case SourceFile:
			m.reloadFileLocked(c)
			m.warnExpiringLocked(c, now)
		case SourceACME:
			st := m.stateLocked(c.Key)
			if st.Obtaining || (!force && now.Before(st.NextRetry)) {
				continue
			}
			m.warnExpiringLocked(c, now)
			if ok, reason := m.needsRenewal(c, now); ok {
				m.logs.Logger(label(c)).Infof("需要签发：%s", reason)
				// 排队时即标记，避免手动续签与定时检查重复签发同一证书
				st.Obtaining = true
				due = append(due, c.Key)
			}
		}
	}
	m.mu.Unlock()
	for _, key := range due {
		m.obtainNow(key)
	}
}

// triggerLocked 在后台为证书执行一次签发。
func (m *Module) triggerLocked(key string) {
	st := m.stateLocked(key)
	if st.Obtaining || m.ctx == nil {
		return
	}
	st.Obtaining = true
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.obtainNow(key)
	}()
}

// obtainNow 同步执行签发，结果写回配置。调用方已把状态标记为 Obtaining。
func (m *Module) obtainNow(key string) {
	cancelQueued := func() {
		m.mu.Lock()
		if st, ok := m.states[key]; ok {
			st.Obtaining = false
		}
		m.mu.Unlock()
	}
	select {
	case m.obtainSem <- struct{}{}:
	case <-m.done():
		cancelQueued()
		return
	}
	defer func() { <-m.obtainSem }()
	// 等待期间模块可能已停止
	select {
	case <-m.done():
		cancelQueued()
		return
	default:
	}

	m.mu.Lock()
	c := m.findLocked(key)
	if c == nil || c.Source != SourceACME {
		if st, ok := m.states[key]; ok {
			st.Obtaining = false
		}
		m.mu.Unlock()
		return
	}
	snapshot := *c
	cfg := c.ACME
	st := m.stateLocked(key)
	st.Obtaining = true
	st.LastAttempt = time.Now()
	var acc *Account
	if a, ok := m.cfg.Accounts[accountKey(cfg.directoryURL(), cfg.Email)]; ok {
		acc = &a
	}
	log := m.logs.Logger(label(c))
	m.mu.Unlock()

	res, err := obtain(cfg, acc, log)

	m.mu.Lock()
	defer m.mu.Unlock()
	st = m.stateLocked(key)
	st.Obtaining = false
	if res != nil && res.Account != nil {
		m.cfg.Accounts[accountKey(cfg.directoryURL(), cfg.Email)] = *res.Account
	}
	if err != nil {
		st.LastError = err.Error()
		backoff := failureBackoff
		if isShortLived(&snapshot) {
			backoff = shortLivedBackoff
		}
		st.NextRetry = time.Now().Add(backoff)
		log.Errorf("%v", err)
		events.Publish(events.Event{
			Type:    events.SSLRenewFailed,
			Source:  label(c),
			Level:   events.LevelError,
			Title:   "证书签发或续期失败：" + label(c),
			Message: err.Error(),
		})
		if res != nil && res.Account != nil {
			_ = m.saveLocked()
		}
		return
	}
	c = m.findLocked(key)
	// 签发期间配置被修改或删除时丢弃结果，由新配置重新触发
	if c == nil || !reflect.DeepEqual(c.ACME, snapshot.ACME) {
		log.Warnf("签发期间配置已变化，丢弃本次结果")
		return
	}
	c.CertPEM, c.KeyPEM, c.IssuerPEM = res.CertPEM, res.KeyPEM, res.IssuerPEM
	c.IssuedProfile = cfg.Profile
	c.UpdatedAt = time.Now()
	st.LastError = ""
	st.NextRetry = time.Time{}
	if err := m.saveLocked(); err != nil {
		log.Errorf("保存证书失败: %v", err)
		return
	}
	if info, err := certInfo(c.CertPEM); err == nil {
		log.Infof("证书签发成功，有效期至 %s", info.NotAfter.Local().Format("2006-01-02 15:04"))
	}
	m.rebuildTLSLocked()
	m.applyMappingLocked(c, true)
	m.scheduleDeploysLocked(c)
}

// done 返回模块退出信号；模块尚未启动时返回永不关闭的通道。
func (m *Module) done() <-chan struct{} {
	if m.ctx == nil {
		return make(chan struct{})
	}
	return m.ctx.Done()
}

// reloadFileLocked 重新读取文件来源的证书，内容变化时更新。
func (m *Module) reloadFileLocked(c *Cert) {
	st := m.stateLocked(c.Key)
	certPEM, err1 := os.ReadFile(c.CertFile)
	keyPEM, err2 := os.ReadFile(c.KeyFile)
	if err := errors.Join(err1, err2); err != nil {
		st.LastError = "读取证书文件失败: " + err.Error()
		return
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		st.LastError = "证书文件无效: " + err.Error()
		return
	}
	st.LastError = ""
	if string(certPEM) == c.CertPEM && string(keyPEM) == c.KeyPEM {
		return
	}
	c.CertPEM, c.KeyPEM = string(certPEM), string(keyPEM)
	c.UpdatedAt = time.Now()
	_ = m.saveLocked()
	m.logs.Logger(label(c)).Infof("已从文件加载新证书")
	m.rebuildTLSLocked()
	m.applyMappingLocked(c, true)
	m.scheduleDeploysLocked(c)
}

// mappingFiles 返回映射目录下需要写入的文件。
func mappingFiles(c *Cert) map[string][]byte {
	base := c.fileBase()
	files := map[string][]byte{
		base + ".crt": pemBytes(c.CertPEM),
		base + ".pem": pemBytes(c.CertPEM),
		base + ".key": pemBytes(c.KeyPEM),
	}
	if c.IssuerPEM != "" {
		files[base+"_issuerCertificate.crt"] = pemBytes(c.IssuerPEM)
	}
	return files
}

// pemBytes 保证 PEM 内容以换行结尾（部分程序拼接证书时依赖结尾换行）。
func pemBytes(s string) []byte {
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s)
}

// applyMappingLocked 把证书写到映射目录，内容变化且 runScript 为 true 时执行脚本。
func (m *Module) applyMappingLocked(c *Cert, runScript bool) {
	st := m.stateLocked(c.Key)
	if !c.Mapping.Enable || c.CertPEM == "" || c.KeyPEM == "" {
		st.MappingNote = ""
		return
	}
	log := m.logs.Logger(label(c))
	if err := os.MkdirAll(c.Mapping.Dir, 0o755); err != nil {
		st.MappingNote = "创建目录失败: " + err.Error()
		log.Errorf("%s", st.MappingNote)
		return
	}
	changed := false
	for name, data := range mappingFiles(c) {
		p := filepath.Join(c.Mapping.Dir, name)
		if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, data) {
			continue
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".key") {
			mode = 0o600
		}
		if err := writeFileAtomic(p, data, mode); err != nil {
			st.MappingNote = "写入失败: " + err.Error()
			log.Errorf("写入 %s 失败: %v", p, err)
			return
		}
		changed = true
	}
	st.MappingNote = "已映射到 " + c.Mapping.Dir
	if !changed {
		return
	}
	log.Infof("证书文件已更新: %s", c.Mapping.Dir)
	if runScript && strings.TrimSpace(c.Mapping.Script) != "" {
		vars := map[string]string{
			"certName": c.fileBase(),
			"certDir":  c.Mapping.Dir,
			"certFile": filepath.Join(c.Mapping.Dir, c.fileBase()+".crt"),
			"keyFile":  filepath.Join(c.Mapping.Dir, c.fileBase()+".key"),
		}
		content := c.Mapping.Script
		// 脚本在后台执行，避免持锁阻塞
		go func() {
			out, err := script.Run(content, vars)
			if err != nil {
				log.Errorf("证书变化脚本执行失败: %v %s", err, script.Summarize(out, 1000))
				return
			}
			log.Infof("证书变化脚本执行完成 %s", script.Summarize(out, 1000))
		}()
	}
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gately-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// rebuildTLSLocked 根据启用的证书重建 SNI 索引。
// 多张证书覆盖同一域名时选用到期时间最晚的一张（到期时间相同取列表中靠前的），
// 这样列表排序与分组只影响展示，不会改变实际使用的证书。
func (m *Module) rebuildTLSLocked() {
	type candidate struct {
		cert     *tls.Certificate
		notAfter time.Time
	}
	exact := map[string]candidate{}
	wild := map[string]candidate{}
	var ipDefault candidate
	// better 判断新证书是否应替换已选中的证书
	better := func(cur candidate, notAfter time.Time) bool {
		return cur.cert == nil || notAfter.After(cur.notAfter)
	}
	for i := range m.cfg.Certs {
		c := &m.cfg.Certs[i]
		if !c.Enable || c.CertPEM == "" || c.KeyPEM == "" {
			continue
		}
		pair, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		if err != nil {
			continue
		}
		info, err := certInfo(c.CertPEM)
		if err != nil {
			continue
		}
		// 已过期的证书不参与选择
		if time.Now().After(info.NotAfter) {
			continue
		}
		cand := candidate{cert: &pair, notAfter: info.NotAfter}
		for _, d := range info.Domains {
			d = strings.ToLower(d)
			if _, ok := parseIPIdentifier(d); ok && better(ipDefault, info.NotAfter) {
				ipDefault = cand
			}
			if strings.HasPrefix(d, "*.") {
				if better(wild[d[2:]], info.NotAfter) {
					wild[d[2:]] = cand
				}
			} else if better(exact[d], info.NotAfter) {
				exact[d] = cand
			}
		}
	}
	exactIdx := make(map[string]*tls.Certificate, len(exact))
	for d, c := range exact {
		exactIdx[d] = c.cert
	}
	wildIdx := make(map[string]*tls.Certificate, len(wild))
	for d, c := range wild {
		wildIdx[d] = c.cert
	}
	m.tlsMu.Lock()
	m.tlsIndex, m.wildcard, m.ipDefault = exactIdx, wildIdx, ipDefault.cert
	m.tlsMu.Unlock()
}

// GetCertificate 按 SNI 选择证书，供 Web 服务模块的 tls.Config 使用。
func (m *Module) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
	// 个别客户端会把 IP 地址放进 SNI，统一成证书索引中的规范写法
	if ip, ok := parseIPIdentifier(name); ok {
		name = ip.String()
	}
	m.tlsMu.RLock()
	defer m.tlsMu.RUnlock()
	if c, ok := m.tlsIndex[name]; ok {
		return c, nil
	}
	if i := strings.IndexByte(name, '.'); i > 0 {
		if c, ok := m.wildcard[name[i+1:]]; ok {
			return c, nil
		}
	}
	if name == "" {
		// 按 IP 地址访问时客户端不发送 SNI：先按连接的本机地址选择 IP 证书，
		// 找不到时（经 NAT 或端口转发，本机地址不是证书中的公网 IP）使用第一张 IP 证书
		if hello.Conn != nil {
			if ap, err := netip.ParseAddrPort(hello.Conn.LocalAddr().String()); err == nil {
				if c, ok := m.tlsIndex[ap.Addr().Unmap().WithZone("").String()]; ok {
					return c, nil
				}
			}
		}
		if m.ipDefault != nil {
			return m.ipDefault, nil
		}
	}
	return nil, fmt.Errorf("没有匹配 %q 的证书", name)
}

// HasCertificate 判断是否有覆盖该域名的证书。
func (m *Module) HasCertificate(name string) bool {
	// 空名（如不带 Host 的 HTTP/1.0 请求）不算有证书；无 SNI 的回退只用于 TLS 握手
	if name == "" {
		return false
	}
	_, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
	return err == nil
}

// CertView 是 API 输出的证书，私钥与凭据脱敏。
type CertView struct {
	Cert
	Info  *CertInfo `json:"info,omitempty"`
	State CertState `json:"state"`
}

func (m *Module) viewLocked(c *Cert) any {
	v := CertView{Cert: *c, State: *m.stateLocked(c.Key)}
	v.KeyPEM = crud.Mask(c.KeyPEM)
	v.ACME.Credentials.Secret = crud.Mask(c.ACME.Credentials.Secret)
	v.ACME.EABHMAC = crud.Mask(c.ACME.EABHMAC)
	if c.CertPEM != "" {
		v.Info, _ = certInfo(c.CertPEM)
	}
	return v
}

// changedLocked 在证书配置变化后更新运行状态。
func (m *Module) changedLocked(key string) {
	c := m.findLocked(key)
	if c == nil {
		return
	}
	m.rebuildTLSLocked()
	if !c.Enable {
		return
	}
	// 部署目标可能被增删或调整顺序，运行状态按新的下标重新累计
	if st := m.stateLocked(key); len(st.Deploys) > len(c.Deploys) {
		st.Deploys = st.Deploys[:len(c.Deploys)]
	}
	for i := range c.Deploys {
		// 用户修改配置后立即重试，不等失败退避结束
		m.deployStateLocked(key, i).NextRetry = time.Time{}
	}
	switch c.Source {
	case SourceUpload:
		c.UpdatedAt = time.Now()
		m.applyMappingLocked(c, true)
		m.scheduleDeploysLocked(c)
	case SourceFile:
		m.reloadFileLocked(c)
		m.scheduleDeploysLocked(c)
	case SourceACME:
		st := m.stateLocked(key)
		st.NextRetry = time.Time{}
		if ok, _ := m.needsRenewal(c, time.Now()); ok {
			m.triggerLocked(key)
		} else {
			m.applyMappingLocked(c, true)
			m.scheduleDeploysLocked(c)
		}
	}
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	certs := &crud.Collection[Cert]{
		Mu:      &m.mu,
		Items:   &m.cfg.Certs,
		Save:    m.saveLocked,
		Key:     func(c *Cert) *string { return &c.Key },
		Enabled: func(c *Cert) *bool { return &c.Enable },
		Group:   func(c *Cert) *string { return &c.Group },
		Validate: func(c *Cert) error {
			return c.Validate(m.cfg.Settings)
		},
		Merge: func(old, updated *Cert) {
			crud.KeepSecret(old.KeyPEM, &updated.KeyPEM)
			crud.KeepSecret(old.ACME.Credentials.Secret, &updated.ACME.Credentials.Secret)
			crud.KeepSecret(old.ACME.EABHMAC, &updated.ACME.EABHMAC)
			// ACME 证书内容由系统维护，编辑配置时保留已签发的证书
			if updated.Source == SourceACME && old.Source == SourceACME {
				updated.CertPEM, updated.KeyPEM, updated.IssuerPEM, updated.UpdatedAt = old.CertPEM, old.KeyPEM, old.IssuerPEM, old.UpdatedAt
				updated.IssuedProfile = old.IssuedProfile
			}
			keepDeployRecords(old, updated)
		},
		View: m.viewLocked,
		// 查看被脱敏的 DNS 凭据密钥与 EAB HMAC 明文；私钥可通过下载接口取得，此处不返回
		Secrets: func(c *Cert) map[string]string {
			return map[string]string{"secret": c.ACME.Credentials.Secret, "eabHmac": c.ACME.EABHMAC}
		},
		Changed: m.changedLocked,
		Removed: func(key string) {
			delete(m.states, key)
			m.rebuildTLSLocked()
		},
	}
	certs.Register(r, "/certs")
	r.HandleFunc("POST /certs/{key}/renew", m.handleRenew)
	r.HandleFunc("GET /certs/{key}/download", m.handleDownload)
	r.HandleFunc("POST /certs/{key}/deploy", m.handleDeploy)
	r.HandleFunc("GET /deploy-targets", m.handleDeployTargets)
	r.HandleFunc("GET /providers", func(w http.ResponseWriter, r *http.Request) {
		var out []dnsprovider.Meta
		for _, p := range dnsprovider.List() {
			if p.SupportsACME {
				out = append(out, p)
			}
		}
		httpx.OK(w, out)
	})
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, m.cfg.Settings)
	})
	r.HandleFunc("PUT /settings", m.handlePutSettings)
}

func (m *Module) handleRenew(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.findLocked(key)
	if c == nil {
		httpx.Error(w, http.StatusNotFound, "证书不存在")
		return
	}
	if c.Source != SourceACME {
		httpx.Error(w, http.StatusBadRequest, "只有 ACME 证书可以续签")
		return
	}
	if m.stateLocked(key).Obtaining {
		httpx.Error(w, http.StatusConflict, "正在签发中")
		return
	}
	m.triggerLocked(key)
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleDownload(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	kind := r.URL.Query().Get("type")
	m.mu.Lock()
	c := m.findLocked(key)
	var data, name string
	if c != nil {
		base := c.fileBase()
		if base == "" {
			base = "certificate"
		}
		switch kind {
		case "crt", "pem":
			data, name = c.CertPEM, base+"."+kind
		case "key":
			data, name = c.KeyPEM, base+".key"
		case "issuer":
			data, name = c.IssuerPEM, base+"_issuerCertificate.crt"
		}
	}
	m.mu.Unlock()
	if data == "" {
		httpx.Error(w, http.StatusNotFound, "没有可下载的内容")
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write([]byte(data))
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(r, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	s.normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	m.cfg.Settings = s
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, s)
}

// expiryWarnDays 是证书剩余天数低于该值时发出到期事件。
const expiryWarnDays = 7

// warnExpiringLocked 在证书即将到期时发布事件。同一证书每天最多一次，调用方需持有 m.mu。
func (m *Module) warnExpiringLocked(c *Cert, now time.Time) {
	if c.CertPEM == "" {
		return
	}
	info, err := certInfo(c.CertPEM)
	if err != nil {
		return
	}
	left := info.NotAfter.Sub(now)
	days := int(left.Hours() / 24)
	shortLived := info.NotAfter.Sub(info.NotBefore) < shortLivedLife
	if shortLived {
		// 短期证书的总有效期不足 7 天，按天数判断会一直提醒；
		// 剩余时间不到续签提前量的一半，说明续签已经多次失败
		if left >= time.Duration(m.cfg.Settings.ShortLivedRenewHours)*time.Hour/2 {
			return
		}
	} else if days >= expiryWarnDays {
		return
	}
	st := m.stateLocked(c.Key)
	if !st.lastExpiryWarn.IsZero() && now.Sub(st.lastExpiryWarn) < 24*time.Hour {
		return
	}
	st.lastExpiryWarn = now
	events.Publish(events.Event{
		Type:    events.SSLExpiring,
		Source:  label(c),
		Level:   events.LevelWarn,
		Title:   "证书即将到期：" + label(c),
		Message: expiryMessage(info, left, shortLived),
	})
}

// expiryMessage 返回到期提醒的说明，短期证书按小时显示剩余时间。
func expiryMessage(info *CertInfo, left time.Duration, shortLived bool) string {
	if shortLived {
		return fmt.Sprintf("剩余 %d 小时（到期时间 %s）", int(left.Hours()), info.NotAfter.Local().Format("2006-01-02 15:04"))
	}
	return fmt.Sprintf("剩余 %d 天（到期时间 %s）", int(left.Hours()/24), info.NotAfter.Format("2006-01-02"))
}
