package app

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

// certModuleID 是证书模块的 ID：管理界面 HTTPS 由它提供证书，设置了 HTTPS 监听时不能关闭。
const certModuleID = "ssl"

// moduleOption 是设置页“功能模块”中的一项，只读。
type moduleOption struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Requires 为该模块依赖的模块 ID
	Requires []string `json:"requires,omitempty"`
}

// moduleOptions 列出全部模块及其依赖，供设置页渲染开关。
func (a *App) moduleOptions() []moduleOption {
	out := make([]moduleOption, 0, len(a.modules))
	for _, e := range a.modules {
		out = append(out, moduleOption{ID: e.mod.ID(), Title: e.mod.Title(), Requires: a.opts.Requires[e.mod.ID()]})
	}
	return out
}

// moduleTitle 返回模块名称，未知模块返回 ID 本身。
func (a *App) moduleTitle(id string) string {
	for _, e := range a.modules {
		if e.mod.ID() == id {
			return e.mod.Title()
		}
	}
	return id
}

// resolveEnabledModules 按设置确定本次启动开启哪些模块。
// 配置文件可能被手工修改或从备份恢复，若关闭了仍被依赖的模块，则保持该模块开启并记录警告，避免依赖方无法工作。
func (a *App) resolveEnabledModules() {
	s := a.settingsSnapshot()
	off := map[string]bool{}
	for _, id := range s.DisabledModules {
		off[id] = true
	}
	for _, e := range a.modules {
		e.enabled = !off[e.mod.ID()]
	}
	// 依赖可能是多级的，循环到不再有变化为止
	for changed := true; changed; {
		changed = false
		for _, e := range a.modules {
			if e.enabled {
				continue
			}
			if by := a.requiredBy(e.mod.ID(), a.enabledIDs(), s.HTTPSListen); len(by) > 0 {
				e.enabled = true
				changed = true
				a.sysLogs.Add(logbuf.LevelWarn, "settings", fmt.Sprintf("功能模块「%s」被「%s」依赖，已保持开启", e.mod.Title(), strings.Join(by, "、")))
			}
		}
	}
}

// enabledIDs 返回当前开启的模块 ID 集合。
func (a *App) enabledIDs() map[string]bool {
	on := map[string]bool{}
	for _, e := range a.modules {
		if e.enabled {
			on[e.mod.ID()] = true
		}
	}
	return on
}

// requiredBy 返回在 enabled 集合中依赖 id 的模块名称；设置了 HTTPS 监听时证书模块还被管理界面依赖。
func (a *App) requiredBy(id string, enabled map[string]bool, httpsListen string) []string {
	var by []string
	for _, e := range a.modules {
		if enabled[e.mod.ID()] && slices.Contains(a.opts.Requires[e.mod.ID()], id) {
			by = append(by, e.mod.Title())
		}
	}
	if id == certModuleID && httpsListen != "" {
		by = append(by, "管理界面 HTTPS")
	}
	return by
}

// validateDisabledModules 校验要关闭的模块：必须是已知模块，且不能被仍开启的模块依赖。
func (a *App) validateDisabledModules(s Settings) error {
	known := map[string]bool{}
	for _, e := range a.modules {
		known[e.mod.ID()] = true
	}
	off := map[string]bool{}
	for _, id := range s.DisabledModules {
		if !known[id] {
			return fmt.Errorf("未知的功能模块: %s", id)
		}
		off[id] = true
	}
	enabled := map[string]bool{}
	for id := range known {
		if !off[id] {
			enabled[id] = true
		}
	}
	for _, id := range s.DisabledModules {
		if by := a.requiredBy(id, enabled, s.HTTPSListen); len(by) > 0 {
			return fmt.Errorf("功能模块「%s」被「%s」依赖，不能关闭", a.moduleTitle(id), strings.Join(by, "、"))
		}
	}
	return nil
}

// disabledModuleIDs 返回本次启动中已关闭的模块 ID，始终为非 nil 切片，便于前端直接使用。
func (a *App) disabledModuleIDs() []string {
	out := []string{}
	for _, e := range a.modules {
		if !e.enabled {
			out = append(out, e.mod.ID())
		}
	}
	return out
}

// moduleDisabledHandler 响应已关闭模块的全部接口。
func (a *App) moduleDisabledHandler(m module.Module) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusForbidden, "功能模块「%s」已关闭，请在 设置 > 功能模块 中开启并重启后使用", m.Title())
	}
}

// skipDisabledImport 以预览方式解析已关闭模块的 Lucky 配置，有可导入内容时返回未导入的报告。
func (a *App) skipDisabledImport(m module.Module, imp luckyimport.Importer, doc luckyimport.Document, opts luckyimport.Options) *luckyimport.Report {
	opts.DryRun = true
	rep, err := imp.ImportLucky(doc, opts)
	if err != nil || rep == nil || rep.Found == 0 {
		return nil
	}
	return &luckyimport.Report{Module: m.ID(), Title: m.Title(), Found: rep.Found,
		Warnings: []string{"该功能模块已关闭，未导入；开启并重启后可重新导入"}}
}

// moduleInherit 表示模块 to 的配置是从模块 from 迁移来的。
type moduleInherit struct {
	from, to string
}

// migrateModules 在初始化之前依次执行各模块的配置迁移。有配置从其他模块迁来时，来源模块已关闭的，
// 新模块也写入关闭列表，保持迁移前的行为；返回迁移关系，令牌权限在令牌加载后补充。
func (a *App) migrateModules(mods []module.Module) ([]moduleInherit, error) {
	var out []moduleInherit
	for _, m := range mods {
		mg, ok := m.(module.Migrator)
		if !ok {
			continue
		}
		from, err := mg.Migrate(a.store)
		if err != nil {
			return nil, fmt.Errorf("模块 %s 迁移配置失败: %w", m.ID(), err)
		}
		if from == "" {
			continue
		}
		out = append(out, moduleInherit{from: from, to: m.ID()})
		fromTitle := from
		for _, o := range mods {
			if o.ID() == from {
				fromTitle = o.Title()
			}
		}
		a.sysLogs.Add(logbuf.LevelInfo, "", fmt.Sprintf("已把「%s」的部分配置迁移到「%s」", fromTitle, m.Title()))
		s := a.settingsSnapshot()
		if !slices.Contains(s.DisabledModules, from) || slices.Contains(s.DisabledModules, m.ID()) {
			continue
		}
		if err := a.disableModuleOnDisk(m.ID()); err != nil {
			return nil, fmt.Errorf("保存模块开关失败: %w", err)
		}
	}
	return out, nil
}

// disableModuleOnDisk 把模块加入关闭列表并保存设置（启动阶段使用，模块尚未开始运行）。
func (a *App) disableModuleOnDisk(id string) error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	next := a.settings
	next.DisabledModules = append(slices.Clone(next.DisabledModules), id)
	next.normalize()
	saved := next
	// 命令行 -listen 覆盖时写回配置文件中的原地址
	if a.opts.Listen != "" {
		saved.Listen = a.fileListen
	}
	if err := a.store.Save(settingsName, saved); err != nil {
		return err
	}
	a.settings = next
	return nil
}

// inheritTokenScopes 让拥有来源模块权限的 API 令牌同时获得迁移目标模块的同级权限，迁移前能管理这些配置的令牌迁移后仍然可以。
func (a *App) inheritTokenScopes(inherits []moduleInherit) error {
	for _, in := range inherits {
		n, err := a.tokens.InheritScopes(in.from, in.to)
		if err != nil {
			return fmt.Errorf("更新 API 令牌权限失败: %w", err)
		}
		if n > 0 {
			a.sysLogs.Add(logbuf.LevelInfo, "", fmt.Sprintf("已为 %d 个拥有「%s」权限的 API 令牌添加「%s」的同级权限", n, in.from, in.to))
		}
	}
	return nil
}
