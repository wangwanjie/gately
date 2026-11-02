package app

import (
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// logsDir 是配置目录下保存运行日志的子目录：logs/<模块 ID>.log，系统日志为 logs/gately.log。
// 备份只收集配置目录顶层的 *.json，不会包含日志。
const logsDir = "logs"

// persistLogs 按设置开启或关闭一个日志缓冲的落盘；source 为模块 ID，系统日志使用 settingsName。
func (a *App) persistLogs(buf *logbuf.Buffer, source string, s Settings) {
	if s.DisableLogPersist {
		buf.ClosePersist()
		return
	}
	path := filepath.Join(a.opts.ConfigDir, logsDir, source+".log")
	if err := buf.Persist(path, int64(s.LogFileMB)<<20); err != nil {
		// 落盘失败不影响运行，日志仍保留在内存
		a.logger.Printf("开启日志落盘失败（%s）: %v", path, err)
	}
}

// applyLogPersist 把日志落盘设置应用到系统日志与全部模块日志。
func (a *App) applyLogPersist(s Settings) {
	a.persistLogs(a.sysLogs, settingsName, s)
	for _, e := range a.modules {
		a.persistLogs(e.logs, e.mod.ID(), s)
	}
}

// closeLogs 把尚未写盘的日志写入文件并停止落盘，在退出或重启前调用。
func (a *App) closeLogs() {
	a.sysLogs.ClosePersist()
	for _, e := range a.modules {
		e.logs.ClosePersist()
	}
}

// privatePaths 是配置目录下保存敏感内容的文件与子目录（支持通配符）：配置、令牌、配置历史、
// frp 生成的配置、审计与运行日志、IP 列表缓存、入侵防护的封禁记录、本地备份、恢复前的备份与 Tailscale 状态。
// bin/ 等存放用户程序的目录不在其中。
var privatePaths = []string{"*.json", "audit.log*", auth.ResetFile, logsDir, "history", "frp", "iplists", "guard", "backups", "backup-*", "tailscale"}

// tightenConfigDir 收紧配置目录下敏感文件的权限，修复旧版本在会忽略创建权限的文件系统
// （如飞牛 fnOS 的 trimacl 卷）上创建的 0755 文件，返回改动的数量。
func tightenConfigDir(dir string) int {
	n := 0
	for _, pattern := range privatePaths {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, p := range matches {
			n += fsperm.Tighten(p)
		}
	}
	return n
}

// logFilterParams 是日志接口支持的过滤参数，接口文档据此列出查询参数。
var logFilterParams = []string{"level", "source", "q", "exclude", "ip", "host", "site", "status", "hideBanned", "hideAdmin", "from", "to"}

// parseLogFilter 解析日志过滤参数：
//   - level：最低级别（debug、info、warn、error）
//   - source：来源标签，精确匹配
//   - q：关键词，空格分隔表示同时包含，双引号包住短语，前缀 - 表示排除
//   - exclude：不包含的关键词，空格分隔，双引号包住短语，包含其中任意一个即排除；与 q 为“且”的关系
//   - ip：客户端地址，支持完整地址、CIDR 与地址片段，多个以逗号分隔，前缀 ! 或 - 表示排除
//   - host：站点名称或域名的一部分
//   - site：站点名称，不区分大小写完整匹配（访问日志的 site 字段），用于只看某个站点的日志
//   - status：状态码，如 404、4xx、500-599，多个以逗号分隔
//   - hideBanned：为 1 时隐藏被封禁来源的访问日志：封禁后被拒绝的请求，以及当前仍在封禁中的来源的全部请求（包括触发封禁的请求）
//   - hideAdmin：为 1 时隐藏带管理后台登录态的请求（管理员经站点反代访问后台产生的访问日志）
//   - from、to：时间范围，RFC 3339 格式或毫秒时间戳
//
// 没有任何过滤条件时返回 nil；参数无效时返回错误信息（完整字面量，便于翻译）。
func parseLogFilter(q url.Values) (*logbuf.Filter, string) {
	f := &logbuf.Filter{
		Source: strings.TrimSpace(q.Get("source")),
		Host:   strings.ToLower(strings.TrimSpace(q.Get("host"))),
		Site:   strings.TrimSpace(q.Get("site")),
		IP:     logbuf.ParseIP(q.Get("ip")),
	}
	if lv := q.Get("level"); lv != "" && lv != "all" {
		if !logbuf.ValidLevel(lv) {
			return nil, "level 参数无效"
		}
		f.MinLevel = lv
	}
	f.Terms, f.Excludes = logbuf.ParseKeywords(q.Get("q"))
	// exclude 中的词全部视为排除词，写不写前缀 - 都一样
	exTerms, exExcludes := logbuf.ParseKeywords(q.Get("exclude"))
	f.Excludes = append(append(f.Excludes, exTerms...), exExcludes...)
	// 只是隐藏，日志本身照常保存，关闭后可以查到
	if boolParam(q.Get("hideBanned")) {
		f.Hide = append(f.Hide, logbuf.FieldCond{Key: logbuf.FieldDeny, Value: logbuf.DenyBan})
		// 触发封禁的请求发生在封禁之前，没有 deny 标记；封禁后由防火墙丢弃的包也不再产生日志。
		// 按当前封禁列表隐藏这些来源的全部请求，解封或到期后重新显示
		f.HideIP = func(ip string) bool {
			a, err := netip.ParseAddr(ip)
			return err == nil && banlist.Default.Banned(a)
		}
	}
	if boolParam(q.Get("hideAdmin")) {
		f.Hide = append(f.Hide, logbuf.FieldCond{Key: logbuf.FieldAdmin})
	}
	if v := strings.TrimSpace(q.Get("status")); v != "" {
		st, ok := logbuf.ParseStatus(v)
		if !ok {
			return nil, "状态码格式无效，可填写 404、4xx 或 500-599"
		}
		f.Status = st
	}
	var ok bool
	if f.From, ok = parseLogTime(q.Get("from")); !ok {
		return nil, "from 参数无效"
	}
	if f.To, ok = parseLogTime(q.Get("to")); !ok {
		return nil, "to 参数无效"
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.From.After(f.To) {
		return nil, "from 不能晚于 to"
	}
	if f.Empty() {
		return nil, ""
	}
	return f, ""
}

// boolParam 判断开关类查询参数是否开启（1 或 true）。
func boolParam(v string) bool {
	v = strings.TrimSpace(v)
	return v == "1" || strings.EqualFold(v, "true")
}

// minLogTimeMsDigits 为毫秒时间戳的最少位数：12 位对应 1973 年起的时间，
// 更短的纯数字（例如 2026）多半是只填了年份，按时间戳解析会变成 1970 年，直接视为无效
const minLogTimeMsDigits = 12

// parseLogTime 解析 RFC 3339 时间或毫秒时间戳（至少 12 位数字），空串返回零值。
func parseLogTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, true
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		if len(strings.TrimLeft(s, "+-")) < minLogTimeMsDigits {
			return time.Time{}, false
		}
		return time.UnixMilli(ms), true
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}
