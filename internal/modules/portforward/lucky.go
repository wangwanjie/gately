package portforward

import (
	"encoding/json"
	"fmt"

	"github.com/wangwanjie/gately/internal/luckyimport"
)

// luckyRule 对应 Lucky 端口转发规则中 Gately 关心的字段（lucky_portforward 配置）。
type luckyRule struct {
	Name              string
	Enable            bool
	ForwardTypes      []string
	ListenAddress     string
	ListenPorts       string
	TargetAddressList []string
	TargetPorts       string
	// OpenFirewallPorts 为 Lucky 的“自动放行防火墙端口”
	OpenFirewallPorts bool
	Options           struct {
		SingleProxyMaxTCPConnections int
		UDPSessionTimeout            int
		SafeMode                     string
		TCPListenTLS                 bool
		TCPRelayTLS                  bool
		TCPStreamEncryptionSource    bool
		TCPStreamEncryptionAccept    bool
		SinglePortSpeedLimit         bool
		RuleSpeedLimit               bool
		UDPPacketSourceEncryption    bool
		UDPPacketAcceptEncryption    bool
	}
}

// convertLuckyRules 把 Lucky 规则转换为 Gately 规则，并列出无法迁移的选项。
// ipFilter 为本次导入的 Lucky IP 过滤名单（可为 nil），用于转换规则的安全模式。
func convertLuckyRules(raw json.RawMessage, ipFilter map[string]luckyimport.IPFilterList) ([]Rule, []string, error) {
	var src []luckyRule
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, nil, err
	}
	var (
		out      []Rule
		warnings []string
	)
	for i, lr := range src {
		label := lr.Name
		if label == "" {
			label = fmt.Sprintf("第 %d 条(%s)", i+1, lr.ListenPorts)
		}
		rule := Rule{
			Name:            lr.Name,
			Enable:          lr.Enable,
			Protocols:       lr.ForwardTypes,
			ListenAddress:   lr.ListenAddress,
			ListenPorts:     lr.ListenPorts,
			TargetAddresses: lr.TargetAddressList,
			TargetPorts:     lr.TargetPorts,
			MaxConnections:  lr.Options.SingleProxyMaxTCPConnections,
			// 沿用 Lucky 的自动放行防火墙设置
			AutoFirewall: lr.OpenFirewallPorts,
		}
		// Lucky 的 UDPSessionTimeout 单位为毫秒（界面默认 30000），Gately 使用秒
		if lr.Options.UDPSessionTimeout > 0 {
			rule.UDPSessionTimeout = max(1, lr.Options.UDPSessionTimeout/1000)
		}
		o := lr.Options
		if o.TCPListenTLS || o.TCPRelayTLS || o.TCPStreamEncryptionSource || o.TCPStreamEncryptionAccept || o.UDPPacketSourceEncryption || o.UDPPacketAcceptEncryption {
			warnings = append(warnings, label+": TLS/流加密选项暂不支持，已按明文转发导入")
		}
		if o.SinglePortSpeedLimit || o.RuleSpeedLimit {
			warnings = append(warnings, label+": 限速选项暂不支持，已忽略")
		}
		// 安全模式转换为来源 IP 访问控制：白名单取 Lucky IP 过滤中的白名单，黑名单取黑名单与全局黑名单
		access, warns := luckyimport.SafeModeAccess(o.SafeMode, ipFilter)
		rule.Access = access
		for _, w := range warns {
			warnings = append(warnings, label+": "+w)
		}
		if err := rule.Validate(); err != nil {
			warnings = append(warnings, label+": 跳过，"+err.Error())
			continue
		}
		out = append(out, rule)
	}
	return out, warnings, nil
}

// ImportLucky 实现 luckyimport.Importer。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	raw, ok := doc["PortForwardsRuleList"]
	if !ok {
		return nil, nil
	}
	rules, warnings, err := convertLuckyRules(raw, opts.IPFilter)
	if err != nil {
		return nil, err
	}
	var total []json.RawMessage
	_ = json.Unmarshal(raw, &total)
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: len(total), Warnings: warnings}
	m.mu.Lock()
	existing := map[string]bool{}
	for _, r := range m.cfg.Rules {
		existing[r.Name] = true
	}
	m.mu.Unlock()
	var fresh []Rule
	for _, r := range rules {
		if existing[r.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped(r.Name))
			continue
		}
		if !opts.KeepEnabled {
			r.Enable = false
		}
		fresh = append(fresh, r)
	}
	rep.Imported = len(fresh)
	if !opts.KeepEnabled && rep.Imported > 0 {
		rep.Warnings = append(rep.Warnings, "导入的规则默认停用：请先停止 Lucky 中对应的规则，再在 Gately 中启用")
	}
	if opts.DryRun {
		return rep, nil
	}
	n, err := m.ImportRules(fresh)
	if err != nil {
		return nil, err
	}
	// 保留启用状态时一并迁移模块总开关
	var lc struct{ Enable *bool }
	if raw, ok := doc["PortForwardsConfigure"]; ok && opts.KeepEnabled {
		_ = json.Unmarshal(raw, &lc)
	}
	if lc.Enable != nil {
		m.mu.Lock()
		if m.cfg.Settings.Enable != *lc.Enable {
			m.cfg.Settings.Enable = *lc.Enable
			if err := m.saveLocked(); err == nil {
				for _, r := range m.cfg.Rules {
					m.restartLocked(r.Key)
				}
			}
			if !*lc.Enable {
				rep.Warnings = append(rep.Warnings, "Lucky 中端口转发模块处于关闭状态，已同步关闭 Gately 的端口转发模块")
			}
		}
		m.mu.Unlock()
	}
	rep.Imported = n
	return rep, nil
}
