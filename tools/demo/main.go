// 命令 demo 生成在线演示实例的配置目录，用法：go run ./tools/demo -out /tmp/gately-demo
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wangwanjie/gately/tools/demo/gen"
)

func main() {
	out := flag.String("out", "", "配置目录（必填）")
	portBase := flag.Int("port-base", 0, "非 0 时数据面监听端口改为从该值起的高位端口（仅测试用）")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "用法: demo -out <配置目录> [-port-base N]")
		os.Exit(2)
	}
	if err := gen.Generate(gen.Options{Dir: *out, PortBase: *portBase}); err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
}
