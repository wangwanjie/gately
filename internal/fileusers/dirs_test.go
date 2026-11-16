package fileusers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fsperm/fspermtest"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// 保存共享用户时拒绝是配置目录或位于其中的目录，包含配置目录的目录可以保存（由文件访问层隐藏配置目录）；
// 升级前已保存的位于配置目录中的目录不再提供，其余用户与目录照常使用。
func TestSharedUserDirsAndConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	r := New()
	if err := r.Init(l.Store); err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]User{
		"配置目录":    {Name: "a", Password: "p", Root: l.Conf},
		"配置目录内":   {Name: "a", Password: "p", Root: filepath.Join(l.Conf, "history")},
		"多目录中的一个": {Name: "a", Password: "p", Mounts: []mountfs.Mount{{Name: "ok", Path: l.Media}, {Name: "bad", Path: l.Conf}}},
	} {
		if err := r.Save([]User{u}); err == nil || !strings.Contains(err.Error(), "配置目录") {
			t.Errorf("%s 应被拒绝，实际 %v", name, err)
		}
	}
	if err := r.Save([]User{{Name: "a", Password: "p", Root: l.Share}, {Name: "b", Password: "p", Mounts: []mountfs.Mount{{Name: "up", Path: filepath.Dir(l.Share)}}}}); err != nil {
		t.Fatalf("包含配置目录的目录应能保存: %v", err)
	}
	if notes := r.HiddenNotes(); len(notes) != 2 {
		t.Fatalf("应说明两个目录中的配置目录已自动隐藏: %v", notes)
	}
	if u, _ := r.Lookup("a"); u.Root != l.Share {
		t.Fatalf("包含配置目录的目录应照常提供: %+v", u)
	}

	// 升级前保存的列表：直接写入文件后重新加载
	old := file{Users: []User{
		{Name: "single", Password: "p1", Root: l.Conf},
		{Name: "multi", Password: "p2", Mounts: []mountfs.Mount{{Name: "ok", Path: l.Media}, {Name: "bad", Path: filepath.Join(l.Conf, "history")}}},
		{Name: "whole", Password: "p3", Root: l.Share},
	}, SelfService: true}
	if err := l.Store.Save(ID, old); err != nil {
		t.Fatal(err)
	}
	st2, _ := config.Open(l.Conf)
	r = New()
	if err := r.Init(st2); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadErr(); err != nil {
		t.Fatalf("已保存的列表不应整体失效: %v", err)
	}
	if got := r.Blocked(); len(got) != 2 {
		t.Fatalf("应报告 2 个被停止提供的目录，实际 %v", got)
	}
	if got := r.HiddenNotes(); len(got) != 1 {
		t.Fatalf("应说明 1 个目录中的配置目录已自动隐藏，实际 %v", got)
	}
	u, ok := r.Lookup("single")
	if !ok || u.Root != BlockedDir {
		t.Fatalf("单目录用户的目录应换成不存在的路径: %+v %v", u, ok)
	}
	if _, err := os.Stat(BlockedDir); err == nil {
		t.Fatalf("占位路径 %s 不应存在", BlockedDir)
	}
	if err := os.MkdirAll(BlockedDir, 0o755); err == nil {
		t.Fatalf("占位路径 %s 不应能被创建", BlockedDir)
	}
	u, _ = r.Lookup("multi")
	if len(u.Mounts) != 2 || u.Mounts[0].Path != l.Media || u.Mounts[1].Path != BlockedDir {
		t.Fatalf("多目录用户只替换位于配置目录中的目录: %+v", u.Mounts)
	}
	if u, _ := r.Lookup("whole"); u.Root != l.Share {
		t.Fatalf("包含配置目录的目录应照常提供: %+v", u)
	}
	for _, lu := range r.List() {
		if lu.Root == l.Conf {
			t.Fatalf("List 不应返回配置目录: %+v", lu)
		}
	}
	// 界面展示的仍是原配置，便于管理员看到并修改
	if m := r.Masked(); m[0].Root != l.Conf {
		t.Fatalf("界面应展示原目录: %+v", m)
	}
	// 自助修改密码不受存量目录影响，且不会把占位路径写回配置
	if err := r.ChangePassword("whole", "p3", "new-password-1"); err != nil {
		t.Fatalf("自助修改密码应成功: %v", err)
	}
	if m := r.Masked(); m[0].Root != l.Conf {
		t.Fatalf("自助修改密码后原目录不应被改写: %+v", m[0])
	}
	// 管理员再次保存时必须先改掉位于配置目录中的目录
	if err := r.Save(r.Masked()); err == nil {
		t.Fatal("管理员保存仍含配置目录的列表应被拒绝")
	}
}
