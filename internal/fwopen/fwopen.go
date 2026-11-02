// Package fwopen 在系统防火墙中自动放行 Gately 自身监听的端口（目前供 STUN 穿透使用）。
//
// 做法是在宿主机已有防火墙的 input 链最前面插入带标记注释的放行规则：
//   - nftables：所有 filter 类型、挂在 input 钩子上的 ip / inet 基础链（OpenWrt fw4、firewalld 等），
//     不同表的基础链各自独立判定，必须插在会丢弃流量的那条链里才有效，所以不能只写 Gately 自己的表；
//   - iptables：filter 表的 INPUT 链（OpenWrt fw3、ufw 等）。iptables-nft 的表交给 iptables 命令处理。
//
// 防火墙重载（如 fw4 reload）会清掉插入的规则，管理器每 30 秒对账一次并补回；
// 不再需要的端口按注释标记删除，进程重启后遗留的规则也会在下次对账时清理。
package fwopen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
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

// ownPrefix 返回本实例规则注释的前缀。
func ownPrefix() string {
	instMu.RLock()
	defer instMu.RUnlock()
	return commentPrefix + instance + ":"
}

// reconcileInterval 为定时对账周期。
const reconcileInterval = 30 * time.Second

// Port 是需要放行的一个端口。
type Port struct {
	// Proto 为 tcp 或 udp
	Proto string
	Port  int
}

func (p Port) String() string {
	return strings.ToUpper(p.Proto) + " " + strconv.Itoa(p.Port)
}

// target 是一条可插入规则的防火墙链。
type target struct {
	// Kind 为 nft 或 iptables
	Kind   string
	Family string
	Table  string
	Chain  string
	// Policy 为链的默认策略（小写），Busy 表示链中有 Gately 以外的规则；两者都不会丢弃流量时无需放行
	Policy string
	Busy   bool
}

func (t target) String() string {
	if t.Kind == "iptables" {
		return "iptables " + t.Chain
	}
	return "nftables " + t.Family + " " + t.Table + " " + t.Chain
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
// log 用于记录之后定时对账中的补回与失败，可为 nil。
func Set(owner string, ports []Port, log *logbuf.Logger) (string, bool) {
	return std.set(owner, ports, log)
}

// Clear 取消 owner 的放行并删除对应规则。
func Clear(owner string) {
	std.clear(owner)
}

// Status 返回 owner 最近一次对账的结果说明，未登记时为空。
func Status(owner string) string {
	return std.status(owner)
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

// normalize 去重并排序，过滤无效端口。
func normalize(in []Port) []Port {
	seen := map[Port]bool{}
	var out []Port
	for _, p := range in {
		p.Proto = strings.ToLower(p.Proto)
		if p.Port <= 0 || p.Port > 65535 || (p.Proto != "tcp" && p.Proto != "udp") || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Proto < out[j].Proto
	})
	return out
}

// comment 生成规则注释：前缀 + 实例 + owner + 协议 + 端口，对账时按注释判断规则是否仍需要。
func comment(owner string, p Port) string {
	return ownPrefix() + owner + ":" + p.Proto + ":" + strconv.Itoa(p.Port)
}

// ownerOf 从注释中取出 owner，非本实例的注释（其他程序或其他 Gately 实例）返回 false。
func ownerOf(c string) (string, bool) {
	prefix := ownPrefix()
	if !strings.HasPrefix(c, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(c, prefix)
	// 去掉末尾的 :协议:端口
	for i := 0; i < 2; i++ {
		idx := strings.LastIndex(rest, ":")
		if idx < 0 {
			return "", false
		}
		rest = rest[:idx]
	}
	return rest, true
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
			keep := ok && t.needed() && hasComment(owner, ports, e.Comment) && !present[e.Comment]
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
			for _, p := range ports {
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
			if okAll {
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

func hasComment(owner string, ports []Port, c string) bool {
	for _, p := range ports {
		if comment(owner, p) == c {
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
		return "没有发现会拦截入站连接的防火墙规则（nftables / iptables），无需放行", true
	}
	return fmt.Sprintf("已在防火墙放行 %s（%s）", portText, strings.Join(r.targets, "、")), true
}
