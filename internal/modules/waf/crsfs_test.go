package waf

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
)

// 内嵌文件系统只认正斜杠：Windows 上 filepath.Join 产生的反斜杠路径必须能被解析。
func TestCRSFSAcceptsBackslash(t *testing.T) {
	const name = `@owasp_crs\REQUEST-901-INITIALIZATION.conf`
	if _, err := fs.ReadFile(coreruleset.FS, name); err == nil {
		t.Skip("底层文件系统已接受反斜杠，无需包装")
	}
	data, err := fs.ReadFile(crsFS, name)
	if err != nil {
		t.Fatalf("反斜杠路径读取失败: %v", err)
	}
	if !strings.Contains(string(data), "901") {
		t.Fatalf("读到的内容不是 REQUEST-901 初始化文件")
	}
}

func TestSlashFSNormalizes(t *testing.T) {
	m := fstest.MapFS{"a/b.conf": {Data: []byte("x")}}
	s := slashFS{base: m}
	if b, err := fs.ReadFile(s, `a\b.conf`); err != nil || string(b) != "x" {
		t.Fatalf("got %q %v", b, err)
	}
	if b, err := fs.ReadFile(s, "a/b.conf"); err != nil || string(b) != "x" {
		t.Fatalf("正斜杠路径应保持可用: %q %v", b, err)
	}
}
