<template>
  <!-- 网页认证页面外观：左侧选样式、填品牌项与自定义 CSS，右侧按尚未保存的设置实时预览 -->
  <el-dialog
    v-model="visible"
    :title="t('loginPage.title')"
    width="1180px"
    class="lp-dialog"
    :fullscreen="isMobile"
    :close-on-click-modal="false"
    destroy-on-close
    append-to-body
    @closed="emit('closed')"
  >
    <div v-if="loadError" class="lp-error"><el-alert type="error" :title="loadError" :closable="false" show-icon /></div>
    <div v-else-if="form" class="lp">
      <div class="lp-side">
        <div class="lp-section">{{ t('loginPage.styleTitle') }}</div>
        <div class="lp-styles" role="radiogroup" :aria-label="t('loginPage.styleTitle')">
          <button
            v-for="id in styleIds"
            :key="id"
            type="button"
            role="radio"
            class="lp-style"
            :class="{ active: currentStyle === id }"
            :aria-checked="currentStyle === id"
            :data-test="`lp-style-${id}`"
            @click="form.style = id === LOGIN_STYLE_DEFAULT ? '' : id"
          >
            <ErrorPageThumb :html="thumbs[id]" :width="1280" :height="820" :title="styleName(id)" :error="thumbError" />
            <span class="lp-style-name">
              {{ styleName(id) }}
              <span v-if="schemeTag(id)" class="lp-tag">{{ schemeTag(id) }}</span>
            </span>
          </button>
        </div>
        <div class="form-help">{{ t('loginPage.styleHelp') }}</div>

        <div class="lp-section">{{ t('loginPage.brandTitle') }}</div>
        <el-form :model="form" label-width="96px" :label-position="isMobile ? 'top' : 'right'" class="lp-form" @submit.prevent>
          <el-form-item :label="t('loginPage.color')">
            <el-color-picker v-model="colorModel" :predefine="palette" data-test="lp-color" />
            <el-button v-if="form.color" link type="primary" class="lp-clear" @click="form.color = ''">{{ t('loginPage.colorReset') }}</el-button>
            <div class="form-help">{{ t('loginPage.colorHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('loginPage.background')">
            <PanelBackgroundInput v-model="form.background" :help="t('loginPage.backgroundHelp')" @uploaded="onBgSet" @update:model-value="onBgSet" />
          </el-form-item>
          <el-form-item v-if="form.background" :label="t('loginPage.bgDim')">
            <el-slider v-model="form.bgDim" :min="0" :max="90" :step="5" :format-tooltip="(v: number) => `${v}%`" class="lp-slider" />
          </el-form-item>
          <el-form-item :label="t('loginPage.logo')">
            <div class="lp-logo">
              <div class="lp-logo-thumb" :class="{ empty: !form.logo }">
                <img v-if="form.logo" :src="form.logo" alt="" referrerpolicy="no-referrer" />
                <ImageIcon v-else :size="18" />
              </div>
              <el-input v-model="logoText" clearable :placeholder="t('loginPage.logoPlaceholder')" data-test="lp-logo-url" />
              <el-button :icon="Upload" @click="logoFile?.click()">{{ t('loginPage.upload') }}</el-button>
              <input ref="logoFile" type="file" accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml" hidden @change="onLogoFile" />
            </div>
            <div class="form-help">{{ t('loginPage.logoHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('loginPage.pageTitle')">
            <el-input v-model="form.title" :maxlength="MAX_TITLE" show-word-limit clearable :placeholder="t('loginPage.titlePlaceholder')" data-test="lp-title" />
          </el-form-item>
          <el-form-item :label="t('loginPage.subtitle')">
            <el-input v-model="form.subtitle" :maxlength="MAX_SUBTITLE" show-word-limit clearable :placeholder="t('loginPage.subtitlePlaceholder')" data-test="lp-subtitle" />
            <div class="form-help">{{ t('loginPage.textHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('loginPage.hideBrand')">
            <el-switch v-model="form.hideBrand" />
            <div class="form-help">{{ t('loginPage.hideBrandHelp') }}</div>
          </el-form-item>
        </el-form>

        <div class="lp-section">{{ t('loginPage.cssTitle') }}</div>
        <el-input
          v-model="form.css"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 14 }"
          class="mono lp-css"
          spellcheck="false"
          :placeholder="cssPlaceholder"
          data-test="lp-css"
        />
        <div class="form-help">
          {{ t('loginPage.cssHelp', { size: '32 KB' }) }}
          <span class="lp-classes mono">{{ cssClasses.join('  ') }}</span>
        </div>
      </div>

      <div class="lp-main">
        <div class="lp-bar">
          <el-segmented v-model="state" :options="stateOptions" size="small" />
          <el-segmented v-model="scheme" :options="schemeOptions" size="small" />
          <el-segmented v-model="device" :options="deviceOptions" size="small" />
        </div>
        <div class="lp-stage" :class="device">
          <ErrorPageThumb
            :html="bigHtml"
            :width="device === 'mobile' ? 390 : 1280"
            :height="device === 'mobile' ? 780 : 820"
            :title="t('loginPage.preview')"
            :error="bigError"
            :style="{ colorScheme: scheme }"
            interactive
            class="lp-big"
          />
        </div>
        <div class="form-help">{{ t('loginPage.previewHelp') }}</div>
        <div v-if="siteOverrides" class="form-help">{{ t('loginPage.siteOverrides', { count: siteOverrides }) }}</div>
      </div>
    </div>
    <div v-else class="lp-loading"><span class="spinner" /></div>

    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="saving" :disabled="!form || !dirty" data-test="lp-save" @click="save">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Image as ImageIcon, Upload } from 'lucide-vue-next'
import { errorText, formatBytes } from '../api'
import ErrorPageThumb from './ErrorPageThumb.vue'
import PanelBackgroundInput from './PanelBackgroundInput.vue'
import { useIsMobile } from '../composables/useIsMobile'
import {
  cssClasses,
  darkOnlyStyles,
  DEFAULT_BG_DIM,
  getLoginPage,
  LOGIN_STYLE_DEFAULT,
  lightOnlyStyles,
  type LoginPage,
  MAX_LOGO_BYTES,
  MAX_SUBTITLE,
  MAX_TITLE,
  normalizeLoginPage,
  previewLoginPage,
  type PreviewState,
  saveLoginPage,
} from '../loginPage'

const emit = defineEmits<{ saved: [style: string]; closed: [] }>()
const visible = defineModel<boolean>({ default: false })
const { t } = useI18n()
const isMobile = useIsMobile()

const form = ref<LoginPage | null>(null)
const original = ref('')
const styles = ref<string[]>([])
const siteStyles = ref<Record<string, number>>({})
const loadError = ref('')
const saving = ref(false)

// 缩略图（登录状态）与大预览各自请求，设置变化后合并刷新
const thumbs = ref<Record<string, string>>({})
const thumbError = ref('')
const bigHtml = ref('')
const bigError = ref('')
const state = ref<PreviewState>('login')
const scheme = ref<'light' | 'dark'>('light')
const device = ref<'desktop' | 'mobile'>('desktop')

const styleIds = computed(() => [LOGIN_STYLE_DEFAULT, ...styles.value])
const currentStyle = computed(() => form.value?.style || LOGIN_STYLE_DEFAULT)
const dirty = computed(() => !!form.value && JSON.stringify(form.value) !== original.value)
const siteOverrides = computed(() => Object.values(siteStyles.value).reduce((a, b) => a + b, 0))

const stateOptions = computed(() => [
  { label: t('loginPage.stateLogin'), value: 'login' },
  { label: t('loginPage.stateTotp'), value: 'totp' },
  { label: t('loginPage.stateError'), value: 'error' },
])
const schemeOptions = computed(() => [
  { label: t('loginPage.light'), value: 'light' },
  { label: t('loginPage.dark'), value: 'dark' },
])
const deviceOptions = computed(() => [
  { label: t('loginPage.desktop'), value: 'desktop' },
  { label: t('loginPage.mobile'), value: 'mobile' },
])

// 主色的常用备选
const palette = ['#0e7c66', '#2563eb', '#7c3aed', '#db2777', '#dc2626', '#ea580c', '#ca8a04', '#0891b2', '#111111']

// el-color-picker 清空时给 null，这里换成空字符串（使用样式自带的颜色）
const colorModel = computed<string | null>({
  get: () => form.value?.color || null,
  set: (v) => {
    if (form.value) {
      form.value.color = (v ?? '').toLowerCase()
    }
  },
})

// logoText 为输入框中的 Logo 地址；上传的内联图片不在输入框中显示
const logoText = computed<string>({
  get: () => (form.value?.logo.startsWith('data:') ? '' : (form.value?.logo ?? '')),
  set: (v) => {
    if (form.value) {
      form.value.logo = (v ?? '').trim()
    }
  },
})
const logoFile = ref<HTMLInputElement>()

const cssPlaceholder = '.card { border-radius: 4px; }\n.btn.primary { letter-spacing: .1em; }'

function styleName(id: string) {
  return t(`loginPage.styles.${id}`)
}

function schemeTag(id: string) {
  if (darkOnlyStyles.includes(id)) {
    return t('loginPage.darkOnly')
  }
  if (lightOnlyStyles.includes(id)) {
    return t('loginPage.lightOnly')
  }
  return ''
}

// onBgSet 首次设置背景图时给一个适中的遮罩浓度，清空时归零
function onBgSet() {
  if (!form.value) {
    return
  }
  if (!form.value.background) {
    form.value.bgDim = 0
  } else if (!form.value.bgDim) {
    form.value.bgDim = DEFAULT_BG_DIM
  }
}

function readAsDataURL(f: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result))
    r.onerror = () => reject(r.error)
    r.readAsDataURL(f)
  })
}

// shrinkLogo 把位图缩放到最长边 512 像素以内并转成 PNG（保留透明背景）
async function shrinkLogo(f: File): Promise<string> {
  const url = URL.createObjectURL(f)
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => reject(new Error('decode'))
      el.src = url
    })
    for (const edge of [512, 320, 200]) {
      const scale = Math.min(1, edge / Math.max(img.naturalWidth, img.naturalHeight, 1))
      const canvas = document.createElement('canvas')
      canvas.width = Math.max(1, Math.round(img.naturalWidth * scale))
      canvas.height = Math.max(1, Math.round(img.naturalHeight * scale))
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        break
      }
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
      const data = canvas.toDataURL('image/png')
      if (data.length <= MAX_LOGO_BYTES) {
        return data
      }
    }
    return ''
  } finally {
    URL.revokeObjectURL(url)
  }
}

async function onLogoFile(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f || !form.value) {
    return
  }
  try {
    // SVG 与已经够小的图片原样保存，其余位图缩小
    let data = ''
    if (f.type === 'image/svg+xml' || f.size < MAX_LOGO_BYTES * 0.7) {
      data = await readAsDataURL(f)
    } else {
      data = await shrinkLogo(f)
    }
    if (!data || data.length > MAX_LOGO_BYTES) {
      ElMessage.error(t('loginPage.logoTooLarge', { size: formatBytes(MAX_LOGO_BYTES) }))
      return
    }
    form.value.logo = data
  } catch {
    ElMessage.error(t('loginPage.logoDecodeFailed'))
  }
}

// 预览请求可能乱序返回，只采用最后一次请求的结果
let thumbSeq = 0
let bigSeq = 0
let timer: ReturnType<typeof setTimeout> | undefined

async function refreshThumbs() {
  if (!form.value) {
    return
  }
  const seq = ++thumbSeq
  try {
    const list = await previewLoginPage(form.value, [], 'login')
    if (seq === thumbSeq) {
      thumbs.value = Object.fromEntries(list.map((p) => [p.id, p.html]))
      thumbError.value = ''
    }
  } catch (e) {
    if (seq === thumbSeq) {
      thumbError.value = errorText(e)
    }
  }
}

async function refreshBig() {
  if (!form.value) {
    return
  }
  const seq = ++bigSeq
  try {
    const [p] = await previewLoginPage(form.value, [currentStyle.value], state.value)
    if (seq === bigSeq) {
      bigHtml.value = p?.html ?? ''
      bigError.value = ''
    }
  } catch (e) {
    if (seq === bigSeq) {
      bigHtml.value = ''
      bigError.value = errorText(e)
    }
  }
}

// 品牌项与 CSS 变化后稍等再刷新，避免每次按键都请求；只切换样式或预览状态时立即刷新大预览
watch(
  () => (form.value ? { ...form.value, style: '' } : null),
  () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      refreshThumbs()
      refreshBig()
    }, 400)
  },
  { deep: true },
)
watch([currentStyle, state], refreshBig)

async function load() {
  loadError.value = ''
  try {
    const v = await getLoginPage()
    styles.value = v.styles
    siteStyles.value = v.siteStyles ?? {}
    form.value = normalizeLoginPage(v.loginPage)
    original.value = JSON.stringify(form.value)
  } catch (e) {
    loadError.value = errorText(e)
  }
}

async function save() {
  if (!form.value) {
    return
  }
  saving.value = true
  try {
    const v = await saveLoginPage(form.value)
    form.value = normalizeLoginPage(v.loginPage)
    original.value = JSON.stringify(form.value)
    ElMessage.success(t('common.saved'))
    emit('saved', form.value.style)
    visible.value = false
  } catch (e) {
    ElMessage.error(errorText(e))
  } finally {
    saving.value = false
  }
}

onMounted(load)
onBeforeUnmount(() => clearTimeout(timer))
</script>

<style scoped>
.lp {
  display: grid;
  grid-template-columns: minmax(0, 440px) minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}
.lp-side {
  max-height: 70vh;
  overflow: auto;
  padding-right: 6px;
}
.lp-section {
  margin: 18px 0 10px;
  font-weight: 600;
}
.lp-section:first-child {
  margin-top: 0;
}
.lp-styles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.lp-style {
  padding: 0;
  border: 2px solid var(--el-border-color-lighter);
  border-radius: 10px;
  background: var(--el-bg-color);
  overflow: hidden;
  cursor: pointer;
  text-align: left;
  font: inherit;
  color: inherit;
}
.lp-style:hover {
  border-color: var(--el-color-primary-light-5);
}
.lp-style.active {
  border-color: var(--el-color-primary);
}
.lp-style:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
}
.lp-style-name {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px 6px;
  font-size: 13px;
}
.lp-tag {
  margin-left: auto;
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.lp-form :deep(.el-form-item) {
  margin-bottom: 14px;
}
.lp-clear {
  margin-left: 10px;
}
.lp-slider {
  width: 100%;
  max-width: 280px;
  margin-left: 6px;
}
.lp-logo {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.lp-logo-thumb {
  width: 36px;
  height: 36px;
  flex: none;
  border-radius: 8px;
  border: 1px solid var(--el-border-color);
  display: grid;
  place-items: center;
  overflow: hidden;
  color: var(--el-text-color-placeholder);
  background: repeating-conic-gradient(var(--el-fill-color) 0 25%, transparent 0 50%) 0 0 / 10px 10px;
}
.lp-logo-thumb img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.lp-logo-thumb.empty {
  background: none;
}
.lp-css :deep(textarea) {
  font-size: 12.5px;
}
.lp-classes {
  display: block;
  margin-top: 4px;
  word-spacing: 4px;
}
.lp-main {
  position: sticky;
  top: 0;
}
.lp-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}
.lp-stage {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  overflow: hidden;
}
.lp-stage.mobile {
  max-width: 300px;
  margin: 0 auto;
}
.lp-loading,
.lp-error {
  min-height: 200px;
  display: grid;
  place-items: center;
}
.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid var(--el-border-color);
  border-top-color: var(--el-color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
@media (max-width: 900px) {
  .lp {
    grid-template-columns: 1fr;
  }
  .lp-side {
    max-height: none;
    overflow: visible;
  }
  .lp-main {
    position: static;
    order: -1;
  }
  .lp-styles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
