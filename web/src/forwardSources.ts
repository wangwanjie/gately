// 读取协议分流与端口转发规则，用来推算只对本机开放、经四层转发进入的 Web 监听的对外端口（见 siteUrl.ts 的 forwardedEntry）。
// 模块未开启或读取失败（如令牌无权限）时跳过，链接按监听端口生成。
import { api } from './api'
import { moduleEnabled } from './nav'
import type { ForwardRule, ForwardSource } from './siteUrl'

export async function loadForwardSources(): Promise<ForwardSource[]> {
  const kinds: ForwardSource['kind'][] = ['protoroute', 'portforward']
  const out = await Promise.all(
    kinds.map(async (kind) => {
      if (!moduleEnabled(kind)) {
        return null
      }
      try {
        return { kind, rules: await api.get<ForwardRule[]>(`/api/${kind}/rules`) }
      } catch {
        return null
      }
    }),
  )
  return out.filter((x): x is ForwardSource => x !== null)
}
