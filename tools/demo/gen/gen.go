// Package gen 生成在线演示实例使用的配置目录：覆盖各模块的逼真数据。
//
// 所有地址与域名都取自文档保留范围（example.com 系、192.0.2.0/24、198.51.100.0/24、203.0.113.0/24、2001:db8::/32），
// 不包含任何真实 IP、域名与密钥；证书为现场生成的自签证书。
package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Options 是生成参数。
type Options struct {
	// Dir 是输出的配置目录，必须已存在或可创建
	Dir string
	// Now 是“当前时间”，证书有效期、日志与访客时间都以它为基准；零值取 time.Now
	Now time.Time
	// PortBase 非 0 时，所有数据面监听端口改为从该值起顺序分配的高位端口（测试避免占用 443、80 等特权端口）
	PortBase int
	// AdminListen 写入全局设置的管理端口，空为 :16680
	AdminListen string
	// TrustedProxies 写入全局设置的可信代理（IP 或 CIDR）。演示实例在容器里由前置反向代理访问，
	// 代理的地址是 Docker 网络里的地址，不属于本机；信任它后才会采信 X-Forwarded-Proto（会话 Cookie 加 Secure）与访客真实 IP
	TrustedProxies []string
}

type generator struct {
	opt   Options
	now   time.Time
	ports map[int]int
	next  int
}

// Generate 把演示数据写入 opt.Dir。已有的同名配置文件会被覆盖。
func Generate(opt Options) error {
	g := &generator{opt: opt, now: opt.Now, ports: map[int]int{}, next: opt.PortBase}
	if g.now.IsZero() {
		g.now = time.Now()
	}
	g.now = g.now.Truncate(time.Second)
	if err := os.MkdirAll(opt.Dir, 0o700); err != nil {
		return err
	}
	steps := []func() error{
		g.settings, g.ssl, g.waf, g.webservice, g.portforward, g.protoroute, g.ddns, g.stun,
		g.frp, g.tailscale, g.netbird, g.guard, g.knock, g.notify, g.cron, g.perf, g.wol, g.misc,
		g.visitors, g.logs,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// port 返回数据面规则使用的监听端口：默认原样，测试时映射到高位端口且同一个端口号映射稳定。
func (g *generator) port(n int) int {
	if g.opt.PortBase == 0 {
		return n
	}
	if p, ok := g.ports[n]; ok {
		return p
	}
	p := g.next
	g.next++
	g.ports[n] = p
	return p
}

func (g *generator) ago(d time.Duration) string {
	return g.now.Add(-d).UTC().Format(time.RFC3339)
}

func (g *generator) write(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.opt.Dir, name+".json"), b, 0o600)
}

// obj 是 JSON 对象的简写。
type obj = map[string]any

func list(v ...any) []any { return v }

func strs(v ...string) []string { return v }

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("demo gen: %v", err))
	}
}

// ---- 全局设置与账号 ----

func (g *generator) settings() error {
	listen := g.opt.AdminListen
	if listen == "" {
		listen = ":16680"
	}
	set := obj{
		"listen":             listen,
		"language":           "zh-CN",
		"timeZone":           "Asia/Shanghai",
		"disableUpdateCheck": true,
		"logLevel":           "info",
	}
	if len(g.opt.TrustedProxies) > 0 {
		set["trustedProxies"] = g.opt.TrustedProxies
	}
	return g.write("gately", set)
}
