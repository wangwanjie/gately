<template>
  <!-- 反向代理"选项"的说明入口：点击问号展开每个选项的作用与适用场景，并可跳转官网详细说明。
       宽屏用气泡就近显示；窄屏气泡会超出屏幕且不遮挡背后内容，改用带遮罩的对话框，背后不再响应滚动 -->
  <el-popover v-if="!isMobile" trigger="click" placement="bottom-start" :width="460">
    <template #reference>
      <button type="button" class="help-btn" :aria-label="t('proxyOptions.title')">
        <CircleHelp :size="14" />
      </button>
    </template>
    <div class="opts-help">
      <div class="opts-title">{{ t('proxyOptions.title') }}</div>
      <dl class="opts-list is-limited">
        <template v-for="it in items" :key="it.id">
          <dt>{{ t(it.label) }}</dt>
          <dd>{{ t(`proxyOptions.${it.id}`) }}</dd>
        </template>
      </dl>
      <a :href="link" target="_blank" rel="noopener" class="opts-link">
        {{ t('proxyOptions.docsLink') }}
        <ExternalLink :size="12" />
      </a>
    </div>
  </el-popover>
  <template v-else>
    <button type="button" class="help-btn" :aria-label="t('proxyOptions.title')" @click="dialogOpen = true">
      <CircleHelp :size="14" />
    </button>
    <el-dialog v-model="dialogOpen" :title="t('proxyOptions.title')" width="92%" top="6vh" append-to-body class="opts-dialog">
      <div class="opts-help">
        <dl class="opts-list">
          <template v-for="it in items" :key="it.id">
            <dt>{{ t(it.label) }}</dt>
            <dd>{{ t(`proxyOptions.${it.id}`) }}</dd>
          </template>
        </dl>
      </div>
      <template #footer>
        <a :href="link" target="_blank" rel="noopener" class="opts-link">
          {{ t('proxyOptions.docsLink') }}
          <ExternalLink :size="12" />
        </a>
      </template>
    </el-dialog>
  </template>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleHelp, ExternalLink } from 'lucide-vue-next'
import { docsUrl } from '../docsUrl'
import { useIsMobile } from '../composables/useIsMobile'

const { t, locale } = useI18n()
const isMobile = useIsMobile()
const dialogOpen = ref(false)
const link = computed(() => docsUrl(locale.value, '/docs/modules/web#proxy-options'))

// 与站点编辑页"选项"中的勾选项一一对应，label 复用勾选项文案
const items = [
  { id: 'skipVerify', label: 'web.optSkipVerify' },
  { id: 'targetHost', label: 'web.optTargetHost' },
  { id: 'noKeepAlive', label: 'web.optNoKeepAlive' },
  { id: 'forwarded', label: 'web.optForwarded' },
  { id: 'sendProxy', label: 'web.optSendProxy' },
  { id: 'sniFromHost', label: 'web.optSniFromHost' },
  { id: 'h2c', label: 'web.optH2C' },
  { id: 'passAuthCookies', label: 'web.optPassAuthCookies' },
]
</script>

<style scoped>
.help-btn {
  display: inline-flex;
  align-items: center;
  padding: 0;
  border: none;
  background: none;
  color: var(--el-text-color-secondary);
  cursor: pointer;
}
.help-btn:hover {
  color: var(--el-color-primary);
}
.opts-help {
  font-size: 13px;
  line-height: 1.6;
}
.opts-title {
  margin-bottom: 6px;
  font-weight: 600;
}
.opts-list {
  margin: 0;
}
/* 气泡内列表限高滚动，避免超出窗口 */
.opts-list.is-limited {
  max-height: min(420px, 55vh);
  overflow-y: auto;
  overscroll-behavior: contain;
}
dt {
  margin-top: 8px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
dt:first-child {
  margin-top: 0;
}
dd {
  margin: 2px 0 0;
  color: var(--el-text-color-regular);
}
.opts-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 10px;
}
</style>

<style>
/* 窄屏对话框：正文区域限高并在内部滚动，标题与官网链接始终可见 */
.opts-dialog .el-dialog__body {
  max-height: 70vh;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.opts-dialog .el-dialog__footer {
  text-align: left;
}
</style>
