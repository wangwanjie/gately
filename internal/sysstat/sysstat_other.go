//go:build !linux && !darwin

package sysstat

import "time"

// 其他平台只提供 Go 运行时与网卡地址等通用信息。
const clockTicks = 100

func readRaw() rawSample {
	return rawSample{at: time.Now()}
}

func fillPlatform(s *Snapshot) {}

func fillProcessOS(p *Process) {}

// readChildren 在其他平台上不读取托管进程的资源占用，只列出进程。
func readChildren(pids []int) map[int]childRaw { return nil }
