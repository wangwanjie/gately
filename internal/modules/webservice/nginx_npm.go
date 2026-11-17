package webservice

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// Nginx Proxy Manager（NPM）生成的配置是普通的 nginx 配置，但有几处固定写法需要专门处理：
//   - 代理主机在 server 中用 set $forward_scheme / $server / $port 设置后端，location / 里 include 的
//     conf.d/include/proxy.conf 再用 proxy_pass $forward_scheme://$server:$port$request_uri 转发；
//   - 「Force SSL」「Block Common Exploits」「Cache Assets」等开关对应 conf.d/include 下的固定片段；
//   - 容器自带的占位页（server_name localhost-nginx-proxy-manager）、443 兜底与管理界面（端口 81）不是用户的站点。

// isNPMConfig 判断配置是否由 Nginx Proxy Manager 生成：代理主机用 $forward_scheme 写后端，站点文件位于 /data/nginx 下。
func isNPMConfig(text string) bool {
	return strings.Contains(text, "$forward_scheme") || strings.Contains(text, "/data/nginx/")
}

// npmSnippetNames 是 NPM 自带、导入时专门处理的片段（conf.d/include 下的文件名）
var npmSnippetNames = []string{"proxy.conf", "force-ssl.conf", "assets.conf", "block-exploits.conf", "letsencrypt-acme-challenge.conf"}

// npmSnippet 返回指令所在文件对应的 NPM 片段名，不是 NPM 片段时返回空串。
func npmSnippet(file string) string {
	dir, name := path.Split(file)
	if !strings.HasSuffix(dir, "conf.d/include/") || !slices.Contains(npmSnippetNames, name) {
		return ""
	}
	return name
}

// npmBuiltinSnippets 为只粘贴了 /data/nginx 下的站点文件、没有 NPM 自带片段时使用的内容（取自 NPM 的镜像），
// 只保留导入用得到的指令
var npmBuiltinSnippets = map[string]string{
	"conf.d/include/proxy.conf": `add_header       X-Served-By $host;
proxy_set_header Host $host;
proxy_set_header X-Forwarded-Scheme $scheme;
proxy_set_header X-Forwarded-Proto  $scheme;
proxy_set_header X-Forwarded-For    $proxy_add_x_forwarded_for;
proxy_set_header X-Real-IP          $remote_addr;
proxy_pass       $forward_scheme://$server:$port$request_uri;`,
	"conf.d/include/force-ssl.conf": `set $test "";
if ($scheme = "http") { set $test "H"; }
if ($test = H) { return 301 https://$host$request_uri; }`,
	"conf.d/include/assets.conf": `location ~* ^.*\.(css|js|jpe?g|gif|png|webp|woff|woff2|eot|ttf|svg|ico|css\.map|js\.map)$ {
	proxy_cache public-cache;
}`,
	"conf.d/include/block-exploits.conf":             `set $block_sql_injections 0;`,
	"conf.d/include/letsencrypt-acme-challenge.conf": `location ^~ /.well-known/acme-challenge/ { root /data/letsencrypt-acme-challenge; }`,
}

// npmBuiltinSnippet 返回 include 参数对应的内置 NPM 片段（已解析），不是 NPM 片段时返回 nil。
func npmBuiltinSnippet(arg string) []*ngDirective {
	name := strings.TrimPrefix(path.Clean(arg), "/etc/nginx/")
	text, ok := npmBuiltinSnippets[name]
	if !ok {
		return nil
	}
	toks, err := tokenizeNginx(text, 1)
	if err != nil {
		return nil
	}
	p := &ngParser{toks: toks, file: name}
	out, err := p.parseBlock(false)
	if err != nil {
		return nil
	}
	return out
}

// matchIncludeSuffix 按路径结尾匹配相对 include：conf.d/include/*.conf 匹配任意目录下的 conf.d/include/ 中的文件。
func matchIncludeSuffix(pattern string, files []string) []string {
	pattern = path.Clean(pattern)
	if pattern == "." || strings.HasPrefix(pattern, "../") {
		return nil
	}
	n := strings.Count(pattern, "/") + 1
	var out []string
	for _, f := range files {
		parts := strings.Split(f, "/")
		// 只匹配 pattern 之前还有上级目录的文件（f 是绝对路径，parts[0] 为空串）
		if len(parts) <= n {
			continue
		}
		if ok, _ := path.Match(pattern, strings.Join(parts[len(parts)-n:], "/")); ok {
			out = append(out, f)
		}
	}
	return out
}

// isNPMBuiltinServer 判断 server 块是否为 NPM 容器自带的占位页、443 兜底或管理界面，它们不是用户添加的站点。
func isNPMBuiltinServer(srv *ngDirective) bool {
	switch srv.File {
	case "/etc/nginx/conf.d/default.conf", "/etc/nginx/conf.d/production.conf":
		return true
	}
	for _, d := range srv.Block {
		if d.Name == "server_name" && (slices.Contains(d.Args, "localhost-nginx-proxy-manager") || slices.Contains(d.Args, "nginxproxymanager")) {
			return true
		}
	}
	return false
}

// npmAssetExts 为 NPM「Cache Assets」缓存的静态资源扩展名（assets.conf 中的正则）
var npmAssetExts = []string{"css", "js", "jpg", "jpeg", "gif", "png", "webp", "woff", "woff2", "eot", "ttf", "svg", "ico", "map"}

// npmAssetCache 返回与 NPM「Cache Assets」对应的缓存设置：静态资源忽略后端缓存指令缓存 30 分钟，其他路径不缓存。
func npmAssetCache() CacheConfig {
	c := CacheConfig{Enable: true, Rules: []CacheRule{{Path: "/", NoCache: true}}}
	for _, ext := range npmAssetExts {
		c.Rules = append(c.Rules, CacheRule{Path: "/*." + ext, TTL: 1800, Override: true})
	}
	return c
}

// npmCertHint 在 HTTPS 站点使用 NPM 管理的证书时返回导入提示：证书文件在 NPM 容器内，不会被自动读取。
func npmCertHint(files []string) string {
	for _, f := range files {
		for _, mount := range []string{"/etc/letsencrypt/", "/data/"} {
			rest, ok := strings.CutPrefix(f, mount)
			if !ok || !strings.Contains(rest, "/npm-") {
				continue
			}
			return fmt.Sprintf("证书文件 %s 在 Nginx Proxy Manager 容器内，对应宿主机上 NPM 挂载到 %s 的目录中的 %s：可在「SSL 证书」页添加证书，来源选「读取文件」（Gately 在 Docker 中时先把该目录挂载进来），也可以直接重新签发", path.Dir(f)+"/", mount, path.Dir(rest)+"/")
		}
	}
	return ""
}

// noteNPMSnippet 记录站点用到的 NPM 片段，这些片段的指令不再逐条转换。
func (c *nginxConverter) noteNPMSnippet(name string) {
	switch name {
	case "force-ssl.conf":
		c.npmForceSSL = true
	case "assets.conf":
		c.npmAssets = true
	case "block-exploits.conf":
		c.npmExploits = true
	}
}

// nginxMaps 是 http 级 map 指令的取值。
type nginxMaps struct {
	// defaults 为各 map 的默认值，只用于近似解析认证跳转地址中的变量
	defaults map[string]string
	// consts 为只有 default 一项的 map，取值与请求无关（如 NPM 的 map $host $forward_scheme { default http; }）
	consts map[string]string
	// scheme 为按 $scheme 取值的 map（如 NPM 的 HSTS 头），每个监听的取值是确定的
	scheme map[string]map[string]string
}

// addMap 记录一个 map 块。同名变量重复定义时与 nginx 一样以最后一个为准。
func (m *nginxMaps) addMap(d *ngDirective) {
	name := strings.TrimPrefix(d.Args[1], "$")
	entries := map[string]string{}
	other := false
	for _, x := range d.Block {
		if len(x.Args) != 1 {
			// hostnames、volatile 等参数或格式不对的项：不当作常量
			other = true
			continue
		}
		entries[x.Name] = x.arg(0)
	}
	delete(m.consts, name)
	delete(m.scheme, name)
	if v, ok := entries["default"]; ok {
		m.defaults[name] = v
		if len(entries) == 1 && !other {
			m.consts[name] = v
		}
	}
	if d.Args[0] == "$scheme" && !other {
		m.scheme[name] = entries
	}
}

// schemeValue 返回按 $scheme 取值的 map 在指定协议下的值；没有对应项与默认值时为空串（nginx 不输出值为空的响应头）。
func schemeValue(entries map[string]string, tls bool) string {
	key := "http"
	if tls {
		key = "https"
	}
	if v, ok := entries[key]; ok {
		return v
	}
	return entries["default"]
}

// topSetVars 返回指令列表中顶层 set 定义的变量（不含 if 块中的赋值）。
func topSetVars(list []*ngDirective) map[string]string {
	vars := map[string]string{}
	for _, d := range list {
		if d.Name == "set" && len(d.Args) >= 2 {
			vars[strings.TrimPrefix(d.Args[0], "$")] = d.Args[1]
		}
	}
	return vars
}

// ifSetVars 返回在 if 块中赋值的变量：它们的值随请求变化，无法静态确定。
func ifSetVars(list []*ngDirective, inIf bool, out map[string]bool) {
	for _, d := range list {
		switch {
		case d.Name == "set" && inIf:
			out[strings.TrimPrefix(d.arg(0), "$")] = true
		case d.isBlock():
			ifSetVars(d.Block, inIf || d.Name == "if", out)
		}
	}
}

// staticValue 用能静态确定的变量值替换 v 中的变量：loc 中顶层的 set 优先于 server 中的 set，其次是只有默认值的 map
// 与按 $scheme 取值的 map。在 if 块中赋值的变量与运行时变量（$host、$request_uri 等）保持原样。
func (c *nginxConverter) staticValue(v string, loc []*ngDirective) string {
	vars := c.srvVars
	if lv := topSetVars(loc); len(lv) > 0 {
		vars = maps.Clone(c.srvVars)
		maps.Copy(vars, lv)
	}
	for range 4 {
		next := reNginxVar.ReplaceAllStringFunc(v, func(m string) string {
			name := strings.Trim(strings.TrimPrefix(m, "$"), "{}")
			if c.dynVars[name] {
				return m
			}
			if val, ok := vars[name]; ok {
				return val
			}
			if val, ok := c.maps.consts[name]; ok {
				return val
			}
			if e, ok := c.maps.scheme[name]; ok {
				return schemeValue(e, c.tls)
			}
			return m
		})
		if next == v {
			break
		}
		v = next
	}
	return v
}
