import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
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

// 最新版本号取自简体中文版本公告的文件名（news/v<版本>.md），按数字逐段比较
const latestVersion = readdirSync(resolve(here, '..', 'news'))
  .map((f) => /^v([\d.]+)\.md$/.exec(f)?.[1])
  .filter((v): v is string => !!v)
  .sort((a, b) => {
    const x = a.split('.').map(Number)
    const y = b.split('.').map(Number)
    for (let i = 0; i < Math.max(x.length, y.length); i++) {
      if ((x[i] ?? 0) !== (y[i] ?? 0)) {
        return (x[i] ?? 0) - (y[i] ?? 0)
      }
    }
    return 0
  })
  .pop()

// 首页的结构化数据：软件信息与常见问题（取当前语言首页的 FAQ 文案）
function homeJsonLd(pack: any, url: string, prefix: string, image: string): string {
  const data = [
    {
      '@context': 'https://schema.org',
      '@type': 'SoftwareApplication',
      name: 'Gately',
      url,
      description: pack.description,
      inLanguage: pack.lang,
      applicationCategory: 'UtilitiesApplication',
      applicationSubCategory: 'Network gateway',
      operatingSystem: 'Linux, macOS, Windows, FreeBSD, OpenWrt, fnOS, Docker',
      softwareVersion: latestVersion,
      license: 'https://www.apache.org/licenses/LICENSE-2.0',
      downloadUrl: `${SITE}${prefix}/download`,
      installUrl: `${SITE}${prefix}/docs/guide/install`,
      image,
      codeRepository: REPO,
      sameAs: [REPO],
      offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' }
    },
    {
      '@context': 'https://schema.org',
      '@type': 'FAQPage',
      inLanguage: pack.lang,
      mainEntity: pack.home.faqs.map((f: { q: string; a: string }) => ({
        '@type': 'Question',
        name: f.q,
        acceptedAnswer: { '@type': 'Answer', text: f.a }
      }))
    }
  ]
  // 内联在 <script> 里，转义 < 以免文案中出现 </script>
  return JSON.stringify(data).replace(/</g, '\\u003c')
}

// ---- 版本公告 RSS ----
// 读取 Markdown 头部的 title / description（值可能带双引号、单引号或不带引号，均为单行）
function readFrontmatter(file: string): Record<string, string> {
  const m = /^---\n([\s\S]*?)\n---/.exec(readFileSync(file, 'utf8'))
  const out: Record<string, string> = {}
  for (const line of m?.[1].split('\n') ?? []) {
    const kv = /^(\w+):\s*(.*)$/.exec(line)
    if (!kv) {
      continue
    }
    let v = kv[2].trim()
    if (v.startsWith('"')) {
      v = JSON.parse(v)
    } else if (v.startsWith("'")) {
      v = v.slice(1, -1).replace(/''/g, "'")
    }
    out[kv[1]] = v
  }
  return out
}

// 发布日期取自简体中文公告正文第一处「YYYY 年 M 月 D 日」，各语言共用（按北京时间零点）
function newsDate(version: string): Date | null {
  const file = resolve(here, '..', 'news', `v${version}.md`)
  if (!existsSync(file)) {
    return null
  }
  const m = /(\d{4}) 年 (\d{1,2}) 月 (\d{1,2}) 日/.exec(readFileSync(file, 'utf8'))
  if (!m) {
    return null
  }
  return new Date(`${m[1]}-${m[2].padStart(2, '0')}-${m[3].padStart(2, '0')}T00:00:00+08:00`)
}

function xmlEscape(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

// 生成某个语言的 RSS 2.0：条目为该语言 news/ 下的全部版本公告，按日期与版本号倒序
function buildFeed(loc: SiteLocale, pack: any): string {
  const p = prefixOf(loc)
  const dir = resolve(here, '..', loc.dir, 'news')
  const items = readdirSync(dir)
    .map((f) => /^v([\d.]+)\.md$/.exec(f)?.[1])
    .filter((v): v is string => !!v)
    .map((version) => ({ version, fm: readFrontmatter(resolve(dir, `v${version}.md`)), date: newsDate(version) }))
    .filter((x): x is { version: string; fm: Record<string, string>; date: Date } => x.date !== null)
    .sort((a, b) => b.date.getTime() - a.date.getTime() || b.version.localeCompare(a.version, 'en', { numeric: true }))
  const index = readFrontmatter(resolve(dir, 'index.md'))
  const self = `${SITE}${p}/feed.xml`
  const entries = items.map(({ version, fm, date }) => {
    const link = `${SITE}${p}/news/v${version}`
    return [
      '    <item>',
      `      <title>${xmlEscape(fm.title ?? `Gately v${version}`)}</title>`,
      `      <link>${link}</link>`,
      `      <guid isPermaLink="true">${link}</guid>`,
      `      <pubDate>${date.toUTCString()}</pubDate>`,
      `      <description>${xmlEscape(fm.description ?? '')}</description>`,
      '    </item>'
    ].join('\n')
  })
  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">',
    '  <channel>',
    `    <title>${xmlEscape(`Gately · ${pack.nav.changelog}`)}</title>`,
    `    <link>${SITE}${p}/news/</link>`,
    `    <atom:link href="${self}" rel="self" type="application/rss+xml" />`,
    `    <description>${xmlEscape(index.description ?? pack.description)}</description>`,
    `    <language>${pack.lang}</language>`,
    items.length ? `    <lastBuildDate>${items[0].date.toUTCString()}</lastBuildDate>` : '',
    ...entries,
    '  </channel>',
    '</rss>',
    ''
  ]
    .filter((line) => line !== '')
    .join('\n')
}

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
      copyright: `© 2026 Gately Contributors · <a href="${REPO}" target="_blank" rel="noreferrer">GitHub</a> · <a href="${p}/feed.xml">RSS</a> · <span class="tg-foot"><a href="${TELEGRAM}" target="_blank" rel="noreferrer">Telegram</a><span class="tg-pop"><img src="/telegram-qr.svg" alt="Telegram QR" width="140" height="140" loading="lazy"></span></span>`
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
        ['meta', { property: 'og:locale', content: pack.lang.replace('-', '_') }],
        ['link', { rel: 'alternate', type: 'application/rss+xml', title: `Gately · ${pack.nav.changelog}`, href: `${prefixOf(loc)}/feed.xml` }]
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
    ['meta', { property: 'og:site_name', content: 'Gately' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
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
  // 构建结束后为每个语言写出版本公告 RSS：简体中文在 /feed.xml，其余语言在 /<语言>/feed.xml
  buildEnd({ outDir }) {
    for (const { loc, pack } of packs) {
      const dir = resolve(outDir, loc.dir)
      mkdirSync(dir, { recursive: true })
      writeFileSync(resolve(dir, 'feed.xml'), buildFeed(loc, pack))
    }
  },
  // 为每个页面输出规范地址、分享卡片信息与多语言互链，便于搜索引擎按语言展示
  transformHead({ pageData, title, description }) {
    const rel = pageData.relativePath.replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
    const current = siteLocales.find((l) => l.dir && rel.startsWith(`${l.dir}/`))
    const slug = current ? rel.slice(current.dir.length + 1) : rel
    const code = current?.code ?? 'zh-CN'
    const url = `${SITE}/${rel}`
    // 分享图只有中英文两张：简繁中文用中文图，其余语言用英文图
    const image = `${SITE}/${code.startsWith('zh') ? 'og-zh' : 'og-en'}.png`
    const tags: HeadConfig[] = [
      ['link', { rel: 'canonical', href: url }],
      ['meta', { property: 'og:url', content: url }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { property: 'og:image', content: image }],
      ['meta', { property: 'og:image:width', content: '1200' }],
      ['meta', { property: 'og:image:height', content: '630' }],
      ['meta', { property: 'og:image:alt', content: 'Gately' }],
      ['meta', { name: 'twitter:image', content: image }]
    ]
    if (slug === '' && pageData.frontmatter.layout === 'page') {
      const own = packs.find(({ loc }) => loc.code === code)
      if (own) {
        tags.push(['script', { type: 'application/ld+json' }, homeJsonLd(own.pack, url, prefixOf(own.loc), image)])
      }
    }
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
