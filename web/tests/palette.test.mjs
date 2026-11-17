// 全局搜索（命令面板）纯函数测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { certEntries, ddnsEntries, forwardEntries, moveIndex, recordName, score, search, siteEntries } from '../src/palette.ts'
import { isPaletteShortcut } from '../src/composables/usePalette.ts'

const page = (title, path, extra = {}) => ({ id: 'page:' + path, kind: 'page', title, sub: '', path, ...extra })

test('匹配不区分大小写，多个词都要命中', () => {
  const e = page('WebDAV 服务', '/files/webdav', { sub: '文件服务 · 文件共享' })
  assert.ok(score(e, 'webdav') > 0)
  assert.ok(score(e, 'WEBDAV 文件') > 0)
  assert.equal(score(e, 'webdav ftp'), 0)
  // 空查询全部匹配
  assert.ok(score(e, '  ') > 0)
})

test('标题开头匹配排在包含匹配之前，页面排在数据之前', () => {
  const entries = [
    { id: 'site:a', kind: 'site', title: 'tailnet.example.com', sub: '', path: '/web' },
    page('网络诊断', '/nettools', { keywords: 'nettools tail' }),
    page('My Tailscale', '/mesh/x'),
    page('Tailscale', '/mesh/tailscale'),
  ]
  // 页面在前：标题开头匹配 > 标题包含 > 关键词；数据排在页面之后
  assert.deepEqual(
    search(entries, 'tail').map((e) => e.id),
    ['page:/mesh/tailscale', 'page:/mesh/x', 'page:/nettools', 'site:a'],
  )
  // 空查询保持原顺序
  assert.deepEqual(
    search(entries, '').map((e) => e.id),
    ['page:/nettools', 'page:/mesh/x', 'page:/mesh/tailscale', 'site:a'],
  )
  // 每类最多 perKind 条
  const many = Array.from({ length: 20 }, (_, i) => ({ id: 'cert:' + i, kind: 'cert', title: `a${i}.com`, sub: '', path: '/ssl' }))
  assert.equal(search(many, 'a', 8).length, 8)
})

test('站点条目：第一个域名作标题，其余域名参与匹配，跳转带站点参数', () => {
  const r = siteEntries([
    { key: 'l1', name: 'HTTPS', port: 443, sites: [{ key: 's1', name: 'NAS', domains: ['nas.example.com', 'files.example.com'] }, { key: 's2', name: '只有名称', domains: [] }, { name: '无 key' }] },
    { key: 'l2', port: 80, sites: null },
  ])
  assert.equal(r.length, 2)
  assert.deepEqual(r[0], {
    id: 'site:l1/s1',
    kind: 'site',
    title: 'nas.example.com',
    sub: 'NAS · HTTPS :443',
    path: '/web',
    query: { editListener: 'l1', editSite: 's1' },
    keywords: 'files.example.com',
  })
  assert.equal(r[1].title, '只有名称')
  assert.ok(score(r[0], 'files.example') > 0)
  assert.deepEqual(siteEntries(null), [])
})

test('端口转发、DDNS 与证书条目', () => {
  const f = forwardEntries([{ key: 'r1', name: 'NAS SSH', protocols: ['tcp'], listenPorts: '2222', targetAddresses: ['192.168.1.10'], targetPorts: '22' }, { name: '没有 key' }])
  assert.equal(f.length, 1)
  assert.equal(f[0].title, 'NAS SSH')
  assert.equal(f[0].sub, 'TCP 2222 → 192.168.1.10:22')
  assert.equal(f[0].path, '/portforward')
  // 按端口与目标地址也能搜到
  assert.ok(score(f[0], '2222') > 0)
  assert.ok(score(f[0], '192.168.1.10') > 0)

  assert.equal(recordName({ sub: '@', zone: 'example.com' }), 'example.com')
  assert.equal(recordName({ sub: '', zone: 'example.com' }), 'example.com')
  assert.equal(recordName({ sub: 'home', zone: 'example.com' }), 'home.example.com')
  const d = ddnsEntries([{ key: 't1', name: '阿里云', records: [{ key: 'a', sub: 'home', zone: 'example.com', type: 'AAAA' }, { sub: '', zone: '' }] }])
  assert.equal(d.length, 1)
  assert.equal(d[0].title, 'home.example.com')
  assert.equal(d[0].sub, '阿里云 · AAAA')

  const c = certEntries([
    { key: 'c1', remark: '泛域名', acme: { domains: ['example.com', '*.example.com'] } },
    { key: 'c2', remark: '', acme: { domains: null }, info: { domains: ['upload.example.org'] } },
    { key: 'c3', remark: '' },
  ])
  assert.equal(c[0].title, 'example.com')
  assert.equal(c[0].sub, '泛域名 · +1')
  assert.deepEqual(c[0].query, { q: 'example.com' })
  assert.equal(c[1].title, 'upload.example.org')
  assert.equal(c[2].title, 'c3')
  assert.equal(c[2].query, undefined)
})

test('上下键循环选择', () => {
  assert.equal(moveIndex(0, 1, 3), 1)
  assert.equal(moveIndex(2, 1, 3), 0)
  assert.equal(moveIndex(0, -1, 3), 2)
  assert.equal(moveIndex(-1, 1, 3), 0)
  assert.equal(moveIndex(-1, -1, 3), 2)
  assert.equal(moveIndex(0, 1, 0), -1)
})

test('快捷键：⌘K 与 Ctrl+K，终端内只认 ⌘K', () => {
  const k = (o) => ({ key: 'k', metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...o })
  assert.equal(isPaletteShortcut(k({ metaKey: true }), false), true)
  assert.equal(isPaletteShortcut(k({ ctrlKey: true }), false), true)
  assert.equal(isPaletteShortcut(k({ ctrlKey: true, key: 'K' }), false), true)
  assert.equal(isPaletteShortcut(k({ ctrlKey: true }), true), false)
  assert.equal(isPaletteShortcut(k({ metaKey: true }), true), true)
  assert.equal(isPaletteShortcut(k({}), false), false)
  assert.equal(isPaletteShortcut(k({ ctrlKey: true, shiftKey: true }), false), false)
  assert.equal(isPaletteShortcut(k({ ctrlKey: true, key: 'j' }), false), false)
})
