package webservice

import (
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// 身份请求头可传递的字段。
const (
	IdentityUser   = "user"   // 用户名：站点用户、共享用户的用户名，第三方登录为 提供方:标识，转发认证为 Remote-User
	IdentityEmail  = "email"  // 邮箱
	IdentityName   = "name"   // 显示名
	IdentityGroups = "groups" // 分组，多个用逗号分隔
)

// maxIdentityHeaders 限制每个站点的身份请求头条数。
const maxIdentityHeaders = 16

// IdentityHeader 把认证用户的一项身份信息写入请求头传给后端（如 Remote-Email）。
// 访客自带的同名请求头（含用下划线代替连字符的写法）在每个请求上都会被删除，没有可信的值时不传。
// 邮箱、显示名、分组只来自共享用户的设置或转发认证复制回来的 Remote-Email、Remote-Name、Remote-Groups，
// 站点用户与第三方登录只有用户名。
type IdentityHeader struct {
	Header string `json:"header"`
	// Field 为 user、email、name、groups 之一
	Field string `json:"field"`
}

// identityFields 为合法的字段；值为 true 表示只有共享用户或转发认证才有该信息。
var identityFields = map[string]bool{IdentityUser: false, IdentityEmail: true, IdentityName: true, IdentityGroups: true}

// forbiddenUserHeaders 是不能用来传递身份信息的请求头：会影响转发本身或与认证冲突
var forbiddenUserHeaders = map[string]bool{
	"Host": true, "Cookie": true, "Authorization": true, "Proxy-Authorization": true,
	"Content-Length": true, "Content-Type": true, "Transfer-Encoding": true, "Connection": true,
	"Upgrade": true, "Te": true, "Trailer": true, "Keep-Alive": true, "Proxy-Connection": true,
	"X-Forwarded-For": true, "X-Forwarded-Host": true, "X-Forwarded-Proto": true, "X-Forwarded-Port": true, "X-Real-Ip": true,
	"X-Real-Port": true, "X-Forwarded-For-Port": true,
}

// headerVariantKey 返回请求头的比较键：不区分大小写，- 与 _ 视为相同（部分后端把两者映射为同一个变量）。
func headerVariantKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// headerVariantEqual 判断两个请求头名称是否属于同一变体（ASCII 大小写不敏感，- 与 _ 等价），与比较
// headerVariantKey 的结果相同，但不分配内存：每个反代请求都要对全部请求头逐个比较。
func headerVariantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x == y {
			continue
		}
		if x == '_' {
			x = '-'
		} else if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y == '_' {
			y = '-'
		} else if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// identityHeaders 返回站点生效的身份请求头。旧配置的 UserHeader 视为一行“用户名”，保存时并入 IdentityHeaders；
// 加载配置不经过校验，运行时也要认它，否则升级后未重新保存的站点会停止传递用户名，访客伪造的同名头也不再被删除。
func (s *Site) identityHeaders() []IdentityHeader {
	if strings.TrimSpace(s.UserHeader) == "" {
		return s.IdentityHeaders
	}
	for _, ih := range s.IdentityHeaders {
		if headerVariantKey(strings.TrimSpace(ih.Header)) == headerVariantKey(strings.TrimSpace(s.UserHeader)) {
			return s.IdentityHeaders
		}
	}
	return append([]IdentityHeader{{Header: strings.TrimSpace(s.UserHeader), Field: IdentityUser}}, s.IdentityHeaders...)
}

// validateIdentityHeaders 整理并校验身份请求头，调用前已确认站点为反向代理且启用了认证。
func (s *Site) validateIdentityHeaders() error {
	s.UserHeader = strings.TrimSpace(s.UserHeader)
	// 旧版用户名请求头与列表中传其他字段的行同名时，运行时以列表为准，保存时明确报出，不悄悄丢掉用户名
	for _, ih := range s.IdentityHeaders {
		if s.UserHeader != "" && headerVariantKey(strings.TrimSpace(ih.Header)) == headerVariantKey(s.UserHeader) && strings.TrimSpace(ih.Field) != IdentityUser {
			return fmt.Errorf("身份请求头 %s 重复", strings.TrimSpace(ih.Header))
		}
	}
	list := s.identityHeaders()
	s.UserHeader = ""
	if len(list) > maxIdentityHeaders {
		return fmt.Errorf("身份请求头最多 %d 条", maxIdentityHeaders)
	}
	// 站点与分路径规则的请求头设置在身份请求头之后写入，同名时会覆盖认证结果，相当于给所有人一个固定身份
	configured := map[string]string{}
	for _, line := range s.RequestHeaders {
		k, _, _ := strings.Cut(line, ":")
		configured[headerVariantKey(strings.TrimSpace(k))] = ""
	}
	for _, l := range s.Locations {
		for _, line := range l.RequestHeaders {
			k, _, _ := strings.Cut(line, ":")
			if _, ok := configured[headerVariantKey(strings.TrimSpace(k))]; !ok {
				configured[headerVariantKey(strings.TrimSpace(k))] = l.Prefix
			}
		}
	}
	out := make([]IdentityHeader, 0, len(list))
	seen := map[string]bool{}
	for i, ih := range list {
		ih.Header, ih.Field = strings.TrimSpace(ih.Header), strings.TrimSpace(ih.Field)
		if ih.Header == "" {
			return fmt.Errorf("第 %d 条身份请求头需要填写请求头名称", i+1)
		}
		if !httpguts.ValidHeaderFieldName(ih.Header) {
			return fmt.Errorf("身份请求头 %q 无效", ih.Header)
		}
		ih.Header = textproto.CanonicalMIMEHeaderKey(ih.Header)
		if forbiddenUserHeaders[ih.Header] || forbiddenUserHeaders[textproto.CanonicalMIMEHeaderKey(strings.ReplaceAll(ih.Header, "_", "-"))] {
			return fmt.Errorf("不能用 %s 传递身份信息", ih.Header)
		}
		needsShared, ok := identityFields[ih.Field]
		if !ok {
			return fmt.Errorf("身份请求头 %s 的字段 %q 无效", ih.Header, ih.Field)
		}
		// 站点用户与第三方登录没有这些信息，提前提示，避免配置了却始终收不到
		if needsShared && !s.SharedUsers && s.ForwardAuth.URL == "" {
			return errors.New("传递邮箱、显示名或分组需要启用共享用户或转发认证")
		}
		key := headerVariantKey(ih.Header)
		if seen[key] {
			return fmt.Errorf("身份请求头 %s 重复", ih.Header)
		}
		seen[key] = true
		if prefix, ok := configured[key]; ok {
			if prefix == "" {
				return fmt.Errorf("身份请求头 %s 与站点的请求头设置冲突", ih.Header)
			}
			return fmt.Errorf("身份请求头 %s 与分路径规则 %s 的请求头设置冲突", ih.Header, prefix)
		}
		out = append(out, ih)
	}
	s.IdentityHeaders = out
	if len(out) == 0 {
		s.IdentityHeaders = nil
	}
	return nil
}

// stripIdentityHeaders 删除访客自带的身份请求头，防止伪造身份。无论路径是否需要认证都执行
func (h *siteHandler) stripIdentityHeaders(r *http.Request) {
	for _, ih := range h.site.identityHeaders() {
		delHeaderVariants(r.Header, ih.Header)
	}
}

// delHeaderVariants 删除名称与 name 相同的请求头，包括用下划线代替连字符的变体（如 X_Remote_User）：
// 部分后端（CGI、WSGI、PHP 等）把 - 与 _ 都映射为同一个变量，只删精确同名的头仍可被伪造。
func delHeaderVariants(h http.Header, name string) {
	for k := range h {
		if headerVariantEqual(k, name) {
			delete(h, k)
		}
	}
	// 反向代理会先删掉 Connection 中列出的逐跳头，再交给后端：访客发 Connection: Remote-Email 就能让
	// Gately 写入的身份头消失。这里把同名令牌从 Connection 中去掉，保证认证通过的请求一定带上身份头
	for _, ck := range []string{"Connection", "Proxy-Connection"} {
		vals, ok := h[ck]
		if !ok {
			continue
		}
		var keep []string
		changed := false
		for _, v := range vals {
			var toks []string
			for _, tok := range strings.Split(v, ",") {
				if tt := strings.TrimSpace(tok); tt != "" && headerVariantEqual(tt, name) {
					changed = true
					continue
				}
				toks = append(toks, tok)
			}
			if joined := strings.TrimSpace(strings.Join(toks, ",")); joined != "" {
				keep = append(keep, joined)
			}
		}
		if !changed {
			continue
		}
		if len(keep) == 0 {
			delete(h, ck)
		} else {
			h[ck] = keep
		}
	}
}

// visitor 是认证识别出的访客身份。
type visitor struct {
	// names 为身份标识，第一项为主标识，见 sessionNames；为空表示未识别出身份
	names []string
	// email、name、groups 为来源可信的身份信息：共享用户的设置，或转发认证复制回来的 Remote-Email、
	// Remote-Name、Remote-Groups；站点用户与第三方登录为空。groups 为逗号分隔的分组
	email, name, groups string
}

// sharedVisitor 返回共享用户的身份。
func sharedVisitor(u AuthUser) visitor {
	return visitor{names: []string{u.Name}, email: u.Email, name: u.DisplayName, groups: strings.Join(u.Groups, ",")}
}

// field 返回身份信息中的一项。
func (v visitor) field(f string) string {
	switch f {
	case IdentityUser:
		if len(v.names) > 0 {
			return v.names[0]
		}
	case IdentityEmail:
		return v.email
	case IdentityName:
		return v.name
	case IdentityGroups:
		return v.groups
	}
	return ""
}

// setIdentityHeaders 把访客的身份信息写入请求头，返回附加到响应缓存键中的标记。
// 未识别出身份（未登录或经分享链接访问）时不传任何身份信息；值为空或含控制字符等非法字符时不写，
// 访客自带的同名头已在认证前删除，后端只会收不到该头。
func (h *siteHandler) setIdentityHeaders(r *http.Request, v visitor) string {
	if len(v.names) == 0 {
		return ""
	}
	var tag strings.Builder
	for i, ih := range h.site.identityHeaders() {
		val := v.field(ih.Field)
		if val == "" || !httpguts.ValidHeaderFieldValue(val) {
			continue
		}
		r.Header.Set(ih.Header, val)
		// 后端可能按身份返回不同内容，缓存也按传出的值区分；值带长度前缀，避免含分隔符的值拼出与他人相同的标记
		tag.WriteString("|" + strconv.Itoa(i) + "." + strconv.Itoa(len(val)) + ":" + val)
	}
	return tag.String()
}
