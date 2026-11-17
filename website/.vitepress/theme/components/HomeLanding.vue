<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useData, withBase } from 'vitepress'
import HeroFlow from './HeroFlow.vue'
import { copyText } from '../clipboard'
import {
  Globe, RadioTower, ArrowLeftRight, ShieldCheck, Waypoints, ShieldAlert, Network, Boxes,
  Terminal, HardDrive, Clock, KeyRound, Copy, Check, Github, Eye,
  Bell, DatabaseBackup, Activity, Bot, UserCheck, ChartLine, AppWindow, LogIn, Server, ArrowRight,
  Layers, ScrollText, History, Braces, Smartphone, Rocket, CircleCheck, ShieldBan, Split, Workflow, Play, X, ArrowDown
} from 'lucide-vue-next'

// 所有文案来自当前语言包（.vitepress/i18n/<语言>.json 的 home 节点），图标按顺序与条目一一对应
const { frontmatter, localeIndex } = useData()
// 文案由 config.mts 的 transformPageData 按页面语言注入
const h = computed(() => frontmatter.value.siteText.home)
const prefix = computed(() => (localeIndex.value === 'root' ? '' : `/${localeIndex.value}`))
const link = (path: string) => withBase(`${prefix.value}${path}`)

const featureIcons = [
  Globe, RadioTower, ArrowLeftRight, KeyRound, Waypoints, ShieldAlert, Network, Boxes, Terminal,
  HardDrive, Clock, ShieldCheck, UserCheck, ChartLine, Bell, DatabaseBackup, Activity, Bot, ShieldBan, Split, Workflow
]
// 选择理由八项：开源透明、一站整合、层层防护、操作审计、配置回滚、完整 API、移动端友好、开箱即用
const whyIcons = [Eye, Layers, ShieldCheck, ScrollText, History, Braces, Smartphone, Rocket]
// 飞牛专区的三个要点：统一网关、登录态直接登录、root 运行
const fnosIcons = [AppWindow, LogIn, Server]

const features = computed(() => h.value.features.map((f: any, i: number) => ({ ...f, icon: featureIcons[i] })))
const whyItems = computed(() => h.value.why.items.map((f: any, i: number) => ({ ...f, icon: whyIcons[i] })))

// 截图展示：与 showcase 文案一一对应；最后一项是手机截图，用手机外框展示
// 截图按界面语言分目录（/shots/<语言>/），根路径（简体中文）对应 zh-CN；拓扑是动图
const shotLang = computed(() => (localeIndex.value === 'root' ? 'zh-CN' : localeIndex.value))
const shots = computed(() => {
  const dir = `/shots/${shotLang.value}`
  return [
    { src: `${dir}/overview.webp`, w: 1600, h: 1112 },
    { src: `${dir}/topology.webp`, w: 1000, h: 625 },
    { src: `${dir}/guard.webp`, w: 1600, h: 1000 },
    { src: `${dir}/config-history.webp`, w: 1600, h: 1000 },
    { src: `${dir}/api-docs.webp`, w: 1600, h: 1000 },
    { src: `${dir}/mobile.webp`, w: 780, h: 1688, phone: true }
  ]
})
const showcase = computed(() => h.value.showcase.map((s: any, i: number) => ({ ...s, shot: shots.value[i] })))

// 首屏下方的产品截图：概览加三个常用模块，标签名复用功能列表里的模块名（Web 服务、动态域名、协议分流）
const productTabs = computed(() => [
  { key: 'overview', label: h.value.product.overview },
  { key: 'web', label: h.value.features[4].title },
  { key: 'ddns', label: h.value.features[0].title },
  { key: 'protoroute', label: h.value.features[19].title }
])
const activeShot = ref('overview')
const activeShotLabel = computed(() => productTabs.value.find((t) => t.key === activeShot.value)?.label ?? '')

// 巡览视频只有中文与英文两版（随官网部署在 /videos/，不进 git），简繁中文用中文版，其余语言用英文版
const tourSrc = computed(() => withBase(`/videos/gately-tour.${shotLang.value.startsWith('zh') ? 'zh' : 'en'}.mp4`))
const tourDialog = ref<HTMLDialogElement>()
const tourVideo = ref<HTMLVideoElement>()
const tourOpen = ref(false)
// 对话框当前播放的视频：巡览视频是 16:10 无声录屏，宣传片与功能全览是 16:9 带配乐
const playing = ref({ src: '', aspect: '16 / 10', muted: true })

// 宣传片与功能全览视频有简体中文与英文两版（随官网部署在 /videos/，不进 git）：简繁中文页用中文版，其余语言用英文版
// 英文版功能全览时长，按实际成片填写
const OVERVIEW_EN_DUR = '5:08'
// 视频文件重新编码后递增，让浏览器不再使用缓存里的旧文件
const FILM_VER = '2'
const filmLang = computed(() => (shotLang.value.startsWith('zh') ? 'zh' : 'en'))
const FILM_TEXT: Record<string, { title: string, desc: string, play: string, promo: string, promoText: string, overview: string, overviewText: string }> = {
  'zh-CN': {
    title: '视频介绍', desc: '先用短片看懂 Gately，再花 5 分钟逐项看完主要功能。画面均为真实界面截图与录屏。', play: '播放',
    promo: '宣传片', promoText: '一个程序替代 nginx、certbot、ddns-go、fail2ban 等，自托管、无遥测、Apache-2.0。',
    overview: '功能全览', overviewText: '安装登录、概览、Web 服务、证书、动态域名、协议分流、入侵防护、内网穿透与配置回滚。'
  },
  'zh-TW': {
    title: '影片介紹', desc: '短片看懂 Gately，再用 5 分鐘逐項看完主要功能。畫面均為真實介面截圖，影片為簡體中文。', play: '播放',
    promo: '宣傳片', promoText: '一個程式替代 nginx、certbot、ddns-go、fail2ban 等，自架、無遙測、Apache-2.0。',
    overview: '功能全覽', overviewText: '安裝登入、概覽、Web 服務、憑證、動態網域、協定分流、入侵防護、內網穿透與設定回滾。'
  },
  en: {
    title: 'Videos', desc: 'A short film on what Gately is, then a 5-minute walk through the main features. Every frame shows the real admin UI.', play: 'Play',
    promo: 'Promo', promoText: 'One program in place of nginx, certbot, ddns-go, fail2ban and more. Self-hosted, no telemetry, Apache-2.0.',
    overview: 'Feature walkthrough', overviewText: 'Install, overview, web services, certificates, DDNS, protocol routing, intrusion protection, tunnels and config rollback.'
  }
}
const filmsText = computed(() => FILM_TEXT[shotLang.value] ?? FILM_TEXT.en)
const films = computed(() => {
  const t = filmsText.value
  const lang = filmLang.value
  return [
    { key: 'promo', title: t.promo, text: t.promoText, dur: '0:42', src: withBase(`/videos/gately-promo.${lang}.mp4?v=${FILM_VER}`), poster: withBase(`/videos/gately-promo.${lang}.jpg`) },
    { key: 'overview', title: t.overview, text: t.overviewText, dur: lang === 'zh' ? '5:08' : OVERVIEW_EN_DUR, src: withBase(`/videos/gately-overview.${lang}.mp4?v=${FILM_VER}`), poster: withBase(`/videos/gately-overview.${lang}.jpg`) }
  ]
})

// 播放时锁住页面滚动：对话框里滚动滚轮或双指滑动触控板，背后的页面保持不动；补上滚动条宽度，避免页面横向跳动
function lockPageScroll(lock: boolean) {
  const root = document.documentElement
  if (lock) {
    const gap = window.innerWidth - root.clientWidth
    root.style.overflow = 'hidden'
    root.style.paddingRight = gap > 0 ? `${gap}px` : ''
  } else {
    root.style.overflow = ''
    root.style.paddingRight = ''
  }
}

function openVideo(src: string, aspect: string, muted: boolean) {
  playing.value = { src, aspect, muted }
  tourOpen.value = true
  lockPageScroll(true)
  tourDialog.value?.showModal()
  nextTick(() => {
    tourVideo.value?.play().catch(() => {})
  })
}

function openTour() {
  openVideo(tourSrc.value, '16 / 10', true)
}

// 关闭对话框（按钮、Esc 或点击遮罩）时暂停播放
function onTourClose() {
  tourOpen.value = false
  tourVideo.value?.pause()
  lockPageScroll(false)
}

function onTourBackdrop(e: MouseEvent) {
  if (e.target === tourDialog.value) {
    tourDialog.value?.close()
  }
}

// 功能按场景分组，数字是 features 数组的下标：网络接入、网站与证书、安全防护、运维工具、监控与自动化
const groupIndexes = [[0, 1, 2, 19, 6], [4, 3, 20], [5, 18, 12, 11], [7, 8, 9, 10, 16], [13, 14, 15, 17]]
const activeGroup = ref(0)
const groupFeatures = computed(() => groupIndexes[activeGroup.value].map((i) => features.value[i]))
const fnosPoints = computed(() => h.value.fnos.points.map((f: any, i: number) => ({ ...f, icon: fnosIcons[i] })))

// 滚动条带里的平台与集成都是专有名词，各语言通用
const marquee = [
  'Linux', 'Docker', 'OpenWrt', 'fnOS', 'macOS', 'Windows', 'FreeBSD', 'Cloudflare', "Let's Encrypt",
  'Tailscale', 'NetBird', 'frp', 'cloudflared', 'OWASP CRS', 'Coraza', 'Prometheus', 'Grafana', 'HTTP/3', 'IPv6',
  'WebDAV', 'SMB', 'Passkey', 'OIDC', 'Telegram', 'ntfy', 'Bark', 'MCP'
]

// Compose 示例：与 docker run 一致加 NET_ADMIN、默认不挂 docker.sock；
// 时区按页面语言给出常见值，与安装文档一致；英文页不写 TZ（容器默认 UTC）
const COMPOSE_TZ: Record<string, string> = {
  'zh-CN': 'Asia/Shanghai', 'zh-TW': 'Asia/Taipei', ja: 'Asia/Tokyo', ko: 'Asia/Seoul', es: 'Europe/Madrid',
  fr: 'Europe/Paris', de: 'Europe/Berlin', ru: 'Europe/Moscow', 'pt-BR': 'America/Sao_Paulo'
}
const compose = computed(() => {
  const tz = COMPOSE_TZ[shotLang.value]
  const lines = [
    'services:',
    '  gately:',
    '    image: ghcr.io/wangwanjie/gately:latest',
    '    container_name: gately',
    '    restart: unless-stopped',
    '    network_mode: host',
    '    cap_add:',
    '      - NET_ADMIN',
    '    volumes:',
    '      - ./data:/etc/gately',
    `      # ${h.value.composeDockerHint}`,
    '      # - /var/run/docker.sock:/var/run/docker.sock'
  ]
  if (tz) {
    lines.push('    environment:', `      - TZ=${tz}`)
  }
  return lines.join('\n')
})

const installTabs = computed(() => [
  { key: 'docker', label: 'Docker', code: h.value.installDocker },
  { key: 'compose', label: 'Compose', code: compose.value },
  { key: 'binary', label: h.value.installTabBinary, code: h.value.installBinary },
  { key: 'fnos', label: h.value.installTabFnos, code: h.value.installFnos },
  { key: 'openwrt', label: 'OpenWrt', code: h.value.installOpenwrt }
])
const activeTab = ref('docker')
const activeLines = computed(() => (installTabs.value.find((x) => x.key === activeTab.value)?.code ?? '').split('\n'))
const copied = ref(false)
// 安装命令所在的 code，复制失败时选中它供手动复制
const installCodeEl = ref<HTMLElement>()
let copiedTimer: ReturnType<typeof setTimeout> | undefined

async function copyCode() {
  const tab = installTabs.value.find((x) => x.key === activeTab.value)
  if (!tab) {
    return
  }
  copied.value = await copyText(tab.code, installCodeEl.value)
  clearTimeout(copiedTimer)
  if (copied.value) {
    copiedTimer = setTimeout(() => {
      copied.value = false
    }, 1800)
  }
}

const openFaq = ref(0)

// ---- 滚动出现 ----
// 挂载后才给根节点加 js 类，脚本没加载时内容保持可见
const root = ref<HTMLElement>()
const ready = ref(false)
let revealIo: IntersectionObserver | undefined

function observeReveal() {
  root.value?.querySelectorAll('[data-r]:not(.in)').forEach((el) => revealIo?.observe(el))
}

onMounted(async () => {
  revealIo = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) {
          continue
        }
        const el = entry.target as HTMLElement
        el.classList.add('in')
        revealIo?.unobserve(el)
      }
    },
    { rootMargin: '0px 0px -8% 0px', threshold: 0.12 }
  )
  ready.value = true
  await nextTick()
  observeReveal()
})

// 切换语言后节点会重建，重新登记
watch(localeIndex, async () => {
  await nextTick()
  observeReveal()
})

onBeforeUnmount(() => {
  // 对话框开着时离开首页，恢复页面滚动
  if (tourOpen.value) {
    lockPageScroll(false)
  }
  revealIo?.disconnect()
  cancelAnimationFrame(spotRaf)
})

// ---- 卡片聚光 ----
// 鼠标在带 data-spot 的卡片上移动时，把位置写进 CSS 变量，由伪元素画出跟随光斑；用 rAF 合并同一帧内的多次移动
let spotRaf = 0
function onSpot(e: PointerEvent) {
  const card = (e.target as HTMLElement).closest('[data-spot]') as HTMLElement | null
  if (!card) {
    return
  }
  cancelAnimationFrame(spotRaf)
  spotRaf = requestAnimationFrame(() => {
    const r = card.getBoundingClientRect()
    card.style.setProperty('--mx', `${e.clientX - r.left}px`)
    card.style.setProperty('--my', `${e.clientY - r.top}px`)
  })
}
</script>

<template>
  <div ref="root" class="gl" :class="{ js: ready }">
    <!-- Hero -->
    <section class="hero">
      <div class="hero-bg" aria-hidden="true">
        <div class="grid-lines" />
        <div class="aurora a1" />
        <div class="aurora a2" />
      </div>
      <div class="wrap hero-grid">
        <div class="hero-text">
          <!-- 有版本公告时链接到公告页，否则显示标语并链接到 GitHub Releases -->
          <!-- 公告文字较长时单行截断，完整内容放在 title 里 -->
          <a v-if="h.announce" class="pill rise" :href="h.announce.link" :title="h.announce.text">
            <span class="dot" /><span class="pill-text">{{ h.announce.text }}</span>
          </a>
          <a v-else class="pill rise" href="https://github.com/wangwanjie/gately/releases" target="_blank" rel="noreferrer">
            <span class="dot" /><span class="pill-text">{{ h.pill }}</span>
          </a>
          <!-- 第一行是定位句，第二行用较小字号列出能力 -->
          <h1>
            <span class="rise" style="--d: .08s">{{ h.titleLine1 }}</span>
            <span class="grad sub rise" style="--d: .18s">{{ h.titleLine2 }}</span>
          </h1>
          <p class="lead rise" style="--d: .28s">{{ h.lead }}</p>
          <div class="cta rise" style="--d: .38s">
            <a class="btn primary" :href="link('/docs/guide/quick-start')">{{ h.ctaStart }}<ArrowRight :size="16" class="arr" /></a>
            <a class="btn" :href="link('/download')">{{ h.ctaDownload }}</a>
            <a class="btn ghost" href="https://github.com/wangwanjie/gately" target="_blank" rel="noreferrer">
              <Github :size="16" />GitHub
            </a>
          </div>
          <p class="meta rise" style="--d: .48s">{{ h.pill }}</p>
        </div>
        <HeroFlow class="hero-visual rise" style="--d: .2s" :labels="h.flow" />
      </div>

      <!-- 真实界面截图：带浏览器外框，可切换模块；右侧入口播放巡览视频 -->
      <div class="wrap product rise" style="--d: .45s">
        <div class="product-bar">
          <div class="product-tabs" role="tablist">
            <button
              v-for="t in productTabs"
              :key="t.key"
              role="tab"
              :aria-selected="activeShot === t.key"
              :class="{ on: activeShot === t.key }"
              @click="activeShot = t.key"
            >
              {{ t.label }}
            </button>
          </div>
          <button class="tour-btn" @click="openTour"><Play :size="15" />{{ h.product.tour }}</button>
        </div>
        <div class="frame product-frame">
          <div class="frame-bar"><i /><i /><i /><span class="frame-title">Gately · {{ activeShotLabel }}</span></div>
          <div class="product-shot">
            <img
              :key="activeShot"
              :src="withBase(`/shots/${shotLang}/ui-${activeShot}.webp`)"
              :alt="`Gately · ${activeShotLabel}`"
              width="1600"
              height="1000"
              loading="lazy"
              decoding="async"
            />
          </div>
        </div>
      </div>
    </section>

    <!-- 巡览视频对话框：打开时才设置 src 并开始加载 -->
    <!-- 弹窗内容不需要滚动：拦下滚轮与触控板滑动，配合 lockPageScroll 让背后页面保持不动 -->
    <dialog ref="tourDialog" class="tour" :aria-label="h.product.tour" @close="onTourClose" @click="onTourBackdrop" @wheel.prevent>
      <div class="tour-box">
        <button class="tour-close" :aria-label="h.product.close" @click="tourDialog?.close()"><X :size="18" /></button>
        <video
          ref="tourVideo"
          :src="tourOpen ? playing.src : undefined"
          :style="{ aspectRatio: playing.aspect }"
          :muted="playing.muted"
          controls
          playsinline
          preload="none"
          width="1440"
          height="900"
        />
      </div>
    </dialog>

    <!-- 视频介绍：宣传片与功能全览，中文页面用中文版、其他语言用英文版；点击封面在上面的对话框里播放 -->
    <section id="videos" class="wrap films">
      <header class="head" data-r>
        <h2>{{ filmsText.title }}</h2>
        <p>{{ filmsText.desc }}</p>
      </header>
      <div class="films-grid">
        <button
          v-for="(f, i) in films"
          :key="f.key"
          class="film"
          data-r
          :style="{ '--i': i }"
          :aria-label="`${filmsText.play}：${f.title}`"
          @click="openVideo(f.src, '16 / 9', false)"
        >
          <span class="film-poster">
            <img :src="f.poster" :alt="f.title" width="1280" height="720" loading="lazy" decoding="async" />
            <span class="film-play"><Play :size="26" /></span>
            <span class="film-dur">{{ f.dur }}</span>
          </span>
          <span class="film-title">{{ f.title }}</span>
          <span class="film-text">{{ f.text }}</span>
        </button>
      </div>
    </section>

    <!-- 能替代哪些工具：左侧是常见做法，右侧是 Gately 里对应的模块，内容与 README 对照表一致 -->
    <section class="wrap replaces">
      <header class="head" data-r>
        <h2>{{ h.replaces.title }}</h2>
        <p>{{ h.replaces.desc }}</p>
      </header>
      <div class="rp-grid">
        <article v-for="(r, i) in h.replaces.items" :key="r.module" class="rp" data-r :style="{ '--i': i % 3 }">
          <p class="rp-from">{{ r.from }}</p>
          <ArrowDown :size="14" class="rp-arrow" aria-hidden="true" />
          <h3>{{ r.module }}</h3>
          <p class="rp-text">{{ r.text }}</p>
        </article>
      </div>
      <!-- eslint-disable-next-line vue/no-v-html -->
      <p class="rp-note" data-r v-html="h.replaces.note" />
    </section>

    <!-- 平台与集成滚动条带 -->
    <section class="marquee" :aria-label="h.worksWith">
      <p class="marquee-cap">{{ h.worksWith }}</p>
      <div class="marquee-view">
        <div class="marquee-track">
          <span v-for="(m, i) in [...marquee, ...marquee]" :key="i" :aria-hidden="i >= marquee.length ? 'true' : undefined">{{ m }}</span>
        </div>
      </div>
    </section>

    <!-- 选择理由：八项卖点 -->
    <section class="wrap block why-block">
      <header class="head" data-r>
        <span class="kicker">01</span>
        <h2>{{ h.why.title }}</h2>
        <p>{{ h.why.desc }}</p>
      </header>
      <div class="why" @pointermove="onSpot">
        <article v-for="(w, i) in whyItems" :key="w.title" class="why-card" data-r data-spot :style="{ '--i': i % 4 }">
          <div class="why-ico"><component :is="w.icon" :size="22" /></div>
          <h3>{{ w.title }}</h3>
          <p>{{ w.text }}</p>
        </article>
      </div>
    </section>

    <!-- 截图展示：文字与截图左右交替 -->
    <section class="wrap show">
      <div v-for="(s, i) in showcase" :key="s.title" class="show-row" :class="{ flip: i % 2 === 1, phone: s.shot.phone }">
        <div class="show-text" data-r>
          <span class="tag">{{ s.tag }}</span>
          <h2>{{ s.title }}</h2>
          <p>{{ s.text }}</p>
          <ul>
            <li v-for="pt in s.points" :key="pt"><CircleCheck :size="18" />{{ pt }}</li>
          </ul>
        </div>
        <div class="show-media" data-r style="--i: 1">
          <div v-if="s.shot.phone" class="device">
            <img :src="withBase(s.shot.src)" :alt="s.title" :width="s.shot.w" :height="s.shot.h" loading="lazy" decoding="async" />
          </div>
          <div v-else class="frame">
            <div class="frame-bar"><i /><i /><i /></div>
            <img :src="withBase(s.shot.src)" :alt="s.title" :width="s.shot.w" :height="s.shot.h" loading="lazy" decoding="async" />
          </div>
        </div>
      </div>
    </section>

    <!-- 功能：按场景分组的标签页 -->
    <section class="wrap block">
      <header class="head" data-r>
        <span class="kicker">02</span>
        <h2>{{ h.featuresTitle }}</h2>
        <p>{{ h.featuresDesc }}</p>
      </header>
      <div class="groups" role="tablist" data-r>
        <button
          v-for="(g, i) in h.featureGroups"
          :key="g"
          role="tab"
          :aria-selected="activeGroup === i"
          :class="{ on: activeGroup === i }"
          @click="activeGroup = i"
        >
          {{ g }}
        </button>
      </div>
      <!-- 切换分组时整组重建，卡片依次浮现 -->
      <div :key="activeGroup" class="grid feat" @pointermove="onSpot">
        <article v-for="(f, i) in groupFeatures" :key="f.title" class="card pop" data-spot :style="{ '--i': i }">
          <div class="ico"><component :is="f.icon" :size="20" /></div>
          <h3>{{ f.title }}</h3>
          <p>{{ f.text }}</p>
        </article>
      </div>
      <p class="more"><a :href="link('/docs/modules/')">{{ h.moreModules }}</a></p>
    </section>

    <!-- 飞牛 fnOS 原生应用：突出统一网关与登录态直接登录 -->
    <section class="wrap block">
      <div class="fnos" data-r>
        <div class="fnos-text">
          <span class="tag">{{ h.fnos.tag }}</span>
          <h2>{{ h.fnos.title }}</h2>
          <p>{{ h.fnos.desc }}</p>
          <div class="cta">
            <a class="btn primary" :href="link('/docs/guide/install#fnos')">{{ h.fnos.cta }}<ArrowRight :size="16" class="arr" /></a>
            <a class="btn" :href="link('/download#fnos')">{{ h.ctaDownload }}</a>
          </div>
        </div>
        <div class="fnos-points">
          <article v-for="(p, i) in fnosPoints" :key="p.title" class="card" data-r :style="{ '--i': i + 1 }">
            <div class="ico"><component :is="p.icon" :size="20" /></div>
            <div>
              <h3>{{ p.title }}</h3>
              <p>{{ p.text }}</p>
            </div>
          </article>
        </div>
      </div>
    </section>

    <!-- 深色区：安装 -->
    <section class="night">
      <div class="night-bg" aria-hidden="true" />
      <div class="wrap">
        <header class="head" data-r>
          <span class="kicker">03</span>
          <h2>{{ h.installTitle }}</h2>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <p v-html="h.installDesc" />
        </header>
        <div class="term" data-r>
          <div class="tabs">
            <span class="lights"><i /><i /><i /></span>
            <button v-for="tab in installTabs" :key="tab.key" :class="{ on: activeTab === tab.key }" @click="activeTab = tab.key">
              {{ tab.label }}
            </button>
            <button class="copy" @click="copyCode">
              <component :is="copied ? Check : Copy" :size="14" />{{ copied ? h.copied : h.copy }}
            </button>
          </div>
          <!-- 每行单独一个 span，切换标签时逐行浮现；复制始终使用原始文本 -->
          <pre><code ref="installCodeEl" :key="activeTab"><span v-for="(line, i) in activeLines" :key="i" class="ln" :class="{ cm: line.trimStart().startsWith('#') }" :style="{ '--i': i }">{{ line }}{{ i < activeLines.length - 1 ? '\n' : '' }}</span><span class="caret" /></code></pre>
        </div>
        <p class="more"><a :href="link('/docs/guide/install')">{{ h.fullInstall }}</a></p>
      </div>
    </section>

    <!-- FAQ -->
    <section class="wrap block narrow">
      <header class="head" data-r>
        <span class="kicker">04</span>
        <h2>{{ h.faqTitle }}</h2>
      </header>
      <div class="faq" data-r>
        <div v-for="(f, i) in h.faqs" :key="f.q" class="qa" :class="{ open: openFaq === i }">
          <button :aria-expanded="openFaq === i" @click="openFaq = openFaq === i ? -1 : i">
            {{ f.q }}<span class="pm" />
          </button>
          <div class="ans"><p>{{ f.a }}</p></div>
        </div>
      </div>
      <p class="more"><a :href="link('/docs/guide/faq')">{{ h.moreFaq }}</a></p>
    </section>

    <!-- CTA -->
    <section class="wrap block">
      <div class="final" data-r>
        <div class="final-inner">
          <h2>{{ h.finalTitle }}</h2>
          <p>{{ h.finalDesc }}</p>
          <div class="cta center">
            <a class="btn primary lg" :href="link('/docs/guide/quick-start')">{{ h.ctaStart }}<ArrowRight :size="18" class="arr" /></a>
            <a class="btn lg" href="https://github.com/wangwanjie/gately" target="_blank" rel="noreferrer"><Github :size="18" />{{ h.starOnGithub }}</a>
          </div>
          <!-- Telegram 群二维码：手机扫码即可加入 -->
          <a class="tg" href="https://t.me/gately_chat" target="_blank" rel="noreferrer">
            <img class="tg-qr" :src="withBase('/telegram-qr.svg')" :alt="h.telegramScan" width="128" height="128" loading="lazy" />
            <span>{{ h.telegramScan }}</span>
          </a>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.gl {
  --acc: #0e7c66;
  --acc-2: #6366f1;
  --line: rgba(14, 124, 102, 0.09);
  --ease: cubic-bezier(.2, .7, .2, 1);
  color: var(--vp-c-text-1);
  overflow-x: hidden;
}
.dark .gl { --acc: #3ed1ae; --acc-2: #a5b4fc; --line: rgba(62, 209, 174, 0.07); }
.wrap { max-width: 1200px; margin: 0 auto; padding: 0 24px; }
.narrow { max-width: 820px; }
.block { padding-top: 96px; padding-bottom: 8px; }
.head { text-align: center; margin-bottom: 44px; }
.head h2 { font-size: 36px; line-height: 1.25; font-weight: 800; letter-spacing: -0.02em; margin: 0; border: 0; padding: 0; }
.head p { margin: 14px auto 0; max-width: 640px; color: var(--vp-c-text-2); line-height: 1.75; font-size: 16px; }
.kicker { display: inline-flex; align-items: center; gap: 10px; margin-bottom: 14px; font-family: var(--vp-font-family-mono); font-size: 12px; letter-spacing: .2em; color: var(--acc); }
.kicker::before, .kicker::after { content: ''; width: 28px; height: 1px; background: linear-gradient(90deg, transparent, currentColor); }
.kicker::after { transform: scaleX(-1); }
.more { text-align: center; margin-top: 28px; }
.more a { color: var(--vp-c-brand-1); font-weight: 600; text-decoration: none; }
.more a:hover { text-decoration: underline; }

/* ---- 进场与滚动出现：只动 opacity / transform ---- */
.rise { animation: rise .9s var(--ease) both; animation-delay: var(--d, 0s); }
@keyframes rise { from { opacity: 0; transform: translateY(22px); } }
.js [data-r] { opacity: 0; transform: translateY(26px); transition: opacity .8s var(--ease), transform .8s var(--ease); transition-delay: calc(var(--i, 0) * 80ms); }
.js [data-r].in { opacity: 1; transform: none; }

/* ---- hero ---- */
.hero { position: relative; padding: 52px 0 24px; isolation: isolate; }
.hero-bg { position: absolute; inset: -80px 0 0; z-index: -1; overflow: hidden; pointer-events: none; }
.grid-lines { position: absolute; inset: 0; background-image: linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px); background-size: 48px 48px; mask-image: radial-gradient(70% 60% at 50% 30%, #000 30%, transparent 75%); -webkit-mask-image: radial-gradient(70% 60% at 50% 30%, #000 30%, transparent 75%); }
/* 极光：预渲染的径向渐变，靠 transform 缓慢漂移，不用 filter: blur */
.aurora { position: absolute; width: 760px; height: 760px; border-radius: 50%; will-change: transform; }
.a1 { top: -260px; left: -180px; background: radial-gradient(closest-side, rgba(24, 168, 136, .22), transparent); animation: drift1 18s ease-in-out infinite alternate; }
.a2 { top: -120px; right: -220px; background: radial-gradient(closest-side, rgba(99, 102, 241, .16), transparent); animation: drift2 22s ease-in-out infinite alternate; }
.dark .a1 { background: radial-gradient(closest-side, rgba(62, 209, 174, .2), transparent); }
.dark .a2 { background: radial-gradient(closest-side, rgba(129, 140, 248, .18), transparent); }
@keyframes drift1 { to { transform: translate(160px, 90px) scale(1.15); } }
@keyframes drift2 { to { transform: translate(-140px, 120px) scale(.9); } }

.hero-grid { display: grid; grid-template-columns: 1.05fr 1fr; gap: 40px; align-items: center; }
/* 允许文字列收窄，公告胶囊才能在列宽内截断 */
.hero-text { min-width: 0; }
.pill { display: inline-flex; align-items: center; gap: 8px; max-width: 100%; padding: 6px 14px 6px 10px; border-radius: 999px; font-size: 13px; color: var(--vp-c-brand-1); background: color-mix(in srgb, var(--vp-c-bg) 70%, transparent); border: 1px solid var(--vp-c-brand-soft); text-decoration: none; transition: border-color .2s, transform .2s; }
.pill:hover { border-color: var(--vp-c-brand-1); }
.pill-text { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pill .dot { flex: none; position: relative; width: 7px; height: 7px; border-radius: 50%; background: var(--vp-c-brand-3); }
.pill .dot::after { content: ''; position: absolute; inset: -4px; border-radius: 50%; border: 1px solid var(--vp-c-brand-3); animation: ring 2s ease-out infinite; }
@keyframes ring { from { transform: scale(.6); opacity: 1; } to { transform: scale(1.6); opacity: 0; } }
h1 { display: flex; flex-direction: column; font-size: 50px; text-wrap: balance; word-break: auto-phrase; line-height: 1.14; font-weight: 800; letter-spacing: -0.03em; margin: 22px 0 20px; }
.grad { background: linear-gradient(100deg, #12967d, #0b5f4f 30%, #6366f1 60%, #12967d 90%); background-size: 220% 100%; -webkit-background-clip: text; background-clip: text; color: transparent; animation: rise .9s var(--ease) both, sheen 9s linear infinite; animation-delay: var(--d, 0s), 1s; }
.dark .grad { background-image: linear-gradient(100deg, #5eead4, #3ed1ae 30%, #a5b4fc 60%, #5eead4 90%); }
@keyframes sheen { to { background-position: -220% 0; } }
h1 .sub { margin-top: 12px; font-size: .56em; line-height: 1.3; letter-spacing: -0.015em; }
.lead { text-wrap: pretty; font-size: 17px; line-height: 1.8; color: var(--vp-c-text-2); max-width: 560px; margin: 0; }
.meta { margin: 22px 0 0; font-size: 13px; color: var(--vp-c-text-3); font-family: var(--vp-font-family-mono); }
.cta { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 30px; }
.cta.center { justify-content: center; }

.btn { position: relative; display: inline-flex; align-items: center; gap: 7px; padding: 11px 22px; border-radius: 999px; font-weight: 600; font-size: 15px; text-decoration: none; color: var(--vp-c-text-1); background: color-mix(in srgb, var(--vp-c-bg-soft) 85%, transparent); border: 1px solid var(--vp-c-divider); transition: border-color .2s, color .2s, transform .2s var(--ease), box-shadow .2s; }
.btn:hover { border-color: var(--vp-c-brand-1); color: var(--vp-c-brand-1); transform: translateY(-1px); }
.btn.primary { color: #fff; background: linear-gradient(135deg, #12967d, #0b5f4f); border-color: transparent; box-shadow: 0 10px 26px -12px rgba(14, 124, 102, .8), inset 0 1px 0 rgba(255, 255, 255, .2); overflow: hidden; }
.dark .btn.primary { color: #04201a; background: linear-gradient(135deg, #5eead4, #26b896); }
.btn.primary:hover { color: #fff; box-shadow: 0 14px 32px -12px rgba(14, 124, 102, .9), inset 0 1px 0 rgba(255, 255, 255, .25); }
.dark .btn.primary:hover { color: #04201a; }
/* 主按钮悬停时一道高光扫过 */
.btn.primary::after { content: ''; position: absolute; inset: 0; background: linear-gradient(110deg, transparent 30%, rgba(255, 255, 255, .35) 50%, transparent 70%); transform: translateX(-120%); transition: transform .7s var(--ease); }
.btn.primary:hover::after { transform: translateX(120%); }
.btn .arr { transition: transform .25s var(--ease); }
.btn:hover .arr { transform: translateX(3px); }
.btn.ghost { background: transparent; }
.btn.lg { padding: 14px 30px; font-size: 16px; }

/* ---- 产品截图 ---- */
.product { margin-top: 44px; }
.product-bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 14px; }
.product-tabs { display: flex; gap: 6px; min-width: 0; overflow-x: auto; scrollbar-width: none; }
.product-tabs::-webkit-scrollbar { display: none; }
.product-tabs button { flex: none; padding: 7px 14px; border-radius: 999px; font-size: 13.5px; font-weight: 600; color: var(--vp-c-text-2); background: transparent; border: 1px solid transparent; cursor: pointer; transition: color .2s, border-color .2s, background .2s; }
.product-tabs button:hover { color: var(--vp-c-brand-1); }
.product-tabs button.on { color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); border-color: color-mix(in srgb, var(--vp-c-brand-1) 30%, transparent); }
.tour-btn { flex: none; display: inline-flex; align-items: center; gap: 7px; padding: 8px 16px; border-radius: 999px; font-size: 14px; font-weight: 600; color: var(--vp-c-text-1); background: var(--vp-c-bg-soft); border: 1px solid var(--vp-c-divider); cursor: pointer; transition: color .2s, border-color .2s; }
.tour-btn:hover { color: var(--vp-c-brand-1); border-color: var(--vp-c-brand-1); }
.tour-btn svg { color: var(--vp-c-brand-1); }
.product-frame { position: relative; }
.product-frame .frame-bar { align-items: center; padding: 8px 12px; }
.frame-title { margin-left: 10px; font-size: 12px; line-height: 1; color: var(--vp-c-text-3); font-family: var(--vp-font-family-mono); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* 按截图比例预留高度，图片加载前后页面不跳动 */
.product-shot { aspect-ratio: 16 / 10; background: var(--vp-c-bg-soft); }
.product-shot img { display: block; width: 100%; height: 100%; object-fit: cover; animation: fade .35s var(--ease) both; }
@keyframes fade { from { opacity: 0; } }
.dark .product-shot img { filter: brightness(.94); }

/* ---- 巡览视频 ---- */
.tour { width: min(1100px, 94vw, 144vh); max-width: none; padding: 0; border: 0; background: transparent; overflow: visible; overscroll-behavior: contain; }
.tour::backdrop { background: rgba(3, 10, 9, .78); }
.tour-box { position: relative; border-radius: 14px; overflow: hidden; background: #000; box-shadow: 0 40px 80px -30px rgba(0, 0, 0, .6); }
.tour video { display: block; width: 100%; height: auto; aspect-ratio: 16 / 10; }
.tour-close { position: absolute; z-index: 1; top: 10px; right: 10px; display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; color: #fff; background: rgba(0, 0, 0, .55); border: 1px solid rgba(255, 255, 255, .25); cursor: pointer; }
.tour-close:hover { background: rgba(0, 0, 0, .8); }

/* ---- 视频介绍 ---- */
.films { padding-top: 72px; }
.films .head { margin-bottom: 32px; }
.films-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 22px; }
.film { display: flex; flex-direction: column; gap: 6px; padding: 0; text-align: left; font: inherit; color: inherit; background: transparent; border: 0; cursor: pointer; }
.film-poster { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: 14px; overflow: hidden; border: 1px solid var(--vp-c-divider); background: #0b1a18; box-shadow: 0 24px 50px -30px rgba(11, 60, 50, .5); }
.film-poster img { display: block; width: 100%; height: 100%; object-fit: cover; transition: transform .5s var(--ease); }
.film:hover .film-poster img { transform: scale(1.03); }
.film-play { position: absolute; left: 50%; top: 50%; display: grid; place-items: center; width: 64px; height: 64px; margin: -32px 0 0 -32px; border-radius: 50%; color: #fff; background: rgba(14, 124, 102, .92); box-shadow: 0 10px 30px -8px rgba(0, 0, 0, .55); transition: transform .25s var(--ease); }
.film-play svg { margin-left: 3px; }
.film:hover .film-play { transform: scale(1.08); }
.film-dur { position: absolute; right: 10px; bottom: 10px; padding: 2px 8px; border-radius: 6px; font-size: 12.5px; font-family: var(--vp-font-family-mono); color: #fff; background: rgba(0, 0, 0, .62); }
.film-title { margin-top: 10px; font-size: 18px; font-weight: 700; color: var(--vp-c-text-1); }
.film:hover .film-title { color: var(--vp-c-brand-1); }
.film-text { font-size: 14px; line-height: 1.65; color: var(--vp-c-text-2); }
@media (max-width: 760px) {
  .films-grid { grid-template-columns: 1fr; }
}

/* ---- 能替代哪些工具 ---- */
.replaces { padding-top: 72px; padding-bottom: 56px; }
.replaces .head { margin-bottom: 32px; }
.rp-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; }
.rp { padding: 18px 20px 20px; border-radius: 14px; border: 1px solid var(--vp-c-divider); background: var(--vp-c-bg-soft); }
.rp-from { margin: 0; font-size: 13px; line-height: 1.5; color: var(--vp-c-text-3); }
.rp-arrow { display: block; margin: 8px 0 6px; color: var(--vp-c-brand-1); opacity: .7; }
.rp h3 { margin: 0 0 6px; font-size: 16px; font-weight: 700; color: var(--vp-c-brand-1); }
.rp-text { margin: 0; font-size: 14px; line-height: 1.65; color: var(--vp-c-text-2); }
.rp-note { margin: 24px auto 0; max-width: 720px; text-align: center; font-size: 14px; line-height: 1.7; color: var(--vp-c-text-2); }
.rp-note :deep(a) { color: var(--vp-c-brand-1); font-weight: 600; text-decoration: none; }
.rp-note :deep(a:hover) { text-decoration: underline; }

/* ---- marquee ---- */
.marquee { padding: 8px 0 12px; }
.marquee-cap { margin: 0 0 14px; text-align: center; font-size: 12px; letter-spacing: .16em; text-transform: uppercase; color: var(--vp-c-text-3); }
.marquee-view { overflow: hidden; mask-image: linear-gradient(90deg, transparent, #000 12%, #000 88%, transparent); -webkit-mask-image: linear-gradient(90deg, transparent, #000 12%, #000 88%, transparent); }
.marquee-track { display: flex; width: max-content; gap: 12px; animation: slide 60s linear infinite; }
.marquee-view:hover .marquee-track { animation-play-state: paused; }
.marquee-track span { padding: 8px 18px; border-radius: 999px; font-size: 14px; font-weight: 600; white-space: nowrap; color: var(--vp-c-text-2); border: 1px solid var(--vp-c-divider); background: var(--vp-c-bg-soft); }
@keyframes slide { to { transform: translateX(calc(-50% - 6px)); } }

/* ---- 选择理由 ---- */
.why { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; }
.why-card { position: relative; padding: 26px 22px; border-radius: 18px; border: 1px solid var(--vp-c-divider); background: linear-gradient(180deg, var(--vp-c-bg-soft), var(--vp-c-bg)); overflow: hidden; }
.why-card::before { content: ''; position: absolute; inset: 0; background: radial-gradient(260px circle at var(--mx, 50%) var(--my, 0%), var(--vp-c-brand-soft), transparent 65%); opacity: 0; transition: opacity .3s; pointer-events: none; }
.why-card:hover::before { opacity: 1; }
.why-card > * { position: relative; }
.why-ico { display: grid; place-items: center; width: 46px; height: 46px; border-radius: 14px; color: #fff; background: linear-gradient(135deg, #18a888, #0b5f4f); box-shadow: 0 10px 22px -10px rgba(14, 124, 102, .7); transition: transform .35s var(--ease); }
.dark .why-ico { color: #04201a; background: linear-gradient(135deg, #5eead4, #26b896); }
.why-card:hover .why-ico { transform: translateY(-3px) rotate(-6deg); }
.why-card h3 { margin: 18px 0 8px; font-size: 18px; font-weight: 700; }
.why-card p { margin: 0; font-size: 14px; line-height: 1.75; color: var(--vp-c-text-2); }

/* ---- 截图展示：左右交替 ---- */
.show { padding-top: 40px; }
.show-row { display: grid; grid-template-columns: 1fr 1.25fr; gap: 56px; align-items: center; padding: 64px 0; }
.show-row.flip { grid-template-columns: 1.25fr 1fr; }
.show-row.flip .show-text { order: 2; }
.show-row.phone { grid-template-columns: 1.25fr 1fr; }
.show-text .tag { display: inline-block; padding: 4px 12px; border-radius: 999px; font-size: 13px; font-weight: 600; color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); }
.show-text h2 { margin: 16px 0 14px; font-size: 32px; line-height: 1.3; font-weight: 800; letter-spacing: -0.02em; border: 0; padding: 0; text-wrap: balance; word-break: auto-phrase; }
.show-text p { margin: 0; color: var(--vp-c-text-2); line-height: 1.8; font-size: 16px; }
.show-text ul { list-style: none; margin: 22px 0 0; padding: 0; display: grid; gap: 12px; }
.show-text li { display: flex; gap: 10px; align-items: flex-start; font-size: 15px; line-height: 1.6; }
.show-text li svg { flex: none; margin-top: 2px; color: var(--vp-c-brand-1); }
/* 截图从侧面滑入：普通行从右侧，交替行从左侧 */
.js .show-media[data-r] { transform: translateX(40px) scale(.97); }
.js .show-row.flip .show-media[data-r] { transform: translateX(-40px) scale(.97); }
.js .show-media[data-r].in, .js .show-row.flip .show-media[data-r].in { transform: none; }
.show-media { position: relative; }
.show-media::before { content: ''; position: absolute; inset: 8% 6% -6%; z-index: -1; border-radius: 30px; background: radial-gradient(closest-side, rgba(24, 168, 136, .35), transparent), radial-gradient(closest-side at 80% 80%, rgba(99, 102, 241, .22), transparent); }
.frame { border-radius: 14px; overflow: hidden; border: 1px solid var(--vp-c-divider); background: var(--vp-c-bg); box-shadow: 0 30px 60px -30px rgba(11, 95, 79, .45), 0 2px 6px rgba(0, 0, 0, .04); transition: transform .5s var(--ease); }
.show-media:hover .frame { transform: translateY(-6px); }
.frame-bar { display: flex; gap: 6px; padding: 10px 12px; background: var(--vp-c-bg-soft); border-bottom: 1px solid var(--vp-c-divider); }
.frame-bar i { width: 9px; height: 9px; border-radius: 50%; background: var(--vp-c-divider); }
.frame img, .device img { display: block; width: 100%; height: auto; }
.device { width: min(300px, 78%); margin: 0 auto; padding: 10px; border-radius: 44px; background: #0b1211; box-shadow: 0 0 0 1px rgba(255, 255, 255, .08) inset, 0 40px 70px -30px rgba(11, 95, 79, .55); transition: transform .5s var(--ease); }
.device img { border-radius: 34px; }
.show-media:hover .device { transform: translateY(-6px) rotate(-1deg); }

/* ---- 功能分组标签 ---- */
.groups { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; margin: -12px auto 32px; }
.groups button { padding: 9px 18px; border-radius: 999px; font-size: 14px; font-weight: 600; color: var(--vp-c-text-2); background: var(--vp-c-bg-soft); border: 1px solid var(--vp-c-divider); cursor: pointer; transition: color .2s, border-color .2s, background .2s; }
.groups button:hover { color: var(--vp-c-brand-1); border-color: var(--vp-c-brand-1); }
.groups button.on { color: #fff; background: linear-gradient(135deg, #12967d, #0b5f4f); border-color: transparent; }
.dark .groups button.on { color: #04201a; background: linear-gradient(135deg, #5eead4, #26b896); }
.pop { animation: pop .5s var(--ease) both; animation-delay: calc(var(--i, 0) * 60ms); }
@keyframes pop { from { opacity: 0; transform: translateY(14px); } }

/* ---- feature cards ---- */
.grid { display: grid; gap: 16px; }
/* 每组卡片数量不同，用居中换行排布，避免最后一行孤零零靠左 */
.feat { display: flex; flex-wrap: wrap; justify-content: center; }
.feat .card { flex: 0 1 calc((100% - 32px) / 3); }
.card { position: relative; padding: 24px; border-radius: 16px; border: 1px solid var(--vp-c-divider); background: var(--vp-c-bg); overflow: hidden; }
.card { transition: transform .3s var(--ease), border-color .3s, box-shadow .3s; }
/* 跟随鼠标的光斑 */
.card::before { content: ''; position: absolute; inset: 0; background: radial-gradient(320px circle at var(--mx, 50%) var(--my, 0%), var(--vp-c-brand-soft), transparent 60%); opacity: 0; transition: opacity .3s; pointer-events: none; }
.card:hover::before { opacity: 1; }
.card:hover { border-color: color-mix(in srgb, var(--vp-c-brand-1) 45%, var(--vp-c-divider)); box-shadow: 0 18px 40px -24px rgba(11, 95, 79, .45); }
.card:hover { transform: translateY(-4px); }
.card > * { position: relative; }
.ico { display: grid; place-items: center; width: 40px; height: 40px; border-radius: 11px; color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); transition: transform .3s var(--ease); }
.card:hover .ico { transform: rotate(-8deg) scale(1.06); }
.card h3 { margin: 16px 0 8px; font-size: 17px; font-weight: 700; }
.card p { margin: 0; font-size: 14px; line-height: 1.75; color: var(--vp-c-text-2); }

/* ---- fnOS ---- */
.fnos { position: relative; display: grid; grid-template-columns: 1fr 1.15fr; gap: 44px; align-items: center; padding: 48px; border-radius: 24px; border: 1px solid var(--vp-c-divider); overflow: hidden; background:
  radial-gradient(60% 90% at 0% 0%, var(--vp-c-brand-soft), transparent 70%),
  radial-gradient(50% 70% at 100% 100%, rgba(99, 102, 241, .08), transparent 70%), var(--vp-c-bg-soft); }
.fnos .tag { display: inline-block; padding: 4px 12px; border-radius: 999px; font-size: 13px; font-weight: 600; color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); }
.fnos h2 { margin: 16px 0 12px; font-size: 34px; line-height: 1.25; font-weight: 800; letter-spacing: -0.02em; border: 0; padding: 0; }
.fnos-text p { margin: 0; color: var(--vp-c-text-2); line-height: 1.8; }
.fnos-points { display: grid; gap: 14px; }
.fnos-points .card { display: grid; grid-template-columns: 40px 1fr; gap: 16px; padding: 18px 20px; }
.fnos-points .card h3 { margin: 0 0 6px; }

/* ---- 深色区：两种主题下都用深底，营造控制台氛围 ---- */
.night { position: relative; margin-top: 104px; padding: 104px 0 88px; color: #e6f2ef; background: #050d0c; isolation: isolate; overflow: hidden; }
.night-bg { position: absolute; inset: 0; z-index: -1; background:
  radial-gradient(50% 40% at 50% 0%, rgba(62, 209, 174, .16), transparent 70%),
  radial-gradient(40% 40% at 90% 70%, rgba(129, 140, 248, .1), transparent 70%); }
.night-bg::before { content: ''; position: absolute; inset: 0; background-image: linear-gradient(rgba(62, 209, 174, .06) 1px, transparent 1px), linear-gradient(90deg, rgba(62, 209, 174, .06) 1px, transparent 1px); background-size: 48px 48px; mask-image: linear-gradient(#000, transparent 70%); -webkit-mask-image: linear-gradient(#000, transparent 70%); }
/* 顶部一道扫描光 */
.night-bg::after { content: ''; position: absolute; left: 0; right: 0; top: 0; height: 1px; background: linear-gradient(90deg, transparent, #3ed1ae, transparent); opacity: .7; }
.night .head p { color: #9fb8b2; }
.night .head p :deep(code) { color: #c8f3e8; background: rgba(62, 209, 174, .12); }
.night .kicker { color: #3ed1ae; }
.night .more a { color: #3ed1ae; }

/* terminal */
.term { max-width: 860px; margin: 0 auto; border-radius: 16px; overflow: hidden; background: #0a1514; border: 1px solid rgba(62, 209, 174, .2); box-shadow: 0 40px 80px -40px rgba(62, 209, 174, .35), 0 0 0 1px rgba(0, 0, 0, .4); }
.tabs { display: flex; align-items: center; gap: 2px; padding: 8px 8px 0; background: #0f1d1b; overflow-x: auto; }
.lights { display: inline-flex; gap: 6px; padding: 0 12px 8px 6px; }
.lights i { width: 10px; height: 10px; border-radius: 50%; background: #ff5f57; }
.lights i:nth-child(2) { background: #febc2e; }
.lights i:nth-child(3) { background: #28c840; }
.tabs button { flex: none; padding: 8px 16px; font-size: 13px; color: #7f9a94; background: transparent; border: 0; border-radius: 8px 8px 0 0; cursor: pointer; transition: color .2s; }
.tabs button:hover { color: #c8f3e8; }
.tabs button.on { color: #e6fffa; background: #0a1514; box-shadow: inset 0 2px 0 #3ed1ae; }
.tabs .copy { margin-left: auto; display: inline-flex; align-items: center; gap: 6px; border-radius: 8px; }
.term pre { margin: 0; padding: 22px 24px 26px; overflow-x: auto; }
.term code { font-family: var(--vp-font-family-mono); font-size: 13.5px; line-height: 1.8; color: #c8f3e8; white-space: pre; }
.ln { display: inline; animation: type .45s var(--ease) both; animation-delay: calc(var(--i) * 55ms); }
.ln.cm { color: #5f7f78; }
@keyframes type { from { opacity: 0; } }
.caret { display: inline-block; width: 8px; height: 1.1em; margin-left: 4px; vertical-align: text-bottom; background: #3ed1ae; animation: blink 1.1s steps(2, start) infinite; }
@keyframes blink { 50% { opacity: 0; } }

/* ---- faq：用 grid 行高过渡做平滑展开 ---- */
.faq { border-top: 1px solid var(--vp-c-divider); }
.qa { border-bottom: 1px solid var(--vp-c-divider); }
.qa button { width: 100%; display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 20px 4px; font-size: 16px; font-weight: 600; text-align: left; color: var(--vp-c-text-1); background: none; border: 0; cursor: pointer; transition: color .2s; }
.qa button:hover { color: var(--vp-c-brand-1); }
.pm { position: relative; flex: none; width: 14px; height: 14px; }
.pm::before, .pm::after { content: ''; position: absolute; left: 0; top: 6px; width: 14px; height: 2px; border-radius: 2px; background: var(--vp-c-brand-1); transition: transform .3s var(--ease); }
.pm::after { transform: rotate(90deg); }
.qa.open .pm::after { transform: rotate(0deg); }
.ans { display: grid; grid-template-rows: 0fr; transition: grid-template-rows .35s var(--ease); }
.ans p { overflow: hidden; margin: 0; padding: 0 4px; line-height: 1.8; color: var(--vp-c-text-2); }
.qa.open .ans { grid-template-rows: 1fr; }
.qa.open .ans p { padding-bottom: 20px; }

/* ---- final：旋转的渐变描边，只旋转伪元素 ---- */
.final { position: relative; padding: 1px; border-radius: 26px; overflow: hidden; margin-bottom: 64px; isolation: isolate; }
.final::before { content: ''; position: absolute; left: 50%; top: 50%; width: 200%; aspect-ratio: 1; z-index: -1; background: conic-gradient(from 0deg, transparent 0 55%, #3ed1ae 70%, #6366f1 85%, transparent 100%); transform: translate(-50%, -50%); animation: turn 8s linear infinite; }
@keyframes turn { to { transform: translate(-50%, -50%) rotate(360deg); } }
.final-inner { text-align: center; padding: 64px 24px 56px; border-radius: 25px; background:
  radial-gradient(70% 120% at 50% 0%, var(--vp-c-brand-soft), transparent 70%), var(--vp-c-bg-soft); }
.final h2 { margin: 0; font-size: 38px; font-weight: 800; letter-spacing: -0.02em; border: 0; padding: 0; }
.final p { margin: 14px 0 0; color: var(--vp-c-text-2); font-size: 16px; }
.tg { display: inline-flex; flex-direction: column; align-items: center; gap: 10px; margin-top: 36px; color: var(--vp-c-text-2); font-size: 14px; text-decoration: none; }
.tg:hover { color: var(--vp-c-brand-1); }
.tg-qr { display: block; border-radius: 12px; border: 1px solid var(--vp-c-divider); background: #fff; transition: transform .3s var(--ease); }
.tg:hover .tg-qr { transform: scale(1.04); }

/* ---- 减少动态效果：关掉循环动画与位移 ---- */
@media (prefers-reduced-motion: reduce) {
  .gl *, .gl *::before, .gl *::after { animation: none !important; transition: none !important; }
  .js [data-r] { opacity: 1 !important; transform: none !important; }
  .marquee-track { flex-wrap: wrap; justify-content: center; width: auto; }
  .marquee-track span[aria-hidden] { display: none; }
}

@media (max-width: 960px) {
  .hero { padding-top: 40px; }
  .hero-grid { grid-template-columns: 1fr; }
  h1 { font-size: 42px; }
  .why { grid-template-columns: repeat(2, 1fr); }
  .rp-grid { grid-template-columns: repeat(2, 1fr); }
  .feat .card { flex-basis: calc((100% - 16px) / 2); }
  .show-row, .show-row.flip, .show-row.phone { grid-template-columns: 1fr; gap: 32px; padding: 48px 0; }
  .show-row.flip .show-text { order: 0; }
  .fnos { grid-template-columns: 1fr; padding: 32px; }
}
@media (max-width: 600px) {
  /* 手机上省掉示意动画，让真实截图紧跟在按钮之后 */
  .hero-visual { display: none; }
  .product { margin-top: 36px; }
  .product-bar { flex-direction: column-reverse; align-items: stretch; }
  .hero .cta { gap: 8px; margin-top: 24px; }
  .hero .cta .btn { padding: 10px 16px; font-size: 14px; }
  .tour-btn { justify-content: center; }
  h1 { font-size: 28px; }
  h1 .sub { margin-top: 8px; font-size: .68em; }
  .lead { font-size: 15px; line-height: 1.75; }
  .block { padding-top: 72px; }
  .why { grid-template-columns: 1fr; }
  .rp-grid { grid-template-columns: 1fr; gap: 10px; }
  .rp { padding: 14px 16px 16px; }
  .rp-arrow { margin: 4px 0 2px; }
  .replaces { padding-top: 56px; padding-bottom: 40px; }
  .feat .card { flex-basis: 100%; }
  .show-text h2 { font-size: 26px; }
  .head h2, .final h2, .fnos h2 { font-size: 26px; }
  .fnos { padding: 22px; }
  .night { margin-top: 80px; padding: 80px 0 64px; }
}
</style>
