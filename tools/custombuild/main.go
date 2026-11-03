// custombuild 按仓库根目录的 modules.json 做定制构建：校验模块、语言档位与构建目标，补齐依赖的模块，
// 计算 go build 的 build tags、前端构建的 GATELY_LOCALES 与组合键（profile），并可直接构建安装包。
// 本地定制构建与 build.gately.cn 的构建工作流共用它，保证两边结果一致。
//
// 用法（在仓库根目录执行）：
//
//	go run ./tools/custombuild resolve -modules ddns,frp,guard -lang cjk-en -targets linux-arm64
//	go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64,fnos-arm -out dist
//
// -modules 为要包含的可选模块（逗号分隔，all 表示全部，留空表示只有核心模块），核心模块总是包含。
// resolve 输出一行 JSON，字段见 resolved；build 的产物见 build.go。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
)

// manifest 是 modules.json 中构建需要的字段。
type manifest struct {
	Modules []struct {
		ID       string   `json:"id"`
		Core     bool     `json:"core"`
		Tag      string   `json:"tag"`
		Requires []string `json:"requires"`
	} `json:"modules"`
	Languages []struct {
		ID      string   `json:"id"`
		Tags    []string `json:"tags"`
		Locales []string `json:"locales"`
	} `json:"languages"`
	Targets []target `json:"targets"`
}

// target 是一个构建目标：kind 为 tar（二进制压缩包）、fpk（飞牛安装包）、openwrt（ipk 与 apk）
// 或 docker（多架构镜像，Platforms 为 buildx 的 --platform）。
type target struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Platforms string `json:"platforms,omitempty"`
}

// resolved 是解析结果。
type resolved struct {
	// Modules 为包含的可选模块（已补齐依赖），按清单顺序
	Modules []string `json:"modules"`
	// Added 为因依赖自动补上的模块
	Added []string `json:"added"`
	Lang  string   `json:"lang"`
	// Tags 为 go build -tags 的值，逗号分隔；全功能、全部语言时为空串
	Tags string `json:"tags"`
	// Locales 为前端构建的 GATELY_LOCALES，逗号分隔
	Locales string `json:"locales"`
	// Profile 是这组选项的组合键：sha256("lang=<档位>;modules=<排序后的模块>") 的前 12 位十六进制，
	// 构建器用它命名产物、判断是否已经构建过
	Profile string `json:"profile"`
	// Targets 为要构建的目标，按传入顺序
	Targets []target `json:"targets"`
	// DockerArgs 为用仓库根目录 Dockerfile 构建定制镜像时的 --build-arg：
	// 没有 Docker 管理模块时不装 docker-cli，没有入侵防护时不装 nftables
	DockerArgs []string `json:"dockerArgs"`
}

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "resolve" && os.Args[1] != "build") {
		fmt.Fprintln(os.Stderr, "用法: custombuild resolve|build [选项]，选项见 -h")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	path := fs.String("manifest", "modules.json", "modules.json 路径")
	mods := fs.String("modules", "", "要包含的可选模块，逗号分隔；all 表示全部，留空表示只有核心模块")
	lang := fs.String("lang", "all", "语言档位：zh / cjk-en / all")
	targets := fs.String("targets", "", "构建目标 ID，逗号分隔，见 modules.json 的 targets")
	version := fs.String("version", "", "build：版本号，例如 v0.6.4")
	out := fs.String("out", "dist", "build：产物目录")
	skipWeb := fs.Bool("skip-web", false, "build：跳过前端构建（web/dist 已按同一语言档位构建过）")
	_ = fs.Parse(os.Args[2:])
	raw, err := os.ReadFile(*path)
	if err != nil {
		fail(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		fail(fmt.Errorf("解析 %s 失败: %w", *path, err))
	}
	r, err := resolve(m, *mods, *lang, *targets)
	if err != nil {
		fail(err)
	}
	if cmd == "resolve" {
		line, _ := json.Marshal(r)
		fmt.Println(string(line))
		return
	}
	if *version == "" {
		fail(errors.New("build 需要 -version"))
	}
	if err := build(r, *version, *out, *skipWeb); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "custombuild:", err)
	os.Exit(1)
}

// resolve 校验选项并补齐依赖。传入核心模块时忽略；未知的模块、语言档位或构建目标返回错误。
func resolve(m manifest, mods, lang, targets string) (resolved, error) {
	want := map[string]bool{}
	optional := map[string]bool{}
	core := map[string]bool{}
	for _, e := range m.Modules {
		if e.Core {
			core[e.ID] = true
		} else {
			optional[e.ID] = true
		}
	}
	for _, id := range strings.Split(mods, ",") {
		id = strings.TrimSpace(id)
		switch {
		case id == "" || core[id]:
		case id == "all":
			for o := range optional {
				want[o] = true
			}
		case optional[id]:
			want[id] = true
		default:
			return resolved{}, fmt.Errorf("未知的功能模块: %q", id)
		}
	}
	// 依赖可能是多级的，循环到不再有变化为止
	var added []string
	for changed := true; changed; {
		changed = false
		for _, e := range m.Modules {
			if !want[e.ID] {
				continue
			}
			for _, dep := range e.Requires {
				if optional[dep] && !want[dep] {
					want[dep] = true
					added = append(added, dep)
					changed = true
				}
			}
		}
	}
	r := resolved{Modules: []string{}, Added: added, Lang: lang, Targets: []target{}}
	if r.Added == nil {
		r.Added = []string{}
	}
	var tags []string
	for _, e := range m.Modules {
		if e.Core {
			continue
		}
		if want[e.ID] {
			r.Modules = append(r.Modules, e.ID)
		} else {
			tags = append(tags, e.Tag)
		}
	}
	found := false
	for _, l := range m.Languages {
		if l.ID == lang {
			tags = append(tags, l.Tags...)
			r.Locales = strings.Join(l.Locales, ",")
			found = true
		}
	}
	if !found {
		return resolved{}, errors.New("未知的语言档位: " + lang)
	}
	r.Tags = strings.Join(tags, ",")
	flag := func(id string) string {
		if want[id] {
			return "1"
		}
		return "0"
	}
	r.DockerArgs = []string{"GATELY_TAGS=" + r.Tags, "GATELY_LOCALES=" + r.Locales, "WITH_DOCKER_CLI=" + flag("docker"), "WITH_NFTABLES=" + flag("guard")}
	for _, id := range strings.Split(targets, ",") {
		id = strings.TrimSpace(id)
		if id == "" || slices.ContainsFunc(r.Targets, func(t target) bool { return t.ID == id }) {
			continue
		}
		i := slices.IndexFunc(m.Targets, func(t target) bool { return t.ID == id })
		if i < 0 {
			return resolved{}, fmt.Errorf("未知的构建目标: %q", id)
		}
		r.Targets = append(r.Targets, m.Targets[i])
	}
	sorted := slices.Clone(r.Modules)
	slices.Sort(sorted)
	sum := sha256.Sum256([]byte("lang=" + lang + ";modules=" + strings.Join(sorted, ",")))
	r.Profile = hex.EncodeToString(sum[:])[:12]
	return r, nil
}
