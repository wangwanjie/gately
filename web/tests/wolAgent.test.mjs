// 网络唤醒客户端安装命令的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { guessPlatform, installCommand, normalizeServer, scriptURL, uninstallCommand, validToken } from '../src/wolAgent.ts'

const tok = 'a'.repeat(64)

test('normalizeServer 去掉末尾斜杠并拒绝可注入命令的字符', () => {
  assert.equal(normalizeServer(' https://g.example.com:16601/ '), 'https://g.example.com:16601')
  assert.equal(normalizeServer('http://[fd00::1]:16601'), 'http://[fd00::1]:16601')
  assert.equal(normalizeServer('http://nas.local/app/gately'), 'http://nas.local/app/gately')
  for (const bad of ['', 'ftp://x', 'g.example.com', "http://x/'; rm -rf /", 'http://x/$(id)', 'http://x y', 'http://x/`id`', 'http://x/"a"', 'http://x?a=1&b=2']) {
    assert.equal(normalizeServer(bad), '', bad)
  }
})

test('validToken 只接受十六进制凭据', () => {
  assert.ok(validToken(tok))
  assert.ok(!validToken('********'))
  assert.ok(!validToken('A'.repeat(64)))
  assert.ok(!validToken('a'.repeat(31)))
  assert.ok(!validToken(`${tok}'`))
})

test('installCommand 地址或凭据无效时不生成命令', () => {
  assert.equal(installCommand('linux', "http://x/'", tok, false), '')
  assert.equal(installCommand('linux', 'http://x', '********', false), '')
  assert.equal(installCommand('windows', 'http://x', '', false), '')
})

test('Linux / macOS 命令经 sudo 安装，凭据放在请求体与环境变量中', () => {
  const c = installCommand('macos', 'https://g.example.com/', tok, false)
  assert.equal(
    c,
    `f=$(mktemp) && curl -fsSL -d 'token=${tok}' -o "$f" 'https://g.example.com/api/wol/public/agent/gately-agent.sh' && sudo GATELY_TOKEN='${tok}' sh "$f" install 'https://g.example.com'; rm -f "$f"`,
  )
  assert.ok(!c.includes('?token'))
  const k = installCommand('macos', 'https://10.0.0.2:16601', tok, true)
  assert.ok(k.startsWith('f=$(mktemp) && curl -fsSL -k '))
  assert.ok(k.includes(" install 'https://10.0.0.2:16601' --insecure; rm -f"))
  // Linux 没有 curl 时用 wget 下载
  const l = installCommand('linux', 'http://10.0.0.2:16601', tok, false)
  assert.ok(l.startsWith('f=$(mktemp) && if command -v curl >/dev/null; then curl -fsSL '))
  assert.ok(l.includes(`else wget -q --post-data='token=${tok}' -O "$f"`))
  assert.ok(l.endsWith(`sudo GATELY_TOKEN='${tok}' sh "$f" install 'http://10.0.0.2:16601'; rm -f "$f"`))
  // 不使用固定的 /tmp 路径
  for (const p of ['linux', 'macos', 'openwrt']) {
    assert.ok(!installCommand(p, 'http://x', tok, false).includes('/tmp/'), p)
  }
})

test('OpenWrt 命令不用 sudo，没有 curl 时用 wget', () => {
  const c = installCommand('openwrt', 'http://192.168.1.2:16601', tok, false)
  assert.ok(!c.includes('sudo'))
  assert.ok(c.includes(`wget -q --post-data='token=${tok}' -O "$f"`))
  assert.ok(c.includes(`; fi && GATELY_TOKEN='${tok}' sh "$f" install 'http://192.168.1.2:16601'`))
  const k = installCommand('openwrt', 'https://r.lan', tok, true)
  assert.ok(k.includes('curl -fsSL -k ') && k.includes('wget -q --no-check-certificate') && k.includes('--insecure; rm -f'))
})

test('Windows 命令下载 ps1 后以管理员 PowerShell 安装', () => {
  const c = installCommand('windows', 'https://g.example.com', tok, false)
  assert.ok(c.includes(`$env:GATELY_TOKEN = '${tok}'`))
  assert.ok(c.includes("'https://g.example.com/api/wol/public/agent/gately-agent.ps1' -OutFile \"$env:TEMP\\gately-agent.ps1\""))
  assert.ok(c.endsWith("install -Server 'https://g.example.com'"))
  assert.ok(c.includes('Tls12'))
  assert.ok(!c.includes('ServerCertificateValidationCallback'))
  const k = installCommand('windows', 'https://g.example.com', tok, true)
  assert.ok(k.includes('ServerCertificateValidationCallback') && k.endsWith(' -Insecure'))
})

test('scriptURL、uninstallCommand 与 guessPlatform', () => {
  assert.equal(scriptURL('http://x', 'windows'), 'http://x/api/wol/public/agent/gately-agent.ps1')
  assert.equal(scriptURL('http://x', 'openwrt'), 'http://x/api/wol/public/agent/gately-agent.sh')
  assert.ok(uninstallCommand('windows').includes('ProgramData\\GatelyAgent\\gately-agent.ps1" uninstall'))
  assert.equal(uninstallCommand('openwrt'), 'sh /usr/bin/gately-agent uninstall')
  assert.equal(uninstallCommand('linux'), 'sudo sh /usr/local/bin/gately-agent uninstall')
  assert.equal(guessPlatform('OpenWrt-24.10.6-x86_64'), 'openwrt')
  assert.equal(guessPlatform('macOS-15.1-arm64'), 'macos')
  assert.equal(guessPlatform('ubuntu-24.04-x86_64'), 'linux')
  assert.equal(guessPlatform('Windows-10.0.22631-AMD64'), 'windows')
  assert.equal(guessPlatform(undefined), 'windows')
})
