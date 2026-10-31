package guard

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// 规则的日志来源
const (
	// SourceFile 跟踪日志文件新增的内容，Target 为文件路径，每行一个
	SourceFile = "file"
	// SourceJournal 跟踪 systemd 日志，Target 为服务单元名，每行一个
	SourceJournal = "journal"
	// SourceCommand 运行一条持续输出日志的命令（例如 OpenWrt 的 logread -f），Target 为命令
	SourceCommand = "command"
	// SourceDocker 跟踪 Docker 容器的日志，Target 为容器名
	SourceDocker = "docker"
	// SourceEvent 统计 Gately 内部事件（管理后台登录失败、WAF 拦截），Target 为事件类型
	SourceEvent = "event"
	// SourceWeb 统计 Web 服务的请求（路径探测），Target 为限定的监听 key，每行一个，留空表示全部监听；
	// Filter、Ignore 为路径规则，见 compilePaths
	SourceWeb = "web"
)

// eventSources 是可作为规则来源的内部事件，事件中必须带有来源 IP。
var eventSources = map[string]bool{events.AuthFailed: true, events.WAFBlocked: true}

const (
	defaultMaxRetry = 5
	defaultFindTime = 600
	// maxTracked 是每条规则同时跟踪的来源地址上限，超出时清理最早的记录，防止伪造地址撑大内存
	maxTracked = 50000
)

// hostPattern 替换过滤规则中的 <HOST>：匹配 IPv4、IPv6（含 IPv4 映射形式，可带方括号），匹配结果再由 netip 校验。
const hostPattern = `\[?(?P<host>[0-9]{1,3}(?:\.[0-9]{1,3}){3}|[0-9A-Fa-f]{0,4}:[0-9A-Fa-f:.]*[0-9A-Fa-f])\]?`

// Jail 是一条检测规则：在日志中按正则找出失败记录，同一来源在 FindTime 内达到 MaxRetry 次即被封禁。
type Jail struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	// Source 为日志来源，见 Source* 常量
	Source string `json:"source"`
	// Target 为来源参数：文件路径、服务单元、命令、容器名或事件类型
	Target string `json:"target"`
	// Filter 为失败记录的正则，每行一条，用 <HOST> 表示来源地址；内部事件来源可留空
	Filter string `json:"filter"`
	// Ignore 为排除的正则，每行一条，匹配的行不计数
	Ignore string `json:"ignore"`
	// MaxRetry 为触发封禁的失败次数
	MaxRetry int `json:"maxRetry"`
	// FindTime 为统计窗口（秒）
	FindTime int `json:"findTime"`
	// BanTime 为封禁时长（秒），0 表示沿用默认时长，-1 表示永久
	BanTime int `json:"banTime"`
	// AnyStatus 只用于 Web 路径探测：为 true 时任何状态码都计数，默认只统计 404 与 444
	AnyStatus bool `json:"anyStatus,omitempty"`
	// UnknownHost 只用于 Web 路径探测：为 true 时只统计域名未匹配任何站点的请求（默认站点或没有站点）
	UnknownHost bool `json:"unknownHost,omitempty"`
	// Preset 为创建时使用的预设 ID，只用于界面展示
	Preset string `json:"preset,omitempty"`
	Remark string `json:"remark"`
}

// splitLines 按行拆分并去掉空行。
func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// compileFilters 编译过滤规则，把 <HOST> 替换为地址匹配式，并要求每条都能取得来源地址。
func compileFilters(text string, needHost bool) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, l := range splitLines(text) {
		expr := strings.ReplaceAll(l, "<HOST>", hostPattern)
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("正则 %q 无效: %v", l, err)
		}
		if needHost && re.SubexpIndex("host") < 0 {
			return nil, fmt.Errorf("正则 %q 中缺少 <HOST>", l)
		}
		out = append(out, re)
	}
	return out, nil
}

// Validate 校验规则并补全默认值。
func (j *Jail) Validate() error {
	if err := crud.NormalizeGroup(&j.Group); err != nil {
		return err
	}
	j.Name = strings.TrimSpace(j.Name)
	if j.Name == "" {
		return errors.New("名称不能为空")
	}
	j.Target = strings.TrimSpace(j.Target)
	switch j.Source {
	case SourceFile, SourceJournal:
		j.Target = strings.Join(splitLines(j.Target), "\n")
		if j.Target == "" {
			if j.Source == SourceFile {
				return errors.New("请填写日志文件路径")
			}
			return errors.New("请填写服务单元名")
		}
		if j.Source == SourceJournal {
			for _, u := range splitLines(j.Target) {
				if strings.HasPrefix(u, "-") || strings.ContainsAny(u, " \t") {
					return fmt.Errorf("服务单元名 %q 无效", u)
				}
			}
		}
	case SourceCommand:
		if j.Target == "" {
			return errors.New("请填写命令")
		}
		if runtime.GOOS == "windows" {
			return errors.New("Windows 不支持命令来源")
		}
	case SourceDocker:
		if j.Target == "" || strings.ContainsAny(j.Target, " \t\n/") {
			return errors.New("请填写容器名")
		}
	case SourceEvent:
		if !eventSources[j.Target] {
			return fmt.Errorf("不支持的事件类型 %q", j.Target)
		}
	case SourceWeb:
		return j.validateWeb()
	default:
		return fmt.Errorf("未知的日志来源 %q", j.Source)
	}
	filters, err := compileFilters(j.Filter, true)
	if err != nil {
		return err
	}
	if len(filters) == 0 && j.Source != SourceEvent {
		return errors.New("至少填写一条过滤规则")
	}
	j.Filter = strings.Join(splitLines(j.Filter), "\n")
	if _, err := compileFilters(j.Ignore, false); err != nil {
		return err
	}
	j.Ignore = strings.Join(splitLines(j.Ignore), "\n")
	// 任意状态码又不限路径、不限未知域名时每个请求都会计数，正常访客访问几次就会被封
	if j.AnyStatus && j.Filter == "" && !j.UnknownHost {
		return errors.New("统计任意状态码时需要填写路径规则或只统计未知域名，否则正常访问也会被封禁")
	}
	return j.validateCount()
}

// validateCount 校验次数、窗口与封禁时长并补全默认值。
func (j *Jail) validateCount() error {
	if j.MaxRetry == 0 {
		j.MaxRetry = defaultMaxRetry
	}
	if j.MaxRetry < 1 || j.MaxRetry > 1000 {
		return errors.New("失败次数应为 1~1000")
	}
	if j.FindTime == 0 {
		j.FindTime = defaultFindTime
	}
	if j.FindTime < 10 || j.FindTime > 7*24*3600 {
		return errors.New("统计窗口应为 10 秒到 7 天")
	}
	if j.BanTime != 0 && j.BanTime != -1 && (j.BanTime < 60 || j.BanTime > maxBanSeconds) {
		return errors.New("封禁时长应在 1 分钟到 365 天之间")
	}
	return nil
}

// validateWeb 校验 Web 路径探测规则：Target 为监听 key，Filter、Ignore 为路径规则（都可留空）。
func (j *Jail) validateWeb() error {
	j.Target = strings.Join(splitLines(j.Target), "\n")
	for _, l := range splitLines(j.Target) {
		if strings.ContainsAny(l, " \t") {
			return fmt.Errorf("监听 %q 无效", l)
		}
	}
	if _, err := compilePaths(j.Filter); err != nil {
		return err
	}
	j.Filter = strings.Join(splitLines(j.Filter), "\n")
	if _, err := compilePaths(j.Ignore); err != nil {
		return err
	}
	j.Ignore = strings.Join(splitLines(j.Ignore), "\n")
	// 任意状态码又不限路径、不限未知域名时每个请求都会计数，正常访客访问几次就会被封
	if j.AnyStatus && j.Filter == "" && !j.UnknownHost {
		return errors.New("统计任意状态码时需要填写路径规则或只统计未知域名，否则正常访问也会被封禁")
	}
	return j.validateCount()
}

// matcher 是编译后的过滤规则。
type matcher struct {
	filters []*regexp.Regexp
	ignores []*regexp.Regexp
	// web 为 Web 路径探测规则，其他来源为 nil
	web *webMatcher
}

func newMatcher(j Jail) (*matcher, error) {
	if j.Source == SourceWeb {
		w, err := newWebMatcher(j)
		if err != nil {
			return nil, err
		}
		return &matcher{web: w}, nil
	}
	f, err := compileFilters(j.Filter, true)
	if err != nil {
		return nil, err
	}
	ig, err := compileFilters(j.Ignore, false)
	if err != nil {
		return nil, err
	}
	return &matcher{filters: f, ignores: ig}, nil
}

// match 返回日志行中的来源地址；不匹配、被排除或地址无效时 ok 为 false。
func (m *matcher) match(line string) (a netip.Addr, ok bool) {
	for _, re := range m.filters {
		sub := re.FindStringSubmatch(line)
		if sub == nil {
			continue
		}
		host := strings.Trim(sub[re.SubexpIndex("host")], "[]")
		addr, err := netip.ParseAddr(host)
		if err != nil {
			continue
		}
		for _, ig := range m.ignores {
			if ig.MatchString(line) {
				return netip.Addr{}, false
			}
		}
		return addr.Unmap(), true
	}
	return netip.Addr{}, false
}

// jailStats 是规则的运行统计。
type jailStats struct {
	matches atomic.Int64
	bans    atomic.Int64
}

// jailRunner 是一条运行中的规则。
type jailRunner struct {
	m      *Module
	jail   Jail
	match  *matcher
	log    *logbuf.Logger
	stats  *jailStats
	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	attempts map[string][]time.Time
	// errs 记录各日志来源当前的错误（按文件路径等区分），恢复后删除
	errs     map[string]string
	lastSeen time.Time
}

// record 记录一次失败，达到次数后封禁。
func (r *jailRunner) record(a netip.Addr, line string) {
	r.stats.matches.Add(1)
	if banlist.Default.Ignored(a) {
		return
	}
	key := banlist.KeyFor(a)
	if _, banned := banlist.Default.Get(key); banned {
		return
	}
	now := time.Now()
	window := time.Duration(r.jail.FindTime) * time.Second
	r.mu.Lock()
	r.lastSeen = now
	list := r.attempts[key]
	// 去掉统计窗口之外的记录
	i := 0
	for i < len(list) && now.Sub(list[i]) > window {
		i++
	}
	list = append(list[i:], now)
	if len(list) < r.jail.MaxRetry {
		if len(r.attempts) >= maxTracked {
			r.pruneLocked(now, window)
		}
		r.attempts[key] = list
		r.mu.Unlock()
		return
	}
	delete(r.attempts, key)
	r.mu.Unlock()
	r.stats.bans.Add(1)
	reason := fmt.Sprintf("%d 秒内失败 %d 次", r.jail.FindTime, len(list))
	if r.jail.Source == SourceWeb {
		// 带上最后一次请求，便于判断是否误封；路径由访客控制，截断后再写入
		reason = fmt.Sprintf("%d 秒内路径探测 %d 次，最近一次：%s", r.jail.FindTime, len(list), clipText(line, 200))
	}
	r.m.ban(a, r.jail.BanTime, reason, banlist.SourceJailPrefix+r.jail.Key, r.jail.Name)
}

// pruneLocked 清理窗口外的记录；仍然过多时整表清空。
func (r *jailRunner) pruneLocked(now time.Time, window time.Duration) {
	for k, list := range r.attempts {
		if len(list) == 0 || now.Sub(list[len(list)-1]) > window {
			delete(r.attempts, k)
		}
	}
	if len(r.attempts) >= maxTracked {
		r.attempts = map[string][]time.Time{}
	}
}

// line 处理一行日志。
func (r *jailRunner) line(text string) {
	if a, ok := r.match.match(text); ok {
		r.record(a, text)
	}
}

func (r *jailRunner) setErr(src, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == "" {
		delete(r.errs, src)
		return
	}
	r.errs[src] = msg
}

// errorText 返回当前的错误信息。
func (r *jailRunner) errorText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var parts []string
	for src, msg := range r.errs {
		if src == "" {
			parts = append(parts, msg)
		} else {
			parts = append(parts, src+": "+msg)
		}
	}
	return strings.Join(parts, "; ")
}

// stopWait 是停止规则时等待读取协程退出的上限。读取挂死的网络文件系统时协程可能退不出来，
// 不能让持有 m.mu 的调用方一直等下去，超时后放弃等待（协程在读取返回后会自行退出）。
var stopWait = 3 * time.Second

func (r *jailRunner) stop() {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(stopWait):
		if r.log != nil {
			r.log.Warnf("读取日志的协程未能及时退出，已放弃等待")
		}
	}
}

// startJailLocked 按配置启动规则（未启用时只保留统计），调用方需持有 m.mu。
func (m *Module) startJailLocked(j Jail) {
	// 规则可能由路径探测改为其他来源或停用，无论哪种情况都重建快照
	defer m.refreshWebLocked()
	stats := &jailStats{}
	if old := m.jails[j.Key]; old != nil {
		stats = old.stats
		old.stop()
		delete(m.jails, j.Key)
	}
	if !j.Enable || !m.started {
		// 停用的规则只保留统计，界面仍可查看
		m.jails[j.Key] = &jailRunner{jail: j, stats: stats, done: closedChan(), cancel: func() {}, errs: map[string]string{}}
		return
	}
	mt, err := newMatcher(j)
	ctx, cancel := context.WithCancel(m.ctx)
	r := &jailRunner{m: m, jail: j, match: mt, log: m.logs.Logger(j.Name), stats: stats, cancel: cancel,
		done: make(chan struct{}), attempts: map[string][]time.Time{}, errs: map[string]string{}}
	m.jails[j.Key] = r
	if err != nil {
		r.setErr("", err.Error())
		close(r.done)
		return
	}

	go func() {
		defer close(r.done)
		r.run(ctx)
	}()
}

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// run 按来源读取日志，直到 ctx 取消。
func (r *jailRunner) run(ctx context.Context) {
	switch r.jail.Source {
	case SourceFile:
		var wg sync.WaitGroup
		for _, p := range splitLines(r.jail.Target) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tailFile(ctx, p, r.line, func(msg string) { r.setErr(p, msg) })
			}()
		}
		wg.Wait()
	case SourceJournal:
		args := []string{"-f", "-n", "0", "-o", "cat"}
		for _, u := range splitLines(r.jail.Target) {
			args = append(args, "-u", u)
		}
		runCommand(ctx, "journalctl", args, r.line, func(msg string) { r.setErr("", msg) }, r.log)
	case SourceCommand:
		runCommand(ctx, "sh", []string{"-c", r.jail.Target}, r.line, func(msg string) { r.setErr("", msg) }, r.log)
	case SourceDocker:
		follow := r.m.deps.FollowDocker
		if follow == nil {
			r.setErr("", "Docker 管理模块未开启")
			<-ctx.Done()
			return
		}
		followDocker(ctx, follow, r.jail.Target, r.line, func(msg string) { r.setErr("", msg) })
	case SourceEvent, SourceWeb:
		// 由 Module.onEvent、Module.onWebHit 推送，这里只等待停止
		<-ctx.Done()
	}
}

// JailView 是带运行状态的规则。
type JailView struct {
	Jail
	Running  bool      `json:"running"`
	Matches  int64     `json:"matches"`
	Bans     int64     `json:"bans"`
	Active   int       `json:"active"`
	Error    string    `json:"error,omitempty"`
	LastSeen time.Time `json:"lastSeen,omitzero"`
}

func (m *Module) jailViewLocked(j *Jail) any {
	v := JailView{Jail: *j}
	if r := m.jails[j.Key]; r != nil {
		v.Matches = r.stats.matches.Load()
		v.Bans = r.stats.bans.Load()
		v.Error = r.errorText()
		r.mu.Lock()
		v.LastSeen = r.lastSeen
		r.mu.Unlock()
		select {
		case <-r.done:
		default:
			v.Running = true
		}
	}
	src := banlist.SourceJailPrefix + j.Key
	for _, e := range banlist.Default.List() {
		if e.Source == src {
			v.Active++
		}
	}
	return v
}

// clipText 把 s 截断到最多 n 个字符，截断时末尾加 "…"。
func clipText(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
