<template>
  <!-- 批量修改中：修改已保存但暂缓生效，旧的访问路径继续可用；改好后一起生效或放弃 -->
  <div v-if="hold" class="hold-banner" data-test="hold-banner">
    <Layers :size="18" class="icon" />
    <div class="text">
      <div class="title">{{ t('changeset.bannerTitle') }}</div>
      <div class="desc">{{ hold.items.length ? t('changeset.bannerDesc', { n: hold.items.length }) : t('changeset.bannerEmpty') }}</div>
      <ul v-if="hold.items.length" class="items">
        <li v-for="it in hold.items" :key="it.module + it.key">{{ itemLabel(it) }}</li>
      </ul>
    </div>
    <div class="actions">
      <el-button size="small" :loading="busy === 'discard'" :disabled="!!busy" data-test="hold-discard" @click="discard">{{ t('changeset.discard') }}</el-button>
      <el-button size="small" type="primary" :loading="busy === 'apply'" :disabled="!!busy" data-test="hold-apply" @click="apply">{{ t('changeset.apply') }}</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Layers } from 'lucide-vue-next'
import { applyHold, checkHold, confirmState, discardHold, type HoldItem } from '../configConfirm'

const { t } = useI18n()
const hold = computed(() => confirmState.hold)
const busy = ref<'' | 'apply' | 'discard'>('')

const moduleKeys: Record<string, string> = {
  portforward: 'nav.items.portforward.title',
  protoroute: 'nav.items.protoroute.title',
  webservice: 'nav.items.web.title',
}

function itemLabel(it: HoldItem): string {
  const mod = moduleKeys[it.module] ? t(moduleKeys[it.module]) : it.module
  return t('changeset.item', { module: mod, name: it.name || t('changeset.deleted', { key: it.key }) })
}

async function apply() {
  busy.value = 'apply'
  // 先检查暂存的监听地址能否在本机使用：不能用的生效后会启动失败，让管理员知道后再决定
  try {
    const problems = await checkHold()
    if (problems.length > 0) {
      try {
        await ElMessageBox.confirm(
          h('div', [
            h('p', t('changeset.checkDesc')),
            h('ul', { class: 'hold-problems' }, problems.map((p) => h('li', t('changeset.checkItem', { item: itemLabel(p), addr: p.addr, error: p.error })))),
          ]),
          t('changeset.checkTitle'),
          { type: 'warning', confirmButtonText: t('changeset.checkContinue'), cancelButtonText: t('common.cancel') },
        )
      } catch {
        busy.value = ''
        return
      }
    }
  } catch {
    // 检查失败不拦生效，确认与回滚照常保护
  }
  try {
    const n = await applyHold()
    ElMessage.success(t('changeset.applied', { n }))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = ''
  }
}

async function discard() {
  try {
    await ElMessageBox.confirm(t('changeset.discardConfirm'), t('changeset.discard'), {
      type: 'warning',
      confirmButtonText: t('changeset.discard'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  busy.value = 'discard'
  try {
    await discardHold()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = ''
  }
}
</script>

<style scoped>
.hold-banner {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 16px;
  padding: 12px 16px;
  border-radius: 10px;
  border: 1px solid var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
}
.icon {
  flex: none;
  margin-top: 2px;
  color: var(--el-color-primary);
}
.text {
  flex: 1;
  min-width: 0;
}
.title {
  font-weight: 600;
}
.desc {
  margin-top: 2px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
}
.items {
  margin: 6px 0 0;
  padding-left: 18px;
  font-size: 13px;
  line-height: 1.7;
}
.actions {
  flex: none;
  display: flex;
  gap: 8px;
}
@media (max-width: 640px) {
  .hold-banner {
    flex-wrap: wrap;
  }
  .actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
