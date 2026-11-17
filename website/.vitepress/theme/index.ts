import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import Layout from './Layout.vue'
import HomeLanding from './components/HomeLanding.vue'
import DownloadList from './components/DownloadList.vue'
import SponsorPanel from './components/SponsorPanel.vue'
import TutorialCatalog from './components/TutorialCatalog.vue'
import TutorialVideo from './components/TutorialVideo.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    // 用户在语言菜单里手动切换语言后记住选择，之后不再按浏览器语言自动跳转
    if (typeof document !== 'undefined') {
      document.addEventListener(
        'click',
        (e) => {
          const a = (e.target as HTMLElement | null)?.closest?.('.VPNavBarTranslations a, .VPNavScreenTranslations a') as HTMLAnchorElement | null
          if (!a) {
            return
          }
          const dir = new URL(a.href, location.href).pathname.split('/')[1] ?? ''
          const known = ['en', 'zh-TW', 'ja', 'ko', 'es', 'fr', 'de', 'ru', 'pt-BR']
          localStorage.setItem('gately.site.locale', known.includes(dir) ? dir : 'root')
        },
        true
      )
    }
    app.component('HomeLanding', HomeLanding)
    app.component('DownloadList', DownloadList)
    app.component('SponsorPanel', SponsorPanel)
    app.component('TutorialCatalog', TutorialCatalog)
    app.component('TutorialVideo', TutorialVideo)
  }
} satisfies Theme
