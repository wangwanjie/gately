<template>
  <!-- 全新安装改完密码后的第一步：选择使用场景，只保留相关的功能模块；「开始使用」清单在总览顶部（GettingStarted.vue） -->
  <el-dialog
    v-model="visible"
    width="min(680px, 94vw)"
    align-center
    append-to-body
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
    class="scene-dialog"
  >
    <template #header>
      <div class="head">
        <Rocket :size="20" class="head-icon" />
        <div>
          <div class="title">{{ t('onboarding.scene.title') }}</div>
          <div class="sub">{{ t('onboarding.scene.subtitle') }}</div>
        </div>
      </div>
    </template>
    <div class="scenes" role="group" :aria-label="t('onboarding.scene.title')">
      <button
        v-for="s in scenes"
        :key="s.id"
        type="button"
        class="scene"
        :class="{ on: picked.includes(s.id) }"
        :aria-pressed="picked.includes(s.id)"
        :data-scene="s.id"
        @click="toggle(s.id)"
      >
        <span class="sc-icon"><component :is="s.icon" :size="20" /></span>
        <span class="sc-text">
          <span class="sc-title">{{ t(`onboarding.scene.items.${s.id}.title`) }}</span>
          <span class="sc-desc">{{ t(`onboarding.scene.items.${s.id}.desc`) }}</span>
        </span>
        <span class="sc-check"><Check v-if="picked.includes(s.id)" :size="14" :stroke-width="3" /></span>
      </button>
    </div>
    <div class="hint">{{ hint }}</div>
    <template #footer>
      <el-button :disabled="saving" @click="skip">{{ t('onboarding.scene.skip') }}</el-button>
      <el-button type="primary" :disabled="!picked.length" :loading="saving" @click="confirm">{{ t('onboarding.scene.confirm') }}</el-button>
    </template>
  </el-dialog>
  <RestartOverlay ref="overlay" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { ArrowLeftRight, Check, FolderSync, Globe, LayoutGrid, Network, Rocket, Server, Waypoints } from 'lucide-vue-next'
import { api, type SystemSettings } from '../api'
import RestartOverlay from './RestartOverlay.vue'
import { absentModules } from '../nav'
import { SCENES, disabledModulesFor, enabledModulesFor, type SceneId } from '../onboarding'
import { onboardingState, refreshOnboarding } from '../composables/useOnboarding'

const { t } = useI18n()
const visible = ref(false)
const saving = ref(false)
const picked = ref<SceneId[]>([])
const overlay = ref<InstanceType<typeof RestartOverlay>>()
// 设置接口返回的全部模块（含依赖），打开对话框时读取，用于提示将保留的功能数量
const modules = ref<{ id: string; requires?: string[] }[]>([])

const icons: Record<SceneId, Component> = {
  web: Server,
  forward: ArrowLeftRight,
  ddns: Globe,
  mesh: Network,
  files: FolderSync,
  vps: Waypoints,
  all: LayoutGrid,
}
const scenes = SCENES.map((s) => ({ id: s.id, icon: icons[s.id] }))

// 「全部功能」与其他场景互斥
function toggle(id: SceneId) {
  if (picked.value.includes(id)) {
    picked.value = picked.value.filter((x) => x !== id)
  } else if (id === 'all') {
    picked.value = ['all']
  } else {
    picked.value = [...picked.value.filter((x) => x !== 'all'), id]
  }
}

const hint = computed(() => {
  const on = enabledModulesFor(picked.value, modules.value)
  if (!picked.value.length || !on) {
    return t('onboarding.scene.hintAll')
  }
  return t('onboarding.scene.hintSome', { count: on.size, total: modules.value.length })
})

// saveScenes 记录场景并结束场景选择；失败时提示，不影响已保存的模块开关
async function saveScenes(list: string[]) {
  await api.post('/api/system/onboarding/scenes', { scenes: list })
  await refreshOnboarding()
}

async function skip() {
  saving.value = true
  try {
    await saveScenes([])
    visible.value = false
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// confirm 按所选场景保存模块开关（复用全局设置接口），记录场景，有变化时重启生效
async function confirm() {
  saving.value = true
  try {
    const s = await api.get<SystemSettings>('/api/system/settings')
    const all = (s.modules ?? []).map((m) => ({ id: m.id, requires: m.requires }))
    // 设置了管理界面 HTTPS 时证书模块不能关闭
    const keep = s.httpsListen ? ['ssl'] : []
    const off = disabledModulesFor(picked.value, all, keep, s.disabledModules ?? [], absentModules.value)
    const changed = JSON.stringify(off) !== JSON.stringify([...(s.disabledModules ?? [])].sort())
    let restart = false
    if (changed) {
      const r = await api.put<SystemSettings>('/api/system/settings', { ...s, disabledModules: off })
      restart = r.restartRequired
    }
    await saveScenes(picked.value)
    visible.value = false
    if (restart) {
      await api.post('/api/system/restart')
      overlay.value?.start(t('onboarding.scene.restarting'))
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await refreshOnboarding()
  if (!onboardingState.value?.scenePending) {
    return
  }
  // 默认不选：跳过即保持全部功能开启
  picked.value = []
  visible.value = true
  try {
    const s = await api.get<SystemSettings>('/api/system/settings')
    modules.value = (s.modules ?? []).filter((m) => !absentModules.value.includes(m.id)).map((m) => ({ id: m.id, requires: m.requires }))
  } catch {
    // 只影响提示文字
  }
})
</script>

<style scoped>
.head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.head-icon {
  color: var(--el-color-primary);
  flex-shrink: 0;
}
.title {
  font-size: 17px;
  font-weight: 600;
}
.sub {
  margin-top: 2px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.scenes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.scene {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px;
  text-align: left;
  font: inherit;
  color: inherit;
  cursor: pointer;
  border: 1px solid var(--el-border-color);
  border-radius: 10px;
  background: var(--el-bg-color);
  transition:
    border-color 0.15s,
    background 0.15s;
}
.scene:hover {
  border-color: var(--el-color-primary-light-5);
}
.scene:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
}
.scene.on {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.sc-icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  border-radius: 9px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.scene.on .sc-icon {
  color: #fff;
  background: var(--el-color-primary);
}
.sc-text {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.sc-title {
  font-size: 14px;
  font-weight: 600;
}
.sc-desc {
  margin-top: 3px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.sc-check {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  border-radius: 50%;
  border: 1px solid var(--el-border-color);
  color: #fff;
}
.scene.on .sc-check {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary);
}
.hint {
  margin-top: 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
@media (max-width: 640px) {
  .scenes {
    grid-template-columns: 1fr;
  }
  .scene {
    padding: 10px 12px;
  }
}
</style>
