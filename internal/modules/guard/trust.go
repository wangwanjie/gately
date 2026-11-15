package guard

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/trustsrc"
)

// 可信来源（见 trustsrc）：模块确认过身份的来源地址（如用正确令牌登录 frps 的 frpc）。
// 家宽出口地址由局域网里的多台设备共用，其中一台做 TCP 打洞时会在短时间内连接服务器的大量未开放端口，
// 扫描检测会把整个出口地址封掉，同一地址上的 frpc 随之断开、无法重连。
// 可信来源加入 input 链的放行集合，只免于诱饵端口与扫描检测；日志规则与其他封禁照常生效。

// establishedPeersFn 可在测试中替换
var establishedPeersFn = establishedPeers

// refreshTrustLocked 按模块登记的可信来源重新计算当前有效的部分，变化时重写放行集合。调用方持有 f.mu。
// 来源在登记的期限内有效；过了期限后，只在它与登记的本机端口之间还有已建立的 TCP 连接时有效。
// 读不到连接状态时只按期限判断。
func (f *firewall) refreshTrustLocked(ctx context.Context) {
	trusted, lapsed := effectiveTrusted(trustsrc.All(), time.Now(), f.trustLapsed)
	f.trustLapsed = lapsed
	text := strings.Join(prefixStrings(trusted), ",")
	if text == f.trustText {
		return
	}
	f.trusted, f.trustText = trusted, text
	if text == "" {
		f.log.Infof("已没有免于诱饵端口与扫描检测的可信来源")
	} else {
		f.log.Infof("免于诱饵端口与扫描检测的可信来源（已登录 frps 的 frpc 等）: %s", strings.Join(prefixStrings(trusted), "、"))
	}
	f.rewriteAllowLocked(ctx)
}

// refreshTrust 在收到可信来源变化通知时立即刷新。
func (f *firewall) refreshTrust(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshTrustLocked(ctx)
}

// trustKey 标识一次登记：同一来源重新登录后期限变化，视为新的登记
type trustKey struct {
	addr  netip.Addr
	port  int
	until int64
}

// effectiveTrusted 返回 list 中当前有效的来源（单个地址的网段，已排序去重），以及已失效的登记（见下）。
// 过了期限的来源只在与登记端口之间还有已建立的连接时有效；某次检查发现连接已断开后，这次登记就此失效（记入 lapsed），
// 之后即使该地址重新建立连接也不再恢复，直到模块重新登记（如 frpc 重新登录、期限变化）。
// 否则家宽地址被运营商分配给别人后，新的持有者只要连上公开的 frps 端口、不必知道令牌，就能免于扫描检测。
// lapsed 为上一次的结果，返回值只保留仍在登记中的部分，不会随登记变化无限增长。
func effectiveTrusted(list []trustsrc.Source, now time.Time, lapsed map[trustKey]bool) ([]netip.Prefix, map[trustKey]bool) {
	if len(list) == 0 {
		return nil, nil
	}
	keyOf := func(s trustsrc.Source) trustKey {
		return trustKey{addr: s.Addr.Unmap(), port: s.Port, until: s.Until.UnixNano()}
	}
	// 只有过了期限、尚未失效的来源需要按连接判断；都不需要时不读取连接状态
	ports := map[int]bool{}
	for _, s := range list {
		if s.Port > 0 && !now.Before(s.Until) && !lapsed[keyOf(s)] {
			ports[s.Port] = true
		}
	}
	var peers map[int]map[netip.Addr]bool
	peersOK := false
	if len(ports) > 0 {
		var err error
		peers, err = establishedPeersFn(ports)
		peersOK = err == nil
	}
	var out []netip.Prefix
	next := map[trustKey]bool{}
	for _, s := range list {
		a := s.Addr.Unmap()
		if !a.IsValid() {
			continue
		}
		if now.Before(s.Until) {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		if s.Port <= 0 {
			continue
		}
		k := keyOf(s)
		if lapsed[k] {
			next[k] = true
			continue
		}
		if peers[s.Port][a] {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		} else if peersOK {
			// 读到了连接状态且连接已断开：这次登记失效。读取失败时无法确认，下次再判断
			next[k] = true
		}
	}
	if len(next) == 0 {
		next = nil
	}
	return mergePrefixes(out), next
}

// trustedFilterLocked 包装导入内核封禁的回调，跳过可信来源：内核规则可能在放行集合更新之前
// 把刚登录的来源加入了临时集合，这些地址不写入封禁表（临时集合中的元素随后照常删除）。调用方持有 f.mu。
func (f *firewall) trustedFilterLocked(fn func(string, []netip.Addr, portHits)) func(string, []netip.Addr, portHits) {
	if fn == nil || len(f.trusted) == 0 {
		return fn
	}
	// IPv6 的内核封禁按 /64 记录，放宽到与放行集合相同的粒度再判断
	wide := allowPrefixes(f.trusted)
	return func(source string, addrs []netip.Addr, hits portHits) {
		kept := make([]netip.Addr, 0, len(addrs))
		for _, a := range addrs {
			if !prefixesContain(wide, a) {
				kept = append(kept, a)
			}
		}
		if len(kept) > 0 {
			fn(source, kept, hits)
		}
	}
}

func prefixesContain(list []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range list {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// tcpEstablished 是 /proc/net/tcp 中 ESTABLISHED 状态的编码
const tcpEstablished = "01"

// establishedPeers 读取与本机 ports 中各端口之间有已建立连接的远端地址，按本机端口分组。
func establishedPeers(ports map[int]bool) (map[int]map[netip.Addr]bool, error) {
	out := map[int]map[netip.Addr]bool{}
	found := false
	for _, name := range procNetFiles {
		fh, err := os.Open(name)
		if err != nil {
			continue
		}
		found = true
		parseEstablished(fh, ports, out)
		fh.Close()
	}
	if !found {
		return nil, errors.New("无法读取 /proc/net/tcp")
	}
	return out, nil
}

// parseEstablished 解析 /proc/net/tcp{,6} 的内容，把本机端口在 ports 中的已建立连接的远端地址加入 out。
func parseEstablished(r io.Reader, ports map[int]bool, out map[int]map[netip.Addr]bool) {
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[3] != tcpEstablished {
			continue
		}
		_, localPort, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(localPort, 16, 16)
		if err != nil || !ports[int(port)] {
			continue
		}
		remote, _, ok := strings.Cut(fields[2], ":")
		if !ok {
			continue
		}
		a, ok := parseProcAddr(remote)
		if !ok {
			continue
		}
		if out[int(port)] == nil {
			out[int(port)] = map[netip.Addr]bool{}
		}
		out[int(port)][a] = true
	}
}

// parseProcAddr 解析 /proc/net/tcp{,6} 中十六进制编码的地址：按 32 位字存放，每个字为主机字节序
// （常见的 x86、ARM 为小端，部分 MIPS 路由器为大端）。IPv4 映射的 IPv6 地址展开为 IPv4。
func parseProcAddr(s string) (netip.Addr, bool) {
	raw, err := hex.DecodeString(s)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.Addr{}, false
	}
	for i := 0; i+4 <= len(raw); i += 4 {
		// 十六进制文本是按主机字节序打印的 32 位整数，转成网络字节序
		binary.BigEndian.PutUint32(raw[i:], procWord(raw[i:i+4]))
	}
	a, ok := netip.AddrFromSlice(raw)
	if !ok {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// procWord 把十六进制解码得到的 4 个字节还原为内核中的 32 位整数：
// 内核用 %08X 打印主机字节序的整数，解码结果是该整数的大端表示，在内存中按主机字节序取值即为原地址的字节。
func procWord(b []byte) uint32 {
	v := binary.BigEndian.Uint32(b)
	var buf [4]byte
	binary.NativeEndian.PutUint32(buf[:], v)
	return binary.BigEndian.Uint32(buf[:])
}
