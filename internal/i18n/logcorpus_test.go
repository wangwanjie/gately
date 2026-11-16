package i18n

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// logFormat 是源码里一处日志调用的格式串。
type logFormat struct {
	format string
	// level 为调用的方法对应的级别（debug / info / warn / error）
	level string
}

// repoFormats 扫描仓库的非测试源码，返回日志调用（Logger 的 Debugf / Infof / Warnf / Errorf）用到的全部格式串，
// 以及 fmt.Errorf / errors.New 里含中文的文案（日志参数里常见的内层错误）。
func repoFormats(t testing.TB) (logs []logFormat, inner []string) {
	t.Helper()
	levels := map[string]string{"Debugf": "debug", "Infof": "info", "Warnf": "warn", "Errorf": "error"}
	seenLog, seenInner := map[string]bool{}, map[string]bool{}
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if filepath.Base(p) == "i18n" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				pkg, _ := sel.X.(*ast.Ident)
				switch {
				case pkg != nil && (pkg.Name == "fmt" && sel.Sel.Name == "Errorf" || pkg.Name == "errors" && sel.Sel.Name == "New"):
					if refHasHan(s) && !seenInner[s] {
						seenInner[s] = true
						inner = append(inner, s)
					}
				case levels[sel.Sel.Name] != "" && (pkg == nil || pkg.Name != "fmt"):
					if !seenLog[s] {
						seenLog[s] = true
						logs = append(logs, logFormat{format: s, level: levels[sel.Sel.Name]})
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].format < logs[j].format })
	sort.Strings(inner)
	return logs, inner
}

// sampleArgs 为格式串的每个动词造一个参数。han 为 false 时全部是地址、域名、英文错误这类不含中文的值；
// 为 true 时 %s / %v / %q 换成含中文的值：用户起的名称，以及由 inner 中的文案生成的内层错误。
// seed 让不同格式串取到不同的样本。
func sampleArgs(format string, han bool, inner []string, seed int) []any {
	ascii := []string{"192.0.2.7", "example.com", "nas-01", "[2001:db8::1]:8443", "/etc/gately/site.conf", "GET"}
	names := []string{"家里的NAS", "客厅电视", "备份任务", "上海电信"}
	var args []any
	for i, v := range Verbs(format) {
		k := seed + i
		switch v[len(v)-1] {
		case 'd', 'x', 'X', 'o', 'b', 'c', 'U':
			args = append(args, 8080+k%7)
		case 'f', 'F', 'g', 'G', 'e', 'E':
			args = append(args, 12.5+float64(k%3))
		case 't':
			args = append(args, k%2 == 0)
		case 'v', 'w':
			switch {
			case !han:
				args = append(args, errors.New("dial tcp 10.0.0.2:443: connect: connection refused"))
			case len(inner) > 0:
				args = append(args, sampleError(inner[k%len(inner)], inner, k))
			default:
				args = append(args, errors.New(names[k%len(names)]))
			}
		default:
			if han {
				args = append(args, names[k%len(names)])
			} else {
				args = append(args, ascii[k%len(ascii)])
			}
		}
	}
	return args
}

// sampleError 用一条内层错误文案生成错误值，文案自带动词时填入不含中文的参数。
func sampleError(format string, inner []string, seed int) error {
	args := sampleArgs(format, false, nil, seed)
	return errors.New(fmt.Sprintf(strings.ReplaceAll(format, "%w", "%v"), args...))
}
