// Package registry 集中创建全部功能模块，供主程序、文档导出与测试共用。
//
// 可选模块各自放在带 build tag 的文件中（例如 mod_waf.go 带 //go:build !no_waf），
// 默认构建包含全部模块；定制构建通过 -tags no_waf,no_smb 等去掉不需要的模块。
// 端口转发、Web 服务与 SSL 证书是核心模块，始终编译。
package registry

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/ssl"
	"github.com/wangwanjie/gately/internal/modules/webservice"
)

// errNotBuilt 为依赖的模块没有编译进本程序时注入函数返回的错误。
func errNotBuilt(title string) error {
	return fmt.Errorf("功能模块「%s」未包含在此构建中", title)
}

// wiring 是模块之间经函数注入的协作能力，避免模块之间直接依赖。
// 提供能力的模块没有编译进来时，对应字段为 nil。
type wiring struct {
	certs *ssl.Module
	web   *webservice.Module
	// waf 为 WAF 模块；未编译时保持接口零值 nil，Web 服务据此忽略站点的 WAF 设置
	waf webservice.WAFProvider
	// files 为存储挂载模块，备份模块用它把备份写到挂载的存储上
	files module.Module
	// 以下由 Web 终端提供：经 SSH 连接上传文件并执行命令、列出 SSH 连接
	sshPush    func(ctx context.Context, conn, dir string, files []ssl.RemoteFile, command string) (string, error)
	sshTargets func() []ssl.SSHTarget
	// 以下由 Docker 模块提供：重启容器、列出容器名称、持续读取容器日志
	restart      func(ctx context.Context, names []string) error
	containers   func(ctx context.Context) ([]string, error)
	followDocker func(ctx context.Context, container string, onLine func(string)) error
}

// part 是一个模块的装配方式。
type part struct {
	id string
	// order 为模块在列表中的位置，决定总览页、设置页等处的显示顺序
	order int
	// requires 为该模块运行时依赖的模块 ID；依赖方开启期间被依赖的模块不能关闭。
	// 被依赖的模块没有编译进来时，这条依赖自动忽略
	requires []string
	// prepare 在创建全部模块之前运行，向 wiring 提供本模块的能力，可为 nil
	prepare func(w *wiring)
	// create 创建模块，此时所有 prepare 都已运行
	create func(w *wiring) module.Module
}

var parts []part

// register 由各模块文件的 init 调用。
func register(p part) {
	parts = append(parts, p)
}

// sortedParts 返回按显示顺序排列的模块装配方式。
func sortedParts() []part {
	out := slices.Clone(parts)
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

// New 创建本次编译包含的全部模块。Web 服务使用证书模块按 SNI 提供 HTTPS 证书，并按站点引用 WAF 实例，
// 因此一并返回证书模块供管理界面 HTTPS 复用。
func New() (mods []module.Module, certs *ssl.Module) {
	w := &wiring{certs: ssl.New()}
	list := sortedParts()
	for _, p := range list {
		if p.prepare != nil {
			p.prepare(w)
		}
	}
	// 证书部署复用 Web 终端的 SSH 连接与 Docker 模块的客户端；对应模块未编译时返回明确的错误
	d := ssl.Deployers{Push: w.sshPush, Restart: w.restart, SSHTargets: w.sshTargets, Containers: w.containers}
	if d.Push == nil {
		d.Push = func(context.Context, string, string, []ssl.RemoteFile, string) (string, error) {
			return "", errNotBuilt("Web 终端")
		}
	}
	if d.SSHTargets == nil {
		d.SSHTargets = func() []ssl.SSHTarget { return []ssl.SSHTarget{} }
	}
	if d.Restart == nil {
		d.Restart = func(context.Context, []string) error { return errNotBuilt("Docker 管理") }
	}
	if d.Containers == nil {
		d.Containers = func(context.Context) ([]string, error) { return nil, errNotBuilt("Docker 管理") }
	}
	w.certs.SetDeployers(d)
	w.web = webservice.New(w.certs, w.waf)
	for _, p := range list {
		mods = append(mods, p.create(w))
	}
	return mods, w.certs
}

// Requires 返回模块间的依赖关系：键为模块 ID，值为它依赖的模块 ID。
// 与 New 中的装配关系一一对应；依赖方开启期间，被依赖的模块不能关闭。只包含本次编译的模块。
func Requires() map[string][]string {
	built := map[string]bool{}
	for _, p := range parts {
		built[p.id] = true
	}
	out := map[string][]string{}
	for _, p := range sortedParts() {
		var req []string
		for _, id := range p.requires {
			if built[id] {
				req = append(req, id)
			}
		}
		if len(req) > 0 {
			out[p.id] = req
		}
	}
	return out
}

// Built 返回本次编译包含的模块 ID，按显示顺序排列。
func Built() []string {
	var out []string
	for _, p := range sortedParts() {
		out = append(out, p.id)
	}
	return out
}

// Absent 返回已知但没有编译进本程序的模块。它们的配置文件原样保留，换回包含它们的构建后继续生效。
func Absent() []Info {
	built := map[string]bool{}
	for _, p := range parts {
		built[p.id] = true
	}
	var out []Info
	for _, c := range Catalog {
		if !built[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// subcommands 为模块提供的命令行子命令，例如 Docker 模块升级 Gately 自身容器时使用的辅助命令。
var subcommands = map[string]func(args []string, out io.Writer) int{}

// Subcommand 按名称查找模块提供的子命令。
func Subcommand(name string) (func(args []string, out io.Writer) int, bool) {
	run, ok := subcommands[name]
	return run, ok
}
