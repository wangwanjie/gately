// 首次启动引导纯函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { SCENES, checklistFor, countEnabled, countSites, disabledModulesFor, emptyFacts, enabledModulesFor, itemDone } from '../src/onboarding.ts'

// 与后端一致的模块清单与依赖
const manifest = JSON.parse(readFileSync(new URL('../../modules.json', import.meta.url), 'utf8'))
const ALL = manifest.modules.map((m) => ({ id: m.id, requires: m.requires }))

test('场景只引用 modules.json 中存在的模块，编号与后端一致', () => {
  const ids = new Set(ALL.map((m) => m.id))
  for (const s of SCENES) {
    for (const m of s.modules) {
      assert.ok(ids.has(m), `${s.id}: ${m}`)
    }
  }
  const goSrc = readFileSync(new URL('../../internal/app/onboarding.go', import.meta.url), 'utf8')
  const goScenes = goSrc.match(/onboardingScenes = \[\]string\{([^}]*)\}/)[1].match(/"([a-z]+)"/g).map((s) => s.slice(1, -1))
  assert.deepEqual(goScenes, SCENES.map((s) => s.id))
})

test('发布成域名：只开启反代、证书及相关模块，其余关闭', () => {
  const off = disabledModulesFor(['web'], ALL)
  for (const id of ['webservice', 'ssl', 'ddns', 'waf', 'guard', 'notify', 'nettools']) {
    assert.ok(!off.includes(id), id)
  }
  for (const id of ['portforward', 'frp', 'tailscale', 'smb', 'docker', 'webterminal']) {
    assert.ok(off.includes(id), id)
  }
  assert.deepEqual(off, [...off].sort())
})

test('依赖的模块一并开启：文件共享开启配置备份时保留存储挂载', () => {
  const on = enabledModulesFor(['files'], ALL)
  assert.ok(on.has('backup') && on.has('storage'))
  // 只开启依赖方时也带上被依赖的模块
  const fake = [{ id: 'a', requires: ['b'] }, { id: 'b', requires: ['c'] }, { id: 'c' }, { id: 'notify' }]
  const off = disabledModulesFor(['x'], fake)
  assert.deepEqual(off, ['a', 'b', 'c'])
})

test('全部功能或跳过：不关闭任何模块；没有编译进来的模块的开关原样保留', () => {
  assert.deepEqual(disabledModulesFor(['all'], ALL), [])
  assert.deepEqual(disabledModulesFor([], ALL), [])
  assert.deepEqual(disabledModulesFor(['web', 'all'], ALL), [])
  assert.deepEqual(disabledModulesFor(['all'], ALL, [], ['docker', 'gone'], ['gone']), ['gone'])
  assert.ok(disabledModulesFor(['ddns'], ALL, [], ['gone'], ['gone']).includes('gone'))
})

test('必须保留的模块（管理界面 HTTPS 用的证书模块）不关闭', () => {
  assert.ok(disabledModulesFor(['ddns'], ALL).includes('ssl'))
  assert.ok(!disabledModulesFor(['ddns'], ALL, ['ssl']).includes('ssl'))
})

test('清单按场景生成、去重，原有四项排在后面，模块关闭的项目不显示', () => {
  assert.deepEqual(checklistFor(['web']), ['site', 'https', 'ddns', 'security', 'check', 'import', 'notify'])
  assert.deepEqual(checklistFor(['web', 'forward']), ['site', 'https', 'ddns', 'portforward', 'stun', 'security', 'check', 'import', 'notify'])
  assert.deepEqual(checklistFor([]), checklistFor(['all']))
  const on = new Set(['webservice', 'ssl'])
  assert.deepEqual(checklistFor(['web'], (id) => on.has(id)), ['site', 'https', 'security', 'check', 'import'])
  // 组网：三种组网任意一个开启即显示
  assert.deepEqual(checklistFor(['mesh'], (id) => id === 'easytier'), ['mesh', 'security', 'check', 'import'])
})

test('完成判断看实际配置，自检与导入看手动标记', () => {
  const f = emptyFacts()
  for (const id of checklistFor(['all'])) {
    assert.equal(itemDone(id, f), false, id)
  }
  f.sites = 1
  f.totp = true
  f.done = ['check']
  assert.equal(itemDone('site', f), true)
  assert.equal(itemDone('https', f), false)
  assert.equal(itemDone('security', f), true)
  assert.equal(itemDone('check', f), true)
  assert.equal(itemDone('import', f), false)
  assert.equal(itemDone('security', { ...emptyFacts(), passkeys: 1 }), true)
})

test('countEnabled 与 countSites：只统计启用的条目，HTTPS 站点要求每个域名都有证书', () => {
  assert.equal(countEnabled(null), 0)
  assert.equal(countEnabled([{ enable: true }, { enable: false }, {}]), 2)
  const listeners = [
    { enable: true, tls: false, sites: [{ key: 'a', enable: true }, { key: 'b', enable: false }] },
    { enable: false, tls: false, sites: [{ key: 'c', enable: true }] },
    {
      enable: true,
      tls: true,
      sites: [{ key: 'd', enable: true }, { key: 'e', enable: true }],
      siteCerts: { d: [{ domain: 'x', name: 'c1' }], e: [{ domain: 'y', name: 'c1' }, { domain: 'z' }] },
    },
  ]
  assert.deepEqual(countSites(listeners), { sites: 3, https: 1 })
  assert.deepEqual(countSites(null), { sites: 0, https: 0 })
})
