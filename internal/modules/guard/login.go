package guard

import (
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// 管理后台登录成功的地址在一段时间内免于自动封禁：登录要经过 TCP 握手与密码（或 Passkey、飞牛登录态）验证，
// 地址确实属于管理员。这样即使有人设法让入侵防护针对这个地址（例如诱导共用出口的设备触发规则），
// 管理员也不会被自己的网关挡在门外。手动封禁不受影响。
var (
	// loginTrustTTL 为免封时长，可在测试中调整
	loginTrustTTL = 24 * time.Hour
	// loginNow 为当前时间，可在测试中替换
	loginNow = time.Now
)

// loginTrustMax 为同时免封的地址数上限，超出时去掉最早到期的
const loginTrustMax = 64

// loginTrustOwner 为在 trustsrc 中登记的名字：登记后这些地址也加入防火墙的放行集合，诱饵端口与扫描检测不处理它们
const loginTrustOwner = "guard-login"

// loginTrust 记录最近登录成功的地址及免封截止时间。
type loginTrust struct {
	mu    sync.Mutex
	until map[netip.Addr]time.Time
}

// note 记录一次登录成功，返回是否是新加入的地址。
func (l *loginTrust) note(a netip.Addr) bool {
	now := loginNow()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.until == nil {
		l.until = map[netip.Addr]time.Time{}
	}
	l.pruneLocked(now)
	_, had := l.until[a]
	l.until[a] = now.Add(loginTrustTTL)
	if len(l.until) > loginTrustMax {
		// 去掉最早到期的地址
		keys := make([]netip.Addr, 0, len(l.until))
		for k := range l.until {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return l.until[keys[i]].Before(l.until[keys[j]]) })
		for _, k := range keys[:len(keys)-loginTrustMax] {
			delete(l.until, k)
		}
	}
	l.publishLocked()
	return !had
}

func (l *loginTrust) pruneLocked(now time.Time) {
	for k, t := range l.until {
		if !now.Before(t) {
			delete(l.until, k)
		}
	}
}

// publishLocked 把当前免封地址登记到 trustsrc，防火墙据此放行。调用方持有 l.mu。
func (l *loginTrust) publishLocked() {
	list := make([]trustsrc.Source, 0, len(l.until))
	for a, t := range l.until {
		list = append(list, trustsrc.Source{Addr: a, Until: t})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr.Less(list[j].Addr) })
	trustsrc.Set(loginTrustOwner, list)
}

// overlaps 判断将被封禁的网段（IPv6 为 /64）是否包含仍在免封期内的登录地址。
func (l *loginTrust) overlaps(p netip.Prefix) bool {
	now := loginNow()
	l.mu.Lock()
	defer l.mu.Unlock()
	for a, t := range l.until {
		if now.Before(t) && p.Contains(a) {
			return true
		}
	}
	return false
}

// clear 清空记录并撤销 trustsrc 登记，模块停止时调用。
func (l *loginTrust) clear() {
	l.mu.Lock()
	l.until = nil
	l.mu.Unlock()
	trustsrc.Clear(loginTrustOwner)
}

// noteLogin 处理管理后台登录成功事件：地址在 loginTrustTTL 内免于自动封禁。
func (m *Module) noteLogin(e events.Event) {
	a, ok := parseAddr(e.Fields["ip"])
	if !ok || !a.IsValid() || a.IsUnspecified() {
		return
	}
	// 持有 m.mu 判断模块仍在运行，避免与 Stop 交错时在清空之后又登记
	m.mu.Lock()
	added := m.started && m.logins.note(a)
	m.mu.Unlock()
	if added {
		m.logs.Logger("").Infof("管理后台登录成功的地址 %s 在 %d 小时内免于自动封禁", banlist.KeyFor(a), int(loginTrustTTL/time.Hour))
	}
}

// ignoreFunc 返回封禁表使用的白名单判断：设置中的白名单（含内网与本机 IPv6 网段）加上最近登录成功的地址。
func (m *Module) ignoreFunc(s Settings, local []netip.Prefix) func(netip.Prefix) bool {
	set := s.ignoreSet(local)
	return func(p netip.Prefix) bool {
		return set.overlaps(p) || m.logins.overlaps(p)
	}
}
