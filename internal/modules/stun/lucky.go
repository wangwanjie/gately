package stun

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/webhook"
)

// luckyRule 对应 lucky_stun 配置中的规则（仅迁移需要的字段）。
type luckyRule struct {
	Name                                      string
	Enable                                    bool
	StunType                                  string
	UseGlobalStunServerList                   bool
	StunServerList                            []string
	TcpKeepAliveServerList                    []string
	StunListenType                            string
	SpecifyNetworkInterface                   string
	NetworkInterfaceReg                       string
	ListenIP                                  string
	ListenPort                                int
	DisableStunAvalidCheck                    bool
	DisablePortForward                        bool
	TargetAddressList                         []string
	TargetPort                                int
	NatPMP                                    bool
	UPnP                                      bool
	UPnPGawayIP                               string
	UPnPLocalPort                             int
	UPnpLocalHost                             string
	UpnPDiyControlAPIUrl                      string
	NatPMPGateway                             string
	StunHeartbeatInterval                     int
	StunTimeout                               int
	StunRetryInterval                         int
	StunAutoRetry                             bool
	GlobalWebhook                             bool
	WebhookEnable                             bool
	WebhookOnlyAddrChange                     bool
	WebhookURL                                string
	WebhookMethod                             string
	WebhookHeaders                            []string
	WebhookRequestBody                        string
	WebhookDisableCallbackSuccessContentCheck bool
	WebhookSuccessContent                     []string
	WebhookInsecureSkipVerify                 bool
	WebHookTimeout                            int
	CallScript                                bool
	CallScriptContent                         string
	Options                                   struct {
		SingleProxyMaxTCPConnections int
		UDPSessionTimeout            int
		TCPListenTLS                 bool
		TCPRelayTLS                  bool
		TCPStreamEncryptionSource    bool
		TCPStreamEncryptionAccept    bool
		SinglePortSpeedLimit         bool
		RuleSpeedLimit               bool
		SafeMode                     string
	}
}

type luckyConfigure struct {
	EnableModule          bool
	WebhookEnable         bool
	WebhookURL            string
	WebhookMethod         string
	WebhookHeaders        []string
	WebhookRequestBody    string
	WebhookSuccessContent []string
	WebHookTimeout        int
	GlobalStunServerList  []string
}

func luckyWebhook(enable bool, url, method string, headers []string, body string, disableCheck bool, success []string, insecure bool, timeout int) webhook.Config {
	c := webhook.Config{Enable: enable, URL: url, Method: method, Headers: headers, Body: body, InsecureSkipVerify: insecure, Timeout: timeout}
	if !disableCheck {
		c.SuccessContent = success
	}
	return c
}

// convertLuckyRules 转换 Lucky STUN 规则。导入后默认停用，避免与仍在运行的 Lucky 抢占端口与 UPnP 映射。
// ipFilter 为本次导入的 Lucky IP 过滤名单（可为 nil），用于转换规则的安全模式。
func convertLuckyRules(raw json.RawMessage, ipFilter map[string]luckyimport.IPFilterList) ([]Rule, []string, int, error) {
	var src []luckyRule
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, nil, 0, err
	}
	var (
		out   []Rule
		warns []string
		// 脚本中引用 Lucky 容器路径的规则，汇总成一条告警
		luckyPathRules []string
	)
	for i, lr := range src {
		label := lr.Name
		if label == "" {
			label = fmt.Sprintf("第 %d 条规则", i+1)
		}
		warn := func(msg string) { warns = append(warns, label+": "+msg) }
		proto := strings.ToLower(lr.StunType)
		if proto != ProtoTCP4 && proto != ProtoUDP4 {
			warn(fmt.Sprintf("穿透类型 %q 暂不支持，已跳过", lr.StunType))
			continue
		}
		r := Rule{
			Name:                lr.Name,
			Enable:              lr.Enable,
			Protocol:            proto,
			LocalPort:           lr.ListenPort,
			UseGlobalServers:    lr.UseGlobalStunServerList,
			StunServers:         lr.StunServerList,
			KeepAliveServers:    lr.TcpKeepAliveServerList,
			DisableForward:      lr.DisablePortForward,
			TargetAddresses:     lr.TargetAddressList,
			TargetPort:          lr.TargetPort,
			UPnP:                lr.UPnP,
			UPnPGateway:         lr.UPnPGawayIP,
			UPnPLocalHost:       lr.UPnpLocalHost,
			UPnPControlURL:      lr.UpnPDiyControlAPIUrl,
			UPnPInternalPort:    lr.UPnPLocalPort,
			NATPMP:              lr.NatPMP,
			NATPMPGateway:       lr.NatPMPGateway,
			HeartbeatInterval:   lr.StunHeartbeatInterval,
			StunTimeout:         lr.StunTimeout,
			RetryInterval:       lr.StunRetryInterval,
			AutoRetry:           lr.StunAutoRetry,
			DisableCheck:        lr.DisableStunAvalidCheck,
			UseGlobalWebhook:    lr.GlobalWebhook,
			WebhookOnlyOnChange: lr.WebhookOnlyAddrChange,
			ScriptEnable:        lr.CallScript && strings.TrimSpace(lr.CallScriptContent) != "",
			Script:              lr.CallScriptContent,
			MaxConnections:      lr.Options.SingleProxyMaxTCPConnections,
		}
		// Lucky 的 UDP 会话超时单位为毫秒
		if lr.Options.UDPSessionTimeout > 0 {
			r.UDPSessionTimeout = max(1, lr.Options.UDPSessionTimeout/1000)
		}
		if lr.StunListenType == "networkInterface" {
			r.Bind = Bind{Mode: BindInterface, Interface: lr.SpecifyNetworkInterface, Regex: lr.NetworkInterfaceReg}
		} else {
			r.Bind = Bind{Mode: BindIP, IP: lr.ListenIP}
		}
		r.Webhook = luckyWebhook(lr.WebhookEnable, lr.WebhookURL, lr.WebhookMethod, lr.WebhookHeaders, lr.WebhookRequestBody,
			lr.WebhookDisableCallbackSuccessContentCheck, lr.WebhookSuccessContent, lr.WebhookInsecureSkipVerify, lr.WebHookTimeout)
		o := lr.Options
		if o.TCPListenTLS || o.TCPRelayTLS || o.TCPStreamEncryptionSource || o.TCPStreamEncryptionAccept {
			warn("TLS/流加密选项暂不支持，已按明文转发导入")
		}
		if o.SinglePortSpeedLimit || o.RuleSpeedLimit {
			warn("限速选项暂不支持，已忽略")
		}
		// 安全模式转换为内置转发的来源 IP 访问控制；不使用内置转发时外部连接不经过 Gately，无需转换
		if !r.DisableForward {
			access, accessWarns := luckyimport.SafeModeAccess(o.SafeMode, ipFilter)
			r.Access = access
			for _, w := range accessWarns {
				warn(w)
			}
		}
		if r.ScriptEnable && strings.Contains(r.Script, "/etc/lucky/") {
			luckyPathRules = append(luckyPathRules, label)
		}
		if err := r.Validate(); err != nil {
			warn("跳过，" + err.Error())
			continue
		}
		out = append(out, r)
	}
	if len(luckyPathRules) > 0 {
		warns = append(warns, fmt.Sprintf("%d 条规则的脚本引用了 Lucky 容器内路径 /etc/lucky/（%s），Docker 部署时请把脚本目录挂载到相同路径",
			len(luckyPathRules), strings.Join(luckyPathRules, "、")))
	}
	return out, warns, len(src), nil
}

// ImportLucky 实现 luckyimport.Importer。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	raw, ok := doc["StunRuleList"]
	if !ok {
		return nil, nil
	}
	rules, warns, found, err := convertLuckyRules(raw, opts.IPFilter)
	if err != nil {
		return nil, err
	}
	var gc luckyConfigure
	if c, ok := doc["StunConfigure"]; ok {
		_ = json.Unmarshal(c, &gc)
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: found, Warnings: warns}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := map[string]bool{}
	for _, r := range m.cfg.Rules {
		existing[r.Name] = true
	}
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
	rules = fresh
	rep.Imported = len(rules)
	if !opts.KeepEnabled && rep.Imported > 0 {
		rep.Warnings = append(rep.Warnings, "导入的规则默认停用：请先停止 Lucky 的同名规则，再在 Gately 中启用，避免端口与 UPnP 映射冲突")
	}
	if opts.DryRun {
		return rep, nil
	}
	oldRules, oldSettings := m.cfg.Rules, m.cfg.Settings
	// 全局 STUN 服务器列表与全局 Webhook 一并迁移
	if len(gc.GlobalStunServerList) > 0 {
		m.cfg.Settings.StunServers = gc.GlobalStunServerList
	}
	if gc.WebhookEnable {
		m.cfg.Settings.Webhook = webhook.Config{Enable: true, URL: gc.WebhookURL, Method: gc.WebhookMethod, Headers: gc.WebhookHeaders,
			Body: gc.WebhookRequestBody, SuccessContent: gc.WebhookSuccessContent, Timeout: gc.WebHookTimeout}
	}
	// 保留启用状态时一并迁移模块总开关（Lucky 中模块关闭时规则不会运行）
	moduleOff := false
	if _, ok := doc["StunConfigure"]; ok && opts.KeepEnabled {
		m.cfg.Settings.Enable = gc.EnableModule
		moduleOff = !gc.EnableModule
	}
	m.cfg.Settings.normalize()
	for _, r := range rules {
		r.Key = crud.NewKey()
		m.cfg.Rules = append(m.cfg.Rules, r)
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Rules, m.cfg.Settings = oldRules, oldSettings
		return nil, err
	}
	m.publishIPSetUsersLocked()
	// 模块开关变化时重启全部规则，否则只启动新导入的规则
	restart := m.cfg.Rules[len(oldRules):]
	if oldSettings.Enable != m.cfg.Settings.Enable {
		restart = m.cfg.Rules
	}
	for _, r := range restart {
		m.restartLocked(r.Key)
	}
	if moduleOff && rep.Imported > 0 {
		rep.Warnings = append(rep.Warnings, "Lucky 中 STUN 模块处于关闭状态，已同步关闭 Gately 的 STUN 模块，规则不会运行")
	}
	return rep, nil
}
