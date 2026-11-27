package waf

import (
	"io/fs"
	"strings"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
)

// slashFS 把路径里的反斜杠换成正斜杠后再访问底层文件系统。
// Coraza 解析 Include 时对相对路径调用 filepath.Join，Windows 上会把
// "@owasp_crs/REQUEST-901-INITIALIZATION.conf" 变成 "@owasp_crs\REQUEST-901-INITIALIZATION.conf"，
// 而内嵌文件系统（embed.FS）只认正斜杠，结果报 file does not exist。
type slashFS struct {
	base fs.FS
}

// Open 实现 fs.FS，先规范化分隔符。
func (s slashFS) Open(name string) (fs.File, error) {
	return s.base.Open(strings.ReplaceAll(name, `\`, "/"))
}

// crsFS 是编译 WAF 实例时使用的根文件系统：内置 CRS，并兼容 Windows 路径分隔符。
var crsFS fs.FS = slashFS{base: coreruleset.FS}
