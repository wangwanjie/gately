// 导航面板的内置图标列表：首次使用时读取一次，各组件共用。
import { ref } from 'vue'
import { api } from '../api'
import type { BuiltinIcon } from '../panels'

const icons = ref<BuiltinIcon[]>([])
const byId = ref(new Map<string, BuiltinIcon>())
let loading: Promise<void> | null = null

export function usePanelIcons() {
  if (!loading) {
    loading = api
      .get<BuiltinIcon[]>('/api/webservice/panels/icons')
      .then((list) => {
        icons.value = list
        byId.value = new Map(list.map((i) => [i.id, i]))
      })
      .catch(() => {
        // 读取失败时允许下次重试，图标按文字显示
        loading = null
      })
  }
  return { icons, byId }
}
