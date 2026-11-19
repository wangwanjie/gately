package app

// 命令行回滚（配置变更防护的第三层）：管理界面已无法访问时，在服务器上运行 gately rollback 撤销改坏的配置。
//
// 与 gately reset-password 相同，命令行把请求写到配置目录，运行中的实例按 resetPollInterval 轮询并处理
// （恢复后重启，避免模块把内存中的配置写回）；实例没有运行时由命令行直接改写配置文件。
// 启动时也会处理遗留的请求（命令行在等待期间被中断的情况）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// RollbackRequestFile 为命令行回滚请求文件。不以 .json 结尾：配置备份与声明式配置都不包含它
const RollbackRequestFile = "rollback-request.state"

// RollbackRequest 是一次命令行回滚：Pending 为回滚待确认的配置变更，否则把 Config 恢复为历史版本 Version。
type RollbackRequest struct {
	Pending   bool      `json:"pending,omitempty"`
	Config    string    `json:"config,omitempty"`
	Version   string    `json:"version,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func rollbackRequestPath(dir string) string {
	return filepath.Join(dir, RollbackRequestFile)
}

// WriteRollbackRequest 写入回滚请求，交给运行中的实例处理。
func WriteRollbackRequest(dir string, req RollbackRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return fsperm.WriteFile(rollbackRequestPath(dir), data, 0o600)
}

// RollbackRequestPending 判断回滚请求是否还没被处理。
func RollbackRequestPending(dir string) bool {
	_, err := os.Stat(rollbackRequestPath(dir))
	return err == nil
}

// TakeRollbackRequest 读取并删除回滚请求，没有时返回 nil。
func TakeRollbackRequest(dir string) (*RollbackRequest, error) {
	data, err := os.ReadFile(rollbackRequestPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.Remove(rollbackRequestPath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var req RollbackRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("回滚请求格式错误: %w", err)
	}
	return &req, nil
}

// OpenConfigStore 以与服务相同的方式打开配置目录（开启历史版本），供命令行使用：
// 命令行改写的配置同样先把当前内容保存为历史版本，回滚本身也能撤销。
func OpenConfigStore(dir string) (*config.Store, error) {
	store, err := config.Open(dir)
	if err != nil {
		return nil, err
	}
	store.EnableHistory(historyKeep, historySkip...)
	return store, nil
}

// ConfigChange 是一次配置保存：Version 为这次保存之前的内容，恢复它即撤销这次保存。
type ConfigChange struct {
	Config  string    `json:"config"`
	Version string    `json:"version"`
	Time    time.Time `json:"time"`
	Size    int64     `json:"size"`
}

// ListConfigChanges 列出最近的配置保存（全部配置合在一起按时间倒序），最多 limit 条。
func ListConfigChanges(store *config.Store, limit int) []ConfigChange {
	var out []ConfigChange
	for _, name := range store.HistoryNames() {
		vs, err := store.Versions(name)
		if err != nil {
			continue
		}
		for _, v := range vs {
			out = append(out, ConfigChange{Config: name, Version: v.ID, Time: v.Time, Size: v.Size})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// PendingSummary 是待确认配置变更的概况，供命令行显示。
type PendingSummary struct {
	Configs     []string
	Deadline    time.Time
	WaitRestart bool
	Rollback    bool
}

// ReadPendingConfirm 读取待确认的配置变更，没有时返回 nil。
func ReadPendingConfirm(dir string) (*PendingSummary, error) {
	p, err := loadPendingConfirm(dir)
	if err != nil || p == nil {
		return nil, err
	}
	return &PendingSummary{Configs: p.names(), Deadline: p.Deadline, WaitRestart: p.WaitRestart, Rollback: p.Rollback}, nil
}

// ApplyRollbackOffline 在实例没有运行时直接执行回滚，返回说明。
func ApplyRollbackOffline(store *config.Store, req RollbackRequest) (string, error) {
	dir := store.Dir()
	if req.Pending {
		p, err := loadPendingConfirm(dir)
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", errors.New("没有待确认的配置变更")
		}
		if errs := restoreConfirmFiles(store, p); len(errs) > 0 {
			return "", errors.New(errs[0])
		}
		if err := removePendingConfirm(dir); err != nil {
			return "", err
		}
		return fmt.Sprintf("已把待确认的配置变更（%s）回滚到修改前的内容", strings.Join(p.names(), "、")), nil
	}
	if err := store.RestoreVersion(req.Config, req.Version); err != nil {
		return "", err
	}
	// 回滚到的版本由管理员明确选择，不能在下次启动时又被待确认变更改回
	_ = removePendingConfirm(dir)
	return fmt.Sprintf("已把配置 %s 回滚到历史版本 %s", req.Config, req.Version), nil
}

// applyRollbackRequest 处理命令行提交的回滚请求：恢复配置并重启。
func (a *App) applyRollbackRequest() {
	req, err := TakeRollbackRequest(a.opts.ConfigDir)
	if err != nil {
		a.sysLogs.Add(logbuf.LevelError, "system", "读取命令行回滚请求失败: "+err.Error())
		return
	}
	if req == nil {
		return
	}
	var msg string
	if req.Pending {
		ok, err := a.rollbackPending()
		switch {
		case err != nil:
			a.sysLogs.Add(logbuf.LevelError, "system", "命令行回滚待确认的配置变更失败: "+err.Error())
			return
		case !ok:
			a.sysLogs.Add(logbuf.LevelWarn, "system", "命令行请求回滚待确认的配置变更，但没有待确认的变更")
			return
		}
		msg = "已通过命令行回滚待确认的配置变更"
	} else {
		a.discardPendingConfirm()
		if err := a.store.RestoreVersion(req.Config, req.Version); err != nil {
			a.sysLogs.Add(logbuf.LevelError, "system", fmt.Sprintf("命令行回滚配置 %s 失败: %v", req.Config, err))
			return
		}
		msg = fmt.Sprintf("已通过命令行把配置 %s 回滚到历史版本 %s，正在重启", req.Config, req.Version)
		a.sysLogs.Add(logbuf.LevelWarn, "system", msg)
		a.scheduleRestart()
	}
	a.audit.Record(audit.Entry{Actor: "cli", Category: audit.CategoryChange, Detail: msg})
}

// recoverRollbackRequest 在启动时、加载配置之前处理遗留的命令行回滚请求。
func recoverRollbackRequest(store *config.Store) []string {
	req, err := TakeRollbackRequest(store.Dir())
	if err != nil {
		return []string{"读取命令行回滚请求失败: " + err.Error()}
	}
	if req == nil {
		return nil
	}
	msg, err := ApplyRollbackOffline(store, *req)
	if err != nil {
		return []string{"处理命令行回滚请求失败: " + err.Error()}
	}
	return []string{msg}
}
