import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, type DefaultTheme, type HeadConfig } from 'vitepress'
import { FALLBACK_CODE, sidebarGroups, siteLocales, type SiteLocale } from './i18n/structure'

const SITE = 'https://gately.cn'
const REPO = 'https://github.com/wangwanjie/gately'
const TELEGRAM = 'https://t.me/gately_chat'
const here = dirname(fileURLToPath(import.meta.url))

// 读取语言包；尚未提供语言包的语言不会出现在站点中
function loadPack(code: string): any | null {
  const file = resolve(here, 'i18n', `${code}.json`)
  return existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : null
}

const packs = siteLocales
  .map((loc) => ({ loc, pack: loadPack(loc.code) }))
  .filter((x): x is { loc: SiteLocale; pack: any } => x.pack !== null)

function prefixOf(loc: SiteLocale) {
  return loc.dir ? `/${loc.dir}` : ''
}

// 侧边栏：各语言共用同一骨架，标题取自语言包
function buildSidebar(loc: SiteLocale, pack: any): DefaultTheme.SidebarItem[] {
  return sidebarGroups.map((g) => ({
    text: pack.sidebar.groups[g.id],
    items: g.slugs.map((slug) => {
      const link = slug === 'modules/index' ? '/docs/modules/' : `/docs/${slug}`
      return { text: pack.sidebar.items[slug], link: `${prefixOf(loc)}${link}` }
    })
  }))
}

function buildThemeConfig(loc: SiteLocale, pack: any): DefaultTheme.Config & Record<string, unknown> {
  const p = prefixOf(loc)
  const ui = pack.ui
  return {
    nav: [
      { text: pack.nav.home, link: `${p}/` },
      { text: pack.nav.docs, link: `${p}/docs/guide/introduction`, activeMatch: `${p}/docs/` },
      { text: pack.nav.modules, link: `${p}/docs/modules/`, activeMatch: `${p}/docs/modules/` },
      { text: pack.nav.download, link: `${p}/download` },
      { text: pack.nav.changelog, link: `${p}/news/`, activeMatch: `${p}/news/` },
      { text: pack.nav.sponsor, link: `${p}/sponsor` }
    ],
    sidebar: { [`${p}/docs/`]: buildSidebar(loc, pack) },
    editLink: { pattern: `${REPO}/edit/main/website/:path`, text: ui.editLink },
    lastUpdated: { text: ui.lastUpdated },
    outline: { label: ui.outlineLabel, level: [2, 3] },
    docFooter: { prev: ui.docPrev, next: ui.docNext },
    darkModeSwitchLabel: ui.darkModeSwitchLabel,
    lightModeSwitchTitle: ui.lightModeSwitchTitle,
    darkModeSwitchTitle: ui.darkModeSwitchTitle,
    sidebarMenuLabel: ui.sidebarMenuLabel,
    returnToTopLabel: ui.returnToTopLabel,
    langMenuLabel: ui.langMenuLabel,
    footer: {
      message: ui.footerMessage,
      copyright: `© 2026 Gately Contributors · <a href="${REPO}" target="_blank" rel="noreferrer">GitHub</a> · <span class="tg-foot"><a href="${TELEGRAM}" target="_blank" rel="noreferrer">Telegram</a><span class="tg-pop"><img src="/telegram-qr.svg" alt="Telegram QR" width="140" height="140" loading="lazy"></span></span>`
    },
    // 首页与下载页组件使用的文案
    aiNotice: ui.aiNotice,
    home: pack.home,
    download: pack.download,
    sponsor: pack.sponsor
  }
}

// 本地搜索的各语言界面文案
const searchLocales = Object.fromEntries(
  packs.map(({ loc, pack }) => [
    loc.dir || 'root',
    {
      translations: {
        button: { buttonText: pack.ui.searchButton, buttonAriaLabel: pack.ui.searchButton },
        modal: {
          noResultsText: pack.ui.noResultsText,
          resetButtonTitle: pack.ui.resetButtonTitle,
          footer: { selectText: pack.ui.selectText, navigateText: pack.ui.navigateText, closeText: pack.ui.closeText }
        }
      }
    }
  ])
)

const locales = Object.fromEntries(
  packs.map(({ loc, pack }) => [
    loc.dir || 'root',
    {
      label: pack.label,
      lang: pack.lang,
      link: loc.dir ? `/${loc.dir}/` : undefined,
      description: pack.description,
      themeConfig: buildThemeConfig(loc, pack),
      head: [
        ['meta', { name: 'keywords', content: pack.keywords }],
        ['meta', { property: 'og:title', content: pack.ogTitle }],
        ['meta', { property: 'og:description', content: pack.description }],
        ['meta', { property: 'og:locale', content: pack.lang.replace('-', '_') }]
      ] as HeadConfig[]
    }
  ])
)

// 首次访问时按浏览器语言跳转到对应语言；用户手动选择过语言则尊重其选择。
// 语言清单与匹配规则由构建时注入，保持与站点语言一致。
const redirectScript = `(function(){try{
var K='gately.site.locale',L=${JSON.stringify(
  packs.map(({ loc }) => ({ code: loc.code, dir: loc.dir, match: loc.match }))
)},F='${FALLBACK_CODE}';
var p=location.pathname,dirs=L.filter(function(l){return l.dir}).map(function(l){return l.dir});
var cur=''; for(var i=0;i<dirs.length;i++){ if(p==='/'+dirs[i]||p.indexOf('/'+dirs[i]+'/')===0){cur=dirs[i];break} }
var saved=localStorage.getItem(K);
if(saved!==null){ return }
var langs=(navigator.languages&&navigator.languages.length)?navigator.languages:[navigator.language||''];
var hit=null;
for(var a=0;a<langs.length&&!hit;a++){var s=String(langs[a]).toLowerCase();
 for(var b=0;b<L.length&&!hit;b++){ var m=L[b].match; for(var c=0;c<m.length;c++){ if(s===m[c]||s.indexOf(m[c]+'-')===0){hit=L[b];break} } } }
if(!hit){ hit=L.filter(function(l){return l.code===F})[0]||L[0] }
if(cur!==hit.dir && cur===''){
  location.replace((hit.dir?'/'+hit.dir:'')+p+location.search+location.hash);
}
}catch(e){}})();`

export default defineConfig({
  title: 'Gately',
  titleTemplate: ':title - Gately',
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: SITE },
  head: [
    // 图标由 web/scripts/gen-icons.mjs 生成；Android Chrome 添加到主屏幕读取 manifest 的 PNG 图标，iOS 读取 apple-touch-icon
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
    ['link', { rel: 'icon', href: '/favicon.ico', sizes: '16x16 32x32 48x48' }],
    ['link', { rel: 'icon', type: 'image/png', sizes: '192x192', href: '/icon-192.png' }],
    ['link', { rel: 'manifest', href: '/manifest.webmanifest' }],
    ['link', { rel: 'apple-touch-icon', sizes: '180x180', href: '/apple-touch-icon.png' }],
    ['meta', { name: 'apple-mobile-web-app-title', content: 'Gately' }],
    ['meta', { name: 'theme-color', content: '#0e7c66' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:url', content: SITE }],
    ['script', {}, redirectScript]
  ],
  locales,
  themeConfig: {
    logo: '/favicon.svg',
    siteTitle: 'Gately',
    socialLinks: [
      { icon: 'github', link: REPO },
      { icon: 'telegram', link: TELEGRAM }
    ],
    search: { provider: 'local', options: { locales: searchLocales } }
  },
  // 为每个页面输出多语言互链，便于搜索引擎按语言展示
  transformHead({ pageData }) {
    const rel = pageData.relativePath.replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
    const current = siteLocales.find((l) => l.dir && rel.startsWith(`${l.dir}/`))
    const slug = current ? rel.slice(current.dir.length + 1) : rel
    const tags: HeadConfig[] = []
    for (const { loc } of packs) {
      const target = `${loc.dir ? `${loc.dir}/` : ''}${slug}`
      const file = resolve(here, '..', `${target}.md`.replace(/\/\.md$/, '/index.md'))
      const idx = resolve(here, '..', target, 'index.md')
      if (!existsSync(file) && !existsSync(idx)) {
        continue
      }
      tags.push(['link', { rel: 'alternate', hreflang: loc.code, href: `${SITE}/${target}` }])
      if (loc.code === FALLBACK_CODE) {
        tags.push(['link', { rel: 'alternate', hreflang: 'x-default', href: `${SITE}/${target}` }])
      }
    }
    return tags
  }
})
