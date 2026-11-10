<template>
  <!-- 导航面板图标输入：文字（emoji）、图片地址，或上传图片（转为 data:image 内联保存） -->
  <div class="icon-input">
    <PanelIcon :icon="model" :name="name" :color="color" :size="36" />
    <el-input v-model="model" clearable class="icon-text" :placeholder="t('panels.iconPlaceholder')" :maxlength="inline ? undefined : 2048">
      <template v-if="inline" #prefix><ImageIcon :size="14" /></template>
    </el-input>
    <el-button :icon="Upload" @click="pick">{{ t('panels.iconUpload') }}</el-button>
    <input ref="file" type="file" accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon,.ico" hidden @change="onFile" />
  </div>
  <div v-if="inline" class="form-help">{{ t('panels.iconInline', { size: formatBytes(model.length) }) }}</div>
  <div v-else-if="kind === 'text' && !textIconOk(model)" class="form-help warn">{{ t('panels.iconTextTooLong') }}</div>
  <div v-else class="form-help">{{ t('panels.iconHelp') }}</div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Image as ImageIcon, Upload } from 'lucide-vue-next'
import PanelIcon from './PanelIcon.vue'
import { formatBytes } from '../api'
import { MAX_ICON_BYTES, iconKind, textIconOk } from '../panels'

// name 与 color 用于预览默认的首字图标
defineProps<{ name?: string; color?: string }>()
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
const file = ref<HTMLInputElement>()

const kind = computed(() => iconKind(model.value))
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

// shrink 把位图缩放到最长边 128 像素以内并转成 PNG，图标显示尺寸很小，原图只会让配置变大
async function shrink(f: File): Promise<string> {
  const url = URL.createObjectURL(f)
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => reject(new Error('decode'))
      el.src = url
    })
    const scale = Math.min(1, 128 / Math.max(img.naturalWidth, img.naturalHeight, 1))
    const w = Math.max(1, Math.round(img.naturalWidth * scale))
    const h = Math.max(1, Math.round(img.naturalHeight * scale))
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    canvas.getContext('2d')?.drawImage(img, 0, 0, w, h)
    return canvas.toDataURL('image/png')
  } finally {
    URL.revokeObjectURL(url)
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
</style>
