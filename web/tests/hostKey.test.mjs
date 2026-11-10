// 主机密钥类型显示名的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { hostKeyTypeLabel } from '../src/components/terminal/hostKey.ts'

test('普通主机密钥按 ssh-keygen -l 的写法显示', () => {
  assert.equal(hostKeyTypeLabel('ssh-ed25519'), 'ED25519')
  assert.equal(hostKeyTypeLabel('ssh-rsa'), 'RSA')
  assert.equal(hostKeyTypeLabel('ecdsa-sha2-nistp256'), 'ECDSA')
  assert.equal(hostKeyTypeLabel(''), '')
})

test('主机证书显示短名加 -CERT，未知类型原样显示', () => {
  assert.equal(hostKeyTypeLabel('ssh-ed25519-cert-v01@openssh.com'), 'ED25519-CERT')
  assert.equal(hostKeyTypeLabel('ssh-rsa-cert-v01@openssh.com'), 'RSA-CERT')
  assert.equal(hostKeyTypeLabel('ecdsa-sha2-nistp384-cert-v01@openssh.com'), 'ECDSA-CERT')
  assert.equal(hostKeyTypeLabel('foo-cert-v01@openssh.com'), 'foo-cert-v01@openssh.com')
  assert.equal(hostKeyTypeLabel('ssh-unknown'), 'ssh-unknown')
})
