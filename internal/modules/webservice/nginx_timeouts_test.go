package webservice

import (
	"strings"
	"testing"
)

// nginx 时间值的解析与向上取整。
func TestNginxSeconds(t *testing.T) {
	for in, want := range map[string]int{"60": 60, "30s": 30, "5m": 300, "1h": 3600, "1m30s": 90, "500ms": 1, "1500ms": 2, "1d": 86400, "2w": 1209600} {
		if got, ok := nginxSeconds(in); !ok || got != want {
			t.Errorf("%q = %d %v，应为 %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "0", "0s", "s", "10x", "1.5s", "-1", "5 s", "9999999999999s"} {
		if _, ok := nginxSeconds(bad); ok {
			t.Errorf("%q 应无法识别", bad)
		}
	}
}

// 导入 nginx：proxy_connect_timeout 与 proxy_read_timeout 转换为站点后端超时，location / 优先于 server，
// 超过上限时按上限导入并提示，其他 location 的不同设置提示未导入，未设置时保持默认。
func TestImportNginxTimeouts(t *testing.T) {
	conf := `server {
	listen 8080;
	server_name a.example.com;
	proxy_connect_timeout 5s;
	proxy_read_timeout 30d;
	location / {
		proxy_pass http://127.0.0.1:3000;
		proxy_read_timeout 1h;
	}
	location /ws/ {
		proxy_pass http://127.0.0.1:3001;
		proxy_read_timeout 1d;
	}
}
server {
	listen 8080;
	server_name b.example.com;
	proxy_connect_timeout 10m;
	proxy_read_timeout 2y;
	location / { proxy_pass http://127.0.0.1:3000; }
}
server {
	listen 8080;
	server_name c.example.com;
	proxy_connect_timeout abc;
	location / { proxy_pass http://127.0.0.1:3000; }
}
server {
	listen 8080;
	server_name d.example.com;
	location / { proxy_pass http://127.0.0.1:3000; }
}`
	rep, err := importNginx(conf, nil, nameCerts{})
	if err != nil {
		t.Fatal(err)
	}
	_, pl, s := findSite(rep, "a.example.com")
	if s == nil || s.Timeouts != (BackendTimeouts{Connect: 5, ResponseHeader: 3600}) {
		t.Fatalf("a: %+v", s)
	}
	if !strings.Contains(strings.Join(pl.Warnings, "\n"), "location /ws/ 的 proxy_read_timeout 未导入") {
		t.Fatalf("a 应提示 /ws/ 的超时未导入: %v", pl.Warnings)
	}
	_, pl, s = findSite(rep, "b.example.com")
	if s == nil || s.Timeouts != (BackendTimeouts{Connect: 300, ResponseHeader: 86400}) || !strings.Contains(strings.Join(pl.Warnings, "\n"), "超过 Gately 的上限") {
		t.Fatalf("b 应按上限导入并提示: %+v %v", s, pl.Warnings)
	}
	_, pl, s = findSite(rep, "c.example.com")
	if s == nil || s.Timeouts != (BackendTimeouts{}) || !strings.Contains(strings.Join(pl.Warnings, "\n"), "无法识别") {
		t.Fatalf("c 应保持默认并提示: %+v %v", s, pl.Warnings)
	}
	_, _, s = findSite(rep, "d.example.com")
	if s == nil || s.Timeouts != (BackendTimeouts{}) {
		t.Fatalf("d 未设置时应为默认: %+v", s)
	}
	// 导入结果能通过保存校验
	for _, name := range []string{"a.example.com", "b.example.com"} {
		_, _, s := findSite(rep, name)
		cp := *s
		if err := cp.validate(false); err != nil {
			t.Fatalf("%s 导入结果应可保存: %v", name, err)
		}
	}
}
