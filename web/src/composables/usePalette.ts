// 全局搜索（命令面板）的共享状态：Layout 渲染面板，侧边栏搜索框、手机顶栏图标与快捷键都通过这里打开。
import { ref } from 'vue'

// paletteOpen 为命令面板是否打开
export const paletteOpen = ref(false)

// pageKey 作为主内容区页面组件的 key：从命令面板跳到当前所在页面并带新参数时加一，让只在挂载时读取参数的页面重新挂载
export const pageKey = ref(0)

export function bumpPageKey() {
  pageKey.value++
}

// isPaletteShortcut 判断按键是否为打开命令面板的快捷键：⌘K（macOS）或 Ctrl+K。
// Web 终端里的 Ctrl+K 是 Shell 的删除到行尾，焦点在终端内时只响应 ⌘K
export function isPaletteShortcut(e: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey'>, inTerminal: boolean): boolean {
  if (e.key.toLowerCase() !== 'k' || e.altKey || e.shiftKey) {
    return false
  }
  if (e.metaKey) {
    return true
  }
  return e.ctrlKey && !inTerminal
}
