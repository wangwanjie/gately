// 站点访问地址拼接的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { accessPort, accessScheme, hostPart, safeHttpUrl, siteLink } from '../src/siteUrl.ts'

const https = (port, extra = {}) => ({ port, tls: true, ...extra })
const http = (port, extra = {}) => ({ port, tls: false, ...extra })
const text = (d, l) => siteLink(d, l)?.text ?? null

test('按 TLS 推算协议，默认端口省略', () => {
  assert.equal(text('a.com', https(443)), 'https://a.com')
  assert.equal(text('a.com', https(8443)), 'https://a.com:8443')
  assert.equal(text('a.com', http(80)), 'http://a.com')
  assert.equal(text('a.com', http(8080)), 'http://a.com:8080')
  // https 的 80 端口、http 的 443 端口都不是协议默认端口，要保留
  assert.equal(text('a.com', https(80)), 'https://a.com:80')
  assert.equal(text('a.com', http(443)), 'http://a.com:443')
})

test('访问协议与端口覆盖监听', () => {
  // 协议分流在 443 收流量，转给 loopback 上的 8443
  assert.equal(text('a.com', https(8443, { accessPort: 443 })), 'https://a.com')
  // CDN 回源到 http:8080，对外是 https:443
  assert.equal(text('a.com', http(8080, { accessScheme: 'https', accessPort: 443 })), 'https://a.com')
  // 只改协议时沿用监听端口
  assert.equal(text('a.com', http(8443, { accessScheme: 'https' })), 'https://a.com:8443')
  // 0 与空值表示不覆盖
  assert.equal(text('a.com', https(8443, { accessScheme: '', accessPort: 0 })), 'https://a.com:8443')
  // 未知协议按监听推算，不会生成其他协议的链接
  assert.equal(text('a.com', https(443, { accessScheme: 'javascript' })), 'https://a.com')
  assert.equal(accessScheme(http(80, { accessScheme: 'ftp' })), 'http')
  assert.equal(accessPort(http(80, { accessPort: -5 })), 80)
})

test('通配与非法域名不生成链接', () => {
  for (const d of ['*', '*.a.com', 'a.*.com', '', '  ', 'a..com', '-a.com', 'a-.com', 'a.com/x', 'a.com?x', 'a.com#x', 'user@a.com', 'a.com:8080', 'javascript:alert(1)', 'a com', 'a.com\\b', '%61.com', `${'a'.repeat(64)}.com`, `${'a.'.repeat(130)}com`]) {
    assert.equal(siteLink(d, https(443)), null, d)
  }
})

test('IP 与国际化域名', () => {
  assert.equal(text('192.168.1.10', http(8080)), 'http://192.168.1.10:8080')
  assert.equal(text('::1', https(8443)), 'https://[::1]:8443')
  assert.equal(text('[2001:db8::1]', https(443)), 'https://[2001:db8::1]')
  assert.equal(siteLink('::1', https(8443)).href, 'https://[::1]:8443')
  assert.equal(hostPart('::g'), null)
  assert.equal(hostPart('1:2:3:4:5:6:7:8:9'), null)
  // 显示与复制保留原写法，链接地址用 punycode
  const l = siteLink('中文.com', https(443))
  assert.equal(l.text, 'https://中文.com')
  assert.equal(l.href, 'https://xn--fiq228c.com')
  assert.equal(text('my_host.lan', http(80)), 'http://my_host.lan')
  assert.equal(text('A.COM', https(443)), 'https://a.com')
})

test('监听端口异常时不生成链接', () => {
  assert.equal(siteLink('a.com', https(0)), null)
  assert.equal(siteLink('a.com', https(70000)), null)
  assert.equal(siteLink('a.com', https(1.5)), null)
})

test('safeHttpUrl 只放行 http(s) 绝对地址', () => {
  assert.equal(safeHttpUrl('https://x.trycloudflare.com'), 'https://x.trycloudflare.com/')
  assert.equal(safeHttpUrl('http://192.168.1.2:8080/files/'), 'http://192.168.1.2:8080/files/')
  for (const u of ['javascript:alert(1)', 'JAVASCRIPT:alert(1)', ' javascript:alert(1)', 'data:text/html,x', 'vbscript:x', 'file:///etc/passwd', '/relative', '//a.com', 'http://', '']) {
    assert.equal(safeHttpUrl(u), null, u)
  }
})
