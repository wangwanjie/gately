package crud

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type batchFixture struct {
	items   []groupItem
	saveErr error
	changed []string
	removed []string
	guard   map[string]bool
	mux     *http.ServeMux
	saves   int
	// checkEnable 为启用时的检查，nil 表示不检查
	checkEnable func(it *groupItem) error
}

func newBatchFixture() *batchFixture {
	f := &batchFixture{
		items: []groupItem{{Key: "a", On: true}, {Key: "b"}, {Key: "c", On: true}, {Key: "d"}},
		guard: map[string]bool{},
	}
	var mu sync.Mutex
	c := &Collection[groupItem]{
		Mu:       &mu,
		Items:    &f.items,
		Save:     func() error { f.saves++; return f.saveErr },
		Key:      func(it *groupItem) *string { return &it.Key },
		Enabled:  func(it *groupItem) *bool { return &it.On },
		Validate: func(*groupItem) error { return nil },
		Changed:  func(k string) { f.changed = append(f.changed, k) },
		Removed:  func(k string) { f.removed = append(f.removed, k) },
		CheckEnable: func(it *groupItem) error {
			if f.checkEnable != nil {
				return f.checkEnable(it)
			}
			return nil
		},
		BeforeRemove: func(it *groupItem) error {
			if f.guard[it.Key] {
				return fmt.Errorf("%s 仍被引用", it.Key)
			}
			return nil
		},
	}
	f.mux = http.NewServeMux()
	c.Register(muxRouter{f.mux}, "/items")
	return f
}

func (f *batchFixture) call(method, path, body string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	f.mux.ServeHTTP(rec, req)
	return rec.Code
}

func (f *batchFixture) keys() string {
	var out []string
	for _, it := range f.items {
		out = append(out, it.Key)
	}
	return strings.Join(out, ",")
}

// 批量启停：只对状态变化的条目调用 Changed，只保存一次；重复 key 只处理一次。
func TestBatchEnable(t *testing.T) {
	f := newBatchFixture()
	if code := f.call("PUT", "/items-enable", `{"keys":["a","b","b","c"],"enable":true}`); code != 200 {
		t.Fatalf("应成功，实际 %d", code)
	}
	if !f.items[1].On || !f.items[0].On || f.items[3].On {
		t.Fatalf("状态不对: %+v", f.items)
	}
	if strings.Join(f.changed, ",") != "b" || f.saves != 1 {
		t.Fatalf("只应对 b 调用 Changed 且保存一次: changed=%v saves=%d", f.changed, f.saves)
	}
	f.changed = nil
	if code := f.call("PUT", "/items-enable", `{"keys":["a","c"],"enable":false}`); code != 200 {
		t.Fatal(code)
	}
	if f.items[0].On || f.items[2].On || strings.Join(f.changed, ",") != "a,c" {
		t.Fatalf("停用结果不对: %+v %v", f.items, f.changed)
	}
}

// 批量启停：未知 key、空列表、超过上限、保存失败时都不修改任何条目，也不调用 Changed。
func TestBatchEnableRejected(t *testing.T) {
	f := newBatchFixture()
	snapshot := fmt.Sprint(f.items)
	if code := f.call("PUT", "/items-enable", `{"keys":["b","x"],"enable":true}`); code != 404 {
		t.Fatalf("未知条目应 404，实际 %d", code)
	}
	if code := f.call("PUT", "/items-enable", `{"keys":[],"enable":true}`); code != 400 {
		t.Fatalf("空列表应 400，实际 %d", code)
	}
	var many []string
	for i := 0; i <= MaxBatch; i++ {
		many = append(many, fmt.Sprintf("%q", fmt.Sprint("k", i)))
	}
	if code := f.call("PUT", "/items-enable", `{"keys":[`+strings.Join(many, ",")+`],"enable":true}`); code != 400 {
		t.Fatalf("超过上限应 400，实际 %d", code)
	}
	f.saveErr = errors.New("磁盘已满")
	if code := f.call("PUT", "/items-enable", `{"keys":["b","d"],"enable":true}`); code != 500 {
		t.Fatalf("保存失败应 500，实际 %d", code)
	}
	if fmt.Sprint(f.items) != snapshot || len(f.changed) != 0 {
		t.Fatalf("失败后应原样保留: %+v changed=%v", f.items, f.changed)
	}
}

// 启用检查：检查时同批条目都已标记为启用，拒绝时单个与批量启用都撤销、不保存、不调用 Changed；停用不检查。
func TestCheckEnable(t *testing.T) {
	f := newBatchFixture()
	// 规则：同时启用的条目最多 2 个（初始 a、c 已启用）
	f.checkEnable = func(*groupItem) error {
		n := 0
		for _, it := range f.items {
			if it.On {
				n++
			}
		}
		if n > 2 {
			return errors.New("最多同时启用 2 个")
		}
		return nil
	}
	snapshot := fmt.Sprint(f.items)
	if code := f.call("PUT", "/items/b/enable", `{"enable":true}`); code != 400 {
		t.Fatalf("单个启用应被拒绝，实际 %d", code)
	}
	if code := f.call("PUT", "/items-enable", `{"keys":["b","d"],"enable":true}`); code != 400 {
		t.Fatalf("批量启用应被拒绝，实际 %d", code)
	}
	if fmt.Sprint(f.items) != snapshot || f.saves != 0 || len(f.changed) != 0 {
		t.Fatalf("拒绝后应原样保留: %+v saves=%d changed=%v", f.items, f.saves, f.changed)
	}
	if code := f.call("PUT", "/items/a/enable", `{"enable":false}`); code != 200 {
		t.Fatalf("停用不应检查，实际 %d", code)
	}
	if code := f.call("PUT", "/items/b/enable", `{"enable":true}`); code != 200 || !f.items[1].On {
		t.Fatalf("腾出名额后应能启用，实际 %d", code)
	}
}

// 批量删除：保持剩余条目顺序，每个被删条目调用一次 Removed。
func TestBatchDelete(t *testing.T) {
	f := newBatchFixture()
	if code := f.call("POST", "/items-delete", `{"keys":["c","a","a"]}`); code != 200 {
		t.Fatalf("应成功，实际 %d", code)
	}
	if f.keys() != "b,d" || strings.Join(f.removed, ",") != "c,a" || f.saves != 1 {
		t.Fatalf("结果不对: %s removed=%v saves=%d", f.keys(), f.removed, f.saves)
	}
}

// 批量删除：有条目仍被引用、不存在、保存失败时一个都不删。
func TestBatchDeleteRejected(t *testing.T) {
	f := newBatchFixture()
	f.guard["c"] = true
	if code := f.call("POST", "/items-delete", `{"keys":["a","c"]}`); code != 409 || f.keys() != "a,b,c,d" {
		t.Fatalf("被引用应 409 且不删: %d %s", code, f.keys())
	}
	if code := f.call("POST", "/items-delete", `{"keys":["a","zz"]}`); code != 404 || f.keys() != "a,b,c,d" {
		t.Fatalf("未知条目应 404 且不删: %d %s", code, f.keys())
	}
	if code := f.call("POST", "/items-delete", `{"keys":[]}`); code != 400 {
		t.Fatalf("空列表应 400，实际 %d", code)
	}
	f.saveErr = errors.New("磁盘已满")
	if code := f.call("POST", "/items-delete", `{"keys":["a","b"]}`); code != 500 || f.keys() != "a,b,c,d" || len(f.removed) != 0 {
		t.Fatalf("保存失败应回滚: %d %s removed=%v", code, f.keys(), f.removed)
	}
}
