<template>
  <div ref="root" class="ep">
    <el-form-item :label="t('errorPages.style')">
      <div class="body">
        <div class="form-help top">{{ scopeHelp }}</div>
        <div class="grid" role="radiogroup" :aria-label="t('errorPages.style')" data-test="ep-styles">
          <template v-for="o in builtinOptions" :key="o.id">
            <ErrorStyleCard :option="o" :active="o.id === selectedId" :focusable="o.id === focusId" @select="select(o.id)" @preview="openPreview(o)" @move="(d: number) => move(o, d)" />
          </template>
        </div>
        <div class="sub-head">
          <span>{{ t('errorPages.customHeading') }}</span>
          <el-button size="small" text type="primary" :icon="FileCode" data-test="ep-manage" @click="managerVisible = true">{{ t('errorPages.manage') }}</el-button>
        </div>
        <div v-if="customOptions.length" class="grid" role="radiogroup" :aria-label="t('errorPages.customHeading')" data-test="ep-custom">
          <template v-for="o in customOptions" :key="o.id">
            <ErrorStyleCard :option="o" :active="o.id === selectedId" :focusable="o.id === focusId" @select="select(o.id)" @preview="openPreview(o)" @move="(d: number) => move(o, d)" />
          </template>
        </div>
        <div v-else class="form-help">{{ t('errorPages.manageHelp') }}</div>
        <el-alert v-if="missingPage" :title="t('errorPages.missingPage')" type="error" :closable="false" show-icon class="gap" />
        <div v-if="notFoundPage && siteType === 'static'" class="form-help">{{ t('errorPages.staticOwn404', { page: notFoundPage }) }}</div>
      </div>
    </el-form-item>

    <!-- 拦截后端错误只对反向代理站点生效 -->
    <el-form-item v-if="siteType === 'proxy'" :label="t('errorPages.intercept')">
      <div class="body">
        <el-switch v-model="model.intercept" data-test="ep-intercept" @change="onInterceptChange" />
        <div class="form-help">{{ t('errorPages.interceptHelp') }}</div>
        <template v-if="model.intercept">
          <el-input v-model="model.codes" class="mono codes" :placeholder="defaultInterceptCodes" data-test="ep-codes" />
          <div class="quick">
            <span class="form-help inline">{{ t('errorPages.quick') }}</span>
            <el-button v-for="q in quickCodes" :key="q" size="small" text type="primary" @click="model.codes = q">{{ q }}</el-button>
          </div>
          <div class="form-help">{{ t('errorPages.codesHelp') }}</div>
          <div v-if="codesError" class="form-help warn">{{ codesError }}</div>
        </template>
      </div>
    </el-form-item>

    <ErrorPagesManager v-model="managerVisible" @changed="loadCustom" />
    <ErrorPagePreview v-model="previewVisible" :title="previewTitle" :load="previewLoader" :initial-code="sampleCode" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FileCode } from 'lucide-vue-next'
import ErrorStyleCard, { type StyleOption } from './ErrorStyleCard.vue'
import ErrorPagePreview from './ErrorPagePreview.vue'
import ErrorPagesManager from './ErrorPagesManager.vue'
import { checkCodes } from '../errorPageUtil'
import { builtinStyles, defaultInterceptCodes, errorPagesApi, loadStyles, type ErrorPageOptions, type ErrorPageView } from '../errorPages'

const model = defineModel<ErrorPageOptions>({ required: true })
const props = defineProps<{
  // siteType 为站点类型；isDefault 为默认站点（其设置同时用于未匹配任何站点的 404）
  siteType: string
  isDefault?: boolean
  notFoundPage?: string
}>()
const { t } = useI18n()

// 缩略图使用的示例状态码：反向代理以连不上后端的 502 为例，重定向以目标不可用的 503 为例，默认站点与其余类型以 404 为例
const sampleCode = computed(() => (props.isDefault ? 404 : props.siteType === 'proxy' ? 502 : props.siteType === 'redirect' ? 503 : 404))
const quickCodes = ['404, 500-599', '5xx', '502, 503, 504', '404']


const styleHtml = ref<Record<string, string>>({})
const styleError = ref('')
const custom = ref<ErrorPageView[]>([])
const customHtml = ref<Record<string, string>>({})
const customError = ref<Record<string, string>>({})
const managerVisible = ref(false)

const selectedId = computed(() => (model.value.style === 'custom' ? `custom:${model.value.page}` : model.value.style))

// missingPage 为引用的自定义页面已不在页面库中（例如在其他地方被删除）
const missingPage = computed(
  () => model.value.style === 'custom' && !!model.value.page && !loadingCustom.value && !customFailed.value && !custom.value.some((p) => p.key === model.value.page),
)

const builtinOptions = computed<StyleOption[]>(() => [
  { id: '', name: t('errorPages.styles.default'), html: styleHtml.value[''] ?? '', error: styleError.value },
  ...builtinStyles.map((id) => ({ id: id as string, name: t(`errorPages.styles.${id}`), html: styleHtml.value[id] ?? '', error: styleError.value })),
])
const customOptions = computed<StyleOption[]>(() =>
  custom.value.map((p) => ({ id: `custom:${p.key}`, name: p.name, html: customHtml.value[p.key] ?? '', error: customError.value[p.key] ?? p.error ?? '', broken: !!p.error })),
)

const scopeHelp = computed(() => {
  if (props.isDefault) {
    return t('errorPages.scopeDefault')
  }
  return props.siteType === 'proxy' ? t('errorPages.scopeProxy') : props.siteType === 'static' ? t('errorPages.scopeStatic') : t('errorPages.scopeOther')
})

// 键盘操作按单选组的惯例：Tab 只停在选中的卡片上（没有选中的可见卡片时停在第一张），方向键在全部卡片间移动并选中
const allOptions = computed(() => [...builtinOptions.value, ...customOptions.value])
const focusId = computed(() => (allOptions.value.some((o) => o.id === selectedId.value) ? selectedId.value : ''))
const root = ref<HTMLElement>()

function move(o: StyleOption, d: number) {
  const list = allOptions.value
  const i = list.findIndex((x) => x.id === o.id)
  const next = list[(i + d + list.length) % list.length]
  select(next.id)
  nextTick(() => root.value?.querySelector<HTMLElement>(`[data-style="${CSS.escape(next.id || 'default')}"]`)?.focus())
}

function select(id: string) {
  if (id.startsWith('custom:')) {
    model.value.style = 'custom'
    model.value.page = id.slice(7)
  } else {
    model.value.style = id
    model.value.page = ''
  }
}

// 开启拦截且没有填写状态码时填入常用值
function onInterceptChange(v: string | number | boolean) {
  if (v && !model.value.codes.trim()) {
    model.value.codes = defaultInterceptCodes
  }
}

// codesError 在前端先做一遍与后端相同的格式检查，保存前就能看到问题
const codesError = computed(() => {
  if (!model.value.intercept) {
    return ''
  }
  const p = checkCodes(model.value.codes)
  if (!p) {
    return ''
  }
  return p.empty ? t('errorPages.codesEmpty') : t('errorPages.codesBad', { code: p.bad })
})

// builtinSeq 与 customSeq 丢弃过期的加载结果：示例状态码变化或页面库刷新时，旧请求的缩略图不会覆盖新的
let builtinSeq = 0
let customSeq = 0

async function loadBuiltin() {
  const my = ++builtinSeq
  try {
    const list = await loadStyles(sampleCode.value)
    if (my === builtinSeq) {
      styleHtml.value = Object.fromEntries(list.map((s) => [s.id, s.html]))
      styleError.value = ''
    }
  } catch (e) {
    if (my === builtinSeq) {
      styleError.value = (e as Error).message
    }
  }
}

const loadingCustom = ref(true)
// customFailed 为页面库列表加载失败：此时无法判断引用的页面是否存在，不提示“已被删除”
const customFailed = ref(false)

async function loadCustom() {
  const my = ++customSeq
  loadingCustom.value = true
  let list: ErrorPageView[] = []
  try {
    list = await errorPagesApi.list()
    customFailed.value = false
  } catch {
    customFailed.value = true
  }
  if (my !== customSeq) {
    return
  }
  custom.value = list
  loadingCustom.value = false
  const code = sampleCode.value
  await Promise.all(
    list.map(async (p) => {
      try {
        const html = (await errorPagesApi.preview(p.key, code)).html
        if (my === customSeq) {
          customHtml.value[p.key] = html
          delete customError.value[p.key]
        }
      } catch (e) {
        if (my === customSeq) {
          customError.value[p.key] = (e as Error).message
        }
      }
    }),
  )
}

watch(sampleCode, () => {
  loadBuiltin()
  loadCustom()
})

onMounted(() => {
  loadBuiltin()
  loadCustom()
})

const previewVisible = ref(false)
const previewTitle = ref('')
const previewLoader = ref<((code: number) => Promise<string>) | null>(null)

function openPreview(o: StyleOption) {
  previewTitle.value = o.name
  if (o.id.startsWith('custom:')) {
    const key = o.id.slice(7)
    previewLoader.value = (code) => errorPagesApi.preview(key, code).then((r) => r.html)
  } else {
    previewLoader.value = (code) => loadStyles(code).then((list) => list.find((s) => s.id === o.id)?.html ?? '')
  }
  previewVisible.value = true
}
</script>

<style scoped>
.body {
  width: 100%;
  min-width: 0;
}
.top {
  margin: 0 0 8px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(132px, 1fr));
  gap: 10px;
}
.gap {
  margin-top: 8px;
}
.sub-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
}
.form-help.inline {
  margin: 0;
}
.codes {
  margin-top: 8px;
  max-width: 360px;
}
.quick {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
}
.quick .el-button + .el-button {
  margin-left: 0;
}
.form-help.warn {
  color: var(--el-color-danger);
}
@media (max-width: 767.98px) {
  .grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
