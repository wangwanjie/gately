package crud

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
)

// decodeLocked 严格解析请求体到 it，但先去掉视图独有的只读键。
// 列表接口输出的是 View 结构（条目加运行状态、统计、证书匹配等展示字段），客户端常把读到的对象改几处后整体提交；
// 这些展示字段不属于条目，严格解析会以未知字段拒绝，新版本加了展示字段而客户端（未刷新的旧页面、脚本）没跟着剥离时，
// 保存就会失败。这里只忽略「视图有而条目没有」的顶层键，其余未知字段照旧拒绝，以便及早暴露前后端不一致。
// sample 为用于确定视图类型的已有条目，可为 nil（此时不忽略任何键）。调用方需持有 Mu。
func (c *Collection[T]) decodeLocked(data []byte, it *T, sample *T) error {
	if keys := c.viewOnlyLocked(sample); len(keys) > 0 {
		var obj map[string]json.RawMessage
		// 不是 JSON 对象时交给下面的严格解析报告错误
		if json.Unmarshal(data, &obj) == nil && obj != nil {
			stripped := false
			for k := range obj {
				if keys[k] {
					delete(obj, k)
					stripped = true
				}
			}
			if stripped {
				if b, err := json.Marshal(obj); err == nil {
					data = b
				}
			}
		}
	}
	return httpx.DecodeJSON(data, it)
}

// viewOnlyLocked 返回视图独有的顶层 JSON 键，按 View 对 sample 的输出类型计算一次后缓存。
func (c *Collection[T]) viewOnlyLocked(sample *T) map[string]bool {
	if c.viewOnlyDone || c.View == nil || sample == nil {
		return c.viewOnly
	}
	c.viewOnlyDone = true
	c.viewOnly = ViewOnlyKeys(reflect.TypeOf(c.View(sample)), reflect.TypeFor[T]())
	return c.viewOnly
}

// ViewOnlyKeys 返回结构 view 比 item 多出的 JSON 键（都按 encoding/json 的规则展开内嵌结构）。
// view 不是结构（例如映射）时返回 nil；与 item 同名的键（例如视图里脱敏后覆盖的字段）不算多出，仍按条目字段解析。
func ViewOnlyKeys(view, item reflect.Type) map[string]bool {
	vk := jsonKeys(view)
	if vk == nil {
		return nil
	}
	ik := jsonKeys(item)
	var out map[string]bool
	for k := range vk {
		if !ik[k] {
			if out == nil {
				out = map[string]bool{}
			}
			out[k] = true
		}
	}
	return out
}

// jsonKeys 返回结构类型（或其指针）在 JSON 编码时的顶层键，非结构返回 nil。
func jsonKeys(t reflect.Type) map[string]bool {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	out := map[string]bool{}
	collectJSONKeys(t, out)
	return out
}

func collectJSONKeys(t reflect.Type, out map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		// 没有指定名称的内嵌结构，其字段提升到外层
		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			collectJSONKeys(ft, out)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
}
