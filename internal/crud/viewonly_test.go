package crud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type voInner struct {
	Port int `json:"port"`
}

type voItem struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	On    bool   `json:"enable"`
	Token string `json:"token"`
	voInner
}

// voView 模拟带运行状态的视图：running/stats/certs 为展示字段，token 覆盖为脱敏值（与条目同名，不算展示字段）
type voView struct {
	voItem
	Running bool           `json:"running"`
	Stats   map[string]int `json:"stats,omitempty"`
	Certs   []string       `json:"siteCerts,omitempty"`
	Token   string         `json:"token"`
}

func newVOFixture(items []voItem) (*http.ServeMux, *[]voItem) {
	var mu sync.Mutex
	c := &Collection[voItem]{
		Mu:       &mu,
		Items:    &items,
		Save:     func() error { return nil },
		Key:      func(it *voItem) *string { return &it.Key },
		Enabled:  func(it *voItem) *bool { return &it.On },
		Validate: func(*voItem) error { return nil },
		View: func(it *voItem) any {
			return voView{voItem: *it, Running: it.On, Stats: map[string]int{"n": 1}, Certs: []string{"x"}, Token: SecretMask}
		},
	}
	mux := http.NewServeMux()
	c.Register(muxRouter{mux}, "/items")
	return mux, &items
}

func voDo(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func TestViewOnlyKeys(t *testing.T) {
	got := ViewOnlyKeys(reflect.TypeFor[voView](), reflect.TypeFor[voItem]())
	want := map[string]bool{"running": true, "stats": true, "siteCerts": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("视图独有键 %v，期望 %v", got, want)
	}
	if ViewOnlyKeys(reflect.TypeFor[map[string]any](), reflect.TypeFor[voItem]()) != nil {
		t.Fatal("映射视图不应有视图独有键")
	}
	if ViewOnlyKeys(reflect.TypeFor[*voItem](), reflect.TypeFor[voItem]()) != nil {
		t.Fatal("视图就是条目本身时不应有视图独有键")
	}
}

// 把列表接口读到的对象（带展示字段）整体提交：修改、复制、新增都接受并忽略展示字段，其他未知字段仍拒绝。
func TestViewOnlyFieldsAccepted(t *testing.T) {
	mux, items := newVOFixture([]voItem{{Key: "a", Name: "A", On: true, Token: "secret", voInner: voInner{Port: 80}}})
	rec := voDo(mux, "GET", "/items", "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("列表: %v %s", err, rec.Body)
	}
	obj := list[0]
	obj["name"] = "A2"
	obj["port"] = 8080
	body, _ := json.Marshal(obj)

	if rec := voDo(mux, "PUT", "/items/a", string(body)); rec.Code != 200 {
		t.Fatalf("修改应接受展示字段: %d %s", rec.Code, rec.Body)
	}
	if it := (*items)[0]; it.Name != "A2" || it.Port != 8080 || it.Token != SecretMask {
		// token 与条目同名，按条目字段解析（是否沿用原值由模块的 Merge 决定，这里没有 Merge）
		t.Fatalf("修改结果不对: %+v", it)
	}
	if rec := voDo(mux, "POST", "/items/a/copy", string(body)); rec.Code != 200 {
		t.Fatalf("复制应接受展示字段: %d %s", rec.Code, rec.Body)
	}
	if rec := voDo(mux, "POST", "/items", string(body)); rec.Code != 200 {
		t.Fatalf("已有条目时新增应接受展示字段: %d %s", rec.Code, rec.Body)
	}
	if len(*items) != 3 {
		t.Fatalf("条目数 %d，期望 3", len(*items))
	}

	obj["bogus"] = true
	body, _ = json.Marshal(obj)
	if rec := voDo(mux, "PUT", "/items/a", string(body)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "bogus") {
		t.Fatalf("其他未知字段应拒绝: %d %s", rec.Code, rec.Body)
	}
	if rec := voDo(mux, "PUT", "/items/a", `[1]`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非对象请求体应拒绝: %d %s", rec.Code, rec.Body)
	}
}

// 没有已有条目时无法确定视图类型，新增照旧严格解析。
func TestViewOnlyEmptyCollection(t *testing.T) {
	mux, _ := newVOFixture(nil)
	if rec := voDo(mux, "POST", "/items", `{"name":"x","running":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("空列表新增带展示字段应按未知字段拒绝: %d %s", rec.Code, rec.Body)
	}
	if rec := voDo(mux, "POST", "/items", `{"name":"x"}`); rec.Code != 200 {
		t.Fatalf("新增: %d %s", rec.Code, rec.Body)
	}
}
