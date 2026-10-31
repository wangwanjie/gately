<template>
  <div class="g-card conn-card">
    <div class="g-card-head">
      <div class="g-card-title"><Cable :size="16" />{{ t('termConn.list.title') }} <span class="meta">{{ t('termConn.list.count', { count: conns.length }) }}</span></div>
      <div class="head-ops">
        <CompactToggle v-model="compact" size="small" />
        <ListTools :gl="gl" size="small" />
      </div>
    </div>
    <GroupedList :gl="gl" flush :compact="compact" :empty-text="t('termConn.list.empty')" :empty-image-size="60" class="list">
      <template #item="{ it, item: c, sorting, bind }">
        <div v-bind="bind" class="conn" :class="{ disabled: !c.enable }" :title="compact ? c.remark : undefined" @click="onTap(c)" @dblclick="onDblClick(c)">
          <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
          <div class="conn-icon" :class="c.type"><component :is="iconOf(c)" :size="18" /></div>
          <div class="conn-main">
            <div class="conn-name">
              <span class="ellipsis">{{ c.name }}</span>
              <el-tag class="tag-type" size="small" effect="plain">{{ t(`termConn.type.${c.type}`) }}</el-tag>
              <el-tag v-if="!c.enable" class="tag-off" size="small" type="info">{{ t('termConn.list.disabledTag') }}</el-tag>
              <span v-if="openCount(c.key)" class="open-dot" :title="t('termConn.list.openSessions', { count: openCount(c.key) })">{{ openCount(c.key) }}</span>
            </div>
            <div class="conn-addr mono">{{ connAddr(c) }}</div>
            <div v-if="c.remark" class="conn-remark">{{ c.remark }}</div>
          </div>
          <div class="conn-ops" @click.stop>
            <SortMoves v-if="sorting" :gl="gl" :it="it" />
            <template v-else>
              <el-button v-if="!isMobile" size="small" type="primary" plain :icon="Play" :disabled="!c.enable" @click="$emit('connect', c)"><span class="btn-text">{{ t('termConn.list.connect') }}</span></el-button>
              <el-dropdown trigger="click" @command="(cmd: string) => onCommand(c, cmd)">
                <el-button class="op" text :icon="Ellipsis" :aria-label="t('termConn.list.more')" />
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="edit" :icon="Pencil">{{ t('termConn.list.edit') }}</el-dropdown-item>
                    <el-dropdown-item command="duplicate" :icon="Copy">{{ t('termConn.list.duplicate') }}</el-dropdown-item>
                    <el-dropdown-item v-if="c.type !== 'telnet'" command="sftp" :icon="FolderOpen">{{ t('termConn.list.sftp') }}</el-dropdown-item>
                    <el-dropdown-item v-if="c.type !== 'telnet'" command="quick" :icon="Star">{{ t('termConn.list.quick') }}</el-dropdown-item>
                    <el-dropdown-item v-if="c.type !== 'local'" command="test" :icon="Plug">{{ t('termConn.list.test') }}</el-dropdown-item>
                    <el-dropdown-item v-if="c.type === 'ssh'" command="resetkey" :icon="KeyRound">{{ t('termConn.list.resetKey') }}</el-dropdown-item>
                    <el-dropdown-item command="delete" :icon="Trash2" divided>{{ t('common.delete') }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </template>
          </div>
        </div>
      </template>
    </GroupedList>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Cable, Copy, Ellipsis, FolderOpen, KeyRound, Network, Pencil, Play, Plug, Server, Star, Terminal as TerminalIcon, Trash2,
} from 'lucide-vue-next'
import { api } from '../../api'
import CompactToggle from '../CompactToggle.vue'
import GroupedList from '../GroupedList.vue'
import ListTools from '../ListTools.vue'
import SortCheck from '../SortCheck.vue'
import SortMoves from '../SortMoves.vue'
import { useGroupedList } from '../../composables/useGroupSort'
import { useCompactView } from '../../composables/useCompactView'
import { MASK, connAddr, normalizeConn, type Conn } from './types'

const props = defineProps<{ conns: Conn[]; isMobile: boolean; openCounts: Record<string, number> }>()
const emit = defineEmits<{
  connect: [c: Conn]
  edit: [c: Conn | null]
  sftp: [c: Conn]
  quick: [c: Conn]
  reload: []
}>()

const { t } = useI18n()
const compact = useCompactView('terminal.connections')
// 连接列表的分组与排序；顺序与分组的修改不影响已打开的会话
const gl = useGroupedList<Conn>({
  storageKey: 'terminal.connections.groups',
  items: () => props.conns,
  base: '/api/webterminal/connections',
  reload: async () => emit('reload'),
  matches: (c, q) => [c.name, c.group ?? '', c.remark, connAddr(c)].join('\n').toLowerCase().includes(q),
})

function iconOf(c: Conn) {
  if (c.type === 'local') {
    return TerminalIcon
  }
  return c.type === 'telnet' ? Network : Server
}

function openCount(key: string): number {
  return props.openCounts[key] ?? 0
}

// 点击卡片即连接（排序模式下不触发）
function onTap(c: Conn) {
  if (gl.sortMode.value || !c.enable) {
    return
  }
  if (props.isMobile) {
    emit('connect', c)
  }
}

// 桌面端双击卡片连接
function onDblClick(c: Conn) {
  if (gl.sortMode.value || !c.enable || props.isMobile) {
    return
  }
  emit('connect', c)
}

// 复制连接：脱敏的密文不能原样提交，需要清空后让用户重新填写。
function duplicate(c: Conn) {
  const copy = normalizeConn(JSON.parse(JSON.stringify(c)))
  copy.key = ''
  copy.name = t('termConn.list.copySuffix', { name: c.name })
  copy.ssh.hostKey = ''
  const clear = (v: string) => (/^\*+$/.test(v) || v === MASK ? '' : v)
  copy.ssh.password = clear(copy.ssh.password)
  copy.ssh.privateKey = clear(copy.ssh.privateKey)
  copy.ssh.passphrase = clear(copy.ssh.passphrase)
  copy.ssh.proxy.password = clear(copy.ssh.proxy.password)
  copy.telnet.password = clear(copy.telnet.password)
  copy.telnet.proxy.password = clear(copy.telnet.proxy.password)
  emit('edit', copy)
  ElMessage.info(t('termConn.list.copyHint'))
}

async function onCommand(c: Conn, cmd: string) {
  try {
    if (cmd === 'edit') {
      emit('edit', c)
    } else if (cmd === 'duplicate') {
      duplicate(c)
    } else if (cmd === 'sftp') {
      emit('sftp', c)
    } else if (cmd === 'quick') {
      emit('quick', c)
    } else if (cmd === 'test') {
      const loading = ElMessage({ message: t('termConn.list.testing'), duration: 0, type: 'info' })
      try {
        const r = await api.post<{ message?: string } | null>(`/api/webterminal/connections/${encodeURIComponent(c.key)}/test`)
        ElMessage.success(r?.message || t('termConn.list.testOk', { name: c.name }))
      } finally {
        loading.close()
      }
    } else if (cmd === 'resetkey') {
      await ElMessageBox.confirm(t('termConn.list.resetConfirm'), t('termConn.list.resetTitle', { name: c.name }), { type: 'warning' })
      await api.del(`/api/webterminal/connections/${encodeURIComponent(c.key)}/hostkey`)
      ElMessage.success(t('termConn.list.resetDone'))
      emit('reload')
    } else if (cmd === 'delete') {
      await ElMessageBox.confirm(t('termConn.list.deleteConfirm', { name: c.name }), t('termConn.list.confirmTitle'), { type: 'warning' })
      await api.del(`/api/webterminal/connections/${encodeURIComponent(c.key)}`)
      emit('reload')
    }
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}
</script>

<style scoped>
.conn-card {
  overflow: auto;
}
.head-ops {
  display: flex;
  gap: 6px;
}
.conn {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--g-border);
  cursor: default;
}
.list :deep(.gl-block:last-child .gl-item:last-child) {
  border-bottom: none;
}
.conn.disabled {
  opacity: 0.55;
}
.conn-icon {
  width: 36px;
  height: 36px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  flex-shrink: 0;
}
.conn-icon.telnet {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning);
}
.conn-icon.local {
  background: var(--el-color-info-light-9);
  color: var(--el-color-info);
}
.conn-main {
  flex: 1;
  min-width: 0;
}
.conn-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
}
.conn-addr {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.conn-remark {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.open-dot {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--el-color-success);
  color: #fff;
  font-size: 11px;
  line-height: 18px;
  text-align: center;
}
.conn-ops {
  display: flex;
  gap: 2px;
  align-items: center;
}
.op {
  padding: 8px;
}
/* 紧凑视图：图标缩小，名称与地址同行，备注收进悬停提示（仅桌面宽度） */
@media (min-width: 768px) {
  .is-compact .conn {
    gap: 8px;
    padding: 4px 10px;
  }
  .is-compact .conn-icon {
    width: 26px;
    height: 26px;
    border-radius: 7px;
  }
  .is-compact .conn-icon :deep(svg) {
    width: 15px;
    height: 15px;
  }
  .is-compact .conn-main {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .is-compact .conn-name {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 60%;
    font-size: 13px;
  }
  /* 侧栏较窄：类型由图标颜色区分，标签和按钮文字收起，把空间留给名称与地址 */
  .is-compact .conn-name {
    flex: 1 1 0;
    max-width: none;
  }
  .is-compact .tag-type,
  .is-compact .tag-off,
  .is-compact .btn-text {
    display: none;
  }
  .is-compact .conn-ops :deep(.el-button--small) {
    padding: 5px 8px;
  }
  .is-compact .conn-ops :deep(.el-button--small .el-icon) {
    margin-right: 0;
  }
  .is-compact .conn-addr {
    flex: 0 1 38%;
    min-width: 0;
    margin-top: 0;
    text-align: right;
  }
  .is-compact .conn-remark {
    display: none;
  }
}
@media (max-width: 767.98px) {
  .conn {
    padding: 12px;
    cursor: pointer;
    -webkit-tap-highlight-color: transparent;
  }
  .conn:active {
    background: var(--el-fill-color-light);
  }
  .op {
    min-width: 40px;
    height: 40px;
    padding: 0;
  }
}
</style>
