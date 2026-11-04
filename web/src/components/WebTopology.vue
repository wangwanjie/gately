<template>
  <!-- Web 服务拓扑视图：互联网 → Gately → 监听 → 站点 / 动作 → 后端 / 去向，每 2 秒刷新 -->
  <div ref="rootEl" class="topo" :class="{ full }">
    <div class="topo-bar">
      <div class="ip-filter" :class="{ on: !!ipFilter }">
        <Filter :size="15" class="bar-icon" />
        <el-select
          v-model="ipModel"
          class="ip-select"
          filterable
          allow-create
          default-first-option
          clearable
          value-on-clear=""
          :placeholder="t('webTopology.filterPlaceholder')"
          :no-data-text="t('webTopology.ipNone')"
          :filter-method="(q: string) => (ipQuery = q)"
          @change="onIpChange"
          @visible-change="(v: boolean) => !v && (ipQuery = '')"
        >
          <template #label="{ value }">{{ showIP(String(value)) }}</template>
          <el-option v-for="ip in ipOptions" :key="ip.ip" :value="ip.ip" :label="showIP(ip.ip)">
            <span class="mono">{{ showIP(ip.ip) }}</span>
            <span class="opt-meta">{{ ip.active ? t('webTopology.ipActive', { n: ip.active }) : ago(ip.last) }}</span>
          </el-option>
        </el-select>
        <template v-if="ipFilter">
          <span class="filter-state">
            {{ ipInfo ? t('webTopology.filtering', { ip: showIP(ipFilter) }) : t('webTopology.ipInactive', { ip: showIP(ipFilter) }) }}
          </span>
          <el-button size="small" :icon="X" @click="clearIp">{{ t('webTopology.clearFilter') }}</el-button>
        </template>
      </div>
      <div class="bar-right">
        <div v-if="data" class="kpis">
          <span class="kpi" :title="t('webTopology.qpsHelp')"><b>{{ formatQps(data.rate.qps) }}</b> {{ t('webTopology.qpsUnit') }}</span>
          <span class="kpi" :title="t('webTopology.bpsHelp')"><b>{{ formatRate(data.rate.bps) }}</b></span>
          <span class="kpi" :title="t('webTopology.activeHelp')"><b>{{ data.active }}</b> {{ t('webTopology.active') }}</span>
          <span class="kpi" :title="t('webTopology.activeIpsHelp')"><b>{{ data.ipTotal }}</b> IP</span>
        </div>
        <!-- 聚焦活动：一键只看最近 60 秒有请求或有进行中请求的站点与后端，忽略折叠与收起，计数为当前活动中的站点数 -->
        <!-- 触屏轻点会触发悬停提示且不会自动关闭，挡住相邻按钮，触屏上不显示 -->
        <el-tooltip :content="activeOnly ? t('webTopology.activeOnlyOff') : t('webTopology.activeOnlyHelp')" placement="bottom" :disabled="noHover">
          <el-button class="active-btn" :class="{ on: activeOnly }" :type="activeOnly ? 'primary' : 'default'" :aria-pressed="activeOnly" @click="toggleActiveOnly">
            <Activity :size="14" class="btn-ic" />{{ t('webTopology.activeOnly') }}<span class="active-count" :class="{ zero: !activeCount }">{{ activeCount > 999 ? '999+' : activeCount }}</span>
          </el-button>
        </el-tooltip>
        <!-- 筛选：按监听、运行异常与拦截原因裁剪拓扑，各条件同时生效 -->
        <el-dropdown trigger="click" placement="bottom-end" :hide-on-click="false" :max-height="460" @command="onFilterCommand">
          <el-button :type="filtering ? 'primary' : 'default'">
            <ListFilter :size="14" class="btn-ic" />{{ filterCount ? t('webTopology.filterN', { n: filterCount }) : t('webTopology.filter') }}<ChevronDown :size="14" class="btn-caret" />
          </el-button>
          <template #dropdown>
            <el-dropdown-menu class="topo-mask-menu">
              <el-dropdown-item command="issues">
                <span class="mm-box" :class="{ on: filter.issues }"><Check v-if="filter.issues" :size="12" /></span>
                <span class="lm-text">
                  <span class="lm-name">{{ t('webTopology.filterIssues') }}</span>
                  <span class="lm-desc">{{ t('webTopology.filterIssuesDesc') }}</span>
                </span>
              </el-dropdown-item>
              <div class="mm-sec">{{ t('webTopology.filterBlocked') }}</div>
              <el-dropdown-item v-for="r in BLOCK_REASONS" :key="r" :command="'blocked:' + r">
                <span class="mm-box" :class="{ on: filter.blocked.includes(r) }"><Check v-if="filter.blocked.includes(r)" :size="12" /></span>
                <span class="lm-text">
                  <span class="lm-name">{{ t(blockKeys[r]) }}</span>
                </span>
              </el-dropdown-item>
              <template v-if="listenerKeys.length > 1">
                <div class="mm-sec">{{ t('webTopology.filterListeners') }}</div>
                <el-dropdown-item v-for="l in data?.listeners ?? []" :key="l.key" :command="'listener:' + l.key">
                  <span class="mm-box" :class="{ on: filter.listeners.includes(l.key) }"><Check v-if="filter.listeners.includes(l.key)" :size="12" /></span>
                  <span class="lm-text">
                    <span class="lm-name">{{ l.name || listenerAddr(l) }}</span>
                    <span v-if="l.name" class="lm-desc mono">{{ listenerAddr(l) }}</span>
                  </span>
                </el-dropdown-item>
              </template>
              <el-dropdown-item v-if="filtering" command="clear" divided>
                <X :size="15" class="lm-icon" />
                <span class="lm-text"><span class="lm-name">{{ t('webTopology.filterClear') }}</span></span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <!-- 布局切换：下拉菜单列出各布局的图标与一句话说明，按钮只显示当前布局 -->
        <el-dropdown trigger="click" placement="bottom-end" @command="(m: LayoutMode) => (layoutMode = m)">
          <el-button>
            <component :is="layoutMeta[layoutMode].icon" :size="14" class="btn-ic" />{{ layoutMeta[layoutMode].label }}<ChevronDown :size="14" class="btn-caret" />
          </el-button>
          <template #dropdown>
            <el-dropdown-menu class="topo-layout-menu">
              <el-dropdown-item v-for="m in LAYOUT_MODES" :key="m" :command="m" :class="{ on: m === layoutMode }">
                <component :is="layoutMeta[m].icon" :size="18" class="lm-icon" />
                <span class="lm-text">
                  <span class="lm-name">{{ layoutMeta[m].label }}</span>
                  <span class="lm-desc">{{ layoutMeta[m].desc }}</span>
                </span>
                <Check v-if="m === layoutMode" :size="15" class="lm-check" />
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <!-- 打码：主按钮开关，右侧下拉选择打码哪些内容 -->
        <el-dropdown split-button trigger="click" :type="masked ? 'primary' : 'default'" :hide-on-click="false" @click="masked = !masked" @command="onMaskCommand">
          <component :is="masked ? EyeOff : Eye" :size="14" class="btn-ic" />{{ t('webTopology.mask') }}
          <template #dropdown>
            <el-dropdown-menu class="topo-mask-menu">
              <div class="mm-head">{{ t('webTopology.maskTip') }}</div>
              <!-- 打码方式：星号遮住部分内容，代号把整个值换成编号 -->
              <el-dropdown-item v-for="st in MASK_STYLES" :key="st" :command="'style:' + st">
                <span class="mm-radio" :class="{ on: maskStyle === st }" />
                <span class="lm-text">
                  <span class="lm-name">{{ t(maskStyleKeys[st]) }}</span>
                  <span class="lm-desc">{{ t(maskStyleKeys[st] + 'Desc') }}</span>
                </span>
              </el-dropdown-item>
              <div class="mm-sec">{{ t('webTopology.maskParts') }}</div>
              <el-dropdown-item v-for="p in MASK_PARTS" :key="p" :command="p">
                <span class="mm-box" :class="{ on: maskParts.has(p) }"><Check v-if="maskParts.has(p)" :size="12" /></span>
                <span class="lm-text">
                  <span class="lm-name">{{ t(maskKeys[p]) }}</span>
                  <span class="lm-desc">{{ t(maskKeys[p] + 'Desc') }}</span>
                </span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button v-if="listenerKeys.length && !activeOnly" :icon="allCollapsed ? ChevronsUpDown : ChevronsDownUp" @click="toggleAll">
          {{ allCollapsed ? t('common.expandAll') : t('common.collapseAll') }}
        </el-button>
      </div>
    </div>

    <div class="topo-main">
      <div class="canvas-wrap">
      <!-- 画布是可平移缩放的视口：拖动平移，滚轮滚动，按住 Ctrl/⌘ 滚动、触控板或双指捏合缩放 -->
      <div
        ref="canvasEl"
        class="canvas"
        :class="{ moving, grabbing: dragging }"
        :style="{ height: viewH + 'px' }"
        @wheel="onWheel"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
        @click.capture="onClickCapture"
        @click="select('gately')"
      >
        <div v-if="!data && !loadError" v-loading="true" class="canvas-empty" />
        <div v-else-if="loadError && !data" class="canvas-empty msg">{{ loadError }}</div>
        <template v-else-if="layout">
          <div class="stage" :class="{ animate: animating }" :style="{ width: box.w + 'px', height: box.h + 'px', transform: `translate(${tx}px, ${ty}px) scale(${scale})` }">
            <Transition name="topo-fade" mode="out-in">
            <svg
              :key="layout.mode"
              class="graph"
              :class="['m-' + layout.mode, { 'has-focus': !!litSet, 'no-motion': !motion }]"
              :width="box.w"
              :height="box.h"
              :viewBox="`${box.x} ${box.y} ${box.w} ${box.h}`"
              role="img"
              :aria-label="t('webTopology.title')"
              @mouseleave="hover = ''"
            >
              <!-- 树形与桑基布局的列标题放在图内，随图一起缩放 -->
              <g v-if="layout.mode === 'tree'" class="cols">
                <text v-for="(c, i) in colTitles" :key="i" :x="i < 2 ? COLS[i].x + COLS[i].w / 2 : COLS[i].x" :y="-HEAD + 14" :text-anchor="i < 2 ? 'middle' : 'start'">{{ c }}</text>
              </g>
              <g v-else-if="layout.mode === 'sankey'" class="cols">
                <text v-for="(c, i) in colTitles" :key="i" :x="SANKEY_COLS[i]" :y="-HEAD + 14">{{ c }}</text>
              </g>
              <!-- 径向布局的同心圆参考线 -->
              <g v-else-if="layout.mode === 'radial'" class="rings">
                <circle v-for="r in ringRadii" :key="r" :cx="layout.width / 2" :cy="layout.height / 2" :r="r" />
              </g>
              <defs>
                <!-- 每种图标只定义一次，节点用 <use> 引用，节点很多时不必为每个节点创建图标组件 -->
                <symbol v-for="(ic, name) in icons" :id="'gtopo-' + name" :key="name" viewBox="0 0 24 24">
                  <component :is="ic" :size="24" />
                </symbol>
                <linearGradient id="topo-edge-hot" x1="0" x2="1" y1="0" y2="0">
                  <stop offset="0" stop-color="var(--topo-accent)" stop-opacity="0.35" />
                  <stop offset="1" stop-color="var(--topo-accent)" stop-opacity="1" />
                </linearGradient>
                <radialGradient id="topo-core" cx="0.5" cy="0.4" r="0.7">
                  <stop offset="0" stop-color="var(--topo-accent)" stop-opacity="var(--topo-core-a)" />
                  <stop offset="1" stop-color="var(--topo-accent)" stop-opacity="var(--topo-core-b)" />
                </radialGradient>
              </defs>
              <!-- 连线：底线 + 有流量时叠加流动虚线；粗细按 QPS 对数增长 -->
              <g class="edges">
                <g v-for="e in layout.edges" :key="e.id" class="edge" :class="edgeClass(e)">
                  <path class="edge-base" :class="{ band: e.band }" :d="e.d" :style="e.band ? undefined : { strokeWidth: edgeWidth(e.rate) }" />
                  <path v-if="animated(e)" class="edge-flow" :class="'lv' + flowLevel(e.rate)" :d="e.flow ?? e.d" :style="{ strokeWidth: e.band ? 1.6 : edgeWidth(e.rate) }" />
                </g>
              </g>
              <g class="edge-labels">
                <template v-for="e in layout.edges" :key="e.id">
                  <text v-if="showLabel(e)" class="edge-label" :class="edgeClass(e)" :x="e.labelX" :y="e.labelY" text-anchor="middle">
                    {{ formatQps(e.rate.qps) }}/s · {{ formatRate(e.rate.bps) }}
                  </text>
                </template>
              </g>
              <!-- 节点 -->
              <g
                v-for="n in layout.nodes"
                :key="n.id"
                class="node"
                :class="nodeClass(n)"
                :transform="`translate(${n.x},${n.y})`"
                @mouseenter="hover = n.id"
                @mouseleave="hover = ''"
                @click.stop="onNodeClick(n)"
              >
                <title v-if="n.kind === 'more'">{{ nodeTitle(n) }}&#10;{{ n.more!.expanded ? t('webTopology.lessHint') : t('webTopology.expandHint') }}</title>
                <title v-else-if="layout.mode === 'radial' && n.kind !== 'internet' && n.kind !== 'gately'">{{ nodeTitle(n) }}&#10;{{ nodeSub(n) }}</title>
                <!-- 桑基图：竖条 + 右侧文字，透明矩形扩大可点击范围 -->
                <template v-if="layout.mode === 'sankey'">
                  <rect class="hit" x="-6" :y="Math.min(0, n.h / 2 - SANKEY_ROW / 2)" :width="SANKEY_LABEL - 16" :height="Math.max(n.h, SANKEY_ROW)" />
                  <rect class="sbar" :width="n.w" :height="n.h" rx="3" />
                  <text class="title halo" :x="n.w + 8" :y="n.h / 2 - 3">{{ fit(n.kind === 'internet' ? t('webTopology.internet') : n.kind === 'gately' ? 'Gately' : nodeTitle(n), SANKEY_LABEL - 40) }}</text>
                  <text class="sub halo" :x="n.w + 8" :y="n.h / 2 + 12">{{ fit(n.kind === 'internet' || n.kind === 'gately' ? hubSub(n) : nodeSub(n), SANKEY_LABEL - 40) }}</text>
                  <g v-if="ipCount(n.id) > 0 && n.kind !== 'upstream' && n.kind !== 'internet' && n.kind !== 'gately'" class="ipbadge">
                    <rect x="-30" :y="n.h / 2 - 7" width="22" height="13" rx="6.5" />
                    <text x="-19" :y="n.h / 2 + 2.5" text-anchor="middle">{{ ipCount(n.id) > 99 ? '99+' : ipCount(n.id) }}</text>
                  </g>
                </template>
                <!-- 旭日图：环形扇区，文字沿半径方向放在扇区中间 -->
                <template v-else-if="layout.mode === 'sunburst' && n.shape">
                  <path class="sarc" :d="n.shape" :style="{ fillOpacity: sunOpacity(n) }" />
                  <text v-if="n.label?.show" class="slabel" v-bind="sunLabel(n)" text-anchor="middle" dominant-baseline="central">{{ fit(n.kind === 'more' && !n.more!.expanded ? '+' + n.more!.count : nodeTitle(n), n.label.max) }}</text>
                </template>
                <!-- 矩形树图：Gately 为最外层容器，父节点顶部显示名称，叶子按流量深浅着色 -->
                <template v-else-if="layout.mode === 'treemap'">
                  <rect class="tm" :class="{ leafbox: n.leaf, root: n.kind === 'gately' }" :width="n.w" :height="n.h" :rx="n.kind === 'gately' ? 10 : 5" :style="n.leaf ? { fillOpacity: sunOpacity(n) } : undefined" />
                  <template v-if="n.leaf">
                    <text v-if="n.w > 36 && n.h > 16" class="tm-title" x="7" :y="Math.min(17, n.h / 2 + 4)">{{ fit(shortTitle(n), n.w - 14) }}</text>
                    <text v-if="n.w > 70 && n.h > 36" class="tm-sub" x="7" y="32">{{ fit(nodeSub(n), n.w - 14) }}</text>
                  </template>
                  <text v-else-if="n.w > 36" class="tm-head" x="8" y="16">{{ fit(nestTitle(n), n.w - 16) }}</text>
                </template>
                <!-- 气泡图：嵌套圆形，父节点名称在圆内顶部 -->
                <template v-else-if="layout.mode === 'pack'">
                  <circle class="pk" :class="{ leafbox: n.leaf, root: n.kind === 'gately' }" :cx="n.w / 2" :cy="n.h / 2" :r="n.w / 2" :style="n.leaf ? { fillOpacity: sunOpacity(n) } : undefined" />
                  <text v-if="n.leaf && n.w > 30" class="pk-title" :x="n.w / 2" :y="n.h / 2" text-anchor="middle" dominant-baseline="central">{{ fit(shortTitle(n), n.w - 8) }}</text>
                  <text v-else-if="!n.leaf && packHeadMax(n) > 30" class="pk-head halo" :x="n.w / 2" y="15" text-anchor="middle">{{ fit(nestTitle(n), packHeadMax(n)) }}</text>
                </template>
                <template v-else-if="n.kind === 'internet' || n.kind === 'gately'">
                  <circle class="hub-halo" :cx="n.w / 2" :cy="n.h / 2" :r="n.w / 2 + 6" />
                  <circle class="hub" :cx="n.w / 2" :cy="n.h / 2" :r="n.w / 2" />
                  <use :href="n.kind === 'internet' ? '#gtopo-globe' : '#gtopo-router'" :x="n.w / 2 - 13" :y="n.h / 2 - (layout.mode === 'sunburst' ? 22 : 13)" width="26" height="26" class="hub-icon" />
                  <template v-if="layout.mode === 'sunburst'">
                    <text class="hub-title" :x="n.w / 2" :y="n.h / 2 + 18" text-anchor="middle">Gately</text>
                    <text class="hub-sub" :x="n.w / 2" :y="n.h / 2 + 31" text-anchor="middle">{{ formatQps(data!.rate.qps) }}/s</text>
                  </template>
                  <!-- 竖向树：文字放在圆形右侧，避免压住竖直连线 -->
                  <template v-else-if="layout.mode === 'vertical'">
                    <text class="hub-title" :x="n.w + 12" :y="n.h / 2 - 2">{{ n.kind === 'internet' ? t('webTopology.internet') : 'Gately' }}</text>
                    <text class="hub-sub" :x="n.w + 12" :y="n.h / 2 + 13">{{ hubSub(n) }}</text>
                  </template>
                  <template v-else>
                    <text class="hub-title" :x="n.w / 2" :y="n.h + 16" text-anchor="middle">{{ n.kind === 'internet' ? t('webTopology.internet') : 'Gately' }}</text>
                    <text class="hub-sub" :x="n.w / 2" :y="n.h + 30" text-anchor="middle">{{ hubSub(n) }}</text>
                  </template>
                </template>
                <!-- 径向布局：圆点 + 沿半径方向的文字 -->
                <template v-else-if="layout.mode === 'radial'">
                  <circle class="rdot" :cx="n.w / 2" :cy="n.h / 2" :r="n.w / 2" />
                  <text v-if="n.kind === 'more' && !n.more!.expanded" class="rmore" :x="n.w / 2" :y="n.h / 2" text-anchor="middle" dominant-baseline="central">
                    {{ n.more!.count > 99 ? '99+' : '+' + n.more!.count }}
                  </text>
                  <use v-else :href="'#gtopo-' + nodeIcon(n)" :x="n.w * 0.22" :y="n.h * 0.22" :width="n.w * 0.56" :height="n.h * 0.56" class="node-icon" />
                  <text class="rlabel" v-bind="radialLabel(n)" dominant-baseline="central">{{ fit(nodeTitle(n), radialLabel(n).max) }}</text>
                  <g v-if="n.kind === 'listener' && !activeOnly" class="fold" @click.stop="toggleListener(n)">
                    <circle :cx="n.w + 1" :cy="-1" r="7" />
                    <use :href="n.collapsed ? '#gtopo-right' : '#gtopo-down'" :x="n.w - 5" :y="-7" width="12" height="12" />
                  </g>
                  <g v-if="ipCount(n.id) > 0 && n.kind !== 'upstream'" class="ipbadge">
                    <rect :x="-14" :y="-9" :width="22" height="13" rx="6.5" />
                    <text :x="-3" :y="0.5" text-anchor="middle">{{ ipCount(n.id) > 99 ? '99+' : ipCount(n.id) }}</text>
                  </g>
                </template>
                <template v-else>
                  <rect class="card" :width="n.w" :height="n.h" rx="10" />
                  <rect class="card-accent" x="0" y="8" width="3" :height="n.h - 16" rx="1.5" />
                  <circle class="icon-bg" cx="23" :cy="n.h / 2" r="14" />
                  <use :href="'#gtopo-' + nodeIcon(n)" x="14" :y="n.h / 2 - 9" width="18" height="18" class="node-icon" />
                  <text class="title" x="44" y="19">{{ fit(nodeTitle(n), n.w - 64) }}</text>
                  <text class="sub" x="44" y="35">{{ fit(nodeSub(n), n.w - 54) }}</text>
                  <circle v-if="n.kind !== 'more'" class="dot" :cx="n.w - 10" cy="10" r="4" />
                  <g v-if="n.kind === 'listener' && !activeOnly" class="fold" @click.stop="toggleListener(n)">
                    <rect :x="n.w - 26" :y="n.h - 22" width="20" height="18" rx="5" />
                    <use :href="n.collapsed ? '#gtopo-right' : '#gtopo-down'" :x="n.w - 23" :y="n.h - 20" width="14" height="14" />
                  </g>
                  <g v-if="ipCount(n.id) > 0 && n.kind !== 'upstream'" class="ipbadge">
                    <rect :x="n.w - 30" :y="-8" :width="26" height="15" rx="7.5" />
                    <text :x="n.w - 17" :y="3" text-anchor="middle">{{ ipCount(n.id) > 99 ? '99+' : ipCount(n.id) }}</text>
                  </g>
                </template>
              </g>
            </svg>
            </Transition>
          </div>
          <div v-if="!layout.nodes.some((n) => n.kind === 'listener')" class="canvas-hint">
            <template v-if="activeOnly && data?.listeners.length">
              <div>{{ query || filtering ? t('webTopology.activeNoMatch') : t('webTopology.activeNone') }}</div>
              <el-button class="hint-btn" size="small" @click.stop="toggleActiveOnly" @pointerdown.stop>{{ t('webTopology.activeShowAll') }}</el-button>
            </template>
            <template v-else>{{ query || filtering ? t('webTopology.noMatch') : t('webTopology.empty') }}</template>
          </div>
          <!-- 缩放控件 -->
          <div class="zoom" @click.stop @pointerdown.stop @wheel.stop>
            <el-tooltip :content="t('webTopology.zoomOut')" placement="top">
              <button type="button" class="zbtn" :aria-label="t('webTopology.zoomOut')" @click="zoomBy(1 / 1.25)"><ZoomOut :size="15" /></button>
            </el-tooltip>
            <el-tooltip :content="t('webTopology.zoomReset')" placement="top">
              <button type="button" class="zbtn pct" @click="zoomTo(1)">{{ Math.round(scale * 100) }}%</button>
            </el-tooltip>
            <el-tooltip :content="t('webTopology.zoomIn')" placement="top">
              <button type="button" class="zbtn" :aria-label="t('webTopology.zoomIn')" @click="zoomBy(1.25)"><ZoomIn :size="15" /></button>
            </el-tooltip>
            <el-tooltip :content="t('webTopology.zoomFit')" placement="top">
              <button type="button" class="zbtn" :aria-label="t('webTopology.zoomFit')" @click="fitView(true, true)"><LocateFixed :size="15" /></button>
            </el-tooltip>
            <el-tooltip :content="t('webTopology.zoomHint')" placement="top">
              <span class="zbtn help"><Hand :size="15" /></span>
            </el-tooltip>
            <span class="zsep" />
            <el-tooltip :content="full ? t('webTopology.exitFull') : t('webTopology.enterFull')" placement="top">
              <button type="button" class="zbtn" :class="{ on: full }" :aria-label="full ? t('webTopology.exitFull') : t('webTopology.enterFull')" @click="toggleFull">
                <component :is="full ? Minimize : Maximize" :size="15" />
              </button>
            </el-tooltip>
          </div>
        </template>
      </div>
      <!-- 拖动调整画布高度（触屏可用），双击恢复自动高度；全屏时不显示 -->
      <div v-if="!full" class="grip" :class="{ active: gripping, custom: !!userH }" @pointerdown="onGripDown" @pointermove="onGripMove" @pointerup="onGripUp" @pointercancel="onGripUp" @dblclick="resetHeight">
        <span class="grip-bar" />
        <span class="grip-text">{{ userH ? t('webTopology.heightCustom', { h: viewH }) : t('webTopology.heightHint') }}</span>
        <button v-if="userH" type="button" class="grip-reset" @pointerdown.stop @click.stop="resetHeight">{{ t('webTopology.heightAuto') }}</button>
      </div>
      </div>

      <aside class="panel">
        <template v-if="detail">
          <div class="panel-head">
            <div class="panel-title">{{ detail.title }}</div>
            <div class="panel-sub">{{ detail.sub }}</div>
            <div v-if="detail.tags.length" class="tags">
              <el-tag v-for="tg in detail.tags" :key="tg.text" size="small" :type="tg.type" effect="plain" round disable-transitions>{{ tg.text }}</el-tag>
            </div>
            <div v-if="detail.error" class="panel-error">{{ detail.error }}</div>
          </div>
          <div class="metrics">
            <div v-for="m in detail.metrics" :key="m.label" class="metric">
              <div class="m-label">
                {{ m.label }}
                <el-tooltip v-if="m.help" :content="m.help" placement="top"><Info :size="12" class="m-help" /></el-tooltip>
              </div>
              <div class="m-value" :class="m.cls">{{ m.value }}</div>
            </div>
          </div>
          <template v-if="seriesFor === detailFocus && data?.series?.length">
            <div class="chart-title">
              {{ t('webTopology.curve') }}
              <span class="chart-meta">{{ t('webTopology.curveMeta', { max: formatQps(seriesMax) }) }}</span>
            </div>
            <Sparkline :series="[{ values: data.series.map((p) => p.requests), color: 'var(--el-color-primary)' }]" :capacity="data.series.length" :height="56" />
            <div class="chart-title">{{ t('webTopology.curveBytes') }}</div>
            <Sparkline :series="[{ values: data.series.map((p) => p.bytes), color: 'var(--g-chart-2)' }]" :capacity="data.series.length" :height="40" />
          </template>
          <div v-if="detail.rows.length" class="rows">
            <div v-for="r in detail.rows" :key="r.label" class="row">
              <span class="r-label">{{ r.label }}</span>
              <span class="r-value" :class="r.cls">{{ r.value }}</span>
            </div>
          </div>
          <div v-if="detail.actions.length" class="panel-actions">
            <el-button v-for="a in detail.actions" :key="a.key" size="small" :icon="a.icon" @click="a.run">{{ a.label }}</el-button>
          </div>
          <div class="chart-title ips-title">
            {{ t('webTopology.activeIps') }}
            <span class="chart-meta">{{ panelIps.length }}</span>
            <el-tooltip :content="t('webTopology.activeIpsHelp')" placement="top"><Info :size="12" class="m-help" /></el-tooltip>
          </div>
          <div v-if="data?.ipTruncated" class="ip-warn">{{ t('webTopology.ipTruncated') }}</div>
          <div class="ip-list">
            <div v-if="!panelIps.length" class="ip-empty">{{ t('webTopology.ipNone') }}</div>
            <div v-for="ip in panelIps" :key="ip.ip" role="button" tabindex="0" class="ip-row" :class="{ on: ip.ip === ipFilter }" @click="setIp(ip.ip)" @keydown.enter="setIp(ip.ip)">
              <span class="mono ip">{{ showIP(ip.ip) }}</span>
              <span class="ip-meta">
                <span v-if="ip.n" class="live">{{ t('webTopology.ipActive', { n: ip.n }) }}</span>
                <span v-else>{{ ago(ip.last) }}</span>
                <!-- 查看该地址在所选站点（未选站点时为全部站点）的访问日志 -->
                <el-tooltip :content="logSite ? t('webTopology.ipLogs') : t('webTopology.ipLogsAll')" placement="top">
                  <button type="button" class="ip-log" :aria-label="t('webTopology.ipLogs')" @click.stop="openLogs(ip.ip)"><FileText :size="13" /></button>
                </el-tooltip>
              </span>
            </div>
          </div>
        </template>
      </aside>
    </div>

    <div class="topo-foot">
      <span class="pulse" :class="{ off: paused || !!loadError }" />
      <span>{{ paused ? t('webTopology.paused') : loadError ? loadError : t('webTopology.updated', { time: updatedAt }) }}</span>
      <span class="foot-note">{{ t('webTopology.note') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
  Activity,
  AppWindow,
  ArrowRightLeft,
  BarChart3,
  Ban,
  Bubbles,
  ChartPie,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUp,
  ChevronsUpDown,
  CornerUpRight,
  Ellipsis,
  EthernetPort,
  Eye,
  EyeOff,
  FileText,
  Filter,
  FolderOpen,
  Globe,
  Hand,
  Info,
  LayoutDashboard,
  ListFilter,
  ListTree,
  LocateFixed,
  Lock,
  Maximize,
  Minimize,
  Network,
  Orbit,
  Pencil,
  Router,
  SearchX,
  Server,
  Unplug,
  Waves,
  X,
  ZoomIn,
  ZoomOut,
} from 'lucide-vue-next'
import { api, formatBytes, formatRate } from '../api'
import Sparkline from './Sparkline.vue'
import type { SiteLogTarget } from './SiteLogDialog.vue'
import { useCollapsed } from '../composables/useCollapsed'
import { useScrollLock } from '../composables/useScrollLock'
import {
  COLS,
  LAYOUT_MODES,
  BLOCK_REASONS,
  RADIAL_LABEL,
  SANKEY_COLS,
  SANKEY_LABEL,
  SANKEY_ROW,
  Aliases,
  activeAt,
  aliasDomain,
  aliasDomainText,
  aliasHost,
  aliasIP,
  aliasText,
  aliasURL,
  buildLayout,
  countActive,
  edgeWidth,
  filterActive,
  filterView,
  flowLevel,
  formatCount,
  formatQps,
  ipPath,
  isColumnar,
  isIPLiteral,
  isValidIP,
  maskHost,
  maskDomain,
  maskDomainText,
  maskIP,
  maskText,
  maskURL,
  nodeCovers,
  pathOf,
  resolveId,
  type LayoutEdge,
  type LayoutMode,
  type LayoutNode,
  type MaskStyle,
  type TopoFilter,
  type TopoIP,
  type TopologyView,
} from '../topology'

const props = defineProps<{ query: string }>()
const emit = defineEmits<{
  stats: [listener: string, site: string, name: string]
  conns: [listener: string, site: string, name: string]
  edit: [listener: string, site: string]
  editListener: [listener: string]
  logs: [target: SiteLogTarget]
}>()

const { t } = useI18n()

const data = ref<TopologyView>()
const loadError = ref('')
const updatedAt = ref('')
const paused = ref(false)
// selected 为点击选中的节点（详情面板展示它），hover 为悬停中的节点（只影响高亮）
const selected = ref('gately')
const hover = ref('')
const ipFilter = ref('')
const ipModel = ref('')
// seriesFor 为当前曲线所属的节点，切换选中节点后等新数据到达再显示，避免把上一个节点的曲线当成新节点的
const seriesFor = ref('')

const maskKey = 'gately.topology.mask'
const masked = ref(readFlag(maskKey))
watch(masked, (v) => writeFlag(maskKey, v))

// 打码范围：visitor 访客 IP，backend 后端与跳转地址，listener 监听地址（含以 IP 为域名的站点与监听错误信息），domain 域名
type MaskPart = 'visitor' | 'backend' | 'listener' | 'domain'
const MASK_PARTS: MaskPart[] = ['visitor', 'backend', 'listener', 'domain']
const maskKeys: Record<MaskPart, string> = {
  visitor: 'webTopology.maskVisitor',
  backend: 'webTopology.maskBackend',
  listener: 'webTopology.maskListener',
  domain: 'webTopology.maskDomain',
}
const maskPartsKey = 'gately.topology.maskParts'
const maskParts = ref(readMaskParts())
function readMaskParts(): Set<MaskPart> {
  try {
    const raw = localStorage.getItem(maskPartsKey)
    if (raw !== null) {
      return new Set(raw.split(',').filter((p): p is MaskPart => MASK_PARTS.includes(p as MaskPart)))
    }
  } catch {
    // 读取失败时用默认值
  }
  // 默认打码全部 IP，域名不打码
  return new Set<MaskPart>(['visitor', 'backend', 'listener'])
}
function toggleMaskPart(p: MaskPart) {
  const next = new Set(maskParts.value)
  if (next.has(p)) {
    next.delete(p)
  } else {
    next.add(p)
  }
  maskParts.value = next
  // 勾选任一项时自动开启打码，方便直接截图
  if (next.has(p)) {
    masked.value = true
  }
  try {
    localStorage.setItem(maskPartsKey, [...next].join(','))
  } catch {
    // 存储不可用时仅本次会话生效
  }
}
// onMaskCommand 处理打码菜单：style: 前缀切换打码方式，其余为打码范围
function onMaskCommand(c: string) {
  if (c.startsWith('style:')) {
    maskStyle.value = c.slice(6) as MaskStyle
    masked.value = true
    return
  }
  toggleMaskPart(c as MaskPart)
}

// 打码方式：stars 星号遮住地址末段与域名中段，alias 把整个值换成代号；代号映射只保存在本页内存中
const MASK_STYLES: MaskStyle[] = ['stars', 'alias']
const maskStyleKeys: Record<MaskStyle, string> = {
  stars: 'webTopology.maskStars',
  alias: 'webTopology.maskAlias',
}
const maskStyleKey = 'gately.topology.maskStyle'
const maskStyle = ref<MaskStyle>(readMaskStyle())
watch(maskStyle, (v) => {
  try {
    localStorage.setItem(maskStyleKey, v)
  } catch {
    // 存储不可用时仅本次会话生效
  }
})
function readMaskStyle(): MaskStyle {
  try {
    const v = localStorage.getItem(maskStyleKey) as MaskStyle
    return MASK_STYLES.includes(v) ? v : 'stars'
  } catch {
    return 'stars'
  }
}
const aliases = new Aliases()
// aliasing 判断当前是否用代号打码
function aliasing() {
  return maskStyle.value === 'alias'
}

// hide 判断某类内容当前是否需要打码
function hide(p: MaskPart) {
  return masked.value && maskParts.value.has(p)
}

// 折叠状态与列表视图分开保存
const { isCollapsed, setCollapsed, setAll } = useCollapsed('web-topology')
const collapsedSet = computed(() => new Set((data.value?.listeners ?? []).filter((l) => isCollapsed(l.key)).map((l) => l.key)))
const listenerKeys = computed(() => (data.value?.listeners ?? []).map((l) => l.key))
const allCollapsed = computed(() => listenerKeys.value.length > 0 && listenerKeys.value.every((k) => isCollapsed(k)))
function toggleAll() {
  setAll(listenerKeys.value, !allCollapsed.value)
}
function toggleListener(n: LayoutNode) {
  if (n.listener) {
    setCollapsed(n.listener.key, !n.collapsed)
  }
}

const query = computed(() => props.query.trim())

// 筛选条件只在本页内存中保存，离开页面后恢复为显示全部
const filter = ref<Required<Omit<TopoFilter, 'active'>>>({ listeners: [], issues: false, blocked: [] })
const filtering = computed(() => filterActive(filter.value))
const filterCount = computed(() => filter.value.listeners.length + filter.value.blocked.length + (filter.value.issues ? 1 : 0))
// onFilterCommand 处理筛选菜单：issues 切换只看异常，blocked:/listener: 前缀切换对应项，clear 清除全部
function onFilterCommand(c: string) {
  const f = { ...filter.value }
  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
  if (c === 'clear') {
    f.listeners = []
    f.blocked = []
    f.issues = false
  } else if (c === 'issues') {
    f.issues = !f.issues
  } else if (c.startsWith('blocked:')) {
    f.blocked = toggle(f.blocked, c.slice(8))
  } else if (c.startsWith('listener:')) {
    f.listeners = toggle(f.listeners, c.slice(9))
  }
  filter.value = f
}
// 已删除的监听不再参与筛选，避免筛选条件残留导致画布一直为空
watch(listenerKeys, (keys) => {
  const kept = filter.value.listeners.filter((k) => keys.includes(k))
  if (kept.length !== filter.value.listeners.length) {
    filter.value = { ...filter.value, listeners: kept }
  }
})

// activeOnly 为“聚焦活动”：只看最近 60 秒有请求或有进行中请求的站点、动作与后端，按设备记住
const activeOnlyKey = 'gately.topology.activeOnly'
const activeOnly = ref(readFlag(activeOnlyKey))
watch(activeOnly, (v) => writeFlag(activeOnlyKey, v))
// activeCount 为按监听等筛选与搜索后活动中的站点数，未开启聚焦时也显示，提示有多少服务正在被访问
const activeCount = computed(() => (data.value ? countActive(filterView(data.value, filter.value), query.value) : 0))
// toggleActiveOnly 切换聚焦活动，并重新适应画布，让留下的节点完整显示
async function toggleActiveOnly() {
  activeOnly.value = !activeOnly.value
  await nextTick()
  refit()
}

// 布局方式（树形 / 径向），记住上次的选择
const layoutKey = 'gately.topology.layout'
const layoutMode = ref<LayoutMode>(readLayoutMode())
watch(layoutMode, (v) => {
  try {
    localStorage.setItem(layoutKey, v)
  } catch {
    // 存储不可用时仅本次会话生效
  }
})
function readLayoutMode(): LayoutMode {
  try {
    const v = localStorage.getItem(layoutKey) as LayoutMode
    return LAYOUT_MODES.includes(v) ? v : 'tree'
  } catch {
    return 'tree'
  }
}

// layoutMeta 为布局菜单中各布局的图标、名称与说明
const layoutIcons: Record<LayoutMode, Component> = { tree: ListTree, vertical: Network, radial: Orbit, sankey: Waves, sunburst: ChartPie, treemap: LayoutDashboard, pack: Bubbles }
const layoutKeys: Record<LayoutMode, [string, string]> = {
  tree: ['webTopology.layoutTree', 'webTopology.layoutTreeDesc'],
  vertical: ['webTopology.layoutVertical', 'webTopology.layoutVerticalDesc'],
  radial: ['webTopology.layoutRadial', 'webTopology.layoutRadialDesc'],
  sankey: ['webTopology.layoutSankey', 'webTopology.layoutSankeyDesc'],
  sunburst: ['webTopology.layoutSunburst', 'webTopology.layoutSunburstDesc'],
  treemap: ['webTopology.layoutTreemap', 'webTopology.layoutTreemapDesc'],
  pack: ['webTopology.layoutPack', 'webTopology.layoutPackDesc'],
}
const layoutMeta = computed(() => {
  const out = {} as Record<LayoutMode, { icon: Component; label: string; desc: string }>
  for (const m of LAYOUT_MODES) {
    out[m] = { icon: layoutIcons[m], label: t(layoutKeys[m][0]), desc: t(layoutKeys[m][1]) }
  }
  return out
})

// expanded 为已展开全部子节点的监听或站点（子节点超过上限时默认收起其余的）
const expanded = ref(new Set<string>())
function toggleMore(n: LayoutNode) {
  if (!n.more) {
    return
  }
  const next = new Set(expanded.value)
  if (n.more.expanded) {
    next.delete(n.more.owner)
  } else {
    next.add(n.more.owner)
  }
  expanded.value = next
}

// 聚焦活动时只保留活动中的节点，并忽略监听折叠与“还有 N 个”收起，活动中的服务全部直接显示
const layout = computed(() => {
  if (!data.value) {
    return undefined
  }
  const v = filterView(data.value, { ...filter.value, active: activeOnly.value })
  if (activeOnly.value) {
    return buildLayout(v, new Set(), query.value, { mode: layoutMode.value, limit: Infinity, upstreamLimit: Infinity })
  }
  return buildLayout(v, collapsedSet.value, query.value, { mode: layoutMode.value, expanded: expanded.value })
})

// noHover 为触屏等没有悬停的设备
const noHover = typeof window !== 'undefined' && !!window.matchMedia?.('(hover: none)').matches
// 系统开启“减少动态效果”时不播放连线动画
const reduceMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
// 连线很多时只给高亮链路播放动画，减少重绘
const motion = computed(() => !reduceMotion)
const manyEdges = computed(() => (layout.value?.edges.length ?? 0) > 120)

// HEAD 为图顶部留给列标题的高度
const HEAD = 26
const colTitles = computed(() => [t('webTopology.colInternet'), 'Gately', t('webTopology.colListener'), t('webTopology.colSite'), t('webTopology.colBackend')])

// 活动 IP
const ipInfo = computed(() => data.value?.ips.find((i) => i.ip === ipFilter.value))
// ipQuery 为下拉框中输入的搜索词：同时按原始地址与打码后的文字匹配，打码时输入完整地址也能搜到
const ipQuery = ref('')
const ipOptions = computed(() => {
  const list = data.value?.ips ?? []
  const q = ipQuery.value.trim().toLowerCase()
  if (!q) {
    return list
  }
  return list.filter((i) => i.ip.toLowerCase().includes(q) || showIP(i.ip).toLowerCase().includes(q))
})

// litSet 为需要高亮的节点集合，null 表示全部正常显示：悬停优先，其次 IP 筛选，最后选中的节点
const litSet = computed<Set<string> | null>(() => {
  const l = layout.value
  if (!l) {
    return null
  }
  // 悬停的节点可能已在刷新中被移除（此时浏览器通常不触发 mouseleave），跳过它
  if (hover.value && l.byId.has(hover.value)) {
    return pathOf(l, hover.value)
  }
  // 筛选的 IP 已不活跃时不再整图变暗，顶部文案会说明它已不活跃
  if (ipFilter.value && ipInfo.value) {
    return ipPath(l, ipInfo.value)
  }
  return pathOf(l, selected.value)
})

// 每个节点上的活动 IP 数（画布上只显示个数）
const ipCounts = computed(() => {
  const m = new Map<string, number>()
  const lay = layout.value
  for (const ip of data.value?.ips ?? []) {
    const seen = new Set<string>()
    for (const n of ip.nodes) {
      const ids = [n]
      if (n.startsWith('s:') || n.startsWith('a:')) {
        ids.push('l:' + n.slice(2).split('/')[0])
      }
      // 被收起的节点计入代替它显示的汇总节点
      const shown = lay ? resolveId(lay, n) : ''
      if (shown && shown !== n) {
        ids.push(shown)
      }
      for (const id of ids) {
        if (!seen.has(id)) {
          seen.add(id)
          m.set(id, (m.get(id) ?? 0) + 1)
        }
      }
    }
  }
  return m
})
function ipCount(id: string) {
  return ipCounts.value.get(id) ?? 0
}

function nodeClass(n: LayoutNode) {
  const lit = litSet.value
  return [
    'k-' + n.kind,
    'st-' + nodeState(n),
    {
      dim: !!lit && !lit.has(n.id),
      hot: !!lit && lit.has(n.id),
      sel: selected.value === n.id,
    },
  ]
}

function edgeClass(e: LayoutEdge) {
  const lit = litSet.value
  const on = !!lit && lit.has(e.from) && lit.has(e.to)
  return { dim: !!lit && !on, hot: on }
}

function animated(e: LayoutEdge) {
  if (!motion.value || flowLevel(e.rate) === 0) {
    return false
  }
  if (manyEdges.value) {
    const lit = litSet.value
    return !!lit && lit.has(e.from) && lit.has(e.to)
  }
  return true
}

// 连线标注只显示在高亮链路上（悬停、选中或按 IP 筛选），全部显示时会与节点文字重叠；平时速率看节点副标题
function showLabel(e: LayoutEdge) {
  const lit = litSet.value
  if (!lit || !lit.has(e.from) || !lit.has(e.to)) {
    return false
  }
  return e.rate.qps > 0 || e.rate.bps > 0
}

// nodeState 返回节点状态，决定状态点颜色：ok 正常、down 故障、off 未运行、expected 预期的拒绝或断开（非故障）、idle 未知
function nodeState(n: LayoutNode): string {
  switch (n.kind) {
    case 'internet':
    case 'gately':
      return data.value?.enabled ? 'ok' : 'off'
    case 'listener':
      if (n.listener?.error) {
        return 'down'
      }
      return n.listener?.running ? 'ok' : 'off'
    case 'site':
      if (!n.site?.running) {
        return 'off'
      }
      return n.site.type === 'close' ? 'expected' : 'ok'
    case 'action':
      return 'expected'
    case 'terminal':
      if (!n.site?.running) {
        return 'off'
      }
      return n.site.type === 'close' ? 'expected' : 'ok'
    case 'upstream':
      if (n.upstream?.healthy === undefined) {
        return 'off'
      }
      return n.upstream.healthy ? 'ok' : 'down'
    case 'more':
      return 'more'
  }
  return 'idle'
}

// icons 为画布用到的图标，在 <defs> 中各定义一次
const icons: Record<string, Component> = {
  globe: Globe,
  router: Router,
  lock: Lock,
  port: EthernetPort,
  ban: Ban,
  redirect: CornerUpRight,
  folder: FolderOpen,
  file: FileText,
  app: AppWindow,
  https: ArrowRightLeft,
  notfound: SearchX,
  unplug: Unplug,
  server: Server,
  right: ChevronRight,
  down: ChevronDown,
  more: Ellipsis,
  less: ChevronsUp,
}

// nodeIcon 返回节点图标在 icons 中的名称
function nodeIcon(n: LayoutNode): string {
  const byType = (type?: string, fallback = 'file') => (type === 'close' ? 'ban' : type === 'redirect' ? 'redirect' : type === 'static' ? 'folder' : type === 'text' ? 'file' : fallback)
  switch (n.kind) {
    case 'listener':
      return n.listener?.tls ? 'lock' : 'port'
    case 'site':
      return byType(n.site?.type, 'app')
    case 'action':
      return n.action?.kind === 'redirect' ? 'https' : 'notfound'
    case 'terminal':
      return n.site?.type === 'close' ? 'unplug' : byType(n.site?.type)
    case 'more':
      return n.more?.expanded ? 'less' : 'more'
    default:
      return 'server'
  }
}

function showIP(ip: string) {
  if (!hide('visitor')) {
    return ip
  }
  return aliasing() ? aliasIP(aliases, ip, 'Visitor') : maskIP(ip)
}

// showURL 打码后端与跳转地址：IP 与主机名都隐藏
function showURL(u: string) {
  if (!hide('backend')) {
    return u
  }
  return aliasing() ? aliasURL(aliases, u, 'Backend') : maskURL(u, true)
}

// showListenIP 打码本机监听地址（含以 IP 作为域名的站点与监听错误信息中的地址）
function showListenIP(h: string) {
  if (!hide('listener')) {
    return h
  }
  return aliasing() ? aliasHost(aliases, h, 'Listen') : maskHost(h)
}

function listenerAddr(l: NonNullable<LayoutNode['listener']>) {
  const host = l.listenIp ? showListenIP(l.listenIp) : ''
  return (host.includes(':') ? `[${host}]` : host) + ':' + l.port
}

function siteName(s: NonNullable<LayoutNode['site']>) {
  if (s.default) {
    return t('web.defaultSite')
  }
  if (s.name) {
    // 名称里常直接写域名，打码域名时一并隐藏
    if (!hide('domain')) {
      return s.name
    }
    return aliasing() ? aliasDomainText(aliases, s.name) : maskDomainText(s.name)
  }
  return (s.domains[0] ? showHost(s.domains[0]) : '') || s.key
}

// showHost 打码站点域名：以 IP 作为域名的站点是本机地址，随监听地址打码，其余随域名打码
function showHost(h: string) {
  if (isIPLiteral(h)) {
    return showListenIP(h)
  }
  if (!hide('domain')) {
    return h
  }
  return aliasing() ? aliasDomain(aliases, h) : maskDomain(h)
}

const typeKeys: Record<string, string> = {
  proxy: 'web.typeProxy',
  redirect: 'web.typeRedirect',
  static: 'web.typeStatic',
  text: 'web.typeText',
  close: 'web.typeClose',
}
// 拦截原因与防护标记的文案键（写全键名，便于文案校验脚本识别）
const blockKeys: Record<string, string> = {
  ban: 'webTopology.block_ban',
  ip: 'webTopology.block_ip',
  country: 'webTopology.block_country',
  rateLimit: 'webTopology.block_rateLimit',
  waf: 'webTopology.block_waf',
}
const guardKeys: Record<string, string> = {
  auth: 'webTopology.guard_auth',
  waf: 'webTopology.guard_waf',
  ip: 'webTopology.guard_ip',
  country: 'webTopology.guard_country',
  rateLimit: 'webTopology.guard_rateLimit',
  denyClose: 'webTopology.guard_denyClose',
}

function typeLabel(type: string) {
  return typeKeys[type] ? t(typeKeys[type]) : type
}

function matchLabel(s: NonNullable<LayoutNode['site']>) {
  if (s.default) {
    return t('webTopology.matchDefault')
  }
  if (s.domains.includes('*')) {
    return t('webTopology.matchAny')
  }
  return s.domains.some((d) => d.startsWith('*.')) ? t('webTopology.matchWildcard') : t('webTopology.matchExact')
}

function rateText(r: { qps: number; bps: number }) {
  return `${formatQps(r.qps)}/s · ${formatRate(r.bps)}`
}

function nodeTitle(n: LayoutNode): string {
  switch (n.kind) {
    case 'listener': {
      const l = n.listener!
      return (l.name ? l.name + ' · ' : '') + listenerAddr(l)
    }
    case 'site':
      return siteName(n.site!)
    case 'action':
      return n.action!.kind === 'redirect' ? t('webTopology.actionRedirect') : t('webTopology.actionNotFound')
    case 'terminal':
      return terminalTitle(n.site!)
    case 'upstream': {
      return showURL(n.upstream!.url)
    }
    case 'more': {
      const m = n.more!
      if (m.expanded) {
        return t('webTopology.showLess', { n: m.count })
      }
      return m.childKind === 'site' ? t('webTopology.moreSites', { n: m.count }) : t('webTopology.moreBackends', { n: m.count })
    }
  }
  return ''
}

function terminalTitle(s: NonNullable<LayoutNode['site']>) {
  switch (s.type) {
    case 'close':
      return t('webTopology.terminalClose')
    case 'redirect':
      // 跳转目标可能是内网地址，打码时一并隐藏
      return s.target ? showURL(s.target) : t('web.typeRedirect')
    case 'static':
      return s.target || t('web.typeStatic')
    default:
      return t('webTopology.terminalText', { code: s.target || '200' })
  }
}

function nodeSub(n: LayoutNode): string {
  switch (n.kind) {
    case 'listener': {
      const l = n.listener!
      if (!l.running) {
        return l.error ? t('webTopology.listenerError') : t('webTopology.notRunning')
      }
      const extra = n.collapsed ? ' · ' + t('webTopology.folded', { n: n.hidden ?? 0 }) : ''
      return rateText(l.rate) + extra
    }
    case 'site': {
      const s = n.site!
      if (!s.running) {
        return s.enable ? t('webTopology.notRunning') : t('webTopology.disabled')
      }
      return `${matchLabel(s)} · ${formatQps(s.rate.qps)}/s · ${t('webTopology.activeShort', { n: s.active })}`
    }
    case 'action':
      return `${n.action!.kind === 'redirect' ? '301 / 308' : '404'} · ${formatQps(n.action!.rate.qps)}/s`
    case 'terminal': {
      const s = n.site!
      if (s.type === 'close') {
        return t('webTopology.expected') + ' · 444'
      }
      return typeLabel(s.type) + ' · ' + rateText(s.rate)
    }
    case 'upstream': {
      const u = n.upstream!
      const scope = u.scope === 'location' ? t('webTopology.scopeLocation', { m: u.match }) : u.scope === 'user' ? t('webTopology.scopeUser', { m: u.match }) : ''
      const health = u.healthy === undefined ? t('webTopology.upUnknown') : u.healthy ? t('webTopology.upHealthy') : t('webTopology.upDown')
      return [scope, u.backup ? t('webTopology.upBackup') : '', health, formatQps(u.rate.qps) + '/s'].filter(Boolean).join(' · ')
    }
    case 'more': {
      const m = n.more!
      if (m.expanded) {
        return t('webTopology.lessHint')
      }
      // 活动中的节点不会被收起，汇总节点里通常都是空闲的
      if (!m.active && !m.rate.qps && !m.rate.qps60) {
        return t('webTopology.moreIdle')
      }
      return t('webTopology.moreTotal', { qps: formatQps(m.rate.qps), n: m.active })
    }
  }
  return ''
}

function hubSub(n: LayoutNode) {
  const v = data.value
  if (!v) {
    return ''
  }
  if (n.kind === 'internet') {
    return t('webTopology.internetSub', { n: v.ipTotal })
  }
  const running = v.listeners.filter((l) => l.running).length
  return t('webTopology.gatelySub', { running, total: v.listeners.length })
}

// fit 按估算宽度截断文本（SVG 文本不会自动省略）：ASCII 约 6.6px，其余字符约 12px
function fit(s: string, px: number): string {
  let w = 0
  for (let i = 0; i < s.length; i++) {
    w += s.charCodeAt(i) < 128 ? 6.6 : 12
    if (w > px) {
      return s.slice(0, Math.max(1, i - 1)) + '…'
    }
  }
  return s
}

function ago(sec: number) {
  const now = data.value?.time ?? Math.floor(Date.now() / 1000)
  const d = Math.max(0, now - sec)
  return d <= 1 ? t('webTopology.ipNow') : t('webTopology.ipAgo', { s: d })
}

// ---- 详情面板 ----

interface Metric {
  label: string
  value: string
  help?: string
  cls?: string
}
interface Detail {
  title: string
  sub: string
  error?: string
  tags: { text: string; type?: 'success' | 'warning' | 'danger' | 'info' | 'primary' }[]
  metrics: Metric[]
  rows: Metric[]
  actions: { key: string; label: string; icon: Component; run: () => void }[]
}

const selectedNode = computed(() => layout.value?.byId.get(selected.value) ?? layout.value?.byId.get('gately'))
// detailFocus 为接口 focus 参数：去向节点的曲线用所属站点的，互联网与 Gately 都是整个实例
const detailFocus = computed(() => {
  const id = selectedNode.value?.id ?? 'gately'
  if (id.startsWith('t:')) {
    return 's:' + id.slice(2)
  }
  return id === 'internet' ? 'gately' : id
})

const seriesMax = computed(() => Math.max(0, ...(data.value?.series ?? []).map((p) => p.requests)))

function rateMetrics(r: { qps: number; bps: number; qps60: number }, bpsHelp?: string): Metric[] {
  return [
    { label: t('webTopology.qps'), value: formatQps(r.qps), help: t('webTopology.qpsHelp') },
    { label: t('webTopology.qps60'), value: formatQps(r.qps60), help: t('webTopology.qps60Help') },
    { label: t('webTopology.bps'), value: formatRate(r.bps), help: bpsHelp ?? t('webTopology.bpsHelp') },
  ]
}

const detail = computed<Detail | undefined>(() => {
  const n = selectedNode.value
  const v = data.value
  if (!n || !v) {
    return undefined
  }
  const d: Detail = { title: '', sub: '', tags: [], metrics: [], rows: [], actions: [] }
  switch (n.kind) {
    case 'internet':
    case 'gately': {
      d.title = n.kind === 'internet' ? t('webTopology.internet') : 'Gately'
      d.sub = n.kind === 'internet' ? t('webTopology.internetDesc') : hubSub(n)
      if (!v.enabled) {
        d.tags.push({ text: t('webTopology.moduleOff'), type: 'info' })
      }
      d.metrics = [
        ...rateMetrics(v.rate),
        { label: t('webTopology.active'), value: String(v.active), help: t('webTopology.activeHelp') },
        { label: t('webTopology.requests'), value: formatCount(v.requests), help: t('webTopology.requestsHelp') },
        { label: t('webTopology.activeIps'), value: String(v.ipTotal), help: t('webTopology.activeIpsHelp') },
      ]
      break
    }
    case 'listener': {
      const l = n.listener!
      d.title = l.name || listenerAddr(l)
      d.sub = [listenerAddr(l), l.tls ? 'HTTPS' : 'HTTP', l.http3 ? 'HTTP/3' : ''].filter(Boolean).join(' · ')
      // 错误信息里常带监听地址（如 listen tcp 192.168.1.5:443: bind: ...）
      d.error = hide('listener') && l.error ? (aliasing() ? aliasText(aliases, l.error, 'Listen') : maskText(l.error)) : l.error
      d.tags.push(l.running ? { text: t('webTopology.running'), type: 'success' } : { text: l.enable ? t('webTopology.notRunning') : t('webTopology.disabled'), type: 'info' })
      if (l.tls && l.plainHttp === 'redirect') {
        d.tags.push({ text: t('webTopology.plainRedirect') })
      } else if (l.tls && l.plainHttp === 'serve') {
        d.tags.push({ text: t('webTopology.plainServe') })
      }
      if (l.proxyProtocol) {
        d.tags.push({ text: 'PROXY protocol' })
      }
      d.metrics = [
        ...rateMetrics(l.rate),
        { label: t('webTopology.active'), value: String(l.active), help: t('webTopology.activeHelp') },
        { label: t('webTopology.requests'), value: formatCount(l.requests), help: t('webTopology.requestsHelp') },
        { label: t('webTopology.sites'), value: String(l.sites.length) },
      ]
      // 桑基、旭日、矩形树图与气泡布局的画布上没有折叠按钮，在面板中提供
      if (!activeOnly.value) {
        d.actions.push({ key: 'fold', label: n.collapsed ? t('common.expand') : t('common.collapse'), icon: n.collapsed ? ChevronsUpDown : ChevronsDownUp, run: () => toggleListener(n) })
      }
      d.actions.push({ key: 'edit', label: t('webTopology.editListener'), icon: Pencil, run: () => emit('editListener', n.id.slice(2)) })
      break
    }
    case 'site': {
      const s = n.site!
      const lk = n.parent!.slice(2)
      d.title = siteName(s)
      d.sub = `${typeLabel(s.type)} · ${matchLabel(s)}${s.group ? ' · ' + s.group : ''}`
      d.tags.push(s.running ? { text: t('webTopology.running'), type: 'success' } : { text: s.enable ? t('webTopology.notRunning') : t('webTopology.disabled'), type: 'info' })
      if (s.type === 'close') {
        d.tags.push({ text: t('webTopology.expected'), type: 'info' })
      }
      for (const g of s.guards) {
        d.tags.push({ text: guardKeys[g] ? t(guardKeys[g]) : g, type: 'primary' })
      }
      d.metrics = [
        ...rateMetrics(s.rate),
        { label: t('webTopology.active'), value: String(s.active), help: t('webTopology.activeHelp') },
        { label: t('webTopology.requests'), value: formatCount(s.requests), help: t('webTopology.requestsHelp') },
        { label: t('webTopology.errors'), value: formatCount(s.errors), help: t('webTopology.errorsHelp'), cls: s.errors ? 'bad' : '' },
      ]
      const blockedTotal = Object.values(s.blocked).reduce((a, b) => a + b, 0)
      d.rows.push({ label: t('webTopology.blocked'), value: formatCount(blockedTotal), help: t('webTopology.blockedHelp'), cls: blockedTotal ? 'warn' : '' })
      for (const [k, label] of Object.entries(blockKeys)) {
        if (s.blocked[k]) {
          d.rows.push({ label: '　' + t(label), value: formatCount(s.blocked[k]), cls: 'warn' })
        }
      }
      d.rows.push({ label: t('webTopology.status'), value: ['2xx', '3xx', '4xx', '5xx'].map((c) => `${c} ${formatCount(s.status[c] ?? 0)}`).join(' · ') })
      if (s.closed) {
        d.rows.push({ label: t('webTopology.closed'), value: formatCount(s.closed) })
      }
      if (s.domains.length && !s.default) {
        d.rows.push({ label: t('webTopology.domains'), value: s.domains.map(showHost).join(', ') })
      }
      if (s.running) {
        d.actions.push({ key: 'stats', label: t('webTopology.openStats'), icon: BarChart3, run: () => emit('stats', lk, s.key, siteName(s)) })
        d.actions.push({ key: 'conns', label: t('webTopology.openConns'), icon: ArrowRightLeft, run: () => emit('conns', lk, s.key, siteName(s)) })
      }
      // 未命名默认站点无法按站点过滤，只在已按 IP 筛选时提供（查看该 IP 在全部站点的日志）
      if (logSite.value || ipFilter.value) {
        d.actions.push({ key: 'logs', label: ipFilter.value ? t('webTopology.openLogsIp') : t('webTopology.openLogs'), icon: FileText, run: () => openLogs(ipFilter.value) })
      }
      d.actions.push({ key: 'edit', label: t('common.edit'), icon: Pencil, run: () => emit('edit', lk, s.key) })
      break
    }
    case 'action': {
      const a = n.action!
      d.title = a.kind === 'redirect' ? t('webTopology.actionRedirect') : t('webTopology.actionNotFound')
      d.sub = a.kind === 'redirect' ? t('webTopology.actionRedirectDesc') : t('webTopology.actionNotFoundDesc')
      d.tags.push({ text: t('webTopology.expected'), type: 'info' })
      d.metrics = [...rateMetrics(a.rate), { label: t('webTopology.requests'), value: formatCount(a.requests), help: t('webTopology.requestsHelp') }]
      break
    }
    case 'terminal': {
      const s = n.site!
      d.title = terminalTitle(s)
      d.sub = s.type === 'close' ? t('webTopology.terminalCloseDesc') : `${typeLabel(s.type)} · ${siteName(s)}`
      if (s.type === 'close') {
        d.tags.push({ text: t('webTopology.expected'), type: 'info' })
      }
      d.metrics = [...rateMetrics(s.rate), { label: t('webTopology.requests'), value: formatCount(s.requests), help: t('webTopology.requestsHelp') }]
      break
    }
    case 'upstream': {
      const u = n.upstream!
      d.title = showURL(u.url)
      d.sub = siteName(n.site!)
      if (u.healthy === undefined) {
        d.tags.push({ text: t('webTopology.upUnknown'), type: 'info' })
      } else {
        d.tags.push(u.healthy ? { text: t('webTopology.upHealthy'), type: 'success' } : { text: t('webTopology.upDown'), type: 'danger' })
      }
      if (u.scope === 'location') {
        d.tags.push({ text: t('webTopology.scopeLocation', { m: u.match }) })
      } else if (u.scope === 'user') {
        d.tags.push({ text: t('webTopology.scopeUser', { m: u.match }) })
      }
      if (u.backup) {
        d.tags.push({ text: t('webTopology.upBackup'), type: 'warning' })
      }
      d.metrics = [
        ...rateMetrics(u.rate, t('webTopology.bpsUpstreamHelp')),
        { label: t('webTopology.conns'), value: String(u.conns), help: t('webTopology.connsHelp') },
        { label: t('webTopology.requests'), value: formatCount(u.requests), help: t('webTopology.upRequestsHelp') },
        { label: t('webTopology.upBytes'), value: formatBytes(u.bytes), help: t('webTopology.bpsUpstreamHelp') },
      ]
      d.rows.push({ label: t('webTopology.weight'), value: String(u.weight) })
      // 访问日志只记录到站点，按后端所属站点查看
      // 未命名默认站点无法按站点过滤，只在已按 IP 筛选时提供（查看该 IP 在全部站点的日志）
      if (logSite.value || ipFilter.value) {
        d.actions.push({ key: 'logs', label: ipFilter.value ? t('webTopology.openLogsIp') : t('webTopology.openLogs'), icon: FileText, run: () => openLogs(ipFilter.value) })
      }
      d.actions.push({ key: 'edit', label: t('common.edit'), icon: Pencil, run: () => emit('edit', n.site!.id.slice(2).split('/')[0], n.site!.key) })
      break
    }
  }
  return d
})

// 详情面板的活动 IP：经过选中节点的地址，n 为它在该节点上进行中的请求数；进行中的排在前面
const panelIps = computed<(TopoIP & { n: number })[]>(() => {
  const node = selectedNode.value
  if (!node) {
    return []
  }
  const list = (data.value?.ips ?? []).filter((ip) => nodeCovers(ip, node.id, node.more?.ids)).map((ip) => ({ ...ip, n: activeAt(ip, node.id, node.more?.ids) }))
  // 接口已按总进行中数与最后出现时间排序，这里只把该节点上进行中的提到前面（稳定排序保留原顺序）
  return list.sort((a, b) => (b.n > 0 ? 1 : 0) - (a.n > 0 ? 1 : 0))
})

// ---- 查看访问日志 ----

// logSite 为当前选中节点所属的站点：站点、后端与终点节点都对应一个站点，其余节点没有。
// 未命名的默认站点在访问日志里没有站点字段，也没有域名可匹配，无法单独过滤，按未选中站点处理
const logSite = computed(() => {
  const n = selectedNode.value
  const s = n && (n.kind === 'site' || n.kind === 'upstream' || n.kind === 'terminal') ? n.site : undefined
  return s && s.default && !s.name ? undefined : s
})

// openLogs 打开访问日志：选中站点类节点时按该站点过滤，ip 非空时再按访客地址过滤；
// 未选中站点时只按地址查看全部站点的访问日志
function openLogs(ip: string) {
  const s = logSite.value
  emit('logs', {
    // 默认站点有名称时访问日志同样带站点字段，按名称过滤；否则会把全部站点的日志当成默认站点的显示
    name: s ? s.name : '',
    label: s ? siteName(s) : '',
    domain: s && !s.default ? s.domains[0] : undefined,
    ip: ip || undefined,
    ipLabel: ip ? showIP(ip) : undefined,
    accessLog: s ? !!s.accessLog : true,
  })
}

// ---- 交互 ----

function select(id: string) {
  // 曲线所属的节点变化后由下方对 detailFocus 的监听立即刷新
  selected.value = id
}

// 点击汇总节点展开或收起，其余节点选中后在详情面板展示
function onNodeClick(n: LayoutNode) {
  if (n.kind === 'more') {
    toggleMore(n)
    return
  }
  select(n.id)
}

// radialLabel 返回径向布局中节点文字的位置：叶子的文字朝外，有子节点的朝内；左半边翻转 180° 保持文字正向
function radialLabel(n: LayoutNode) {
  const r = n.w / 2
  const a = n.angle ?? 0
  const deg = (a * 180) / Math.PI
  const flip = Math.cos(a) < -1e-6
  const outward = !!n.leaf
  const off = r + 5
  const max = outward ? RADIAL_LABEL - 24 : n.kind === 'listener' ? 92 : 140
  return {
    attrs: {
      transform: `translate(${r},${r}) rotate(${flip ? deg + 180 : deg})`,
      x: (outward ? off : -off) * (flip ? -1 : 1),
      'text-anchor': outward !== flip ? 'start' : 'end',
    },
    max,
  }
}

// sunLabel 返回旭日图扇区文字的位置与方向：沿半径方向，左半边翻转保持正向
function sunLabel(n: LayoutNode) {
  const l = n.label!
  const deg = (l.angle * 180) / Math.PI
  const flip = Math.cos(l.angle) < -1e-6
  return { transform: `translate(${l.x},${l.y}) rotate(${flip ? deg + 180 : deg})` }
}

// nestTitle 返回矩形树图、气泡图中父节点（容器）的名称与 QPS
function nestTitle(n: LayoutNode) {
  if (n.kind === 'gately') {
    return `Gately · ${formatQps(data.value?.rate.qps ?? 0)}/s`
  }
  const r = n.listener?.rate ?? n.site?.rate ?? n.more?.rate
  return nodeTitle(n) + (r ? ` · ${formatQps(r.qps)}/s` : '')
}

// shortTitle 为小方块、小圆中的名称：后端地址去掉 http:// 或 https:// 前缀，多显示一些主机信息
function shortTitle(n: LayoutNode) {
  const s = nodeTitle(n)
  return n.kind === 'upstream' || n.kind === 'terminal' ? s.replace(/^https?:\/\//, '') : s
}

// packHeadMax 返回气泡图父节点圆内顶部（距顶边 15px 处）可放文字的宽度
function packHeadMax(n: LayoutNode) {
  const r = n.w / 2
  const d = r - 11
  return d > 0 && d < r ? 2 * Math.sqrt(r * r - d * d) - 12 : 0
}

// sunOpacity 按 QPS 对数增长的扇区填充不透明度，流量越大颜色越深
function sunOpacity(n: LayoutNode) {
  const r = n.listener?.rate ?? n.action?.rate ?? n.upstream?.rate ?? n.more?.rate ?? n.site?.rate
  const q = r ? Math.max(r.qps, r.qps60) : 0
  return 0.22 + 0.6 * Math.min(1, Math.log10(1 + q) / 2)
}

// ---- 平移与缩放 ----

// box 为图内容的范围（SVG viewBox）：树形布局在顶部多留出列标题的高度
const box = computed(() => {
  const l = layout.value
  if (!l) {
    return { x: 0, y: 0, w: 1, h: 1 }
  }
  return isColumnar(l.mode) ? { x: 0, y: -HEAD, w: l.width, h: l.height + HEAD + 8 } : { x: 0, y: 0, w: l.width, h: l.height }
})

// 径向布局三层圆环的半径（取各层任一节点到圆心的距离）
const ringRadii = computed(() => {
  const l = layout.value
  if (!l || l.mode !== 'radial') {
    return []
  }
  const out: number[] = []
  for (const col of [2, 3, 4]) {
    const n = l.nodes.find((x) => x.col === col)
    if (n) {
      out.push(Math.round(Math.hypot(n.x + n.w / 2 - l.width / 2, n.y + n.h / 2 - l.height / 2)))
    }
  }
  return out
})

const canvasEl = ref<HTMLElement>()
const rootEl = ref<HTMLElement>()
const viewW = ref(800)
const rootW = ref(1200)
const winH = ref(typeof window !== 'undefined' ? window.innerHeight : 800)
const scale = ref(1)
const tx = ref(0)
const ty = ref(0)
// moving 为平移缩放进行中（期间提示浏览器合成层，结束后重新清晰绘制），animating 为按钮触发的平滑过渡
const moving = ref(false)
const animating = ref(false)
const dragging = ref(false)
const MIN_SCALE = 0.15
const MAX_SCALE = 2.5
// 内容可以拖出视口的最大距离
const EDGE = 48

// treeScale 为树形布局默认的缩放：适应宽度，最小 0.5 倍（窄屏下文字不至于太小，可再手动缩小）
const treeScale = computed(() => Math.min(1, Math.max(0.5, viewW.value / box.value.w)))

// userH 为用户拖动设定的画布高度（0 表示自动），按设备记住
const heightKey = 'gately.topology.height'
const MIN_H = 220
const userH = ref(readHeight())
function readHeight() {
  try {
    const n = Number(localStorage.getItem(heightKey))
    return Number.isFinite(n) && n >= MIN_H ? n : 0
  } catch {
    return 0
  }
}
// maxUserH 为可设定的最大高度：窗口高度的 3 倍，足够把较大的拓扑完整展开后整页滚动查看
const maxUserH = computed(() => Math.max(MIN_H + 100, winH.value * 3))

// full 为画布铺满当前窗口，fullH 为此时画布可用的高度
const full = ref(false)
const fullH = ref(600)

// viewH 为视口高度：全屏时铺满；用户设定过高度时用设定值；否则按默认缩放时内容的高度，最多到窗口可用高度（面板在画布下方时为 70% 窗口高度）
const viewH = computed(() => {
  if (full.value) {
    return Math.max(MIN_H, fullH.value)
  }
  if (userH.value) {
    return Math.round(Math.min(maxUserH.value, Math.max(MIN_H, userH.value)))
  }
  const maxH = rootW.value <= 1100 ? winH.value * 0.7 : winH.value - 230
  const mode = layout.value?.mode
  // 竖向树按原始大小显示高度（横向拖动查看），按列排列的布局适应宽度，其余按宽度等比缩放
  const s = mode === 'vertical' ? 1 : mode && !isColumnar(mode) ? Math.min(1, viewW.value / box.value.w) : treeScale.value
  return Math.round(Math.max(320, Math.min(box.value.h * s + 2, Math.max(360, maxH))))
})

function clampScale(s: number) {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, s))
}

// clampPan 限制平移范围：内容比视口小时居中（树形布局靠上），否则最多拖出 EDGE 像素
function clampPan(x: number, y: number, s: number): [number, number] {
  const cw = box.value.w * s
  const ch = box.value.h * s
  const vw = viewW.value
  const vh = viewH.value
  const nx = cw <= vw ? (vw - cw) / 2 : Math.min(EDGE, Math.max(vw - cw - EDGE, x))
  const ny = ch <= vh ? (layout.value && isColumnar(layout.value.mode) ? 0 : (vh - ch) / 2) : Math.min(EDGE, Math.max(vh - ch - EDGE, y))
  return [nx, ny]
}

function setView(s: number, x: number, y: number) {
  scale.value = s
  ;[tx.value, ty.value] = clampPan(x, y, s)
}

// zoomAt 以视口内的点 (px, py) 为中心缩放到 s，该点下的内容保持不动
function zoomAt(s: number, px: number, py: number) {
  const ns = clampScale(s)
  const cx = (px - tx.value) / scale.value
  const cy = (py - ty.value) / scale.value
  setView(ns, px - cx * ns, py - cy * ns)
}

let animTimer: number | undefined
function animate() {
  userMoved = true
  animating.value = true
  window.clearTimeout(animTimer)
  animTimer = window.setTimeout(() => (animating.value = false), 260)
}

function zoomBy(f: number) {
  animate()
  zoomAt(scale.value * f, viewW.value / 2, viewH.value / 2)
}

function zoomTo(s: number) {
  animate()
  zoomAt(s, viewW.value / 2, viewH.value / 2)
}

// fitView 让内容适应视口：all 为显示全部；否则树形与桑基布局只适应宽度（从顶部开始），
// 竖向树只适应高度并以 Gately 为水平中心（横向拖动查看），其余布局显示全部
function fitView(all = false, smooth = false) {
  // 视口宽度可能还没被 ResizeObserver 更新，先直接读取
  viewW.value = canvasEl.value?.clientWidth || viewW.value
  const sw = viewW.value / box.value.w
  const sh = viewH.value / box.value.h
  const mode = layout.value?.mode
  if (smooth) {
    animate()
  }
  // 聚焦活动时节点通常不多，各布局都尽量完整显示，不必再滚动查找；树形等布局最小 0.5 倍，避免文字太小
  if (!all && activeOnly.value) {
    const floor = mode && (isColumnar(mode) || mode === 'vertical') ? 0.5 : MIN_SCALE
    setView(Math.min(1, Math.max(floor, Math.min(sw, sh))), 0, 0)
    return
  }
  if (!all && mode === 'vertical') {
    const s = Math.min(1, Math.max(0.5, sh))
    const g = layout.value!.byId.get('gately')
    const gx = g ? g.x + g.w / 2 : box.value.w / 2
    setView(s, viewW.value / 2 - gx * s, 0)
    return
  }
  const tree = !!mode && isColumnar(mode)
  const s = all || !tree ? Math.min(1, Math.max(MIN_SCALE, Math.min(sw, sh))) : treeScale.value
  setView(s, 0, 0)
}

// refit 重新适应视口，并回到“未手动调整”状态
function refit() {
  userMoved = false
  fitView()
}

// userMoved 表示用户已手动平移或缩放过；在此之前视口尺寸变化时自动重新适应
let userMoved = false
let moveTimer: number | undefined
function markMoving() {
  userMoved = true
  moving.value = true
  window.clearTimeout(moveTimer)
  moveTimer = window.setTimeout(() => (moving.value = false), 160)
}

function localPoint(e: { clientX: number; clientY: number }) {
  const r = canvasEl.value!.getBoundingClientRect()
  return { x: e.clientX - r.left, y: e.clientY - r.top }
}

// 滚轮平移画布，已到边缘时交给页面滚动；按住 Ctrl/⌘（触控板捏合时浏览器也会带上 Ctrl）时缩放
function onWheel(e: WheelEvent) {
  if (!layout.value) {
    return
  }
  const unit = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? viewH.value : 1
  if (e.ctrlKey || e.metaKey) {
    e.preventDefault()
    const d = Math.max(-60, Math.min(60, e.deltaY * unit))
    const p = localPoint(e)
    zoomAt(scale.value * Math.exp(-d * 0.01), p.x, p.y)
    markMoving()
    return
  }
  const [nx, ny] = clampPan(tx.value - e.deltaX * unit, ty.value - e.deltaY * unit, scale.value)
  if (Math.abs(nx - tx.value) < 0.5 && Math.abs(ny - ty.value) < 0.5) {
    return
  }
  e.preventDefault()
  tx.value = nx
  ty.value = ny
  markMoving()
}

// 指针拖动平移、双指捏合缩放；拖动过后吞掉随之而来的点击，避免误选节点
const pointers = new Map<number, { x: number; y: number }>()
let dragStart: { x: number; y: number; tx: number; ty: number } | null = null
let pinch: { d: number; s: number; cx: number; cy: number } | null = null
let suppressClick = false

function startPinch() {
  const [a, b] = [...pointers.values()]
  const mx = (a.x + b.x) / 2
  const my = (a.y + b.y) / 2
  pinch = { d: Math.max(1, Math.hypot(a.x - b.x, a.y - b.y)), s: scale.value, cx: (mx - tx.value) / scale.value, cy: (my - ty.value) / scale.value }
}

// capture 捕获指针，拖出元素范围后仍能收到移动事件；指针已失效时忽略
function capture(el: HTMLElement | undefined, id: number) {
  try {
    el?.setPointerCapture?.(id)
  } catch {
    // 指针已抬起或不存在
  }
}

function onPointerDown(e: PointerEvent) {
  if (!layout.value || (e.pointerType === 'mouse' && e.button !== 0)) {
    return
  }
  pointers.set(e.pointerId, localPoint(e))
  if (pointers.size === 1) {
    suppressClick = false
    const p = pointers.get(e.pointerId)!
    dragStart = { x: p.x, y: p.y, tx: tx.value, ty: ty.value }
  } else if (pointers.size === 2) {
    dragStart = null
    suppressClick = true
    startPinch()
  }
}

function onPointerMove(e: PointerEvent) {
  if (!pointers.has(e.pointerId)) {
    return
  }
  const p = localPoint(e)
  pointers.set(e.pointerId, p)
  if (pinch && pointers.size >= 2) {
    const [a, b] = [...pointers.values()]
    const s = clampScale((pinch.s * Math.hypot(a.x - b.x, a.y - b.y)) / pinch.d)
    setView(s, (a.x + b.x) / 2 - pinch.cx * s, (a.y + b.y) / 2 - pinch.cy * s)
    markMoving()
    return
  }
  if (!dragStart) {
    return
  }
  const dx = p.x - dragStart.x
  const dy = p.y - dragStart.y
  if (!dragging.value) {
    // 移动超过 4 像素才算拖动，轻点仍是点击
    if (Math.hypot(dx, dy) < 4) {
      return
    }
    dragging.value = true
    suppressClick = true
    capture(canvasEl.value, e.pointerId)
  }
  setView(scale.value, dragStart.tx + dx, dragStart.ty + dy)
  markMoving()
}

function onPointerUp(e: PointerEvent) {
  pointers.delete(e.pointerId)
  if (pointers.size < 2) {
    pinch = null
  }
  if (pointers.size === 1) {
    // 双指松开一指后继续用剩下的手指拖动
    const p = [...pointers.values()][0]
    dragStart = { x: p.x, y: p.y, tx: tx.value, ty: ty.value }
    dragging.value = true
  } else if (!pointers.size) {
    dragStart = null
    dragging.value = false
  }
}

function onClickCapture(e: MouseEvent) {
  if (suppressClick) {
    suppressClick = false
    e.stopImmediatePropagation()
    e.preventDefault()
  }
}

// Safari 的触控板捏合不产生 Ctrl+滚轮，而是 gesture 事件
let gestureScale = 1
function onGestureStart(e: Event) {
  e.preventDefault()
  gestureScale = scale.value
}
function onGestureChange(e: Event) {
  e.preventDefault()
  const g = e as Event & { scale: number; clientX: number; clientY: number }
  const p = localPoint(g)
  zoomAt(gestureScale * g.scale, p.x, p.y)
  markMoving()
}

let resizeObs: ResizeObserver | undefined
function onWinResize() {
  winH.value = window.innerHeight
  nextTick(measureFull)
}

// 首次拿到数据、切换布局时重新适应视口；内容尺寸或视口变化时只修正平移范围
let fitted = false
watch(
  () => !!layout.value,
  async (has) => {
    if (has && !fitted) {
      fitted = true
      await nextTick()
      refit()
    }
  },
)
watch(layoutMode, async () => {
  await nextTick()
  refit()
})
// 视口或内容尺寸变化（展开、聚焦活动时服务出现或消失）时，未手动平移缩放过就重新适应，否则只修正平移范围
watch([box, viewW, viewH], () => {
  if (!userMoved && layout.value) {
    fitView()
    return
  }
  setView(scale.value, tx.value, ty.value)
})

// ---- 调整高度 ----

let gripStart: { y: number; h: number } | null = null
const gripping = ref(false)
function onGripDown(e: PointerEvent) {
  if (e.pointerType === 'mouse' && e.button !== 0) {
    return
  }
  gripStart = { y: e.clientY, h: viewH.value }
  gripping.value = true
  capture(e.currentTarget as HTMLElement, e.pointerId)
  e.preventDefault()
}
function onGripMove(e: PointerEvent) {
  if (!gripStart) {
    return
  }
  userH.value = Math.round(Math.min(maxUserH.value, Math.max(MIN_H, gripStart.h + e.clientY - gripStart.y)))
}
function onGripUp() {
  if (!gripStart) {
    return
  }
  gripStart = null
  gripping.value = false
  try {
    localStorage.setItem(heightKey, String(userH.value))
  } catch {
    // 存储不可用时仅本次会话生效
  }
}
function resetHeight() {
  userH.value = 0
  try {
    localStorage.removeItem(heightKey)
  } catch {
    // 忽略
  }
}

// ---- 窗口内全屏 ----

// 全屏时锁定页面滚动，只允许拓扑区域内部（详情面板、IP 列表）滚动
const lockScroll = useScrollLock('.topo.full')
async function toggleFull() {
  full.value = !full.value
  lockScroll(full.value)
  await nextTick()
  measureFull()
  refit()
}
// measureFull 计算全屏时画布可用的高度：窗口高度减去画布上方的工具栏与下方的说明
function measureFull() {
  if (!full.value || !canvasEl.value || !rootEl.value) {
    return
  }
  // 进入全屏后容器宽度立即变化，ResizeObserver 可能还没回调，直接读取
  rootW.value = rootEl.value.clientWidth || rootW.value
  const top = canvasEl.value.getBoundingClientRect().top - rootEl.value.getBoundingClientRect().top
  const foot = rootEl.value.querySelector('.topo-foot')?.getBoundingClientRect().height ?? 0
  // 详情面板在画布下方（窄屏）时不预留高度，画布占满一屏，向下滚动查看面板
  fullH.value = Math.floor(winH.value - top - (rootW.value <= 1100 ? 0 : foot) - 36)
}
function onKeydown(e: KeyboardEvent) {
  // 弹窗打开时 Esc 先关闭弹窗
  if (e.key === 'Escape' && full.value && !document.querySelector('.el-overlay:not([style*="display: none"])')) {
    toggleFull()
  }
}

function setIp(ip: string) {
  ipFilter.value = ip
  ipModel.value = ip
  reload()
}

function onIpChange(v: string) {
  const s = (v ?? '').trim()
  // 手动输入的不完整地址（如 1.2.3）不交给后端，否则每次刷新都会报错、画布停在旧数据
  if (s && !isValidIP(s)) {
    ElMessage.warning(t('webTopology.ipInvalid'))
    ipModel.value = ipFilter.value
    return
  }
  ipFilter.value = s
  reload()
}

function clearIp() {
  ipFilter.value = ''
  ipModel.value = ''
  reload()
}

// ---- 轮询 ----

let timer: number | undefined
let seq = 0
let alive = true

async function load() {
  const my = ++seq
  const focus = detailFocus.value
  try {
    const q = new URLSearchParams({ focus })
    // 筛选值在选择时已校验为完整地址
    if (ipFilter.value && isValidIP(ipFilter.value)) {
      q.set('ip', ipFilter.value)
    }
    const v = await api.get<TopologyView>('/api/webservice/topology?' + q.toString())
    if (my !== seq || !alive) {
      return
    }
    data.value = v
    seriesFor.value = focus
    loadError.value = ''
    updatedAt.value = new Date().toLocaleTimeString()
  } catch (e) {
    if (my === seq && alive) {
      loadError.value = (e as Error).message
    }
  }
}

// schedule 上一次请求结束后再安排下一次，避免慢网络下请求堆积；页面在后台时暂停
function schedule() {
  window.clearTimeout(timer)
  timer = window.setTimeout(async () => {
    if (!alive) {
      return
    }
    if (document.hidden) {
      paused.value = true
      schedule()
      return
    }
    paused.value = false
    await load()
    if (alive) {
      schedule()
    }
  }, 2000)
}

async function reload() {
  window.clearTimeout(timer)
  await load()
  if (alive) {
    schedule()
  }
}

function onVisibility() {
  if (!document.hidden && paused.value) {
    paused.value = false
    reload()
  }
}

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibility)
  window.addEventListener('resize', onWinResize)
  document.addEventListener('keydown', onKeydown)
  const el = canvasEl.value
  if (el) {
    el.addEventListener('gesturestart', onGestureStart, { passive: false })
    el.addEventListener('gesturechange', onGestureChange, { passive: false })
  }
  resizeObs = new ResizeObserver(() => {
    viewW.value = canvasEl.value?.clientWidth || viewW.value
    rootW.value = rootEl.value?.clientWidth || rootW.value
  })
  if (el) {
    resizeObs.observe(el)
  }
  if (rootEl.value) {
    resizeObs.observe(rootEl.value)
  }
  reload()
})

onBeforeUnmount(() => {
  alive = false
  window.clearTimeout(timer)
  window.clearTimeout(animTimer)
  window.clearTimeout(moveTimer)
  document.removeEventListener('visibilitychange', onVisibility)
  window.removeEventListener('resize', onWinResize)
  document.removeEventListener('keydown', onKeydown)
  canvasEl.value?.removeEventListener('gesturestart', onGestureStart)
  canvasEl.value?.removeEventListener('gesturechange', onGestureChange)
  resizeObs?.disconnect()
})

// 选中的节点因折叠或搜索暂时消失时，详情面板回落到 Gately（见 selectedNode），节点重新出现后恢复选中；
// 曲线所属节点变化时立即刷新，避免最多 2 秒没有曲线
watch(detailFocus, (v, old) => {
  if (v !== old) {
    reload()
  }
})

function readFlag(k: string) {
  try {
    return localStorage.getItem(k) === '1'
  } catch {
    return false
  }
}

function writeFlag(k: string, v: boolean) {
  try {
    localStorage.setItem(k, v ? '1' : '0')
  } catch {
    // 存储不可用时仅本次会话生效
  }
}

defineExpose({ reload })
</script>

<style scoped>
.topo {
  /* 画布配色跟随当前皮肤：亮色皮肤用浅色画布，暗色皮肤见下方 html.dark */
  --topo-accent: var(--el-color-primary);
  --topo-bg: color-mix(in srgb, var(--el-color-primary) 3%, var(--el-bg-color));
  --topo-bg2: color-mix(in srgb, var(--el-color-primary) 6%, var(--el-bg-color-page));
  --topo-card: var(--el-bg-color);
  --topo-card-hi: color-mix(in srgb, var(--el-color-primary) 6%, var(--el-bg-color));
  --topo-card-stroke: color-mix(in srgb, var(--el-text-color-primary) 14%, transparent);
  --topo-card-stroke-hi: color-mix(in srgb, var(--el-text-color-primary) 26%, transparent);
  --topo-line: color-mix(in srgb, var(--el-text-color-primary) 20%, var(--el-bg-color));
  --topo-grid: color-mix(in srgb, var(--el-text-color-primary) 12%, transparent);
  --topo-glow: color-mix(in srgb, var(--el-color-primary) 8%, transparent);
  --topo-text: var(--el-text-color-primary);
  --topo-sub: var(--el-text-color-secondary);
  --topo-ok: #10b981;
  --topo-down: #ef4444;
  --topo-off: #94a3b8;
  --topo-expected: #a3acb9;
  --topo-hub-icon: var(--el-color-primary);
  --topo-core-a: 0.32;
  --topo-core-b: 0.06;
  --topo-shadow: 0 1px 2px color-mix(in srgb, var(--el-text-color-primary) 8%, transparent);
  display: flex;
  flex-direction: column;
  gap: 12px;
  /* 按自身宽度决定详情面板放在右侧还是画布下方 */
  container-type: inline-size;
}
:global(html.dark) .topo {
  /* 暗色皮肤：在皮肤底色上叠加一点深蓝，保留科技感，同时与各暗色皮肤协调 */
  --topo-bg: color-mix(in srgb, var(--el-bg-color-page) 70%, #0b1220);
  --topo-bg2: color-mix(in srgb, var(--el-bg-color) 70%, #111b2e);
  --topo-card: color-mix(in srgb, var(--el-bg-color) 72%, #131d31);
  --topo-card-hi: color-mix(in srgb, var(--el-bg-color) 60%, #1c2a46);
  --topo-card-stroke: rgba(148, 163, 184, 0.18);
  --topo-card-stroke-hi: rgba(148, 163, 184, 0.35);
  --topo-line: color-mix(in srgb, var(--el-text-color-primary) 18%, var(--topo-bg));
  --topo-grid: rgba(148, 163, 184, 0.12);
  --topo-glow: rgba(56, 189, 248, 0.08);
  --topo-ok: #34d399;
  --topo-down: #f87171;
  --topo-off: #64748b;
  --topo-expected: #94a3b8;
  --topo-hub-icon: #fff;
  --topo-core-a: 0.55;
  --topo-core-b: 0.08;
  --topo-shadow: none;
}
.topo-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.ip-filter {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 6px 10px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  background: var(--el-bg-color);
  transition: border-color 0.2s, box-shadow 0.2s;
}
.ip-filter.on {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 3px var(--el-color-primary-light-9);
}
.bar-icon {
  color: var(--el-text-color-secondary);
}
.ip-select {
  width: 230px;
}
.filter-state {
  font-size: 13px;
  color: var(--el-color-primary);
}
.opt-meta {
  float: right;
  margin-left: 16px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.kpis {
  display: flex;
  gap: 6px;
  margin-right: 4px;
}
.kpi {
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.kpi b {
  color: var(--el-text-color-primary);
  font-variant-numeric: tabular-nums;
}
.topo-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 12px;
  align-items: start;
}
@container (max-width: 1100px) {
  .topo-main {
    grid-template-columns: minmax(0, 1fr);
  }
  .panel {
    position: static;
    max-height: none;
  }
}
.canvas {
  position: relative;
  overflow: hidden;
  min-height: 320px;
  border-radius: calc(var(--g-radius) + 2px);
  background:
    radial-gradient(circle at 1px 1px, var(--topo-grid) 1px, transparent 0) 0 0 / 22px 22px,
    radial-gradient(1200px 500px at 10% 0%, var(--topo-glow), transparent 60%),
    linear-gradient(160deg, var(--topo-bg2), var(--topo-bg));
  box-shadow: inset 0 0 0 1px var(--topo-card-stroke);
  /* 拖动与捏合由组件处理，不让浏览器滚动页面或选中文字 */
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;
  cursor: grab;
}
.canvas.grabbing {
  cursor: grabbing;
}
.stage {
  position: absolute;
  top: 0;
  left: 0;
  transform-origin: 0 0;
}
.stage.animate {
  transition: transform 0.25s ease;
}
.canvas.moving .stage {
  will-change: transform;
}
.zoom {
  position: absolute;
  right: 10px;
  bottom: 10px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--topo-card) 88%, transparent);
  box-shadow: inset 0 0 0 1px var(--topo-card-stroke), 0 4px 14px rgba(0, 0, 0, 0.12);
  backdrop-filter: blur(6px);
  cursor: default;
}
.zbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: none;
  color: var(--topo-sub);
  font: inherit;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}
.zbtn:hover {
  background: color-mix(in srgb, var(--topo-accent) 14%, transparent);
  color: var(--topo-text);
}
.zbtn.pct {
  min-width: 46px;
}
.zbtn.help {
  cursor: help;
}
.btn-ic {
  margin-right: 5px;
}
.btn-caret {
  margin-left: 4px;
  opacity: 0.6;
}
.zsep {
  width: 1px;
  height: 18px;
  margin: 0 2px;
  background: var(--topo-card-stroke);
}
.zbtn.on {
  color: var(--topo-accent);
}

/* 画布与调整高度的拖动条 */
.canvas-wrap {
  min-width: 0;
}
.grip {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 18px;
  margin-top: 2px;
  cursor: ns-resize;
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;
  color: var(--el-text-color-placeholder);
  font-size: 11px;
}
.grip-bar {
  width: 44px;
  height: 4px;
  border-radius: 2px;
  background: var(--el-border-color);
  transition: background 0.2s, width 0.2s;
}
.grip:hover .grip-bar,
.grip.active .grip-bar {
  width: 64px;
  background: var(--el-color-primary);
}
.grip-text {
  position: absolute;
  left: calc(50% + 44px);
  white-space: nowrap;
  opacity: 0;
  transition: opacity 0.2s;
}
.grip:hover .grip-text,
.grip.active .grip-text,
.grip.custom .grip-text {
  opacity: 1;
}
.grip-reset {
  position: absolute;
  right: 0;
  padding: 0 6px;
  border: 0;
  background: none;
  color: var(--el-color-primary);
  font: inherit;
  cursor: pointer;
}
@media (hover: none) {
  /* 触屏没有悬停，拖动条加高便于按住 */
  .grip {
    height: 26px;
  }
  .grip-bar {
    width: 56px;
    height: 5px;
  }
}

/* 窗口内全屏：铺满浏览器窗口，Esc 或按钮退出 */
.topo.full {
  position: fixed;
  inset: 0;
  z-index: 2000;
  padding: 14px 16px 10px;
  overflow: auto;
  background: var(--el-bg-color-page);
}
.topo.full .panel {
  top: 0;
  max-height: calc(100vh - 120px);
}

/* 切换布局时淡入淡出 */
.topo-fade-enter-active,
.topo-fade-leave-active {
  transition: opacity 0.18s ease;
}
.topo-fade-enter-from,
.topo-fade-leave-to {
  opacity: 0;
}

/* 桑基图 */
.hit {
  fill: transparent;
}
.sbar {
  fill: var(--topo-off);
  transition: fill 0.2s;
}
.st-ok .sbar {
  fill: var(--topo-accent);
}
.st-down .sbar {
  fill: var(--topo-down);
}
.st-expected .sbar {
  fill: var(--topo-expected);
}
.k-more .sbar {
  fill: none;
  stroke: var(--topo-accent);
  stroke-dasharray: 3 2;
}
.node.sel .sbar {
  filter: drop-shadow(0 0 6px color-mix(in srgb, var(--topo-accent) 70%, transparent));
}
.halo {
  paint-order: stroke;
  stroke: var(--topo-bg);
  stroke-width: 3px;
  stroke-linejoin: round;
}
.edge-base.band {
  fill: var(--topo-accent);
  fill-opacity: 0.16;
  stroke: none;
}
.edge.hot .edge-base.band {
  stroke: none;
  fill-opacity: 0.38;
}

/* 矩形树图与气泡图 */
.tm,
.pk {
  fill: var(--topo-card);
  stroke: var(--topo-card-stroke);
  stroke-width: 1;
  transition: fill 0.2s, stroke 0.2s;
}
.tm.root,
.pk.root {
  fill: color-mix(in srgb, var(--topo-accent) 4%, transparent);
  stroke-dasharray: 4 4;
}
.k-listener .tm,
.k-listener .pk {
  fill: color-mix(in srgb, var(--topo-accent) 6%, var(--topo-card));
}
.tm.leafbox,
.pk.leafbox {
  fill: var(--topo-off);
  stroke: var(--topo-bg);
}
.st-ok .tm.leafbox,
.st-ok .pk.leafbox {
  fill: var(--topo-accent);
}
.st-down .tm.leafbox,
.st-down .pk.leafbox {
  fill: var(--topo-down);
}
.st-expected .tm.leafbox,
.st-expected .pk.leafbox {
  fill: var(--topo-expected);
}
.k-more .tm,
.k-more .pk {
  stroke: var(--topo-accent);
  stroke-dasharray: 3 2;
}
.node:hover .tm,
.node.hot .tm,
.node:hover .pk,
.node.hot .pk {
  stroke: var(--topo-card-stroke-hi);
}
.node.sel .tm,
.node.sel .pk {
  stroke: var(--topo-accent);
  stroke-width: 2;
}
.tm-head,
.pk-head {
  font-size: 11.5px;
  font-weight: 600;
  fill: var(--topo-text);
  pointer-events: none;
}
.tm-title,
.pk-title {
  font-size: 11px;
  font-weight: 600;
  fill: var(--topo-text);
  pointer-events: none;
  paint-order: stroke;
  stroke: var(--topo-card);
  stroke-width: 2.5px;
  stroke-linejoin: round;
}
.tm-sub {
  font-size: 10.5px;
  fill: var(--topo-text);
  opacity: 0.75;
  pointer-events: none;
}

/* 旭日图 */
.sarc {
  fill: var(--topo-off);
  stroke: var(--topo-bg);
  stroke-width: 1;
  transition: fill 0.2s;
}
.st-ok .sarc {
  fill: var(--topo-accent);
}
.st-down .sarc {
  fill: var(--topo-down);
}
.st-expected .sarc {
  fill: var(--topo-expected);
}
.k-more .sarc {
  fill: var(--topo-accent);
  stroke: var(--topo-accent);
  stroke-dasharray: 3 2;
}
.node:hover .sarc,
.node.hot .sarc {
  stroke: var(--topo-text);
  stroke-width: 1.2;
}
.node.sel .sarc {
  stroke: var(--topo-accent);
  stroke-width: 2.5;
}
.slabel {
  font-size: 11px;
  fill: var(--topo-text);
  pointer-events: none;
  paint-order: stroke;
  stroke: var(--topo-bg);
  stroke-width: 2.5px;
  stroke-linejoin: round;
}
.k-listener .slabel {
  font-weight: 600;
}
.canvas-empty {
  height: 300px;
}
.canvas-empty.msg,
.canvas-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--topo-sub);
  font-size: 13px;
}
.canvas-hint {
  position: absolute;
  inset: 40% 0 auto;
  /* 聚焦活动没有结果时文字下方还有“显示全部”按钮，上下排列 */
  flex-direction: column;
  padding: 0 16px;
  text-align: center;
  pointer-events: none;
}
.hint-btn {
  margin-top: 10px;
  pointer-events: auto;
}
/* 聚焦活动按钮右侧的活动站点数：有活动时用主色，没有时灰色 */
.active-count {
  min-width: 18px;
  height: 18px;
  margin-left: 6px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--el-color-primary-light-8);
  color: var(--el-color-primary);
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  font-variant-numeric: tabular-nums;
}
.active-count.zero {
  background: var(--el-fill-color);
  color: var(--el-text-color-secondary);
}
.active-btn.on .active-count {
  background: rgba(255, 255, 255, 0.25);
  color: #fff;
}
.cols text {
  font-size: 10.5px;
  letter-spacing: 0.08em;
  fill: var(--topo-sub);
  opacity: 0.8;
}
.graph {
  display: block;
  overflow: visible;
  font-family: var(--g-font);
}
.rings circle {
  fill: none;
  stroke: var(--topo-grid);
  stroke-dasharray: 2 6;
}

/* 连线 */
.edge-base {
  fill: none;
  stroke: var(--topo-line);
  stroke-linecap: round;
  transition: stroke 0.25s, opacity 0.25s;
}
.edge-flow {
  fill: none;
  stroke: var(--topo-accent);
  stroke-linecap: round;
  stroke-dasharray: 2 14;
  opacity: 0.85;
  animation: topo-flow 1.6s linear infinite;
}
.edge-flow.lv1 {
  animation-duration: 2.6s;
  opacity: 0.55;
}
.edge-flow.lv3 {
  animation-duration: 0.8s;
}
.no-motion .edge-flow {
  animation: none;
}
.edge.hot .edge-base {
  stroke: url(#topo-edge-hot);
}
.edge.dim {
  opacity: 0.18;
}
@keyframes topo-flow {
  to {
    stroke-dashoffset: -32;
  }
}
.edge-label {
  font-size: 10.5px;
  fill: var(--topo-sub);
  paint-order: stroke;
  stroke: var(--topo-bg);
  stroke-width: 3px;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
}
.edge-label.hot {
  fill: var(--topo-text);
}

/* 节点 */
.node {
  cursor: pointer;
  transition: opacity 0.25s;
}
.node.dim {
  opacity: 0.28;
}
.card {
  fill: var(--topo-card);
  stroke: var(--topo-card-stroke);
  stroke-width: 1;
  filter: drop-shadow(var(--topo-shadow));
  transition: fill 0.2s, stroke 0.2s;
}
.node:hover .card,
.node.hot .card {
  fill: var(--topo-card-hi);
  stroke: var(--topo-card-stroke-hi);
}
.k-more .card {
  stroke-dasharray: 4 3;
}
.k-more .title {
  fill: var(--topo-accent);
}
.node.sel .card {
  stroke: var(--topo-accent);
  stroke-width: 1.5;
  filter: drop-shadow(0 0 8px color-mix(in srgb, var(--topo-accent) 55%, transparent));
}
.card-accent {
  fill: var(--topo-off);
}
.st-ok .card-accent {
  fill: var(--topo-ok);
}
.st-down .card-accent {
  fill: var(--topo-down);
}
.st-expected .card-accent {
  fill: var(--topo-expected);
}
.icon-bg {
  fill: rgba(148, 163, 184, 0.1);
}
.st-ok .icon-bg {
  fill: color-mix(in srgb, var(--topo-accent) 18%, transparent);
}
.st-down .icon-bg {
  fill: rgba(248, 113, 113, 0.16);
}
.node-icon {
  color: var(--topo-sub);
}
.st-ok .node-icon {
  color: var(--topo-accent);
}
.st-down .node-icon {
  color: var(--topo-down);
}
.title {
  font-size: 12.5px;
  font-weight: 600;
  fill: var(--topo-text);
}
.sub {
  font-size: 11px;
  fill: var(--topo-sub);
  font-variant-numeric: tabular-nums;
}
.st-off .title {
  fill: var(--topo-sub);
}
.dot {
  fill: var(--topo-off);
}
.st-ok .dot {
  fill: var(--topo-ok);
}
.st-down .dot {
  fill: var(--topo-down);
  animation: topo-blink 1.4s ease-in-out infinite;
}
.st-expected .dot {
  fill: var(--topo-expected);
}
.no-motion .st-down .dot {
  animation: none;
}
@keyframes topo-blink {
  50% {
    opacity: 0.35;
  }
}
.fold rect,
.fold circle {
  fill: color-mix(in srgb, var(--topo-sub) 16%, var(--topo-card));
}
.fold:hover rect,
.fold:hover circle {
  fill: color-mix(in srgb, var(--topo-sub) 30%, var(--topo-card));
}
.fold {
  color: var(--topo-sub);
}
.ipbadge rect {
  fill: var(--topo-accent);
}
.ipbadge text {
  font-size: 9.5px;
  font-weight: 700;
  fill: #fff;
}

/* 互联网与 Gately */
.hub {
  fill: url(#topo-core);
  stroke: color-mix(in srgb, var(--topo-accent) 70%, transparent);
  stroke-width: 1.5;
}
.hub-halo {
  fill: none;
  stroke: color-mix(in srgb, var(--topo-accent) 30%, transparent);
  stroke-width: 1;
  stroke-dasharray: 3 5;
  transform-box: fill-box;
  transform-origin: center;
  animation: topo-spin 24s linear infinite;
}
.no-motion .hub-halo {
  animation: none;
}
@keyframes topo-spin {
  to {
    transform: rotate(360deg);
  }
}
.k-gately .hub {
  stroke-width: 2;
}
.node.sel .hub {
  filter: drop-shadow(0 0 10px color-mix(in srgb, var(--topo-accent) 70%, transparent));
}
.hub-icon {
  color: var(--topo-hub-icon);
}

/* 径向布局的节点 */
.rdot {
  fill: var(--topo-card);
  stroke: var(--topo-off);
  stroke-width: 1.5;
  transition: fill 0.2s;
}
.st-ok .rdot {
  stroke: var(--topo-ok);
}
.st-down .rdot {
  stroke: var(--topo-down);
}
.st-expected .rdot {
  stroke: var(--topo-expected);
}
.k-more .rdot {
  stroke: var(--topo-accent);
  stroke-dasharray: 3 2;
}
.node:hover .rdot,
.node.hot .rdot {
  fill: var(--topo-card-hi);
}
.node.sel .rdot {
  stroke: var(--topo-accent);
  stroke-width: 2.5;
  filter: drop-shadow(0 0 6px color-mix(in srgb, var(--topo-accent) 60%, transparent));
}
.rmore {
  font-size: 9px;
  font-weight: 700;
  fill: var(--topo-accent);
}
.rlabel {
  font-size: 11.5px;
  fill: var(--topo-text);
  paint-order: stroke;
  stroke: var(--topo-bg);
  stroke-width: 3px;
  stroke-linejoin: round;
}
.k-listener .rlabel {
  font-weight: 600;
}
.k-more .rlabel {
  fill: var(--topo-accent);
}
.st-off .rlabel {
  fill: var(--topo-sub);
}
.m-radial .edge.hot .edge-base {
  stroke: var(--topo-accent);
  stroke-opacity: 0.75;
}
.st-off .hub-icon {
  color: var(--topo-sub);
}
.hub-title {
  font-size: 12.5px;
  font-weight: 600;
  fill: var(--topo-text);
}
.hub-sub {
  font-size: 10.5px;
  fill: var(--topo-sub);
}

/* 详情面板 */
.panel {
  /* 吸在顶栏与页面头部下方 */
  position: sticky;
  top: calc(var(--g-sticky-top) + 16px);
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: calc(100vh - var(--g-sticky-top) - 32px);
  overflow: auto;
  padding: 14px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  background: var(--el-bg-color);
}
.panel-title {
  font-size: 15px;
  font-weight: 600;
  word-break: break-all;
}
.panel-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 8px;
}
.panel-error {
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}
.metric {
  padding: 8px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.m-label {
  display: flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.m-help {
  color: var(--el-text-color-placeholder);
  cursor: help;
}
.m-value {
  margin-top: 2px;
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  word-break: break-all;
}
.bad {
  color: var(--el-color-danger);
}
.warn {
  color: var(--el-color-warning);
}
.chart-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-regular);
}
.chart-meta {
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.rows {
  display: flex;
  flex-direction: column;
}
.row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 5px 0;
  border-bottom: 1px dashed var(--g-border);
  font-size: 12px;
}
.r-label {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
  white-space: pre;
}
.r-value {
  text-align: right;
  word-break: break-all;
  font-variant-numeric: tabular-nums;
}
.panel-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.panel-actions .el-button + .el-button {
  margin-left: 0;
}
.ips-title {
  margin-top: 4px;
}
.ip-warn {
  font-size: 12px;
  color: var(--el-color-warning);
}
.ip-list {
  display: flex;
  flex-direction: column;
  max-height: 280px;
  overflow: auto;
  border: 1px solid var(--g-border);
  border-radius: 8px;
}
.ip-empty {
  padding: 14px;
  text-align: center;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.ip-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 10px;
  border: 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background: none;
  color: inherit;
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}
.ip-row:last-child {
  border-bottom: 0;
}
.ip-row:hover {
  background: var(--el-fill-color-light);
}
.ip-row.on {
  background: var(--el-color-primary-light-9);
}
.ip {
  word-break: break-all;
}
.ip-meta {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--el-text-color-secondary);
}
.ip-log {
  display: inline-flex;
  padding: 2px;
  border: 0;
  border-radius: 4px;
  background: none;
  color: var(--el-text-color-secondary);
  cursor: pointer;
}
.ip-log:hover {
  color: var(--el-color-primary);
  background: var(--el-fill-color);
}
.live {
  color: var(--el-color-success);
}
.mono {
  font-family: var(--g-mono);
}
.topo-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.foot-note {
  flex-basis: 100%;
  color: var(--el-text-color-placeholder);
}
.pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--el-color-success);
  box-shadow: 0 0 0 0 color-mix(in srgb, var(--el-color-success) 60%, transparent);
  animation: topo-pulse 2s ease-out infinite;
}
.pulse.off {
  background: var(--el-text-color-placeholder);
  animation: none;
}
@keyframes topo-pulse {
  70% {
    box-shadow: 0 0 0 6px transparent;
  }
}
@media (prefers-reduced-motion: reduce) {
  .pulse {
    animation: none;
  }
}
</style>

<style>
/* 下拉菜单渲染在 body 下，不能用 scoped 样式 */
.topo-layout-menu .el-dropdown-menu__item,
.topo-mask-menu .el-dropdown-menu__item {
  gap: 10px;
  min-width: 250px;
  padding: 8px 14px;
  line-height: 1.35;
}
.topo-layout-menu .el-dropdown-menu__item.on {
  color: var(--el-color-primary);
}
.topo-layout-menu .lm-icon {
  flex-shrink: 0;
}
.topo-layout-menu .lm-text,
.topo-mask-menu .lm-text {
  display: flex;
  flex-direction: column;
  flex: 1;
}
.topo-layout-menu .lm-name,
.topo-mask-menu .lm-name {
  font-weight: 600;
}
.topo-layout-menu .lm-desc,
.topo-mask-menu .lm-desc {
  max-width: 260px;
  font-size: 12px;
  white-space: normal;
  color: var(--el-text-color-secondary);
}
.topo-layout-menu .lm-check {
  flex-shrink: 0;
}
.topo-mask-menu .mm-head {
  max-width: 290px;
  padding: 6px 14px 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  border-bottom: 1px solid var(--el-border-color-lighter);
  margin-bottom: 4px;
}
.topo-mask-menu .mm-box {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  color: #fff;
}
.topo-mask-menu .mm-sec {
  padding: 8px 14px 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  border-top: 1px solid var(--el-border-color-lighter);
  margin-top: 4px;
}
.topo-mask-menu .mm-radio {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  border: 1px solid var(--el-border-color);
  border-radius: 50%;
}
.topo-mask-menu .mm-radio.on {
  border: 5px solid var(--el-color-primary);
}
.topo-mask-menu .lm-icon {
  flex-shrink: 0;
}
.topo-mask-menu .mono {
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
}
.topo-mask-menu .mm-box.on {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary);
}
</style>
