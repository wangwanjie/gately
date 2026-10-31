<template>
  <div>
    <PageHeader :title="t('webUsers.title')" :desc="t('webUsers.desc')" :icon="UserCog" />

    <el-alert v-if="loadError" type="error" :title="loadError" :closable="false" show-icon class="block-gap" />

    <template v-if="form">
      <div class="g-card">
        <div class="g-card-head">
          <div class="g-card-title"><UsersRound :size="16" />{{ t('web.sharedUsers') }} <span class="g-card-desc">{{ t('webUsers.usersDesc') }}</span></div>
        </div>
        <div class="g-card-body">
          <div class="notice">{{ t('web.sharedUsersHelp') }}</div>
          <SharedUsers v-model="form.authUsers" />
          <el-form :model="form" label-width="130px" :label-position="labelPosition" class="sub-form" @submit.prevent>
            <el-form-item :label="t('web.ssoDomain')">
              <el-input v-model="form.ssoDomain" class="w-260" placeholder="example.com" />
              <div class="form-help">{{ t('web.ssoDomainHelp') }}</div>
            </el-form-item>
            <el-form-item :label="t('web.selfServicePassword')">
              <el-switch v-model="form.selfServicePassword" />
              <div class="form-help">{{ t('web.selfServicePasswordHelp', { site: '/__gately_auth/password', oidc: '/__gately_oidc/password' }) }}</div>
            </el-form-item>
          </el-form>
        </div>
      </div>

      <div class="g-card">
        <div class="g-card-head">
          <div class="g-card-title"><Globe :size="16" />{{ t('webUsers.sites.title') }} <span class="g-card-desc">{{ t('webUsers.sites.count', { on: checkedCount, total: sites.length }) }}</span></div>
        </div>
        <div class="g-card-body">
          <div class="notice">{{ t('webUsers.sites.help') }}</div>
          <div v-if="sites.length" class="toolbar">
            <el-input v-model="keyword" clearable :prefix-icon="Search" :placeholder="t('webUsers.sites.search')" class="w-260" />
            <el-button size="small" :disabled="!selectable.length" @click="setVisible(true)">{{ t('webUsers.sites.checkVisible') }}</el-button>
            <el-button size="small" :disabled="!selectable.length" @click="setVisible(false)">{{ t('webUsers.sites.uncheckVisible') }}</el-button>
            <el-button size="small" :icon="allFolded ? ChevronsUpDown : ChevronsDownUp" @click="foldAll(!allFolded)">
              {{ allFolded ? t('common.expandAll') : t('common.collapseAll') }}
            </el-button>
          </div>
          <div v-for="g in groups" :key="g.listener" class="lst">
            <!-- 监听器标题：点击收起或展开其下的站点；搜索时全部展开 -->
            <div class="lst-head" @click="fold.setCollapsed(g.listener, !fold.isCollapsed(g.listener))">
              <ChevronRight :size="14" class="lst-arrow" :class="{ open: !isFolded(g.listener) }" />
              <span class="lst-name">{{ g.name }}</span>
              <el-tag size="small" type="info">{{ g.tls ? 'HTTPS' : 'HTTP' }} :{{ g.port }}</el-tag>
              <el-tag v-if="!g.enable" size="small" type="info">{{ t('webUsers.sites.disabled') }}</el-tag>
              <span class="lst-count">{{ t('webUsers.sites.groupCount', { on: countOn(g.sites), total: g.sites.length }) }}</span>
            </div>
            <template v-for="sec in isFolded(g.listener) ? [] : g.sections" :key="sec.name">
              <!-- 站点按 Web 服务中的分组归类，每组可单独收起 -->
              <GroupHeader
                v-if="sec.grouped"
                class="site-group"
                :name="sec.name"
                :count-text="t('webUsers.sites.groupCount', { on: countOn(sec.items), total: sec.items.length })"
                :collapsed="isFolded(`${g.listener}/${sec.name}`)"
                :renamable="false"
                @toggle="fold.setCollapsed(`${g.listener}/${sec.name}`, !fold.isCollapsed(`${g.listener}/${sec.name}`))"
              />
            <div v-for="s in isFolded(`${g.listener}/${sec.name}`) && sec.grouped ? [] : sec.items" :key="s.site" class="site-row" :class="{ 'is-grouped': sec.grouped }">
              <el-checkbox v-model="checked[keyOf(s)]" :disabled="lockReason(s) !== ''">
                <span class="site-name">{{ siteName(s) }}</span>
              </el-checkbox>
              <span class="site-domains mono">{{ s.domains?.join(', ') }}</span>
              <span class="tags">
                <el-tag v-if="s.default" size="small" type="info">{{ t('webUsers.sites.defaultSite') }}</el-tag>
                <el-tag v-if="!s.enable" size="small" type="info">{{ t('webUsers.sites.disabled') }}</el-tag>
                <el-tag v-if="s.forwardAuth" size="small" type="info">{{ t('webUsers.sites.forwardAuth') }}</el-tag>
                <template v-else>
                  <el-tag v-if="s.webAuth" size="small">{{ t('webUsers.sites.webAuth') }}</el-tag>
                  <el-tag v-if="s.basicAuth" size="small">{{ t('webUsers.sites.basicAuth') }}</el-tag>
                  <el-tag v-if="!s.webAuth && !s.basicAuth" size="small" type="warning">{{ t('webUsers.sites.noAuth') }}</el-tag>
                </template>
                <el-tag v-if="willEnableAuth(s)" size="small" type="success">{{ t('webUsers.sites.willEnableAuth') }}</el-tag>
              </span>
              <div v-if="lockReason(s)" class="form-help lock">{{ lockReason(s) }}</div>
            </div>
            </template>
          </div>
          <el-empty v-if="!sites.length" :description="t('webUsers.sites.none')" :image-size="60" />
          <el-empty v-else-if="!groups.length" :description="t('webUsers.sites.noMatch')" :image-size="60" />
        </div>
      </div>

      <div class="g-card">
        <div class="g-card-head">
          <div class="g-card-title"><KeyRound :size="16" />{{ t('web.oidcDivider') }}</div>
        </div>
        <div class="g-card-body">
          <div class="notice">{{ t('web.oidcIntro') }}</div>
          <el-form :model="form" label-width="130px" :label-position="labelPosition" @submit.prevent>
            <OIDCProvider v-model="form.oidc" />
          </el-form>
        </div>
      </div>

      <SaveBar :dirty="dirty" :saving="saving" @save="save" @reset="reset" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ChevronRight, ChevronsDownUp, ChevronsUpDown, Globe, KeyRound, Search, UserCog, UsersRound } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import GroupHeader from '../components/GroupHeader.vue'
import { useCollapsed } from '../composables/useCollapsed'
import { buildSections } from '../groupSort'
import SaveBar from '../components/SaveBar.vue'
import SharedUsers from '../components/SharedUsers.vue'
import OIDCProvider from '../components/OIDCProvider.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { type AuthUser, fromAPI, toAPI } from '../sharedUsers'
import { oidcFromAPI, oidcToAPI, type OIDCSettings } from '../oidc'

// 认证用户页面：集中管理 Web 服务的共享用户、单点登录、自助改密与 OIDC 身份提供方，
// 并批量设置各站点是否使用共享用户，接口为 /api/webservice/users，只读写这些设置
const { t } = useI18n()
const { labelPosition } = useFormLayout()

// UserSite 与后端 webservice.UserSite 对应
interface UserSite {
  listener: string
  listenerName: string
  port: number
  tls: boolean
  listenerEnable: boolean
  site: string
  name: string
  domains: string[] | null
  // group 为站点在 Web 服务列表中的分组
  group?: string
  enable: boolean
  default: boolean
  type: string
  basicAuth: boolean
  webAuth: boolean
  sharedUsers: boolean
  otherAuth: boolean
  forwardAuth: boolean
}

interface UsersReply {
  authUsers: AuthUser[] | null
  ssoDomain: string
  selfServicePassword: boolean
  oidc: Partial<OIDCSettings> | null
  sites: UserSite[] | null
}

interface UsersForm {
  authUsers: AuthUser[]
  ssoDomain: string
  selfServicePassword: boolean
  oidc: OIDCSettings
}

const form = ref<UsersForm>()
const sites = ref<UserSite[]>([])
// checked 为各站点勾选状态，键为 监听器key/站点key
const checked = ref<Record<string, boolean>>({})
const keyword = ref('')
const saving = ref(false)
const loadError = ref('')
// savedJSON 为最近一次加载或保存后的表单快照，用于判断是否有未保存的修改
const savedJSON = ref('')
let lastReply: UsersReply | undefined

function keyOf(s: UserSite): string {
  return `${s.listener}/${s.site}`
}

function siteName(s: UserSite): string {
  if (s.default) {
    return t('webUsers.sites.defaultSite')
  }
  return s.name || s.domains?.[0] || s.site
}

// lockReason 返回站点不能在这里切换的原因，空串表示可以切换
function lockReason(s: UserSite): string {
  if (s.forwardAuth) {
    return t('webUsers.sites.lockForward')
  }
  // 只靠共享用户认证的站点取消后会变为无需登录，需要在站点设置中明确关闭认证
  if (s.sharedUsers && (s.webAuth || s.basicAuth) && !s.otherAuth) {
    return t('webUsers.sites.lockOnly')
  }
  return ''
}

// willEnableAuth 判断保存后是否会为站点开启网页认证（站点原本未开启任何认证）
function willEnableAuth(s: UserSite): boolean {
  return !s.sharedUsers && !!checked.value[keyOf(s)] && !s.webAuth && !s.basicAuth
}

const checkedCount = computed(() => sites.value.filter((s) => checked.value[keyOf(s)]).length)

// matched 为按关键词（站点名称、域名或监听器名称）筛选后的站点
const matched = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) {
    return sites.value
  }
  return sites.value.filter((s) => [siteName(s), s.listenerName, ...(s.domains ?? [])].some((v) => v.toLowerCase().includes(kw)))
})

// selectable 为筛选结果中可以切换的站点
const selectable = computed(() => matched.value.filter((s) => lockReason(s) === ''))

// groups 把筛选后的站点按监听器归类，保持后端返回的顺序；监听器内再按站点在 Web 服务中的分组归类，
// 分组顺序与 Web 服务页一致，未分组的站点（含默认站点）在最后
const groups = computed(() => {
  const out: { listener: string; name: string; port: number; tls: boolean; enable: boolean; sites: UserSite[] }[] = []
  for (const s of matched.value) {
    let g = out.find((x) => x.listener === s.listener)
    if (!g) {
      g = { listener: s.listener, name: s.listenerName || `:${s.port}`, port: s.port, tls: s.tls, enable: s.listenerEnable, sites: [] }
      out.push(g)
    }
    g.sites.push(s)
  }
  return out.map((g) => {
    const secs = buildSections(g.sites, (s) => (s.default ? '' : (s.group ?? '')))
    const grouped = secs.some((sec) => sec.name)
    return { ...g, sections: secs.map((sec) => ({ ...sec, grouped })) }
  })
})

// 监听器与站点分组的收起状态，键为 监听器key 与 监听器key/分组名；搜索时全部展开
const fold = useCollapsed('webUsers.sites')
function isFolded(id: string) {
  return !keyword.value.trim() && fold.isCollapsed(id)
}
const allFolded = fold.allCollapsed(() => groups.value.map((g) => g.listener))
// foldAll 收起时收起全部监听器；展开时监听器与站点分组全部展开
function foldAll(v: boolean) {
  fold.setAll(groups.value.map((g) => g.listener), v)
}

// countOn 统计已勾选的站点数
function countOn(list: UserSite[]) {
  return list.filter((s) => checked.value[keyOf(s)]).length
}

function setVisible(on: boolean) {
  for (const s of selectable.value) {
    checked.value[keyOf(s)] = on
  }
}

// changedSites 为勾选状态与已保存状态不同的站点
function changedSites() {
  return sites.value
    .filter((s) => !!checked.value[keyOf(s)] !== s.sharedUsers)
    .map((s) => ({ listener: s.listener, site: s.site, sharedUsers: !!checked.value[keyOf(s)] }))
}

function snapshot(): string {
  if (!form.value) {
    return ''
  }
  const f = form.value
  return JSON.stringify({ users: toAPI(f.authUsers), sso: f.ssoDomain.trim(), self: f.selfServicePassword, oidc: oidcToAPI(f.oidc), sites: changedSites() })
}

const dirty = computed(() => !!form.value && snapshot() !== savedJSON.value)

function apply(r: UsersReply) {
  lastReply = r
  form.value = {
    authUsers: fromAPI(r.authUsers),
    ssoDomain: r.ssoDomain ?? '',
    selfServicePassword: !!r.selfServicePassword,
    oidc: oidcFromAPI(r.oidc),
  }
  sites.value = r.sites ?? []
  checked.value = Object.fromEntries(sites.value.map((s) => [keyOf(s), s.sharedUsers]))
  savedJSON.value = snapshot()
}

async function load() {
  try {
    apply(await api.get<UsersReply>('/api/webservice/users'))
    loadError.value = ''
  } catch (e) {
    loadError.value = (e as Error).message
  }
}

// reset 撤销未保存的修改，回到最近一次加载或保存的状态
function reset() {
  if (lastReply) {
    apply(lastReply)
  }
}

async function save() {
  const f = form.value
  if (!f) {
    return
  }
  saving.value = true
  try {
    const body = { authUsers: toAPI(f.authUsers), ssoDomain: f.ssoDomain.trim(), selfServicePassword: f.selfServicePassword, oidc: oidcToAPI(f.oidc), sites: changedSites() }
    apply(await api.put<UsersReply>('/api/webservice/users', body))
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// 有未保存的修改时，离开页面前确认，避免误操作丢失修改
onBeforeRouteLeave(async () => {
  if (!dirty.value) {
    return true
  }
  try {
    await ElMessageBox.confirm(t('webUsers.leaveConfirm'), t('webUsers.leaveTitle'), { type: 'warning', confirmButtonText: t('webUsers.leave'), cancelButtonText: t('common.cancel') })
    return true
  } catch {
    return false
  }
})

function onBeforeUnload(e: BeforeUnloadEvent) {
  if (dirty.value) {
    e.preventDefault()
  }
}

onMounted(() => {
  window.addEventListener('beforeunload', onBeforeUnload)
  load()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.w-260 {
  width: 260px;
  max-width: 100%;
}
.notice {
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
  margin-bottom: 10px;
}
.sub-form {
  margin-top: 18px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.toolbar .el-button + .el-button {
  margin-left: 0;
}
.lst + .lst {
  margin-top: 14px;
}
.lst-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--g-border);
  cursor: pointer;
  user-select: none;
}
.lst-arrow {
  color: var(--el-text-color-secondary);
  transition: transform 0.15s;
}
.lst-arrow.open {
  transform: rotate(90deg);
}
.lst-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.site-group {
  margin-top: 6px;
  border-radius: var(--g-radius);
}
.site-row.is-grouped {
  padding-left: 22px;
}
.lst-name {
  font-size: 13px;
  font-weight: 600;
}
.site-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  column-gap: 10px;
  padding: 6px 0 6px 4px;
  border-bottom: 1px dashed var(--g-border);
}
.site-row:last-child {
  border-bottom: none;
}
.site-name {
  font-weight: 500;
}
.site-domains {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.tags {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 6px;
}
.lock {
  flex-basis: 100%;
  margin-top: 0;
  padding-left: 24px;
}
</style>
