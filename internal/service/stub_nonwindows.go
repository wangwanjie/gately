//go:build !windows

package service

import "context"

// IsWindowsService 在 Windows 以外的系统上恒为 false。
func IsWindowsService() bool {
	return false
}

// RunAsWindowsService 在 Windows 以外的系统上不会被调用。
func RunAsWindowsService(run func(ctx context.Context)) int {
	return 1
}
