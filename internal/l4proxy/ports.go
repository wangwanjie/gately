// Package l4proxy 是四层（TCP/UDP）中转的公共部分：端口表达式、目标选择与故障转移、PROXY 协议头、
// 带连接数限制的 TCP 监听与流量统计，供端口转发与协议分流模块共用。
package l4proxy

import (
	"fmt"
	"strconv"
	"strings"
)

// 单条规则允许展开的最大端口数，防止误填 1-65535 一次占满资源
const maxPortsPerRule = 1024

// PortPair 是一组监听端口到目标端口的映射。
type PortPair struct {
	Listen int `json:"listen"`
	Target int `json:"target"`
}

// ParsePorts 解析端口表达式，支持 "80"、"80,443"、"8000-8010" 及其组合，保持书写顺序。
func ParsePorts(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("端口不能为空")
	}
	var out []int
	for _, part := range strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == '，' || r == ' '
	}) {
		lo, hi, isRange := strings.Cut(part, "-")
		start, err := parsePort(lo)
		if err != nil {
			return nil, err
		}
		end := start
		if isRange {
			end, err = parsePort(hi)
			if err != nil {
				return nil, err
			}
			if end < start {
				return nil, fmt.Errorf("端口范围 %q 起始大于结束", part)
			}
		}
		for p := start; p <= end; p++ {
			out = append(out, p)
			if len(out) > maxPortsPerRule {
				return nil, fmt.Errorf("单条规则最多 %d 个端口", maxPortsPerRule)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("端口不能为空")
	}
	return out, nil
}

func parsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("无效端口 %q", s)
	}
	return p, nil
}

// MapPorts 计算监听端口与目标端口的映射。
// 目标端口数量必须与监听端口相同（一一对应），或只有一个（全部转发到同一端口）。
// 目标端口为空时与监听端口相同。
func MapPorts(listenSpec, targetSpec string) ([]PortPair, error) {
	listen, err := ParsePorts(listenSpec)
	if err != nil {
		return nil, fmt.Errorf("监听端口: %w", err)
	}
	var target []int
	if strings.TrimSpace(targetSpec) == "" {
		target = listen
	} else {
		target, err = ParsePorts(targetSpec)
		if err != nil {
			return nil, fmt.Errorf("目标端口: %w", err)
		}
	}
	if len(target) != 1 && len(target) != len(listen) {
		return nil, fmt.Errorf("目标端口数量(%d)需与监听端口数量(%d)相同或只填一个", len(target), len(listen))
	}
	pairs := make([]PortPair, len(listen))
	for i, lp := range listen {
		tp := target[0]
		if len(target) > 1 {
			tp = target[i]
		}
		pairs[i] = PortPair{Listen: lp, Target: tp}
	}
	return pairs, nil
}
