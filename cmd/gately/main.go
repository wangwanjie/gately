// Gately 是开源的网络网关工具集，适用于家庭网络、NAS 与各类服务器：端口转发、DDNS、STUN 穿透、反向代理与证书管理。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	// 内嵌 IANA 时区数据（约 450 KB）：OpenWrt 等精简系统没有 /usr/share/zoneinfo，否则设置里的时区一律无法加载
	_ "time/tzdata"

	"github.com/wangwanjie/gately/internal/app"
	"github.com/wangwanjie/gately/internal/ctl"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/modules/registry"
	"github.com/wangwanjie/gately/internal/systz"
	"github.com/wangwanjie/gately/internal/version"
	"github.com/wangwanjie/gately/web"
)

func main() {
	// 子命令：ctl 调用运行中的实例，mcp 提供 MCP 服务；其余参数走服务启动流程
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "ctl":
			// 命令行自身的提示按系统语言翻译
			os.Exit(ctl.RunCtl(os.Args[2:], i18n.NewWriterFor(os.Stdout, i18n.FromEnv()), i18n.NewWriterFor(os.Stderr, i18n.FromEnv())))
		case "mcp":
			os.Exit(ctl.RunMCP(os.Args[2:], os.Stdin, os.Stdout, i18n.NewWriterFor(os.Stderr, i18n.FromEnv())))
		case "reset-password", "reset-totp":
			// 在服务器上重置管理员密码或关闭两步验证，提示按系统语言翻译
			os.Exit(runReset(os.Args[1], os.Args[2:], os.Stdin, i18n.NewWriterFor(os.Stdout, i18n.FromEnv())))
		case "version":
			// 与 -version 等价，避免 docker exec gately gately version 误启动第二个服务实例
			printVersion()
			return
		}
		// 模块提供的子命令，例如 Docker 管理升级 Gately 自身时在辅助容器中重建 Gately 容器
		if run, ok := registry.Subcommand(os.Args[1]); ok {
			os.Exit(run(os.Args[2:], os.Stdout))
		}
	}
	// 标准日志（含启动失败等 log.Fatalf）按服务端语言翻译
	log.SetOutput(i18n.NewWriter(os.Stderr))
	configDir := flag.String("c", defaultConfigDir(), "configuration directory / 配置目录")
	listen := flag.String("listen", "", "admin UI listen address, overrides the config file, e.g. :16680 / 管理界面监听地址，覆盖配置文件")
	showVersion := flag.Bool("version", false, "print version info and exit / 输出版本信息")
	apiDocs := flag.String("api-docs", "", "export API docs (openapi.json and API.md) to the given directory and exit / 导出 API 文档到指定目录后退出")
	applyDir := flag.String("apply", os.Getenv("GATELY_APPLY_DIR"), "apply <name>.json files from this directory to the config directory on every start (also GATELY_APPLY_DIR) / 每次启动把该目录下的配置文件覆盖到配置目录")
	gatewaySocket := flag.String("gateway-socket", "", "also serve the admin UI on this Unix socket, e.g. for the fnOS unified gateway / 额外在该 Unix Socket 上提供管理界面（如飞牛 fnOS 统一网关）")
	gatewayPrefix := flag.String("gateway-prefix", "", "path prefix used behind the gateway socket, e.g. /app/gately / 经网关访问时的路径前缀")
	flag.Parse()

	// 多余的位置参数不属于任何子命令，拒绝启动，避免误启动第二个服务实例去抢占端口
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n", flag.Arg(0))
		os.Exit(2)
	}

	if *apiDocs != "" {
		if err := exportAPIDocs(*apiDocs); err != nil {
			log.Fatalf("导出 API 文档失败: %v", err)
		}
		return
	}

	if *showVersion {
		printVersion()
		return
	}

	// OpenWrt 没有 /etc/localtime，procd 也不传 TZ，按系统配置的时区计时；设置中指定的时区随后会覆盖它
	systz.Init()

	// Web 服务使用证书模块按 SNI 提供 HTTPS 证书，并按站点引用 WAF 实例
	mods, certs := registry.New()
	a, err := app.New(app.Options{
		ConfigDir: *configDir,
		Listen:    *listen,
		Web:       web.FS(),
		Certs:     certs,
		Modules:   mods,
		Requires:  registry.Requires(),
		Absent:    registry.Absent(),
		ApplyDir:  *applyDir,
		// 自动化部署时用环境变量指定初始密码，不指定时使用默认密码 gately.cn
		InitialPassword: os.Getenv("GATELY_INITIAL_PASSWORD"),
		GatewaySocket:   *gatewaySocket,
		GatewayPrefix:   *gatewayPrefix,
		// 崩溃信息保存到配置目录的 crash.log，容器重启后看不到 stderr 时也能随诊断包导出
		CrashLog: true,
	})
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := a.Run(ctx); err != nil {
		log.Fatalf("运行失败: %v", err)
	}
}

// defaultConfigDir 优先使用环境变量 GATELY_CONFIG_DIR，否则为当前目录下的 data。
func defaultConfigDir() string {
	if d := os.Getenv("GATELY_CONFIG_DIR"); d != "" {
		return d
	}
	return "data"
}

// exportAPIDocs 在临时配置目录中构建应用（不监听端口），把 API 文档写到 dir。
func exportAPIDocs(dir string) error {
	tmp, err := os.MkdirTemp("", "gately-apidocs-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	mods, certs := registry.New()
	a, err := app.New(app.Options{ConfigDir: tmp, Certs: certs, Modules: mods})
	if err != nil {
		return err
	}
	// 简体中文输出在 dir 根目录，其余语言输出在 dir/<语言>/ 下，文档由同一份接口目录翻译生成，无需手工同步
	for _, lang := range i18n.Langs {
		out := dir
		if lang != i18n.SourceLang {
			out = filepath.Join(dir, lang)
		}
		spec, md, err := a.APIDocs(lang)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "openapi.json"), spec, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "API.md"), md, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// printVersion 以 JSON 输出版本信息。
func printVersion() {
	info, _ := json.Marshal(map[string]string{
		"version":   version.Version,
		"commit":    version.Commit,
		"buildDate": version.BuildDate,
		"go":        runtime.Version(),
	})
	fmt.Println(string(info))
}
