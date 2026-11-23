//go:build !linux && !darwin && !freebsd && !windows

package sysstat

func diskUsage(dir string) *Disk {
	return nil
}
