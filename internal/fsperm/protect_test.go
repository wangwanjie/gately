package fsperm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOverlap(t *testing.T) {
	base := t.TempDir()
	conf := mkdir(t, base, "srv", "gately", "data")
	inside := mkdir(t, conf, "backups")
	sibling := mkdir(t, base, "srv", "gately", "www")
	similar := mkdir(t, base, "srv", "gately", "data-old")
	other := mkdir(t, base, "share")
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"配置目录本身", conf, true},
		{"末尾带分隔符", conf + string(filepath.Separator), true},
		{"含 .. 的写法", filepath.Join(sibling, "..", "data"), true},
		{"配置目录内", inside, true},
		{"配置目录内尚不存在的子目录", filepath.Join(conf, "not", "yet"), true},
		{"上级目录", filepath.Dir(conf), true},
		{"更上级目录", base, true},
		{"根目录", filepath.VolumeName(conf) + string(filepath.Separator), true},
		{"同级目录", sibling, false},
		{"名称只是前缀相同", similar, false},
		{"无关目录", other, false},
		{"无关且不存在的目录", filepath.Join(other, "missing"), false},
	}
	for _, c := range cases {
		if got := Overlap(conf, c.path); got != c.want {
			t.Errorf("%s: Overlap(%q) = %v，期望 %v", c.name, c.path, got, c.want)
		}
	}
}

func TestOverlapRelative(t *testing.T) {
	base := t.TempDir()
	conf := mkdir(t, base, "data")
	mkdir(t, base, "www")
	t.Chdir(base)
	if !Overlap(conf, "data") || !Overlap(conf, ".") || !Overlap(conf, filepath.Join("www", "..")) {
		t.Fatal("相对路径应按当前目录解析后判定为重叠")
	}
	if Overlap(conf, "www") {
		t.Fatal("同级的相对目录不应判定为重叠")
	}
	// 配置目录以相对路径给出（默认的 data）时同样生效
	if !Overlap("data", filepath.Join(base, "data", "x")) {
		t.Fatal("相对的配置目录应转为绝对路径后比较")
	}
}

func TestOverlapSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("创建符号链接需要特权")
	}
	base := t.TempDir()
	conf := mkdir(t, base, "etc", "gately")
	inside := mkdir(t, conf, "history")
	other := mkdir(t, base, "share")
	link := func(target, name string) string {
		p := filepath.Join(base, name)
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, p := range map[string]string{
		"指向配置目录的链接":      link(conf, "l1"),
		"指向配置目录内的链接":     link(inside, "l2"),
		"指向上级目录的链接":      link(filepath.Dir(conf), "l3"),
		"经过链接再进入配置目录":    filepath.Join(link(filepath.Dir(conf), "l4"), "gately", "history"),
		"链接到链接":          link(link(conf, "l5"), "l6"),
		"链接下尚不存在的子目录":    filepath.Join(link(conf, "l7"), "new", "dir"),
		"配置目录自身经链接访问的写法": filepath.Join(link(base, "l8"), "etc", "gately"),
	} {
		if !Overlap(conf, p) {
			t.Errorf("%s（%s）应判定为重叠", name, p)
		}
	}
	if Overlap(conf, link(other, "ok")) {
		t.Error("指向无关目录的链接不应判定为重叠")
	}
	// 配置目录本身是符号链接（例如 /etc/gately -> /mnt/disk/gately）时，真实位置同样受保护
	realConf := mkdir(t, base, "disk", "gately")
	confLink := link(realConf, "conf")
	if !Overlap(confLink, realConf) || !Overlap(confLink, filepath.Join(realConf, "sub")) || !Overlap(confLink, filepath.Dir(realConf)) {
		t.Error("配置目录为符号链接时，其真实位置应受保护")
	}
}

func TestOverlapCaseInsensitive(t *testing.T) {
	base := t.TempDir()
	conf := mkdir(t, base, "GatelyData")
	// 只在不区分大小写的文件系统上有意义：换一种大小写仍能打开同一个目录
	alt := filepath.Join(base, "gatelydata")
	if _, err := os.Stat(alt); err != nil {
		t.Skip("文件系统区分大小写")
	}
	if !Overlap(conf, alt) || !Overlap(conf, filepath.Join(strings.ToUpper(alt), "sub")) {
		t.Fatal("大小写不同的写法应判定为重叠")
	}
}

func TestProtectAndCheck(t *testing.T) {
	base := t.TempDir()
	conf := mkdir(t, base, "conf")
	share := mkdir(t, base, "share")
	Protect(conf)
	Protect(conf)
	Protect("")
	if got := Protected(filepath.Join(conf, "x")); got != conf {
		t.Fatalf("Protected = %q，期望 %q", got, conf)
	}
	if err := CheckExposable(base); err == nil {
		t.Fatal("包含配置目录的目录应被拒绝")
	}
	if err := CheckExposable(conf); err == nil {
		t.Fatal("配置目录应被拒绝")
	}
	if err := CheckExposable(share); err != nil {
		t.Fatalf("无关目录不应被拒绝: %v", err)
	}
	protectMu.RLock()
	n := 0
	for _, d := range protected {
		if d == conf {
			n++
		}
	}
	protectMu.RUnlock()
	if n != 1 {
		t.Fatalf("重复登记应只保留一份，实际 %d", n)
	}
}
