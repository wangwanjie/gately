package wol

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"math/big"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBroker 是测试用的 MQTT 服务端：记录每次连接收到的 CONNECT 客户端 ID 与订阅主题，
// 测试经 send 向当前连接下发报文，经 got 读取客户端发来的报文。
type fakeBroker struct {
	t      *testing.T
	ln     net.Listener
	pool   *x509.CertPool
	connCh chan *brokerConn
	// connack 为 CONNACK 返回码
	mu      sync.Mutex
	connack byte
}

type brokerConn struct {
	c        net.Conn
	clientID string
	topics   []string
	// pkts 为订阅完成后收到的报文
	pkts chan packet
}

func selfSignedTLS(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "bemfa.com"}, DNSNames: []string{"bemfa.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// startBroker 启动服务端并把客户端的连接参数指向它，测试结束时恢复。
func startBroker(t *testing.T) *fakeBroker {
	t.Helper()
	cert, pool := selfSignedTLS(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBroker{t: t, ln: ln, pool: pool, connCh: make(chan *brokerConn, 8)}
	oldAddr, oldTLS, oldKA, oldMin, oldDeb := bemfaAddr, bemfaTLSConfig, bemfaKeepAlive, bemfaBackoffMin, bemfaDebounce
	bemfaAddr = ln.Addr().String()
	bemfaTLSConfig = func() *tls.Config {
		return &tls.Config{ServerName: "bemfa.com", RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	bemfaKeepAlive, bemfaBackoffMin, bemfaDebounce = 2*time.Second, 50*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() {
		ln.Close()
		bemfaAddr, bemfaTLSConfig, bemfaKeepAlive, bemfaBackoffMin, bemfaDebounce = oldAddr, oldTLS, oldKA, oldMin, oldDeb
	})
	go b.serve()
	return b
}

func (b *fakeBroker) serve() {
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		go b.handle(c)
	}
}

func (b *fakeBroker) handle(c net.Conn) {
	r := bufio.NewReader(c)
	p, err := readPacket(r)
	if err != nil || p.typ != 1 {
		c.Close()
		return
	}
	// CONNECT：协议名、级别、标志、保活后为客户端 ID
	body := p.body
	pl := int(binary.BigEndian.Uint16(body))
	body = body[2+pl+4:]
	idLen := int(binary.BigEndian.Uint16(body))
	bc := &brokerConn{c: c, clientID: string(body[2 : 2+idLen]), pkts: make(chan packet, 32)}
	b.mu.Lock()
	code := b.connack
	b.mu.Unlock()
	_, _ = c.Write([]byte{0x20, 2, 0, code})
	if code != 0 {
		c.Close()
		b.connCh <- bc
		return
	}
	p, err = readPacket(r)
	if err != nil || p.typ != 8 {
		c.Close()
		return
	}
	id := p.body[:2]
	rest := p.body[2:]
	for len(rest) > 2 {
		n := int(binary.BigEndian.Uint16(rest))
		bc.topics = append(bc.topics, string(rest[2:2+n]))
		rest = rest[2+n+1:]
	}
	ack := []byte{0x90, byte(2 + len(bc.topics)), id[0], id[1]}
	for range bc.topics {
		ack = append(ack, 0)
	}
	_, _ = c.Write(ack)
	b.connCh <- bc
	for {
		p, err := readPacket(r)
		if err != nil {
			close(bc.pkts)
			return
		}
		bc.pkts <- p
	}
}

// next 等待下一次连接。
func (b *fakeBroker) next() *brokerConn {
	b.t.Helper()
	select {
	case c := <-b.connCh:
		return c
	case <-time.After(5 * time.Second):
		b.t.Fatal("等待客户端连接超时")
		return nil
	}
}

// publish 下发一条消息；qos 为 1 时带报文标识符 7，retain 设置保留标志。
func (bc *brokerConn) publish(topic, payload string, qos byte, retain bool) {
	h := byte(0x30) | qos<<1
	if retain {
		h |= 1
	}
	body := appendString(nil, topic)
	if qos > 0 {
		body = append(body, 0, 7)
	}
	body = append(body, payload...)
	_, _ = bc.c.Write(append(appendRemaining([]byte{h}, len(body)), body...))
}

// waitPacket 等待客户端发来指定类型的报文，返回它；其他类型（如心跳）跳过。
func (bc *brokerConn) waitPacket(t *testing.T, typ byte) packet {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case p, ok := <-bc.pkts:
			if !ok {
				t.Fatalf("等待报文 %d 时连接已关闭", typ)
			}
			if p.typ == typ {
				return p
			}
		case <-deadline:
			t.Fatalf("等待报文 %d 超时", typ)
		}
	}
}

// wakeRecorder 截获发送的魔术包，返回已发送次数的读取函数。
func wakeRecorder(t *testing.T) func() int {
	var mu sync.Mutex
	n := 0
	old := sendFunc
	sendFunc = func(string, []byte) error {
		mu.Lock()
		n++
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() { sendFunc = old })
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待%s超时", what)
}

func TestBemfaPackets(t *testing.T) {
	for _, n := range []int{0, 127, 128, 16383, 16384, 2097151} {
		b := appendRemaining([]byte{0x30}, n)
		pkt := append(b, make([]byte, n)...)
		if n > maxPacket {
			if _, err := readPacket(bufio.NewReader(strings.NewReader(string(pkt)))); err == nil {
				t.Fatalf("%d 字节应被拒绝", n)
			}
			continue
		}
		p, err := readPacket(bufio.NewReader(strings.NewReader(string(pkt))))
		if err != nil || len(p.body) != n || p.typ != 3 {
			t.Fatalf("长度 %d 解码错误: %v", n, err)
		}
	}
	// 剩余长度超过 4 字节编码
	if _, err := readPacket(bufio.NewReader(strings.NewReader("\x30\xff\xff\xff\xff\x01"))); err == nil {
		t.Fatal("无效的长度编码应报错")
	}
	p, _ := readPacket(bufio.NewReader(strings.NewReader(string(publishPacket("pc001/up", "on")))))
	if topic, payload, id, err := parsePublish(p); err != nil || topic != "pc001/up" || payload != "on" || id != 0 {
		t.Fatalf("PUBLISH 编解码错误: %q %q %d %v", topic, payload, id, err)
	}
	for _, bad := range [][]byte{{}, {0}, {0, 9, 'a'}} {
		if _, _, _, err := parsePublish(packet{typ: 3, body: bad}); err == nil {
			t.Fatalf("截断的 PUBLISH 应报错: %v", bad)
		}
	}
	if _, _, _, err := parsePublish(packet{typ: 3, flags: 2, body: []byte{0, 1, 'a', 0}}); err == nil {
		t.Fatal("QoS 1 缺少报文标识符应报错")
	}
}

// 主题只允许字母、数字、下划线与连字符，不允许 MQTT 分隔符与通配符。
func TestBemfaTopicValidate(t *testing.T) {
	for topic, ok := range map[string]bool{"pc001": true, "My_PC-006": true, "a/b": false, "a+": false, "#": false, "中文": false, strings.Repeat("a", 65): false} {
		d := Device{Name: "pc", MACs: []string{"aa:bb:cc:dd:ee:ff"}, BemfaTopic: topic}
		if err := d.Validate(); (err == nil) != ok {
			t.Fatalf("主题 %q: %v", topic, err)
		}
	}
}

func TestBemfaSettingsValidate(t *testing.T) {
	cases := []struct {
		s  BemfaSettings
		ok bool
	}{
		{BemfaSettings{}, true},
		{BemfaSettings{Enable: true}, false},
		{BemfaSettings{Enable: true, UID: " 4d9ec352e0376f2110a0c601a2857225 "}, true},
		{BemfaSettings{UID: "abc&topic=x"}, false},
		{BemfaSettings{UID: strings.Repeat("a", 129)}, false},
	}
	for _, tc := range cases {
		s := tc.s
		if err := s.Validate(); (err == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc.s, err)
		}
	}
}

// 完整流程：开启后以私钥连接并订阅设备主题；on 唤醒并写回状态；off 关机（QoS 1 时确认）；
// 保留消息不执行；去重时间内重复的指令只执行一次；修改主题后重新订阅；停用后断开。
func TestBemfaFlow(t *testing.T) {
	broker := startBroker(t)
	sent := wakeRecorder(t)
	var mu sync.Mutex
	var shutdowns int
	exec := func(ctx context.Context, conn, cmd string) (string, error) {
		mu.Lock()
		shutdowns++
		mu.Unlock()
		return "", nil
	}
	m, srv, _ := newTestModule(t, exec)
	pc := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "broadcastIps": []string{"192.168.1.255"},
		"repeat": 1, "shutdownConn": "c1", "shutdownCommand": "poweroff", "bemfaTopic": "pc001"})
	addDevice(t, srv, map[string]any{"name": "nas", "macs": []string{"aa:bb:cc:dd:ee:01"}, "bemfaTopic": "nas006"})

	const uid = "4d9ec352e0376f2110a0c601a2857225"
	if code, body := call(t, srv, "PUT", "/bemfa", map[string]any{"enable": true, "uid": uid}); code != 200 || strings.Contains(body, uid) {
		t.Fatalf("保存设置失败或私钥未脱敏: %d %s", code, body)
	}
	bc := broker.next()
	if bc.clientID != uid || !slices.Equal(bc.topics, []string{"nas006", "pc001"}) {
		t.Fatalf("连接参数错误: %q %v", bc.clientID, bc.topics)
	}
	waitFor(t, "状态变为已连接", func() bool { s := m.bemfa.Status(); return s.Connected && s.Topics == 2 })
	if code, body := call(t, srv, "GET", "/bemfa", nil); code != 200 || !strings.Contains(body, `"connected":true`) || strings.Contains(body, uid) {
		t.Fatalf("状态接口错误: %d %s", code, body)
	}
	if code, body := call(t, srv, "GET", "/bemfa/secrets", nil); code != 200 || !strings.Contains(body, uid) {
		t.Fatalf("查看私钥失败: %d %s", code, body)
	}

	// 保留消息是上一次的状态，不执行
	bc.publish("pc001", "on", 0, true)
	bc.publish("pc001", "on", 0, false)
	waitFor(t, "唤醒", func() bool { return sent() == 1 })
	up := bc.waitPacket(t, 3)
	if topic, payload, _, _ := parsePublish(up); topic != "pc001/up" || payload != "on" {
		t.Fatalf("应写回开启状态: %q %q", topic, payload)
	}
	// 去重时间内重复的指令忽略；其他内容忽略
	bc.publish("pc001", "on", 0, false)
	bc.publish("pc001", "toggle", 0, false)
	bc.publish("unknown001", "on", 0, false)
	time.Sleep(100 * time.Millisecond)
	if sent() != 1 {
		t.Fatalf("重复指令不应再次唤醒: %d", sent())
	}
	time.Sleep(bemfaDebounce)
	bc.publish("PC001", "on#50", 0, false)
	waitFor(t, "第二次唤醒", func() bool { return sent() == 2 })

	// off 关机；QoS 1 的消息要确认
	bc.publish("pc001", "off", 1, false)
	ack := bc.waitPacket(t, 4)
	if len(ack.body) != 2 || ack.body[1] != 7 {
		t.Fatalf("PUBACK 错误: %v", ack.body)
	}
	waitFor(t, "关机", func() bool { mu.Lock(); defer mu.Unlock(); return shutdowns == 1 })
	// 未配置关机的设备收到 off 只记日志
	bc.publish("nas006", "off", 0, false)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if shutdowns != 1 {
		t.Fatalf("未配置关机的设备不应执行: %d", shutdowns)
	}
	mu.Unlock()

	// 修改主题后按新主题重新订阅；只改名称不重新连接
	if code, body := call(t, srv, "PUT", "/devices/"+pc, map[string]any{"name": "pc2", "macs": []string{"aa:bb:cc:dd:ee:ff"},
		"shutdownConn": "c1", "shutdownCommand": "poweroff", "bemfaTopic": "pc001"}); code != 200 {
		t.Fatalf("修改设备失败: %d %s", code, body)
	}
	select {
	case c := <-broker.connCh:
		t.Fatalf("主题未变不应重新连接: %v", c.topics)
	case <-time.After(200 * time.Millisecond):
	}
	if code, body := call(t, srv, "PUT", "/devices/"+pc, map[string]any{"name": "pc2", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "desk001"}); code != 200 {
		t.Fatalf("修改设备失败: %d %s", code, body)
	}
	bc2 := broker.next()
	if !slices.Equal(bc2.topics, []string{"desk001", "nas006"}) {
		t.Fatalf("应按新主题订阅: %v", bc2.topics)
	}

	// 停用后断开
	if code, _ := call(t, srv, "PUT", "/bemfa", map[string]any{"enable": false, "uid": "********"}); code != 200 {
		t.Fatal("停用失败")
	}
	waitFor(t, "断开", func() bool { return !m.bemfa.Status().Connected })
	m.mu.Lock()
	kept := m.cfg.Bemfa.UID
	m.mu.Unlock()
	if kept != uid {
		t.Fatalf("提交占位符应保留原私钥: %q", kept)
	}
}

// 主题不能被两台设备同时使用，复制设备时同样检查。
func TestBemfaTopicUnique(t *testing.T) {
	_, srv, _ := newTestModule(t, nil)
	key := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc001"})
	if code, body := call(t, srv, "POST", "/devices", map[string]any{"name": "pc-b", "macs": []string{"aa:bb:cc:dd:ee:02"}, "bemfaTopic": "PC001"}); code != 400 || !strings.Contains(body, "已被设备") {
		t.Fatalf("重复主题应拒绝: %d %s", code, body)
	}
	// 修改自身保留原主题不算冲突
	if code, body := call(t, srv, "PUT", "/devices/"+key, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc001"}); code != 200 {
		t.Fatalf("修改自身应允许: %d %s", code, body)
	}
	if code, body := call(t, srv, "POST", "/devices/"+key+"/copy", map[string]any{"name": "pc copy", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc001"}); code != 400 {
		t.Fatalf("复制时沿用主题应拒绝: %d %s", code, body)
	}
	if code, body := call(t, srv, "POST", "/devices/"+key+"/copy", map[string]any{"name": "pc copy", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc002"}); code != 200 {
		t.Fatalf("复制时换主题应允许: %d %s", code, body)
	}
}

// 服务器拒绝连接时显示原因并按退避重试，相同原因只记一次日志。
func TestBemfaRejected(t *testing.T) {
	broker := startBroker(t)
	broker.mu.Lock()
	broker.connack = 5
	broker.mu.Unlock()
	m, srv, logs := newTestModule(t, nil)
	addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc001"})
	if code, _ := call(t, srv, "PUT", "/bemfa", map[string]any{"enable": true, "uid": "abc123"}); code != 200 {
		t.Fatal("保存失败")
	}
	broker.next()
	broker.next()
	waitFor(t, "显示错误", func() bool { return strings.Contains(m.bemfa.Status().Error, "返回码 5") })
	n := 0
	for _, e := range logs.Since(0) {
		if strings.Contains(e.Message, "返回码 5") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("相同原因应只记一次日志: %d", n)
	}
}

// 证书校验不通过时不发送私钥。
func TestBemfaVerifiesCertificate(t *testing.T) {
	broker := startBroker(t)
	bemfaTLSConfig = func() *tls.Config { return &tls.Config{ServerName: "bemfa.com", MinVersion: tls.VersionTLS12} }
	m, srv, _ := newTestModule(t, nil)
	addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc001"})
	if code, _ := call(t, srv, "PUT", "/bemfa", map[string]any{"enable": true, "uid": "abc123"}); code != 200 {
		t.Fatal("保存失败")
	}
	waitFor(t, "证书错误", func() bool { return strings.Contains(m.bemfa.Status().Error, "certificate") })
	select {
	case c := <-broker.connCh:
		t.Fatalf("证书不可信时不应发送私钥: %q", c.clientID)
	case <-time.After(100 * time.Millisecond):
	}
}

// 按保活间隔的一半发送心跳；服务器超过 1.5 倍保活间隔没有任何数据时断开重连。
func TestBemfaKeepAlive(t *testing.T) {
	broker := startBroker(t)
	bemfaKeepAlive = 200 * time.Millisecond
	_, srv, _ := newTestModule(t, nil)
	addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "bemfaTopic": "pc001"})
	if code, _ := call(t, srv, "PUT", "/bemfa", map[string]any{"enable": true, "uid": "abc123"}); code != 200 {
		t.Fatal("保存失败")
	}
	bc := broker.next()
	if p := bc.waitPacket(t, 12); len(p.body) != 0 {
		t.Fatalf("PINGREQ 格式错误: %v", p.body)
	}
	// 测试服务端不回复心跳，客户端应判定断线并重新连接
	broker.next()
}
