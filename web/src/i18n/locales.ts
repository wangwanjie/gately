// 受支持的界面语言清单：官网、README 与管理界面保持同一批语言；定制构建只包含其中一部分。
import { builtLocales } from 'virtual:gately-locales'

export interface LocaleMeta {
  code: string
  // 语言自称，用于语言切换菜单
  name: string
  // 浏览器语言（navigator.languages）前缀匹配规则，按顺序命中
  match: string[]
}

export const SOURCE_LOCALE = 'zh-CN'

const allLocales: LocaleMeta[] = [
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

// 本次构建包含的语言
export const locales: LocaleMeta[] = allLocales.filter((l) => builtLocales.includes(l.code))

// 回退语言：包含英语时为英语，否则为简体中文
export const FALLBACK_LOCALE = builtLocales.includes('en') ? 'en' : SOURCE_LOCALE

export function isSupported(code: string | null | undefined): code is string {
  return !!code && locales.some((l) => l.code === code)
}

// 按浏览器语言偏好挑选界面语言，命中不了时使用回退语言。
export function detectLocale(preferred: readonly string[]): string {
  for (const raw of preferred) {
    const lang = raw.toLowerCase()
    // 繁体地区需先于泛 zh 匹配，因此按精确前缀逐项检查
    const hit = locales.find((l) => l.match.some((m) => lang === m || lang.startsWith(m + '-')))
    if (hit) {
      return hit.code
    }
  }
  return FALLBACK_LOCALE
}
