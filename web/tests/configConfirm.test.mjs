// 配置变更确认与访问路径提醒的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
import { test, beforeEach } from 'node:test'
import assert from 'node:assert/strict'
import { registerHooks } from 'node:module'

// configConfirm.ts 依赖的 ./api 会连带加载路由与全部页面，accessPath.ts 依赖的 ./i18n 会加载语言包，
// 测试时换成最小替身：api 转发到 globalThis.__api，t 返回键名与参数，便于断言
const stubs = {
  './api': 'export const api = { get: (...a) => globalThis.__api.get(...a), post: (...a) => globalThis.__api.post(...a) }',
  './i18n': 'export const t = (k, p) => (p ? k + JSON.stringify(p) : k)',
}
registerHooks({
  resolve(specifier, context, next) {
    if (context.parentURL?.includes('/src/') && stubs[specifier]) {
      return { url: 'data:text/javascript,' + encodeURIComponent(stubs[specifier]), shortCircuit: true }
    }
    if (specifier.startsWith('.') && !/\.\w+$/.test(specifier) && context.parentURL?.includes('/src/')) {
      return next(specifier + '.ts', context)
    }
    return next(specifier, context)
  },
})

// 内存版 sessionStorage
function fakeStorage() {
  const m = new Map()
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, String(v)),
    removeItem: (k) => m.delete(k),
    map: m,
  }
}
globalThis.sessionStorage = fakeStorage()

const cc = await import('../src/configConfirm.ts')
const ap = await import('../src/accessPath.ts')
const { confirmState } = cc

// 记录 api 调用并按路径返回预设结果
function stubApi(replies = {}) {
  const calls = []
  const reply = (method, url, body) => {
    calls.push([method, url, body])
    const r = replies[`${method} ${url}`]
    if (r instanceof Error) {
      return Promise.reject(r)
    }
    return Promise.resolve(typeof r === 'function' ? r(body) : r)
  }
  globalThis.__api = {
    get: (url) => reply('GET', url),
    post: (url, body) => reply('POST', url, body),
  }
  return calls
}

beforeEach(() => {
  confirmState.enabled = true
  confirmState.timeout = 90
  confirmState.hold = null
  confirmState.configs = []
  confirmState.phase = 'idle'
  globalThis.sessionStorage.map.clear()
})

test('受保护的写请求：三个模块的写请求、保存全局设置与重启', () => {
  assert.equal(cc.isProtectedWrite('PUT', '/api/portforward/rules/a'), true)
  assert.equal(cc.isProtectedWrite('DELETE', '/api/protoroute/rules/a?x=1'), true)
  assert.equal(cc.isProtectedWrite('POST', '/api/webservice/sites/batch'), true)
  assert.equal(cc.isProtectedWrite('PUT', '/api/system/settings'), true)
  assert.equal(cc.isProtectedWrite('POST', '/api/system/restart'), true)
  // 读请求、其他模块、其他系统接口不受保护
  assert.equal(cc.isProtectedWrite('GET', '/api/portforward/rules'), false)
  assert.equal(cc.isProtectedWrite('HEAD', '/api/webservice/listeners'), false)
  assert.equal(cc.isProtectedWrite('PUT', '/api/ddns/tasks/a'), false)
  assert.equal(cc.isProtectedWrite('POST', '/api/system/settings'), false)
  assert.equal(cc.isProtectedWrite('PUT', '/api/portforwardx/rules/a'), false)
})

test('确认请求头：确认开启时带上；关闭时只在批量修改期间带上', () => {
  assert.equal(cc.confirmHeaderFor('PUT', '/api/portforward/rules/a'), true)
  confirmState.enabled = false
  assert.equal(cc.confirmHeaderFor('PUT', '/api/portforward/rules/a'), false)
  confirmState.hold = { items: [], configs: [] }
  assert.equal(cc.confirmHeaderFor('PUT', '/api/portforward/rules/a'), true)
  assert.equal(cc.confirmHeaderFor('PUT', '/api/ddns/tasks/a'), false)
})

test('经确认重启：记下等待确认，重启后的页面据此自动确认', () => {
  cc.afterConfirmWrite('POST', '/api/system/restart')
  assert.equal(sessionStorage.getItem('gately-confirm-restart'), '1')
  assert.equal(sessionStorage.getItem('gately-confirm-watch'), '1')
  assert.equal(confirmState.phase, 'idle')
})

test('批量修改期间保存不确认，只刷新横幅上暂存的项目', async () => {
  const hold = { items: [{ module: 'portforward', key: 'a' }], configs: ['portforward'] }
  const calls = stubApi({ 'GET /api/system/confirm': { enabled: true, timeout: 60, pending: null, rolledBack: null, hold } })
  confirmState.hold = { items: [], configs: [] }
  cc.afterConfirmWrite('PUT', '/api/portforward/rules/a')
  await new Promise((r) => setTimeout(r, 0))
  assert.deepEqual(calls.map((c) => c.slice(0, 2)), [['GET', '/api/system/confirm']])
  assert.deepEqual(confirmState.hold, hold)
  assert.equal(confirmState.timeout, 60)
  assert.equal(confirmState.phase, 'idle')
})

test('批量修改：开始、检查、无需确认时全部生效、放弃且无改动时不重启', async () => {
  const hold = { items: [{ module: 'webservice', key: 'l1', name: 'web' }], configs: [] }
  stubApi({
    'POST /api/system/changeset/begin': hold,
    'POST /api/system/changeset/check': {},
    'POST /api/system/changeset/apply': { applied: 1, confirm: false, timeout: 90 },
    'POST /api/system/changeset/discard': { restart: false },
  })
  await cc.beginHold()
  assert.deepEqual(confirmState.hold, hold)
  // 服务端没有返回 problems 时按没有问题处理
  assert.deepEqual(await cc.checkHold(), [])
  assert.equal(await cc.applyHold(), 1)
  assert.equal(confirmState.hold, null)
  assert.equal(confirmState.phase, 'idle')

  await cc.beginHold()
  await cc.discardHold()
  assert.equal(confirmState.hold, null)
  assert.equal(confirmState.phase, 'idle')
})

test('批量修改全部生效时服务端报错（非网络中断）：保留批量修改状态并抛出', async () => {
  const hold = { items: [{ module: 'portforward', key: 'a' }], configs: [] }
  stubApi({ 'POST /api/system/changeset/apply': new Error('bad request') })
  confirmState.hold = hold
  await assert.rejects(cc.applyHold(), /bad request/)
  assert.deepEqual(confirmState.hold, hold)
})

test('访问路径：每一跳的说明', () => {
  assert.equal(ap.hopLabel({ kind: 'client', addr: '1.2.3.4' }), 'accessPath.hop.client{"addr":"1.2.3.4"}')
  assert.equal(ap.hopLabel({ kind: 'gateway' }), 'accessPath.hop.gateway')
  assert.equal(ap.hopLabel({ kind: 'portforward', rule: 'r1' }), 'accessPath.hop.portforward{"name":"r1"}')
  assert.equal(ap.hopLabel({ kind: 'portforward', rule: 'r1', name: '转发', missing: true }), 'accessPath.hop.portforward{"name":"转发"}accessPath.missing')
  assert.equal(
    ap.hopLabel({ kind: 'protoroute', name: 'p', route: 'tls' }),
    'accessPath.hop.protoroute{"name":"p"}accessPath.hop.route{"route":"tls"}',
  )
  // 默认站点没有名称
  assert.equal(
    ap.hopLabel({ kind: 'webservice', name: 'w' }),
    'accessPath.hop.webservice{"name":"w"}accessPath.hop.site{"site":"accessPath.defaultSite"}',
  )
  assert.equal(ap.hopLabel({ kind: 'admin', listen: ':16680', https: true }), 'accessPath.hop.adminHttps{"listen":":16680"}')
  assert.equal(ap.hopLabel({ kind: 'admin', listen: ':16680' }), 'accessPath.hop.admin{"listen":":16680"}')
  // 未知类型原样显示
  assert.equal(ap.hopLabel({ kind: 'future' }), 'future')
})

test('访问路径：风险说明引用受影响的一跳，序号越界时不引用', () => {
  const path = { hops: [{ kind: 'client', addr: 'a' }, { kind: 'portforward', name: 'r' }] }
  const hop = 'accessPath.hop.portforward{"name":"r"}'
  assert.equal(ap.riskText({ kind: 'edit', hop: 1 }, path), `accessPath.risk.edit${JSON.stringify({ hop })}`)
  assert.equal(ap.riskText({ kind: 'delete', hop: 5 }, path), `accessPath.risk.delete${JSON.stringify({ hop: '' })}`)
  assert.equal(
    ap.riskText({ kind: 'moduleOff', hop: 1, module: 'portforward' }, path),
    `accessPath.risk.moduleOff${JSON.stringify({ module: 'nav.items.portforward.title', hop })}`,
  )
  assert.equal(
    ap.riskText({ kind: 'listenUnavailable', hop: 1, addr: '127.0.0.2:80', error: 'can\'t assign' }, path),
    `accessPath.risk.listenUnavailable${JSON.stringify({ hop, addr: '127.0.0.2:80', error: 'can\'t assign' })}`,
  )
  assert.equal(ap.riskText({ kind: 'lanOnly', hop: -1 }, path), 'accessPath.risk.lanOnly')
  assert.equal(ap.riskText({ kind: 'future', hop: 0 }, path), 'future')
})

test('访问路径提醒：同时只有一个，新提醒把未答复的前一个按取消处理', async () => {
  const check = { path: { hops: [] }, risks: [] }
  const first = ap.askRisk(check)
  assert.equal(ap.riskPrompt.visible, true)
  const second = ap.askRisk(check)
  assert.equal(await first, 'cancel')
  ap.answerRisk('hold')
  assert.equal(await second, 'hold')
  assert.equal(ap.riskPrompt.visible, false)
  // 没有等待中的提醒时答复不出错
  ap.answerRisk('save')
})

test('访问路径检查：请求体为空时发 null，检查失败或连不上时不拦保存', async () => {
  const origFetch = globalThis.fetch
  try {
    let sent
    globalThis.fetch = async (url, init) => {
      sent = { url, init }
      return { ok: true, json: async () => ({ path: { hops: [] }, risks: [{ kind: 'delete', hop: 0 }] }) }
    }
    const r = await ap.checkAccess('DELETE', '/api/portforward/rules/a', undefined)
    assert.equal(sent.url, '/api/system/access-path/check')
    assert.deepEqual(JSON.parse(sent.init.body), { method: 'DELETE', path: '/api/portforward/rules/a', body: null })
    assert.equal(r.risks.length, 1)

    globalThis.fetch = async () => ({ ok: false })
    assert.equal(await ap.checkAccess('PUT', '/api/system/settings', {}), null)
    globalThis.fetch = async () => {
      throw new TypeError('Failed to fetch')
    }
    assert.equal(await ap.checkAccess('PUT', '/api/system/settings', {}), null)
  } finally {
    globalThis.fetch = origFetch
  }
})
