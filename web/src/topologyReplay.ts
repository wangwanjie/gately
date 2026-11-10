// 拓扑回放的纯函数：时间轴几何、播放节奏与时间格式。界面见 components/TopologyReplay.vue。
import type { TopologyView } from './topology'

export interface ReplaySettings {
  enable: boolean
  sizeMB: number
}

export interface ReplayBucket {
  t: number
  qps: number
  bps: number
  active: number
  ips: number
  issue?: boolean
}

export interface ReplayIndex {
  settings: ReplaySettings
  used: number
  limit: number
  frames: number
  samples: number
  start: number
  end: number
  step: number
  from: number
  to: number
  bucket: number
  buckets: ReplayBucket[]
}

// 播放速度（相对真实时间的倍数）与时间轴范围（秒，0 表示全部录制数据）
export const REPLAY_SPEEDS = [1, 5, 10, 30, 60]
export const REPLAY_DEFAULT_SPEED = 10
export const REPLAY_RANGES = [0, 900, 3600, 6 * 3600, 24 * 3600]
// 回放容量上限（MiB），与后端 replayMaxMB 一致
export const REPLAY_MAX_MB = 256
// 播放时两次取帧的最短间隔（毫秒），速度更快时一次前进多个采样
const MIN_TICK_MS = 250

// timeToX 把时刻换算为时间轴上的横坐标，范围只有一个时刻时放在最右侧。
export function timeToX(t: number, from: number, to: number, width: number): number {
  if (to <= from) {
    return width
  }
  return Math.min(width, Math.max(0, ((t - from) / (to - from)) * width))
}

// xToTime 把横坐标换算为时刻，对齐到采样间隔并限制在范围内。
export function xToTime(x: number, from: number, to: number, width: number, step: number): number {
  if (to <= from || width <= 0) {
    return to
  }
  const raw = from + (Math.min(width, Math.max(0, x)) / width) * (to - from)
  const s = step > 0 ? step : 1
  return Math.min(to, Math.max(from, from + Math.round((raw - from) / s) * s))
}

export interface TimelineGeom {
  // area 为流量面积图路径，line 为顶边
  area: string
  line: string
  // issues 为有异常的桶（横坐标与宽度）
  issues: { x: number; w: number }[]
  // gaps 为没有录制数据的区段
  gaps: { x: number; w: number }[]
  // peak 为范围内的最大 QPS
  peak: number
}

// timeline 生成时间轴的面积图：纵向按范围内峰值线性缩放，没有数据的区段断开并单独标出。
export function timeline(buckets: ReplayBucket[], from: number, to: number, bucket: number, width: number, height: number): TimelineGeom {
  const out: TimelineGeom = { area: '', line: '', issues: [], gaps: [], peak: 0 }
  if (!buckets.length || width <= 0 || height <= 0) {
    return out
  }
  const bw = bucket > 0 ? bucket : 1
  // 横坐标按桶的时间范围 [t, t + bucket) 计算，最后一个桶延伸到 to
  const span = Math.max(1, to + bw - from)
  const xOf = (t: number) => Math.min(width, Math.max(0, ((t - from) / span) * width))
  out.peak = Math.max(...buckets.map((b) => b.qps))
  const yOf = (q: number) => (out.peak > 0 ? height - 1 - (q / out.peak) * (height - 3) : height - 1)
  let area = ''
  let line = ''
  let runStart = -1
  let prevEnd = from
  const close = (endX: number) => {
    if (runStart >= 0) {
      area += `L${endX.toFixed(1)},${height}L${runStart.toFixed(1)},${height}Z`
      runStart = -1
    }
  }
  for (const b of buckets) {
    const x0 = xOf(b.t)
    const x1 = xOf(b.t + bw)
    if (b.t > prevEnd) {
      // 与上一个桶之间有空档：先闭合上一段，再记一个空档
      close(xOf(prevEnd))
      out.gaps.push({ x: xOf(prevEnd), w: x0 - xOf(prevEnd) })
    }
    const y = yOf(b.qps).toFixed(1)
    if (runStart < 0) {
      runStart = x0
      area += `M${x0.toFixed(1)},${height}L${x0.toFixed(1)},${y}`
      line += `M${x0.toFixed(1)},${y}`
    } else {
      area += `L${x0.toFixed(1)},${y}`
      line += `L${x0.toFixed(1)},${y}`
    }
    area += `L${x1.toFixed(1)},${y}`
    line += `L${x1.toFixed(1)},${y}`
    if (b.issue) {
      out.issues.push({ x: x0, w: Math.max(2, x1 - x0) })
    }
    prevEnd = b.t + bw
  }
  close(xOf(prevEnd))
  out.area = area
  out.line = line
  return out
}

// bucketAt 返回时刻所在的桶，没有数据时返回 undefined。
export function bucketAt(buckets: ReplayBucket[], bucket: number, t: number): ReplayBucket | undefined {
  let lo = 0
  let hi = buckets.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    const b = buckets[mid]
    if (t < b.t) {
      hi = mid - 1
    } else if (t >= b.t + bucket) {
      lo = mid + 1
    } else {
      return b
    }
  }
  return undefined
}

// playTick 返回播放时的取帧间隔（毫秒）与每次前进的秒数（采样间隔的整数倍）。
export function playTick(speed: number, step: number): { ms: number; advance: number } {
  const s = step > 0 ? step : 5
  const v = speed > 0 ? speed : 1
  const ms = (s * 1000) / v
  if (ms >= MIN_TICK_MS) {
    return { ms: Math.round(ms), advance: s }
  }
  const n = Math.ceil(MIN_TICK_MS / ms)
  return { ms: Math.round(ms * n), advance: s * n }
}

// frameIdle 判断回放帧是否空闲：没有速率、进行中请求与活动 IP。
export function frameIdle(v: Pick<TopologyView, 'rate' | 'active' | 'ipTotal'>): boolean {
  return !v.rate.qps && !v.rate.bps && !v.rate.qps60 && !v.active && !v.ipTotal
}

export interface FrameMeta {
  frameStart: number
  frameEnd: number
  end: number
  next: number
  step: number
}

// nextCursor 计算播放的下一个时刻；返回 null 表示已播放到最后。
// skipIdle 时，当前帧空闲且所在段还有后续采样（内容不变的空闲时段），直接跳到该段之后的第一个采样。
export function nextCursor(at: number, meta: FrameMeta, idle: boolean, advance: number, skipIdle: boolean): number | null {
  if (!meta.next || at >= meta.end) {
    return null
  }
  if (skipIdle && idle && meta.frameEnd > at) {
    return Math.min(meta.end, meta.frameEnd + meta.step)
  }
  return Math.min(meta.end, Math.max(meta.next, at + advance))
}

function pad(n: number): string {
  return n < 10 ? '0' + n : String(n)
}

// formatClock 把 Unix 秒格式化为本地时间 HH:mm:ss，withDate 时前面加上 MM-DD。
export function formatClock(sec: number, withDate = false): string {
  const d = new Date(sec * 1000)
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  return withDate ? `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${time}` : time
}

// sameDay 判断两个时刻是否在本地时间的同一天。
export function sameDay(a: number, b: number): boolean {
  const x = new Date(a * 1000)
  const y = new Date(b * 1000)
  return x.getFullYear() === y.getFullYear() && x.getMonth() === y.getMonth() && x.getDate() === y.getDate()
}

// spanParts 把秒数拆成天、小时、分钟（不足一分钟时为秒），供界面按语言拼接时长。
export function spanParts(sec: number): { d: number; h: number; m: number; s: number } {
  const v = Math.max(0, Math.floor(sec))
  return { d: Math.floor(v / 86400), h: Math.floor((v % 86400) / 3600), m: Math.floor((v % 3600) / 60), s: v % 60 }
}
