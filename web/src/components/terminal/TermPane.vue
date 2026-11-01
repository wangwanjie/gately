<template>
  <div class="term-pane" :style="{ background: bg }">
    <div ref="host" class="term-host"></div>
    <!-- 移动端选择模式：长按后在终端上方叠一层纯文本，交给系统原生的选择手柄、放大镜和拷贝菜单 -->
    <div
      v-if="selecting"
      ref="selLayer"
      class="sel-layer"
      :style="selStyle"
      @copy="onLayerCopy"
    >
      <!-- 顶部留出操作条的高度，缓冲区开头几行也能滚到操作条下方 -->
      <div class="sel-spacer"></div>
      <div ref="selRowsEl">
        <div v-for="(r, i) in selRows" :key="i" class="sel-row" :data-r="i">{{ r || '\u200b' }}</div>
      </div>
    </div>
    <div v-if="selecting" ref="selBar" class="sel-bar" @mousedown.prevent>
      <button class="sel-btn primary" @click="copySelected"><Copy :size="15" />{{ t('terminal.select.copy') }}</button>
      <button class="sel-btn" @click="selectAllRows"><TextSelect :size="15" />{{ t('terminal.select.all') }}</button>
      <button class="sel-btn" @click="pasteFromBar"><ClipboardPaste :size="15" />{{ t('terminal.select.paste') }}</button>
      <button class="sel-btn icon" :aria-label="t('terminal.select.exit')" @click="exitSelect"><X :size="16" /></button>
    </div>
    <div v-if="status === 'reconnecting' || status === 'connecting' || status === 'closed' || status === 'ended'" class="term-badge" :class="status">
      <template v-if="status === 'connecting'">{{ t('terminal.pane.connecting') }}</template>
      <template v-else-if="status === 'reconnecting'">{{ t('terminal.pane.reconnecting', { count: attempts }) }}</template>
      <template v-else>
        <span>{{ status === 'ended' ? t('terminal.pane.ended') : t('terminal.pane.closed') }}</span>
        <button v-if="status === 'closed'" class="badge-btn" @click="reconnect">{{ t('terminal.pane.reconnect') }}</button>
        <button class="badge-btn" @click="restart">{{ t('terminal.pane.newSession') }}</button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { ClipboardPaste, Copy, TextSelect, X } from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import { ApiError, api } from '../../api'
import { copyText, recordClip } from './clipboard'
import { ensureAccess, isCancel } from './security'
import { ctrlChar, keySequence, type KeyName } from './terminalKeys'
import { findTheme } from './themes'
import { FONT_MAX, FONT_MIN } from './prefs'
import type { PaneStatus } from './types'

const { t } = useI18n()

const props = defineProps<{
  connKey: string
  // 初始附着的会话 ID，为空时新建会话
  sessionId?: string
  fontSize: number
  themeName: string
  // 心跳间隔（秒）
  heartbeat: number
  // 虚拟键盘的粘滞修饰键，仅活动窗格会传入 true
  ctrl: boolean
  alt: boolean
}>()

const emit = defineEmits<{
  session: [id: string]
  status: [s: PaneStatus]
  notice: [msg: string]
  'update:ctrl': [v: boolean]
  'update:alt': [v: boolean]
  zoom: [size: number]
  // 选择模式里点了复制，文本已按终端逻辑行拼好（软换行不插入换行符）
  copy: [text: string]
  // 选择模式里点了粘贴
  paste: []
}>()

const host = ref<HTMLElement>()
const status = ref<PaneStatus>('connecting')
const attempts = ref(0)
const bg = computed(() => findTheme(props.themeName).theme.background ?? '#1d1f21')

const MAX_RETRY = 30
let term: Terminal | undefined
let fit: FitAddon | undefined
let search: SearchAddon | undefined
let ws: WebSocket | undefined
let observer: ResizeObserver | undefined
let sid = props.sessionId ?? ''
let ended = false
let disposed = false
let generation = 0
let needReset = false
let retryTimer: ReturnType<typeof setTimeout> | undefined
let hbTimer: ReturnType<typeof setInterval> | undefined
let fitFrame = 0

function setStatus(s: PaneStatus) {
  status.value = s
  emit('status', s)
}

function writeNote(text: string, color = '33') {
  term?.write(`\r\n\x1b[${color}m[Gately] ${text}\x1b[0m\r\n`)
}

function sendJSON(msg: object) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg))
  }
}

function sendInput(d: string) {
  sendJSON({ t: 'i', d })
}

function startHeartbeat() {
  stopHeartbeat()
  const sec = Math.max(5, props.heartbeat || 30)
  hbTimer = setInterval(() => sendJSON({ t: 'p' }), sec * 1000)
}

function stopHeartbeat() {
  if (hbTimer) {
    clearInterval(hbTimer)
    hbTimer = undefined
  }
}

// connect 建立 WebSocket：有 sid 时附着已有会话（回放缓冲），否则新建会话。
async function connect() {
  if (retryTimer) {
    clearTimeout(retryTimer)
    retryTimer = undefined
  }
  const gen = ++generation
  setStatus(sid && attempts.value > 0 ? 'reconnecting' : 'connecting')
  try {
    await ensureAccess()
  } catch (e) {
    if (gen !== generation || disposed) {
      return
    }
    if (isCancel(e)) {
      writeNote(t('terminal.pane.cancelled'), '90')
      setStatus('closed')
      return
    }
    if (sid) {
      scheduleRetry()
      return
    }
    writeNote((e as Error).message, '31')
    setStatus('closed')
    return
  }
  if (gen !== generation || disposed || !term) {
    return
  }
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const q = `cols=${term.cols}&rows=${term.rows}`
  const path = sid
    ? `/api/webterminal/sessions/${encodeURIComponent(sid)}/ws?${q}`
    : `/api/webterminal/connections/${encodeURIComponent(props.connKey)}/ws?${q}`
  // 附着已有会话时，服务端会先回放缓冲，需要在收到 s 消息后清屏避免内容重复
  needReset = !!sid
  const sock = new WebSocket(`${proto}://${location.host}${path}`)
  sock.binaryType = 'arraybuffer'
  ws = sock
  sock.onopen = () => {
    startHeartbeat()
  }
  sock.onmessage = (ev) => {
    if (typeof ev.data === 'string') {
      onControl(ev.data)
      return
    }
    term?.write(new Uint8Array(ev.data as ArrayBuffer))
  }
  sock.onclose = () => {
    if (ws !== sock) {
      return
    }
    ws = undefined
    stopHeartbeat()
    if (ended || disposed) {
      return
    }
    if (!sid) {
      writeNote(t('terminal.pane.failed'), '31')
      setStatus('closed')
      return
    }
    scheduleRetry()
  }
}

function onControl(raw: string) {
  let m: { t: string; d?: string }
  try {
    m = JSON.parse(raw)
  } catch {
    term?.write(raw)
    return
  }
  if (m.t === 's' && m.d) {
    if (needReset) {
      term?.reset()
      needReset = false
    }
    sid = m.d
    attempts.value = 0
    emit('session', sid)
    setStatus('connected')
    if (term) {
      sendJSON({ t: 'r', c: term.cols, r: term.rows })
    }
  } else if (m.t === 'n') {
    writeNote(m.d ?? '')
    emit('notice', m.d ?? '')
  } else if (m.t === 'x') {
    ended = true
    writeNote(t('terminal.pane.endedReason', { reason: m.d ?? '' }), '31')
    setStatus('ended')
  }
}

// scheduleRetry 按指数退避（1s、2s、4s … 最长 30s）重新附着到会话。
function scheduleRetry() {
  attempts.value++
  if (attempts.value > MAX_RETRY) {
    writeNote(t('terminal.pane.retryGiveUp'), '31')
    setStatus('closed')
    return
  }
  setStatus('reconnecting')
  const delay = Math.min(30000, 1000 * Math.pow(2, attempts.value - 1))
  retryTimer = setTimeout(async () => {
    retryTimer = undefined
    if (disposed || ended) {
      return
    }
    if (attempts.value >= 2) {
      // 多次失败后确认会话是否仍然存在，已被回收则不再重连
      try {
        await api.get(`/api/webterminal/sessions/${encodeURIComponent(sid)}`)
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) {
          ended = true
          writeNote(t('terminal.pane.sessionGone'), '31')
          setStatus('ended')
          return
        }
      }
    }
    connect()
  }, delay)
}

// reconnect 立即重连当前会话（用于手动重连、网络恢复、页面重新可见）。
function reconnect() {
  if (disposed || ended) {
    return
  }
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    return
  }
  attempts.value = sid ? 1 : 0
  connect()
}

// restart 丢弃旧会话信息，新建一个全新的会话。
function restart() {
  generation++
  ws?.close()
  ws = undefined
  sid = ''
  ended = false
  attempts.value = 0
  term?.reset()
  connect()
}

function onWake() {
  if (document.visibilityState === 'visible') {
    reconnect()
  }
}

// applyModifiers 把软键盘输入与粘滞的 Ctrl / Alt 合并。
function applyModifiers(d: string): string {
  if (!props.ctrl && !props.alt) {
    return d
  }
  if (d.length !== 1) {
    return d
  }
  let out = d
  if (props.ctrl) {
    out = ctrlChar(out)
    emit('update:ctrl', false)
  }
  if (props.alt) {
    out = '\x1b' + out
    emit('update:alt', false)
  }
  return out
}

// sendKey 发送虚拟按键，自动带上粘滞修饰键。
function sendKey(name: KeyName) {
  const seq = keySequence(name, {
    appCursor: !!term?.modes.applicationCursorKeysMode,
    ctrl: props.ctrl,
    alt: props.alt,
  })
  if (props.ctrl) {
    emit('update:ctrl', false)
  }
  if (props.alt) {
    emit('update:alt', false)
  }
  sendInput(seq)
}

// sendText 发送文本；单字符会合并粘滞修饰键，多字符原样发送。
function sendText(text: string) {
  sendInput(applyModifiers(text))
}

function doFit() {
  if (fitFrame) {
    return
  }
  fitFrame = requestAnimationFrame(() => {
    fitFrame = 0
    const el = host.value
    if (!el || !term || el.clientWidth < 20 || el.clientHeight < 20) {
      return
    }
    try {
      fit?.fit()
    } catch {
      // 容器尚未完成布局时忽略
    }
  })
}

// ---- 触屏手势：单指滚动回看历史，双指缩放字号 ----
let tMode: '' | 'scroll' | 'pinch' = ''
let tStartY = 0
let tLastY = 0
let tAcc = 0
let tMoved = false
let tDist0 = 1
let tSize0 = 13
let tStartX = 0
let pressTimer: ReturnType<typeof setTimeout> | undefined
// 单指按住不动超过该时长进入选择模式
const LONG_PRESS_MS = 450

function cancelPress() {
  if (pressTimer) {
    clearTimeout(pressTimer)
    pressTimer = undefined
  }
}

function touchDist(e: TouchEvent): number {
  const a = e.touches[0]
  const b = e.touches[1]
  return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY) || 1
}

function onTouchStart(e: TouchEvent) {
  cancelPress()
  if (e.touches.length === 2) {
    tMode = 'pinch'
    tDist0 = touchDist(e)
    tSize0 = props.fontSize
    return
  }
  if (e.touches.length === 1) {
    tMode = 'scroll'
    tStartY = e.touches[0].clientY
    tStartX = e.touches[0].clientX
    tLastY = tStartY
    tAcc = 0
    tMoved = false
    const x = tStartX
    const y = tStartY
    pressTimer = setTimeout(() => {
      pressTimer = undefined
      // 长按期间没有滑动才进入选择模式，并选中手指下的词
      if (tMode === 'scroll' && !tMoved) {
        tMode = ''
        enterSelect({ x, y })
      }
    }, LONG_PRESS_MS)
  }
}

function onTouchMove(e: TouchEvent) {
  if (!term) {
    return
  }
  if (tMode === 'pinch' && e.touches.length === 2) {
    e.preventDefault()
    e.stopPropagation()
    const size = Math.min(FONT_MAX, Math.max(FONT_MIN, Math.round((tSize0 * touchDist(e)) / tDist0)))
    if (size !== props.fontSize) {
      emit('zoom', size)
    }
    return
  }
  if (tMode === 'scroll' && e.touches.length === 1) {
    const y = e.touches[0].clientY
    if (Math.abs(y - tStartY) > 6 || Math.abs(e.touches[0].clientX - tStartX) > 6) {
      tMoved = true
      cancelPress()
    }
    if (!tMoved) {
      return
    }
    e.preventDefault()
    e.stopPropagation()
    tAcc += tLastY - y
    tLastY = y
    const lineH = Math.max(8, (host.value?.clientHeight ?? 0) / Math.max(1, term.rows))
    const lines = Math.trunc(tAcc / lineH)
    if (lines !== 0) {
      tAcc -= lines * lineH
      if (term.buffer.active.type === 'alternate') {
        // vim、less 等全屏程序没有回滚缓冲，改为发送方向键翻动
        const seq = lines > 0 ? '\x1b[B' : '\x1b[A'
        sendInput(seq.repeat(Math.min(Math.abs(lines), 10)))
      } else {
        term.scrollLines(lines)
      }
    }
  }
}

function onTouchEnd(e: TouchEvent) {
  cancelPress()
  if (tMode === 'scroll' && !tMoved) {
    term?.focus()
  }
  if (e.touches.length === 0) {
    tMode = ''
  }
}

// 系统手势、来电或通知打断触摸时浏览器发 touchcancel 而不是 touchend，同样取消长按计时，避免误入选择模式
function onTouchCancel() {
  cancelPress()
  tMode = ''
}

let lastDataAt = 0
let lastData = ''

// iOS 第三方键盘（如搜狗英文模式）常以 keyCode=229 且无 composition 事件的方式提交字符，
// xterm 自身的 textarea 差值检测可能漏掉它们。这里监听 input 事件，
// 若 xterm 在短时间内没有通过 onData 发出该字符，则手动补发，避免丢字也避免重复。
function setupIOSImeFallback() {
  const ta = term?.textarea
  if (!ta) {
    return
  }
  const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
  let composing = false
  let lastKeyCode = 0
  ta.addEventListener('keydown', (e) => {
    lastKeyCode = e.keyCode
  }, true)
  ta.addEventListener('compositionstart', () => {
    composing = true
  }, true)
  ta.addEventListener('compositionend', () => {
    composing = false
  }, true)
  ta.addEventListener('input', (e) => {
    const ie = e as InputEvent
    if (!isIOS || composing || lastKeyCode !== 229 || ie.inputType !== 'insertText' || !ie.data) {
      return
    }
    const data = ie.data
    const t0 = Date.now()
    setTimeout(() => {
      // xterm 已经在此期间发出同样的数据，无需补发
      if (lastDataAt >= t0 && lastData.includes(data)) {
        return
      }
      term?.input(data, true)
    }, 60)
  }, true)
}

onMounted(() => {
  const el = host.value!
  term = new Terminal({
    cursorBlink: true,
    fontSize: props.fontSize,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
    theme: findTheme(props.themeName).theme,
    scrollback: 5000,
    allowProposedApi: true,
  })
  fit = new FitAddon()
  search = new SearchAddon()
  term.loadAddon(fit)
  term.loadAddon(search)
  term.loadAddon(new WebLinksAddon())
  term.open(el)
  fit.fit()

  term.onData((d) => {
    lastDataAt = Date.now()
    lastData = d
    sendInput(applyModifiers(d))
  })
  setupIOSImeFallback()
  term.onResize(({ cols, rows }) => sendJSON({ t: 'r', c: cols, r: rows }))

  observer = new ResizeObserver(() => doFit())
  observer.observe(el)
  el.addEventListener('touchstart', onTouchStart, { passive: true, capture: true })
  el.addEventListener('touchmove', onTouchMove, { passive: false, capture: true })
  el.addEventListener('touchend', onTouchEnd, { passive: true, capture: true })
  el.addEventListener('touchcancel', onTouchCancel, { passive: true, capture: true })
  window.addEventListener('online', reconnect)
  document.addEventListener('visibilitychange', onWake)

  connect()
})

watch(
  () => props.fontSize,
  (v) => {
    if (term) {
      term.options.fontSize = v
      doFit()
    }
  },
)
// 选中“跟随界面主题”时，界面换肤也会让配色重新计算
watch(
  () => findTheme(props.themeName).theme,
  (th) => {
    if (term) {
      term.options.theme = th
    }
  },
)
watch(
  () => props.heartbeat,
  () => {
    if (hbTimer) {
      startHeartbeat()
    }
  },
)

onBeforeUnmount(() => {
  disposed = true
  generation++
  if (retryTimer) {
    clearTimeout(retryTimer)
  }
  stopHeartbeat()
  cancelPress()
  observer?.disconnect()
  window.removeEventListener('online', reconnect)
  document.removeEventListener('visibilitychange', onWake)
  const el = host.value
  el?.removeEventListener('touchstart', onTouchStart, { capture: true })
  el?.removeEventListener('touchmove', onTouchMove, { capture: true })
  el?.removeEventListener('touchend', onTouchEnd, { capture: true })
  el?.removeEventListener('touchcancel', onTouchCancel, { capture: true })
  ws?.close()
  term?.dispose()
  if (fitFrame) {
    cancelAnimationFrame(fitFrame)
  }
})

// ---- 移动端选择模式 ----
// xterm 不支持触摸选择，长按后把整个缓冲区渲染成纯文本叠在终端上方，
// 行高、字号、字宽与终端一致并滚动到当前位置，看上去文字原地不动，选择交给系统完成。
const selecting = ref(false)
const selLayer = ref<HTMLElement>()
const selRowsEl = ref<HTMLElement>()
const selBar = ref<HTMLElement>()
// 叠层顶部空白的高度，与操作条占位一致（见 .sel-spacer）
const SEL_SPACER = 56
// 缓冲区每一行的文本；后一行是软换行续行时保留行尾空格，便于拼回原文
const selRows = ref<string[]>([])
// selWrapped[i] 为 true 表示第 i 行是上一行因超出宽度自动折下来的续行
let selWrapped: boolean[] = []
const selStyle = ref<Record<string, string>>({})

// cellMetrics 读取终端单元格的实际尺寸，与 xterm 渲染保持一致。
function cellMetrics(): { lineH: number; cellW: number } {
  const screen = term?.element?.querySelector('.xterm-screen') as HTMLElement | null
  const rows = Math.max(1, term?.rows ?? 1)
  const cols = Math.max(1, term?.cols ?? 1)
  const h = screen?.clientHeight || host.value?.clientHeight || rows * 17
  const w = screen?.clientWidth || 0
  return { lineH: h / rows, cellW: w / cols }
}

// enterSelect 进入选择模式；at 为长按位置时选中该处的词，为 'screen' 时选中当前屏幕。
function enterSelect(at?: { x: number; y: number } | 'screen') {
  if (!term || selecting.value) {
    return
  }
  const buf = term.buffer.active
  const rows: string[] = []
  const wrapped: boolean[] = []
  for (let i = 0; i < buf.length; i++) {
    const line = buf.getLine(i)
    const next = buf.getLine(i + 1)
    wrapped.push(!!line?.isWrapped)
    rows.push(line ? line.translateToString(!next?.isWrapped) : '')
  }
  selRows.value = rows
  selWrapped = wrapped
  const { lineH, cellW } = cellMetrics()
  const th = findTheme(props.themeName).theme
  selStyle.value = {
    fontFamily: String(term.options.fontFamily ?? 'monospace'),
    fontSize: props.fontSize + 'px',
    '--row-h': lineH + 'px',
    background: th.background ?? '#1d1f21',
    color: th.foreground ?? '#c5c8c6',
  }
  selecting.value = true
  // 收起软键盘，避免键盘挡住选择区域
  term.blur()
  const top = buf.viewportY
  const screenRows = term.rows
  nextTick(() => {
    const layer = selLayer.value
    if (!layer) {
      return
    }
    // 按终端实测字宽补偿字间距，让每列与终端对齐
    if (cellW > 0) {
      const probe = document.createElement('span')
      probe.textContent = 'W'.repeat(40)
      layer.appendChild(probe)
      const cw = probe.getBoundingClientRect().width / 40
      probe.remove()
      if (cw > 0) {
        layer.style.letterSpacing = cellW - cw + 'px'
      }
    }
    layer.scrollTop = SEL_SPACER + top * lineH
    if (at === 'screen') {
      selectRows(top, Math.min(rows.length - 1, top + screenRows - 1))
    } else if (at) {
      selectWordAt(at.x, at.y)
    }
  })
}

function exitSelect() {
  if (!selecting.value) {
    return
  }
  selecting.value = false
  selRows.value = []
  selWrapped = []
  window.getSelection()?.removeAllRanges()
}

function rowEl(r: number): HTMLElement | null {
  return selLayer.value?.querySelector(`.sel-row[data-r="${r}"]`) ?? null
}

// selectRows 选中第 a 行到第 b 行（含）的全部文本。
function selectRows(a: number, b: number) {
  const first = rowEl(a)?.firstChild
  const last = rowEl(b)?.firstChild
  if (!first || !last) {
    return
  }
  const range = document.createRange()
  range.setStart(first, 0)
  range.setEnd(last, last.textContent?.length ?? 0)
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(range)
}

function selectAllRows() {
  selectRows(0, selRows.value.length - 1)
}

// caretAt 取屏幕坐标处的文本位置，兼容 caretPositionFromPoint（标准）与 caretRangeFromPoint（WebKit）。
function caretAt(x: number, y: number): { node: Node; offset: number } | null {
  const d = document as Document & {
    caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null
    caretRangeFromPoint?: (x: number, y: number) => Range | null
  }
  if (d.caretPositionFromPoint) {
    const p = d.caretPositionFromPoint(x, y)
    return p ? { node: p.offsetNode, offset: p.offset } : null
  }
  if (d.caretRangeFromPoint) {
    const r = d.caretRangeFromPoint(x, y)
    return r ? { node: r.startContainer, offset: r.startOffset } : null
  }
  return null
}

// selectWordAt 选中坐标处连续的非空白字符；按在空白处时选中整行。
function selectWordAt(x: number, y: number) {
  // 操作条盖在顶部几行上，取位置时先隐藏，避免命中操作条
  const bar = selBar.value
  if (bar) {
    bar.style.visibility = 'hidden'
  }
  const pos = caretAt(x, y)
  if (bar) {
    bar.style.visibility = ''
    // 选中的行被操作条挡住时，把内容往下滚出操作条的高度
    if (selLayer.value && y < bar.getBoundingClientRect().bottom) {
      selLayer.value.scrollTop -= SEL_SPACER
    }
  }
  if (!pos || pos.node.nodeType !== Node.TEXT_NODE || !selLayer.value?.contains(pos.node)) {
    return
  }
  const text = pos.node.textContent ?? ''
  const isWord = (c: string | undefined) => !!c && !/[\s\u200b]/.test(c)
  let a = Math.min(pos.offset, text.length)
  // 手指落在词的右半边时 offset 指向词后一位，往回看一个字符
  if (!isWord(text[a]) && isWord(text[a - 1])) {
    a--
  }
  const range = document.createRange()
  if (!isWord(text[a])) {
    range.selectNodeContents(pos.node)
  } else {
    let b = a
    while (a > 0 && isWord(text[a - 1])) {
      a--
    }
    while (b < text.length && isWord(text[b])) {
      b++
    }
    range.setStart(pos.node, a)
    range.setEnd(pos.node, b)
  }
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(range)
}

// rowPos 把 DOM 选区端点换算成（行号, 行内偏移）。
function rowPos(node: Node, offset: number, isEnd: boolean): { r: number; o: number } | null {
  const layer = selLayer.value
  const rows = selRows.value
  if (!layer || !rows.length) {
    return null
  }
  const lastEnd = { r: rows.length - 1, o: rows[rows.length - 1].length }
  if (node === layer) {
    // 端点落在叠层本身（顶部空白或行容器之外）时，按位置取首行开头或末行结尾
    return offset <= 1 ? { r: 0, o: 0 } : lastEnd
  }
  if (node === selRowsEl.value) {
    // 端点落在行容器上时 offset 是行的下标
    return offset >= rows.length ? lastEnd : { r: offset, o: 0 }
  }
  const el = (node.nodeType === Node.TEXT_NODE ? node.parentElement : (node as Element))?.closest<HTMLElement>('.sel-row')
  if (!el || !layer.contains(el)) {
    return null
  }
  const r = Number(el.dataset.r)
  const len = rows[r]?.length ?? 0
  if (node.nodeType === Node.TEXT_NODE) {
    return { r, o: Math.min(offset, len) }
  }
  return { r, o: isEnd && offset > 0 ? len : 0 }
}

// getSelectedText 按终端逻辑行拼出选中的文本：软换行的续行直接相连，不插入换行符。
function getSelectedText(): string {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0 || sel.isCollapsed) {
    return ''
  }
  const range = sel.getRangeAt(0)
  const s = rowPos(range.startContainer, range.startOffset, false)
  const e = rowPos(range.endContainer, range.endOffset, true)
  if (!s || !e) {
    return ''
  }
  const rows = selRows.value
  let out = ''
  for (let r = s.r; r <= e.r; r++) {
    out += rows[r].slice(r === s.r ? s.o : 0, r === e.r ? e.o : undefined)
    if (r < e.r && !selWrapped[r + 1]) {
      out += '\n'
    }
  }
  // 选到末尾的空行时去掉多余的换行
  return out.replace(/\n+$/, '')
}

// 系统拷贝菜单或快捷键复制时，同样写入按逻辑行拼好的文本
function onLayerCopy(e: ClipboardEvent) {
  const text = getSelectedText()
  if (!text || !e.clipboardData) {
    return
  }
  e.preventDefault()
  e.clipboardData.setData('text/plain', text)
  recordClip(text)
}

async function copySelected() {
  const text = getSelectedText()
  if (!text) {
    ElMessage.info(t('terminal.select.empty'))
    return
  }
  if (await copyText(text)) {
    recordClip(text)
    ElMessage.success(t('common.copied'))
    exitSelect()
  } else {
    ElMessage.error(t('terminal.msg.copyFailed'))
  }
}

function pasteFromBar() {
  exitSelect()
  emit('paste')
}

// ---- 暴露给父组件的操作 ----
function focus() {
  term?.focus()
}
function refit() {
  nextTick(doFit)
}
function getSelection(): string {
  return term?.getSelection() ?? ''
}
// getScreenText 返回当前可视屏幕的文本，用于没有选区时的复制兜底。
function getScreenText(): string {
  if (!term) {
    return ''
  }
  const buf = term.buffer.active
  const lines: string[] = []
  for (let i = 0; i < term.rows; i++) {
    const line = buf.getLine(buf.viewportY + i)
    lines.push(line ? line.translateToString(true) : '')
  }
  return lines.join('\n').replace(/\s+$/, '')
}
function paste(text: string) {
  term?.paste(text)
}
// bracketedPaste 返回远端程序是否开启了括号粘贴模式（开启时多行粘贴不会被逐行执行）。
function bracketedPaste(): boolean {
  return !!term?.modes.bracketedPasteMode
}
function selectScreen() {
  enterSelect('screen')
}
function clear() {
  term?.clear()
}
function findNext(q: string): boolean {
  return search?.findNext(q, { incremental: false }) ?? false
}
function findPrev(q: string): boolean {
  return search?.findPrevious(q) ?? false
}
function clearSearch() {
  search?.clearDecorations()
  term?.clearSelection()
}

defineExpose({ focus, refit, getSelection, getScreenText, paste, bracketedPaste, selectScreen, exitSelect, clear, findNext, findPrev, clearSearch, sendKey, sendText, sendInput, reconnect, restart })
</script>

<style scoped>
.term-pane {
  position: relative;
  height: 100%;
  width: 100%;
  padding: 6px 0 6px 8px;
  box-sizing: border-box;
  overflow: hidden;
  /* 单指拖动由脚本接管为回滚，避免浏览器自带的页面滚动 */
  touch-action: none;
}
.term-host {
  height: 100%;
  width: 100%;
}
.sel-layer {
  position: absolute;
  top: 6px;
  left: 8px;
  right: 0;
  bottom: 6px;
  z-index: 4;
  overflow: auto;
  overscroll-behavior: contain;
  /* 叠层自身是滚动容器，恢复原生滚动与文字选择 */
  touch-action: pan-x pan-y;
  user-select: text;
  -webkit-user-select: text;
  -webkit-touch-callout: default;
  box-sizing: border-box;
  scrollbar-width: none;
}
.sel-layer::-webkit-scrollbar {
  display: none;
}
.sel-spacer {
  height: 56px;
}
.sel-row {
  height: var(--row-h);
  line-height: var(--row-h);
  white-space: pre;
}
/* 操作条放在顶部：最新输出在屏幕底部，是最常要复制的内容，不能被挡住 */
.sel-bar {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px;
  border-radius: 12px;
  background: var(--el-bg-color-overlay);
  box-shadow: var(--el-box-shadow-light);
  user-select: none;
  -webkit-user-select: none;
}
.sel-btn {
  flex: 0 0 auto;
  height: 36px;
  padding: 0 10px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  font-size: 13px;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.sel-btn.primary {
  background: var(--el-color-primary);
  border-color: var(--el-color-primary);
  color: #fff;
}
.sel-btn.icon {
  padding: 0 8px;
}
.sel-btn :deep(svg) {
  pointer-events: none;
}
.term-badge {
  position: absolute;
  top: 8px;
  right: 14px;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 14px;
  font-size: 12px;
  color: #fff;
  background: rgba(0, 0, 0, 0.7);
}
.term-badge.closed,
.term-badge.ended {
  background: rgba(190, 60, 60, 0.9);
}
.badge-btn {
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid rgba(255, 255, 255, 0.6);
  border-radius: 12px;
  background: transparent;
  color: #fff;
  font-size: 12px;
  cursor: pointer;
}
</style>
