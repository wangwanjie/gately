package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// 演示账号：只读，一键进入时由服务端使用，密码不需要告知访客。
const (
	demoUsername = "demo"
	demoPassword = "Visitor-Readonly-Tour-7391"
)

// setupDemo 在演示模式下确保存在名为 demo 的只读账号，并关闭演示中不可用的模块与 HTTPS 管理端口。
func (a *App) setupDemo() error {
	if !demo.Enabled() {
		return nil
	}
	exists := false
	for _, u := range a.auth.Users() {
		if u.Username == demoUsername {
			exists = true
			break
		}
	}
	if !exists {
		if err := a.auth.AddUser(demoUsername, demoPassword); err != nil {
			return err
		}
	}
	// 演示实例不展示首次启动引导
	if err := a.auth.DismissOnboarding(); err != nil {
		return err
	}
	a.settingsMu.Lock()
	for _, id := range demo.DisabledModules() {
		if !slices.Contains(a.settings.DisabledModules, id) {
			a.settings.DisabledModules = append(a.settings.DisabledModules, id)
		}
	}
	// 演示实例只有一个管理端口：不开 HTTPS 管理端口与统一网关
	a.settings.HTTPSListen = ""
	a.settingsMu.Unlock()
	a.opts.GatewaySocket = ""
	loadDemoBans(a.opts.ConfigDir)
	return nil
}

// loadDemoBans 把演示数据里的封禁表装入内存：入侵防护模块没有启动，不会自己读取它。
func loadDemoBans(dir string) {
	data, err := os.ReadFile(filepath.Join(dir, "guard", "bans.json"))
	if err != nil {
		return
	}
	var f struct {
		Bans     []banlist.Entry            `json:"bans"`
		Offenses map[string]banlist.Offense `json:"offenses"`
	}
	if json.Unmarshal(data, &f) == nil {
		banlist.Default.Import(f.Bans, f.Offenses)
	}
}

// prepareDemoDir 在加载配置前执行：已有快照时把配置目录恢复为快照（每次启动都是干净的演示数据）。
func prepareDemoDir(configDir string) error {
	snap := demo.SnapshotDir(configDir)
	if demo.HasSnapshot(snap) {
		return demo.Restore(configDir, snap)
	}
	return nil
}

// snapshotDemo 在初始化完成后保存快照（只在没有快照时），之后每次启动与定时重置都恢复到这个状态。
func (a *App) snapshotDemo() error {
	if !demo.Enabled() {
		return nil
	}
	snap := demo.SnapshotDir(a.opts.ConfigDir)
	if demo.HasSnapshot(snap) {
		return nil
	}
	return demo.Snapshot(a.opts.ConfigDir, snap)
}

// runDemoReset 每隔一个周期请求重启：新进程启动时把配置目录恢复为快照。
func (a *App) runDemoReset(ctx context.Context) {
	t := time.NewTimer(demo.Interval())
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
		a.sysLogs.Add(logbuf.LevelInfo, "", "演示数据到达重置时间，正在重启并恢复")
		select {
		case a.restartCh <- struct{}{}:
		default:
		}
	}
}

// demoGuard 在演示模式下拒绝 demo.Blocked 判定的请求，对管理员会话同样生效。
func (a *App) demoGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if demo.Blocked(r.Method, r.URL.Path) {
			httpx.Error(w, http.StatusForbidden, "%s", demo.Message)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleDemoInfo 返回演示模式信息，登录页与页面顶部横幅据此显示；未登录即可访问。
func (a *App) handleDemoInfo(w http.ResponseWriter, r *http.Request) {
	if !demo.Enabled() {
		httpx.OK(w, map[string]any{"enabled": false})
		return
	}
	httpx.OK(w, map[string]any{
		"enabled":   true,
		"nextReset": a.startTime.Add(demo.Interval()).Unix(),
		"interval":  int64(demo.Interval() / time.Second),
	})
}

// handleDemoLogin 以只读演示账号登录，不需要密码；非演示模式下不存在这个入口。
func (a *App) handleDemoLogin(w http.ResponseWriter, r *http.Request) {
	if !demo.Enabled() {
		http.NotFound(w, r)
		return
	}
	ip := httpx.ClientIP(r)
	src, _ := a.loginSource(r, ip)
	token, exp, err := a.auth.Login(src, demoUsername, demoPassword)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrLocked) || errors.Is(err, auth.ErrBusy) {
			status = http.StatusTooManyRequests
		}
		httpx.Error(w, status, "%v", err)
		return
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
	httpx.OK(w, map[string]any{"username": demoUsername, "expires": exp})
}
