<template>
  <div>
    <el-form-item :label="t('webhookEditor.enable')">
      <el-switch v-model="model.enable" />
    </el-form-item>
    <template v-if="model.enable">
      <el-form-item :label="t('webhookEditor.url')">
        <el-input v-model="model.url" placeholder="https://example.com/notify?ip={ipAddr}" />
      </el-form-item>
      <el-form-item :label="t('webhookEditor.method')">
        <el-select v-model="model.method" style="width: 120px">
          <el-option v-for="m in ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']" :key="m" :label="m" :value="m" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('webhookEditor.headers')">
        <el-input v-model="headersText" type="textarea" :rows="2" :placeholder="t('webhookEditor.headersPlaceholder')" />
      </el-form-item>
      <el-form-item v-if="model.method !== 'GET'" :label="t('webhookEditor.body')">
        <el-input v-model="model.body" type="textarea" :rows="3" class="mono" />
      </el-form-item>
      <el-form-item :label="t('webhookEditor.successMark')">
        <el-input v-model="successText" type="textarea" :rows="2" :placeholder="t('webhookEditor.successPlaceholder')" />
      </el-form-item>
      <el-form-item :label="t('webhookEditor.timeout')">
        <el-input-number v-model="model.timeout" :min="0" :max="120" />
        <el-checkbox v-model="model.insecureSkipVerify" class="skip">{{ t('webhookEditor.ignoreCertError') }}</el-checkbox>
      </el-form-item>
      <el-form-item :label="t('webhookEditor.proxy')">
        <ProxyInput v-model="proxy" :owner="model" />
        <div class="form-help">{{ t('webhookEditor.proxyHelp') }}</div>
      </el-form-item>
      <el-form-item :label="t('webhookEditor.variables')">
        <div class="vars">
          <div class="vars-tip">
            <i18n-t keypath="webhookEditor.varsTip" tag="span">
              <template #plain><span class="mono">{name}</span></template>
              <template #hash><span class="mono">#{name}</span></template>
            </i18n-t>
          </div>
          <el-tag v-for="v in variables" :key="v" size="small" type="info" class="var mono">{{ '{' + v + '}' }}</el-tag>
        </div>
      </el-form-item>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useListText, splitLines } from '../composables/useListText'
import ProxyInput from './ProxyInput.vue'

export interface WebhookConfig {
  enable: boolean
  url: string
  method: string
  headers: string[] | null
  body: string
  successContent: string[] | null
  timeout: number
  insecureSkipVerify: boolean
  // proxy 为请求使用的代理：留空不指定，update 使用全局更新代理，或代理地址
  proxy?: string
}

const { t } = useI18n()
const props = defineProps<{ variables: string[] }>()
const model = defineModel<WebhookConfig>({ required: true })

// 多行文本与数组互转
// 输入框保留原始文本，回车可正常换行（见 useListText）
const headersText = useListText(() => model.value.headers, (v) => (model.value.headers = v), { split: splitLines })
const successText = useListText(() => model.value.successContent, (v) => (model.value.successContent = v), { split: splitLines })
const variables = computed(() => props.variables)
// 旧配置没有 proxy 字段，读写时按空字符串处理
const proxy = computed({
  get: () => model.value.proxy ?? '',
  set: (v: string) => (model.value.proxy = v),
})
</script>

<style scoped>
.skip {
  margin-left: 16px;
}
.vars {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.vars-tip {
  margin-bottom: 4px;
}
@media (max-width: 767.98px) {
  .skip {
    margin-left: 0;
    width: 100%;
  }
}
.var {
  margin: 2px 4px 2px 0;
}
</style>
