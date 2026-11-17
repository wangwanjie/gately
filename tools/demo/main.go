// 命令 demo 生成在线演示实例的配置目录，用法：go run ./tools/demo -out /tmp/gately-demo
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wangwanjie/gately/tools/demo/gen"
)

func main() {
	out := flag.String("out", "", "配置目录（必填）")
	portBase := flag.Int("port-base", 0, "非 0 时数据面监听端口改为从该值起的高位端口（仅测试用）")
	trusted := flag.String("trusted-proxies", "", "可信代理，逗号分隔的 IP 或 CIDR，写入全局设置（前置反向代理不在本机时设置）")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "用法: demo -out <配置目录> [-port-base N] [-trusted-proxies CIDR,...]")
		os.Exit(2)
	}
	if err := gen.Generate(gen.Options{Dir: *out, PortBase: *portBase, TrustedProxies: splitList(*trusted)}); err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
}

// splitList 把逗号分隔的列表拆开，去掉空白与空项。
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
