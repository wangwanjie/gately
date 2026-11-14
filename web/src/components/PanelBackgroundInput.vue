<template>
  <!-- 导航面板背景图片：图片地址，或上传图片（压缩后转为 data:image 内联保存） -->
  <div class="bg-input">
    <div class="bg-thumb" :class="{ empty: !model }" data-test="bg-thumb">
      <img v-if="model" :src="model" alt="" referrerpolicy="no-referrer" />
      <ImageIcon v-else :size="18" />
    </div>
    <el-input v-model="text" clearable class="bg-text" :placeholder="t('panels.bgPlaceholder')" data-test="bg-url" />
    <el-button :icon="Upload" data-test="bg-upload" @click="file?.click()">{{ t('panels.iconUpload') }}</el-button>
    <input ref="file" type="file" accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml" hidden @change="onFile" />
  </div>
  <div v-if="inline" class="form-help">{{ t('panels.bgInline', { size: formatBytes(model.length) }) }}</div>
  <div v-else class="form-help">{{ t('panels.bgHelp') }}</div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Image as ImageIcon, Upload } from 'lucide-vue-next'
import { formatBytes } from '../api'
import { MAX_BG_BYTES } from '../panels'

const model = defineModel<string>({ default: '' })
const emit = defineEmits<{ uploaded: [] }>()
const { t } = useI18n()
const file = ref<HTMLInputElement>()

// inline 为上传的内联图片：输入框里不显示很长的 data URL
const inline = computed(() => model.value.startsWith('data:'))
const text = computed<string>({
  get: () => (inline.value ? '' : model.value),
  set: (v) => {
    model.value = (v ?? '').trim()
  },
})

function readAsDataURL(f: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result))
    r.onerror = () => reject(r.error)
    r.readAsDataURL(f)
  })
}

// compress 把位图缩放到最长边 1920 像素以内并转成 JPEG；仍超过上限时逐步降低质量与尺寸，最终放不下返回空串
async function compress(f: File): Promise<string> {
  const url = URL.createObjectURL(f)
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => reject(new Error('decode'))
      el.src = url
    })
    for (const [edge, q] of [[1920, 0.82], [1920, 0.65], [1280, 0.6], [960, 0.5]]) {
      const scale = Math.min(1, edge / Math.max(img.naturalWidth, img.naturalHeight, 1))
      const canvas = document.createElement('canvas')
      canvas.width = Math.max(1, Math.round(img.naturalWidth * scale))
      canvas.height = Math.max(1, Math.round(img.naturalHeight * scale))
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        break
      }
      // JPEG 没有透明通道，先铺白底
      ctx.fillStyle = '#fff'
      ctx.fillRect(0, 0, canvas.width, canvas.height)
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
      const data = canvas.toDataURL('image/jpeg', q)
      if (data.length <= MAX_BG_BYTES) {
        return data
      }
    }
    return ''
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
    // SVG 原样保存（矢量图无损）；已经够小的 GIF 保留动画；其余位图压缩
    let data = ''
    if (f.type === 'image/svg+xml' || (f.type === 'image/gif' && f.size < MAX_BG_BYTES * 0.7)) {
      data = await readAsDataURL(f)
    } else {
      data = await compress(f)
    }
    if (!data || data.length > MAX_BG_BYTES) {
      ElMessage.error(t('panels.bgTooLarge', { size: formatBytes(MAX_BG_BYTES) }))
      return
    }
    model.value = data
    emit('uploaded')
  } catch {
    ElMessage.error(t('panels.iconDecodeFailed'))
  }
}
</script>

<style scoped>
.bg-input {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.bg-thumb {
  width: 56px;
  height: 36px;
  flex: none;
  border-radius: 6px;
  border: 1px solid var(--el-border-color);
  overflow: hidden;
  display: grid;
  place-items: center;
  color: var(--el-text-color-placeholder);
}
.bg-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.bg-text {
  flex: 1;
  min-width: 0;
}
@media (max-width: 768px) {
  .bg-input {
    flex-wrap: wrap;
  }
  .bg-text {
    flex-basis: calc(100% - 64px);
  }
}
</style>
