<template>
  <div>
    <PageHeader :title="t('web.title')" :desc="t('web.desc')" :icon="Server">
      <el-button v-if="listeners.length" :icon="allListenersCollapsed ? ChevronsUpDown : ChevronsDownUp" @click="setAll(listeners.map((l) => l.key), !allListenersCollapsed)">
        {{ allListenersCollapsed ? t('common.expandAll') : t('common.collapseAll') }}
      </el-button>
      <el-input
        v-if="hasSites"
        v-model="filterText"
        class="site-filter"
        clearable
        :disabled="sortMode"
        :prefix-icon="Search"
        :placeholder="t('web.filterPlaceholder')"
      />
      <CompactToggle v-model="compact" />
      <el-button v-if="canSort" :type="sortMode ? 'primary' : 'default'" :icon="ArrowUpDown" @click="sel.toggleSortMode()">
        {{ sortMode ? t('groupSort.sortDone') : t('groupSort.sort') }}
      </el-button>
      <el-button :icon="FileInput" @click="nginxImport?.open()">{{ t('web.importNginx') }}</el-button>
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('web.globalSettings') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openListener()">{{ t('web.addListener') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="moduleOff" :title="t('web.moduleOffTitle')" :desc="t('web.moduleOffDesc')" :loading="enabling" @enable="enableModule" />

    <SortToolbar
      v-if="sortMode"
      :selected-count="sel.selected.value.size"
      :total="allSiteIds.length"
      :groups="allGroupNames"
      :busy="busy"
      @select-all="sel.setSelected(allSiteIds, true)"
      @clear="sel.clearSelection()"
      @set-group="setGroupSelected"
    />

    <div v-loading="loading" class="card-list">
      <el-empty v-if="!loading && !listeners.length" :description="t('web.empty')" />
      <EntityCard
        v-for="(l, li) in listeners"
        :key="l.key"
        :off="!l.enable || moduleOff"
        collapsible
        :collapsed="isCollapsed(l.key)"
        :collapsed-hint="t('web.sitesCollapsed', { count: siteRows(l).length })"
        @update:collapsed="(v: boolean) => setCollapsed(l.key, v)"
      >
        <template #lead>
          <el-switch :model-value="l.enable" @change="(v: string | number | boolean) => toggleListener(l, Boolean(v))" />
        </template>
        <template #title>
          <span>{{ l.name || t('web.unnamedListener') }}</span>
          <span class="tags">
            <el-tag size="small" effect="plain" class="mono">{{ l.tls ? 'https' : 'http' }}://{{ l.listenIp || '*' }}:{{ l.port }}</el-tag>
            <el-tag size="small" type="info">{{ networkText(l.network) }}</el-tag>
            <el-tag v-if="l.tls && l.plainHttp" size="small" type="info">{{ plainHttpText(l.plainHttp) }}</el-tag>
            <el-tag v-if="l.acceptProxyProtocol" size="small" type="info">{{ t('web.acceptProxyTag') }}</el-tag>
          </span>
        </template>
        <template v-if="l.remark" #sub>{{ l.remark }}</template>
        <template #status>
          <StatusBadge v-bind="listenerStatus(l)" />
        </template>
        <template #extra>
          <div class="sites" :class="{ 'is-compact': compact, 'is-sorting': sortMode }">
            <template v-for="item in displayItems(l)" :key="item.id">
            <GroupHeader
              v-if="item.kind === 'group'"
              :name="item.name"
              :count-text="t('web.groupCount', { count: item.count })"
              :collapsed="item.collapsed"
              :sorting="sortMode"
              :can-up="item.canUp"
              :can-down="item.canDown"
              :checked="item.checked"
              :indeterminate="item.indeterminate"
              :class="sel.dropClass(item.id)"
              @toggle="toggleGroup(l, item.name)"
              @rename="renameGroup(l, item.name)"
              @up="moveGroup(l, item.index, -1)"
              @down="moveGroup(l, item.index, 1)"
              @check="(v: boolean) => sel.setSelected(item.ids, v)"
              @dragover.prevent="sel.dragOver(item.id, $event, false)"
              @dragleave="sel.dragLeave(item.id)"
              @drop.prevent="onDrop(l)"
            />
            <div
              v-else
              class="site"
              :class="[{ 'is-off': !item.item.site.enable, 'is-grouped': item.grouped }, item.item.isDefault ? {} : sel.dropClass(item.id)]"
              :draggable="sortMode && !isMobile && !item.item.isDefault"
              @dragstart="onDragStart(l, item.id, $event)"
              @dragover.prevent="!item.item.isDefault && sel.dragOver(item.id, $event)"
              @dragleave="sel.dragLeave(item.id)"
              @drop.prevent="onDrop(l)"
              @dragend="sel.resetDrag()"
            >
              <div class="site-switch">
                <el-checkbox
                  v-if="sortMode && !item.item.isDefault"
                  :model-value="sel.isSelected(item.id)"
                  @change="(v: string | number | boolean) => sel.setSelected([item.id], Boolean(v))"
                />
                <el-switch v-else :model-value="item.item.site.enable" size="small" @change="(v: string | number | boolean) => toggleSite(l, item.item, Boolean(v))" />
              </div>
              <div class="site-main">
                <div v-if="item.item.isDefault" class="site-domains"><el-tag size="small" type="warning">{{ t('web.defaultSite') }}</el-tag> <span class="meta">{{ t('web.unmatchedDomains') }}</span></div>
                <div v-else class="site-domains mono">{{ item.item.site.domains.join(', ') }}</div>
                <div v-if="item.item.site.name" class="meta">{{ item.item.site.name }}</div>
              </div>
              <div class="site-target">
                <el-tag size="small" effect="plain">{{ typeText(item.item.site.type) }}</el-tag>
                <span class="mono target">{{ summary(item.item.site) }}</span>
              </div>
              <div class="site-opts">
                <StatusBadge v-if="!item.item.site.enable" type="off" :text="t('web.statusOff')" />
                <el-tag v-if="item.item.site.webAuth" size="small" type="warning">{{ t('web.webAuthTag') }}</el-tag>
                <el-tag v-else-if="item.item.site.basicAuth" size="small" type="warning">{{ t('web.basicAuthTag') }}</el-tag>
                <el-tag v-else-if="item.item.site.forwardAuth?.url" size="small" type="warning">{{ t('web.forwardAuthTag') }}</el-tag>
                <el-tag v-if="item.item.site.locations?.length" size="small" type="info">{{ t('web.locationsTag', { count: item.item.site.locations.length }) }}</el-tag>
                <el-tag v-if="item.item.site.userRoutes?.length" size="small" type="info">{{ t('web.userRoutesTag', { count: item.item.site.userRoutes.length }) }}</el-tag>
                <el-tag v-if="item.item.site.ipMode" size="small" type="info">{{ item.item.site.ipMode === 'whitelist' ? t('web.whitelist') : t('web.blacklist') }}</el-tag>
                <el-tag v-if="item.item.site.sendProxyProtocol" size="small" type="info">PROXY v2</el-tag>
                <el-tag v-if="downCount(l, item.item) > 0" size="small" type="danger">{{ t('web.upstreamsDown', { count: downCount(l, item.item) }) }}</el-tag>
                <el-tag v-if="item.item.site.insecureSkipVerify" size="small" type="info">{{ t('web.skipVerifyTag') }}</el-tag>
                <el-tag v-if="item.item.site.waf" size="small" :type="wafInstances.some((w) => w.key === item.item.site.waf) ? 'danger' : 'info'">
                  WAF{{ wafInstances.some((w) => w.key === item.item.site.waf) ? '' : t('web.wafMissingSuffix') }}
                </el-tag>
              </div>
              <div class="site-stats">
                <template v-if="item.item.site.key && l.stats?.[item.item.site.key]">
                  <span>{{ t('web.requestsCount', { count: l.stats[item.item.site.key].requests }) }}</span>
                  <span v-if="l.stats[item.item.site.key].errors" class="text-danger"> · {{ t('web.errorsCount', { count: l.stats[item.item.site.key].errors }) }}</span>
                </template>
                <span v-else class="muted">-</span>
              </div>
              <div v-if="sortMode" class="site-actions">
                <template v-if="!item.item.isDefault">
                  <GripVertical v-if="!isMobile" :size="16" class="grip" />
                  <el-button text size="small" :icon="ChevronUp" :disabled="!item.canUp" :aria-label="t('groupSort.moveUp')" :title="t('groupSort.moveUp')" @click="moveSite(l, item.id, -1)" />
                  <el-button text size="small" :icon="ChevronDown" :disabled="!item.canDown" :aria-label="t('groupSort.moveDown')" :title="t('groupSort.moveDown')" @click="moveSite(l, item.id, 1)" />
                </template>
                <span v-else class="meta">{{ t('web.defaultSiteLast') }}</span>
              </div>
              <div v-else class="site-actions">
                <el-button v-if="l.running" text size="small" :icon="BarChart3" @click="openStats(l, item.item)">{{ t('web.stats') }}</el-button>
                <el-button text size="small" :icon="Pencil" @click="openSite(l, item.item)">{{ t('common.edit') }}</el-button>
                <el-popconfirm v-if="item.item.isDefault" :title="t('web.resetDefaultConfirm')" width="260" @confirm="resetDefaultSite(l)">
                  <template #reference>
                    <el-button text size="small" type="danger" :icon="Trash2" :aria-label="t('common.reset')" />
                  </template>
                </el-popconfirm>
                <el-popconfirm v-else :title="t('web.deleteSiteConfirm')" @confirm="removeSite(l, item.item)">
                  <template #reference>
                    <el-button text size="small" type="danger" :icon="Trash2" :aria-label="t('web.deleteSite')" />
                  </template>
                </el-popconfirm>
              </div>
            </div>
            </template>
            <div v-if="!siteRows(l).length" class="meta empty-sites">{{ t('web.noSites') }}</div>
            <div v-else-if="!displayItems(l).length" class="meta empty-sites">{{ t('web.noMatchedSites') }}</div>
          </div>
        </template>
        <template v-if="sortMode" #actions>
          <el-button text size="small" :icon="ChevronUp" :disabled="li === 0" @click="moveListener(li, -1)">{{ t('groupSort.moveUp') }}</el-button>
          <el-button text size="small" :icon="ChevronDown" :disabled="li === listeners.length - 1" @click="moveListener(li, 1)">{{ t('groupSort.moveDown') }}</el-button>
        </template>
        <template v-else #actions>
          <el-button text size="small" type="primary" :icon="Plus" @click="openSite(l)">{{ t('web.addSite') }}</el-button>
          <!-- 默认站点未配置时不在站点列表中显示，这里提供配置入口 -->
          <el-button v-if="!hasDefaultRow(l)" text size="small" :icon="Shield" @click="openDefaultSite(l)">{{ t('web.setDefaultSite') }}</el-button>
          <el-button text size="small" :icon="Pencil" @click="openListener(l)">{{ t('web.editListener') }}</el-button>
          <!-- 规则的存储与执行都在入侵防护，这里只是带上监听跳过去创建 -->
          <el-button text size="small" :icon="ShieldBan" :title="t('web.probeRuleHelp')" @click="router.push({ name: 'guard', query: { webProbe: l.key } })">{{ t('web.probeRule') }}</el-button>
          <el-popconfirm :title="t('web.deleteListenerConfirm')" width="240" @confirm="removeListener(l)">
            <template #reference>
              <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
            </template>
          </el-popconfirm>
        </template>
      </EntityCard>
    </div>

    <LogCard url="/api/webservice/logs" clearable />
    <SiteStats ref="statsDialog" />

    <el-dialog v-model="listenerVisible" :title="editingListener.key ? t('web.editListener') : t('web.addListener')" width="560px" :fullscreen="isMobile">
      <el-alert v-if="!editingListener.key" :title="t('web.newListenerHint')" type="info" :closable="false" show-icon class="listener-hint" />
      <el-form :model="editingListener" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editingListener.name" /></el-form-item>
        <el-form-item :label="t('web.network')">
          <el-radio-group v-model="editingListener.network">
            <el-radio-button value="tcp">{{ t('web.networkDual') }}</el-radio-button>
            <el-radio-button value="tcp4">{{ t('web.networkIpv4') }}</el-radio-button>
            <el-radio-button value="tcp6">{{ t('web.networkIpv6') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('web.listenIp')"><el-input v-model="editingListener.listenIp" :placeholder="t('web.listenIpPlaceholder')" /></el-form-item>
        <el-form-item :label="t('web.port')" required><el-input-number v-model="editingListener.port" :min="1" :max="65535" /></el-form-item>
        <el-form-item label="HTTPS">
          <el-switch v-model="editingListener.tls" />
          <span class="help">{{ t('web.tlsHelp') }}</span>
        </el-form-item>
        <el-form-item v-if="editingListener.tls" :label="t('web.minTls')">
          <el-select v-model="editingListener.minTls" class="w-120">
            <el-option v-for="v in ['1.0', '1.1', '1.2', '1.3']" :key="v" :label="'TLS ' + v" :value="v" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="editingListener.tls" :label="t('web.plainHttp')">
          <el-radio-group v-model="editingListener.plainHttp">
            <el-radio-button value="">{{ t('web.plainHttpOff') }}</el-radio-button>
            <el-radio-button value="redirect">{{ t('web.plainHttpRedirect') }}</el-radio-button>
            <el-radio-button value="serve">{{ t('web.plainHttpServe') }}</el-radio-button>
          </el-radio-group>
          <span class="help">{{ t('web.plainHttpHelp') }}</span>
        </el-form-item>
        <el-form-item v-if="editingListener.tls" label="HTTP/3">
          <el-switch v-model="editingListener.http3" />
          <span class="help">{{ t('web.http3Help') }}</span>
        </el-form-item>
        <el-form-item :label="t('web.acceptProxy')">
          <el-switch v-model="editingListener.acceptProxyProtocol" />
          <span class="help">{{ t('web.acceptProxyHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editingListener.remark" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editingListener.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="listenerVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveListener">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="siteVisible" :title="siteIsDefault ? t('web.defaultSite') : site.key ? t('web.editSite') : t('web.addSite')" width="760px" top="5vh" :fullscreen="isMobile">
      <el-form :model="site" label-width="130px" :label-position="labelPosition">
        <el-alert v-if="siteIsDefault" :title="t('web.defaultSiteHint')" type="info" :closable="false" show-icon class="listener-hint" />
        <el-tabs v-model="siteTab">
          <el-tab-pane :label="t('web.tabBasic')" name="basic">
            <el-form-item v-if="!siteIsDefault" :label="t('web.domains')" required>
              <el-input v-model="domainsText" type="textarea" :rows="2" class="mono" :placeholder="t('web.domainsPlaceholder')" />
            </el-form-item>
            <el-form-item :label="t('common.name')"><el-input v-model="site.name" /></el-form-item>
            <GroupInput v-if="!siteIsDefault" v-model="site.group" :groups="allGroupNames" />
            <el-form-item :label="t('web.type')">
              <el-radio-group v-model="site.type">
                <el-radio-button value="proxy">{{ t('web.typeProxy') }}</el-radio-button>
                <el-radio-button value="redirect">{{ t('web.typeRedirect') }}</el-radio-button>
                <el-radio-button value="static">{{ t('web.typeStatic') }}</el-radio-button>
                <el-radio-button value="text">{{ t('web.typeText') }}</el-radio-button>
                <el-radio-button value="close">{{ t('web.typeClose') }}</el-radio-button>
              </el-radio-group>
              <div v-if="site.type === 'close'" class="form-help">{{ t('web.typeCloseHelp') }}</div>
              <div v-if="site.type === 'close'" class="form-help">{{ t('web.typeCloseLimit') }}</div>
            </el-form-item>
            <template v-if="site.type === 'proxy'">
              <el-form-item :label="t('web.targets')" required>
                <el-input v-model="targetsText" type="textarea" :rows="2" class="mono" :placeholder="t('web.targetsPlaceholder')" />
              </el-form-item>
              <el-form-item>
                <template #label>
                  <!-- 表单标签是顶端对齐的 flex 容器，文字与问号需包一层才能垂直居中 -->
                  <span class="label-with-help">{{ t('web.options') }}<ProxyOptionsHelp /></span>
                </template>
                <el-checkbox v-model="site.insecureSkipVerify">{{ t('web.optSkipVerify') }}</el-checkbox>
                <el-checkbox v-model="site.useTargetHost">{{ t('web.optTargetHost') }}</el-checkbox>
                <el-checkbox v-model="site.disableKeepAlives">{{ t('web.optNoKeepAlive') }}</el-checkbox>
                <el-checkbox v-model="site.forwardedHeaders">{{ t('web.optForwarded') }}</el-checkbox>
                <el-checkbox v-model="site.sendProxyProtocol">{{ t('web.optSendProxy') }}</el-checkbox>
                <el-checkbox v-model="site.upstreamSniFromHost">{{ t('web.optSniFromHost') }}</el-checkbox>
                <!-- 认证交给后端 Gately 时才能转发登录 Cookie，本站点开了认证就不可勾选 -->
                <el-checkbox v-model="site.passAuthCookies" :disabled="localAuthOn && !site.passAuthCookies">{{ t('web.optPassAuthCookies') }}</el-checkbox>
                <div class="form-help">{{ t('web.optSniFromHostHelp') }}</div>
                <div v-if="site.passAuthCookies" class="form-help">{{ t('web.optPassAuthCookiesHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('web.locations')">
                <div class="locations">
                  <div v-for="(loc, i) in site.locations" :key="locKey(loc)" class="location">
                    <div class="location-row">
                      <el-input v-model="loc.prefix" class="mono location-prefix" placeholder="/api/" />
                      <ListInput
                        v-model="loc.targets"
                        :split="splitCsv"
                        separator=", "
                        class="mono location-targets"
                        :placeholder="t('web.locationTargetsPlaceholder')"
                      />
                      <el-button text type="danger" :icon="Trash2" @click="site.locations.splice(i, 1)" />
                    </div>
                    <!-- 该路径追加的请求头，与站点请求头共用预设和校验 -->
                    <HeaderEditor v-model="loc.requestHeaders" direction="request" :placeholder="t('web.locationHeadersPlaceholder')" :show-help="false" />
                  </div>
                  <el-button size="small" :icon="Plus" @click="site.locations.push({ prefix: '', targets: [], requestHeaders: [] })">{{ t('web.addLocation') }}</el-button>
                  <div class="form-help">{{ t('web.locationsHelp') }}</div>
                </div>
              </el-form-item>
            </template>
            <template v-if="site.type === 'redirect'">
              <el-form-item :label="t('web.redirectTo')" required>
                <el-input v-model="site.redirectTo" placeholder="https://example.com" />
                <div class="form-help">{{ t('web.redirectHostHelp') }}</div>
                <div class="form-help">{{ t('web.redirectVarsHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('web.statusCode')">
                <el-select v-model="site.redirectCode" class="w-120">
                  <el-option v-for="c in [301, 302, 307, 308]" :key="c" :label="c" :value="c" />
                </el-select>
                <el-checkbox v-model="site.keepPath" class="inline">{{ t('web.keepPath') }}</el-checkbox>
              </el-form-item>
            </template>
            <template v-if="site.type === 'static'">
              <el-form-item :label="t('web.root')" required><el-input v-model="site.root" placeholder="/data/www" /></el-form-item>
              <el-form-item :label="t('web.listDir')"><el-switch v-model="site.listDir" /></el-form-item>
              <el-form-item :label="t('web.cleanUrls')">
                <el-switch v-model="site.cleanUrls" />
                <div class="form-help">{{ t('web.cleanUrlsHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('web.notFoundPage')"><el-input v-model="site.notFoundPage" class="w-260" placeholder="404.html" /></el-form-item>
            </template>
            <template v-if="site.type === 'text'">
              <el-form-item :label="t('web.statusCode')"><el-input-number v-model="site.statusCode" :min="100" :max="599" /></el-form-item>
              <el-form-item label="Content-Type"><ContentTypeInput v-model="site.contentType" /></el-form-item>
              <el-form-item :label="t('web.content')"><el-input v-model="site.text" type="textarea" :rows="4" /></el-form-item>
            </template>
            <el-form-item :label="t('web.accessLog')"><el-switch v-model="site.accessLog" /></el-form-item>
            <el-form-item :label="t('common.enable')"><el-switch v-model="site.enable" /></el-form-item>
          </el-tab-pane>
          <!-- 断开连接的站点不经过认证、访问控制、流量与请求头处理，只保留基本设置 -->
          <el-tab-pane v-if="site.type !== 'close'" :label="t('web.tabSecurity')" name="security">
            <el-form-item :label="t('web.auth')">
              <el-checkbox v-model="site.webAuth" :disabled="forwardAuthOn">{{ t('web.webAuth') }}</el-checkbox>
              <el-checkbox v-model="site.basicAuth" :disabled="forwardAuthOn">{{ t('web.basicAuth') }}</el-checkbox>
              <el-checkbox v-model="forwardAuthOn" :disabled="site.webAuth || site.basicAuth">{{ t('web.forwardAuth') }}</el-checkbox>
            </el-form-item>
            <template v-if="forwardAuthOn">
              <el-form-item :label="t('web.forwardAuthUrl')" required>
                <el-input v-model="site.forwardAuth.url" class="mono" placeholder="http://127.0.0.1:9091/api/authz/auth-request" />
                <div class="form-help">{{ t('web.forwardAuthUrlHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('web.forwardAuthLogin')">
                <el-input v-model="site.forwardAuth.loginUrl" class="mono" placeholder="https://auth.example.com/?rd={url}&rm={method}" />
                <div class="form-help">{{ t('web.forwardAuthLoginHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('web.forwardAuthHeaders')">
                <el-input v-model="fwdHeadersText" class="mono" placeholder="Remote-User, Remote-Groups, Remote-Email, Remote-Name" />
              </el-form-item>
            </template>
            <el-form-item v-if="site.webAuth || site.basicAuth" :label="t('web.sharedUsersSite')">
              <el-checkbox v-model="site.sharedUsers">{{ t('web.sharedUsersSiteLabel') }}</el-checkbox>
              <div class="form-help">{{ t('web.sharedUsersSiteHelp') }}</div>
            </el-form-item>
            <el-form-item v-if="site.webAuth || site.basicAuth" :label="t('web.users')">
              <el-input v-model="usersText" type="textarea" :rows="3" class="mono" :placeholder="t('web.usersPlaceholder')" />
            </el-form-item>
            <el-form-item v-if="site.webAuth" :label="t('web.thirdParty')">
              <el-input v-model="oauthUsersText" type="textarea" :rows="2" class="mono" :placeholder="t('web.oauthUsersPlaceholder')" />
              <div class="form-help">{{ t('web.providerHelp') }}</div>
            </el-form-item>
            <!-- 按用户分流与传递用户名依赖认证结果，只在反向代理站点启用认证后显示 -->
            <template v-if="site.type === 'proxy' && localAuthOn">
              <el-form-item :label="t('web.userRoutes')">
                <div class="locations">
                  <div v-for="(ur, i) in site.userRoutes" :key="locKey(ur)" class="location">
                    <div class="location-row">
                      <ListInput v-model="ur.users" :split="splitCsv" separator=", " class="mono user-route-users" :placeholder="t('web.userRouteUsersPlaceholder')" />
                      <ListInput v-model="ur.targets" :split="splitCsv" separator=", " class="mono location-targets" :placeholder="t('web.userRouteTargetsPlaceholder')" />
                      <el-button text type="danger" :icon="Trash2" :aria-label="t('common.delete')" @click="site.userRoutes.splice(i, 1)" />
                    </div>
                  </div>
                  <el-button size="small" :icon="Plus" @click="site.userRoutes.push({ users: [], targets: [] })">{{ t('web.addUserRoute') }}</el-button>
                  <div class="form-help">{{ t('web.userRoutesHelp') }}</div>
                </div>
              </el-form-item>
              <el-form-item :label="t('web.userHeader')">
                <el-input v-model="site.userHeader" class="mono w-260" placeholder="X-Remote-User" />
                <div class="form-help">{{ t('web.userHeaderHelp') }}</div>
              </el-form-item>
            </template>
            <el-form-item :label="t('web.ipAccess')">
              <el-radio-group v-model="site.ipMode">
                <el-radio-button value="">{{ t('web.off') }}</el-radio-button>
                <el-radio-button value="whitelist">{{ t('web.whitelist') }}</el-radio-button>
                <el-radio-button value="blacklist">{{ t('web.blacklist') }}</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item v-if="site.ipMode" :label="t('web.ipRange')">
              <el-input v-model="ipText" type="textarea" :rows="3" class="mono" :placeholder="t('web.ipRangePlaceholder')" />
            </el-form-item>
            <el-form-item v-if="site.ipMode" :label="t('web.ipSets')">
              <el-select v-model="site.ipSets" multiple filterable :placeholder="t('web.ipSetsPlaceholder')" :no-data-text="t('web.ipSetsEmpty')" class="w-full">
                <el-option v-for="o in ipLists" :key="o.key" :label="o.name" :value="o.key">
                  <span>{{ o.name }}</span>
                  <span v-if="!o.loaded" class="opt-warn">{{ t('web.ipSetNotLoaded') }}</span>
                </el-option>
              </el-select>
              <div class="form-help">{{ site.ipMode === 'whitelist' ? t('web.ipSetsWhitelistHelp') : t('web.ipSetsHelp') }}</div>
            </el-form-item>
            <SiteAccess v-model:site="site" :tls="siteTls" :port="sitePort" :ip-lists="ipLists" />
            <el-form-item :label="t('web.denyAction')">
              <el-radio-group v-model="site.denyClose">
                <el-radio-button :value="false">{{ t('web.deny403') }}</el-radio-button>
                <el-radio-button :value="true">{{ t('web.denyClose') }}</el-radio-button>
              </el-radio-group>
              <div class="form-help">{{ t('web.denyActionHelp') }}</div>
            </el-form-item>
            <el-form-item :label="t('web.wafInstance')">
              <el-select v-model="site.waf" clearable :placeholder="t('web.wafNone')" class="w-260">
                <el-option v-for="w in wafInstances" :key="w.key" :label="w.name" :value="w.key" />
                <el-option v-if="site.waf && !wafInstances.some((w) => w.key === site.waf)" :label="site.waf + t('web.wafMissingSuffix')" :value="site.waf" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('web.cors')"><el-switch v-model="site.cors" /></el-form-item>
          </el-tab-pane>
          <el-tab-pane v-if="site.type !== 'close'" :label="t('web.tabTraffic')" name="traffic">
            <SiteTraffic v-model:site="site" />
          </el-tab-pane>
          <el-tab-pane v-if="site.type !== 'close'" :label="t('web.tabHeaders')" name="headers">
            <el-form-item :label="t('web.requestHeaders')">
              <HeaderEditor v-model="site.requestHeaders" direction="request" :placeholder="t('web.requestHeadersPlaceholder')" />
            </el-form-item>
            <el-form-item :label="t('web.responseHeaders')">
              <HeaderEditor v-model="site.responseHeaders" direction="response" :placeholder="t('web.responseHeadersPlaceholder')" />
            </el-form-item>
          </el-tab-pane>
        </el-tabs>
      </el-form>
      <template #footer>
        <el-button @click="siteVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveSite">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settingsVisible" :title="t('web.settingsTitle')" width="640px" :fullscreen="isMobile">
      <el-form v-if="settings" :model="settings" label-width="150px" :label-position="labelPosition">
        <el-form-item :label="t('web.enableModule')"><el-switch v-model="settings.enable" /></el-form-item>
        <el-form-item :label="t('web.trustedProxies')">
          <el-input v-model="trustedText" type="textarea" :rows="4" class="mono" :placeholder="t('web.trustedPlaceholder')" />
        </el-form-item>
        <el-divider content-position="left">{{ t('web.thirdPartyDivider') }}</el-divider>
        <el-form-item :label="t('web.callbackBase')">
          <el-input v-if="settings" v-model="settings.oauthCallbackBase" :placeholder="t('web.callbackPlaceholder')" />
          <div class="form-help">{{ t('web.callbackHelp', { url: (settings?.oauthCallbackBase || 'https://...').replace(/\/$/, '') + '/__gately_auth/oauth/callback' }) }}</div>
        </el-form-item>
        <OAuthProviders v-if="settings" v-model="settings.oauthProviders" secrets-url="/api/webservice/settings/secrets/oauth" />
        <el-divider content-position="left">{{ t('web.sharedUsersDivider') }}</el-divider>
        <!-- 共享用户与 OIDC 身份提供方在「认证用户」页面集中管理，这里只显示摘要 -->
        <el-form-item :label="t('web.sharedUsers')">
          <div class="users-summary">
            <span>{{ t('web.usersSummary', { users: settings.authUsers.length, clients: settings.oidc.clients.length }) }}</span>
            <el-tag size="small" :type="settings.oidc.enable ? 'success' : 'info'">{{ settings.oidc.enable ? t('web.oidcOn') : t('web.oidcOff') }}</el-tag>
            <router-link to="/webusers" class="users-link" @click="settingsVisible = false">{{ t('web.manageUsers') }}</router-link>
          </div>
          <div class="form-help">{{ t('web.manageUsersHelp') }}</div>
        </el-form-item>
        <AccessSettings v-if="settings" v-model:auto-ban-seconds="settings.autoBanSeconds" />
        <IPListsManager v-if="settings" @changed="loadIPLists" />
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
    <NginxImport ref="nginxImport" @imported="load" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowUpDown,
  BarChart3,
  ChevronDown,
  ChevronsDownUp,
  ChevronsUpDown,
  ChevronUp,
  FileInput,
  GripVertical,
  Pencil,
  Plus,
  Search,
  Server,
  Shield,
  ShieldBan,
  SlidersHorizontal,
  Trash2,
} from 'lucide-vue-next'
import { api, type OAuthProvider } from '../api'
import GroupHeader from '../components/GroupHeader.vue'
import SortToolbar from '../components/SortToolbar.vue'
import GroupInput from '../components/GroupInput.vue'
import { useGroupRename, useSortSelect } from '../composables/useGroupSort'
import { buildSections, displayItems as displayGroupItems, dropTarget, flatten, moveInto, moveSection, moveStep, renameSection, type GroupItem, type Section } from '../groupSort'
import PageHeader from '../components/PageHeader.vue'
import ModuleBanner from '../components/ModuleBanner.vue'
import CompactToggle from '../components/CompactToggle.vue'
import EntityCard from '../components/EntityCard.vue'
import { useCompactView } from '../composables/useCompactView'
import { useCollapsed } from '../composables/useCollapsed'
import OAuthProviders from '../components/OAuthProviders.vue'
import { oidcFromAPI, type OIDCSettings } from '../oidc'
import { type AuthUser, fromAPI } from '../sharedUsers'
import AccessSettings from '../components/AccessSettings.vue'
import IPListsManager from '../components/IPListsManager.vue'
import NginxImport from '../components/NginxImport.vue'
import SiteTraffic, { type Rewrite } from '../components/SiteTraffic.vue'
import SiteStats from '../components/SiteStats.vue'
import SiteAccess, { type PathRule, type ShareLink } from '../components/SiteAccess.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLines } from '../composables/useListText'
import ListInput from '../components/ListInput.vue'
import HeaderEditor from '../components/HeaderEditor.vue'
import ContentTypeInput from '../components/ContentTypeInput.vue'
import ProxyOptionsHelp from '../components/ProxyOptionsHelp.vue'
import { usePolling } from '../composables/usePolling'

interface Site {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  domains: string[]
  type: string
  targets: string[] | null
  insecureSkipVerify: boolean
  useTargetHost: boolean
  disableKeepAlives: boolean
  sendProxyProtocol: boolean
  forwardedHeaders: boolean
  // 连接 HTTPS 后端时使用访客域名作为 SNI（后端按 SNI 分流时需要）
  upstreamSniFromHost: boolean
  // 后端是负责认证的另一台 Gately 时，把 Gately 登录 Cookie 原样转发给它
  passAuthCookies: boolean
  locations: Location[]
  userRoutes: UserRoute[]
  userHeader: string
  redirectTo: string
  redirectCode: number
  keepPath: boolean
  root: string
  listDir: boolean
  cleanUrls: boolean
  notFoundPage: string
  text: string
  contentType: string
  statusCode: number
  basicAuth: boolean
  webAuth: boolean
  // 同时接受全局设置中的共享用户（支持 TOTP 与单点登录）
  sharedUsers: boolean
  users: string[] | null
  oauthUsers: string[] | null
  ipMode: string
  ipList: string[] | null
  // ipSets 为引用的订阅 IP 列表 key
  ipSets: string[] | null
  requestHeaders: string[] | null
  responseHeaders: string[] | null
  cors: boolean
  accessLog: boolean
  waf: string
  remark: string
  authOptional: boolean
  forwardAuth: { url: string; loginUrl: string; copyHeaders: string[] | null }
  countryMode: string
  countries: string[] | null
  // denyClose 为 true 时访问控制拒绝的请求直接断开连接（记为 444），不返回 403
  denyClose: boolean
  rules: PathRule[]
  shareLinks: ShareLink[]
  lbPolicy: string
  health: { enable: boolean; mode: string; path: string; interval: number; timeout: number; fails: number; passes: number }
  compress: boolean
  rateLimit: { enable: boolean; rate: number; burst: number; maxConns: number }
  cache: { enable: boolean; ttl: number; maxMb: number }
  rewrites: Rewrite[]
}

// Location 把某个路径前缀分到另一组后端
interface Location {
  prefix: string
  targets: string[] | null
  requestHeaders: string[] | null
}

// UserRoute 把列出的认证用户分到另一组后端
interface UserRoute {
  users: string[] | null
  targets: string[] | null
}

interface Listener {
  key?: string
  name: string
  enable: boolean
  network: string
  listenIp: string
  port: number
  tls: boolean
  minTls: string
  http3: boolean
  // 开启 HTTPS 后同端口收到明文 HTTP 的处理方式：'' 不接受，redirect 跳转到 HTTPS，serve 直接服务
  plainHttp?: '' | 'redirect' | 'serve'
  acceptProxyProtocol: boolean
  sites: Site[] | null
  default: Site
  remark: string
}

interface ListenerView extends Listener {
  key: string
  running: boolean
  error?: string
  stats: Record<string, { requests: number; active: number; errors: number }>
  upstreams?: Record<string, { url: string; healthy: boolean }[]>
}

interface SiteRow {
  site: Site
  index: number
  isDefault: boolean
}

const { t } = useI18n()
const router = useRouter()
const compact = useCompactView('web')
const listeners = ref<ListenerView[]>([])
// 每个监听下的站点列表可单独收起，状态按监听 key 持久化
const { isCollapsed, setCollapsed, setAll, allCollapsed } = useCollapsed('web')
const allListenersCollapsed = allCollapsed(() => listeners.value.map((l) => l.key))
const wafInstances = ref<{ key: string; name: string }[]>([])
const nginxImport = ref<InstanceType<typeof NginxImport>>()
// ipLists 为订阅的 IP 列表，供站点与路径规则的访问控制引用
const ipLists = ref<{ key: string; name: string; loaded: boolean }[]>([])
const loading = ref(true)
const saving = ref(false)
const listenerVisible = ref(false)
const editingListener = ref<Listener>(emptyListener())
const siteVisible = ref(false)
const siteTab = ref('basic')
const site = ref<Site>(emptySite())
const siteIsDefault = ref(false)
let siteOwner: ListenerView | null = null
// 分享链接地址需要所属监听器的协议与端口
const siteTls = ref(false)
const sitePort = ref(0)
let siteIndex = -1
const settingsVisible = ref(false)
const { isMobile, labelPosition } = useFormLayout()
// moduleEnabled 为模块全局开关，undefined 表示尚未加载
const moduleEnabled = ref<boolean>()
const moduleOff = computed(() => moduleEnabled.value === false)
const enabling = ref(false)
interface WebSettings {
  enable: boolean
  trustedProxies: string[] | null
  oauthCallbackBase: string
  oauthProviders: OAuthProvider[]
  autoBanSeconds: number
  authUsers: AuthUser[]
  ssoDomain: string
  oidc: OIDCSettings
  // selfServicePassword 为是否允许共享用户自助修改密码
  selfServicePassword: boolean
}
const settings = ref<WebSettings>()
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 5000)

// 按逗号（含中文逗号）或空白拆分
// 分路径后端没有自己的 ID：按对象分配稳定的 key，删除前面的路径时，后面路径的输入框状态（如文本模式）不会串位
const locKeys = new WeakMap<object, number>()
let locSeq = 0
function locKey(loc: object) {
  let k = locKeys.get(loc)
  if (k === undefined) {
    k = ++locSeq
    locKeys.set(loc, k)
  }
  return k
}

const splitCsv = (v: string) =>
  v
    .split(/[\s,，]+/)
    .map((x) => x.trim())
    .filter(Boolean)
// 输入框保留原始文本，回车可正常换行（见 useListText）
const bind = (get: () => string[] | null | undefined, set: (v: string[]) => void) => useListText(get, set, { split: splitLines })
const domainsText = bind(() => site.value.domains, (v) => (site.value.domains = v))
const targetsText = bind(() => site.value.targets, (v) => (site.value.targets = v))
const usersText = bind(() => site.value.users, (v) => (site.value.users = v))
const oauthUsersText = bind(() => site.value.oauthUsers, (v) => (site.value.oauthUsers = v))
const ipText = bind(() => site.value.ipList, (v) => (site.value.ipList = v))
const fwdHeadersText = useListText(() => site.value.forwardAuth?.copyHeaders, (v) => (site.value.forwardAuth.copyHeaders = v), {
  split: splitCsv,
  separator: ', ',
})
// 转发认证以地址非空表示启用，勾选框单独记录，取消勾选时保存为空地址
const forwardAuthOn = ref(false)
// 本站点是否启用了任一认证方式，启用时不能转发 Gately 登录 Cookie
const localAuthOn = computed(() => site.value.webAuth || site.value.basicAuth || forwardAuthOn.value)
const trustedText = bind(
  () => settings.value?.trustedProxies,
  (v) => {
    if (settings.value) {
      settings.value.trustedProxies = v
    }
  },
)

function emptySite(): Site {
  return {
    name: '',
    enable: true,
    group: '',
    domains: [],
    type: 'proxy',
    targets: [],
    insecureSkipVerify: false,
    useTargetHost: false,
    disableKeepAlives: false,
    sendProxyProtocol: false,
    forwardedHeaders: true,
    upstreamSniFromHost: false,
    passAuthCookies: false,
    locations: [],
    userRoutes: [],
    userHeader: '',
    redirectTo: '',
    redirectCode: 302,
    keepPath: true,
    root: '',
    listDir: false,
    cleanUrls: false,
    notFoundPage: '',
    text: '',
    contentType: '',
    statusCode: 200,
    basicAuth: false,
    webAuth: false,
    sharedUsers: false,
    users: [],
    oauthUsers: [],
    ipMode: '',
    ipList: [],
    ipSets: [],
    requestHeaders: [],
    responseHeaders: [],
    cors: false,
    accessLog: false,
    waf: '',
    remark: '',
    authOptional: false,
    forwardAuth: { url: '', loginUrl: '', copyHeaders: [] },
    countryMode: '',
    countries: [],
    denyClose: false,
    rules: [],
    shareLinks: [],
    lbPolicy: '',
    health: { enable: false, mode: 'http', path: '/', interval: 10, timeout: 3, fails: 3, passes: 2 },
    compress: false,
    rateLimit: { enable: false, rate: 10, burst: 20, maxConns: 0 },
    cache: { enable: false, ttl: 0, maxMb: 64 },
    rewrites: [],
  }
}

function emptyListener(): Listener {
  return {
    name: '',
    enable: true,
    network: 'tcp',
    listenIp: '',
    port: 443,
    tls: true,
    minTls: '1.2',
    http3: false,
    plainHttp: '',
    acceptProxyProtocol: false,
    sites: [],
    default: { ...emptySite(), enable: false },
    remark: '',
  }
}

function plainHttpText(v: string) {
  return v === 'redirect' ? t('web.plainHttpRedirectTag') : t('web.plainHttpServeTag')
}

function networkText(n: string) {
  const map: Record<string, string> = { tcp: t('web.networkDual'), tcp4: t('web.networkIpv4'), tcp6: t('web.networkIpv6') }
  return map[n] ?? n
}

// listenerStatus 计算监听展示状态：停用的监听只显示“已停用”
function listenerStatus(l: ListenerView): { type: StatusType; text: string; title?: string } {
  if (moduleOff.value) {
    return { type: 'off', text: t('web.moduleOff') }
  }
  if (!l.enable) {
    return { type: 'off', text: t('web.statusOff') }
  }
  if (l.error) {
    return { type: 'error', text: l.error, title: l.error }
  }
  if (l.running) {
    return { type: 'running', text: t('common.running') }
  }
  return { type: 'warn', text: t('web.statusNotRunning') }
}

function typeText(ty: string) {
  const map: Record<string, string> = { proxy: t('web.typeProxy'), redirect: t('web.typeRedirect'), static: t('web.typeStatic'), text: t('web.typeText'), close: t('web.typeClose') }
  return map[ty] ?? ty
}

function summary(s: Site) {
  switch (s.type) {
    case 'proxy':
      return (s.targets ?? []).join(' | ')
    case 'redirect':
      return `${s.redirectCode} → ${s.redirectTo}`
    case 'static':
      return s.root
    case 'close':
      return '444'
    default:
      return `${s.statusCode} ${s.text.slice(0, 30)}`
  }
}

// hasDefaultRow 判断默认站点是否已配置（已配置时显示在站点列表最后）
function hasDefaultRow(l: ListenerView) {
  return siteRows(l).some((r) => r.isDefault)
}

function siteRows(l: ListenerView): SiteRow[] {
  const rows = (l.sites ?? []).map((s, i) => ({ site: s, index: i, isDefault: false }))
  if (l.default.enable || (l.default.targets ?? []).length || l.default.type !== 'proxy') {
    rows.push({ site: l.default, index: -1, isDefault: true })
  }
  return rows
}

// 列表的筛选、排序与分组 ----

// filterText 按域名、名称、分组、后端与备注筛选站点（排序模式下不筛选，避免只调整到部分站点的顺序）
const filterText = ref('')
// 排序模式、多选与拖动状态；站点 id 为 监听key/站点key，拖动只在同一监听器内生效
const sel = useSortSelect()
const { sortMode } = sel
// 分组的收起状态按 监听key/分组名 持久化，与监听器的收起状态分开存放
const groupFold = useCollapsed('web.groups')
const hasSites = computed(() => listeners.value.some((l) => siteRows(l).length > 0))
const canSort = computed(() => listeners.value.length > 1 || listeners.value.some((l) => (l.sites ?? []).length > 1))
const busy = ref(false)

function siteId(l: ListenerView, row: SiteRow) {
  return `${l.key}/${row.site.key ?? row.index}`
}

// siteSections 把普通站点按分组归类：分组按首次出现的顺序排列，未分组的站点放在最后；默认站点不参与
function siteSections(l: ListenerView): Section<SiteRow>[] {
  const rows = (l.sites ?? []).map((s, i) => ({ site: s, index: i, isDefault: false }))
  return buildSections(rows, (r) => r.site.group ?? '')
}

const allSiteIds = computed(() => listeners.value.flatMap((l) => flatten(siteSections(l)).map((r) => siteId(l, r))))
const allGroupNames = computed(() => listeners.value.flatMap((l) => (l.sites ?? []).map((s) => s.group ?? '')).filter(Boolean))

function siteMatches(s: Site, q: string) {
  return [s.name, s.group ?? '', s.remark, ...(s.domains ?? []), summary(s)].join('\n').toLowerCase().includes(q)
}

function groupCollapsed(l: ListenerView, name: string) {
  return groupFold.isCollapsed(`${l.key}/${name}`)
}

function toggleGroup(l: ListenerView, name: string) {
  groupFold.setCollapsed(`${l.key}/${name}`, !groupCollapsed(l, name))
}

// displayItems 生成监听器下依次显示的分组标题与站点行；默认站点固定在最后
function displayItems(l: ListenerView): GroupItem<SiteRow>[] {
  const items = displayGroupItems(siteSections(l), (r) => siteId(l, r), {
    query: filterText.value,
    sorting: sortMode.value,
    matches: (r, q) => siteMatches(r.site, q),
    isCollapsed: (name) => groupCollapsed(l, name),
    isSelected: sel.isSelected,
    groupIdPrefix: `${l.key}/`,
  })
  const q = sortMode.value ? '' : filterText.value.trim().toLowerCase()
  const deflt = siteRows(l).find((r) => r.isDefault)
  if (deflt && (!q || siteMatches(deflt.site, q))) {
    items.push({ kind: 'item', id: `${l.key}/__default`, item: deflt, grouped: false, canUp: false, canDown: false })
  }
  return items
}

// saveSiteOrder 按新的分组与顺序重排站点并保存；站点匹配与顺序无关，只改变列表中的显示顺序
async function saveSiteOrder(lv: ListenerView, sections: Section<SiteRow>[] | null) {
  if (!sections) {
    return
  }
  const l = plain(lv)
  const sites = l.sites ?? []
  l.sites = sections.flatMap((sec) => sec.items.map((r) => ({ ...sites[r.index], group: sec.name })))
  busy.value = true
  try {
    await putListener(l)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

// moveSite 在站点所在分组内上移或下移一位
async function moveSite(l: ListenerView, id: string, d: number) {
  await saveSiteOrder(l, moveStep(siteSections(l), (r) => siteId(l, r), id, d))
  await load()
}

// moveGroup 整体移动一个分组；未分组的站点始终在最后
async function moveGroup(l: ListenerView, index: number, d: number) {
  await saveSiteOrder(l, moveSection(siteSections(l), index, d))
  await load()
}

// setGroupSelected 把选中的站点移到指定分组末尾（留空为移出分组），每个监听器各自保存
async function setGroupSelected(name: string) {
  const ids = new Set(sel.selected.value)
  for (const l of listeners.value) {
    const next = moveInto(siteSections(l), (r) => siteId(l, r), ids, { group: name })
    await saveSiteOrder(l, next)
  }
  await load()
}

// renameGroup 重命名监听器下的一个分组：组内站点一起改名，位置与顺序不变；新名称已存在时并入该分组
const askGroupName = useGroupRename()
async function renameGroup(l: ListenerView, name: string) {
  const names = [...new Set((l.sites ?? []).map((s) => s.group ?? '').filter(Boolean))]
  const to = await askGroupName(name, names)
  if (to === null) {
    return
  }
  // 合并到已有分组时两个分组的站点按原位置合在一起：先出现的分组位置保留
  const merged = renameSection(siteSections(l), name, to)
  const wasCollapsed = groupCollapsed(l, name)
  await saveSiteOrder(l, merged)
  groupFold.setCollapsed(`${l.key}/${to}`, wasCollapsed || groupCollapsed(l, to))
  groupFold.setCollapsed(`${l.key}/${name}`, false)
  await load()
}

async function moveListener(i: number, d: number) {
  const keys = listeners.value.map((l) => l.key ?? '')
  const j = i + d
  if (j < 0 || j >= keys.length) {
    return
  }
  ;[keys[i], keys[j]] = [keys[j], keys[i]]
  try {
    await api.put('/api/webservice/listeners-order', { keys })
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
  await load()
}

// onDragStart 开始拖动：拖动选中的站点时同一监听器内的全部选中站点一起移动
function onDragStart(l: ListenerView, id: string, e: DragEvent) {
  sel.dragStart(id, allSiteIds.value.filter((x) => x.startsWith(`${l.key}/`)), e)
}

// onDrop 放下站点：放到站点上方或下方即插入该位置并加入其分组，放到分组标题上即移到该分组末尾
async function onDrop(l: ListenerView) {
  const d = sel.takeDrop()
  const prefix = `${l.key}/`
  if (!d || !d.over.id.startsWith(prefix) || ![...d.ids].every((id) => id.startsWith(prefix))) {
    return
  }
  const sections = siteSections(l)
  const idOf = (r: SiteRow) => siteId(l, r)
  const target = dropTarget(sections, idOf, d.over, prefix)
  if (target) {
    await saveSiteOrder(l, moveInto(sections, idOf, d.ids, target))
    await load()
  }
}

// 去掉运行状态字段，得到可提交的监听配置
function plain(l: ListenerView): Listener {
  // 剥离仅用于展示的运行态字段，后端保存接口会拒绝未知字段
  const { running: _r, error: _e, stats: _s, upstreams: _u, ...rest } = l
  return JSON.parse(JSON.stringify(rest))
}

async function putListener(l: Listener) {
  const { key, ...body } = l
  await api.put(`/api/webservice/listeners/${key}`, body)
}

async function load() {
  try {
    listeners.value = await api.get<ListenerView[]>('/api/webservice/listeners')
  } finally {
    loading.value = false
  }
}

function openListener(l?: ListenerView) {
  editingListener.value = l ? plain(l) : emptyListener()
  listenerVisible.value = true
}

async function saveListener() {
  saving.value = true
  try {
    const { key, ...body } = editingListener.value
    if (key) {
      await api.put(`/api/webservice/listeners/${key}`, body)
    } else {
      await api.post('/api/webservice/listeners', body)
    }
    listenerVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggleListener(l: ListenerView, enable: boolean) {
  try {
    await api.put(`/api/webservice/listeners/${l.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeListener(l: ListenerView) {
  try {
    await api.del(`/api/webservice/listeners/${l.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

const statsDialog = ref<InstanceType<typeof SiteStats>>()

function openStats(l: ListenerView, row: SiteRow) {
  statsDialog.value?.open(l.key ?? '', row.isDefault ? 'default' : (row.site.key ?? ''), row.site.name || row.site.domains?.[0] || t('web.defaultSite'))
}

// downCount 统计站点中不可用的后端数量
function downCount(l: ListenerView, row: SiteRow) {
  const k = row.isDefault ? 'default' : (row.site.key ?? '')
  return (l.upstreams?.[k] ?? []).filter((u) => !u.healthy).length
}

function openSite(l: ListenerView, row?: SiteRow) {
  loadIPLists()
  siteOwner = l
  siteTls.value = l.tls
  sitePort.value = l.port
  siteIndex = row ? row.index : -1
  siteIsDefault.value = !!row?.isDefault
  // 旧配置中新增字段为 null，合并默认值后保证数组可直接编辑
  const draft: Site = row ? JSON.parse(JSON.stringify(row.site)) : emptySite()
  draft.rules = (draft.rules ?? []).map((r) => ({ ...r, ipSets: r.ipSets ?? [] }))
  draft.shareLinks = draft.shareLinks ?? []
  draft.ipSets = draft.ipSets ?? []
  draft.countries = draft.countries ?? []
  draft.rewrites = draft.rewrites ?? []
  draft.locations = draft.locations ?? []
  draft.userRoutes = draft.userRoutes ?? []
  draft.userHeader = draft.userHeader ?? ''
  draft.forwardAuth = Object.assign({ url: '', loginUrl: '', copyHeaders: [] }, draft.forwardAuth)
  forwardAuthOn.value = !!draft.forwardAuth.url
  // 旧配置没有这些对象，补全默认值后可直接编辑
  const defaults = emptySite()
  draft.health = { ...defaults.health, ...(draft.health ?? {}) }
  draft.rateLimit = { ...defaults.rateLimit, ...(draft.rateLimit ?? {}) }
  draft.cache = { ...defaults.cache, ...(draft.cache ?? {}) }
  site.value = draft
  siteTab.value = 'basic'
  siteVisible.value = true
}

// openDefaultSite 打开未配置的默认站点：用户主动配置兜底规则，编辑框里默认打开启用开关
function openDefaultSite(l: ListenerView) {
  openSite(l, { site: l.default, index: -1, isDefault: true })
  site.value.enable = true
}

async function saveSite() {
  if (!siteOwner) {
    return
  }
  if (!forwardAuthOn.value) {
    site.value.forwardAuth = { url: '', loginUrl: '', copyHeaders: [] }
  }
  // 关闭认证或改为其他类型后，按用户分流不再显示，也不再生效，一并清空，避免保存时被隐藏的设置拦住
  if (site.value.type !== 'proxy' || !localAuthOn.value) {
    site.value.userRoutes = []
    site.value.userHeader = ''
  }
  const l = plain(siteOwner)
  l.sites = l.sites ?? []
  if (siteIsDefault.value) {
    l.default = site.value
  } else if (siteIndex >= 0) {
    l.sites[siteIndex] = site.value
  } else {
    l.sites.push(site.value)
  }
  saving.value = true
  try {
    await putListener(l)
    siteVisible.value = false
    ElMessage.success(t('web.savedReloaded'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggleSite(lv: ListenerView, row: SiteRow, enable: boolean) {
  const l = plain(lv)
  if (row.isDefault) {
    l.default.enable = enable
  } else if (l.sites) {
    l.sites[row.index].enable = enable
  }
  try {
    await putListener(l)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeSite(lv: ListenerView, row: SiteRow) {
  const l = plain(lv)
  l.sites?.splice(row.index, 1)
  try {
    await putListener(l)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// resetDefaultSite 清空默认站点配置并保持停用（默认站点无法删除）
async function resetDefaultSite(lv: ListenerView) {
  const l = plain(lv)
  l.default = { ...emptySite(), key: lv.default.key, enable: false }
  try {
    await putListener(l)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function fetchSettings() {
  const s = await api.get<
    Omit<WebSettings, 'oauthProviders' | 'authUsers' | 'oidc'> & { oauthProviders: OAuthProvider[] | null; authUsers: AuthUser[] | null; oidc?: Partial<OIDCSettings> | null }
  >('/api/webservice/settings')
  moduleEnabled.value = s.enable
  return { ...s, oauthProviders: s.oauthProviders ?? [], authUsers: fromAPI(s.authUsers), ssoDomain: s.ssoDomain ?? '', oidc: oidcFromAPI(s.oidc), selfServicePassword: !!s.selfServicePassword }
}

// settingsBody 生成全局设置的请求体：共享用户、单点登录域、自助改密与 OIDC 由认证用户页维护，
// 这里不提交，后端对省略的字段保留原值，避免对话框里的旧数据覆盖认证用户页刚保存的修改
function settingsBody(s: WebSettings) {
  const { authUsers: _u, ssoDomain: _d, selfServicePassword: _p, oidc: _o, ...rest } = s
  return rest
}

async function openSettings() {
  settings.value = await fetchSettings()
  settingsVisible.value = true
}

// enableModule 在横幅中一键启用模块
async function enableModule() {
  enabling.value = true
  try {
    const s = await fetchSettings()
    await api.put('/api/webservice/settings', { ...settingsBody(s), enable: true })
    ElMessage.success(t('web.moduleEnabled'))
    await Promise.all([fetchSettings(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    enabling.value = false
  }
}

async function saveSettings() {
  try {
    const s = settings.value
    if (!s) {
      return
    }
    await api.put('/api/webservice/settings', settingsBody(s))
    settingsVisible.value = false
    ElMessage.success(t('common.saved'))
    await Promise.all([fetchSettings(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// loadIPLists 读取订阅 IP 列表；读取失败时只是不显示可选项
async function loadIPLists() {
  try {
    ipLists.value = await api.get('/api/webservice/iplists')
  } catch {
    ipLists.value = []
  }
}

onMounted(async () => {
  fetchSettings()
  loadIPLists()
  wafInstances.value = await api.get('/api/waf/instances')
  await load()
  polling.start()
})
</script>

<style scoped>
.users-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.users-link {
  color: var(--el-color-primary);
  text-decoration: none;
}
.users-link:hover {
  text-decoration: underline;
}
.label-with-help {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.listener-hint {
  margin-bottom: 16px;
}
.locations {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.location {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--g-border);
  border-radius: 6px;
}
.location-row {
  display: flex;
  gap: 6px;
  align-items: flex-start;
}
.location-prefix {
  width: 120px;
  flex: none;
}
.user-route-users {
  width: 200px;
  flex: none;
}
.location-targets {
  flex: 1;
  min-width: 0;
}
.sites {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  overflow: hidden;
}
.site {
  display: grid;
  grid-template-columns: 40px minmax(0, 1.5fr) minmax(0, 1.8fr) minmax(0, 1.3fr) 110px auto;
  gap: 8px 14px;
  align-items: center;
  padding: 9px 8px 9px 12px;
  font-size: 13px;
}
.site + .site,
.group-header + .site,
.site + .group-header {
  border-top: 1px solid var(--g-border);
}
.site-filter {
  width: 220px;
}
.grip {
  color: var(--el-text-color-secondary);
  cursor: grab;
}
.sites.is-sorting .site[draggable='true'] {
  cursor: grab;
}
.site.dragging {
  opacity: 0.4;
}
/* 拖动时的插入位置：上方或下方显示一条主题色线 */
.site.drop-before {
  box-shadow: inset 0 2px 0 var(--el-color-primary);
}
.site.drop-after {
  box-shadow: inset 0 -2px 0 var(--el-color-primary);
}
.site.is-off .site-main,
.site.is-off .site-target,
.site.is-off .site-stats {
  opacity: 0.55;
}
.site-main,
.site-target {
  min-width: 0;
}
.site-domains {
  word-break: break-all;
}
.target {
  margin-left: 6px;
  font-size: 12px;
  word-break: break-all;
}
.site-opts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.site-stats {
  font-size: 12px;
  white-space: nowrap;
}
.site-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}
.site-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}
.empty-sites {
  padding: 10px 12px;
}
.inline {
  margin-left: 16px;
}
.w-120 {
  width: 120px;
}
.w-full {
  width: 100%;
}
.opt-warn {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-color-warning);
}
.w-260 {
  width: 260px;
}
.el-checkbox {
  white-space: normal;
  height: auto;
  min-height: 32px;
}
/* 紧凑视图：站点行压成单行，域名与说明同行，超长内容省略（仅桌面宽度） */
@media (min-width: 768px) {
  .sites.is-compact .site {
    gap: 4px 12px;
    padding: 3px 8px 3px 12px;
    font-size: 12px;
  }
  .sites.is-compact .site-main {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .sites.is-compact .site-domains,
  .sites.is-compact .site-main .meta,
  .sites.is-compact .site-target {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    word-break: normal;
  }
  /* 域名优先完整显示，名称说明只占剩余空间 */
  .sites.is-compact .site {
    grid-template-columns: 40px minmax(0, 1.9fr) minmax(0, 1.6fr) minmax(0, 1fr) 100px auto;
  }
  .sites.is-compact .site-main .site-domains {
    flex: 0 1 auto;
  }
  .sites.is-compact .site-main .meta {
    flex: 1 1 0;
  }
  .sites.is-compact .site-opts {
    flex-wrap: nowrap;
    overflow: hidden;
  }
  .sites.is-compact .site-opts :deep(.el-tag) {
    flex-shrink: 0;
  }
}
@media (max-width: 767.98px) {
  .site-filter {
    width: 160px;
  }
  .user-route-users {
    width: 120px;
  }
  .site {
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: 10px;
  }
  .site-switch {
    grid-row: 1;
    grid-column: 1;
  }
  .site-main {
    grid-row: 1;
    grid-column: 2;
  }
  .site-actions {
    grid-row: 1;
    grid-column: 3;
  }
  .site-target,
  .site-opts,
  .site-stats {
    grid-column: 2 / 4;
  }
  .site-opts:empty {
    display: none;
  }
  .w-260 {
    width: 100%;
  }
  .inline {
    margin-left: 0;
  }
}
</style>
