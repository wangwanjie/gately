// 受支持的界面语言清单：官网、README 与管理界面保持同一批语言；定制构建只包含其中一部分。
import { builtLocales } from 'virtual:gately-locales'
import { SOURCE_LOCALE, allLocales, fallbackOf, pickLocale, type LocaleMeta } from './localeMeta'

export { SOURCE_LOCALE }
export type { LocaleMeta }

// 本次构建包含的语言
export const locales: LocaleMeta[] = allLocales.filter((l) => builtLocales.includes(l.code))

// 回退语言：包含英语时为英语，否则为简体中文
export const FALLBACK_LOCALE = fallbackOf(builtLocales)

export function isSupported(code: string | null | undefined): code is string {
  return !!code && locales.some((l) => l.code === code)
}

// 按浏览器语言偏好挑选界面语言，命中不了时使用回退语言。
export function detectLocale(preferred: readonly string[]): string {
  return pickLocale(preferred, locales, FALLBACK_LOCALE)
}
