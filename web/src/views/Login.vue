<template>
  <div class="login">
    <!-- 左侧品牌区：主题渐变 + 网格纹理 + 网络拓扑装饰，仅宽屏显示 -->
    <section class="hero">
      <div class="hero-grid" aria-hidden="true" />
      <div class="hero-glow g1" aria-hidden="true" />
      <div class="hero-glow g2" aria-hidden="true" />
      <svg class="hero-net" viewBox="0 0 400 300" aria-hidden="true">
        <g class="links">
          <line x1="60" y1="70" x2="190" y2="140" />
          <line x1="190" y1="140" x2="330" y2="60" />
          <line x1="190" y1="140" x2="300" y2="230" />
          <line x1="190" y1="140" x2="80" y2="240" />
          <line x1="330" y1="60" x2="300" y2="230" />
        </g>
        <circle class="node hub" cx="190" cy="140" r="9" />
        <circle class="node" cx="60" cy="70" r="5" />
        <circle class="node" cx="330" cy="60" r="5" />
        <circle class="node" cx="300" cy="230" r="5" />
        <circle class="node" cx="80" cy="240" r="5" />
        <circle class="pulse" cx="190" cy="140" r="9" />
      </svg>

      <div class="hero-inner">
        <div class="hero-brand">
          <AppLogo :size="38" />
          <span>Gately</span>
        </div>
        <div class="hero-copy">
          <span class="badge"><span class="dot" />{{ t('login.badge') }}</span>
          <h1>{{ t('login.heroLine1') }}<br /><span class="accent">{{ t('login.heroLine2') }}</span></h1>
        </div>
        <ul class="features">
          <li>
            <span class="f-icon"><ArrowLeftRight :size="16" /></span>{{ t('login.feature1') }}
          </li>
          <li>
            <span class="f-icon"><Server :size="16" /></span>{{ t('login.feature2') }}
          </li>
          <li>
            <span class="f-icon"><ShieldCheck :size="16" /></span>{{ t('login.feature3') }}
          </li>
        </ul>
      </div>
    </section>

    <!-- 右侧表单区 -->
    <section class="form-side">
      <div class="toolbar">
        <LocaleSwitcher />
        <ThemePicker />
      </div>

      <div class="form-wrap">
        <div class="mobile-brand">
          <AppLogo :size="48" />
          <div class="mb-name">Gately</div>
          <div class="mb-sub">{{ t('login.mobileSub') }}</div>
        </div>

        <header class="form-head">
          <h2>{{ t('login.title') }}</h2>
          <p class="sub">{{ t('login.subtitle') }}</p>
        </header>

        <el-form :model="form" label-position="top" size="large" class="login-form" @submit.prevent="submit">
          <el-form-item :label="t('login.username')">
            <el-input v-model="form.username" autocomplete="username" :placeholder="t('login.usernamePlaceholder')" :prefix-icon="UserRound" />
          </el-form-item>
          <el-form-item :label="t('login.password')">
            <el-input v-model="form.password" type="password" show-password autocomplete="current-password" :placeholder="t('login.passwordPlaceholder')" :prefix-icon="LockKeyhole" />
          </el-form-item>
          <el-button type="primary" native-type="submit" :loading="loading && !needCode" class="submit">
            {{ t('login.submit') }}
            <ArrowRight :size="16" class="submit-icon" />
          </el-button>
        </el-form>

        <div v-if="canPasskey || fnosUser" class="third">
          <div class="third-title"><span>{{ t('login.or') }}</span></div>
          <div class="third-stack">
            <!-- 经飞牛统一网关访问且已开启飞牛登录态自动登录时显示 -->
            <button v-if="fnosUser" type="button" class="third-btn passkey-btn" :disabled="loading || fnosBusy" @click="loginWithFnos">
              <LoaderCircle v-if="fnosBusy" :size="16" class="spin" />
              <Server v-else :size="16" />
              <span class="third-name">{{ t('login.fnos', { user: fnosUser }) }}</span>
            </button>
            <button v-if="canPasskey" type="button" class="third-btn passkey-btn" :disabled="loading" @click="loginWithPasskey">
              <LoaderCircle v-if="passkeyBusy" :size="16" class="spin" />
              <Fingerprint v-else :size="16" />
              <span class="third-name">{{ t('login.passkey') }}</span>
            </button>
          </div>
        </div>

        <div v-if="providers.length" class="third">
          <div class="third-title"><span>{{ t('login.thirdParty') }}</span></div>
          <div class="third-list" :class="{ single: providers.length === 1 }">
            <a
              v-for="p in providers"
              :key="p.key"
              :href="p.url"
              class="third-btn"
              :class="{ busy: redirecting === p.key, disabled: redirecting && redirecting !== p.key }"
              :aria-busy="redirecting === p.key"
              @click="startThird($event, p)"
            >
              <LoaderCircle v-if="redirecting === p.key" :size="16" class="spin" />
              <svg v-else-if="p.type === 'github'" class="brand" viewBox="0 0 24 24" aria-hidden="true">
                <path
                  fill="currentColor"
                  d="M12 .5C5.65.5.5 5.65.5 12a11.5 11.5 0 0 0 7.86 10.92c.58.1.79-.25.79-.56v-2c-3.2.7-3.87-1.37-3.87-1.37-.52-1.33-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.56-.29-5.25-1.28-5.25-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 5.77 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.27 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5Z"
                />
              </svg>
              <svg v-else-if="p.type === 'google'" class="brand" viewBox="0 0 24 24" aria-hidden="true">
                <path fill="#4285F4" d="M23.5 12.27c0-.85-.08-1.67-.22-2.45H12v4.64h6.45a5.52 5.52 0 0 1-2.4 3.62v3h3.88c2.27-2.09 3.57-5.17 3.57-8.81Z" />
                <path fill="#34A853" d="M12 24c3.24 0 5.96-1.07 7.94-2.9l-3.88-3.01c-1.07.72-2.45 1.15-4.06 1.15-3.12 0-5.77-2.11-6.71-4.95H1.28v3.11A12 12 0 0 0 12 24Z" />
                <path fill="#FBBC05" d="M5.29 14.29a7.2 7.2 0 0 1 0-4.58V6.6H1.28a12 12 0 0 0 0 10.8l4.01-3.11Z" />
                <path fill="#EA4335" d="M12 4.77c1.76 0 3.34.61 4.59 1.8l3.44-3.44A11.5 11.5 0 0 0 12 0 12 12 0 0 0 1.28 6.6l4.01 3.11C6.23 6.88 8.88 4.77 12 4.77Z" />
              </svg>
              <KeyRound v-else :size="16" />
              <span class="third-name">{{ redirecting === p.key ? t('login.redirecting', { name: p.name }) : t('login.loginWith', { name: p.name }) }}</span>
            </a>
          </div>
        </div>
      </div>
    </section>

    <!-- 两步验证弹窗：输入满 6 位自动提交 -->
    <el-dialog
      v-model="needCode"
      width="420px"
      class="totp-dialog"
      modal-class="totp-modal"
      align-center
      append-to-body
      :close-on-click-modal="false"
      @opened="focusCode"
      @closed="back"
    >
      <div class="totp">
        <div class="totp-icon"><ShieldCheck :size="26" /></div>
        <h3>{{ t('login.totpTitle') }}</h3>
        <i18n-t keypath="login.totpPrompt" tag="p" class="totp-sub">
          <template #user><b>{{ form.username }}</b></template>
        </i18n-t>
        <OtpInput ref="codeInput" v-model="form.code" :disabled="loading" :aria-label="t('login.code')" @complete="submit" />
        <div class="totp-status">
          <template v-if="loading"><LoaderCircle :size="14" class="spin" />{{ t('login.verifying') }}</template>
          <template v-else>{{ t('login.totpAuto') }}</template>
        </div>
        <el-button class="back" text @click="needCode = false"><ArrowLeft :size="14" />{{ t('login.backToAccount') }}</el-button>
        <!-- 丢失验证器时只能在服务器上通过命令行关闭两步验证 -->
        <p class="totp-lost">{{ t('login.totpLost') }}</p>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { LoaderCircle, ArrowLeft, Fingerprint, ArrowLeftRight, ArrowRight, KeyRound, LockKeyhole, Server, ShieldCheck, UserRound } from 'lucide-vue-next'
import { api, ApiError } from '../api'
import AppLogo from '../components/AppLogo.vue'
import ThemePicker from '../components/ThemePicker.vue'
import LocaleSwitcher from '../components/LocaleSwitcher.vue'
import OtpInput from '../components/OtpInput.vue'
import { getPasskey, passkeySupported, type GetOptions } from '../composables/passkey'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const form = reactive({ username: '', password: '', code: '' })
const loading = ref(false)
const needCode = ref(false)
const codeInput = ref<InstanceType<typeof OtpInput>>()
// 浏览器支持 WebAuthn 时显示 Passkey 登录；服务端尚未注册 Passkey 时点击会提示
const canPasskey = passkeySupported()
const passkeyBusy = ref(false)
const providers = ref<{ key: string; name: string; type: string; url: string }[]>([])
// 可凭飞牛登录态登录的飞牛用户名（仅经飞牛统一网关访问且已开启时由后端返回）
const fnosUser = ref('')
const fnosBusy = ref(false)

// 正在跳转的第三方登录方式；非空时按钮显示进度并禁止重复点击
const redirecting = ref('')

function startThird(e: MouseEvent, p: { key: string }) {
  if (redirecting.value) {
    e.preventDefault()
    return
  }
  redirecting.value = p.key
}

// 从授权页点浏览器返回时页面可能来自缓存，需复位按钮状态
window.addEventListener('pageshow', (e) => {
  if (e.persisted) {
    redirecting.value = ''
  }
})

// 第三方登录失败时，后端带着原因跳回登录页
if (typeof route.query.oauthError === 'string') {
  ElMessage.error(route.query.oauthError)
  router.replace({ path: '/login' })
}

onMounted(async () => {
  try {
    const r = await api.get<{ providers: typeof providers.value; fnosUser?: string }>('/api/login/providers')
    providers.value = r.providers ?? []
    fnosUser.value = r.fnosUser ?? ''
  } catch {
    // 登录方式获取失败不影响密码登录
  }
})

async function submit() {
  if (loading.value) {
    return
  }
  loading.value = true
  try {
    await api.post('/api/login', form)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/overview'
    router.replace(redirect)
  } catch (e) {
    // 密码正确但需要两步验证码时弹出验证码输入框
    if (e instanceof ApiError && e.data?.totpRequired) {
      if (!needCode.value) {
        needCode.value = true
        return
      }
      // 验证码错误：抖动提示并清空，方便直接重新输入
      form.code = ''
      codeInput.value?.shake()
    }
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
    if (needCode.value) {
      await nextTick()
      focusCode()
    }
  }
}

// 使用 Passkey 登录：由验证器完成用户验证，无需密码与验证码
async function loginWithPasskey() {
  if (loading.value || passkeyBusy.value) {
    return
  }
  passkeyBusy.value = true
  try {
    const opts = await api.post<GetOptions>('/api/login/passkey/begin', {})
    const credential = await getPasskey(opts)
    await api.post('/api/login/passkey/finish', { challenge: opts.challenge, credential })
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/overview'
    router.replace(redirect)
  } catch (e) {
    // 用户在系统弹窗中取消时不提示
    if ((e as Error).name !== 'NotAllowedError') {
      ElMessage.error((e as Error).message)
    }
  } finally {
    passkeyBusy.value = false
  }
}

// 使用飞牛登录态登录：飞牛网关已校验登录，后端直接签发管理员会话
async function loginWithFnos() {
  if (loading.value || fnosBusy.value) {
    return
  }
  fnosBusy.value = true
  try {
    await api.post('/api/login/fnos', {})
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/overview'
    router.replace(redirect)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    fnosBusy.value = false
  }
}

function focusCode() {
  codeInput.value?.focus()
}

// 关闭验证码弹窗后回到账号密码步骤
function back() {
  needCode.value = false
  form.code = ''
}
</script>

<style scoped>
.login {
  --hero-w: 46%;
  min-height: 100%;
  display: flex;
  background: var(--el-bg-color);
}

/* ---------- 左侧品牌区 ---------- */
.hero {
  position: relative;
  width: var(--hero-w);
  max-width: 720px;
  flex-shrink: 0;
  overflow: hidden;
  color: #eafaf5;
  background: linear-gradient(155deg, var(--g-hero-1) 0%, var(--g-hero-2) 55%, var(--g-hero-3) 100%);
  isolation: isolate;
}
.hero-grid {
  position: absolute;
  inset: 0;
  z-index: -1;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.07) 1px, transparent 1px);
  background-size: 44px 44px;
  mask-image: radial-gradient(ellipse 80% 70% at 60% 40%, #000 30%, transparent 75%);
}
.hero-glow {
  position: absolute;
  z-index: -1;
  border-radius: 50%;
  filter: blur(70px);
  opacity: 0.55;
}
.hero-glow.g1 {
  width: 420px;
  height: 420px;
  top: -120px;
  right: -100px;
  background: var(--g-logo-dot);
  opacity: 0.2;
  animation: drift 16s ease-in-out infinite alternate;
}
.hero-glow.g2 {
  width: 360px;
  height: 360px;
  bottom: -140px;
  left: -80px;
  background: var(--g-hero-3);
  animation: drift 20s ease-in-out infinite alternate-reverse;
}
.hero-net {
  position: absolute;
  z-index: -1;
  right: -70px;
  top: 4%;
  width: 64%;
  max-width: 440px;
  opacity: 0.45;
}
.hero-net .links line {
  stroke: rgba(255, 255, 255, 0.35);
  stroke-width: 1.2;
  stroke-dasharray: 4 6;
  animation: flow 3s linear infinite;
}
.hero-net .node {
  fill: rgba(255, 255, 255, 0.85);
}
.hero-net .hub {
  fill: var(--g-logo-dot);
}
.hero-net .pulse {
  fill: none;
  stroke: var(--g-logo-dot);
  stroke-width: 2;
  transform-origin: 190px 140px;
  animation: pulse 2.6s ease-out infinite;
}
.hero-inner {
  height: 100%;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 44px 52px;
}
.hero-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 21px;
  font-weight: 700;
  letter-spacing: 0.2px;
  color: #fff;
}
.hero-brand :deep(.logo-bg) {
  fill: rgba(255, 255, 255, 0.16);
}
.hero-copy {
  margin: auto 0;
  padding: 40px 0;
}
.badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 5px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 500;
  color: #fff;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.18);
  backdrop-filter: blur(6px);
}
.badge .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--g-logo-dot);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--g-logo-dot) 30%, transparent);
}
.hero h1 {
  margin: 20px 0 0;
  font-size: clamp(30px, 3.2vw, 44px);
  line-height: 1.25;
  font-weight: 800;
  letter-spacing: -0.5px;
  color: #fff;
}
.hero h1 .accent {
  background: linear-gradient(90deg, #fff 0%, var(--g-logo-dot) 120%);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.features {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.features li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
  font-size: 14px;
  background: rgba(255, 255, 255, 0.07);
  border: 1px solid rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(8px);
}
.f-icon {
  flex-shrink: 0;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  background: rgba(255, 255, 255, 0.14);
}

/* ---------- 右侧表单区 ---------- */
.form-side {
  position: relative;
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 80px 32px 48px;
}
.toolbar {
  position: absolute;
  top: 18px;
  right: 20px;
  display: flex;
  gap: 8px;
  z-index: 1;
}
.form-wrap {
  width: 100%;
  max-width: 384px;
  animation: rise 0.5s cubic-bezier(0.2, 0.8, 0.2, 1) both;
}
.mobile-brand {
  display: none;
}
.form-head {
  margin-bottom: 28px;
}
h2 {
  margin: 0;
  font-size: 28px;
  font-weight: 750;
  letter-spacing: -0.3px;
}
.sub {
  margin: 8px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 14px;
}
.login-form :deep(.el-form-item__label) {
  font-weight: 500;
  margin-bottom: 6px;
}
.login-form :deep(.el-input__wrapper) {
  height: 46px;
  border-radius: 11px;
  padding: 0 14px;
  background: var(--el-fill-color-light);
  box-shadow: 0 0 0 1px transparent inset;
  transition:
    box-shadow 0.15s,
    background 0.15s;
}
.login-form :deep(.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px var(--el-border-color) inset;
}
.login-form :deep(.el-input__wrapper.is-focus) {
  background: var(--el-bg-color);
  box-shadow:
    0 0 0 1px var(--el-color-primary) inset,
    0 0 0 4px color-mix(in srgb, var(--el-color-primary) 16%, transparent);
}
.login-form :deep(.el-input__prefix) {
  margin-right: 4px;
}
.submit {
  width: 100%;
  height: 46px;
  margin-top: 8px;
  border-radius: 11px;
  font-size: 15px;
  font-weight: 600;
  border: 0;
  background: linear-gradient(135deg, var(--el-color-primary-light-3) -40%, var(--el-color-primary) 60%);
  box-shadow: 0 10px 24px -10px color-mix(in srgb, var(--el-color-primary) 70%, transparent);
  transition:
    transform 0.15s,
    box-shadow 0.15s,
    filter 0.15s;
}
.submit:hover {
  filter: brightness(1.06);
  transform: translateY(-1px);
  box-shadow: 0 14px 28px -12px color-mix(in srgb, var(--el-color-primary) 80%, transparent);
}
.submit:active {
  transform: translateY(0);
}
.submit-icon {
  margin-left: 6px;
  transition: transform 0.15s;
}
.submit:hover .submit-icon {
  transform: translateX(3px);
}

.third {
  margin-top: 26px;
}
.third-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: var(--el-text-color-placeholder);
  font-size: 12px;
}
.third-title::before,
.third-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--g-border);
}
.third-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
}
.third-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-width: 0;
  height: 44px;
  padding: 0 12px;
  border: 1px solid var(--el-border-color);
  border-radius: 11px;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color);
  text-decoration: none;
  font-size: 14px;
  font-weight: 500;
  transition:
    background 0.15s,
    border-color 0.15s,
    transform 0.15s;
}
.third-btn:hover {
  border-color: var(--el-border-color-darker, var(--el-border-color));
  background: var(--el-fill-color-light);
  transform: translateY(-1px);
}
.third-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.brand {
  width: 17px;
  height: 17px;
  flex-shrink: 0;
}
.third-stack {
  display: grid;
  gap: 10px;
}
.passkey-btn {
  width: 100%;
  cursor: pointer;
  font-family: inherit;
}
.passkey-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.third-btn.busy {
  opacity: 0.8;
  pointer-events: none;
}
.third-btn.disabled {
  opacity: 0.5;
  pointer-events: none;
}
.spin {
  flex-shrink: 0;
  animation: spin 1s linear infinite;
}

/* ---------- 两步验证弹窗 ---------- */
.totp {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 0 6px 4px;
}
.totp-icon {
  width: 56px;
  height: 56px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  box-shadow: 0 0 0 6px color-mix(in srgb, var(--el-color-primary) 7%, transparent);
}
.totp h3 {
  margin: 18px 0 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}
.totp-sub {
  margin: 8px 0 24px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.totp-sub b {
  color: var(--el-text-color-primary);
}
.totp :deep(.otp) {
  width: 100%;
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
.back {
  margin-top: 10px;
  gap: 4px;
}
.totp-lost {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.5;
  text-align: center;
  color: var(--el-text-color-placeholder);
}

@keyframes rise {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
@keyframes drift {
  to {
    transform: translate(-40px, 50px) scale(1.1);
  }
}
@keyframes flow {
  to {
    stroke-dashoffset: -20;
  }
}
@keyframes pulse {
  from {
    transform: scale(1);
    opacity: 0.9;
  }
  to {
    transform: scale(4);
    opacity: 0;
  }
}

/* 中等宽度收窄品牌区 */
@media (max-width: 1199.98px) {
  .login {
    --hero-w: 42%;
  }
  .hero-inner {
    padding: 36px 40px;
  }
}

/* 窄屏隐藏品牌区，表单居中并铺主题光晕背景 */
@media (max-width: 899.98px) {
  .hero {
    display: none;
  }
  .form-side {
    align-items: flex-start;
    padding: 72px 20px 32px;
    background:
      radial-gradient(600px 320px at 50% -80px, color-mix(in srgb, var(--el-color-primary) 18%, transparent), transparent 70%),
      var(--el-bg-color);
  }
  .toolbar {
    top: 14px;
    right: 14px;
  }
  .mobile-brand {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    margin-bottom: 28px;
  }
  .mb-name {
    margin-top: 12px;
    font-size: 22px;
    font-weight: 750;
  }
  .mb-sub {
    margin-top: 4px;
    font-size: 13px;
    color: var(--el-text-color-secondary);
  }
  .form-head {
    text-align: center;
  }
  h2 {
    font-size: 22px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .form-wrap,
  .hero-glow,
  .hero-net .links line,
  .hero-net .pulse {
    animation: none;
  }
}
</style>

<style>
/* 弹窗挂在 body 下，需非 scoped 样式 */
.totp-modal {
  backdrop-filter: blur(6px);
}
.totp-dialog {
  max-width: calc(100vw - 32px);
  border-radius: 18px;
}
.totp-dialog .el-dialog__header {
  padding-bottom: 0;
}
</style>
