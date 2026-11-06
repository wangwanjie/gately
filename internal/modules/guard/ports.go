package guard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// portRange 是闭区间 [lo, hi] 的端口范围。
type portRange struct {
	lo, hi int
}

func (p portRange) String() string {
	if p.lo == p.hi {
		return strconv.Itoa(p.lo)
	}
	return fmt.Sprintf("%d-%d", p.lo, p.hi)
}

// parsePorts 解析逗号、空白或换行分隔的端口与范围（如 23,3389,5900-5910），合并重叠与相邻的范围。
func parsePorts(s string) ([]portRange, error) {
	var out []portRange
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\n' || r == '\t' || r == '\r' }) {
		lo, hi, isRange := strings.Cut(f, "-")
		a, err := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		if err == nil && isRange {
			b, err = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err != nil || a < 1 || b > 65535 || a > b {
			return nil, fmt.Errorf("端口 %q 无效", f)
		}
		out = append(out, portRange{a, b})
	}
	return mergePorts(out), nil
}

// mergePorts 排序并合并重叠或相邻的范围。
func mergePorts(in []portRange) []portRange {
	if len(in) == 0 {
		return nil
	}
	sort.Slice(in, func(i, j int) bool { return in[i].lo < in[j].lo })
	out := []portRange{in[0]}
	for _, p := range in[1:] {
		last := &out[len(out)-1]
		if p.lo <= last.hi+1 {
			last.hi = max(last.hi, p.hi)
			continue
		}
		out = append(out, p)
	}
	return out
}

func formatPorts(list []portRange) string {
	parts := make([]string, 0, len(list))
	for _, p := range list {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ",")
}

// countPorts 返回范围内的端口总数。
func countPorts(list []portRange) int {
	n := 0
	for _, p := range list {
		n += p.hi - p.lo + 1
	}
	return n
}

// subtractPorts 从范围中去掉指定的端口，返回剩余范围与被去掉的端口。
func subtractPorts(list []portRange, remove map[int]bool) (rest []portRange, removed []int) {
	for _, p := range list {
		start := p.lo
		for port := p.lo; port <= p.hi; port++ {
			if !remove[port] {
				continue
			}
			removed = append(removed, port)
			if port > start {
				rest = append(rest, portRange{start, port - 1})
			}
			start = port + 1
		}
		if start <= p.hi {
			rest = append(rest, portRange{start, p.hi})
		}
	}
	return rest, removed
}

// procNetFiles 是列出 TCP 套接字的 proc 文件，可在测试中替换
var procNetFiles = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// listeningPortsFn 可在测试中替换
var listeningPortsFn = listeningPorts

// listeningPorts 读取本机网络命名空间中处于监听状态、且不只绑定在回环地址上的 TCP 端口。
func listeningPorts() (map[int]bool, error) {
	ports := map[int]bool{}
	found := false
	for _, name := range procNetFiles {
		f, err := os.Open(name)
		if err != nil {
			continue
		}
		found = true
		parseProcNet(f, ports)
		f.Close()
	}
	if !found {
		return nil, errors.New("无法读取 /proc/net/tcp")
	}
	return ports, nil
}

// tcpListen 是 /proc/net/tcp 中 LISTEN 状态的编码
const tcpListen = "0A"

// parseProcNet 解析 /proc/net/tcp{,6} 的内容，把非回环地址上的监听端口加入 ports。
func parseProcNet(f io.Reader, ports map[int]bool) {
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[3] != tcpListen {
			continue
		}
		addr, portHex, ok := strings.Cut(fields[1], ":")
		if !ok || isLoopbackHex(addr) {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err == nil && port > 0 {
			ports[int(port)] = true
		}
	}
}

// isLoopbackHex 判断 /proc/net/tcp 中十六进制编码的地址是否为回环地址。
// IPv4 按主机字节序（小端）存放，127.x.x.x 的最后一个字节为 7F；IPv6 的 ::1 为 00000000000000000000000001000000。
func isLoopbackHex(addr string) bool {
	switch len(addr) {
	case 8:
		return strings.HasSuffix(strings.ToUpper(addr), "7F")
	case 32:
		up := strings.ToUpper(addr)
		// IPv4 映射的 ::ffff:127.x.x.x
		if strings.HasPrefix(up, "0000000000000000FFFF0000") {
			return strings.HasSuffix(up, "7F")
		}
		return up == "00000000000000000000000001000000"
	}
	return false
}
