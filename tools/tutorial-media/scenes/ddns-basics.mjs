// 教程「动态域名入门」的截图：空列表的常见场景模板、任务基本设置、解析记录、IP 来源（IPv6 用网卡）、
// Webhook、任务卡片与运行日志。空列表与运行日志为模拟数据，保存请求由 fakeWrites 拦截。
import { BASE, open, go, shot, mark, fakeWrites } from '../lib.mjs'

const { browser, page } = await open({ height: 1100 })

async function fresh(route) {
  await page.goto('about:blank')
  await go(page, route)
}

// fakeWrites 先注册：Playwright 按注册的倒序匹配路由，下面模拟 GET 的处理器先于它执行
await fakeWrites(page)

let emptyList = true
await page.route('**/api/ddns/tasks', async (route) => {
  if (route.request().method() !== 'GET' || !emptyList) {
    return route.fallback()
  }
  await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
})

// 示例运行日志：只在首次加载时返回
const now = Date.now()
const src = 'Cloudflare：example.com'
const logLines = [
  ['info', 'IPv4 地址: 203.0.113.7 -> 203.0.113.10'],
  ['info', 'A example.com 已更新 -> 203.0.113.10'],
  ['info', 'A blog.example.com 已更新 -> 203.0.113.10'],
  ['info', 'Webhook 调用成功'],
  ['info', 'IPv6 地址: 2001:db8::7 -> 2001:db8::10'],
  ['info', 'AAAA home.example.com 已更新 -> 2001:db8::10'],
  ['info', 'Webhook 调用成功']
]
const entries = logLines.map(([level, message], i) => ({
  seq: 500 + i,
  time: new Date(now - (logLines.length - i) * 41000).toISOString(),
  level,
  source: src,
  message
}))
await page.route('**/api/ddns/logs?*', async (route) => {
  const u = new URL(route.request().url())
  const first = u.searchParams.get('since') === '0' && !u.searchParams.has('before')
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(first ? entries : []) })
})

const dlg = () => page.locator('.el-dialog:visible').last()
const tab = async (name) => {
  await dlg().getByRole('tab', { name }).click()
  await page.waitForTimeout(400)
}

// 1. 空列表：从常见场景开始
{
  await fresh('ddns')
  const empty = page.locator('.tpl-empty').first()
  await empty.waitFor()
  await mark(page, page.locator('[data-tpl=cloudflare4]'))
  await shot(page, 'ddns-basics-templates', page.locator('.el-empty, .tpl-empty').first())
}

// 2. 基本：点「Cloudflare 家宽 IPv4」后预填的表单，填任务名称与令牌
{
  await page.locator('[data-tpl=cloudflare4]').click()
  await dlg().waitFor()
  await dlg().getByLabel('任务名称').fill('家里的宽带')
  await dlg().locator('.el-form-item', { hasText: 'API Token' }).locator('input').first().fill('cf-token-for-gately-example')
  await dlg().locator('.el-form-item', { hasText: '代理状态' }).locator('input').first().fill('false')
  await page.waitForTimeout(300)
  await shot(page, 'ddns-basics-task', dlg())
}

// 3. 解析记录：根域名与 nas 两条 A 记录，再加一条 AAAA
{
  await tab('解析记录')
  const rows = () => dlg().locator('.rec-edit')
  const fillRow = async (i, host, zone) => {
    const r = rows().nth(i)
    const inputs = r.locator('.el-input input')
    await inputs.nth(0).fill(host)
    await inputs.nth(1).fill(zone)
  }
  if (await rows().count() === 0) {
    await dlg().getByRole('button', { name: 'IPv4 记录' }).click()
  }
  await fillRow(0, '@', 'example.com')
  await dlg().getByRole('button', { name: 'IPv4 记录' }).click()
  await page.waitForTimeout(200)
  await fillRow(1, 'nas', 'example.com')
  await page.waitForTimeout(300)
  await shot(page, 'ddns-basics-records', dlg())
}

// 4. IP 来源：IPv4 用查询接口，IPv6 用网卡
{
  await tab('IP 来源')
  await dlg().locator('.el-form-item', { hasText: '获取 IPv6' }).locator('.el-switch').click()
  await page.waitForTimeout(400)
  const v6Method = dlg().locator('.el-radio-group').nth(1)
  await v6Method.getByText('网卡').click()
  await page.waitForTimeout(300)
  const sel = dlg().locator('.el-select').last()
  await sel.click()
  await page.waitForTimeout(300)
  const opt = page.locator('.el-select-dropdown__item:visible').first()
  if (await opt.count()) {
    await opt.click()
  } else {
    await page.keyboard.press('Escape')
  }
  await page.waitForTimeout(300)
  await shot(page, 'ddns-basics-ip-source', dlg())
}

// 5. Webhook：IP 变化或同步失败时调用
{
  await tab('Webhook')
  await dlg().locator('.el-form-item', { hasText: '启用 Webhook' }).locator('.el-switch').click()
  await page.waitForTimeout(400)
  await dlg().locator('.el-form-item', { hasText: '请求地址' }).locator('input').first().fill('https://ntfy.example.net/home-ddns')
  // 请求方法改为 POST 后才显示「请求体」
  await dlg().locator('.el-form-item', { hasText: '请求方法' }).locator('.el-select').click()
  await page.waitForTimeout(300)
  await page.locator('.el-select-dropdown__item:visible', { hasText: /^POST$/ }).first().click()
  await page.waitForTimeout(300)
  const body = dlg().locator('.el-form-item', { hasText: '请求体' }).locator('textarea').first()
  if (await body.count()) {
    await body.fill('{taskName}：IP 已变为 {ipAddr}\n更新成功：{successDomains}\n更新失败：{failedDomains}')
  }
  await page.waitForTimeout(300)
  await shot(page, 'ddns-basics-webhook', dlg())
  await page.keyboard.press('Escape')
}

// 6. 任务卡片：记录状态与「检查」「强制同步」
{
  emptyList = false
  await fresh('ddns')
  const card = page.locator('.entity').filter({ hasText: 'Cloudflare：example.com' }).first()
  await card.waitFor()
  await mark(page, card.getByRole('button', { name: '强制同步' }))
  await shot(page, 'ddns-basics-card', card)
}

// 7. 运行日志
{
  const log = page.locator('.log-card').first()
  await log.getByText('Webhook 调用成功').first().waitFor()
  await page.waitForTimeout(500)
  await shot(page, 'ddns-basics-logs', log)
}

await browser.close()
