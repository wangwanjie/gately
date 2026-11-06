package fwopen

import (
	"slices"
	"sync"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// Group 供按 key 管理多条放行（Web 服务监听、端口转发规则等）的模块使用：
// 调用方在持有自己的锁时用 Want 登记期望状态，立即返回；后台协程按登记顺序串行写防火墙，
// 不会因为执行 nft / iptables 命令阻塞模块的接口，也不会因并发的放行与撤销乱序。
type Group struct {
	prefix string
	// m 为写防火墙的管理器，默认为全局实例，测试时替换
	m *manager

	// op 串行化写防火墙的操作：后台协程每次处理一个 key，ClearAll 等当前操作结束后再撤销
	op sync.Mutex
	mu sync.Mutex
	// want 为期望放行的端口；applied 为已交给 fwopen 的端口，用于判断是否需要重新放行
	want    map[string]groupEntry
	applied map[string][]Port
	// last 为每个 key 上次记录到日志的结果，相同结果不重复记录
	last    map[string]string
	pending map[string]bool
	running bool
}

type groupEntry struct {
	ports []Port
	log   *logbuf.Logger
}

// NewGroup 创建一组放行，prefix 区分模块（如 web、pf），作为规则 owner 的前缀。
func NewGroup(prefix string) *Group {
	return &Group{
		prefix:  prefix,
		m:       std,
		want:    map[string]groupEntry{},
		applied: map[string][]Port{},
		last:    map[string]string{},
		pending: map[string]bool{},
	}
}

func (g *Group) owner(key string) string {
	return safeOwner(g.prefix + ":" + key)
}

// Want 登记 key 期望放行的端口；ports 为空表示撤销。log 用于记录放行结果，可为 nil。
func (g *Group) Want(key string, ports []Port, log *logbuf.Logger) {
	ports = normalize(ports)
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(ports) == 0 {
		if _, ok := g.want[key]; !ok && g.applied[key] == nil {
			return
		}
		delete(g.want, key)
	} else {
		g.want[key] = groupEntry{ports: ports, log: log}
	}
	g.pending[key] = true
	if !g.running {
		g.running = true
		go g.worker()
	}
}

// Status 返回 key 的放行结果说明；已登记但尚未写入防火墙时返回“正在放行”。
func (g *Group) Status(key string) string {
	g.mu.Lock()
	_, wanted := g.want[key]
	pending := g.pending[key]
	g.mu.Unlock()
	if !wanted {
		return ""
	}
	if s := g.m.status(g.owner(key)); s != "" && !pending {
		return s
	}
	return "正在放行防火墙端口"
}

// Failed 判断 key 最近一次放行是否失败（例如没有权限或缺少 nft / iptables 命令），正在放行或未登记时为 false。
func (g *Group) Failed(key string) bool {
	g.mu.Lock()
	_, wanted := g.want[key]
	pending := g.pending[key]
	g.mu.Unlock()
	return wanted && !pending && g.m.failed(g.owner(key))
}

// worker 依次处理待办的 key，直到没有待办后退出。
func (g *Group) worker() {
	for {
		g.mu.Lock()
		var key string
		for k := range g.pending {
			key = k
			break
		}
		if key == "" {
			g.running = false
			g.mu.Unlock()
			return
		}
		g.mu.Unlock()
		g.op.Lock()
		g.mu.Lock()
		if !g.pending[key] {
			// ClearAll 已在等待期间撤销全部放行
			g.mu.Unlock()
			g.op.Unlock()
			continue
		}
		e, wanted := g.want[key]
		prev := g.applied[key]
		g.mu.Unlock()

		var status string
		ok := true
		switch {
		case !wanted:
			g.m.clear(g.owner(key))
		case slices.Equal(prev, e.ports) && g.m.status(g.owner(key)) != "":
			// 端口未变且已在定时对账中维护，无需重复写入
		default:
			status, ok = g.m.set(g.owner(key), e.ports, e.log)
		}

		g.mu.Lock()
		if !wanted {
			delete(g.applied, key)
			delete(g.last, key)
		} else {
			g.applied[key] = e.ports
		}
		// 处理期间又有新的登记时保留待办，下一轮按最新状态处理
		cur, still := g.want[key]
		if still == wanted && (!still || slices.Equal(cur.ports, e.ports)) {
			delete(g.pending, key)
		}
		logIt := wanted && status != "" && status != g.last[key]
		if logIt {
			g.last[key] = status
		}
		g.mu.Unlock()
		g.op.Unlock()

		if logIt && e.log != nil {
			if ok {
				e.log.Infof("%s", status)
			} else {
				e.log.Warnf("%s", status)
			}
		}
	}
}

// ClearAll 同步撤销本组的全部放行，用于模块停止（包括进程退出）时及时关闭端口，
// 不留给下次启动时的清理：Gately 停止后这些端口可能被其他程序监听。
func (g *Group) ClearAll() {
	g.mu.Lock()
	keys := map[string]bool{}
	for k := range g.want {
		keys[k] = true
	}
	for k := range g.applied {
		keys[k] = true
	}
	g.want = map[string]groupEntry{}
	g.pending = map[string]bool{}
	g.mu.Unlock()

	g.op.Lock()
	defer g.op.Unlock()
	g.mu.Lock()
	// 等待期间后台协程可能刚放行了某个 key，一并撤销
	for k := range g.applied {
		keys[k] = true
	}
	g.applied = map[string][]Port{}
	g.last = map[string]string{}
	g.mu.Unlock()
	for k := range keys {
		g.m.clear(g.owner(k))
	}
}
