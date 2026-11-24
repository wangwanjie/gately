<template>
  <!-- 导航面板图标输入：文字（emoji）、图片地址，或上传图片（转为 data:image 内联保存） -->
  <div class="icon-input">
    <PanelIcon :icon="model" :name="name" :color="color" :size="36" />
    <el-input v-model="model" clearable class="icon-text" :placeholder="t('panels.iconPlaceholder')" :maxlength="inline ? undefined : 2048">
      <template v-if="inline" #prefix><ImageIcon :size="14" /></template>
    </el-input>
    <el-button :icon="Shapes" data-test="icon-pick" @click="pickerVisible = true">{{ t('panels.iconPick') }}</el-button>
    <el-button :icon="Upload" @click="pick">{{ t('panels.iconUpload') }}</el-button>
    <el-button v-if="urls" :icon="Globe" :loading="fetching" :disabled="!urls.length" :title="urls.length ? t('panels.iconFetchHelp') : t('panels.iconFetchNoUrl')" data-test="icon-fetch" @click="fetchIcon">{{ t('panels.iconFetch') }}</el-button>
    <input ref="file" type="file" accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon,.ico" hidden @change="onFile" />
  </div>
  <!-- 第二行：常用图标网站（新标签页打开，复制图片地址或下载后上传）与去除图片留白 -->
  <div class="icon-tools">
    <el-dropdown trigger="click" data-test="icon-sites">
      <el-button text size="small" :icon="ExternalLink">{{ t('panels.iconSites') }}<ChevronDown :size="12" class="caret" /></el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item v-for="s in ICON_SITES" :key="s.url" @click="openSite(s.url)">
            <span class="site-item"><span>{{ s.name }}</span><span class="site-host">{{ hostOf(s.url) }}</span></span>
          </el-dropdown-item>
          <div class="site-help">{{ t('panels.iconSitesHelp') }}</div>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
    <el-button v-if="kind === 'image'" text size="small" :icon="Crop" :loading="trimming" :title="t('panels.iconTrimHelp')" data-test="icon-trim" @click="trim">{{ t('panels.iconTrim') }}</el-button>
  </div>
  <PanelIconPicker v-model:visible="pickerVisible" :model-value="model" @select="(v: string) => (model = v)" />
  <div v-if="builtin" class="form-help">{{ t('panels.iconBuiltin', { name: builtin.name }) }}</div>
  <div v-else-if="kind === 'builtin'" class="form-help warn">{{ t('panels.iconBuiltinMissing') }}</div>
  <div v-else-if="inline" class="form-help">{{ t('panels.iconInline', { size: formatBytes(model.length) }) }}</div>
  <div v-else-if="kind === 'text' && !textIconOk(model)" class="form-help warn">{{ t('panels.iconTextTooLong') }}</div>
  <div v-else class="form-help">{{ t('panels.iconHelp') }}</div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ChevronDown, Crop, ExternalLink, Globe, Image as ImageIcon, Shapes, Upload } from 'lucide-vue-next'
import PanelIcon from './PanelIcon.vue'
import PanelIconPicker from './PanelIconPicker.vue'
import { api, errorText, formatBytes } from '../api'
import { MAX_ICON_BYTES, builtinId, iconKind, textIconOk } from '../panels'
import { usePanelIcons } from '../composables/usePanelIcons'
import { ICON_SITES, contentBox, squareAround, svgForRender, trimSvg } from '../iconTrim'

// name 与 color 用于预览默认的首字图标；urls 为“获取图标”要尝试的链接地址，不传时不显示该按钮
const props = defineProps<{ name?: string; color?: string; urls?: string[] }>()
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
const file = ref<HTMLInputElement>()

const kind = computed(() => iconKind(model.value))
const pickerVisible = ref(false)
const { byId } = usePanelIcons()
const builtin = computed(() => byId.value.get(builtinId(model.value)) ?? null)
// inline 为上传的内联图片：输入框里是很长的 data URL，提示占用大小
const inline = computed(() => model.value.startsWith('data:'))

function pick() {
  file.value?.click()
}

function readAsDataURL(f: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result))
    r.onerror = () => reject(r.error)
    r.readAsDataURL(f)
  })
}

// shrinkSrc 把位图缩放到最长边 256 像素以内并转成 PNG（2×2 大块与大图标放大显示时仍清晰）；
// 超过内联上限时改为 128 像素。原图只会让配置变大
async function shrinkSrc(src: string): Promise<string> {
  const img = await new Promise<HTMLImageElement>((resolve, reject) => {
    const el = new Image()
    el.onload = () => resolve(el)
    el.onerror = () => reject(new Error('decode'))
    el.src = src
  })
  let out = ''
  for (const edge of [256, 128]) {
    const scale = Math.min(1, edge / Math.max(img.naturalWidth, img.naturalHeight, 1))
    const w = Math.max(1, Math.round(img.naturalWidth * scale))
    const h = Math.max(1, Math.round(img.naturalHeight * scale))
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    canvas.getContext('2d')?.drawImage(img, 0, 0, w, h)
    out = canvas.toDataURL('image/png')
    if (out.length <= MAX_ICON_BYTES) {
      break
    }
  }
  return out
}

async function shrink(f: File): Promise<string> {
  const url = URL.createObjectURL(f)
  try {
    return await shrinkSrc(url)
  } finally {
    URL.revokeObjectURL(url)
  }
}

// fetchIcon 由 Gately 访问链接地址，取网页声明的图标（找不到时用 /favicon.ico）；SVG 原样保存，
// 其余格式缩小成 PNG；ICO 能在浏览器中解码时同样缩小，否则在不超过上限时原样保存
const fetching = ref(false)
async function fetchIcon() {
  if (!props.urls?.length || fetching.value) {
    return
  }
  fetching.value = true
  try {
    const r = await api.post<{ icon: string; source: string }>('/api/webservice/panels/favicon', { urls: props.urls })
    let data = r.icon
    if (!data.startsWith('data:image/svg+xml')) {
      try {
        data = await shrinkSrc(r.icon)
      } catch {
        data = r.icon
      }
    }
    if (data.length > MAX_ICON_BYTES) {
      ElMessage.error(t('panels.iconTooLarge', { size: formatBytes(MAX_ICON_BYTES) }))
      return
    }
    model.value = data
    ElMessage.success(t('panels.iconFetched'))
  } catch (e) {
    ElMessage.error(errorText(e))
  } finally {
    fetching.value = false
  }
}

function hostOf(u: string) {
  return new URL(u).host
}

function openSite(u: string) {
  window.open(u, '_blank', 'noopener,noreferrer')
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const el = new Image()
    el.onload = () => resolve(el)
    el.onerror = () => reject(new Error('decode'))
    el.src = src
  })
}

function pixels(img: HTMLImageElement, w: number, h: number) {
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) {
    throw new Error('canvas')
  }
  ctx.drawImage(img, 0, 0, w, h)
  return ctx.getImageData(0, 0, w, h).data
}

function b64ToText(b64: string) {
  return new TextDecoder().decode(Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)))
}

function textToB64(s: string) {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  for (let i = 0; i < bytes.length; i += 0x8000) {
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  }
  return btoa(bin)
}

// trimSvgData 改写 SVG 的 viewBox，矢量内容不变；trimBitmap 裁成正方形 PNG（最长边 256，超过上限改 128）
async function trimSvgData(data: string): Promise<string | 'blank' | 'mixed' | 'none'> {
  const text = b64ToText(data.slice(data.indexOf(',') + 1))
  const r = svgForRender(text)
  if (!r) {
    // 读不出坐标范围的 SVG 按位图处理
    return trimBitmap(data)
  }
  const img = await loadImage('data:image/svg+xml;base64,' + textToB64(r.svg))
  const found = contentBox(pixels(img, r.w, r.h), r.w, r.h)
  if (typeof found === 'string') {
    return found
  }
  const out = trimSvg(text, found.box, r.w, r.h)
  return out ? 'data:image/svg+xml;base64,' + textToB64(out) : trimBitmap(data)
}

async function trimBitmap(data: string): Promise<string | 'blank' | 'mixed' | 'none'> {
  const img = await loadImage(data)
  const k = Math.min(1, 512 / Math.max(img.naturalWidth, img.naturalHeight, 1))
  const w = Math.max(1, Math.round(img.naturalWidth * k))
  const h = Math.max(1, Math.round(img.naturalHeight * k))
  const found = contentBox(pixels(img, w, h), w, h)
  if (typeof found === 'string') {
    return found
  }
  const sq = squareAround(found.box)
  let out = ''
  for (const edge of [256, 128]) {
    const side = Math.max(1, Math.min(edge, Math.round(sq.w / k)))
    const canvas = document.createElement('canvas')
    canvas.width = side
    canvas.height = side
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      throw new Error('canvas')
    }
    if (found.bg !== 'transparent') {
      ctx.fillStyle = `rgb(${found.bg.join(',')})`
      ctx.fillRect(0, 0, side, side)
    }
    // 正方形范围按原图坐标换算后整体缩放到 side；超出原图的部分保持背景
    const s = side / (sq.w / k)
    ctx.drawImage(img, -sq.x / k * s, -sq.y / k * s, img.naturalWidth * s, img.naturalHeight * s)
    out = canvas.toDataURL('image/png')
    if (out.length <= MAX_ICON_BYTES) {
      break
    }
  }
  return out
}

// trim 去除图片四周的留白：远程图片地址先由 Gately 取回（与“获取图标”相同的接口），结果保存为内联图片
const trimming = ref(false)
async function trim() {
  if (trimming.value) {
    return
  }
  trimming.value = true
  try {
    let data = model.value.trim()
    if (!data.startsWith('data:')) {
      const r = await api.post<{ icon: string }>('/api/webservice/panels/favicon', { urls: [data] })
      data = r.icon
    }
    let out: string
    try {
      out = data.startsWith('data:image/svg+xml;base64,') ? await trimSvgData(data) : await trimBitmap(data)
    } catch {
      ElMessage.error(t('panels.iconDecodeFailed'))
      return
    }
    if (out === 'blank' || out === 'mixed' || out === 'none') {
      ElMessage.warning(t(`panels.iconTrim_${out}`))
      return
    }
    if (out.length > MAX_ICON_BYTES) {
      ElMessage.error(t('panels.iconTooLarge', { size: formatBytes(MAX_ICON_BYTES) }))
      return
    }
    model.value = out
    ElMessage.success(t('panels.iconTrimmed'))
  } catch (e) {
    ElMessage.error(errorText(e))
  } finally {
    trimming.value = false
  }
}

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) {
    return
  }
  try {
    // SVG 与 ICO 原样保存（矢量图缩放无损，ICO 浏览器画不到画布上），其余位图先缩小
    const keep = f.type === 'image/svg+xml' || f.type === 'image/x-icon' || f.type === 'image/vnd.microsoft.icon' || /\.ico$/i.test(f.name)
    let data = keep ? await readAsDataURL(f) : await shrink(f)
    if (/\.ico$/i.test(f.name) && !data.startsWith('data:image/')) {
      data = data.replace(/^data:[^;]*;/, 'data:image/x-icon;')
    }
    if (data.length > MAX_ICON_BYTES) {
      ElMessage.error(t('panels.iconTooLarge', { size: formatBytes(MAX_ICON_BYTES) }))
      return
    }
    model.value = data
  } catch {
    ElMessage.error(t('panels.iconDecodeFailed'))
  }
}
</script>

<style scoped>
.icon-input {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.icon-text {
  flex: 1;
  min-width: 0;
}
.icon-input .el-button + .el-button {
  margin-left: 0;
}
.icon-tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  width: 100%;
  margin-top: 2px;
}
.icon-tools .el-button + .el-button {
  margin-left: 0;
}
.caret {
  margin-left: 2px;
}
.site-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}
.site-host {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.site-help {
  max-width: 260px;
  padding: 6px 16px 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.5;
}
@media (max-width: 768px) {
  .icon-input {
    flex-wrap: wrap;
  }
  .icon-text {
    flex-basis: calc(100% - 44px);
  }
}
</style>
