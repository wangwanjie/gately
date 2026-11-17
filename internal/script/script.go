// Package script 执行用户配置的 Shell 脚本，变量通过环境变量传入以避免注入。
package script

import (
	"context"
	"github.com/wangwanjie/gately/internal/demo"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Timeout 是单次脚本执行的超时。
var Timeout = 2 * time.Minute

// waitDelay 是取消后等待输出管道关闭的时间
const waitDelay = 3 * time.Second

// Run 以 sh -c（Windows 为临时 .bat）执行脚本。每个变量同时以原名和 GATELY_ 大写前缀两种形式注入环境变量。
func Run(content string, vars map[string]string) ([]byte, error) {
	// 演示模式下不执行任何外部命令
	if err := demo.Check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		f, err := os.CreateTemp("", "gately-*.bat")
		if err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		_, _ = f.WriteString(content)
		f.Close()
		cmd = exec.CommandContext(ctx, "cmd", "/C", f.Name())
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", content)
	}
	Prepare(cmd)
	cmd.Env = os.Environ()
	for k, v := range vars {
		cmd.Env = append(cmd.Env, k+"="+v, "GATELY_"+strings.ToUpper(k)+"="+v)
	}
	return cmd.CombinedOutput()
}

// Summarize 把输出整理为适合写日志的单段文本。
func Summarize(out []byte, limit int) string {
	text := strings.TrimSpace(string(out))
	r := []rune(text)
	if len(r) > limit {
		return string(r[:limit]) + "..."
	}
	return text
}
