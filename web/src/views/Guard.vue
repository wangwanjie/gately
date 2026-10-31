<template>
  <div>
    <PageHeader :title="t('guard.title')" :desc="t('guard.desc')" :icon="ShieldBan">
      <el-button :icon="RefreshCw" @click="loadAll">{{ t('common.refresh') }}</el-button>
      <el-button type="primary" :icon="Ban" @click="openBan">{{ t('guard.ban.add') }}</el-button>
    </PageHeader>

    <!-- 防火墙状态 -->
    <div v-if="status" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Flame :size="16" />{{ t('guard.fw.title') }} <span v-if="status.firewall.version" class="g-card-desc">{{ status.firewall.version }}</span></div>
        <div class="head-actions">
          <StatusBadge v-bind="fwBadge" />
          <el-button v-if="status.firewall.active" size="small" :icon="FileCode" @click="showRules">{{ t('guard.fw.viewRules') }}</el-button>
        </div>
      </div>
      <div class="g-card-body">
        <div class="stats">
          <div class="stat">
            <div class="stat-value">{{ status.bans }}</div>
            <div class="stat-label">{{ t('guard.fw.activeBans') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ dropped }}</div>
            <div class="stat-label">{{ t('guard.fw.dropped') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.firewall.hits?.trap ?? 0 }}</div>
            <div class="stat-label">{{ t('guard.fw.trapHits') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.firewall.hits?.scan ?? 0 }}</div>
            <div class="stat-label">{{ t('guard.fw.scanHits') }}</div>
          </div>
        </div>
        <el-alert v-if="fwProblem" :type="fwProblem.type" :title="fwProblem.title" :closable="false" show-icon class="fw-alert">
          <div>{{ fwProblem.hint }}</div>
          <div v-if="status.firewall.error" class="mono err-detail">{{ status.firewall.error }}</div>
        </el-alert>
      </div>
    </div>

    <el-tabs v-model="tab" class="guard-tabs">
      <!-- 封禁列表 -->
      <el-tab-pane :label="t('guard.tabs.bans', { count: bans.length })" name="bans">
        <div class="toolbar">
          <el-input v-model="filter" :placeholder="t('guard.ban.search')" clearable class="search" :prefix-icon="Search" />
        </div>
        <el-table :data="filteredBans" :empty-text="t('guard.ban.empty')" size="small" class="ban-table">
          <el-table-column :label="t('guard.ban.address')" min-width="170">
            <template #default="{ row }"><span class="mono">{{ row.key }}</span></template>
          </el-table-column>
          <el-table-column :label="t('guard.ban.source')" min-width="140">
            <template #default="{ row }">
              <el-tag size="small" :type="sourceTag(row).type" effect="plain">{{ sourceTag(row).text }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('guard.ban.reason')" prop="reason" min-width="180" show-overflow-tooltip />
          <el-table-column :label="t('guard.ban.count')" width="80" align="center">
            <template #default="{ row }">{{ row.count }}</template>
          </el-table-column>
          <el-table-column :label="t('guard.ban.created')" min-width="150">
            <template #default="{ row }">{{ formatTime(row.created) }}</template>
          </el-table-column>
          <el-table-column :label="t('guard.ban.until')" min-width="150">
            <template #default="{ row }">
              <span v-if="!row.until" class="text-danger">{{ t('guard.ban.permanent') }}</span>
              <span v-else :title="formatTime(row.until)">{{ remaining(row.until) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="90" align="center">
            <template #default="{ row }">
              <el-button text size="small" type="primary" @click="unban(row.key)">{{ t('guard.ban.unban') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 日志规则 -->
      <el-tab-pane :label="t('guard.tabs.jails')" name="jails">
        <div class="toolbar">
          <span class="help">{{ t('guard.jail.intro') }}</span>
          <div class="toolbar-actions">
            <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
            <CompactToggle v-model="compact" />
            <el-dropdown trigger="click" @command="addFromPreset">
              <el-button :icon="LayoutTemplate">{{ t('guard.jail.fromPreset') }}</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-for="p in presets" :key="p.preset" :command="p.preset">
                    <div class="preset-item">
                      <span>{{ presetName(p) }}</span>
                      <span class="preset-desc">{{ t(`guard.presets.${p.preset}.desc`) }}</span>
                    </div>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button type="primary" :icon="Plus" @click="openJail()">{{ t('guard.jail.add') }}</el-button>
          </div>
        </div>
        <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('guard.jail.empty')">
          <template #item="{ it, item: j, sorting, bind }">
            <EntityCard v-bind="bind" :off="!j.enable" :compact="compact || sorting">
              <template #lead>
                <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
                <el-switch v-else :model-value="j.enable" @change="(v: string | number | boolean) => toggleJail(j, Boolean(v))" />
              </template>
              <template #title>
                <span>{{ j.name }}</span>
                <el-tag size="small" type="info" effect="plain">{{ t(`guard.source.${j.source}`) }}</el-tag>
              </template>
              <template v-if="j.remark" #sub>{{ j.remark }}</template>
              <template #status><StatusBadge v-bind="jailBadge(j)" /></template>
              <Field :label="sourceLabel(j.source)" mono wide>
                <div v-for="line in targetLines(j)" :key="line" class="target-line">{{ line }}</div>
              </Field>
              <Field v-if="j.source === 'web'" :label="t('guard.jail.paths')" mono wide>
                <div v-for="line in pathLines(j)" :key="line" class="target-line">{{ line }}</div>
              </Field>
              <Field v-if="j.source === 'web'" :label="t('guard.jail.webCount')">{{ webScopeText(j) }}</Field>
              <Field :label="t('guard.jail.condition')">{{ t('guard.jail.conditionValue', { window: humanDuration(j.findTime), count: j.maxRetry }) }}</Field>
              <Field :label="t('guard.jail.banTime')">{{ banTimeText(j.banTime) }}</Field>
              <Field :label="t('guard.jail.matches')">{{ j.matches }}</Field>
              <Field :label="t('guard.jail.bans')">{{ t('guard.jail.bansValue', { total: j.bans, active: j.active }) }}</Field>
              <template v-if="!sorting && (j.error && j.enable)" #extra>
                <div class="err-line">{{ j.error }}</div>
              </template>
              <template v-if="sorting" #actions>
                <SortMoves :gl="gl" :it="it" />
              </template>
              <template v-else #actions>
                <el-button text size="small" :icon="Pencil" @click="openJail(j)">{{ t('common.edit') }}</el-button>
                <el-popconfirm :title="t('guard.jail.deleteConfirm')" width="240" @confirm="removeJail(j)">
                  <template #reference>
                    <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </EntityCard>
          </template>
        </GroupedList>
      </el-tab-pane>

      <!-- 诱饵端口与扫描检测 -->
      <el-tab-pane :label="t('guard.tabs.scan')" name="scan">
        <template v-if="settings && status">
          <el-alert v-if="!status.firewall.active" type="warning" :closable="false" show-icon class="block-gap" :title="t('guard.scan.needFirewall')" />
          <div class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><Crosshair :size="16" />{{ t('guard.trap.title') }}</div>
              <el-button type="primary" size="small" :icon="Save" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
            </div>
            <div class="g-card-body">
              <div class="form-help top-help">{{ t('guard.trap.help') }}</div>
              <el-form label-width="130px" :label-position="labelPosition">
                <el-form-item :label="t('common.enable')"><el-switch v-model="settings.trap.enable" /></el-form-item>
                <el-form-item :label="t('guard.trap.ports')">
                  <el-input v-model="settings.trap.ports" class="mono w-420" :placeholder="defaultTrapPorts" />
                  <div class="form-help">{{ t('guard.trap.portsHelp') }}</div>
                </el-form-item>
                <el-form-item :label="t('guard.banTime')">
                  <BanTimeInput v-model="settings.trap.banSeconds" />
                </el-form-item>
                <el-form-item v-if="settings.trap.enable" :label="t('common.status')">
                  <div class="rows">
                    <div v-if="status.firewall.active">{{ t('guard.trap.effective', { ports: status.firewall.trapPorts || t('common.none') }) }}</div>
                    <div v-else-if="status.userTrapPorts.length">{{ t('guard.trap.userspace', { ports: status.userTrapPorts.join(', ') }) }}</div>
                    <div v-if="status.firewall.trapSkipped?.length" class="text-warning">{{ t('guard.trap.skipped', { ports: status.firewall.trapSkipped.join(', ') }) }}</div>
                  </div>
                </el-form-item>
              </el-form>
            </div>
          </div>

          <div class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><Radar :size="16" />{{ t('guard.scan.title') }}</div>
              <el-button type="primary" size="small" :icon="Save" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
            </div>
            <div class="g-card-body">
              <div class="form-help top-help">{{ t('guard.scan.help') }}</div>
              <el-form label-width="130px" :label-position="labelPosition">
                <el-form-item :label="t('common.enable')"><el-switch v-model="settings.scan.enable" :disabled="!status.firewall.active && !settings.scan.enable" /></el-form-item>
                <el-form-item :label="t('guard.scan.threshold')">
                  <el-input-number v-model="settings.scan.threshold" :min="3" :max="1000" />
                  <span class="unit">{{ t('guard.scan.perMinute') }}</span>
                  <div class="form-help">{{ t('guard.scan.thresholdHelp') }}</div>
                </el-form-item>
                <el-form-item :label="t('guard.scan.exclude')">
                  <el-input v-model="settings.scan.excludePorts" class="mono w-420" placeholder="8443,10000-10100" />
                  <div class="form-help">{{ t('guard.scan.excludeHelp', { count: status.firewall.openPorts }) }}</div>
                </el-form-item>
                <el-form-item :label="t('guard.banTime')">
                  <BanTimeInput v-model="settings.scan.banSeconds" />
                </el-form-item>
              </el-form>
            </div>
          </div>
        </template>
      </el-tab-pane>

      <!-- 全局设置 -->
      <el-tab-pane :label="t('guard.tabs.settings')" name="settings">
        <div v-if="settings && status" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><SlidersHorizontal :size="16" />{{ t('guard.tabs.settings') }}</div>
            <el-button type="primary" size="small" :icon="Save" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
          </div>
          <div class="g-card-body">
            <el-form label-width="150px" :label-position="labelPosition">
              <el-form-item :label="t('guard.settings.firewall')">
                <el-switch v-model="settings.firewall" />
                <div class="form-help">{{ t('guard.settings.firewallHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('guard.settings.banSeconds')">
                <DurationInput v-model="settings.banSeconds" />
              </el-form-item>
              <el-form-item :label="t('guard.settings.increment')">
                <el-switch v-model="settings.increment" />
                <div class="form-help">{{ t('guard.settings.incrementHelp') }}</div>
              </el-form-item>
              <el-form-item v-if="settings.increment" :label="t('guard.settings.maxBanSeconds')">
                <DurationInput v-model="settings.maxBanSeconds" />
              </el-form-item>
              <el-divider content-position="left">{{ t('guard.settings.whitelist') }}</el-divider>
              <el-form-item :label="t('guard.settings.ignorePrivate')">
                <el-switch v-model="settings.ignorePrivate" />
                <div class="form-help">{{ t('guard.settings.ignorePrivateHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('guard.settings.ignoreList')">
                <ListInput v-model="settings.ignoreList" type="textarea" :rows="4" class="mono w-420" placeholder="203.0.113.0/24&#10;2001:db8::/48" />
                <div class="form-help">{{ t('guard.settings.ignoreListHelp') }}</div>
              </el-form-item>
            </el-form>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <LogCard url="/api/guard/logs" clearable />

    <!-- 手动封禁 -->
    <el-dialog v-model="banVisible" :title="t('guard.ban.add')" width="480px" :fullscreen="isMobile">
      <el-form label-width="100px" :label-position="labelPosition">
        <el-form-item :label="t('guard.ban.address')">
          <el-input v-model="banForm.key" class="mono" placeholder="203.0.113.7 / 198.51.100.0/24" />
        </el-form-item>
        <el-form-item :label="t('guard.banTime')">
          <BanTimeInput v-model="banForm.seconds" />
        </el-form-item>
        <el-form-item :label="t('guard.ban.reason')">
          <el-input v-model="banForm.reason" :placeholder="t('guard.ban.reasonPlaceholder')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="banVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" :disabled="!banForm.key.trim()" @click="addBan">{{ t('guard.ban.submit') }}</el-button>
      </template>
    </el-dialog>

    <!-- 规则编辑 -->
    <el-dialog v-model="jailVisible" :title="editing.key ? t('guard.jail.edit') : t('guard.jail.add')" width="760px" top="5vh" :fullscreen="isMobile">
      <el-form :model="editing" label-width="120px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('guard.jail.source')">
          <el-radio-group :model-value="editing.source" @update:model-value="setSource">
            <el-radio-button v-for="s in sources" :key="s" :value="s">{{ t(`guard.source.${s}`) }}</el-radio-button>
          </el-radio-group>
          <div class="form-help block">{{ sourceHelp }}</div>
        </el-form-item>
        <el-form-item :label="sourceLabel(editing.source)">
          <el-select v-if="editing.source === 'event'" v-model="editing.target" class="w-420">
            <el-option v-for="e in eventTypes" :key="e" :label="t(`guard.event.${eventKey(e)}`)" :value="e" />
          </el-select>
          <template v-else-if="editing.source === 'web'">
            <el-select v-model="webListeners" multiple clearable class="w-420" :placeholder="t('guard.jail.allListeners')">
              <el-option v-for="l in listeners" :key="l.key" :label="listenerLabel(l)" :value="l.key" />
            </el-select>
            <div class="form-help block">{{ t('guard.jail.listenersHelp') }}</div>
          </template>
          <el-input
            v-else
            v-model="editing.target"
            :type="editing.source === 'file' || editing.source === 'journal' ? 'textarea' : 'text'"
            :rows="2"
            class="mono"
            :placeholder="targetPlaceholder"
          />
        </el-form-item>
        <el-form-item v-if="editing.source === 'web' && !editing.key" :label="t('guard.jail.webPreset')">
          <el-select :model-value="editing.preset ?? ''" class="w-420" @update:model-value="applyWebPreset">
            <el-option :label="t('guard.jail.webPresetNone')" value="" />
            <el-option v-for="p in webPresets" :key="p.preset" :label="presetName(p)" :value="p.preset!" />
          </el-select>
          <div v-if="editing.preset" class="form-help block">{{ t(`guard.presets.${editing.preset}.desc`) }}</div>
        </el-form-item>
        <template v-if="editing.source === 'web'">
          <el-form-item :label="t('guard.jail.paths')">
            <el-input v-model="editing.filter" type="textarea" :rows="4" class="mono" :placeholder="t('guard.jail.pathsPlaceholder')" />
            <div class="form-help block">{{ t('guard.jail.pathsHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('guard.jail.ignorePaths')">
            <el-input v-model="editing.ignore" type="textarea" :rows="2" class="mono" placeholder="/favicon.ico" />
            <div class="form-help block">{{ t('guard.jail.ignorePathsHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('guard.jail.anyStatus')">
            <el-switch v-model="editing.anyStatus" />
            <div class="form-help block">{{ t('guard.jail.anyStatusHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('guard.jail.unknownHost')">
            <el-switch v-model="editing.unknownHost" />
            <div class="form-help block">{{ t('guard.jail.unknownHostHelp') }}</div>
          </el-form-item>
        </template>
        <template v-else>
          <el-form-item :label="t('guard.jail.filter')">
            <el-input v-model="editing.filter" type="textarea" :rows="4" class="mono" :placeholder="editing.source === 'event' ? t('guard.jail.filterEventPlaceholder') : 'Failed password for .* from <HOST>'" />
            <div class="form-help block">{{ t('guard.jail.filterHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('guard.jail.ignore')">
            <el-input v-model="editing.ignore" type="textarea" :rows="2" class="mono" />
            <div class="form-help block">{{ t('guard.jail.ignoreHelp') }}</div>
          </el-form-item>
        </template>
        <el-form-item :label="t('guard.jail.condition')">
          <div class="cond">
            <DurationInput v-model="editing.findTime" :max="7 * 86400" />
            <span>{{ editing.source === 'web' ? t('guard.jail.withinWeb') : t('guard.jail.within') }}</span>
            <el-input-number v-model="editing.maxRetry" :min="1" :max="1000" />
            <span>{{ t('guard.jail.timesThenBan') }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('guard.banTime')">
          <BanTimeInput v-model="editing.banTime" />
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>

        <el-divider content-position="left">{{ t('guard.jail.testTitle') }}</el-divider>
        <el-form-item :label="editing.source === 'web' ? t('guard.jail.testPaths') : t('guard.jail.testLines')">
          <el-input v-model="testLines" type="textarea" :rows="4" class="mono" :placeholder="editing.source === 'web' ? t('guard.jail.testPathsPlaceholder') : t('guard.jail.testPlaceholder')" />
          <div class="test-actions">
            <el-button size="small" :icon="FlaskConical" :loading="testing" :disabled="!testLines.trim() || (editing.source !== 'web' && !editing.filter.trim())" @click="runTest">{{ t('common.test') }}</el-button>
            <span v-if="testResults" class="help">{{ t('guard.jail.testSummary', { matched: testResults.filter((r) => r.matched).length, total: testResults.length }) }}</span>
          </div>
          <div v-if="testResults" class="test-results">
            <div v-for="(r, i) in testResults" :key="i" class="test-row" :class="{ hit: r.matched }">
              <el-tag size="small" :type="r.matched ? (r.ignored ? 'info' : 'danger') : 'info'" effect="plain">
                {{ r.matched ? (r.ignored ? t('guard.jail.testIgnored', { host: r.host }) : r.host || t('guard.jail.testHit')) : t('guard.jail.testNoMatch') }}
              </el-tag>
              <span class="mono line">{{ r.line }}</span>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="jailVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveJail">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 防火墙规则 -->
    <el-dialog v-model="rulesVisible" :title="t('guard.fw.rulesTitle')" width="820px" :fullscreen="isMobile">
      <pre class="rules mono">{{ rulesText }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Ban, Crosshair, FileCode, Flame, FlaskConical, LayoutTemplate, Pencil, Plus, Radar, RefreshCw, Save, Search, ShieldBan, SlidersHorizontal, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import CompactToggle from '../components/CompactToggle.vue'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import ListInput from '../components/ListInput.vue'
import DurationInput from '../components/DurationInput.vue'
import BanTimeInput from '../components/BanTimeInput.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface FirewallStatus {
  enabled: boolean
  active: boolean
  reason?: string
  error?: string
  version?: string
  dropped: Record<string, number>
  hits: Record<string, number>
  openPorts: number
  trapPorts: string
  trapSkipped: number[]
}

interface GuardStatus {
  firewall: FirewallStatus
  bans: number
  userTrapPorts: number[]
  platform: { os: string; inDocker: boolean; journal: boolean; docker: boolean }
}

interface Settings {
  firewall: boolean
  banSeconds: number
  increment: boolean
  maxBanSeconds: number
  ignorePrivate: boolean
  ignoreList: string[]
  trap: { enable: boolean; ports: string; banSeconds: number }
  scan: { enable: boolean; threshold: number; excludePorts: string; banSeconds: number }
}

interface BanEntry {
  key: string
  until?: string
  created: string
  reason: string
  source: string
  count: number
  jail?: string
}

interface Jail {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  source: string
  target: string
  filter: string
  ignore: string
  maxRetry: number
  findTime: number
  banTime: number
  // anyStatus、unknownHost 只用于 Web 路径探测
  anyStatus?: boolean
  unknownHost?: boolean
  preset?: string
  remark: string
}

interface WebListener {
  key: string
  name: string
  port: number
}

interface JailView extends Jail {
  key: string
  running: boolean
  matches: number
  bans: number
  active: number
  error?: string
}

interface TestResult {
  line: string
  matched: boolean
  host?: string
  ignored?: boolean
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { isMobile, labelPosition } = useFormLayout()

const sources = ['file', 'journal', 'command', 'docker', 'event', 'web']
const eventTypes = ['auth.failed', 'waf.blocked']
const defaultTrapPorts = '23,135,1433,1521,3389,5900,6379,11211'

const tab = ref('bans')
const status = ref<GuardStatus>()
const settings = ref<Settings>()
const bans = ref<BanEntry[]>([])
const jails = ref<JailView[]>([])
const compact = useCompactView('guard.jails')
// 规则列表的搜索、分组与排序；顺序与分组的修改不会重启规则
const gl = useGroupedList<JailView>({
  storageKey: 'guard.jails.groups',
  items: () => jails.value,
  base: '/api/guard/jails',
  reload: loadJails,
  matches: (j, q) => [j.name, j.group ?? '', j.remark, ...targetLines(j)].join('\n').toLowerCase().includes(q),
})
const presets = ref<Jail[]>([])
// listeners 为 Web 服务的监听，供路径探测规则选择
const listeners = ref<WebListener[]>([])
const filter = ref('')
const saving = ref(false)
const banVisible = ref(false)
const banForm = ref({ key: '', seconds: 0, reason: '' })
const jailVisible = ref(false)
const editing = ref<Jail>(emptyJail())
const testLines = ref('')
const testResults = ref<TestResult[]>()
const testing = ref(false)
const rulesVisible = ref(false)
const rulesText = ref('')
// 定时刷新，标签页在后台时暂停
const polling = usePolling(refresh, 10000)

function emptyJail(): Jail {
  return { name: '', enable: true, source: 'file', target: '', filter: '', ignore: '', maxRetry: 5, findTime: 600, banTime: 0, remark: '' }
}

const dropped = computed(() => Object.values(status.value?.firewall.dropped ?? {}).reduce((a, b) => a + b, 0))

const fwBadge = computed((): { type: StatusType; text: string } => {
  const fw = status.value?.firewall
  if (!fw?.enabled) {
    return { type: 'off', text: t('guard.fw.disabled') }
  }
  if (fw.active) {
    return { type: 'running', text: t('guard.fw.active') }
  }
  return { type: fw.reason === 'os' ? 'info' : 'warn', text: t('guard.fw.inactive') }
})

// fwProblem 解释防火墙未生效的原因与处理办法
const fwProblem = computed(() => {
  const st = status.value
  const fw = st?.firewall
  if (!st || !fw || fw.active) {
    return undefined
  }
  if (!fw.enabled) {
    return { type: 'info' as const, title: t('guard.fw.disabledTitle'), hint: t('guard.fw.disabledHint') }
  }
  const docker = st.platform.inDocker
  switch (fw.reason) {
    case 'os':
      return { type: 'info' as const, title: t('guard.fw.reasonOS'), hint: t('guard.fw.hintOS') }
    case 'missing':
      return { type: 'warning' as const, title: t('guard.fw.reasonMissing'), hint: docker ? t('guard.fw.hintMissingDocker') : t('guard.fw.hintMissing') }
    case 'permission':
      return { type: 'warning' as const, title: t('guard.fw.reasonPermission'), hint: docker ? t('guard.fw.hintPermissionDocker') : t('guard.fw.hintPermission') }
    default:
      return { type: 'error' as const, title: t('guard.fw.reasonError'), hint: t('guard.fw.hintError') }
  }
})

const filteredBans = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) {
    return bans.value
  }
  return bans.value.filter((b) => [b.key, b.reason, b.jail ?? '', sourceTag(b).text].some((s) => s.toLowerCase().includes(q)))
})

function sourceTag(b: BanEntry): { type: 'danger' | 'warning' | 'info' | 'primary' | 'success'; text: string } {
  switch (b.source) {
    case 'manual':
      return { type: 'info', text: t('guard.ban.sourceManual') }
    case 'webservice':
      return { type: 'warning', text: t('guard.ban.sourceWeb') }
    case 'trap':
      return { type: 'danger', text: t('guard.ban.sourceTrap') }
    case 'scan':
      return { type: 'danger', text: t('guard.ban.sourceScan') }
  }
  if (b.source.startsWith('jail:')) {
    return { type: 'primary', text: b.jail || t('guard.ban.sourceJailDeleted') }
  }
  return { type: 'info', text: b.source }
}

function formatTime(s?: string) {
  return s ? new Date(s).toLocaleString() : '-'
}

// humanDuration 把秒数显示为最合适的单位
function humanDuration(sec: number) {
  if (sec % 86400 === 0) {
    return t('guard.unit.days', { n: sec / 86400 })
  }
  if (sec % 3600 === 0) {
    return t('guard.unit.hours', { n: sec / 3600 })
  }
  if (sec % 60 === 0) {
    return t('guard.unit.minutes', { n: sec / 60 })
  }
  return t('guard.unit.seconds', { n: sec })
}

function remaining(until: string) {
  const sec = Math.max(0, Math.round((new Date(until).getTime() - Date.now()) / 1000))
  if (sec >= 86400) {
    return t('guard.ban.remainDays', { d: Math.floor(sec / 86400), h: Math.floor((sec % 86400) / 3600) })
  }
  if (sec >= 3600) {
    return t('guard.ban.remainHours', { h: Math.floor(sec / 3600), m: Math.floor((sec % 3600) / 60) })
  }
  return t('guard.ban.remainMinutes', { m: Math.max(1, Math.ceil(sec / 60)) })
}

function banTimeText(sec: number) {
  if (sec === 0) {
    return t('guard.banTimeDefaultValue', { d: humanDuration(settings.value?.banSeconds ?? 3600) })
  }
  if (sec < 0) {
    return t('components.banTime.permanent')
  }
  return humanDuration(sec)
}

// webListeners 把以换行分隔的监听 key 映射为多选框的值，空数组表示全部监听
const webListeners = computed({
  get: () => editing.value.target.split('\n').filter(Boolean),
  set: (v: string[]) => {
    editing.value.target = v.join('\n')
  },
})

const webPresets = computed(() => presets.value.filter((p) => p.source === 'web'))

function listenerLabel(l: WebListener) {
  return `${l.name || t('web.unnamedListener')} (:${l.port})`
}

function listenerName(key: string) {
  const l = listeners.value.find((x) => x.key === key)
  return l ? listenerLabel(l) : key
}

function pathLines(j: Jail) {
  const lines = j.filter.split('\n').filter(Boolean)
  if (!lines.length) {
    return [t('guard.jail.allPaths')]
  }
  // 诱饵路径较多，卡片上只列前几条
  return lines.length > 4 ? [...lines.slice(0, 3), t('guard.jail.morePaths', { n: lines.length - 3 })] : lines
}

function webScopeText(j: Jail) {
  const status = j.anyStatus ? t('guard.jail.scopeAnyStatus') : t('guard.jail.scopeNotFound')
  return j.unknownHost ? `${status} · ${t('guard.jail.scopeUnknownHost')}` : status
}

// setSource 切换来源；涉及事件或 Web 来源时目标的含义不同，清空后重新选择
function setSource(v: string | number | boolean | undefined) {
  const next = String(v)
  const prev = editing.value.source
  if (prev !== next && [prev, next].some((s) => s === 'web' || s === 'event')) {
    editing.value.target = next === 'event' ? eventTypes[0] : ''
    editing.value.preset = undefined
  }
  editing.value.source = next
}

// applyWebPreset 套用 Web 路径探测模板，保留已选的监听
function applyWebPreset(id: string) {
  const target = editing.value.target
  const p = webPresets.value.find((x) => x.preset === id)
  if (!p) {
    editing.value = { ...emptyJail(), source: 'web', target, name: editing.value.name, remark: editing.value.remark }
    return
  }
  editing.value = { ...emptyJail(), ...JSON.parse(JSON.stringify(p)), target, name: presetName(p), enable: true, remark: editing.value.remark }
  testResults.value = undefined
}

function eventKey(e: string) {
  return e === 'auth.failed' ? 'authFailed' : 'wafBlocked'
}

function sourceLabel(source: string) {
  return t(`guard.target.${source}`)
}

function targetLines(j: Jail) {
  if (j.source === 'event') {
    return [t(`guard.event.${eventKey(j.target)}`)]
  }
  if (j.source === 'web') {
    const keys = j.target.split('\n').filter(Boolean)
    return keys.length ? keys.map(listenerName) : [t('guard.jail.allListeners')]
  }
  return j.target.split('\n').filter(Boolean)
}

const targetPlaceholder = computed(() => {
  switch (editing.value.source) {
    case 'file':
      return '/var/log/auth.log'
    case 'journal':
      return 'ssh.service'
    case 'command':
      return 'logread -f'
    default:
      return 'vaultwarden'
  }
})

// sourceHelp 说明当前来源在本机是否可用
const sourceHelp = computed(() => {
  const p = status.value?.platform
  switch (editing.value.source) {
    case 'file':
      return p?.inDocker ? t('guard.sourceHelp.fileDocker') : t('guard.sourceHelp.file')
    case 'journal':
      return p?.journal ? t('guard.sourceHelp.journal') : t('guard.sourceHelp.journalMissing')
    case 'command':
      return t('guard.sourceHelp.command')
    case 'docker':
      return t('guard.sourceHelp.docker')
    case 'web':
      return t('guard.sourceHelp.web')
    default:
      return t('guard.sourceHelp.event')
  }
})

function presetName(p: Jail) {
  return t(`guard.presets.${p.preset}.name`)
}

function jailBadge(j: JailView): { type: StatusType; text: string; title?: string } {
  if (!j.enable) {
    return { type: 'off', text: t('guard.jail.statusOff') }
  }
  if (j.error) {
    return { type: 'error', text: t('guard.jail.statusError'), title: j.error }
  }
  return { type: 'running', text: t('guard.jail.statusRunning') }
}

async function loadStatus() {
  status.value = await api.get<GuardStatus>('/api/guard/status')
}

async function loadBans() {
  bans.value = await api.get<BanEntry[]>('/api/guard/bans')
}

async function loadJails() {
  jails.value = await api.get<JailView[]>('/api/guard/jails')
}

async function loadAll() {
  try {
    const [s] = await Promise.all([api.get<Settings>('/api/guard/settings'), loadStatus(), loadBans(), loadJails()])
    settings.value = s
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// refresh 定时刷新状态、封禁与规则统计，不覆盖正在编辑的设置
async function refresh() {
  try {
    await Promise.all([loadStatus(), loadBans(), loadJails()])
  } catch {
    // 定时刷新失败时保持原内容
  }
}

async function saveSettings() {
  if (!settings.value) {
    return
  }
  saving.value = true
  try {
    settings.value = await api.put<Settings>('/api/guard/settings', settings.value)
    ElMessage.success(t('guard.savedApplied'))
    await loadStatus()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

function openBan() {
  banForm.value = { key: '', seconds: 0, reason: '' }
  banVisible.value = true
}

async function addBan() {
  saving.value = true
  try {
    await api.post('/api/guard/bans', banForm.value)
    banVisible.value = false
    ElMessage.success(t('guard.ban.added'))
    await Promise.all([loadBans(), loadStatus()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function unban(key: string) {
  try {
    await api.del('/api/guard/bans', { key })
    ElMessage.success(t('guard.ban.removed', { key }))
    await Promise.all([loadBans(), loadStatus()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function openJail(j?: JailView) {
  if (j) {
    const { running: _r, matches: _m, bans: _b, active: _a, error: _e, ...jail } = j
    editing.value = JSON.parse(JSON.stringify(jail))
  } else {
    editing.value = emptyJail()
  }
  testLines.value = ''
  testResults.value = undefined
  jailVisible.value = true
}

function addFromPreset(id: string) {
  const p = presets.value.find((x) => x.preset === id)
  if (!p) {
    return
  }
  editing.value = { ...emptyJail(), ...JSON.parse(JSON.stringify(p)), name: presetName(p), enable: true, remark: '' }
  testLines.value = ''
  testResults.value = undefined
  jailVisible.value = true
}

async function saveJail() {
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/guard/jails/${key}`, body)
    } else {
      await api.post('/api/guard/jails', body)
    }
    jailVisible.value = false
    ElMessage.success(t('guard.savedApplied'))
    await loadJails()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggleJail(j: JailView, enable: boolean) {
  try {
    await api.put(`/api/guard/jails/${j.key}/enable`, { enable })
    await loadJails()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeJail(j: JailView) {
  try {
    await api.del(`/api/guard/jails/${j.key}`)
    await loadJails()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function runTest() {
  testing.value = true
  try {
    testResults.value = await api.post<TestResult[]>('/api/guard/jails/test', { source: editing.value.source, filter: editing.value.filter, ignore: editing.value.ignore, lines: testLines.value })
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    testing.value = false
  }
}

async function showRules() {
  try {
    rulesText.value = (await api.get<{ rules: string }>('/api/guard/firewall/rules')).rules
    rulesVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function loadListeners() {
  try {
    listeners.value = await api.get<WebListener[]>('/api/webservice/listeners')
  } catch {
    // Web 服务未开启时只能选择全部监听
    listeners.value = []
  }
}

// openWebProbe 处理从 Web 服务监听卡片跳转过来的快捷创建：预选该监听并套用诱饵路径模板
function openWebProbe(key: string) {
  tab.value = 'jails'
  openJail()
  editing.value = { ...editing.value, source: 'web', target: key }
  applyWebPreset('web-decoy')
}

onMounted(async () => {
  await Promise.all([loadAll(), loadListeners()])
  try {
    presets.value = await api.get<Jail[]>('/api/guard/presets')
  } catch {
    presets.value = []
  }
  const key = route.query.webProbe
  if (typeof key === 'string' && key) {
    // 去掉查询参数，刷新页面时不再重复打开
    void router.replace({ query: {} })
    openWebProbe(key)
  }
  polling.start()
})
</script>

<style scoped>
.head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.stat {
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.stat-value {
  font-size: 22px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.stat-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.fw-alert {
  margin-top: 12px;
}
.err-detail {
  margin-top: 4px;
  font-size: 12px;
  word-break: break-all;
}
.guard-tabs {
  margin-top: 16px;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.search {
  width: 280px;
}
.ban-table {
  width: 100%;
}
.preset-item {
  display: flex;
  flex-direction: column;
  line-height: 1.4;
  padding: 2px 0;
}
.preset-desc {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.target-line {
  word-break: break-all;
}
.err-line {
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.top-help {
  margin-bottom: 12px;
}
.unit {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
}
.w-420 {
  width: 420px;
  max-width: 100%;
}
.cond {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.test-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
}
.test-results {
  width: 100%;
  margin-top: 8px;
  max-height: 240px;
  overflow: auto;
  border: 1px solid var(--g-border);
  border-radius: 6px;
  padding: 6px 8px;
}
.test-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 2px 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.test-row.hit {
  color: var(--el-text-color-primary);
}
.test-row .line {
  flex: 1;
  min-width: 0;
  line-height: 22px;
  word-break: break-all;
}
.rules {
  margin: 0;
  max-height: 65vh;
  overflow: auto;
  font-size: 12px;
  white-space: pre;
}
.text-danger {
  color: var(--el-color-danger);
}
.text-warning {
  color: var(--el-color-warning);
}
@media (max-width: 767.98px) {
  .stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .search {
    width: 100%;
  }
}
</style>
