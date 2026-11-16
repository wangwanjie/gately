package fileusers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// 保存共享用户时拒绝与配置目录重叠的目录；升级前已保存的这类目录不再提供，其余用户与目录照常使用。
func TestSharedUserDirsOutsideConfigDir(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
	share := filepath.Join(base, "share")
	for _, d := range []string{filepath.Join(conf, "history"), share} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	st, err := config.Open(conf)
	if err != nil {
		t.Fatal(err)
	}
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]User{
		"配置目录":      {Name: "a", Password: "p", Root: conf},
		"配置目录内":     {Name: "a", Password: "p", Root: filepath.Join(conf, "history")},
		"包含配置目录":    {Name: "a", Password: "p", Root: base},
		"多目录中的一个":   {Name: "a", Password: "p", Mounts: []mountfs.Mount{{Name: "ok", Path: share}, {Name: "bad", Path: conf}}},
		"多目录中的上级目录": {Name: "a", Password: "p", Mounts: []mountfs.Mount{{Name: "bad", Path: filepath.Dir(base)}}},
	} {
		if err := r.Save([]User{u}); err == nil || !strings.Contains(err.Error(), "配置目录") {
			t.Errorf("%s 应被拒绝，实际 %v", name, err)
		}
	}
	if err := r.Save([]User{{Name: "a", Password: "p", Root: share}}); err != nil {
		t.Fatalf("无关目录应能保存: %v", err)
	}

	// 升级前保存的列表：直接写入文件后重新加载
	old := file{Users: []User{
		{Name: "single", Password: "p1", Root: conf},
		{Name: "multi", Password: "p2", Mounts: []mountfs.Mount{{Name: "ok", Path: share}, {Name: "bad", Path: base}}},
		{Name: "fine", Password: "p3", Root: share},
	}, SelfService: true}
	if err := st.Save(ID, old); err != nil {
		t.Fatal(err)
	}
	st2, _ := config.Open(conf)
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
	if len(u.Mounts) != 2 || u.Mounts[0].Path != share || u.Mounts[1].Path != BlockedDir {
		t.Fatalf("多目录用户只替换重叠的目录: %+v", u.Mounts)
	}
	for _, lu := range r.List() {
		if lu.Root == conf {
			t.Fatalf("List 不应返回配置目录: %+v", lu)
		}
		for _, mt := range lu.Mounts {
			if mt.Path == base {
				t.Fatalf("List 不应返回包含配置目录的目录: %+v", lu)
			}
		}
	}
	// 界面展示的仍是原配置，便于管理员看到并修改
	if m := r.Masked(); m[0].Root != conf || m[1].Mounts[1].Path != base {
		t.Fatalf("界面应展示原目录: %+v", m)
	}
	// 自助修改密码不受存量目录影响，且不会把占位路径写回配置
	if err := r.ChangePassword("fine", "p3", "new-password-1"); err != nil {
		t.Fatalf("自助修改密码应成功: %v", err)
	}
	if m := r.Masked(); m[0].Root != conf {
		t.Fatalf("自助修改密码后原目录不应被改写: %+v", m[0])
	}
	// 管理员再次保存时必须先改掉这些目录
	if err := r.Save(r.Masked()); err == nil {
		t.Fatal("管理员保存仍含配置目录的列表应被拒绝")
	}
}
