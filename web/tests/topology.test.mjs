// 拓扑视图纯函数的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ROW, PAD_Y, Aliases, activeAt, actionActive, countActive, seenNodes, siteActive, upstreamActive, aliasDomain, aliasDomainText, aliasIP, aliasText, aliasURL, filterActive, filterView, buildLayout, resolveId, isColumnar, maskDomain, maskDomainText, edgeWidth, flowLevel, formatCount, formatQps, ipPath, isValidIP, maskIP, maskText, maskURL, nodeCovers, pathOf, siteMatches, terminalId } from '../src/topology.ts'

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
// idleUp、idleSite 生成最近 60 秒没有请求的后端与站点
const idleUp = (id, url) => ({ ...up(id, url), rate: rate() })
const idleSite = (id, upId) => site(id, { rate: rate(), upstreams: [idleUp(upId, 'http://10.0.9.' + id.length)] })
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

test('buildLayout 子节点超过上限时收进汇总节点，展开后在末尾提供收起入口', () => {
  const v = view()
  const l0 = v.listeners[0]
  // 前 8 个站点活动中，其余空闲
  const many = Array.from({ length: 12 }, (_, i) => site(`s:a/x${i}`, i < 8 ? { active: 1, upstreams: [up(`u:a/x${i}/0`, 'http://10.0.1.' + i)] } : idleSite(`s:a/x${i}`, `u:a/x${i}/0`)))
  l0.sites = many
  const l = buildLayout(v, new Set(), '', { limit: 8 })
  const m = l.byId.get('m:l:a')
  assert.equal(m.kind, 'more')
  assert.equal(m.more.count, 4)
  assert.equal(m.more.expanded, false)
  assert.equal(m.more.childKind, 'site')
  assert.equal(m.more.active, 0)
  assert.equal(m.more.rate.qps, 0)
  assert.equal(l.byId.has('s:a/x8'), false)
  // 监听级动作不收起，排在汇总节点之后
  assert.deepEqual(l.children.get('l:a').slice(-2), ['m:l:a', 'a:a/redirect'])
  // 被收起的站点与其后端都映射到汇总节点，IP 链路高亮汇总节点
  assert.equal(resolveId(l, 'u:a/x9/0'), 'm:l:a')
  const p = ipPath(l, { ip: '1.2.3.4', nodes: ['s:a/x9', 'u:a/x9/0'], active: 1, last: 1 })
  assert.deepEqual([...p].sort(), ['gately', 'internet', 'l:a', 'm:l:a'])
  const ip = { ip: 'x', nodes: ['s:a/x10'], active: 1, activeAt: { 's:a/x10': 1 }, last: 0 }
  assert.ok(nodeCovers(ip, 'm:l:a', m.more.ids))
  assert.equal(activeAt(ip, 'm:l:a', m.more.ids), 1)

  // 展开后显示全部，汇总节点作为收起入口留在末尾
  const e = buildLayout(v, new Set(), '', { limit: 8, expanded: new Set(['l:a']) })
  assert.equal(e.byId.has('s:a/x11'), true)
  assert.equal(e.byId.get('m:l:a').more.expanded, true)
  assert.equal(e.alias.size, 0)
  // 只多出一个时直接显示
  l0.sites = many.slice(0, 9)
  assert.equal(buildLayout(v, new Set(), '', { limit: 8 }).byId.has('m:l:a'), false)
})

test('buildLayout 站点后端过多时收起，嵌套收起沿映射找到显示的节点', () => {
  const v = view()
  const s = v.listeners[0].sites[0]
  s.upstreams = Array.from({ length: 10 }, (_, i) => idleUp(`u:a/app/${i}`, 'http://10.0.0.' + i))
  const l = buildLayout(v, new Set(), '', { upstreamLimit: 6 })
  assert.equal(l.byId.get('m:s:a/app').more.childKind, 'upstream')
  assert.equal(l.byId.get('m:s:a/app').more.count, 4)
  assert.equal(resolveId(l, 'u:a/app/9'), 'm:s:a/app')
  // 有具体后端但被收起时高亮汇总节点
  const p = ipPath(l, { ip: '1.2.3.4', nodes: ['s:a/app', 'u:a/app/9'], active: 1, last: 1 })
  assert.ok(p.has('m:s:a/app') && !p.has('u:a/app/0'))
  assert.equal(resolveId(l, 'u:none'), '')
})

test('buildLayout 径向布局以 Gately 为圆心，各层节点在同心圆上', () => {
  const l = buildLayout(view(), new Set(), '', { mode: 'radial' })
  assert.equal(l.mode, 'radial')
  assert.equal(l.width, l.height)
  const c = l.width / 2
  const g = l.byId.get('gately')
  assert.equal(g.x + g.w / 2, c)
  const dist = (id) => {
    const n = l.byId.get(id)
    return Math.round(Math.hypot(n.x + n.w / 2 - c, n.y + n.h / 2 - c))
  }
  assert.equal(dist('l:a'), dist('l:b'))
  assert.equal(dist('s:a/app'), dist('a:a/redirect'))
  assert.equal(dist('u:a/app/0'), dist('t:a/default'))
  assert.ok(dist('l:a') < dist('s:a/app') && dist('s:a/app') < dist('u:a/app/0'))
  // 叶子文字朝外，父节点位于子节点角度的中间
  assert.equal(l.byId.get('u:a/app/0').leaf, true)
  assert.equal(l.byId.get('s:a/app').leaf, false)
  const a = (id) => l.byId.get(id).angle
  assert.ok(Math.abs(a('s:a/app') - (a('u:a/app/0') + a('u:a/app/1')) / 2) < 1e-9)
  assert.equal(l.edges.length, l.nodes.length - 1)
  // 叶子很多时放大圆环，外圈节点不至于重叠
  const v = view()
  v.listeners[0].sites = Array.from({ length: 300 }, (_, i) => site(`s:a/y${i}`, { upstreams: [up(`u:a/y${i}/0`, 'http://h' + i)] }))
  const big = buildLayout(v, new Set(), '', { mode: 'radial', limit: 1000 })
  assert.ok(big.width > l.width)
})

test('buildLayout 桑基图按行排版，连线带按子节点 QPS 切分父节点竖条', () => {
  const l = buildLayout(view(), new Set(), '', { mode: 'sankey' })
  assert.equal(l.mode, 'sankey')
  assert.ok(isColumnar('sankey') && !isColumnar('sunburst'))
  const bands = l.edges.filter((e) => e.from === 'l:a')
  assert.ok(bands.length > 0 && bands.every((e) => e.band && e.flow))
  // 同一父节点切出的各段首尾相接，合计等于父节点竖条高度
  const parent = l.byId.get('s:a/app')
  const seg = l.edges.filter((e) => e.from === 's:a/app').map((e) => e.d.match(/^M[\d.]+,([\d.]+)/)[1] * 1)
  assert.equal(seg[0], parent.y)
  assert.ok(seg[1] > seg[0] && seg[1] < parent.y + parent.h)
  // 竖条至少 6px，流量大的节点更高
  const h = (id) => l.byId.get(id).h
  assert.ok(l.nodes.every((n) => n.h >= 6))
  assert.ok(h('gately') >= h('l:a'))
  assert.equal(l.edges.length, l.nodes.length - 1)
})

test('buildLayout 旭日图：父扇区覆盖子扇区角度，没有连线与互联网节点', () => {
  const l = buildLayout(view(), new Set(), '', { mode: 'sunburst' })
  assert.equal(l.edges.length, 0)
  assert.equal(l.byId.has('internet'), false)
  assert.ok(l.byId.get('gately'))
  const s = l.byId.get('s:a/app')
  assert.ok(s.shape.startsWith('M') && s.label)
  const a = (id) => l.byId.get(id).angle
  assert.ok(Math.abs(a('s:a/app') - (a('u:a/app/0') + a('u:a/app/1')) / 2) < 1e-9)
  // 高亮链路仍按父子关系
  assert.ok(pathOf(l, 'u:a/app/1').has('l:a'))
})

test('maskDomain 与 maskDomainText 打码域名', () => {
  assert.equal(maskDomain('nas.home.example.com'), 'nas.***.com')
  assert.equal(maskDomain('example.com'), 'e***.com')
  assert.equal(maskDomain('*.apps.example.test'), '*.apps.***.test')
  assert.equal(maskDomain('localhost'), 'localhost')
  assert.equal(maskDomain('10.0.0.1'), '10.0.0.1')
  assert.equal(maskDomainText('kg_nexterm.stun.vanjay.cn 主站 v1.2'), 'kg_nexterm.***.cn 主站 v1.2')
  // 首段通配：域名列表与站点名称里的写法都要打码，不能因为首段含 * 漏掉
  assert.equal(maskDomain('api-*.example.com'), 'api-*.***.com')
  assert.equal(maskDomain('app-*-dev.home.example.com'), 'app-*-dev.***.com')
  assert.equal(maskDomainText('api-*.example.com'), 'api-*.***.com')
  assert.equal(maskDomainText('主站 app-*-dev.home.example.com, *.example.org'), '主站 app-*-dev.***.com, *.e***.org')
  assert.equal(maskDomainText('*api.example.com'), '*api.***.com')
  assert.ok(!maskDomainText('站点 api-*.secret-corp.com').includes('secret-corp'))
  assert.equal(maskURL('http://nas.lan:5000/x', true), 'http://n***.lan:5000/x')
  assert.equal(maskURL('http://nas.lan:5000/x'), 'http://nas.lan:5000/x')
})

test('buildLayout 竖向树：各层在固定高度，父节点位于子节点水平中点', () => {
  const l = buildLayout(view(), new Set(), '', { mode: 'vertical' })
  const n = (id) => l.byId.get(id)
  assert.equal(n('l:a').y, n('l:b').y)
  assert.ok(n('l:a').y < n('s:a/app').y && n('s:a/app').y < n('u:a/app/0').y)
  const cx = (id) => n(id).x + n(id).w / 2
  assert.equal(cx('s:a/app'), (cx('u:a/app/0') + cx('u:a/app/1')) / 2)
  assert.equal(cx('internet'), cx('gately'))
  assert.equal(l.edges.length, l.nodes.length - 1)
})

test('buildLayout 矩形树图与气泡图：Gately 为最外层容器，子节点在父节点范围内，没有连线', () => {
  for (const mode of ['treemap', 'pack']) {
    const l = buildLayout(view(), new Set(), '', { mode })
    assert.equal(l.edges.length, 0, mode)
    assert.equal(l.byId.has('internet'), false, mode)
    // 先父后子：父节点在数组中排在子节点之前，绘制时子节点在上层
    const idx = (id) => l.nodes.findIndex((x) => x.id === id)
    assert.ok(idx('gately') < idx('l:a') && idx('l:a') < idx('s:a/app') && idx('s:a/app') < idx('u:a/app/0'), mode)
    const inside = (c, p) => {
      const a = l.byId.get(c)
      const b = l.byId.get(p)
      return a.x >= b.x - 0.01 && a.y >= b.y - 0.01 && a.x + a.w <= b.x + b.w + 0.01 && a.y + a.h <= b.y + b.h + 0.01
    }
    assert.ok(inside('u:a/app/0', 's:a/app') && inside('s:a/app', 'l:a') && inside('l:a', 'gately'), mode)
    assert.equal(l.byId.get('u:a/app/0').leaf, true)
    // 叶子面积相同
    if (mode === 'pack') {
      assert.ok(Math.abs(l.byId.get('u:a/app/0').w - l.byId.get('u:b/nas/0').w) < 0.01)
    }
  }
})

test('buildLayout 矩形树图父节点顶部留出名称的高度', () => {
  const l = buildLayout(view(), new Set(), '', { mode: 'treemap' })
  assert.ok(l.byId.get('l:a').y - l.byId.get('gately').y >= 20)
  assert.ok(l.byId.get('u:a/app/0').y - l.byId.get('s:a/app').y >= 18)
})

test('filterView 按监听、异常与拦截原因筛选', () => {
  const v = view()
  assert.equal(filterView(v, {}), v)
  assert.equal(filterActive({ listeners: [], blocked: [] }), false)
  // 只按监听筛选：保留整个监听
  const onlyA = filterView(v, { listeners: ['a'] })
  assert.deepEqual(onlyA.listeners.map((l) => l.key), ['a'])
  assert.equal(onlyA.listeners[0].actions.length, 1)
  // 只看异常：App 有不健康的后端，默认站点正常；监听级动作隐藏
  const issues = filterView(v, { issues: true })
  assert.deepEqual(issues.listeners[0].sites.map((s) => s.key), ['app'])
  assert.equal(issues.listeners[0].actions.length, 0)
  // 停用的监听不算启动失败
  assert.ok(!issues.listeners.some((l) => l.key === 'b'))
  // 启动失败的监听保留全部站点
  v.listeners[1].enable = true
  v.listeners[1].error = 'listen tcp :80: bind: address already in use'
  const failed = filterView(v, { issues: true })
  assert.equal(failed.listeners.find((l) => l.key === 'b').sites.length, v.listeners[1].sites.length)
  // 按拦截原因：只保留出现过所选原因拦截的站点，无剩余站点的监听隐藏
  v.listeners[0].sites[1].blocked = { waf: 2 }
  const waf = filterView(v, { blocked: ['waf'] })
  assert.deepEqual(waf.listeners.map((l) => l.sites.map((s) => s.key)), [['default']])
  assert.equal(filterView(v, { blocked: ['ban'] }).listeners.length, 0)
  // 原数据不被修改
  assert.equal(v.listeners[0].sites.length, 2)
})

test('代号打码：同值同号、不同值不同号、不保留片段', () => {
  const a = new Aliases()
  assert.equal(aliasIP(a, '203.0.113.45', 'Visitor'), 'Visitor-01')
  assert.equal(aliasIP(a, '203.0.113.46', 'Visitor'), 'Visitor-02')
  assert.equal(aliasIP(a, '203.0.113.45', 'Visitor'), 'Visitor-01')
  assert.equal(aliasIP(a, '2001:DB8::1', 'Visitor'), 'Visitor-03')
  assert.equal(aliasIP(a, '2001:db8::1', 'Visitor'), 'Visitor-03')
  assert.equal(aliasIP(a, '0.0.0.0', 'Listen'), '0.0.0.0')
  // 类别之间独立编号
  assert.equal(aliasIP(a, '203.0.113.45', 'Backend'), 'Backend-01')
  assert.equal(aliasURL(a, 'http://10.0.0.1:8080/api', 'Backend'), 'http://Backend-02:8080/api')
  assert.equal(aliasURL(a, 'https://[fd00::9]:8443', 'Backend'), 'https://[Backend-03]:8443')
  // 域名：同一可注册域名下的子域名共享 domain 编号，主机部分各自编号，只保留公共后缀
  assert.equal(aliasDomain(a, 'example.com'), 'domain-01.com')
  assert.equal(aliasDomain(a, 'nas.home.example.com'), 'host-01.domain-01.com')
  assert.equal(aliasDomain(a, 'app.example.com'), 'host-02.domain-01.com')
  assert.equal(aliasDomain(a, '*.example.com'), '*.domain-01.com')
  assert.equal(aliasDomain(a, 'shop.example.com.cn'), 'host-03.domain-02.com.cn')
  assert.equal(aliasDomain(a, 'nas'), 'host-04')
  assert.equal(aliasDomain(a, 'NAS'), 'host-04')
  assert.equal(aliasDomain(a, '*'), '*')
  // 首段通配不保留任何片段（包括首段里的字面量），可注册域名照常共享 domain 编号
  assert.equal(aliasDomain(a, 'api-*.example.com'), 'host-05.domain-01.com')
  assert.equal(aliasDomainText(a, '站点 api-*.example.com'), '站点 host-05.domain-01.com')
  assert.ok(!aliasDomainText(a, 'app-*-dev.secret-corp.com').includes('secret-corp'))
  assert.ok(!aliasDomainText(a, 'app-*-dev.secret-corp.com').includes('app-'))
  assert.equal(aliasURL(a, 'http://app.example.com:5000', 'Backend'), 'http://host-02.domain-01.com:5000')
  assert.equal(aliasDomainText(a, '主站 app.example.com'), '主站 host-02.domain-01.com')
  assert.equal(aliasText(a, 'listen tcp 192.168.1.5:443: bind: address already in use', 'Listen'), 'listen tcp Listen-01:443: bind: address already in use')
  assert.equal(aliasText(a, 'listen tcp [fd00::5]:443: bind', 'Listen'), 'listen tcp [Listen-02]:443: bind')
  // 新的映射表重新编号
  assert.equal(aliasIP(new Aliases(), '203.0.113.46', 'Visitor'), 'Visitor-01')
})

test('siteActive 等按 60 秒均值、进行中请求与活动 IP 判断活动', () => {
  const idle = site('s:a/q', { rate: rate() })
  assert.equal(siteActive(idle), false)
  // 只有 60 秒均值（10 秒均值已归零）仍算活动，避免偶发请求的站点反复出现、消失
  assert.equal(siteActive({ ...idle, rate: { qps: 0, bps: 0, qps60: 0.02 } }), true)
  assert.equal(siteActive({ ...idle, active: 1 }), true)
  // 长连接只有下行流量时同样算活动
  assert.equal(siteActive({ ...idle, rate: { qps: 0, bps: 512, qps60: 0 } }), true)
  assert.equal(siteActive(idle, new Set(['s:a/q'])), true)
  const u = idleUp('u:a/q/0', 'http://10.0.0.1')
  assert.equal(upstreamActive(u), false)
  assert.equal(upstreamActive({ ...u, conns: 2 }), true)
  assert.equal(upstreamActive(u, new Set(['u:a/q/0'])), true)
  const a = { id: 'a:a/notFound', kind: 'notFound', requests: 9, rate: rate() }
  assert.equal(actionActive(a), false)
  assert.equal(actionActive(a, new Set(['a:a/notFound'])), true)
  const v = view()
  v.ips = [{ ip: '1.1.1.1', nodes: ['s:a/app', 'u:a/app/1'], active: 1, last: 99 }]
  assert.deepEqual([...seenNodes(v)].sort(), ['s:a/app', 'u:a/app/1'])
})

test('filterView 只看活动：保留活动中的站点、动作与后端，隐藏没有活动的监听', () => {
  const v = view()
  const [la, lb] = v.listeners
  la.sites.push(idleSite('s:a/idle', 'u:a/idle/0'))
  // app 只有一个后端活动中；默认站点（断开连接）空闲；跳转动作空闲
  la.sites[0].upstreams[1] = idleUp('u:a/app/1', 'http://10.0.0.2:80')
  la.sites[1] = { ...la.sites[1], rate: rate() }
  la.actions = [{ id: 'a:a/redirect', kind: 'redirect', requests: 3, rate: rate() }, { id: 'a:a/notFound', kind: 'notFound', requests: 1, rate: rate(0.5) }]
  lb.sites = [idleSite('s:b/nas', 'u:b/nas/0')]
  // 低频请求的站点均值为 0，但活动 IP 经过它
  la.sites.push(site('s:a/rare', { rate: rate(), upstreams: [idleUp('u:a/rare/0', 'http://10.0.0.9')] }))
  v.ips = [{ ip: '1.1.1.1', nodes: ['s:a/rare'], active: 0, last: 90 }]

  const f = filterView(v, { active: true })
  assert.ok(filterActive({ active: true }))
  assert.deepEqual(f.listeners.map((l) => l.key), ['a'])
  const a = f.listeners[0]
  assert.deepEqual(a.sites.map((s) => s.id), ['s:a/app', 's:a/rare'])
  assert.deepEqual(a.sites[0].upstreams.map((u) => u.id), ['u:a/app/0'])
  // 站点活动但后端都没有记录到请求（如被访问控制拦截）时，站点作为末端显示
  assert.deepEqual(a.sites[1].upstreams, [])
  assert.deepEqual(a.actions.map((x) => x.id), ['a:a/notFound'])
  // 不修改原数据
  assert.equal(v.listeners[0].sites[0].upstreams.length, 2)
  assert.equal(v.listeners.length, 2)

  const l = buildLayout(f, new Set(), '')
  assert.ok(l.byId.has('u:a/app/0') && !l.byId.has('u:a/app/1') && !l.byId.has('s:a/idle'))
  assert.equal(l.byId.get('s:a/rare').leaf, true)

  // 与监听筛选同时生效
  assert.equal(filterView(v, { active: true, listeners: ['b'] }).listeners.length, 0)
  // 与只看异常同时生效：异常但空闲的站点不显示
  la.sites[0].upstreams[0] = { ...la.sites[0].upstreams[0], healthy: false }
  assert.deepEqual(filterView(v, { active: true, issues: true }).listeners[0].sites.map((s) => s.id), ['s:a/app'])
  la.sites[0] = { ...la.sites[0], rate: rate(), upstreams: [idleUp('u:a/app/0', 'http://10.0.0.1:80')] }
  assert.equal(filterView(v, { active: true, issues: true }).listeners.length, 0)
})

test('buildLayout 收起多余节点时活动中的站点与后端始终显示', () => {
  const v = view()
  const l0 = v.listeners[0]
  // 12 个站点只有 x3 与 x10 活动中，上限 2：x0、x1 按顺序显示，x3、x10 超出上限仍显示
  l0.sites = Array.from({ length: 12 }, (_, i) => (i === 3 || i === 10 ? site(`s:a/x${i}`, { upstreams: [up(`u:a/x${i}/0`, 'http://10.0.1.' + i)] }) : idleSite(`s:a/x${i}`, `u:a/x${i}/0`)))
  const l = buildLayout(v, new Set(), '', { limit: 2 })
  assert.deepEqual(l.children.get('l:a'), ['s:a/x0', 's:a/x1', 's:a/x3', 's:a/x10', 'm:l:a', 'a:a/redirect'])
  const m = l.byId.get('m:l:a').more
  assert.equal(m.count, 8)
  assert.ok(!m.ids.includes('s:a/x3') && !m.ids.includes('s:a/x10'))
  assert.equal(resolveId(l, 's:a/x10'), 's:a/x10')
  assert.equal(resolveId(l, 'u:a/x11/0'), 'm:l:a')
  // 活动 IP 经过的站点也算活动
  v.ips = [{ ip: '1.1.1.1', nodes: ['s:a/x7'], active: 0, last: 90 }]
  assert.ok(buildLayout(v, new Set(), '', { limit: 2 }).byId.has('s:a/x7'))
  // 超出上限的只剩一个空闲节点时直接显示，不出现“还有 1 个”
  l0.sites = l0.sites.slice(0, 4)
  v.ips = []
  const few = buildLayout(v, new Set(), '', { limit: 2 })
  assert.equal(few.byId.has('m:l:a'), false)
  assert.equal(few.byId.has('s:a/x2'), true)
  // 后端同理
  const s = l0.sites[3]
  s.upstreams = Array.from({ length: 6 }, (_, i) => (i === 5 ? up(`u:a/x3/${i}`, 'http://10.0.2.' + i) : idleUp(`u:a/x3/${i}`, 'http://10.0.2.' + i)))
  const ul = buildLayout(v, new Set(), '', { upstreamLimit: 1 })
  assert.deepEqual(ul.children.get('s:a/x3'), ['u:a/x3/0', 'u:a/x3/5', 'm:s:a/x3'])
  assert.equal(ul.byId.get('m:s:a/x3').more.count, 4)
})

test('countActive 统计活动中的站点并按搜索词过滤', () => {
  const v = view()
  v.listeners[0].sites.push(idleSite('s:a/idle', 'u:a/idle/0'))
  // app、默认站点、nas（未运行但速率非零）计入
  assert.equal(countActive(v), 3)
  assert.equal(countActive(v, 'app.example'), 1)
  assert.equal(countActive(v, 'nomatch'), 0)
  for (const l of v.listeners) {
    l.sites = l.sites.map((s) => ({ ...s, rate: rate() }))
  }
  assert.equal(countActive(v), 0)
  v.ips = [{ ip: '1.1.1.1', nodes: ['s:b/nas'], active: 1, last: 99 }]
  assert.equal(countActive(v), 1)
})

test('buildLayout 非反向代理站点先列去向节点，再列按用户分流的后端（#79）', () => {
  const v = view()
  v.listeners[0].sites.splice(1, 0, site('s:a/st', { type: 'static', target: '/data/www', upstreams: [up('u:a/st/0', 'http://10.0.0.9:80')] }))
  const l = buildLayout(v, new Set(), '')
  const kids = l.nodes.filter((n) => n.parent === 's:a/st').map((n) => n.id)
  assert.deepEqual(kids, ['t:a/st', 'u:a/st/0'])
  assert.equal(l.edges.find((x) => x.to === 'u:a/st/0').from, 's:a/st')
  // 只看活动时去掉空闲的分流后端，去向节点保留
  v.listeners[0].sites[1].upstreams = [idleUp('u:a/st/0', 'http://10.0.0.9:80')]
  const act = filterView(v, { active: true })
  const st = act.listeners[0].sites.find((s) => s.id === 's:a/st')
  assert.deepEqual(st.upstreams, [])
  const kids2 = buildLayout(act, new Set(), '').nodes.filter((n) => n.parent === 's:a/st').map((n) => n.id)
  assert.deepEqual(kids2, ['t:a/st'])
})
