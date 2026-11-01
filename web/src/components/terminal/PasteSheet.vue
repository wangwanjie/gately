<template>
  <!-- 粘贴面板：读不到系统剪贴板时在此手动粘贴，也可从最近复制记录中一键发送。移动端为底部抽屉，桌面端为对话框。 -->
  <component :is="isMobile ? ElDrawer : ElDialog" v-bind="wrapperProps" :model-value="modelValue" @update:model-value="(v: boolean) => $emit('update:modelValue', v)" @opened="onOpened">
    <div class="paste-sheet">
      <div v-if="isMobile" class="sheet-head">{{ t('terminal.paste.title') }}</div>
      <div v-if="manual" class="tip">{{ t('terminal.msg.pasteManual') }}</div>
      <el-input ref="inputRef" v-model="text" type="textarea" :rows="3" resize="none" :placeholder="t('terminal.paste.placeholder')" />
      <div class="actions">
        <el-button type="primary" :disabled="!text" @click="send(text)">{{ t('terminal.paste.send') }}</el-button>
      </div>
      <div class="recent-head">
        <span>{{ t('terminal.paste.recent') }}</span>
        <el-button v-if="recentClips.length" text size="small" @click="clearClips">{{ t('terminal.paste.clear') }}</el-button>
      </div>
      <div class="list">
        <div v-for="c in recentClips" :key="c" class="item">
          <button class="body" @click="send(c)">
            <span class="mono preview">{{ preview(c) }}</span>
            <span v-if="lineCount(c) > 1" class="lines">{{ t('terminal.paste.lines', { count: lineCount(c) }) }}</span>
          </button>
          <el-button text :icon="X" :aria-label="t('terminal.paste.remove')" @click="removeClip(c)" />
        </div>
        <div v-if="!recentClips.length" class="empty">{{ t('terminal.paste.empty') }}</div>
      </div>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElDialog, ElDrawer } from 'element-plus'
import { X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { clearClips, recentClips, removeClip } from './clipboard'

const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
  isMobile: boolean
  // 浏览器不允许读取剪贴板，需要手动粘贴
  manual: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean]; send: [text: string] }>()

const text = ref('')
const inputRef = ref<{ focus: () => void }>()

const wrapperProps = computed(() =>
  props.isMobile
    ? { direction: 'btt', size: 'auto', withHeader: false, class: 'sheet' }
    : { title: t('terminal.paste.title'), width: '520px', class: 'term-dialog', appendToBody: true },
)

watch(
  () => props.modelValue,
  (v) => {
    if (v) {
      text.value = ''
    }
  },
)

// 需要手动粘贴时直接聚焦输入框，用户长按即可调出系统粘贴菜单
function onOpened() {
  if (props.manual) {
    inputRef.value?.focus()
  }
}

function lineCount(s: string): number {
  return s.replace(/\r?\n$/, '').split(/\r\n|\r|\n/).length
}

// 列表里只显示第一行，过长时截断
function preview(s: string): string {
  const first = s.replace(/^\s+/, '').split(/\r\n|\r|\n/)[0]
  return first.length > 120 ? first.slice(0, 120) + '…' : first
}

function send(s: string) {
  if (s) {
    emit('send', s)
  }
}
</script>

<style scoped>
.paste-sheet {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-bottom: env(safe-area-inset-bottom);
}
.sheet-head {
  font-weight: 600;
}
.tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.actions {
  display: flex;
  justify-content: flex-end;
}
.recent-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.list {
  max-height: 40vh;
  overflow: auto;
}
.item {
  display: flex;
  align-items: center;
  gap: 4px;
  border-bottom: 1px solid var(--g-border);
}
.body {
  flex: 1;
  min-width: 0;
  min-height: 44px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
  border: none;
  background: transparent;
  color: var(--el-text-color-primary);
  text-align: left;
  cursor: pointer;
}
.body:active {
  background: var(--el-fill-color-light);
}
.preview {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}
.lines {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
.empty {
  padding: 12px 0;
  text-align: center;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
</style>
