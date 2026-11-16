// 读取 Gately 的配置目录（/api/status 的 configDir），整个页面只请求一次；供目录输入框提示与配置目录的关系。
import { ref, type Ref } from 'vue'
import { api, type Status } from '../api'

export interface ConfigDirInfo {
  dir: string
  // fold 为 true 时服务器的文件名不区分大小写（macOS、Windows）
  fold: boolean
}

const info = ref<ConfigDirInfo | null>(null)
let pending: Promise<void> | null = null

export function useConfigDir(): Ref<ConfigDirInfo | null> {
  if (!pending) {
    pending = api
      .get<Status>('/api/status')
      .then((s) => {
        info.value = { dir: s.configDir, fold: s.os === 'darwin' || s.os === 'windows' }
      })
      .catch(() => {
        // 取不到时不提示，下次使用时重试；保存时服务端仍会校验
        pending = null
      })
  }
  return info
}
