// Element Plus 组件库自带文案（分页、日期选择器等）与界面语言对应，只打包本次构建包含的语言。
import type { Language } from 'element-plus/es/locale'
import { elementLoaders } from 'virtual:gately-locales'
import { FALLBACK_LOCALE } from './locales'

export async function loadElementLocale(code: string): Promise<Language> {
  return (await (elementLoaders[code] ?? elementLoaders[FALLBACK_LOCALE])()).default
}
