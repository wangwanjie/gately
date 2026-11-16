<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useData } from 'vitepress'

interface Asset {
  name: string
  size: number
  browser_download_url: string
  download_count: number
}
interface Release {
  tag_name: string
  published_at: string
  html_url: string
  assets: Asset[]
}

const { frontmatter, lang } = useData()
// 文案由 config.mts 的 transformPageData 按页面语言注入
const d = computed(() => frontmatter.value.siteText.download)
const REPO = 'wangwanjie/gately'
const release = ref<Release | null>(null)
const loading = ref(true)
const failed = ref(false)

// 把文件名里的系统/架构识别成可读标签，便于用户挑选
function describe(name: string, other: string): { os: string; arch: string } {
  const n = name.toLowerCase()
  // 飞牛 fnOS 原生应用安装包（.fpk）与 OpenWrt 安装包（.ipk / .apk）各自成组
  const os = n.includes('fnos') || n.endsWith('.fpk')
    ? 'fnOS'
    : n.includes('openwrt') || n.endsWith('.ipk') || n.endsWith('.apk')
      ? 'OpenWrt'
      : n.includes('darwin') || n.includes('macos')
        ? 'macOS'
        : n.includes('windows')
          ? 'Windows'
          : n.includes('freebsd')
            ? 'FreeBSD'
            : n.includes('linux')
              ? 'Linux'
              : other
  // LuCI 插件 luci-app-gately 与架构无关，标记为 LuCI 以便和程序安装包区分
  if (n.startsWith('luci-app-')) {
    return { os, arch: 'LuCI' }
  }
  const arch = ['amd64', 'x86_64', 'x86', 'arm64', 'aarch64', 'armv7', 'armv6', 'arm', 'mipsle', 'mips64le', 'mips', 'riscv64', '386']
    .find((a) => n.includes(a)) ?? ''
  return { os, arch }
}

const assets = computed(() => {
  const list = (release.value?.assets ?? []).filter((a) => !/\.(sha256|sig|txt|pem)$/i.test(a.name) && !/checksums/i.test(a.name))
  return list.map((a) => ({ ...a, ...describe(a.name, d.value.other) }))
})
const groups = computed(() => {
  const map = new Map<string, typeof assets.value>()
  for (const a of assets.value) {
    if (!map.has(a.os)) {
      map.set(a.os, [])
    }
    map.get(a.os)!.push(a)
  }
  return [...map.entries()]
})
const checksum = computed(() => (release.value?.assets ?? []).find((a) => /checksums/i.test(a.name)))

// 把带 {releases} / {image} 占位的提示拆成片段，占位处渲染为链接与代码
const failedParts = computed(() => String(d.value.failed).split(/(\{releases\}|\{image\})/).filter(Boolean))

function size(n: number) {
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

onMounted(async () => {
  try {
    const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`)
    if (!res.ok) {
      throw new Error(String(res.status))
    }
    release.value = await res.json()
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="dl">
    <div v-if="loading" class="tip">{{ d.loading }}</div>
    <div v-else-if="failed || !release" class="tip">
      <template v-for="(part, i) in failedParts" :key="i">
        <a v-if="part === '{releases}'" :href="`https://github.com/${REPO}/releases`" target="_blank" rel="noreferrer">GitHub Releases</a>
        <code v-else-if="part === '{image}'">ghcr.io/wangwanjie/gately:latest</code>
        <template v-else>{{ part }}</template>
      </template>
    </div>
    <template v-else>
      <div class="ver">
        <strong>{{ release.tag_name }}</strong>
        <span>{{ d.publishedAt.replace('{date}', new Date(release.published_at).toLocaleDateString(lang)) }}</span>
        <a :href="release.html_url" target="_blank" rel="noreferrer">{{ d.releaseNotes }}</a>
        <a v-if="checksum" :href="checksum.browser_download_url" target="_blank" rel="noreferrer">{{ d.checksums }}</a>
      </div>
      <section v-for="[os, list] in groups" :key="os" class="grp">
        <h3>{{ os }}</h3>
        <ul>
          <li v-for="a in list" :key="a.name">
            <a :href="a.browser_download_url" target="_blank" rel="noreferrer">{{ a.name }}</a>
            <span class="arch" v-if="a.arch">{{ a.arch }}</span>
            <span class="sz">{{ size(a.size) }}</span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<style scoped>
.dl { margin: 20px 0; }
.tip { padding: 16px 18px; border-radius: 12px; background: var(--vp-c-bg-soft); border: 1px solid var(--vp-c-divider); line-height: 1.8; }
.ver { display: flex; flex-wrap: wrap; align-items: baseline; gap: 14px; margin-bottom: 8px; }
.ver strong { font-size: 24px; color: var(--vp-c-brand-1); }
.ver span { color: var(--vp-c-text-2); font-size: 14px; }
.grp h3 { margin: 24px 0 8px; font-size: 16px; border: 0; padding: 0; }
.grp ul { list-style: none; margin: 0; padding: 0; border: 1px solid var(--vp-c-divider); border-radius: 12px; overflow: hidden; }
.grp li { display: flex; align-items: center; gap: 12px; padding: 11px 16px; border-bottom: 1px solid var(--vp-c-divider); font-size: 14px; }
.grp li:last-child { border-bottom: 0; }
.grp li a { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.arch { padding: 2px 10px; border-radius: 999px; font-size: 12px; color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); }
.sz { color: var(--vp-c-text-3); font-size: 13px; min-width: 64px; text-align: right; }
</style>
