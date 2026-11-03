//go:build lang_zh

package i18n

import "embed"

// lang_zh 档位只包含简体中文：源码文案就是简体中文，不嵌入任何译文。
var localeFS embed.FS

var builtLangs = []string{"zh-CN"}
