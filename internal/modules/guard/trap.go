package guard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/openports"
)

// maxUserTrapPorts 是用户态诱饵端口的数量上限，避免填写大范围时占用过多套接字
const maxUserTrapPorts = 64

// userTrap 在防火墙不可用（或内核不支持重定向）时由 Gately 自己监听诱饵端口：完成握手的连接即被封禁。
// 防火墙规则生效时诱饵端口的连接由内核重定向给 Gately 确认握手（见 handshake.go），同样只封禁完成握手的来源。
type userTrap struct {
	// wanted 为要求监听的端口（含监听失败的），用于判断端口表是否变化
	wanted []int
	cancel context.CancelFunc
	wg     sync.WaitGroup
	ports  []int
	errs   map[int]string
}

// startUserTrap 在指定端口上监听，onHit 在有连接时调用。
func startUserTrap(ctx context.Context, ports []int, log *logbuf.Logger, onHit func(netip.Addr, int)) *userTrap {
	ctx, cancel := context.WithCancel(ctx)
	t := &userTrap{wanted: ports, cancel: cancel, errs: map[int]string{}}
	var lc net.ListenConfig
	for _, port := range ports {
		ln, err := lc.Listen(ctx, "tcp", ":"+strconv.Itoa(port))
		if err != nil {
			t.errs[port] = err.Error()
			log.Warnf("诱饵端口 %d 监听失败: %v", port, err)
			continue
		}
		t.ports = append(t.ports, port)
		t.wg.Add(2)
		go func() {
			defer t.wg.Done()
			<-ctx.Done()
			ln.Close()
		}()
		go func() {
			defer t.wg.Done()
			for {
				c, err := ln.Accept()
				if err != nil {
					if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
						return
					}
					continue
				}
				if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil {
					onHit(ap.Addr().Unmap(), port)
				}
				c.Close()
			}
		}()
	}
	sort.Ints(t.ports)
	if len(t.ports) > 0 {
		log.Infof("防火墙不可用，改由 Gately 监听 %d 个诱饵端口", len(t.ports))
	}
	return t
}

func (t *userTrap) stop() {
	t.cancel()
	t.wg.Wait()
}

// listening 返回实际监听中的端口（创建后不再变化）。
func (t *userTrap) listening() []int {
	return append([]int{}, t.ports...)
}

// applyTrapLocked 按当前状态启停用户态诱饵端口：防火墙的重定向规则生效时由内核交给 Gately 确认，不再直接监听。调用方需持有 m.mu。
func (m *Module) applyTrapLocked() {
	m.updateTrapLocked(true)
}

// refreshTrap 在端口变化后（模块登记的端口变化、定时轮询）重新计算用户态诱饵端口，端口表不变时什么都不做。
// 由防火墙轮询协程调用（见 firewall.onPorts），不持有 m.mu。
func (m *Module) refreshTrap() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateTrapLocked(false)
}

// userTrapPortsLocked 计算用户态诱饵要监听的端口：配置的诱饵端口去掉正在监听（含宽限，不含诱饵自己的监听）
// 与模块登记的端口，最多 maxUserTrapPorts 个。调用方需持有 m.mu。
func (m *Module) userTrapPortsLocked(s Settings) []int {
	ranges, _ := parsePorts(s.Trap.Ports)
	extra, _ := openports.All()
	rest, _ := subtractPortsFunc(ranges, protectedPorts(m.fw.trapListening(), extra))
	var ports []int
	for _, r := range rest {
		for p := r.lo; p <= r.hi && len(ports) < maxUserTrapPorts; p++ {
			ports = append(ports, p)
		}
	}
	return ports
}

// updateTrapLocked 按当前状态启停用户态诱饵端口；force 为 false 时要监听的端口与当前相同则保持现有监听。调用方需持有 m.mu。
func (m *Module) updateTrapLocked(force bool) {
	s := m.cfg.Settings
	var ports []int
	want := m.started && s.Trap.Enable && !m.fw.kernelTrap()
	if want {
		ports = m.userTrapPortsLocked(s)
	}
	if !force {
		if m.trap == nil && !want {
			return
		}
		if m.trap != nil && want && slices.Equal(m.trap.wanted, ports) {
			return
		}
	}
	if m.trap != nil {
		m.trap.stop()
		m.trap = nil
		m.fw.setOwnPorts(nil)
	}
	if !want {
		return
	}
	log := m.logs.Logger("诱饵端口")
	m.trap = startUserTrap(m.ctx, ports, log, func(a netip.Addr, port int) {
		if banlist.Default.Ignored(a) || m.skipPortBan(a) {
			return
		}
		m.ban(a, s.Trap.BanSeconds, fmt.Sprintf("连接诱饵端口 %d", port), banlist.SourceTrap, "诱饵端口")
	})
	// 这些端口是 Gately 自己的监听，防火墙读取监听端口时不把它们当成对外服务
	m.fw.setOwnPorts(m.trap.listening())
}
