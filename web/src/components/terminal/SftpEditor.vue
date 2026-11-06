<template>
  <el-dialog
    :model-value="modelValue"
    :fullscreen="isMobile"
    width="820px"
    top="6vh"
    class="term-dialog editor-dialog"
    :close-on-click-modal="false"
    :before-close="beforeClose"
    append-to-body
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @open="load"
  >
    <template #header>
      <div class="e-title">
        <span class="ellipsis mono">{{ path }}</span>
        <span v-if="dirty" class="dirty">{{ t('sftp.editor.dirty') }}</span>
      </div>
    </template>
    <el-skeleton v-if="loading" :rows="6" animated />
    <el-alert v-else-if="error" type="warning" :closable="false" :title="error" show-icon />
    <textarea
      v-else
      ref="area"
      v-model="text"
      class="area mono"
      spellcheck="false"
      autocapitalize="off"
      autocomplete="off"
      wrap="off"
      @keydown="onKey"
    />
    <template #footer>
      <el-button @click="close">{{ t('common.close') }}</el-button>
      <el-button type="primary" :icon="Save" :loading="saving" :disabled="!!error || loading || !dirty" @click="save">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Save } from 'lucide-vue-next'
import { ApiError } from '../../api'
import { useIsMobile } from '../../composables/useIsMobile'
import { post, readText } from './sftpApi'

const props = defineProps<{ modelValue: boolean; connKey: string; path: string }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean]; saved: [] }>()

const { t } = useI18n()
const isMobile = useIsMobile()
const text = ref('')
const original = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const dirty = computed(() => !error.value && text.value !== original.value)

async function load() {
  loading.value = true
  error.value = ''
  try {
    original.value = await readText(props.connKey, props.path)
    text.value = original.value
  } catch (e) {
    if (e instanceof ApiError && e.status === 413) {
      error.value = t('sftp.editor.tooLarge')
    } else if (e instanceof ApiError && e.status === 415) {
      error.value = t('sftp.editor.binary')
    } else {
      error.value = (e as Error).message
    }
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await post(props.connKey, 'write', { path: props.path, content: text.value })
    original.value = text.value
    ElMessage.success(t('common.saved'))
    emit('saved')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

function onKey(e: KeyboardEvent) {
  // Ctrl/Cmd + S 保存
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    if (dirty.value) {
      save()
    }
    return
  }
  // Tab 键插入制表符而不是切换焦点
  if (e.key === 'Tab' && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
    e.preventDefault()
    const ta = e.target as HTMLTextAreaElement
    const s = ta.selectionStart
    text.value = text.value.slice(0, s) + '\t' + text.value.slice(ta.selectionEnd)
    requestAnimationFrame(() => {
      ta.selectionStart = ta.selectionEnd = s + 1
    })
  }
}

// beforeClose 拦截右上角关闭、Esc 与底部「关闭」按钮：有未保存修改时先确认。
// 对话框自己关闭（右上角 X、Esc）后会发出 update:model-value，上面已转发给父组件，
// 否则父组件的可见状态仍为 true，下次打开文件时赋值不变，对话框就不会再出现。
async function beforeClose(done: () => void) {
  if (!dirty.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('sftp.editor.discardConfirm'), t('sftp.editor.discardTitle'), {
      type: 'warning',
      confirmButtonText: t('sftp.editor.discard'),
      cancelButtonText: t('sftp.editor.keepEditing'),
    })
    done()
  } catch {
    // 用户选择继续编辑
  }
}

function close() {
  beforeClose(() => emit('update:modelValue', false))
}
</script>

<style scoped>
.e-title {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding-right: 24px;
  font-size: 14px;
  font-weight: 600;
}
.dirty {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 400;
  color: var(--el-color-warning);
}
.area {
  display: block;
  width: 100%;
  height: 60vh;
  padding: 10px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius-sm);
  background: var(--el-fill-color-blank);
  color: var(--el-text-color-primary);
  font-size: 13px;
  line-height: 1.55;
  resize: none;
  tab-size: 4;
  outline: none;
  white-space: pre;
  overflow: auto;
}
.area:focus {
  border-color: var(--el-color-primary);
}
@media (max-width: 767.98px) {
  .area {
    height: calc(100dvh - 190px);
    font-size: 14px;
  }
}
</style>
