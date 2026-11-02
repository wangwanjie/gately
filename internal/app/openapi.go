package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/version"
)

// 官网地址，写入 OpenAPI 的 externalDocs
const websiteURL = "https://gately.cn"

// endpoint 是文档中的一个接口。
type endpoint struct {
	Method  string
	Path    string
	Tag     string
	Summary string
	Desc    string
	// Scope 为所需权限；Denied 表示令牌不可访问
	Scope string
	// Req / Resp 为 schema（可能是 $ref），为 nil 表示无
	Req     map[string]any
	ReqFree bool
	Resp    map[string]any
	Query   []string
}

// docModel 是文档的中间表示，OpenAPI JSON 与 Markdown 都从它渲染。
type docModel struct {
	Endpoints []endpoint
	Schemas   map[string]map[string]any
	Tags      []string
	Domains   []scopeDomain
}

var pathParamRe = regexp.MustCompile(`\{([^}/]+)\}`)

// buildDoc 根据已登记的路由生成文档模型。
func (a *App) buildDoc() docModel {
	// 路由表来自处理器构建，重复构建只会刷新登记结果
	if len(a.routes) == 0 {
		a.Handler()
	}
	titles := map[string]string{}
	for _, e := range a.modules {
		titles[e.mod.ID()] = e.mod.Title()
	}
	schemas := newSchemaSet()
	var eps []endpoint
	for _, r := range a.routes {
		key := r.Method + " " + r.Path
		ep := endpoint{Method: r.Method, Path: r.Path, Scope: apitoken.Required(r.Method, r.Path)}
		ep.Tag = docTag(r, titles)
		// 日志接口支持 since 增量拉取、before 加载更早的日志、limit 限制条数，以及按级别、关键词、IP 等过滤
		if r.Method == http.MethodGet && (r.Path == "/api/logs" || (r.Module != "" && r.Path == "/api/"+r.Module+"/logs")) {
			ep.Query = append([]string{"since", "before", "limit"}, logFilterParams...)
		}
		d, hasDoc := a.routeDocs[key]
		if hasDoc {
			if d.Req != nil {
				ep.Req = schemas.of(reflect.TypeOf(d.Req))
			}
			if d.Resp != nil {
				ep.Resp = schemas.of(reflect.TypeOf(d.Resp))
			}
		}
		if c, ok := apiCatalog[key]; ok {
			ep.Summary, ep.Desc, ep.ReqFree = c.Summary, c.Desc, c.Body
		} else if c, ok := apiCatalog[genericKey(r)]; ok {
			ep.Summary, ep.Desc, ep.ReqFree = c.Summary, c.Desc, c.Body
		} else if hasDoc {
			ep.Summary = crudSummary(r, d.Summary)
		}
		if ep.Req != nil {
			ep.ReqFree = false
		}
		eps = append(eps, ep)
	}
	sort.SliceStable(eps, func(i, j int) bool {
		if eps[i].Tag != eps[j].Tag {
			return tagRank(eps[i].Tag) < tagRank(eps[j].Tag)
		}
		return false
	})
	var tags []string
	seen := map[string]bool{}
	for _, e := range eps {
		if !seen[e.Tag] {
			seen[e.Tag] = true
			tags = append(tags, e.Tag)
		}
	}
	return docModel{Endpoints: eps, Schemas: schemas.defs, Tags: tags, Domains: a.scopeDomains()}
}

// genericPaths 是各模块共用的接口（模块日志、托管程序的在线安装与恢复默认配置），路径相对模块前缀。
var genericPaths = []string{"/logs", "/install", "/install/rollback", "/install/backups/{id}", "/reset"}

// genericKey 把模块日志等通用接口映射到目录中的通用条目。
func genericKey(r routeInfo) string {
	if r.Module == "" {
		return ""
	}
	for _, p := range genericPaths {
		if r.Path == "/api/"+r.Module+p {
			return r.Method + " /api/{module}" + p
		}
	}
	return ""
}

func docTag(r routeInfo, titles map[string]string) string {
	if r.Module != "" {
		return titles[r.Module]
	}
	switch {
	case strings.HasPrefix(r.Path, "/api/tokens"):
		return "API 令牌"
	case strings.HasPrefix(r.Path, "/api/account"):
		return "账号"
	case r.Path == "/api/import/lucky":
		return "配置导入"
	case r.Path == "/api/fileusers" || strings.HasPrefix(r.Path, "/api/fileusers/"):
		return "文件服务共享用户"
	}
	return "系统"
}

// tagRank 让系统类接口排在功能模块之后，功能模块保持注册顺序。
func tagRank(tag string) int {
	switch tag {
	case "文件服务共享用户":
		return 999
	case "系统":
		return 1000
	case "配置导入":
		return 1001
	case "API 令牌":
		return 1002
	case "账号":
		return 1003
	}
	return 0
}

// crudSummary 由通用动作和集合名称拼出中文摘要，例如“获取 DDNS 任务列表”。
func crudSummary(r routeInfo, verb string) string {
	// 模块自定义的完整摘要原样使用，只有通用 CRUD 动作才拼接集合名称
	switch verb {
	case "列表", "新增", "修改", "删除", "调整排序", "批量设置分组", "启用或停用", "批量启用或停用", "批量删除":
	default:
		return verb
	}
	rest := strings.TrimPrefix(r.Path, "/api/"+r.Module)
	coll := strings.TrimPrefix(rest, "/")
	coll, _, _ = strings.Cut(coll, "/")
	for _, suffix := range []string{"-order", "-group", "-enable", "-delete"} {
		coll = strings.TrimSuffix(coll, suffix)
	}
	noun := crudNouns[r.Module+"/"+coll]
	if noun == "" {
		noun = "条目"
	}
	// 用整句格式串而不是拼接，便于各语言按自己的语序翻译
	switch verb {
	case "列表":
		// 名词本身以“列表”结尾（如订阅 IP 列表）时不再重复
		if strings.HasSuffix(noun, "列表") {
			return fmt.Sprintf("获取%s", noun)
		}
		return fmt.Sprintf("获取%s列表", noun)
	case "调整排序":
		return fmt.Sprintf("调整%s排序", noun)
	case "批量设置分组":
		return fmt.Sprintf("批量设置%s分组", noun)
	case "启用或停用":
		return fmt.Sprintf("启用或停用%s", noun)
	case "批量启用或停用":
		return fmt.Sprintf("批量启用或停用%s", noun)
	case "批量删除":
		return fmt.Sprintf("批量删除%s", noun)
	case "新增":
		return fmt.Sprintf("新增%s", noun)
	case "修改":
		return fmt.Sprintf("修改%s", noun)
	}
	return fmt.Sprintf("删除%s", noun)
}

func (m docModel) buildOpenAPI() map[string]any {
	paths := map[string]any{}
	for _, e := range m.Endpoints {
		item, _ := paths[e.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[e.Path] = item
		}
		item[strings.ToLower(e.Method)] = m.operation(e)
	}
	var tags []map[string]any
	for _, t := range m.Tags {
		tags = append(tags, map[string]any{"name": t})
	}
	schemas := map[string]any{"Error": map[string]any{
		"type":       "object",
		"properties": map[string]any{"error": map[string]any{"type": "string", "description": "错误说明"}},
	}}
	for k, v := range m.Schemas {
		schemas[k] = v
	}
	var scopes []string
	for _, d := range m.Domains {
		scopes = append(scopes, d.ID)
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Gately 管理 API",
			"version":     version.Version,
			"description": apiIntro,
		},
		"externalDocs": map[string]any{"description": "Gately 官网", "url": websiteURL},
		"servers": []map[string]any{{
			"url":         "http://localhost:16680",
			"description": "默认管理地址，请替换为你的 Gately 地址",
		}},
		"tags":  tags,
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type": "http", "scheme": "bearer",
					"description": "在“API 令牌”页创建令牌，请求头携带 Authorization: Bearer gtl_xxxx",
				},
			},
			"schemas": schemas,
		},
		"x-scope-domains": scopes,
	}
}

func (m docModel) operation(e endpoint) map[string]any {
	op := map[string]any{"tags": []string{e.Tag}, "summary": e.Summary}
	desc := e.Desc
	switch e.Scope {
	case apitoken.Denied:
		desc += "\n\n" + "**仅登录会话可用，API 令牌无法访问此接口。**"
	case apitoken.Any:
		desc += "\n\n" + "任何有效的 API 令牌均可访问，无需特定权限。"
	default:
		desc += "\n\n" + fmt.Sprintf("所需权限：`%s`。", e.Scope)
	}
	op["description"] = strings.TrimSpace(desc)
	op["x-required-scope"] = e.Scope
	op["x-token-allowed"] = e.Scope != apitoken.Denied
	if e.Scope != apitoken.Denied {
		op["security"] = []map[string][]string{{"bearerAuth": {}}}
	}
	var params []map[string]any
	for _, mt := range pathParamRe.FindAllStringSubmatch(e.Path, -1) {
		params = append(params, map[string]any{"name": mt[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
	}
	for _, q := range e.Query {
		// 序号与条数为整数，过滤参数为字符串
		typ := "string"
		if q == "since" || q == "before" || q == "limit" {
			typ = "integer"
		}
		params = append(params, map[string]any{"name": q, "in": "query", "schema": map[string]any{"type": typ},
			"description": logQueryDesc[q]})
	}
	if len(params) > 0 {
		op["parameters"] = params
	}
	if e.Req != nil || e.ReqFree {
		body := e.Req
		if body == nil {
			body = map[string]any{"type": "object"}
		}
		op["requestBody"] = map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": body}}}
	}
	ok := map[string]any{"description": "成功"}
	if e.Resp != nil {
		ok["content"] = map[string]any{"application/json": map[string]any{"schema": e.Resp}}
	}
	errResp := func(desc string) map[string]any {
		return map[string]any{"description": desc, "content": map[string]any{"application/json": map[string]any{
			"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
	}
	responses := map[string]any{"200": ok, "400": errResp("请求参数错误"), "401": errResp("令牌无效、已过期或未登录")}
	if e.Scope != apitoken.Any {
		responses["403"] = errResp("令牌缺少所需权限")
	}
	op["responses"] = responses
	return op
}

// logQueryDesc 是日志接口查询参数的说明。
var logQueryDesc = map[string]string{
	"since":      "增量拉取：只返回序号大于该值的日志",
	"before":     "加载更早的日志：只返回序号小于该值的日志",
	"limit":      "最多返回的条数，取符合条件的最新几条；0 或不传表示不限",
	"level":      "最低级别：debug、info、warn 或 error",
	"source":     "来源标签，不区分大小写精确匹配",
	"q":          "关键词：空格分隔表示同时包含，双引号包住短语，前缀 - 表示排除",
	"exclude":    "不包含的关键词：空格分隔，双引号包住短语，包含其中任意一个即排除；与 q 同时生效",
	"ip":         "客户端地址：完整地址、CIDR 或地址片段，多个以逗号分隔，前缀 ! 或 - 表示排除",
	"host":       "站点名称或域名的一部分",
	"status":     "状态码，如 404、4xx、500-599，多个以逗号分隔",
	"hideBanned": "为 1 时隐藏来源已被封禁而被拒绝的访问日志（只影响返回结果，日志照常保存）",
	"hideAdmin":  "为 1 时隐藏带管理后台登录态的请求，即管理员经 Web 服务站点访问后台产生的访问日志",
	"from":       "开始时间，RFC 3339 格式或毫秒时间戳",
	"to":         "结束时间，RFC 3339 格式或毫秒时间戳",
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.buildDoc().buildOpenAPI())
}

// APIDocs 返回指定语言的 OpenAPI JSON 与 Markdown 文档，供命令行导出静态文件；lang 为空或 zh-CN 时输出中文原文。
func (a *App) APIDocs(lang string) (openapi []byte, markdown []byte, err error) {
	m := a.buildDoc()
	raw, err := json.Marshal(m.buildOpenAPI())
	if err != nil {
		return nil, nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, nil, err
	}
	// 说明文字按语言翻译；翻译表里没有的内容保持中文原文
	v = trDocDeep(lang, v)
	openapi, err = json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return append(openapi, '\n'), []byte(m.markdown(func(x string) string { return i18n.Tr(lang, x) })), nil
}

// trDocDeep 翻译文档中的字符串；接口说明由多个段落（以空行分隔）拼成，整体没有译文时按段落翻译。
// 多段说明先只做整句匹配：Tr 整体匹配不上时会做片段替换，结果与原文不同，就不会再按段落精确翻译，
// 留下中外文混排的句子。
func trDocDeep(lang string, v any) any {
	switch x := v.(type) {
	case string:
		if !strings.Contains(x, "\n\n") {
			return i18n.Tr(lang, x)
		}
		if out := i18n.TrExact(lang, x); out != x {
			return out
		}
		parts := strings.Split(x, "\n\n")
		for i := range parts {
			// 由多个字符串常量拼成的说明（如 apiIntro），译文表的键是各常量原文，带结尾的空行
			if out := i18n.TrExact(lang, parts[i]+"\n\n"); out != parts[i]+"\n\n" {
				parts[i] = strings.TrimSuffix(out, "\n\n")
				continue
			}
			parts[i] = i18n.Tr(lang, parts[i])
		}
		return strings.Join(parts, "\n\n")
	case []any:
		for i := range x {
			x[i] = trDocDeep(lang, x[i])
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = trDocDeep(lang, e)
		}
		return x
	}
	return v
}
