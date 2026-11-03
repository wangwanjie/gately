// 国际化入口：文案按语言目录拆分（src/locales/<语言>/<命名空间>.json），按需加载。
import { createI18n } from 'vue-i18n'
import { messageLoaders } from 'virtual:gately-locales'
import { FALLBACK_LOCALE, detectLocale, isSupported } from './locales'

const STORAGE_KEY = 'gately.locale'

// 只含本次构建包含的语言，见 vite.config.ts 的 gately-locales 插件
const loaders = messageLoaders

export const i18n = createI18n({
  legacy: false,
  locale: FALLBACK_LOCALE,
  fallbackLocale: FALLBACK_LOCALE,
  // 缺失键回退到回退语言时不刷控制台
  missingWarn: false,
  fallbackWarn: false,
  messages: {},
})

// 供 .ts 模块（非组件）使用的全局翻译函数
export const t = i18n.global.t as (key: string, ...args: any[]) => string

const loaded = new Set<string>()

async function loadMessages(code: string) {
  if (loaded.has(code)) {
    return
  }
  const prefix = `../locales/${code}/`
  const merged: Record<string, unknown> = {}
  await Promise.all(
    Object.entries(loaders)
      .filter(([path]) => path.startsWith(prefix))
      .map(async ([path, load]) => {
        // 文件名即命名空间，例如 wol.json -> wol.*
        const ns = path.slice(prefix.length).replace(/\.json$/, '')
        merged[ns] = (await load()).default
      }),
  )
  i18n.global.setLocaleMessage(code, merged as any)
  loaded.add(code)
}

export function savedLocale(): string | null {
  const v = localStorage.getItem(STORAGE_KEY)
  return isSupported(v) ? v : null
}

export function initialLocale(): string {
  return savedLocale() ?? detectLocale(navigator.languages?.length ? navigator.languages : [navigator.language])
}

// 切换界面语言：先加载文案，再生效并记住选择。
export async function setLocale(code: string, persist = true) {
  const target = isSupported(code) ? code : FALLBACK_LOCALE
  await loadMessages(FALLBACK_LOCALE)
  await loadMessages(target)
  i18n.global.locale.value = target as any
  document.documentElement.lang = target
  if (persist) {
    localStorage.setItem(STORAGE_KEY, target)
  }
}

export async function setupI18n() {
  await setLocale(initialLocale(), false)
}
