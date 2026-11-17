// Package demo 实现在线演示模式：访客可以只读浏览一个带演示数据的实例。
//
// 演示模式由一个集中的开关控制，各执行点（脚本、托管进程、出站请求、防火墙写入、升级等）
// 调用 Check 或 Enabled 判断，统一返回 ErrUnavailable。管理接口层面由 Blocked 决定哪些请求一律拒绝，
// 即使持有管理员会话也无法绕过。
package demo

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
)

// Message 是演示模式下不可用的统一提示，中文文案是 i18n 的键，由前端与后端翻译为 10 种语言。
const Message = "演示模式下不可用"

// ErrUnavailable 是各执行点在演示模式下返回的统一错误。
var ErrUnavailable = errors.New(Message)

var enabled atomic.Bool

// Enabled 返回是否处于演示模式。
func Enabled() bool {
	return enabled.Load()
}

// Set 开关演示模式，只在启动时调用（测试中可随时切换）。
func Set(on bool) {
	enabled.Store(on)
}

// Check 在演示模式下返回 ErrUnavailable，其余情况返回 nil。需要执行外部动作的位置先调用它。
func Check() error {
	if enabled.Load() {
		return ErrUnavailable
	}
	return nil
}

// disabledModules 是演示模式下强制关闭的模块：它们的全部功能都是执行命令、访问宿主机或容器。
var disabledModules = []string{"docker", "webterminal"}

// DisabledModules 返回演示模式下强制关闭的模块 ID。
func DisabledModules() []string {
	return append([]string(nil), disabledModules...)
}

// allowedWrites 是演示模式下仍允许的写请求（方法 + 路径）：只涉及当前登录会话的界面偏好与纯校验，
// 不会改变演示数据或触发任何外部动作。
var allowedWrites = map[string]bool{
	"PUT /api/account/ui-language":        true,
	"POST /api/account/password-check":    true,
	"POST /api/system/onboarding/dismiss": true,
	"POST /api/logout":                    true,
}

// blockedReadPrefixes 是演示模式下一律拒绝读取的路径：文件与存储内容、备份与诊断包导出、更新检查（对外请求）、
// 终端与 Docker 的全部接口（含 WebSocket）。
var blockedReadPrefixes = []string{
	"/api/system/update",
	"/api/system/backup",
	"/api/system/diagnostics",
	"/api/system/upgrade",
	"/api/storage/files/",
	"/api/backup/local",
	"/api/webterminal/",
	"/api/docker/",
}

// Blocked 判断演示模式下是否一律拒绝该管理接口请求，与账号角色无关。
// 规则：写请求（POST/PUT/PATCH/DELETE 等）除少数界面偏好外全部拒绝，读请求中涉及文件内容、导出与对外请求的接口拒绝。
// 未开启演示模式时始终返回 false。
func Blocked(method, path string) bool {
	if !enabled.Load() {
		return false
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		// 只读请求：这两个接口只返回缓存的状态，不会发起对外请求
		if path == "/api/system/update/state" || path == "/api/system/upgrade/status" {
			return false
		}
		for _, p := range blockedReadPrefixes {
			if path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p) {
				return true
			}
		}
		return false
	}
	return !allowedWrites[method+" "+path]
}
