// Package crud 为"有序规则列表"类配置提供通用的增删改查、启停与排序接口。
//
// 各模块只需提供取 key/启用字段的访问器、校验函数和变更回调，即可获得一致的 REST 接口：
//
//	GET    /<path>               列表
//	POST   /<path>               新增
//	POST   /<path>/{key}/copy    以已有条目为模板新增（复制），仍为占位符的密钥沿用模板的值
//	PUT    /<path>/{key}         修改
//	DELETE /<path>/{key}         删除
//	PUT    /<path>/{key}/enable  启停，body {"enable": bool}
//	PUT    /<path>-order         排序，body {"keys": [...]}
//	PUT    /<path>-group         批量设置分组，body {"keys": [...], "group": "..."}（仅在提供 Group 时注册）
//	PUT    /<path>-enable        批量启停，body {"keys": [...], "enable": bool}，全部成功才保存
//	POST   /<path>-delete        批量删除，body {"keys": [...]}，任一条目拒绝删除时都不删
//	GET    /<path>/{key}/secrets 查看条目中被脱敏的密钥明文（仅在提供 Secrets 时注册）
package crud

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// Collection 管理一个 []T 列表。Mu 与模块共享，回调在持锁状态下执行。
type Collection[T any] struct {
	Mu    *sync.Mutex
	Items *[]T
	// Save 持久化整个模块配置
	Save func() error
	// Key 与 Enabled 返回字段指针，用于读写
	Key     func(*T) *string
	Enabled func(*T) *bool
	// Validate 校验并规范化输入，在持锁状态下调用（新增、修改、复制均如此），不要在其中再次加 Mu
	Validate func(*T) error
	// Check 在持锁状态下做依赖其他配置的校验，例如引用的条目是否存在；在 Validate 之后调用，可为 nil
	Check func(*T) error
	// CheckEnable 在启用条目时（单个与批量启用）持锁调用，此时条目及同批要启用的条目都已标记为启用，
	// 用于检查“同时启用的条目之间不能冲突”一类规则；返回错误时撤销本次启用并拒绝。停用不调用；可为 nil
	CheckEnable func(*T) error
	// Merge 在修改时合并旧值，例如保留被脱敏的密钥；可为 nil
	Merge func(old, updated *T)
	// View 把条目转换为输出结构（附加运行状态、脱敏）；可为 nil
	View func(*T) any
	// Changed 在条目新增、修改、启停后调用，用于重启运行实例
	Changed func(key string)
	// Removed 在条目删除后调用
	Removed func(key string)
	// BeforeRemove 在删除前调用（持锁），返回错误时拒绝删除，例如条目仍被引用；可为 nil
	BeforeRemove func(it *T) error
	// Secrets 返回条目中被脱敏字段的明文（字段名 → 值），供管理界面“显示密码”使用；可为 nil。
	// 接口路径含 secrets 片段，按写权限鉴权，只读账号与只读令牌无法调用
	Secrets func(it *T) map[string]string
	// SecretsGuard 在查看密钥明文前调用（未持锁），返回 false 时已写入响应并拒绝，
	// 例如 Web 终端要求先通过二次验证；可为 nil
	SecretsGuard func(w http.ResponseWriter, r *http.Request) bool
	// Max 为条目数量上限，只在新增时检查，0 表示不限
	Max int
	// Group 返回条目的分组字段指针；提供时注册批量设置分组接口。分组只影响管理界面的展示，
	// 修改分组不调用 Changed，不会重启运行实例；可为 nil
	Group func(*T) *string
	// Copy 在复制条目（以已有条目为模板新增）时调用（持锁），src 为模板条目，dst 为客户端提交的新条目：
	// 把 dst 中仍为占位符的密钥恢复为模板的值，使密钥不必经过浏览器；并清除不应随复制带走的系统维护字段，
	// 例如已签发的证书、下载记录。只读取 src，不要让 dst 与 src 共用切片或映射；可为 nil
	Copy func(src, dst *T)
}

// MaxGroupLen 为分组名称的最大字符数。
const MaxGroupLen = 64

// NormalizeGroup 去掉分组名称首尾空白并校验长度，留空表示未分组。
func NormalizeGroup(g *string) error {
	*g = strings.TrimSpace(*g)
	if utf8.RuneCountInString(*g) > MaxGroupLen {
		return fmt.Errorf("分组名称不能超过 %d 个字符", MaxGroupLen)
	}
	return nil
}

// AssignKeys 为 n 个子条目中没有 key 或 key 重复的分配新 key（prefix 加序号，冲突时追加下划线），
// 用于监听中的站点、DDNS 任务中的记录等嵌套列表。先登记全部已有 key 再分配，插在列表中间的新条目
// （例如复制出的条目）不会抢走后面已有条目的 key；重复时保留第一个，后面的重新分配。
func AssignKeys(n int, key func(i int) *string, prefix string) {
	used := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		if k := *key(i); k != "" {
			used[k] = true
		}
	}
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		k := key(i)
		if *k != "" && !seen[*k] {
			seen[*k] = true
			continue
		}
		nk := fmt.Sprintf("%s%d", prefix, i+1)
		for used[nk] {
			nk += "_"
		}
		*k = nk
		used[nk] = true
		seen[nk] = true
	}
}

// NewKey 生成随机 key。
func NewKey() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (c *Collection[T]) index(key string) int {
	return slices.IndexFunc(*c.Items, func(it T) bool { return *c.Key(&it) == key })
}

// Find 返回 key 对应条目的指针，调用方需持有 Mu。
func (c *Collection[T]) Find(key string) *T {
	i := c.index(key)
	if i < 0 {
		return nil
	}
	return &(*c.Items)[i]
}

func (c *Collection[T]) view(it *T) any {
	if c.View != nil {
		return c.View(it)
	}
	return it
}

// Register 在 path 下注册全部接口。
func (c *Collection[T]) Register(r module.Router, path string) {
	r.HandleFunc("GET "+path, c.list)
	r.HandleFunc("POST "+path, c.create)
	r.HandleFunc("POST "+path+"/{key}/copy", c.copyItem)
	r.HandleFunc("PUT "+path+"/{key}", c.update)
	r.HandleFunc("DELETE "+path+"/{key}", c.remove)
	r.HandleFunc("PUT "+path+"/{key}/enable", c.enable)
	r.HandleFunc("PUT "+path+"-order", c.order)
	r.HandleFunc("PUT "+path+"-enable", c.batchEnable)
	r.HandleFunc("POST "+path+"-delete", c.batchDelete)

	// 登记接口文档：列表返回条目数组，新增与修改的请求体为条目本身
	var item T
	module.DocOf(r, "GET "+path, module.Doc{Summary: "列表", Resp: []T{}})
	module.DocOf(r, "POST "+path, module.Doc{Summary: "新增", Req: &item, Resp: &item})
	module.DocOf(r, "POST "+path+"/{key}/copy", module.Doc{Summary: "复制", Req: &item, Resp: &item})
	module.DocOf(r, "PUT "+path+"/{key}", module.Doc{Summary: "修改", Req: &item, Resp: &item})
	module.DocOf(r, "DELETE "+path+"/{key}", module.Doc{Summary: "删除"})
	module.DocOf(r, "PUT "+path+"/{key}/enable", module.Doc{Summary: "启用或停用", Req: &enableRequest{}})
	module.DocOf(r, "PUT "+path+"-order", module.Doc{Summary: "调整排序", Req: &orderRequest{}})
	module.DocOf(r, "PUT "+path+"-enable", module.Doc{Summary: "批量启用或停用", Req: &batchEnableRequest{}})
	module.DocOf(r, "POST "+path+"-delete", module.Doc{Summary: "批量删除", Req: &batchDeleteRequest{}})
	if c.Group != nil {
		r.HandleFunc("PUT "+path+"-group", c.setGroup)
		module.DocOf(r, "PUT "+path+"-group", module.Doc{Summary: "批量设置分组", Req: &groupRequest{}})
	}
	if c.Secrets != nil {
		r.HandleFunc("GET "+path+"/{key}/secrets", c.secrets)
		module.DocOf(r, "GET "+path+"/{key}/secrets", module.Doc{Summary: "查看密钥明文", Resp: map[string]string{}})
	}
}

func (c *Collection[T]) secrets(w http.ResponseWriter, r *http.Request) {
	if c.SecretsGuard != nil && !c.SecretsGuard(w, r) {
		return
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	it := c.Find(r.PathValue("key"))
	if it == nil {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	httpx.OK(w, c.Secrets(it))
}

func (c *Collection[T]) list(w http.ResponseWriter, r *http.Request) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	out := make([]any, 0, len(*c.Items))
	for i := range *c.Items {
		out = append(out, c.view(&(*c.Items)[i]))
	}
	httpx.OK(w, out)
}

func (c *Collection[T]) create(w http.ResponseWriter, r *http.Request) {
	var it T
	if err := httpx.ReadJSON(r, &it); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// 与修改、复制一样持锁校验：部分模块的校验会读取模块配置（例如 Web 服务检查引用的 IP 列表是否存在）
	c.Mu.Lock()
	defer c.Mu.Unlock()
	if err := c.Validate(&it); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c.insertLocked(w, it, len(*c.Items))
}

// copyItem 以路径中的条目为模板新增：请求体为新条目（通常是在模板基础上修改过的内容），
// 仍为占位符的密钥由 Copy 沿用模板的值。新条目排在模板之后，其余校验与新增相同。
func (c *Collection[T]) copyItem(w http.ResponseWriter, r *http.Request) {
	var it T
	if err := httpx.ReadJSON(r, &it); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// 沿用模板的密钥等同于取得密钥的使用权（例如把副本指向别的主机），与查看明文一样先过门禁，
	// 如 Web 终端的二次验证。门禁会读取模块配置，须在加锁前调用
	if c.SecretsGuard != nil && !c.SecretsGuard(w, r) {
		return
	}
	key := r.PathValue("key")
	c.Mu.Lock()
	defer c.Mu.Unlock()
	src := c.Find(key)
	if src == nil {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	if c.Copy != nil {
		c.Copy(src, &it)
	}
	if err := c.Validate(&it); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c.insertLocked(w, it, c.index(key)+1)
}

// insertLocked 检查数量上限与引用后，为已校验的条目分配新 key 并插入到 at 位置，保存失败时回滚。调用方需持有 Mu。
func (c *Collection[T]) insertLocked(w http.ResponseWriter, it T, at int) {
	if c.Max > 0 && len(*c.Items) >= c.Max {
		httpx.Error(w, http.StatusBadRequest, "最多只能添加 %d 条", c.Max)
		return
	}
	// 新条目的 key 由服务端分配，请求体中的 key 不参与检查，免得 Check 把新条目当成已有条目而跳过比较
	*c.Key(&it) = ""
	if c.Check != nil {
		if err := c.Check(&it); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	key := NewKey()
	*c.Key(&it) = key
	*c.Items = slices.Insert(*c.Items, at, it)
	if err := c.Save(); err != nil {
		*c.Items = slices.Delete(*c.Items, at, at+1)
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	if c.Changed != nil {
		c.Changed(key)
	}
	httpx.OK(w, c.view(c.Find(key)))
}

func (c *Collection[T]) update(w http.ResponseWriter, r *http.Request) {
	var it T
	if err := httpx.ReadJSON(r, &it); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := r.PathValue("key")
	c.Mu.Lock()
	defer c.Mu.Unlock()
	i := c.index(key)
	if i < 0 {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	old := (*c.Items)[i]
	// 以路径中的 key 为准（请求体通常不带 key），校验与检查时条目已带上自己的 key，可以据此排除自身
	*c.Key(&it) = key
	if c.Merge != nil {
		c.Merge(&old, &it)
	}
	if err := c.Validate(&it); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if c.Check != nil {
		if err := c.Check(&it); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	*c.Key(&it) = key
	(*c.Items)[i] = it
	if err := c.Save(); err != nil {
		(*c.Items)[i] = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	if c.Changed != nil {
		c.Changed(key)
	}
	httpx.OK(w, c.view(&(*c.Items)[i]))
}

func (c *Collection[T]) remove(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	c.Mu.Lock()
	defer c.Mu.Unlock()
	i := c.index(key)
	if i < 0 {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	if c.BeforeRemove != nil {
		if err := c.BeforeRemove(&(*c.Items)[i]); err != nil {
			httpx.Error(w, http.StatusConflict, "%v", err)
			return
		}
	}
	old := slices.Clone(*c.Items)
	*c.Items = slices.Delete(*c.Items, i, i+1)
	if err := c.Save(); err != nil {
		*c.Items = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	if c.Removed != nil {
		c.Removed(key)
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

type enableRequest struct {
	Enable bool `json:"enable"`
}

func (c *Collection[T]) enable(w http.ResponseWriter, r *http.Request) {
	var req enableRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := r.PathValue("key")
	c.Mu.Lock()
	defer c.Mu.Unlock()
	it := c.Find(key)
	if it == nil {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	en := c.Enabled(it)
	prev := *en
	*en = req.Enable
	if req.Enable && c.CheckEnable != nil {
		if err := c.CheckEnable(it); err != nil {
			*en = prev
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	if err := c.Save(); err != nil {
		*en = prev
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	if c.Changed != nil {
		c.Changed(key)
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

type orderRequest struct {
	Keys []string `json:"keys"`
}

func (c *Collection[T]) order(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	if len(req.Keys) != len(*c.Items) {
		httpx.Error(w, http.StatusBadRequest, "排序列表与条目数量不一致")
		return
	}
	reordered := make([]T, 0, len(req.Keys))
	seen := map[string]bool{}
	for _, k := range req.Keys {
		i := c.index(k)
		if i < 0 || seen[k] {
			httpx.Error(w, http.StatusBadRequest, "排序列表包含未知或重复条目 %s", k)
			return
		}
		seen[k] = true
		reordered = append(reordered, (*c.Items)[i])
	}
	old := *c.Items
	*c.Items = reordered
	if err := c.Save(); err != nil {
		*c.Items = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

type groupRequest struct {
	Keys []string `json:"keys"`
	// Group 为目标分组名称，留空表示移出分组
	Group string `json:"group"`
}

// setGroup 把多个条目移到同一分组，条目顺序不变
func (c *Collection[T]) setGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if len(req.Keys) == 0 {
		httpx.Error(w, http.StatusBadRequest, "请选择要设置分组的条目")
		return
	}
	if err := NormalizeGroup(&req.Group); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	idx := make([]int, 0, len(req.Keys))
	for _, k := range req.Keys {
		i := c.index(k)
		if i < 0 {
			httpx.Error(w, http.StatusNotFound, "条目 %s 不存在", k)
			return
		}
		idx = append(idx, i)
	}
	old := slices.Clone(*c.Items)
	for _, i := range idx {
		*c.Group(&(*c.Items)[i]) = req.Group
	}
	if err := c.Save(); err != nil {
		*c.Items = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

// MaxBatch 为一次批量操作最多处理的条目数，避免一次请求长时间占用模块锁与反复重启实例。
const MaxBatch = 500

// BatchKeys 校验批量操作的 key 列表：不能为空、不超过 MaxBatch，去重并保持原有顺序。
func BatchKeys(keys []string) ([]string, error) {
	// 先按原始数量粗略限制（允许一定的重复），避免超大请求体占用 CPU 与内存
	if len(keys) > MaxBatch*2 {
		return nil, fmt.Errorf("一次最多操作 %d 个条目", MaxBatch)
	}
	var out []string
	seen := map[string]bool{}
	for _, k := range keys {
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("请选择要操作的条目")
	}
	if len(out) > MaxBatch {
		return nil, fmt.Errorf("一次最多操作 %d 个条目", MaxBatch)
	}
	return out, nil
}

type batchEnableRequest struct {
	Keys   []string `json:"keys"`
	Enable bool     `json:"enable"`
}

// batchEnable 批量启停：先确认全部条目存在，再统一修改并保存一次；只对状态真正变化的条目调用 Changed。
func (c *Collection[T]) batchEnable(w http.ResponseWriter, r *http.Request) {
	var req batchEnableRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	keys, err := BatchKeys(req.Keys)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	its := make([]*T, 0, len(keys))
	for _, k := range keys {
		it := c.Find(k)
		if it == nil {
			httpx.Error(w, http.StatusNotFound, "条目 %s 不存在", k)
			return
		}
		its = append(its, it)
	}
	var changed []string
	var prev []bool
	for i, it := range its {
		en := c.Enabled(it)
		if *en != req.Enable {
			changed = append(changed, keys[i])
			prev = append(prev, *en)
			*en = req.Enable
		}
	}
	// restore 撤销本次批量启停
	restore := func() {
		for i, k := range changed {
			*c.Enabled(c.Find(k)) = prev[i]
		}
	}
	if req.Enable && c.CheckEnable != nil {
		for _, k := range changed {
			if err := c.CheckEnable(c.Find(k)); err != nil {
				restore()
				httpx.Error(w, http.StatusBadRequest, "%v", err)
				return
			}
		}
	}
	if len(changed) > 0 {
		if err := c.Save(); err != nil {
			restore()
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		if c.Changed != nil {
			for _, k := range changed {
				c.Changed(k)
			}
		}
	}
	httpx.OK(w, map[string]int{"changed": len(changed)})
}

type batchDeleteRequest struct {
	Keys []string `json:"keys"`
}

// batchDelete 批量删除：任一条目不存在或被拒绝删除（BeforeRemove 返回错误）时都不删除，错误信息中带上条目 key。
func (c *Collection[T]) batchDelete(w http.ResponseWriter, r *http.Request) {
	var req batchDeleteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	keys, err := BatchKeys(req.Keys)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	for _, k := range keys {
		it := c.Find(k)
		if it == nil {
			httpx.Error(w, http.StatusNotFound, "条目 %s 不存在", k)
			return
		}
		if c.BeforeRemove != nil {
			if err := c.BeforeRemove(it); err != nil {
				httpx.Error(w, http.StatusConflict, "%v", err)
				return
			}
		}
	}
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	old := slices.Clone(*c.Items)
	kept := make([]T, 0, len(old))
	for _, it := range old {
		if !drop[*c.Key(&it)] {
			kept = append(kept, it)
		}
	}
	*c.Items = kept
	if err := c.Save(); err != nil {
		*c.Items = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	if c.Removed != nil {
		for _, k := range keys {
			c.Removed(k)
		}
	}
	httpx.OK(w, map[string]int{"deleted": len(keys)})
}

// SecretMask 是 API 输出中替代真实密钥的占位符。客户端原样提交时服务端保留旧值。
const SecretMask = "********"

// Mask 返回脱敏后的值。
func Mask(s string) string {
	if s == "" {
		return ""
	}
	return SecretMask
}

// SecretsHandler 返回查看单例设置中密钥明文的接口处理函数，get 在持有 mu 时调用。
// 注册路径须以 /secrets 结尾，使其按写权限鉴权（见 apitoken.Required）。
func SecretsHandler(mu *sync.Mutex, get func() map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		httpx.OK(w, get())
	}
}

// KeepSecret 若新值为占位符则恢复旧值。
func KeepSecret(old string, updated *string) {
	if *updated == SecretMask {
		*updated = old
	}
}

// KeepSecrets 把列表中仍为占位符的密钥恢复为原值，用于按名称区分、没有独立 ID 的用户列表。
// 先按去空白后的名称匹配；找不到时视为改名，取同一位置的原条目，但要求条目数量不变且该原条目的名称
// 不在新列表中，避免调换顺序或删除条目时取错。仍匹配不上的占位符被清空，交由校验报错。
func KeepSecrets[T any](old, updated []T, name func(*T) string, secret func(*T) *string) {
	byName := map[string]string{}
	for i := range old {
		byName[strings.TrimSpace(name(&old[i]))] = *secret(&old[i])
	}
	newNames := map[string]bool{}
	for i := range updated {
		newNames[strings.TrimSpace(name(&updated[i]))] = true
	}
	for i := range updated {
		u := &updated[i]
		if *secret(u) != SecretMask {
			continue
		}
		prev, ok := byName[strings.TrimSpace(name(u))]
		if !ok && len(old) == len(updated) && !newNames[strings.TrimSpace(name(&old[i]))] {
			prev = *secret(&old[i])
		}
		KeepSecret(prev, secret(u))
	}
}
