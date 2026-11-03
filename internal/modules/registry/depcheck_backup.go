//go:build no_storage && !no_backup

package registry

// 配置备份模块依赖存储挂载模块：使用 no_storage 构建时必须同时带上 no_backup。
// 这里引用一个不存在的标识符，让错误的组合在编译期失败，报错信息直接说明原因。
var _ = backupModuleRequiresStorageModule_addNoBackupTag
