package apitoken

import (
	"net/http"
	"strings"
)

// 权限格式为 "<模块>:<read|write>"，写权限包含读权限。
// 模块位置可用 "*" 表示全部模块；单独的 "*" 等价于 "*:write"。
const (
	LevelRead  = "read"
	LevelWrite = "write"
	// Any 表示接口只要求令牌有效，不要求具体权限
	Any = ""
)

// 非模块的内置权限域
const (
	DomainStatus = "status"
	DomainLogs   = "logs"
	DomainImport = "import"
	// DomainMetrics 对应全局 Prometheus 指标接口 /api/metrics
	DomainMetrics = "metrics"
	// DomainFileUsers 对应 WebDAV、FTP、SMB 共用的文件服务共享用户接口 /api/fileusers
	DomainFileUsers = "fileusers"
)

// Denied 表示接口永远禁止令牌访问，只能使用登录会话。
const Denied = "!denied"

// forbiddenPrefixes 是禁止令牌访问的路径前缀：账号、两步验证、令牌管理、系统级高危操作。
var forbiddenPrefixes = []string{
	"/api/account",
	"/api/audit",
	"/api/history",
	"/api/tokens",
	"/api/system/settings",
	"/api/system/token",
	"/api/system/backup",
	"/api/system/diagnostics",
	"/api/system/restore",
	"/api/system/restart",
	// 在线升级会替换可执行文件
	"/api/system/upgrade",
	"/api/system/selfcheck",
	"/api/system/onboarding",
	// Web 终端的安全设置（两步验证、协议确认）等同于账号安全，只允许会话修改
	"/api/webterminal/security",
	// 本地备份文件含全部密钥，只允许会话下载
	"/api/backup/local",
	"/api/login",
	"/api/logout",
}

// sensitiveGetSuffix 列出会返回敏感内容或建立交互通道的 GET 路径片段，这些接口按写权限处理。
var sensitiveGetSegments = map[string]bool{
	"ws":              true, // WebSocket 终端
	"exec":            true, // 容器内执行命令
	"download":        true, // 证书私钥等文件下载
	"export":          true, // 镜像、数据卷导出
	"read":            true, // 容器内文件读取
	"config":          true, // compose 配置可能含密钥
	"preview-archive": true, // 读取压缩包内文件
	"secrets":         true, // 查看被脱敏的密码、令牌明文
	"files":           true, // 容器、存储挂载的目录浏览（files/list 等）
	"sftp":            true, // Web 终端 SFTP 对本机和远程主机的目录浏览（sftp/{key}/list 等）
}

// sharedFileUserScope 是通过旧接口 /api/<服务>/shared-users 修改文件服务共享用户所需的权限：
// WebDAV、FTP、SMB 共用同一份共享用户，修改会改变三个服务的登录账号，因此三个服务的写权限缺一不可。
// 新接口 /api/fileusers 使用单独的 fileusers 权限。
const sharedFileUserScope = "webdav:write,ftp:write,smb:write"

// Required 返回访问 method+path 所需的权限（如 "ddns:read"），需要多个权限时以逗号分隔、须全部具备。
// 返回 Any 表示任何有效令牌均可访问，返回 Denied 表示令牌不可访问。
func Required(method, path string) string {
	if !strings.HasPrefix(path, "/api/") {
		return Denied
	}
	for _, p := range forbiddenPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return Denied
		}
	}
	level := LevelWrite
	if method == http.MethodGet || method == http.MethodHead {
		level = LevelRead
	}
	switch path {
	case "/api/session", "/api/openapi.json":
		return Any
	case "/api/status", "/api/system/info", "/api/system/update":
		return DomainStatus + ":" + LevelRead
	case "/api/logs":
		return DomainLogs + ":" + LevelRead
	case "/api/import/lucky":
		return DomainImport + ":" + LevelWrite
	}
	rest := strings.TrimPrefix(path, "/api/")
	mod, tail, _ := strings.Cut(rest, "/")
	if mod == "" {
		return Denied
	}
	if level == LevelWrite && tail == "shared-users" && (mod == "webdav" || mod == "ftp" || mod == "smb") {
		return sharedFileUserScope
	}
	// 模块日志属于只读信息，清空日志属于写操作
	if level == LevelRead {
		for _, seg := range strings.Split(tail, "/") {
			if sensitiveGetSegments[seg] {
				level = LevelWrite
				break
			}
		}
	}
	return mod + ":" + level
}

// Match 判断令牌权限列表 scopes 是否满足所需权限 need。
func Match(scopes []string, need string) bool {
	if need == Any {
		return true
	}
	if need == Denied {
		return false
	}
	if strings.Contains(need, ",") {
		for _, one := range strings.Split(need, ",") {
			if !Match(scopes, one) {
				return false
			}
		}
		return true
	}
	mod, level, _ := strings.Cut(need, ":")
	for _, s := range scopes {
		if s == "*" {
			return true
		}
		sm, sl, ok := strings.Cut(s, ":")
		if !ok || (sm != "*" && sm != mod) {
			continue
		}
		if sl == LevelWrite || sl == level {
			return true
		}
	}
	return false
}

// ValidScope 判断单个权限字符串的格式与模块是否有效，modules 为全部可用权限域。
func ValidScope(s string, domains map[string]bool) bool {
	if s == "*" {
		return true
	}
	mod, level, ok := strings.Cut(s, ":")
	if !ok || (level != LevelRead && level != LevelWrite) {
		return false
	}
	return mod == "*" || domains[mod]
}
