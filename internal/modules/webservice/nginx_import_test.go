package webservice

import (
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
)

// 模拟 nginx -T 的输出：主配置 include 的站点文件以“# configuration file”标记分段
const sampleNginxT = `nginx: the configuration file /etc/nginx/nginx.conf syntax is ok
# configuration file /etc/nginx/nginx.conf:
events { worker_connections 1024; }
http {
    gzip on;
    upstream app_pool {
        least_conn;
        server 10.0.0.11:8080 weight=3;
        server 10.0.0.12:8080 backup;
        server 10.0.0.13:8080 down;
    }
    include /etc/nginx/conf.d/*.conf;
}
stream {
    server { listen 3306; proxy_pass 10.0.0.5:3306; }
}

# configuration file /etc/nginx/conf.d/sites.conf:
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl;
    listen 443 quic reuseport;
    server_name app.example.com www.app.example.com;
    ssl_certificate /etc/ssl/app/fullchain.pem;
    ssl_certificate_key /etc/ssl/app/privkey.pem;
    add_header Strict-Transport-Security "max-age=63072000" always;
    add_header X-Request-Id $request_id;
    location / {
        proxy_pass http://app_pool;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header X-App "gately";
    }
    location /api/ {
        proxy_pass http://127.0.0.1:9000;
    }
    location ^~ /admin {
        allow 10.0.0.0/8;
        allow 192.168.1.10;
        deny all;
    }
    location /legacy {
        return 301 https://new.example.com/;
    }
    location ~* \.(png|jpg)$ { expires 30d; }
}

server {
    listen 443 ssl;
    server_name .static.example.com;
    root /var/www/site;
    location / {
        try_files $uri $uri.html $uri/ =404;
        autoindex on;
    }
    error_page 404 /404.html;
}

server {
    listen 443 ssl;
    server_name nas.example.com;
    auth_basic "private";
    auth_basic_user_file /etc/nginx/.htpasswd;
    location / {
        proxy_pass https://192.168.1.2:5001/;
        proxy_ssl_server_name on;
        proxy_ssl_name $host;
    }
}

server {
    listen 8080;
    server_name health.example.com;
    location / { default_type application/json; return 200 '{"ok":true}'; }
}

server {
    listen 8080;
    server_name old.example.com;
    rewrite ^/(.*)$ https://example.com/$1 permanent;
}

server {
    listen 8080;
    server_name dyn.example.com;
    location / { proxy_pass http://$backend; }
}
`

func findSite(rep *NginxImportReport, name string) (*NginxListenerPlan, *NginxSitePlan, *Site) {
	for _, p := range rep.plans {
		for i := range p.sites {
			if p.sites[i].plan.Name == name {
				return p, p.sites[i].plan, &p.sites[i].site
			}
		}
	}
	return nil, nil, nil
}

func TestImportNginx(t *testing.T) {
	existing := []Listener{{Key: "web", Name: "已有 HTTPS", Port: 443, TLS: true, Sites: []Site{{Domains: []string{"www.app.example.com"}}}}}
	rep, err := importNginx(sampleNginxT, existing, nameCerts{"app.example.com": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("stream 应提示改用端口转发: %v", rep.Warnings)
	}
	if len(rep.plans) != 3 {
		t.Fatalf("应按 80、443、8080 分为 3 个监听: %+v", rep.Listeners)
	}

	// 80 端口：默认站点整体跳转 HTTPS
	p, pl, s := findSite(rep, "默认站点")
	if p == nil || p.Port != 80 || !pl.Default || s.Type != TypeRedirect || s.RedirectTo != "https://{host}" || !s.KeepPath || s.RedirectCode != 301 {
		t.Fatalf("默认跳转转换错误: %+v %+v", pl, s)
	}

	// 443：合并到已有监听，upstream 展开为带权重的后端，已存在的域名不重复导入
	p, pl, s = findSite(rep, "app.example.com")
	if p.ExistingKey != "web" || !p.HTTP3 {
		t.Fatalf("应合并到已有 443 监听并识别 QUIC: %+v", p)
	}
	if s.Type != TypeProxy || strings.Join(s.Targets, ",") != "http://10.0.0.11:8080 weight=3,http://10.0.0.12:8080 backup" || s.LBPolicy != "least_conn" {
		t.Fatalf("upstream 转换错误: %+v", s)
	}
	if !s.ForwardedHeaders || s.UseTargetHost || strings.Join(s.RequestHeaders, ",") != "X-App: gately" || !s.Compress {
		t.Fatalf("请求头与压缩转换错误: %+v", s)
	}
	if strings.Join(s.ResponseHeaders, ",") != "Strict-Transport-Security: max-age=63072000" {
		t.Fatalf("响应头转换错误: %v", s.ResponseHeaders)
	}
	if strings.Join(s.Domains, ",") != "app.example.com" || len(pl.CertMissing) != 0 || pl.CertFiles[0] != "/etc/ssl/app/fullchain.pem" {
		t.Fatalf("域名或证书检查错误: %+v", pl)
	}
	if len(s.Locations) != 1 || s.Locations[0].Prefix != "/api/" || s.Locations[0].Targets[0] != "http://127.0.0.1:9000" {
		t.Fatalf("分路径后端错误: %+v", s.Locations)
	}
	if len(s.Rules) != 1 || s.Rules[0].Prefix != "/admin" || s.Rules[0].IPMode != IPModeWhitelist || len(s.Rules[0].IPList) != 2 {
		t.Fatalf("路径访问控制错误: %+v", s.Rules)
	}
	if len(s.Rewrites) != 1 || !s.Rewrites[0].Redirect || s.Rewrites[0].To != "https://new.example.com/" {
		t.Fatalf("路径跳转错误: %+v", s.Rewrites)
	}
	joined := strings.Join(pl.Warnings, "\n")
	for _, want := range []string{"X-Request-Id", "正则 location", "已存在的域名"} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少提示 %q: %s", want, joined)
		}
	}
	if !strings.Contains(strings.Join(pl.Ignored, ","), "expires") && len(pl.Ignored) != 0 {
		t.Logf("未转换指令: %v", pl.Ignored)
	}

	// 静态站点：.example.com 展开、干净 URL、目录列表、404 页面
	_, pl, s = findSite(rep, "static.example.com")
	if s.Type != TypeStatic || s.Root != "/var/www/site" || !s.CleanURLs || !s.ListDir || s.NotFoundPage != "404.html" {
		t.Fatalf("静态站点转换错误: %+v", s)
	}
	if strings.Join(s.Domains, ",") != "static.example.com,*.static.example.com" || len(pl.CertMissing) != 2 {
		t.Fatalf("域名展开或证书缺失检查错误: %+v", pl)
	}

	// HTTPS 后端：默认不校验证书，SNI 使用访问域名；proxy_pass 带 / 等价于不带路径
	_, pl, s = findSite(rep, "nas.example.com")
	if s.Targets[0] != "https://192.168.1.2:5001" || !s.InsecureSkipVerify || !s.UpstreamSNIFromHost || !s.BasicAuth || !s.SharedUsers {
		t.Fatalf("HTTPS 后端转换错误: %+v", s)
	}

	// 8080：固定内容、rewrite 跳转、变量后端跳过
	p, _, s = findSite(rep, "health.example.com")
	if p.Port != 8080 || p.ExistingKey != "" || s.Type != TypeText || s.Text != `{"ok":true}` || s.ContentType != "application/json" || s.StatusCode != 200 {
		t.Fatalf("固定内容转换错误: %+v", s)
	}
	_, _, s = findSite(rep, "old.example.com")
	if s.Type != TypeRedirect || s.RedirectTo != "https://example.com" || !s.KeepPath || s.RedirectCode != 301 {
		t.Fatalf("rewrite 跳转转换错误: %+v", s)
	}
	_, pl, _ = findSite(rep, "dyn.example.com")
	if pl.Skipped == "" {
		t.Fatal("变量后端应跳过")
	}
	if rep.Sites != 6 || rep.Skipped != 1 {
		t.Fatalf("统计错误: sites=%d skipped=%d", rep.Sites, rep.Skipped)
	}
}

func TestApplyNginx(t *testing.T) {
	m := New(nil, nil)
	m.cfg.Listeners = []Listener{{Key: "web", Name: "已有 HTTPS", Port: 443, TLS: true, Sites: []Site{{Key: "s1", Domains: []string{"exist.example.com"}, Type: TypeText, Text: "x"}}}}
	st := t.TempDir()
	m.store = mustStore(t, st)
	rep, err := importNginx(sampleNginxT, m.cfg.Listeners, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.applyNginxLocked(rep, false); err != nil {
		t.Fatal(err)
	}
	if len(m.cfg.Listeners) != 3 {
		t.Fatalf("应新建 80 与 8080 监听: %d", len(m.cfg.Listeners))
	}
	web := m.cfg.Listeners[0]
	if len(web.Sites) != 4 || web.Sites[0].Key != "s1" {
		t.Fatalf("应合并到已有监听并保留原站点: %+v", web.Sites)
	}
	for _, s := range web.Sites[1:] {
		if s.Enable || s.Key == "" || !strings.Contains(s.Remark, "nginx") {
			t.Fatalf("导入的站点应为停用状态并带来源备注: %+v", s)
		}
	}
	for _, l := range m.cfg.Listeners[1:] {
		if l.Enable || l.Key == "" || !strings.HasPrefix(l.Name, "nginx :") {
			t.Fatalf("新建监听应为停用状态: %+v", l)
		}
	}
	if !m.cfg.Listeners[1].Default.Enable && m.cfg.Listeners[1].Default.Type != TypeRedirect {
		t.Fatalf("80 端口应导入默认跳转站点: %+v", m.cfg.Listeners[1].Default)
	}
	// 再导入一次：域名都已存在，全部跳过
	rep, err = importNginx(sampleNginxT, m.cfg.Listeners, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sites != 0 {
		t.Fatalf("重复导入应全部跳过: %+v", rep.Listeners)
	}
}

func TestParseNginxErrors(t *testing.T) {
	for _, bad := range []string{"server { listen 80;", "server { listen 80; }}", `server { return 200 "abc; }`, "server { listen 80 }"} {
		if _, _, err := parseNginx(bad); err == nil {
			t.Errorf("应报告语法错误: %q", bad)
		}
	}
	if _, err := importNginx("events {}", nil, nil); err == nil {
		t.Fatal("没有 server 块时应报错")
	}
}

func mustStore(t *testing.T, dir string) *config.Store {
	t.Helper()
	st, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// Authelia 官方示例风格：片段文件经 include 引入，http 级 map 提供门户域名
const sampleAuthelia = `# configuration file /etc/nginx/nginx.conf:
http {
    map $host $portal_host { default 'auth.example.com'; }
    include /etc/nginx/conf.d/*.conf;
}
# configuration file /etc/nginx/snippets/authelia-location.conf:
set $upstream_authelia http://127.0.0.1:9091/api/authz/auth-request;
location /internal/authelia/authz {
    internal;
    proxy_pass $upstream_authelia;
    proxy_set_header X-Original-URL $scheme://$http_host$request_uri;
}
# configuration file /etc/nginx/snippets/authelia-authrequest.conf:
auth_request /internal/authelia/authz;
auth_request_set $user $upstream_http_remote_user;
auth_request_set $groups $upstream_http_remote_groups;
proxy_set_header Remote-User $user;
proxy_set_header Remote-Groups $groups;
set $target_url $scheme://$http_host$request_uri;
error_page 401 =302 https://$portal_host/?rd=$target_url&rm=$request_method;
# configuration file /etc/nginx/conf.d/apps.conf:
server {
    listen 443 ssl;
    server_name code.example.com;
    include /etc/nginx/snippets/authelia-location.conf;
    location / {
        include /etc/nginx/snippets/authelia-authrequest.conf;
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Range $http_range;
    }
    location = /404.html { internal; root /usr/share/nginx/html; }
}
server {
    listen 443 ssl;
    server_name hs.example.com;
    include /etc/nginx/snippets/authelia-location.conf;
    location / { proxy_pass http://127.0.0.1:8080; }
    location /admin/ {
        include /etc/nginx/snippets/authelia-authrequest.conf;
        proxy_pass http://127.0.0.1:8081;
    }
}
`

func TestImportNginxAuthelia(t *testing.T) {
	rep, err := importNginx(sampleAuthelia, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, pl, s := findSite(rep, "code.example.com")
	want := ForwardAuth{URL: "http://127.0.0.1:9091/api/authz/auth-request", LoginURL: "https://auth.example.com/?rd={url}&rm={method}", CopyHeaders: []string{"Remote-User", "Remote-Groups"}}
	if s.ForwardAuth.URL != want.URL || s.ForwardAuth.LoginURL != want.LoginURL || strings.Join(s.ForwardAuth.CopyHeaders, ",") != "Remote-User,Remote-Groups" || s.AuthOptional {
		t.Fatalf("整站 auth_request 应转换为转发认证: %+v", s.ForwardAuth)
	}
	if len(s.RequestHeaders) != 0 || len(s.Locations) != 0 {
		t.Fatalf("透传的请求头与 internal location 不应导入: %+v %+v", s.RequestHeaders, s.Locations)
	}
	for _, w := range pl.Warnings {
		if strings.Contains(w, "Range") || strings.Contains(w, "404.html") || strings.Contains(w, "authz") && !strings.Contains(w, "转发认证") {
			t.Errorf("不应提示: %s", w)
		}
	}
	_, _, s = findSite(rep, "hs.example.com")
	if s.ForwardAuth.URL != want.URL || !s.AuthOptional || len(s.Rules) != 1 || s.Rules[0].Prefix != "/admin" || s.Rules[0].Auth != RuleAuthOn {
		t.Fatalf("部分路径的 auth_request 应转换为仅对该路径认证: %+v %+v", s.ForwardAuth, s.Rules)
	}
	if len(s.Locations) != 1 || s.Locations[0].Targets[0] != "http://127.0.0.1:8081" {
		t.Fatalf("分路径后端错误: %+v", s.Locations)
	}
}

// error_page 497 是 nginx 同端口明文 HTTP 跳转 HTTPS 的写法：新建监听时开启「跳转到 HTTPS」，合并到已有监听时只提示
func TestImportNginxPlainRedirect(t *testing.T) {
	conf := `server {
	listen 8443 ssl;
	server_name a.example.com;
	error_page 497 https://$host:$server_port$request_uri;
	return 200 "ok";
}
server {
	listen 9443 ssl;
	server_name b.example.com;
	error_page 497 =301 https://$host:$server_port$request_uri;
	return 200 "ok";
}
server {
	listen 10443 ssl;
	server_name c.example.com;
	return 200 "ok";
}`
	existing := []Listener{{Key: "old", Port: 9443, TLS: true}}
	rep, err := importNginx(conf, existing, nameCerts{})
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ := findSite(rep, "a.example.com")
	if p == nil || p.listener == nil || p.listener.PlainHTTP != PlainHTTPRedirect {
		t.Fatalf("新建监听应开启同端口 HTTP 跳转: %+v", p)
	}
	p, pl, _ := findSite(rep, "b.example.com")
	if p == nil || p.ExistingKey != "old" || !strings.Contains(strings.Join(pl.Warnings, "\n"), "error_page 497") {
		t.Fatalf("合并到已有监听时应提示: %+v %+v", p, pl)
	}
	p, _, _ = findSite(rep, "c.example.com")
	if p == nil || p.listener == nil || p.listener.PlainHTTP != "" {
		t.Fatalf("未配置 error_page 497 时不应开启: %+v", p)
	}
}

// proxy_pass 带 URI 时 nginx 把匹配的前缀替换为该路径：转换为去前缀的分路径后端，URI 路径并入后端地址。
func TestImportNginxLocationStripPrefix(t *testing.T) {
	conf := `server {
	listen 8080;
	server_name s.example.com;
	location / { proxy_pass http://10.0.0.1:80; }
	location /api/zyqd/ { proxy_pass http://192.168.105.1:9999/; }
	location /v1/ { proxy_pass http://10.0.0.2:81/inner/; }
	location /same/ { proxy_pass http://10.0.0.3:82/same/; }
	location /keep/ { proxy_pass http://10.0.0.4:83; }
	location /q/ { proxy_pass http://10.0.0.5:84/x?a=1; }
	location /site/ { proxy_pass http://10.0.0.1:80/; }
	location /cat/ { proxy_pass http://10.0.0.6:85/v1; }
	location = /f { proxy_pass http://10.0.0.7:86/x.php; }
	location = /e { proxy_pass http://10.0.0.8:87/; }
}`
	rep, err := importNginx(conf, nil, nameCerts{})
	if err != nil {
		t.Fatal(err)
	}
	_, pl, s := findSite(rep, "s.example.com")
	got := map[string]Location{}
	for _, l := range s.Locations {
		got[l.Prefix] = l
	}
	if l := got["/api/zyqd/"]; !l.StripPrefix || len(l.Targets) != 1 || l.Targets[0] != "http://192.168.105.1:9999" {
		t.Errorf("根路径 URI 应去前缀: %+v", l)
	}
	if l := got["/v1/"]; !l.StripPrefix || len(l.Targets) != 1 || l.Targets[0] != "http://10.0.0.2:81/inner/" {
		t.Errorf("带路径 URI 应去前缀并拼接: %+v", l)
	}
	if l := got["/same/"]; l.StripPrefix {
		t.Errorf("URI 与前缀相同等价于原样转发: %+v", l)
	}
	if l := got["/keep/"]; l.StripPrefix {
		t.Errorf("没有 URI 时原样转发: %+v", l)
	}
	if l := got["/q/"]; l.StripPrefix || !strings.Contains(strings.Join(pl.Warnings, "\n"), "/q/") {
		t.Errorf("带查询串的 URI 应提示: %+v %v", l, pl.Warnings)
	}
	// 语义与 nginx 不完全一致的两种写法：照常导入并提示
	warns := strings.Join(pl.Warnings, "\n")
	if l := got["/cat/"]; !l.StripPrefix || !strings.Contains(warns, "/cat/") {
		t.Errorf("URI 不以 / 结尾时应导入并提示: %+v %v", l, pl.Warnings)
	}
	if l := got["/f"]; !l.StripPrefix || !strings.Contains(warns, "location = /f 的 proxy_pass") {
		t.Errorf("精确匹配带 URI 时应导入并提示: %+v %v", l, pl.Warnings)
	}
	// 精确匹配且 URI 为 / 时两边都发往 /，不提示
	if strings.Contains(warns, "location = /e 的 proxy_pass") {
		t.Errorf("URI 为 / 的精确匹配不应提示: %v", pl.Warnings)
	}
	// 与站点后端相同、仅需去前缀的路径仍要保留规则
	if l, ok := got["/site/"]; !ok || !l.StripPrefix || len(l.Targets) != 0 {
		t.Errorf("沿用站点后端的去前缀规则丢失: %+v", l)
	}
}
