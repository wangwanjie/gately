// 站点访问地址拼接的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { accessPort, accessScheme, forwardedEntry, hostPart, parsePorts, safeHttpUrl, siteLink } from '../src/siteUrl.ts'

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

// 与 B760 实际配置相同：协议分流 4433 把 anylink 送到 127.0.0.1:4436，其余域名进 127.0.0.2:4435 的 Web 监听
const sniRule = {
  name: '4433 SNI 分流',
  enable: true,
  listenPorts: '4433',
  routes: [{ name: 'anylink', protocol: 'tls', domains: ['anylink.ddns.vanjay.cn'], targetAddresses: ['127.0.0.1'], targetPort: 4436 }],
  targetAddresses: ['127.0.0.2'],
  targetPorts: '4435',
}
const disabledRule = { ...sniRule, name: '4443 SNI 分流', enable: false, listenPorts: '4443' }
const proto = (...rules) => [{ kind: 'protoroute', rules }]
const inner = { listenIp: '127.0.0.2', port: 4435, tls: true }

test('经协议分流默认目标进入的监听用分流入口端口', () => {
  const e = forwardedEntry(inner, 'emby.ddns.vanjay.cn', proto(disabledRule, sniRule))
  assert.deepEqual(e, { kind: 'protoroute', rule: '4433 SNI 分流', port: 4433 })
  assert.equal(siteLink('emby.ddns.vanjay.cn', { ...inner, accessPort: e.port }).text, 'https://emby.ddns.vanjay.cn:4433')
  // 不区分域名时按默认目标
  assert.equal(forwardedEntry(inner, '', proto(sniRule))?.port, 4433)
})

test('协议分流路由按域名命中，目标地址与端口必须对应监听', () => {
  const anylink = { listenIp: '', port: 4436, tls: true }
  assert.equal(forwardedEntry(anylink, 'anylink.ddns.vanjay.cn', proto(sniRule))?.port, 4433)
  // 命中路由的域名不会去默认目标，4435 监听上同名域名不推算
  assert.equal(forwardedEntry(inner, 'anylink.ddns.vanjay.cn', proto(sniRule)), null)
  // 停用规则、停用路由不计入
  assert.equal(forwardedEntry(inner, 'a.com', proto(disabledRule)), null)
  const r = { ...sniRule, routes: [{ ...sniRule.routes[0], domains: ['*.ddns.vanjay.cn'], targetAddresses: ['127.0.0.2'], targetPort: 4435, disabled: true }] }
  assert.equal(forwardedEntry(inner, 'x.ddns.vanjay.cn', proto(r))?.port, 4433)
  // 监听具体回环地址时，转发到其他回环地址连不上，不计入
  assert.equal(forwardedEntry({ ...inner, listenIp: '127.0.0.1' }, 'a.com', proto(sniRule)), null)
  // 监听全部地址时回环目标可达
  assert.equal(forwardedEntry({ ...inner, listenIp: '' }, 'a.com', proto(sniRule))?.port, 4433)
  // 只接受 acme-tls/1 的路由不影响浏览器访问
  const acme = { ...sniRule, routes: [{ name: 'acme', protocol: 'tls', domains: ['a.com'], alpn: ['acme-tls/1'], targetAddresses: ['127.0.0.1'], targetPort: 9 }] }
  assert.equal(forwardedEntry(inner, 'a.com', proto(acme))?.port, 4433)
})

test('端口转发按端口映射推算', () => {
  const pf = (rule) => [{ kind: 'portforward', rules: [{ name: 'pf', enable: true, ...rule }] }]
  const l = { listenIp: '', port: 8443, tls: true }
  assert.equal(forwardedEntry(l, 'a.com', pf({ listenPorts: '443', targetAddresses: ['127.0.0.1'], targetPorts: '8443' }))?.port, 443)
  assert.equal(forwardedEntry(l, 'a.com', pf({ listenPorts: '9000-9002', targetAddresses: ['::1'], targetPorts: '8441-8443' }))?.port, 9002)
  // 目标端口留空即与监听端口相同
  assert.equal(forwardedEntry(l, 'a.com', pf({ listenPorts: '8443', targetAddresses: ['[::1]'], targetPorts: '' }))?.port, 8443)
  // 转发到其他主机不计入
  assert.equal(forwardedEntry(l, 'a.com', pf({ listenPorts: '443', targetAddresses: ['192.168.1.2'], targetPorts: '8443' })), null)
})

test('端口表达式解析与后端一致', () => {
  assert.deepEqual(parsePorts('80, 443，8000-8002'), [80, 443, 8000, 8001, 8002])
  assert.equal(parsePorts(''), null)
  assert.equal(parsePorts('0'), null)
  assert.equal(parsePorts('10-5'), null)
  assert.equal(parsePorts('1-2000'), null)
})

test('通配路由按后缀长度优先，与书写顺序无关', () => {
  // 与后端一致：*.ddns.vanjay.cn 比 *.vanjay.cn 更具体，即使写在后面也先命中
  const r = {
    ...sniRule,
    routes: [
      { name: 'wide', protocol: 'tls', domains: ['*.vanjay.cn'], targetAddresses: ['127.0.0.1'], targetPort: 8443 },
      { name: 'narrow', protocol: 'tls', domains: ['*.ddns.vanjay.cn'], targetAddresses: ['127.0.0.2'], targetPort: 4435 },
    ],
  }
  assert.equal(forwardedEntry(inner, 'emby.ddns.vanjay.cn', proto(r))?.port, 4433)
  // 只匹配较宽通配的域名走 wide 路由，不进 4435 监听
  assert.equal(forwardedEntry(inner, 'a.vanjay.cn', proto(r)), null)
})

test('只转发 UDP 的端口转发规则不作为站点入口', () => {
  const pf = (protocols) => [{ kind: 'portforward', rules: [{ name: 'h3', enable: true, protocols, listenPorts: '443', targetAddresses: ['127.0.0.1'], targetPorts: '8443' }] }]
  const l = { listenIp: '', port: 8443, tls: true }
  assert.equal(forwardedEntry(l, 'a.com', pf(['udp'])), null)
  assert.equal(forwardedEntry(l, 'a.com', pf(['udp6', 'udp4'])), null)
  assert.equal(forwardedEntry(l, 'a.com', pf(['tcp', 'udp']))?.port, 443)
  assert.equal(forwardedEntry(l, 'a.com', pf(['tcp6']))?.port, 443)
  // 未带协议字段（旧数据）按 TCP 处理
  assert.equal(forwardedEntry(l, 'a.com', pf(undefined))?.port, 443)
})
