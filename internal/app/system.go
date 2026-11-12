package app

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/backup"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/resolver"
	"github.com/wangwanjie/gately/internal/version"
)

// CertProvider 由证书模块实现，用于管理界面的 HTTPS。
type CertProvider interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	HasCertificate(name string) bool
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// ---- 备份与恢复 ----

// handleBackup 把配置目录中的 JSON 配置打包为 zip 下载。备份包含密钥与证书私钥，请妥善保管。
func (a *App) handleBackup(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	data, err := backup.Build(a.opts.ConfigDir, "", now)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+backup.Name(now, false)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
	a.sysLogs.Add(logbuf.LevelInfo, "system", "已导出配置备份")
}

// handleRestore 用备份 zip 覆盖配置文件，随后重启生效。原配置先备份到 config/backup-时间 目录。
// 加密备份（定时备份可设置口令）需同时提交表单字段 password。
func (a *App) handleRestore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "读取上传文件失败: %v", err)
		return
	}
	fhs := r.MultipartForm.File["file"]
	if len(fhs) != 1 {
		httpx.Error(w, http.StatusBadRequest, "请上传一个备份文件")
		return
	}
	data, err := readUpload(fhs[0])
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	restored, err := backup.Parse(data, r.FormValue("password"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, backup.ErrPasswordRequired) {
			status = http.StatusUnprocessableEntity
		}
		httpx.Error(w, status, "%v", err)
		return
	}
	// keepAccount=1：保留当前管理员账号，不用备份中的 account.json 覆盖（账号、密码、两步验证与只读账号均不变），
	// 适合把别处的备份迁到已改过密码的新实例。此时备份无需包含 account.json。
	keepAccount := r.FormValue("keepAccount") == "1"
	if keepAccount {
		delete(restored, accountName+".json")
		if len(restored) == 0 {
			httpx.Error(w, http.StatusBadRequest, "备份中没有可恢复的配置文件，不像是 Gately 的配置备份")
			return
		}
	} else if _, ok := restored[accountName+".json"]; !ok {
		httpx.Error(w, http.StatusBadRequest, "备份中缺少 %s.json，不像是 Gately 的配置备份", accountName)
		return
	}
	if b, ok := restored[accountName+".json"]; ok {
		// 备份里的会话密钥、会话纪元与吊销列表停留在备份时：原样写回会让此后“退出所有设备”或登出吊销的令牌重新生效。
		// 换一个新的会话密钥，恢复前签发的令牌（含泄露的）一律失效，纪元与吊销列表随之清空
		nb, err := rotateSessionSecret(b)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%s.json 格式错误: %v", accountName, err)
			return
		}
		restored[accountName+".json"] = nb
	}
	dir := filepath.Join(a.opts.ConfigDir, "backup-"+time.Now().Format("20060102-150405"))
	if err := fsperm.MkdirAll(dir, 0o700); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	old, _ := filepath.Glob(filepath.Join(a.opts.ConfigDir, "*.json"))
	for _, f := range old {
		if b, err := os.ReadFile(f); err == nil {
			_ = fsperm.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
		}
	}
	for name, b := range restored {
		if err := a.store.WriteRaw(strings.TrimSuffix(name, ".json"), b); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "写入 %s 失败: %v", name, err)
			return
		}
	}
	a.sysLogs.Add(logbuf.LevelWarn, "system", fmt.Sprintf("已从备份恢复 %d 个配置文件，原配置保存在 %s", len(restored), dir))
	httpx.OK(w, map[string]any{"restored": len(restored), "previous": dir})
	a.scheduleRestart()
}

// rotateSessionSecret 把账号文件中的会话密钥换成新值，并去掉会话纪元与吊销列表；其余字段原样保留。
func rotateSessionSecret(b []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	secret, _ := json.Marshal(auth.NewSessionSecret())
	m["sessionSecret"] = secret
	delete(m, "sessionEpoch")
	delete(m, "revoked")
	return json.MarshalIndent(m, "", "  ")
}

// ---- 重启 ----

func (a *App) handleRestart(w http.ResponseWriter, r *http.Request) {
	a.sysLogs.Add(logbuf.LevelWarn, "system", "收到重启请求")
	httpx.OK(w, map[string]bool{"ok": true})
	a.scheduleRestart()
}

// scheduleRestart 在响应写出后重启：先优雅停止模块，再以相同参数重新执行自身。
func (a *App) scheduleRestart() {
	go func() {
		time.Sleep(500 * time.Millisecond)
		select {
		case a.restartCh <- struct{}{}:
		default:
		}
	}()
}

// restartProcess 执行重启，测试中可替换。
var restartProcess = execSelf

// execSelf 用当前可执行文件替换进程；失败时退出，由 Docker / systemd 的重启策略拉起。
func execSelf() {
	exe, err := os.Executable()
	if err == nil && runtime.GOOS != "windows" {
		err = syscall.Exec(exe, os.Args, os.Environ())
	}
	fmt.Fprintf(os.Stderr, "重启失败，退出进程: %v\n", err)
	os.Exit(3)
}

// ---- 系统信息与更新检查 ----

func (a *App) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	host, _ := os.Hostname()
	httpx.OK(w, map[string]any{
		"version": version.Version, "commit": version.Commit, "buildDate": version.BuildDate,
		"goVersion": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"hostname": host, "pid": os.Getpid(), "startTime": a.startTime, "configDir": a.opts.ConfigDir,
		"goroutines": runtime.NumGoroutine(), "memSys": ms.Sys, "memAlloc": ms.Alloc,
		"numCPU": runtime.NumCPU(), "maxProcs": runtime.GOMAXPROCS(0), "timeZone": time.Local.String(),
		"inDocker": fileExists("/.dockerenv"), "selfUpgrade": a.upgradeSupport(), "build": a.buildProfile(),
	})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- Lucky 基础设置导入 ----

// luckyBase 是 Lucky 基础设置中可迁移的字段。
type luckyBase struct {
	AllowInternetaccess         *bool
	TokenExpirationHour         int
	MaxConsecutiveLoginFailures int
	TimeZone                    string
	CustomDNSList               string
	CustomDNSA                  string
	CustomDNSB                  string
	LogMaxSize                  int
	GCPercent                   int
	SetGCPercent                bool
	SafeURL                     string
	AdminWebListenPort          int
	TwoFAEnable                 bool
	EnableOpenToken             bool
	// 第三方登录管理后台：身份以 Key 引用 lucky_thirdPartyAuthManager 中的身份表
	EnableThirdAuthLogin    bool
	AllowAllThirdAuthUsers  bool
	ThirdAuthLoginUserList  any
	ThirdAuthLoginSkipTwoFA bool
}

// importLuckyBase 迁移 Lucky 的基础设置。管理端口、安全入口、管理员账号与两步验证不迁移，避免导入后无法登录。
func (a *App) importLuckyBase(doc luckyimport.Document, opts luckyimport.Options) *luckyimport.Report {
	if _, ok := doc["AdminWebListenPort"]; !ok {
		return nil
	}
	raw, _ := json.Marshal(doc)
	var lb luckyBase
	if err := json.Unmarshal(raw, &lb); err != nil {
		return &luckyimport.Report{Module: "settings", Title: "全局设置", Found: 1, Warnings: []string{"解析失败: " + err.Error()}}
	}
	rep := &luckyimport.Report{Module: "settings", Title: "全局设置", Found: 1, Imported: 1}
	s := a.settingsSnapshot()
	if lb.AllowInternetaccess != nil {
		s.LANOnly = !*lb.AllowInternetaccess
	}
	if lb.MaxConsecutiveLoginFailures > 0 && lb.MaxConsecutiveLoginFailures <= 100 {
		s.MaxLoginFailures = lb.MaxConsecutiveLoginFailures
	}
	if lb.TimeZone != "" {
		if _, err := time.LoadLocation(lb.TimeZone); err == nil {
			s.TimeZone = lb.TimeZone
		}
	}
	var dns []string
	for _, v := range append(strings.FieldsFunc(lb.CustomDNSList, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }), lb.CustomDNSA, lb.CustomDNSB) {
		if v = strings.TrimSpace(v); v != "" {
			dns = append(dns, v)
		}
	}
	var (
		badDNS []string
		good   []string
	)
	seen := map[string]bool{}
	for _, d := range dns {
		// Lucky 的 default 表示使用系统 DNS，Gately 未配置自定义服务器时即走系统 DNS，无需导入
		if strings.EqualFold(d, "default") {
			continue
		}
		// Lucky 3.0 新增的 doh3@ 是走 HTTP/3 的 DoH，Gately 统一按 doh@ 导入
		if len(d) > 5 && strings.EqualFold(d[:5], "doh3@") {
			d = "doh@" + d[5:]
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		if _, err := resolver.Parse(d); err != nil {
			badDNS = append(badDNS, d)
			continue
		}
		good = append(good, d)
	}
	if len(good) > 0 {
		s.DNSServers = good
	}
	if len(badDNS) > 0 {
		rep.Warnings = append(rep.Warnings, "以下 DNS 服务器格式不支持，已忽略："+strings.Join(badDNS, "、"))
	}
	if lb.SetGCPercent && lb.GCPercent > 0 {
		s.GCPercent = lb.GCPercent
	}
	if err := s.Validate(); err != nil {
		rep.Imported = 0
		rep.Warnings = append(rep.Warnings, "设置无效，未导入: "+err.Error())
		return rep
	}
	a.mergeLuckyThirdLogin(&s, lb, opts, rep)
	var notes []string
	if lb.AdminWebListenPort > 0 {
		notes = append(notes, fmt.Sprintf("管理端口（原配置为 %d）", lb.AdminWebListenPort))
	}
	if lb.SafeURL != "" {
		notes = append(notes, "安全入口")
	}
	notes = append(notes, "管理员账号")
	if lb.TwoFAEnable {
		notes = append(notes, "两步验证")
	}
	rep.Warnings = append(rep.Warnings, "以下设置需在 Gately 中手动配置："+strings.Join(notes, "、"))
	if lb.EnableOpenToken {
		rep.Warnings = append(rep.Warnings, "Lucky 的 OpenToken 未迁移，如需脚本调用请在设置中生成 API 令牌")
	}
	if lb.TokenExpirationHour > 0 && !opts.DryRun {
		_ = a.auth.SetTokenTTL(lb.TokenExpirationHour)
	}
	if opts.DryRun {
		return rep
	}
	a.settingsMu.Lock()
	if a.opts.Listen != "" {
		s.Listen = a.fileListen
	}
	err := a.store.Save(settingsName, s)
	if a.opts.Listen != "" {
		s.Listen = a.settings.Listen
	}
	if err == nil {
		a.settings = s
	}
	a.settingsMu.Unlock()
	if err != nil {
		rep.Imported = 0
		rep.Warnings = append(rep.Warnings, "保存失败: "+err.Error())
		return rep
	}
	a.applyRuntime()
	return rep
}
