package fileusers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/mountfs"
)

func TestValidate(t *testing.T) {
	ok := []User{
		{Name: " alice ", Password: "p", Root: " /data/a "},
		{Name: "bob", Password: "p", Root: "/ignored", ReadOnly: true, Mounts: []mountfs.Mount{{Name: "m", Path: "/data/m"}}},
	}
	if err := Validate(ok); err != nil {
		t.Fatal(err)
	}
	if ok[0].Name != "alice" || ok[0].Root != "/data/a" || ok[1].Root != "" || ok[1].ReadOnly {
		t.Fatalf("规范化结果不符: %+v", ok)
	}
	bad := map[string][]User{
		"空用户名":   {{Name: " ", Password: "p", Root: "/a"}},
		"空密码":    {{Name: "a", Root: "/a"}},
		"含冒号":    {{Name: "a:b", Password: "p", Root: "/a"}},
		"大小写重复":  {{Name: "Ann", Password: "p", Root: "/a"}, {Name: "ann", Password: "p", Root: "/b"}},
		"相对路径":   {{Name: "a", Password: "p", Root: "data"}},
		"挂载名称无效": {{Name: "a", Password: "p", Mounts: []mountfs.Mount{{Name: "", Path: "/a"}}}},
	}
	for name, users := range bad {
		if err := Validate(users); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
}

// TestValidateRemark 验证备注去掉首尾空白、超过 200 个字符时拒绝保存。
func TestValidateRemark(t *testing.T) {
	users := []User{{Name: "tv", Password: "p", Root: "/a", Remark: "  客厅电视  "}}
	if err := Validate(users); err != nil {
		t.Fatal(err)
	}
	if users[0].Remark != "客厅电视" {
		t.Fatalf("备注未去掉首尾空白: %q", users[0].Remark)
	}
	users[0].Remark = strings.Repeat("备", 200)
	if err := Validate(users); err != nil {
		t.Fatalf("200 个字符的备注应允许: %v", err)
	}
	users[0].Remark = strings.Repeat("备", 201)
	if err := Validate(users); err == nil {
		t.Fatal("超过 200 个字符的备注应拒绝")
	}
}

func TestCheckPassword(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if !CheckPassword("pw", "pw") || CheckPassword("pw", "bad") || !CheckPassword(string(h), "pw") || CheckPassword(string(h), "bad") {
		t.Fatal("密码校验结果错误")
	}
}

// TestSaveAndReload 验证保存时沿用脱敏占位符的原密码、写入 fileusers.json、通知订阅者，换配置目录时重新读取。
func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(dir)
	r := New()
	if err := r.Save(nil); err == nil {
		t.Fatal("未初始化时保存应失败")
	}
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	notified := 0
	cancel := r.Subscribe(func() { notified++ })
	if err := r.Save([]User{{Name: "alice", Password: "pa", Root: "/a"}, {Name: "bob", Password: "pb", Root: "/b"}}); err != nil {
		t.Fatal(err)
	}
	if notified != 1 {
		t.Fatalf("保存后应通知一次: %d", notified)
	}
	masked := r.Masked()
	if masked[0].Password != crud.SecretMask {
		t.Fatal("读取时密码应脱敏")
	}
	// 提交脱敏占位符表示不修改密码；alice 改名为 alice2 时按位置保留原密码
	masked[0].Name = "alice2"
	if err := r.Save(masked); err != nil {
		t.Fatal(err)
	}
	if u, ok := r.Lookup("alice2"); !ok || u.Password != "pa" {
		t.Fatalf("改名后应保留原密码: %+v", u)
	}
	if u, _ := r.Lookup("bob"); u.Password != "pb" {
		t.Fatal("未修改的密码应保留")
	}
	// 校验失败时不保存、不通知
	if err := r.Save([]User{{Name: "x", Password: "p", Root: "rel"}}); err == nil || notified != 2 || len(r.List()) != 2 {
		t.Fatalf("校验失败不应改动: %v %d", err, notified)
	}
	cancel()
	if err := r.Save(r.List()); err != nil || notified != 2 {
		t.Fatal("取消订阅后不应再通知")
	}

	// 同一配置目录再次 Init 不重新读取；新实例从文件读到保存的内容
	r2 := New()
	if err := r2.Init(st); err != nil {
		t.Fatal(err)
	}
	if u, ok := r2.Lookup("alice2"); !ok || u.Password != "pa" {
		t.Fatalf("应从 fileusers.json 读取: %+v", u)
	}
	other, _ := config.Open(t.TempDir())
	if err := r2.Init(other); err != nil || len(r2.List()) != 0 {
		t.Fatal("换配置目录后应重新读取")
	}
}

// TestCheckPasswordEmpty 验证保存的密码为空时任何密码（包括空密码）都不能通过。
func TestCheckPasswordEmpty(t *testing.T) {
	if CheckPassword("", "") || CheckPassword("", "x") {
		t.Fatal("空密码不应通过校验")
	}
}

// TestLoadInvalid 验证 fileusers.json 未通过校验（如被手工改成空密码）时共享用户都不能登录，
// 但列表仍可读取用于修改，保存一份有效的列表后恢复。
func TestLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(dir)
	if err := st.Save(ID, file{Users: []User{{Name: "alice", Password: "", Root: "/a"}, {Name: "bob", Password: "pb", Root: "/b"}}}); err != nil {
		t.Fatal(err)
	}
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	if r.LoadErr() == nil || len(r.List()) != 0 {
		t.Fatalf("无效的列表不应可用: %v %v", r.LoadErr(), r.List())
	}
	if _, ok := r.Lookup("bob"); ok {
		t.Fatal("无效的列表中的用户不应能登录")
	}
	if len(r.Masked()) != 2 {
		t.Fatal("无效的列表仍应可读取")
	}
	if err := r.Save([]User{{Name: "bob", Password: crud.SecretMask, Root: "/b"}}); err != nil {
		t.Fatal(err)
	}
	if u, ok := r.Lookup("bob"); r.LoadErr() != nil || !ok || u.Password != "pb" {
		t.Fatalf("保存有效的列表后应恢复: %v %+v", r.LoadErr(), u)
	}
}

// TestAddCheck 验证保存前的检查可以拒绝保存，取消后不再检查；换配置目录时清空旧的检查与订阅。
func TestAddCheck(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	notified := 0
	r.Subscribe(func() { notified++ })
	cancel := r.AddCheck("webdav", func(users []User) error {
		if len(users) == 0 {
			return errors.New("至少保留一个")
		}
		return nil
	})
	if err := r.Save([]User{{Name: "a", Password: "p", Root: "/a"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(nil); err == nil || len(r.List()) != 1 || notified != 1 {
		t.Fatalf("检查不通过时不应保存: %v %d", err, notified)
	}
	cancel()
	if err := r.Save(nil); err != nil || notified != 2 {
		t.Fatalf("取消检查后应可保存: %v", err)
	}
	r.AddCheck("webdav", func([]User) error { return errors.New("拒绝") })
	other, _ := config.Open(t.TempDir())
	if err := r.Init(other); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(nil); err != nil || notified != 2 {
		t.Fatalf("换配置目录后旧的检查与订阅应清空: %v %d", err, notified)
	}
}

// TestAddCheckInactive 验证在「功能模块」中关闭的服务不执行保存前检查，开启的服务照常检查。
func TestAddCheckInactive(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	r.AddCheck("ftp", func([]User) error { return errors.New("FTP 拒绝") })
	active := map[string]bool{"ftp": false}
	r.SetActive(func(id string) bool { return active[id] })
	if err := r.Save(nil); err != nil {
		t.Fatalf("已关闭的服务不应拦截保存: %v", err)
	}
	active["ftp"] = true
	if err := r.Save(nil); err == nil {
		t.Fatal("开启的服务应执行检查")
	}
}

// TestServices 验证各服务的使用情况按侧边栏顺序返回、可按 ID 过滤，换配置目录时清空。
func TestServices(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	r.AddService("smb", func() Service { return Service{Enable: true, CaseInsensitive: true} })
	r.AddService("webdav", func() Service { return Service{SharedUsers: true, LocalUsers: []string{"a"}} })
	r.AddService("ftp", func() Service { return Service{} })
	got := r.Services(nil)
	if len(got) != 3 || got[0].ID != "webdav" || got[1].ID != "ftp" || got[2].ID != "smb" {
		t.Fatalf("顺序不符: %+v", got)
	}
	if got[1].LocalUsers == nil || !got[0].SharedUsers || !got[2].CaseInsensitive {
		t.Fatalf("内容不符: %+v", got)
	}
	if got := r.Services(func(id string) bool { return id != "ftp" }); len(got) != 2 || got[1].ID != "smb" {
		t.Fatalf("过滤结果不符: %+v", got)
	}
	other, _ := config.Open(t.TempDir())
	if err := r.Init(other); err != nil {
		t.Fatal(err)
	}
	if got := r.Services(nil); len(got) != 0 {
		t.Fatalf("换配置目录后应清空: %+v", got)
	}
}

// TestCountShared 验证只统计没有同名本地用户的共享用户，fold 控制用户名是否区分大小写。
func TestCountShared(t *testing.T) {
	shared := []User{{Name: "alice"}, {Name: "Bob"}, {Name: "carol"}}
	cases := []struct {
		name  string
		local []string
		fold  bool
		want  int
	}{
		{"没有本地用户", nil, false, 3},
		{"同名共享用户不计入", []string{"alice", "dave"}, false, 2},
		{"区分大小写时 bob 与 Bob 不同名", []string{"bob"}, false, 3},
		{"不区分大小写时 bob 覆盖 Bob", []string{"bob"}, true, 2},
		{"全部同名", []string{"alice", "Bob", "carol"}, false, 0},
	}
	for _, c := range cases {
		if got := CountShared(c.local, shared, c.fold); got != c.want {
			t.Errorf("%s: 得到 %d，期望 %d", c.name, got, c.want)
		}
	}
	if got := CountShared([]string{"a"}, nil, false); got != 0 {
		t.Errorf("没有共享用户时应为 0: %d", got)
	}
}

// TestPasswords 验证查看共享用户密码明文的接口按用户名返回原始密码。
func TestPasswords(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	if err := r.Save([]User{{Name: "alice", Password: "pa", Root: "/a"}, {Name: "bob", Password: "pb", Root: "/b"}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.HandleSecrets()(rec, httptest.NewRequest("GET", "/api/fileusers/secrets", nil))
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(out) != 2 || out["alice"] != "pa" || out["bob"] != "pb" {
		t.Fatalf("应按用户名返回密码明文: %d %s", rec.Code, rec.Body.String())
	}
}

// TestChangePassword 验证共享用户自助修改密码：开关、当前密码校验、新密码规则、保持原有的明文或哈希格式并通知各服务。
func TestChangePassword(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("hashed-old-1"), bcrypt.MinCost)
	if err := r.Save([]User{{Name: "alice", Password: "plain-old-1", Root: "/a"}, {Name: "bob", Password: string(hash), Root: "/b"}}); err != nil {
		t.Fatal(err)
	}
	if r.SelfService() {
		t.Fatal("自助修改密码默认关闭")
	}
	if err := r.ChangePassword("alice", "plain-old-1", "plain-new-1"); !errors.Is(err, ErrSelfServiceOff) {
		t.Fatalf("未开启时应拒绝: %v", err)
	}
	on := true
	if err := r.save(r.List(), &on); err != nil || !r.SelfService() {
		t.Fatalf("开启失败: %v", err)
	}
	notified := 0
	r.Subscribe(func() { notified++ })

	for _, c := range []struct{ name, cur, next string }{
		{"alice", "wrong", "plain-new-1"},
		{"nobody", "plain-old-1", "plain-new-1"},
		{"Alice", "plain-old-1", "plain-new-1"},
	} {
		if err := r.ChangePassword(c.name, c.cur, c.next); !errors.Is(err, ErrBadCredentials) {
			t.Errorf("%+v 应返回用户名或密码错误: %v", c, err)
		}
	}
	for _, next := range []string{"short", "plain-old-1", crud.SecretMask, "$2a$10$abcdefgh"} {
		if err := r.ChangePassword("alice", "plain-old-1", next); err == nil || errors.Is(err, ErrBadCredentials) {
			t.Errorf("新密码 %q 应被拒绝: %v", next, err)
		}
	}
	if notified != 0 {
		t.Fatal("失败时不应保存")
	}
	if err := r.ChangePassword("alice", "plain-old-1", "plain-new-1"); err != nil {
		t.Fatal(err)
	}
	if u, _ := r.Lookup("alice"); u.Password != "plain-new-1" || u.Root != "/a" {
		t.Fatalf("明文密码应保持明文保存: %+v", u)
	}
	if err := r.ChangePassword("bob", "hashed-old-1", "hashed-new-1"); err != nil {
		t.Fatal(err)
	}
	if u, _ := r.Lookup("bob"); !CheckPassword(u.Password, "hashed-new-1") || u.Password == "hashed-new-1" {
		t.Fatalf("哈希密码应保持哈希保存: %+v", u)
	}
	if notified != 2 {
		t.Fatalf("每次修改应通知一次: %d", notified)
	}
	// 开关与修改后的密码写入文件
	r2 := New()
	if err := r2.Init(st); err != nil {
		t.Fatal(err)
	}
	if u, _ := r2.Lookup("alice"); !r2.SelfService() || u.Password != "plain-new-1" {
		t.Fatalf("应从文件读到开关与新密码: %v %+v", r2.SelfService(), u)
	}
}

// TestHandlePutSelfService 验证接口读写自助修改密码开关，旧接口不带该字段时保持原值。
func TestHandlePutSelfService(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	r := New()
	if err := r.Init(st); err != nil {
		t.Fatal(err)
	}
	put := func(body string) map[string]any {
		req := httptest.NewRequest("PUT", "/api/fileusers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.HandlePut(nil)(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != http.StatusOK {
			t.Fatalf("保存失败: %d %s", rec.Code, rec.Body.String())
		}
		return out
	}
	if out := put(`{"users":[{"name":"a","password":"p","root":"/a"}],"selfService":true}`); out["selfService"] != true {
		t.Fatalf("应开启: %v", out)
	}
	if out := put(`{"users":[{"name":"a","password":"********","root":"/a"}]}`); out["selfService"] != true {
		t.Fatalf("不带字段时应保持原值: %v", out)
	}
	if out := put(`{"users":[{"name":"a","password":"********","root":"/a"}],"selfService":false}`); out["selfService"] != false {
		t.Fatalf("应关闭: %v", out)
	}
}
