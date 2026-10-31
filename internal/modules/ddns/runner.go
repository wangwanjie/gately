package ddns

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/webhook"
)

// RecordState 是一条记录的同步状态。
type RecordState struct {
	Value    string    `json:"value,omitempty"`
	LastSync time.Time `json:"lastSync,omitzero"`
	// LastAction 为 created/updated/unchanged
	LastAction string    `json:"lastAction,omitempty"`
	LastError  string    `json:"lastError,omitempty"`
	LastCheck  time.Time `json:"lastCheck,omitzero"`
}

// TaskState 是任务的运行状态。
type TaskState struct {
	IPv4      string                 `json:"ipv4,omitempty"`
	IPv6      string                 `json:"ipv6,omitempty"`
	IPError   string                 `json:"ipError,omitempty"`
	LastRun   time.Time              `json:"lastRun,omitzero"`
	NextRun   time.Time              `json:"nextRun,omitzero"`
	Running   bool                   `json:"running"`
	Records   map[string]RecordState `json:"records"`
	WebhookAt time.Time              `json:"webhookAt,omitzero"`
	WebhookOK bool                   `json:"webhookOk"`
}

// providerFactory 允许测试替换服务商。
var providerFactory = dnsprovider.New

// ipGetter 允许测试替换 IP 获取。
var ipGetter = ipsource.Get

type runner struct {
	task   Task
	log    *logbuf.Logger
	cancel context.CancelFunc
	done   chan struct{}
	// trigger 用于手动立即同步，true 表示强制
	trigger chan bool
	// stunWebhook 在 Webhook 引用的 STUN 规则地址变化时置位，下次同步即使记录没有变化也调用 Webhook
	stunWebhook atomic.Bool

	mu    sync.Mutex
	state TaskState
}

// prevTask 为修改前的任务配置，与 prev 一起用于判断哪些记录的同步状态仍然有效。
func newRunner(t Task, log *logbuf.Logger, prev *TaskState, prevTask *Task) *runner {
	r := &runner{task: t, log: log, trigger: make(chan bool, 1), done: make(chan struct{})}
	r.state.Records = map[string]RecordState{}
	// 修改任务后保留仍存在且未改动记录的状态，避免无谓的重复同步
	if prev != nil {
		r.state.IPv4, r.state.IPv6 = prev.IPv4, prev.IPv6
		old := map[string]Record{}
		if prevTask != nil && prevTask.Provider == t.Provider {
			for _, rec := range prevTask.Records {
				old[rec.Key] = rec
			}
		}
		for _, rec := range t.Records {
			st, ok := prev.Records[rec.Key]
			if !ok {
				continue
			}
			// 改了域名、类型、线路或 TTL（例如把 lucky.example.com 改成 gately.example.com）的记录，
			// 旧状态对应的是另一条解析，保留会被当作“已同步”而永远不创建新记录
			if o, found := old[rec.Key]; prevTask != nil && (!found || !sameTarget(o, rec)) {
				continue
			}
			r.state.Records[rec.Key] = st
		}
	}
	return r
}

// sameTarget 判断两条记录是否指向服务商处的同一条解析（记录值由每轮渲染比较，不在此判断）。
func sameTarget(a, b Record) bool {
	return strings.EqualFold(a.Zone, b.Zone) && strings.EqualFold(a.Sub, b.Sub) && a.Type == b.Type && a.Line == b.Line && a.TTL == b.TTL
}

func (r *runner) start(parent context.Context, firstDelay time.Duration) {
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.mu.Lock()
	r.state.Running = true
	r.state.NextRun = time.Now().Add(firstDelay)
	r.mu.Unlock()
	go r.loop(ctx, firstDelay)
}

func (r *runner) stop() {
	if r.cancel != nil {
		r.cancel()
		<-r.done
	}
	r.mu.Lock()
	r.state.Running = false
	r.state.NextRun = time.Time{}
	r.mu.Unlock()
}

func (r *runner) snapshot() TaskState {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.state
	s.Records = make(map[string]RecordState, len(r.state.Records))
	for k, v := range r.state.Records {
		s.Records[k] = v
	}
	return s
}

// requestSync 请求立即同步；已有待处理请求时合并。
func (r *runner) requestSync(force bool) {
	select {
	case r.trigger <- force:
	default:
	}
}

func (r *runner) loop(ctx context.Context, firstDelay time.Duration) {
	defer close(r.done)
	interval := time.Duration(r.task.Interval) * time.Second
	timer := time.NewTimer(firstDelay)
	defer timer.Stop()
	for {
		force := false
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case force = <-r.trigger:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		r.syncOnce(ctx, force)
		timer.Reset(interval)
		r.mu.Lock()
		r.state.NextRun = time.Now().Add(interval)
		r.mu.Unlock()
	}
}

// syncOnce 执行一轮：获取 IP、渲染记录值、按需调用服务商、必要时触发 Webhook。
func (r *runner) syncOnce(ctx context.Context, force bool) {
	t := r.task
	timeout := time.Duration(t.HTTPTimeout) * time.Second
	needV4, needV6 := t.needs()
	vars := map[string]string{}
	var ipErrs []string

	r.mu.Lock()
	prevV4, prevV6 := r.state.IPv4, r.state.IPv6
	r.mu.Unlock()

	fetch := func(need bool, src ipsource.Source, fam ipsource.Family, name string) string {
		if !need {
			return ""
		}
		fctx, cancel := context.WithTimeout(ctx, timeout*2)
		defer cancel()
		addr, err := ipGetter(fctx, src, fam)
		if err != nil {
			ipErrs = append(ipErrs, fmt.Sprintf("获取 %s 失败: %v", name, err))
			return ""
		}
		return addr.String()
	}
	v4 := fetch(needV4, t.IPv4, ipsource.IPv4, "IPv4")
	v6 := fetch(needV6, t.IPv6, ipsource.IPv6, "IPv6")
	if v4 != "" {
		vars["ipv4Addr"] = v4
	}
	if v6 != "" {
		vars["ipv6Addr"] = v6
	}
	for _, e := range ipErrs {
		r.log.Errorf("%s", e)
	}
	if v4 != "" && v4 != prevV4 {
		r.log.Infof("IPv4 地址: %s -> %s", orDash(prevV4), v4)
	}
	if v6 != "" && v6 != prevV6 {
		r.log.Infof("IPv6 地址: %s -> %s", orDash(prevV6), v6)
	}

	r.mu.Lock()
	r.state.LastRun = time.Now()
	r.state.IPError = strings.Join(ipErrs, "; ")
	if v4 != "" {
		r.state.IPv4 = v4
	}
	if v6 != "" {
		r.state.IPv6 = v6
	}
	r.mu.Unlock()

	provider, err := providerFactory(t.Provider, t.Credentials, &http.Client{Timeout: timeout})
	if err != nil {
		r.log.Errorf("创建服务商客户端失败: %v", err)
		return
	}

	var success, failed []string
	forceAfter := time.Duration(t.ForceInterval) * time.Second
	for _, rec := range t.Records {
		if !rec.Enable {
			continue
		}
		fqdn := dnsprovider.Record{Zone: rec.Zone, Sub: rec.Sub}.FQDN()
		// {STUN_规则名_PORT} 等变量取 STUN 穿透规则当前的公网地址；规则还没有地址时本轮跳过
		stunVals, missing := stunaddr.Vars(rec.Value)
		if len(missing) > 0 {
			msg := fmt.Sprintf("STUN 规则“%s”暂无公网地址", strings.Join(missing, "”“"))
			r.setRecord(rec.Key, func(s *RecordState) { s.LastError = msg; s.LastCheck = time.Now() })
			r.log.Warnf("%s %s 跳过：%s", rec.Type, fqdn, msg)
			continue
		}
		maps.Copy(vars, stunVals)
		value := webhook.Render(rec.Value, vars, nil)
		if strings.Contains(value, VarIPv4) || strings.Contains(value, VarIPv6) {
			// 对应 IP 获取失败，本轮跳过
			continue
		}
		if (rec.Type == "A" || rec.Type == "AAAA") && !validAddr(value, rec.Type) {
			r.setRecord(rec.Key, func(s *RecordState) { s.LastError = "记录值不是有效地址: " + value; s.LastCheck = time.Now() })
			r.log.Errorf("%s %s 记录值 %q 不是有效地址", rec.Type, fqdn, value)
			continue
		}
		r.mu.Lock()
		st := r.state.Records[rec.Key]
		r.mu.Unlock()
		due := force || st.Value != value || st.LastError != "" || st.LastSync.IsZero() ||
			(forceAfter > 0 && time.Since(st.LastSync) >= forceAfter)
		if !due {
			r.setRecord(rec.Key, func(s *RecordState) { s.LastCheck = time.Now() })
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, timeout*3)
		action, err := provider.Upsert(cctx, dnsprovider.Record{
			Zone: rec.Zone, Sub: rec.Sub, Type: rec.Type, Value: value, TTL: rec.TTL, Line: rec.Line,
		})
		cancel()
		if err != nil {
			failed = append(failed, fqdn)
			r.setRecord(rec.Key, func(s *RecordState) { s.LastError = err.Error(); s.LastCheck = time.Now() })
			r.log.Errorf("%s %s 同步失败: %v", rec.Type, fqdn, err)
			events.Publish(events.Event{
				Type:    events.DDNSFailed,
				Source:  t.Name,
				Level:   events.LevelError,
				Title:   "动态域名更新失败：" + fqdn,
				Message: fmt.Sprintf("%s 记录 %s 同步失败：%v", rec.Type, fqdn, err),
				Fields:  map[string]string{"domain": fqdn, "error": err.Error()},
			})
			continue
		}
		now := time.Now()
		r.setRecord(rec.Key, func(s *RecordState) {
			s.Value, s.LastSync, s.LastAction, s.LastError, s.LastCheck = value, now, action, "", now
		})
		switch action {
		case dnsprovider.ActionUnchanged:
			r.log.Debugf("%s %s 无需更新 (%s)", rec.Type, fqdn, value)
		case dnsprovider.ActionCreated:
			success = append(success, fqdn)
			r.log.Infof("%s %s 已创建 -> %s", rec.Type, fqdn, value)
			publishDDNSChanged(t.Name, fqdn, rec.Type, value, "已创建")
		default:
			success = append(success, fqdn)
			r.log.Infof("%s %s 已更新 -> %s", rec.Type, fqdn, value)
			publishDDNSChanged(t.Name, fqdn, rec.Type, value, "已更新")
		}
	}

	stunChanged := r.stunWebhook.Swap(false)
	if t.Webhook.Enable && (len(success) > 0 || len(failed) > 0 || stunChanged) {
		r.sendWebhook(ctx, vars, success, failed)
	}
}

func (r *runner) sendWebhook(ctx context.Context, vars map[string]string, success, failed []string) {
	// ipAddr 优先取 IPv4，仅有 IPv6 时为 IPv6
	ipAddr := vars["ipv4Addr"]
	if ipAddr == "" {
		ipAddr = vars["ipv6Addr"]
	}
	wv := map[string]string{
		"ipv4Addr":           vars["ipv4Addr"],
		"ipv6Addr":           vars["ipv6Addr"],
		"ipAddr":             ipAddr,
		"successDomains":     strings.Join(success, ","),
		"failedDomains":      strings.Join(failed, ","),
		"successDomainsLine": strings.Join(success, "\n"),
		"failedDomainsLine":  strings.Join(failed, "\n"),
		"taskName":           r.task.Name,
		"time":               time.Now().Format("2006-01-02 15:04:05"),
	}
	// Webhook 中也可以引用 {STUN_规则名_PORT} 等变量
	stunVals, _ := stunaddr.Vars(webhookTexts(r.task.Webhook)...)
	maps.Copy(wv, stunVals)
	_, err := webhook.Send(ctx, r.task.Webhook, wv)
	r.mu.Lock()
	r.state.WebhookAt = time.Now()
	r.state.WebhookOK = err == nil
	r.mu.Unlock()
	if err != nil {
		r.log.Errorf("Webhook 调用失败: %v", err)
		return
	}
	r.log.Infof("Webhook 调用成功")
}

// webhookTexts 返回 Webhook 中可以引用变量的文本。
func webhookTexts(c webhook.Config) []string {
	return append([]string{c.URL, c.Body}, c.Headers...)
}

func (r *runner) setRecord(key string, f func(*RecordState)) {
	r.mu.Lock()
	st := r.state.Records[key]
	f(&st)
	r.state.Records[key] = st
	r.mu.Unlock()
}

func validAddr(v, typ string) bool {
	a, err := netip.ParseAddr(v)
	if err != nil {
		return false
	}
	return (typ == "A") == a.Is4()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// publishDDNSChanged 通知解析记录已创建或更新。
func publishDDNSChanged(task, fqdn, typ, value, action string) {
	events.Publish(events.Event{
		Type:    events.DDNSChanged,
		Source:  task,
		Level:   events.LevelInfo,
		Title:   "动态域名" + action + "：" + fqdn,
		Message: fmt.Sprintf("%s 记录 %s %s，当前值 %s", typ, fqdn, action, value),
		Fields:  map[string]string{"domain": fqdn, "value": value},
	})
}
