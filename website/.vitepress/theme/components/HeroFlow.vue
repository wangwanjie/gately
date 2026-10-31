<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Globe, Smartphone, Laptop, HardDrive, Boxes, House } from 'lucide-vue-next'

// 首页主视觉：公网请求从左侧进入，经过中间的 Gately 核心分发到右侧服务，红色请求在核心处被拦下。
// 动画只用 SVG SMIL 与 CSS transform，离开视口或开启「减少动态效果」时暂停，避免空耗 CPU。
const props = defineProps<{
  labels: { in: string; out: string; blocked: string }
  title: string
  rows: [string, string, string][]
}>()

// 画布坐标（viewBox 560×400）：核心居中，左右各三个节点
const CX = 280
const CY = 190
const ys = [70, 190, 310]
const inIcons = [Globe, Smartphone, Laptop]
const outIcons = [HardDrive, Boxes, House]

// 左侧节点到核心、核心到右侧节点的连线；两段在核心处切线水平，可直接拼成一条完整路径
const leftPath = (y: number) => `M70 ${y} C170 ${y} 190 ${CY} ${CX} ${CY}`
const rightPath = (y: number) => `M${CX} ${CY} C370 ${CY} 390 ${y} 490 ${y}`
const throughPath = (a: number, b: number) => `${leftPath(ys[a])} C370 ${CY} 390 ${ys[b]} 490 ${ys[b]}`

// 放行的请求：起点、终点、时长与起始延迟错开，看起来像持续不断的流量；back 表示从服务返回的响应
const packets = [
  { id: 'gf-p0', d: throughPath(0, 1), dur: 3.2, begin: 0, back: false },
  { id: 'gf-p1', d: throughPath(1, 0), dur: 3.6, begin: 1.1, back: false },
  { id: 'gf-p2', d: throughPath(2, 2), dur: 3.4, begin: 2.2, back: false },
  { id: 'gf-p3', d: throughPath(1, 2), dur: 4, begin: 0.6, back: true },
  { id: 'gf-p4', d: throughPath(0, 0), dur: 3.8, begin: 2.8, back: true }
]
// 被拦截的请求沿左下连线走到核心后消失，同一周期内核心闪一圈红光并提示「已拦截」
const BLOCK_DUR = 4.6

const root = ref<HTMLElement>()
const svg = ref<SVGSVGElement>()
const paused = ref(false)
let io: IntersectionObserver | undefined
let motionQuery: MediaQueryList | undefined

function apply(visible: boolean) {
  const reduce = motionQuery?.matches ?? false
  paused.value = reduce || !visible
  if (!svg.value) {
    return
  }
  if (paused.value) {
    svg.value.pauseAnimations()
  } else {
    svg.value.unpauseAnimations()
  }
}

let visible = true
const onMotionChange = () => apply(visible)

onMounted(() => {
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  motionQuery.addEventListener('change', onMotionChange)
  io = new IntersectionObserver(([entry]) => {
    visible = entry.isIntersecting
    apply(visible)
  })
  if (root.value) {
    io.observe(root.value)
  }
  apply(visible)
})

onBeforeUnmount(() => {
  io?.disconnect()
  motionQuery?.removeEventListener('change', onMotionChange)
})
</script>

<template>
  <div ref="root" class="flow" :class="{ paused }" aria-hidden="true">
    <div class="canvas">
      <svg ref="svg" class="stage" viewBox="0 0 560 400" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">
        <defs>
          <radialGradient id="gf-glow">
            <stop offset="0" stop-color="var(--fl-a)" stop-opacity=".45" />
            <stop offset="1" stop-color="var(--fl-a)" stop-opacity="0" />
          </radialGradient>
          <linearGradient id="gf-core" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stop-color="#1fc09c" />
            <stop offset="1" stop-color="#0b5f4f" />
          </linearGradient>
          <path v-for="p in packets" :id="p.id" :key="p.id" :d="p.d" />
          <path id="gf-bad" :d="leftPath(ys[2])" />
        </defs>

        <!-- 列标题 -->
        <text class="cap" x="48" y="24" text-anchor="middle">{{ props.labels.in }}</text>
        <text class="cap" x="512" y="24" text-anchor="middle">{{ props.labels.out }}</text>

        <!-- 连线：底线 + 流动虚线 -->
        <g class="wires">
          <path v-for="y in ys" :key="'l' + y" :d="leftPath(y)" />
          <path v-for="y in ys" :key="'r' + y" :d="rightPath(y)" />
        </g>
        <g class="wires dash">
          <path v-for="y in ys" :key="'dl' + y" :d="leftPath(y)" />
          <path v-for="y in ys" :key="'dr' + y" :d="rightPath(y)" />
        </g>

        <!-- 放行的请求：小圆点带一圈光晕，沿完整路径穿过核心 -->
        <g v-for="p in packets" :key="'m' + p.id" class="pkt" :class="{ back: p.back }">
          <circle r="7" class="halo" />
          <circle r="3" class="dot" />
          <animateMotion
            :dur="`${p.dur}s`"
            :begin="`${p.begin}s`"
            repeatCount="indefinite"
            calcMode="spline"
            keyTimes="0;1"
            :keyPoints="p.back ? '1;0' : '0;1'"
            keySplines="0.45 0 0.55 1"
          >
            <mpath :xlink:href="`#${p.id}`" />
          </animateMotion>
        </g>

        <!-- 被拦截的请求 -->
        <g class="pkt bad" opacity="0">
          <circle r="7" class="halo" />
          <circle r="3" class="dot" />
          <animateMotion :dur="`${BLOCK_DUR}s`" begin="1.5s" repeatCount="indefinite" keyTimes="0;0.42;1" keyPoints="0;0.86;0.86" calcMode="linear">
            <mpath xlink:href="#gf-bad" />
          </animateMotion>
          <animate attributeName="opacity" :dur="`${BLOCK_DUR}s`" begin="1.5s" repeatCount="indefinite" keyTimes="0;0.04;0.4;0.43;1" values="0;1;1;0;0" />
        </g>

        <!-- 核心 -->
        <circle :cx="CX" :cy="CY" r="110" fill="url(#gf-glow)" class="breath" />
        <circle :cx="CX" :cy="CY" r="74" class="orbit o1" />
        <circle :cx="CX" :cy="CY" r="58" class="orbit o2" />
        <circle :cx="CX" :cy="CY" r="40" class="ping" />
        <circle :cx="CX" :cy="CY" r="40" fill="url(#gf-core)" class="core" />
        <!-- 与 favicon.svg 相同的 G 字标，放大 1.5 倍后居中 -->
        <g :transform="`translate(${CX - 24} ${CY - 24}) scale(1.5)`">
          <path d="M21.7 10.3A8 8 0 1 0 24 16h-7" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
          <circle cx="23.6" cy="8.6" r="2.2" fill="#9ff3dc" />
        </g>

        <!-- 拦截时核心外的红色冲击环与提示，与被拦截请求同一周期 -->
        <circle :cx="CX" :cy="CY" r="42" class="shock" opacity="0">
          <animate attributeName="r" :dur="`${BLOCK_DUR}s`" begin="1.5s" repeatCount="indefinite" keyTimes="0;0.42;0.62;1" values="42;42;70;70" />
          <animate attributeName="opacity" :dur="`${BLOCK_DUR}s`" begin="1.5s" repeatCount="indefinite" keyTimes="0;0.41;0.43;0.62;1" values="0;0;.9;0;0" />
        </circle>
        <g opacity="0">
          <rect :x="CX - 40" :y="CY + 86" width="80" height="24" rx="12" class="tag-bg" />
          <text :x="CX" :y="CY + 102" text-anchor="middle" class="tag-text">{{ props.labels.blocked }}</text>
          <animate attributeName="opacity" :dur="`${BLOCK_DUR}s`" begin="1.5s" repeatCount="indefinite" keyTimes="0;0.42;0.46;0.7;0.76;1" values="0;0;1;1;0;0" />
        </g>
      </svg>

      <!-- 节点图标：在与 SVG 同尺寸的 canvas 内用百分比定位，对应 SVG 坐标 -->
      <span v-for="(icon, i) in inIcons" :key="'in' + i" class="node" :style="{ left: `${(48 / 560) * 100}%`, top: `${(ys[i] / 400) * 100}%` }">
        <component :is="icon" :size="18" />
      </span>
      <span v-for="(icon, i) in outIcons" :key="'out' + i" class="node" :style="{ left: `${(512 / 560) * 100}%`, top: `${(ys[i] / 400) * 100}%` }">
        <component :is="icon" :size="18" />
      </span>
    </div>

    <!-- 运行状态面板：复用语言包里的 mockRows -->
    <div class="panel">
      <div class="panel-bar"><i /><i /><i /><span>{{ props.title }}</span><b class="live">LIVE</b></div>
      <ul>
        <li v-for="(r, i) in props.rows" :key="r[0]" :style="{ '--d': `${i * 0.35}s` }">
          <span class="state" :class="r[2]" />
          <strong>{{ r[0] }}</strong>
          <em>{{ r[1] }}</em>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.flow {
  --fl-a: #18a888;
  --fl-line: rgba(14, 124, 102, 0.22);
  --fl-dash: rgba(14, 124, 102, 0.55);
  --fl-node: #ffffff;
  --fl-bad: #ef4444;
  position: relative;
}
.dark .flow {
  --fl-a: #3ed1ae;
  --fl-line: rgba(62, 209, 174, 0.18);
  --fl-dash: rgba(62, 209, 174, 0.6);
  --fl-node: #0d1716;
}
.canvas { position: relative; }
.stage { display: block; width: 100%; height: auto; overflow: visible; }
.cap { font-size: 12px; font-weight: 600; letter-spacing: .08em; fill: var(--vp-c-text-3); }

.wires path { fill: none; stroke: var(--fl-line); stroke-width: 1.5; }
.wires.dash path { stroke: var(--fl-dash); stroke-dasharray: 2 12; stroke-linecap: round; animation: dash 1.6s linear infinite; }
@keyframes dash { to { stroke-dashoffset: -28; } }

.pkt .dot { fill: var(--fl-a); }
.pkt .halo { fill: var(--fl-a); opacity: .22; }
.pkt.back .dot, .pkt.back .halo { fill: #818cf8; }
.pkt.bad .dot, .pkt.bad .halo { fill: var(--fl-bad); }

/* 旋转、呼吸都只改 transform / opacity，由合成层完成 */
.orbit, .ping, .breath { transform-box: fill-box; transform-origin: center; }
.orbit { fill: none; stroke: var(--fl-dash); }
.o1 { stroke-width: 1; stroke-dasharray: 3 9; opacity: .55; animation: spin 26s linear infinite; }
.o2 { stroke-width: 1.5; stroke-dasharray: 60 40 6 40; animation: spin 14s linear infinite reverse; }
.ping { fill: none; stroke: var(--fl-a); stroke-width: 2; animation: ping 2.8s cubic-bezier(.2, .6, .3, 1) infinite; }
.breath { animation: breath 5s ease-in-out infinite; }
.core { filter: drop-shadow(0 10px 24px rgba(11, 95, 79, .45)); }
.shock { fill: none; stroke: var(--fl-bad); stroke-width: 2; }
.tag-bg { fill: rgba(239, 68, 68, .12); stroke: rgba(239, 68, 68, .5); }
.tag-text { font-size: 12px; font-weight: 600; fill: var(--fl-bad); }
@keyframes spin { to { transform: rotate(360deg); } }
@keyframes ping { 0% { transform: scale(1); opacity: .7; } 100% { transform: scale(1.9); opacity: 0; } }
@keyframes breath { 0%, 100% { transform: scale(.92); opacity: .75; } 50% { transform: scale(1.06); opacity: 1; } }

.node {
  position: absolute;
  display: grid;
  place-items: center;
  width: 7.8%;
  aspect-ratio: 1;
  min-width: 30px;
  border-radius: 50%;
  translate: -50% -50%;
  color: var(--vp-c-brand-1);
  background: var(--fl-node);
  border: 1px solid var(--fl-line);
  box-shadow: 0 6px 18px -8px rgba(11, 95, 79, .35);
}

.panel {
  position: relative;
  margin: -6px 6% 0;
  border-radius: 14px;
  border: 1px solid var(--vp-c-divider);
  background: color-mix(in srgb, var(--vp-c-bg) 82%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  box-shadow: 0 24px 50px -28px rgba(11, 95, 79, .45);
  overflow: hidden;
}
.panel-bar { display: flex; align-items: center; gap: 6px; padding: 9px 14px; border-bottom: 1px solid var(--vp-c-divider); }
.panel-bar i { width: 8px; height: 8px; border-radius: 50%; background: var(--vp-c-divider); }
.panel-bar span { margin-left: 8px; font-size: 12px; color: var(--vp-c-text-3); font-family: var(--vp-font-family-mono); }
.live { margin-left: auto; display: inline-flex; align-items: center; gap: 6px; font-size: 10px; font-weight: 700; letter-spacing: .12em; color: #22c55e; }
.live::before { content: ''; width: 6px; height: 6px; border-radius: 50%; background: currentColor; animation: blink 1.6s steps(2, start) infinite; }
@keyframes blink { 50% { opacity: .2; } }
.panel ul { display: grid; grid-template-columns: 1fr 1fr; margin: 0; padding: 6px; list-style: none; }
.panel li { display: grid; grid-template-columns: 10px auto 1fr; align-items: center; gap: 8px; padding: 7px 8px; font-size: 12.5px; min-width: 0; }
.panel li strong { font-weight: 600; white-space: nowrap; }
.panel li em { font-style: normal; color: var(--vp-c-text-3); font-size: 11.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; text-align: right; }
.state { position: relative; width: 7px; height: 7px; border-radius: 50%; }
.state::after { content: ''; position: absolute; inset: 0; border-radius: 50%; background: inherit; animation: ping 2.4s ease-out infinite; animation-delay: var(--d); }
.state.ok { background: #22c55e; }
.state.warn { background: #f59e0b; }

.paused *, .paused *::before, .paused *::after { animation-play-state: paused !important; }
@media (prefers-reduced-motion: reduce) {
  .pkt { display: none; }
}
@media (max-width: 600px) {
  .panel ul { grid-template-columns: 1fr; }
  .panel { margin: 4px 0 0; }
}
</style>
