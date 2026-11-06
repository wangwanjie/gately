// Package fwopen 在系统防火墙中自动放行 Gately 自身监听的端口（STUN 穿透、Web 服务监听、端口转发与协议分流使用）。
//
// 做法是在宿主机已有防火墙的 input 链最前面插入带标记注释的放行规则：
//   - nftables：所有 filter 类型、挂在 input 钩子上的 ip / inet 基础链（OpenWrt fw4、firewalld 等），
//     不同表的基础链各自独立判定，必须插在会丢弃流量的那条链里才有效，所以不能只写 Gately 自己的表；
//   - nftables 的 ip6 表与不限定地址族的 inet 表同样处理，IPv6 端口也能放行；
//   - iptables / ip6tables：filter 表的 INPUT 链（OpenWrt fw3、ufw 等）。iptables-nft 的表交给对应命令处理。
//
// 每条规则只放行一个端口或一段连续端口，可限定地址族与目标地址（监听绑定了具体 IP 时），
// 不会放开整个区域或其他端口。
//
// 防火墙重载（如 fw4 reload）会清掉插入的规则，管理器每 30 秒对账一次并补回；
// 不再需要的端口按注释标记删除，进程重启后遗留的规则也会在下次对账时清理。
package fwopen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// commentPrefix 标记 Gately 插入的放行规则，只删除带此前缀的规则。
const commentPrefix = "gately-open:"

// instance 区分同一主机（同一网络命名空间）上的多个 Gately 实例：注释为 前缀 + 实例 + ":" + owner + …，
// 对账只处理本实例的规则，避免多个实例互删放行规则。由 SetInstance 按配置目录生成。
var (
	instMu   sync.RWMutex
	instance = "default"
)

// SetInstance 按配置目录设置本实例标识（目录绝对路径的短哈希），应在首次放行前调用。
func SetInstance(dir string) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	sum := sha256.Sum256([]byte(dir))
	instMu.Lock()
	instance = hex.EncodeToString(sum[:4])
	instMu.Unlock()
}

// setupOnce 保证实例标识与启动清理只做一次：多个模块都会调用 Setup。
var setupOnce sync.Once

// Setup 按配置目录设置实例标识，并在后台清理上次运行遗留的规则（例如进程被强制结束）。
// 使用自动放行的模块在 Init 中调用，只有第一次调用生效；之后启动的规则会重新放行。
func Setup(dir string) {
	setupOnce.Do(func() {
		SetInstance(dir)
		go Sweep(context.Background())
	})
}

// inContainer 判断是否运行在容器中，用于提示 bridge 网络需要在宿主机放行。
func inContainer() bool {
	for _, f := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

// ownPrefix 返回本实例规则注释的前缀。
func ownPrefix() string {
	instMu.RLock()
	defer instMu.RUnlock()
	return commentPrefix + instance + ":"
}

// reconcileInterval 为定时对账周期。
const reconcileInterval = 30 * time.Second

// Port 是需要放行的一个端口或一段连续端口。
type Port struct {
	// Proto 为 tcp 或 udp
	Proto string
	Port  int
	// End 大于 Port 时表示放行 Port～End 的连续端口，否则只放行 Port
	End int
	// Family 为 4（只放行 IPv4）、6（只放行 IPv6），空表示两者都放行；Addr 非空时以 Addr 的地址族为准
	Family string
	// Addr 非空时只放行发往该地址的连接（监听绑定了具体 IP 时使用），不含端口与 IPv6 区域标识
	Addr string
}

// last 返回区间的最后一个端口。
func (p Port) last() int {
	if p.End > p.Port {
		return p.End
	}
	return p.Port
}

// portText 为端口或端口区间的文字形式，如 443、8000-8010。
func (p Port) portText() string {
	if p.last() > p.Port {
		return strconv.Itoa(p.Port) + "-" + strconv.Itoa(p.last())
	}
	return strconv.Itoa(p.Port)
}

func (p Port) String() string {
	s := strings.ToUpper(p.Proto) + " "
	switch {
	case p.Addr != "" && strings.Contains(p.Addr, ":"):
		s += "[" + p.Addr + "]:" + p.portText()
	case p.Addr != "":
		s += p.Addr + ":" + p.portText()
	default:
		s += p.portText()
		if p.Family == "4" {
			s += " (IPv4)"
		} else if p.Family == "6" {
			s += " (IPv6)"
		}
	}
	return s
}

// v4 / v6 表示该端口是否需要放行对应地址族。
func (p Port) v4() bool {
	return p.Family != "6"
}

func (p Port) v6() bool {
	return p.Family != "4"
}

// key 是端口在注释中的标识：协议 + 地址族 + 端口（区间）+ 目标地址摘要，不含冒号，便于从注释中拆出 owner。
func (p Port) key() string {
	k := p.Proto + p.Family + "-" + p.portText()
	if p.Addr != "" {
		sum := sha256.Sum256([]byte(p.Addr))
		k += "-" + hex.EncodeToString(sum[:4])
	}
	return k
}

// target 是一条可插入规则的防火墙链。
type target struct {
	// Kind 为 nft、iptables 或 ip6tables
	Kind   string
	Family string
	Table  string
	Chain  string
	// Policy 为链的默认策略（小写），Busy 表示链中有 Gately 以外的规则；两者都不会丢弃流量时无需放行
	Policy string
	Busy   bool
}

func (t target) String() string {
	if t.Kind == "iptables" || t.Kind == "ip6tables" {
		return t.Kind + " " + t.Chain
	}
	return "nftables " + t.Family + " " + t.Table + " " + t.Chain
}

// accepts 判断端口是否需要写入这条链：只处理 IPv4 的链（ip 表、iptables）跳过只放行 IPv6 的端口，反之亦然。
func (t target) accepts(p Port) bool {
	switch {
	case t.Kind == "ip6tables" || t.Family == "ip6":
		return p.v6()
	case t.Kind == "iptables" || t.Family == "ip":
		return p.v4()
	}
	return true
}

// needed 判断这条链是否可能丢弃入站流量。
func (t target) needed() bool {
	return t.Policy == "drop" || t.Busy
}

// existing 是链中一条 Gately 放行规则：Ref 为删除时使用的句柄（nft）或规则参数（iptables）。
type existing struct {
	Comment string
	Ref     string
}

// backend 封装具体防火墙的读写，测试时替换为假实现。
type backend interface {
	// targets 返回可插入规则的链及其中已有的 Gately 规则
	targets(ctx context.Context) ([]target, map[string][]existing, error)
	add(ctx context.Context, t target, comment string, p Port) error
	del(ctx context.Context, t target, e existing) error
}

// errUnsupported 由不支持的平台返回。
type errUnsupported struct{}

func (errUnsupported) Error() string { return "自动放行防火墙只支持 Linux" }

type ownerState struct {
	ports []Port
	log   *logbuf.Logger
	// status 为最近一次对账结果说明，ok 表示全部放行成功
	status string
	ok     bool
}

type manager struct {
	// mu 保护 owners；run 串行化对账，避免并发写防火墙
	mu      sync.Mutex
	run     sync.Mutex
	be      backend
	owners  map[string]*ownerState
	looping bool
}

var std = &manager{be: newBackend(), owners: map[string]*ownerState{}}

// Set 登记 owner 需要放行的端口并立即对账，返回结果说明与是否全部放行成功。
// log 用于记录之后定时对账中的补回与失败，可为 nil。ports 为空时等同 Clear。
func Set(owner string, ports []Port, log *logbuf.Logger) (string, bool) {
	return std.set(safeOwner(owner), ports, log)
}

// Clear 取消 owner 的放行并删除对应规则。
func Clear(owner string) {
	std.clear(safeOwner(owner))
}

// Status 返回 owner 最近一次对账的结果说明，未登记时为空。
func Status(owner string) string {
	return std.status(safeOwner(owner))
}

// safeOwnerRe 为可以原样写入规则注释的 owner 字符。
var safeOwnerRe = regexp.MustCompile(`^[A-Za-z0-9_.:\-]{1,48}$`)

// safeOwner 把 owner 限制为安全字符：含其他字符或过长时改用摘要，避免破坏 nft / iptables 命令与注释长度上限。
func safeOwner(owner string) string {
	if safeOwnerRe.MatchString(owner) {
		return owner
	}
	sum := sha256.Sum256([]byte(owner))
	return "h-" + hex.EncodeToString(sum[:8])
}

// failed 判断 owner 最近一次对账是否有端口放行失败，未登记时为 false。
func (m *manager) failed(owner string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.owners[owner]; o != nil {
		return o.status != "" && !o.ok
	}
	return false
}

func (m *manager) status(owner string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.owners[owner]; o != nil {
		return o.status
	}
	return ""
}

// Sweep 对账一次，删除上次运行遗留的规则（例如进程被强制结束）。没有找到防火墙或没有权限时静默返回。
func Sweep(ctx context.Context) {
	std.reconcile(ctx, false)
}

func (m *manager) set(owner string, ports []Port, log *logbuf.Logger) (string, bool) {
	ports = normalize(ports)
	if len(ports) == 0 {
		m.clear(owner)
		return "", true
	}
	m.mu.Lock()
	o := m.owners[owner]
	if o == nil {
		o = &ownerState{}
		m.owners[owner] = o
	}
	o.ports, o.log = ports, log
	start := !m.looping
	m.looping = true
	m.mu.Unlock()
	if start {
		go m.loop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m.reconcile(ctx, false)
	m.mu.Lock()
	defer m.mu.Unlock()
	if o := m.owners[owner]; o != nil {
		return o.status, o.ok
	}
	return "", false
}

func (m *manager) clear(owner string) {
	m.mu.Lock()
	_, ok := m.owners[owner]
	delete(m.owners, owner)
	m.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m.reconcile(ctx, false)
}

// loop 定时对账，没有登记的端口时退出。
func (m *manager) loop() {
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()
	for range t.C {
		m.mu.Lock()
		if len(m.owners) == 0 {
			m.looping = false
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		m.reconcile(ctx, true)
		cancel()
	}
}

// normalize 规范化、去重并排序，过滤无效端口与地址。
func normalize(in []Port) []Port {
	seen := map[Port]bool{}
	var out []Port
	for _, p := range in {
		p.Proto = strings.ToLower(p.Proto)
		if p.Port <= 0 || p.Port > 65535 || (p.Proto != "tcp" && p.Proto != "udp") {
			continue
		}
		if p.End <= p.Port || p.End > 65535 {
			p.End = 0
		}
		if p.Addr != "" {
			ip := net.ParseIP(p.Addr)
			if ip == nil {
				// 地址无效时不放行，避免误放行到不受限制的规则
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				p.Addr, p.Family = ip4.String(), "4"
			} else {
				p.Addr, p.Family = ip.String(), "6"
			}
		}
		if p.Family != "4" && p.Family != "6" {
			p.Family = ""
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].key() < out[j].key()
	})
	return out
}

// comment 生成规则注释：前缀 + 实例 + owner + 端口标识，对账时按注释判断规则是否仍需要。
func comment(owner string, p Port) string {
	return ownPrefix() + owner + ":" + p.key()
}

// ownerOf 从注释中取出 owner，非本实例的注释（其他程序或其他 Gately 实例）返回 false。
// 旧版注释（…:协议:端口）解析出的 owner 带协议后缀，不会与任何登记匹配，对账时按遗留规则删除并以新格式补回。
func ownerOf(c string) (string, bool) {
	prefix := ownPrefix()
	if !strings.HasPrefix(c, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(c, prefix)
	// 去掉末尾的 :端口标识
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		return "", false
	}
	return rest[:idx], true
}

// reconcile 让防火墙中的 Gately 规则与登记一致：删除多余的，补上缺少的，并更新每个 owner 的状态说明。
// periodic 为 true 时把补回与新出现的失败写入 owner 的日志。
func (m *manager) reconcile(ctx context.Context, periodic bool) {
	m.run.Lock()
	defer m.run.Unlock()

	m.mu.Lock()
	want := map[string][]Port{}
	for k, o := range m.owners {
		want[k] = append([]Port{}, o.ports...)
	}
	m.mu.Unlock()

	targets, have, err := m.be.targets(ctx)
	results := map[string]*ownerResult{}
	for owner, ports := range want {
		results[owner] = &ownerResult{ports: ports}
	}
	if err != nil {
		for _, r := range results {
			r.fail = append(r.fail, err.Error())
		}
		m.publish(results, periodic)
		return
	}
	for _, t := range targets {
		key := t.String()
		present := map[string]bool{}
		for _, e := range have[key] {
			owner, mine := ownerOf(e.Comment)
			if !mine {
				// 其他 Gately 实例插入的规则由它自己维护，不动
				present[e.Comment] = true
				continue
			}
			ports, ok := want[owner]
			keep := ok && t.needed() && hasComment(owner, ports, t, e.Comment) && !present[e.Comment]
			if keep {
				present[e.Comment] = true
				continue
			}
			if derr := m.be.del(ctx, t, e); derr != nil && results[owner] != nil {
				results[owner].fail = append(results[owner].fail, fmt.Sprintf("%s: %v", t, derr))
			}
		}
		if !t.needed() {
			continue
		}
		for owner, ports := range want {
			r := results[owner]
			okAll := true
			used := false
			for _, p := range ports {
				if !t.accepts(p) {
					continue
				}
				used = true
				c := comment(owner, p)
				if present[c] {
					continue
				}
				if aerr := m.be.add(ctx, t, c, p); aerr != nil {
					okAll = false
					r.fail = append(r.fail, fmt.Sprintf("%s: %v", t, aerr))
					continue
				}
				r.added++
			}
			if okAll && used {
				r.targets = append(r.targets, key)
			}
		}
	}
	m.publish(results, periodic)
}

type ownerResult struct {
	ports   []Port
	targets []string
	fail    []string
	added   int
}

// hasComment 判断注释是否对应 owner 在这条链上仍需要的某个端口。
func hasComment(owner string, ports []Port, t target, c string) bool {
	for _, p := range ports {
		if t.accepts(p) && comment(owner, p) == c {
			return true
		}
	}
	return false
}

// publish 生成每个 owner 的状态说明，定时对账时记录补回与失败。
func (m *manager) publish(results map[string]*ownerResult, periodic bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for owner, r := range results {
		o := m.owners[owner]
		if o == nil {
			continue
		}
		status, ok := describe(r)
		if periodic && o.log != nil {
			switch {
			case !ok && status != o.status:
				o.log.Warnf("%s", status)
			case ok && r.added > 0:
				o.log.Infof("防火墙规则被清除（例如防火墙重新加载），已重新放行：%s", status)
			}
		}
		o.status, o.ok = status, ok
	}
}

func describe(r *ownerResult) (string, bool) {
	var ports []string
	for _, p := range r.ports {
		ports = append(ports, p.String())
	}
	portText := strings.Join(ports, "、")
	if len(r.fail) > 0 {
		return fmt.Sprintf("放行防火墙端口 %s 失败：%s", portText, strings.Join(r.fail, "；")), false
	}
	if len(r.targets) == 0 {
		if inContainer() {
			return "容器内没有发现会拦截入站连接的防火墙规则，无需放行；Docker bridge 网络的映射端口需要在宿主机或路由器的防火墙中放行", true
		}
		return "没有发现会拦截入站连接的防火墙规则（nftables / iptables），无需放行", true
	}
	return fmt.Sprintf("已在防火墙放行 %s（%s）", portText, strings.Join(r.targets, "、")), true
}

// ForTesting 把全局实例的后端换成不修改系统防火墙的实现（结果为“只支持 Linux”的失败），
// 供其他包的测试调用：即使测试以 root 身份在 Linux 上运行，也不会改动宿主机的防火墙。
func ForTesting() {
	std.run.Lock()
	defer std.run.Unlock()
	std.be = unsupported{}
}
