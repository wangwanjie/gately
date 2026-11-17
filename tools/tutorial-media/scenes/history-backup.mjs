// 教程「配置历史、备份与迁移」截图。
// 演示实例的配置历史只有一个版本，配置历史列表与差异用示例数据（page.route 拦截 /api/history）展示真实界面。
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { open, go, shot, mark, fakeWrites } from '../lib.mjs'

// 恢复卡片需要选一个备份文件：在临时目录放一个空 zip，只用来显示文件名，不会上传
const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'tut-hb-'))
const dummyZip = path.join(tmpDir, 'gately-backup-20261117-030000.zip')
fs.writeFileSync(dummyZip, Buffer.from([0x50, 0x4b, 0x05, 0x06, ...new Array(18).fill(0)]))

const { browser, page } = await open({ height: 1000 })
// 演示模式下部分后台请求被拒绝，会弹出「演示模式下不可用」提示，截图时隐藏
const hideToast = () => page.addStyleTag({ content: '.el-message{display:none!important}' })

const ago = (min) => new Date(Date.now() - min * 60000).toISOString()
const id = (t) => t.replace(/[-:]/g, '').replace(/\.(\d{3})Z$/, '.$1000000Z')
const versions = (mins, sizes) => mins.map((m, i) => { const t = ago(m); return { id: id(t), time: t, size: sizes[i] } })
const history = [
  { name: 'ddns', title: '动态域名', versions: versions([2880, 9000], [1460, 1388]) },
  { name: 'gately', versions: versions([60 * 24 * 5], [612]) },
  { name: 'guard', title: '入侵防护', versions: versions([4320], [2210]) },
  { name: 'ssl', title: 'SSL 证书', versions: versions([1440, 20160], [8120, 8096]) },
  { name: 'webservice', title: 'Web 服务', versions: versions([12, 95, 1500, 4400, 10080], [18432, 18410, 18120, 17650, 16890]) },
]
const diff = { lines: [
  { op: '@', text: '214' },
  { op: ' ', text: '          "name": "Jellyfin",' },
  { op: ' ', text: '          "domains": [' },
  { op: ' ', text: '            "jellyfin.example.com"' },
  { op: ' ', text: '          ],' },
  { op: ' ', text: '          "targets": [' },
  { op: '-', text: '            "http://192.168.1.20:8096"' },
  { op: '+', text: '            "http://192.168.1.21:8096"' },
  { op: ' ', text: '          ],' },
  { op: '-', text: '          "webAuth": false,' },
  { op: '+', text: '          "webAuth": true,' },
  { op: ' ', text: '          "waf": "",' },
  { op: '@', text: '388' },
] }
await page.route('**/api/history', (r) => r.fulfill({ json: history }))
// 定时备份：示例的挂载点、目的地、最近一次结果与本地备份文件（演示实例没有存储挂载）
const day = (n, h = 3) => { const d = new Date(); d.setDate(d.getDate() - n); d.setHours(h, 0, 2, 0); return d }
const stamp = (d) => `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}-030002`
const mounts = [{ key: 'm-nas', name: 'NAS', type: 'webdav', enable: true, readOnly: false }, { key: 'm-smb', name: '旧电脑共享', type: 'smb', enable: true, readOnly: false }]
const targets = [{ key: 't-nas', name: 'NAS 备份', enable: true, mount: 'm-nas', dir: 'gately-backups' }]
const status = { running: false, manual: false, lastRun: day(0).toISOString(), nextRun: day(-1).toISOString(), results: [
  { target: '本地目录 (backups)', ok: true, file: `gately-backup-${stamp(day(0))}.zip.enc`, pruned: 1 },
  { target: 'NAS 备份', ok: true, file: `gately-backup-${stamp(day(0))}.zip.enc`, pruned: 1 },
] }
const local = [0, 1, 2].map((n) => ({ name: `gately-backup-${stamp(day(n))}.zip.enc`, size: 182000 - n * 900, modTime: day(n).toISOString() }))
await page.route('**/api/backup/settings', (r) => r.request().method() === 'GET' ? r.fulfill({ json: { enable: true, schedule: '0 3 * * *', keep: 7, localEnable: true, password: '********', hasPassword: true } }) : r.fallback())
await page.route('**/api/backup/mounts', (r) => r.fulfill({ json: mounts }))
await page.route('**/api/backup/targets', (r) => r.request().method() === 'GET' ? r.fulfill({ json: targets }) : r.fallback())
await page.route('**/api/backup/status', (r) => r.fulfill({ json: status }))
await page.route('**/api/backup/local', (r) => r.fulfill({ json: local }))
await page.route('**/api/history/*/*/diff', (r) => r.fulfill({ json: diff }))

// 1. 配置历史列表
await go(page, 'logs/history')
await page.locator('.f-name').click()
await page.waitForTimeout(300)
await page.locator('.el-select-dropdown__item:visible', { hasText: 'webservice.json' }).click()
await page.waitForTimeout(500)
await shot(page, 'history-backup-history')

// 2. 对比差异
await page.getByRole('button', { name: '对比' }).nth(1).click()
await page.waitForTimeout(800)
await shot(page, 'history-backup-diff', page.locator('.el-dialog:visible').last())
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 3. 回滚确认（只打开确认框，点取消）
await page.getByRole('button', { name: '回滚' }).nth(1).click()
await page.waitForTimeout(600)
await shot(page, 'history-backup-rollback', page.locator('.el-message-box:visible'))
await page.getByRole('button', { name: '取消' }).click()
await page.waitForTimeout(400)

// 4. 备份与恢复卡片
await go(page, 'settings?tab=backup')
await hideToast()
const backupCard = page.locator('.g-card', { hasText: '备份 Gately 配置' }).first()
const restoreCard = page.locator('.g-card', { hasText: '恢复配置' }).first()
await restoreCard.locator('input[type=file]').setInputFiles(dummyZip)
await page.waitForTimeout(400)
await restoreCard.locator('.el-checkbox', { hasText: '保留当前管理员账号' }).click()
await mark(page, backupCard.getByRole('button', { name: '下载备份' }))
await shot(page, 'history-backup-manual', page.locator('.g-card', { hasText: '备份 Gately 配置' }).locator('xpath=..'))

// 5. 恢复确认框
await restoreCard.getByRole('button', { name: '恢复', exact: true }).click()
await page.waitForTimeout(600)
await page.locator('.el-message-box:visible input').fill('恢复')
await shot(page, 'history-backup-restore', page.locator('.el-message-box:visible'))
await page.getByRole('button', { name: '取消' }).click()
await page.waitForTimeout(400)

// 6. 定时备份
const sched = page.locator('.bk').first()
// 开关会立即保存，演示实例拒绝写入，这里把写请求当作成功
await fakeWrites(page)
await page.waitForTimeout(300)
await shot(page, 'history-backup-schedule', sched)

// 7. 添加远程目的地
await sched.getByRole('button', { name: '添加' }).click()
await page.waitForTimeout(600)
const td = page.locator('.el-dialog:visible').last()
await td.locator('input').first().fill('NAS 备份')
await td.locator('.el-select').first().click()
await page.waitForTimeout(400)
await page.locator('.el-select-dropdown__item:visible', { hasText: 'NAS' }).first().click()
await page.waitForTimeout(300)
await td.getByPlaceholder('如 gately-backups，留空为根目录').fill('gately-backups')
await shot(page, 'history-backup-target', td)
await page.keyboard.press('Escape')

// 8. 诊断包
await go(page, 'settings?tab=diagnostics')
await hideToast()
await shot(page, 'history-backup-diagnostics', page.locator('.el-card, .g-card', { hasText: '导出诊断包' }).last())
await browser.close()
fs.rmSync(tmpDir, { recursive: true, force: true })
