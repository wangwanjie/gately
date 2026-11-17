package webservice

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// parseNginxTimed 在限定时间内解析，超时即判失败（修复前这些输入会展开出天文数字的指令）。
func parseNginxTimed(t *testing.T, text string) ([]*ngDirective, []string, error) {
	t.Helper()
	type result struct {
		out   []*ngDirective
		warns []string
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		out, warns, err := parseNginx(text)
		ch <- result{out, warns, err}
	}()
	select {
	case r := <-ch:
		return r.out, r.warns, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("解析没有在 10 秒内结束：include 展开失控")
		return nil, nil, nil
	}
}

func countDirectives(list []*ngDirective) int {
	n := 0
	for _, d := range list {
		n++
		n += countDirectives(d.Block)
	}
	return n
}

// 文件包含自己：每层 10 次、最多 16 层，不设防时要展开 10^16 条指令。
func TestNginxIncludeSelfCycle(t *testing.T) {
	text := "# configuration file /etc/nginx/nginx.conf:\n" + strings.Repeat("include nginx.conf;\n", 10) + "worker_processes 1;\n"
	out, warns, err := parseNginxTimed(t, text)
	if err != nil {
		t.Fatalf("包含自己的配置应忽略循环后照常解析: %v", err)
	}
	if len(out) != 1 || out[0].Name != "worker_processes" {
		t.Fatalf("应只剩下普通指令: %+v", out)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "循环") {
		t.Fatalf("应提示一次循环包含: %v", warns)
	}
}

// 两个文件相互包含，通配符也匹配到自己。
func TestNginxIncludeMutualCycle(t *testing.T) {
	text := `# configuration file /etc/nginx/nginx.conf:
http {
    include conf.d/*.conf;
    include conf.d/*.conf;
}
# configuration file /etc/nginx/conf.d/a.conf:
include /etc/nginx/conf.d/*.conf;
server { listen 80; server_name a.test; }
# configuration file /etc/nginx/conf.d/b.conf:
include /etc/nginx/conf.d/a.conf;
include /etc/nginx/nginx.conf;
server { listen 80; server_name b.test; }
`
	out, warns, err := parseNginxTimed(t, text)
	if err != nil {
		t.Fatal(err)
	}
	if n := countDirectives(out); n > 200 {
		t.Fatalf("相互包含不应让指令成倍增加，实际 %d 条", n)
	}
	servers := 0
	var walk func([]*ngDirective)
	walk = func(list []*ngDirective) {
		for _, d := range list {
			if d.Name == "server_name" {
				servers++
			}
			walk(d.Block)
		}
	}
	walk(out)
	if servers == 0 {
		t.Fatal("循环之外的站点应照常解析出来")
	}
	if len(warns) == 0 {
		t.Fatalf("应提示循环包含: %v", warns)
	}
}

// 没有循环但层层重复包含：16 层、每层 10 次，同样是指数级。超过上限时报错而不是耗尽内存。
func TestNginxIncludeFanOutLimit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&b, "# configuration file /etc/nginx/f%d.conf:\n", i)
		if i == 15 {
			b.WriteString("worker_processes 1;\n")
			continue
		}
		b.WriteString(strings.Repeat(fmt.Sprintf("include f%d.conf;\n", i+1), 10))
	}
	if b.Len() > 4096 {
		t.Fatalf("测试输入应保持很小，实际 %d 字节", b.Len())
	}
	out, _, err := parseNginxTimed(t, b.String())
	if err == nil || !strings.Contains(err.Error(), "include") {
		t.Fatalf("展开结果超过上限时应报错，实际 %d 条 %v", countDirectives(out), err)
	}
}

// 层层重复包含、最内层文件没有任何指令（只有注释）：展开结果始终为空，但展开次数仍是指数级，
// 每次展开 include 本身也要计入上限，否则几百字节的输入就能让解析一直占满 CPU。
func TestNginxIncludeFanOutEmptyLeaf(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&b, "# configuration file /etc/nginx/f%d.conf:\n", i)
		if i == 15 {
			b.WriteString("# empty\n")
			continue
		}
		b.WriteString(strings.Repeat(fmt.Sprintf("include f%d.conf;\n", i+1), 10))
	}
	if b.Len() > 4096 {
		t.Fatalf("测试输入应保持很小，实际 %d 字节", b.Len())
	}
	_, _, err := parseNginxTimed(t, b.String())
	if err == nil || !strings.Contains(err.Error(), "include") {
		t.Fatalf("展开次数超过上限时应报错，实际 %v", err)
	}
}

// 同一个片段被多处包含是正常用法，照常全部展开；上限以内的重复包含不受影响。
func TestNginxIncludeRepeatedSnippet(t *testing.T) {
	text := `# configuration file /etc/nginx/nginx.conf:
http {
    server {
        listen 80;
        location /a { include snippets/proxy.conf; }
        location /b { include snippets/proxy.conf; }
        location /c { include snippets/proxy.conf; }
    }
}
# configuration file /etc/nginx/snippets/proxy.conf:
proxy_set_header Host $host;
proxy_pass http://127.0.0.1:8080;
`
	out, warns, err := parseNginxTimed(t, text)
	if err != nil || len(warns) != 0 {
		t.Fatalf("重复包含同一片段不应报错或告警: %v %v", err, warns)
	}
	passes := 0
	var walk func([]*ngDirective)
	walk = func(list []*ngDirective) {
		for _, d := range list {
			if d.Name == "proxy_pass" {
				passes++
			}
			walk(d.Block)
		}
	}
	walk(out)
	if passes != 3 {
		t.Fatalf("片段应在三个 location 里各展开一次，实际 %d 次", passes)
	}
	// 被包含过的文件不再作为顶层配置重复出现
	for _, d := range out {
		if d.Name == "proxy_pass" || d.Name == "proxy_set_header" {
			t.Fatalf("片段不应出现在顶层: %+v", d)
		}
	}
}
