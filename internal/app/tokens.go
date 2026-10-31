package app

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// builtinDomains 是非模块的内置权限域及其显示名称。
var builtinDomains = []scopeDomain{
	{ID: apitoken.DomainStatus, Title: "系统状态", Builtin: true, ReadOnly: true},
	{ID: apitoken.DomainLogs, Title: "系统日志", Builtin: true, ReadOnly: true},
	{ID: apitoken.DomainMetrics, Title: "监控指标（Prometheus）", Builtin: true, ReadOnly: true},
	{ID: apitoken.DomainImport, Title: "Lucky 配置导入", Builtin: true, WriteOnly: true},
	{ID: apitoken.DomainFileUsers, Title: "文件服务共享用户", Builtin: true},
}

// scopeDomain 是一个可授权的权限域（模块或内置域）。
type scopeDomain struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Builtin 为 true 表示不是功能模块
	Builtin bool `json:"builtin"`
	// WriteOnly 表示该域没有只读接口，界面只提供写权限
	WriteOnly bool `json:"writeOnly,omitempty"`
	// ReadOnly 表示该域没有写接口，界面只提供读权限
	ReadOnly bool `json:"readOnly,omitempty"`
	// CanExec 表示该域的写权限可以在服务器上执行命令（命令、脚本或程序路径），界面据此提示风险
	CanExec bool `json:"canExec,omitempty"`
}

// execDomains 是写权限可以在服务器上执行命令的模块：DDNS 命令取 IP、定时任务、入侵防护命令日志源、
// STUN 规则脚本、证书映射脚本与本机部署命令、Web 终端、Docker 容器内执行与 Compose、
// 以及可指定程序路径的 frp、File Browser、Cloudflare Tunnel。
// 这些模块的写权限等同于 Gately 进程用户的命令执行权限，只应授予完全信任的脚本。
var execDomains = map[string]bool{
	"ddns": true, "cron": true, "guard": true, "stun": true, "ssl": true, "webterminal": true,
	"docker": true, "frp": true, "filebrowser": true, "cloudflared": true,
}

func (a *App) scopeDomains() []scopeDomain {
	out := make([]scopeDomain, 0, len(a.modules)+len(builtinDomains))
	for _, e := range a.modules {
		out = append(out, scopeDomain{ID: e.mod.ID(), Title: e.mod.Title(), CanExec: execDomains[e.mod.ID()]})
	}
	return append(out, builtinDomains...)
}

// initTokens 打开令牌存储，并把旧版全局令牌迁移为遗留令牌。
func (a *App) initTokens() error {
	var ids []string
	for _, d := range a.scopeDomains() {
		ids = append(ids, d.ID)
	}
	m, err := apitoken.Open(a.store, ids)
	if err != nil {
		return fmt.Errorf("加载 API 令牌失败: %w", err)
	}
	a.tokens = m
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settings.LegacyAPIToken == "" {
		return nil
	}
	if err := m.ImportLegacy(a.settings.LegacyAPIToken); err != nil {
		return fmt.Errorf("迁移旧版 API 令牌失败: %w", err)
	}
	next := a.settings
	next.LegacyAPIToken = ""
	// 命令行 -listen 覆盖时写回配置文件中的原地址
	if a.opts.Listen != "" {
		next.Listen = a.fileListen
	}
	if err := a.store.Save(settingsName, next); err != nil {
		return fmt.Errorf("清除旧版 API 令牌失败: %w", err)
	}
	a.settings.LegacyAPIToken = ""
	a.logger.Printf("已把旧版 API 令牌迁移为“遗留令牌”，可在 API 令牌页调整权限或删除")
	return nil
}

// bearerToken 取 Authorization: Bearer 后的内容。
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// tokenPeek 判断请求是否携带当前有效的 API 令牌（不检查权限、不记录使用）。
func (a *App) tokenPeek(r *http.Request) bool {
	return a.tokens.Peek(bearerToken(r))
}

// requireAuth 接受 API 令牌或登录会话。令牌按路径与方法检查权限，账号与系统级接口永远拒绝令牌。
func (a *App) requireAuth(next http.Handler) http.Handler {
	session := a.auth.Require(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := bearerToken(r); raw != "" {
			ip := httpx.ClientIP(r)
			if t, err := a.tokens.Authenticate(raw, ip); err == nil {
				need := apitoken.Required(r.Method, r.URL.Path)
				if !apitoken.Match(t.Scopes, need) {
					a.sysLogs.Add(logbuf.LevelWarn, "token", fmt.Sprintf("令牌“%s”（来自 %s）无权访问 %s %s，需要权限 %s", t.Name, ip, r.Method, r.URL.Path, need))
					if need == apitoken.Denied {
						httpx.Error(w, http.StatusForbidden, "该接口不允许使用 API 令牌访问，请使用登录会话")
					} else {
						httpx.Error(w, http.StatusForbidden, "令牌缺少权限 %s", need)
					}
					return
				}
				ctx := auth.WithUser(r.Context(), "api-token:"+t.Name)
				// 接口内部按权限决定是否返回敏感字段时（如容器环境变量），按令牌自身权限判定
				scopes := t.Scopes
				ctx = auth.WithScopeCheck(ctx, func(need string) bool {
					return apitoken.Match(scopes, need)
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// 不是有效的 API 令牌时，继续按登录会话令牌校验（Bearer 头同样可携带会话令牌）
		}
		session.ServeHTTP(w, r)
	})
}

type tokenSecretResponse struct {
	Token apitoken.View `json:"token"`
	// Secret 是令牌明文，仅此一次返回
	Secret string `json:"secret"`
}

func (a *App) handleTokenList(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.tokens.List())
}

func (a *App) handleTokenScopes(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.scopeDomains())
}

func (a *App) tokenError(w http.ResponseWriter, err error) {
	if errors.Is(err, apitoken.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "%v", err)
		return
	}
	httpx.Error(w, http.StatusBadRequest, "%v", err)
}

func (a *App) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	var in apitoken.Input
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	v, secret, err := a.tokens.Create(in)
	if err != nil {
		a.tokenError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelInfo, "token", fmt.Sprintf("已创建 API 令牌“%s”，权限: %s", v.Name, strings.Join(v.Scopes, ", ")))
	httpx.OK(w, tokenSecretResponse{Token: v, Secret: secret})
}

func (a *App) handleTokenUpdate(w http.ResponseWriter, r *http.Request) {
	var in apitoken.Input
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	v, err := a.tokens.Update(r.PathValue("id"), in)
	if err != nil {
		a.tokenError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelInfo, "token", fmt.Sprintf("已修改 API 令牌“%s”，权限: %s", v.Name, strings.Join(v.Scopes, ", ")))
	httpx.OK(w, v)
}

func (a *App) handleTokenRegenerate(w http.ResponseWriter, r *http.Request) {
	v, secret, err := a.tokens.Regenerate(r.PathValue("id"))
	if err != nil {
		a.tokenError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelWarn, "token", fmt.Sprintf("已重新生成 API 令牌“%s”，旧值立即失效", v.Name))
	httpx.OK(w, tokenSecretResponse{Token: v, Secret: secret})
}

func (a *App) handleTokenDelete(w http.ResponseWriter, r *http.Request) {
	name, err := a.tokens.Delete(r.PathValue("id"))
	if err != nil {
		a.tokenError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelWarn, "token", fmt.Sprintf("已删除 API 令牌“%s”", name))
	httpx.OK(w, map[string]bool{"ok": true})
}

// domainIDs 返回全部权限域 ID，供文档引用。
func (a *App) domainIDs() []string {
	var ids []string
	for _, d := range a.scopeDomains() {
		ids = append(ids, d.ID)
	}
	slices.Sort(ids)
	return ids
}
