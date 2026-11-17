// 教程「OIDC 单点登录」的截图：共享用户资料、OIDC 身份提供方、添加客户端、客户端列表、站点使用共享用户
import { open, go, shot, mark } from '../lib.mjs'

const { browser, page } = await open()
// 0. 站点「安全」页：网页认证 + 使用共享用户，与 OIDC 共用账号
await go(page, 'web')
const row = page.locator('.site').filter({ hasText: 'grafana.example.com' }).first()
await row.locator('.site-actions').getByRole('button', { name: '编辑' }).click()
const sd = page.locator('.el-dialog:visible').last()
await sd.waitFor()
await sd.getByRole('tab', { name: '安全' }).click()
await page.waitForTimeout(500)
// 只保留认证相关的几项，其余表单项临时隐藏，截出紧凑的一块
const keep = ['认证', '共享用户', '用户', '第三方登录']
await sd.locator('.el-tab-pane:visible .el-form-item').evaluateAll((els, k) => {
  for (const e of els) {
    const label = e.querySelector(':scope > .el-form-item__label')?.innerText.trim() || ''
    if (!k.includes(label)) {
      e.style.display = 'none'
    }
  }
}, keep)
await mark(page, sd.getByText('使用共享用户（支持 TOTP 与单点登录）'))
await page.waitForTimeout(300)
await shot(page, 'oidc-sso-site-auth', sd.locator('.el-tab-pane:visible').first())


await go(page, 'webusers')

const usersCard = page.locator('.g-card').filter({ hasText: '共享用户' }).first()
const oidcCard = page.locator('.g-card').filter({ hasText: 'OIDC 身份提供方' }).last()

// 1. 编辑共享用户：邮箱、显示名、分组会作为 OIDC 声明提供给应用
await usersCard.locator('.item').filter({ hasText: 'alice' }).getByRole('button', { name: '编辑' }).click()
let dlg = page.locator('.el-dialog').filter({ hasText: '编辑共享用户' })
await dlg.waitFor()
// 分组选择器里再加一个 admins，演示「管理员分组」
const groupSelect = dlg.locator('.el-form-item').filter({ hasText: '分组' }).locator('.el-select')
await groupSelect.click()
await page.keyboard.type('family')
await page.keyboard.press('Enter')
await page.keyboard.type('admins')
await page.keyboard.press('Enter')
await page.keyboard.press('Escape')
await page.waitForTimeout(300)
await dlg.locator('.el-dialog__header').click()
await mark(page, dlg.locator('.el-form-item').filter({ hasText: '邮箱' }).first())
await mark(page, dlg.locator('.el-form-item').filter({ hasText: '分组' }).first())
await page.waitForTimeout(300)
await shot(page, 'oidc-sso-user', dlg.locator('.el-dialog').or(dlg).first())
await dlg.getByRole('button', { name: '确定' }).click()
await page.waitForTimeout(400)

// 2. 单点登录域
const ssoInput = usersCard.locator('.sub-form').getByPlaceholder('example.com')
await ssoInput.fill('example.com')
await mark(page, usersCard.locator('.sub-form .el-input').first())
await shot(page, 'oidc-sso-domain', usersCard.locator('.sub-form'))

// 3. 启用 OIDC、填签发者地址，展开端点列表
await oidcCard.locator('.el-switch').first().click()
await oidcCard.getByPlaceholder('https://auth.example.com').fill('https://auth.example.com')
await page.waitForTimeout(300)
await oidcCard.getByText('需要手动填写端点时').click()
await page.waitForTimeout(600)
await shot(page, 'oidc-sso-provider', oidcCard)

// 4. 添加 Gitea 客户端
async function addClient({ name, id, redirect, allow }) {
  await oidcCard.getByRole('button', { name: '添加客户端' }).click()
  const d = page.locator('.el-dialog').filter({ hasText: 'Client ID' }).last()
  await d.waitFor()
  await d.getByPlaceholder('名称，如 Gitea').fill(name)
  await d.getByPlaceholder('例如 gitea').fill(id)
  await d.getByRole('button', { name: '生成' }).click()
  await d.locator('textarea').nth(0).fill(redirect)
  await d.locator('textarea').nth(1).fill(allow)
  return d
}
dlg = await addClient({
  name: 'Gitea',
  id: 'gitea',
  redirect: 'https://git.example.com/user/oauth2/gately/callback',
  allow: 'group:family\ngroup:admins'
})
await page.waitForTimeout(300)
await mark(page, dlg.getByRole('button', { name: '生成' }))
await shot(page, 'oidc-sso-client', dlg)
await dlg.getByRole('button', { name: '确定' }).click()
await page.waitForTimeout(400)

// 5. 再加 Grafana 与 OpenList，截客户端列表
dlg = await addClient({ name: 'Grafana', id: 'grafana', redirect: 'https://grafana.example.com/login/generic_oauth', allow: '' })
await dlg.getByRole('button', { name: '确定' }).click()
await page.waitForTimeout(300)
dlg = await addClient({
  name: 'OpenList',
  id: 'openlist',
  redirect: 'https://pan.example.com/api/auth/sso_callback?method=sso_get_token\nhttps://pan.example.com/api/auth/sso_callback?method=sso_get_ssoid',
  allow: 'alice\nbob'
})
await dlg.getByRole('button', { name: '确定' }).click()
await page.waitForTimeout(400)
// 底部保存栏会盖住列表，截图时临时隐藏
await page.addStyleTag({ content: '.save-bar{visibility:hidden!important}' })
await shot(page, 'oidc-sso-clients', oidcCard.locator('.el-form-item').filter({ hasText: '客户端' }).last())

await browser.close()
