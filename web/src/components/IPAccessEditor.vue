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
  <!-- 优先拒绝名单：先于白名单、黑名单判断，与模式无关；调用方不绑定 deny-list 时不显示 -->
  <template v-if="denyList !== undefined && denySets !== undefined">
    <el-form-item :label="t('ipAccessEditor.ipDeny')" data-test="ip-deny">
      <el-input v-model="denyText" type="textarea" :rows="2" class="mono" :placeholder="t('ipAccessEditor.rangePlaceholder')" />
      <div class="form-help">{{ t('ipAccessEditor.ipDenyHelp') }}</div>
    </el-form-item>
    <el-form-item :label="t('ipAccessEditor.ipDenySets')" data-test="ip-deny-sets">
      <el-select v-model="denySets" multiple filterable :placeholder="t('ipAccessEditor.setsPlaceholder')" :no-data-text="t('ipAccessEditor.setsEmpty')" class="w-full">
        <el-option v-for="o in ipLists" :key="o.key" :label="o.name" :value="o.key">
          <span>{{ o.name }}</span>
          <span v-if="!o.loaded" class="opt-warn">{{ t('ipAccessEditor.setNotLoaded') }}</span>
        </el-option>
      </el-select>
    </el-form-item>
  </template>
  <!-- 需要敲门：来源地址还必须持有所选端口敲门规则的有效授权；敲门模块未编译时不显示（已设置的引用仍显示以便清除） -->
  <el-form-item v-if="knock !== undefined && (moduleBuilt('knock') || knock)" :label="t('ipAccessEditor.knock')">
    <KnockSelect v-model="knock" />
    <div class="form-help">{{ t('ipAccessEditor.knockHelp') }}</div>
  </el-form-item>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { useListText, splitLines } from '../composables/useListText'
import { moduleBuilt } from '../nav'
import KnockSelect from './KnockSelect.vue'

// note 为附加在说明后的模块提示，例如 STUN 修改名单不会重新打洞
defineProps<{ note?: string }>()

// mode 为 ''（关闭）、whitelist、blacklist；list 为手填 IP / 网段；sets 为引用的订阅 IP 列表 key。
// 后端空名单返回 null，调用方打开编辑器时应统一为数组
const mode = defineModel<string>('mode', { required: true })
const list = defineModel<string[]>('list', { required: true })
const sets = defineModel<string[]>('sets', { required: true })
// knock 为端口敲门规则 key，留空表示不要求敲门；调用方不绑定时不显示该项
const knock = defineModel<string | undefined>('knock')
// 优先拒绝名单（手填 IP / 网段与订阅列表 key），调用方不绑定时不显示；后端空名单返回 null，调用方打开编辑器时应统一为数组
const denyList = defineModel<string[] | undefined>('denyList')
const denySets = defineModel<string[] | undefined>('denySets')

const { t } = useI18n()
// 输入框保留原始文本，回车可正常换行（见 useListText）
const ipText = useListText(() => list.value, (v) => (list.value = v), { split: splitLines })
const denyText = useListText(() => denyList.value ?? [], (v) => (denyList.value = v), { split: splitLines })

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
