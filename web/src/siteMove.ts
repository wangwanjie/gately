// Web 服务站点移到其他监听的纯函数（拖放目标解析与明文传输风险判断），供 Web.vue 与 WebBatch.vue 使用。
// 本文件不依赖 Vue，可直接用 node --test 测试（web/tests/siteMove.test.mjs）。
// 站点 id 为 “监听key/站点key”，分组标题 id 为 “监听key/g:分组名”，默认站点行 id 为 “监听key/__default”。

export interface SiteRef {
  listener: string
  site: string
}

// MoveTarget 为移动站点的目标：before / after 为目标监听中的站点 key（插到它前后并加入它的分组），
// group 为放入的分组（空字符串为未分组），都不填时保留原分组放在末尾
export interface MoveTarget {
  listener: string
  group?: string
  before?: string
  after?: string
}

// MoveListener 是可作为移动目标的监听：groups 为其中已有的分组名称，authSites 为开启了认证的站点 key
export interface MoveListener {
  key: string
  name: string
  tls: boolean
  port: number
  groups: string[]
  authSites: string[]
}

// CrossDrop 为一次跨监听拖放：from 为站点原来所在的监听，target 为放置位置
export interface CrossDrop {
  from: string
  target: MoveTarget
}

// sourceListener 返回拖动的站点所在的监听 key；站点来自多个监听或找不到时返回空字符串
function sourceListener(ids: string[], listenerKeys: string[]) {
  let from = ''
  for (const id of ids) {
    const k = listenerKeys.find((x) => id.startsWith(`${x}/`))
    if (!k || (from && k !== from)) {
      return ''
    }
    from = k
  }
  return from
}

// crossListenerDrop 解析拖放：把 ids 中的站点放到监听 to 内 over 指向的位置。
// 站点都来自同一个其他监听、放置位置在 to 内且不是默认站点时返回移动目标，否则返回 null（同监听内的排序另行处理）。
export function crossListenerDrop(ids: string[], over: { id: string; after: boolean }, to: string, listenerKeys: string[]): CrossDrop | null {
  const prefix = `${to}/`
  if (!ids.length || !over.id.startsWith(prefix)) {
    return null
  }
  const from = sourceListener(ids, listenerKeys)
  if (!from || from === to) {
    return null
  }
  const rest = over.id.slice(prefix.length)
  if (!rest || rest === '__default') {
    return null
  }
  if (rest.startsWith('g:')) {
    return { from, target: { listener: to, group: rest.slice(2) } }
  }
  return { from, target: over.after ? { listener: to, after: rest } : { listener: to, before: rest } }
}

// movePlainRisk 判断是否要把开启了认证的站点从 HTTPS 监听移到明文 HTTP 监听：移动后密码与登录 Cookie 将以明文传输。
// 已在目标监听中的站点不移动，不计入
export function movePlainRisk(refs: SiteRef[], target: string, listeners: MoveListener[]) {
  const to = listeners.find((l) => l.key === target)
  if (!to || to.tls) {
    return false
  }
  return refs.some((ref) => {
    const from = listeners.find((l) => l.key === ref.listener)
    return !!from && from.key !== target && from.tls && from.authSites.includes(ref.site)
  })
}
