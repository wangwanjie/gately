package crud

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// 数量上限只限制新增：达到上限后新增被拒绝且不保存，修改既有条目不受影响。
func TestMaxItems(t *testing.T) {
	items := []groupItem{{Key: "a"}, {Key: "b"}}
	saves := 0
	var mu sync.Mutex
	c := &Collection[groupItem]{
		Mu: &mu, Items: &items, Max: 2,
		Save:     func() error { saves++; return nil },
		Key:      func(it *groupItem) *string { return &it.Key },
		Enabled:  func(it *groupItem) *bool { return &it.On },
		Validate: func(*groupItem) error { return nil },
	}
	mux := http.NewServeMux()
	c.Register(muxRouter{mux}, "/items")
	call := func(method, path, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := call("POST", "/items", `{"On":true}`); code != 400 || len(items) != 2 || saves != 0 {
		t.Fatalf("达到上限应拒绝新增: code=%d len=%d saves=%d", code, len(items), saves)
	}
	if code := call("PUT", "/items/a", `{"On":true}`); code != 200 || !items[0].On {
		t.Fatalf("达到上限仍应允许修改: %d", code)
	}
	c.Max = 3
	if code := call("POST", "/items", `{}`); code != 200 || len(items) != 3 {
		t.Fatalf("未达上限应允许新增: %d", code)
	}
}
