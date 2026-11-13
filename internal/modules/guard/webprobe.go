package guard

import (
	"fmt"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/wangwanjie/gately/internal/webprobe"
)

// statusClosed 是 Web 服务断开连接（return 444）时记录的状态码。
const statusClosed = 444

// maxMatchPath 是参与路径规则匹配的最大长度。更长的路径只取开头与结尾各一半拼接后匹配，
// 前缀与后缀类规则（/.git*、*.php）照常生效，避免超长请求行让每个请求的匹配耗时随长度增长。
const maxMatchPath = 4096

// pathRule 是一条编译后的路径规则：re 不为空时为正则，否则为按 * 切开的通配片段（已转小写）。
type pathRule struct {
	re    *regexp.Regexp
	parts []string
}

// match 判断路径是否命中规则。通配规则做分段的前缀、包含、后缀比较，耗时与路径长度线性相关，不经过正则。
func (r pathRule) match(p, lower string) bool {
	if r.re != nil {
		return r.re.MatchString(p)
	}
	return globMatch(r.parts, lower)
}

// globMatch 判断 s 是否与按 * 切开的片段整体匹配：首段为前缀、末段为后缀，中间各段依次出现。
func globMatch(parts []string, s string) bool {
	if len(parts) == 1 {
		return s == parts[0]
	}
	first, last := parts[0], parts[len(parts)-1]
	if len(s) < len(first)+len(last) || !strings.HasPrefix(s, first) || !strings.HasSuffix(s, last) {
		return false
	}
	mid := s[len(first) : len(s)-len(last)]
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(mid, part)
		if i < 0 {
			return false
		}
		mid = mid[i+len(part):]
	}
	return true
}

// compilePaths 编译路径规则：每行一条，以 ~ 开头为正则（按 Go 正则语法，不自动锚定），
// 其余为通配，* 匹配任意字符（含 /），整条路径匹配且不区分大小写。
func compilePaths(text string) ([]pathRule, error) {
	var out []pathRule
	for _, l := range splitLines(text) {
		if rest, ok := strings.CutPrefix(l, "~"); ok {
			expr := strings.TrimSpace(rest)
			if expr == "" {
				return nil, fmt.Errorf("路径规则 %q 缺少正则", l)
			}
			re, err := regexp.Compile(expr)
			if err != nil {
				return nil, fmt.Errorf("路径规则 %q 无效: %v", l, err)
			}
			out = append(out, pathRule{re: re})
			continue
		}
		if !strings.HasPrefix(l, "/") && !strings.HasPrefix(l, "*") {
			return nil, fmt.Errorf("路径规则 %q 应以 / 或 * 开头", l)
		}
		out = append(out, pathRule{parts: strings.Split(strings.ToLower(l), "*")})
	}
	return out, nil
}

// webMatcher 是编译后的 Web 路径探测规则。
type webMatcher struct {
	// listeners 为限定的监听 key，为空表示全部监听
	listeners   map[string]bool
	paths       []pathRule
	ignores     []pathRule
	anyStatus   bool
	unknownHost bool
}

func newWebMatcher(j Jail) (*webMatcher, error) {
	paths, err := compilePaths(j.Filter)
	if err != nil {
		return nil, err
	}
	ignores, err := compilePaths(j.Ignore)
	if err != nil {
		return nil, err
	}
	w := &webMatcher{paths: paths, ignores: ignores, anyStatus: j.AnyStatus, unknownHost: j.UnknownHost}
	if ls := splitLines(j.Target); len(ls) > 0 {
		w.listeners = map[string]bool{}
		for _, l := range ls {
			w.listeners[l] = true
		}
	}
	return w, nil
}

// matchPath 判断路径是否命中任一规则。同时比较原始路径与规范化后的路径，
// 防止用 //.env、/a/../.env 这类写法绕过。
func matchPath(rules []pathRule, p string) bool {
	if len(rules) == 0 {
		return false
	}
	if p == "" {
		p = "/"
	}
	if len(p) > maxMatchPath {
		p = p[:maxMatchPath/2] + p[len(p)-maxMatchPath/2:]
	}
	clean := p
	if strings.HasPrefix(p, "/") {
		clean = path.Clean(p)
	}
	lower, cleanLower := strings.ToLower(p), strings.ToLower(clean)
	for _, r := range rules {
		if r.match(p, lower) || (clean != p && r.match(clean, cleanLower)) {
			return true
		}
	}
	return false
}

// pathMatched 判断路径是否命中规则：未填写路径规则时匹配全部路径，命中排除规则时不计数。
func (w *webMatcher) pathMatched(p string) bool {
	if len(w.paths) > 0 && !matchPath(w.paths, p) {
		return false
	}
	return !matchPath(w.ignores, p)
}

// match 判断一次请求是否计入规则。默认只统计 404 与 444（访问了不存在的内容或被断开连接），
// 避免把访问正常页面的用户和共用出口 IP 的用户算进去。
func (w *webMatcher) match(h webprobe.Hit) bool {
	if w.listeners != nil && !w.listeners[h.Listener] {
		return false
	}
	if w.unknownHost && !h.Unknown {
		return false
	}
	if !w.anyStatus && h.Status != http.StatusNotFound && h.Status != statusClosed {
		return false
	}
	return w.pathMatched(h.Path)
}

// refreshWebLocked 重建路径探测规则的快照，供每个请求无锁读取，调用方需持有 m.mu。
func (m *Module) refreshWebLocked() {
	var list []*jailRunner
	for _, r := range m.jails {
		// 停用的规则只是占位（m 为 nil），编译失败的规则没有 web
		if r.m != nil && r.jail.Source == SourceWeb && r.match != nil && r.match.web != nil {
			list = append(list, r)
		}
	}
	m.webJails.Store(&list)
	m.refreshTLSLocked()
}

// onWebHit 由 Web 服务在每个请求结束时同步调用，命中路径探测规则时计数。
func (m *Module) onWebHit(h webprobe.Hit) {
	p := m.webJails.Load()
	if p == nil || len(*p) == 0 || !h.Addr.IsValid() {
		return
	}
	a := h.Addr.Unmap()
	for _, r := range *p {
		if r.match.web.match(h) {
			// 主机名与路径由访客控制，%q 转义换行等控制字符，避免伪造日志行与通知内容
			r.record(a, func() string { return fmt.Sprintf("%s %q %d", h.Method, h.Host+h.Path, h.Status) })
		}
	}
}

// tlsMatcher 是编译后的 TLS 握手失败规则。
type tlsMatcher struct {
	// listeners 为限定的监听 key，为空表示全部监听
	listeners map[string]bool
	kinds     map[string]bool
}

// parseTLSKinds 解析每行一个的原因类别，未知类别报错，去重。
func parseTLSKinds(text string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, l := range splitLines(text) {
		if !slices.Contains(webprobe.TLSKinds, l) {
			return nil, fmt.Errorf("未知的握手失败类型 %q", l)
		}
		out[l] = true
	}
	return out, nil
}

// sortedKinds 按 webprobe.TLSKinds 的顺序返回已选类别，保证保存结果稳定。
func sortedKinds(m map[string]bool) []string {
	var out []string
	for _, k := range webprobe.TLSKinds {
		if m[k] {
			out = append(out, k)
		}
	}
	return out
}

func newTLSMatcher(j Jail) (*tlsMatcher, error) {
	kinds, err := parseTLSKinds(j.Filter)
	if err != nil {
		return nil, err
	}
	t := &tlsMatcher{kinds: kinds}
	if ls := splitLines(j.Target); len(ls) > 0 {
		t.listeners = map[string]bool{}
		for _, l := range ls {
			t.listeners[l] = true
		}
	}
	return t, nil
}

func (t *tlsMatcher) match(f webprobe.TLSFail) bool {
	if t.listeners != nil && !t.listeners[f.Listener] {
		return false
	}
	return t.kinds[f.Kind]
}

// refreshTLSLocked 重建 TLS 握手失败规则的快照，调用方需持有 m.mu。
func (m *Module) refreshTLSLocked() {
	var list []*jailRunner
	for _, r := range m.jails {
		if r.m != nil && r.jail.Source == SourceTLS && r.match != nil && r.match.tls != nil {
			list = append(list, r)
		}
	}
	m.tlsJails.Store(&list)
	// 没有启用的规则时注销处理函数，Web 服务每次握手失败只剩一次原子读取；模块停止后不再注册
	if m.started && len(list) > 0 {
		webprobe.SetTLSHandler(m.onTLSFail)
	} else {
		webprobe.SetTLSHandler(nil)
	}
}

// onTLSFail 由 Web 服务在每次 TLS 握手失败时同步调用，命中规则时计数。
func (m *Module) onTLSFail(f webprobe.TLSFail) {
	p := m.tlsJails.Load()
	if p == nil || len(*p) == 0 || !f.Addr.IsValid() {
		return
	}
	a := f.Addr.Unmap()
	for _, r := range *p {
		if r.match.tls.match(f) {
			// 原因文本来自对端（如 SNI 名称），%q 转义控制字符
			r.record(a, func() string { return fmt.Sprintf("%s %q", f.Kind, f.Reason) })
		}
	}
}
