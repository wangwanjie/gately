package ddns

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/webhook"
)

// 记录值中可用的变量
const (
	VarIPv4 = "{ipv4Addr}"
	VarIPv6 = "{ipv6Addr}"
)

// 支持的记录类型
var validTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true, "TXT": true}

// 主机记录只允许字母数字、短横线、下划线、点、通配符和 @
var reSub = regexp.MustCompile(`^(@|\*|[A-Za-z0-9_*]([A-Za-z0-9_.*-]*[A-Za-z0-9_])?)$`)

// 域名格式的宽松校验
var reZone = regexp.MustCompile(`^([A-Za-z0-9-]+\.)+[A-Za-z0-9-]+$`)

// Record 是任务中的一条解析记录。
type Record struct {
	Key    string `json:"key"`
	Enable bool   `json:"enable"`
	// Zone 为主域名（在服务商处托管的域名），例如 example.com
	Zone string `json:"zone"`
	// Sub 为主机记录，根域名填 @
	Sub  string `json:"sub"`
	Type string `json:"type"`
	// Value 支持变量 {ipv4Addr} {ipv6Addr}，以及 {STUN_规则名_PORT} {STUN_规则名_IP} {STUN_规则名_ADDR}
	Value  string `json:"value"`
	TTL    int    `json:"ttl"`
	Line   string `json:"line"`
	Remark string `json:"remark"`
	// Group 为任务内记录列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group string `json:"group"`
}

// Task 是一个 DDNS 任务：一组记录共用 IP 来源与服务商凭据。
type Task struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group       string                  `json:"group"`
	IPv4        ipsource.Source         `json:"ipv4"`
	IPv6        ipsource.Source         `json:"ipv6"`
	Provider    string                  `json:"provider"`
	Credentials dnsprovider.Credentials `json:"credentials"`
	Records     []Record                `json:"records"`
	// Interval 为检查周期（秒）
	Interval int `json:"interval"`
	// ForceInterval 为强制向服务商同步的周期（秒），0 表示只在 IP 变化时同步
	ForceInterval int            `json:"forceInterval"`
	HTTPTimeout   int            `json:"httpTimeout"`
	Webhook       webhook.Config `json:"webhook"`
	Remark        string         `json:"remark"`
}

// Settings 是模块级配置。
type Settings struct {
	Enable bool `json:"enable"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	Tasks    []Task   `json:"tasks"`
}

const (
	minInterval     = 30
	defaultInterval = 300
	defaultForce    = 3600
	defaultTimeout  = 20
)

// needs 返回任务是否需要获取 IPv4 / IPv6。
func (t *Task) needs() (v4, v6 bool) {
	for _, r := range t.Records {
		if !r.Enable {
			continue
		}
		if strings.Contains(r.Value, VarIPv4) {
			v4 = true
		}
		if strings.Contains(r.Value, VarIPv6) {
			v6 = true
		}
	}
	return
}

// stunRefs 判断已启用的记录值、Webhook 是否引用了指定 STUN 规则（按规则名或 key）的变量。
func (t *Task) stunRefs(rules ...string) (inRecords, inWebhook bool) {
	refs := func(s string) bool {
		for _, rule := range rules {
			if rule != "" && stunaddr.References(s, rule) {
				return true
			}
		}
		return false
	}
	for _, r := range t.Records {
		if r.Enable && refs(r.Value) {
			inRecords = true
			break
		}
	}
	if t.Webhook.Enable {
		for _, s := range webhookTexts(t.Webhook) {
			if refs(s) {
				inWebhook = true
				break
			}
		}
	}
	return
}

// Validate 校验并补全默认值。
func (t *Task) Validate() error {
	t.Name = strings.TrimSpace(t.Name)
	if err := crud.NormalizeGroup(&t.Group); err != nil {
		return err
	}
	if _, ok := dnsprovider.Lookup(t.Provider); !ok {
		return fmt.Errorf("不支持的 DNS 服务商 %q", t.Provider)
	}
	if t.Interval == 0 {
		t.Interval = defaultInterval
	}
	if t.Interval < minInterval {
		return fmt.Errorf("检查周期不能小于 %d 秒", minInterval)
	}
	if t.ForceInterval < 0 || t.HTTPTimeout < 0 {
		return errors.New("周期与超时不能为负数")
	}
	if t.HTTPTimeout == 0 {
		t.HTTPTimeout = defaultTimeout
	}
	if len(t.Records) == 0 {
		return errors.New("至少添加一条记录")
	}
	seen := map[string]bool{}
	for i := range t.Records {
		r := &t.Records[i]
		if err := r.validate(); err != nil {
			return fmt.Errorf("第 %d 条记录: %w", i+1, err)
		}
		id := r.Type + " " + r.Sub + "." + r.Zone + " " + r.Line
		if seen[id] {
			return fmt.Errorf("记录 %s %s 重复", r.Type, r.Sub+"."+r.Zone)
		}
		seen[id] = true
		if r.Key == "" {
			r.Key = fmt.Sprintf("r%d", i+1)
		}
	}
	// 记录 key 需唯一，用于跟踪同步状态
	keys := map[string]bool{}
	for i := range t.Records {
		for keys[t.Records[i].Key] {
			t.Records[i].Key += "_"
		}
		keys[t.Records[i].Key] = true
	}
	v4, v6 := t.needs()
	if v4 && !t.IPv4.Enable {
		return errors.New("有记录使用 {ipv4Addr}，请启用 IPv4 获取")
	}
	if v6 && !t.IPv6.Enable {
		return errors.New("有记录使用 {ipv6Addr}，请启用 IPv6 获取")
	}
	if err := t.IPv4.Validate(); err != nil {
		return fmt.Errorf("IPv4 获取: %w", err)
	}
	if err := t.IPv6.Validate(); err != nil {
		return fmt.Errorf("IPv6 获取: %w", err)
	}
	return t.Webhook.Validate()
}

func (r *Record) validate() error {
	if err := crud.NormalizeGroup(&r.Group); err != nil {
		return err
	}
	r.Zone = strings.ToLower(strings.Trim(strings.TrimSpace(r.Zone), "."))
	r.Sub = strings.TrimSpace(r.Sub)
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	r.Value = strings.TrimSpace(r.Value)
	if r.Sub == "" {
		r.Sub = "@"
	}
	if !reZone.MatchString(r.Zone) {
		return fmt.Errorf("主域名 %q 无效", r.Zone)
	}
	if !reSub.MatchString(r.Sub) {
		return fmt.Errorf("主机记录 %q 无效", r.Sub)
	}
	if !validTypes[r.Type] {
		return fmt.Errorf("不支持的记录类型 %q", r.Type)
	}
	if r.Value == "" {
		switch r.Type {
		case "A":
			r.Value = VarIPv4
		case "AAAA":
			r.Value = VarIPv6
		default:
			return errors.New("记录值不能为空")
		}
	}
	// 不含变量的 A/AAAA 记录值需要是合法地址
	if !strings.Contains(r.Value, "{") {
		if r.Type == "A" || r.Type == "AAAA" {
			a, err := netip.ParseAddr(r.Value)
			if err != nil || (r.Type == "A") != a.Is4() {
				return fmt.Errorf("记录值 %q 不是有效的 %s 地址", r.Value, r.Type)
			}
		}
	}
	if r.TTL < 0 {
		return errors.New("TTL 不能为负数")
	}
	return nil
}
