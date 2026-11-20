<template>
  <div>
    <PageHeader :title="t('knock.title')" doc="modules/knock" :desc="t('knock.desc')" :icon="KeyRound">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button :icon="Settings2" @click="openSettings">{{ t('knock.settings.button') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('knock.rule.add') }}</el-button>
    </PageHeader>

    <!-- 运行状态：敲门端口、防火墙拦截与报文统计 -->
    <div v-if="status" class="g-card status-card">
      <div class="g-card-head">
        <div class="g-card-title"><Activity :size="16" />{{ t('knock.status.title') }}</div>
        <div class="head-actions">
          <el-button v-if="status.firewall.active" size="small" :icon="FileCode" @click="showRules">{{ t('knock.status.viewRules') }}</el-button>
        </div>
      </div>
      <div class="g-card-body">
        <div class="status-rows">
          <div class="status-row">
            <span class="status-label">{{ t('knock.status.listen') }}</span>
            <StatusBadge v-bind="listenBadge" />
            <span v-if="status.openStatus" class="muted">{{ status.openStatus }}</span>
          </div>
          <div class="status-row">
            <span class="status-label">{{ t('knock.status.firewall') }}</span>
            <StatusBadge v-bind="fwBadge" />
            <span v-if="status.firewall.active" class="muted mono">{{ status.firewall.backend }} · {{ status.firewall.ports.join(', ') }}</span>
          </div>
        </div>
        <el-alert v-if="listenProblem" type="error" :title="listenProblem" :closable="false" show-icon class="status-alert" />
        <el-alert v-if="fwProblem" :type="fwProblem.type" :title="fwProblem.title" :closable="false" show-icon class="status-alert">
          <div>{{ fwProblem.hint }}</div>
          <div v-if="status.firewall.error" class="mono err-detail">{{ status.firewall.error }}</div>
        </el-alert>
        <!-- iptables 后端没有 ip6tables 时只拦截 IPv4，受保护端口经 IPv6 仍可直接访问 -->
        <el-alert v-if="status.firewall.active && status.firewall.noIPv6" type="warning" :title="t('knock.status.noIPv6')" :closable="false" show-icon class="status-alert">
          <div>{{ t('knock.status.noIPv6Hint') }}</div>
        </el-alert>
        <div class="stats">
          <div class="stat">
            <div class="stat-value">{{ status.counters.accepted }}</div>
            <div class="stat-label">{{ t('knock.status.accepted') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.counters.rejected }}</div>
            <div class="stat-label">{{ t('knock.status.rejected') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.firewall.dropped }}</div>
            <div class="stat-label">{{ t('knock.status.dropped') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.firewall.allowed }}</div>
            <div class="stat-label">{{ t('knock.status.allowed') }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 当前有效的授权 -->
    <div v-if="status" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Unlock :size="16" />{{ t('knock.grants.title', { count: status.grants.length }) }}</div>
      </div>
      <div class="g-card-body">
        <div v-if="!status.grants.length" class="empty-hint">{{ t('knock.grants.empty') }}</div>
        <div v-else class="grants">
          <div v-for="g in status.grants" :key="g.addr + g.rule" class="grant">
            <div class="grant-main">
              <span class="mono grant-addr">{{ g.addr }}</span>
              <IpGeo :ip="g.addr" />
              <el-tag size="small" effect="plain">{{ g.ruleName || g.rule }}</el-tag>
              <el-tag v-if="g.source === 'web'" size="small" type="success" effect="plain" data-test="grant-web">{{ t('knock.grants.web') }}</el-tag>
              <el-tag v-else-if="g.identity" size="small" type="primary" effect="plain" class="ident-tag" data-test="grant-identity"><UserRound :size="12" />{{ g.identity }}</el-tag>
            </div>
            <div v-if="g.source === 'web'" class="grant-who muted" data-test="grant-who">{{ t('knock.grants.webWho', { user: g.user || '-', site: g.siteName || g.site || '-' }) }}</div>
            <div class="grant-meta">
              <span v-if="g.ports.length" class="mono">{{ g.ports.join(', ') }}</span>
              <span v-else class="muted">{{ t('knock.grants.gatelyOnly') }}</span>
              <span class="remain">{{ t('knock.grants.remain', { t: remainText(g.until) }) }}</span>
            </div>
            <el-popconfirm :title="t('knock.grants.revokeConfirm')" @confirm="revoke(g)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Lock" class="grant-revoke">{{ t('knock.grants.revoke') }}</el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <GroupedList :gl="gl" :error="loadError" @retry="load" :compact="compact" grid :empty-text="t('knock.rule.empty')">
      <template #item="{ it, item: r, sorting, bind }">
        <EntityCard v-bind="bind" :off="!r.enable" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="r.enable" :aria-label="t('common.toggleItem', { name: r.name || '-' })" @change="(v: string | number | boolean) => toggle(r, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ r.name }}</span>
            <el-tag v-if="r.firewall" size="small" type="warning" effect="plain">{{ t('knock.rule.firewallTag') }}</el-tag>
          </template>
          <template v-if="r.remark" #sub>{{ r.remark }}</template>
          <Field :label="t('knock.rule.ports')" :mono="r.ports.length > 0">
            <span v-if="r.ports.length">{{ r.ports.join(', ') }}</span>
            <span v-else class="muted" data-test="rule-web-only">{{ t('knock.rule.webOnly') }}</span>
          </Field>
          <Field :label="t('knock.rule.timeout')">{{ timeoutText(r) }}</Field>
          <Field v-if="r.user" :label="t('knock.rule.user')" mono>{{ r.user }}</Field>
          <Field v-if="r.identities?.length || r.singleAddr" :label="t('knock.identity.title')" wide>
            <div class="ident-tags" data-test="rule-identities">
              <el-tag v-if="r.sharedKeyOff" size="small" type="info" effect="plain">{{ t('knock.identity.sharedOffTag') }}</el-tag>
              <el-tag v-if="r.singleAddr" size="small" type="warning" effect="plain">{{ t('knock.identity.singleAddrTag') }}</el-tag>
              <el-tag v-for="id in r.identities ?? []" :key="id.id" size="small" :type="id.enable ? 'primary' : 'info'" effect="plain" :class="{ 'ident-off': !id.enable }" :title="identityTitle(r, id)">
                {{ id.name }}<span class="ident-count">{{ (id.id && r.identityStats?.[id.id]?.knocks) || 0 }}</span>
              </el-tag>
            </div>
          </Field>
          <Field :label="t('knock.rule.knocks')">
            {{ t('knock.rule.knockStats', { ok: r.stats.knocks, bad: r.stats.rejected }) }}
            <div v-if="r.stats.lastAt" class="muted">{{ t('knock.rule.lastKnock', { addr: r.stats.lastAddr, time: formatTime(r.stats.lastAt) }) }}</div>
          </Field>
          <Field v-if="r.usedBy.length" :label="t('knock.rule.usedBy')" wide>
            <div v-for="u in r.usedBy" :key="u" class="used-by">{{ u }}</div>
          </Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openEditor(r)">{{ t('common.edit') }}</el-button>
            <el-button text size="small" :icon="CopyPlus" @click="openEditor(r, true)">{{ t('common.duplicate') }}</el-button>
            <el-popconfirm :title="t('knock.rule.confirmDelete')" @confirm="remove(r)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
            <!-- 没有端口的规则只经网页登录放行，不接受敲门报文，没有客户端命令 -->
            <el-button v-if="r.ports.length" type="primary" size="small" :icon="Terminal" class="client-btn" @click="openClient(r)">{{ t('knock.client.button') }}</el-button>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/knock/logs" clearable />

    <!-- 规则编辑 -->
    <el-dialog v-model="editorVisible" :title="editing.key ? t('knock.rule.edit') : t('knock.rule.add')" width="600px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" />
      <el-form :model="editing" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" :placeholder="t('knock.rule.namePlaceholder')" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('knock.rule.ports')">
          <el-input v-model="portsText" class="mono" placeholder="tcp/22, udp/51820" />
          <div class="form-help">{{ t('knock.rule.portsHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.firewall')">
          <el-switch v-model="editing.firewall" />
          <div class="form-help">{{ t('knock.rule.firewallHelp') }}</div>
          <div v-if="editing.firewall" class="form-help warn">{{ t('knock.rule.firewallWarn') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.encKey')">
          <div class="key-row">
            <SecretInput v-model="editing.encKey" class="mono" autocomplete="off" :placeholder="t('knock.rule.keyAuto')" :reveal="secretOf(editing.key && !copyFrom && `/api/knock/rules/${editing.key}/secrets`, 'encKey')" />
            <el-button :icon="RefreshCw" @click="editing.encKey = randomKey(32)">{{ t('knock.rule.generate') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.hmacKey')">
          <div class="key-row">
            <SecretInput v-model="editing.hmacKey" class="mono" autocomplete="off" :placeholder="t('knock.rule.keyAuto')" :reveal="secretOf(editing.key && !copyFrom && `/api/knock/rules/${editing.key}/secrets`, 'hmacKey')" />
            <el-button :icon="RefreshCw" @click="editing.hmacKey = randomKey(64)">{{ t('knock.rule.generate') }}</el-button>
          </div>
          <div class="form-help">{{ t('knock.rule.keysHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.hmacType')">
          <el-select v-model="editing.hmacType" class="w-200">
            <el-option v-for="h in hmacTypes" :key="h" :label="h.toUpperCase()" :value="h" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('knock.rule.timeout')">
          <el-input-number v-model="editing.timeout" :min="5" :max="86400" />
          <span class="unit">{{ t('knock.unit.seconds') }}</span>
          <div class="form-help">{{ t('knock.rule.timeoutHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.maxTimeout')">
          <el-input-number v-model="editing.maxTimeout" :min="0" :max="86400" />
          <span class="unit">{{ t('knock.unit.seconds') }}</span>
          <div class="form-help">{{ t('knock.rule.maxTimeoutHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.user')">
          <el-input v-model="editing.user" class="mono" :placeholder="t('knock.rule.userPlaceholder')" />
          <div class="form-help">{{ t('knock.rule.userHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.requireAddr')">
          <el-switch v-model="editing.requireAddr" />
          <div class="form-help">{{ t('knock.rule.requireAddrHelp') }}</div>
        </el-form-item>
        <!-- 身份：每人一份密钥，敲门时据此识别是谁，可单独停用、删除或更换 -->
        <el-form-item :label="t('knock.identity.title')">
          <div class="identities">
            <div class="form-help">{{ t('knock.identity.help') }}</div>
            <div v-for="(id, i) in editing.identities ?? []" :key="uidOf(id)" class="identity" data-test="identity-row">
              <div class="identity-head">
                <el-switch v-model="id.enable" :aria-label="t('common.toggleItem', { name: id.name || '-' })" />
                <el-input v-model="id.name" class="identity-name" :placeholder="t('knock.identity.namePlaceholder')" data-test="identity-name" />
                <el-button text size="small" :icon="KeyRound" :type="openIdentities.has(id) ? 'primary' : undefined" data-test="identity-keys" @click="toggleIdentity(id)">{{ t('knock.identity.keys') }}</el-button>
                <el-button text size="small" type="danger" :icon="Trash2" :aria-label="t('common.delete')" data-test="identity-remove" @click="removeIdentity(i)" />
              </div>
              <div v-if="openIdentities.has(id)" class="identity-keys">
                <div class="key-label">{{ t('knock.rule.encKey') }}</div>
                <div class="key-row">
                  <SecretInput v-model="id.encKey" class="mono" autocomplete="off" :placeholder="t('knock.rule.keyAuto')" :reveal="identitySecret(id, 'encKey')" />
                  <el-button :icon="RefreshCw" @click="id.encKey = randomKey(32)">{{ t('knock.rule.generate') }}</el-button>
                </div>
                <div class="key-label">{{ t('knock.rule.hmacKey') }}</div>
                <div class="key-row">
                  <SecretInput v-model="id.hmacKey" class="mono" autocomplete="off" :placeholder="t('knock.rule.keyAuto')" :reveal="identitySecret(id, 'hmacKey')" />
                  <el-button :icon="RefreshCw" @click="id.hmacKey = randomKey(64)">{{ t('knock.rule.generate') }}</el-button>
                </div>
                <div class="key-label">{{ t('common.remark') }}</div>
                <el-input v-model="id.remark" />
              </div>
            </div>
            <el-button size="small" :icon="UserPlus" :disabled="!editing.ports.length || (editing.identities?.length ?? 0) >= maxIdentities" data-test="identity-add" @click="addIdentity">{{ t('knock.identity.add') }}</el-button>
            <div v-if="!editing.ports.length" class="form-help">{{ t('knock.identity.needPorts') }}</div>
          </div>
        </el-form-item>
        <el-form-item v-if="editing.identities?.length" :label="t('knock.identity.sharedOff')">
          <el-switch v-model="editing.sharedKeyOff" data-test="shared-off" />
          <div class="form-help">{{ t('knock.identity.sharedOffHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.identity.singleAddr')">
          <el-switch v-model="editing.singleAddr" data-test="single-addr" />
          <div class="form-help">{{ t('knock.identity.singleAddrHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 全局设置 -->
    <el-dialog v-model="settingsVisible" :title="t('knock.settings.title')" width="560px" :fullscreen="isMobile">
      <el-form v-if="settingsForm" :model="settingsForm" label-width="140px" :label-position="labelPosition">
        <el-form-item :label="t('knock.settings.port')">
          <el-input-number v-model="settingsForm.port" :min="0" :max="65535" />
          <div class="form-help">{{ t('knock.settings.portHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.autoFirewall')">
          <el-switch v-model="settingsForm.autoFirewall" />
          <div class="form-help">{{ t('knock.settings.autoFirewallHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.allowLan')">
          <el-switch v-model="settingsForm.allowLan" />
          <div class="form-help">{{ t('knock.settings.allowLanHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.allowList')">
          <el-input v-model="allowText" type="textarea" :rows="3" class="mono" placeholder="203.0.113.0/24" />
          <div class="form-help">{{ t('knock.settings.allowListHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.maxAge')">
          <el-input-number v-model="settingsForm.maxAge" :min="10" :max="3600" />
          <span class="unit">{{ t('knock.unit.seconds') }}</span>
          <div class="form-help">{{ t('knock.settings.maxAgeHelp') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 客户端配置：生成 fwknop 命令与 .fwknoprc 配置段 -->
    <el-dialog v-model="clientVisible" :title="t('knock.client.title', { name: clientRule?.name })" width="680px" :fullscreen="isMobile">
      <el-form label-width="120px" :label-position="labelPosition">
        <!-- 规则有身份时按身份生成：每人拿到的只有自己的密钥 -->
        <el-form-item v-if="clientIdentities.length > 1" :label="t('knock.client.identity')">
          <el-select v-model="client.identity" class="w-200" data-test="client-identity" @change="loadClientKeys">
            <el-option v-for="o in clientIdentities" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('knock.client.server')">
          <el-input v-model="client.server" class="mono" :placeholder="t('knock.client.serverPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('knock.client.via')">
          <el-radio-group v-model="client.via">
            <el-radio-button value="udp">{{ t('knock.client.viaUdp', { port: settings?.port || 62201 }) }}</el-radio-button>
            <el-radio-button value="tcp">{{ t('knock.client.viaTcp') }}</el-radio-button>
          </el-radio-group>
          <div v-if="client.via === 'tcp'" class="tcp-port">
            <el-input-number v-model="client.tcpPort" :min="1" :max="65535" />
            <span class="form-help">{{ t('knock.client.tcpHelp') }}</span>
          </div>
          <div v-else-if="!settings?.port" class="form-help warn">{{ t('knock.client.udpOff') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.client.addr')">
          <el-radio-group v-model="client.addr">
            <el-radio-button value="resolve">{{ t('knock.client.addrResolve') }}</el-radio-button>
            <el-radio-button value="source" :disabled="clientRule?.requireAddr">{{ t('knock.client.addrSource') }}</el-radio-button>
            <el-radio-button value="fixed">{{ t('knock.client.addrFixed') }}</el-radio-button>
          </el-radio-group>
          <el-input v-if="client.addr === 'fixed'" v-model="client.fixed" class="mono fixed-ip" placeholder="203.0.113.10" />
          <div class="form-help">{{ t(`knock.client.addrHelp.${client.addr}`) }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.client.ports')">
          <el-checkbox-group v-model="client.ports">
            <el-checkbox v-for="p in clientRule?.ports ?? []" :key="p" :value="p" class="mono">{{ p }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <div v-if="clientError" class="form-help warn">{{ clientError }}</div>
      <template v-else>
        <!-- 推荐 ~/.fwknoprc：命令行中的密钥会留在 shell 历史与进程列表中 -->
        <div class="snippet-head">
          <span>{{ t('knock.client.rc') }}</span>
          <el-button size="small" :icon="Copy" @click="copyText(rcText)">{{ t('common.copy') }}</el-button>
        </div>
        <pre class="snippet mono">{{ rcText }}</pre>
        <div class="form-help">{{ t('knock.client.rcHelp', { name: stanzaName }) }}</div>
        <div class="snippet-head">
          <span>{{ t('knock.client.command') }}</span>
          <el-button size="small" :icon="Copy" @click="copyText(commandText)">{{ t('common.copy') }}</el-button>
        </div>
        <pre class="snippet mono">{{ commandText }}</pre>
        <div class="form-help warn">{{ t('knock.client.commandWarn') }}</div>
      </template>
      <div class="form-help">{{ t('knock.client.install') }}</div>
    </el-dialog>

    <el-dialog v-model="rulesVisible" :title="t('knock.status.viewRules')" width="760px" :fullscreen="isMobile">
      <pre class="rules mono">{{ rulesText }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, shallowReactive } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Activity, Copy, CopyPlus, FileCode, KeyRound, Lock, Pencil, Plus, RefreshCw, Settings2, Terminal, Trash2, Unlock, UserPlus, UserRound } from 'lucide-vue-next'
import { api, errorText } from '../api'
import { secretOf } from '../secrets'
import SecretInput from '../components/SecretInput.vue'
import PageHeader from '../components/PageHeader.vue'
import IpGeo from '../components/IpGeo.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import CompactToggle from '../components/CompactToggle.vue'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import CopyHint from '../components/CopyHint.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useCompactView } from '../composables/useCompactView'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLines } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'
import { copyText } from '../composables/useClipboard'
import { createItem, uniqueName, type CopySource } from '../copyDraft'

interface Rule {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  // ports 为可以请求放行的端口，如 tcp/22
  ports: string[]
  // firewall 为 true 时在系统防火墙中拦截这些端口
  firewall: boolean
  encKey: string
  hmacKey: string
  hmacType: string
  timeout: number
  maxTimeout: number
  user: string
  requireAddr: boolean
  // identities 为规则的身份（每人一份密钥）；sharedKeyOff 停用规则自身的共享密钥；singleAddr 每个身份同时只放行一个地址
  identities?: Identity[]
  sharedKeyOff?: boolean
  singleAddr?: boolean
  remark: string
}

interface Identity {
  // id 为身份在规则内的标识，新身份留空由服务端分配
  id?: string
  name: string
  enable: boolean
  encKey: string
  hmacKey: string
  remark?: string
}

interface KnockStats {
  knocks: number
  rejected: number
  lastAt?: string
  lastAddr?: string
}

interface RuleView extends Rule {
  key: string
  stats: KnockStats
  // identityStats 为各身份的统计：身份 ID -> 统计
  identityStats?: Record<string, KnockStats>
  // usedBy 为引用该规则的端口转发、协议分流、STUN 规则与路由
  usedBy: string[]
}

interface KnockSettings {
  port: number
  autoFirewall: boolean
  allowLan: boolean
  allowList: string[]
  maxAge: number
}

interface Grant {
  addr: string
  rule: string
  ruleName: string
  until: string
  // ports 为写入防火墙的端口，只用于 Gately 自身入口的授权为空
  ports: string[]
  // source 为授权来源：spa（敲门报文）、web（在 Web 站点的放行入口登录）；addr 为网段时带前缀长度
  source?: string
  // identity 为敲门报文授权的身份名称（共享密钥为空）
  identity?: string
  // user、siteName 为网页登录放行的用户与入口站点
  user?: string
  site?: string
  siteName?: string
}

interface Status {
  listening: boolean
  port: number
  listenError?: string
  openStatus?: string
  firewall: {
    needed: boolean
    active: boolean
    reason?: string
    error?: string
    backend?: string
    version?: string
    // noIPv6 为 true 时防火墙拦截只覆盖 IPv4
    noIPv6?: boolean
    ports: string[]
    dropped: number
    allowed: number
  }
  grants: Grant[]
  counters: { accepted: number; rejected: number; ignored: number }
  os: string
  now: string
}

const { t } = useI18n()
// 列表最近一次加载失败的原因，空为成功；接口出错时列表显示错误而不是空状态
const loadError = ref('')
const { isMobile, labelPosition } = useFormLayout()
const compact = useCompactView('knock')
const rules = ref<RuleView[]>([])
const status = ref<Status>()
const settings = ref<KnockSettings>()
// serverOffset 为服务器时间与本机时间之差（毫秒），计算授权剩余时间时校正浏览器时钟
const serverOffset = ref(0)
const tick = ref(0)
const gl = useGroupedList<RuleView>({
  storageKey: 'knock.groups',
  items: () => rules.value,
  base: '/api/knock/rules',
  reload: load,
  matches: (r, q) => [r.name, r.group ?? '', r.remark, r.user, ...r.ports, ...(r.identities ?? []).map((i) => i.name)].join('\n').toLowerCase().includes(q),
})
const hmacTypes = ['sha256', 'sha384', 'sha512']
const saving = ref(false)
const editorVisible = ref(false)
const editing = ref<Rule>(empty())
const copyFrom = ref<CopySource | null>(null)
const portsText = useListText(
  () => editing.value.ports,
  (v) => (editing.value.ports = v),
  { split: (v) => v.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean), separator: ', ' },
)
const settingsVisible = ref(false)
const settingsForm = ref<KnockSettings>()
const allowText = useListText(
  () => settingsForm.value?.allowList ?? [],
  (v) => {
    if (settingsForm.value) {
      settingsForm.value.allowList = v
    }
  },
  { split: splitLines },
)
const rulesVisible = ref(false)
const rulesText = ref('')
const clientVisible = ref(false)
const clientRule = ref<RuleView>()
const clientKeys = ref<{ encKey: string; hmacKey: string }>()
// identity 为生成配置所用的身份 ID，空串为共享密钥
const client = reactive({ server: '', via: 'udp', tcpPort: 443, addr: 'resolve', fixed: '', ports: [] as string[], identity: '' })
// maxIdentities 与后端每条规则的身份上限一致
const maxIdentities = 64
// openIdentities 为编辑框中展开了密钥的身份；uidOf 为身份草稿分配列表 key（新身份还没有 ID，不能用 id）
const openIdentities = shallowReactive(new Set<Identity>())
const uids = new WeakMap<Identity, number>()
let nextUid = 0

// 每 5 秒刷新状态与授权；剩余时间每秒更新
usePolling(load, 5000)
usePolling(() => {
  tick.value++
}, 1000)

function empty(): Rule {
  return {
    name: '',
    enable: true,
    ports: ['tcp/22'],
    firewall: true,
    encKey: '',
    hmacKey: '',
    hmacType: 'sha256',
    timeout: 30,
    maxTimeout: 300,
    user: '',
    requireAddr: false,
    identities: [],
    sharedKeyOff: false,
    singleAddr: false,
    remark: '',
    group: '',
  }
}

function uidOf(id: Identity) {
  let n = uids.get(id)
  if (n === undefined) {
    n = ++nextUid
    uids.set(id, n)
  }
  return n
}

function addIdentity() {
  const list = (editing.value.identities ??= [])
  // 默认名称“成员 N”：取不重复的最小序号，用户再改成真实的人名或设备名
  const names = new Set(list.map((x) => x.name.trim().toLowerCase()))
  let n = list.length + 1
  while (names.has(t('knock.identity.defaultName', { n }).toLowerCase())) {
    n++
  }
  list.push({ name: t('knock.identity.defaultName', { n }), enable: true, encKey: '', hmacKey: '', remark: '' })
}

function removeIdentity(i: number) {
  const list = editing.value.identities ?? []
  const [id] = list.splice(i, 1)
  if (id) {
    openIdentities.delete(id)
  }
}

function toggleIdentity(id: Identity) {
  if (openIdentities.has(id)) {
    openIdentities.delete(id)
  } else {
    openIdentities.add(id)
  }
}

// identitySecret 返回已保存身份的密钥明文读取函数；新身份与复制中的规则没有明文可读
function identitySecret(id: Identity, field: 'encKey' | 'hmacKey') {
  return secretOf(editing.value.key && !copyFrom.value && id.id && `/api/knock/rules/${editing.value.key}/secrets`, `identities.${id.id}.${field}`)
}

// identityTitle 为卡片上身份标签的悬停说明：敲门次数与最近一次
function identityTitle(r: RuleView, id: Identity) {
  const st = id.id ? r.identityStats?.[id.id] : undefined
  const parts = [t('knock.rule.knockStats', { ok: st?.knocks ?? 0, bad: st?.rejected ?? 0 })]
  if (st?.lastAt) {
    parts.push(t('knock.rule.lastKnock', { addr: st.lastAddr, time: formatTime(st.lastAt) }))
  }
  if (!id.enable) {
    parts.unshift(t('knock.identity.disabled'))
  }
  return parts.join('\n')
}

async function load() {
  try {
    const [r, s, st] = await Promise.all([
      api.get<RuleView[]>('/api/knock/rules'),
      api.get<Status>('/api/knock/status'),
      api.get<KnockSettings>('/api/knock/settings'),
    ])
    rules.value = r
    status.value = s
    settings.value = st
    serverOffset.value = new Date(s.now).getTime() - Date.now()
    loadError.value = ''
  } catch (e) {
    loadError.value = errorText(e)
  }
}

const listenBadge = computed((): { type: StatusType; text: string } => {
  const s = status.value
  if (!s) {
    return { type: 'pending', text: '' }
  }
  if (s.listenError) {
    return { type: 'error', text: t('knock.status.listenFailed', { port: s.port }) }
  }
  if (s.listening) {
    return { type: 'running', text: t('knock.status.listening', { port: s.port }) }
  }
  if (!s.port) {
    return { type: 'info', text: t('knock.status.udpOff') }
  }
  // 启用的规则都没有端口（只经网页登录放行）时不需要接收敲门报文
  if (rules.value.some((r) => r.enable)) {
    return { type: 'info', text: t('knock.status.webOnly') }
  }
  return { type: 'off', text: t('knock.status.noRules') }
})

const listenProblem = computed(() => (status.value?.listenError ? t('knock.status.listenError', { err: status.value.listenError }) : ''))

const fwBadge = computed((): { type: StatusType; text: string } => {
  const f = status.value?.firewall
  if (!f || !f.needed) {
    return { type: 'off', text: t('knock.status.fwNotNeeded') }
  }
  if (f.active) {
    return { type: 'running', text: t('knock.status.fwActive') }
  }
  return { type: 'warn', text: t('knock.status.fwInactive') }
})

const fwProblem = computed(() => {
  const f = status.value?.firewall
  if (!f || !f.needed || f.active) {
    return null
  }
  const reason = f.reason || 'error'
  return {
    type: reason === 'os' || reason === 'unsupported' ? ('info' as const) : ('warning' as const),
    title: t(`knock.reason.${reason}.title`),
    hint: t(`knock.reason.${reason}.hint`),
  }
})

function formatTime(s?: string) {
  return s ? new Date(s).toLocaleString() : '-'
}

// remainText 返回授权剩余时间，按服务器时间计算
function remainText(until: string) {
  void tick.value
  const sec = Math.max(0, Math.round((new Date(until).getTime() - Date.now() - serverOffset.value) / 1000))
  if (sec >= 3600) {
    return t('knock.unit.hm', { h: Math.floor(sec / 3600), m: Math.floor((sec % 3600) / 60) })
  }
  if (sec >= 60) {
    return t('knock.unit.ms', { m: Math.floor(sec / 60), s: sec % 60 })
  }
  return t('knock.unit.s', { s: sec })
}

function timeoutText(r: Rule) {
  if (r.maxTimeout > 0) {
    return t('knock.rule.timeoutWithMax', { n: r.timeout, max: r.maxTimeout })
  }
  return t('knock.rule.timeoutValue', { n: r.timeout })
}

// randomKey 在浏览器中生成 n 字节随机密钥（base64），与 fwknop --key-gen 的长度一致
function randomKey(n: number) {
  const b = new Uint8Array(n)
  crypto.getRandomValues(b)
  return btoa(String.fromCharCode(...b))
}

function openEditor(r?: RuleView, duplicate = false) {
  copyFrom.value = null
  openIdentities.clear()
  if (!r) {
    editing.value = empty()
  } else {
    const draft: Rule = JSON.parse(JSON.stringify({ ...r, stats: undefined, usedBy: undefined, identityStats: undefined }))
    draft.identities ??= []
    if (duplicate) {
      // 复制的规则留空密钥，由服务端生成新密钥：共用密钥时敲门只会匹配到前一条规则
      copyFrom.value = { key: r.key, name: r.name }
      draft.key = undefined
      draft.name = uniqueName(r.name, rules.value.map((x) => x.name))
      draft.encKey = ''
      draft.hmacKey = ''
      // 复制的身份同样由服务端生成新密钥：各人的密钥只属于原规则
      for (const id of draft.identities ?? []) {
        id.encKey = ''
        id.hmacKey = ''
      }
    }
    editing.value = draft
  }
  editorVisible.value = true
}

async function save() {
  saving.value = true
  try {
    const body = { ...editing.value }
    if (body.key && !copyFrom.value) {
      await api.put(`/api/knock/rules/${body.key}`, body)
    } else {
      await createItem('/api/knock/rules', body, copyFrom.value)
    }
    editorVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggle(r: RuleView, enable: boolean) {
  try {
    await api.put(`/api/knock/rules/${r.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(r: RuleView) {
  try {
    await api.del(`/api/knock/rules/${r.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function revoke(g: Grant) {
  try {
    await api.del('/api/knock/grants', { addr: g.addr, rule: g.rule })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function openSettings() {
  if (!settings.value) {
    return
  }
  settingsForm.value = JSON.parse(JSON.stringify(settings.value))
  settingsVisible.value = true
}

async function saveSettings() {
  if (!settingsForm.value) {
    return
  }
  saving.value = true
  try {
    await api.put('/api/knock/settings', settingsForm.value)
    settingsVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function showRules() {
  try {
    rulesText.value = (await api.get<{ rules: string }>('/api/knock/firewall/rules')).rules
    rulesVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function openClient(r: RuleView) {
  clientRule.value = r
  clientKeys.value = undefined
  client.server = client.server || window.location.hostname
  client.ports = r.ports.slice(0, 1)
  if (r.requireAddr && client.addr === 'source') {
    client.addr = 'resolve'
  }
  // 默认选第一个可用的密钥：共享密钥停用时为第一个启用的身份
  client.identity = clientOptions(r)[0]?.value ?? ''
  clientVisible.value = true
  await loadClientKeys()
}

// clientOptions 返回规则可生成客户端配置的密钥：未停用的共享密钥与启用的身份
function clientOptions(r?: RuleView) {
  if (!r) {
    return []
  }
  const out: { value: string; label: string }[] = []
  if (!r.sharedKeyOff) {
    out.push({ value: '', label: t('knock.client.sharedKey') })
  }
  for (const id of r.identities ?? []) {
    if (id.enable && id.id) {
      out.push({ value: id.id, label: id.name })
    }
  }
  return out
}

const clientIdentities = computed(() => clientOptions(clientRule.value))

// loadClientKeys 读取所选身份（或共享密钥）的密钥明文
async function loadClientKeys() {
  const r = clientRule.value
  if (!r) {
    return
  }
  clientKeys.value = undefined
  const want = client.identity
  const prefix = want ? `identities.${want}.` : ''
  try {
    const m = await api.get<Record<string, string>>(`/api/knock/rules/${r.key}/secrets`)
    // 等待期间切换了身份时丢弃旧结果
    if (client.identity !== want || clientRule.value !== r) {
      return
    }
    clientKeys.value = { encKey: m?.[prefix + 'encKey'] ?? '', hmacKey: m?.[prefix + 'hmacKey'] ?? '' }
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// stanzaName 为 .fwknoprc 中的配置段名称，只保留字母数字与连字符
// 选了身份时加上身份名称，同一台电脑保存多个身份的配置段时不会重名
const stanzaName = computed(() => {
  const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '')
  const r = clientRule.value
  let name = 'gately-' + (slug(r?.name ?? '') || r?.key || 'knock')
  const id = r?.identities?.find((x) => x.id === client.identity && client.identity)
  if (id) {
    name += '-' + (slug(id.name) || id.id)
  }
  return name
})

const clientError = computed(() => {
  if (!clientIdentities.value.length) {
    return t('knock.client.noKeys')
  }
  if (!clientKeys.value) {
    return t('knock.client.loading')
  }
  if (!client.server.trim()) {
    return t('knock.client.needServer')
  }
  if (!client.ports.length) {
    return t('knock.client.needPorts')
  }
  if (client.addr === 'fixed' && !client.fixed.trim()) {
    return t('knock.client.needFixed')
  }
  return ''
})

const clientPort = computed(() => (client.via === 'tcp' ? client.tcpPort : settings.value?.port || 62201))

// shellQuote 为命令行参数加单引号：只含安全字符（包括 base64 的 + / =）时原样输出，否则用单引号包住
function shellQuote(s: string) {
  return /^[A-Za-z0-9_./:+=@%-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`
}

const commandText = computed(() => {
  const r = clientRule.value
  const k = clientKeys.value
  if (!r || !k) {
    return ''
  }
  const args = ['fwknop', '-A', client.ports.join(','), '-D', shellQuote(client.server.trim()), '-p', String(clientPort.value)]
  if (client.via === 'tcp') {
    args.push('-P', 'tcp')
  }
  if (client.addr === 'resolve') {
    args.push('-R')
  } else if (client.addr === 'source') {
    args.push('-s')
  } else {
    args.push('-a', shellQuote(client.fixed.trim()))
  }
  if (r.user) {
    args.push('-U', shellQuote(r.user))
  }
  // 密钥由服务端规范化为标准 base64，这里仍按参数加引号，防止手改配置等情况下带入空白或特殊字符
  args.push('--use-hmac', '--key-base64-rijndael', shellQuote(k.encKey), '--key-base64-hmac', shellQuote(k.hmacKey))
  if (r.hmacType !== 'sha256') {
    args.push('--hmac-digest-type', r.hmacType)
  }
  return args.join(' ')
})

const rcText = computed(() => {
  const r = clientRule.value
  const k = clientKeys.value
  if (!r || !k) {
    return ''
  }
  const lines = [`[${stanzaName.value}]`, `SPA_SERVER          ${client.server.trim()}`, `SPA_SERVER_PORT     ${clientPort.value}`, `ACCESS              ${client.ports.join(',')}`]
  if (client.via === 'tcp') {
    lines.push('SPA_SERVER_PROTO    tcp')
  }
  if (client.addr === 'resolve') {
    lines.push('ALLOW_IP            resolve')
  } else if (client.addr === 'source') {
    lines.push('ALLOW_IP            source')
  } else {
    lines.push(`ALLOW_IP            ${client.fixed.trim()}`)
  }
  if (r.user) {
    lines.push(`SPOOF_USER          ${r.user}`)
  }
  lines.push(`KEY_BASE64          ${k.encKey}`, `HMAC_KEY_BASE64     ${k.hmacKey}`, 'USE_HMAC            Y')
  if (r.hmacType !== 'sha256') {
    lines.push(`HMAC_DIGEST_TYPE    ${r.hmacType.toUpperCase()}`)
  }
  return lines.join('\n')
})

onMounted(load)
</script>

<style scoped>
.status-card {
  margin-bottom: 16px;
}
.head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.status-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}
.status-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.status-label {
  min-width: 96px;
  white-space: nowrap;
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.status-alert {
  margin-bottom: 12px;
}
.err-detail {
  margin-top: 4px;
  font-size: 12px;
  word-break: break-all;
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
.muted {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.form-help.warn {
  color: var(--el-color-warning);
}
.empty-hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.grants {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 10px;
}
.grant {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  min-width: 0;
}
.grant-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.grant-addr {
  font-weight: 600;
  word-break: break-all;
}
.grant-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
}
.remain {
  color: var(--el-color-success);
  font-variant-numeric: tabular-nums;
}
.grant-revoke {
  align-self: flex-end;
}
.used-by {
  word-break: break-all;
}
.ident-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.ident-count {
  margin-left: 6px;
  font-variant-numeric: tabular-nums;
  opacity: 0.7;
}
.ident-off {
  text-decoration: line-through;
}
.ident-tag :deep(.el-tag__content) {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.identities {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  min-width: 0;
}
.identity {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  box-sizing: border-box;
}
.identity-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.identity-head .el-button + .el-button {
  margin-left: 0;
}
.identity-name {
  flex: 1;
  min-width: 0;
}
.identity-keys {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}
.key-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.4;
}
.client-btn {
  margin-left: auto;
}
.key-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.key-row > :first-child {
  flex: 1;
  min-width: 0;
}
.w-200 {
  width: 200px;
}
.unit {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
}
.tcp-port {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.fixed-ip {
  margin-top: 8px;
  max-width: 260px;
}
.snippet-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 12px 0 6px;
  font-weight: 600;
}
.snippet {
  margin: 0;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.rules {
  margin: 0;
  max-height: 65vh;
  overflow: auto;
  font-size: 12px;
  white-space: pre;
}
@media (max-width: 767.98px) {
  .stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .grants {
    grid-template-columns: 1fr;
  }
  .key-row {
    flex-direction: column;
  }
}
</style>
