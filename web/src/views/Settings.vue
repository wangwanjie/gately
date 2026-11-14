<template>
  <div>
    <PageHeader :title="t('settings.title')" :desc="t('settings.desc')" :icon="Settings2" />

    <div v-if="form?.restartRequired" class="restart-banner">
      <RotateCw :size="18" class="rb-icon" />
      <div class="rb-text">
        <div class="rb-title">{{ t('settings.restartBanner.title') }}</div>
        <div class="rb-desc">{{ t('settings.restartBanner.desc') }}</div>
      </div>
      <el-button type="warning" :icon="RotateCw" @click="confirmRestart">{{ t('settings.restartBanner.action') }}</el-button>
    </div>

    <el-tabs v-model="tab" class="settings-tabs">
      <!-- ============ 账号与安全 ============ -->
      <el-tab-pane name="account">
        <template #label><span class="tab-label"><UserRound :size="15" />{{ t('settings.tabs.account') }}</span></template>
        <SecurityCenter @go="goFix" />

        <div class="grid-2">
          <div id="sec-account" class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><KeyRound :size="16" />{{ t('settings.admin.title') }}</div>
            </div>
            <div class="g-card-body">
              <el-form :model="account" label-width="100px" :label-position="labelPosition" class="narrow">
                <el-form-item :label="t('settings.currentPassword')" required>
                  <el-input v-model="account.oldPassword" type="password" show-password autocomplete="current-password" />
                </el-form-item>
                <el-form-item :label="t('settings.admin.username')" required>
                  <el-input v-model="account.username" autocomplete="username" />
                </el-form-item>
                <el-form-item :label="t('settings.admin.newPassword')" required>
                  <el-input v-model="account.newPassword" type="password" show-password :placeholder="t('settings.admin.newPasswordPlaceholder')" autocomplete="new-password" />
                  <PasswordStrength :password="account.newPassword" :username="account.username" />
                </el-form-item>
                <el-form-item :label="t('settings.admin.confirmPassword')" required>
                  <el-input v-model="account.confirm" type="password" show-password autocomplete="new-password" />
                </el-form-item>
                <el-form-item label=" ">
                  <el-button type="primary" :loading="accountSaving" @click="saveAccount">{{ t('settings.admin.submit') }}</el-button>
                  <div class="form-help">{{ t('settings.admin.help') }}</div>
                </el-form-item>
              </el-form>
            </div>
          </div>

          <div id="sec-twofactor" class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><Smartphone :size="16" />{{ t('settings.totp.title') }}</div>
              <StatusBadge :type="totp.enabled ? 'ok' : 'off'" :text="totp.enabled ? t('common.enabled') : t('settings.notEnabled')" />
            </div>
            <div class="g-card-body">
              <template v-if="totp.enabled">
                <p class="desc">{{ t('settings.totp.enabledDesc') }}</p>
                <el-button type="danger" plain :icon="ShieldOff" @click="openTOTP('disable')">{{ t('settings.totp.disable') }}</el-button>
              </template>
              <template v-else>
                <p class="desc">{{ t('settings.totp.introDesc') }}</p>
                <el-button type="primary" plain :icon="ShieldCheck" :loading="totp.preparing" @click="setupTOTP">{{ t('settings.totp.start') }}</el-button>
              </template>
            </div>
          </div>

          <!-- 两步验证弹窗：启用时展示二维码，启用与关闭都需当前密码，验证码输满 6 位自动提交 -->
          <el-dialog
            v-model="totp.dialog"
            :title="totp.mode === 'enable' ? t('settings.totp.enableTitle') : t('settings.totp.disable')"
            width="440px"
            class="totp-dialog"
            align-center
            :close-on-click-modal="false"
            @opened="focusTOTP"
            @closed="cancelTOTP"
          >
            <template v-if="totp.mode === 'enable'">
              <p class="desc">{{ t('settings.totp.scanDesc') }}</p>
              <div class="qr-wrap">
                <img v-if="totp.qr" :src="totp.qr" class="qr" :alt="t('settings.totp.qrAlt')" />
                <div class="secret-box">
                  <div class="meta">{{ t('settings.totp.manualHint') }}</div>
                  <div ref="totpSecretEl" class="mono secret">{{ totp.secret }}</div>
                  <el-button size="small" text :icon="Copy" @click="copyText(totp.secret, { el: totpSecretEl })">{{ t('settings.totp.copySecret') }}</el-button>
                </div>
              </div>
            </template>
            <p v-else class="desc">{{ t('settings.totp.disablePrompt') }}</p>
            <div class="totp-field">
              <div class="totp-label">{{ t('settings.currentPassword') }}</div>
              <el-input
                ref="totpPasswordInput"
                v-model="totp.password"
                type="password"
                show-password
                autocomplete="current-password"
                :disabled="totp.submitting"
                @keyup.enter="submitTOTP"
              />
            </div>
            <div class="totp-field">
              <div class="totp-label">{{ t('settings.totp.code') }}</div>
              <OtpInput ref="totpCodeInput" v-model="totp.code" :disabled="totp.submitting" :aria-label="t('settings.totp.code')" @complete="submitTOTP" />
            </div>
            <div class="totp-status">
              <template v-if="totp.submitting"><LoaderCircle :size="14" class="spin" />{{ t('settings.totp.verifying') }}</template>
              <template v-else>{{ t('settings.totp.autoHint') }}</template>
            </div>
          </el-dialog>

          <!-- 退出所有设备：使全部管理界面会话失效，API 令牌不受影响 -->
          <div class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><LogOut :size="16" />{{ t('settings.sessions.title') }}</div>
            </div>
            <div class="g-card-body">
              <p class="desc">{{ t('settings.sessions.desc') }}</p>
              <el-button type="danger" plain :icon="LogOut" :loading="sessionsRevoking" @click="revokeAllSessions">{{ t('settings.sessions.button') }}</el-button>
            </div>
          </div>
        </div>

        <PasskeyManager @changed="loadSecurity" />

        <ReadonlyUsers />

        <div v-if="form" id="sec-loginprotect" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><LockKeyhole :size="16" />{{ t('settings.loginProtect.title') }}</div>
          </div>
          <div class="g-card-body">
            <el-form :model="form" label-width="140px" :label-position="labelPosition">
              <el-form-item :label="t('settings.loginProtect.sessionHours')">
                <el-input-number v-model="form.sessionHours" :min="1" :max="8760" />
                <span class="help">{{ t('settings.loginProtect.sessionHoursHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.loginProtect.maxFailures')">
                <el-input-number v-model="form.maxLoginFailures" :min="0" :max="100" />
                <span class="help">{{ t('settings.loginProtect.maxFailuresHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.loginProtect.lockMinutes')">
                <el-input-number v-model="form.lockMinutes" :min="0" :max="10080" />
                <span class="help">{{ t('settings.loginProtect.lockMinutesHelp') }}</span>
              </el-form-item>
              <!-- 陌生来源全局失败预算：抵御靠大量不同地址分散尝试、绕过单 IP 限流的爆破 -->
              <el-form-item :label="t('security.limit.label')">
                <el-switch v-model="globalLimitOn" />
                <el-input-number v-if="globalLimitOn" v-model="globalLimitValue" :min="1" :max="10000" class="limit-num" />
                <div class="form-help">{{ t('security.limit.help') }}</div>
              </el-form-item>
            </el-form>
          </div>
        </div>

        <div v-if="form" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><KeyRound :size="16" />{{ t('settings.thirdLogin.title') }}</div>
            <StatusBadge :type="login.enable ? 'ok' : 'off'" :text="login.enable ? t('common.enabled') : t('settings.notEnabled')" />
          </div>
          <div class="g-card-body">
            <p class="desc">{{ t('settings.thirdLogin.desc') }}</p>
            <el-form :model="login" label-width="140px" :label-position="labelPosition">
              <el-form-item :label="t('settings.thirdLogin.enable')"><el-switch v-model="login.enable" /></el-form-item>
              <el-form-item :label="t('settings.thirdLogin.callbackBase')">
                <el-input v-model="login.callbackBase" :placeholder="t('settings.thirdLogin.callbackBasePlaceholder')" />
                <i18n-t keypath="settings.thirdLogin.callbackHelp" tag="div" class="form-help">
                  <template #url>
                    <span class="mono">{{ (login.callbackBase || 'https://...').replace(/\/$/, '') }}/api/login/oauth/callback</span>
                  </template>
                </i18n-t>
              </el-form-item>
              <el-form-item :label="t('settings.thirdLogin.providers')"><OAuthProviders v-model="login.providers" secrets-url="/api/system/settings/secrets" /></el-form-item>
              <el-form-item :label="t('settings.thirdLogin.allowed')">
                <el-input v-model="allowedText" type="textarea" :rows="4" class="mono" :placeholder="t('settings.thirdLogin.allowedPlaceholder')" />
                <div class="form-help">{{ t('settings.thirdLogin.allowedHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.thirdLogin.skipTotp')">
                <el-switch v-model="login.skipTotp" />
                <span class="help">{{ t('settings.thirdLogin.skipTotpHelp') }}</span>
              </el-form-item>
            </el-form>
          </div>
        </div>

        <!-- 单令牌已升级为多令牌 + 权限控制，管理入口迁移到独立页面 -->
        <div class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Code2 :size="16" />{{ t('settings.apiToken.title') }}</div>
          </div>
          <div class="g-card-body">
            <p class="desc token-moved">{{ t('settings.apiToken.desc') }}</p>
            <div class="token-links">
              <router-link to="/api-tokens"><el-button type="primary" :icon="KeyRound">{{ t('settings.apiToken.manage') }}</el-button></router-link>
              <router-link to="/api-docs"><el-button :icon="BookOpen">{{ t('settings.apiToken.docs') }}</el-button></router-link>
            </div>
          </div>
        </div>
        <SaveBar :dirty="dirty" :saving="saving" @save="saveSettings" @reset="resetSettings" />
      </el-tab-pane>

      <!-- ============ 访问控制 ============ -->
      <el-tab-pane name="access">
        <template #label><span class="tab-label"><ShieldCheck :size="15" />{{ t('settings.tabs.access') }}</span></template>
        <div v-if="form" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Network :size="16" />{{ t('settings.listen.title') }}</div>
          </div>
          <div class="g-card-body">
            <el-form :model="form" label-width="140px" :label-position="labelPosition">
              <el-form-item :label="t('settings.listen.http')">
                <el-input v-model="form.listen" placeholder=":16680" class="w-280 mono" />
                <span class="help"><el-tag size="small" type="warning">{{ t('settings.listen.needRestart') }}</el-tag> {{ t('settings.listen.httpHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.listen.https')">
                <el-input v-model="form.httpsListen" :placeholder="t('settings.listen.httpsPlaceholder')" class="w-280 mono" />
                <span class="help"><el-tag size="small" type="warning">{{ t('settings.listen.needRestart') }}</el-tag> {{ t('settings.listen.httpsHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.listen.forceHttps')">
                <el-switch v-model="form.forceHTTPS" :disabled="!form.httpsListen && !form.forceHTTPS" />
                <span class="help">{{ t('settings.listen.forceHttpsHelp') }}</span>
              </el-form-item>
            </el-form>
          </div>
        </div>
        <div v-if="form" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><DoorClosed :size="16" />{{ t('settings.restrict.title') }}</div>
          </div>
          <div class="g-card-body">
            <el-form :model="form" label-width="140px" :label-position="labelPosition">
              <el-form-item id="sec-entry" :label="t('settings.restrict.safePath')">
                <el-input v-model="form.safePath" :placeholder="t('settings.restrict.safePathPlaceholder')" class="w-280 mono">
                  <template #append>
                    <el-button :icon="Sparkles" :aria-label="t('settings.restrict.random')" @click="randomEntry" />
                  </template>
                </el-input>
                <i18n-t keypath="settings.restrict.safePathHelp" tag="div" class="form-help">
                  <template #url>
                    <span class="mono code">{{ t('settings.restrict.urlPrefix') }}{{ entryPath || '/<' + t('settings.restrict.entryName') + '>' }}</span>
                  </template>
                </i18n-t>
              </el-form-item>
              <el-form-item :label="t('settings.restrict.lanOnly')">
                <el-switch v-model="form.lanOnly" />
                <span class="help">{{ t('settings.restrict.lanOnlyHelp') }}</span>
              </el-form-item>
              <!-- 仅飞牛 fpk 包（统一网关方式运行）显示 -->
              <el-form-item v-if="form.gatewayAvailable" :label="t('settings.restrict.gatewaySSO')">
                <el-switch v-model="form.gatewaySSO" />
                <span class="help">{{ t('settings.restrict.gatewaySSOHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.restrict.trustProxy')">
                <el-switch v-model="trustProxy" />
                <span class="help">{{ t('settings.restrict.trustProxyHelp') }}</span>
              </el-form-item>
              <el-form-item v-if="trustProxy" :label="t('settings.restrict.trustedProxies')">
                <el-select v-model="trustedProxies" multiple filterable allow-create default-first-option :reserve-keyword="false" :placeholder="t('settings.restrict.trustedProxiesPlaceholder')" class="w-480" />
                <div class="help">{{ t('settings.restrict.trustedProxiesHelp') }}</div>
              </el-form-item>
              <!-- 未授权访问（封禁、仅内网拦截、未通过安全入口）的响应方式，用于隐藏管理端口 -->
              <el-form-item :label="t('settings.restrict.deny.label')">
                <el-select v-model="denyModeSel" class="w-280" data-test="deny-mode">
                  <el-option v-for="m in denyModes" :key="m.value" :value="m.value" :label="m.label" />
                </el-select>
                <div class="form-help">{{ denyModeHelp }}</div>
                <div v-if="denyWarnings.length" class="deny-warns">
                  <div v-for="w in denyWarnings" :key="w" class="deny-warn"><TriangleAlert :size="13" /><span>{{ w }}</span></div>
                </div>
              </el-form-item>
              <template v-if="form.denyMode === 'custom'">
                <el-form-item :label="t('settings.restrict.deny.status')">
                  <el-select v-model="form.denyStatus" class="w-280" data-test="deny-status">
                    <el-option v-for="c in denyStatuses" :key="c.value" :value="c.value" :label="c.label" />
                  </el-select>
                </el-form-item>
                <el-form-item :label="t('settings.restrict.deny.page')">
                  <!-- 输入框与大小栏放在同一容器，大小栏始终贴在输入框下方 -->
                  <div class="w-480">
                    <el-input v-model="form.denyPage" type="textarea" :rows="8" class="mono deny-page" spellcheck="false" :placeholder="t('settings.restrict.deny.pagePlaceholder')" data-test="deny-page" />
                    <div class="deny-page-bar">
                      <span :class="['deny-size', { over: denyPageBytes > denyPageMax }]">{{ formatKB(denyPageBytes) }} / {{ formatKB(denyPageMax) }}</span>
                      <span class="deny-page-ops">
                        <el-button size="small" link type="primary" @click="fillDenySample">{{ t('settings.restrict.deny.sample') }}</el-button>
                        <el-button size="small" link type="primary" :icon="Eye" :disabled="!form.denyPage" @click="denyPreview = true">{{ t('settings.restrict.deny.preview') }}</el-button>
                      </span>
                    </div>
                  </div>
                  <div class="form-help">{{ t('settings.restrict.deny.pageHelp') }}</div>
                </el-form-item>
              </template>
            </el-form>
          </div>
        </div>
        <SaveBar :dirty="dirty" :saving="saving" @save="saveSettings" @reset="resetSettings" />
      </el-tab-pane>

      <!-- ============ 网络与运行 ============ -->
      <el-tab-pane name="runtime">
        <template #label><span class="tab-label"><Cpu :size="15" />{{ t('settings.tabs.runtime') }}</span></template>
        <div v-if="form" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Globe :size="16" />{{ t('settings.network.title') }}</div>
          </div>
          <div class="g-card-body">
            <el-form :model="form" label-width="140px" :label-position="labelPosition">
              <el-form-item :label="t('settings.network.dnsServers')">
                <el-select
                  v-model="dnsServers"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                  :reserve-keyword="false"
                  :placeholder="t('settings.network.dnsPlaceholder')"
                  class="w-480"
                >
                  <el-option v-for="d in dnsPresets" :key="d.value" :label="`${d.value}（${d.label}）`" :value="d.value" />
                </el-select>
                <div class="form-help">{{ t('settings.network.dnsHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.network.autoUpdateCheck')">
                <el-switch :model-value="!form.disableUpdateCheck" @update:model-value="setAutoUpdateCheck" />
                <div class="form-help">{{ t('settings.network.autoUpdateCheckHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.network.updateProxy')">
                <el-input v-model="form.updateProxy" clearable :placeholder="t('settings.network.updateProxyPlaceholder')" class="w-480" />
                <div class="form-help">{{ t('settings.network.updateProxyHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.network.restrictOutbound')">
                <el-switch v-model="form.restrictOutbound" />
                <span class="help">{{ t('settings.network.restrictOutboundSwitch') }}</span>
                <div class="form-help">{{ t('settings.network.restrictOutboundHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.network.timeZone')">
                <el-select v-model="form.timeZone" filterable allow-create default-first-option clearable value-on-clear="" :placeholder="t('settings.network.timeZonePlaceholder')" class="w-280">
                  <el-option v-for="z in timeZones" :key="z.value" :label="`${z.label}  ${z.value}`" :value="z.value" />
                </el-select>
                <div class="form-help">{{ t('settings.network.timeZoneHelp', { zone: info?.timeZone || '-' }) }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.network.serverLanguage')">
                <el-select v-model="form.language" class="w-280" :empty-values="[null, undefined]">
                  <el-option value="" :label="t('settings.network.serverLanguageAuto')" />
                  <el-option v-for="l in locales" :key="l.code" :value="l.code" :label="l.name" />
                </el-select>
                <div class="form-help">{{ t('settings.network.serverLanguageHelp') }}</div>
              </el-form-item>
            </el-form>
          </div>
        </div>
        <div v-if="form" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Gauge :size="16" />{{ t('settings.runtime.title') }}</div>
          </div>
          <div class="g-card-body">
            <el-form :model="form" label-width="140px" :label-position="labelPosition">
              <el-form-item :label="t('settings.runtime.logLines')">
                <el-input-number v-model="form.logLines" :min="100" :max="20000" :step="500" />
                <span class="help">{{ t('settings.runtime.logLinesHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.runtime.logPersist')">
                <el-switch v-model="logPersist" />
                <span class="help">{{ t('settings.runtime.logPersistHelp') }}</span>
              </el-form-item>
              <el-form-item v-if="logPersist" :label="t('settings.runtime.logFileMB')">
                <el-input-number v-model="form.logFileMB" :min="0" :max="100" />
                <span class="help">{{ t('settings.runtime.logFileMBHelp') }}</span>
              </el-form-item>
              <!-- 访问日志（Web 服务）按天归档压缩，单独设置保留天数与总大小；0 表示默认值 -->
              <el-form-item v-if="logPersist" :label="t('settings.runtime.accessLogDays')">
                <el-input-number v-model="form.accessLogDays" :min="0" :max="3650" />
                <span class="help">{{ t('settings.runtime.accessLogDaysHelp') }}</span>
              </el-form-item>
              <el-form-item v-if="logPersist" :label="t('settings.runtime.accessLogMB')">
                <el-input-number v-model="form.accessLogMB" :min="0" :max="102400" :step="16" />
                <span class="help">{{ t('settings.runtime.accessLogMBHelp') }}</span>
              </el-form-item>
              <el-form-item v-if="logPersist" :label="t('settings.runtime.logDir')">
                <el-input v-model="form.logDir" class="w-480 mono" clearable :placeholder="t('settings.runtime.logDirPlaceholder')" />
                <div class="form-help">{{ t('settings.runtime.logDirHelp') }}</div>
              </el-form-item>
              <el-form-item :label="t('settings.runtime.gcPercent')">
                <el-input-number v-model="form.gcPercent" :min="0" :max="1000" :step="10" />
                <span class="help">{{ t('settings.runtime.gcPercentHelp') }}</span>
              </el-form-item>
              <el-form-item :label="t('settings.runtime.maxProcs')">
                <el-input-number v-model="form.maxProcs" :min="0" :max="1024" />
                <span class="help">{{ t('settings.runtime.maxProcsHelp', { count: info?.numCPU ?? '-' }) }}</span>
              </el-form-item>
            </el-form>
          </div>
        </div>
        <div class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><RotateCw :size="16" />{{ t('settings.restart.title') }}</div>
          </div>
          <div class="g-card-body restart-row">
            <span class="muted">{{ t('settings.restart.desc') }}</span>
            <el-button type="warning" plain :icon="RotateCw" @click="confirmRestart">{{ t('settings.restart.button') }}</el-button>
          </div>
        </div>
        <SaveBar :dirty="dirty" :saving="saving" @save="saveSettings" @reset="resetSettings" />
      </el-tab-pane>

      <!-- ============ 功能模块 ============ -->
      <el-tab-pane name="modules">
        <template #label><span class="tab-label"><Blocks :size="15" />{{ t('settings.tabs.modules') }}</span></template>
        <div v-if="form" class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Blocks :size="16" />{{ t('settings.modules.title') }}</div>
          </div>
          <div class="g-card-body">
            <div class="form-help modules-desc"><el-tag size="small" type="warning">{{ t('settings.listen.needRestart') }}</el-tag> {{ t('settings.modules.desc') }}</div>
            <div v-for="g in moduleGroups" :key="g.titleKey" class="module-group">
              <div class="module-group-title">{{ t(g.titleKey) }}</div>
              <div class="module-list">
                <div v-for="m in g.items" :key="m.id" class="module-row" :class="{ off: !m.on }">
                  <component :is="m.icon" :size="18" class="module-icon" />
                  <div class="module-text">
                    <div class="module-name">{{ m.title }}</div>
                    <div class="module-desc">{{ m.lockedBy ? t('settings.modules.requiredBy', { names: m.lockedBy }) : m.desc }}</div>
                  </div>
                  <el-switch :model-value="m.on" :disabled="!!m.lockedBy" :aria-label="m.title" @update:model-value="(v: string | number | boolean) => toggleModule(m.id, !!v)" />
                </div>
              </div>
            </div>
          </div>
        </div>
        <SaveBar :dirty="dirty" :saving="saving" @save="saveSettings" @reset="resetSettings" />
      </el-tab-pane>

      <!-- ============ 备份与迁移 ============ -->
      <el-tab-pane name="backup">
        <template #label><span class="tab-label"><ArchiveRestore :size="15" />{{ t('settings.tabs.backup') }}</span></template>
        <div class="g-card">
          <div class="g-card-head">
            <div class="g-card-title"><Import :size="16" />{{ t('settings.import.title') }}</div>
          </div>
          <div class="g-card-body">
            <i18n-t keypath="settings.import.desc" tag="p" class="desc">
              <template #dir><span class="mono code">/app/conf</span></template>
              <template #lkcf><span class="mono code">*.lkcf</span></template>
              <template #zip><span class="mono code">.zip</span></template>
            </i18n-t>
            <el-upload v-model:file-list="files" :auto-upload="false" multiple accept=".lkcf,.zip" drag class="upload">
              <UploadCloud :size="32" :stroke-width="1.5" class="upload-icon" />
              <i18n-t keypath="settings.import.drag" tag="div" class="upload-text">
                <template #click><em>{{ t('settings.import.dragClick') }}</em></template>
              </i18n-t>
              <div class="meta">{{ t('settings.import.dragHint') }}</div>
            </el-upload>
            <div class="keep-enable">
              <el-switch v-model="keepEnable" />
              <div>
                <div class="ke-title">{{ t('settings.import.keepEnableTitle') }}</div>
                <i18n-t keypath="settings.import.keepEnableDesc" tag="div" class="ke-desc">
                  <template #bold><b>{{ t('settings.import.keepEnableBold') }}</b></template>
                </i18n-t>
              </div>
            </div>
            <div class="ops">
              <el-button :icon="Eye" :disabled="!files.length" :loading="busy && !importing" @click="runImport(true)">{{ t('settings.import.preview') }}</el-button>
              <el-button type="primary" :icon="Import" :disabled="!files.length || !previewed || imported" :loading="importing" @click="confirmImport">{{ t('settings.import.confirmImport') }}</el-button>
              <span v-if="previewed && !imported" class="meta">{{ t('settings.import.previewDone') }}</span>
              <span v-if="imported" class="text-ok ops-done"><CircleCheck :size="14" />{{ t('settings.import.imported') }}</span>
            </div>

            <div v-for="r in results" :key="r.file" class="result">
              <div class="result-file"><FileText :size="14" /><span class="mono">{{ r.file }}</span></div>
              <el-alert v-if="r.error" type="error" :title="r.error" :closable="false" show-icon />
              <el-alert v-else-if="!r.reports.length" type="info" :title="t('settings.import.noModules')" :closable="false" show-icon />
              <el-table v-else :data="r.reports" size="small" row-key="module" class="result-table">
                <el-table-column type="expand" width="32">
                  <template #default="{ row }">
                    <ul v-if="row.warnings?.length" class="warns">
                      <li v-for="w in row.warnings" :key="w">{{ w }}</li>
                    </ul>
                    <div v-else class="meta warns">{{ t('settings.import.noWarnings') }}</div>
                  </template>
                </el-table-column>
                <el-table-column prop="title" :label="t('settings.import.colModule')" min-width="90" />
                <el-table-column prop="found" :label="t('settings.import.colFound')" width="60" align="right" />
                <el-table-column :label="imported ? t('settings.import.imported') : t('settings.import.colImportable')" width="70" align="right">
                  <template #default="{ row }"><b>{{ row.imported }}</b></template>
                </el-table-column>
                <el-table-column :label="t('settings.import.colWarnings')" width="70" align="right">
                  <template #default="{ row }">
                    <span v-if="row.warnings?.length" class="warn-count">{{ row.warnings.length }}</span>
                    <span v-else class="muted">0</span>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </div>
        </div>

        <div class="grid-2">
          <div class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><DownloadCloud :size="16" />{{ t('settings.backup.title') }}</div>
            </div>
            <div class="g-card-body">
              <p class="desc">{{ t('settings.backup.desc') }}</p>
              <el-button type="primary" plain :icon="Download" :loading="downloading" @click="downloadBackup">{{ t('settings.backup.download') }}</el-button>
            </div>
          </div>
          <div class="g-card">
            <div class="g-card-head">
              <div class="g-card-title"><UploadCloud :size="16" />{{ t('settings.restore.title') }}</div>
            </div>
            <div class="g-card-body">
              <i18n-t keypath="settings.restore.desc" tag="p" class="desc">
                <template #bold><b>{{ t('settings.restore.descBold') }}</b></template>
              </i18n-t>
              <div class="restore-row">
                <label class="file-btn">
                  <FileArchive :size="14" />
                  <span>{{ restoreFile ? restoreFile.name : t('settings.restore.pickFile') }}</span>
                  <input type="file" accept=".zip,.enc" @change="onRestorePick" />
                </label>
                <el-input v-if="restoreFile?.name.endsWith('.enc')" v-model="restorePassword" show-password :placeholder="t('settings.restore.passwordPlaceholder')" style="width: 160px" />
                <el-button type="danger" :icon="ArchiveRestore" :disabled="!restoreFile" :loading="restoring" @click="confirmRestore">{{ t('settings.restore.button') }}</el-button>
              </div>
              <el-checkbox v-model="restoreKeepAccount" class="restore-keep">{{ t('settings.restore.keepAccount') }}</el-checkbox>
              <p class="desc restore-keep-hint">{{ t('settings.restore.keepAccountHint') }}</p>
            </div>
          </div>
        </div>

        <!-- 配置备份模块关闭时隐藏定时备份 -->
        <BackupSchedule v-if="moduleEnabled('backup')" />
      </el-tab-pane>

      <!-- ============ 诊断 ============ -->
      <el-tab-pane name="diagnostics">
        <template #label><span class="tab-label"><Stethoscope :size="15" />{{ t('settings.tabs.diagnostics') }}</span></template>
        <DiagnosticsCard />
      </el-tab-pane>

    </el-tabs>

    <!-- 自定义页面预览：iframe 不带任何 sandbox 许可，页面中的脚本、表单与外部资源都不会执行 -->
    <el-dialog v-model="denyPreview" :title="t('settings.restrict.deny.previewTitle', { code: form?.denyStatus ?? 200 })" width="760px" class="deny-preview-dialog" append-to-body>
      <iframe v-if="denyPreview" class="deny-preview" sandbox="" referrerpolicy="no-referrer" :srcdoc="denyPreviewDoc" :title="t('settings.restrict.deny.preview')" />
    </el-dialog>

    <RestartOverlay ref="overlay" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import BackupSchedule from '../components/BackupSchedule.vue'
import DiagnosticsCard from '../components/DiagnosticsCard.vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { ElMessage, ElMessageBox, type InputInstance, type UploadUserFile } from 'element-plus'
import {
  BookOpen,
  LoaderCircle,
  ShieldOff,
  ArchiveRestore,
  Blocks,
  CircleCheck,
  Code2,
  Copy,
  Cpu,
  DoorClosed,
  Download,
  DownloadCloud,
  Eye,
  FileArchive,
  FileText,
  Gauge,
  Globe,
  Import,
  KeyRound,
  LockKeyhole,
  LogOut,
  Network,
  RotateCw,
  Settings2,
  ShieldCheck,
  Smartphone,
  Sparkles,
  Stethoscope,
  TriangleAlert,
  UploadCloud,
  UserRound,
} from 'lucide-vue-next'
import { api, type ImportFileResult, type SystemInfo, type SystemSettings } from '../api'
import PageHeader from '../components/PageHeader.vue'
import { useListText, splitLines } from '../composables/useListText'
import { copyText } from '../composables/useClipboard'
import OtpInput from '../components/OtpInput.vue'
import PasskeyManager from '../components/PasskeyManager.vue'
import SecurityCenter from '../components/SecurityCenter.vue'
import PasswordStrength from '../components/PasswordStrength.vue'
import { fixTarget } from '../security'
import { loadSecurity } from '../composables/useSecurity'
import ReadonlyUsers from '../components/ReadonlyUsers.vue'
import OAuthProviders from '../components/OAuthProviders.vue'
import StatusBadge from '../components/StatusBadge.vue'
import SaveBar from '../components/SaveBar.vue'
import RestartOverlay from '../components/RestartOverlay.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { locales } from '../i18n/locales'
import { moduleEnabled, navByModule, navGroups } from '../nav'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { labelPosition } = useFormLayout()
const tabs = ['account', 'access', 'runtime', 'modules', 'backup', 'diagnostics']
const tab = ref(typeof route.query.tab === 'string' && tabs.includes(route.query.tab) ? route.query.tab : 'account')
const overlay = ref<InstanceType<typeof RestartOverlay>>()

// “关于”已独立成页，兼容旧链接
if (route.query.tab === 'about') {
  router.replace('/about')
}

// 标签页与地址栏同步，便于从用户菜单直接跳转
watch(tab, (t) => {
  router.replace({ query: { ...route.query, tab: t } })
})

// 安全评分的“去处理”：切换到对应页签，滚动到相关设置并短暂高亮
function goFix(id: string) {
  const target = fixTarget(id)
  focusAnchor(target.tab, target.anchor)
}

function focusAnchor(tabName: string, anchor: string) {
  tab.value = tabName
  // 页签内容在切换后才渲染，等两帧再定位
  nextTick(() => {
    setTimeout(() => {
      const el = document.getElementById(anchor)
      if (!el) {
        return
      }
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      el.classList.add('sec-flash')
      setTimeout(() => el.classList.remove('sec-flash'), 1800)
    }, 120)
  })
}

// 顶部加固提示跳转时带有 focus 锚点：定位后移除参数，刷新页面不会重复滚动
onMounted(() => {
  const focus = route.query.focus
  if (typeof focus === 'string' && focus.startsWith('sec-')) {
    focusAnchor(tab.value, focus)
    const { focus: _f, ...rest } = route.query
    router.replace({ query: rest })
  }
})

// ---------- 全局设置 ----------
const form = ref<SystemSettings>()
const savedJSON = ref('')
const saving = ref(false)
const dirty = computed(() => !!form.value && JSON.stringify(normalized(form.value)) !== savedJSON.value)

// 陌生来源失败预算：0 为默认值 20，负数为关闭
const globalLimitOn = computed({
  get: () => (form.value?.loginGlobalLimit ?? 0) >= 0,
  set: (v: boolean) => {
    if (form.value) {
      form.value.loginGlobalLimit = v ? 0 : -1
    }
  },
})
const globalLimitValue = computed({
  get: () => ((form.value?.loginGlobalLimit ?? 0) > 0 ? form.value!.loginGlobalLimit : 20),
  set: (v: number) => {
    if (form.value) {
      form.value.loginGlobalLimit = v === 20 ? 0 : v
    }
  },
})

function normalized(s: SystemSettings) {
  return {
    ...s,
    dnsServers: s.dnsServers ?? [],
    trustedProxies: s.trustedProxies ?? [],
    thirdLogin: normalizedLogin(s.thirdLogin),
    disabledModules: [...(s.disabledModules ?? [])].sort(),
    restartRequired: false,
  }
}

// 后端的空列表会序列化为 null，统一成数组便于编辑与比较
function normalizedLogin(t?: SystemSettings['thirdLogin']): SystemSettings['thirdLogin'] {
  return {
    enable: t?.enable ?? false,
    callbackBase: t?.callbackBase ?? '',
    providers: t?.providers ?? [],
    allowed: t?.allowed ?? [],
    skipTotp: t?.skipTotp ?? false,
  }
}

// 第三方登录设置（模板中仅在表单加载后渲染）
const login = computed(() => form.value?.thirdLogin ?? normalizedLogin())

// 允许的身份：输入框保留用户原始文本（含空行、行尾换行），只把解析后的列表写回表单（见 useListText）
const allowedText = useListText(
  () => login.value.allowed,
  (v) => {
    if (form.value) {
      form.value.thirdLogin.allowed = v
    }
  },
  { split: splitLines },
)

const dnsServers = computed({
  get: () => form.value?.dnsServers ?? [],
  set: (v: string[]) => {
    if (form.value) {
      form.value.dnsServers = v.map((s) => s.trim()).filter(Boolean)
    }
  },
})

// 界面上用"日志落盘"开关，配置中存的是关闭标志（默认落盘）
const logPersist = computed({
  get: () => !form.value?.disableLogPersist,
  set: (v: boolean) => {
    if (form.value) {
      form.value.disableLogPersist = !v
    }
  },
})

// 界面上用"信任"开关，配置中存的是关闭标志（默认信任）
const trustProxy = computed({
  get: () => !form.value?.disableProxyHeaders,
  set: (v: boolean) => {
    if (form.value) {
      form.value.disableProxyHeaders = !v
    }
  },
})

const trustedProxies = computed({
  get: () => form.value?.trustedProxies ?? [],
  set: (v: string[]) => {
    if (form.value) {
      form.value.trustedProxies = v.map((s) => s.trim()).filter(Boolean)
    }
  },
})

const entryPath = computed(() => {
  const p = (form.value?.safePath ?? '').trim()
  if (!p) {
    return ''
  }
  return p.startsWith('/') ? p : '/' + p
})

function applySettings(s: SystemSettings) {
  form.value = { ...s, dnsServers: s.dnsServers ?? [], disabledModules: s.disabledModules ?? [], thirdLogin: normalizedLogin(s.thirdLogin) }
  savedJSON.value = JSON.stringify(normalized(form.value))
}

async function loadSettings() {
  applySettings(await api.get<SystemSettings>('/api/system/settings'))
}

async function saveSettings() {
  if (!form.value) {
    return
  }
  saving.value = true
  try {
    const r = await api.put<SystemSettings>('/api/system/settings', form.value)
    applySettings(r)
    ElMessage.success(r.restartRequired ? t('settings.save.savedNeedRestart') : t('settings.save.saved'))
    loadInfo()
    // 安全入口、锁定策略等会影响安全评分
    loadSecurity()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// setAutoUpdateCheck 切换“自动检查更新”：设置中保存的是关闭标记 disableUpdateCheck，默认开启
function setAutoUpdateCheck(v: string | number | boolean) {
  if (form.value) {
    form.value.disableUpdateCheck = !v
  }
}

function resetSettings() {
  if (savedJSON.value) {
    form.value = { ...JSON.parse(savedJSON.value), restartRequired: form.value?.restartRequired ?? false }
  }
}

// ---------- 未授权访问的响应方式 ----------
// 与后端 deny.go 一致：自定义页面可选的状态码与大小上限（按 UTF-8 字节计）
const denyPageMax = 64 * 1024
const denyPreview = ref(false)
const denyModes = computed(() =>
  ['default', 'notfound', 'blank', 'close', 'drop', 'custom'].map((value) => ({ value, label: t(`settings.restrict.deny.modes.${value}`) })),
)
// 默认方式在配置中是空串，el-select 会把空串当作未选择而显示占位文字，界面上用 default 代替
const denyModeSel = computed({
  get: () => form.value?.denyMode || 'default',
  set: (v: string) => {
    if (form.value) {
      form.value.denyMode = (v === 'default' ? '' : v) as SystemSettings['denyMode']
    }
  },
})
// 状态码说明用 HTTP 标准原因短语，不随界面语言变化
const denyStatuses = [
  [200, 'OK'],
  [403, 'Forbidden'],
  [404, 'Not Found'],
  [410, 'Gone'],
  [500, 'Internal Server Error'],
  [502, 'Bad Gateway'],
  [503, 'Service Unavailable'],
].map(([value, reason]) => ({ value: value as number, label: `${value} ${reason}` }))
const denyModeHelp = computed(() => t(`settings.restrict.deny.help.${form.value?.denyMode || 'default'}`))
// 预览时加上与服务端相同的 CSP（deny.go 的 denyPageCSP，sandbox 由 iframe 属性提供），
// 外部图片、样式表等不会加载，预览效果与访客所见一致，也不会让第三方得知管理员的地址；
// meta 放在 doctype 之后，避免页面退回怪异模式
const denyPreviewCSP = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; media-src data:; form-action 'none'; base-uri 'none'">`
const denyPreviewDoc = computed(() => {
  const page = form.value?.denyPage ?? ''
  const m = /^\s*<!doctype[^>]*>/i.exec(page)
  return m ? m[0] + denyPreviewCSP + page.slice(m[0].length) : denyPreviewCSP + page
})
const denyPageBytes = computed(() => new TextEncoder().encode(form.value?.denyPage ?? '').length)

// denyWarnings 提示所选方式的边界：入口输错时不再有提示、端口本身仍然开放
const denyWarnings = computed(() => {
  const f = form.value
  if (!f || !f.denyMode) {
    return []
  }
  const out: string[] = []
  if (f.safePath.trim()) {
    out.push(t('settings.restrict.deny.warnEntry'))
    // 入口 Cookie 在 HTTPS 下写入带 Secure，http:// 请求不带它，隐藏方式下拦截先于强制 HTTPS 跳转
    if (f.forceHTTPS) {
      out.push(t('settings.restrict.deny.warnHttp'))
    }
  }
  if (f.denyMode === 'close' || f.denyMode === 'drop') {
    out.push(t('settings.restrict.deny.warnPort'))
  }
  if (!f.safePath.trim() && !f.lanOnly) {
    out.push(t('settings.restrict.deny.warnNoTrigger'))
  }
  return out
})

// formatKB 按 0.1 KB 向上取整，刚超出上限时不会显示成与上限相同的数值
function formatKB(n: number) {
  if (n < 1024) {
    return `${n} B`
  }
  return `${(Math.ceil((n / 1024) * 10) / 10).toFixed(1).replace(/\.0$/, '')} KB`
}

async function fillDenySample() {
  const f = form.value
  if (!f) {
    return
  }
  if (f.denyPage.trim()) {
    try {
      await ElMessageBox.confirm(t('settings.restrict.deny.sampleConfirm'), { type: 'warning' })
    } catch {
      return
    }
  }
  f.denyPage = denySample
}

// 示例页面：仿照常见 Web 服务器的默认欢迎页，不含任何产品信息
const denySample = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Welcome</title>
<style>
body { margin: 0; padding: 48px 24px; font-family: sans-serif; color: #333; }
h1 { font-size: 28px; font-weight: normal; }
</style>
</head>
<body>
<h1>It works!</h1>
<p>This is the default web page for this server.</p>
</body>
</html>
`

function randomEntry() {
  if (form.value) {
    form.value.safePath = '/' + Math.random().toString(36).slice(2, 10)
  }
}

// TOTP 密钥所在节点，自动复制失败时选中它供手动复制
const totpSecretEl = ref<HTMLElement>()

// 预设项的显示名依赖当前语言，用 computed 保证切换语言后刷新
const dnsPresets = computed(() => [
  { value: '223.5.5.5', label: t('settings.network.dns.ali') },
  { value: '119.29.29.29', label: t('settings.network.dns.tencent') },
  { value: '180.76.76.76', label: t('settings.network.dns.baidu') },
  { value: '114.114.114.114', label: t('settings.network.dns.114') },
  { value: '1.1.1.1', label: t('settings.network.dns.cloudflare') },
  { value: '8.8.8.8', label: t('settings.network.dns.google') },
  { value: '9.9.9.9', label: t('settings.network.dns.quad9') },
])

const timeZones = computed(() => [
  { value: 'Asia/Shanghai', label: t('settings.network.zones.china') },
  { value: 'Asia/Hong_Kong', label: t('settings.network.zones.hongKong') },
  { value: 'Asia/Taipei', label: t('settings.network.zones.taipei') },
  { value: 'Asia/Tokyo', label: t('settings.network.zones.tokyo') },
  { value: 'Asia/Seoul', label: t('settings.network.zones.seoul') },
  { value: 'Asia/Singapore', label: t('settings.network.zones.singapore') },
  { value: 'Europe/London', label: t('settings.network.zones.london') },
  { value: 'Europe/Berlin', label: t('settings.network.zones.berlin') },
  { value: 'Europe/Moscow', label: t('settings.network.zones.moscow') },
  { value: 'America/New_York', label: t('settings.network.zones.newYork') },
  { value: 'America/Chicago', label: t('settings.network.zones.chicago') },
  { value: 'America/Los_Angeles', label: t('settings.network.zones.losAngeles') },
  { value: 'Australia/Sydney', label: t('settings.network.zones.sydney') },
  { value: 'UTC', label: t('settings.network.zones.utc') },
])

// ---------- 功能模块 ----------
// 模块显示名：有导航项的用导航名称（与侧边栏一致），其余用后端名称
function moduleLabel(id: string): string {
  const nav = navByModule(id)
  return nav ? t(nav.titleKey) : (form.value?.modules?.find((m) => m.id === id)?.title ?? id)
}

// lockedBy 返回仍依赖该模块的开启模块（含管理界面 HTTPS），非空时不能关闭
function lockedBy(id: string): string {
  const f = form.value
  if (!f) {
    return ''
  }
  const off = f.disabledModules ?? []
  const names = (f.modules ?? []).filter((m) => !off.includes(m.id) && (m.requires ?? []).includes(id)).map((m) => moduleLabel(m.id))
  if (id === 'ssl' && f.httpsListen) {
    names.push(t('settings.modules.adminHttps'))
  }
  return names.join(t('settings.modules.separator'))
}

// 没有导航项的模块的说明文案
const extraDesc: Record<string, string> = { backup: 'settings.modules.backupDesc' }

// 按侧边栏分组排列；没有导航项的模块（如配置备份）归入系统分组
const moduleGroups = computed(() => {
  const all = form.value?.modules ?? []
  const off = form.value?.disabledModules ?? []
  const item = (id: string) => {
    const nav = navByModule(id)
    return {
      id,
      title: moduleLabel(id),
      icon: nav?.icon ?? ArchiveRestore,
      desc: nav?.descKey ? t(nav.descKey) : extraDesc[id] ? t(extraDesc[id]) : '',
      on: !off.includes(id),
      lockedBy: off.includes(id) ? '' : lockedBy(id),
    }
  }
  const placed = new Set<string>()
  const groups = navGroups
    .map((g) => {
      const items = g.items.filter((i) => i.moduleId && all.some((m) => m.id === i.moduleId)).map((i) => item(i.moduleId as string))
      items.forEach((i) => placed.add(i.id))
      return { titleKey: g.titleKey, items }
    })
    .filter((g) => g.items.length > 0)
  const rest = all.filter((m) => !placed.has(m.id)).map((m) => item(m.id))
  const system = groups.find((g) => g.titleKey === 'nav.groups.system')
  if (system) {
    system.items.push(...rest)
  } else if (rest.length) {
    groups.push({ titleKey: 'nav.groups.system', items: rest })
  }
  return groups
})

// toggleModule 切换模块开关；开启时连同它依赖的模块一起开启
function toggleModule(id: string, on: boolean) {
  const f = form.value
  if (!f) {
    return
  }
  const off = new Set(f.disabledModules ?? [])
  if (on) {
    // 依赖可能是多级的，逐层开启；seen 防止重复访问
    const seen = new Set<string>()
    const stack = [id]
    while (stack.length) {
      const cur = stack.pop() as string
      if (seen.has(cur)) {
        continue
      }
      seen.add(cur)
      off.delete(cur)
      stack.push(...(f.modules?.find((m) => m.id === cur)?.requires ?? []))
    }
  } else {
    off.add(id)
  }
  f.disabledModules = [...off].sort()
}

// ---------- 账号 ----------
const account = reactive({ oldPassword: '', username: '', newPassword: '', confirm: '' })
const accountSaving = ref(false)

async function saveAccount() {
  if (account.newPassword !== account.confirm) {
    ElMessage.error(t('settings.admin.mismatch'))
    return
  }
  accountSaving.value = true
  try {
    await api.put('/api/account', { oldPassword: account.oldPassword, username: account.username, newPassword: account.newPassword })
    ElMessage.success(t('settings.admin.changed'))
    router.replace({ name: 'login' })
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    accountSaving.value = false
  }
}

// ---------- 退出所有设备 ----------
const sessionsRevoking = ref(false)

async function revokeAllSessions() {
  try {
    await ElMessageBox.confirm(t('settings.sessions.confirm'), t('settings.sessions.button'), { type: 'warning' })
  } catch {
    return
  }
  sessionsRevoking.value = true
  try {
    await api.post('/api/account/sessions/revoke')
    ElMessage.success(t('settings.sessions.done'))
    router.replace({ name: 'login' })
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    sessionsRevoking.value = false
  }
}

// ---------- 两步验证 ----------
const totp = reactive({
  enabled: false,
  secret: '',
  qr: '',
  code: '',
  password: '',
  // 弹窗状态：mode 区分启用与关闭
  dialog: false,
  mode: 'enable' as 'enable' | 'disable',
  preparing: false,
  submitting: false,
})
const totpPasswordInput = ref<InputInstance>()
const totpCodeInput = ref<InstanceType<typeof OtpInput>>()

async function loadTOTP() {
  const r = await api.get<{ enabled: boolean }>('/api/account/totp')
  totp.enabled = r.enabled
}

// 生成新密钥与二维码后弹出启用弹窗
async function setupTOTP() {
  totp.preparing = true
  try {
    const r = await api.post<{ secret: string; url: string }>('/api/account/totp/setup')
    totp.secret = r.secret
    totp.qr = await QRCode.toDataURL(r.url, { width: 180, margin: 1 })
    openTOTP('enable')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    totp.preparing = false
  }
}

function openTOTP(mode: 'enable' | 'disable') {
  totp.mode = mode
  totp.code = ''
  totp.password = ''
  totp.dialog = true
}

// 弹窗关闭后清空敏感输入与未确认的密钥
function cancelTOTP() {
  Object.assign(totp, { secret: '', qr: '', code: '', password: '', submitting: false })
}

// 先填密码再输验证码；密码为空时聚焦密码框
function focusTOTP() {
  if (!totp.password) {
    totpPasswordInput.value?.focus()
  } else {
    totpCodeInput.value?.focus()
  }
}

async function submitTOTP() {
  if (totp.submitting) {
    return
  }
  if (!totp.password) {
    ElMessage.warning(t('settings.totp.passwordRequired'))
    totpPasswordInput.value?.focus()
    return
  }
  if (totp.code.length < 6) {
    totpCodeInput.value?.focus()
    return
  }
  totp.submitting = true
  const enabling = totp.mode === 'enable'
  try {
    // 启用与关闭时后端都要求同时校验密码与当前验证码
    await api.post(enabling ? '/api/account/totp/enable' : '/api/account/totp/disable', { password: totp.password, code: totp.code })
    totp.enabled = enabling
    totp.dialog = false
    loadSecurity()
    ElMessage.success(t(enabling ? 'settings.totp.enabledMsg' : 'settings.totp.disabledMsg'))
  } catch (e) {
    ElMessage.error((e as Error).message)
    // 验证失败：抖动并清空验证码，方便重新输入
    totp.code = ''
    totpCodeInput.value?.shake()
    totp.submitting = false
    await nextTick()
    totpCodeInput.value?.focus()
  } finally {
    totp.submitting = false
  }
}

// ---------- Lucky 导入 ----------
const files = ref<UploadUserFile[]>([])
const results = ref<ImportFileResult[]>([])
const keepEnable = ref(false)
const busy = ref(false)
const importing = ref(false)
const previewed = ref(false)
const imported = ref(false)

// 文件列表或启用选项变化后需要重新预览
watch([files, keepEnable], () => {
  previewed.value = false
  imported.value = false
  results.value = []
})

async function runImport(dryRun: boolean) {
  const fd = new FormData()
  for (const f of files.value) {
    if (f.raw) {
      fd.append('files', f.raw, f.name)
    }
  }
  const q = new URLSearchParams()
  if (dryRun) {
    q.set('dryRun', '1')
  }
  if (keepEnable.value) {
    q.set('enable', '1')
  }
  busy.value = true
  importing.value = !dryRun
  try {
    const qs = q.toString()
    const r = await api.post<ImportFileResult[]>(`/api/import/lucky${qs ? '?' + qs : ''}`, fd)
    results.value = r
    previewed.value = true
    imported.value = !dryRun
    if (!dryRun) {
      const total = r.reduce((n, f) => n + f.reports.reduce((m, x) => m + x.imported, 0), 0)
      ElMessage.success(t('settings.import.done', { total }))
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
    importing.value = false
  }
}

async function confirmImport() {
  const total = results.value.reduce((n, f) => n + f.reports.reduce((m, x) => m + x.imported, 0), 0)
  try {
    await ElMessageBox.confirm(
      keepEnable.value
        ? t('settings.import.confirmKeep', { total })
        : t('settings.import.confirmDisabled', { total }),
      t('settings.import.confirmImport'),
      { type: keepEnable.value ? 'warning' : 'info', confirmButtonText: t('settings.import.confirmButton'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  await runImport(false)
}

// ---------- 备份与恢复 ----------
const downloading = ref(false)
const restoreFile = ref<File>()
const restorePassword = ref('')
// 恢复时保留当前管理员账号（不用备份中的 account.json 覆盖）
const restoreKeepAccount = ref(false)
const restoring = ref(false)

async function downloadBackup() {
  downloading.value = true
  try {
    const resp = await fetch('/api/system/backup', { credentials: 'same-origin' })
    if (!resp.ok) {
      const body = await resp.text()
      let msg = t('settings.backup.downloadFailed', { status: resp.status })
      try {
        msg = JSON.parse(body).error ?? msg
      } catch {
        // 非 JSON 错误响应保持默认提示
      }
      throw new Error(msg)
    }
    const blob = await resp.blob()
    const cd = resp.headers.get('Content-Disposition') ?? ''
    const m = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(cd)
    const name = m ? decodeURIComponent(m[1]) : `gately-backup-${new Date().toISOString().slice(0, 10)}.zip`
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    document.body.appendChild(a)
    a.click()
    a.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    downloading.value = false
  }
}

function onRestorePick(e: Event) {
  restoreFile.value = (e.target as HTMLInputElement).files?.[0]
}

async function confirmRestore() {
  if (!restoreFile.value) {
    return
  }
  // 确认口令随当前语言变化
  const confirmWord = t('settings.restore.word')
  try {
    await ElMessageBox.prompt(
      t('settings.restore.confirm', { name: restoreFile.value.name, word: confirmWord }),
      t('settings.restore.title'),
      {
        type: 'warning',
        confirmButtonText: t('settings.restore.confirmButton'),
        cancelButtonText: t('common.cancel'),
        confirmButtonClass: 'el-button--danger',
        inputPlaceholder: confirmWord,
        inputValidator: (v: string) => v === confirmWord || t('settings.restore.inputHint', { word: confirmWord }),
      },
    )
  } catch {
    return
  }
  const fd = new FormData()
  fd.append('file', restoreFile.value, restoreFile.value.name)
  if (restorePassword.value) {
    fd.append('password', restorePassword.value)
  }
  if (restoreKeepAccount.value) {
    fd.append('keepAccount', '1')
  }
  restoring.value = true
  try {
    const r = await api.post<{ restored: number; previous: string }>('/api/system/restore', fd)
    ElMessage.success(t('settings.restore.success', { count: r.restored, previous: r.previous }))
    overlay.value?.start(t('settings.restore.progress'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    restoring.value = false
  }
}

// ---------- 系统信息 / 重启 ----------
const info = ref<SystemInfo>()

async function loadInfo() {
  try {
    info.value = await api.get<SystemInfo>('/api/system/info')
  } catch {
    // 忽略，相关提示显示为“-”
  }
}

async function confirmRestart() {
  try {
    await ElMessageBox.confirm(t('settings.restart.confirm'), t('settings.restart.button'), {
      type: 'warning',
      confirmButtonText: t('settings.restart.confirmButton'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await api.post('/api/system/restart')
    overlay.value?.start(t('settings.restart.progress'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(async () => {
  loadTOTP()
  loadInfo()
  loadSettings()
  try {
    const s = await api.get<{ username: string }>('/api/session')
    account.username = s.username
  } catch {
    // 未登录时 api 已跳转
  }
})
</script>

<style scoped>
.restart-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  margin-bottom: 12px;
  border-radius: var(--g-radius);
  border: 1px solid var(--el-color-warning-light-5);
  background: var(--el-color-warning-light-9);
}
.rb-icon {
  color: var(--el-color-warning);
  flex-shrink: 0;
}
.rb-text {
  flex: 1;
  min-width: 0;
}
.rb-title {
  font-weight: 600;
}
.rb-desc {
  font-size: 12px;
  color: var(--el-text-color-regular);
}
.settings-tabs :deep(.el-tabs__header) {
  margin-bottom: 16px;
}
.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  align-items: start;
}
.grid-2 + .g-card,
.grid-2 + .bk,
.g-card + .grid-2 {
  margin-top: 16px;
}
.grid-2 .g-card + .g-card {
  margin-top: 0;
}
.narrow {
  max-width: 460px;
}
.desc {
  margin: 0 0 14px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--el-text-color-regular);
}
.code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  word-break: break-all;
}
.qr-wrap {
  display: flex;
  gap: 16px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.qr {
  display: block;
  border-radius: 8px;
  border: 1px solid var(--g-border);
  background: #fff;
  padding: 6px;
}
.totp-field {
  margin-top: 16px;
}
.totp-label {
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-regular);
}
.totp-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 20px;
  margin-top: 16px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
.totp-status .spin {
  animation: totp-spin 1s linear infinite;
}
@keyframes totp-spin {
  to {
    transform: rotate(360deg);
  }
}
.secret-box {
  min-width: 0;
  flex: 1;
}
.secret {
  font-size: 13px;
  word-break: break-all;
  margin: 4px 0;
}
.token-moved {
  margin-bottom: 12px;
}
.token-links {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.w-280 {
  width: 280px;
}
/* 右对齐标签在德语、俄语等长文案下会换成多行：去掉固定的 32px 行高让标签随内容增高，避免压到下一项；
   单行标签 6px 上边距 + 20px 行高，文字中线仍与 32px 高的输入框对齐 */
.settings-tabs :deep(.el-form--label-right .el-form-item__label) {
  height: auto;
  min-height: 32px;
  padding-top: 6px;
  padding-bottom: 6px;
  line-height: 20px;
}
.deny-warns {
  display: grid;
  gap: 4px;
  width: 100%;
  margin-top: 6px;
}
.deny-warn {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-color-warning-dark-2, var(--el-color-warning));
}
.deny-warn svg {
  flex: none;
  margin-top: 2px;
}
.deny-page :deep(textarea) {
  font-size: 12px;
  line-height: 1.5;
}
.deny-page-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 4px;
}
.deny-page-ops {
  display: inline-flex;
  gap: 4px;
}
.deny-page-ops .el-button + .el-button {
  margin-left: 0;
}
.deny-size {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
.deny-size.over {
  color: var(--el-color-danger);
  font-weight: 600;
}
.deny-preview {
  display: block;
  width: 100%;
  height: min(60vh, 520px);
  border: 1px solid var(--g-border);
  border-radius: 8px;
  background: #fff;
}
.w-480 {
  width: 480px;
}
.upload :deep(.el-upload-dragger) {
  padding: 22px 16px;
  border-radius: 10px;
}
.upload-icon {
  color: var(--el-color-primary);
}
.upload-text {
  margin-top: 6px;
  font-size: 14px;
  color: var(--el-text-color-regular);
}
.upload-text em {
  color: var(--el-color-primary);
  font-style: normal;
}
.keep-enable {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin-top: 14px;
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid var(--g-border);
  background: var(--el-fill-color-lighter);
}
.ke-title {
  font-weight: 500;
}
.ke-desc {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.ke-desc b {
  color: var(--el-color-warning);
  font-weight: 600;
}
.ops {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
  margin: 14px 0 4px;
}
.ops .el-button + .el-button {
  margin-left: 0;
}
.ops-done {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
}
.result {
  margin-top: 16px;
}
.result-file {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 6px;
  word-break: break-all;
}
.result-table {
  border: 1px solid var(--g-border);
  border-radius: 8px;
}
.warns {
  margin: 0;
  padding: 4px 16px 4px 36px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--el-color-warning-dark-2, var(--el-color-warning));
}
div.warns {
  padding-left: 36px;
}
.warn-count {
  display: inline-block;
  min-width: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning);
  font-weight: 600;
  text-align: center;
}
.restore-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.restore-keep {
  margin-top: 10px;
  height: auto;
  white-space: normal;
}
.restore-keep-hint {
  margin: 2px 0 0 24px;
  font-size: 12px;
}
.file-btn {
  flex: 1;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border: 1px dashed var(--el-border-color);
  border-radius: 6px;
  font-size: 13px;
  color: var(--el-text-color-regular);
  cursor: pointer;
  overflow: hidden;
}
.file-btn span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-btn:hover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}
.file-btn input {
  display: none;
}
.modules-desc {
  margin: 0 0 6px;
}
.module-group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
  padding: 14px 0 8px;
}
.module-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 10px;
}
.module-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
}
.module-row.off .module-icon,
.module-row.off .module-name {
  color: var(--el-text-color-placeholder);
}
.module-icon {
  flex-shrink: 0;
  color: var(--el-color-primary);
}
.module-text {
  flex: 1;
  min-width: 0;
}
.module-name {
  font-size: 14px;
  font-weight: 500;
}
.module-desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.restart-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 13px;
}
@media (max-width: 1000px) {
  .grid-2 {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 767.98px) {
  .restart-banner {
    flex-wrap: wrap;
  }
  .restart-banner .el-button {
    width: 100%;
  }
  .w-280,
  .w-480 {
    width: 100%;
  }
  .settings-tabs :deep(.el-tabs__item) {
    padding: 0 12px;
  }
}

/* 从安全评分跳转过来时短暂高亮目标设置 */
.sec-flash {
  animation: sec-flash 1.8s ease;
  border-radius: var(--g-radius);
}
@keyframes sec-flash {
  0%,
  60% {
    box-shadow: 0 0 0 3px var(--el-color-primary-light-5);
    background-color: var(--el-color-primary-light-9);
  }
  100% {
    box-shadow: 0 0 0 3px transparent;
    background-color: transparent;
  }
}
.limit-num {
  margin-left: 12px;
}
</style>
