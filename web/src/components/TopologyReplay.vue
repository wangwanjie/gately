<template>
  <!-- 拓扑回放控制条：时间轴（按流量画出面积图，红色为有异常的时段）、播放控制与录制设置。
       取到的历史快照通过 frame 事件交给拓扑图，拓扑图用与实时视图相同的方式渲染 -->
  <div class="replay" :class="{ compact }">
    <template v-if="!index">
      <div v-loading="true" class="rp-loading" />
    </template>
    <!-- 未开启录制 -->
    <div v-else-if="!index.settings.enable" class="rp-off">
      <History :size="16" class="rp-off-icon" />
      <div class="rp-off-text">
        <div class="rp-off-title">{{ t('topologyReplay.offTitle') }}</div>
        <div class="rp-off-desc">{{ t('topologyReplay.offDesc', { mb: index.settings.sizeMB }) }}</div>
      </div>
      <div class="rp-off-actions">
        <el-button v-if="!isReadonly" type="primary" size="small" :loading="saving" @click="enableNow">{{ t('topologyReplay.enable') }}</el-button>
        <span v-else class="rp-muted">{{ t('topologyReplay.readonlyOff') }}</span>
        <el-button size="small" :icon="X" @click="emit('close')">{{ t('topologyReplay.backLive') }}</el-button>
      </div>
    </div>
    <!-- 已开启但还没有数据 -->
    <div v-else-if="!index.frames" class="rp-off">
      <span class="rec-dot" />
      <div class="rp-off-text">
        <div class="rp-off-title">{{ t('topologyReplay.waitTitle') }}</div>
        <div class="rp-off-desc">{{ t('topologyReplay.waitDesc', { s: index.step }) }}</div>
      </div>
      <div class="rp-off-actions">
        <el-button size="small" :icon="Settings2" @click="settingsOpen = true">{{ t('topologyReplay.settings') }}</el-button>
        <el-button size="small" :icon="X" @click="emit('close')">{{ t('topologyReplay.backLive') }}</el-button>
      </div>
    </div>
    <template v-else>
      <div class="rp-controls">
        <el-tooltip :content="playing ? t('topologyReplay.pause') : t('topologyReplay.play')" placement="top" :disabled="noHover">
          <button type="button" class="rp-btn play" :aria-label="playing ? t('topologyReplay.pause') : t('topologyReplay.play')" @click="togglePlay">
            <component :is="playing ? Pause : Play" :size="16" />
          </button>
        </el-tooltip>
        <el-tooltip :content="t('topologyReplay.stepBack')" placement="top" :disabled="noHover">
          <button type="button" class="rp-btn" :disabled="!meta?.prev" :aria-label="t('topologyReplay.stepBack')" @click="step(-1)"><StepBack :size="15" /></button>
        </el-tooltip>
        <el-tooltip :content="t('topologyReplay.stepForward')" placement="top" :disabled="noHover">
          <button type="button" class="rp-btn" :disabled="!meta?.next" :aria-label="t('topologyReplay.stepForward')" @click="step(1)"><StepForward :size="15" /></button>
        </el-tooltip>
        <div class="rp-time" :title="fullTime(cursor)">
          <span class="rp-clock mono">{{ clock(cursor) }}</span>
          <span class="rp-ago">{{ agoText }}</span>
        </div>
      </div>

      <!-- 时间轴：点击或拖动定位，悬停显示时刻与当时的 QPS；键盘左右键逐帧移动 -->
      <div
        ref="trackEl"
        class="rp-track"
        role="slider"
        tabindex="0"
        :aria-label="t('topologyReplay.timeline')"
        :aria-valuemin="range.from"
        :aria-valuemax="range.to"
        :aria-valuenow="cursor"
        :aria-valuetext="fullTime(cursor)"
        @pointerdown="onTrackDown"
        @pointermove="onTrackMove"
        @pointerup="onTrackUp"
        @pointercancel="onTrackUp"
        @pointerleave="hoverX = -1"
        @keydown.left.prevent="step(-1)"
        @keydown.right.prevent="step(1)"
      >
        <svg class="rp-svg" :width="trackW" :height="trackH" :viewBox="`0 0 ${trackW} ${trackH}`" aria-hidden="true">
          <rect v-for="(g, i) in geom.gaps" :key="'g' + i" class="rp-gap" :x="g.x" y="0" :width="Math.max(1, g.w)" :height="trackH" />
          <path class="rp-area" :d="geom.area" />
          <path class="rp-line" :d="geom.line" />
          <rect v-for="(s, i) in geom.issues" :key="'i' + i" class="rp-issue" :x="s.x" :y="trackH - 4" :width="s.w" height="4" />
          <line v-if="hoverX >= 0" class="rp-hover" :x1="hoverX" :x2="hoverX" y1="0" :y2="trackH" />
          <line class="rp-cursor" :x1="cursorX" :x2="cursorX" y1="0" :y2="trackH" />
        </svg>
        <span class="rp-knob" :style="{ left: cursorX + 'px' }" />
        <div v-if="hoverX >= 0 && hoverInfo" class="rp-tip" :style="{ left: tipLeft + 'px' }">
          <span class="mono">{{ clock(hoverInfo.t) }}</span>
          <span v-if="hoverInfo.b">{{ formatQps(hoverInfo.b.qps) }}/s · {{ hoverInfo.b.ips }} IP</span>
          <span v-else class="rp-muted">{{ t('topologyReplay.noData') }}</span>
        </div>
        <span class="rp-edge l mono">{{ clock(range.from) }}</span>
        <span class="rp-edge r mono">{{ clock(range.to) }}</span>
      </div>

      <div class="rp-options">
        <el-select v-model="speed" size="small" class="rp-speed" :aria-label="t('topologyReplay.speed')">
          <el-option v-for="s in REPLAY_SPEEDS" :key="s" :value="s" :label="s + '×'" />
        </el-select>
        <el-select v-model="rangeSec" size="small" class="rp-range" :aria-label="t('topologyReplay.range')">
          <el-option v-for="r in REPLAY_RANGES" :key="r" :value="r" :label="rangeLabel(r)" />
        </el-select>
        <el-tooltip :content="t('topologyReplay.skipIdleHelp')" placement="top" :disabled="noHover">
          <el-checkbox v-model="skipIdle" size="small" class="rp-skip">{{ t('topologyReplay.skipIdle') }}</el-checkbox>
        </el-tooltip>
        <el-tooltip :content="t('topologyReplay.settings')" placement="top" :disabled="noHover">
          <button type="button" class="rp-btn" :aria-label="t('topologyReplay.settings')" @click="settingsOpen = true"><Settings2 :size="15" /></button>
        </el-tooltip>
        <el-button size="small" type="primary" plain class="rp-live" @click="emit('close')">
          <Radio :size="14" class="btn-ic" />{{ t('topologyReplay.backLive') }}
        </el-button>
      </div>
    </template>
    <div v-if="loadError" class="rp-error">{{ loadError }}</div>

    <!-- 录制设置 -->
    <el-dialog v-model="settingsOpen" :title="t('topologyReplay.settingsTitle')" width="440px" append-to-body class="rp-dialog" @open="resetForm">
      <el-form label-position="top" @submit.prevent>
        <el-form-item>
          <el-switch v-model="form.enable" :disabled="isReadonly" :active-text="t('topologyReplay.record')" />
          <div class="rp-hint">{{ t('topologyReplay.recordHint', { s: index?.step ?? 5 }) }}</div>
        </el-form-item>
        <el-form-item :label="t('topologyReplay.size')">
          <el-input-number v-model="form.sizeMB" :min="1" :max="REPLAY_MAX_MB" :step="1" step-strictly :disabled="isReadonly" controls-position="right" />
          <span class="rp-unit">MiB</span>
          <div class="rp-hint">{{ t('topologyReplay.sizeHint') }}</div>
        </el-form-item>
        <div v-if="index?.frames" class="rp-usage">
          <div>{{ t('topologyReplay.usage', { used: formatBytes(index.used), limit: formatBytes(index.limit) }) }}</div>
          <el-progress :percentage="usagePct" :show-text="false" :stroke-width="6" />
          <div class="rp-muted">{{ t('topologyReplay.usageSpan', { span: spanText(index.end - index.start + index.step), frames: index.frames, samples: index.samples }) }}</div>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="settingsOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button v-if="!isReadonly" type="primary" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { History, Pause, Play, Radio, Settings2, StepBack, StepForward, X } from 'lucide-vue-next'
import { api, formatBytes } from '../api'
import { isReadonly } from '../nav'
import { formatQps, type TopologyView } from '../topology'
import {
  REPLAY_DEFAULT_SPEED,
  REPLAY_MAX_MB,
  REPLAY_RANGES,
  REPLAY_SPEEDS,
  bucketAt,
  formatClock,
  frameIdle,
  nextCursor,
  playTick,
  sameDay,
  spanParts,
  timeToX,
  timeline,
  xToTime,
  type ReplayIndex,
  type ReplaySettings,
} from '../topologyReplay'

// focus 为拓扑详情面板选中的节点，回放帧据此附带该节点的曲线
const props = defineProps<{ focus: string }>()
// frame 为取到的回放快照；empty 表示没有可回放的数据（录制未开启、刚开启或已清空），拓扑图据此回到实时视图
const emit = defineEmits<{ frame: [v: TopologyView]; empty: []; close: [] }>()
const { t } = useI18n()

const noHover = typeof window !== 'undefined' && !!window.matchMedia?.('(hover: none)').matches

const index = ref<ReplayIndex>()
const loadError = ref('')
const cursor = ref(0)
const meta = ref<TopologyView['replay']>()
const playing = ref(false)
const lastFrame = ref<TopologyView>()

// 播放速度、时间轴范围与跳过空闲在本机记住
const speedKey = 'gately.topology.replaySpeed'
const rangeKey = 'gately.topology.replayRange'
const skipKey = 'gately.topology.replaySkipIdle'
function readNum(key: string, allowed: number[], def: number): number {
  try {
    const v = Number(localStorage.getItem(key))
    return allowed.includes(v) ? v : def
  } catch {
    return def
  }
}
function save(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    // 隐私模式等无法写入时忽略
  }
}
const speed = ref(readNum(speedKey, REPLAY_SPEEDS, REPLAY_DEFAULT_SPEED))
const rangeSec = ref(readNum(rangeKey, REPLAY_RANGES, 0))
const skipIdle = ref((() => {
  try {
    return localStorage.getItem(skipKey) !== '0'
  } catch {
    return true
  }
})())
watch(speed, (v) => {
  save(speedKey, String(v))
  if (playing.value) {
    schedulePlay()
  }
})
watch(rangeSec, (v) => {
  save(rangeKey, String(v))
  loadIndex()
})
watch(skipIdle, (v) => save(skipKey, v ? '1' : '0'))

// ---- 时间轴 ----

const trackEl = ref<HTMLElement>()
const trackW = ref(600)
const compact = ref(false)
const trackH = computed(() => (compact.value ? 34 : 38))
let resizeObs: ResizeObserver | undefined

// 显示范围以后端返回的 from、to 为准（取最近 N 秒时由后端按最后一帧截取）
const range = computed(() => (index.value ? { from: index.value.from, to: index.value.to } : { from: 0, to: 0 }))
const geom = computed(() => {
  const idx = index.value
  if (!idx) {
    return timeline([], 0, 0, 0, 0, 0)
  }
  return timeline(idx.buckets, idx.from, idx.to, idx.bucket, trackW.value, trackH.value)
})
// 时间轴以桶为单位，最后一个桶覆盖到 to + bucket，光标与面积图用同一个比例
const axisTo = computed(() => (index.value ? index.value.to + Math.max(1, index.value.bucket) : 0))
const cursorX = computed(() => timeToX(cursor.value, range.value.from, axisTo.value, trackW.value))

const hoverX = ref(-1)
const hoverInfo = computed(() => {
  const idx = index.value
  if (!idx || hoverX.value < 0) {
    return undefined
  }
  const tm = xToTime(hoverX.value, range.value.from, axisTo.value, trackW.value, idx.step)
  return { t: Math.min(tm, idx.to), b: bucketAt(idx.buckets, idx.bucket, tm) }
})
const tipLeft = computed(() => Math.min(Math.max(hoverX.value, 70), trackW.value - 70))

const now = ref(Math.floor(Date.now() / 1000))
const agoText = computed(() => {
  const d = now.value - cursor.value
  if (d < 10) {
    return t('topologyReplay.justNow')
  }
  return t('topologyReplay.ago', { span: spanText(d) })
})

function clock(sec: number) {
  const idx = index.value
  const withDate = !!idx && (!sameDay(idx.start, idx.end) || !sameDay(sec, now.value))
  return formatClock(sec, withDate)
}
function fullTime(sec: number) {
  return new Date(sec * 1000).toLocaleString()
}

function spanText(sec: number): string {
  const p = spanParts(sec)
  if (p.d) {
    return t('topologyReplay.spanDH', { d: p.d, h: p.h })
  }
  if (p.h) {
    return t('topologyReplay.spanHM', { h: p.h, m: p.m })
  }
  if (p.m) {
    return t('topologyReplay.spanM', { m: p.m })
  }
  return t('topologyReplay.spanS', { s: p.s })
}
function rangeLabel(r: number) {
  return r ? t('topologyReplay.rangeLast', { span: spanText(r) }) : t('topologyReplay.rangeAll')
}

// ---- 取数据 ----

let alive = true
let indexTimer: number | undefined
let waitTimer: number | undefined
let playTimer: number | undefined

async function loadIndex() {
  // 每 2 个像素一个桶，范围由后端按最后一帧截取，录制继续时也不必先知道结尾
  const q = new URLSearchParams({ points: String(Math.max(60, Math.min(1000, Math.round(trackW.value / 2)))) })
  if (rangeSec.value) {
    q.set('last', String(rangeSec.value))
  }
  try {
    const idx = await api.get<ReplayIndex>('/api/webservice/topology/replay?' + q.toString())
    if (!alive) {
      return
    }
    // 第一次取到数据、或刚刚有数据时，停在最新的一帧；范围按最新结尾重新计算
    const first = !index.value?.frames && idx.frames > 0
    index.value = idx
    loadError.value = ''
    if (first) {
      cursor.value = idx.end
      await requestFrame(idx.end)
    } else if (idx.frames && (cursor.value < idx.start || cursor.value > idx.end)) {
      // 光标所在的帧已被淘汰：移到现存的范围内
      await requestFrame(Math.min(Math.max(cursor.value, idx.start), idx.end))
    } else if (!idx.frames) {
      meta.value = undefined
      lastFrame.value = undefined
      playing.value = false
      emit('empty')
    }
    // 刚开启录制、还没有数据：每 2 秒查一次，第一帧录好后尽快显示
    window.clearTimeout(waitTimer)
    if (idx.settings.enable && !idx.frames) {
      waitTimer = window.setTimeout(loadIndex, 2000)
    }
  } catch (e) {
    if (alive) {
      loadError.value = (e as Error).message
    }
  }
}

// 取帧：同一时间只有一个请求，拖动时间轴期间的中间位置合并为最后一个
let inflight = false
let pending: number | null = null
async function requestFrame(tm: number): Promise<void> {
  if (inflight) {
    pending = tm
    return
  }
  inflight = true
  try {
    const q = new URLSearchParams({ t: String(Math.max(0, Math.round(tm))) })
    if (props.focus) {
      q.set('focus', props.focus)
    }
    const v = await api.get<TopologyView>('/api/webservice/topology/replay/frame?' + q.toString())
    if (!alive) {
      return
    }
    lastFrame.value = v
    meta.value = v.replay
    if (pending === null) {
      cursor.value = v.time
    }
    loadError.value = ''
    emit('frame', v)
  } catch (e) {
    if (alive) {
      loadError.value = (e as Error).message
      playing.value = false
    }
  } finally {
    inflight = false
  }
  if (pending !== null && alive) {
    const p = pending
    pending = null
    await requestFrame(p)
  }
}

// 详情面板换了节点：重新取当前帧，带上新节点的曲线
watch(
  () => props.focus,
  () => {
    if (index.value?.frames && cursor.value) {
      requestFrame(cursor.value)
    }
  },
)

// ---- 播放 ----

function schedulePlay() {
  window.clearTimeout(playTimer)
  if (!playing.value) {
    return
  }
  const tick = playTick(speed.value, index.value?.step ?? 5)
  playTimer = window.setTimeout(async () => {
    if (!alive || !playing.value) {
      return
    }
    // 页面在后台时不前进，回到前台后继续
    if (document.hidden) {
      schedulePlay()
      return
    }
    const v = lastFrame.value
    const m = meta.value
    if (!v || !m) {
      playing.value = false
      return
    }
    const nxt = nextCursor(cursor.value, m, frameIdle(v), tick.advance, skipIdle.value)
    if (nxt === null) {
      playing.value = false
      return
    }
    cursor.value = nxt
    await requestFrame(nxt)
    schedulePlay()
  }, tick.ms)
}

function togglePlay() {
  if (playing.value) {
    playing.value = false
    window.clearTimeout(playTimer)
    return
  }
  // 已在最后一帧时从头播放
  if (meta.value && !meta.value.next) {
    cursor.value = range.value.from
    requestFrame(range.value.from)
  }
  playing.value = true
  schedulePlay()
}

function step(dir: -1 | 1) {
  playing.value = false
  window.clearTimeout(playTimer)
  const m = meta.value
  const target = dir < 0 ? m?.prev : m?.next
  if (target) {
    cursor.value = target
    requestFrame(target)
  }
}

// ---- 时间轴拖动 ----

let dragging = false
function trackXOf(e: PointerEvent): number {
  const r = trackEl.value?.getBoundingClientRect()
  return r ? e.clientX - r.left : 0
}
function seekX(x: number) {
  const idx = index.value
  if (!idx) {
    return
  }
  const tm = Math.min(idx.to, xToTime(x, range.value.from, axisTo.value, trackW.value, idx.step))
  cursor.value = tm
  requestFrame(tm)
}
function onTrackDown(e: PointerEvent) {
  if (e.button !== 0) {
    return
  }
  dragging = true
  playing.value = false
  window.clearTimeout(playTimer)
  // 捕获指针后拖出时间轴也能继续定位；指针已失效时会抛异常，不影响定位
  try {
    trackEl.value?.setPointerCapture?.(e.pointerId)
  } catch {
    // 忽略
  }
  seekX(trackXOf(e))
}
function onTrackMove(e: PointerEvent) {
  const x = trackXOf(e)
  if (e.pointerType === 'mouse') {
    hoverX.value = Math.min(Math.max(0, x), trackW.value)
  }
  if (dragging) {
    seekX(x)
  }
}
function onTrackUp(e: PointerEvent) {
  dragging = false
  try {
    trackEl.value?.releasePointerCapture?.(e.pointerId)
  } catch {
    // 忽略
  }
  if (e.pointerType !== 'mouse') {
    hoverX.value = -1
  }
}

// 空格播放或暂停（焦点在输入框、按钮、弹窗内时不处理，避免与它们的按键冲突）
function onKeydown(e: KeyboardEvent) {
  if (e.key !== ' ' || !index.value?.frames) {
    return
  }
  const el = e.target as HTMLElement | null
  if (el && (el.closest('input, textarea, select, button, [contenteditable="true"], .el-overlay') || (el !== document.body && el !== trackEl.value))) {
    return
  }
  e.preventDefault()
  togglePlay()
}

// ---- 设置 ----

const settingsOpen = ref(false)
const saving = ref(false)
const form = ref<ReplaySettings>({ enable: false, sizeMB: 8 })
function resetForm() {
  form.value = { ...(index.value?.settings ?? { enable: false, sizeMB: 8 }) }
}
const usagePct = computed(() => {
  const idx = index.value
  return idx && idx.limit ? Math.min(100, Math.round((idx.used / idx.limit) * 100)) : 0
})

async function putSettings(s: ReplaySettings): Promise<boolean> {
  saving.value = true
  try {
    const idx = await api.put<ReplayIndex>('/api/webservice/topology/replay/settings', s)
    if (!alive) {
      return false
    }
    index.value = { ...idx, buckets: index.value?.buckets ?? [] }
    await loadIndex()
    return true
  } catch (e) {
    ElMessage.error((e as Error).message)
    return false
  } finally {
    saving.value = false
  }
}

async function enableNow() {
  const s = { enable: true, sizeMB: index.value?.settings.sizeMB || 8 }
  if (await putSettings(s)) {
    ElMessage.success(t('topologyReplay.enabled'))
  }
}

async function saveSettings() {
  const cur = index.value?.settings
  // 关闭录制会清空已录制的数据，先确认
  if (cur?.enable && !form.value.enable && index.value?.frames) {
    try {
      await ElMessageBox.confirm(t('topologyReplay.disableConfirm'), t('topologyReplay.settingsTitle'), { type: 'warning' })
    } catch {
      return
    }
  }
  if (await putSettings({ ...form.value })) {
    settingsOpen.value = false
    ElMessage.success(t('common.saved'))
    if (!form.value.enable) {
      playing.value = false
    }
  }
}

// observeTrack 开始跟踪时间轴的宽度（时间轴可能晚于挂载才出现，或被替换成新元素）
function observeTrack(el: HTMLElement | undefined, old?: HTMLElement) {
  if (!alive) {
    return
  }
  if (!resizeObs) {
    resizeObs = new ResizeObserver(() => {
      const w = trackEl.value?.clientWidth
      if (w) {
        trackW.value = w
      }
      compact.value = window.innerWidth < 768
    })
  }
  if (old && old !== el) {
    resizeObs.unobserve(old)
  }
  if (el) {
    resizeObs.observe(el)
    trackW.value = el.clientWidth || trackW.value
  }
}

// 在 setup 顶层注册，组件卸载时随之停止；放在 onMounted 的 await 之后注册的 watch 不会自动停止
watch(trackEl, (el, old) => observeTrack(el, old), { flush: 'post' })

onMounted(async () => {
  document.addEventListener('keydown', onKeydown)
  await nextTick()
  compact.value = window.innerWidth < 768
  observeTrack(trackEl.value)
  await loadIndex()
  // 录制在继续：定期刷新时间轴，新帧追加到末尾、超出容量的旧帧从开头淘汰
  indexTimer = window.setInterval(() => {
    now.value = Math.floor(Date.now() / 1000)
    if (!document.hidden && !dragging) {
      loadIndex()
    }
  }, 15000)
})

onBeforeUnmount(() => {
  alive = false
  window.clearTimeout(playTimer)
  window.clearInterval(indexTimer)
  window.clearTimeout(waitTimer)
  document.removeEventListener('keydown', onKeydown)
  resizeObs?.disconnect()
})

defineExpose({ togglePlay, step })
</script>

<style scoped>
.replay {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px 14px;
  padding: 8px 12px;
  border: 1px solid var(--el-color-primary-light-7);
  border-radius: var(--g-radius);
  background: color-mix(in srgb, var(--el-color-primary) 5%, var(--el-bg-color));
}
.rp-loading {
  grid-column: 1 / -1;
  height: 40px;
}
.rp-off {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.rp-off-icon {
  color: var(--el-color-primary);
  flex-shrink: 0;
}
.rp-off-text {
  flex: 1;
  min-width: 220px;
}
.rp-off-title {
  font-weight: 600;
  font-size: 13px;
}
.rp-off-desc {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.rp-off-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.rec-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--el-color-danger);
  animation: rp-blink 1.4s ease-in-out infinite;
  flex-shrink: 0;
}
@keyframes rp-blink {
  50% {
    opacity: 0.25;
  }
}
.rp-controls {
  display: flex;
  align-items: center;
  gap: 4px;
}
.rp-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  background: var(--el-bg-color);
  color: var(--el-text-color-regular);
  cursor: pointer;
}
.rp-btn:hover:not(:disabled) {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary-light-5);
}
.rp-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.rp-btn.play {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border-color: var(--el-color-primary);
  background: var(--el-color-primary);
  color: #fff;
}
.rp-btn.play:hover {
  color: #fff;
  filter: brightness(1.08);
}
.rp-time {
  display: flex;
  flex-direction: column;
  margin-left: 6px;
  line-height: 1.2;
  min-width: 92px;
}
.rp-clock {
  font-size: 14px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.rp-ago {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.rp-track {
  position: relative;
  /* 下方 14px 放范围两端的时刻 */
  padding-bottom: 14px;
  cursor: pointer;
  touch-action: none;
  user-select: none;
  outline: none;
  border-radius: 6px;
}
.rp-track:focus-visible {
  box-shadow: 0 0 0 2px var(--el-color-primary-light-5);
}
.rp-svg {
  display: block;
  width: 100%;
  border-radius: 6px;
  background: var(--el-fill-color-light);
}
.rp-area {
  fill: color-mix(in srgb, var(--el-color-primary) 22%, transparent);
}
.rp-line {
  fill: none;
  stroke: var(--el-color-primary);
  stroke-width: 1.2;
}
.rp-gap {
  fill: var(--el-fill-color-darker);
  opacity: 0.6;
}
.rp-issue {
  fill: var(--el-color-danger);
}
.rp-hover {
  stroke: var(--el-text-color-placeholder);
  stroke-dasharray: 2 2;
}
.rp-cursor {
  stroke: var(--el-color-primary);
  stroke-width: 2;
}
.rp-knob {
  position: absolute;
  top: -4px;
  width: 10px;
  height: 10px;
  margin-left: -5px;
  border-radius: 50%;
  background: var(--el-color-primary);
  box-shadow: 0 0 0 2px var(--el-bg-color);
  pointer-events: none;
}
.rp-tip {
  position: absolute;
  bottom: calc(100% + 6px);
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--el-bg-color-overlay);
  box-shadow: var(--el-box-shadow-light);
  font-size: 12px;
  white-space: nowrap;
  pointer-events: none;
  z-index: 2;
}
.rp-edge {
  position: absolute;
  bottom: 0;
  font-size: 10px;
  color: var(--el-text-color-placeholder);
}
.rp-edge.l {
  left: 2px;
}
.rp-edge.r {
  right: 2px;
}
.rp-options {
  display: flex;
  align-items: center;
  gap: 8px;
}
.rp-speed {
  width: 72px;
}
.rp-range {
  width: 130px;
}
.rp-skip {
  margin-right: 0;
}
.rp-live .btn-ic {
  margin-right: 4px;
}
.rp-error {
  grid-column: 1 / -1;
  font-size: 12px;
  color: var(--el-color-danger);
}
.rp-muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.mono {
  font-family: var(--g-mono);
}
.rp-hint {
  width: 100%;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.rp-unit {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
}
.rp-usage {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
}
/* 窄屏：时间轴占满一行，控制按钮与选项分在上下两行 */
@container (max-width: 900px) {
  .replay {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }
  .rp-controls {
    order: 1;
  }
  .rp-track {
    order: 2;
  }
  .rp-options {
    order: 3;
    flex-wrap: wrap;
  }
  .rp-live {
    margin-left: auto;
  }
}
.replay.compact .rp-range {
  width: 112px;
}
@media (prefers-reduced-motion: reduce) {
  .rec-dot {
    animation: none;
  }
}
</style>
