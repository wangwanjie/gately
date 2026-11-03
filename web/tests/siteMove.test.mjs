// 站点换监听纯函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { crossListenerDrop, movePlainRisk } from '../src/siteMove.ts'

const keys = ['pub', 'lan', 'pub2']

test('crossListenerDrop 放到其他监听的站点前后', () => {
  assert.deepEqual(crossListenerDrop(['pub/a', 'pub/b'], { id: 'lan/x', after: false }, 'lan', keys), { from: 'pub', target: { listener: 'lan', before: 'x' } })
  assert.deepEqual(crossListenerDrop(['pub/a'], { id: 'lan/x', after: true }, 'lan', keys), { from: 'pub', target: { listener: 'lan', after: 'x' } })
})

test('crossListenerDrop 放到分组标题与空监听放置区', () => {
  assert.deepEqual(crossListenerDrop(['pub/a'], { id: 'lan/g:应用', after: false }, 'lan', keys), { from: 'pub', target: { listener: 'lan', group: '应用' } })
  // 空监听的放置区 id 为 “监听key/g:”，表示未分组
  assert.deepEqual(crossListenerDrop(['pub/a'], { id: 'lan/g:', after: false }, 'lan', keys), { from: 'pub', target: { listener: 'lan', group: '' } })
  // 分组名中含斜杠与 g: 时原样保留
  assert.deepEqual(crossListenerDrop(['pub/a'], { id: 'lan/g:a/g:b', after: false }, 'lan', keys).target, { listener: 'lan', group: 'a/g:b' })
})

test('crossListenerDrop 同监听、混合来源、默认站点与无效位置返回 null', () => {
  assert.equal(crossListenerDrop(['lan/a'], { id: 'lan/x', after: false }, 'lan', keys), null)
  assert.equal(crossListenerDrop(['pub/a', 'lan/b'], { id: 'lan/x', after: false }, 'lan', keys), null)
  assert.equal(crossListenerDrop(['pub/a'], { id: 'lan/__default', after: false }, 'lan', keys), null)
  assert.equal(crossListenerDrop(['pub/a'], { id: 'pub2/x', after: false }, 'lan', keys), null)
  assert.equal(crossListenerDrop(['nope/a'], { id: 'lan/x', after: false }, 'lan', keys), null)
  assert.equal(crossListenerDrop([], { id: 'lan/x', after: false }, 'lan', keys), null)
  assert.equal(crossListenerDrop(['pub/a'], { id: 'lan/', after: false }, 'lan', keys), null)
})

test('crossListenerDrop 监听 key 互为前缀时不混淆', () => {
  // pub2 以 pub 开头，必须按 “key/” 匹配
  assert.deepEqual(crossListenerDrop(['pub2/a'], { id: 'lan/x', after: false }, 'lan', keys).from, 'pub2')
  assert.equal(crossListenerDrop(['pub2/a'], { id: 'pub2/x', after: false }, 'pub2', keys), null)
})

const ls = [
  { key: 'https', name: '', tls: true, port: 443, groups: [], authSites: ['auth'] },
  { key: 'http', name: '', tls: false, port: 80, groups: [], authSites: ['auth2'] },
  { key: 'https2', name: '', tls: true, port: 8443, groups: [], authSites: [] },
]

test('movePlainRisk 只在带认证的站点从 HTTPS 移到 HTTP 时提醒', () => {
  assert.equal(movePlainRisk([{ listener: 'https', site: 'auth' }], 'http', ls), true)
  assert.equal(movePlainRisk([{ listener: 'https', site: 'open' }], 'http', ls), false)
  assert.equal(movePlainRisk([{ listener: 'https', site: 'auth' }], 'https2', ls), false)
  // 已是明文 HTTP 的站点移到另一个 HTTP 监听不算新增风险
  assert.equal(movePlainRisk([{ listener: 'http', site: 'auth2' }], 'http', ls), false)
  assert.equal(movePlainRisk([{ listener: 'https', site: 'open' }, { listener: 'https', site: 'auth' }], 'http', ls), true)
  assert.equal(movePlainRisk([{ listener: 'https', site: 'auth' }], 'missing', ls), false)
})

test('crossListenerDrop 分组标题忽略上下半区，站点 key 原样作为插入锚点', () => {
  // 分组标题只记录目标，after 不影响结果
  assert.deepEqual(crossListenerDrop(['pub/a'], { id: 'lan/g:应用', after: true }, 'lan', keys), { from: 'pub', target: { listener: 'lan', group: '应用' } })
  // 随机十六进制 key 与带下划线的自动编号 key
  assert.deepEqual(crossListenerDrop(['pub/a'], { id: 'lan/s1_', after: true }, 'lan', keys).target, { listener: 'lan', after: 's1_' })
  assert.deepEqual(crossListenerDrop(['pub/3f9a0c1b2d4e'], { id: 'lan/x', after: false }, 'lan', keys).from, 'pub')
})

test('crossListenerDrop 放置位置属于其他监听或目标 key 是被拖站点所在监听的前缀时返回 null', () => {
  // 放到 pub2 的站点上，但处理放置的是 pub（over 不以 “pub/” 开头）
  assert.equal(crossListenerDrop(['lan/a'], { id: 'pub2/x', after: false }, 'pub', keys), null)
})

test('movePlainRisk 来源监听不存在或目标为 HTTPS 时不提醒', () => {
  assert.equal(movePlainRisk([{ listener: 'gone', site: 'auth' }], 'http', ls), false)
  assert.equal(movePlainRisk([], 'http', ls), false)
  // 目标中已有同 key 的认证站点不影响判断，只看被移动站点在来源监听中的认证状态
  assert.equal(movePlainRisk([{ listener: 'https2', site: 'auth2' }], 'http', ls), false)
})
