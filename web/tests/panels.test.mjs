// 导航面板纯函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  emptyLink, groupLinks, moveLink, moveGroup, insertLink, importableSites, linksFromSites, iconKind, safeIconSrc,
  initial, paletteColor, textIconOk, hasVars, toPanel,
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
