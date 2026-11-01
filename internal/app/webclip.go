package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

// 主屏幕图标与安全入口。
// iOS 把网页添加到主屏幕（Web Clip）时另行请求 apple-touch-icon，不带页面的 Cookie；开启安全入口后，
// 这个请求被当作未通过入口而返回 404，系统只能用标题首字母生成占位图标。
// 这里给页面中图标与 manifest 的地址追加由安全入口派生的签名参数 k，带有效签名的请求可读取这几份图标文件。
// 签名只出现在已通过入口（或已登录）后返回的页面里，不知道入口的访客仍然只能得到 404，无法据此识别出管理后台。

// homeIconFiles 是可凭签名参数读取的图标与 manifest 文件（相对前端根目录）。
var homeIconFiles = map[string]bool{
	"favicon.svg":           true,
	"favicon.ico":           true,
	"icon-192.png":          true,
	"icon-512.png":          true,
	"icon-maskable-512.png": true,
	"apple-touch-icon.png":  true,
	"manifest.webmanifest":  true,
}

// iconKey 返回安全入口对应的图标签名；与入口 Cookie 使用不同的派生标签，签名泄露也无法当作入口 Cookie 使用。
func (a *App) iconKey(safePath string) string {
	m := hmac.New(sha256.New, []byte(a.auth.EntrySecret()))
	m.Write([]byte("icon|" + safePath))
	return hex.EncodeToString(m.Sum(nil))[:24]
}

// iconKeyValid 判断请求的是图标文件且带有当前安全入口的有效签名。
func (a *App) iconKeyValid(r *http.Request, safePath string) bool {
	if safePath == "" || !homeIconFiles[strings.TrimPrefix(r.URL.Path, "/")] {
		return false
	}
	k := r.URL.Query().Get("k")
	return k != "" && hmac.Equal([]byte(k), []byte(a.iconKey(safePath)))
}

// signIconLinks 给 index.html 中的图标与 manifest 地址追加签名参数（地址已由 absolutizeAssets 改写为以前缀开头的绝对路径）。
func signIconLinks(index []byte, prefix, key string) []byte {
	s := string(index)
	for name := range homeIconFiles {
		url := prefix + "/" + name
		s = strings.ReplaceAll(s, `="`+url+`"`, `="`+url+"?k="+key+`"`)
	}
	return []byte(s)
}

// signManifest 给 manifest 中各图标的 src 追加签名参数。
// manifest 里的图标地址相对 manifest 本身解析，不会继承 manifest 地址上的查询参数，所以需要逐个改写。
func signManifest(data []byte, key string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	raw, ok := m["icons"]
	if !ok {
		return data, nil
	}
	var icons []map[string]any
	if err := json.Unmarshal(raw, &icons); err != nil {
		return nil, err
	}
	for _, icon := range icons {
		src, _ := icon["src"].(string)
		if src == "" {
			continue
		}
		sep := "?"
		if strings.Contains(src, "?") {
			sep = "&"
		}
		icon["src"] = src + sep + "k=" + key
	}
	raw, err := json.Marshal(icons)
	if err != nil {
		return nil, err
	}
	m["icons"] = raw
	return json.Marshal(m)
}

// serveSignedManifest 在请求带有效图标签名时返回图标地址已签名的 manifest；返回 false 表示按普通静态文件处理。
func (a *App) serveSignedManifest(w http.ResponseWriter, r *http.Request, web fs.FS) bool {
	safePath := a.settingsSnapshot().SafePath
	if !a.iconKeyValid(r, safePath) {
		return false
	}
	data, err := fs.ReadFile(web, "manifest.webmanifest")
	if err != nil {
		return false
	}
	signed, err := signManifest(data, a.iconKey(safePath))
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(signed)
	return true
}
