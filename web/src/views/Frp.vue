<template>
  <div>
    <PageHeader :title="t('frp.title')" :desc="t('frp.desc')" :icon="Waypoints">
      <ResetConfigButton base="/api/frp" :desc="t('components.resetConfig.frp')" @done="onReset" />
      <el-button :icon="FileUp" @click="openImport">{{ t('frp.importConfig') }}</el-button>
    </PageHeader>

    <el-tabs v-model="tab">
      <el-tab-pane :label="t('frp.clientTab')" name="client">
        <div class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Plug :size="16" />{{ t('frp.clientInstances') }} <span v-if="settings?.frpc.version" class="g-card-desc">{{ settings.frpc.version }}</span></div>
            <div class="head-actions">
              <ListTools :gl="glClients" size="small" />
              <CompactToggle v-model="compactClients" size="small" />
              <el-button type="primary" size="small" :icon="Plus" @click="openClient()">{{ t('frp.addClient') }}</el-button>
            </div>
          </div>
          <div class="g-card-body">
            <GroupedList :gl="glClients" :compact="compactClients" grid :empty-text="t('frp.noClients')" :empty-image-size="60">
              <template #item="{ it, item: c, sorting, bind }">
                <EntityCard v-bind="bind" :off="!c.enable" :compact="compactClients || sorting">
                  <template v-if="sorting" #lead>
                    <SortCheck :gl="glClients" :id="it.id" />
                  </template>
                  <template #title>{{ c.name }}</template>
                  <template #sub><span class="mono">{{ hostPort(c.serverAddr || t('frp.notFilled'), c.serverPort) }}</span><template v-if="c.remark"> · {{ c.remark }}</template></template>
                  <template #status><StatusBadge v-bind="instanceBadge(c)" /></template>
                  <Field :label="t('frp.protocol')" mono>{{ c.protocol || 'tcp' }}</Field>
                  <Field v-if="c.user" :label="t('frp.user')" mono>{{ c.user }}</Field>
                  <Field :label="t('frp.proxies')" mono>{{ proxyStats(c.key) }}</Field>
                  <template v-if="sorting" #actions>
                    <SortMoves :gl="glClients" :it="it" />
                  </template>
                  <template v-else #actions>
                    <el-switch :model-value="c.enable" size="small" @change="(v: boolean | string | number) => toggleInstance('clients', c.key, Boolean(v))" />
                    <el-button text size="small" :icon="RefreshCw" :disabled="!c.enable" @click="restart('clients', c.key)">{{ t('common.restart') }}</el-button>
                    <el-button text size="small" :icon="Pencil" @click="openClient(c)">{{ t('common.edit') }}</el-button>
                    <el-popconfirm :title="proxiesOf(c.key).length ? t('frp.confirmDeleteClientProxies', { count: proxiesOf(c.key).length }) : t('frp.confirmDeleteClient')" width="260" @confirm="removeInstance('clients', c.key)">
                      <template #reference>
                        <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </EntityCard>
              </template>
            </GroupedList>
          </div>
        </div>

        <div class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><ArrowRightLeft :size="16" />{{ t('frp.proxies') }} <span class="g-card-desc">{{ t('frp.proxiesDesc') }}</span></div>
            <div class="head-actions">
              <ListTools :gl="glProxies" size="small" :placeholder="t('frp.proxyFilterPlaceholder')" />
              <el-select v-if="clients.length > 1" v-model="proxyFilter" size="small" class="w-160" :disabled="glProxies.sortMode.value" :empty-values="[null, undefined]">
                <el-option :label="t('frp.allClients')" value="" />
                <el-option v-for="c in clients" :key="c.key" :label="c.name" :value="c.key" />
              </el-select>
              <CompactToggle v-model="compactProxies" size="small" />
              <el-tooltip :content="t('frp.needClient')" :disabled="clients.length > 0" placement="top">
                <span><el-button size="small" :icon="Plus" :disabled="!clients.length" @click="openProxy()">{{ t('frp.addProxy') }}</el-button></span>
              </el-tooltip>
            </div>
          </div>
          <div class="g-card-body">
            <GroupedList :gl="glProxies" :compact="compactProxies" grid :empty-text="t('frp.noProxies')" :empty-image-size="60">
              <template #item="{ it, item: p, sorting, bind }">
                <EntityCard v-bind="bind" :off="!p.enable || !clientOf(p)?.enable" :compact="compactProxies || sorting">
                  <template v-if="sorting" #lead>
                    <SortCheck :gl="glProxies" :id="it.id" />
                  </template>
                  <template #title>{{ p.name }}</template>
                  <template v-if="p.remark" #sub>{{ p.remark }}</template>
                  <template #status>
                    <el-tag v-if="clients.length > 1" size="small" effect="plain">{{ clientOf(p)?.name ?? '—' }}</el-tag>
                    <el-tag size="small" type="info">{{ p.type }}</el-tag>
                  </template>
                  <Field :label="t('frp.local')" mono>{{ p.localIp }}:{{ p.localPort }}</Field>
                  <Field v-if="p.type === 'tcp' || p.type === 'udp'" :label="t('frp.remotePort')" mono>{{ p.remotePort }}</Field>
                  <Field v-else-if="p.type === 'http' || p.type === 'https'" :label="t('frp.domain')" mono>
                    <div v-for="d in p.customDomains" :key="d">{{ d }}</div>
                    <div v-if="p.subdomain">{{ t('frp.subdomainSuffix', { sub: p.subdomain }) }}</div>
                  </Field>
                  <template v-if="sorting" #actions>
                    <SortMoves :gl="glProxies" :it="it" />
                  </template>
                  <template v-else #actions>
                    <el-switch :model-value="p.enable" size="small" @change="(v: boolean | string | number) => toggleProxy(p, Boolean(v))" />
                    <el-button text size="small" :icon="Pencil" @click="openProxy(p)">{{ t('common.edit') }}</el-button>
                    <el-popconfirm :title="t('frp.confirmDeleteProxy')" @confirm="removeProxy(p)">
                      <template #reference>
                        <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </EntityCard>
              </template>
            </GroupedList>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('frp.serverTab')" name="server">
        <div class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Server :size="16" />{{ t('frp.serverInstances') }} <span v-if="settings?.frps.version" class="g-card-desc">{{ settings.frps.version }}</span></div>
            <div class="head-actions">
              <ListTools :gl="glServers" size="small" />
              <CompactToggle v-model="compactServers" size="small" />
              <el-button type="primary" size="small" :icon="Plus" @click="openServer()">{{ t('frp.addServer') }}</el-button>
            </div>
          </div>
          <div class="g-card-body">
            <GroupedList :gl="glServers" :compact="compactServers" grid :empty-text="t('frp.noServers')" :empty-image-size="60">
              <template #item="{ it, item: s, sorting, bind }">
                <EntityCard v-bind="bind" :off="!s.enable" :compact="compactServers || sorting">
                  <template v-if="sorting" #lead>
                    <SortCheck :gl="glServers" :id="it.id" />
                  </template>
                  <template #title>{{ s.name }}</template>
                  <template #sub><span class="mono">{{ hostPort(s.bindAddr, s.bindPort) }}</span><template v-if="s.remark"> · {{ s.remark }}</template></template>
                  <template #status><StatusBadge v-bind="instanceBadge(s)" /></template>
                  <Field v-if="s.vhostHttpPort" :label="t('frp.httpPort')" mono>{{ s.vhostHttpPort }}</Field>
                  <Field v-if="s.vhostHttpsPort" :label="t('frp.httpsPort')" mono>{{ s.vhostHttpsPort }}</Field>
                  <Field v-if="s.subdomainHost" :label="t('frp.subdomainHost')" mono>{{ s.subdomainHost }}</Field>
                  <Field v-if="s.allowPorts" :label="t('frp.allowPorts')" mono>{{ s.allowPorts }}</Field>
                  <Field v-if="s.dashboardPort" :label="t('frp.dashboard')" mono>{{ hostPort(s.dashboardAddr, s.dashboardPort) }}</Field>
                  <template v-if="sorting" #actions>
                    <SortMoves :gl="glServers" :it="it" />
                  </template>
                  <template v-else #actions>
                    <el-switch :model-value="s.enable" size="small" @change="(v: boolean | string | number) => toggleInstance('servers', s.key, Boolean(v))" />
                    <el-button text size="small" :icon="RefreshCw" :disabled="!s.enable" @click="restart('servers', s.key)">{{ t('common.restart') }}</el-button>
                    <el-button text size="small" :icon="Pencil" @click="openServer(s)">{{ t('common.edit') }}</el-button>
                    <el-popconfirm :title="t('frp.confirmDeleteServer')" @confirm="removeInstance('servers', s.key)">
                      <template #reference>
                        <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </EntityCard>
              </template>
            </GroupedList>
            <div class="form-help">{{ t('frp.firewallHelp') }}</div>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Terminal :size="16" />{{ t('frp.program') }}</div>
        <el-button size="small" :icon="Save" :loading="savingSettings" @click="saveSettings">{{ t('common.save') }}</el-button>
      </div>
      <div class="g-card-body">
        <el-form label-width="110px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('frp.frpcPath')">
            <el-input v-model="frpcPath" :placeholder="settings?.frpc.binary || t('frp.autoFind')" class="mono" />
            <div v-if="settings?.frpc.error && clients.some((c) => c.enable)" class="form-help warn">{{ settings.frpc.error }}</div>
          </el-form-item>
          <el-form-item :label="t('frp.frpsPath')">
            <el-input v-model="frpsPath" :placeholder="settings?.frps.binary || t('frp.autoFind')" class="mono" />
            <div v-if="settings?.frps.error && servers.some((s) => s.enable)" class="form-help warn">{{ settings.frps.error }}</div>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <ProgramInstall :key="resetCount" base="/api/frp" :active-paths="[settings?.frpc.binary, settings?.frps.binary]" @changed="load" />

    <LogCard url="/api/frp/logs" clearable />

    <!-- frpc 实例编辑 -->
    <el-dialog v-model="clientVisible" :title="editingClient.key ? t('frp.editClient') : t('frp.addClient')" width="560px" :fullscreen="isMobile">
      <el-form :model="editingClient" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editingClient.name" :placeholder="t('frp.clientNamePlaceholder')" /></el-form-item>
        <GroupInput v-model="editingClient.group" :groups="glClients.groupNames.value" />
        <el-form-item :label="t('common.enable')"><el-switch v-model="editingClient.enable" /></el-form-item>
        <el-form-item :label="t('frp.serverAddr')"><el-input v-model="editingClient.serverAddr" :placeholder="t('frp.serverAddrPlaceholder')" /></el-form-item>
        <el-form-item :label="t('frp.serverPort')"><el-input-number v-model="editingClient.serverPort" :min="1" :max="65535" /></el-form-item>
        <el-form-item :label="t('frp.token')">
          <SecretInput v-model="editingClient.token" :reveal="secretOf(editingClient.key && `/api/frp/clients/${editingClient.key}/secrets`, 'token')" :placeholder="t('frp.tokenClientPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('frp.user')"><el-input v-model="editingClient.user" :placeholder="t('frp.userPlaceholder')" /></el-form-item>
        <el-form-item :label="t('frp.protocol')">
          <el-select v-model="editingClient.protocol" class="w-200" :empty-values="[null, undefined]">
            <el-option :label="t('frp.tcpDefault')" value="" />
            <el-option label="kcp" value="kcp" />
            <el-option label="quic" value="quic" />
            <el-option label="websocket" value="websocket" />
            <el-option label="wss" value="wss" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editingClient.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="clientVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="savingInstance" @click="saveClient">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- frps 实例编辑 -->
    <el-dialog v-model="serverVisible" :title="editingServer.key ? t('frp.editServer') : t('frp.addServer')" width="640px" :fullscreen="isMobile">
      <el-form :model="editingServer" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editingServer.name" /></el-form-item>
        <GroupInput v-model="editingServer.group" :groups="glServers.groupNames.value" />
        <el-form-item :label="t('common.enable')"><el-switch v-model="editingServer.enable" /></el-form-item>
        <el-form-item :label="t('frp.bindAddr')">
          <el-input v-model="editingServer.bindAddr" :placeholder="t('frp.allAddrs')" class="w-200 mono" />
          <div class="form-help">{{ t('frp.bindAddrHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('frp.bindPort')"><el-input-number v-model="editingServer.bindPort" :min="1" :max="65535" /></el-form-item>
        <el-form-item :label="t('frp.token')">
          <SecretInput v-model="editingServer.token" :reveal="secretOf(editingServer.key && `/api/frp/servers/${editingServer.key}/secrets`, 'token')" :placeholder="t('frp.tokenServerPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('frp.proxyBindAddr')">
          <el-input v-model="editingServer.proxyBindAddr" :placeholder="t('frp.allAddrs')" class="w-200 mono" />
          <div class="form-help">{{ t('frp.proxyBindAddrHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('frp.httpPort')"><el-input-number v-model="editingServer.vhostHttpPort" :min="0" :max="65535" /><span class="help">{{ t('frp.httpPortHelp') }}</span></el-form-item>
        <el-form-item :label="t('frp.httpsPort')"><el-input-number v-model="editingServer.vhostHttpsPort" :min="0" :max="65535" /><span class="help">{{ t('frp.httpsPortHelp') }}</span></el-form-item>
        <el-form-item :label="t('frp.subdomainHost')"><el-input v-model="editingServer.subdomainHost" :placeholder="t('frp.subdomainHostPlaceholder')" /></el-form-item>
        <el-form-item :label="t('frp.allowPorts')">
          <el-input v-model="editingServer.allowPorts" :placeholder="t('frp.allowPortsPlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('frp.dashboardPort')"><el-input-number v-model="editingServer.dashboardPort" :min="0" :max="65535" /><span class="help">{{ t('frp.dashboardPortHelp') }}</span></el-form-item>
        <template v-if="editingServer.dashboardPort">
          <el-form-item :label="t('frp.dashboardAddr')"><el-input v-model="editingServer.dashboardAddr" placeholder="127.0.0.1" class="w-200" /></el-form-item>
          <el-form-item :label="t('frp.dashboardUser')"><el-input v-model="editingServer.dashboardUser" class="w-200" /></el-form-item>
          <el-form-item :label="t('frp.dashboardPwd')">
            <SecretInput v-model="editingServer.dashboardPwd" :reveal="secretOf(editingServer.key && `/api/frp/servers/${editingServer.key}/secrets`, 'dashboardPwd')" class="w-200" />
          </el-form-item>
        </template>
        <el-form-item :label="t('common.remark')"><el-input v-model="editingServer.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="serverVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="savingInstance" @click="saveServer">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importVisible" :title="tab === 'client' ? t('frp.importFrpc') : t('frp.importFrps')" width="640px" :fullscreen="isMobile">
      <el-alert type="info" :closable="false" show-icon class="block-gap" :title="t('frp.importTip')" />
      <el-form label-width="100px" :label-position="labelPosition" class="import-form">
        <el-form-item :label="t('frp.importTarget')">
          <el-select v-model="importTarget" class="w-260" :empty-values="[null, undefined]" @change="importPreview = undefined">
            <el-option :label="t('frp.newInstance')" value="" />
            <el-option v-for="it in importTargets" :key="it.key" :label="it.name" :value="it.key" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="!importTarget" :label="t('frp.instanceName')">
          <el-input v-model="importName" class="w-260" :placeholder="tab === 'client' ? t('frp.importNameClientPlaceholder') : 'frps'" />
        </el-form-item>
      </el-form>
      <div class="import-file">
        <el-button size="small" :icon="FileUp" @click="pickFile">{{ t('frp.pickFile') }}</el-button>
        <span class="help">{{ t('frp.pasteHint') }}</span>
        <input ref="fileInput" type="file" class="hidden-input" @change="onFile" />
      </div>
      <el-input v-model="importText" type="textarea" :rows="10" class="mono" :placeholder="tab === 'client' ? t('frp.pastePlaceholder') : t('frp.pastePlaceholderServer')" @input="importPreview = undefined" />
      <el-radio-group v-if="tab === 'client' && importTarget" v-model="proxyMode" class="import-mode" @change="importPreview = undefined">
        <el-radio value="append">{{ t('frp.modeAppend') }}</el-radio>
        <el-radio value="replace">{{ t('frp.modeReplace') }}</el-radio>
      </el-radio-group>
      <div v-if="importPreview" class="import-preview">
        <div class="preview-title">{{ t('frp.parseResult') }}</div>
        <div v-if="importPreview.client" class="preview-line mono">
          {{ importPreview.client.serverAddr || t('frp.notFilled') }}:{{ importPreview.client.serverPort }}
          <template v-if="importPreview.client.protocol"> · {{ importPreview.client.protocol }}</template>
          <template v-if="importPreview.client.token"> · {{ t('frp.hasToken') }}</template>
        </div>
        <div v-if="importPreview.server" class="preview-line mono">
          bind {{ hostPort(importPreview.server.bindAddr, importPreview.server.bindPort) }}
          <template v-if="importPreview.server.proxyBindAddr"> · proxy {{ importPreview.server.proxyBindAddr }}</template>
          <template v-if="importPreview.server.vhostHttpPort"> · http {{ importPreview.server.vhostHttpPort }}</template>
          <template v-if="importPreview.server.vhostHttpsPort"> · https {{ importPreview.server.vhostHttpsPort }}</template>
          <template v-if="importPreview.server.subdomainHost"> · {{ importPreview.server.subdomainHost }}</template>
          <template v-if="importPreview.server.dashboardPort"> · {{ t('frp.dashboard') }} {{ importPreview.server.dashboardAddr }}:{{ importPreview.server.dashboardPort }}</template>
        </div>
        <div v-if="tab === 'client'" class="preview-line">
          {{ t('frp.proxyCount', { count: importPreview.proxies.length }) }}
          <div class="tags">
            <el-tag v-for="p in importPreview.proxies" :key="p.name" size="small" type="info">{{ p.name }} · {{ p.type }}</el-tag>
          </div>
        </div>
        <el-alert v-for="w in importPreview.warnings" :key="w" type="warning" :title="w" :closable="false" show-icon class="preview-warn" />
        <el-alert v-if="importPreview.applied" type="success" :closable="false" show-icon :title="importDoneText" class="preview-warn" />
      </div>
      <template #footer>
        <el-button @click="importVisible = false">{{ t('common.close') }}</el-button>
        <el-button :loading="importing" :disabled="!importText.trim()" @click="runImport(false)">{{ t('frp.parsePreview') }}</el-button>
        <el-button type="primary" :loading="importing" :disabled="!importPreview || importPreview.applied" @click="runImport(true)">{{ t('common.import') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="proxyVisible" :title="editingProxy.key ? t('frp.editProxy') : t('frp.addProxy')" width="540px" :fullscreen="isMobile">
      <el-form :model="editingProxy" label-width="100px" :label-position="labelPosition">
        <el-form-item :label="t('frp.owner')">
          <el-select v-model="editingProxy.client">
            <el-option v-for="c in clients" :key="c.key" :label="c.name" :value="c.key" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.name')"><el-input v-model="editingProxy.name" /></el-form-item>
        <GroupInput v-model="editingProxy.group" :groups="glProxies.groupNames.value" />
        <el-form-item :label="t('frp.type')">
          <el-select v-model="editingProxy.type">
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
            <el-option label="http" value="http" />
            <el-option label="https" value="https" />
            <el-option :label="t('frp.stcpLabel')" value="stcp" />
            <el-option :label="t('frp.xtcpLabel')" value="xtcp" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('frp.localAddr')"><el-input v-model="editingProxy.localIp" placeholder="127.0.0.1" /></el-form-item>
        <el-form-item :label="t('frp.localPort')"><el-input-number v-model="editingProxy.localPort" :min="1" :max="65535" /></el-form-item>
        <el-form-item v-if="editingProxy.type === 'tcp' || editingProxy.type === 'udp'" :label="t('frp.remotePort')">
          <el-input-number v-model="editingProxy.remotePort" :min="1" :max="65535" />
        </el-form-item>
        <template v-if="editingProxy.type === 'http' || editingProxy.type === 'https'">
          <el-form-item :label="t('frp.customDomains')"><el-input v-model="domainsText" :placeholder="t('frp.domainsPlaceholder')" /></el-form-item>
          <el-form-item :label="t('frp.subdomain')"><el-input v-model="editingProxy.subdomain" :placeholder="t('frp.subdomainPlaceholder')" /></el-form-item>
        </template>
        <el-form-item v-if="editingProxy.type === 'stcp' || editingProxy.type === 'xtcp'" :label="t('frp.secretKey')">
          <SecretInput v-model="editingProxy.secretKey" :reveal="secretOf(editingProxy.key && `/api/frp/proxies/${editingProxy.key}/secrets`, 'secretKey')" />
          <div class="form-help">{{ t('frp.secretHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editingProxy.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="proxyVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveProxy">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ArrowRightLeft, FileUp, Pencil, Plug, Plus, RefreshCw, Save, Server, Terminal, Trash2, Waypoints } from 'lucide-vue-next'
import { api } from '../api'
import { secretOf } from '../secrets'
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
import ProgramInstall from '../components/ProgramInstall.vue'
import ResetConfigButton from '../components/ResetConfigButton.vue'
import SecretInput from '../components/SecretInput.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitCommasOrSpaces } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'

interface ClientSettings {
  serverAddr: string
  serverPort: number
  token: string
  user: string
  protocol: string
}
interface ServerSettings {
  bindAddr: string
  bindPort: number
  token: string
  proxyBindAddr: string
  vhostHttpPort: number
  vhostHttpsPort: number
  subdomainHost: string
  allowPorts: string
  dashboardPort: number
  dashboardAddr: string
  dashboardUser: string
  dashboardPwd: string
}
// Client、Server 与后端实例字段一致；保存时只提交这些字段（后端拒绝未知字段）
interface Client extends ClientSettings {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  remark: string
}
interface Server extends ServerSettings {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  remark: string
}
interface InstanceState {
  state: { running: boolean; error?: string }
  error: string
}
type ClientView = Client & InstanceState & { key: string }
type ServerView = Server & InstanceState & { key: string }
interface BinaryView {
  binary: string
  version: string
  error: string
}
interface SettingsView {
  settings: { frpcPath: string; frpsPath: string }
  frpc: BinaryView
  frps: BinaryView
}
interface Proxy {
  key?: string
  client: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  type: string
  enable: boolean
  localIp: string
  localPort: number
  remotePort: number
  customDomains: string[] | null
  subdomain: string
  secretKey: string
  remark: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

// hostPort 拼接地址与端口，IPv6 地址加方括号；地址为空时只显示端口
function hostPort(addr: string, port: number): string {
  if (!addr) {
    return String(port)
  }
  return addr.includes(':') ? `[${addr}]:${port}` : `${addr}:${port}`
}

const tab = ref('client')
const clients = ref<ClientView[]>([])
const servers = ref<ServerView[]>([])
const proxies = ref<Proxy[]>([])
const settings = ref<SettingsView>()
const frpcPath = ref('')
const frpsPath = ref('')
const savingSettings = ref(false)
const savingInstance = ref(false)
// proxyFilter 为代理列表只显示的 frpc 实例 key，空表示全部
const proxyFilter = ref('')
// 定时刷新，标签页在后台时暂停
const polling = usePolling(refresh, 5000)

// 三个列表各自的视图、搜索、分组与排序；顺序与分组的修改不会重启实例
const compactClients = useCompactView('frp.clients')
const compactServers = useCompactView('frp.servers')
const compactProxies = useCompactView('frp.proxies')
const glClients = useGroupedList<ClientView>({
  storageKey: 'frp.clients.groups',
  items: () => clients.value,
  base: '/api/frp/clients',
  reload: load,
  matches: (c, q) => [c.name, c.group ?? '', c.remark, c.serverAddr].join('\n').toLowerCase().includes(q),
})
const glServers = useGroupedList<ServerView>({
  storageKey: 'frp.servers.groups',
  items: () => servers.value,
  base: '/api/frp/servers',
  reload: load,
  matches: (sv, q) => [sv.name, sv.group ?? '', sv.remark, sv.bindAddr].join('\n').toLowerCase().includes(q),
})
// 代理可按 frpc 实例筛选；排序模式下显示全部代理，保证调整的是完整顺序
const glProxies = useGroupedList<Proxy>({
  storageKey: 'frp.proxies.groups',
  items: () => proxies.value,
  base: '/api/frp/proxies',
  reload: load,
  visible: (p) => !proxyFilter.value || p.client === proxyFilter.value,
  matches: (p, q) =>
    [p.name, p.group ?? '', p.remark, p.type, `${p.localIp}:${p.localPort}`, String(p.remotePort || ''), ...(p.customDomains ?? []), p.subdomain ?? '', clientOf(p)?.name ?? '']
      .join('\n')
      .toLowerCase()
      .includes(q),
})

function proxiesOf(key: string): Proxy[] {
  return proxies.value.filter((p) => p.client === key)
}

function clientOf(p: Proxy): ClientView | undefined {
  return clients.value.find((c) => c.key === p.client)
}

// proxyStats 返回实例的“已启用 / 全部”代理数
function proxyStats(key: string): string {
  const list = proxiesOf(key)
  return `${list.filter((p) => p.enable).length} / ${list.length}`
}

function instanceBadge(v: { enable: boolean } & InstanceState): { type: StatusType; text: string; title?: string } {
  if (!v.enable) {
    return { type: 'off', text: t('frp.disabled') }
  }
  if (v.error) {
    return { type: 'error', text: t('frp.startFailed'), title: v.error }
  }
  return v.state.running ? { type: 'running', text: t('common.running') } : { type: 'warn', text: t('frp.notRunning') }
}

async function load() {
  const [c, s, p, st] = await Promise.all([
    api.get<ClientView[]>('/api/frp/clients'),
    api.get<ServerView[]>('/api/frp/servers'),
    api.get<Proxy[]>('/api/frp/proxies'),
    api.get<SettingsView>('/api/frp/settings'),
  ])
  clients.value = c
  servers.value = s
  proxies.value = p
  settings.value = st
  frpcPath.value = st.settings.frpcPath
  frpsPath.value = st.settings.frpsPath
  if (proxyFilter.value && !c.some((x) => x.key === proxyFilter.value)) {
    proxyFilter.value = ''
  }
}

// refresh 只刷新实例运行状态，不影响正在编辑的表单
async function refresh() {
  try {
    const [c, s] = await Promise.all([api.get<ClientView[]>('/api/frp/clients'), api.get<ServerView[]>('/api/frp/servers')])
    clients.value = c
    servers.value = s
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

// resetCount 在恢复默认配置后递增，让在线安装卡片重新读取下载代理
const resetCount = ref(0)

async function onReset() {
  resetCount.value++
  await load()
}

async function saveSettings() {
  savingSettings.value = true
  try {
    await api.put('/api/frp/settings', { frpcPath: frpcPath.value, frpsPath: frpsPath.value })
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    savingSettings.value = false
  }
}

// ---- 实例 ----

const clientVisible = ref(false)
const editingClient = ref<Client>(emptyClient())
const serverVisible = ref(false)
const editingServer = ref<Server>(emptyServer())

function emptyClient(): Client {
  return { name: '', group: '', enable: true, serverAddr: '', serverPort: 7000, token: '', user: '', protocol: '', remark: '' }
}

function emptyServer(): Server {
  return {
    name: 'frps',
    group: '',
    enable: true,
    bindAddr: '',
    bindPort: 7000,
    token: '',
    proxyBindAddr: '',
    vhostHttpPort: 0,
    vhostHttpsPort: 0,
    subdomainHost: '',
    allowPorts: '',
    dashboardPort: 0,
    dashboardAddr: '127.0.0.1',
    dashboardUser: '',
    dashboardPwd: '',
    remark: '',
  }
}

// pickClient 只取实例字段，去掉运行状态
function pickClient(v: Client): Client {
  const { key, name, group, enable, serverAddr, serverPort, token, user, protocol, remark } = v
  return { key, name, group, enable, serverAddr, serverPort, token, user, protocol, remark }
}

function pickServer(v: Server): Server {
  const base = emptyServer()
  const out = { key: v.key } as Server
  for (const k of Object.keys(base) as (keyof Server)[]) {
    ;(out as unknown as Record<string, unknown>)[k] = v[k]
  }
  return out
}

function openClient(c?: ClientView) {
  editingClient.value = c ? pickClient(c) : emptyClient()
  clientVisible.value = true
}

function openServer(s?: ServerView) {
  editingServer.value = s ? pickServer(s) : emptyServer()
  serverVisible.value = true
}

async function saveClient() {
  const { key, ...body } = editingClient.value
  // 名称留空时用服务端地址
  if (!body.name.trim()) {
    body.name = body.serverAddr.trim() || 'frpc'
  }
  savingInstance.value = true
  try {
    if (key) {
      await api.put(`/api/frp/clients/${key}`, body)
    } else {
      await api.post('/api/frp/clients', body)
    }
    clientVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    savingInstance.value = false
  }
}

async function saveServer() {
  const { key, ...body } = editingServer.value
  savingInstance.value = true
  try {
    if (key) {
      await api.put(`/api/frp/servers/${key}`, body)
    } else {
      await api.post('/api/frp/servers', body)
    }
    serverVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    savingInstance.value = false
  }
}

async function toggleInstance(kind: 'clients' | 'servers', key: string, enable: boolean) {
  try {
    await api.put(`/api/frp/${kind}/${key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(kind: 'clients' | 'servers', key: string) {
  try {
    await api.post(`/api/frp/${kind}/${key}/restart`)
    await refresh()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeInstance(kind: 'clients' | 'servers', key: string) {
  try {
    await api.del(`/api/frp/${kind}/${key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// ---- 代理 ----

const proxyVisible = ref(false)
const editingProxy = ref<Proxy>(emptyProxy())

// 输入框保留原始文本，逗号、空格可正常输入（见 useListText）
const domainsText = useListText(() => editingProxy.value.customDomains, (v) => (editingProxy.value.customDomains = v), {
  split: splitCommasOrSpaces,
  separator: ', ',
})

function emptyProxy(): Proxy {
  // 新代理默认归属当前筛选的实例，未筛选时归属第一个实例
  const client = proxyFilter.value || clients.value[0]?.key || ''
  return { client, name: '', type: 'tcp', enable: true, localIp: '127.0.0.1', localPort: 22, remotePort: 6000, customDomains: [], subdomain: '', secretKey: '', remark: '' }
}

function openProxy(p?: Proxy) {
  editingProxy.value = p ? JSON.parse(JSON.stringify(p)) : emptyProxy()
  proxyVisible.value = true
}

async function saveProxy() {
  try {
    const { key, ...body } = editingProxy.value
    if (key) {
      await api.put(`/api/frp/proxies/${key}`, body)
    } else {
      await api.post('/api/frp/proxies', body)
    }
    proxyVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggleProxy(p: Proxy, enable: boolean) {
  try {
    await api.put(`/api/frp/proxies/${p.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeProxy(p: Proxy) {
  try {
    await api.del(`/api/frp/proxies/${p.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// ---- 导入 ----

interface ImportResult {
  applied: boolean
  role: string
  client?: ClientSettings
  server?: ServerSettings
  proxies: Proxy[]
  warnings: string[]
  added: number
  key?: string
}
const importVisible = ref(false)
const importText = ref('')
const proxyMode = ref('append')
// importTarget 为导入到的已有实例 key，空表示新建实例
const importTarget = ref('')
const importName = ref('')
const importing = ref(false)
const importPreview = ref<ImportResult>()
const fileInput = ref<HTMLInputElement>()
const importTargets = computed(() => (tab.value === 'client' ? clients.value : servers.value))
const importDoneText = computed(() => (tab.value === 'client' ? t('frp.importedClient', { count: importPreview.value?.added ?? 0 }) : t('frp.importedServer')))

function openImport() {
  importText.value = ''
  importTarget.value = ''
  importName.value = ''
  importPreview.value = undefined
  importVisible.value = true
}

function pickFile() {
  fileInput.value?.click()
}

// onFile 读取所选配置文件并填入文本框
async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) {
    importText.value = await file.text()
    importPreview.value = undefined
  }
  input.value = ''
}

// runImport 先预览再导入；apply 为 true 时保存并刷新页面数据
async function runImport(apply: boolean) {
  importing.value = true
  try {
    importPreview.value = await api.post<ImportResult>('/api/frp/import', {
      role: tab.value,
      content: importText.value,
      apply,
      proxyMode: proxyMode.value,
      target: importTarget.value,
      name: importName.value,
    })
    if (apply) {
      await load()
      // 导入后筛选到该实例，便于查看导入的代理
      if (tab.value === 'client' && importPreview.value.key) {
        proxyFilter.value = clients.value.length > 1 ? importPreview.value.key : ''
      }
    }
  } catch (e) {
    importPreview.value = undefined
    ElMessage.error((e as Error).message)
  } finally {
    importing.value = false
  }
}

onMounted(() => {
  load()
  polling.start()
})
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.head-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
}
.svc-form {
  max-width: 640px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.form-help.warn {
  color: var(--el-color-warning);
}
.import-form .el-form-item {
  margin-bottom: 12px;
}
.import-file {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.hidden-input {
  display: none;
}
.import-mode {
  margin-top: 10px;
}
.import-preview {
  margin-top: 14px;
}
.preview-title {
  font-weight: 500;
  margin-bottom: 6px;
}
.preview-line {
  margin-bottom: 8px;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 6px;
}
.preview-warn {
  margin-top: 6px;
}
.w-160 {
  width: 160px;
}
.w-200 {
  width: 200px;
}
.w-260 {
  width: 260px;
}
</style>
