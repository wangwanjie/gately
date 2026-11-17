// 快速建站向导纯函数与应用模板数据的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  certCovers,
  domainError,
  findCert,
  hostOfTarget,
  listenerChoices,
  listenerPayload,
  normalizeDomain,
  normalizeTarget,
  planListener,
  sitePayload,
  siteURL,
  targetError,
  templateTarget,
} from '../src/quickSite.ts'

const templates = JSON.parse(readFileSync(new URL('../src/siteTemplates.json', import.meta.url), 'utf8'))
const icons = JSON.parse(readFileSync(new URL('../../internal/webpage/panelicons.json', import.meta.url), 'utf8'))

test('应用模板：编号唯一、图标来自导航面板内置图标、端口与协议有效', () => {
  const ids = new Set()
  const iconIds = new Set(icons.map((i) => i.id))
  for (const t of templates) {
    assert.ok(!ids.has(t.id), t.id)
    ids.add(t.id)
    assert.ok(iconIds.has(t.icon), `${t.id} 图标 ${t.icon}`)
    assert.ok(['http', 'https'].includes(t.scheme), t.id)
    assert.ok(Number.isInteger(t.port) && t.port > 0 && t.port < 65536, t.id)
    // 跳过证书校验只对 https 后端有意义
    if (t.site.insecureSkipVerify) {
      assert.equal(t.scheme, 'https', t.id)
    }
    for (const v of Object.values(t.site.clientTimeouts ?? {})) {
      assert.ok(v >= 0 && v <= 3600, t.id)
    }
  }
  for (const id of ['homeassistant', 'jellyfin', 'emby', 'vaultwarden', 'nextcloud', 'immich', 'proxmox', 'synology', 'fnos']) {
    assert.ok(ids.has(id), id)
  }
})

test('域名规范化与校验', () => {
  assert.equal(normalizeDomain(' https://App.Example.com:8443/x?y '), 'app.example.com')
  assert.equal(normalizeDomain('nas.example.com.'), 'nas.example.com')
  assert.equal(domainError(''), 'empty')
  assert.equal(domainError('example'), 'format')
  assert.equal(domainError('a b.com'), 'format')
  assert.equal(domainError('-a.com'), 'format')
  assert.equal(domainError('app.example.com'), '')
  assert.equal(domainError('*.example.com'), '')
  assert.equal(domainError('a.*.com'), 'format')
  assert.equal(domainError('xn--fiqs8s.cn'), '')
})

test('后端地址：没写协议时补 http://，443 端口补 https://', () => {
  assert.equal(normalizeTarget('192.168.1.5:8123'), 'http://192.168.1.5:8123')
  assert.equal(normalizeTarget('nas:443/'), 'https://nas:443')
  assert.equal(normalizeTarget('https://pve:8006/'), 'https://pve:8006')
  assert.equal(normalizeTarget(''), '')
  assert.equal(targetError('http://1.2.3.4:80'), '')
  assert.equal(targetError('ftp://1.2.3.4'), 'format')
  assert.equal(targetError('http://u:p@1.2.3.4'), 'format')
  assert.equal(targetError(''), 'empty')
  assert.equal(hostOfTarget('192.168.1.5:8123'), '192.168.1.5')
})

test('模板生成后端地址：保留用户填写的主机，没有端口时用模板端口', () => {
  const pve = templates.find((t) => t.id === 'proxmox')
  assert.equal(templateTarget(pve, '192.168.1.9'), 'https://192.168.1.9:8006')
  assert.equal(templateTarget(pve, 'http://192.168.1.9/'), 'https://192.168.1.9:8006')
  assert.equal(templateTarget(pve, '192.168.1.9:9000'), 'https://192.168.1.9:9000')
  assert.equal(templateTarget(pve, ''), 'https://127.0.0.1:8006')
  assert.equal(templateTarget(pve, '[fd00::1]:8006'), 'https://[fd00::1]:8006')
})

test('证书匹配：精确与一级通配，已签发的优先，停用的不算', () => {
  assert.ok(certCovers(['app.example.com'], 'APP.example.com'))
  assert.ok(certCovers(['*.example.com'], 'app.example.com'))
  assert.ok(!certCovers(['*.example.com'], 'example.com'))
  assert.ok(!certCovers(['*.example.com'], 'a.b.example.com'))
  const certs = [
    { key: 'off', enable: false, info: { domains: ['app.example.com'] } },
    { key: 'pending', enable: true, acme: { domains: ['app.example.com'] } },
    { key: 'wild', enable: true, acme: { domains: ['*.example.com'] }, info: { domains: ['*.example.com', 'example.com'] } },
  ]
  assert.equal(findCert(certs, 'app.example.com').key, 'wild')
  assert.equal(findCert(certs.slice(0, 2), 'app.example.com').key, 'pending')
  assert.equal(findCert(certs, 'other.org'), null)
})

test('监听：复用协议一致的监听，标准端口优先；新建 HTTPS 监听同时在 80 跳转', () => {
  const ls = [
    { key: 'h8080', name: 'a', enable: true, tls: false, port: 8080 },
    { key: 'h80', name: 'b', enable: true, tls: false, port: 80 },
    { key: 's8443', name: 'c', enable: false, tls: true, port: 8443 },
  ]
  assert.deepEqual(listenerChoices(ls, false).map((l) => l.key), ['h80', 'h8080'])
  assert.deepEqual(listenerChoices(ls, true).map((l) => l.key), ['s8443'])
  // 80 已被 HTTP 监听占用：新建的 HTTPS 监听不再开 80
  assert.deepEqual(planListener(ls, true), { name: 'HTTPS 443', port: 443, tls: true, httpPort: 0 })
  assert.deepEqual(planListener([], true), { name: 'HTTPS 443', port: 443, tls: true, httpPort: 80 })
  assert.deepEqual(planListener([], true, 19843), { name: 'HTTPS 19843', port: 19843, tls: true, httpPort: 80 })
  assert.deepEqual(planListener([], false), { name: 'HTTP 80', port: 80, tls: false, httpPort: 0 })
  assert.deepEqual(planListener([{ key: 'x', name: 'x', enable: true, tls: true, port: 443, httpPort: 80 }], true, 8443).httpPort, 0)
  const body = listenerPayload(planListener([], true), [{ name: 's' }])
  assert.equal(body.httpPortMode, 'redirect')
  assert.equal(listenerPayload(planListener([], false), []).httpPortMode, '')
})

test('站点请求体：模板选项、跳过证书校验只对 https 后端生效', () => {
  const pve = templates.find((t) => t.id === 'proxmox')
  const s = sitePayload('pve.example.com', 'https://10.0.0.2:8006', pve, true)
  assert.deepEqual(s.domains, ['pve.example.com'])
  assert.equal(s.insecureSkipVerify, true)
  assert.equal(s.name, 'Proxmox VE')
  assert.equal(sitePayload('a.example.com', 'http://10.0.0.2:80', null, true).insecureSkipVerify, false)
  const immich = templates.find((t) => t.id === 'immich')
  assert.deepEqual(sitePayload('i.example.com', 'http://10.0.0.2:2283', immich, false).clientTimeouts, { readIdle: 300, writeIdle: 300 })
  assert.equal(sitePayload('a.example.com', 'http://x:1', null, false).clientTimeouts, undefined)
})

test('访问地址：标准端口省略，通配域名给出示例地址', () => {
  assert.equal(siteURL('a.example.com', true, 443), 'https://a.example.com')
  assert.equal(siteURL('a.example.com', false, 19841), 'http://a.example.com:19841')
  assert.equal(siteURL('*.example.com', true, 443), 'https://www.example.com')
})
