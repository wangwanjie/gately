// 教程「端口敲门」的截图：运行状态与授权、添加规则、fwknop 客户端配置、全局设置、
// 端口转发的“需要敲门”、协议分流的敲门路由、站点的敲门准入与登录后放行
import { open, go, shot, mark } from '../lib.mjs'

// 视口加高，让较长的对话框一次截全
const { browser, page } = await open({ height: 1600 })

const field = (dlg, label) => dlg.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: new RegExp('^' + label + '$') }) }).first()

// 1. 端口敲门页：运行状态与当前授权
await go(page, 'knock')
await page.waitForTimeout(600)
await page.setViewportSize({ width: 1440, height: 1050 })
await page.waitForTimeout(400)
await shot(page, 'port-knocking-status')
await page.setViewportSize({ width: 1440, height: 1600 })

// 2. 添加规则：保护本机 SSH
await page.getByRole('button', { name: '添加规则' }).first().click()
let dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('SSH 敲门')
await field(dlg, '端口').locator('input').fill('tcp/22')
await field(dlg, '防火墙拦截').locator('.el-switch').click()
await field(dlg, '要求写明地址').locator('.el-switch').click()
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await mark(page, field(dlg, '防火墙拦截'))
await shot(page, 'port-knocking-rule', dlg)
await page.keyboard.press('Escape')
await page.waitForTimeout(400)

// 3. 客户端配置：生成 fwknop 命令与 ~/.fwknoprc 配置段
await page.getByRole('button', { name: '客户端' }).first().click()
dlg = page.locator('.el-dialog:visible').filter({ hasText: 'fwknop 客户端配置' }).first()
await dlg.waitFor()
await page.waitForTimeout(800)
await field(dlg, '服务器地址').locator('input').fill('ssh.example.com')
await page.waitForTimeout(400)
await shot(page, 'port-knocking-client', dlg)
await page.keyboard.press('Escape')
await page.waitForTimeout(400)

// 4. 全局设置
await page.getByRole('button', { name: '全局设置' }).first().click()
dlg = page.locator('.el-dialog:visible').filter({ hasText: '端口敲门设置' }).first()
await dlg.waitFor()
await page.waitForTimeout(400)
await shot(page, 'port-knocking-settings', dlg)
await page.keyboard.press('Escape')

// 5. 端口转发规则：IP 访问控制中选择“需要敲门”
await go(page, 'portforward')
await page.getByRole('button', { name: '添加规则' }).first().click()
dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('SSH 外网入口')
await field(dlg, '监听端口').locator('input').fill('2222')
// 目标地址是多行列表，逐字输入并移开焦点，让表单校验拿到新值
await field(dlg, '目标地址').locator('textarea').first().click()
await page.keyboard.type('192.168.1.10')
await page.keyboard.press('Tab')
await field(dlg, '目标端口').locator('input').fill('22')
await dlg.getByText('高级设置').click()
await page.waitForTimeout(400)
await field(dlg, '需要敲门').locator('.el-select').click()
await page.locator('.el-select-dropdown:visible .el-select-dropdown__item', { hasText: 'SSH 敲门' }).first().click()
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
// 目标地址框失焦时界面会误报英文的 targetAddresses is required（保存时不受影响），截图前去掉这条提示
await field(dlg, '目标地址').evaluate((el) => {
  el.classList.remove('is-error')
  el.querySelector('.el-form-item__error')?.remove()
})
await mark(page, field(dlg, '需要敲门'))
await shot(page, 'port-knocking-portforward', dlg)
await page.keyboard.press('Escape')

// 6. 协议分流：443 上 HTTPS 对所有人开放，敲门路由接收 fwknop -P tcp，SSH 路由需要敲门
await go(page, 'protoroute')
await page.getByRole('button', { name: '添加规则' }).first().click()
dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('443 入口')
await field(dlg, '监听端口').locator('input').fill('443')
const card = (n) => dlg.locator('.route-card').nth(n)
const pick = async (sel, text) => {
  await sel.click()
  await page.locator('.el-select-dropdown:visible .el-select-dropdown__item', { hasText: text }).first().click()
  await page.waitForTimeout(250)
}
await pick(card(0).locator('.route-proto'), '端口敲门')
await card(0).locator('.route-title input').fill('knock')
await dlg.getByRole('button', { name: '添加路由' }).click()
await page.waitForTimeout(300)
await pick(card(1).locator('.route-proto'), /^SSH$/)
await card(1).locator('.route-title input').fill('ssh')
await card(1).locator('input[placeholder^="目标地址"]').fill('127.0.0.1')
await card(1).locator('input[placeholder="目标端口"]').fill('22')
await pick(card(1).locator('.el-select').last(), 'SSH 敲门')
await field(dlg, '默认目标').locator('input, textarea').first().fill('127.0.0.1')
await field(dlg, '目标端口').locator('input').first().fill('8443')
await pick(field(dlg, '默认目标 PROXY').locator('.el-select'), 'v2')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await mark(page, card(0))
await mark(page, card(1))
await shot(page, 'port-knocking-protoroute', dlg)
await page.keyboard.press('Escape')

// 7. 站点「安全」页：敲门准入与登录后放行
await go(page, 'web')
await page.locator('.site').filter({ hasText: 'photos.example.com' }).first().getByRole('button', { name: '编辑' }).click()
dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(500)
await dlg.getByRole('tab', { name: '安全' }).click()
await page.waitForTimeout(500)
// 选择放行规则后出现放行时长、IPv6 放行范围、双栈探测与放行页面
await pick(field(dlg, '登录后放行').locator('.el-select').first(), 'SSH 敲门')
await page.waitForTimeout(400)
await shot(page, 'port-knocking-site', dlg.locator('.site-knock'))
await browser.close()
