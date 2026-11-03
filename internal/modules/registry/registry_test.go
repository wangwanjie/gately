package registry

import (
	"slices"
	"testing"
)

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
