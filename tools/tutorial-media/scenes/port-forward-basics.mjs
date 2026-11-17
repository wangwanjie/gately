// 教程「端口转发入门」的截图：规则列表、空状态的场景模板、RDP 规则、端口段规则、高级设置、全局设置、规则卡片与运行日志
import { open, go, shot, mark, fakeWrites } from '../lib.mjs'

const RULES = '**/api/portforward/rules'

// 打开「添加规则」对话框
async function addRule(page) {
  await page.getByRole('button', { name: '添加规则' }).click()
  await page.waitForTimeout(600)
}

// 在对话框里按表单项标签找到输入框
function field(page, label) {
  return page.locator('.el-dialog .el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: new RegExp(`^${label}$`) }) })
}

// 目标地址输入框失焦时会误报 targetAddresses is required（表单项的 required 检查的是保存时才写入的数组），
// 截图前清掉这条校验提示，避免误导读者
async function clearFalseError(page) {
  await page.evaluate(() => {
    document.querySelectorAll('.el-dialog .el-form-item__error').forEach((el) => el.remove())
    document.querySelectorAll('.el-dialog .el-form-item.is-error').forEach((el) => el.classList.remove('is-error'))
  })
}

// 勾选或取消勾选协议
async function setProtocols(page, wanted) {
  for (const p of ['TCP', 'UDP', 'TCP4', 'UDP4', 'TCP6', 'UDP6']) {
    const box = page.locator('.el-dialog .el-checkbox').filter({ hasText: new RegExp(`^${p}$`) })
    const checked = await box.evaluate((el) => el.classList.contains('is-checked'))
    if (checked !== wanted.includes(p)) {
      await box.click()
    }
  }
}

// ---------- 1. 规则列表 ----------
{
  const { browser, page } = await open({ height: 900 })
  await go(page, 'portforward')
  await shot(page, 'port-forward-basics-list')
  await browser.close()
}

// ---------- 2. 空状态的场景模板与 RDP 规则 ----------
{
  const { browser, page } = await open({ height: 1100 })
  // 列表返回空，展示第一次使用时的场景模板
  await page.route(RULES, (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    }
    return route.continue()
  })
  await go(page, 'portforward')
  const tpl = page.locator('.tpl-empty')
  await mark(page, page.locator('[data-tpl=rdp]'))
  await shot(page, 'port-forward-basics-templates', tpl.locator('xpath=ancestor::*[contains(@class,"el-empty") or contains(@class,"grouped")][1]').or(tpl).first(), { pad: 40 })
  await page.locator('[data-tpl=rdp]').click()
  await page.waitForTimeout(600)
  await field(page, '目标地址').locator('textarea').fill('192.168.1.20')
  await page.locator('.el-dialog .el-dialog__title').click()
  await page.waitForTimeout(300)
  await clearFalseError(page)
  await mark(page, field(page, '目标地址').locator('textarea'))
  await shot(page, 'port-forward-basics-rdp', '.el-dialog')
  await browser.close()
}

// ---------- 3. 端口段与 UDP 游戏服务器 ----------
{
  const { browser, page } = await open({ height: 1100 })
  await go(page, 'portforward')
  await addRule(page)
  await field(page, '名称').locator('input').fill('游戏服务器')
  await setProtocols(page, ['UDP'])
  await field(page, '监听端口').locator('input').fill('27015-27020')
  await field(page, '目标地址').locator('textarea').fill('192.168.1.30')
  await field(page, '目标端口').locator('input').fill('')
  await page.locator('.el-dialog .el-dialog__title').click()
  await page.waitForTimeout(300)
  await clearFalseError(page)
  await mark(page, field(page, '监听端口').locator('input'))
  await shot(page, 'port-forward-basics-range', '.el-dialog')
  await browser.close()
}

// ---------- 4. 高级设置：主备目标、防火墙、连接数、PROXY 协议、白名单 ----------
{
  const { browser, page } = await open({ height: 2300 })
  await go(page, 'portforward')
  await addRule(page)
  await field(page, '名称').locator('input').fill('NAS SSH')
  await field(page, '监听端口').locator('input').fill('2222')
  await field(page, '目标地址').locator('textarea').fill('192.168.1.10\n192.168.1.11')
  await field(page, '目标端口').locator('input').fill('22')
  await page.getByText('高级设置').click()
  await page.waitForTimeout(400)
  await field(page, '自动放行防火墙').locator('.el-switch').click()
  await field(page, '最大连接数').locator('input').fill('64')
  await field(page, '单来源最大连接数').locator('input').fill('8')
  await page.locator('.el-dialog .el-radio-button').filter({ hasText: '白名单' }).click()
  await page.waitForTimeout(300)
  await field(page, 'IP / 网段').locator('textarea').fill('192.168.1.0/24\n198.51.100.0/24\n2001:db8:1::/48')
  await field(page, '备注').locator('input').fill('只允许家里和公司出口')
  await page.locator('.el-dialog .el-dialog__title').click()
  await page.waitForTimeout(300)
  await clearFalseError(page)
  await mark(page, field(page, '目标地址').locator('textarea'))
  await mark(page, page.locator('.el-dialog .el-radio-group').first())
  await shot(page, 'port-forward-basics-advanced', '.el-dialog')
  await browser.close()
}

// ---------- 5. 全局设置 ----------
{
  const { browser, page } = await open({ height: 1100 })
  await go(page, 'portforward')
  await page.getByRole('button', { name: '全局设置' }).click()
  await page.waitForTimeout(700)
  await shot(page, 'port-forward-basics-settings', page.locator('.el-dialog').filter({ hasText: '端口转发全局设置' }))
  await browser.close()
}

// ---------- 6. 规则卡片的统计与运行日志 ----------
{
  const { browser, page } = await open({ height: 1000 })
  await fakeWrites(page)
  const now = Date.now()
  const at = (s) => new Date(now - s * 1000).toISOString()
  const entries = [
    { level: 'info', source: 'NAS SSH', message: 'tcp :2222 -> 192.168.1.10|192.168.1.11:22 已启动', t: 600 },
    { level: 'info', source: '游戏服务器', message: 'udp :27015 -> 192.168.1.30:27015 已启动', t: 600 },
    { level: 'debug', source: 'NAS SSH', message: '198.51.100.23:50412 -> 192.168.1.10:22 已连接', t: 420 },
    { level: 'debug', source: 'NAS SSH', message: '203.0.113.77:41822 不满足来源 IP 访问控制，已断开', t: 300 },
    { level: 'warn', source: 'NAS SSH', message: '198.51.100.23:50430 连接目标失败: dial tcp 192.168.1.11:22: connect: connection refused', t: 180 },
    { level: 'debug', source: 'NAS SSH', message: '198.51.100.23:50431 -> 192.168.1.10:22 已连接', t: 179 },
    { level: 'error', source: 'Windows 远程桌面', message: 'tcp :3389 监听失败: listen tcp :3389: bind: address already in use', t: 60 },
  ].map((e, i) => ({ seq: i + 1, time: at(e.t), level: e.level, source: e.source, message: e.message }))
  await page.route('**/api/portforward/logs**', (route) => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/sources')) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(['NAS SSH', '游戏服务器', 'Windows 远程桌面']) })
    }
    const since = Number(url.searchParams.get('since') ?? 0)
    const before = url.searchParams.get('before')
    const list = before ? [] : entries.filter((e) => e.seq > since)
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(list) })
  })
  await go(page, 'portforward')
  const card = page.locator('.entity').filter({ hasText: 'SSH 跳板' }).first()
  await mark(page, card.locator('.field').filter({ has: page.locator('.field-label', { hasText: /^连接$/ }) }))
  await mark(page, card.locator('.field').filter({ has: page.locator('.field-label', { hasText: /^已拒绝$/ }) }))
  await shot(page, 'port-forward-basics-card', card)
  const log = page.locator('.log-card').first()
  await log.scrollIntoViewIfNeeded()
  await page.waitForTimeout(1500)
  await shot(page, 'port-forward-basics-logs', log)
  await browser.close()
}
