package crud

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type copyItem struct {
	Key   string
	Name  string
	On    bool
	Token string
	// Stamp 模拟系统维护的字段，复制时应被清空
	Stamp string
}

type copyFixture struct {
	items   []copyItem
	saveErr error
	changed []string
	guardOK bool
	guarded int
	mux     *http.ServeMux
	coll    *Collection[copyItem]
}

func newCopyFixture() *copyFixture {
	f := &copyFixture{
		items:   []copyItem{{Key: "a", Name: "A", On: true, Token: "secret-a", Stamp: "s-a"}, {Key: "b", Name: "B", Token: "secret-b"}},
		guardOK: true,
	}
	var mu sync.Mutex
	f.coll = &Collection[copyItem]{
		Mu:      &mu,
		Items:   &f.items,
		Save:    func() error { return f.saveErr },
		Key:     func(it *copyItem) *string { return &it.Key },
		Enabled: func(it *copyItem) *bool { return &it.On },
		Validate: func(it *copyItem) error {
			if strings.TrimSpace(it.Name) == "" {
				return errors.New("名称不能为空")
			}
			if it.Token == SecretMask {
				return errors.New("密钥不能为占位符")
			}
			return nil
		},
		Copy: func(src, dst *copyItem) {
			KeepSecret(src.Token, &dst.Token)
			dst.Stamp = ""
		},
		Changed: func(k string) { f.changed = append(f.changed, k) },
		Secrets: func(it *copyItem) map[string]string { return map[string]string{"token": it.Token} },
		SecretsGuard: func(w http.ResponseWriter, _ *http.Request) bool {
			f.guarded++
			if !f.guardOK {
				httpx403(w)
			}
			return f.guardOK
		},
	}
	f.mux = http.NewServeMux()
	f.coll.Register(muxRouter{f.mux}, "/items")
	return f
}

func httpx403(w http.ResponseWriter) {
	http.Error(w, `{"error":"需要先完成二次验证","need2fa":true}`, http.StatusForbidden)
}

func (f *copyFixture) post(path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *copyFixture) names() string {
	var out []string
	for _, it := range f.items {
		out = append(out, it.Name)
	}
	return strings.Join(out, ",")
}

// 复制：占位符密钥沿用模板，系统字段被清空，新条目有新 key 并排在模板之后，模板本身不变。
func TestCopyInheritsSecretAndInsertsAfterSource(t *testing.T) {
	f := newCopyFixture()
	rec := f.post("/items/a/copy", `{"name":"A 副本","on":true,"token":"********","stamp":"s-a"}`)
	if rec.Code != 200 {
		t.Fatalf("复制失败: %d %s", rec.Code, rec.Body.String())
	}
	if got := f.names(); got != "A,A 副本,B" {
		t.Fatalf("副本应排在模板之后: %s", got)
	}
	cp := f.items[1]
	if cp.Key == "" || cp.Key == "a" || cp.Key == "b" {
		t.Fatalf("副本应分配新 key: %q", cp.Key)
	}
	if cp.Token != "secret-a" {
		t.Fatalf("占位符应沿用模板的密钥，实际 %q", cp.Token)
	}
	if cp.Stamp != "" {
		t.Fatalf("系统维护的字段应被清空，实际 %q", cp.Stamp)
	}
	if f.items[0] != (copyItem{Key: "a", Name: "A", On: true, Token: "secret-a", Stamp: "s-a"}) {
		t.Fatalf("模板不应被修改: %+v", f.items[0])
	}
	if len(f.changed) != 1 || f.changed[0] != cp.Key {
		t.Fatalf("应只对副本调用 Changed: %v", f.changed)
	}
	// 响应为副本本身，便于前端定位
	var out copyItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Key != cp.Key {
		t.Fatalf("响应应为副本: %s", rec.Body.String())
	}
	if f.guarded != 1 {
		t.Fatalf("复制前应检查门禁 1 次，实际 %d", f.guarded)
	}
}

// 新填写的密钥原样保存，不被模板覆盖；复制最后一个条目时追加到末尾。
func TestCopyKeepsNewSecret(t *testing.T) {
	f := newCopyFixture()
	if rec := f.post("/items/b/copy", `{"name":"B2","token":"new-token"}`); rec.Code != 200 {
		t.Fatalf("复制失败: %d %s", rec.Code, rec.Body.String())
	}
	if got := f.names(); got != "A,B,B2" {
		t.Fatalf("复制末尾条目应追加到最后: %s", got)
	}
	if f.items[2].Token != "new-token" {
		t.Fatalf("新密钥应原样保存: %q", f.items[2].Token)
	}
}

// 模板不存在、门禁拒绝、校验失败、保存失败时都不新增条目，也不调用 Changed。
func TestCopyRejects(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		body  string
		setup func(*copyFixture)
		code  int
	}{
		{"模板不存在", "/items/x/copy", `{"name":"X","token":"t"}`, nil, 404},
		{"门禁拒绝", "/items/a/copy", `{"name":"A2","token":"********"}`, func(f *copyFixture) { f.guardOK = false }, 403},
		{"校验失败", "/items/a/copy", `{"name":" ","token":"********"}`, nil, 400},
		{"未知字段", "/items/a/copy", `{"name":"A2","evil":1}`, nil, 400},
		{"保存失败", "/items/a/copy", `{"name":"A2","token":"********"}`, func(f *copyFixture) { f.saveErr = errors.New("disk full") }, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newCopyFixture()
			if c.setup != nil {
				c.setup(f)
			}
			rec := f.post(c.path, c.body)
			if rec.Code != c.code {
				t.Fatalf("期望 %d，实际 %d %s", c.code, rec.Code, rec.Body.String())
			}
			if got := f.names(); got != "A,B" {
				t.Fatalf("失败时不应新增或残留条目: %s", got)
			}
			if len(f.changed) != 0 {
				t.Fatalf("失败时不应调用 Changed: %v", f.changed)
			}
		})
	}
}

// 数量上限对复制同样生效。
func TestCopyRespectsMax(t *testing.T) {
	f := newCopyFixture()
	f.coll.Max = 2
	if rec := f.post("/items/a/copy", `{"name":"A2","token":"********"}`); rec.Code != 400 || !strings.Contains(rec.Body.String(), "最多只能添加") {
		t.Fatalf("超过上限应拒绝: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.items) != 2 {
		t.Fatalf("不应新增: %d", len(f.items))
	}
}

// 未提供 Copy 时复制与新增相同：占位符不会被替换成模板的密钥，交由校验处理；
// 未提供 SecretsGuard 时不检查门禁。
func TestCopyWithoutHook(t *testing.T) {
	f := newCopyFixture()
	f.coll.Copy = nil
	f.coll.SecretsGuard = nil
	f.mux = http.NewServeMux()
	f.coll.Register(muxRouter{f.mux}, "/items")
	if rec := f.post("/items/a/copy", `{"name":"A2","token":"********"}`); rec.Code != 400 {
		t.Fatalf("没有 Copy 时占位符不应被替换: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.post("/items/a/copy", `{"name":"A2","token":"t2","stamp":"x"}`); rec.Code != 200 {
		t.Fatalf("复制失败: %d %s", rec.Code, rec.Body.String())
	}
	if f.items[1].Token != "t2" || f.items[1].Stamp != "x" {
		t.Fatalf("没有 Copy 时应原样保存: %+v", f.items[1])
	}
}

// 普通新增仍追加到末尾，保存失败时回滚。
func TestCreateStillAppends(t *testing.T) {
	f := newCopyFixture()
	if rec := f.post("/items", `{"name":"C","token":"t"}`); rec.Code != 200 {
		t.Fatalf("新增失败: %d %s", rec.Code, rec.Body.String())
	}
	if got := f.names(); got != "A,B,C" {
		t.Fatalf("新增应追加到末尾: %s", got)
	}
	f.saveErr = errors.New("disk full")
	if rec := f.post("/items", `{"name":"D","token":"t"}`); rec.Code != 500 {
		t.Fatalf("保存失败应返回 500: %d", rec.Code)
	}
	if got := f.names(); got != "A,B,C" {
		t.Fatalf("保存失败应回滚: %s", got)
	}
}

func assign(keys ...string) string {
	AssignKeys(len(keys), func(i int) *string { return &keys[i] }, "r")
	return strings.Join(keys, ",")
}

// 已有 key 保持不变，新条目避开已有 key；重复 key 保留第一个。
func TestAssignKeys(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"", "", ""}, "r1,r2,r3"},
		{[]string{"r1", "r2", "", "r3"}, "r1,r2,r3_,r3"},
		{[]string{"r1", "", "r2"}, "r1,r2_,r2"},
		{[]string{"x", "x", "r2"}, "x,r2_,r2"},
		{[]string{"", "r1"}, "r1_,r1"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := assign(c.in...); got != c.want {
			t.Errorf("AssignKeys(%v) = %s，期望 %s", c.in, got, c.want)
		}
	}
}
