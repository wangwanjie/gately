// 导航面板纯函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  emptyLink, groupLinks, moveLink, moveLinks, moveGroup, insertLink, importableSites, linksFromSites, iconKind, safeIconSrc,
  initial, paletteColor, textIconOk, hasVars, toPanel, mergeLinks, freeName, MAX_LINKS,
  emptyPanel, layoutDefaults, normalizeLayoutValue, LAYOUT_LIMITS,
  sizeChoice, sizeValue, faviconUrls, SIZE_CHOICES, renameLinkGroup,
} from '../src/panels.ts'

const L = (name, group = '') => ({ ...emptyLink(group), name, url: `https://${name}.example.com` })
const names = (links) => links.map((l) => l.name)

test('groupLinks 按分组首次出现的顺序', () => {
  const links = [L('a', 'x'), L('b'), L('c', 'x'), L('d', ' y ')]
  assert.deepEqual(groupLinks(links), [{ name: 'x', items: [0, 2] }, { name: '', items: [1] }, { name: 'y', items: [3] }])
  assert.deepEqual(groupLinks([]), [])
})

test('moveLink 只在分组内移动，并让同组链接相邻', () => {
  const links = [L('a', 'x'), L('b'), L('c', 'x')]
  assert.deepEqual(names(moveLink(links, 2, -1)), ['c', 'a', 'b'])
  assert.deepEqual(names(moveLink(links, 0, 1)), ['c', 'a', 'b'])
  // 越界不变（返回原数组）
  assert.equal(moveLink(links, 0, -1), links)
  assert.equal(moveLink(links, 1, 1), links)
  assert.equal(moveLink(links, 9, 1), links)
})

test('moveGroup 整组移动', () => {
  const links = [L('a', 'x'), L('b'), L('c', 'x'), L('d', 'y')]
  assert.deepEqual(names(moveGroup(links, 'y', -1)), ['a', 'c', 'd', 'b'])
  assert.deepEqual(names(moveGroup(links, 'x', 1)), ['b', 'a', 'c', 'd'])
  assert.equal(moveGroup(links, 'x', -1), links)
  assert.equal(moveGroup(links, 'none', 1), links)
})

test('面板分组：新面板为未分组，toPanel 保留分组、旧数据缺省为空', () => {
  assert.equal(emptyPanel().group, '')
  assert.equal(toPanel({ ...emptyPanel(), name: 'a', group: '家庭' }).group, '家庭')
  const old = { ...emptyPanel(), name: 'b' }
  delete old.group
  assert.equal(toPanel(old).group, '')
})

test('moveLinks 在分组内拖动到任意位置', () => {
  const links = [L('a', 'x'), L('b', 'x'), L('c', 'x'), L('d', 'x'), L('e', 'x')]
  // 最后一个拖到最前
  assert.deepEqual(names(moveLinks(links, [4], { group: 'x', before: 0 })), ['e', 'a', 'b', 'c', 'd'])
  // 第一个拖到第三个之后
  assert.deepEqual(names(moveLinks([...links], [0], { group: 'x', after: 2 })), ['b', 'c', 'a', 'd', 'e'])
  // 放在分组标题上：移到该组末尾
  assert.deepEqual(names(moveLinks([...links], [1], { group: 'x' })), ['a', 'c', 'd', 'e', 'b'])
  // 位置没有变化返回 null
  assert.equal(moveLinks([...links], [4], { group: 'x' }), null)
  assert.equal(moveLinks([...links], [0], { group: 'x', before: 1 }), null)
})

test('moveLinks 多选一起移动，保持原有显示顺序', () => {
  const links = [L('a', 'x'), L('b', 'x'), L('c', 'x'), L('d', 'x'), L('e', 'x'), L('f', 'x')]
  // 选中的顺序与传入顺序无关，按显示顺序排列
  assert.deepEqual(names(moveLinks(links, [5, 3, 4], { group: 'x', before: 0 })), ['d', 'e', 'f', 'a', 'b', 'c'])
  // 不相邻的多项放到中间
  assert.deepEqual(names(moveLinks([...links], [0, 5], { group: 'x', after: 2 })), ['b', 'c', 'a', 'f', 'd', 'e'])
  // 放到被移动的链接自身旁边没有意义
  assert.equal(moveLinks([...links], [0, 1], { group: 'x', before: 1 }), null)
  assert.equal(moveLinks([...links], [0, 1], { group: 'x', after: 0 }), null)
})

test('moveLinks 跨分组移动并改写分组，分组顺序不变', () => {
  const mk = () => [L('a1', 'A'), L('a2', 'A'), L('a3', 'A'), L('u'), L('b1', 'B'), L('b2', 'B')]
  // A 组的两项拖到 B 组 b1 之后
  let links = mk()
  let r = moveLinks(links, [0, 2], { group: 'B', after: 4 })
  assert.deepEqual(names(r), ['a2', 'u', 'b1', 'a1', 'a3', 'b2'])
  assert.deepEqual(r.map((l) => l.group), ['A', '', 'B', 'B', 'B', 'B'])
  // 保持对象身份，编辑器靠它记住行 id 与选中状态
  assert.equal(r[3], links[0])
  // 跨多个分组选中后放到分组标题上：移到该组末尾
  links = mk()
  r = moveLinks(links, [1, 3, 5], { group: 'A' })
  assert.deepEqual(names(r), ['a1', 'a3', 'a2', 'u', 'b2', 'b1'])
  assert.deepEqual(groupLinks(r).map((g) => g.name), ['A', 'B'])
  // 整组移走后该分组消失，未分组放在原来的位置
  links = mk()
  r = moveLinks(links, [4, 5], { group: '', before: 3 })
  assert.deepEqual(names(r), ['a1', 'a2', 'a3', 'b1', 'b2', 'u'])
  assert.deepEqual(groupLinks(r).map((g) => g.name), ['A', ''])
  // 移到新分组：放在最后，分组名去掉首尾空白
  links = mk()
  r = moveLinks(links, [0], { group: '  New ' })
  assert.deepEqual(names(r), ['a2', 'a3', 'u', 'b1', 'b2', 'a1'])
  assert.equal(r[5].group, 'New')
  // 分组名首尾空白与已有分组视为同一组
  links = [L('a', ' x '), L('b', 'y')]
  r = moveLinks(links, [1], { group: 'x', before: 0 })
  assert.deepEqual(names(r), ['b', 'a'])
  assert.deepEqual(groupLinks(r).map((g) => g.name), ['x'])
})

test('moveLinks 忽略无效下标与不一致的位置', () => {
  const links = [L('a', 'A'), L('b', 'B')]
  assert.equal(moveLinks(links, [], { group: 'A' }), null)
  assert.equal(moveLinks(links, [-1, 9, 1.5], { group: 'A' }), null)
  // 目标链接不在目标分组中
  assert.equal(moveLinks(links, [0], { group: 'A', before: 1 }), null)
  // 无效的项被忽略，其余照常移动
  assert.deepEqual(names(moveLinks(links, [1, 7], { group: 'A', before: 0 })), ['b', 'a'])
})

test('insertLink 放到同组最后，没有同组时放到最后', () => {
  const links = [L('a', 'x'), L('b'), L('c', 'x'), L('d', 'y')]
  let r = insertLink(links, L('n', 'x'))
  assert.equal(r.index, 3)
  assert.deepEqual(names(r.links), ['a', 'b', 'c', 'n', 'd'])
  r = insertLink(links, L('n', 'z'))
  assert.equal(r.index, 4)
  assert.equal(links.length, 4)
})

test('importableSites 取第一个能拼出地址的域名，跳过断开连接与导航面板站点', () => {
  const listeners = [
    {
      key: 'l1', name: 'HTTPS', port: 443, enable: true, sites: [
        { key: 's1', name: 'NAS', group: '存储', enable: true, type: 'proxy', domains: ['*.example.com', 'nas.example.com'] },
        { key: 's2', name: '', enable: true, type: 'redirect', domains: ['go.example.com'] },
        { key: 's3', name: '拒绝', enable: true, type: 'close', domains: ['x.example.com'] },
        { key: 's4', name: '导航', enable: true, type: 'panel', domains: ['nav.example.com'] },
        { key: 's5', name: '通配', enable: true, type: 'proxy', domains: ['*.wild.example.com'] },
      ],
    },
    { key: 'l2', name: '', port: 8080, enable: true, sites: [{ key: 's1', name: 'App', enable: false, type: 'static', domains: null }] },
  ]
  const linkFor = (l, d) => (d.includes('*') ? null : `${l.port === 443 ? 'https' : 'http'}://${d}`)
  const out = importableSites(listeners, linkFor)
  assert.deepEqual(out.map((s) => [s.id, s.name, s.url, s.group, s.listener]), [
    ['l1/s1', 'NAS', 'https://nas.example.com', '存储', 'HTTPS'],
    ['l1/s2', 'go.example.com', 'https://go.example.com', '', 'HTTPS'],
  ])
})

test('linksFromSites 跳过已有地址与重复地址', () => {
  const sites = [
    { id: 'a', name: 'A', group: 'g', url: 'https://a.example.com' },
    { id: 'b', name: 'B', group: '', url: 'https://b.example.com' },
    { id: 'c', name: 'C', group: '', url: 'https://b.example.com' },
  ]
  const out = linksFromSites(sites, [{ ...emptyLink(), name: 'old', url: 'https://a.example.com' }])
  assert.deepEqual(out.map((l) => [l.name, l.url, l.group]), [['B', 'https://b.example.com', '']])
  assert.equal(out[0].sameTab, false)
})

test('图标写法与安全地址', () => {
  assert.equal(iconKind(''), 'none')
  assert.equal(iconKind(' 📦 '), 'text')
  assert.equal(iconKind('https://cdn.example.com/a.svg'), 'image')
  assert.equal(iconKind('data:image/png;base64,AAAA'), 'image')
  assert.equal(safeIconSrc('data:image/png;base64,AAAA'), 'data:image/png;base64,AAAA')
  assert.equal(safeIconSrc('data:text/html;base64,AAAA'), null)
  assert.equal(safeIconSrc('data:image/svg+xml,<svg onload=alert(1)>'), null)
  assert.equal(safeIconSrc('javascript:alert(1)'), null)
  assert.equal(safeIconSrc('https://u:p@cdn.example.com/a.png'), null)
  assert.equal(safeIconSrc('https://cdn.example.com/a.png'), 'https://cdn.example.com/a.png')
  assert.ok(textIconOk('👨‍👩‍👧'))
  assert.ok(!textIconOk('一二三四五六七八九'))
  assert.ok(!textIconOk('<b>'))
})

test('initial 与 paletteColor 与访客页面一致', () => {
  assert.equal(initial(' nas'), 'N')
  assert.equal(initial('群晖'), '群')
  assert.equal(initial('  '), '?')
  assert.equal(initial('★ 7'), '7')
  // 与 Go 端 paletteColor 相同的 FNV-1a：NAS → #b45309（见 internal/webpage/panel.go）
  assert.equal(paletteColor('NAS'), '#b45309')
  assert.equal(paletteColor('内网'), '#15803d')
})

test('hasVars 与 toPanel', () => {
  assert.ok(hasVars('https://{host}:8443'))
  assert.ok(!hasVars('https://a.example.com'))
  const p = toPanel({ key: 'k', name: 'n', enable: true, usedBy: ['a'], links: [{ name: 'x', url: 'https://x' }] })
  assert.equal('usedBy' in p, false)
  assert.equal('key' in p, false)
  assert.equal(p.links[0].lanUrl, '')
  assert.deepEqual(p.lanCidrs, [])
})

test('内置图标写法、底色深浅与筛选', async () => {
  const { builtinId, lightColor, filterIcons, iconKind } = await import('../src/panels.ts')
  assert.equal(iconKind('builtin:jellyfin'), 'builtin')
  assert.equal(builtinId(' builtin:jellyfin '), 'jellyfin')
  assert.equal(builtinId('https://x'), '')
  // 与 Go 端 webpage.LightColor 相同的阈值
  assert.ok(lightColor('#ffc135'))
  assert.ok(lightColor('#b5b5b6'))
  assert.ok(!lightColor('#0e7c66'))
  assert.ok(!lightColor('#f38020'))
  assert.ok(!lightColor('red'))
  const list = [
    { id: 'jellyfin', name: 'Jellyfin', cat: 'apps' },
    { id: 'frp', name: 'frp', cat: 'gately' },
    { id: 'house', name: '主页', cat: 'generic' },
  ]
  assert.deepEqual(filterIcons(list, '', '').map((i) => i.id), ['jellyfin', 'frp', 'house'])
  assert.deepEqual(filterIcons(list, 'gately', '').map((i) => i.id), ['frp'])
  assert.deepEqual(filterIcons(list, '', 'JELLY').map((i) => i.id), ['jellyfin'])
  assert.deepEqual(filterIcons(list, '', '主').map((i) => i.id), ['house'])
  assert.deepEqual(filterIcons(list, 'apps', 'frp'), [])
})

test('toPanel 规范化其他访问方式', () => {
  const p = toPanel({ name: 'n', enable: true, links: [{ name: 'x', url: 'https://x', alts: [{ name: 'v6', url: 'https://[::1]', extra: 1 }] }, { name: 'y', url: 'https://y' }] })
  assert.deepEqual(p.links[0].alts, [{ name: 'v6', url: 'https://[::1]' }])
  assert.deepEqual(p.links[1].alts, [])
  assert.deepEqual(emptyLink().alts, [])
})

test('搜索引擎：地址检查、增删与移动时默认选项跟随', async () => {
  const m = await import('../src/panels.ts')
  assert.equal(m.engineUrlProblem('https://www.google.com/search?q={q}'), '')
  assert.equal(m.engineUrlProblem('https://www.google.com/search'), 'q')
  assert.equal(m.engineUrlProblem('ftp://x.com/?q={q}'), 'scheme')
  assert.equal(m.engineUrlProblem('javascript:{q}'), 'scheme')
  assert.equal(m.engineUrlProblem('https://{q}.com/'), 'scheme')
  for (const e of m.ENGINE_PRESETS) {
    assert.equal(m.engineUrlProblem(e.url), '', e.name)
  }
  const E = (name) => ({ name, url: `https://${name}.example.com/?q={q}` })
  let list = m.addEngine([], E('a'))
  list = m.addEngine(list, E('b'))
  assert.equal(m.addEngine(list, E('A')), list, '名称重复不添加')
  assert.equal(m.addEngine(Array.from({ length: m.MAX_ENGINES }, (_, i) => E('e' + i)), E('x')).length, m.MAX_ENGINES)
  list = m.addEngine(list, E('c'))
  // 默认 3 = c；删除 a 后 c 变为第 2 个
  let r = m.removeEngine(list, 3, 0)
  assert.deepEqual(r.engines.map((e) => e.name), ['b', 'c'])
  assert.equal(r.def, 2)
  assert.equal(m.removeEngine(list, 3, 2).def, 0, '删除默认引擎回到本站')
  assert.equal(m.removeEngine(list, 1, 2).def, 1)
  assert.equal(m.removeEngine(list, 0, 1).def, 0)
  assert.equal(m.removeEngine(list, 2, 9).engines, list)
  r = m.moveEngine(list, 1, 0, 1)
  assert.deepEqual(r.engines.map((e) => e.name), ['b', 'a', 'c'])
  assert.equal(r.def, 2, '默认的 a 随之下移')
  assert.equal(m.moveEngine(list, 2, 0, 1).def, 1, '被换上来的默认引擎位置跟随')
  assert.equal(m.moveEngine(list, 0, 0, 1).def, 0)
  assert.equal(m.moveEngine(list, 1, 0, -1).engines, list)
})

test('toPanel 补全搜索、背景与登录可见字段', () => {
  const p = toPanel({ name: 'x', enable: true, links: [{ name: 'a', url: 'https://a.example.com' }] })
  assert.deepEqual([p.engines, p.defaultEngine, p.background, p.bgDim], [[], 0, '', 0])
  assert.equal(p.links[0].authOnly, false)
  assert.equal(toPanel({ ...p, links: [{ ...p.links[0], authOnly: true }] }).links[0].authOnly, true)
})

test('toPanel 补全卡片尺寸与背景模糊字段', () => {
  const p = toPanel({ name: 'x', enable: true, links: [{ name: 'a', url: 'https://a.example.com' }] })
  assert.equal(p.bgBlur, 0)
  assert.deepEqual([p.links[0].size, p.links[0].hideName, p.links[0].hideDesc], ['', false, false])
  const q = toPanel({ ...p, bgBlur: 8, links: [{ ...p.links[0], size: 'small', hideName: true, hideDesc: true }] })
  assert.equal(q.bgBlur, 8)
  assert.deepEqual([q.links[0].size, q.links[0].hideName, q.links[0].hideDesc], ['small', true, true])
})

test('toPanel 补全标签页标题、图标与页面描述', () => {
  const p = toPanel({ name: 'x', enable: true, links: [] })
  assert.deepEqual([p.tabTitle, p.favicon, p.description], ['', '', ''])
  const q = toPanel({ ...p, tabTitle: 'T', favicon: '📦', description: 'D' })
  assert.deepEqual([q.tabTitle, q.favicon, q.description], ['T', '📦', 'D'])
})

test('toPanel 补全页脚字段', () => {
  const p = toPanel({ name: 'x', enable: true, links: [] })
  assert.deepEqual([p.footer, p.footerUrl, p.hideFooter], ['', '', false])
  const q = toPanel({ ...p, footer: '© x', footerUrl: 'https://e.com', hideFooter: true })
  assert.deepEqual([q.footer, q.footerUrl, q.hideFooter], ['© x', 'https://e.com', true])
})

test('mergeLinks 并入导入的链接：同名分组接在组尾，可跳过重复地址，超过上限的不加入', () => {
  const existing = [L('a', 'x'), L('b', 'y')]
  const incoming = [L('c', 'x'), L('a', 'z'), L('d', 'z'), L('d', 'w')]
  const r = mergeLinks(existing, incoming, true)
  // a 与已有地址重复、第二个 d 与导入内容中的 d 重复，都跳过
  assert.deepEqual(names(r.links), ['a', 'c', 'b', 'd'])
  assert.deepEqual(r.links.map((l) => l.group), ['x', 'x', 'y', 'z'])
  assert.equal(r.added, 2)
  assert.equal(r.duplicates, 2)
  assert.equal(r.overflow, 0)
  // 不跳过时全部加入；原数组不被修改
  const all = mergeLinks(existing, incoming, false)
  assert.equal(all.added, 4)
  assert.equal(existing.length, 2)
  // 上限
  const many = Array.from({ length: MAX_LINKS - 1 }, (_, i) => L(`m${i}`))
  const over = mergeLinks(many, [L('p1'), L('p2'), L('p3')], true)
  assert.equal(over.links.length, MAX_LINKS)
  assert.equal(over.added, 1)
  assert.equal(over.overflow, 2)
})

test('mergeLinks 补齐缺少的字段并复制其他访问方式', () => {
  const alts = [{ name: 'IPv6', url: 'https://[2001:db8::1]' }]
  const r = mergeLinks([], [{ name: 'n', group: '', url: 'https://n.example.com', alts }], false)
  assert.equal(r.links[0].hide, false)
  assert.equal(r.links[0].size, '')
  assert.deepEqual(r.links[0].alts, alts)
  assert.notEqual(r.links[0].alts, alts)
})

test('freeName 未占用时原样返回，否则依次编号', () => {
  assert.equal(freeName('Sun-Panel', ['Home']), 'Sun-Panel')
  assert.equal(freeName('Sun-Panel', ['Sun-Panel', ' Sun-Panel 2 ']), 'Sun-Panel 3')
})

test('布局：清空为 0（默认），越界收紧到范围，小数取整', () => {
  assert.equal(normalizeLayoutValue('iconSize', null), 0)
  assert.equal(normalizeLayoutValue('iconSize', undefined), 0)
  assert.equal(normalizeLayoutValue('iconSize', 0), 0)
  assert.equal(normalizeLayoutValue('iconSize', -5), 0)
  assert.equal(normalizeLayoutValue('iconSize', Number.NaN), 0)
  assert.equal(normalizeLayoutValue('iconSize', 10), 24)
  assert.equal(normalizeLayoutValue('iconSize', 999), 128)
  assert.equal(normalizeLayoutValue('iconSize', 71.6), 72)
  assert.equal(normalizeLayoutValue('columns', 13), 12)
  assert.equal(normalizeLayoutValue('mobileColumns', 9), 8)
  assert.equal(normalizeLayoutValue('gap', 2), 4)
  assert.equal(normalizeLayoutValue('maxWidth', 100), 640)
  assert.equal(normalizeLayoutValue('maxWidth', 5000), 3840)
  // 与后端的范围一致
  assert.deepEqual(Object.fromEntries(Object.entries(LAYOUT_LIMITS).map(([k, v]) => [k, [v.min, v.max]])),
    { iconSize: [24, 128], columns: [1, 12], mobileColumns: [1, 8], gap: [4, 48], maxWidth: [640, 3840] })
})

test('布局：默认值随样式变化，旧配置载入后为默认布局', () => {
  assert.equal(layoutDefaults('').iconSize, 40)
  assert.equal(layoutDefaults('app').iconSize, 64)
  assert.equal(layoutDefaults('').mobileColumns, 2)
  assert.equal(layoutDefaults('app').columns, 0)
  const p = emptyPanel()
  assert.equal(p.cardStyle, '')
  assert.deepEqual([p.iconSize, p.columns, p.mobileColumns, p.gap, p.maxWidth], [0, 0, 0, 0, 0])
  // 旧版面板没有这些字段，未知样式按卡片处理
  const old = toPanel({ name: 'x', enable: true, links: [] })
  assert.equal(old.cardStyle, '')
  assert.equal(old.iconSize, 0)
  assert.equal(toPanel({ name: 'x', enable: true, links: [], cardStyle: 'weird' }).cardStyle, '')
  // 分组数量与多地址角标默认关闭
  assert.equal(old.groupCount, false)
  assert.equal(old.altBadge, false)
  assert.deepEqual([p.groupCount, p.altBadge], [false, false])
  const on = toPanel({ name: 'x', enable: true, links: [], groupCount: true, altBadge: true })
  assert.deepEqual([on.groupCount, on.altBadge], [true, true])
  const app = toPanel({ name: 'x', enable: true, links: [], cardStyle: 'app', iconSize: 72, columns: 6, mobileColumns: 4, gap: 20, maxWidth: 1600 })
  assert.deepEqual([app.cardStyle, app.iconSize, app.columns, app.mobileColumns, app.gap, app.maxWidth], ['app', 72, 6, 4, 20, 1600])
})

test('尺寸：默认尺寸在卡片样式下为 1×2、图标样式下为 1×1，选回默认尺寸时保存为空', () => {
  assert.deepEqual(SIZE_CHOICES, ['1x1', '1x2', '2x2'])
  assert.equal(sizeChoice('', ''), '1x2')
  assert.equal(sizeChoice('', 'app'), '1x1')
  assert.equal(sizeChoice(undefined, ''), '1x2')
  for (const style of ['', 'app']) {
    assert.equal(sizeChoice('small', style), '1x1')
    assert.equal(sizeChoice('wide', style), '1x2')
    assert.equal(sizeChoice('big', style), '2x2')
    // 往返换算：选什么显示什么
    for (const c of SIZE_CHOICES) {
      assert.equal(sizeChoice(sizeValue(c, style), style), c)
    }
  }
  assert.deepEqual(['1x1', '1x2', '2x2'].map((c) => sizeValue(c, '')), ['small', '', 'big'])
  assert.deepEqual(['1x1', '1x2', '2x2'].map((c) => sizeValue(c, 'app')), ['', 'wide', 'big'])
})

test('faviconUrls：内网地址优先，只取 http/https，去重并最多 10 个', () => {
  const l = { url: 'https://nas.example.com', lanUrl: ' http://192.168.1.2:5000 ', alts: [{ name: 'a', url: 'https://nas.example.com' }, { name: 'b', url: 'smb://nas/x' }, { name: 'c', url: 'HTTPS://v6.example.com' }] }
  assert.deepEqual(faviconUrls(l), ['http://192.168.1.2:5000', 'https://nas.example.com', 'HTTPS://v6.example.com'])
  assert.deepEqual(faviconUrls({ url: '', lanUrl: '', alts: [] }), [])
  const many = { url: 'https://a.example.com', lanUrl: '', alts: Array.from({ length: 12 }, (_, i) => ({ name: String(i), url: `https://h${i}.example.com` })) }
  assert.equal(faviconUrls(many).length, 10)
})

test('开关：旧配置载入后语言切换隐藏、换肤与锁标记显示、点击弹出选择；未知点击方式按默认', () => {
  const old = toPanel({ name: 'x', enable: true, links: [] })
  assert.deepEqual([old.altClick, old.hideAppName, old.hideLock, old.showLang, old.hideSkin], ['', false, false, false, false])
  const p = emptyPanel()
  assert.deepEqual([p.altClick, p.hideAppName, p.hideLock, p.showLang, p.hideSkin], ['', false, false, false, false])
  const on = toPanel({ name: 'x', enable: true, links: [], altClick: 'direct', hideAppName: true, hideLock: true, showLang: true, hideSkin: true })
  assert.deepEqual([on.altClick, on.hideAppName, on.hideLock, on.showLang, on.hideSkin], ['direct', true, true, true, true])
  assert.equal(toPanel({ name: 'x', enable: true, links: [], altClick: 'menu' }).altClick, '')
  // 链接尺寸原样保留
  const q = toPanel({ name: 'x', enable: true, links: [{ ...emptyLink(), name: 'a', url: 'https://a.example.com', size: 'wide' }, { ...emptyLink(), name: 'b', url: 'https://b.example.com', size: 'big' }] })
  assert.deepEqual(q.links.map((l) => l.size), ['wide', 'big'])
})

test('renameLinkGroup 改名保持位置，合并到先出现的位置，登录后可见随之迁移', () => {
  // 普通改名：位置不变，authGroups 跟着改名
  let links = [L('a', 'x'), L('b', 'y'), L('c', 'x')]
  let r = renameLinkGroup(links, ['x'], 'x', ' 新组 ')
  assert.deepEqual(names(r.links), ['a', 'c', 'b'])
  assert.deepEqual(r.links.map((l) => l.group), ['新组', '新组', 'y'])
  assert.deepEqual(r.authGroups, ['新组'])
  assert.equal(r.links[0], links[0], '保留对象身份')

  // 合并到已有分组：放在两者中先出现的位置
  links = [L('a', 'x'), L('b', 'y'), L('c', 'z'), L('d', 'x')]
  r = renameLinkGroup(links, [], 'z', 'x')
  assert.deepEqual(names(r.links), ['a', 'd', 'c', 'b'])
  links = [L('a', 'x'), L('b', 'y'), L('c', 'x')]
  r = renameLinkGroup(links, [], 'x', 'y')
  assert.deepEqual(names(r.links), ['a', 'c', 'b'])
  assert.ok(r.links.every((l) => l.group === 'y'))

  // 登录后可见分组并入普通分组：被并入的链接逐个勾上，目标组原有链接不变
  links = [L('a', 'secret'), L('b', 'pub')]
  r = renameLinkGroup(links, ['secret'], 'secret', 'pub')
  assert.deepEqual(r.authGroups, [])
  assert.equal(r.links.find((l) => l.name === 'a').authOnly, true)
  assert.equal(r.links.find((l) => l.name === 'b').authOnly, false)

  // 并入登录后可见分组：不改链接，authGroups 保留目标组
  links = [L('a', 'pub'), L('b', 'secret')]
  r = renameLinkGroup(links, ['secret'], 'pub', 'secret')
  assert.deepEqual(r.authGroups, ['secret'])
  assert.equal(r.links[0].authOnly, false)

  // 无效：名称未变、为空、分组不存在
  links = [L('a', 'x')]
  assert.equal(renameLinkGroup(links, [], 'x', ' x '), null)
  assert.equal(renameLinkGroup(links, [], 'x', '  '), null)
  assert.equal(renameLinkGroup(links, [], 'nope', 'y'), null)
})
