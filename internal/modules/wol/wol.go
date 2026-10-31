// Package wol 实现网络唤醒模块：向局域网发送魔术包唤醒设备，并可通过 TCP 端口探测设备是否在线。
package wol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "wol"

// Device 是一台可唤醒的设备。
type Device struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group string   `json:"group"`
	MACs  []string `json:"macs"`
	// BroadcastIPs 为空时向 255.255.255.255 及各网卡广播地址发送
	BroadcastIPs []string `json:"broadcastIps"`
	Port         int      `json:"port"`
	Repeat       int      `json:"repeat"`
	// CheckAddr 为 host:port，用于探测设备是否在线（例如 192.168.1.10:3389），留空不探测
	CheckAddr string `json:"checkAddr"`
	Remark    string `json:"remark"`
	// Enable 为了复用通用接口保留，网络唤醒设备始终可用
	Enable bool `json:"enable"`
}

type fileConfig struct {
	Devices []Device `json:"devices"`
}

// MagicPacket 生成魔术包：6 字节 0xFF 后跟 16 次 MAC。
func MagicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil || len(hw) != 6 {
		return nil, fmt.Errorf("无效的 MAC 地址 %q", mac)
	}
	pkt := make([]byte, 0, 102)
	for i := 0; i < 6; i++ {
		pkt = append(pkt, 0xFF)
	}
	for i := 0; i < 16; i++ {
		pkt = append(pkt, hw...)
	}
	return pkt, nil
}

// Validate 校验设备配置。
func (d *Device) Validate() error {
	if err := crud.NormalizeGroup(&d.Group); err != nil {
		return err
	}
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return errors.New("设备名称不能为空")
	}
	var macs []string
	for _, m := range d.MACs {
		if m = strings.TrimSpace(m); m == "" {
			continue
		}
		hw, err := net.ParseMAC(m)
		if err != nil || len(hw) != 6 {
			return fmt.Errorf("无效的 MAC 地址 %q", m)
		}
		macs = append(macs, hw.String())
	}
	if len(macs) == 0 {
		return errors.New("至少填写一个 MAC 地址")
	}
	d.MACs = macs
	var bcs []string
	for _, b := range d.BroadcastIPs {
		if b = strings.TrimSpace(b); b == "" {
			continue
		}
		if ip := net.ParseIP(b); ip == nil || ip.To4() == nil {
			return fmt.Errorf("广播地址 %q 不是 IPv4", b)
		}
		bcs = append(bcs, b)
	}
	d.BroadcastIPs = bcs
	if d.Port == 0 {
		d.Port = 9
	}
	if d.Port < 1 || d.Port > 65535 {
		return errors.New("端口无效")
	}
	if d.Repeat <= 0 {
		d.Repeat = 3
	}
	if d.Repeat > 20 {
		d.Repeat = 20
	}
	d.CheckAddr = strings.TrimSpace(d.CheckAddr)
	if d.CheckAddr != "" {
		if _, _, err := net.SplitHostPort(d.CheckAddr); err != nil {
			return errors.New("在线探测地址格式应为 主机:端口")
		}
	}
	d.Enable = true
	return nil
}

// interfaceBroadcasts 返回各启用网卡的 IPv4 广播地址。
func interfaceBroadcasts() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP.To4()
			mask := net.IP(ipn.Mask).To4()
			if mask == nil {
				continue
			}
			bc := make(net.IP, 4)
			for i := range bc {
				bc[i] = ip[i] | ^mask[i]
			}
			out = append(out, bc.String())
		}
	}
	return out
}

// sendFunc 允许测试截获发送的数据包。
var sendFunc = func(addr string, pkt []byte) error {
	c, err := net.Dial("udp4", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Write(pkt)
	return err
}

// Wake 发送魔术包，返回实际发送的目标数量。
func Wake(d Device) (int, error) {
	targets := d.BroadcastIPs
	if len(targets) == 0 {
		targets = append([]string{"255.255.255.255"}, interfaceBroadcasts()...)
	}
	sent := 0
	var lastErr error
	for _, mac := range d.MACs {
		pkt, err := MagicPacket(mac)
		if err != nil {
			return sent, err
		}
		for i := 0; i < d.Repeat; i++ {
			for _, t := range targets {
				if err := sendFunc(net.JoinHostPort(t, strconv.Itoa(d.Port)), pkt); err != nil {
					lastErr = err
					continue
				}
				sent++
			}
		}
	}
	if sent == 0 && lastErr != nil {
		return 0, lastErr
	}
	return sent, nil
}

// Module 是网络唤醒模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	logs  *logbuf.Buffer
	cfg   fileConfig
}

// New 创建模块。
func New() *Module { return &Module{} }

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "网络唤醒" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error { return nil }

// Stop 实现 module.Module。
func (m *Module) Stop() error { return nil }

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	devices := &crud.Collection[Device]{
		Mu:       &m.mu,
		Items:    &m.cfg.Devices,
		Save:     m.saveLocked,
		Key:      func(d *Device) *string { return &d.Key },
		Enabled:  func(d *Device) *bool { return &d.Enable },
		Group:    func(d *Device) *string { return &d.Group },
		Validate: func(d *Device) error { return d.Validate() },
	}
	devices.Register(r, "/devices")
	r.HandleFunc("POST /devices/{key}/wake", m.handleWake)
	r.HandleFunc("GET /status", m.handleStatus)
}

func (m *Module) find(key string) (Device, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.cfg.Devices {
		if d.Key == key {
			return d, true
		}
	}
	return Device{}, false
}

func (m *Module) handleWake(w http.ResponseWriter, r *http.Request) {
	d, ok := m.find(r.PathValue("key"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "设备不存在")
		return
	}
	n, err := Wake(d)
	log := m.logs.Logger(d.Name)
	if err != nil {
		log.Errorf("发送唤醒包失败: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "发送失败: %v", err)
		return
	}
	log.Infof("已发送 %d 个唤醒包（%s）", n, strings.Join(d.MACs, ", "))
	httpx.OK(w, map[string]int{"sent": n})
}

// handleStatus 并发探测所有设备的在线状态。
func (m *Module) handleStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	devices := append([]Device{}, m.cfg.Devices...)
	m.mu.Unlock()
	out := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, d := range devices {
		if d.CheckAddr == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := "offline"
			if c, err := net.DialTimeout("tcp", d.CheckAddr, 1500*time.Millisecond); err == nil {
				c.Close()
				status = "online"
			}
			mu.Lock()
			out[d.Key] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	httpx.OK(w, out)
}
