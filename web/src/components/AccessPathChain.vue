<template>
  <!-- 访问路径：由外向内逐跳列出，经过 Gately 转发的跳高亮，受影响的跳标红 -->
  <ol class="path-chain" data-test="access-path">
    <li v-for="(h, i) in path.hops" :key="i" :class="['hop', 'k-' + h.kind, { risk: riskHops.includes(i) }]">
      <span class="label">{{ hopLabel(h) }}</span>
      <span v-if="h.listen && h.kind !== 'admin'" class="addr mono">{{ h.listen }}</span>
      <ArrowRight v-if="i < path.hops.length - 1" :size="14" class="sep" />
    </li>
  </ol>
</template>

<script setup lang="ts">
import { ArrowRight } from 'lucide-vue-next'
import { hopLabel, type AccessPath } from '../accessPath'

withDefaults(defineProps<{ path: AccessPath; riskHops?: number[] }>(), { riskHops: () => [] })
</script>

<style scoped>
.path-chain {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.hop {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.label {
  padding: 3px 8px;
  border-radius: 6px;
  border: 1px solid var(--g-border);
  font-size: 13px;
  background: var(--el-fill-color-light);
}
.k-portforward .label,
.k-protoroute .label,
.k-webservice .label {
  border-color: var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
}
.risk .label {
  border-color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger);
}
.addr {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.sep {
  color: var(--el-text-color-placeholder);
}
</style>
