package fsperm

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Guard 屏蔽共享目录中的受保护目录：共享目录包含配置目录时（例如整个数据盘被共享），
// 配置目录这棵子树对文件服务不可见、不可读写，其余文件照常提供。
//
// 各文件服务在每次访问前用 Hidden 判断目标路径，在改名、删除、修改权限前用 Touches 判断源与目标，
// 列目录时用 HiddenChild 过滤条目。判断方式与 Overlap 相同：规范化路径、解析符号链接后的路径、
// 文件系统中的实际对象，任一种命中即屏蔽，指向配置目录的符号链接、大小写不同的写法都无法绕过。
//
// 共享目录不包含任何受保护目录时 NewGuard 返回 nil，nil 上的方法一律返回 false，调用方无需判空。
type Guard struct {
	// dirs 为共享目录中的受保护目录（绝对路径），resolved 为解析符号链接后的路径
	dirs     []string
	resolved []string
	// stats 为受保护目录的文件信息，列目录时据此快速判断条目是否就是受保护目录；取不到时为 nil
	stats []os.FileInfo
	// root 为共享目录（规范化的绝对路径）；rootClear 表示构造时已确认共享目录本身及其各级上级都不是受保护目录，
	// 按文件对象比较共享目录中的路径时，只需检查共享目录以下的各级
	root      string
	rootClear bool
}

// NewGuard 返回共享目录 root 的屏蔽规则；root 不包含受保护目录时返回 nil。
// root 本身就是受保护目录或位于其中时同样返回规则（所有路径都被屏蔽），作为保存校验之外的兜底。
func NewGuard(root string) *Guard {
	g := Guard{root: absClean(root), rootClear: true}
	for _, dir := range protectedDirs() {
		rootInside := insideOf(dir, root)
		if !insideOf(root, dir) && !rootInside {
			continue
		}
		if rootInside {
			g.rootClear = false
		}
		abs := absClean(dir)
		g.dirs = append(g.dirs, abs)
		g.resolved = append(g.resolved, resolve(abs))
		st, err := os.Stat(abs)
		if err != nil {
			st = nil
		}
		g.stats = append(g.stats, st)
	}
	if len(g.dirs) == 0 {
		return nil
	}
	return &g
}

// Hidden 判断绝对路径 p 是否为受保护目录或位于其中：读取、列出、打开、创建都应拒绝，并按不存在处理。
//
// 判断规则与 insideOf 相同（规范化路径、解析符号链接后的路径、文件系统中的实际对象），但文件服务每个操作都要调用，
// 这里省掉每次重复的工作：受保护目录的解析结果与文件信息用构造时算好的，p 只解析一次，
// 逐级比较文件对象时只查共享目录以下的各级（共享目录及其上级在构造时已确认不是受保护目录）。
func (g *Guard) Hidden(p string) bool {
	if g == nil {
		return false
	}
	ap := absClean(p)
	if ap == "" {
		return false
	}
	for _, dir := range g.dirs {
		if lexicalWithin(dir, ap) {
			return true
		}
	}
	rp := resolve(ap)
	for _, dir := range g.resolved {
		if lexicalWithin(dir, rp) {
			return true
		}
	}
	return g.sameOnPath(ap)
}

// sameOnPath 判断 p（规范化的绝对路径）或它的某一级上级目录是否与某个受保护目录是同一个文件系统对象，
// 用于识别同一目录的另一个挂载位置（如 bind mount）。
func (g *Guard) sameOnPath(p string) bool {
	wants := make([]os.FileInfo, 0, len(g.stats))
	for i, st := range g.stats {
		if st == nil {
			// 构造时受保护目录还不存在，按当前状态再取一次
			st, _ = os.Stat(g.dirs[i])
		}
		if st != nil {
			wants = append(wants, st)
		}
	}
	if len(wants) == 0 {
		return false
	}
	stop := ""
	if g.rootClear && g.root != "" && within(g.root, p) {
		stop = g.root
	}
	for cur := p; cur != stop; {
		if st, err := os.Stat(cur); err == nil {
			for _, w := range wants {
				if os.SameFile(st, w) {
					return true
				}
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
	return false
}

// Touches 判断绝对路径 p 是否与受保护目录重叠（是它、位于其中或包含它）：改名、删除、修改权限应拒绝，
// 否则移动或删除一个上级目录就会把配置目录一起带走。
func (g *Guard) Touches(p string) bool {
	if g == nil {
		return false
	}
	if g.Hidden(p) {
		return true
	}
	for _, dir := range g.dirs {
		if insideOf(p, dir) {
			return true
		}
	}
	return false
}

// HiddenChild 判断目录 dir（绝对路径，本身未被屏蔽）中由 fi 描述的条目是否应从列表中隐藏。
// fi 为条目自身的信息（Lstat）。普通文件不可能是受保护目录；目录先按文件系统对象与路径比较，
// 不必逐项解析整条路径；符号链接按完整规则判断它指向哪里。
func (g *Guard) HiddenChild(dir string, fi fs.FileInfo) bool {
	if g == nil || fi == nil {
		return false
	}
	child := filepath.Join(dir, fi.Name())
	if fi.Mode()&fs.ModeSymlink != 0 {
		return g.Hidden(child)
	}
	if !fi.IsDir() {
		return false
	}
	for i, d := range g.dirs {
		if st := g.stats[i]; st != nil && os.SameFile(fi, st) {
			return true
		}
		if foldCase(absClean(child)) == foldCase(d) || foldCase(absClean(child)) == foldCase(g.resolved[i]) {
			return true
		}
	}
	return false
}

// FilterInfos 返回去掉被隐藏条目后的列表，dir 为这些条目所在的目录（绝对路径）。
func (g *Guard) FilterInfos(dir string, infos []fs.FileInfo) []fs.FileInfo {
	if g == nil {
		return infos
	}
	out := infos[:0]
	for _, fi := range infos {
		if !g.HiddenChild(dir, fi) {
			out = append(out, fi)
		}
	}
	return out
}

// FilterEntries 与 FilterInfos 相同，用于 os.ReadDir 的结果；只对目录与符号链接读取条目信息。
func (g *Guard) FilterEntries(dir string, entries []fs.DirEntry) []fs.DirEntry {
	if g == nil {
		return entries
	}
	out := entries[:0]
	for _, e := range entries {
		if e.IsDir() || e.Type()&fs.ModeSymlink != 0 {
			if fi, err := e.Info(); err == nil && g.HiddenChild(dir, fi) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}
