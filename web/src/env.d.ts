declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}

// 由 vite.config.ts 的 gately-locales 插件生成，只包含本次构建的界面语言
declare module 'virtual:gately-locales' {
  import type { Language } from 'element-plus/es/locale'
  export const builtLocales: string[]
  export const messageLoaders: Record<string, () => Promise<{ default: Record<string, unknown> }>>
  export const elementLoaders: Record<string, () => Promise<{ default: Language }>>
}
