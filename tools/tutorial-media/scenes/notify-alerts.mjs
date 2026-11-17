// 教程「通知与告警」的截图：通知中心的渠道、规则与投递记录，性能监控的告警规则。
// 运行：在 tools/tutorial-media 目录执行 node scenes/notify-alerts.mjs
import { open, go, shot, mark, fakeWrites } from '../lib.mjs'

const S = 'notify-alerts'
const { browser, page } = await open({ height: 1100 })

// 同一路由内跳转不会重新加载页面，上一步打开的对话框还在：跳转后刷新，每张图从干净的页面开始
async function reset(route) {
  await go(page, route)
  await page.reload()
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(800)
}

// 在当前对话框里选下拉框的某一项：label 为表单项名称，option 为选项文字
async function pick(dialog, label, option) {
  await dialog.locator('.el-form-item').filter({ hasText: label }).first().locator('.el-select').first().click()
  await page.waitForTimeout(300)
  await page.locator('.el-select-dropdown:visible .el-select-dropdown__item').filter({ hasText: option }).first().click()
  await page.waitForTimeout(300)
}

// 在当前对话框里填写某个表单项的输入框（第 n 个输入框，默认第一个）
async function fill(dialog, label, value, n = 0) {
  const item = dialog.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: new RegExp(`^${label}$`) }) }).first()
  await item.locator('input, textarea').nth(n).fill(value)
}

// 打开「添加渠道」并切换到指定类型
async function openChannel(kind) {
  await reset('notify')
  await page.getByRole('button', { name: '添加渠道' }).click()
  await page.waitForTimeout(500)
  const dlg = page.locator('.el-dialog:visible').first()
  if (kind !== 'Telegram') {
    await pick(dlg, '类型', kind)
  }
  return dlg
}

// 1. 通知中心总览：在渠道卡片上点「测试」后出现「测试通知已发送」（演示实例只读，测试请求被拦截并当作成功）
await reset('notify')
await fakeWrites(page)
const testBtn = page.locator('.el-button').filter({ hasText: /^测试$/ }).first()
await testBtn.click()
await page.locator('.el-message').first().waitFor()
await mark(page, testBtn)
await mark(page, page.getByRole('button', { name: '添加渠道' }))
await page.waitForTimeout(300)
await shot(page, `${S}-overview`)

// 2. Telegram 渠道
{
  const dlg = await openChannel('Telegram')
  await fill(dlg, '名称', 'Telegram 个人')
  await fill(dlg, 'Bot Token', '123456789:AAExampleTokenForDocsOnly')
  await fill(dlg, 'Chat ID', '100000001')
  await shot(page, `${S}-telegram`, '.el-dialog:visible')
}

// 3. 邮件 SMTP 渠道
{
  const dlg = await openChannel('邮件 SMTP')
  await fill(dlg, '名称', '运维邮箱')
  await fill(dlg, '服务器', 'smtp.example.com')
  await pick(dlg, '加密方式', 'SSL/TLS（465）')
  await dlg.locator('.el-form-item').filter({ hasText: '端口' }).locator('input').first().fill('465')
  await fill(dlg, '用户名', 'gately@example.com')
  await fill(dlg, '密码', 'app-password-example')
  await fill(dlg, '收件人', 'admin@example.com, ops@example.net')
  await shot(page, `${S}-smtp`, '.el-dialog:visible')
}

// 4. Webhook 渠道（以企业内部接收服务为例）
{
  const dlg = await openChannel('Webhook')
  await fill(dlg, '名称', '团队 Webhook')
  await fill(dlg, '请求地址', 'https://hooks.example.com/gately')
  await fill(dlg, '请求头', 'Authorization: Bearer example-token')
  await fill(dlg, '请求体', '{"title": "{title}",\n "text": "{text}",\n "level": "{level}"}')
  await dlg.locator('textarea').last().evaluate((el) => { el.scrollTop = 0 })
  await shot(page, `${S}-webhook`, '.el-dialog:visible')
}

// 5. 添加规则：点「关键故障」模板后选择渠道
{
  await reset('notify')
  await page.getByRole('button', { name: '添加规则' }).click()
  await page.waitForTimeout(500)
  const dlg = page.locator('.el-dialog:visible').first()
  const tpl = dlg.getByRole('button', { name: '关键故障' })
  await tpl.click()
  await page.waitForTimeout(300)
  await dlg.locator('.el-form-item').filter({ hasText: '渠道' }).last().locator('.el-select').click()
  await page.waitForTimeout(300)
  await page.locator('.el-select-dropdown:visible .el-select-dropdown__item').filter({ hasText: 'Telegram 个人' }).first().click()
  await page.locator('.el-select-dropdown:visible .el-select-dropdown__item').filter({ hasText: '邮件' }).first().click()
  await dlg.locator('.el-dialog__title').click()
  await page.waitForTimeout(300)
  await mark(page, tpl)
  await shot(page, `${S}-rule`, '.el-dialog:visible')
}

// 6. 「最近投递」：每次投递的事件、规则、渠道与结果
{
  await reset('notify')
  // 把「最近投递」标题与表格临时包进一个元素里，一起截图（只改当前页面的 DOM，刷新即恢复）
  await page.evaluate(() => {
    const head = [...document.querySelectorAll('h3.section')].find((h) => h.textContent.includes('最近投递'))
    const table = head.nextElementSibling
    const box = document.createElement('div')
    box.id = 'tut-deliveries'
    head.before(box)
    box.append(head, table)
  })
  await shot(page, `${S}-deliveries`, '#tut-deliveries')
}

// 7. 性能监控的告警规则与「还没有订阅性能告警」提示
{
  await reset('perf')
  const sec = page.locator('section').filter({ has: page.locator('h3.section', { hasText: '告警规则' }) }).first()
  // 与上方站点说明文字拉开距离，截图边缘不带进上一段的文字
  await sec.evaluate((el) => { el.style.marginTop = '28px' })
  await mark(page, sec.locator('.el-alert'))
  await shot(page, `${S}-perf-rules`, sec)
}

// 8. 添加告警规则：站点 5xx 过多模板（表单较长，加高视口让对话框完整显示）
{
  await page.setViewportSize({ width: 1440, height: 1500 })
  await reset('perf')
  await page.getByRole('button', { name: '添加告警规则' }).click()
  await page.waitForTimeout(500)
  const dlg = page.locator('.el-dialog:visible').first()
  const tpl = dlg.getByRole('button', { name: '站点 5xx 过多' })
  await tpl.click()
  await page.waitForTimeout(400)
  await mark(page, tpl)
  await shot(page, `${S}-perf-rule`, '.el-dialog:visible')
}

await browser.close()
