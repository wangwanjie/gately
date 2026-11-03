//go:build !lang_zh && !lang_cjken

package i18n

import "embed"

// 默认构建包含全部语言的译文。
//
//go:embed locales/*.json.gz
var localeFS embed.FS

var builtLangs = AllLangs
