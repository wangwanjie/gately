import { ref } from 'vue'

// 终端剪贴板辅助：写入复用 useClipboard 的实现（含 HTTP 下的降级），读取在非安全上下文下不可用。
export { writeClipboard as copyText } from '../../composables/useClipboard'

// readClipboardText 读取剪贴板文本，不可用时返回 null（由调用方改用手动粘贴框）。
export async function readClipboardText(): Promise<string | null> {
  if (navigator.clipboard && navigator.clipboard.readText && window.isSecureContext) {
    try {
      return await navigator.clipboard.readText()
    } catch {
      return null
    }
  }
  return null
}

export { sanitizePaste } from './pasteText'

// ---- 最近复制记录 ----
// HTTP 访问时读不到系统剪贴板，在 Gately 里复制或粘贴过的内容记在本标签页里，粘贴面板中可一键重发。
// 内容可能含敏感信息，只存 sessionStorage，关闭标签页即清除。
const CLIPS_KEY = 'gately-term-clips'
const MAX_CLIPS = 10
// 过长的内容不记录，避免撑满存储
const MAX_CLIP_LEN = 64 * 1024

function loadClips(): string[] {
  try {
    const v = JSON.parse(sessionStorage.getItem(CLIPS_KEY) ?? '[]')
    return Array.isArray(v) ? v.filter((x) => typeof x === 'string').slice(0, MAX_CLIPS) : []
  } catch {
    return []
  }
}

export const recentClips = ref<string[]>(loadClips())

function saveClips() {
  try {
    sessionStorage.setItem(CLIPS_KEY, JSON.stringify(recentClips.value))
  } catch {
    // 存储不可用（如隐私模式配额为 0）时只保留在内存中
  }
}

// recordClip 把文本放到记录最前面，重复内容只保留一条。
export function recordClip(text: string) {
  if (!text.trim() || text.length > MAX_CLIP_LEN) {
    return
  }
  recentClips.value = [text, ...recentClips.value.filter((x) => x !== text)].slice(0, MAX_CLIPS)
  saveClips()
}

export function removeClip(text: string) {
  recentClips.value = recentClips.value.filter((x) => x !== text)
  saveClips()
}

export function clearClips() {
  recentClips.value = []
  saveClips()
}
