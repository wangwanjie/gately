package fsperm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// 受保护目录
//
// Gately 的配置目录里有管理员密码哈希、会话签名密钥、API 令牌、证书私钥和各模块的密钥。
// 把服务器目录提供出去的功能（存储挂载、静态站点、WebDAV、FTP、SMB）一旦把它包含进去，
// 能使用这些功能的人就能读到会话签名密钥并签出管理员会话。因此这些功能在保存配置时
// 用 CheckExposable 拒绝与配置目录重叠的目录，在实际提供服务前再用 Protected 判断一次
// （升级前已保存的配置不经过保存时的校验）。

var (
	protectMu sync.RWMutex
	protected []string
)

// Protect 把 dir 登记为受保护目录；相对路径按当前工作目录转为绝对路径，重复登记只保留一份。
// 配置存储在打开时自动登记自己的目录（见 config.Open）。
func Protect(dir string) {
	if dir == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	protectMu.Lock()
	defer protectMu.Unlock()
	if !slices.Contains(protected, abs) {
		protected = append(protected, abs)
	}
}

// Protected 返回与 path 重叠的受保护目录，没有时返回空串。
func Protected(path string) string {
	protectMu.RLock()
	dirs := slices.Clone(protected)
	protectMu.RUnlock()
	for _, dir := range dirs {
		if Overlap(dir, path) {
			return dir
		}
	}
	return ""
}

// CheckExposable 供保存配置时校验将要对外提供的目录：path 与受保护目录重叠时返回错误。
func CheckExposable(path string) error {
	if Protected(path) != "" {
		return fmt.Errorf("目录 %s 不能使用：它是 Gately 的配置目录、位于配置目录内或包含配置目录，配置目录保存着登录密钥", path)
	}
	return nil
}

// Overlap 判断两个目录是否重叠：相同，或其中一个位于另一个之内。
// 依次按规范化后的路径、解析符号链接后的路径、文件系统中的实际对象比较，任一种判定为重叠即返回 true：
// 指向配置目录的符号链接、大小写不同的写法（Windows 与 macOS 默认不区分大小写）、同一目录的另一个挂载位置都算重叠。
func Overlap(a, b string) bool {
	a, b = absClean(a), absClean(b)
	if a == "" || b == "" {
		return false
	}
	if lexicalOverlap(a, b) || lexicalOverlap(resolve(a), resolve(b)) {
		return true
	}
	return sameOnPath(a, b) || sameOnPath(b, a)
}

// absClean 返回规范化的绝对路径，无法转换时返回空串。
func absClean(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	return abs
}

// foldCase 在文件名不区分大小写的系统上把路径转为小写再比较。
func foldCase(p string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

// lexicalOverlap 只按路径文本判断：相同或一方是另一方的上级目录。
func lexicalOverlap(a, b string) bool {
	a, b = foldCase(a), foldCase(b)
	return within(a, b) || within(b, a)
}

// within 判断 child 是否等于 parent 或位于 parent 之内，两者须已规范化。
func within(parent, child string) bool {
	if parent == child {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(parent, sep) {
		parent += sep
	}
	return strings.HasPrefix(child, parent)
}

// resolve 解析路径中的符号链接；路径的末尾几级还不存在时，解析存在的那部分上级目录再拼回剩余部分。
func resolve(p string) string {
	rest := ""
	for cur := p; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// sameOnPath 判断 dir 是否与 target 或 target 的某一级上级目录是同一个文件系统对象，即 target 位于 dir 之内。
func sameOnPath(dir, target string) bool {
	want, err := os.Stat(dir)
	if err != nil {
		return false
	}
	for cur := target; ; {
		if st, err := os.Stat(cur); err == nil && os.SameFile(st, want) {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}
