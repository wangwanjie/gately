<template>
  <!-- 四层入口规则的来源 IP 访问控制：模式、手填 IP / 网段、引用 Web 服务中订阅的 IP 列表 -->
  <el-form-item :label="t('ipAccessEditor.title')">
    <el-radio-group v-model="mode">
      <el-radio-button value="">{{ t('ipAccessEditor.off') }}</el-radio-button>
      <el-radio-button value="whitelist">{{ t('ipAccessEditor.whitelist') }}</el-radio-button>
      <el-radio-button value="blacklist">{{ t('ipAccessEditor.blacklist') }}</el-radio-button>
    </el-radio-group>
    <div class="form-help">{{ t('ipAccessEditor.help') }}</div>
    <div v-if="note" class="form-help">{{ note }}</div>
  </el-form-item>
  <template v-if="mode">
    <el-form-item :label="t('ipAccessEditor.range')">
      <el-input v-model="ipText" type="textarea" :rows="3" class="mono" :placeholder="t('ipAccessEditor.rangePlaceholder')" />
    </el-form-item>
    <el-form-item :label="t('ipAccessEditor.sets')">
      <el-select v-model="sets" multiple filterable :placeholder="t('ipAccessEditor.setsPlaceholder')" :no-data-text="t('ipAccessEditor.setsEmpty')" class="w-full">
        <el-option v-for="o in ipLists" :key="o.key" :label="o.name" :value="o.key">
          <span>{{ o.name }}</span>
          <span v-if="!o.loaded" class="opt-warn">{{ t('ipAccessEditor.setNotLoaded') }}</span>
        </el-option>
      </el-select>
      <div class="form-help">{{ mode === 'whitelist' ? t('ipAccessEditor.setsWhitelistHelp') : t('ipAccessEditor.setsHelp') }}</div>
    </el-form-item>
  </template>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { useListText, splitLines } from '../composables/useListText'

// note 为附加在说明后的模块提示，例如 STUN 修改名单不会重新打洞
defineProps<{ note?: string }>()

// mode 为 ''（关闭）、whitelist、blacklist；list 为手填 IP / 网段；sets 为引用的订阅 IP 列表 key。
// 后端空名单返回 null，调用方打开编辑器时应统一为数组
const mode = defineModel<string>('mode', { required: true })
const list = defineModel<string[]>('list', { required: true })
const sets = defineModel<string[]>('sets', { required: true })

const { t } = useI18n()
// 输入框保留原始文本，回车可正常换行（见 useListText）
const ipText = useListText(() => list.value, (v) => (list.value = v), { split: splitLines })

// ipLists 为 Web 服务中订阅的 IP 列表
const ipLists = ref<{ key: string; name: string; loaded: boolean }[]>([])

// 读取订阅 IP 列表；读取失败（如令牌没有 Web 服务权限）时只是不显示可选项
onMounted(async () => {
  try {
    ipLists.value = await api.get('/api/webservice/iplists')
  } catch {
    ipLists.value = []
  }
})
</script>

<style scoped>
.w-full {
  width: 100%;
}
.opt-warn {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-color-warning);
}
</style>
