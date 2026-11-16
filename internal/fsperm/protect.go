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
// 把服务器目录提供出去的功能（存储挂载、静态站点、WebDAV、FTP、SMB）不能让人读到它，否则能使用这些功能的人
// 就能读到会话签名密钥并签出管理员会话。按目录与配置目录的关系分两种情况处理：
//   - 目录就是配置目录或位于其中：保存时用 CheckExposable 拒绝；升级前已保存的，提供服务前用 Inside 判断并停止提供。
//   - 目录包含配置目录（例如把 NAS 的整个数据盘共享出去，Gately 的数据目录就在其中）：照常提供，
//     但配置目录这棵子树对该服务不可见、不可读写，由各服务在文件访问层经 Guard 屏蔽。
// File Browser 等把目录直接交给外部程序的功能无法屏蔽子树，对“包含”同样拒绝，用 Protected 判断。

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

// Protected 返回与 path 重叠（相同、位于其中或包含它）的受保护目录，没有时返回空串。
func Protected(path string) string {
	for _, dir := range protectedDirs() {
		if Overlap(dir, path) {
			return dir
		}
	}
	return ""
}

// Inside 返回等于 path 或包含 path 的受保护目录（即 path 就是受保护目录或位于其中），没有时返回空串。
func Inside(path string) string {
	for _, dir := range protectedDirs() {
		if insideOf(dir, path) {
			return dir
		}
	}
	return ""
}

// Contains 返回位于 path 之内（不等于 path）的受保护目录，没有时返回空串：path 作为共享目录时，这棵子树需要屏蔽。
func Contains(path string) string {
	for _, dir := range protectedDirs() {
		if insideOf(path, dir) && !insideOf(dir, path) {
			return dir
		}
	}
	return ""
}

func protectedDirs() []string {
	protectMu.RLock()
	defer protectMu.RUnlock()
	return slices.Clone(protected)
}

// CheckExposable 供保存配置时校验将要对外提供的目录：path 就是受保护目录或位于其中时返回错误。
// 包含受保护目录的 path 可以使用，提供服务时由 Guard 屏蔽其中的受保护目录。
func CheckExposable(path string) error {
	if Inside(path) != "" {
		return fmt.Errorf("目录 %s 不能使用：它是 Gately 的配置目录或位于配置目录内，配置目录保存着登录密钥", path)
	}
	return nil
}

// CheckWholeDir 供把目录整个交给外部程序的功能（File Browser）校验目录：与受保护目录重叠（含包含关系）时返回错误，
// 这类功能无法屏蔽目录中的一部分。
func CheckWholeDir(path string) error {
	if dir := Protected(path); dir != "" {
		return fmt.Errorf("目录 %s 不能使用：它与 Gately 的配置目录 %s 重叠，这里的目录由外部程序直接读取，无法隐藏其中的配置目录，请改用不包含配置目录的目录", path, dir)
	}
	return nil
}

// Overlap 判断两个目录是否重叠：相同，或其中一个位于另一个之内。
// 依次按规范化后的路径、解析符号链接后的路径、文件系统中的实际对象比较，任一种判定为重叠即返回 true：
// 指向配置目录的符号链接、大小写不同的写法（Windows 与 macOS 默认不区分大小写）、同一目录的另一个挂载位置都算重叠。
func Overlap(a, b string) bool {
	return insideOf(a, b) || insideOf(b, a)
}

// insideOf 判断 p 是否等于 dir 或位于 dir 之内，比较方式同 Overlap。
func insideOf(dir, p string) bool {
	dir, p = absClean(dir), absClean(p)
	if dir == "" || p == "" {
		return false
	}
	if lexicalWithin(dir, p) || lexicalWithin(resolve(dir), resolve(p)) {
		return true
	}
	return sameOnPath(dir, p)
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

// lexicalWithin 只按路径文本判断 child 是否等于 parent 或位于其中。
func lexicalWithin(parent, child string) bool {
	return within(foldCase(parent), foldCase(child))
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
