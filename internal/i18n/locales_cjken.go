//go:build lang_cjken && !lang_zh

package i18n

import "embed"

// lang_cjken 档位只包含简体中文（源码原文，无需译文）、繁体中文与英语。
//
//go:embed locales/en.json.gz locales/zh-TW.json.gz
var localeFS embed.FS

var builtLangs = []string{"zh-CN", "en", "zh-TW"}
