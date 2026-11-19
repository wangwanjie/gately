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
	"github.com/wangwanjie/gately/internal/crashlog"
	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpcompress"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/loginips"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/proxyx"
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
	// Absent 为定制构建中没有编译进来的模块。它们的配置文件、设置中的开关与令牌权限原样保留，
	// 换回包含它们的构建后继续生效；接口统一返回“未包含在此构建中”
	Absent []module.Info
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
	// CrashLog 为 true 时把运行时崩溃信息保存到配置目录的 crash.log，并在启动时报告上次运行是否异常结束。
	// 只有服务进程开启：它会设置整个进程的崩溃输出，测试与导出文档等场景不需要
	CrashLog bool
	// Echo 为日志回显使用的标准错误输出；为 nil 时使用进程共用的 StderrEcho
	Echo *logbuf.Echo
	// Demo 开启在线演示模式：数据面服务全部不启动，只读访客一键进入，写操作与外部动作一律不可用，数据定时重置
	Demo bool
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
	// onboardMu 保护首次启动引导进度（onboarding.json）的读改写
	onboardMu sync.Mutex
	// loginIPs 记录登录管理后台成功过的来源，用于新地址登录告警
	loginIPs *loginips.Tracker
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
	// upd 为最近一次检查更新的结果，由 updMu 保护，updOnce 负责首次从文件读取；
	// updCheckMu 串行化 Gately 本身的检查与结果写回，updProgMu 串行化托管程序的检查，
	// updWake 在重新开启自动检查时唤醒检查协程
	upd        updateCache
	updMu      sync.Mutex
	updOnce    sync.Once
	updCheckMu sync.Mutex
	updProgMu  sync.Mutex
	updWake    chan struct{}
	restartCh  chan struct{}
	// holdCtx 在服务停止时取消，结束“不响应”方式挂起的未授权连接，不拖慢重启与退出
	holdCtx    context.Context
	holdCancel context.CancelFunc
	auth       *auth.Manager
	tokens     *apitoken.Manager
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
	// echo 把运行日志异步回显到标准错误输出，退出与重启前要 Flush
	echo *logbuf.Echo
	// pendingTOTP 是生成后尚未确认的两步验证密钥
	loginUsed usedStates
	// passkeyChallenges 保存已下发的 WebAuthn 登录挑战（未登录即可请求），passkeyRegChallenges 保存注册挑战（需要登录会话）
	passkeyChallenges    challengeStore
	passkeyRegChallenges challengeStore
	pendingMu            sync.Mutex
	pendingTOTP          string
	// crash 为本次运行的崩溃记录，未开启时为 nil（方法均可在 nil 上调用）
	crash *crashlog.Recorder
	// gwOrigins 为经统一网关访问时已确认的管理界面页面来源，见 gatewayOriginHost
	gwOrigins gatewayOrigins
	// confirm 为待确认的配置变更（超时自动回滚），见 confirm.go
	confirm confirmState
	// bootSettings 为本次启动时配置文件中的全局设置，用于判断需要重启才生效的设置是否改过
	bootSettings Settings
}

// New 加载配置并初始化所有模块，但不启动网络服务。
func New(opts Options) (_ *App, err error) {
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
	if opts.Demo {
		demo.Set(true)
		if err := prepareDemoDir(opts.ConfigDir); err != nil {
			return nil, fmt.Errorf("恢复演示数据失败: %w", err)
		}
	}
	store, err := config.Open(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	store.EnableHistory(historyKeep, historySkip...)
	// 上次运行留下的待确认配置变更：已超时或已决定回滚时，在加载任何配置之前恢复改动前的内容
	var confirmRec confirmRecovery
	if !opts.Demo {
		// 命令行回滚请求先处理：它可能正是要回滚待确认的变更
		cliMsgs := recoverRollbackRequest(store)
		confirmRec = recoverPendingConfirm(store, time.Now())
		confirmRec.cliMsgs = cliMsgs
	}
	if opts.ApplyDir != "" {
		// 声明式配置目录里同样是带密钥的配置文件，与配置目录一样不允许被存储挂载、静态站点或文件服务提供出去
		fsperm.Protect(opts.ApplyDir)
		n, err := applyConfigDir(store, opts.ApplyDir)
		if err != nil {
			return nil, fmt.Errorf("应用声明式配置失败: %w", err)
		}
		log.Printf("已从 %s 应用 %d 个配置文件", opts.ApplyDir, n)
	}
	// 全新安装（配置目录里还没有任何配置文件）首次登录后先询问使用场景；须在写入任何配置之前判断
	fresh := freshConfigDir(store.Dir())
	// 运行日志异步回显到标准错误输出；标准日志按服务端语言翻译后经同一个出口输出，两者的先后顺序保持一致
	echo := opts.Echo
	if echo == nil {
		echo = StderrEcho()
	}
	logger := log.New(i18n.NewWriter(echo), "", log.LstdFlags)
	holdCtx, holdCancel := context.WithCancel(context.Background())
	a := &App{
		holdCtx:    holdCtx,
		holdCancel: holdCancel,
		opts:       opts,
		store:      store,
		settings:   Settings{Listen: DefaultListen},
		sysLogs:    logbuf.New(defaultLogLines, nil),
		startTime:  time.Now(),
		hostStats:  sysstat.New(opts.ConfigDir),
		logger:     logger,
		echo:       echo,
		restartCh:  make(chan struct{}, 1),
		updWake:    make(chan struct{}, 1),
		audit:      audit.New(opts.ConfigDir),
	}
	a.confirm.recovered = confirmRec
	if _, err := store.Load(settingsName, &a.settings); err != nil {
		return nil, err
	}
	a.settings.normalize()
	a.bootSettings = a.settings
	a.sysLogs.SetEcho(echo)
	a.sysLogs.SetMinLevel(effectiveLogLevel(a.settings))
	// 先按设置开启日志落盘并恢复上次的日志，后续初始化过程中的日志也能写入文件
	a.sysLogs.Resize(a.settings.LogLines)
	a.persistLogs(a.sysLogs, settingsName, false, a.settings)
	if opts.CrashLog {
		a.startCrashLog()
		// 之后初始化失败时进程会退出：同样写进崩溃记录，下次启动不会误报为被强制结束
		defer func() {
			if err != nil {
				a.crash.Fatal("start failed: " + err.Error())
			}
		}()
	}
	if n := tightenConfigDir(opts.ConfigDir); n > 0 {
		a.sysLogs.Add(logbuf.LevelInfo, "", fmt.Sprintf("已收紧配置目录中 %d 个文件或目录的权限", n))
	}
	a.initLoginIPs()
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
	if err := a.markFreshInstall(fresh); err != nil {
		return nil, err
	}
	// 服务未运行期间通过命令行提交的重置请求，在启动时应用
	a.applyAccountReset()
	if err := a.setupDemo(); err != nil {
		return nil, fmt.Errorf("初始化演示账号失败: %w", err)
	}
	// 管理后台经本机 Web 服务站点反代访问时，站点据此识别管理员自己的请求并在访问日志中标记，日志页可隐藏它们。
	// 只看后台登录 Cookie：Authorization 头多是发给后端应用的，不必逐个校验；令牌带签名，访客无法伪造
	httpx.SetSessionCheck(func(r *http.Request) string {
		c, err := r.Cookie(auth.CookieName)
		if err != nil || c.Value == "" {
			return ""
		}
		id, ok := a.auth.Identify(c.Value)
		// 只认管理员会话：只读账号的 Cookie 一旦泄露，不能被用来给扫描流量打上“后台请求”标记、在默认视图中隐藏
		if !ok || id.Role != auth.RoleAdmin {
			return ""
		}
		return id.User
	})
	// 站点反代到其他后端时只剥离本实例签发的后台 Cookie，后端是另一台 Gately 时它自己的会话、安全入口标记与
	// 第三方登录随机数照常转发（见 webservice/admincookie.go）
	httpx.SetOwnAdminCookieCheck(a.ownAdminCookie)
	// 后台登录桥接：只有管理员会话（不含只读账号）才能签发，凭据绑定签发它的会话，登出或改密码后随之失效
	httpx.SetAdminBridge(func(r *http.Request) (string, time.Time) {
		c, err := r.Cookie(auth.CookieName)
		if err != nil || c.Value == "" {
			return "", time.Time{}
		}
		v, exp, ok := a.auth.IssueBridge(c.Value)
		if !ok {
			return "", time.Time{}
		}
		return v, exp
	}, func(value string) string {
		user, _ := a.auth.VerifyBridge(value)
		return user
	})

	// 文件服务共享用户由 WebDAV、FTP、SMB 共用，接口 /api/fileusers 属于应用本身，三个模块都关闭时也能读写
	if err := fileusers.Default.Init(store); err != nil {
		return nil, fmt.Errorf("加载文件服务共享用户失败: %w", err)
	}
	// 升级前保存的共享用户目录可能与配置目录重叠：这样的目录不再提供，提示用户修改
	for _, msg := range fileusers.Default.Blocked() {
		a.sysLogs.Add(logbuf.LevelWarn, "", msg)
	}
	for _, msg := range fileusers.Default.HiddenNotes() {
		a.sysLogs.Add(logbuf.LevelInfo, "", msg)
	}
	// 需要迁移配置的模块先迁移，再统一初始化：迁移可能改写其他模块的配置文件
	inherits, err := a.migrateModules(opts.Modules)
	if err != nil {
		return nil, err
	}
	// 通知、Webhook、订阅下载等可以选择复用更新代理，每次使用时读取最新设置
	proxyx.SetUpdateProxy(func() string { return a.settingsSnapshot().UpdateProxy })
	seen := map[string]bool{}
	for _, m := range opts.Modules {
		if seen[m.ID()] || m.ID() == settingsName || m.ID() == accountName {
			return nil, fmt.Errorf("模块 ID 冲突: %s", m.ID())
		}
		seen[m.ID()] = true
		entry := &moduleEntry{mod: m, logs: logbuf.New(a.settings.LogLines, nil)}
		entry.logs.SetEcho(echo)
		entry.logs.SetMinLevel(effectiveLogLevel(a.settings))
		_, access := m.(module.AccessLogger)
		a.persistLogs(entry.logs, m.ID(), access, a.settings)
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
	if err := a.snapshotDemo(); err != nil {
		return nil, fmt.Errorf("保存演示快照失败: %w", err)
	}
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
	mux.HandleFunc("GET /api/demo", a.handleDemoInfo)
	mux.HandleFunc("POST /api/login/demo", a.handleDemoLogin)
	mux.HandleFunc("POST /api/login/fnos", a.handleGatewayLogin)
	mux.HandleFunc("GET "+thirdauth.StartPath, a.handleThirdStart)
	mux.HandleFunc("GET "+thirdauth.CallbackPath, a.handleThirdCallback)

	rec.add("GET /api/session", a.handleSession, "")
	rec.add("GET /api/status", a.handleStatus, "")
	rec.add("PUT /api/account", a.handleAccount, "")
	rec.add("POST /api/account/sessions/revoke", a.handleRevokeSessions, "")
	rec.add("PUT /api/account/ui-language", a.handleUILanguage, "")
	rec.add("GET /api/system/security", a.handleSecurityStatus, "")
	rec.add("POST /api/account/password-check", a.handlePasswordCheck, "")
	rec.add("GET /api/account/password-login", a.handlePasswordLoginGet, "")
	rec.add("PUT /api/account/password-login", a.handlePasswordLoginSet, "")
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
	rec.add("GET /api/logs/sources", a.logSourcesHandler(a.sysLogs, nil), "")
	rec.add("GET /api/logs/export", a.logExportHandler(a.sysLogs, "system"), "")
	rec.add("POST /api/import/lucky", a.handleLuckyImport, "")
	rec.add("GET /api/system/settings", a.handleGetSettings, "")
	rec.add("PUT /api/system/settings", a.handlePutSettings, "")
	// 查看第三方登录 Client Secret 明文，按登录方式标识索引；/api/system/settings 下的接口只允许登录会话访问，只读账号不可调用
	rec.add("GET /api/system/settings/secrets", a.handleSettingsSecrets, "")
	rec.docs["GET /api/system/settings/secrets"] = module.Doc{Summary: "查看第三方登录 Client Secret 明文", Resp: map[string]string{}}
	rec.add("GET /api/system/info", a.handleSystemInfo, "")
	rec.add("GET /api/system/update", a.handleCheckUpdate, "")
	rec.add("GET /api/system/update/state", a.handleUpdateState, "")
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
	rec.add("GET /api/system/confirm", a.handleConfirmStatus, "")
	rec.add("GET /api/system/confirm/connect", a.handleConfirmSocket, "")
	rec.add("POST /api/system/confirm/rollback", a.handleConfirmRollback, "")
	rec.add("GET /api/system/access-path", a.handleAccessPath, "")
	rec.add("POST /api/system/access-path/check", a.handleAccessCheck, "")
	rec.add("POST /api/system/selfcheck", a.handleSelfCheck, "")
	rec.add("GET /api/system/onboarding", a.handleOnboarding, "")
	rec.add("POST /api/system/onboarding/dismiss", a.handleOnboardingDismiss, "")
	rec.add("POST /api/system/onboarding/scenes", a.handleOnboardingScenes, "")
	rec.add("POST /api/system/onboarding/done", a.handleOnboardingDone, "")
	rec.add("POST /api/system/probe", a.handleProbe, "")
	for _, e := range a.modules {
		prefix := "/api/" + e.mod.ID()
		if !e.enabled {
			// 已关闭的模块不注册任何接口，整个前缀统一返回"已关闭"；
			// 该模式不带请求方法，不会登记到路由表，接口文档中也就不再出现这些接口
			rec.add(prefix+"/", a.moduleDisabledHandler(e.mod), e.mod.ID())
			continue
		}
		rec.add("GET "+prefix+"/logs", a.logsHandler(e.logs), e.mod.ID())
		rec.add("GET "+prefix+"/logs/sources", a.logSourcesHandler(e.logs, e.mod), e.mod.ID())
		rec.add("GET "+prefix+"/logs/export", a.logExportHandler(e.logs, e.mod.ID()), e.mod.ID())
		rec.add("DELETE "+prefix+"/logs", func(w http.ResponseWriter, r *http.Request) {
			e.logs.Clear()
			httpx.OK(w, map[string]bool{"ok": true})
		}, e.mod.ID())
		e.mod.Routes(prefixRouter{rec: rec, prefix: prefix, module: e.mod.ID()})
		// 模块的免登录接口（例如被控设备上的客户端）直接挂在外层 mux 上，比 /api/ 更具体，不经过登录鉴权；
		// 仍经过 accessControl 的封禁、仅限内网与安全入口检查（凭据有效时越过安全入口，见 entryPassed）
		if pr, ok := e.mod.(module.PublicRouter); ok {
			pr.PublicRoutes(publicRouter{mux: mux, rec: rec, prefix: prefix + publicSegment, module: e.mod.ID()})
		}
	}
	for _, m := range a.opts.Absent {
		rec.add("/api/"+m.ID+"/", a.moduleAbsentHandler(m), m.ID)
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
	mux.Handle("/api/", a.requireAuth(a.demoGuard(a.roleGuard(a.defaultPasswordGuard(a.redactReadOnly(a.audit.Middleware(a.demoOverlay(a.confirmGuard(protected)))))))))
	mux.Handle("/", a.staticHandler())
	// 压缩放在翻译之外，压缩的是翻译后的最终内容；已预压缩的静态资源自带 Content-Encoding，不会重复压缩
	// 最外层限制请求体读取速度，登录等未认证接口同样受限
	return limitBodyRead(securityHeaders(httpcompress.Handler(i18n.Middleware(a.accessControl(httpx.SameOrigin(apiSecurityHeaders(mux)))))))
}

// Run 启动所有模块和管理服务，阻塞直到 ctx 取消。收到重启请求时停止全部服务后重新执行自身。
func (a *App) Run(parent context.Context) error {
	err := a.run(parent)
	if err != nil {
		// 进程随后退出：错误写进崩溃记录，下次启动时与崩溃一样整理进 crash.log
		a.crash.Fatal("run failed: " + err.Error())
		return err
	}
	a.crash.Close()
	return nil
}

func (a *App) run(parent context.Context) error {
	// 模块使用独立的可取消上下文：重启时先取消，确保后台协程退出后 Stop 不会一直等待
	ctx, cancelModules := context.WithCancel(parent)
	defer cancelModules()
	var disabled []string
	for _, e := range a.modules {
		if !e.enabled {
			disabled = append(disabled, e.mod.Title())
			continue
		}
		// 演示模式不启动任何模块：数据面服务不监听端口，后台任务也不运行
		if demo.Enabled() {
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
	// 待确认的配置变更在模块启动后开始计时：通知中心此时已订阅事件，可以发出回滚通知
	a.startConfirm()
	defer a.stopConfirmTimer()
	if demo.Enabled() {
		a.sysLogs.Add(logbuf.LevelInfo, "", "演示模式：全部数据面服务不启动，写操作与对外动作不可用")
		go a.runDemoReset(ctx)
	} else {
		go a.runUpdateCheck(ctx)
	}
	s := a.settingsSnapshot()
	handler := a.Handler()
	servers := []*http.Server{{Addr: s.Listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ConnContext: denyConnContext}}
	errCh := make(chan error, 2)
	httpLn, err := net.Listen("tcp", s.Listen)
	if err != nil {
		cancelModules()
		a.stopModules()
		a.closeLogs()
		return err
	}
	// 登记后台实际监听的地址（-listen 覆盖时为覆盖后的地址）：Web 服务只把后台登录 Cookie 转给反向代理到这些地址的站点后端
	adminAddrs := []string{httpLn.Addr().String()}
	defer httpx.SetAdminListen(nil)
	go func() {
		a.sysLogs.Add(logbuf.LevelInfo, "", "管理界面监听 "+s.Listen)
		errCh <- servers[0].Serve(&denyListener{Listener: httpLn, a: a})
	}()
	if s.HTTPSListen != "" && a.opts.Certs != nil {
		tlsSrv := &http.Server{
			Addr: s.HTTPSListen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ConnContext: denyConnContext,
			TLSConfig: &tls.Config{GetCertificate: a.opts.Certs.GetCertificate, MinVersion: tls.VersionTLS12},
		}
		servers = append(servers, tlsSrv)
		ln, err := net.Listen("tcp", s.HTTPSListen)
		if err != nil {
			// HTTPS 失败不影响 HTTP 管理界面
			a.sysLogs.Add(logbuf.LevelError, "", "HTTPS 管理界面监听失败: "+err.Error())
		} else {
			a.httpsReady.Store(true)
			adminAddrs = append(adminAddrs, ln.Addr().String())
			a.sysLogs.Add(logbuf.LevelInfo, "", "HTTPS 管理界面监听 "+s.HTTPSListen)
			go func() {
				if err := tlsSrv.ServeTLS(&denyListener{Listener: ln, a: a}, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					a.sysLogs.Add(logbuf.LevelError, "", "HTTPS 管理界面退出: "+err.Error())
				}
			}()
		}
	}
	httpx.SetAdminListen(adminAddrs)
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
			a.holdCancel()
			for _, srv := range servers[1:] {
				_ = srv.Close()
			}
			cancelModules()
			a.stopModules()
			a.closeLogs()
			return err
		}
	}
	// 先结束挂起的未授权连接，Shutdown 不必等它们超时
	a.holdCancel()
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
		// 重启前结束本次的崩溃记录，新进程启动时不会把这次重启当成异常结束
		a.crash.Close()
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

// maxActorRunes 是审计记录中未认证请求带来的用户名的最大长度。
const maxActorRunes = 64

// clipActor 截断来自未认证请求的用户名再写入审计记录：登录接口无需登录即可访问，
// 请求体最大数 MB，不截断会让每次失败的登录都向审计文件写入同样大小的内容。
func clipActor(s string) string {
	if len(s) <= maxActorRunes {
		return s
	}
	r := []rune(s)
	if len(r) <= maxActorRunes {
		return s
	}
	return string(r[:maxActorRunes]) + "…"
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	ip := httpx.ClientIP(r)
	src, ipTrusted := a.loginSource(r, ip)
	token, exp, err := a.auth.Login(src, req.Username, req.Password, req.Code)
	if errors.Is(err, auth.ErrTOTPRequired) {
		// 密码正确但缺少验证码，提示前端显示验证码输入框
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error(), "totpRequired": true})
		return
	}
	if err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "auth", fmt.Sprintf("来自 %s 的登录失败: %v", ip, err))
		a.audit.Record(audit.Entry{Actor: clipActor(req.Username), IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/login", Status: http.StatusUnauthorized, Detail: err.Error()})
		events.Publish(events.Event{Type: events.AuthFailed, Source: "管理后台", Level: events.LevelWarn,
			Title: "管理后台登录失败", Message: fmt.Sprintf("来自 %s 的登录失败：%v", ip, err), Fields: authFailedFields(ip, ipTrusted)})
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrLocked) || errors.Is(err, auth.ErrBusy) {
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
	a.noteLoginIP(ip, req.Username, "密码")
	if req.Username == a.auth.Username() {
		a.learnGatewayLoginOrigin(r)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(r),
	})
	httpx.OK(w, map[string]any{"username": req.Username, "expires": exp})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := auth.TokenFromRequest(r)
	if user, ok := a.auth.Verify(token); ok {
		// 服务端吊销该令牌，泄露的令牌在登出后立即失效
		if err := a.auth.Revoke(token); err != nil {
			a.logger.Printf("吊销会话令牌失败: %v", err)
		}
		a.audit.Record(audit.Entry{Actor: user, IP: httpx.ClientIP(r), Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/logout", Status: http.StatusOK, Detail: "退出登录"})
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	httpx.OK(w, map[string]bool{"ok": true})
}

// handleRevokeSessions 使所有已登录的管理界面会话失效（退出所有设备），包含当前会话；API 令牌不受影响。
func (a *App) handleRevokeSessions(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.RevokeAll(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	user := auth.UserFromContext(r.Context())
	a.sysLogs.Add(logbuf.LevelWarn, "auth", fmt.Sprintf("%s 从 %s 退出了所有设备，全部管理界面会话已失效", user, httpx.ClientIP(r)))
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	httpx.OK(w, map[string]bool{"ok": true})
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	role := auth.RoleFromContext(r.Context())
	if role == "" {
		role = auth.RoleAdmin
	}
	// disabledModules 为已关闭的功能模块，absentModules 为没有编译进来的模块，前端据此隐藏导航与页面
	// passwordIssue 为主账号密码的问题（default 默认密码、weak 强度不足），前端据此强制管理员修改密码；
	// defaultPassword 为旧版字段，与 passwordIssue == "default" 等价
	issue := ""
	if role == auth.RoleAdmin {
		issue = a.auth.PasswordIssue()
	}
	httpx.OK(w, map[string]any{"username": auth.UserFromContext(r.Context()), "role": role, "disabledModules": a.disabledModuleIDs(),
		"absentModules": a.absentModuleIDs(), "defaultPassword": issue == "default", "passwordIssue": issue})
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
			// 演示模式没有运行中的服务，摘要换成与演示数据一致的模拟值
			if demo.Enabled() {
				if items := a.demoSummary(e.mod.ID()); items != nil {
					info.Summary = nil
					for _, it := range items {
						info.Summary = append(info.Summary, module.SummaryItem{Label: it.Label, Value: it.Value, Level: it.Level})
					}
				}
			}
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
		"build":      a.buildProfile(),
		"timeZone":   zoneLabel(),
		"configDir":  a.opts.ConfigDir,
		"host":       hostForRequest(r, a.hostStats.Snapshot()),
	})
}

// hostForRequest 按请求者的权限过滤托管进程：概览接口只要求 status:read，而托管进程带有各模块的实例名称，
// 只列出请求者有读权限的模块（API 令牌按自身权限，只读账号与管理员可读全部模块）。
func hostForRequest(r *http.Request, s sysstat.Snapshot) sysstat.Snapshot {
	out := make([]sysstat.ManagedProcess, 0, len(s.Managed))
	for _, m := range s.Managed {
		if auth.Allowed(r.Context(), m.Module+":"+apitoken.LevelRead) {
			out = append(out, m)
		}
	}
	s.Managed = out
	return s
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
	viaToken := auth.IsToken(r.Context())
	if viaToken {
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
		if rep := a.importLuckyIdentities(pe.doc, &ids, opts, viaToken); rep != nil {
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
	// 端口转发与 STUN 规则的安全模式引用 IP 过滤中的名单，同样先汇总
	opts.IPFilter = luckyimport.IPFilterLists(docs, time.Now())
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
				if rep := a.importLuckyBase(doc, opts, viaToken); rep != nil {
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
		lang := i18n.RequestLang(r)
		if f != nil && lang != i18n.SourceLang {
			f.Translate = func(s string) string { return i18n.Tr(lang, s) }
			f.TranslateMessage = func(e *logbuf.Entry) string { return translateLogMessage(lang, e) }
		}
		entries := buf.QueryContext(r.Context(), since, before, limit, f)
		// 这里直接按请求语言翻译好再返回：内存里的日志带着格式串，一次查表就能得到译文，
		// 交给翻译中间件则只能拿到渲染后的文字、逐句整句匹配。只读身份的响应还要先脱敏再翻译，仍由中间件处理
		if lang != i18n.SourceLang && !needsRedaction(r) {
			translateLogEntries(lang, entries)
			i18n.SkipTranslation(r)
		}
		httpx.OK(w, entries)
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

// iconCacheControl 是根目录图标文件（favicon、应用图标）的缓存头。
const iconCacheControl = "private, max-age=86400"

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
				// 凭签名读取 manifest 时，图标地址同样要带上签名，否则安装时取图标仍被安全入口拦截
				if name == "manifest.webmanifest" && a.serveSignedManifest(w, r, web) {
					return
				}
				// 带哈希的构建产物可长期缓存
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else if homeIconFiles[name] && name != "manifest.webmanifest" {
					// 根目录的图标文件名不带哈希，不能长期缓存，但给一天缓存即可避免每次打开页面都重新取；
					// 用 private 防止共享缓存把开启安全入口后才返回的图标发给未通过入口的访客
					w.Header().Set("Cache-Control", iconCacheControl)
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
		} else if safePath := a.settingsSnapshot().SafePath; safePath != "" {
			// 开启安全入口时给图标地址签名，iOS 添加到主屏幕时不带 Cookie 也能取到图标；经网关访问时不经过安全入口，无需签名
			index = signIconLinks(index, prefix, a.iconKey(safePath))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", uiCSP)
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

// uiCSP 是管理界面页面（index.html）的内容安全策略，作为出现注入漏洞时的兜底：脚本只能来自管理界面自己的文件，
// 页面里即使被插入了 <script>、事件属性或外部脚本地址也不会执行；页面不能被其他站点嵌入，表单只能提交回自身。
//   - style-src 允许内联样式：Element Plus、终端等组件在运行时写入样式；
//   - img-src 允许 data:、blob: 与任意 http(s) 图片：二维码是 data: 地址，导航面板的图标与背景可以填任意图片地址；
//   - connect-src 允许 ws:/wss:：终端与容器 Shell 走 WebSocket，部分浏览器不认为 'self' 包含它们；
//   - 页面内嵌的预览（错误页、导航面板、未授权访问页面）是 srcdoc 内联框架，沿用这份策略，并各自带有更严格的策略。
const uiCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https: http:; " +
	"font-src 'self' data:; media-src 'self' data: blob:; connect-src 'self' ws: wss:; worker-src 'self' blob:; frame-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// apiCSP 是接口响应的内容安全策略：接口只返回数据与下载内容，即使某个响应被浏览器当成页面打开，其中的脚本与外部资源也不会加载。
const apiCSP = "default-src 'none'; frame-ancestors 'none'"

// apiSecurityHeaders 给 /api/ 下的响应加上 apiCSP。放在访问控制之后：被访问控制拦下的请求返回的是
// “未授权访问响应”（可能是用户自定义的页面，有自己的策略），不经过这里。
func apiSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Security-Policy", apiCSP)
		}
		next.ServeHTTP(w, r)
	})
}

// cookieSecure 判断登录 Cookie 是否加 Secure 属性（只经 HTTPS 发送）：请求本身是 HTTPS，或者经 HTTPS 的反向代理而来。
// X-Forwarded-Proto 只在请求来自可信代理或飞牛统一网关时采信，访客自己带上这个请求头没有作用。
func cookieSecure(r *http.Request) bool {
	if requestSecure(r) {
		return true
	}
	_, viaGateway := gatewayPrefix(r)
	return viaGateway && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
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

// publicSegment 是模块免登录接口在模块前缀下的路径片段。
const publicSegment = "/public"

// publicRouter 把模块的免登录接口挂到 /api/<ID>/public 前缀下，不经过受保护接口的鉴权；
// 同时在路由表中标记为免登录接口，仅用于接口文档。
type publicRouter struct {
	mux    *http.ServeMux
	rec    *routeRecorder
	prefix string
	module string
}

func (p publicRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		p.mux.HandleFunc(p.prefix+pattern, h)
		return
	}
	p.mux.HandleFunc(method+" "+p.prefix+path, h)
	if p.rec != nil {
		p.rec.addPublic(method+" "+p.prefix+path, p.module)
	}
}

// publicAuth 判断请求是否为某个已开启模块的免登录接口，且带有该模块认可的有效凭据。
func (a *App) publicAuth(r *http.Request) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/")
	if !ok {
		return false
	}
	id, tail, _ := strings.Cut(rest, "/")
	if !strings.HasPrefix("/"+tail, publicSegment+"/") {
		return false
	}
	for _, e := range a.modules {
		if e.mod.ID() != id || !e.enabled {
			continue
		}
		if pr, ok := e.mod.(module.PublicRouter); ok {
			return pr.PublicAuth(r)
		}
	}
	return false
}

// Doc 实现 module.DocRouter，登记模块接口的文档信息。
func (p prefixRouter) Doc(pattern string, d module.Doc) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		return
	}
	p.rec.docs[method+" "+p.prefix+path] = d
}
