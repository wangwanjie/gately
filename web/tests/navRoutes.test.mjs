// 导航收敛后的路由表测试：旧地址全部重定向到存在的新位置，汇总页标签与导航、路由定义一致。node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { firstTab, hubOf, hubTabs, legacyRoutes, resolveLegacy } from '../src/navRoutes.ts'

const navSrc = readFileSync(new URL('../src/nav.ts', import.meta.url), 'utf8')
const routerSrc = readFileSync(new URL('../src/router.ts', import.meta.url), 'utf8')

// navPaths 为 nav.ts 里定义的全部页面路径
const navPaths = new Set([...navSrc.matchAll(/\{ path: '(\/[^']*)'/g)].map((m) => m[1]))

// routerPaths 从 router.ts 推出全部可进入的完整路径：顶层子路由与汇总页（path + children）
function routerPaths() {
  const out = new Set()
  const hubBlock = /path: '([a-z-]+)',\s+component: Hub,[\s\S]*?children: \[([\s\S]*?)\n\s+\],/g
  for (const m of routerSrc.matchAll(hubBlock)) {
    for (const c of m[2].matchAll(/\{ path: '([a-z-]*)', name: '[a-zA-Z-]+', component:/g)) {
      out.add('/' + m[1] + (c[1] ? '/' + c[1] : ''))
    }
  }
  for (const m of routerSrc.matchAll(/^ {8}\{ path: '([a-z-]+)', name: '[a-zA-Z-]+', component:/gm)) {
    out.add('/' + m[1])
  }
  return out
}

test('旧地址全部重定向到新位置', () => {
  const expected = {
    '/webtopology': '/web/topology',
    '/webvisitors': '/web/visitors',
    '/webpanels': '/web/panels',
    '/tailscale': '/mesh/tailscale',
    '/netbird': '/mesh/netbird',
    '/easytier': '/mesh/easytier',
    '/webdav': '/files/webdav',
    '/ftp': '/files/ftp',
    '/smb': '/files/smb',
    '/fileusers': '/files/users',
    '/audit': '/logs/audit',
    '/history': '/logs/history',
  }
  assert.deepEqual(legacyRoutes, expected)
  for (const [from, to] of Object.entries(expected)) {
    assert.equal(resolveLegacy(from), to)
  }
  // 未合并的页面与 API 页面地址不变
  for (const p of ['/web', '/ssl', '/webusers', '/frp', '/storage', '/settings', '/api-tokens', '/api-docs', '/overview']) {
    assert.equal(resolveLegacy(p), p)
  }
})

test('重定向目标都是存在的路由和导航页面', () => {
  const routes = routerPaths()
  for (const to of Object.values(legacyRoutes)) {
    assert.ok(routes.has(to), `路由里没有 ${to}`)
    assert.ok(navPaths.has(to), `nav.ts 里没有 ${to}`)
  }
  // 旧地址在路由里注册为重定向，且不再作为页面出现在导航里
  assert.match(routerSrc, /Object\.entries\(legacyRoutes\)\.map\(\(\[from, to\]\) => \(\{ path: from\.slice\(1\), redirect: to \}\)\)/)
  for (const from of Object.keys(legacyRoutes)) {
    assert.ok(!navPaths.has(from), `nav.ts 仍有旧地址 ${from}`)
    assert.ok(!routes.has(from), `路由仍把 ${from} 当页面`)
  }
})

test('汇总页的每个标签都有路由和导航项，且标签不重复', () => {
  const routes = routerPaths()
  const seen = new Set()
  for (const [hub, tabs] of Object.entries(hubTabs)) {
    assert.ok(navPaths.has(hub), `nav.ts 里没有汇总页入口 ${hub}`)
    for (const p of tabs) {
      assert.ok(!seen.has(p), `标签 ${p} 重复`)
      seen.add(p)
      assert.ok(routes.has(p), `路由里没有标签 ${p}`)
      assert.ok(navPaths.has(p), `nav.ts 里没有标签 ${p}`)
      assert.ok(navSrc.includes(`path: '${p}'`) && new RegExp(`path: '${p}'[^}]*hub: '${hub}'`).test(navSrc), `${p} 没有标记 hub: '${hub}'`)
    }
  }
})

test('hubOf 识别汇总页与标签', () => {
  assert.equal(hubOf('/files'), '/files')
  assert.equal(hubOf('/files/smb'), '/files')
  assert.equal(hubOf('/web'), '/web')
  assert.equal(hubOf('/web/panels'), '/web')
  assert.equal(hubOf('/logs/audit'), '/logs')
  assert.equal(hubOf('/ssl'), '')
  assert.equal(hubOf('/webdav'), '')
})

test('firstTab 跳过不可进入的标签', () => {
  const all = () => true
  assert.equal(firstTab('/mesh', all), '/mesh/tailscale')
  // 只开了 EasyTier
  assert.equal(firstTab('/mesh', (p) => p === '/mesh/easytier'), '/mesh/easytier')
  // 关闭 WebDAV 后落到 FTP
  assert.equal(firstTab('/files', (p) => p !== '/files/webdav'), '/files/ftp')
  // 全部关闭
  assert.equal(firstTab('/files', () => false), '')
  assert.equal(firstTab('/nope', all), '')
})

test('侧边栏入口数量收敛', () => {
  // 侧边栏入口：不带 hub 与 hidden 的页面，加上路径与 hub 相同的标签（Web 服务）
  const items = [...navSrc.matchAll(/\{ path: '(\/[^']*)'[^\n]*\}/g)].map((m) => m[0])
  const sidebar = items.filter((s) => {
    if (s.includes('hidden: true')) {
      return false
    }
    const hub = s.match(/hub: '([^']*)'/)
    const path = s.match(/path: '([^']*)'/)[1]
    return !hub || hub[1] === path
  })
  assert.equal(items.length, 42)
  assert.equal(sidebar.length, 27)
})
