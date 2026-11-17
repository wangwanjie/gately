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
import { Globe, Image as ImageIcon, Shapes, Upload } from 'lucide-vue-next'
import PanelIcon from './PanelIcon.vue'
import PanelIconPicker from './PanelIconPicker.vue'
import { api, errorText, formatBytes } from '../api'
import { MAX_ICON_BYTES, builtinId, iconKind, textIconOk } from '../panels'
import { usePanelIcons } from '../composables/usePanelIcons'

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
@media (max-width: 768px) {
  .icon-input {
    flex-wrap: wrap;
  }
  .icon-text {
    flex-basis: calc(100% - 44px);
  }
}
</style>
