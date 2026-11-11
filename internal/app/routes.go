package app

import (
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/module"
)

// routeInfo 是登记过的一条受保护接口。
type routeInfo struct {
	// Method / Path 例如 GET、/api/ddns/tasks/{key}
	Method string
	Path   string
	// Module 为所属模块 ID，应用自身接口为空
	Module string
	// Public 表示模块的免登录接口（/api/<ID>/public 下），凭模块自己的凭据访问，只用于生成文档
	Public bool
}

// routeRecorder 在向 mux 注册接口的同时记录路由表和文档信息。
type routeRecorder struct {
	mux    *http.ServeMux
	routes []routeInfo
	docs   map[string]module.Doc
}

func (r *routeRecorder) add(pattern string, h http.HandlerFunc, mod string) {
	r.mux.HandleFunc(pattern, h)
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		return
	}
	r.routes = append(r.routes, routeInfo{Method: method, Path: path, Module: mod})
}

// addPublic 登记一条免登录接口供文档使用；接口本身由 publicRouter 直接挂到外层 mux。
func (r *routeRecorder) addPublic(pattern, mod string) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		return
	}
	r.routes = append(r.routes, routeInfo{Method: method, Path: path, Module: mod, Public: true})
}
