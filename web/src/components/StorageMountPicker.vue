<template>
  <!-- 从「存储挂载」中选取本地目录填入路径输入框：放在输入框的 suffix 插槽里；读不到挂载点（模块关闭或无权限）时不显示 -->
  <el-dropdown v-if="state.available" trigger="click" placement="bottom-end" :teleported="true" @command="onPick" @visible-change="onOpen">
    <button type="button" class="smp-trigger" :title="t('fileUsers.pickStorage')" :aria-label="t('fileUsers.pickStorage')" @click.stop>
      <HardDrive :size="14" />
    </button>
    <template #dropdown>
      <el-dropdown-menu class="smp-menu">
        <div class="smp-title">{{ t('fileUsers.pickStorage') }}</div>
        <el-dropdown-item v-for="m in state.mounts" :key="m.key" :command="m">
          <div class="smp-item">
            <span class="smp-name">{{ m.name }}<span v-if="m.readOnly" class="smp-ro">{{ t('fileUsers.readOnly') }}</span></span>
            <code class="smp-path mono">{{ m.path }}</code>
          </div>
        </el-dropdown-item>
        <div v-if="!state.mounts.length" class="smp-empty">
          {{ t('fileUsers.pickStorageEmpty') }}
          <router-link to="/storage">{{ t('fileUsers.pickStorageAdd') }}</router-link>
        </div>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script lang="ts">
import { reactive } from 'vue'
import { api } from '../api'

// LocalMount 是可以选作文件服务目录的存储挂载点：只有本地目录（local）有本机路径，远程 WebDAV 与 SMB 挂载不能作为共享目录
export interface LocalMount {
  key: string
  name: string
  path: string
  readOnly: boolean
}

interface StorageMount extends LocalMount {
  type: string
  enable: boolean
}

// 同一页面上多个选择器共用一份挂载点列表，首次显示时读取，展开下拉时刷新；
// available 在读取成功后才为 true，避免模块关闭时按钮先出现再消失
const state = reactive<{ available: boolean; loaded: boolean; mounts: LocalMount[] }>({ available: false, loaded: false, mounts: [] })
let loading: Promise<void> | null = null

function load(): Promise<void> {
  loading ??= api
    .get<StorageMount[] | null>('/api/storage/mounts')
    .then((list) => {
      state.mounts = (list ?? []).filter((m) => m.type === 'local' && m.enable && m.path).map((m) => ({ key: m.key, name: m.name, path: m.path, readOnly: !!m.readOnly }))
      state.available = true
    })
    .catch(() => {
      // 存储模块已关闭、账号无权读取等情况下隐藏选择按钮
      state.available = false
    })
    .finally(() => {
      state.loaded = true
      loading = null
    })
  return loading
}
</script>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HardDrive } from 'lucide-vue-next'

const emit = defineEmits<{ pick: [mount: LocalMount] }>()
const { t } = useI18n()

onMounted(() => {
  if (!state.loaded) {
    void load()
  }
})

function onOpen(visible: boolean) {
  if (visible) {
    void load()
  }
}

function onPick(m: LocalMount) {
  emit('pick', m)
}
</script>

<style scoped>
.smp-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--el-text-color-secondary);
  cursor: pointer;
}
.smp-trigger:hover,
.smp-trigger:focus-visible {
  color: var(--el-color-primary);
  background: var(--el-fill-color-light);
  outline: none;
}
</style>

<style>
/* 下拉菜单挂在 body 下，样式不能用 scoped */
.smp-menu {
  max-width: min(420px, calc(100vw - 24px));
  max-height: 320px;
  overflow-y: auto;
}
.smp-menu .smp-title {
  padding: 4px 16px 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.smp-menu .smp-item {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.4;
  padding: 2px 0;
}
.smp-menu .el-dropdown-menu__item {
  max-width: 100%;
}
.smp-menu .smp-name {
  font-size: 13px;
  color: var(--el-text-color-primary);
}
.smp-menu .smp-ro {
  margin-left: 6px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.smp-menu .smp-path {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 360px;
}
.smp-menu .smp-empty {
  padding: 6px 16px 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.smp-menu .smp-empty a {
  margin-left: 4px;
  color: var(--el-color-primary);
}
</style>
