// 教程「API、命令行与 MCP 自动化」截图。
// 演示实例没有 API 令牌，且拒绝创建；令牌列表与「令牌仅显示这一次」弹窗用示例数据（page.route）展示真实界面。
import { open, go, shot, mark } from '../lib.mjs'

const { browser, page } = await open({ height: 1000 })
const hideToast = () => page.addStyleTag({ content: '.el-message{display:none!important}' })

const now = Date.now()
const iso = (min) => new Date(now + min * 60000).toISOString()
const secret = 'gtl_' + '3f9a1c7e5b2d4086a1e9c3b7f5d2a8e4c6b0d9f1a3e5c7b9'
const tk = (id, name, scopes, opts = {}) => ({ id, name, hint: 'gtl_' + id.slice(0, 4) + '…' + id.slice(-4), scopes, legacy: false, disabled: false, expired: false, createdAt: iso(-60 * 24 * 10), expiresAt: null, lastUsedAt: null, lastUsedIP: '', ...opts })
const tokens = [
  tk('3f9a77c1', 'DDNS 与证书脚本', ['ddns:write', 'ssl:write', 'webservice:read', 'status:read'], { expiresAt: iso(60 * 24 * 90), lastUsedAt: iso(-3), lastUsedIP: '192.168.1.30', createdAt: iso(-5) }),
  tk('9c02e5aa', 'Prometheus 抓取', ['metrics:read'], { lastUsedAt: iso(-1), lastUsedIP: '192.168.1.40' }),
  tk('51bd03f8', 'AI 助手（只读）', ['*:read'], { expiresAt: iso(60 * 24 * 30), lastUsedAt: iso(-60 * 5), lastUsedIP: '127.0.0.1' }),
]
let created = false
await page.route('**/api/tokens', (r) => {
  if (r.request().method() === 'POST') {
    created = true
    return r.fulfill({ json: { token: tokens[0], secret } })
  }
  return r.fulfill({ json: created ? tokens : [] })
})

// 1. 新建令牌：名称、过期时间与权限
await go(page, 'api-tokens')
await hideToast()
await page.getByRole('button', { name: '新建令牌' }).first().click()
await page.waitForTimeout(800)
const dlg = page.locator('.el-dialog:visible').last()
await dlg.getByPlaceholder('例如：监控脚本、CI 部署').fill('DDNS 与证书脚本')
await dlg.locator('.el-form-item', { hasText: '过期时间' }).locator('.el-select').click()
await page.waitForTimeout(300)
await page.locator('.el-select-dropdown__item:visible', { hasText: '90 天' }).click()
await page.waitForTimeout(300)
const pick = async (id, level) => {
  const row = dlg.locator('div', { has: page.locator('code, .mono, span', { hasText: new RegExp('^' + id + '$') }) }).filter({ has: page.locator('.el-radio-button, .el-segmented, label') }).last()
  await row.getByText(level, { exact: true }).click()
}
await pick('ddns', '读写')
await pick('ssl', '读写')
await pick('webservice', '只读')
await pick('status', '只读')
await page.waitForTimeout(300)
// 弹窗内容较长，截取名称、过期时间到前几个模块的部分
await dlg.locator('.el-dialog__body').evaluate((el) => { el.scrollTop = 0 })
await shot(page, 'api-automation-create', dlg)

// 2. 创建后只显示一次的令牌明文
await dlg.getByRole('button', { name: '新建', exact: true }).click()
await page.waitForTimeout(1000)
await shot(page, 'api-automation-secret', page.locator('.el-dialog:visible').last())
await page.getByRole('button', { name: '我已保存' }).click()
await page.waitForTimeout(600)

// 3. 令牌列表
await page.mouse.move(5, 900)
await shot(page, 'api-automation-tokens')

// 4. API 文档：搜索并展开「立即同步一个 DDNS 任务」
await go(page, 'api-docs')
await hideToast()
await page.getByPlaceholder('搜索路径或摘要').fill('/sync')
await page.waitForTimeout(600)
const item = page.getByText('立即同步一个 DDNS 任务').first()
await item.click()
await page.waitForTimeout(600)
await shot(page, 'api-automation-docs')
await browser.close()
