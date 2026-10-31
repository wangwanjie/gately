// Package notify 实现通知中心：订阅进程内事件，按规则经 Webhook、Telegram、Bark、企业微信、钉钉、飞书、邮件等渠道推送。
package notify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/webhook"
)

// ID 是模块标识。
const ID = "notify"

// historySize 是内存中保留的投递记录条数。
const historySize = 200

// Channel 是一个通知渠道。各渠道只使用与其类型相关的字段：
//
//	webhook   Webhook
//	telegram  Secret=Bot Token，Target=Chat ID，URL 可选（自建 API 地址）
//	bark      Secret=设备 Key，URL 可选（自建服务地址）
//	wecom     URL=机器人地址
//	dingtalk  URL=机器人地址，Secret=加签密钥（可选）
//	feishu    URL=机器人地址，Secret=签名校验密钥（可选）
//	ntfy      URL 可选（默认 ntfy.sh），Target=主题，Secret=访问令牌（可选）
//	gotify    URL=服务地址，Secret=应用令牌
//	pushover  Secret=应用令牌，Target=用户 Key，URL 可选
//	discord / slack  URL=Webhook 地址
//	serverchan / pushplus  Secret=SendKey / token，URL 可选
//	smtp      Host/Port/Security/User/Secret=密码/From，Target=收件人（逗号分隔）
type Channel struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Kind   string `json:"kind"`

	URL      string         `json:"url"`
	Secret   string         `json:"secret"`
	Target   string         `json:"target"`
	Host     string         `json:"host"`
	Port     int            `json:"port"`
	Security string         `json:"security"`
	User     string         `json:"user"`
	From     string         `json:"from"`
	Webhook  webhook.Config `json:"webhook"`
	Remark   string         `json:"remark"`
}

// Rule 把一组事件路由到若干渠道。
type Rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	// Events 为事件类型，支持 path.Match 通配（如 ddns.*），"*" 匹配全部
	Events []string `json:"events"`
	// MinLevel 为最低级别 info / warn / error，留空为 info
	MinLevel string `json:"minLevel"`
	// Channels 为渠道 key 列表
	Channels []string `json:"channels"`
	// Silence 为同一来源同一类型事件的静默秒数，期间重复事件被丢弃，0 表示不去重
	Silence int `json:"silence"`
}

// Record 是一次投递记录。
type Record struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Title   string    `json:"title"`
	Rule    string    `json:"rule"`
	Channel string    `json:"channel"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
}

type fileConfig struct {
	Channels []Channel `json:"channels"`
	Rules    []Rule    `json:"rules"`
}

// channelView 在输出时隐藏密钥，仅告知是否已设置。
type channelView struct {
	Channel
	Secret    string `json:"secret"`
	HasSecret bool   `json:"hasSecret"`
}

// Validate 校验并规范化渠道配置。
func (c *Channel) Validate() error {
	if err := crud.NormalizeGroup(&c.Group); err != nil {
		return err
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("渠道名称不能为空")
	}
	c.URL = strings.TrimSpace(c.URL)
	c.Target = strings.TrimSpace(c.Target)
	switch c.Kind {
	case KindWebhook:
		c.Webhook.Enable = true
		return c.Webhook.Validate()
	case KindTelegram:
		if c.Secret == "" || c.Target == "" {
			return errors.New("Telegram 需要填写 Bot Token 与 Chat ID")
		}
		return validateOptionalURL(c.URL)
	case KindBark:
		if c.Secret == "" {
			return errors.New("Bark 需要填写设备 Key")
		}
		return validateOptionalURL(c.URL)
	case KindWeCom, KindDingTalk, KindFeishu, KindDiscord, KindSlack:
		return validateRequiredURL(c.URL)
	case KindNtfy:
		if c.Target == "" {
			return errors.New("ntfy 需要填写主题")
		}
		return validateOptionalURL(c.URL)
	case KindGotify:
		if c.Secret == "" {
			return errors.New("Gotify 需要填写应用令牌")
		}
		return validateRequiredURL(c.URL)
	case KindPushover:
		if c.Secret == "" || c.Target == "" {
			return errors.New("Pushover 需要填写应用令牌与用户 Key")
		}
		return validateOptionalURL(c.URL)
	case KindServerChan:
		if c.Secret == "" {
			return errors.New("Server酱 需要填写 SendKey")
		}
		return validateOptionalURL(c.URL)
	case KindPushPlus:
		if c.Secret == "" {
			return errors.New("PushPlus 需要填写 token")
		}
		return validateOptionalURL(c.URL)
	case KindSMTP:
		c.Host = strings.TrimSpace(c.Host)
		if c.Host == "" || c.Target == "" {
			return errors.New("邮件需要填写服务器与收件人")
		}
		switch c.Security {
		case SecurityNone, SecurityStartTLS, SecurityTLS:
		case "":
			c.Security = SecurityStartTLS
		default:
			return fmt.Errorf("不支持的加密方式 %q", c.Security)
		}
		if c.Port == 0 {
			c.Port = map[string]int{SecurityTLS: 465, SecurityStartTLS: 587, SecurityNone: 25}[c.Security]
		}
		if c.Port < 1 || c.Port > 65535 {
			return errors.New("SMTP 端口无效")
		}
		if c.From == "" && c.User == "" {
			return errors.New("发件人或用户名至少填写一项")
		}
		return nil
	}
	return fmt.Errorf("不支持的渠道类型 %q", c.Kind)
}

func validateRequiredURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("地址无效，需要 http(s):// 开头的完整地址")
	}
	return nil
}

func validateOptionalURL(s string) error {
	if s == "" {
		return nil
	}
	return validateRequiredURL(s)
}

// Validate 校验并规范化规则。
func (r *Rule) Validate() error {
	if err := crud.NormalizeGroup(&r.Group); err != nil {
		return err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("规则名称不能为空")
	}
	var evs []string
	for _, e := range r.Events {
		if e = strings.TrimSpace(e); e == "" {
			continue
		}
		if _, err := path.Match(e, "x"); err != nil {
			return fmt.Errorf("事件匹配式 %q 无效", e)
		}
		evs = append(evs, e)
	}
	if len(evs) == 0 {
		return errors.New("至少选择一个事件")
	}
	r.Events = evs
	switch r.MinLevel {
	case "":
		r.MinLevel = events.LevelInfo
	case events.LevelInfo, events.LevelWarn, events.LevelError:
	default:
		return fmt.Errorf("不支持的级别 %q", r.MinLevel)
	}
	if len(r.Channels) == 0 {
		return errors.New("至少选择一个渠道")
	}
	if r.Silence < 0 {
		r.Silence = 0
	}
	return nil
}

func (r *Rule) matches(e events.Event) bool {
	if !r.Enable || events.LevelRank(e.Level) < events.LevelRank(r.MinLevel) {
		return false
	}
	for _, p := range r.Events {
		if p == "*" {
			return true
		}
		if ok, _ := path.Match(p, e.Type); ok {
			return true
		}
	}
	return false
}

// Module 是通知中心模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	logs  *logbuf.Buffer
	log   *logbuf.Logger
	cfg   fileConfig

	// lastSent 记录 规则key|类型|来源 上次投递时间，用于静默去重
	lastSent map[string]time.Time
	history  []Record
	unsub    func()
	// sender 允许测试替换实际投递
	sender func(ctx context.Context, ch Channel, msg Message) error
	now    func() time.Time
}

// New 创建模块。
func New() *Module {
	return &Module{lastSent: map[string]time.Time{}, sender: send, now: time.Now}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "通知中心" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.log = m.logs.Logger("通知")
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module：订阅默认事件总线。
func (m *Module) Start(context.Context) error {
	if m.unsub != nil {
		return nil
	}
	m.unsub = events.Subscribe(m.handle)
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	if m.unsub != nil {
		m.unsub()
		m.unsub = nil
	}
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// dispatch 是一次待执行的投递。
type dispatch struct {
	rule string
	ch   Channel
}

// plan 在持锁状态下匹配规则并应用静默去重，返回需要投递的渠道。
func (m *Module) plan(e events.Event) []dispatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	var out []dispatch
	seen := map[string]bool{}
	for _, r := range m.cfg.Rules {
		if !r.matches(e) {
			continue
		}
		if r.Silence > 0 {
			k := r.Key + "|" + e.Type + "|" + e.Source
			if last, ok := m.lastSent[k]; ok && now.Sub(last) < time.Duration(r.Silence)*time.Second {
				continue
			}
			m.lastSent[k] = now
		}
		for _, ck := range r.Channels {
			// 同一事件命中多条规则时，同一渠道只发一次
			if seen[ck] {
				continue
			}
			for _, ch := range m.cfg.Channels {
				if ch.Key == ck && ch.Enable {
					seen[ck] = true
					out = append(out, dispatch{rule: r.Name, ch: ch})
				}
			}
		}
	}
	return out
}

func (m *Module) handle(e events.Event) {
	m.process(e, true)
}

// handleSync 同步投递，供测试使用。
func (m *Module) handleSync(e events.Event) {
	m.process(e, false)
}

func (m *Module) process(e events.Event, async bool) {
	// 测试事件只走测试接口，避免重复投递
	if e.Type == events.NotifyTest {
		return
	}
	msg := NewMessage(e)
	for _, d := range m.plan(e) {
		if async {
			go m.deliver(d.rule, d.ch, msg)
		} else {
			_ = m.deliver(d.rule, d.ch, msg)
		}
	}
}

func (m *Module) deliver(rule string, ch Channel, msg Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	err := m.sender(ctx, ch, msg)
	rec := Record{Time: m.now(), Type: msg.Event.Type, Title: msg.Title, Rule: rule, Channel: ch.Name, OK: err == nil}
	if err != nil {
		rec.Error = err.Error()
		m.log.Warnf("经渠道 %s 推送「%s」失败: %v", ch.Name, msg.Title, err)
	}
	m.mu.Lock()
	m.history = append(m.history, rec)
	if len(m.history) > historySize {
		m.history = m.history[len(m.history)-historySize:]
	}
	m.mu.Unlock()
	return err
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	channels := &crud.Collection[Channel]{
		Mu:       &m.mu,
		Items:    &m.cfg.Channels,
		Save:     m.saveLocked,
		Key:      func(c *Channel) *string { return &c.Key },
		Enabled:  func(c *Channel) *bool { return &c.Enable },
		Group:    func(c *Channel) *string { return &c.Group },
		Validate: func(c *Channel) error { return c.Validate() },
		// 密钥留空表示保持原值
		Merge: func(old, updated *Channel) {
			if updated.Secret == "" {
				updated.Secret = old.Secret
			}
		},
		View: func(c *Channel) any {
			return channelView{Channel: *c, HasSecret: c.Secret != ""}
		},
		Secrets: func(c *Channel) map[string]string { return map[string]string{"secret": c.Secret} },
	}
	channels.Register(r, "/channels")
	rules := &crud.Collection[Rule]{
		Mu:       &m.mu,
		Items:    &m.cfg.Rules,
		Save:     m.saveLocked,
		Key:      func(x *Rule) *string { return &x.Key },
		Enabled:  func(x *Rule) *bool { return &x.Enable },
		Group:    func(x *Rule) *string { return &x.Group },
		Validate: func(x *Rule) error { return x.Validate() },
	}
	rules.Register(r, "/rules")
	r.HandleFunc("GET /event-types", m.handleEventTypes)
	r.HandleFunc("GET /history", m.handleHistory)
	r.HandleFunc("POST /channels/{key}/test", m.handleTest)
	module.DocOf(r, "GET /event-types", module.Doc{Summary: "可订阅的事件类型", Resp: []events.TypeInfo{}})
	module.DocOf(r, "GET /history", module.Doc{Summary: "最近的投递记录", Resp: []Record{}})
	module.DocOf(r, "POST /channels/{key}/test", module.Doc{Summary: "向渠道发送测试通知"})
}

func (m *Module) handleEventTypes(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, events.Catalog)
}

func (m *Module) handleHistory(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.history))
	// 新记录在前
	for i := len(m.history) - 1; i >= 0; i-- {
		out = append(out, m.history[i])
	}
	httpx.OK(w, out)
}

func (m *Module) handleTest(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	m.mu.Lock()
	var ch *Channel
	for i := range m.cfg.Channels {
		if m.cfg.Channels[i].Key == key {
			c := m.cfg.Channels[i]
			ch = &c
			break
		}
	}
	m.mu.Unlock()
	if ch == nil {
		httpx.Error(w, http.StatusNotFound, "渠道不存在")
		return
	}
	msg := NewMessage(events.Event{
		Type:    events.NotifyTest,
		Source:  "通知中心",
		Level:   events.LevelInfo,
		Title:   "测试通知",
		Message: "这是一条来自 Gately 的测试通知。",
		Time:    m.now(),
	})
	if err := m.deliver("测试", *ch, msg); err != nil {
		httpx.Error(w, http.StatusBadGateway, "发送失败: %v", err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}
