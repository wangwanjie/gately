//go:build linux || darwin

package parent

// 本文件为 Gately 新增，不属于上游 gosamba：按 config.ShareConfig.Guard 屏蔽共享目录中的一部分。
// Gately 用它隐藏共享目录中的 Gately 配置目录（例如整个数据盘被共享、Gately 的数据目录就在其中时）：
// 打开、创建、作为改名目标按不存在处理，列目录与变更通知中不出现，改名或删除它及其上级目录被拒绝。

// shareHidden 判断共享中的本机路径 p 是否被屏蔽。
func shareHidden(tree *Tree, p string) bool {
	return tree != nil && tree.Share.Guard != nil && tree.Share.Guard.Hidden(p)
}

// shareTouches 判断改名或删除本机路径 p 是否会动到被屏蔽的部分。
func shareTouches(tree *Tree, p string) bool {
	return tree != nil && tree.Share.Guard != nil && tree.Share.Guard.Touches(p)
}
