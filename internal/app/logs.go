package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/sysstat"
)

// logsDir 是配置目录下保存运行日志的子目录：logs/<模块 ID>.log，系统日志为 logs/gately.log。
// 设置了日志目录（Settings.LogDir）时改用该目录。备份只收集配置目录顶层的 *.json，不会包含日志。
const logsDir = "logs"

// 访问日志（Web 服务）归档的默认保留策略
const (
	defaultAccessLogDays = 7
	defaultAccessLogMB   = 32
	// accessLogDiskShare 为默认总大小占日志所在分区容量的最大比例（1/20 即 5%）：
	// 闪存只有几十 MB 的路由器上自动调低，避免日志占满存储
	accessLogDiskShare = 20
)

// logDirPath 返回运行日志的保存目录。
func (a *App) logDirPath(s Settings) string {
	if s.LogDir != "" {
		return s.LogDir
	}
	return filepath.Join(a.opts.ConfigDir, logsDir)
}

// persistLogs 按设置开启或关闭一个日志缓冲的落盘；source 为模块 ID，系统日志使用 settingsName。
// access 为 true 表示访问日志，按天归档并压缩，按访问日志的保留天数与总大小清理。
func (a *App) persistLogs(buf *logbuf.Buffer, source string, access bool, s Settings) {
	if s.DisableLogPersist {
		buf.ClosePersist()
		return
	}
	dir := a.logDirPath(s)
	path := filepath.Join(dir, source+".log")
	opt := logbuf.PersistOptions{FileBytes: int64(s.LogFileMB) << 20}
	if access {
		opt = logbuf.PersistOptions{Archive: true, KeepDays: accessLogDays(s), KeepBytes: accessLogBytes(s, dir)}
	}
	if err := buf.PersistWith(path, opt); err != nil {
		// 落盘失败不影响运行，日志仍保留在内存
		a.logger.Printf("开启日志落盘失败（%s）: %v", path, err)
	}
}

// accessLogDays 返回访问日志归档的保留天数。
func accessLogDays(s Settings) int {
	if s.AccessLogDays > 0 {
		return s.AccessLogDays
	}
	return defaultAccessLogDays
}

// accessLogBytes 返回访问日志的总大小上限：填写了就用填写的值，否则为默认值，
// 但不超过日志所在分区容量的 1/accessLogDiskShare，也不低于允许填写的最小值。
func accessLogBytes(s Settings, dir string) int64 {
	if s.AccessLogMB > 0 {
		return int64(s.AccessLogMB) << 20
	}
	limit := int64(defaultAccessLogMB) << 20
	if d := sysstat.DiskUsage(dir); d != nil {
		limit = min(limit, int64(d.Total/accessLogDiskShare))
	}
	return max(limit, int64(minAccessLogMB)<<20)
}

// applyLogPersist 把日志落盘设置应用到系统日志与全部模块日志。
func (a *App) applyLogPersist(s Settings) {
	a.persistLogs(a.sysLogs, settingsName, false, s)
	for _, e := range a.modules {
		_, access := e.mod.(module.AccessLogger)
		a.persistLogs(e.logs, e.mod.ID(), access, s)
	}
}

// closeLogs 把尚未写盘的日志写入文件并停止落盘，在退出或重启前调用。
func (a *App) closeLogs() {
	a.sysLogs.ClosePersist()
	for _, e := range a.modules {
		e.logs.ClosePersist()
	}
}

// validateLogDir 校验日志目录的格式：必须是绝对路径，不能是根目录，不能含控制字符。
func validateLogDir(dir string) error {
	if dir == "" {
		return nil
	}
	if len(dir) > maxLogDirLen || strings.ContainsFunc(dir, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errors.New("日志目录无效")
	}
	if !filepath.IsAbs(dir) {
		return errors.New("日志目录必须是绝对路径")
	}
	if filepath.Dir(dir) == dir {
		return errors.New("日志目录不能是根目录")
	}
	return nil
}

// checkLogDir 确认日志目录可用：不是配置目录本身（日志与配置文件混在一起，备份与权限收紧都会受影响），
// 不是已有的普通文件，能创建并写入。会按需创建目录（权限 0700）。
func (a *App) checkLogDir(dir string) error {
	if abs, err := filepath.Abs(a.opts.ConfigDir); err == nil && filepath.Clean(abs) == dir {
		return errors.New("日志目录不能是配置目录本身，可以使用其中的子目录")
	}
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return errors.New("日志目录已存在同名文件")
		}
	} else if err := fsperm.MkdirAll(dir, 0o700); err != nil {
		// 新建的目录设为 0700；已存在的目录不改权限，日志文件本身总是 0600
		return fmt.Errorf("无法创建日志目录: %v", err)
	}
	f, err := os.CreateTemp(dir, ".gately-write-test-*")
	if err != nil {
		return fmt.Errorf("日志目录不可写: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// privatePaths 是配置目录下保存敏感内容的文件与子目录（支持通配符）：配置、令牌、配置历史、
// frp 生成的配置、审计与运行日志、IP 列表缓存、入侵防护的封禁记录、本地备份、恢复前的备份与 Tailscale、NetBird 状态。
// bin/ 等存放用户程序的目录不在其中。
var privatePaths = []string{"*.json", "audit.log*", auth.ResetFile, logsDir, "history", "frp", "iplists", "guard", "backups", "backup-*", "tailscale", "netbird"}

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
var logFilterParams = []string{"level", "source", "q", "exclude", "ip", "host", "site", "listener", "status", "hideBanned", "hideAdmin", "from", "to"}

// parseLogFilter 解析日志过滤参数：
//   - level：最低级别（debug、info、warn、error）
//   - source：来源标签，精确匹配
//   - q：关键词，空格分隔表示同时包含，双引号包住短语，前缀 - 表示排除
//   - exclude：不包含的关键词，空格分隔，双引号包住短语，包含其中任意一个即排除；与 q 为“且”的关系
//   - ip：客户端地址，支持完整地址、CIDR 与地址片段，多个以逗号分隔，前缀 ! 或 - 表示排除
//   - host：站点名称或域名的一部分
//   - site：站点名称，不区分大小写完整匹配（访问日志的 site 字段），用于只看某个站点的日志
//   - listener：Web 服务监听器 key，完整匹配，用于区分同名站点或同名监听器（如分开监听的 IPv4、IPv6）
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
		// 监听器 key 只在 Web 服务日志里出现，其他模块的日志没有该字段，带上时不会匹配任何日志
		Listener: strings.TrimSpace(q.Get("listener")),
		IP:       logbuf.ParseIP(q.Get("ip")),
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

// logSourcesHandler 返回日志的全部来源标签（按名称排序）：日志中出现过的来源（含落盘记录），
// 加上模块配置中的来源（mod 实现 module.LogSourcer 时），日志页的来源下拉据此列出，不依赖已加载的日志。
func (a *App) logSourcesHandler(buf *logbuf.Buffer, mod module.Module) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		set := map[string]struct{}{}
		for _, s := range buf.Sources() {
			set[s] = struct{}{}
		}
		if ls, ok := mod.(module.LogSourcer); ok {
			for _, s := range ls.LogSources() {
				if s != "" {
					set[s] = struct{}{}
				}
			}
		}
		out := make([]string, 0, len(set))
		for s := range set {
			out = append(out, s)
		}
		sort.Strings(out)
		httpx.OK(w, out)
	}
}
