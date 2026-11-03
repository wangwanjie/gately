package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/version"
)

// ---- 自动检查更新与安全公告 ----
//
// 默认每 12 小时访问一次 GitHub：读取 Gately 的发布列表与已公开的安全公告（GitHub Security Advisories），
// 判断是否有新版本、当前版本是否受已公开的安全问题影响，并为已安装的托管程序查询上游最新版本。
// 结果保存在配置目录的 update-check.json，界面据此显示升级提示；发现新版本或新的安全问题时向通知中心发布事件，
// 同一版本、同一安全问题只通知一次。请求均为匿名 GET，不携带设备、配置或使用情况等任何信息。
// 设置 disableUpdateCheck 后不再定期访问，只在点击“检查更新”时访问。

// updateCheckName 是检查结果在配置目录中的文件名（运行状态，不记录配置历史）
const updateCheckName = "update-check"

// 以下变量在测试中可替换
var (
	// 发布列表取 100 个（GitHub 单页上限）：发版频繁时 30 个只覆盖约两周，落后较多的版本会漏掉其后的安全修复版本
	releasesAPI   = "https://api.github.com/repos/wangwanjie/gately/releases?per_page=100"
	advisoriesAPI = "https://api.github.com/repos/wangwanjie/gately/security-advisories?state=published&per_page=100"
	// updateCheckInterval 为两次自动检查的间隔，updateCheckRetry 为检查失败后的重试间隔
	updateCheckInterval = 12 * time.Hour
	updateCheckRetry    = time.Hour
	// updateCheckDelay 为启动后（或重新开启自动检查后）首次检查前的等待，避开启动阶段的网络与 DNS 尚未就绪
	updateCheckDelay = 3 * time.Minute
	// manualCheckMinGap 内重复点击“检查更新”直接返回上次结果，避免触发 GitHub 匿名接口的频率限制（每小时 60 次）
	manualCheckMinGap = 30 * time.Second
)

const (
	// updateCheckTimeout 为一次检查访问 GitHub 的总时限（托管程序另有各自的时限）
	updateCheckTimeout = time.Minute
	// maxReleasesBody / maxAdvisoriesBody 为响应大小上限，maxNotesBytes 为缓存的更新说明长度上限
	maxReleasesBody   = 8 << 20
	maxAdvisoriesBody = 4 << 20
	maxNotesBytes     = 64 << 10
	// maxAdvisorySummary 为安全公告标题的长度上限（字符）
	maxAdvisorySummary = 300
	// selfPackage 是安全公告中 Gately 的包名（Go 模块路径）
	selfPackage = "github.com/wangwanjie/gately"
	repoURL     = "https://github.com/wangwanjie/gately"
)

// releaseBrief 是一个发布版本的摘要。
type releaseBrief struct {
	Version     string    `json:"version"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
}

// advisoryRange 是安全公告中的一段受影响版本，写法同 GitHub：如 "<= 0.6.0.4"、">= 0.5.0, < 0.6.1"。
type advisoryRange struct {
	Vulnerable string `json:"vulnerable"`
	Patched    string `json:"patched,omitempty"`
}

// securityAdvisory 是一条已公开的安全公告。
type securityAdvisory struct {
	ID          string          `json:"id"`
	CVE         string          `json:"cve,omitempty"`
	Summary     string          `json:"summary"`
	Severity    string          `json:"severity,omitempty"`
	URL         string          `json:"url"`
	PublishedAt time.Time       `json:"publishedAt"`
	Ranges      []advisoryRange `json:"ranges"`
}

// updateNotified 记录已经通知过的内容，重启后不重复通知。
type updateNotified struct {
	// Version 为已通知过的最新版本
	Version string `json:"version,omitempty"`
	// Security 为已通知过的安全公告 ID 与安全修复版本号
	Security []string `json:"security,omitempty"`
	// Programs 为托管程序已通知过的最新版本，键为程序标识
	Programs map[string]string `json:"programs,omitempty"`
}

// updateCache 是自动检查与手动检查共用的结果，保存在 update-check.json。
// 只保存 GitHub 上的原始信息，是否有更新、是否受影响在读取时按当前版本计算，升级后无需重新检查即可得到正确结果。
type updateCache struct {
	// CheckedAt 为最近一次成功检查的时间
	CheckedAt time.Time `json:"checkedAt,omitzero"`
	// Error / FailedAt 为最近一次失败的原因与时间，之后成功一次即清空
	Error    string    `json:"error,omitempty"`
	FailedAt time.Time `json:"failedAt,omitzero"`
	// Latest 为最新正式版，Notes 为它的更新说明
	Latest *releaseBrief `json:"latest,omitempty"`
	Notes  string        `json:"notes,omitempty"`
	// SecurityReleases 为更新说明带有安全章节的版本
	SecurityReleases []releaseBrief `json:"securityReleases,omitempty"`
	// Advisories 为 Gately 已公开的全部安全公告
	Advisories []securityAdvisory `json:"advisories,omitempty"`
	Notified   updateNotified     `json:"notified"`
}

// updateView 是返回给界面的检查结果。
type updateView struct {
	// AutoCheck 表示是否开启了自动检查
	AutoCheck bool       `json:"autoCheck"`
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
	Error     string     `json:"error,omitempty"`
	FailedAt  *time.Time `json:"failedAt,omitempty"`
	Current   string     `json:"current"`
	// Latest 等字段从未成功检查过时为空
	Latest      string     `json:"latest"`
	URL         string     `json:"url"`
	Notes       string     `json:"notes"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	HasUpdate   bool       `json:"hasUpdate"`
	// Comparable 为当前版本与最新版本号能否比较新旧（开发版等无法比较）
	Comparable bool `json:"comparable"`
	// Security 表示当前版本受已公开的安全问题影响：Advisories 为影响当前版本的安全公告，
	// SecurityReleases 为比当前版本新、带有安全修复的版本
	Security         bool               `json:"security"`
	Advisories       []securityAdvisory `json:"advisories"`
	SecurityReleases []releaseBrief     `json:"securityReleases"`
	// SelfUpgrade 为能否在线升级，前端据此显示“立即升级”或对应部署方式的升级说明
	SelfUpgrade upgradeSupport `json:"selfUpgrade"`
}

// programInstallers 由托管外部程序的模块（frp、Tailscale、NetBird、Cloudflare 隧道、File Browser）实现。
type programInstallers interface {
	ProgramInstallers() []*binstall.Installer
}

// ---- 读取 GitHub ----

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

type ghAdvisory struct {
	GHSAID      string     `json:"ghsa_id"`
	CVEID       string     `json:"cve_id"`
	HTMLURL     string     `json:"html_url"`
	Summary     string     `json:"summary"`
	Severity    string     `json:"severity"`
	State       string     `json:"state"`
	PublishedAt time.Time  `json:"published_at"`
	WithdrawnAt *time.Time `json:"withdrawn_at"`
	Vulns       []struct {
		Package struct {
			Name string `json:"name"`
		} `json:"package"`
		VulnerableRange string `json:"vulnerable_version_range"`
		Patched         string `json:"patched_versions"`
	} `json:"vulnerabilities"`
}

// getGitHubJSON 以匿名 GET 读取 GitHub API，响应超过 limit 字节视为异常。
func getGitHubJSON(ctx context.Context, client *http.Client, rawURL string, limit int64, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("无法访问 GitHub: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub 返回 %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return fmt.Errorf("无法访问 GitHub: %v", err)
	}
	if int64(len(data)) > limit {
		return errors.New("GitHub 返回的数据过大，已忽略")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("解析版本信息失败: %v", err)
	}
	return nil
}

// securityHeadingRe 匹配更新说明中以“安全”开头的章节标题，例如 "## Security / 安全修复"、"### 安全修复"。
// 英文只认单独的 Security（其后为行尾或 / : | ( - 等分隔符）或 Security fix / update / hardening / advisory / patch / issue /
// vulnerability 开头的标题，“Security entrance（安全入口）”等功能名称不会误判；
// 中文只认“安全修复 / 安全加固 / 安全更新 / 安全问题”开头的标题，“安全入口”等功能名称同样不会误判。
// 按行匹配，由 hasSecuritySection 跳过代码块。中文写成 \x{...} 转义：源码中的中文字面量会被当作待翻译的文案提取
var securityHeadingRe = regexp.MustCompile(`(?i)^#{1,4}[ \t]+(?:security(?:[ \t]+(?:fix|update|harden|advisor|patch|issue|vulnerab)|[ \t]*(?:$|[/:|(\-\x{FF08}\x{FF1A}]))|\x{5B89}\x{5168}\x{4FEE}\x{590D}|\x{5B89}\x{5168}\x{52A0}\x{56FA}|\x{5B89}\x{5168}\x{66F4}\x{65B0}|\x{5B89}\x{5168}\x{95EE}\x{9898})`)

// hasSecuritySection 判断更新说明是否带有安全章节。代码块（``` 或 ~~~ 围起）中的行不算标题，
// 避免 shell 注释 "# security ..." 之类的示例被误判为安全修复版本。
func hasSecuritySection(body string) bool {
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		if securityHeadingRe.MatchString(line) {
			return true
		}
	}
	return false
}

// ghsaIDRe 为 GitHub 安全公告编号的格式
var ghsaIDRe = regexp.MustCompile(`^GHSA(?:-[23456789cfghjmpqrvwx]{4}){3}$`)

// parseReleases 从发布列表中取出最新正式版与带安全章节的版本；草稿、预发布与版本号无法识别的发布被忽略。
func parseReleases(list []ghRelease) (latest *releaseBrief, notes string, security []releaseBrief) {
	for _, r := range list {
		if r.Draft || r.Prerelease {
			continue
		}
		if _, ok := parseVersion(r.TagName); !ok {
			continue
		}
		b := releaseBrief{Version: r.TagName, URL: releaseURL(r.TagName, r.HTMLURL), PublishedAt: r.PublishedAt}
		if latest == nil || versionNewer(r.TagName, latest.Version) {
			latest = &b
			notes = truncateUTF8(r.Body, maxNotesBytes)
		}
		if hasSecuritySection(r.Body) {
			security = append(security, b)
		}
	}
	return latest, notes, security
}

// releaseURL 只采用指向本仓库发布页的地址，其余情况按版本号拼出，避免界面与通知中出现外部链接。
func releaseURL(tag, raw string) string {
	if strings.HasPrefix(raw, repoURL+"/releases/") && !strings.ContainsAny(raw, " \"'<>") {
		return raw
	}
	return repoURL + "/releases/tag/" + tag
}

// parseAdvisories 取出已公开、未撤回且涉及 Gately 本身的安全公告。
func parseAdvisories(list []ghAdvisory) []securityAdvisory {
	var out []securityAdvisory
	for _, a := range list {
		if a.WithdrawnAt != nil || (a.State != "" && a.State != "published") || !ghsaIDRe.MatchString(a.GHSAID) {
			continue
		}
		adv := securityAdvisory{
			ID:          a.GHSAID,
			Summary:     truncateRunes(strings.TrimSpace(a.Summary), maxAdvisorySummary),
			URL:         repoURL + "/security/advisories/" + a.GHSAID,
			PublishedAt: a.PublishedAt,
		}
		if strings.HasPrefix(a.CVEID, "CVE-") && len(a.CVEID) <= 32 {
			adv.CVE = a.CVEID
		}
		switch sev := strings.ToLower(a.Severity); sev {
		case "low", "medium", "high", "critical":
			adv.Severity = sev
		}
		for _, v := range a.Vulns {
			// 包名为空（未填写）或为 Gately 的模块路径时才算 Gately 本身
			if name := strings.TrimSpace(v.Package.Name); name != "" && !strings.EqualFold(name, selfPackage) {
				continue
			}
			adv.Ranges = append(adv.Ranges, advisoryRange{Vulnerable: strings.TrimSpace(v.VulnerableRange), Patched: strings.TrimSpace(v.Patched)})
		}
		if len(adv.Ranges) > 0 {
			out = append(out, adv)
		}
		if len(out) >= 100 {
			break
		}
	}
	return out
}

// truncateUTF8 把字符串截断到 n 字节以内，不切断多字节字符。
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// ---- 版本范围 ----

// compareVersion 比较两个已解析的版本号：a 新于 b 返回 1，相同返回 0，旧于 b 返回 -1。规则同 versionNewer。
func compareVersion(a, b parsedVersion) int {
	for i := range a.nums {
		if a.nums[i] != b.nums[i] {
			if a.nums[i] > b.nums[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	case a.pre > b.pre:
		return 1
	}
	return -1
}

// parseLooseVersion 解析安全公告中的版本号，允许只写 主.次（按 0 补齐修订号），如 "0.6"。
func parseLooseVersion(s string) (parsedVersion, bool) {
	s = strings.TrimSpace(s)
	if v, ok := parseVersion(s); ok {
		return v, true
	}
	core := strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	switch strings.Count(core, ".") {
	case 0:
		return parseVersion(strings.Replace(s, core, core+".0.0", 1))
	case 1:
		return parseVersion(strings.Replace(s, core, core+".0", 1))
	}
	return parsedVersion{}, false
}

// versionInRange 判断版本 v 是否落在 GitHub 写法的版本范围内，范围由逗号分隔的条件组成（同时满足），
// 每个条件为 =、>=、>、<=、< 加版本号，不写运算符视为 =。ok 为 false 表示范围无法解析。
func versionInRange(v parsedVersion, rng string) (in, ok bool) {
	rng = strings.TrimSpace(rng)
	if rng == "" {
		return false, false
	}
	for _, cond := range strings.Split(rng, ",") {
		cond = strings.TrimSpace(cond)
		op := "="
		for _, p := range []string{">=", "<=", "==", ">", "<", "="} {
			if strings.HasPrefix(cond, p) {
				op, cond = p, strings.TrimSpace(cond[len(p):])
				break
			}
		}
		bound, ok := parseLooseVersion(cond)
		if !ok {
			return false, false
		}
		c := compareVersion(v, bound)
		var hit bool
		switch op {
		case ">=":
			hit = c >= 0
		case ">":
			hit = c > 0
		case "<=":
			hit = c <= 0
		case "<":
			hit = c < 0
		default:
			hit = c == 0
		}
		if !hit {
			return false, true
		}
	}
	return true, true
}

// affects 判断安全公告是否影响版本 cur。受影响范围无法解析时退回比较修复版本：早于修复版本即视为受影响。
func (adv securityAdvisory) affects(cur parsedVersion) bool {
	for _, r := range adv.Ranges {
		in, ok := versionInRange(cur, r.Vulnerable)
		if ok {
			if in {
				return true
			}
			continue
		}
		if p, ok := parseLooseVersion(strings.Split(r.Patched, ",")[0]); ok && compareVersion(cur, p) < 0 {
			return true
		}
	}
	return false
}

// ---- 结果 ----

// describeRe 匹配 git describe --tags --dirty 生成的版本号（make 本地构建）：标签后跟 -<提交数>-g<哈希> 与 / 或 -dirty
var describeRe = regexp.MustCompile(`^(v?\d+(?:\.\d+){2,3})(?:-\d+-g[0-9a-f]{4,40})?(?:-dirty)?$`)

// releaseBase 返回比较新旧时使用的版本号：git describe 生成的版本是在标签之后构建的，按标签本身比较，
// 否则 "v0.6.3-5-gabc1234" 会被当成 v0.6.3 的预发布版，误报“有新版本 v0.6.3”乃至安全更新。
func releaseBase(cur string) string {
	if m := describeRe.FindStringSubmatch(strings.TrimSpace(cur)); m != nil {
		return m[1]
	}
	return cur
}

// view 按当前版本计算检查结果。
func (c updateCache) view(display string, autoCheck bool) updateView {
	v := updateView{AutoCheck: autoCheck, Current: display, Error: c.Error, Advisories: []securityAdvisory{}, SecurityReleases: []releaseBrief{}}
	// 界面与通知显示原始版本号，比较时使用标签版本号
	cur := releaseBase(display)
	if !c.CheckedAt.IsZero() {
		t := c.CheckedAt
		v.CheckedAt = &t
	}
	if !c.FailedAt.IsZero() {
		t := c.FailedAt
		v.FailedAt = &t
	}
	if c.Latest == nil {
		return v
	}
	v.Latest, v.URL, v.Notes = c.Latest.Version, c.Latest.URL, c.Notes
	if !c.Latest.PublishedAt.IsZero() {
		t := c.Latest.PublishedAt
		v.PublishedAt = &t
	}
	v.Comparable = versionComparable(c.Latest.Version, cur)
	v.HasUpdate = versionNewer(c.Latest.Version, cur)
	curV, ok := parseVersion(cur)
	if !ok {
		// 开发版等无法判断是否受影响
		return v
	}
	for _, adv := range c.Advisories {
		if adv.affects(curV) {
			v.Advisories = append(v.Advisories, adv)
		}
	}
	for _, r := range c.SecurityReleases {
		if versionNewer(r.Version, cur) {
			v.SecurityReleases = append(v.SecurityReleases, r)
		}
	}
	v.Security = len(v.Advisories) > 0 || len(v.SecurityReleases) > 0
	return v
}

// securityKeys 返回检查结果中需要通知的安全条目：安全公告 ID 与安全修复版本号。
func (v updateView) securityKeys() []string {
	var keys []string
	for _, a := range v.Advisories {
		keys = append(keys, a.ID)
	}
	for _, r := range v.SecurityReleases {
		keys = append(keys, r.Version)
	}
	slices.Sort(keys)
	return keys
}

// ---- 检查 ----

// loadUpdateCache 读取上次的检查结果，文件损坏时从空结果开始。
func (a *App) loadUpdateCache() {
	var c updateCache
	if _, err := a.store.Load(updateCheckName, &c); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "update", "读取更新检查结果失败，将重新检查: "+err.Error())
		c = updateCache{}
	}
	a.updMu.Lock()
	a.upd = c
	a.updMu.Unlock()
}

// updateSnapshot 返回检查结果的副本，首次调用时从文件读取。
func (a *App) updateSnapshot() updateCache {
	a.updOnce.Do(a.loadUpdateCache)
	a.updMu.Lock()
	defer a.updMu.Unlock()
	c := a.upd
	c.Notified.Programs = cloneMap(c.Notified.Programs)
	c.Notified.Security = slices.Clone(c.Notified.Security)
	return c
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// saveUpdateCache 更新内存中的结果并写入文件；写入失败只记录日志，下次检查时重试。
func (a *App) saveUpdateCache(c updateCache) {
	a.updOnce.Do(func() {})
	a.updMu.Lock()
	a.upd = c
	a.updMu.Unlock()
	if err := a.store.Save(updateCheckName, c); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "update", "保存更新检查结果失败: "+err.Error())
	}
}

// fetchUpdateInfo 读取发布列表与安全公告。安全公告读取失败不影响版本检查，返回的 advErr 单独记录。
func fetchUpdateInfo(ctx context.Context, client *http.Client) (c updateCache, advErr, err error) {
	var rels []ghRelease
	if err := getGitHubJSON(ctx, client, releasesAPI, maxReleasesBody, &rels); err != nil {
		return c, nil, err
	}
	latest, notes, security := parseReleases(rels)
	if latest == nil {
		return c, nil, errors.New("GitHub 上没有找到正式版本")
	}
	c.Latest, c.Notes, c.SecurityReleases = latest, notes, security
	var advs []ghAdvisory
	if err := getGitHubJSON(ctx, client, advisoriesAPI, maxAdvisoriesBody, &advs); err != nil {
		return c, err, nil
	}
	c.Advisories = parseAdvisories(advs)
	return c, nil, nil
}

// checkUpdates 检查 Gately 的新版本与安全公告，保存结果并发布通知；includePrograms 为 true 时（自动检查）随后检查托管程序，
// 手动检查不检查托管程序（它们各有检查按钮）。Gately 的检查之间互斥，托管程序的检查在其后单独进行，
// 不会让手动“检查更新”等待托管程序查询完成。
func (a *App) checkUpdates(ctx context.Context, includePrograms bool) (updateView, error) {
	view, err := a.checkSelf(ctx, 0)
	if includePrograms {
		a.checkAllPrograms(ctx)
	}
	return view, err
}

// checkSelf 检查 Gately 本身。多个检查不会同时进行：后到的等待前一个完成。
// minGap 大于 0 时（手动检查），拿到锁后若距上次检查不足 minGap 则直接返回上次结果，
// 同时到达的多个请求因此只访问一次 GitHub。
func (a *App) checkSelf(ctx context.Context, minGap time.Duration) (updateView, error) {
	a.updCheckMu.Lock()
	defer a.updCheckMu.Unlock()
	prev := a.updateSnapshot()
	if minGap > 0 {
		if v, hit, err := a.recentResult(prev, minGap); hit {
			return v, err
		}
	}
	cctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	fetched, advErr, err := fetchUpdateInfo(cctx, a.updateClient())
	if err != nil && ctx.Err() != nil {
		// 调用方取消（程序退出、重启）不算检查失败，不改动保存的结果
		return prev.view(version.Version, !a.settingsSnapshot().DisableUpdateCheck), err
	}
	next := prev
	if err != nil {
		next.Error, next.FailedAt = err.Error(), time.Now()
		// 同样的错误只在第一次记录，避免无法访问 GitHub 时每次重试都写日志
		if prev.Error != err.Error() {
			a.sysLogs.Add(logbuf.LevelWarn, "update", "检查更新失败: "+err.Error())
		}
	} else {
		next.CheckedAt, next.Error, next.FailedAt = time.Now(), "", time.Time{}
		next.Latest, next.Notes, next.SecurityReleases = fetched.Latest, fetched.Notes, fetched.SecurityReleases
		if advErr != nil {
			// 安全公告读取失败时沿用上次的公告列表
			a.sysLogs.Add(logbuf.LevelWarn, "update", "读取安全公告失败: "+advErr.Error())
		} else {
			next.Advisories = fetched.Advisories
		}
	}
	view := next.view(version.Version, !a.settingsSnapshot().DisableUpdateCheck)
	if err == nil {
		a.notifyUpdate(view, &next.Notified)
	}
	a.saveUpdateCache(next)
	view.SelfUpgrade = a.upgradeSupport()
	return view, err
}

// checkAllPrograms 检查托管程序并保存通知记录。查询期间不持有 updCheckMu，
// 只在最后读取最新结果、写回托管程序的通知记录时短暂持有，避免覆盖期间完成的 Gately 检查结果。
func (a *App) checkAllPrograms(ctx context.Context) {
	a.updProgMu.Lock()
	defer a.updProgMu.Unlock()
	n := updateNotified{Programs: a.updateSnapshot().Notified.Programs}
	a.checkPrograms(ctx, &n)
	a.updCheckMu.Lock()
	defer a.updCheckMu.Unlock()
	c := a.updateSnapshot()
	c.Notified.Programs = n.Programs
	a.saveUpdateCache(c)
}

// notifyUpdate 在发现新的安全问题或新版本时发布通知，并记录已通知的内容。
// 有安全问题时只发安全更新通知（其中已包含最新版本），不再另发新版本通知。
func (a *App) notifyUpdate(v updateView, n *updateNotified) {
	keys := v.securityKeys()
	fresh := false
	for _, k := range keys {
		if !slices.Contains(n.Security, k) {
			fresh = true
			break
		}
	}
	// 只保留当前仍相关的条目：升级后不再相关的条目自然移除，文件不会无限增长
	n.Security = keys
	if fresh {
		a.publishSecurity(v)
		n.Version = v.Latest
		return
	}
	if v.HasUpdate && v.Latest != n.Version {
		n.Version = v.Latest
		published := ""
		if v.PublishedAt != nil {
			published = v.PublishedAt.Format("2006-01-02")
		}
		a.sysLogs.Add(logbuf.LevelInfo, "update", fmt.Sprintf("发现新版本 %s（当前 %s）", v.Latest, v.Current))
		events.Publish(events.Event{
			Type: events.SystemUpdate, Source: "Gately", Level: events.LevelInfo,
			Title:   fmt.Sprintf("Gately 有新版本 %s", v.Latest),
			Message: fmt.Sprintf("当前版本 %s，最新版本 %s（发布于 %s）。更新内容：%s", v.Current, v.Latest, published, v.URL),
			Fields:  map[string]string{"current": v.Current, "latest": v.Latest, "url": v.URL},
		})
	}
}

// publishSecurity 发布安全更新通知，列出影响当前版本的安全公告与带安全修复的新版本。
func (a *App) publishSecurity(v updateView) {
	// 拼接用的分隔符与括号不含中文，通知按服务端语言翻译后各语言都通顺
	var lines, ids []string
	for _, adv := range v.Advisories {
		ids = append(ids, adv.ID)
		sev := adv.Severity
		if sev == "" {
			sev = "unknown"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s) %s %s", adv.ID, sev, adv.Summary, adv.URL))
	}
	var vers []string
	for _, r := range v.SecurityReleases {
		vers = append(vers, r.Version)
	}
	if len(vers) > 0 {
		lines = append(lines, fmt.Sprintf("以下新版本包含安全修复：%s", strings.Join(vers, ", ")))
	}
	advice := fmt.Sprintf("请尽快升级到 %s：%s", v.Latest, v.URL)
	if !v.HasUpdate {
		// 已是最新版本仍受影响（尚无修复版本）时提示关注公告
		advice = "当前已是最新版本，修复版本发布前请按公告中的缓解措施处理"
	}
	lines = append(lines, advice)
	a.sysLogs.Add(logbuf.LevelWarn, "update", fmt.Sprintf("当前版本 %s 受安全问题影响：%s", v.Current, strings.Join(append(ids, vers...), ", ")))
	events.Publish(events.Event{
		Type: events.SystemSecurity, Source: "Gately", Level: events.LevelError,
		Title:   fmt.Sprintf("Gately 安全更新：当前版本 %s 受安全问题影响", v.Current),
		Message: strings.Join(lines, "\n"),
		Fields: map[string]string{
			"current": v.Current, "latest": v.Latest, "url": v.URL,
			"advisories": strings.Join(ids, ","), "securityReleases": strings.Join(vers, ","),
		},
	})
}

// checkPrograms 为已安装到 bin 下的托管程序查询上游最新版本，有新版本时通知一次。
// 只检查已开启的模块；查询失败只记录日志，不影响其他程序。
func (a *App) checkPrograms(ctx context.Context, n *updateNotified) {
	seen := map[string]bool{}
	for _, e := range a.modules {
		pm, ok := e.mod.(programInstallers)
		if !e.enabled || !ok {
			continue
		}
		for _, in := range pm.ProgramInstallers() {
			if in == nil || !in.Installed().Present {
				continue
			}
			key := in.Key()
			// 先登记再判断是否已取消：程序退出时未检查的程序保留通知记录，重启后不会重复通知同一版本
			seen[key] = true
			if ctx.Err() != nil {
				continue
			}
			l, err := in.CheckLatest(ctx)
			if err != nil {
				if !errors.Is(err, binstall.ErrChecking) {
					e.logs.Add(logbuf.LevelWarn, key, fmt.Sprintf("检查 %s 更新失败: %v", in.Name(), err))
				}
				continue
			}
			if !l.HasUpdate || n.Programs[key] == l.Version {
				continue
			}
			if n.Programs == nil {
				n.Programs = map[string]string{}
			}
			n.Programs[key] = l.Version
			cur := in.Installed().Version
			e.logs.Add(logbuf.LevelInfo, key, fmt.Sprintf("%s 有新版本 %s（当前 %s）", in.Name(), l.Version, cur))
			events.Publish(events.Event{
				Type: events.ProgramUpdate, Source: in.Name(), Level: events.LevelInfo,
				Title:   fmt.Sprintf("%s 有新版本 %s", in.Name(), l.Version),
				Message: fmt.Sprintf("%s 当前版本 %s，最新版本 %s，可在 %s 页面的“在线安装”中更新", in.Name(), cur, l.Version, e.mod.Title()),
				Fields:  map[string]string{"program": key, "current": cur, "latest": l.Version},
			})
		}
	}
	// 卸载或关闭的程序不再保留通知记录
	for k := range n.Programs {
		if !seen[k] {
			delete(n.Programs, k)
		}
	}
}

// nextUpdateCheck 返回距离下一次自动检查的等待时间：上次成功检查未满间隔时等到满间隔，
// 最近一次失败时按重试间隔，均不少于 updateCheckDelay。
func nextUpdateCheck(c updateCache, now time.Time) time.Duration {
	wait := updateCheckDelay
	if !c.CheckedAt.IsZero() {
		if d := c.CheckedAt.Add(updateCheckInterval).Sub(now); d > wait {
			wait = d
		}
	}
	if !c.FailedAt.IsZero() && c.FailedAt.After(c.CheckedAt) {
		if d := c.FailedAt.Add(updateCheckRetry).Sub(now); d > wait {
			wait = d
		}
	}
	// 系统时间被往回调过时，避免等待超过一个间隔
	return min(wait, updateCheckInterval)
}

// runUpdateCheck 定期自动检查更新，阻塞直到 ctx 取消。关闭自动检查后保持等待，重新开启时由 updWake 唤醒。
func (a *App) runUpdateCheck(ctx context.Context) {
	timer := time.NewTimer(nextUpdateCheck(a.updateSnapshot(), time.Now()))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.updWake:
			timer.Reset(nextUpdateCheck(a.updateSnapshot(), time.Now()))
			continue
		case <-timer.C:
		}
		if a.settingsSnapshot().DisableUpdateCheck {
			timer.Reset(updateCheckInterval)
			continue
		}
		_, _ = a.checkUpdates(ctx, true)
		timer.Reset(nextUpdateCheck(a.updateSnapshot(), time.Now()))
	}
}

// wakeUpdateCheck 在重新开启自动检查后让检查协程按新的设置重新计时。
func (a *App) wakeUpdateCheck() {
	select {
	case a.updWake <- struct{}{}:
	default:
	}
}

// ---- 接口 ----

// handleCheckUpdate 立即检查 Gately 是否有新版本与安全问题（“检查更新”按钮）。
// 距上次检查（成功或失败）不足 manualCheckMinGap 时直接返回上次结果，失败的结果同样返回 502，
// 避免反复点击或脚本调用触发 GitHub 匿名接口的频率限制。
func (a *App) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	// 检查与请求的生命周期脱钩：用户关闭页面时检查照常完成，不会被记成一次失败。
	// 最短间隔在 checkSelf 拿到锁后判断，并发的多个请求不会各自访问 GitHub
	v, err := a.checkSelf(context.WithoutCancel(r.Context()), manualCheckMinGap)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	httpx.OK(w, v)
}

// recentResult 判断距上次检查（成功或失败）是否不足 gap：是则 hit 为 true 并返回上次的结果，
// 上次失败时以 err 返回当时的错误。系统时间往回调过（上次时间在未来）时视为已超过间隔。
func (a *App) recentResult(c updateCache, gap time.Duration) (v updateView, hit bool, err error) {
	last := c.CheckedAt
	if c.FailedAt.After(last) {
		last = c.FailedAt
	}
	since := time.Since(last)
	if last.IsZero() || since < 0 || since >= gap {
		return updateView{}, false, nil
	}
	v = c.view(version.Version, !a.settingsSnapshot().DisableUpdateCheck)
	v.SelfUpgrade = a.upgradeSupport()
	if c.Error != "" && c.FailedAt.After(c.CheckedAt) {
		return v, true, errors.New(c.Error)
	}
	return v, true, nil
}

// handleUpdateState 返回上次检查的结果，不访问网络，供界面显示升级提示。
func (a *App) handleUpdateState(w http.ResponseWriter, r *http.Request) {
	c := a.updateSnapshot()
	v := c.view(version.Version, !a.settingsSnapshot().DisableUpdateCheck)
	v.SelfUpgrade = a.upgradeSupport()
	httpx.OK(w, v)
}
