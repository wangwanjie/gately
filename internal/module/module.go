// Package module 定义功能模块的统一接口。
//
// 每个模块拥有独立的配置文件（config.Store 中以 ID 命名）、独立的日志缓冲，
// 以及挂载在 /api/<ID>/ 下的管理接口。
package module

import (
	"context"
	"net/http"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// Env 是框架注入给模块的运行环境。
type Env struct {
	Store *config.Store
	Logs  *logbuf.Buffer
}

// HTTPClientAware 由需要访问外网（例如下载托管程序）的模块实现。
// 框架注入的函数每次调用都返回遵循“更新代理”设置的客户端，代理修改后立即生效。
type HTTPClientAware interface {
	SetHTTPClient(fn func() *http.Client)
}

// Router 用于注册模块接口。pattern 采用 Go 1.22 ServeMux 语法，
// 但路径部分相对模块前缀，例如 "GET /rules" 实际挂载为 "GET /api/<ID>/rules"。
type Router interface {
	HandleFunc(pattern string, h http.HandlerFunc)
}

// Module 是所有功能模块需要实现的接口。
type Module interface {
	// ID 是模块唯一标识，同时作为配置文件名和 API 前缀
	ID() string
	// Title 是界面显示名称
	Title() string
	// Init 加载配置，不启动任何网络服务
	Init(env Env) error
	// Start 按配置启动服务，ctx 在程序退出时取消
	Start(ctx context.Context) error
	// Stop 停止全部服务并释放端口
	Stop() error
	// Routes 注册管理接口
	Routes(r Router)
}

// Migrator 由需要在全部模块初始化之前迁移配置的模块实现，例如从其他模块的配置文件中接收拆分出来的规则。
// 返回的 from 非空表示本次有配置从模块 from 迁来：已有 from 权限的 API 令牌会同时获得本模块的同级权限，
// from 被关闭时本模块也一并关闭，保持迁移前的行为。没有需要迁移的内容时返回空串。
type Migrator interface {
	Migrate(store *config.Store) (from string, err error)
}

// SummaryItem 是总览页展示的一项指标。Level 为 ok / warn / error / info，用于着色。
type SummaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Level string `json:"level,omitempty"`
}

// Summarizer 由希望在总览页展示状态的模块实现。
type Summarizer interface {
	Summary() []SummaryItem
}

// Doc 描述一个接口的文档信息，Req / Resp 为示例值（通常是结构体指针或切片），用于生成数据结构说明。
type Doc struct {
	Summary string
	Req     any
	Resp    any
}

// DocRouter 由支持接口文档的 Router 实现。模块可通过类型断言为接口补充文档信息，
// pattern 与 HandleFunc 的写法一致（相对模块前缀）。
type DocRouter interface {
	Doc(pattern string, d Doc)
}

// DocOf 在 Router 支持文档时登记接口文档，否则忽略。
func DocOf(r Router, pattern string, d Doc) {
	if dr, ok := r.(DocRouter); ok {
		dr.Doc(pattern, d)
	}
}
