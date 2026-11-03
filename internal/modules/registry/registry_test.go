package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/i18n"
)

// manifest 是仓库根目录 modules.json 中需要校验的字段。
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
}

// 默认构建包含全部模块，与 Catalog 一一对应，顺序一致。
func TestDefaultBuildHasAllModules(t *testing.T) {
	mods, certs := New()
	if certs == nil {
		t.Fatal("应返回证书模块")
	}
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.ID())
		i := slices.IndexFunc(Catalog, func(c Info) bool { return c.ID == m.ID() })
		if i < 0 || Catalog[i].Title != m.Title() {
			t.Fatalf("模块 %s 与 Catalog 中的名称不一致", m.ID())
		}
	}
	var want []string
	for _, c := range Catalog {
		want = append(want, c.ID)
	}
	if !slices.Equal(ids, want) || !slices.Equal(Built(), want) || len(Absent()) != 0 {
		t.Fatalf("默认构建应包含全部模块且顺序与 Catalog 一致: %v", ids)
	}
	req := Requires()
	if !slices.Equal(req["webservice"], []string{"ssl", "waf"}) || !slices.Equal(req["backup"], []string{"storage"}) {
		t.Fatalf("依赖关系不对: %v", req)
	}
	if _, ok := Subcommand("docker-self-upgrade"); !ok {
		t.Fatal("应提供 Docker 自身升级子命令")
	}
}

// modules.json 与代码一致：模块与 Catalog 同序，核心模块没有 build tag，可选模块都有 mod_<ID>.go 且带 !no_<ID>，
// 模块间的依赖有对应的编译期检查，语言档位的 tag 存在于 i18n 包且语言属于 Gately 支持的语言。
func TestManifestMatchesCode(t *testing.T) {
	raw, err := os.ReadFile("../../../modules.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Modules) != len(Catalog) {
		t.Fatalf("modules.json 有 %d 个模块，Catalog 有 %d 个", len(m.Modules), len(Catalog))
	}
	for i, e := range m.Modules {
		if e.ID != Catalog[i].ID {
			t.Fatalf("第 %d 个模块应为 %s，modules.json 中是 %s", i, Catalog[i].ID, e.ID)
		}
		src, err := os.ReadFile("mod_" + e.ID + ".go")
		if e.Core {
			if e.Tag != "" || err == nil {
				t.Fatalf("核心模块 %s 不应有 build tag 或 mod_%s.go", e.ID, e.ID)
			}
			continue
		}
		if e.Tag != "no_"+e.ID || err != nil || !strings.HasPrefix(string(src), "//go:build !"+e.Tag+"\n") {
			t.Fatalf("可选模块 %s 的 tag 应为 no_%s，且 mod_%s.go 以 //go:build !no_%s 开头", e.ID, e.ID, e.ID, e.ID)
		}
		for _, dep := range e.Requires {
			check, err := os.ReadFile("depcheck_" + e.ID + ".go")
			if err != nil || !strings.HasPrefix(string(check), "//go:build no_"+dep+" && !no_"+e.ID+"\n") {
				t.Fatalf("%s 依赖 %s，需要 depcheck_%s.go 在编译期拒绝只去掉 %s 的组合", e.ID, dep, e.ID, dep)
			}
		}
	}
	i18nFiles, _ := filepath.Glob("../../i18n/locales_*.go")
	var i18nSrc strings.Builder
	for _, f := range i18nFiles {
		b, _ := os.ReadFile(f)
		i18nSrc.Write(b)
	}
	for _, l := range m.Languages {
		for _, tag := range l.Tags {
			if !strings.Contains(i18nSrc.String(), tag) {
				t.Fatalf("语言档位 %s 的 tag %s 在 internal/i18n/locales_*.go 中不存在", l.ID, tag)
			}
		}
		for _, code := range l.Locales {
			if !slices.Contains(i18n.AllLangs, code) {
				t.Fatalf("语言档位 %s 包含未知语言 %s", l.ID, code)
			}
		}
		if !slices.Contains(l.Locales, i18n.SourceLang) {
			t.Fatalf("语言档位 %s 必须包含简体中文", l.ID)
		}
	}
}
