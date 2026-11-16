<template>
  <!-- 定制构建只含一种语言时没有可切换的语言，不显示 -->
  <el-dropdown v-if="locales.length > 1" trigger="click" popper-class="locale-popper" @command="onCommand" @visible-change="lockScroll">
    <button class="picker-btn" :aria-label="t('common.language')" :title="t('common.language')">
      <Languages :size="18" />
    </button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item v-for="l in locales" :key="l.code" :command="l.code" :class="{ active: l.code === locale }">
          <Check v-if="l.code === locale" :size="14" class="check" />
          <span v-else class="check" />
          {{ l.name }}
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup lang="ts">
import { Check, Languages } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { setLocale } from '../i18n'
import { locales } from '../i18n/locales'
import { useScrollLock } from '../composables/useScrollLock'

// remember 为 true 时（管理员已登录）把新语言告诉后端：服务端语言为「跟随系统」且环境变量未指定语言时，日志与通知随之切换
const props = defineProps<{ remember?: boolean }>()

const { t, locale } = useI18n()

// 菜单打开期间锁定页面滚动，滚轮和触摸滑动不穿透到下方页面
const lockScroll = useScrollLock('.locale-popper')

async function onCommand(code: string) {
  if (code === locale.value) {
    return
  }
  await setLocale(code)
  if (props.remember) {
    // 只影响服务端输出语言，失败时不打扰用户，下次登录还会再记录
    api.put('/api/account/ui-language', { language: locale.value }).catch(() => undefined)
  }
}
</script>

<style scoped>
.picker-btn {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: 1px solid var(--g-border);
  background: var(--el-bg-color);
  color: var(--el-text-color-regular);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    color 0.15s,
    border-color 0.15s;
}
.picker-btn:hover {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary);
}
.check {
  display: inline-block;
  width: 14px;
  margin-right: 6px;
}
.active {
  color: var(--el-color-primary);
}
/* 手机上点击目标至少 40px */
@media (max-width: 767.98px) {
  .picker-btn {
    width: 40px;
    height: 40px;
  }
}
</style>
