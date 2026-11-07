//go:build !linux && !darwin && !windows

package service

import (
	"errors"
	"io"
)

const supportsUserMode = false

// DefaultConfigDir 返回默认的配置目录。
func DefaultConfigDir(user bool) string {
	return "/etc/gately"
}

// checkInstallSafety 在不支持注册服务的系统上无事可做。
func checkInstallSafety(sp Spec, out io.Writer) {}

func platformRun(action string, sp Spec, out io.Writer) error {
	return errors.New("当前系统不支持注册服务")
}
