// IP 归属地的查询与缓存：各处显示 IP 的组件只管读，同一时刻出现的地址合并成一次批量查询（/api/geoip/lookup），
// 结果按地址缓存在内存里。是否显示归属地按浏览器记在 localStorage，默认显示。
// 没有导入 GeoIP 数据库时接口只标出内网地址，其他地址没有结果。
import { reactive, ref, watch } from 'vue'
import { api } from '../api'
import { i18n } from '../i18n'
import type { GeoLoc } from '../ipGeo'

const storageKey = 'gately.ipGeo'
// 每批最多查询的地址数（接口上限 200），批与批依次发出
const batchSize = 100
// 缓存上限，超过后整体清空，仍在显示的地址会重新查询
const cacheLimit = 5000

function readEnabled(): boolean {
  try {
    return localStorage.getItem(storageKey) !== '0'
  } catch {
    return true
  }
}

// ipGeoEnabled 为是否在 IP 旁显示归属地（本浏览器）
export const ipGeoEnabled = ref(readEnabled())
watch(ipGeoEnabled, (v) => {
  try {
    localStorage.setItem(storageKey, v ? '1' : '0')
  } catch {
    // 写入失败时仅本次会话生效
  }
})

// cache 中 null 表示已查过但库中未收录
const cache = reactive(new Map<string, GeoLoc | null>())
const queued = new Set<string>()
let timer: ReturnType<typeof setTimeout> | undefined
let running = false
// generation 在清空缓存（切换语言、更换数据库）时递增，丢弃此前发出的请求的结果
let generation = 0

function currentLocale(): string {
  return String(i18n.global.locale.value)
}

async function flush() {
  timer = undefined
  if (running) {
    return
  }
  running = true
  try {
    while (queued.size) {
      const batch = [...queued].slice(0, batchSize)
      batch.forEach((k) => queued.delete(k))
      const gen = generation
      const qs = batch.map((k) => 'ip=' + encodeURIComponent(k)).join('&')
      let res: { items: Record<string, GeoLoc> }
      try {
        res = await api.get(`/api/geoip/lookup?lang=${encodeURIComponent(currentLocale())}&${qs}`)
      } catch {
        // 查询失败（网络中断、旧版本后端没有该接口）时不显示，也不反复重试
        if (gen === generation) {
          batch.forEach((k) => cache.set(k, null))
        }
        continue
      }
      if (gen !== generation) {
        continue
      }
      if (cache.size + batch.length > cacheLimit) {
        cache.clear()
      }
      for (const k of batch) {
        cache.set(k, res.items?.[k] ?? null)
      }
    }
  } finally {
    running = false
    if (queued.size) {
      schedule()
    }
  }
}

function schedule() {
  if (!timer) {
    timer = setTimeout(flush, 30)
  }
}

// geoOf 返回地址的归属地；尚未查询时排队查询并先返回 undefined（结果到达后依赖它的组件自动刷新）。
// key 应为 lookupKey 处理过的地址。
export function geoOf(key: string): GeoLoc | undefined {
  if (!key || !ipGeoEnabled.value) {
    return undefined
  }
  const v = cache.get(key)
  if (v !== undefined) {
    return v ?? undefined
  }
  if (!queued.has(key)) {
    queued.add(key)
    schedule()
  }
  return undefined
}

// resetIpGeo 清空缓存，导入、更换或移除 GeoIP 数据库后调用，显示中的地址会重新查询
export function resetIpGeo() {
  generation++
  cache.clear()
  queued.clear()
}

// 切换界面语言后省市名称的语言随之变化
watch(() => i18n.global.locale.value, resetIpGeo)
