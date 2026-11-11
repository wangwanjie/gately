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

const sha = 'b'.repeat(64)

test('installCommand 地址、凭据或校验值无效时不生成命令', () => {
  assert.equal(installCommand('linux', "http://x/'", tok, false, sha), '')
  assert.equal(installCommand('linux', 'http://x', '********', false, sha), '')
  assert.equal(installCommand('windows', 'http://x', '', false, sha), '')
  assert.equal(installCommand('linux', 'http://x', tok, false, ''), '')
  assert.equal(installCommand('windows', 'http://x', tok, false, `${sha}'`), '')
  assert.equal(installCommand('macos', 'http://x', tok, false, 'B'.repeat(64)), '')
})

test('Linux / macOS 命令校验脚本后经 sudo 安装，凭据不出现在命令行参数中', () => {
  const c = installCommand('macos', 'https://g.example.com/', tok, false, sha)
  assert.equal(
    c,
    `(export GATELY_TOKEN='${tok}'; f=$(mktemp) && t=$(mktemp) && printf 'token=%s' "$GATELY_TOKEN" >"$t" && ` +
      `curl -fsSL -d @"$t" -o "$f" 'https://g.example.com/api/wol/public/agent/gately-agent.sh' && ` +
      `h=$( (sha256sum "$f" || shasum -a 256 "$f") 2>/dev/null) && { [ "\${h%% *}" = '${sha}' ] || { echo 'gately-agent: checksum mismatch' >&2; false; }; } && ` +
      `sudo --preserve-env=GATELY_TOKEN sh "$f" install 'https://g.example.com'; r=$?; rm -f "$f" "$t"; exit $r)`,
  )
  assert.ok(!c.includes('?token'))
  const k = installCommand('macos', 'https://10.0.0.2:16601', tok, true, sha)
  assert.ok(k.includes('curl -fsSL -k -d @"$t"'))
  assert.ok(k.includes(" install 'https://10.0.0.2:16601' --insecure; r=$?"))
  // Linux 没有 curl 时用 wget 下载
  const l = installCommand('linux', 'http://10.0.0.2:16601', tok, false, sha)
  assert.ok(l.includes('if command -v curl >/dev/null; then curl -fsSL -d @"$t" '))
  assert.ok(l.includes(`else wget -q --post-file="$t" -O "$f"`))
  assert.ok(l.includes(`sudo --preserve-env=GATELY_TOKEN sh "$f" install 'http://10.0.0.2:16601'; r=$?`))
  for (const p of ['linux', 'macos', 'openwrt']) {
    const cmd = installCommand(p, 'http://x', tok, false, sha)
    // 不使用固定的 /tmp 路径；凭据只出现在 export 中（内置命令，不产生带参数的进程）
    assert.ok(!cmd.includes('/tmp/'), p)
    assert.equal(cmd.split(tok).length, 2, p)
    assert.ok(!cmd.includes(`GATELY_TOKEN='${tok}' sh`), p)
    assert.ok(cmd.includes(`[ "\${h%% *}" = '${sha}' ]`), p)
  }
})

test('OpenWrt 命令不用 sudo，没有 curl 时用 wget', () => {
  const c = installCommand('openwrt', 'http://192.168.1.2:16601', tok, false, sha)
  assert.ok(!c.includes('sudo'))
  assert.ok(c.includes('wget -q --post-file="$t" -O "$f"'))
  assert.ok(c.includes(`; } && sh "$f" install 'http://192.168.1.2:16601'; r=$?`))
  const k = installCommand('openwrt', 'https://r.lan', tok, true, sha)
  assert.ok(k.includes('curl -fsSL -k ') && k.includes('wget -q --no-check-certificate') && k.includes('--insecure; r=$?'))
})

test('Windows 命令下载 ps1、校验后以管理员 PowerShell 安装', () => {
  const c = installCommand('windows', 'https://g.example.com', tok, false, sha)
  assert.ok(c.includes(`$env:GATELY_TOKEN = '${tok}'`))
  assert.ok(c.includes("'https://g.example.com/api/wol/public/agent/gately-agent.ps1' -OutFile \"$env:TEMP\\gately-agent.ps1\""))
  assert.ok(c.includes(`(Get-FileHash "$env:TEMP\\gately-agent.ps1" -Algorithm SHA256).Hash -ne '${sha}') { throw`))
  assert.ok(c.indexOf('Get-FileHash') < c.indexOf('powershell -NoProfile'))
  assert.ok(c.endsWith("install -Server 'https://g.example.com'"))
  assert.ok(c.includes('Tls12'))
  assert.ok(!c.includes('ServerCertificateValidationCallback'))
  const k = installCommand('windows', 'https://g.example.com', tok, true, sha)
  assert.ok(k.includes('ServerCertificateValidationCallback') && k.endsWith(' -Insecure'))
})

test('scriptURL、uninstallCommand 与 guessPlatform', () => {
  assert.equal(scriptURL('http://x', 'windows'), 'http://x/api/wol/public/agent/gately-agent.ps1')
  assert.equal(scriptURL('http://x', 'openwrt'), 'http://x/api/wol/public/agent/gately-agent.sh')
  assert.ok(uninstallCommand('windows').includes('ProgramData\\GatelyAgent\\gately-agent.ps1" uninstall'))
  assert.equal(uninstallCommand('openwrt'), 'sh /usr/bin/gately-agent uninstall')
  assert.equal(uninstallCommand('linux'), 'sudo sh /usr/local/bin/gately-agent uninstall')
  assert.equal(uninstallCommand('macos'), 'sudo sh /Library/Gately/gately-agent uninstall')
  assert.equal(guessPlatform('OpenWrt-24.10.6-x86_64'), 'openwrt')
  assert.equal(guessPlatform('macOS-15.1-arm64'), 'macos')
  assert.equal(guessPlatform('ubuntu-24.04-x86_64'), 'linux')
  assert.equal(guessPlatform('Windows-10.0.22631-AMD64'), 'windows')
  assert.equal(guessPlatform(undefined), 'windows')
})
