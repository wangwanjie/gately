package storage

import (
	"fmt"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/luckyimport"
)

// ImportLucky 实现 luckyimport.Importer，迁移 Lucky 存储管理中的本地目录与 WebDAV 存储。
// 沿用 Lucky 的存储 Key，FTP / WebDAV 用户引用的存储在之后单独导入时仍能解析。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	items, ok, err := luckyimport.StorageList(doc)
	if !ok {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: len(items)}
	var mounts []Mount
	for _, it := range items {
		mt, warns, skip := convertLuckyStore(it, opts)
		rep.Warnings = append(rep.Warnings, warns...)
		if skip != "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 跳过，%s", it.Remark, skip))
			continue
		}
		if err := validate(&mt); err != nil {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 跳过，%v", it.Remark, err))
			continue
		}
		mounts = append(mounts, mt)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	names, keys := map[string]bool{}, map[string]bool{}
	for _, mt := range m.cfg.Mounts {
		names[mt.Name], keys[mt.Key] = true, true
	}
	var fresh []Mount
	for _, mt := range mounts {
		if names[mt.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped(mt.Name))
			continue
		}
		if mt.Key == "" || keys[mt.Key] {
			mt.Key = crud.NewKey()
		}
		names[mt.Name], keys[mt.Key] = true, true
		fresh = append(fresh, mt)
	}
	rep.Imported = len(fresh)
	if opts.DryRun {
		return rep, nil
	}
	old := m.cfg.Mounts
	m.cfg.Mounts = append(append([]Mount(nil), old...), fresh...)
	if err := m.store.Save(ID, m.cfg); err != nil {
		m.cfg.Mounts = old
		return nil, err
	}
	return rep, nil
}

// convertLuckyStore 转换一项存储，skip 非空时表示无法迁移及原因。
func convertLuckyStore(it luckyimport.StorageItem, opts luckyimport.Options) (mt Mount, warnings []string, skip string) {
	mt = Mount{
		Key:      it.Key,
		Name:     it.Remark,
		Type:     it.Type,
		Enable:   it.Enable && opts.KeepEnabled,
		ReadOnly: !it.Writable,
	}
	switch it.Type {
	case luckyimport.StoreLocal:
		mt.Path = it.Params.LocalPath
	case luckyimport.StoreWebDAV:
		mt.URL = it.Params.WebDAVURL
		mt.SkipVerify = it.Params.InsecureSkipVerify
		if !it.Params.WebDAVAnonymous {
			mt.Username, mt.Password = it.Params.WebDAVUser, it.Params.WebDAVPasswd
		}
		if it.Params.Proxy != "" {
			warnings = append(warnings, fmt.Sprintf("%s: 经 %s 代理访问暂不支持，将直接连接", it.Remark, it.Params.Proxy))
		}
	case luckyimport.StoreAliyunpan:
		// 阿里云盘授权经 Lucky 官方服务中转，Gately 不连接该服务
		return mt, nil, "阿里云盘存储暂不支持"
	default:
		return mt, nil, fmt.Sprintf("不支持的存储类型 %q", it.Type)
	}
	if it.SystemMount.Enable {
		warnings = append(warnings, fmt.Sprintf("%s: 挂载到系统目录 %s 暂不支持，已忽略", it.Remark, it.SystemMount.MountPoint))
	}
	return mt, warnings, ""
}

// MountPath 返回本地挂载点的目录，供导入 Lucky FTP / WebDAV 用户时解析存储引用。
func (m *Module) MountPath(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mt := range m.cfg.Mounts {
		if mt.Key == key && mt.Type == TypeLocal {
			return mt.Path, true
		}
	}
	return "", false
}
