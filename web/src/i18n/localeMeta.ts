// 界面语言清单与按浏览器语言挑选语言的纯函数。
// 不依赖构建期虚拟模块，vite.config.ts 与测试也会引用它（生成首屏语言包预加载脚本时复用同一份匹配规则）。

export interface LocaleMeta {
  code: string
  // 语言自称，用于语言切换菜单
  name: string
  // 浏览器语言（navigator.languages）前缀匹配规则，按顺序命中
  match: string[]
}

export const SOURCE_LOCALE = 'zh-CN'

// 保存界面语言的 localStorage 键
export const LOCALE_STORAGE_KEY = 'gately.locale'

export const allLocales: LocaleMeta[] = [
  { code: 'en', name: 'English', match: ['en'] },
  { code: 'zh-CN', name: '简体中文', match: ['zh-cn', 'zh-sg', 'zh-hans', 'zh'] },
  { code: 'zh-TW', name: '繁體中文', match: ['zh-tw', 'zh-hk', 'zh-mo', 'zh-hant'] },
  { code: 'ja', name: '日本語', match: ['ja'] },
  { code: 'ko', name: '한국어', match: ['ko'] },
  { code: 'es', name: 'Español', match: ['es'] },
  { code: 'fr', name: 'Français', match: ['fr'] },
  { code: 'de', name: 'Deutsch', match: ['de'] },
  { code: 'ru', name: 'Русский', match: ['ru'] },
  { code: 'pt-BR', name: 'Português (Brasil)', match: ['pt'] },
]

// 回退语言：包含英语时为英语，否则为简体中文
export function fallbackOf(codes: readonly string[]): string {
  return codes.includes('en') ? 'en' : SOURCE_LOCALE
}

// 按浏览器语言偏好在 available 中挑选界面语言，命中不了时返回 fallback。
export function pickLocale(preferred: readonly string[], available: readonly LocaleMeta[], fallback: string): string {
  for (const raw of preferred) {
    const lang = raw.toLowerCase()
    // 繁体地区需先于泛 zh 匹配，因此按精确前缀逐项检查
    const hit = available.find((l) => l.match.some((m) => lang === m || lang.startsWith(m + '-')))
    if (hit) {
      return hit.code
    }
  }
  return fallback
}
