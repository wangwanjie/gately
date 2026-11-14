package frp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// 代理导入方式。
const (
	// ProxyAppend 只追加名称不存在的代理，同名代理保持不变
	ProxyAppend = "append"
	// ProxyReplace 用导入的代理替换现有全部代理
	ProxyReplace = "replace"
)

// ImportRequest 是导入请求。Apply 为 false 时只返回解析预览，不保存。
type ImportRequest struct {
	// Role 为 client（frpc）或 server（frps）
	Role    string `json:"role"`
	Content string `json:"content"`
	Apply   bool   `json:"apply"`
	// ProxyMode 为 append 或 replace，仅导入到已有 frpc 实例时有效，留空等同 append
	ProxyMode string `json:"proxyMode"`
	// Target 为导入到的已有实例 key，留空时新建一个停用的实例
	Target string `json:"target"`
	// Name 为新建实例的名称，留空时使用服务端地址（frpc）或 frps
	Name string `json:"name"`
}

// ImportResponse 是导入响应，密钥均已脱敏。
type ImportResponse struct {
	Applied  bool            `json:"applied"`
	Role     string          `json:"role"`
	Client   *ClientSettings `json:"client,omitempty"`
	Server   *ServerSettings `json:"server,omitempty"`
	Proxies  []Proxy         `json:"proxies"`
	Warnings []string        `json:"warnings"`
	// Added 为实际新增的代理数量
	Added int `json:"added"`
	// Key 为导入到的实例 key，仅 Applied 为 true 时有值
	Key string `json:"key,omitempty"`
}

func (m *Module) registerImport(r module.Router) {
	module.DocOf(r, "POST /import", module.Doc{
		Summary: "导入已有的 frpc / frps 配置（支持旧版 ini 与新版 toml）；apply 为 false 时仅预览",
		Req:     &ImportRequest{},
		Resp:    &ImportResponse{},
	})
	r.HandleFunc("POST /import", func(w http.ResponseWriter, req *http.Request) {
		var in ImportRequest
		if err := httpx.ReadJSON(req, &in); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if in.ProxyMode != "" && in.ProxyMode != ProxyAppend && in.ProxyMode != ProxyReplace {
			httpx.Error(w, http.StatusBadRequest, "不支持的代理导入方式 %q", in.ProxyMode)
			return
		}
		res, err := ParseImport(in.Role, in.Content)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		out := &ImportResponse{Role: res.Role, Proxies: res.Proxies, Warnings: res.Warnings}
		m.mu.Lock()
		defer m.mu.Unlock()
		if in.Apply {
			if err := m.applyImportLocked(res, in, out); err != nil {
				code := http.StatusInternalServerError
				if errors.Is(err, errTargetNotFound) || errors.Is(err, errInvalidImport) {
					code = http.StatusBadRequest
				}
				httpx.Error(w, code, "%v", err)
				return
			}
			out.Applied = true
		}
		if res.Client != nil {
			c := *res.Client
			c.Token = crud.Mask(c.Token)
			out.Client = &c
		}
		if res.Server != nil {
			s := *res.Server
			s.Token, s.DashboardPwd = crud.Mask(s.Token), crud.Mask(s.DashboardPwd)
			out.Server = &s
		}
		masked := make([]Proxy, len(out.Proxies))
		for i, p := range out.Proxies {
			p.SecretKey = crud.Mask(p.SecretKey)
			masked[i] = p
		}
		out.Proxies = masked
		httpx.OK(w, out)
	})
}

// applyImportLocked 保存导入结果并重启对应实例；调用方持有 mu。
// 导入到已有实例时沿用其名称与启用状态；新建的实例默认停用，避免与正在运行的同名连接冲突。
func (m *Module) applyImportLocked(res *ImportResult, in ImportRequest, out *ImportResponse) error {
	oldCfg := m.cfg
	// 复制切片再修改，保存失败时 oldCfg 不受影响
	m.cfg.Clients = append([]Client(nil), m.cfg.Clients...)
	m.cfg.Servers = append([]Server(nil), m.cfg.Servers...)
	m.cfg.Proxies = append([]Proxy(nil), m.cfg.Proxies...)
	name := strings.TrimSpace(in.Name)
	switch res.Role {
	case RoleClient:
		var c *Client
		if in.Target != "" {
			if c = m.findClient(in.Target); c == nil {
				m.cfg = oldCfg
				return errTargetNotFound
			}
			c.ClientSettings = *res.Client
			if in.ProxyMode == ProxyReplace {
				kept := m.cfg.Proxies[:0]
				for _, p := range m.cfg.Proxies {
					if p.Client != c.Key {
						kept = append(kept, p)
					}
				}
				m.cfg.Proxies = kept
			}
		} else {
			if name == "" {
				name = res.Client.ServerAddr
			}
			if name == "" {
				name = binClient
			}
			m.cfg.Clients = append(m.cfg.Clients, Client{Key: crud.NewKey(), Name: name, ClientSettings: *res.Client})
			c = &m.cfg.Clients[len(m.cfg.Clients)-1]
		}
		out.Key = c.Key
		// 导入到已启用的实例时，导入的设置同样要通过校验（如缺少服务端地址），避免保存出编辑时无法通过校验的状态
		if err := c.Validate(); err != nil {
			m.cfg = oldCfg
			return fmt.Errorf("%w: %v", errInvalidImport, err)
		}
		exists := map[string]bool{}
		for _, p := range m.cfg.Proxies {
			if p.Client == c.Key {
				exists[p.Name] = true
			}
		}
		var skipped []string
		for _, p := range res.Proxies {
			if exists[p.Name] {
				skipped = append(skipped, p.Name)
				continue
			}
			exists[p.Name] = true
			p.Key, p.Client = crud.NewKey(), c.Key
			m.cfg.Proxies = append(m.cfg.Proxies, p)
			out.Added++
		}
		if len(skipped) > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("已存在同名代理，保持原样：%s", strings.Join(skipped, "、")))
		}
	case RoleServer:
		if in.Target != "" {
			s := m.findServer(in.Target)
			if s == nil {
				m.cfg = oldCfg
				return errTargetNotFound
			}
			old := s.ServerSettings
			s.ServerSettings = *res.Server
			// 导入的 frps 配置不含 Gately 的来源 IP 访问控制，沿用实例原有设置
			s.IPMode, s.IPList, s.IPSets = old.IPMode, old.IPList, old.IPSets
			s.IPDenyList, s.IPDenySets = old.IPDenyList, old.IPDenySets
			out.Key = s.Key
			if err := s.Validate(); err != nil {
				m.cfg = oldCfg
				return fmt.Errorf("%w: %v", errInvalidImport, err)
			}
		} else {
			if name == "" {
				name = binServer
			}
			s := Server{Key: crud.NewKey(), Name: name, ServerSettings: *res.Server}
			m.cfg.Servers = append(m.cfg.Servers, s)
			out.Key = s.Key
		}
	}
	if err := m.saveLocked(); err != nil {
		m.cfg = oldCfg
		return fmt.Errorf("保存失败: %v", err)
	}
	if res.Role == RoleClient {
		m.applyClientLocked(out.Key)
	} else {
		m.applyServerLocked(out.Key)
	}
	return nil
}

// errTargetNotFound 表示导入目标实例不存在。
var errTargetNotFound = errors.New("要导入到的实例不存在")

// errInvalidImport 表示导入后的实例设置未通过校验。
var errInvalidImport = errors.New("导入的设置无效")
