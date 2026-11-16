package webservice

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/crud"
)

// 站点用户（Site.Users，每项 用户名:密码）的保存与输出：保存时明文密码转为 bcrypt 哈希，接口只返回掩码。
// 存量配置中的明文照常可用（verifyUser 两种都能比较），下次保存所在监听时转换。

// siteUserCost 为站点用户密码的 bcrypt 成本，与共享用户相同；测试中调低以加快速度。
var siteUserCost = bcrypt.DefaultCost

// maxBcryptPassword 为 bcrypt 能处理的最大密码字节数。
const maxBcryptPassword = 72

// hashSiteUsers 把站点用户的明文密码转为 bcrypt 哈希；已是有效哈希的保持不变（再次保存不会改变，会话凭据指纹也不变）。
// 密码为空或仍是掩码（没有可沿用的原密码）、哈希无效、超过 bcrypt 上限时返回错误。
func hashSiteUsers(users []string) ([]string, error) {
	if len(users) == 0 {
		return users, nil
	}
	out := make([]string, 0, len(users))
	for _, u := range users {
		name, pass, _ := strings.Cut(u, ":")
		switch {
		case pass == "" || pass == crud.SecretMask:
			return nil, fmt.Errorf("请为站点用户 %s 设置密码", name)
		case strings.HasPrefix(pass, "$2"):
			if _, err := bcrypt.Cost([]byte(pass)); err != nil {
				return nil, fmt.Errorf("站点用户 %s 的密码哈希无效", name)
			}
		case len(pass) > maxBcryptPassword:
			return nil, fmt.Errorf("站点用户 %s 的密码不能超过 %d 字节", name, maxBcryptPassword)
		default:
			h, err := bcrypt.GenerateFromPassword([]byte(pass), siteUserCost)
			if err != nil {
				return nil, err
			}
			pass = string(h)
		}
		out = append(out, name+":"+pass)
	}
	return out, nil
}

// maskSiteUsers 返回用于接口输出的站点用户：密码一律显示为掩码。
func maskSiteUsers(users []string) []string {
	if users == nil {
		return nil
	}
	out := make([]string, len(users))
	for i, u := range users {
		name, pass, ok := strings.Cut(u, ":")
		if ok && pass != "" {
			u = name + ":" + crud.SecretMask
		}
		out[i] = u
	}
	return out
}

// maskListenerUsers 返回站点用户密码被掩码的监听副本（不修改原配置）。
func maskListenerUsers(l Listener) Listener {
	if len(l.Sites) > 0 {
		sites := make([]Site, len(l.Sites))
		for i, s := range l.Sites {
			s.Users = maskSiteUsers(s.Users)
			sites[i] = s
		}
		l.Sites = sites
	}
	l.Default.Users = maskSiteUsers(l.Default.Users)
	return l
}

// siteUserEntry 是拆开的站点用户条目，用于按用户名沿用原密码。
type siteUserEntry struct {
	name, pass string
}

func splitSiteUsers(users []string) []siteUserEntry {
	out := make([]siteUserEntry, len(users))
	for i, u := range users {
		name, pass, _ := strings.Cut(strings.TrimSpace(u), ":")
		out[i] = siteUserEntry{name: name, pass: pass}
	}
	return out
}

// keepSiteUsers 把 updated 中密码为掩码或留空的用户恢复为 old 中同名用户（改名时按位置，规则见 crud.KeepSecrets）的密码。
func keepSiteUsers(old, updated []string) []string {
	if len(updated) == 0 {
		return updated
	}
	entries := splitSiteUsers(updated)
	for i := range entries {
		// 留空与掩码同样表示不修改
		if entries[i].pass == "" {
			entries[i].pass = crud.SecretMask
		}
	}
	crud.KeepSecrets(splitSiteUsers(old), entries, func(e *siteUserEntry) string { return e.name }, func(e *siteUserEntry) *string { return &e.pass })
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.name + ":" + e.pass
	}
	// 仍为空的保持原样（交给校验报错），未提交冒号的条目同样原样保留
	for i, u := range updated {
		if !strings.Contains(u, ":") {
			out[i] = u
		}
	}
	return out
}

// restoreListenerUsers 沿用 old（同一监听已保存的配置，可为 nil）中各站点用户的密码：按站点 key 对应，
// 默认站点对应默认站点。仍为掩码的条目（例如在界面中复制出的新站点）再按用户名在全部已保存的站点中查找，
// 只有一个不同的密码时沿用它。调用方需持有模块锁。
func (m *Module) restoreListenerUsers(old *Listener, l *Listener) {
	byKey := map[string][]string{}
	if old != nil {
		for _, s := range old.Sites {
			if s.Key != "" {
				byKey[s.Key] = s.Users
			}
		}
	}
	restore := func(s *Site, prev []string, hasPrev bool) {
		if len(s.Users) == 0 {
			return
		}
		if hasPrev {
			s.Users = keepSiteUsers(prev, s.Users)
		}
		s.Users = m.resolveMaskedUsers(s.Users)
	}
	for i := range l.Sites {
		prev, ok := byKey[l.Sites[i].Key]
		restore(&l.Sites[i], prev, ok && l.Sites[i].Key != "")
	}
	if old != nil {
		restore(&l.Default, old.Default.Users, true)
	} else {
		restore(&l.Default, nil, false)
	}
}

// resolveMaskedUsers 把仍为掩码的条目按用户名在全部已保存的站点中查找原密码，只有一个不同的密码时沿用。
func (m *Module) resolveMaskedUsers(users []string) []string {
	var out []string
	for i, u := range users {
		name, pass, _ := strings.Cut(strings.TrimSpace(u), ":")
		if pass != crud.SecretMask {
			continue
		}
		found := ""
		ambiguous := false
		check := func(s Site) {
			for _, ou := range s.Users {
				on, op, _ := strings.Cut(ou, ":")
				if on != name || op == "" || op == crud.SecretMask {
					continue
				}
				if found != "" && found != op {
					ambiguous = true
				}
				found = op
			}
		}
		for _, l := range m.cfg.Listeners {
			for _, s := range l.Sites {
				check(s)
			}
			check(l.Default)
		}
		if found == "" || ambiguous {
			continue
		}
		if out == nil {
			out = append([]string(nil), users...)
		}
		out[i] = name + ":" + found
	}
	if out == nil {
		return users
	}
	return out
}

// findListenerLocked 返回已保存的同 key 监听，key 为空或不存在时返回 nil。调用方需持有模块锁。
func (m *Module) findListenerLocked(key string) *Listener {
	if key == "" {
		return nil
	}
	for i := range m.cfg.Listeners {
		if m.cfg.Listeners[i].Key == key {
			return &m.cfg.Listeners[i]
		}
	}
	return nil
}
