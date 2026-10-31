package protoroute

import (
	"encoding/json"
	"fmt"

	"github.com/wangwanjie/gately/internal/config"
)

// portForwardID 是端口转发模块的配置名与权限域。这里直接读写它的配置文件，避免两个模块之间的代码依赖。
const portForwardID = "portforward"

// legacyRule 是旧版端口转发中 mode 为 "sni" 的规则：路由在 sniRoutes 字段，其余字段与本模块的规则同名。
type legacyRule struct {
	Rule
	Mode      string  `json:"mode"`
	SNIRoutes []Route `json:"sniRoutes"`
}

// portForwardFile 按原顺序保留端口转发配置的两个顶层字段，规则以原始 JSON 保存，迁移时其余规则原样写回。
type portForwardFile struct {
	Settings json.RawMessage   `json:"settings,omitempty"`
	Rules    []json.RawMessage `json:"rules"`
}

// legacySettings 是端口转发全局设置中与本模块对应的字段。
type legacySettings struct {
	Enable                *bool `json:"enable"`
	DefaultMaxConnections int   `json:"defaultMaxConnections"`
	DialTimeout           int   `json:"dialTimeout"`
}

// migrateFromPortForward 把 portforward.json 中的 SNI 分流规则移到 protoroute.json，返回迁移的规则数。
// 规则的 key、名称、启用状态、分组等配置原样保留；本模块配置文件不存在时，
// 模块开关、默认连接数与连接超时沿用端口转发的全局设置。
// 先写本模块配置再改写端口转发配置：中途失败时下次启动会再迁移一次，按 key 覆盖，不会产生重复规则。
func migrateFromPortForward(store *config.Store) (int, error) {
	var pf portForwardFile
	found, err := store.Load(portForwardID, &pf)
	if err != nil || !found {
		return 0, err
	}
	var keep []json.RawMessage
	var moved []Rule
	for _, raw := range pf.Rules {
		var lr legacyRule
		if err := json.Unmarshal(raw, &lr); err != nil {
			return 0, fmt.Errorf("解析端口转发规则失败: %w", err)
		}
		if lr.Mode != "sni" {
			keep = append(keep, raw)
			continue
		}
		rule := lr.Rule
		rule.Routes = lr.SNIRoutes
		for i := range rule.Routes {
			// 旧版路由只有 SNI 域名，即 TLS 路由
			if rule.Routes[i].Protocol == "" {
				rule.Routes[i].Protocol = ProtoTLS
			}
		}
		moved = append(moved, rule)
	}
	if len(moved) == 0 {
		return 0, nil
	}

	cfg := fileConfig{Settings: defaultSettings()}
	exists, err := store.Load(ID, &cfg)
	if err != nil {
		return 0, err
	}
	if !exists && len(pf.Settings) > 0 {
		var ls legacySettings
		if err := json.Unmarshal(pf.Settings, &ls); err == nil {
			if ls.Enable != nil {
				cfg.Settings.Enable = *ls.Enable
			}
			cfg.Settings.DefaultMaxConnections = ls.DefaultMaxConnections
			cfg.Settings.DialTimeout = ls.DialTimeout
			cfg.Settings.normalize()
		}
	}
	for _, rule := range moved {
		replaced := false
		for i := range cfg.Rules {
			if rule.Key != "" && cfg.Rules[i].Key == rule.Key {
				cfg.Rules[i] = rule
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Rules = append(cfg.Rules, rule)
		}
	}
	if err := store.Save(ID, cfg); err != nil {
		return 0, fmt.Errorf("保存协议分流配置失败: %w", err)
	}
	pf.Rules = keep
	if pf.Rules == nil {
		pf.Rules = []json.RawMessage{}
	}
	if err := store.Save(portForwardID, pf); err != nil {
		return 0, fmt.Errorf("改写端口转发配置失败: %w", err)
	}
	return len(moved), nil
}
