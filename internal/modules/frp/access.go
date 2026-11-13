package frp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
)

// loginPluginName 是写入 frps 配置的登录插件名称。
const loginPluginName = "gately-login"

// serverAccess 是一个 frps 实例编译后的来源 IP 访问控制。
type serverAccess struct {
	mode     string
	prefixes []netip.Prefix
	sets     []string
}

// loginGate 是本机回环地址上的 frps 登录插件端点：frpc 每次登录（frps 校验令牌之前）时，
// frps 把来源地址发到这里，按封禁表与实例的访问控制决定是否接受。
type loginGate struct {
	ln  net.Listener
	srv *http.Server
	// token 是本次运行随机生成的路径令牌，只写入权限 0600 的 frps 配置文件，本机其他程序无法冒用端点
	token string
}

// startLoginGate 在 127.0.0.1 的随机端口启动登录插件端点。
func startLoginGate(m *Module) (*loginGate, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	g := &loginGate{ln: ln, token: hex.EncodeToString(buf)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /frps/{token}/{key}", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.PathValue("token")), []byte(g.token)) != 1 {
			http.NotFound(w, r)
			return
		}
		m.handleLogin(w, r, r.PathValue("key"))
	})
	g.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	go func() {
		_ = g.srv.Serve(ln)
	}()
	return g, nil
}

func (g *loginGate) close() {
	_ = g.srv.Close()
}

// pluginTOML 返回追加到 frps 配置末尾的登录插件段。
func (g *loginGate) pluginTOML(key string) string {
	return fmt.Sprintf("\n[[httpPlugins]]\nname = %s\naddr = %s\npath = %s\nops = [\"Login\", \"NewProxy\"]\n",
		quote(loginPluginName), quote(g.ln.Addr().String()), quote("/frps/"+g.token+"/"+key))
}

// loginRequest 是 frps 插件请求中本模块关心的部分。
type loginRequest struct {
	Op      string `json:"op"`
	Content struct {
		// ClientAddress 为 Login 操作的来源地址
		ClientAddress string `json:"client_address"`
		// ProxyType 与 RemotePort 为 NewProxy 操作中代理的类型与申请的远程端口
		ProxyType  string `json:"proxy_type"`
		RemotePort int    `json:"remote_port"`
	} `json:"content"`
}

// loginResponse 是 frps 插件响应：放行时 unchange 为 true，表示不修改登录内容。
type loginResponse struct {
	Reject       bool   `json:"reject"`
	RejectReason string `json:"reject_reason,omitempty"`
	Unchange     bool   `json:"unchange"`
}

// handleLogin 处理 frps 的 Login 插件请求。只读访问控制快照，不获取模块锁：
// 模块持锁停止 frps 时，frps 可能正等待本端点的响应，持锁会让两边互相等待。
func (m *Module) handleLogin(w http.ResponseWriter, r *http.Request, key string) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp := loginResponse{Unchange: true}
	switch req.Op {
	case "Login":
		if reason := m.checkLogin(key, req.Content.ClientAddress); reason != "" {
			resp = loginResponse{Reject: true, RejectReason: reason}
			m.rejectedCounter(key).Add(1)
		}
	case "NewProxy":
		// 只记录 TCP 代理申请的远程端口，不改变也不拒绝代理；登录已通过令牌与来源检查
		// 只有运行中的实例会学习（见 learnProxyPort），已删除或伪造的实例 key 找不到学习状态
		if req.Content.ProxyType == "tcp" {
			m.learnProxyPort(key, req.Content.RemotePort)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// checkLogin 判断来源能否登录实例 key，返回拒绝原因，放行时为空。
// 封禁优先于名单：封禁的地址即使命中白名单也会被拒绝。拒绝原因写入 frps 日志并返回给 frpc，使用英文与 frp 自身日志一致。
func (m *Module) checkLogin(key, clientAddr string) string {
	var acc *serverAccess
	if rules := m.access.Load(); rules != nil {
		acc = (*rules)[key]
	}
	if acc == nil {
		return "gately: frps instance not found"
	}
	a, ok := parseClientAddr(clientAddr)
	if !ok {
		// 无法得知来源时，白名单按不在名单中处理，其它模式放行
		if acc.mode == iplist.ModeWhitelist {
			return fmt.Sprintf("gately: unknown client address %q", clientAddr)
		}
		return ""
	}
	if banlist.Default.Banned(a) {
		return fmt.Sprintf("gately: client address %s is banned", a)
	}
	if !iplist.Default.Allowed(acc.mode, a, acc.prefixes, acc.sets) {
		if acc.mode == iplist.ModeWhitelist {
			return fmt.Sprintf("gately: client address %s is not in the allowlist", a)
		}
		return fmt.Sprintf("gately: client address %s is in the blocklist", a)
	}
	return ""
}

// parseClientAddr 解析 frps 传来的 host:port 形式的来源地址。
func parseClientAddr(s string) (netip.Addr, bool) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap(), true
	}
	return netip.Addr{}, false
}

// rejectedCounter 返回实例被拒绝登录次数的计数器。
func (m *Module) rejectedCounter(key string) *atomic.Uint64 {
	v, _ := m.rejected.LoadOrStore(key, new(atomic.Uint64))
	return v.(*atomic.Uint64)
}

// rejectedCount 返回实例自 Gately 启动以来被拒绝的登录次数。
func (m *Module) rejectedCount(key string) uint64 {
	if v, ok := m.rejected.Load(key); ok {
		return v.(*atomic.Uint64).Load()
	}
	return 0
}

// publishAccessLocked 编译全部 frps 实例的访问控制并发布快照，同时向 IP 列表注册表登记对订阅列表的引用。
// 调用方持有 mu（或处于 Init）。
func (m *Module) publishAccessLocked() {
	rules := make(map[string]*serverAccess, len(m.cfg.Servers))
	users := map[string][]string{}
	for i := range m.cfg.Servers {
		s := &m.cfg.Servers[i]
		// 配置已校验过，这里解析失败只可能来自手改的配置文件，按空名单处理
		prefixes, _ := iplist.ParsePrefixes(s.IPList)
		rules[s.Key] = &serverAccess{mode: s.IPMode, prefixes: prefixes, sets: s.IPSets}
		if s.IPMode == iplist.ModeOff {
			continue
		}
		for _, k := range s.IPSets {
			users[k] = append(users[k], "frps / "+s.Name)
		}
	}
	m.access.Store(&rules)
	iplist.Default.SetUsers(ID, users)
}

// checkIPSets 检查启用了访问控制的实例引用的订阅列表都存在。
func checkIPSets(s *Server) error {
	if s.IPMode == iplist.ModeOff {
		return nil
	}
	for _, k := range s.IPSets {
		if _, ok := iplist.Default.Name(k); !ok {
			return fmt.Errorf("引用的 IP 列表 %q 不存在", k)
		}
	}
	return nil
}

// accessState 保存访问控制的运行时状态，嵌入 Module。
type accessState struct {
	// gate 为登录插件端点，模块启动时创建；gateErr 为启动失败的原因
	gate    *loginGate
	gateErr string
	// access 为各实例访问控制的只读快照，实例 key -> 规则
	access atomic.Pointer[map[string]*serverAccess]
	// rejected 为各实例被拒绝的登录次数，实例 key -> *atomic.Uint64
	rejected sync.Map
	// learned 为各实例 frpc 申请过的 TCP 代理远程端口（实例 key -> 端口集合），由 learnMu 保护，见 learnProxyPort
	learnMu sync.Mutex
	learned map[string]*learnState
}
