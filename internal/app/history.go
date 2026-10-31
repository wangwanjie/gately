package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// 每个配置文件保留的历史版本数，以及不记录历史的配置（账号凭据、频繁写入的令牌使用记录）。
const historyKeep = 30

var historySkip = []string{accountName, "tokens", migrationsName}

type historyFile struct {
	Name string `json:"name"`
	// Title 为所属模块的名称，非模块配置为空
	Title    string           `json:"title,omitempty"`
	Versions []config.Version `json:"versions"`
}

// handleHistoryList 列出所有有历史版本的配置及其版本。
func (a *App) handleHistoryList(w http.ResponseWriter, r *http.Request) {
	titles := map[string]string{}
	for _, e := range a.modules {
		titles[e.mod.ID()] = e.mod.Title()
	}
	// 文件服务共享用户不属于某个模块，由 WebDAV、FTP、SMB 共用
	titles[fileusers.ID] = "文件服务共享用户"
	out := []historyFile{}
	for _, name := range a.store.HistoryNames() {
		vs, err := a.store.Versions(name)
		if err != nil || len(vs) == 0 {
			continue
		}
		out = append(out, historyFile{Name: name, Title: titles[name], Versions: vs})
	}
	httpx.OK(w, out)
}

func (a *App) historyError(w http.ResponseWriter, err error) {
	if errors.Is(err, config.ErrNoVersion) {
		httpx.Error(w, http.StatusNotFound, "%v", err)
		return
	}
	httpx.Error(w, http.StatusInternalServerError, "%v", err)
}

// handleHistoryDiff 返回历史版本与当前配置的行级差异（"-" 为历史版本中的内容，"+" 为当前内容）。
func (a *App) handleHistoryDiff(w http.ResponseWriter, r *http.Request) {
	name, id := r.PathValue("name"), r.PathValue("id")
	old, err := a.store.ReadVersion(name, id)
	if err != nil {
		a.historyError(w, err)
		return
	}
	cur, err := a.store.ReadRaw(name)
	if err != nil {
		a.historyError(w, err)
		return
	}
	httpx.OK(w, map[string]any{"lines": config.Diff(string(old), string(cur))})
}

// handleHistoryRestore 把配置回滚到历史版本，随后重启使模块重新加载。
func (a *App) handleHistoryRestore(w http.ResponseWriter, r *http.Request) {
	name, id := r.PathValue("name"), r.PathValue("id")
	if err := a.store.RestoreVersion(name, id); err != nil {
		a.historyError(w, err)
		return
	}
	a.sysLogs.Add(logbuf.LevelWarn, "system", fmt.Sprintf("配置 %s 已回滚到历史版本 %s，正在重启", name, id))
	httpx.OK(w, map[string]bool{"ok": true})
	a.scheduleRestart()
}
