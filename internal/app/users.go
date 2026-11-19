package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// roleGuard 限制只读账号：只允许需要读权限的接口，写操作与账号、令牌、备份等高危接口一律拒绝。
// 管理员与 API 令牌（自带权限范围，上下文中没有角色）不受影响。
func (a *App) roleGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.RoleFromContext(r.Context()) == auth.RoleReadonly {
			need := apitoken.Required(r.Method, r.URL.Path)
			if need != apitoken.Any && (need == apitoken.Denied || !strings.HasSuffix(need, ":"+apitoken.LevelRead)) {
				httpx.Error(w, http.StatusForbidden, "只读账号不能执行此操作")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type userRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *App) handleUserList(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.auth.Users())
}

func (a *App) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := a.auth.AddUser(req.Username, req.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.sysLogs.Add(logbuf.LevelInfo, "auth", fmt.Sprintf("已创建只读账号 %s", req.Username))
	httpx.OK(w, a.auth.Users())
}

func (a *App) handleUserPassword(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	name := r.PathValue("name")
	if err := a.auth.SetUserPassword(name, req.Password); err != nil {
		a.userError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelInfo, "auth", fmt.Sprintf("已重置账号 %s 的密码", name))
	httpx.OK(w, map[string]bool{"ok": true})
}

func (a *App) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := a.auth.DeleteUser(name); err != nil {
		a.userError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelWarn, "auth", fmt.Sprintf("已删除账号 %s", name))
	httpx.OK(w, a.auth.Users())
}

func (a *App) userError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrUserNotFound) {
		httpx.Error(w, http.StatusNotFound, "%v", err)
		return
	}
	httpx.Error(w, http.StatusBadRequest, "%v", err)
}

// resetPollInterval 是运行中的实例检查命令行重置请求的间隔。
const resetPollInterval = 2 * time.Second

// watchAccountReset 定期检查命令行（gately reset-password / reset-totp）提交的重置请求，
// 运行中的实例无需重启即可生效。
func (a *App) watchAccountReset(ctx context.Context) {
	t := time.NewTicker(resetPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if auth.ResetPending(a.opts.ConfigDir) {
				a.applyAccountReset()
			}
			// 命令行 gately rollback 提交的回滚请求
			if RollbackRequestPending(a.opts.ConfigDir) {
				a.applyRollbackRequest()
			}
		}
	}
}

// applyAccountReset 应用配置目录中的重置请求，并记入系统日志与审计日志。
func (a *App) applyAccountReset() {
	req, err := auth.TakeResetRequest(a.opts.ConfigDir)
	if err != nil {
		a.sysLogs.Add(logbuf.LevelError, "auth", "读取账号重置请求失败: "+err.Error())
		return
	}
	if req == nil {
		return
	}
	if err := a.auth.ApplyReset(*req); err != nil {
		a.sysLogs.Add(logbuf.LevelError, "auth", "应用账号重置请求失败: "+err.Error())
		return
	}
	var msg string
	switch {
	case req.PasswordHash != "" && req.DisableTOTP:
		msg = "已通过命令行重置管理员密码并关闭两步验证，所有已登录会话已失效"
	case req.PasswordHash != "":
		msg = "已通过命令行重置管理员密码，所有已登录会话已失效"
	default:
		msg = "已通过命令行关闭两步验证，所有已登录会话已失效"
	}
	a.sysLogs.Add(logbuf.LevelWarn, "auth", msg)
	a.audit.Record(audit.Entry{Actor: "cli", Category: audit.CategoryChange, Detail: msg})
}
