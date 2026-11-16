<template>
  <!-- 转发目标地址的即时提示：地址写成「主机:端口」时提示端口会拆到目标端口（可一键拆分），与目标端口冲突或写法无效时用错误色提示。
       规则与后端 l4proxy.SplitTargets 一致，保存时后端同样会拆分或拒绝 -->
  <div v-if="check.kind" class="target-hint" :class="check.kind === 'split' ? 'is-info' : 'is-error'" role="status">
    <template v-if="check.kind === 'split'">
      {{ t('components.targetHint.split', { addr: check.addr, port: check.port }) }}
      <el-button link type="primary" size="small" @click="emit('split', { hosts: check.hosts, port: check.port })">{{ t('components.targetHint.splitNow') }}</el-button>
    </template>
    <template v-else-if="check.kind === 'conflict'">{{ t('components.targetHint.conflict', { addr: check.addr, port: check.port, ports: ports }) }}</template>
    <template v-else-if="check.kind === 'mixed'">{{ t('components.targetHint.mixed') }}</template>
    <template v-else>{{ t('components.targetHint.invalid', { addr: check.addr }) }}</template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { checkTargets } from '../netaddr'

// lines 为目标地址（每项一个地址）；ports 为目标端口（字符串，空为未填）
const props = defineProps<{ lines: string[]; ports: string }>()
const emit = defineEmits<{ split: [v: { hosts: string[]; port: number }] }>()
const { t } = useI18n()
const check = computed(() => checkTargets(props.lines, props.ports))
</script>

<style scoped>
.target-hint {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
}
.target-hint.is-info {
  color: var(--g-text-warning);
}
.target-hint.is-error {
  color: var(--g-text-danger);
}
</style>
