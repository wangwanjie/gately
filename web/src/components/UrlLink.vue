<template>
  <span v-if="safeHref" class="url-link">
    <a ref="anchor" :href="safeHref" target="_blank" rel="noopener noreferrer" class="url-link-a">{{ text ?? href }}</a>
    <button type="button" class="url-link-copy" :title="t('components.urlLink.copy')" :aria-label="t('components.urlLink.copy')" @click="copy">
      <Copy :size="12" />
    </button>
  </span>
  <span v-else>{{ text ?? href }}</span>
</template>

<script setup lang="ts">
// UrlLink 显示可在新标签页打开的地址，旁边带复制按钮。
// 只渲染 http(s) 地址为链接，其他协议（如 javascript:）退回纯文本；
// rel 带 noreferrer，打开的站点拿不到管理界面的地址。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy } from 'lucide-vue-next'
import { copyText } from '../composables/useClipboard'
import { safeHttpUrl } from '../siteUrl'

const props = defineProps<{
  href: string
  // text 为显示的文字，默认显示 href
  text?: string
  // copyValue 为复制的内容，默认复制 href
  copyValue?: string
}>()

const { t } = useI18n()
const anchor = ref<HTMLElement | null>(null)
const safeHref = computed(() => safeHttpUrl(props.href))

function copy() {
  copyText(props.copyValue ?? props.href, { el: anchor.value, success: t('components.urlLink.copied') })
}
</script>

<style scoped>
.url-link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  max-width: 100%;
  vertical-align: bottom;
}
.url-link-a {
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--el-color-primary);
  text-decoration: none;
}
.url-link-a:hover,
.url-link-a:focus-visible {
  text-decoration: underline;
}
.url-link-copy {
  display: inline-flex;
  flex: none;
  align-items: center;
  padding: 2px;
  border: 0;
  border-radius: 3px;
  background: none;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
}
.url-link-copy:hover {
  color: var(--el-color-primary);
  background: var(--el-fill-color);
}
/* 桌面端悬停或键盘聚焦时显示复制按钮，触屏设备没有悬停，始终显示 */
.url-link:hover .url-link-copy,
.url-link:focus-within .url-link-copy {
  opacity: 1;
}
@media (hover: none) {
  .url-link-copy {
    opacity: 1;
  }
}
</style>
