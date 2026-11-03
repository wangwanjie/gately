<template>
  <!-- 错误页预览：可切换状态码与桌面 / 手机视口，页面放在空 sandbox 的 iframe 中显示 -->
  <el-dialog :model-value="modelValue" :title="title" width="900px" top="6vh" append-to-body :fullscreen="isMobile" @update:model-value="emit('update:modelValue', $event)">
    <div class="bar">
      <el-radio-group v-model="code" size="small" :aria-label="t('errorPages.previewCode')">
        <el-radio-button v-for="c in codes" :key="c" :value="c">{{ c }}</el-radio-button>
      </el-radio-group>
      <el-radio-group v-model="device" size="small" :aria-label="t('errorPages.previewDevice')">
        <el-radio-button value="desktop">{{ t('errorPages.desktop') }}</el-radio-button>
        <el-radio-button value="mobile">{{ t('errorPages.mobile') }}</el-radio-button>
      </el-radio-group>
    </div>
    <div class="stage" :class="device" data-test="error-preview">
      <ErrorPageThumb :html="html" :error="error" :width="device === 'mobile' ? 390 : 1280" :height="device === 'mobile' ? 780 : 800" :title="title" interactive />
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ErrorPageThumb from './ErrorPageThumb.vue'
import { useFormLayout } from '../composables/useIsMobile'

const props = defineProps<{
  modelValue: boolean
  title: string
  // load 按状态码返回要预览的页面
  load: ((code: number) => Promise<string>) | null
  initialCode?: number
}>()
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()
const { t } = useI18n()
const { isMobile } = useFormLayout()

const codes = [404, 500, 502, 503, 504]
const code = ref(502)
const device = ref<'desktop' | 'mobile'>('desktop')
const html = ref('')
const error = ref('')
// seq 丢弃过期的请求结果：快速切换状态码时只显示最后一次
let seq = 0

async function refresh() {
  if (!props.modelValue || !props.load) {
    return
  }
  const my = ++seq
  html.value = ''
  error.value = ''
  try {
    const v = await props.load(code.value)
    if (my === seq) {
      html.value = v
    }
  } catch (e) {
    if (my === seq) {
      error.value = (e as Error).message
    }
  }
}

watch(
  () => props.modelValue,
  (v) => {
    if (v) {
      code.value = props.initialCode ?? 502
      device.value = isMobile.value ? 'mobile' : 'desktop'
      refresh()
    }
  },
)
watch(code, refresh)
</script>

<style scoped>
.bar {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.stage {
  margin: 0 auto;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  overflow: hidden;
}
.stage.mobile {
  max-width: 320px;
}
</style>
