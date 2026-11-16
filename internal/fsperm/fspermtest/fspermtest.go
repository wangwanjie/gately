// Package fspermtest 为各文件服务的测试建立同一种目录布局：整个数据盘被共享，Gately 的配置目录就在其中。
// 只供测试使用。本包不是 _test.go 文件，其中的中文字符串会被 i18nsync 当作界面文案收进翻译表，
// 所以测试失败信息用英文书写。
package fspermtest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
)

// Secret 是配置目录中 account.json 的内容里的标记，任何响应里都不应出现它。
const Secret = "top-secret-session-value"

// Layout 是测试目录布局（均为绝对路径）：
//
//	Share/                       共享出去的目录（相当于 /vol1）
//	  media/hello.txt            普通文件，内容为 hello
//	  1000/docker/gately/        配置目录 Conf，内有 account.json 与 history/
//	  1000/docker/gately-old/    名称只是前缀相同的目录，内有 keep.txt
//	  link -> 1000/docker/gately 指向配置目录的相对符号链接（Windows 上不建）
//	  up   -> 1000/docker        指向配置目录上级的相对符号链接（Windows 上不建）
type Layout struct {
	Share, Conf, Media, Docker, Similar string
	// Store 为以 Conf 打开的配置存储，打开时已把 Conf 登记为受保护目录
	Store *config.Store
	// Links 表示是否建立了符号链接
	Links bool
}

// New 建立布局并打开配置存储。
func New(t testing.TB) Layout {
	t.Helper()
	share := filepath.Join(t.TempDir(), "vol1")
	l := Layout{
		Share:   share,
		Media:   filepath.Join(share, "media"),
		Docker:  filepath.Join(share, "1000", "docker"),
		Conf:    filepath.Join(share, "1000", "docker", "gately"),
		Similar: filepath.Join(share, "1000", "docker", "gately-old"),
	}
	for _, d := range []string{l.Media, filepath.Join(l.Conf, "history"), l.Similar} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	st, err := config.Open(l.Conf)
	if err != nil {
		t.Fatal(err)
	}
	l.Store = st
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(l.Conf, "account.json"), `{"sessionSecret":"`+Secret+`"}`)
	write(filepath.Join(l.Media, "hello.txt"), "hello")
	write(filepath.Join(l.Similar, "keep.txt"), "keep")
	if runtime.GOOS != "windows" {
		// 相对链接：共享目录内的链接通常如此，os.Root 也只跟随不越出根目录的相对链接
		if err := os.Symlink(filepath.Join("1000", "docker", "gately"), filepath.Join(share, "link")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("1000", "docker"), filepath.Join(share, "up")); err != nil {
			t.Fatal(err)
		}
		l.Links = true
	}
	return l
}

// HiddenFiles 返回从共享目录出发、指向配置目录中文件的各种写法（以 / 分隔的相对路径），都必须读不到：
// 直接路径、含 .. 的路径、大小写不同的写法、经符号链接的路径。
func (l Layout) HiddenFiles() []string {
	out := []string{
		"1000/docker/gately/account.json",
		"media/../1000/docker/gately/account.json",
		"1000/docker/GATELY/account.json",
		"1000/DOCKER/Gately/account.json",
	}
	if l.Links {
		out = append(out, "link/account.json", "up/gately/account.json")
	}
	return out
}

// HiddenDirs 返回指向配置目录本身的各种写法，列目录同样必须失败。
func (l Layout) HiddenDirs() []string {
	out := []string{"1000/docker/gately", "1000/docker/GATELY"}
	if l.Links {
		out = append(out, "link", "up/gately")
	}
	return out
}

// VisibleFiles 返回共享目录中照常可读的文件及其内容。
func (l Layout) VisibleFiles() map[string]string {
	out := map[string]string{"media/hello.txt": "hello", "1000/docker/gately-old/keep.txt": "keep"}
	if l.Links {
		out["up/gately-old/keep.txt"] = "keep"
	}
	return out
}

// Listings 返回各目录的列表中必须出现（true）或不得出现（false）的名称。
func (l Layout) Listings() map[string]map[string]bool {
	root := map[string]bool{"media": true, "1000": true}
	if l.Links {
		root["link"], root["up"] = false, true
	}
	out := map[string]map[string]bool{
		"":            root,
		"1000/docker": {"gately": false, "gately-old": true},
	}
	if l.Links {
		out["up"] = map[string]bool{"gately": false, "gately-old": true}
	}
	return out
}

// Renames 返回必须被拒绝的改名（源, 目标）：移入、移出配置目录，移动配置目录本身或它的上级目录。
func (l Layout) Renames() [][2]string {
	return [][2]string{
		{"media/hello.txt", "1000/docker/gately/hello.txt"},
		{"1000/docker/gately/account.json", "media/stolen.json"},
		{"1000/docker/gately", "media/g"},
		{"1000/docker", "media/d"},
		{"1000", "x1000"},
	}
}

// Removals 返回必须被拒绝的删除：配置目录、其中的文件，以及包含它的上级目录。
func (l Layout) Removals() []string {
	return []string{"1000/docker/gately/account.json", "1000/docker/gately", "1000/docker", "1000"}
}

// Creates 返回必须被拒绝的新建路径（在配置目录中新建文件或目录）。
func (l Layout) Creates() []string {
	out := []string{"1000/docker/gately/new.txt", "1000/docker/gately/newdir"}
	if l.Links {
		out = append(out, "link/new2.txt")
	}
	return out
}

// Check 确认操作结束后配置目录完好、没有多出文件，共享目录中的普通文件仍在。
func (l Layout) Check(t testing.TB) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(l.Conf, "account.json"))
	if err != nil || !containsSecret(string(b)) {
		t.Fatalf("file in the configuration directory was modified: %q %v", b, err)
	}
	entries, err := os.ReadDir(l.Conf)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch e.Name() {
		case "account.json", "history":
		default:
			if filepath.Ext(e.Name()) != ".json" && e.Name() != ".history" {
				t.Fatalf("unexpected entry %s appeared in the configuration directory", e.Name())
			}
		}
	}
	if _, err := os.Stat(filepath.Join(l.Media, "hello.txt")); err != nil {
		t.Fatalf("ordinary file in the share should be untouched: %v", err)
	}
}

func containsSecret(s string) bool { return strings.Contains(s, Secret) }
