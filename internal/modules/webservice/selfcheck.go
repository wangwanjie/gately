package webservice

import (
	"context"
	"crypto/tls"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/hostname"
	"github.com/wangwanjie/gately/internal/module"
)

// SelfCheck 实现 module.SelfChecker：检查 HTTPS 监听中启用的站点是否都有覆盖其域名的证书。
// 没有证书的域名在 TLS 握手阶段就会失败，浏览器只会提示连接错误，日志里也不容易看出原因。
func (m *Module) SelfCheck(_ context.Context, _ module.CheckEnv) []module.CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.certs == nil || !m.cfg.Settings.Enable {
		return nil
	}
	var out []module.CheckResult
	covered := 0
	for i := range m.cfg.Listeners {
		l := &m.cfg.Listeners[i]
		if !l.Enable || !l.TLS {
			continue
		}
		for j := range l.Sites {
			s := &l.Sites[j]
			if !s.Enable {
				continue
			}
			var missing []string
			for _, d := range s.Domains {
				name, ok := certCheckName(d)
				if !ok {
					continue
				}
				if _, err := m.certs.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil {
					missing = append(missing, d)
				}
			}
			if len(missing) > 0 {
				out = append(out, module.CheckResult{ID: "web.certMissing", Status: module.CheckWarn, Params: map[string]string{
					"site": listenerSiteLabel(l, s), "domains": strings.Join(missing, ", "), "port": strconv.Itoa(l.Port),
				}})
				continue
			}
			covered++
		}
	}
	if covered > 0 && len(out) == 0 {
		out = append(out, module.CheckResult{ID: "web.certOk", Status: module.CheckOK, Params: map[string]string{"count": strconv.Itoa(covered)}})
	}
	return out
}

// certCheckName 把站点域名转换为用于匹配证书的名称：通配符域名 *.example.com 需要通配符证书，
// 用一个子域名去匹配即可；“*” 这类兜底写法与无法匹配证书的模式跳过。
func certCheckName(d string) (string, bool) {
	d = strings.ToLower(strings.TrimSpace(d))
	switch {
	case d == "" || d == "*":
		return "", false
	case strings.HasPrefix(d, "*."):
		rest := d[2:]
		if strings.Contains(rest, "*") {
			return "", false
		}
		return "gately-check." + rest, true
	case strings.Contains(d, "*"):
		if name, ok := hostname.NormalizePattern(d); ok {
			return strings.Replace(name, "*", "x", 1), true
		}
		return "", false
	}
	return d, true
}
