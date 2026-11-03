# 参与贡献

Gately 是社区驱动的开源项目，欢迎提交 Issue 与 Pull Request。仓库：<https://github.com/wangwanjie/gately>

## 本地开发

依赖：Go 1.26+、Node.js 24+、pnpm。

```bash
make web      # 构建前端（输出到 web/dist，由 go:embed 嵌入）
make build    # 编译 bin/gately
make test     # go test -race ./...
make lint     # gofmt、go vet、前端类型检查
```

前端开发时运行 `cd web && pnpm dev`，`/api` 会代理到本地 `127.0.0.1:16680`。

## 贡献约定

- 兼容性代码只能基于公开可观察的行为、接口和配置格式，请勿提交从第三方二进制或前端产物中复制的代码。
- 新模块需要：配置结构与校验、运行时启停、REST 接口、单元测试；能从 Lucky 迁移的模块实现 `luckyimport.Importer`，希望在总览页展示状态的模块实现 `module.Summarizer`。
- 可选模块在 `internal/modules/registry/mod_<模块>.go` 中注册，文件带 `//go:build !no_<模块>`，同时登记到 `catalog.go` 与根目录的 `modules.json`（`TestManifestMatchesCode` 会检查三者一致）；模块之间的协作（站点的 WAF、证书部署到 SSH 与 Docker 等）通过注入的函数提供，缺少对方模块时返回「未包含在此构建中」，核心代码不能直接引用可选模块的包。CI 会用三个预设的 tag 组合执行 vet 与编译。
- 代码注释使用中文；控制语句必须使用大括号。

## 额外测试

部分测试默认跳过，需要额外条件：

| 测试 | 启用方式 |
| --- | --- |
| ACME 端到端（Pebble） | 安装 pebble 与 pebble-challtestsrv，设置 `GATELY_PEBBLE_BIN`、`GATELY_PEBBLE_SRC` |
| 真实公共 STUN 服务器 | `GATELY_NET_TEST=1` |
| 用真实 Lucky 配置验证导入 | `GATELY_LUCKY_SAMPLE` 等，见各模块 `TestImportRealSample` |

## 许可证

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## 多语言

- 管理界面文案位于 `web/src/locales/<语言>/<命名空间>.json`，源语言为简体中文，新增或修改文案时至少同步更新 `zh-CN` 与 `en`，提交前运行 `cd web && node scripts/check-i18n.mjs`。约定详见 `web/src/locales/README.md`。
- 官网语言包位于 `website/.vitepress/i18n/<语言>.json`，各语言页面位于 `website/<语言目录>/`。
- 后端运行时文案（错误说明、日志、通知、接口文档）以源码里的中文字面量为键，译文在 `internal/i18n/locales/<语言>.json`。新增或修改中文文案后运行 `go run ./internal/i18n/cmd/i18nsync -root .` 把新键同步进各语言文件并补全译文，`go test ./internal/i18n` 会检查所有语言是否完整、格式动词是否与原文一致。需要拼接的句子请使用 `fmt.Sprintf` 的整句格式串，不要拼接多段中文，否则无法按各语言语序翻译。
- 接口文档由程序按各语言生成：`make api-docs` 输出到 `docs/api/<语言>/`，无需手工同步。
- 当前支持：简体中文、English、繁體中文、日本語、한국어、Español、Français、Deutsch、Русский、Português (Brasil)。
