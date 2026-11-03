package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wangwanjie/gately/internal/i18n"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/proxyx"
	"github.com/wangwanjie/gately/internal/webhook"
)

// 渠道类型
const (
	KindWebhook  = "webhook"
	KindTelegram = "telegram"
	KindBark     = "bark"
	KindWeCom    = "wecom"
	KindDingTalk = "dingtalk"
	KindFeishu   = "feishu"
	KindSMTP     = "smtp"
	KindNtfy     = "ntfy"
	KindGotify   = "gotify"
	KindPushover = "pushover"
	KindDiscord  = "discord"
	KindSlack    = "slack"
	// KindServerChan 为 Server酱（Turbo 版）
	KindServerChan = "serverchan"
	KindPushPlus   = "pushplus"
)

// Message 是投递给渠道的内容。
type Message struct {
	Event events.Event
	// Title 与 Text 已经过格式化，可直接展示
	Title string
	Text  string
	// Server 为发出通知的服务器名称，为空时不显示
	Server string
}

var levelLabel = map[string]string{
	events.LevelInfo:  "信息",
	events.LevelWarn:  "警告",
	events.LevelError: "错误",
}

// NewMessage 把事件格式化为人可读的消息。server 为服务器名称，非空时写进标题与正文，便于区分多台服务器发出的通知。
func NewMessage(e events.Event, server string) Message {
	// 通知内容按服务端语言翻译（事件文案由程序生成，用户自定义的内容不在翻译表中，会原样保留）
	lang := i18n.ServerLang()
	tr := func(s string) string { return i18n.Tr(lang, s) }
	e.Title = tr(e.Title)
	e.Message = tr(e.Message)
	title := e.Title
	if title == "" {
		title = e.Type
	}
	label := tr(levelLabel[e.Level])
	if label == "" {
		label = e.Level
	}
	var b strings.Builder
	if e.Message != "" {
		b.WriteString(e.Message)
		b.WriteString("\n")
	}
	if server != "" {
		b.WriteString(tr("服务器：") + server + "\n")
	}
	if e.Source != "" {
		// 来源可能是程序文案（“管理后台”“配置备份”），也可能是用户填写的任务名，只做整句匹配翻译，避免改写用户名称
		b.WriteString(tr("来源：") + i18n.TrExact(lang, e.Source) + "\n")
	}
	b.WriteString(tr("级别：") + label + "\n")
	b.WriteString(tr("时间：") + e.Time.Format("2006-01-02 15:04:05"))
	prefix := "[Gately] "
	if server != "" {
		prefix = "[Gately · " + server + "] "
	}
	return Message{Event: e, Title: prefix + title, Text: b.String(), Server: server}
}

// vars 返回 Webhook 模板变量。
func (m Message) vars() map[string]string {
	v := map[string]string{
		"title":   m.Title,
		"server":  m.Server,
		"message": m.Event.Message,
		"text":    m.Text,
		"type":    m.Event.Type,
		"source":  m.Event.Source,
		"level":   m.Event.Level,
		"time":    m.Event.Time.Format(time.RFC3339),
	}
	for k, val := range m.Event.Fields {
		if _, exists := v[k]; !exists {
			v[k] = val
		}
	}
	return v
}

// httpTimeout 为单次 HTTP 投递的超时
const httpTimeout = 20 * time.Second

// newClient 按渠道的代理设置返回 HTTP 客户端；测试可替换。
var newClient = func(proxy string) *http.Client {
	return proxyx.Client(proxy, httpTimeout, false)
}

// clientKey 是 context 中保存本次投递所用客户端的键
type clientKey struct{}

// clientFrom 取出 send 放入 context 的客户端；没有时（直接调用发送函数的测试）按未设置代理处理。
func clientFrom(ctx context.Context) *http.Client {
	if c, ok := ctx.Value(clientKey{}).(*http.Client); ok {
		return c
	}
	return newClient("")
}

// send 按渠道类型投递消息。ch.Proxy 为已合并全局设置后的实际代理。
func send(ctx context.Context, ch Channel, msg Message) error {
	ctx = context.WithValue(ctx, clientKey{}, newClient(ch.Proxy))
	switch ch.Kind {
	case KindWebhook:
		cfg := ch.Webhook
		cfg.Enable = true
		cfg.Proxy = ch.Proxy
		_, err := webhook.Send(ctx, cfg, msg.vars())
		return err
	case KindTelegram:
		return sendTelegram(ctx, ch, msg)
	case KindBark:
		return sendBark(ctx, ch, msg)
	case KindWeCom:
		return postJSON(ctx, ch.URL, map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": msg.Title + "\n" + msg.Text},
		}, "errcode", 0)
	case KindDingTalk:
		return sendDingTalk(ctx, ch, msg)
	case KindFeishu:
		return sendFeishu(ctx, ch, msg)
	case KindSMTP:
		return sendSMTP(ctx, ch, msg)
	case KindNtfy:
		return sendNtfy(ctx, ch, msg)
	case KindGotify:
		return sendGotify(ctx, ch, msg)
	case KindPushover:
		return sendPushover(ctx, ch, msg)
	case KindDiscord:
		return postJSON(ctx, ch.URL, map[string]any{"content": truncate(msg.Title+"\n"+msg.Text, 2000)}, "", 0)
	case KindSlack:
		return postJSON(ctx, ch.URL, map[string]any{"text": msg.Title + "\n" + msg.Text}, "", 0)
	case KindServerChan:
		return sendServerChan(ctx, ch, msg)
	case KindPushPlus:
		return sendPushPlus(ctx, ch, msg)
	}
	return fmt.Errorf("不支持的渠道类型 %q", ch.Kind)
}

// postJSON 发送 JSON；okField 非空时要求响应 JSON 中该字段等于 okValue（数字），兼容企业微信、钉钉等“HTTP 200 但业务失败”的返回。
func postJSON(ctx context.Context, target string, body any, okField string, okValue float64) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := clientFrom(ctx).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, shorten(string(raw)))
	}
	if okField == "" {
		return nil
	}
	var parsed map[string]any
	if json.Unmarshal(raw, &parsed) != nil {
		return nil
	}
	v, ok := parsed[okField]
	if !ok {
		return nil
	}
	if n, isNum := v.(float64); isNum && n != okValue {
		return fmt.Errorf("渠道返回错误: %s", shorten(string(raw)))
	}
	return nil
}

func shorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 200 {
		return string(r[:200]) + "..."
	}
	return s
}

func sendTelegram(ctx context.Context, ch Channel, msg Message) error {
	base := strings.TrimRight(ch.URL, "/")
	if base == "" {
		base = "https://api.telegram.org"
	}
	target := base + "/bot" + ch.Secret + "/sendMessage"
	return postJSON(ctx, target, map[string]any{
		"chat_id": ch.Target,
		"text":    msg.Title + "\n" + msg.Text,
	}, "", 0)
}

func sendBark(ctx context.Context, ch Channel, msg Message) error {
	base := strings.TrimRight(ch.URL, "/")
	if base == "" {
		base = "https://api.day.app"
	}
	target := base + "/" + url.PathEscape(ch.Secret)
	return postJSON(ctx, target, map[string]any{
		"title": msg.Title,
		"body":  msg.Text,
		"group": "Gately",
	}, "code", 200)
}

func sendDingTalk(ctx context.Context, ch Channel, msg Message) error {
	target := ch.URL
	if ch.Secret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		mac := hmac.New(sha256.New, []byte(ch.Secret))
		mac.Write([]byte(ts + "\n" + ch.Secret))
		sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "timestamp=" + ts + "&sign=" + url.QueryEscape(sign)
	}
	return postJSON(ctx, target, map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": msg.Title + "\n" + msg.Text},
	}, "errcode", 0)
}

func sendFeishu(ctx context.Context, ch Channel, msg Message) error {
	body := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": msg.Title + "\n" + msg.Text},
	}
	if ch.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(ts+"\n"+ch.Secret))
		body["timestamp"] = ts
		body["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	return postJSON(ctx, ch.URL, body, "code", 0)
}

// smtpDial 建立到 SMTP 服务器的 TCP 连接；测试可替换。
var smtpDial = proxyx.Dial

// SMTP 加密方式
const (
	SecurityNone     = "none"
	SecurityStartTLS = "starttls"
	SecurityTLS      = "tls"
)

func sendSMTP(ctx context.Context, ch Channel, msg Message) error {
	addr := net.JoinHostPort(ch.Host, strconv.Itoa(ch.Port))
	tlsCfg := &tls.Config{ServerName: ch.Host}
	// 配置了代理时经代理建立 TCP 连接（http/https 代理用 CONNECT，socks5 由代理解析域名），否则直连
	conn, err := smtpDial(ctx, ch.Proxy, addr, 15*time.Second)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if ch.Security == SecurityTLS {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return err
		}
		conn = tc
	}
	c, err := smtp.NewClient(conn, ch.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if ch.Security == SecurityStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("服务器不支持 STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if ch.User != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(smtp.PlainAuth("", ch.User, ch.Secret, ch.Host)); err != nil {
				return err
			}
		}
	}
	from := ch.From
	if from == "" {
		from = ch.User
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	var rcpts []string
	for _, t := range strings.Split(ch.Target, ",") {
		if t = strings.TrimSpace(t); t != "" {
			rcpts = append(rcpts, t)
			if err := c.Rcpt(t); err != nil {
				return err
			}
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(rcpts, ", ") + "\r\n")
	buf.WriteString("Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(msg.Title)) + "?=\r\n")
	buf.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(msg.Text))
	for len(enc) > 76 {
		buf.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	buf.WriteString(enc + "\r\n")
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// truncate 按字符数截断，超出时追加省略号。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// levelPriority 把事件级别映射为 1（最低）到 5（最高）的优先级，供 ntfy、Gotify 等使用。
func levelPriority(level string) int {
	switch level {
	case events.LevelError:
		return 5
	case events.LevelWarn:
		return 4
	}
	return 3
}

// doRequest 发送请求并按 HTTP 状态码判断成败，返回响应体。
func doRequest(req *http.Request) ([]byte, error) {
	resp, err := clientFrom(req.Context()).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("HTTP %d: %s", resp.StatusCode, shorten(string(raw)))
	}
	return raw, nil
}

// sendNtfy 向 ntfy 主题发布消息；Secret 非空时作为访问令牌。
func sendNtfy(ctx context.Context, ch Channel, msg Message) error {
	base := strings.TrimRight(ch.URL, "/")
	if base == "" {
		base = "https://ntfy.sh"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+url.PathEscape(ch.Target), strings.NewReader(msg.Text))
	if err != nil {
		return err
	}
	// 标题可能含非 ASCII 字符，使用 RFC 2047 编码，ntfy 会自动解码
	req.Header.Set("Title", mime.QEncoding.Encode("utf-8", msg.Title))
	req.Header.Set("Priority", strconv.Itoa(levelPriority(msg.Event.Level)))
	if ch.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+ch.Secret)
	}
	_, err = doRequest(req)
	return err
}

// sendGotify 通过应用令牌推送消息。
func sendGotify(ctx context.Context, ch Channel, msg Message) error {
	target := strings.TrimRight(ch.URL, "/") + "/message"
	data, err := json.Marshal(map[string]any{"title": msg.Title, "message": msg.Text, "priority": levelPriority(msg.Event.Level)*2 - 2})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// 令牌放在请求头而不是查询串，避免出现在代理或服务端访问日志里
	req.Header.Set("X-Gotify-Key", ch.Secret)
	_, err = doRequest(req)
	return err
}

// sendPushover 使用应用令牌（Secret）向用户（Target）推送。
func sendPushover(ctx context.Context, ch Channel, msg Message) error {
	target := ch.URL
	if target == "" {
		target = "https://api.pushover.net/1/messages.json"
	}
	form := url.Values{
		"token":    {ch.Secret},
		"user":     {ch.Target},
		"title":    {msg.Title},
		"message":  {truncate(msg.Text, 1024)},
		"priority": {strconv.Itoa(pushoverPriority(msg.Event.Level))},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = doRequest(req)
	return err
}

// pushoverPriority 的取值范围为 -2 到 2；错误使用 1（高优先级），避免要求确认的紧急级别 2。
func pushoverPriority(level string) int {
	switch level {
	case events.LevelError:
		return 1
	case events.LevelWarn:
		return 0
	}
	return -1
}

// sendServerChan 通过 Server酱 Turbo 版推送，Secret 为 SendKey。
func sendServerChan(ctx context.Context, ch Channel, msg Message) error {
	target := ch.URL
	if target == "" {
		target = "https://sctapi.ftqq.com/" + url.PathEscape(ch.Secret) + ".send"
	}
	form := url.Values{"title": {msg.Title}, "desp": {msg.Text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	raw, err := doRequest(req)
	if err != nil {
		return err
	}
	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Code != 0 {
		return fmt.Errorf("渠道返回错误: %s", shorten(string(raw)))
	}
	return nil
}

// sendPushPlus 通过 PushPlus 推送，Secret 为 token；业务成功码为 200。
func sendPushPlus(ctx context.Context, ch Channel, msg Message) error {
	target := ch.URL
	if target == "" {
		target = "https://www.pushplus.plus/send"
	}
	return postJSON(ctx, target, map[string]any{
		"token":    ch.Secret,
		"title":    msg.Title,
		"content":  strings.ReplaceAll(msg.Text, "\n", "<br/>"),
		"template": "html",
	}, "code", 200)
}
