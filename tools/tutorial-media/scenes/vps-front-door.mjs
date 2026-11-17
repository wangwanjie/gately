// 教程「云服务器做入口，回家保留真实 IP」的截图：VPS 上的 frps 实例与协议分流规则，
// 家里的 frpc 实例、tcp 代理、Web 服务可信 PROXY 来源与接收 PROXY 协议的监听。
// 两台 Gately 的界面相同，这里都用同一个演示实例截取，弹窗里填的是示例值。
import { open, go, shot, mark } from '../lib.mjs'

const S = 'vps-front-door'
const { browser, page } = await open({ height: 1100 })

const dialog = () => page.locator('.el-dialog:visible').last()
// 按标签文字找表单项（标签完全相同），避免帮助文字里的同名词误匹配
const item = (d, label) => d.locator('.el-form-item').filter({ has: page.locator(':scope > .el-form-item__label', { hasText: new RegExp(`^${label}$`) }) }).first()
async function fill(d, label, value, nth = 0) {
  await item(d, label).locator('input, textarea').nth(nth).fill(String(value))
}
async function pick(target, option) {
  await target.click()
  await page.waitForTimeout(300)
  await page.locator('.el-select-dropdown__item:visible').filter({ hasText: option }).first().click()
  await page.waitForTimeout(200)
}
// 只保留指定标签的表单项，截出紧凑的对话框
async function keepOnly(d, labels) {
  await d.locator('.el-form-item').evaluateAll((els, k) => {
    for (const e of els) {
      const label = e.querySelector(':scope > .el-form-item__label')?.innerText.trim() || ''
      if (label && !k.includes(label)) {
        e.style.display = 'none'
      }
    }
  }, labels)
}

// 1. VPS：frps 实例
await go(page, 'frp')
await page.getByRole('tab', { name: '服务端 frps' }).click()
await page.waitForTimeout(500)
await page.getByRole('button', { name: '添加 frps 实例' }).click()
let d = dialog()
await d.waitFor()
await page.waitForTimeout(500)
await fill(d, '名称', 'frps-home')
await fill(d, '绑定端口', 7000)
await fill(d, '令牌', 'f3Kq9-Example-Token-Use-Your-Own')
await item(d, '令牌').locator('input').evaluate((el) => { el.type = 'text' })
await fill(d, '代理监听地址', '127.0.0.1')
await fill(d, '允许的远程端口', '15000-15100')
const fw = item(d, '自动放行防火墙').locator('.el-switch, .el-checkbox').first()
if (!(await fw.evaluate((el) => el.classList.contains('is-checked')))) {
  await fw.click()
}
await keepOnly(d, ['名称', '启用', '监听地址', '绑定端口', '令牌', '代理监听地址', '允许的远程端口', '自动放行防火墙'])
await mark(page, item(d, '代理监听地址').locator('.el-input'))
await page.waitForTimeout(300)
await shot(page, `${S}-frps`, d)
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 2. 家里：frpc 实例
await page.reload()
await page.waitForTimeout(800)
await page.getByRole('tab', { name: '客户端 frpc' }).click()
await page.waitForTimeout(500)
await page.getByRole('button', { name: '添加 frpc 实例' }).click()
d = dialog()
await d.waitFor()
await page.waitForTimeout(500)
await fill(d, '名称', 'VPS 入口')
await fill(d, '服务端地址', '203.0.113.10')
await fill(d, '服务端端口', 7000)
await fill(d, '令牌', 'f3Kq9-Example-Token-Use-Your-Own')
await item(d, '令牌').locator('input').evaluate((el) => { el.type = 'text' })
await keepOnly(d, ['名称', '启用', '服务端地址', '服务端端口', '令牌', '传输协议'])
await page.waitForTimeout(300)
await shot(page, `${S}-frpc`, d)
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 3. 家里：tcp 代理，把本机 HTTPS 监听映射到 VPS 的 15443
await page.reload()
await page.waitForTimeout(800)
await page.getByRole('tab', { name: '客户端 frpc' }).click()
await page.waitForTimeout(500)
await page.getByRole('button', { name: '添加代理' }).click()
d = dialog()
await d.waitFor()
await page.waitForTimeout(500)
await fill(d, '名称', 'home-https')
await pick(item(d, '类型').locator('.el-select'), 'tcp')
await fill(d, '本地地址', '127.0.0.1')
await fill(d, '本地端口', 443)
await fill(d, '远程端口', 15443)
await page.waitForTimeout(300)
await mark(page, item(d, '远程端口').locator('.el-input-number, .el-input').first())
await shot(page, `${S}-proxy`, d)
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 4. VPS：协议分流规则，443 上 SSH 与默认目标都送进隧道
await go(page, 'protoroute')
await page.getByRole('button', { name: '添加规则' }).click()
d = dialog()
await d.waitFor()
await page.waitForTimeout(500)
await fill(d, '名称', '443 入口（回家）')
await fill(d, '监听端口', '443')
const routes = item(d, '分流路由')
// 第 1 条路由：SSH
await pick(routes.locator('.el-select').first(), 'SSH')
await routes.getByPlaceholder('路由名称，例如 anylink').first().fill('home-ssh')
await routes.getByPlaceholder('目标地址，多个用逗号分隔').first().fill('127.0.0.1')
await routes.getByPlaceholder('目标端口').first().fill('15022')
await item(d, '默认目标').locator('textarea').fill('127.0.0.1')
await fill(d, '目标端口', '15443')
await pick(item(d, '默认目标 PROXY').locator('.el-select'), 'PROXY v2')
const pfw = item(d, '自动放行防火墙').locator('.el-switch, .el-checkbox').first()
if (!(await pfw.evaluate((el) => el.classList.contains('is-checked')))) {
  await pfw.click()
}
await keepOnly(d, ['名称', '协议', '监听地址', '监听端口', '分流路由', '默认目标', '目标端口', '默认目标 PROXY', '自动放行防火墙'])
await mark(page, item(d, '默认目标 PROXY').locator('.el-select'))
await page.waitForTimeout(300)
await shot(page, `${S}-protoroute`, d)
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 5. 家里：Web 服务全局设置的可信 PROXY 来源
await go(page, 'web')
await page.getByRole('button', { name: '全局设置' }).click()
d = dialog()
await d.waitFor()
await page.waitForTimeout(500)
await item(d, '可信 PROXY 来源').locator('textarea').fill('127.0.0.0/8')
await shot(page, `${S}-trusted`, item(d, '可信 PROXY 来源'))
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 6. 家里：HTTPS 监听开启接收 PROXY 协议
await page.reload()
await page.waitForTimeout(1000)
await page.getByRole('button', { name: '添加监听' }).click()
d = dialog()
await d.waitFor()
await page.waitForTimeout(500)
await fill(d, '名称', '家里 HTTPS（局域网与隧道）')
await fill(d, '端口', 443)
await d.getByText('高级设置').click()
await page.waitForTimeout(400)
const ap = item(d, '接收 PROXY 协议').locator('.el-switch, .el-checkbox').first()
if (!(await ap.evaluate((el) => el.classList.contains('is-checked')))) {
  await ap.click()
}
await mark(page, item(d, '接收 PROXY 协议'))
await keepOnly(d, ['名称', '监听地址', '端口', 'HTTPS', '启用', '网络', '接收 PROXY 协议', '自动放行防火墙'])
await page.waitForTimeout(300)
await shot(page, `${S}-listener`, d)

await browser.close()
