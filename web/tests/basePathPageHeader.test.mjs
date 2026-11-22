// 经飞牛网关访问时给本站请求加 X-Gately-Page 的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
// 经 HTTP 地址打开时浏览器不发 Sec-Fetch-Site，后端凭这个请求头确认写请求来自管理界面自己
import { test } from 'node:test'
import assert from 'node:assert/strict'

// 最小浏览器环境：页面在网关前缀 /app/gately 下，经 HTTP 地址打开
const origin = 'http://nas.example.com:5000'
const fetched = []
globalThis.window = {
  location: { href: origin + '/app/gately/#/overview', origin, host: 'nas.example.com:5000' },
  fetch: (input, init) => {
    fetched.push({ input, init })
    return Promise.resolve(null)
  },
  open: () => null,
  WebSocket: class {},
}
globalThis.document = {
  querySelector: (sel) => (sel === 'meta[name="gately-base"]' ? { getAttribute: () => '/app/gately' } : null),
  addEventListener: () => {},
}
class FakeXHR {
  headers = {}
  open(method, url) {
    this.method = method
    this.url = String(url)
  }
  setRequestHeader(k, v) {
    this.headers[k] = v
  }
}
globalThis.XMLHttpRequest = FakeXHR
globalThis.HTMLAnchorElement = class {
  click() {}
}

const { pageHeader, isSameOrigin } = await import('../src/basePath.ts')

test('发往本站的 fetch 带上 X-Gately-Page 并补上前缀，保留原有请求头', async () => {
  fetched.length = 0
  await window.fetch('/api/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: '{}' })
  const { input, init } = fetched[0]
  assert.equal(input, '/app/gately/api/settings')
  const h = new Headers(init.headers)
  assert.equal(h.get(pageHeader), '1')
  assert.equal(h.get('Content-Type'), 'application/json')
  assert.equal(init.method, 'PUT')
})

test('完整地址与 Request 对象指向本站时同样带上', async () => {
  fetched.length = 0
  await window.fetch(origin + '/api/status')
  assert.equal(new Headers(fetched[0].init.headers).get(pageHeader), '1')
  await window.fetch(new Request(origin + '/app/gately/api/status', { headers: { Accept: 'application/json' } }))
  const h = new Headers(fetched[1].init.headers)
  assert.equal(h.get(pageHeader), '1')
  assert.equal(h.get('Accept'), 'application/json')
})

test('发往其他来源的请求不加（包括同主机其他端口）', async () => {
  fetched.length = 0
  await window.fetch('https://api.github.com/repos/x/y/releases')
  await window.fetch('http://nas.example.com:8096/x')
  for (const f of fetched) {
    assert.equal(new Headers(f.init?.headers).has(pageHeader), false)
  }
  assert.equal(isSameOrigin('http://nas.example.com:8096/x'), false)
  assert.equal(isSameOrigin('/api/x'), true)
})

test('发往本站的 XHR（上传文件）带上请求头', () => {
  const x = new XMLHttpRequest()
  x.open('POST', '/api/system/restore')
  assert.equal(x.url, '/app/gately/api/system/restore')
  assert.equal(x.headers[pageHeader], '1')
  const y = new XMLHttpRequest()
  y.open('GET', 'https://example.com/a')
  assert.equal(y.headers[pageHeader], undefined)
})
