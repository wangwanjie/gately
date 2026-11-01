package diag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
)

// maxArrayItems 限制数组保留的元素数，超出部分只记录数量，避免诊断包过大。
const maxArrayItems = 100

// verbatimKeys 是值可以原样保留的字段：只会出现固定枚举值（协议、模式、服务商 ID 等），不会包含用户自填内容。
var verbatimKeys = toSet(
	"type", "mode", "protocol", "protocols", "proto", "network", "provider", "kind", "family", "level", "minLevel",
	"tlsMode", "minTls", "lbPolicy", "authType", "keyType", "method", "format", "countryMode", "proxyMode",
	"ipMode", "channel", "role", "state", "status", "backendState", "language", "timeZone", "schedule",
	"repeat", "version", "os", "arch", "edgeIpVersion", "scope", "scopes", "tool", "healthStatus", "driver",
	"contentType", "acme", "restart", "tlsVersion", "shell",
)

// timeKeys 是时间戳字段，证书到期、任务上次运行时间对定位问题很有帮助，时间本身不含隐私。
var timeKeys = toSet(
	"createdAt", "updatedAt", "lastRun", "nextRun", "lastCheck", "lastSync", "lastAttempt", "nextRetry",
	"notBefore", "notAfter", "expiresAt", "lastUsedAt", "lastSeen", "lastAction", "lastActive", "started",
	"finished", "startedAt", "modTime", "time", "sampledAt",
)

// addrKeys 是地址类字段：保留端口与 IP 类别，主机一律移除。
var addrKeys = toSet(
	"listen", "httpsListen", "listenAddress", "listenIp", "bind", "bindAddr", "localAddr", "localIp", "target",
	"targets", "targetAddresses", "to", "from", "dest", "destination", "upstreams", "addr", "addrs",
	"serverAddr", "server", "publicAddr", "stunServer", "stunServers", "keepAliveServers", "checkAddr",
	"upnpGateway", "natpmpGateway", "dashboardAddr", "broadcastIps", "ip", "ips", "ipv4", "ipv6",
	"lastUsedIP", "ipList", "resolvers", "dnsServers", "dns", "curAddr", "trustedProxies", "realIPFrom", "host", "hostIp",
	"upnpLocalHost", "upnpControlUrl", "publicHost", "socks5", "proxy",
)

// textKeys 是运行时错误等自由文本字段：保留内容，但按日志规则脱敏。
var textKeys = toSet("error", "lastError", "ipError", "message", "reason", "lastExit", "warnings", "errors", "summary")

// sensitiveKeyRe 匹配可能承载密钥的字段名，用于决定数字是否需要移除。
var sensitiveKeyRe = regexp.MustCompile(`(?i)pass|pwd|pin|secret|token|key|salt|hash|otp|totp|cookie|credential|auth|code`)

// plainKeyRe 约束可保留的对象键：普通标识符，且不含长数字串或十六进制串。
var (
	plainKeyRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)
	digitRunRe  = regexp.MustCompile(`\d{6,}`)
	hexLikeRe   = regexp.MustCompile(`^[0-9a-fA-F]{12,}$`)
	numberOnlyR = regexp.MustCompile(`^\d+$`)
)

func toSet(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

// SanitizeJSON 把配置文件转换为脱敏后的结构视图：保留字段名、层级、数字、布尔值和白名单枚举，
// 其余字符串替换为 <redacted>（空字符串保持为空，便于判断“是否已填写”）。
func (r *Redactor) SanitizeJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// 占位符含 < >，关闭 HTML 转义以保持可读
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r.sanitize(v, "")); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sanitize 递归处理一个值，key 为其所属字段名（数组元素沿用数组的字段名）。
func (r *Redactor) sanitize(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		anon := 0
		for k, child := range x {
			name := k
			if !plainKeyRe.MatchString(k) || digitRunRe.MatchString(k) || hexLikeRe.MatchString(k) || numberOnlyR.MatchString(k) {
				// 域名、ID、用户自定义名称等动态键不保留
				anon++
				name = fmt.Sprintf("<key-%d>", anon)
				child = r.sanitize(child, "")
			} else {
				child = r.sanitize(child, k)
			}
			out[name] = child
		}
		return out
	case []any:
		n := len(x)
		if n > maxArrayItems {
			x = x[:maxArrayItems]
		}
		out := make([]any, 0, len(x)+1)
		for _, child := range x {
			out = append(out, r.sanitize(child, key))
		}
		if n > maxArrayItems {
			out = append(out, fmt.Sprintf("<%d more items omitted>", n-maxArrayItems))
		}
		return out
	case string:
		return r.sanitizeString(x, key)
	case json.Number:
		if sensitiveKeyRe.MatchString(key) && !verbatimKeys[key] {
			return Redacted
		}
		return x
	default:
		// bool 与 null 不含隐私
		return x
	}
}

func (r *Redactor) sanitizeString(s, key string) string {
	if s == "" {
		return s
	}
	switch {
	case verbatimKeys[key]:
		// 枚举字段即便被填入了意外内容，也按日志规则再过滤一遍
		return r.Text(s)
	case timeKeys[key]:
		return r.Text(s)
	case addrKeys[key]:
		return r.Addr(s)
	case textKeys[key]:
		return r.Text(s)
	}
	return Redacted
}
