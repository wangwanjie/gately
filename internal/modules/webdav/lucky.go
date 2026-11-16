package webdav

import (
	"encoding/json"
	"fmt"

	"github.com/wangwanjie/gately/internal/luckyimport"
)

// ImportLucky 实现 luckyimport.Importer，迁移监听参数与用户（导入后默认停用）。
// Lucky 用户的挂载列表转换为多目录用户；与 Gately 中已有用户同名的用户跳过，已有用户保持不变。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	raw, ok := doc["WebDavConf"]
	if !ok {
		return nil, nil
	}
	var src struct {
		Enable     bool
		ListenIP   string
		ListenPort int
		TLSEnable  bool
		Users      json.RawMessage
	}
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, err
	}
	users, warnings, err := luckyimport.ConvertFileUsers(src.Users, opts)
	if err != nil {
		return nil, err
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: 1, Warnings: warnings}

	m.mu.Lock()
	defer m.mu.Unlock()
	s := Settings{ListenIP: src.ListenIP, Port: src.ListenPort, TLS: src.TLSEnable, Users: append([]User(nil), m.settings.Users...)}
	existing := map[string]bool{}
	for _, u := range s.Users {
		existing[u.Name] = true
	}
	added := 0
	for _, u := range users {
		if existing[u.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped("用户 "+u.Name))
			continue
		}
		s.Users = append(s.Users, User{Name: u.Name, Password: u.Password, Mounts: u.Mounts})
		added++
	}
	s.Enable = src.Enable && opts.KeepEnabled && len(s.Users) > 0
	if err := s.Validate(); err != nil {
		rep.Warnings = append(rep.Warnings, "跳过，"+err.Error())
		return rep, nil
	}
	if err := s.checkDirs(); err != nil {
		rep.Warnings = append(rep.Warnings, "跳过，"+err.Error())
		return rep, nil
	}
	rep.Imported = 1
	rep.Warnings = append(rep.Warnings, fmt.Sprintf("导入 %d 个用户", added))
	if !s.Enable {
		rep.Warnings = append(rep.Warnings, "WebDAV 设置已导入并保持停用")
	}
	if opts.DryRun {
		return rep, nil
	}
	if err := m.store.Save(ID, s); err != nil {
		return nil, err
	}
	m.settings = s
	m.restartLocked()
	return rep, nil
}
