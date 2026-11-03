package ddns

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/webhook"
)

// luckyTask 对应 lucky_ddns 配置中的任务（仅列出迁移需要的字段）。
type luckyTask struct {
	TaskName            string
	Enable              bool
	V4QueryIPEnable     bool
	V4QueryIPType       string
	V4QueryUrl          string
	V4NetInterface      string
	V4NetInterfaceIPReg string
	V4GetIPScript       string
	V6QueryIPEnable     bool
	V6QueryIPType       string
	V6QueryUrl          string
	V6NetInterface      string
	V6NetInterfaceIPReg string
	V6GetIPScript       string
	Records             []struct {
		SyncRecordData map[string]any
		Key            string
		Disable        bool
	}
	Domains  []string
	TaskType string
	DNS      struct {
		Name          string
		ID            string
		Secret        string
		ForceInterval int
		Callback      struct {
			URL                    string
			Method                 string
			Headers                []string
			RequestBody            string
			CallbackSuccessContent []string
		}
	}
	WebhookEnable                             bool
	WebhookURL                                string
	WebhookMethod                             string
	WebhookHeaders                            []string
	WebhookRequestBody                        string
	WebhookDisableCallbackSuccessContentCheck bool
	WebhookSuccessContent                     []string
	WebhookInsecureSkipVerify                 bool
	WebHookTimeout                            int
	Intervals                                 int
	HttpClientTimeout                         int
}

// Lucky 官方的 IP 查询服务器。迁移时移除以兑现"不连接项目方服务器"的承诺。
var luckyOfficialHosts = []string{"66666.host", "666666.host"}

func isLuckyOfficial(u string) bool {
	for _, h := range luckyOfficialHosts {
		if strings.Contains(u, h) {
			return true
		}
	}
	return false
}

// convertSource 转换 IP 获取配置，返回告警。
func convertSource(enable bool, typ, urls, iface, reg, script string, fam ipsource.Family) (ipsource.Source, []string) {
	var warns []string
	s := ipsource.Source{Enable: enable, Interface: iface, Regex: reg, Command: script}
	switch typ {
	case "netInterface":
		s.Type = ipsource.TypeInterface
	case "command", "cmd":
		s.Type = ipsource.TypeCommand
	default:
		s.Type = ipsource.TypeURL
	}
	removed := 0
	for _, u := range strings.Split(urls, "\n") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if isLuckyOfficial(u) {
			removed++
			continue
		}
		s.URLs = append(s.URLs, u)
	}
	if removed > 0 && enable {
		warns = append(warns, fmt.Sprintf("IPv%d 查询地址中移除了 %d 个 Lucky 官方服务器地址", fam, removed))
	}
	// Lucky 配置里常见的 ipw.cn 等地址已停止服务，导入时换成替代地址
	s.URLs, _ = ipsource.ReplaceRetiredURLs(s.URLs)
	if len(s.URLs) == 0 {
		if fam == ipsource.IPv4 {
			s.URLs = append([]string{}, ipsource.DefaultIPv4URLs...)
		} else {
			s.URLs = append([]string{}, ipsource.DefaultIPv6URLs...)
		}
	}
	// Lucky 的网卡正则可能写成 @1 表示取第 N 个地址，Gately 暂不支持
	if strings.HasPrefix(s.Regex, "@") {
		if enable {
			warns = append(warns, fmt.Sprintf("IPv%d 网卡地址选择 %q（按序号）暂不支持，已改为自动选择公网地址", fam, s.Regex))
		}
		s.Regex = ""
	}
	return s, warns
}

func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// convertLuckyTasks 转换 Lucky DDNS 任务。
func convertLuckyTasks(raw json.RawMessage) ([]Task, []string, int, error) {
	var src []luckyTask
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, nil, 0, err
	}
	var (
		out   []Task
		warns []string
	)
	for i, lt := range src {
		label := lt.TaskName
		if label == "" {
			label = fmt.Sprintf("第 %d 个任务", i+1)
		}
		warn := func(msg string) { warns = append(warns, label+": "+msg) }

		if _, ok := dnsprovider.Lookup(lt.DNS.Name); !ok {
			warn(fmt.Sprintf("服务商 %q 暂不支持，已跳过", lt.DNS.Name))
			continue
		}
		t := Task{
			Name:          lt.TaskName,
			Enable:        lt.Enable,
			Provider:      lt.DNS.Name,
			Credentials:   dnsprovider.Credentials{ID: lt.DNS.ID, Secret: lt.DNS.Secret},
			Interval:      max(lt.Intervals, minInterval),
			ForceInterval: lt.DNS.ForceInterval,
			HTTPTimeout:   lt.HttpClientTimeout,
		}
		var w4, w6 []string
		t.IPv4, w4 = convertSource(lt.V4QueryIPEnable, lt.V4QueryIPType, lt.V4QueryUrl, lt.V4NetInterface, lt.V4NetInterfaceIPReg, lt.V4GetIPScript, ipsource.IPv4)
		t.IPv6, w6 = convertSource(lt.V6QueryIPEnable, lt.V6QueryIPType, lt.V6QueryUrl, lt.V6NetInterface, lt.V6NetInterfaceIPReg, lt.V6GetIPScript, ipsource.IPv6)
		for _, w := range append(w4, w6...) {
			warn(w)
		}
		if lt.DNS.Name == "callback" {
			cb := lt.DNS.Callback
			t.Credentials.Extra = map[string]string{
				"url":            cb.URL,
				"method":         cb.Method,
				"headers":        strings.Join(cb.Headers, "\n"),
				"body":           cb.RequestBody,
				"successContent": strings.Join(cb.CallbackSuccessContent, "\n"),
			}
		}

		for j, lr := range lt.Records {
			d := lr.SyncRecordData
			rec := Record{
				Key:    lr.Key,
				Enable: !lr.Disable,
				Zone:   str(d, "DomainName"),
				Sub:    str(d, "SubDomainName"),
				Type:   str(d, "type"),
				Line:   str(d, "line"),
				Remark: str(d, "remark"),
			}
			if rec.Key == "" {
				rec.Key = fmt.Sprintf("r%d", j+1)
			}
			if v, ok := d["ttl"].(float64); ok {
				rec.TTL = int(v)
			}
			rec.Value = str(d, "ipv4Address")
			if rec.Value == "" {
				rec.Value = str(d, "ipv6Address")
			}
			if rec.Value == "" {
				rec.Value = str(d, "value")
			}
			t.Records = append(t.Records, rec)
		}
		// 旧版 Lucky 使用 Domains 列表，记录类型由任务类型决定
		if len(lt.Records) == 0 {
			typ, val := "A", VarIPv4
			if lt.TaskType == "IPv6" {
				typ, val = "AAAA", VarIPv6
			}
			for _, full := range lt.Domains {
				sub, zone := dnsprovider.SplitFQDN(full)
				t.Records = append(t.Records, Record{Enable: true, Zone: zone, Sub: sub, Type: typ, Value: val})
			}
		}

		if lt.WebhookEnable {
			t.Webhook = webhook.Config{
				Enable:             true,
				URL:                lt.WebhookURL,
				Method:             lt.WebhookMethod,
				Headers:            lt.WebhookHeaders,
				Body:               lt.WebhookRequestBody,
				Timeout:            lt.WebHookTimeout,
				InsecureSkipVerify: lt.WebhookInsecureSkipVerify,
			}
			if !lt.WebhookDisableCallbackSuccessContentCheck {
				t.Webhook.SuccessContent = lt.WebhookSuccessContent
			}
		}
		if err := t.Validate(); err != nil {
			warn("跳过，" + err.Error())
			continue
		}
		out = append(out, t)
	}
	return out, warns, len(src), nil
}

// ImportLucky 实现 luckyimport.Importer。
func (m *Module) ImportLucky(doc luckyimport.Document, opts luckyimport.Options) (*luckyimport.Report, error) {
	raw, ok := doc["DDNSTaskList"]
	if !ok {
		return nil, nil
	}
	tasks, warns, found, err := convertLuckyTasks(raw)
	if err != nil {
		return nil, err
	}
	rep := &luckyimport.Report{Module: ID, Title: m.Title(), Found: found, Warnings: warns}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := map[string]bool{}
	for _, t := range m.cfg.Tasks {
		existing[t.Name] = true
	}
	var fresh []Task
	for _, t := range tasks {
		if existing[t.Name] {
			rep.Warnings = append(rep.Warnings, luckyimport.Skipped(t.Name))
			continue
		}
		if !opts.KeepEnabled {
			t.Enable = false
		}
		fresh = append(fresh, t)
	}
	rep.Imported = len(fresh)
	if !opts.KeepEnabled && rep.Imported > 0 {
		rep.Warnings = append(rep.Warnings, "导入的任务默认停用，请确认 Gately 运行在目标设备上后再逐个启用")
	}
	if opts.DryRun {
		return rep, nil
	}
	old, oldSettings := m.cfg.Tasks, m.cfg.Settings
	var lc struct{ Enable *bool }
	if raw, ok := doc["DDNSConfigure"]; ok && opts.KeepEnabled {
		_ = json.Unmarshal(raw, &lc)
		if lc.Enable != nil {
			m.cfg.Settings.Enable = *lc.Enable
			if !*lc.Enable && rep.Imported > 0 {
				rep.Warnings = append(rep.Warnings, "Lucky 中动态域名模块处于关闭状态，已同步关闭 Gately 的动态域名模块")
			}
		}
	}
	for _, t := range fresh {
		t.Key = crud.NewKey()
		m.cfg.Tasks = append(m.cfg.Tasks, t)
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Tasks, m.cfg.Settings = old, oldSettings
		return nil, err
	}
	// 模块开关变化时重启全部任务，否则只启动新导入的任务
	restart := m.cfg.Tasks[len(old):]
	if oldSettings.Enable != m.cfg.Settings.Enable {
		restart = m.cfg.Tasks
	}
	for _, t := range restart {
		m.restartLocked(t.Key)
	}
	return rep, nil
}
