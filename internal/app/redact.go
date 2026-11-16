package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/crud"
)

// 只读身份（只读账号、只有 read 权限的 API 令牌）不能拿到凭据明文：
// 站点共享用户的密码、分享链接令牌、自定义请求头里的 Authorization、Webhook 地址里的令牌等。
// 管理员与具备对应模块写权限的令牌不受影响，仍可看到原值（与 /secrets 接口按写权限放行的规则一致）。

// secretKeys 是值为字符串时一律脱敏的字段名（小写）。
var secretKeys = map[string]bool{
	"password": true, "passwordhash": true, "secret": true, "token": true, "authsecret": true,
	"privatekey": true, "passphrase": true, "clientsecret": true, "apitoken": true, "secretkey": true,
	"adminpassword": true, "totpsecret": true, "psk": true, "sessionsecret": true,
	// 网络唤醒客户端凭据：设备列表本身已脱敏，这里兜住配置历史对比等其他返回原始配置的接口
	"agenttoken": true,
	// 以下字段各模块的列表接口本身已脱敏，同样在这里兜底：组网密钥与认证密钥、端口敲门密钥、ACME EAB 密钥、
	// frps 面板密码、OVH 的 Consumer Key、终端两步验证密钥、OIDC 签名私钥、证书私钥、双层部署的认证标记密钥
	"authkey": true, "setupkey": true, "networksecret": true, "enckey": true, "hmackey": true, "eabhmac": true,
	"dashboardpwd": true, "consumerkey": true, "twofakey": true, "signingkey": true, "keypem": true, "authmarkerkey": true,
}

// opaqueKeys 是内容由用户自由填写、常夹带密钥的字段名（小写），只读身份一律看不到内容：
// 命令与脚本（如 curl -H 'Authorization: …'、mysql -p…）。
var opaqueKeys = map[string]bool{"command": true, "script": true, "shutdowncommand": true}

// isRequestBody 判断对象 m 中的 body 字段是否为 HTTP 请求体模板（Webhook、网络唤醒的 HTTP 关机等，
// 常含接口密钥，如 {"token":"…"} 或表单 key=…）：同一对象里有 method 或 headers 字段时即是。
// 版本更新说明、导航面板图标等其他名为 body 的字段不受影响。
func isRequestBody(m map[string]any) bool {
	for k := range m {
		if lk := strings.ToLower(k); lk == "method" || lk == "headers" {
			return true
		}
	}
	return false
}

// extraPlainKeys 是 DNS 服务商凭据的附加字段（credentials.extra）中不属于密钥的字段名（小写），其余附加字段一律脱敏。
// 新增服务商的附加字段默认按密钥处理，确认不含凭据后再加入这里。
var extraPlainKeys = map[string]bool{
	"endpoint": true, "authtype": true, "proxied": true, "server": true, "url": true, "method": true,
	"successcontent": true, "hostedzoneid": true, "teamid": true,
}

// headerKeys 是值为 "名称: 值" 列表（或名称到值的映射）的字段名（小写），只脱敏值的部分。
var headerKeys = map[string]bool{"headers": true, "requestheaders": true, "responseheaders": true}

// redactJSON 递归脱敏解码后的 JSON。
func redactJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		requestBody := isRequestBody(t)
		for k, val := range t {
			lk := strings.ToLower(k)
			switch {
			case secretKeys[lk] || strings.Contains(lk, "password") && !strings.HasPrefix(lk, "has") && !strings.HasSuffix(lk, "enabled"):
				if s, ok := val.(string); ok {
					if s != "" {
						t[k] = crud.SecretMask
					}
					continue
				}
				t[k] = redactJSON(val)
			case headerKeys[lk]:
				t[k] = redactHeaders(val)
			case strings.Contains(lk, "args") || opaqueKeys[lk] || lk == "body" && requestBody:
				// 附加启动参数常写有密钥（如 --network-secret=…、--auth-key=…），命令、脚本与请求体同样常夹带密钥，整段脱敏
				t[k] = redactOpaque(val)
			case strings.HasSuffix(lk, "env"):
				// env、daemonEnv 等环境变量
				t[k] = redactEnv(val)
			case lk == "users":
				t[k] = redactUsers(val)
			case lk == "credentials":
				t[k] = redactCredentials(val)
			default:
				t[k] = redactJSON(val)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactJSON(t[i])
		}
		return t
	case string:
		// 地址里常夹带凭据（后端地址 http://user:pass@…、Webhook 的 ?token=… 或路径中的令牌），
		// 任何字段中的地址都脱敏，包括夹在日志、错误信息等文本中间的地址
		if strings.Contains(t, "://") {
			return urlPattern.ReplaceAllStringFunc(t, redactURL)
		}
	}
	return v
}

// urlPattern 匹配文本中的地址，到空白、引号或尖括号为止。
var urlPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`)

// tokenLike 判断路径中的一段是否像令牌：至少 16 个字符且同时含字母与数字，
// 如 Discord、Slack、飞书等 Webhook 地址路径里的密钥，Bark 的设备 Key。
func tokenLike(seg string) bool {
	if len(seg) < 16 {
		return false
	}
	letter, digit := false, false
	for _, c := range seg {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			letter = true
		}
	}
	return letter && digit
}

// redactOpaque 整段隐藏自由文本：非空字符串换成掩码，字符串列表逐项处理，映射隐藏全部值（保留键名）。
func redactOpaque(v any) any {
	switch t := v.(type) {
	case string:
		if t != "" {
			return crud.SecretMask
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactOpaque(t[i])
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = redactOpaque(t[k])
		}
		return t
	}
	return v
}

// redactCredentials 处理 DNS 服务商凭据（{id, secret, extra}）：secret 按普通规则脱敏，
// extra 中只有 extraPlainKeys 列出的字段保留（其中的地址仍按地址规则脱敏），其余一律隐藏。
// 其他形态的 credentials（如 Web 终端的凭据列表）按普通规则处理。
func redactCredentials(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return redactJSON(v)
	}
	if extra, ok := m["extra"].(map[string]any); ok {
		for k, val := range extra {
			if extraPlainKeys[strings.ToLower(k)] {
				extra[k] = redactJSON(val)
				continue
			}
			extra[k] = redactOpaque(val)
		}
		delete(m, "extra")
		redactJSON(m)
		m["extra"] = extra
		return m
	}
	return redactJSON(m)
}

// redactHeaders 脱敏请求头列表（"名称: 值"）、每行一个 "名称: 值" 的文本或映射，保留名称。
func redactHeaders(v any) any {
	switch t := v.(type) {
	case string:
		// DDNS 自定义回调等把请求头写成多行文本
		lines := strings.Split(t, "\n")
		for i, line := range lines {
			if name, val, found := strings.Cut(line, ":"); found && strings.TrimSpace(val) != "" {
				lines[i] = name + ": " + crud.SecretMask
			} else if !found && strings.TrimSpace(line) != "" {
				// 没有冒号的行无法区分名称与值，整行隐藏
				lines[i] = crud.SecretMask
			}
		}
		return strings.Join(lines, "\n")
	case []any:
		for i, e := range t {
			s, ok := e.(string)
			if !ok {
				t[i] = redactJSON(e)
				continue
			}
			if name, val, found := strings.Cut(s, ":"); found && strings.TrimSpace(val) != "" {
				t[i] = name + ": " + crud.SecretMask
			}
		}
		return t
	case map[string]any:
		for k, e := range t {
			if s, ok := e.(string); ok && s != "" {
				t[k] = crud.SecretMask
			}
		}
		return t
	}
	return v
}

// redactUsers 脱敏站点的 "用户名:密码" 列表；元素是对象时按普通规则处理。
func redactUsers(v any) any {
	arr, ok := v.([]any)
	if !ok {
		return redactJSON(v)
	}
	for i, e := range arr {
		s, isStr := e.(string)
		if !isStr {
			arr[i] = redactJSON(e)
			continue
		}
		if name, pass, found := strings.Cut(s, ":"); found && pass != "" {
			arr[i] = name + ":" + crud.SecretMask
		}
	}
	return arr
}

// redactURL 脱敏地址中的密码、像令牌的路径段和查询参数值；没有这些内容的地址原样返回。
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return s
	}
	changed := false
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), crud.SecretMask)
			changed = true
		}
	}
	// 按转义后的路径逐段判断，替换后保留原有转义，掩码中的 * 不会被转义成 %2A
	if segs := strings.Split(u.EscapedPath(), "/"); len(segs) > 1 {
		masked := false
		for i, seg := range segs {
			if tokenLike(seg) {
				segs[i] = crud.SecretMask
				masked = true
			}
		}
		if raw := strings.Join(segs, "/"); masked {
			if p, err := url.PathUnescape(raw); err == nil {
				u.Path, u.RawPath = p, raw
				changed = true
			}
		}
	}
	if q := u.Query(); len(q) > 0 {
		for k := range q {
			q[k] = []string{crud.SecretMask}
		}
		u.RawQuery = q.Encode()
		changed = true
	}
	if !changed {
		return s
	}
	return u.String()
}

// needsRedaction 判断当前身份对本次读取是否只有读权限（没有该模块的写权限）。
func needsRedaction(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	need := apitoken.Required(r.Method, r.URL.Path)
	mod, _, _ := strings.Cut(need, ":")
	if need == apitoken.Any || strings.Contains(need, ",") || mod == "" {
		return false
	}
	return !auth.Allowed(r.Context(), mod+":"+apitoken.LevelWrite)
}

// redactReadOnly 对只读身份的 JSON 响应做凭据脱敏，其他响应（下载、流、WebSocket）原样透传。
func (a *App) redactReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !needsRedaction(r) {
			next.ServeHTTP(w, r)
			return
		}
		rw := &redactWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		rw.finish()
	})
}

// redactWriter 只缓冲 JSON 响应；其余响应在写头时直接放行。
type redactWriter struct {
	http.ResponseWriter
	decided bool
	buffer  bool
	status  int
	buf     bytes.Buffer
}

func (w *redactWriter) decide(code int) {
	if w.decided {
		return
	}
	w.decided = true
	w.status = code
	h := w.Header()
	w.buffer = strings.HasPrefix(h.Get("Content-Type"), "application/json") && h.Get("Content-Encoding") == ""
	if !w.buffer {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *redactWriter) WriteHeader(code int) { w.decide(code) }

func (w *redactWriter) Write(b []byte) (int, error) {
	w.decide(http.StatusOK)
	if w.buffer {
		return w.buf.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *redactWriter) Flush() {
	if w.decided && !w.buffer {
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (w *redactWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *redactWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// finish 在处理器返回后写出脱敏后的 JSON；解析失败时保持原样，避免误伤。
func (w *redactWriter) finish() {
	if !w.buffer {
		return
	}
	body := w.buf.Bytes()
	if len(body) > 0 {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil {
			var out bytes.Buffer
			enc := json.NewEncoder(&out)
			enc.SetEscapeHTML(false)
			if enc.Encode(redactJSON(v)) == nil {
				body = out.Bytes()
			}
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(body)
}

// redactEnv 脱敏环境变量：保留变量名，隐藏非空的值。支持每行一个 KEY=VALUE 的文本与名称到值的映射。
func redactEnv(v any) any {
	switch t := v.(type) {
	case string:
		if t == "" {
			return t
		}
		lines := strings.Split(t, "\n")
		for i, line := range lines {
			name, val, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(val) != "" {
				lines[i] = name + "=" + crud.SecretMask
			} else if !ok && strings.TrimSpace(line) != "" {
				// 没有等号的行无法区分名称与值，整行隐藏
				lines[i] = crud.SecretMask
			}
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && s != "" {
				t[k] = crud.SecretMask
			}
		}
		return t
	}
	return redactJSON(v)
}
