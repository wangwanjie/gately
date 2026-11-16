package fsperm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 共享目录包含配置目录时，Guard 只屏蔽配置目录这棵子树，其余路径照常可用。
func TestGuard(t *testing.T) {
	base := t.TempDir()
	share := mkdir(t, base, "vol1")
	conf := mkdir(t, share, "1000", "docker", "gately")
	mkdir(t, conf, "history")
	other := mkdir(t, share, "1000", "media")
	similar := mkdir(t, share, "1000", "docker", "gately-old")
	Protect(conf)

	if NewGuard(other) != nil {
		t.Fatal("不包含配置目录的共享目录不需要屏蔽")
	}
	var nilGuard *Guard
	if nilGuard.Hidden(conf) || nilGuard.Touches(conf) || nilGuard.HiddenChild(share, nil) {
		t.Fatal("nil Guard 不屏蔽任何路径")
	}
	g := NewGuard(share)
	if g == nil {
		t.Fatal("包含配置目录的共享目录应有屏蔽规则")
	}
	hidden := map[string]string{
		"配置目录":      conf,
		"配置目录内的文件":  filepath.Join(conf, "account.json"),
		"配置目录内的子目录": filepath.Join(conf, "history", "x"),
		"含 .. 的写法":  filepath.Join(other, "..", "docker", "gately", "account.json"),
		"尚不存在的子目录":  filepath.Join(conf, "new", "file"),
		"末尾带分隔符的写法": conf + string(filepath.Separator),
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(share, "link")
		if err := os.Symlink(conf, link); err != nil {
			t.Fatal(err)
		}
		hidden["指向配置目录的链接"] = link
		hidden["经链接访问配置目录内的文件"] = filepath.Join(link, "account.json")
		up := filepath.Join(share, "up")
		if err := os.Symlink(filepath.Dir(conf), up); err != nil {
			t.Fatal(err)
		}
		hidden["经指向上级目录的链接进入"] = filepath.Join(up, "gately", "account.json")
	}
	if _, err := os.Stat(strings.ToUpper(conf)); err == nil {
		hidden["大小写不同的写法"] = filepath.Join(strings.ToUpper(conf), "account.json")
	}
	for name, p := range hidden {
		if !g.Hidden(p) || !g.Touches(p) {
			t.Errorf("%s（%s）应被屏蔽", name, p)
		}
	}
	for name, p := range map[string]string{
		"共享目录本身": share, "同级目录": other, "名称只是前缀相同": similar, "上级目录": filepath.Dir(conf),
		"普通文件": filepath.Join(other, "a.mp4"),
	} {
		if g.Hidden(p) {
			t.Errorf("%s（%s）不应被屏蔽", name, p)
		}
	}
	// 改名、删除、修改权限：包含配置目录的上级目录同样不能动
	for _, p := range []string{filepath.Dir(conf), filepath.Join(share, "1000"), share} {
		if !g.Touches(p) {
			t.Errorf("移动或删除 %s 会带走配置目录，应拒绝", p)
		}
	}
	if g.Touches(other) || g.Touches(similar) {
		t.Error("与配置目录无关的目录可以改名删除")
	}

	// 列目录：配置目录与指向它的链接从列表中去掉，其余条目保留
	dir := filepath.Dir(conf)
	if runtime.GOOS != "windows" {
		if err := os.Symlink(conf, filepath.Join(dir, "alias")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "gately.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range g.FilterEntries(dir, entries) {
		names = append(names, e.Name())
	}
	got := strings.Join(names, ",")
	if strings.Contains(got, "gately,") || strings.HasSuffix(got, ",gately") || strings.Contains(got, "alias") || !strings.Contains(got, "gately-old") || !strings.Contains(got, "gately.txt") {
		t.Fatalf("列表应只去掉配置目录及指向它的链接: %s", got)
	}
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(g.FilterInfos(dir, infos)); n != len(names) {
		t.Fatalf("FilterInfos 与 FilterEntries 结果应一致: %d / %d", n, len(names))
	}
}

// 共享目录本身就是配置目录或位于其中（升级前保存的配置）：所有路径都被屏蔽。
func TestGuardRootInsideConfig(t *testing.T) {
	conf := mkdir(t, t.TempDir(), "conf")
	Protect(conf)
	g := NewGuard(filepath.Join(conf))
	if g == nil || !g.Hidden(filepath.Join(conf, "account.json")) || !g.Hidden(conf) {
		t.Fatal("共享目录就是配置目录时应全部屏蔽")
	}
}
