// Package app 负责组装配置、认证、模块与管理 HTTP 服务。
package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/httpcompress"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/sysstat"
	"github.com/wangwanjie/gately/internal/thirdauth"
	"github.com/wangwanjie/gately/internal/version"
)

// 基础配置与账号配置的文件名
const (
	settingsName = "gately"
	accountName  = "account"
)

// DefaultListen 是管理界面默认监听地址，避开 Lucky 的 16601 以便迁移期间并存。
const DefaultListen = ":16680"

// Options 是启动参数。
type Options struct {
	ConfigDir string
	// Listen 非空时覆盖配置文件中的监听地址
	Listen string
	// Web 是嵌入的前端文件系统，可为 nil
	Web fs.FS
	// Modules 是全部功能模块；其中被设置关闭的模块只加载配置，不启动服务，也不提供管理接口
	Modules []module.Module
	// Requires 为模块间的依赖关系（模块 ID → 它依赖的模块 ID），依赖方开启时被依赖的模块不能关闭；可为 nil
	Requires map[string][]string
	// Certs 为管理界面 HTTPS 提供证书，可为 nil
	Certs CertProvider
	// ApplyDir 非空时，每次启动把该目录下的 <名称>.json 覆盖到配置目录（声明式配置，适合 GitOps、Docker 与 Kubernetes）
	ApplyDir string
	// InitialPassword 非空时作为首次启动的管理员初始密码，不使用默认密码也不写入日志
	InitialPassword string
	// GatewaySocket 非空时额外在该 Unix Socket 上提供管理界面，供飞牛 fnOS 统一网关等反向代理接入
	GatewaySocket string
	// GatewayPrefix 是经网关访问时的路径前缀，例如 /app/gately；为空表示不加前缀
	GatewayPrefix string
}

type moduleEntry struct {
	mod  module.Module
	logs *logbuf.Buffer
	// enabled 为本次启动时模块是否开启，进程运行期间不变；修改开关需重启生效
	enabled bool
}

// App 是一个运行中的 Gately 实例。
type App struct {
	opts       Options
	store      *config.Store
	settingsMu sync.RWMutex
	settings   Settings
	// fileListen 为配置文件中的监听地址（命令行 -listen 覆盖时用于回写）
	fileListen string
	// restartPending 表示有设置需要重启后生效；httpsReady 表示 HTTPS 管理服务已启动
	restartPending atomic.Bool
	httpsReady     atomic.Bool
	// upgrading 防止同时执行多个在线升级；upgradeSt 为在线升级任务的进度，由 upgradeMu 保护
	upgrading atomic.Bool
	upgradeMu sync.Mutex
	upgradeSt upgradeStatus
	// upgradeCancel 取消进行中的在线升级，同样由 upgradeMu 保护
	upgradeCancel context.CancelFunc
	restartCh     chan struct{}
	auth          *auth.Manager
	tokens        *apitoken.Manager
	// audit 记录管理后台的操作审计日志
	audit *audit.Log
	// routes 为最近一次构建处理器时登记的全部接口，用于生成 API 文档
	routes    []routeInfo
	routeDocs map[string]module.Doc
	sysLogs   *logbuf.Buffer
	modules   []*moduleEntry
	startTime time.Time
	// hostStats 采集概览页展示的主机信息，保存上一次采样用于计算使用率与速率
	hostStats *sysstat.Collector
	logger    *log.Logger
	// pendingTOTP 是生成后尚未确认的两步验证密钥
	loginUsed usedStates
	// passkeyChallenges 保存已下发的 WebAuthn 挑战
	passkeyChallenges challengeStore
	pendingMu         sync.Mutex
	pendingTOTP       string
}

// New 加载配置并初始化所有模块，但不启动网络服务。
func New(opts Options) (*App, error) {
	prefix, err := normalizeGatewayPrefix(opts.GatewayPrefix)
	if err != nil {
		return nil, err
	}
	opts.GatewayPrefix = prefix
	// 配置目录转为绝对路径：默认值是相对当前目录的 data，托管程序（如 File Browser）在自己的
	// 工作目录里启动，相对的程序、配置与数据库路径会按子进程工作目录解析而找不到
	if opts.ConfigDir != "" {
		abs, err := filepath.Abs(opts.ConfigDir)
		if err != nil {
			return nil, err
		}
		opts.ConfigDir = abs
	}
	store, err := config.Open(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	store.EnableHistory(historyKeep, historySkip...)
	if opts.ApplyDir != "" {
		n, err := applyConfigDir(store, opts.ApplyDir)
		if err != nil {
			return nil, fmt.Errorf("应用声明式配置失败: %w", err)
		}
		log.Printf("已从 %s 应用 %d 个配置文件", opts.ApplyDir, n)
	}
	// 标准日志按服务端语言翻译后输出
	logger := log.New(i18n.NewWriter(os.Stderr), "", log.LstdFlags)
	a := &App{
		opts:      opts,
		store:     store,
		settings:  Settings{Listen: DefaultListen},
		sysLogs:   logbuf.New(defaultLogLines, logger),
		startTime: time.Now(),
		hostStats: sysstat.New(opts.ConfigDir),
		logger:    logger,
		restartCh: make(chan struct{}, 1),
		audit:     audit.New(opts.ConfigDir),
	}
	if _, err := store.Load(settingsName, &a.settings); err != nil {
		return nil, err
	}
	a.settings.normalize()
	// 先按设置开启日志落盘并恢复上次的日志，后续初始化过程中的日志也能写入文件
	a.sysLogs.Resize(a.settings.LogLines)
	a.persistLogs(a.sysLogs, settingsName, a.settings)
	if n := tightenConfigDir(opts.ConfigDir); n > 0 {
		a.sysLogs.Add(logbuf.LevelInfo, "", fmt.Sprintf("已收紧配置目录中 %d 个文件或目录的权限", n))
	}
	a.fileListen = a.settings.Listen
	if opts.Listen != "" {
		a.settings.Listen = opts.Listen
	}

	var acc auth.Account
	if _, err := store.Load(accountName, &acc); err != nil {
		return nil, err
	}
	a.auth = auth.NewManager(acc, func(acc auth.Account) error {
		return store.Save(accountName, acc)
	})
	if err := a.auth.SetBootstrapPassword(opts.InitialPassword); err != nil {
		return nil, err
	}
	initial, err := a.auth.EnsureInitialized()
	if err != nil {
		return nil, fmt.Errorf("初始化管理员账号失败: %w", err)
	}
	if initial != "" {
		// 默认密码只在首次启动时打印一次，登录后应立即修改
		// 日志同时包含英文关键词 "initial password"，便于各语言用户统一用 grep -i "initial password" 检索
		a.logger.Printf("==== 已创建管理员账号 admin / Admin account \"admin\" created, initial password: %s ，请登录后立即修改 / change it after your first login ====", initial)
	}
	// 服务未运行期间通过命令行提交的重置请求，在启动时应用
	a.applyAccountReset()

	// 文件服务共享用户由 WebDAV、FTP、SMB 共用，接口 /api/fileusers 属于应用本身，三个模块都关闭时也能读写
	if err := fileusers.Default.Init(store); err != nil {
		return nil, fmt.Errorf("加载文件服务共享用户失败: %w", err)
	}
	// 需要迁移配置的模块先迁移，再统一初始化：迁移可能改写其他模块的配置文件
	inherits, err := a.migrateModules(opts.Modules)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, m := range opts.Modules {
		if seen[m.ID()] || m.ID() == settingsName || m.ID() == accountName {
			return nil, fmt.Errorf("模块 ID 冲突: %s", m.ID())
		}
		seen[m.ID()] = true
		entry := &moduleEntry{mod: m, logs: logbuf.New(a.settings.LogLines, logger)}
		a.persistLogs(entry.logs, m.ID(), a.settings)
		if hc, ok := m.(module.HTTPClientAware); ok {
			hc.SetHTTPClient(a.updateClient)
		}
		if err := m.Init(module.Env{Store: store, Logs: entry.logs}); err != nil {
			return nil, fmt.Errorf("模块 %s 初始化失败: %w", m.ID(), err)
		}
		a.modules = append(a.modules, entry)
	}
	a.resolveEnabledModules()
	// 已关闭的文件服务不参与共享用户的保存检查
	fileusers.Default.SetActive(a.moduleEnabled)
	if err := a.initTokens(); err != nil {
		return nil, err
	}
	if err := a.inheritTokenScopes(inherits); err != nil {
		return nil, err
	}
	a.audit.SetHook(publishAudit)
	a.applyRuntime()
	return a, nil
}

// Handler 返回完整的 HTTP 处理器（API + 前端）。
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	protected := http.NewServeMux()
	rec := &routeRecorder{mux: protected, docs: map[string]module.Doc{}}

	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("POST /api/login/passkey/begin", a.handlePasskeyLoginBegin)
	mux.HandleFunc("POST /api/login/passkey/finish", a.handlePasskeyLoginFinish)
	mux.HandleFunc("GET /api/login/providers", a.handleThirdProviders)
	mux.HandleFunc("POST /api/login/fnos", a.handleGatewayLogin)
	mux.HandleFunc("GET "+thirdauth.StartPath, a.handleThirdStart)
	mux.HandleFunc("GET "+thirdauth.CallbackPath, a.handleThirdCallback)

	rec.add("GET /api/session", a.handleSession, "")
	rec.add("GET /api/status", a.handleStatus, "")
	rec.add("PUT /api/account", a.handleAccount, "")
	rec.add("PUT /api/account/ui-language", a.handleUILanguage, "")
	rec.add("GET /api/account/passkeys", a.handlePasskeyList, "")
	rec.add("POST /api/account/passkeys/begin", a.handlePasskeyBegin, "")
	rec.add("POST /api/account/passkeys", a.handlePasskeyRegister, "")
	rec.add("DELETE /api/account/passkeys/{id}", a.handlePasskeyDelete, "")
	rec.add("GET /api/account/users", a.handleUserList, "")
	rec.add("POST /api/account/users", a.handleUserCreate, "")
	rec.add("PUT /api/account/users/{name}", a.handleUserPassword, "")
	rec.add("DELETE /api/account/users/{name}", a.handleUserDelete, "")
	rec.add("GET /api/account/totp", a.handleTOTPStatus, "")
	rec.add("POST /api/account/totp/setup", a.handleTOTPSetup, "")
	rec.add("POST /api/account/totp/enable", a.handleTOTPEnable, "")
	rec.add("POST /api/account/totp/disable", a.handleTOTPDisable, "")
	rec.add("GET /api/logs", a.logsHandler(a.sysLogs), "")
	rec.add("POST /api/import/lucky", a.handleLuckyImport, "")
	rec.add("GET /api/system/settings", a.handleGetSettings, "")
	rec.add("PUT /api/system/settings", a.handlePutSettings, "")
	// 查看第三方登录 Client Secret 明文，按登录方式标识索引；/api/system/settings 下的接口只允许登录会话访问，只读账号不可调用
	rec.add("GET /api/system/settings/secrets", a.handleSettingsSecrets, "")
	rec.docs["GET /api/system/settings/secrets"] = module.Doc{Summary: "查看第三方登录 Client Secret 明文", Resp: map[string]string{}}
	rec.add("GET /api/system/info", a.handleSystemInfo, "")
	rec.add("GET /api/system/update", a.handleCheckUpdate, "")
	rec.add("POST /api/system/upgrade", a.handleUpgrade, "")
	rec.add("GET /api/system/upgrade/status", a.handleUpgradeStatus, "")
	rec.add("POST /api/system/upgrade/cancel", a.handleUpgradeCancel, "")
	rec.add("POST /api/system/upgrade/upload", a.handleUpgradeUpload, "")
	rec.add("GET /api/system/backup", a.handleBackup, "")
	rec.add("GET /api/system/diagnostics", a.handleDiagnostics, "")
	rec.add("GET /api/audit", a.handleAudit, "")
	rec.add("GET /api/metrics", a.handleMetrics, "")
	rec.add("GET /api/history", a.handleHistoryList, "")
	rec.add("GET /api/history/{name}/{id}/diff", a.handleHistoryDiff, "")
	rec.add("POST /api/history/{name}/{id}/restore", a.handleHistoryRestore, "")
	rec.add("POST /api/system/restore", a.handleRestore, "")
	rec.add("POST /api/system/restart", a.handleRestart, "")
	rec.add("POST /api/system/selfcheck", a.handleSelfCheck, "")
	rec.add("GET /api/system/onboarding", a.handleOnboarding, "")
	rec.add("POST /api/system/onboarding/dismiss", a.handleOnboardingDismiss, "")
	for _, e := range a.modules {
		prefix := "/api/" + e.mod.ID()
		if !e.enabled {
			// 已关闭的模块不注册任何接口，整个前缀统一返回"已关闭"；
			// 该模式不带请求方法，不会登记到路由表，接口文档中也就不再出现这些接口
			rec.add(prefix+"/", a.moduleDisabledHandler(e.mod), e.mod.ID())
			continue
		}
		rec.add("GET "+prefix+"/logs", a.logsHandler(e.logs), e.mod.ID())
		rec.add("DELETE "+prefix+"/logs", func(w http.ResponseWriter, r *http.Request) {
			e.logs.Clear()
			httpx.OK(w, map[string]bool{"ok": true})
		}, e.mod.ID())
		e.mod.Routes(prefixRouter{rec: rec, prefix: prefix, module: e.mod.ID()})
	}
	rec.add("GET /api/fileusers", fileusers.Default.HandleGet(a.fileUsersExtra), "")
	rec.add("PUT /api/fileusers", fileusers.Default.HandlePut(a.fileUsersExtra), "")
	// 查看共享用户密码明文，路径含 secrets 片段，按 fileusers 写权限鉴权
	rec.add("GET /api/fileusers/secrets", fileusers.Default.HandleSecrets(), "")
	rec.docs["GET /api/fileusers/secrets"] = module.Doc{Summary: "查看共享用户密码明文", Resp: map[string]string{}}
	rec.add("GET /api/tokens", a.handleTokenList, "")
	rec.add("GET /api/tokens/scopes", a.handleTokenScopes, "")
	rec.add("POST /api/tokens", a.handleTokenCreate, "")
	rec.add("PUT /api/tokens/order", a.handleTokenOrder, "")
	rec.add("PUT /api/tokens/group", a.handleTokenGroup, "")
	rec.add("PUT /api/tokens/{id}", a.handleTokenUpdate, "")
	rec.add("POST /api/tokens/{id}/regenerate", a.handleTokenRegenerate, "")
	rec.add("DELETE /api/tokens/{id}", a.handleTokenDelete, "")
	rec.add("GET /api/openapi.json", a.handleOpenAPI, "")
	a.routes = rec.routes
	a.routeDocs = rec.docs
	mux.Handle("/api/", a.requireAuth(a.roleGuard(a.defaultPasswordGuard(a.redactReadOnly(a.audit.Middleware(protected))))))
	mux.Handle("/", a.staticHandler())
	// 压缩放在翻译之外，压缩的是翻译后的最终内容；已预压缩的静态资源自带 Content-Encoding，不会重复压缩
	return securityHeaders(httpcompress.Handler(i18n.Middleware(a.accessControl(httpx.SameOrigin(mux)))))
}

// Run 启动所有模块和管理服务，阻塞直到 ctx 取消。收到重启请求时停止全部服务后重新执行自身。
func (a *App) Run(parent context.Context) error {
	// 模块使用独立的可取消上下文：重启时先取消，确保后台协程退出后 Stop 不会一直等待
	ctx, cancelModules := context.WithCancel(parent)
	defer cancelModules()
	var disabled []string
	for _, e := range a.modules {
		if !e.enabled {
			disabled = append(disabled, e.mod.Title())
			continue
		}
		if err := e.mod.Start(ctx); err != nil {
			// 单个模块失败不影响其他模块，错误写入日志供界面查看
			e.logs.Add(logbuf.LevelError, "", "模块启动失败: "+err.Error())
		}
	}
	if len(disabled) > 0 {
		a.sysLogs.Add(logbuf.LevelInfo, "", "以下功能模块已关闭，未启动："+strings.Join(disabled, "、"))
	}
	go a.watchAccountReset(ctx)
	s := a.settingsSnapshot()
	handler := a.Handler()
	servers := []*http.Server{{Addr: s.Listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}}
	errCh := make(chan error, 2)
	go func() {
		a.sysLogs.Add(logbuf.LevelInfo, "", "管理界面监听 "+s.Listen)
		errCh <- servers[0].ListenAndServe()
	}()
	if s.HTTPSListen != "" && a.opts.Certs != nil {
		tlsSrv := &http.Server{
			Addr: s.HTTPSListen, Handler: handler, ReadHeaderTimeout: 10 * time.Second,
			TLSConfig: &tls.Config{GetCertificate: a.opts.Certs.GetCertificate, MinVersion: tls.VersionTLS12},
		}
		servers = append(servers, tlsSrv)
		ln, err := net.Listen("tcp", s.HTTPSListen)
		if err != nil {
			// HTTPS 失败不影响 HTTP 管理界面
			a.sysLogs.Add(logbuf.LevelError, "", "HTTPS 管理界面监听失败: "+err.Error())
		} else {
			a.httpsReady.Store(true)
			a.sysLogs.Add(logbuf.LevelInfo, "", "HTTPS 管理界面监听 "+s.HTTPSListen)
			go func() {
				if err := tlsSrv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					a.sysLogs.Add(logbuf.LevelError, "", "HTTPS 管理界面退出: "+err.Error())
				}
			}()
		}
	}
	if a.opts.GatewaySocket != "" {
		// 统一网关监听失败只记录日志，不影响 TCP 管理界面
		gwSrv := &http.Server{Handler: a.gatewayHandler(a.opts.GatewayPrefix, handler), ReadHeaderTimeout: 10 * time.Second}
		ln, err := listenUnix(a.opts.GatewaySocket)
		if err != nil {
			a.sysLogs.Add(logbuf.LevelError, "", "统一网关 Socket 监听失败: "+err.Error())
		} else {
			servers = append(servers, gwSrv)
			a.sysLogs.Add(logbuf.LevelInfo, "", fmt.Sprintf("统一网关监听 %s，路径前缀 %q", a.opts.GatewaySocket, a.opts.GatewayPrefix))
			go func() {
				if err := gwSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					a.sysLogs.Add(logbuf.LevelError, "", "统一网关监听退出: "+err.Error())
				}
			}()
		}
	}

	restart := false
	select {
	case <-ctx.Done():
	case <-a.restartCh:
		restart = true
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			for _, srv := range servers[1:] {
				_ = srv.Close()
			}
			cancelModules()
			a.stopModules()
			a.closeLogs()
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}
	cancelModules()
	a.stopModules()
	// 把节流中的令牌最近使用信息写入磁盘
	a.tokens.Flush()
	// 模块停止过程中的日志也要写入文件，再重启或退出
	a.closeLogs()
	if restart {
		a.logger.Printf("正在重启…")
		restartProcess()
	}
	return nil
}

func (a *App) stopModules() {
	// 逆序停止，和启动顺序对称
	for i := len(a.modules) - 1; i >= 0; i-- {
		// 未启动的模块无需停止
		if !a.modules[i].enabled {
			continue
		}
		if err := a.modules[i].mod.Stop(); err != nil {
			a.logger.Printf("停止模块 %s 失败: %v", a.modules[i].mod.ID(), err)
		}
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Code 为两步验证码，未启用时忽略
	Code string `json:"code"`
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	ip := httpx.ClientIP(r)
	token, exp, err := a.auth.Login(ip, req.Username, req.Password, req.Code)
	if errors.Is(err, auth.ErrTOTPRequired) {
		// 密码正确但缺少验证码，提示前端显示验证码输入框
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error(), "totpRequired": true})
		return
	}
	if err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "auth", fmt.Sprintf("来自 %s 的登录失败: %v", ip, err))
		a.audit.Record(audit.Entry{Actor: req.Username, IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/login", Status: http.StatusUnauthorized, Detail: err.Error()})
		events.Publish(events.Event{Type: events.AuthFailed, Source: "管理后台", Level: events.LevelWarn,
			Title: "管理后台登录失败", Message: fmt.Sprintf("来自 %s 的登录失败：%v", ip, err), Fields: map[string]string{"ip": ip}})
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrLocked) {
			status = http.StatusTooManyRequests
		}
		httpx.Error(w, status, "%v", err)
		return
	}
	// 先记录界面语言，随后的登录日志与通知即按该语言输出；只读账号的界面语言不影响服务端输出
	if req.Username == a.auth.Username() {
		a.rememberUILanguage(r)
	}
	a.sysLogs.Add(logbuf.LevelInfo, "auth", fmt.Sprintf("%s 从 %s 登录", req.Username, ip))
	a.audit.Record(audit.Entry{Actor: req.Username, IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/login", Status: http.StatusOK, Detail: "密码登录"})
	events.Publish(events.Event{Type: events.LoginSuccessful, Source: "管理后台", Level: events.LevelInfo,
		Title: "管理后台登录成功", Message: fmt.Sprintf("%s 从 %s 登录", req.Username, ip), Fields: map[string]string{"ip": ip, "user": req.Username}})
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})
	httpx.OK(w, map[string]any{"username": req.Username, "expires": exp})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if user, ok := a.auth.Verify(auth.TokenFromRequest(r)); ok {
		a.audit.Record(audit.Entry{Actor: user, IP: httpx.ClientIP(r), Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/logout", Status: http.StatusOK, Detail: "退出登录"})
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	httpx.OK(w, map[string]bool{"ok": true})
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	role := auth.RoleFromContext(r.Context())
	if role == "" {
		role = auth.RoleAdmin
	}
	// disabledModules 为已关闭的功能模块，前端据此隐藏导航与页面
	// defaultPassword 为主账号仍在使用默认密码，前端据此提醒管理员修改
	httpx.OK(w, map[string]any{"username": auth.UserFromContext(r.Context()), "role": role, "disabledModules": a.disabledModuleIDs(),
		"defaultPassword": role == auth.RoleAdmin && a.auth.UsingDefaultPassword()})
}

type moduleInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Enabled 为模块是否开启；关闭的模块不提供摘要
	Enabled bool                 `json:"enabled"`
	Summary []module.SummaryItem `json:"summary,omitempty"`
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	mods := make([]moduleInfo, 0, len(a.modules))
	for _, e := range a.modules {
		info := moduleInfo{ID: e.mod.ID(), Title: e.mod.Title(), Enabled: e.enabled}
		if sm, ok := e.mod.(module.Summarizer); ok && e.enabled {
			info.Summary = sm.Summary()
		}
		mods = append(mods, info)
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	httpx.OK(w, map[string]any{
		"version":    version.Version,
		"commit":     version.Commit,
		"buildDate":  version.BuildDate,
		"goVersion":  runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"startTime":  a.startTime,
		"goroutines": runtime.NumGoroutine(),
		"memAlloc":   mem.Alloc,
		"modules":    mods,
		"timeZone":   zoneLabel(),
		"configDir":  a.opts.ConfigDir,
		"host":       a.hostStats.Snapshot(),
	})
}

// zoneLabel 返回时区名称；未显式设置时区（显示为 Local）时改用缩写与 UTC 偏移，例如 CST UTC+08:00。
func zoneLabel() string {
	name, offset := time.Now().Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	utc := fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, offset%3600/60)
	if loc := time.Local.String(); loc != "Local" {
		return loc + " (" + utc + ")"
	}
	return name + " " + utc
}

type accountRequest struct {
	OldPassword string `json:"oldPassword"`
	Username    string `json:"username"`
	NewPassword string `json:"newPassword"`
}

func (a *App) handleAccount(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := a.auth.ChangeCredentials(req.OldPassword, req.Username, req.NewPassword); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.sysLogs.Add(logbuf.LevelInfo, "auth", "管理员账号已修改，所有会话已失效")
	httpx.OK(w, map[string]bool{"ok": true})
}

func (a *App) handleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, map[string]bool{"enabled": a.auth.TOTPEnabled()})
}

// handleTOTPSetup 生成待确认的密钥，确认验证码后才真正启用。
func (a *App) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	secret := auth.NewTOTPSecret()
	a.pendingMu.Lock()
	a.pendingTOTP = secret
	a.pendingMu.Unlock()
	httpx.OK(w, map[string]string{"secret": secret, "url": auth.TOTPURL(secret, a.auth.Username())})
}

type totpRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (a *App) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	var req totpRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.pendingMu.Lock()
	secret := a.pendingTOTP
	a.pendingMu.Unlock()
	if secret == "" {
		httpx.Error(w, http.StatusBadRequest, "请先生成密钥")
		return
	}
	if !auth.ValidateTOTP(secret, req.Code, time.Now()) {
		httpx.Error(w, http.StatusBadRequest, "验证码错误，请确认验证器时间同步")
		return
	}
	if err := a.auth.SetTOTP(req.Password, "", secret); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.pendingMu.Lock()
	a.pendingTOTP = ""
	a.pendingMu.Unlock()
	a.sysLogs.Add(logbuf.LevelInfo, "auth", "已启用两步验证")
	httpx.OK(w, map[string]bool{"ok": true})
}

func (a *App) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	var req totpRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := a.auth.SetTOTP(req.Password, req.Code, ""); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.sysLogs.Add(logbuf.LevelWarn, "auth", "已关闭两步验证")
	httpx.OK(w, map[string]bool{"ok": true})
}

// handleLuckyImport 接收 multipart 上传的 .lkcf 文件或 Lucky 导出的整站配置 zip（字段名 files），
// 分发给实现了 luckyimport.Importer 的模块。?dryRun=1 时只预览不写入，?enable=1 时保留 Lucky 中的启用状态。
func (a *App) handleLuckyImport(w http.ResponseWriter, r *http.Request) {
	// 限制总请求体与文件数量，防止超大上传占用内存与磁盘
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "读取上传文件失败: %v", err)
		return
	}
	opts := luckyimport.Options{DryRun: r.URL.Query().Get("dryRun") == "1", KeepEnabled: r.URL.Query().Get("enable") == "1"}
	// API 令牌只有 import:write 权限，不能借 enable=1 让导入的 DDNS 命令来源、定时任务等直接生效执行；
	// 需要启用时由管理员会话导入，或导入后在对应模块里手动启用
	if strings.HasPrefix(auth.UserFromContext(r.Context()), "api-token:") {
		opts.KeepEnabled = false
	}
	type fileResult struct {
		File    string                `json:"file"`
		Error   string                `json:"error,omitempty"`
		Reports []*luckyimport.Report `json:"reports"`
	}
	results := []fileResult{}
	if len(r.MultipartForm.File["files"]) > 64 {
		httpx.Error(w, http.StatusBadRequest, "一次最多导入 64 个文件")
		return
	}
	var entries []luckyimport.Entry
	for _, fh := range r.MultipartForm.File["files"] {
		data, err := readUpload(fh)
		if err == nil && luckyimport.IsArchive(data) {
			var sub []luckyimport.Entry
			if sub, err = luckyimport.ExpandArchive(fh.Filename, data); err == nil {
				entries = append(entries, sub...)
				continue
			}
		}
		if err != nil {
			results = append(results, fileResult{File: fh.Filename, Error: err.Error(), Reports: []*luckyimport.Report{}})
			continue
		}
		entries = append(entries, luckyimport.Entry{Name: fh.Filename, Data: data})
	}
	// 先解析全部文件：第三方登录身份表需要排在最前，站点与基础设置才能按 Key 解析出允许的身份
	type parsedEntry struct {
		name string
		doc  luckyimport.Document
		err  error
	}
	var parsed []parsedEntry
	for _, en := range entries {
		doc, err := luckyimport.ParseAny(en.Data)
		parsed = append(parsed, parsedEntry{name: en.Name, doc: doc, err: err})
	}
	ids, ierr := thirdauth.LoadLuckyIdentities(a.store)
	if ierr != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "import", "读取已导入的第三方身份表失败: "+ierr.Error())
	}
	identityReports := map[int]*luckyimport.Report{}
	for i, pe := range parsed {
		if pe.err != nil {
			continue
		}
		if rep := a.importLuckyIdentities(pe.doc, &ids, opts); rep != nil {
			identityReports[i] = rep
		}
	}
	if len(ids.List) > 0 {
		opts.ResolveThirdParty = ids.Resolve
	}
	// FTP / WebDAV 用户可能引用存储管理中的存储，先汇总本次上传的存储列表，再回退到已导入的存储
	var docs []luckyimport.Document
	for _, pe := range parsed {
		if pe.err == nil {
			docs = append(docs, pe.doc)
		}
	}
	opts.ResolveStore = a.luckyStoreResolver(docs)
	for i, pe := range parsed {
		res := fileResult{File: pe.name, Reports: []*luckyimport.Report{}}
		doc, err := pe.doc, pe.err
		if rep, ok := identityReports[i]; ok {
			res.Reports = append(res.Reports, rep)
		}
		if err == nil {
			for _, e := range a.modules {
				imp, ok := e.mod.(luckyimport.Importer)
				if !ok {
					continue
				}
				if !e.enabled {
					// 导入会直接作用到运行中的规则，已关闭的模块只预览，告知用户开启后再导入
					if rep := a.skipDisabledImport(e.mod, imp, doc, opts); rep != nil {
						res.Reports = append(res.Reports, rep)
					}
					continue
				}
				rep, ierr := imp.ImportLucky(doc, opts)
				if ierr != nil {
					err = fmt.Errorf("%s: %w", e.mod.Title(), ierr)
					break
				}
				if rep != nil {
					res.Reports = append(res.Reports, rep)
					if !opts.DryRun {
						e.logs.Add(logbuf.LevelInfo, "import", fmt.Sprintf("从 Lucky 导入 %d/%d 条", rep.Imported, rep.Found))
					}
				}
			}
			if err == nil {
				if rep := a.importLuckyBase(doc, opts); rep != nil {
					res.Reports = append(res.Reports, rep)
				}
			}
		}
		if err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	httpx.OK(w, results)
}

// luckyStoreResolver 汇总 docs 中的 Lucky 存储管理列表，生成存储 Key 到本地目录的解析函数；
// 本次未上传的存储回退到存储模块中已导入的同 Key 挂载点。
func (a *App) luckyStoreResolver(docs []luckyimport.Document) func(string) (string, string) {
	items := map[string]luckyimport.StorageItem{}
	for _, doc := range docs {
		list, ok, err := luckyimport.StorageList(doc)
		if !ok || err != nil {
			continue
		}
		for _, it := range list {
			items[it.Key] = it
		}
	}
	type pathLookup interface {
		MountPath(key string) (string, bool)
	}
	var fallback func(string) (string, bool)
	for _, e := range a.modules {
		if pl, ok := e.mod.(pathLookup); ok {
			fallback = pl.MountPath
			break
		}
	}
	return luckyimport.StoreResolver(items, fallback)
}

func readUpload(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 32<<20))
}

// logsHandler 支持 ?since=<seq> 增量拉取；?before=<seq> 拉取更早的日志；?limit=<n> 只返回其中最新的 n 条。
// 过滤参数见 parseLogFilter，增量拉取与加载更早的日志都只返回满足条件的日志。
func (a *App) logsHandler(buf *logbuf.Buffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		since, ok1 := parseSeq(q.Get("since"))
		before, ok2 := parseSeq(q.Get("before"))
		limit, ok3 := 0, true
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			limit, ok3 = n, err == nil && n >= 0
		}
		// 错误信息写成完整字面量，便于按语言翻译
		switch {
		case !ok1:
			httpx.Error(w, http.StatusBadRequest, "since 参数无效")
			return
		case !ok2:
			httpx.Error(w, http.StatusBadRequest, "before 参数无效")
			return
		case !ok3:
			httpx.Error(w, http.StatusBadRequest, "limit 参数无效")
			return
		}
		f, msg := parseLogFilter(q)
		if msg != "" {
			httpx.Error(w, http.StatusBadRequest, "%s", msg)
			return
		}
		// 返回的日志会按请求语言翻译，关键词也要能匹配译文
		if lang := i18n.RequestLang(r); f != nil && lang != i18n.SourceLang {
			f.Translate = func(s string) string { return i18n.Tr(lang, s) }
		}
		httpx.OK(w, buf.QueryContext(r.Context(), since, before, limit, f))
	}
}

// parseSeq 解析日志序号参数，空串视为 0。
func parseSeq(s string) (uint64, bool) {
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseUint(s, 10, 64)
	return v, err == nil
}

// staticHandler 提供前端单页应用，未知路径回退到 index.html。
func (a *App) staticHandler() http.Handler {
	if a.opts.Web == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "前端未构建：请在 web 目录执行 pnpm install && pnpm build 后重新编译", http.StatusNotFound)
		})
	}
	// 构建产物的文本资源只嵌入 .gz，包装后原文件名仍可 Stat/Open
	web := gzipOnlyFS{fsys: a.opts.Web}
	files := http.FileServer(http.FS(web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" {
			if _, err := fs.Stat(web, name); err == nil {
				// 带哈希的构建产物可长期缓存
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				if a.servePrecompressed(w, r, name) {
					return
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(a.opts.Web, "index.html")
		if err != nil {
			http.Error(w, "前端未构建：请在 web 目录执行 pnpm install && pnpm build 后重新编译", http.StatusNotFound)
			return
		}
		// 前端资源以相对路径构建，这里改写成带前缀的绝对路径，任意路径回退到首页时资源都能加载；
		// 经统一网关访问时同时告知前端路径前缀
		prefix, _ := gatewayPrefix(r)
		index = absolutizeAssets(index, prefix)
		if prefix != "" {
			index = injectBase(index, prefix)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// prefixRouter 把模块的相对路由挂到 /api/<ID> 前缀下，并登记接口文档。
type prefixRouter struct {
	rec    *routeRecorder
	prefix string
	module string
}

func (p prefixRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		p.rec.add(p.prefix+pattern, h, p.module)
		return
	}
	p.rec.add(method+" "+p.prefix+path, h, p.module)
}

// Doc 实现 module.DocRouter，登记模块接口的文档信息。
func (p prefixRouter) Doc(pattern string, d module.Doc) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		return
	}
	p.rec.docs[method+" "+p.prefix+path] = d
}
