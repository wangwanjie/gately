<template>
  <!-- 保存前的提醒：这次修改会影响当前访问管理界面经过的转发 -->
  <el-dialog
    :model-value="riskPrompt.visible"
    :title="t('accessPath.dialogTitle')"
    width="560px"
    append-to-body
    :close-on-click-modal="false"
    class="access-risk-dialog"
    data-test="access-risk-dialog"
    @close="answerRisk('cancel')"
  >
    <template v-if="check">
      <ul class="risks">
        <li v-for="(r, i) in check.risks" :key="i">{{ riskText(r, check.path) }}</li>
      </ul>
      <div class="path-title">{{ t('accessPath.current') }}</div>
      <AccessPathChain :path="check.path" :risk-hops="check.risks.map((r) => r.hop)" />
      <p class="note">
        {{ confirmState.enabled ? t('accessPath.noteConfirm', { seconds: confirmState.timeout }) : t('accessPath.noteNoConfirm') }}
      </p>
      <!-- 有意的多步修改：先暂存，改完其他地方后一起生效 -->
      <p class="note hold-note">{{ t('accessPath.holdNote') }}</p>
    </template>
    <template #footer>
      <el-button data-test="access-risk-cancel" @click="answerRisk('cancel')">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" plain data-test="access-risk-hold" @click="answerRisk('hold')">{{ t('accessPath.hold') }}</el-button>
      <el-button type="warning" data-test="access-risk-continue" @click="answerRisk('save')">{{ t('accessPath.continue') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AccessPathChain from './AccessPathChain.vue'
import { answerRisk, riskPrompt, riskText } from '../accessPath'
import { confirmState } from '../configConfirm'

const { t } = useI18n()
const check = computed(() => riskPrompt.check)
</script>

<style scoped>
.risks {
  margin: 0 0 14px;
  padding-left: 20px;
  line-height: 1.8;
  color: var(--el-color-danger);
  font-weight: 500;
}
.path-title {
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.hold-note {
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--el-color-primary-light-9);
  color: var(--el-text-color-regular);
}
.note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--el-text-color-secondary);
}
</style>
