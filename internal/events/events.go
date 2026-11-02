// Package events 提供进程内事件总线：各模块在状态变化时发布事件，通知中心等订阅者异步消费。
//
// 发布永远不会阻塞调用方：事件投递给每个订阅者时各自在独立协程中执行。
package events

import (
	"sync"
	"time"
)

// 事件级别，从低到高。
const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// 事件类型。新增类型时同步更新 Catalog。
const (
	DDNSChanged     = "ddns.changed"
	DDNSFailed      = "ddns.failed"
	SSLExpiring     = "ssl.expiring"
	SSLRenewFailed  = "ssl.renew_failed"
	SSLDeployFailed = "ssl.deploy_failed"
	WAFBlocked      = "waf.blocked"
	UpstreamDown    = "upstream.down"
	UpstreamUp      = "upstream.up"
	DockerExited    = "docker.exited"
	ProcessExited   = "process.exited"
	BackupDone      = "backup.done"
	BackupFailed    = "backup.failed"
	AuthFailed      = "auth.failed"
	AuthBanned      = "auth.banned"
	GuardBanned     = "guard.banned"
	NotifyTest      = "notify.test"
	STUNChanged     = "stun.changed"
	LoginSuccessful = "login.success"
	AuditChange     = "audit.change"
	AuditAccess     = "audit.access"
	PerfAlert       = "perf.alert"
	PerfRecovered   = "perf.recovered"
	// SystemUpdate / SystemSecurity 由自动检查更新发布：Gately 有新版本、当前版本受安全问题影响
	SystemUpdate   = "system.update"
	SystemSecurity = "system.security"
	// ProgramUpdate 为托管程序（frp、Tailscale 等）有新版本，DockerUpdate 为 Docker 定时检查发现容器镜像有新版本
	ProgramUpdate = "program.update"
	DockerUpdate  = "docker.update"
)

// Event 是一次可通知的事件。
type Event struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Level  string `json:"level"`
	Title  string `json:"title"`
	// Message 是面向人的详细描述
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Time    time.Time         `json:"time"`
}

// TypeInfo 描述一种事件类型，供界面展示。
type TypeInfo struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

// Catalog 列出全部已知事件类型。
var Catalog = []TypeInfo{
	{DDNSChanged, "动态域名：解析已更新", LevelInfo},
	{DDNSFailed, "动态域名：更新失败", LevelError},
	{STUNChanged, "STUN：穿透地址变化", LevelInfo},
	{SSLExpiring, "证书：即将到期", LevelWarn},
	{SSLRenewFailed, "证书：续期失败", LevelError},
	{SSLDeployFailed, "证书：部署失败", LevelError},
	{WAFBlocked, "WAF：拦截请求", LevelWarn},
	{UpstreamDown, "反向代理：上游不可用", LevelError},
	{UpstreamUp, "反向代理：上游恢复", LevelInfo},
	{DockerExited, "Docker：容器退出", LevelWarn},
	{DockerUpdate, "Docker：镜像有新版本", LevelInfo},
	{ProcessExited, "子进程异常退出", LevelError},
	{BackupDone, "备份：完成", LevelInfo},
	{BackupFailed, "备份：失败", LevelError},
	{AuthFailed, "认证：连续失败", LevelWarn},
	{AuthBanned, "认证：IP 已被封禁", LevelWarn},
	{GuardBanned, "入侵防护：IP 已被封禁", LevelWarn},
	{LoginSuccessful, "管理后台：登录成功", LevelInfo},
	{AuditChange, "管理后台：配置变更", LevelInfo},
	{AuditAccess, "管理后台：访问敏感内容", LevelWarn},
	{PerfAlert, "性能监控：触发告警", LevelWarn},
	{PerfRecovered, "性能监控：告警恢复", LevelInfo},
	{SystemUpdate, "Gately：发现新版本", LevelInfo},
	{SystemSecurity, "Gately：安全更新", LevelError},
	{ProgramUpdate, "托管程序：发现新版本", LevelInfo},
	{NotifyTest, "测试通知", LevelInfo},
}

// LevelRank 返回级别的数值，未知级别按 info 处理。
func LevelRank(level string) int {
	switch level {
	case LevelWarn:
		return 1
	case LevelError:
		return 2
	}
	return 0
}

// Handler 处理一个事件。
type Handler func(Event)

// Bus 是事件总线。
type Bus struct {
	mu       sync.RWMutex
	next     int
	handlers map[int]Handler
}

// NewBus 创建独立的总线，主要用于测试。
func NewBus() *Bus {
	return &Bus{handlers: map[int]Handler{}}
}

// Subscribe 注册订阅者，返回取消订阅的函数。
func (b *Bus) Subscribe(h Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	b.handlers[id] = h
	return func() {
		b.mu.Lock()
		delete(b.handlers, id)
		b.mu.Unlock()
	}
}

// Publish 异步投递事件。缺省的时间与级别会被补全。
func (b *Bus) Publish(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Level == "" {
		e.Level = LevelInfo
	}
	b.mu.RLock()
	hs := make([]Handler, 0, len(b.handlers))
	for _, h := range b.handlers {
		hs = append(hs, h)
	}
	b.mu.RUnlock()
	for _, h := range hs {
		go h(e)
	}
}

// Default 是进程级默认总线。
var Default = NewBus()

// Publish 向默认总线发布事件。
func Publish(e Event) {
	Default.Publish(e)
}

// Subscribe 订阅默认总线。
func Subscribe(h Handler) func() {
	return Default.Subscribe(h)
}
