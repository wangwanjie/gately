// 拓扑视图纯函数的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ROW, PAD_Y, activeAt, buildLayout, edgeWidth, flowLevel, formatCount, formatQps, ipPath, isValidIP, maskIP, maskText, maskURL, nodeCovers, pathOf, siteMatches, terminalId } from '../src/topology.ts'

const rate = (qps = 0, bps = 0) => ({ qps, bps, qps60: qps })
const up = (id, url, healthy = true) => ({ id, url, weight: 1, healthy, conns: 0, requests: 0, bytes: 0, rate: rate(1, 10) })
const site = (id, extra = {}) => ({
  id,
  key: id.split('/')[1],
  name: '',
  type: 'proxy',
  domains: [],
  enable: true,
  running: true,
  guards: [],
  requests: 0,
  active: 0,
  errors: 0,
  closed: 0,
  rate: rate(2, 20),
  status: {},
  blocked: {},
  upstreams: [],
  ...extra,
})
const view = () => ({
  time: 100,
  window: 10,
  enabled: true,
  requests: 0,
  active: 0,
  rate: rate(5, 50),
  ipTotal: 0,
  ipTruncated: false,
  ips: [],
  listeners: [
    {
      id: 'l:a',
      key: 'a',
      name: 'web',
      enable: true,
      running: true,
      network: 'tcp',
      listenIp: '',
      port: 443,
      tls: true,
      http3: false,
      plainHttp: 'redirect',
      proxyProtocol: false,
      requests: 0,
      active: 0,
      rate: rate(4, 40),
      sites: [
        site('s:a/app', { name: 'App', domains: ['app.example.com'], upstreams: [up('u:a/app/0', 'http://10.0.0.1:80'), up('u:a/app/1', 'http://10.0.0.2:80', false)] }),
        site('s:a/default', { type: 'close', default: true }),
      ],
      actions: [{ id: 'a:a/redirect', kind: 'redirect', requests: 3, rate: rate(1, 0) }],
    },
    {
      id: 'l:b',
      key: 'b',
      name: 'lan',
      enable: false,
      running: false,
      network: 'tcp',
      listenIp: '',
      port: 80,
      tls: false,
      http3: false,
      plainHttp: '',
      proxyProtocol: false,
      requests: 0,
      active: 0,
      rate: rate(),
      sites: [site('s:b/nas', { domains: ['nas.lan'], running: false, upstreams: [up('u:b/nas/0', 'http://192.168.1.20:5000')] })],
      actions: [],
    },
  ],
})

test('buildLayout 每个叶子占一行，父节点位于子节点中点', () => {
  const l = buildLayout(view(), new Set(), '')
  const ids = l.nodes.map((n) => n.id)
  assert.deepEqual(
    ids.sort(),
    ['a:a/redirect', 'gately', 'internet', 'l:a', 'l:b', 's:a/app', 's:a/default', 's:b/nas', 't:a/default', 'u:a/app/0', 'u:a/app/1', 'u:b/nas/0'].sort(),
  )
  // 叶子：u0、u1、t:default、action、u:b → 5 行
  assert.equal(l.height, PAD_Y * 2 + 5 * ROW)
  const c = (id) => l.byId.get(id).y + l.byId.get(id).h / 2
  assert.equal(c('s:a/app'), (c('u:a/app/0') + c('u:a/app/1')) / 2)
  assert.equal(c('l:a'), (c('s:a/app') + c('a:a/redirect')) / 2)
  assert.equal(l.byId.get('t:a/default').parent, 's:a/default')
  assert.equal(terminalId('s:a/default'), 't:a/default')
  // 每个非根节点一条连线，速率取下游节点
  assert.equal(l.edges.length, l.nodes.length - 1)
  const e = l.edges.find((x) => x.to === 'u:a/app/0')
  assert.equal(e.from, 's:a/app')
  assert.equal(e.rate.qps, 1)
  assert.equal(l.edges.find((x) => x.to === 'gately').rate.qps, 5)
})

test('buildLayout 折叠监听只占一行并记录隐藏数量，搜索隐藏不匹配的站点与监听', () => {
  const l = buildLayout(view(), new Set(['a']), '')
  assert.equal(l.byId.get('l:a').collapsed, true)
  assert.equal(l.byId.get('l:a').hidden, 3)
  assert.equal(l.byId.has('s:a/app'), false)
  assert.equal(l.height, PAD_Y * 2 + 2 * ROW)

  const q = buildLayout(view(), new Set(), 'app')
  assert.equal(q.byId.has('l:b'), false)
  assert.equal(q.byId.has('s:a/app'), true)
  // 搜索时监听级动作与不匹配的站点不显示，计入隐藏数量
  assert.equal(q.byId.has('a:a/redirect'), false)
  assert.equal(q.byId.has('s:a/default'), false)
  assert.equal(q.byId.get('l:a').hidden, 2)
  // 搜索后端地址
  assert.equal(buildLayout(view(), new Set(), '10.0.0.2').byId.has('s:a/app'), true)
  // 监听名称匹配时显示全部站点
  const ln = buildLayout(view(), new Set(), 'lan')
  assert.equal(ln.byId.has('s:b/nas'), true)
  // 没有任何匹配时只剩互联网与 Gately
  assert.deepEqual(buildLayout(view(), new Set(), 'zzz').nodes.map((n) => n.id).sort(), ['gately', 'internet'])
})

test('pathOf 返回整条链路，互联网与 Gately 不变暗', () => {
  const l = buildLayout(view(), new Set(), '')
  assert.equal(pathOf(l, 'gately'), null)
  assert.equal(pathOf(l, 'internet'), null)
  assert.equal(pathOf(l, 'nope'), null)
  assert.deepEqual([...pathOf(l, 's:a/app')].sort(), ['gately', 'internet', 'l:a', 's:a/app', 'u:a/app/0', 'u:a/app/1'].sort())
  assert.deepEqual([...pathOf(l, 'u:a/app/1')].sort(), ['gately', 'internet', 'l:a', 's:a/app', 'u:a/app/1'].sort())
  assert.ok(pathOf(l, 'l:a').has('t:a/default'))
})

test('ipPath 有具体后端时只高亮该后端，否则高亮站点全部后端', () => {
  const l = buildLayout(view(), new Set(), '')
  const exact = ipPath(l, { ip: '1.2.3.4', nodes: ['s:a/app', 'u:a/app/1'], active: 1, last: 1 })
  assert.ok(exact.has('u:a/app/1'))
  assert.ok(!exact.has('u:a/app/0'))
  assert.ok(exact.has('internet') && exact.has('l:a'))
  const all = ipPath(l, { ip: '1.2.3.4', nodes: ['s:a/app'], active: 0, last: 1 })
  assert.ok(all.has('u:a/app/0') && all.has('u:a/app/1'))
  const act = ipPath(l, { ip: '1.2.3.4', nodes: ['a:a/redirect', 's:x/gone'], active: 0, last: 1 })
  assert.deepEqual([...act].sort(), ['a:a/redirect', 'gately', 'internet', 'l:a'])
  assert.equal(ipPath(l, undefined).size, 0)
})

test('nodeCovers 按节点层级判断地址是否经过', () => {
  const ip = { ip: '1.2.3.4', nodes: ['s:a/default', 'a:a/redirect'], active: 0, last: 1 }
  assert.ok(nodeCovers(ip, 'gately'))
  assert.ok(nodeCovers(ip, 'l:a'))
  assert.ok(!nodeCovers(ip, 'l:b'))
  assert.ok(nodeCovers(ip, 't:a/default'))
  assert.ok(!nodeCovers(ip, 's:a/app'))
  // 监听 key 是另一个 key 的前缀时不能误判
  assert.ok(!nodeCovers({ ip: 'x', nodes: ['s:ab/x'], active: 0, last: 0 }, 'l:a'))
})

test('maskIP 与 maskURL 打码地址', () => {
  assert.equal(maskIP('203.0.113.45'), '203.0.113.*')
  assert.equal(maskIP('2001:db8:1:2::5'), '2001:db8:1:2:*')
  assert.equal(maskIP('2001:db8:1:2:3:4:5:6'), '2001:db8:1:2:*')
  assert.equal(maskIP('::1'), '0:0:0:0:*')
  assert.equal(maskURL('http://192.168.1.10:5000/x'), 'http://192.168.1.*:5000/x')
  assert.equal(maskURL('https://[fd00:1:2::9]:8443'), 'https://[fd00:1:2:0:*]:8443')
  assert.equal(maskURL('http://nas.lan:5000'), 'http://nas.lan:5000')
  assert.equal(maskURL('not a url'), 'not a url')
})

test('siteMatches 匹配名称、域名、分组与后端', () => {
  const s = site('s:a/x', { name: 'Photos', group: 'Home', domains: ['img.example.com'], upstreams: [up('u', 'http://10.1.1.1')] })
  for (const q of ['', 'photo', 'HOME', 'img.', '10.1.1']) {
    assert.ok(siteMatches(s, q), q)
  }
  assert.ok(!siteMatches(s, 'video'))
})

test('格式化与连线样式', () => {
  assert.equal(formatQps(0), '0')
  assert.equal(formatQps(0.05), '<0.1')
  assert.equal(formatQps(3.25), '3.3')
  assert.equal(formatQps(250.4), '250')
  assert.equal(formatQps(12345), '12k')
  assert.equal(formatCount(9999), '9999')
  assert.equal(formatCount(128814), '129k')
  assert.equal(formatCount(2500000), '2.5M')
  assert.equal(flowLevel(rate()), 0)
  assert.equal(flowLevel({ qps: 0, bps: 10, qps60: 0 }), 1)
  assert.equal(flowLevel(rate(5)), 2)
  assert.equal(flowLevel(rate(50)), 3)
  assert.ok(edgeWidth(rate(1000)) > edgeWidth(rate(1)))
  assert.ok(edgeWidth(rate(1e9)) <= 5.4)
})

test('activeAt 按节点层级统计进行中请求', () => {
  const ip = { ip: 'x', nodes: [], active: 3, activeAt: { 's:a/app': 2, 'u:a/app/0': 2, 's:b/nas': 1 }, last: 0 }
  assert.equal(activeAt(ip, 'gately'), 3)
  assert.equal(activeAt(ip, 'l:a'), 2)
  assert.equal(activeAt(ip, 'l:b'), 1)
  assert.equal(activeAt(ip, 's:a/app'), 2)
  assert.equal(activeAt(ip, 'u:a/app/0'), 2)
  assert.equal(activeAt(ip, 'u:a/app/1'), 0)
  assert.equal(activeAt(ip, 't:b/nas'), 1)
  assert.equal(activeAt({ ip: 'y', nodes: [], active: 0, last: 0 }, 's:a/app'), 0)
})

test('isValidIP 只接受完整的地址', () => {
  for (const ok of ['1.2.3.4', '255.255.255.255', '::1', '2001:db8::1', '::ffff:1.2.3.4']) {
    assert.equal(isValidIP(ok), true, ok)
  }
  for (const bad of ['', '1.2.3', '1.2.3.', '256.1.1.1', 'cafe', '2001:db8::zz', ':::', '1.2.3.4/24', 'example.com']) {
    assert.equal(isValidIP(bad), false, bad)
  }
})

test('maskText 打码错误信息里的地址，通配地址与时间不变', () => {
  assert.equal(maskText('listen tcp 192.168.1.5:443: bind: address already in use'), 'listen tcp 192.168.1.*:443: bind: address already in use')
  assert.equal(maskText('listen tcp [2001:db8:1:2::5]:443: bind'), 'listen tcp [2001:db8:1:2:*]:443: bind')
  assert.equal(maskText('dial 10.0.0.1 and 2001:db8:1:2::9 failed at 12:30:45'), 'dial 10.0.0.* and 2001:db8:1:2:* failed at 12:30:45')
  assert.equal(maskText('listen tcp 0.0.0.0:80: bind'), 'listen tcp 0.0.0.0:80: bind')
  assert.equal(maskIP('::'), '::')
})
