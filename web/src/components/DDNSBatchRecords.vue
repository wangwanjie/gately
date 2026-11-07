<template>
  <!-- DDNS 任务编辑框里批量添加解析记录：每行一个域名，实时预览将添加的记录、重复跳过的行与无效行，
       确认后把记录追加到编辑中的任务，随任务一起保存 -->
  <el-dialog v-model="visible" :title="t('ddns.batchTitle')" width="640px" :fullscreen="isMobile" append-to-body @open="init">
    <el-input
      v-model="text"
      type="textarea"
      :rows="8"
      class="mono batch-text"
      :placeholder="'www.example.com\nnas.example.com\nemby\nv6.example.com AAAA\ncdn.example.com CNAME target.example.net'"
      data-test="ddns-batch-text"
    />
    <div class="form-help">{{ t('ddns.batchHelp') }}</div>

    <el-form label-width="110px" :label-position="labelPosition" class="batch-form" @submit.prevent>
      <el-form-item :label="t('ddns.batchZone')">
        <el-input v-model="zone" class="mono w-260" placeholder="example.com" />
        <span class="help">{{ t('ddns.batchZoneHelp') }}</span>
      </el-form-item>
      <el-form-item :label="t('ddns.type')">
        <el-checkbox-group v-model="types">
          <el-checkbox value="A">A (IPv4)</el-checkbox>
          <el-checkbox value="AAAA">AAAA (IPv6)</el-checkbox>
        </el-checkbox-group>
        <span class="help">{{ t('ddns.batchTypesHelp') }}</span>
      </el-form-item>
      <el-form-item label="TTL">
        <el-input-number v-model="ttl" :min="0" :controls="false" class="w-120" />
        <span class="help">{{ t('ddns.batchTtlHelp') }}</span>
      </el-form-item>
      <el-form-item v-if="supportsLine" :label="t('ddns.line')">
        <el-input v-model="line" class="w-260" :placeholder="t('ddns.default')" />
      </el-form-item>
      <el-form-item :label="t('common.remark')">
        <el-input v-model="remark" :maxlength="100" :placeholder="t('ddns.remarkPlaceholder')" />
      </el-form-item>
    </el-form>

    <div v-if="hasInput" class="batch-result" data-test="ddns-batch-result">
      <div class="summary">
        <span class="ok">{{ t('ddns.batchWillAdd', { count: result.records.length }) }}</span>
        <span v-if="result.duplicates.length" class="dup" :title="result.duplicates.map((d) => d.text).join('\n')">
          {{ t('ddns.batchDuplicates', { count: result.duplicates.length }) }}
        </span>
        <span v-if="result.errors.length" class="err">{{ t('ddns.batchErrors', { count: result.errors.length }) }}</span>
      </div>
      <ul v-if="result.errors.length" class="errors">
        <li v-for="e in result.errors" :key="e.line">
          <span class="ln">{{ t('ddns.batchLineNo', { line: e.line }) }}</span>
          <span class="mono src">{{ e.text }}</span>
          <span class="why">{{ t(`ddns.batchErr.${e.reason}`) }}</span>
        </li>
      </ul>
      <ul v-if="result.records.length" class="preview">
        <li v-for="(r, i) in result.records" :key="i" class="mono">
          <span class="ty">{{ r.type }}</span>
          <span class="dn">{{ r.sub === '@' ? r.zone : `${r.sub}.${r.zone}` }}</span>
          <span class="val">{{ r.value }}</span>
        </li>
      </ul>
    </div>

    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :disabled="!canSubmit" data-test="ddns-batch-submit" @click="submit">
        {{ t('ddns.batchSubmit', { count: result.records.length }) }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { parseBatch } from '../ddnsBatch'
import { useFormLayout } from '../composables/useIsMobile'

export interface BatchNewRecord {
  enable: boolean
  zone: string
  sub: string
  type: string
  value: string
  ttl: number
  line: string
  remark: string
}

const props = defineProps<{
  // existing 为编辑中任务已有的记录，用于去重与默认主域名
  existing: { sub: string; zone: string; type: string; line?: string }[]
  // knownZones 为可用来拆分完整域名的其他主域名（例如同服务商其他任务里的）
  knownZones: string[]
  supportsLine?: boolean
  // defaultTypes 为打开时默认勾选的类型，按任务已启用的 IP 获取方式决定
  defaultTypes: string[]
}>()
const emit = defineEmits<{ add: [records: BatchNewRecord[]] }>()
const visible = defineModel<boolean>({ required: true })

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const text = ref('')
const zone = ref('')
const types = ref<string[]>(['A'])
const ttl = ref(0)
const line = ref('')
const remark = ref('')

// init 每次打开时重置输入，默认主域名取已有记录里最常用的
function init() {
  text.value = ''
  ttl.value = 0
  line.value = ''
  remark.value = ''
  types.value = [...props.defaultTypes]
  const count = new Map<string, number>()
  for (const r of props.existing) {
    if (r.zone) {
      count.set(r.zone, (count.get(r.zone) ?? 0) + 1)
    }
  }
  zone.value = [...count.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] ?? ''
}

const hasInput = computed(() => text.value.split('\n').some((s) => s.trim() && !s.trim().startsWith('#')))

const result = computed(() =>
  parseBatch(text.value, {
    zone: zone.value,
    types: types.value,
    knownZones: [...props.existing.map((r) => r.zone), ...props.knownZones],
    existing: props.existing.filter((r) => r.zone),
    line: line.value.trim(),
  }),
)

// 有无效行时不允许添加，避免悄悄丢掉用户输入的域名
const canSubmit = computed(() => result.value.records.length > 0 && !result.value.errors.length)

function submit() {
  emit(
    'add',
    result.value.records.map((r) => ({
      enable: true,
      zone: r.zone,
      sub: r.sub,
      type: r.type,
      value: r.value,
      ttl: ttl.value || 0,
      line: line.value.trim(),
      remark: remark.value.trim(),
    })),
  )
  visible.value = false
}
</script>

<style scoped>
.batch-text :deep(textarea) {
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
}
.batch-form {
  margin-top: 14px;
}
.w-260 {
  width: 260px;
}
.w-120 {
  width: 120px;
}
.help {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.5;
}
.batch-result {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 13px;
}
.summary {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
}
.summary .ok {
  color: var(--el-color-success);
}
.summary .dup {
  color: var(--el-color-warning);
}
.summary .err {
  color: var(--el-color-danger);
}
.errors,
.preview {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  max-height: 180px;
  overflow: auto;
}
.errors li {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  color: var(--el-color-danger);
  line-height: 1.8;
}
.errors .src {
  word-break: break-all;
}
.errors .why {
  color: var(--el-text-color-secondary);
}
.preview li {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr) minmax(0, 1fr);
  gap: 10px;
  line-height: 1.8;
  font-size: 12px;
}
.preview .dn,
.preview .val {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.preview .val {
  color: var(--el-text-color-secondary);
}
@media (max-width: 767.98px) {
  .w-260 {
    width: 100%;
  }
  .help {
    margin-left: 0;
    display: block;
  }
}
</style>
