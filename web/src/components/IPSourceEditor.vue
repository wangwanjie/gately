<template>
  <div>
    <el-form-item :label="t('ipSourceEditor.fetchIp', { family })">
      <el-switch v-model="model.enable" />
    </el-form-item>
    <template v-if="model.enable">
      <el-form-item :label="t('ipSourceEditor.method')">
        <el-radio-group v-model="model.type">
          <el-radio-button value="url">{{ t('ipSourceEditor.typeUrl') }}</el-radio-button>
          <el-radio-button value="netInterface">{{ t('ipSourceEditor.typeInterface') }}</el-radio-button>
          <el-radio-button value="command">{{ t('ipSourceEditor.typeCommand') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="model.type === 'url'" :label="t('ipSourceEditor.urls')">
        <el-input v-model="urlsText" type="textarea" :rows="4" :placeholder="t('ipSourceEditor.urlsPlaceholder')" class="mono" />
      </el-form-item>
      <el-form-item v-if="model.type === 'netInterface'" :label="t('ipSourceEditor.interface')">
        <el-select v-model="model.interface" filterable allow-create style="width: 100%" :placeholder="t('ipSourceEditor.interfacePlaceholder')">
          <el-option v-for="i in interfaces" :key="i.name" :label="i.name" :value="i.name">
            <span>{{ i.name }}</span>
            <span class="iface-addrs">{{ i.addrs.join('  ') }}</span>
          </el-option>
        </el-select>
      </el-form-item>
      <el-form-item v-if="model.type === 'command'" :label="t('ipSourceEditor.command')">
        <el-input v-model="model.command" :placeholder="t('ipSourceEditor.commandPlaceholder')" class="mono" />
      </el-form-item>
      <el-form-item :label="t('ipSourceEditor.regex')">
        <el-input v-model="model.regex" :placeholder="t('ipSourceEditor.regexPlaceholder')" class="mono" />
      </el-form-item>
      <el-form-item>
        <el-button v-if="moduleBuilt('ddns')" size="small" :loading="testing" @click="test">{{ t('ipSourceEditor.testFetch') }}</el-button>
        <span v-if="result" class="result" :class="{ err: resultError }">{{ result }}</span>
      </el-form-item>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { moduleBuilt } from '../nav'
import { useListText, splitLines } from '../composables/useListText'

export interface IPSource {
  enable: boolean
  type: string
  urls: string[] | null
  interface: string
  regex: string
  command: string
}

export interface NetInterface {
  name: string
  addrs: string[]
}

const { t } = useI18n()
const props = defineProps<{ family: 4 | 6; interfaces: NetInterface[] }>()
const model = defineModel<IPSource>({ required: true })
const testing = ref(false)
const result = ref('')
const resultError = ref(false)
const interfaces = computed(() => props.interfaces)

// 输入框保留原始文本，回车可正常换行（见 useListText）
const urlsText = useListText(() => model.value.urls, (v) => (model.value.urls = v), { split: splitLines })

async function test() {
  testing.value = true
  result.value = ''
  try {
    const r = await api.post<{ ip: string }>('/api/ddns/iptest', { source: model.value, family: props.family })
    result.value = t('ipSourceEditor.fetchSuccess', { ip: r.ip })
    resultError.value = false
  } catch (e) {
    result.value = (e as Error).message
    resultError.value = true
  } finally {
    testing.value = false
  }
}
</script>

<style scoped>
.iface-addrs {
  float: right;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-left: 16px;
}
.result {
  margin-left: 12px;
  word-break: break-all;
  font-size: 13px;
  color: var(--el-color-success);
}
.result.err {
  color: var(--el-color-danger);
}
</style>
