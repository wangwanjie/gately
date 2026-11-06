//go:build !unix

package logbuf

// oNoFollow 在不支持 O_NOFOLLOW 的平台上为 0；创建临时文件仍用 O_EXCL，不会跟随已存在的符号链接
const oNoFollow = 0
