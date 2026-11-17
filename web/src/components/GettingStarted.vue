<template>
  <!-- 总览顶部的「开始使用」清单：按首次启动选择的使用场景生成，各项根据实际配置自动判断是否完成；可收起或关闭 -->
  <section v-if="show" class="gs" :class="{ collapsed }" data-testid="getting-started">
    <header class="gs-head">
      <Rocket :size="18" class="gs-icon" />
      <div class="gs-title-wrap">
        <div class="gs-title">{{ t('onboarding.card.title') }}</div>
        <div class="gs-sub">{{ allDone ? t('onboarding.card.allDone') : t('onboarding.card.progress', { done: doneCount, total: items.length }) }}</div>
      </div>
      <el-progress class="gs-bar" :percentage="percent" :show-text="false" :stroke-width="6" :status="allDone ? 'success' : undefined" />
      <el-button text size="small" :icon="collapsed ? ChevronDown : ChevronUp" :aria-label="collapsed ? t('common.expand') : t('common.collapse')" @click="setCollapsed(!collapsed)" />
      <el-popconfirm :title="t('onboarding.card.closeConfirm')" width="260" :confirm-button-text="t('onboarding.card.close')" @confirm="close">
        <template #reference>
          <el-button text size="small" :icon="X" :aria-label="t('onboarding.card.close')" />
        </template>
      </el-popconfirm>
    </header>
    <ul v-show="!collapsed" class="gs-list">
      <li v-for="it in items" :key="it.id" :class="{ done: it.done }" :data-item="it.id">
        <span class="gs-state" :aria-label="it.done ? t('onboarding.card.doneLabel') : t('onboarding.card.todoLabel')">
          <CircleCheck v-if="it.done" :size="20" />
          <component :is="it.icon" v-else :size="18" />
        </span>
        <div class="gs-text">
          <div class="gs-item-title">{{ t(`onboarding.items.${it.id}.title`) }}</div>
          <div class="gs-item-desc">{{ t(`onboarding.items.${it.id}.desc`) }}</div>
        </div>
        <el-button v-if="!it.done" size="small" :type="it.id === firstTodo ? 'primary' : 'default'" @click="run(it.id)">{{ t(`onboarding.items.${it.id}.action`) }}</el-button>
        <el-button v-else-if="it.id === 'site' || it.id === 'import'" size="small" text @click="run(it.id)">{{ t('onboarding.card.again') }}</el-button>
      </li>
    </ul>
  </section>
  <el-dialog v-model="importVisible" :title="t('onboarding.import.title')" width="min(480px, 94vw)" append-to-body>
    <div class="imp-desc">{{ t('onboarding.import.desc') }}</div>
    <ul class="imp-list">
      <li v-for="s in importSources" :key="s.id">
        <div class="imp-text">
          <div class="imp-name">{{ s.name }}</div>
          <div class="imp-hint">{{ t(`onboarding.import.${s.id}`) }}</div>
        </div>
        <el-button size="small" :disabled="!s.enabled" @click="openImport(s.path)">{{ t('onboarding.import.open') }}</el-button>
      </li>
    </ul>
  </el-dialog>
  <SelfCheck ref="selfCheck" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
  ArrowLeftRight,
  Bell,
  ChevronDown,
  ChevronUp,
  CircleCheck,
  DatabaseBackup,
  Fingerprint,
  FolderSync,
  Globe,
  Lock,
  Network,
  Radar,
  Rocket,
  Server,
  Split,
  Stethoscope,
  Users,
  Waypoints,
  X,
} from 'lucide-vue-next'
import { api } from '../api'
import SelfCheck from './SelfCheck.vue'
import { moduleEnabled } from '../nav'
import { checklistFor, countEnabled, countSites, emptyFacts, itemDone, type ChecklistFacts, type ChecklistId } from '../onboarding'
import { dismissOnboarding, markOnboardingDone, onboardingState, refreshOnboarding } from '../composables/useOnboarding'

const { t } = useI18n()
const router = useRouter()
const selfCheck = ref<InstanceType<typeof SelfCheck>>()
const facts = ref<ChecklistFacts>(emptyFacts())
const importVisible = ref(false)

const COLLAPSE_KEY = 'gately.gettingStarted.collapsed'
const collapsed = ref(localStorage.getItem(COLLAPSE_KEY) === '1')
function setCollapsed(v: boolean) {
  collapsed.value = v
  localStorage.setItem(COLLAPSE_KEY, v ? '1' : '0')
}

const icons: Record<ChecklistId, Component> = {
  site: Server,
  https: Lock,
  ddns: Globe,
  portforward: ArrowLeftRight,
  stun: Radar,
  mesh: Network,
  fileuser: Users,
  fileservice: FolderSync,
  protoroute: Split,
  frps: Waypoints,
  security: Fingerprint,
  check: Stethoscope,
  import: DatabaseBackup,
  notify: Bell,
}

// 场景选择完成后才显示清单（场景选择对话框打开时不显示）
const show = computed(() => !!onboardingState.value?.pending && !onboardingState.value.scenePending)
const ids = computed(() => checklistFor(onboardingState.value?.scenes ?? [], moduleEnabled))
const items = computed(() => ids.value.map((id) => ({ id, icon: icons[id], done: itemDone(id, { ...facts.value, done: onboardingState.value?.done ?? [] }) })))
const doneCount = computed(() => items.value.filter((i) => i.done).length)
const allDone = computed(() => items.value.length > 0 && doneCount.value === items.value.length)
const percent = computed(() => (items.value.length ? Math.round((doneCount.value * 100) / items.value.length) : 0))
const firstTodo = computed(() => items.value.find((i) => !i.done)?.id ?? '')

// 第一个已开启的模块页面，组网与文件服务按开启的模块跳转
function firstEnabled(list: [string, string][]): string {
  return list.find(([mod]) => moduleEnabled(mod))?.[1] ?? list[0][1]
}

const importSources = computed(() => [
  { id: 'nginx', name: 'nginx', path: '/web?import=nginx', enabled: moduleEnabled('webservice') },
  { id: 'npm', name: 'Nginx Proxy Manager', path: '/web?import=nginx', enabled: moduleEnabled('webservice') },
  { id: 'frp', name: 'frp', path: '/frp?import=1', enabled: moduleEnabled('frp') },
  { id: 'lucky', name: 'Lucky', path: '/settings?tab=backup', enabled: true },
])

function openImport(path: string) {
  importVisible.value = false
  void markOnboardingDone('import')
  router.push(path)
}

function run(id: ChecklistId) {
  switch (id) {
    case 'site':
    case 'https':
      router.push({ path: '/web', query: { quick: id === 'https' ? 'https' : '1' } })
      break
    case 'check':
      // 运行一次自检后由 SelfCheck 标记完成
      selfCheck.value?.open()
      break
    case 'import':
      importVisible.value = true
      break
    case 'security':
      router.push({ path: '/settings', query: { tab: 'account' } })
      break
    case 'mesh':
      router.push(firstEnabled([['tailscale', '/tailscale'], ['easytier', '/easytier'], ['netbird', '/netbird']]))
      break
    case 'fileuser':
      router.push('/fileusers')
      break
    case 'fileservice':
      router.push(firstEnabled([['webdav', '/webdav'], ['smb', '/smb'], ['ftp', '/ftp'], ['filebrowser', '/filebrowser']]))
      break
    case 'frps':
      router.push('/frp')
      break
    default:
      router.push(`/${id}`)
  }
}

async function close() {
  try {
    await dismissOnboarding()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// 只读取当前清单需要的接口；模块关闭或读取失败的项目按未完成处理
async function loadFacts() {
  const need = new Set(ids.value)
  const f = emptyFacts()
  const jobs: Promise<unknown>[] = []
  const get = <T,>(url: string, apply: (v: T) => void) => jobs.push(api.get<T>(url).then(apply).catch(() => undefined))
  type Listeners = Parameters<typeof countSites>[0]
  if ((need.has('site') || need.has('https')) && moduleEnabled('webservice')) {
    get<Listeners>('/api/webservice/listeners', (v) => {
      const c = countSites(v)
      f.sites = c.sites
      f.httpsSites = c.https
    })
  }
  const lists: [ChecklistId, string, string, keyof ChecklistFacts][] = [
    ['ddns', 'ddns', '/api/ddns/tasks', 'ddnsTasks'],
    ['portforward', 'portforward', '/api/portforward/rules', 'portforwardRules'],
    ['stun', 'stun', '/api/stun/rules', 'stunRules'],
    ['protoroute', 'protoroute', '/api/protoroute/rules', 'protorouteRules'],
    ['frps', 'frp', '/api/frp/servers', 'frpServers'],
    ['notify', 'notify', '/api/notify/channels', 'notifyChannels'],
  ]
  for (const [item, mod, url, key] of lists) {
    if (need.has(item) && moduleEnabled(mod)) {
      get<{ enable?: boolean }[]>(url, (v) => {
        ;(f[key] as number) = countEnabled(v)
      })
    }
  }
  if (need.has('mesh')) {
    for (const mod of ['tailscale', 'netbird', 'easytier']) {
      if (moduleEnabled(mod)) {
        get<{ enable?: boolean }[]>(`/api/${mod}/instances`, (v) => (f.meshInstances += countEnabled(v)))
      }
    }
  }
  if (need.has('fileuser')) {
    get<{ users?: unknown[] | null }>('/api/fileusers', (v) => (f.fileUsers = v.users?.length ?? 0))
  }
  if (need.has('fileservice')) {
    for (const mod of ['webdav', 'smb', 'ftp']) {
      if (moduleEnabled(mod)) {
        get<{ enable?: boolean }>(`/api/${mod}/settings`, (v) => (f.fileServices += v.enable ? 1 : 0))
      }
    }
    if (moduleEnabled('filebrowser')) {
      get<{ enable?: boolean }[]>('/api/filebrowser/instances', (v) => (f.fileServices += countEnabled(v)))
    }
  }
  if (need.has('security')) {
    get<{ enabled: boolean }>('/api/account/totp', (v) => (f.totp = v.enabled))
    get<unknown[] | null>('/api/account/passkeys', (v) => (f.passkeys = v?.length ?? 0))
  }
  await Promise.all(jobs)
  facts.value = f
}

watch(show, (v) => {
  if (v) {
    void loadFacts()
  }
})

onMounted(async () => {
  if (!onboardingState.value) {
    await refreshOnboarding()
  }
  if (show.value) {
    await loadFacts()
  }
})
</script>

<style scoped>
.gs {
  margin-bottom: 16px;
  padding: 14px 16px;
  border: 1px solid var(--el-color-primary-light-7);
  border-radius: 12px;
  background: linear-gradient(135deg, var(--el-color-primary-light-9), var(--el-bg-color) 70%);
}
.gs-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.gs-icon {
  flex-shrink: 0;
  color: var(--el-color-primary);
}
.gs-title-wrap {
  flex: 1;
  min-width: 0;
}
.gs-title {
  font-size: 15px;
  font-weight: 600;
}
.gs-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.gs-bar {
  width: 140px;
  flex-shrink: 0;
}
.gs-list {
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 20px;
}
.gs-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 2px;
  border-top: 1px solid var(--el-border-color-extra-light);
}
.gs-state {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border-radius: 8px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.gs-list li.done .gs-state {
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
}
.gs-text {
  flex: 1;
  min-width: 0;
}
.gs-item-title {
  font-size: 14px;
  font-weight: 500;
}
.gs-list li.done .gs-item-title {
  color: var(--el-text-color-secondary);
  text-decoration: line-through;
}
.gs-item-desc {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.imp-desc {
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.imp-list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.imp-list li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}
.imp-list li:last-child {
  border-bottom: 0;
}
.imp-text {
  flex: 1;
  min-width: 0;
}
.imp-name {
  font-weight: 500;
}
.imp-hint {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
@media (max-width: 900px) {
  .gs-list {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 640px) {
  .gs {
    padding: 12px;
  }
  .gs-bar {
    display: none;
  }
}
</style>
