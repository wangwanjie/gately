<template>
  <!-- 管理端安全评分：按当前配置逐项检查，给出得分与一键修复入口 -->
  <div class="g-card sec-center" :class="status?.level">
    <div class="g-card-head">
      <div class="g-card-title"><ShieldCheck :size="16" />{{ t('security.center.title') }}</div>
      <el-button text size="small" :icon="RefreshCw" :loading="loading" @click="load">{{ t('security.center.refresh') }}</el-button>
    </div>
    <div class="g-card-body">
      <p class="desc">{{ t('security.center.desc') }}</p>
      <div v-if="status" class="sc-main">
        <div class="sc-score" :class="status.level">
          <svg viewBox="0 0 100 100" class="ring" role="img" :aria-label="t('security.center.scoreLabel', { score: status.score })">
            <circle cx="50" cy="50" r="44" class="track" />
            <circle cx="50" cy="50" r="44" class="bar" :stroke-dasharray="`${(status.score / 100) * CIRC} ${CIRC}`" transform="rotate(-90 50 50)" />
          </svg>
          <div class="num">
            <b>{{ status.score }}</b>
            <span>{{ t(`security.center.level.${status.level}`) }}</span>
          </div>
        </div>
        <div class="sc-side">
          <div class="exposure" :class="{ on: status.exposed }">
            <component :is="status.exposed ? Globe : Lock" :size="14" />
            {{ status.exposed ? t('security.center.exposed') : t('security.center.notExposed') }}
          </div>
          <ul class="sc-items">
            <li v-for="it in ordered" :key="it.id" :class="it.status">
              <span class="ic">
                <CircleCheck v-if="it.status === 'ok'" :size="17" />
                <ShieldAlert v-else-if="it.status === 'risk'" :size="17" />
                <TriangleAlert v-else :size="17" />
              </span>
              <div class="tx">
                <div class="tt">{{ t(`security.items.${it.id}.title`) }}</div>
                <div class="dd">{{ t(`security.items.${it.id}.${it.status === 'ok' ? 'ok' : it.id === 'password' ? status.passwordIssue || 'weak' : 'warn'}`) }}</div>
              </div>
              <el-button v-if="it.status !== 'ok'" size="small" :type="it.status === 'risk' ? 'danger' : 'primary'" plain @click="$emit('go', it.id)">{{ t('security.center.fix') }}</el-button>
            </li>
          </ul>
        </div>
      </div>
      <el-skeleton v-else-if="loading" :rows="4" animated />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleCheck, Globe, Lock, RefreshCw, ShieldAlert, ShieldCheck, TriangleAlert } from 'lucide-vue-next'
import { ITEM_ORDER } from '../security'
import { useSecurity } from '../composables/useSecurity'

defineEmits<{ (e: 'go', id: string): void }>()
const { t } = useI18n()
const { status, loading, load } = useSecurity()

// 圆环周长：r=44
const CIRC = 2 * Math.PI * 44

// 未通过的在前（按扣分多少），通过的在后，保持固定顺序
const ordered = computed(() => {
  const items = [...(status.value?.items ?? [])]
  return items.sort((a, b) => {
    const fa = a.status !== 'ok' ? 0 : 1
    const fb = b.status !== 'ok' ? 0 : 1
    return fa - fb || b.points - a.points || ITEM_ORDER.indexOf(a.id) - ITEM_ORDER.indexOf(b.id)
  })
})

onMounted(() => {
  load()
})
</script>

<style scoped>
.sc-main {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}
.sc-score {
  position: relative;
  flex-shrink: 0;
  width: 116px;
  height: 116px;
}
.ring {
  width: 100%;
  height: 100%;
}
.ring circle {
  fill: none;
  stroke-width: 9;
}
.ring .track {
  stroke: var(--el-fill-color-dark);
}
.ring .bar {
  stroke-linecap: round;
  stroke: var(--el-color-success);
  transition: stroke-dasharray 0.4s ease;
}
.sc-score.warn .bar {
  stroke: var(--el-color-warning);
}
.sc-score.risk .bar {
  stroke: var(--el-color-danger);
}
.num {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  line-height: 1.2;
}
.num b {
  font-size: 30px;
  font-weight: 700;
}
.num span {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.sc-side {
  flex: 1;
  min-width: 0;
}
.exposure {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 10px;
  margin-bottom: 8px;
  border-radius: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
}
.exposure.on {
  color: var(--el-color-warning-dark-2);
  background: var(--el-color-warning-light-9);
}
.sc-items {
  margin: 0;
  padding: 0;
  list-style: none;
}
.sc-items li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 0;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}
.sc-items li:last-child {
  border-bottom: 0;
}
.ic {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  color: var(--el-color-success);
}
li.warn .ic {
  color: var(--el-color-warning);
}
li.risk .ic {
  color: var(--el-color-danger);
}
.tx {
  flex: 1;
  min-width: 0;
}
.tt {
  font-size: 14px;
  font-weight: 500;
}
.dd {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
  overflow-wrap: anywhere;
}
/* 移动端：评分环居中置顶，检查项的修复按钮换到文字下方 */
@media (max-width: 767.98px) {
  .sc-main {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }
  .sc-score {
    align-self: center;
  }
  .sc-items li {
    flex-wrap: wrap;
  }
  .sc-items li .el-button {
    margin-left: 27px;
  }
}
</style>
