package wol

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 巴法云接入：以 MQTT 3.1.1 经 TLS 连接 bemfa.com:9503，客户端 ID 为用户私钥（无需用户名密码），订阅各设备的主题。
// 巴法云把小爱同学、小度、Google Assistant 等语音助手的开关指令发到对应主题：on 唤醒设备，off 远程关机。
// 只实现所需的最小子集：CONNECT、SUBSCRIBE（QoS 0）、接收 PUBLISH（QoS 0/1）、PINGREQ 与发布状态（QoS 0）。

// BemfaSettings 是巴法云接入设置。
type BemfaSettings struct {
	Enable bool `json:"enable"`
	// UID 为巴法云控制台中的用户私钥，作为 MQTT 客户端 ID，等同于账号凭据
	UID string `json:"uid"`
}

// 以下为连接参数，测试中替换为本地服务器。
var (
	bemfaAddr      = "bemfa.com:9503"
	bemfaTLSConfig = func() *tls.Config { return &tls.Config{ServerName: "bemfa.com", MinVersion: tls.VersionTLS12} }
	// bemfaKeepAlive 为 MQTT 保活间隔，按它的一半发送心跳，超过 1.5 倍未收到任何数据视为断线
	bemfaKeepAlive = 60 * time.Second
	// bemfaBackoffMin、bemfaBackoffMax 为断线重连的等待时间范围，连续失败时逐次加倍
	bemfaBackoffMin = 5 * time.Second
	bemfaBackoffMax = 5 * time.Minute
	// bemfaDebounce 内同一设备的同一指令只执行一次，语音助手可能重复下发
	bemfaDebounce = 10 * time.Second
)

// maxPacket 为接收的 MQTT 报文上限，开关指令只有几个字节，超出视为异常并断开。
const maxPacket = 64 << 10

// validTopic 判断巴法云主题是否只含字母、数字、下划线与连字符且不超过 64 个字符；
// 不允许 / + # 等 MQTT 主题分隔符与通配符，避免订阅到其他主题。
func validTopic(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// validUID 判断私钥格式：巴法云私钥为 32 位十六进制字符串，这里放宽为 1~128 个字母与数字。
func validUID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Validate 校验巴法云设置。
func (s *BemfaSettings) Validate() error {
	s.UID = strings.TrimSpace(s.UID)
	if s.UID != "" && !validUID(s.UID) {
		return errors.New("巴法云私钥只能包含字母与数字")
	}
	if s.Enable && s.UID == "" {
		return errors.New("请填写巴法云私钥")
	}
	return nil
}

// BemfaStatus 是巴法云连接状态。
type BemfaStatus struct {
	Connected bool `json:"connected"`
	// Topics 为已订阅的主题数
	Topics int `json:"topics"`
	// Error 为最近一次连接失败的原因，连接成功后清空
	Error string `json:"error,omitempty"`
	// Since 为进入当前状态的时间（Unix 秒）
	Since int64 `json:"since,omitempty"`
}

// bemfaClient 维持与巴法云的连接。
type bemfaClient struct {
	m *Module
	// kickCh 在设置或设备变化时通知重新检查订阅
	kickCh chan struct{}
	done   chan struct{}

	mu     sync.Mutex
	status BemfaStatus
	// conn 为当前连接，发布状态时使用；wmu 串行化写入
	conn net.Conn
	wmu  sync.Mutex
	// last 记录各设备各指令最近一次执行的时间，用于去重
	last map[string]time.Time
}

func newBemfaClient(m *Module) *bemfaClient {
	return &bemfaClient{m: m, kickCh: make(chan struct{}, 1), last: map[string]time.Time{}}
}

// kick 通知连接按最新设置调整，不阻塞。
func (b *bemfaClient) kick() {
	select {
	case b.kickCh <- struct{}{}:
	default:
	}
}

func (b *bemfaClient) start(ctx context.Context) {
	b.done = make(chan struct{})
	go func() {
		defer close(b.done)
		b.run(ctx)
	}()
}

// wait 等待连接循环退出。
func (b *bemfaClient) wait() {
	if b.done != nil {
		<-b.done
	}
}

func (b *bemfaClient) setStatus(s BemfaStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s.Connected != b.status.Connected || s.Error != b.status.Error || b.status.Since == 0 {
		s.Since = time.Now().Unix()
	} else {
		s.Since = b.status.Since
	}
	b.status = s
}

// Status 返回连接状态。
func (b *bemfaClient) Status() BemfaStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

// want 返回当前应使用的私钥与订阅主题（已排序）；未开启或没有设备设置主题时 uid 为空。
func (b *bemfaClient) want() (uid string, topics []string) {
	m := b.m
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Bemfa.Enable || m.cfg.Bemfa.UID == "" {
		return "", nil
	}
	for _, d := range m.cfg.Devices {
		if d.BemfaTopic != "" {
			topics = append(topics, d.BemfaTopic)
		}
	}
	if len(topics) == 0 {
		return "", nil
	}
	slices.Sort(topics)
	return m.cfg.Bemfa.UID, topics
}

// errResubscribe 表示设置变化，需要按新设置重新连接。
var errResubscribe = errors.New("设置已变化")

// run 为连接循环：按设置连接、订阅并处理指令，断线后按退避时间重连，设置变化时立即调整。
func (b *bemfaClient) run(ctx context.Context) {
	log := b.m.logs.Logger("巴法云")
	backoff := bemfaBackoffMin
	lastErr := ""
	for ctx.Err() == nil {
		uid, topics := b.want()
		if uid == "" {
			b.setStatus(BemfaStatus{})
			lastErr = ""
			select {
			case <-ctx.Done():
				return
			case <-b.kickCh:
			}
			continue
		}
		start := time.Now()
		err := b.session(ctx, uid, topics, log)
		if ctx.Err() != nil {
			b.setStatus(BemfaStatus{})
			return
		}
		if errors.Is(err, errResubscribe) {
			backoff = bemfaBackoffMin
			continue
		}
		// 连接维持过一段时间后断开，按首次失败重新计算退避
		if time.Since(start) > 2*bemfaKeepAlive {
			backoff = bemfaBackoffMin
		}
		msg := err.Error()
		b.setStatus(BemfaStatus{Topics: len(topics), Error: msg})
		// 相同原因的连续失败只记一次，避免断网时刷屏
		if msg != lastErr {
			log.Warnf("巴法云连接断开: %v，%s 后重试", err, backoff)
			lastErr = msg
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			b.setStatus(BemfaStatus{})
			return
		case <-b.kickCh:
			t.Stop()
			backoff = bemfaBackoffMin
		case <-t.C:
			backoff = min(backoff*2, bemfaBackoffMax)
		}
	}
}

// packet 是一个收到的 MQTT 报文。
type packet struct {
	typ   byte
	flags byte
	body  []byte
}

// session 建立一次连接并处理到断开为止。
func (b *bemfaClient) session(ctx context.Context, uid string, topics []string, log *logbuf.Logger) error {
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}, Config: bemfaTLSConfig()}
	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	c, err := d.DialContext(dctx, "tcp", bemfaAddr)
	cancel()
	if err != nil {
		return err
	}
	defer c.Close()
	// ctx 取消时关闭连接，阻塞中的读取随之返回
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()

	r := bufio.NewReader(c)
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	if err := b.write(c, connectPacket(uid, bemfaKeepAlive)); err != nil {
		return err
	}
	p, err := readPacket(r)
	if err != nil {
		return fmt.Errorf("等待连接确认失败: %w", err)
	}
	if p.typ != 2 || len(p.body) < 2 {
		return errors.New("服务器未返回连接确认")
	}
	if p.body[1] != 0 {
		return fmt.Errorf("服务器拒绝连接（返回码 %d），请检查私钥", p.body[1])
	}
	if err := b.write(c, subscribePacket(1, topics)); err != nil {
		return err
	}
	for {
		p, err := readPacket(r)
		if err != nil {
			return fmt.Errorf("等待订阅确认失败: %w", err)
		}
		if p.typ == 9 {
			// SUBACK：报文标识符之后每个主题一个返回码，0x80 为失败
			if len(p.body) < 2 || slices.Contains(p.body[2:], 0x80) {
				return errors.New("订阅主题失败，请确认已在巴法云控制台创建这些主题")
			}
			break
		}
	}
	_ = c.SetDeadline(time.Time{})
	b.mu.Lock()
	b.conn = c
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.conn = nil
		b.mu.Unlock()
	}()
	b.setStatus(BemfaStatus{Connected: true, Topics: len(topics)})
	log.Infof("已连接巴法云，订阅 %d 个主题", len(topics))

	// 读取在独立的 goroutine 中进行，主循环负责心跳与设置变化；本次连接结束时 sctx 取消，读取 goroutine 随之退出
	sctx, scancel := context.WithCancel(ctx)
	pkts := make(chan packet)
	rerr := make(chan error, 1)
	readTimeout := bemfaKeepAlive * 3 / 2
	var wg sync.WaitGroup
	wg.Add(1)
	// 返回前关闭连接并等读取 goroutine 退出，不留下后台 goroutine
	defer func() {
		_ = c.Close()
		scancel()
		wg.Wait()
	}()
	go func() {
		defer wg.Done()
		for {
			_ = c.SetReadDeadline(time.Now().Add(readTimeout))
			p, err := readPacket(r)
			if err != nil {
				rerr <- err
				return
			}
			select {
			case pkts <- p:
			case <-sctx.Done():
				return
			}
		}
	}()
	ping := time.NewTicker(readTimeout / 3)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-rerr:
			if errors.Is(err, io.EOF) {
				return errors.New("服务器关闭了连接")
			}
			return err
		case <-ping.C:
			if err := b.write(c, []byte{0xC0, 0}); err != nil {
				return err
			}
		case <-b.kickCh:
			if u, ts := b.want(); u != uid || !slices.Equal(ts, topics) {
				return errResubscribe
			}
		case p := <-pkts:
			if p.typ != 3 {
				continue
			}
			topic, payload, id, err := parsePublish(p)
			if err != nil {
				return err
			}
			// QoS 1 需要确认，否则服务器会重发
			if id != 0 {
				if err := b.write(c, []byte{0x40, 2, byte(id >> 8), byte(id)}); err != nil {
					return err
				}
			}
			// 保留消息是主题上一次的状态，连接时由服务器补发，不是新的指令，执行它会在每次重连时开关设备
			if p.flags&1 != 0 {
				continue
			}
			b.dispatch(topic, payload)
		}
	}
}

// write 串行写入一个报文，写超时 10 秒。
func (b *bemfaClient) write(c net.Conn, pkt []byte) error {
	b.wmu.Lock()
	defer b.wmu.Unlock()
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.Write(pkt)
	return err
}

// publishState 把设备状态写回巴法云（主题加 /up 只更新云端数据、不推送），语音助手与巴法 App 据此显示开关状态。
// 未连接时忽略。
func (b *bemfaClient) publishState(topic, state string) {
	b.mu.Lock()
	c := b.conn
	b.mu.Unlock()
	if c == nil || topic == "" {
		return
	}
	_ = b.write(c, publishPacket(topic+"/up", state))
}

// dispatch 处理一条开关指令：on 唤醒，off 关机；其他内容忽略。指令可带 #参数（如亮度），只取 # 之前的部分。
func (b *bemfaClient) dispatch(topic, payload string) {
	cmd, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(payload)), "#")
	if cmd != "on" && cmd != "off" {
		return
	}
	m := b.m
	var dev Device
	found := false
	m.mu.Lock()
	for _, d := range m.cfg.Devices {
		if d.BemfaTopic != "" && strings.EqualFold(d.BemfaTopic, topic) {
			dev, found = d, true
			break
		}
	}
	m.mu.Unlock()
	if !found {
		return
	}
	b.mu.Lock()
	k := dev.Key + "/" + cmd
	if t, ok := b.last[k]; ok && time.Since(t) < bemfaDebounce {
		b.mu.Unlock()
		return
	}
	b.last[k] = time.Now()
	b.mu.Unlock()
	log := m.logs.Logger(dev.Name)
	// 关机要等 SSH 执行完，放到独立 goroutine 中，不阻塞接收其他指令与心跳
	go func() {
		if cmd == "on" {
			log.Infof("收到巴法云指令 %s，唤醒设备", cmd)
			if _, err := m.wake(dev); err == nil {
				b.publishState(dev.BemfaTopic, "on")
			}
			return
		}
		if dev.ShutdownConn == "" {
			log.Warnf("收到巴法云关机指令，但该设备未配置远程关机")
			return
		}
		log.Infof("收到巴法云指令 %s，远程关机", cmd)
		if _, err := m.shutdown(context.Background(), dev); err == nil {
			b.publishState(dev.BemfaTopic, "off")
		}
	}()
}

// ---- MQTT 3.1.1 报文编解码 ----

// appendRemaining 追加剩余长度（变长编码）。
func appendRemaining(b []byte, n int) []byte {
	for {
		d := byte(n % 128)
		n /= 128
		if n > 0 {
			d |= 0x80
		}
		b = append(b, d)
		if n == 0 {
			return b
		}
	}
}

func appendString(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

// connectPacket 生成 CONNECT：清除会话，客户端 ID 为私钥，不带用户名密码。
func connectPacket(clientID string, keepAlive time.Duration) []byte {
	var body []byte
	body = appendString(body, "MQTT")
	body = append(body, 4, 0x02)
	body = binary.BigEndian.AppendUint16(body, uint16(keepAlive/time.Second))
	body = appendString(body, clientID)
	return append(appendRemaining([]byte{0x10}, len(body)), body...)
}

// subscribePacket 生成 SUBSCRIBE，全部主题以 QoS 0 订阅。
func subscribePacket(id uint16, topics []string) []byte {
	body := binary.BigEndian.AppendUint16(nil, id)
	for _, t := range topics {
		body = appendString(body, t)
		body = append(body, 0)
	}
	return append(appendRemaining([]byte{0x82}, len(body)), body...)
}

// publishPacket 生成 QoS 0 的 PUBLISH。
func publishPacket(topic, payload string) []byte {
	body := appendString(nil, topic)
	body = append(body, payload...)
	return append(appendRemaining([]byte{0x30}, len(body)), body...)
}

// readPacket 读取一个报文，剩余长度超过 maxPacket 时报错。
func readPacket(r *bufio.Reader) (packet, error) {
	h, err := r.ReadByte()
	if err != nil {
		return packet{}, err
	}
	n, mul := 0, 1
	for i := 0; ; i++ {
		if i == 4 {
			return packet{}, errors.New("报文长度编码无效")
		}
		d, err := r.ReadByte()
		if err != nil {
			return packet{}, err
		}
		n += int(d&0x7f) * mul
		if d&0x80 == 0 {
			break
		}
		mul *= 128
	}
	if n > maxPacket {
		return packet{}, fmt.Errorf("报文过大（%d 字节）", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return packet{}, err
	}
	return packet{typ: h >> 4, flags: h & 0x0f, body: body}, nil
}

// parsePublish 解析 PUBLISH，返回主题、内容与报文标识符（QoS 0 时为 0）。
func parsePublish(p packet) (topic, payload string, id uint16, err error) {
	b := p.body
	if len(b) < 2 {
		return "", "", 0, errors.New("PUBLISH 报文无效")
	}
	tl := int(binary.BigEndian.Uint16(b))
	if len(b) < 2+tl {
		return "", "", 0, errors.New("PUBLISH 报文无效")
	}
	topic, b = string(b[2:2+tl]), b[2+tl:]
	if qos := (p.flags >> 1) & 3; qos > 0 {
		if len(b) < 2 {
			return "", "", 0, errors.New("PUBLISH 报文无效")
		}
		id, b = binary.BigEndian.Uint16(b), b[2:]
	}
	return topic, string(b), id, nil
}

// ---- 接口 ----

// bemfaView 是巴法云设置接口的输出，私钥脱敏。
type bemfaView struct {
	Settings BemfaSettings `json:"settings"`
	Status   BemfaStatus   `json:"status"`
}

func (m *Module) bemfaRoutes(r module.Router) {
	r.HandleFunc("GET /bemfa", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		s := m.cfg.Bemfa
		m.mu.Unlock()
		s.UID = crud.Mask(s.UID)
		httpx.OK(w, bemfaView{Settings: s, Status: m.bemfa.Status()})
	})
	module.DocOf(r, "GET /bemfa", module.Doc{Summary: "巴法云接入设置与连接状态", Resp: bemfaView{}})
	// 路径含 secrets 片段，按写权限鉴权，只读账号与只读令牌无法查看私钥
	r.HandleFunc("GET /bemfa/secrets", crud.SecretsHandler(&m.mu, func() map[string]string {
		return map[string]string{"uid": m.cfg.Bemfa.UID}
	}))
	module.DocOf(r, "GET /bemfa/secrets", module.Doc{Summary: "查看巴法云私钥明文", Resp: map[string]string{}})
	r.HandleFunc("PUT /bemfa", func(w http.ResponseWriter, req *http.Request) {
		var s BemfaSettings
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		m.mu.Lock()
		crud.KeepSecret(m.cfg.Bemfa.UID, &s.UID)
		if err := s.Validate(); err != nil {
			m.mu.Unlock()
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		old := m.cfg.Bemfa
		m.cfg.Bemfa = s
		if err := m.saveLocked(); err != nil {
			m.cfg.Bemfa = old
			m.mu.Unlock()
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		m.mu.Unlock()
		m.bemfa.kick()
		s.UID = crud.Mask(s.UID)
		httpx.OK(w, bemfaView{Settings: s, Status: m.bemfa.Status()})
	})
	module.DocOf(r, "PUT /bemfa", module.Doc{Summary: "修改巴法云接入设置", Req: BemfaSettings{}, Resp: bemfaView{}})
}
