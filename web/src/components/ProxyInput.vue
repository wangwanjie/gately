<template>
  <!-- 出站代理设置：取值为 ''、direct、update 或代理地址，含义见后端 internal/proxyx -->
  <div class="proxy-input">
    <el-select :model-value="mode" class="mode" @update:model-value="setMode">
      <el-option v-if="inherit" :label="t('components.proxyInput.inherit')" value="inherit" />
      <el-option :label="inherit ? t('components.proxyInput.direct') : t('components.proxyInput.none')" value="none" />
      <el-option :label="t('components.proxyInput.update')" value="update" />
      <el-option :label="t('components.proxyInput.custom')" value="custom" />
    </el-select>
    <el-input
      v-if="mode === 'custom'"
      :model-value="model"
      class="mono addr"
      clearable
      placeholder="http://127.0.0.1:7890"
      @update:model-value="(v: string) => (model = v)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// inherit 为 true 时（例如通知渠道）留空表示跟随上一级设置，另提供 direct 强制直连；
// 否则留空即不指定代理（HTTP 请求仍遵循 HTTPS_PROXY 等环境变量）。
// owner 为正在编辑的对象：弹窗复用本组件编辑另一条记录时据此清掉“已选自定义但未填写”的状态
const props = defineProps<{ inherit?: boolean; owner?: object }>()
const model = defineModel<string>({ default: '' })

type Mode = 'inherit' | 'none' | 'update' | 'custom'

// 选了“自定义”但还没填地址时，值仍为空，用 customPicked 记住选择，避免下拉框跳回默认项
const customPicked = ref(false)

const mode = computed<Mode>(() => {
  const v = (model.value ?? '').trim().toLowerCase()
  if (v === 'update') {
    return 'update'
  }
  if (v === 'direct') {
    return 'none'
  }
  if (v) {
    return 'custom'
  }
  if (customPicked.value) {
    return 'custom'
  }
  return props.inherit ? 'inherit' : 'none'
})

// 切换模式时保留上一次填写的代理地址，切回“自定义”可以继续使用（只在同一条记录内）
let lastCustom = ''
watch(
  () => props.owner,
  () => {
    customPicked.value = false
    lastCustom = ''
  },
)

function setMode(m: Mode) {
  const v = (model.value ?? '').trim()
  if (mode.value === 'custom' && v) {
    lastCustom = v
  }
  customPicked.value = m === 'custom'
  switch (m) {
    case 'inherit':
      model.value = ''
      break
    case 'none':
      model.value = props.inherit ? 'direct' : ''
      break
    case 'update':
      model.value = 'update'
      break
    case 'custom':
      model.value = lastCustom
      break
  }
}
</script>

<style scoped>
.proxy-input {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  width: 100%;
}
.mode {
  width: 200px;
}
.addr {
  flex: 1;
  min-width: 220px;
}
</style>
