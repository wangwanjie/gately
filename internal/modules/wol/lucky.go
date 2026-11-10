package wol

import (
	"encoding/json"
	"fmt"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/luckyimport"
)

type luckyDevice struct {
	DeviceName          string
	MacList             []string
	BroadcastIPs        []string
	Port                int
	Relay               bool
	Repeat              int
	IOT_DianDeng_Enable bool
	IOT_Bemfa_Enable    bool
}

// ImportLucky 实现 luckyimport.Importer。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	raw, ok := doc["WOLDeviceList"]
	if !ok {
		return nil, nil
	}
	var src []luckyDevice
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, err
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: len(src)}
	var devices []Device
	for _, ld := range src {
		d := Device{Name: ld.DeviceName, MACs: ld.MacList, BroadcastIPs: ld.BroadcastIPs, Port: ld.Port, Repeat: ld.Repeat}
		if ld.IOT_DianDeng_Enable {
			rep.Warnings = append(rep.Warnings, ld.DeviceName+": 点灯科技语音接入暂不支持，已忽略")
		}
		// Lucky 的巴法云主题与私钥不在设备配置中，导入后需在设备与巴法云设置中重新填写
		if ld.IOT_Bemfa_Enable {
			rep.Warnings = append(rep.Warnings, ld.DeviceName+": 巴法云接入未随导入迁移，请在设备中填写巴法云主题并开启巴法云接入")
		}
		if err := d.Validate(); err != nil {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: 跳过，%v", ld.DeviceName, err))
			continue
		}
		devices = append(devices, d)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := map[string]bool{}
	for _, d := range m.cfg.Devices {
		existing[d.Name] = true
	}
	var fresh []Device
	for _, d := range devices {
		if existing[d.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped(d.Name))
			continue
		}
		fresh = append(fresh, d)
	}
	rep.Imported = len(fresh)
	if opts.DryRun {
		return rep, nil
	}
	old := m.cfg.Devices
	for _, d := range fresh {
		d.Key = crud.NewKey()
		m.cfg.Devices = append(m.cfg.Devices, d)
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Devices = old
		return nil, err
	}
	return rep, nil
}
