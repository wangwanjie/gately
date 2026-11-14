// 最先执行：经统一网关访问时给站内请求补上路径前缀
import './basePath'
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import router from './router'
import { i18n, setupI18n } from './i18n'
import { initTheme } from './composables/useTheme'
import { installModalScrollGuard } from './composables/useScrollLock'
import './style.css'

// 页面模块或样式加载失败时（网络抖动、升级后旧文件已不存在）自动刷新一次；10 秒内不重复刷新，避免循环
window.addEventListener('vite:preloadError', (e) => {
  const key = 'gately.preloadReloadAt'
  const last = Number(sessionStorage.getItem(key) || 0)
  if (Date.now() - last > 10_000) {
    sessionStorage.setItem(key, String(Date.now()))
    e.preventDefault()
    window.location.reload()
  }
})

// 挂载前应用主题，避免首屏亮暗闪烁
initTheme()

// 先加载界面语言文案，再挂载，避免首屏出现未翻译的键名
setupI18n().then(() => {
  // 对话框、确认框、抽屉打开时，滚动不穿透到下方页面
  installModalScrollGuard()
  createApp(App).use(i18n).use(router).use(ElementPlus).mount('#app')
})
