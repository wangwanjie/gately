<script setup lang="ts">
import { computed, ref } from 'vue'
import { useData } from 'vitepress'
import { copyText } from '../clipboard'

const { frontmatter, lang } = useData()
// 文案由 config.mts 的 transformPageData 按页面语言注入
const s = computed(() => frontmatter.value.siteText.sponsor)

// 收款渠道集中在这里维护，各语言页面共用；换码或换地址只改这一处
const AFDIAN = 'https://ifdian.net/a/gately'
const EVM = '0x17b699795b2dfacbe10b76d77d9bf50340cc54be'
const wallets = [
  { network: 'TRC20 (Tron)', address: 'TQMuNoHmcUAZb8swHenWuZBVrSznahen2f' },
  { network: 'ERC20 (Ethereum)', address: EVM },
  { network: 'Arbitrum One', address: EVM }
]
// 地址二维码（内容为纯地址）；ERC20 与 Arbitrum One 地址相同，共用一张
const cryptoQrs = [
  { src: '/donate/usdt-trc20.png', label: 'USDT · TRC20' },
  { src: '/donate/usdt-evm.png', label: 'USDT · ERC20 / Arbitrum One' }
]

// 中文用户优先展示国内渠道，其他语言优先展示 USDT
const cryptoFirst = computed(() => !String(lang.value).toLowerCase().startsWith('zh'))

const copied = ref('')
let timer: ReturnType<typeof setTimeout> | undefined

// 复制钱包地址；失败时选中按钮前面的地址文本，不显示「已复制」
async function copy(key: string, text: string, e: MouseEvent) {
  const addr = (e.currentTarget as HTMLElement).previousElementSibling as HTMLElement | null
  if (!(await copyText(text, addr))) {
    return
  }
  copied.value = key
  clearTimeout(timer)
  timer = setTimeout(() => {
    copied.value = ''
  }, 1800)
}
</script>

<template>
  <div class="sp" :class="{ 'crypto-first': cryptoFirst }">
    <section class="card cn">
      <h2>{{ s.cnTitle }}</h2>
      <p class="desc">{{ s.cnDesc }}</p>
      <div class="afdian">
        <a class="btn" :href="AFDIAN" target="_blank" rel="noreferrer">{{ s.afdianButton }}</a>
        <span class="muted">{{ s.afdianDesc }}</span>
      </div>
      <div class="qrs">
        <!-- 爱发电二维码内容为赞助主页链接，扫码不会暴露收款人实名 -->
        <figure>
          <img src="/donate/afdian.png" :alt="s.afdianQr" width="240" height="304" loading="lazy" />
          <figcaption>{{ s.afdianQr }}</figcaption>
        </figure>
        <figure>
          <img src="/donate/wechat.png" :alt="s.wechat" width="240" height="304" loading="lazy" />
          <figcaption>{{ s.wechat }}</figcaption>
        </figure>
      </div>
    </section>

    <section class="card crypto">
      <h2>{{ s.cryptoTitle }}</h2>
      <p class="desc">{{ s.cryptoDesc }}</p>
      <ul class="wallets">
        <li v-for="w in wallets" :key="w.network">
          <span class="net">{{ w.network }}</span>
          <code class="addr">{{ w.address }}</code>
          <button type="button" class="copy" @click="copy(w.network, w.address, $event)">
            {{ copied === w.network ? s.copied : s.copy }}
          </button>
        </li>
      </ul>
      <div class="qrs crypto-qrs">
        <figure v-for="q in cryptoQrs" :key="q.src">
          <img :src="q.src" :alt="q.label" width="180" height="180" loading="lazy" />
          <figcaption>{{ q.label }}</figcaption>
        </figure>
      </div>
      <p class="warn">{{ s.cryptoWarn }}</p>
    </section>

    <p class="note">{{ s.note }}</p>
  </div>
</template>

<style scoped>
.sp { display: flex; flex-direction: column; gap: 20px; margin: 24px 0; }
.sp.crypto-first .crypto { order: -1; }
.card { padding: 20px 22px; border-radius: 14px; border: 1px solid var(--vp-c-divider); background: var(--vp-c-bg-soft); }
.card h2 { margin: 0 0 6px; padding: 0; border: 0; font-size: 18px; }
.desc { margin: 0 0 14px; color: var(--vp-c-text-2); font-size: 14px; line-height: 1.7; }
.afdian { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-bottom: 16px; }
.btn { display: inline-block; padding: 8px 20px; border-radius: 999px; background: #946ce6; color: #fff !important; font-weight: 600; text-decoration: none !important; }
.btn:hover { opacity: 0.88; }
.muted { color: var(--vp-c-text-2); font-size: 13px; }
.qrs { display: flex; flex-wrap: wrap; gap: 20px; }
.qrs figure { margin: 0; text-align: center; }
.qrs img { width: 200px; height: auto; border-radius: 12px; }
.crypto-qrs { margin-top: 16px; }
.crypto-qrs img { width: 160px; border-radius: 10px; }
.qrs figcaption { margin-top: 6px; font-size: 13px; color: var(--vp-c-text-2); }
.wallets { list-style: none; margin: 0; padding: 0; border: 1px solid var(--vp-c-divider); border-radius: 12px; overflow: hidden; background: var(--vp-c-bg); }
.wallets li { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--vp-c-divider); }
.wallets li:last-child { border-bottom: 0; }
.net { min-width: 140px; font-size: 13px; font-weight: 600; color: var(--vp-c-brand-1); }
.addr { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 13px; }
.copy { padding: 3px 12px; border-radius: 999px; border: 1px solid var(--vp-c-divider); font-size: 12px; color: var(--vp-c-text-1); background: var(--vp-c-bg-soft); cursor: pointer; }
.copy:hover { border-color: var(--vp-c-brand-1); color: var(--vp-c-brand-1); }
.warn { margin: 12px 0 0; font-size: 13px; color: var(--vp-c-warning-1); line-height: 1.7; }
.note { margin: 0; color: var(--vp-c-text-2); font-size: 14px; line-height: 1.7; }
</style>
