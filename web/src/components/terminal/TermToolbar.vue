<template>
  <div class="toolbar">
    <template v-if="!isMobile">
      <el-tooltip :content="t('terminal.toolbar.sftp')" placement="bottom"><el-button class="tb" text :icon="FolderOpen" :disabled="!sftpEnabled" @click="$emit('action', 'sftp')" /></el-tooltip>
      <el-tooltip :content="t('terminal.toolbar.search')" placement="bottom"><el-button class="tb" text :icon="Search" @click="$emit('action', 'search')" /></el-tooltip>
      <el-tooltip :content="t('terminal.toolbar.copySelection')" placement="bottom"><el-button class="tb" text :icon="Copy" @click="$emit('action', 'copy')" /></el-tooltip>
      <el-tooltip :content="t('terminal.toolbar.paste')" placement="bottom"><el-button class="tb" text :icon="ClipboardPaste" @click="$emit('action', 'paste')" /></el-tooltip>
      <span class="sep" />
      <el-tooltip :content="t('terminal.toolbar.fontDec')" placement="bottom"><el-button class="tb" text :icon="Minus" @click="$emit('action', 'font-dec')" /></el-tooltip>
      <span class="font-size">{{ fontSize }}</span>
      <el-tooltip :content="t('terminal.toolbar.fontInc')" placement="bottom"><el-button class="tb" text :icon="Plus" @click="$emit('action', 'font-inc')" /></el-tooltip>
      <el-dropdown trigger="click" @command="(n: string) => $emit('update:themeName', n)">
        <el-button class="tb" text :icon="Palette" :title="t('terminal.toolbar.theme')" />
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="th in termThemes" :key="th.name" :command="th.name" :class="{ 'is-current': th.name === themeName }">
              <span class="swatch" :style="{ background: th.theme.background, borderColor: th.theme.foreground }" />{{ themeLabel(th) }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span class="sep" />
      <el-dropdown trigger="click" @command="(s: Shortcut) => $emit('shortcut', s)">
        <el-button class="tb wide" text :icon="Zap">{{ t('terminal.toolbar.shortcuts') }}</el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="(s, i) in shortcuts" :key="i" :command="s">
              {{ s.name }}<span class="sc-cmd mono">{{ s.command }}</span>
            </el-dropdown-item>
            <el-dropdown-item v-if="!shortcuts.length" disabled>{{ t('terminal.toolbar.noShortcuts') }}</el-dropdown-item>
            <el-dropdown-item divided :icon="Settings2" @click="$emit('action', 'shortcuts-manage')">{{ t('terminal.toolbar.manageShortcuts') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-dropdown trigger="click" @command="(c: string) => $emit('action', c)">
        <el-button class="tb wide" text :icon="Columns2">{{ splitMode ? t('terminal.toolbar.splitting') : t('terminal.toolbar.split') }}</el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="split-h" :icon="Columns2">{{ t('terminal.toolbar.splitH') }}</el-dropdown-item>
            <el-dropdown-item command="split-v" :icon="Rows2">{{ t('terminal.toolbar.splitV') }}</el-dropdown-item>
            <el-dropdown-item command="split-grid" :icon="LayoutGrid">{{ t('terminal.toolbar.splitGrid') }}</el-dropdown-item>
            <el-dropdown-item command="split-off" :icon="Square" :disabled="!splitMode" divided>{{ t('terminal.toolbar.splitOff') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span class="grow" />
      <el-tooltip :content="t('terminal.toolbar.clear')" placement="bottom"><el-button class="tb" text :icon="Eraser" @click="$emit('action', 'clear')" /></el-tooltip>
      <el-tooltip :content="t('terminal.toolbar.sessions')" placement="bottom"><el-button class="tb" text :icon="ListTree" @click="$emit('action', 'sessions')" /></el-tooltip>
      <el-tooltip :content="fullscreen ? t('terminal.toolbar.exitFullscreen') : t('terminal.toolbar.fullscreen')" placement="bottom">
        <el-button class="tb" text :icon="fullscreen ? Minimize2 : Maximize2" @click="$emit('action', 'fullscreen')" />
      </el-tooltip>
    </template>

    <template v-else>
      <el-button class="tb" text :icon="FolderOpen" :disabled="!sftpEnabled" :aria-label="t('terminal.toolbar.fileManage')" @click="$emit('action', 'sftp')" />
      <el-button class="tb" text :icon="Search" :aria-label="t('terminal.toolbar.search')" @click="$emit('action', 'search')" />
      <el-dropdown trigger="click" placement="bottom-end" @command="onMore">
        <el-button class="tb" text :icon="Ellipsis" :aria-label="t('common.more')" />
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="copy" :icon="Copy">{{ t('terminal.toolbar.copyMobile') }}</el-dropdown-item>
            <el-dropdown-item command="paste" :icon="ClipboardPaste">{{ t('terminal.toolbar.paste') }}</el-dropdown-item>
            <el-dropdown-item command="font-inc" :icon="Plus">{{ t('terminal.toolbar.fontIncWithSize', { size: fontSize }) }}</el-dropdown-item>
            <el-dropdown-item command="font-dec" :icon="Minus">{{ t('terminal.toolbar.fontDec') }}</el-dropdown-item>
            <el-dropdown-item command="clear" :icon="Eraser">{{ t('terminal.toolbar.clear') }}</el-dropdown-item>
            <el-dropdown-item command="sessions" :icon="ListTree">{{ t('terminal.toolbar.sessions') }}</el-dropdown-item>
            <el-dropdown-item command="shortcuts-manage" :icon="Settings2">{{ t('terminal.toolbar.manageShortcuts') }}</el-dropdown-item>
            <el-dropdown-item command="keybar-pin" :icon="keybarPinned ? SquareCheck : Square">{{ t('terminal.toolbar.keybarPinned') }}</el-dropdown-item>
            <el-dropdown-item
              v-for="th in termThemes"
              :key="th.name"
              :command="'theme:' + th.name"
              :divided="th.name === termThemes[0].name"
              :class="{ 'is-current': th.name === themeName }"
            >
              <span class="swatch" :style="{ background: th.theme.background, borderColor: th.theme.foreground }" />{{ t('terminal.toolbar.themeItem', { name: themeLabel(th) }) }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </template>
  </div>
</template>

<script setup lang="ts">
import {
  ClipboardPaste, Columns2, Copy, Ellipsis, Eraser, FolderOpen, LayoutGrid, ListTree, Maximize2, Minimize2, Minus, Palette, Plus, Rows2, Search, Settings2, Square, SquareCheck, Zap,
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { termThemes, themeLabel } from './themes'
import type { Shortcut } from './types'

const { t } = useI18n()

defineProps<{
  isMobile: boolean
  sftpEnabled: boolean
  fontSize: number
  themeName: string
  shortcuts: Shortcut[]
  splitMode: boolean
  fullscreen: boolean
  keybarPinned?: boolean
}>()
const emit = defineEmits<{
  action: [name: string]
  'update:themeName': [name: string]
  shortcut: [s: Shortcut]
}>()

function onMore(cmd: string) {
  if (cmd.startsWith('theme:')) {
    emit('update:themeName', cmd.slice(6))
    return
  }
  emit('action', cmd)
}
</script>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}
.toolbar :deep(.el-button + .el-button) {
  margin-left: 0;
}
.tb {
  padding: 8px;
  min-width: 32px;
}
.tb.wide {
  padding: 8px 10px;
}
.sep {
  width: 1px;
  height: 18px;
  margin: 0 4px;
  background: var(--g-border);
}
.grow {
  flex: 1;
}
.font-size {
  min-width: 22px;
  text-align: center;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.swatch {
  display: inline-block;
  width: 14px;
  height: 14px;
  margin-right: 8px;
  border: 1px solid;
  border-radius: 4px;
  vertical-align: -2px;
}
.sc-cmd {
  margin-left: 12px;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.is-current {
  color: var(--el-color-primary);
}
@media (max-width: 767.98px) {
  .tb {
    min-width: 40px;
    height: 40px;
    padding: 0;
  }
}
</style>
