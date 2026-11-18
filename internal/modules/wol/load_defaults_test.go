package wol

import (
	"io"
	"log"
	"sync"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// initFromRaw 把原始 JSON 结构写入配置目录后初始化模块，模拟手工编辑或外部工具生成的配置文件。
func initFromRaw(t *testing.T, raw map[string]any) *Module {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ID, raw); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(10, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// captureSends 截获发出的唤醒包，返回目标地址列表的读取函数。
func captureSends(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var sent []string
	old := sendFunc
	sendFunc = func(addr string, pkt []byte) error {
		mu.Lock()
		sent = append(sent, addr)
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() { sendFunc = old })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), sent...)
	}
}

// 配置文件缺端口与重复次数时加载后补上默认值，否则重复次数 0 时一个包都不发却显示成功。
func TestInitFillsMissingDeviceDefaults(t *testing.T) {
	sent := captureSends(t)
	m := initFromRaw(t, map[string]any{"devices": []any{
		map[string]any{"key": "d1", "name": "pc", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:ff"}, "broadcastIps": []string{"192.168.1.255"}},
		// 旧配置只填了 SSH 连接、没有关机方式
		map[string]any{"key": "d2", "name": "nas", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:01"}, "shutdownConn": "c1"},
		// HTTP 关机未指定请求方法，有请求体时按 POST
		map[string]any{"key": "d3", "name": "box", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:02"}, "shutdownMode": "http",
			"shutdownHttp": map[string]any{"url": "http://192.168.1.2/off", "body": `{"a":1}`}},
	}})
	d := m.cfg.Devices[0]
	if d.Port != 9 || d.Repeat != 3 {
		t.Fatalf("未补默认值: port=%d repeat=%d", d.Port, d.Repeat)
	}
	n, err := Wake(d)
	got := sent()
	if err != nil || n != 3 || len(got) != 3 {
		t.Fatalf("应发送 3 个唤醒包: n=%d err=%v sent=%v", n, err, got)
	}
	for _, a := range got {
		if a != "192.168.1.255:9" {
			t.Fatalf("目标地址不符: %v", got)
		}
	}
	if m.cfg.Devices[1].ShutdownMode != ModeSSH {
		t.Fatalf("旧配置的关机方式未补全: %q", m.cfg.Devices[1].ShutdownMode)
	}
	if m.cfg.Devices[2].ShutdownHTTP.Method != "POST" {
		t.Fatalf("HTTP 关机请求方法未补默认值: %q", m.cfg.Devices[2].ShutdownHTTP.Method)
	}
}

// 已有的值保持不变；超过上限的重复次数与保存时一样收拢到 20。
func TestInitKeepsExistingDeviceValues(t *testing.T) {
	m := initFromRaw(t, map[string]any{"devices": []any{
		map[string]any{"key": "d1", "name": "pc", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:ff"}, "port": 7, "repeat": 5},
		map[string]any{"key": "d2", "name": "many", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:01"}, "repeat": 99},
		map[string]any{"key": "d3", "name": "box", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:02"}, "shutdownMode": "http",
			"shutdownHttp": map[string]any{"url": "http://192.168.1.2/off", "method": "PUT"}},
		// 没有选择关机方式时不补 HTTP 请求方法
		map[string]any{"key": "d4", "name": "plain", "enable": true, "macs": []string{"aa:bb:cc:dd:ee:03"}},
	}})
	if d := m.cfg.Devices[0]; d.Port != 7 || d.Repeat != 5 {
		t.Fatalf("已有值被改动: port=%d repeat=%d", d.Port, d.Repeat)
	}
	if r := m.cfg.Devices[1].Repeat; r != 20 {
		t.Fatalf("重复次数应收拢到 20: %d", r)
	}
	if mth := m.cfg.Devices[2].ShutdownHTTP.Method; mth != "PUT" {
		t.Fatalf("HTTP 请求方法被改动: %q", mth)
	}
	if d := m.cfg.Devices[3]; d.ShutdownMode != "" || d.ShutdownHTTP.Method != "" {
		t.Fatalf("未选关机方式的设备不应补关机设置: %+v", d)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameDeviceDefaults(t *testing.T) {
	d := Device{Name: "pc", MACs: []string{"aa:bb:cc:dd:ee:ff"}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if d.Port != 9 || d.Repeat != 3 {
		t.Fatalf("保存时默认值不符: port=%d repeat=%d", d.Port, d.Repeat)
	}
	h := Device{Name: "box", MACs: []string{"aa:bb:cc:dd:ee:02"}, ShutdownMode: ModeHTTP, ShutdownHTTP: HTTPAction{URL: "http://192.168.1.2/off"}}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if h.ShutdownHTTP.Method != "GET" {
		t.Fatalf("无请求体时应为 GET: %q", h.ShutdownHTTP.Method)
	}
}
