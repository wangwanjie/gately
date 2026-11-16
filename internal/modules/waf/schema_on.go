//go:build !coraza.disabled_operators.validateSchema

package waf

// validateSchemaAvailable 表示本次构建是否包含 Coraza 的 @validateSchema 操作符。
// 发布构建带 coraza.disabled_operators.validateSchema 标签去掉它（及其依赖的 JSON Schema、YAML 库，约 2.3 MB），
// 见 schema_off.go；CRS 与内置规则都不使用它。
const validateSchemaAvailable = true
