package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
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
}

// headerKeys 是值为 "名称: 值" 列表（或名称到值的映射）的字段名（小写），只脱敏值的部分。
var headerKeys = map[string]bool{"headers": true, "requestheaders": true, "responseheaders": true}

// urlKeys 是值为地址的字段名（小写），地址里常夹带令牌，脱敏用户信息中的密码与查询参数的值。
var urlKeys = map[string]bool{"url": true, "webhookurl": true, "callbackurl": true}

// redactJSON 递归脱敏解码后的 JSON。
func redactJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
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
			case lk == "users":
				t[k] = redactUsers(val)
			case urlKeys[lk]:
				if s, ok := val.(string); ok {
					t[k] = redactURL(s)
					continue
				}
				t[k] = redactJSON(val)
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
	}
	return v
}

// redactHeaders 脱敏请求头列表（"名称: 值"）或映射，保留名称。
func redactHeaders(v any) any {
	switch t := v.(type) {
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

// redactURL 脱敏地址中的密码和查询参数值；没有这些内容的地址原样返回。
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
