// 教程「异地组网」的截图：Tailscale 实例与节点、添加 Tailscale 实例、NetBird 实例与节点、添加 NetBird 实例、
// 添加 EasyTier 实例、添加 Cloudflare 隧道
import { open, go, shot, mark } from '../lib.mjs'

// 视口加高，让较长的对话框一次截全
const { browser, page } = await open({ height: 1800 })

const field = (dlg, label) => dlg.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: new RegExp('^' + label + '$') }) }).first()
const sw = async (dlg, label, on) => {
  const s = field(dlg, label).locator('.el-switch').first()
  const checked = await s.evaluate((el) => el.classList.contains('is-checked'))
  if (checked !== on) {
    await s.click()
  }
}

// 1. Tailscale：实例卡片与节点列表
await go(page, 'mesh/tailscale')
await page.waitForTimeout(600)
// 截实例的节点卡片：本机地址、tailnet 与各节点在线状态
await shot(page, 'mesh-vpn-tailscale', page.locator('.g-card').filter({ hasText: '退出登录' }).first())

// 2. 添加 Tailscale 实例
await page.getByRole('button', { name: '添加实例' }).first().click()
let dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('家里')
await field(dlg, '设备名称').locator('input').fill('gately-home')
await field(dlg, '子网路由').locator('input').fill('192.168.1.0/24')
await sw(dlg, '出口节点', true)
await sw(dlg, '用户态网络', true)
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await mark(page, field(dlg, '子网路由'))
await shot(page, 'mesh-vpn-tailscale-dialog', dlg)
await page.keyboard.press('Escape')

// 3. NetBird：实例卡片与节点（直连 / 中继、延迟）
await go(page, 'mesh/netbird')
await page.waitForTimeout(600)
await shot(page, 'mesh-vpn-netbird', page.locator('.g-card').filter({ hasText: '退出登录' }).first())

// 4. 添加 NetBird 实例：自建管理服务 + 设置密钥
await page.getByRole('button', { name: '添加实例' }).first().click()
dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('家里')
await field(dlg, '设置密钥').locator('input').fill('A1B2C3D4-0000-4000-8000-EXAMPLEKEY01')
await field(dlg, '管理服务地址').locator('input').fill('https://netbird.example.com')
await field(dlg, '设备名称').locator('input').fill('gately-home')
await sw(dlg, '作为路由节点', true)
await sw(dlg, '用户态网络', true)
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await shot(page, 'mesh-vpn-netbird-dialog', dlg)
await page.keyboard.press('Escape')

// 5. 添加 EasyTier 实例
await go(page, 'mesh/easytier')
await page.getByRole('button', { name: '添加实例' }).first().click()
dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('家里')
await field(dlg, '组网名称').locator('input').fill('home-net')
await field(dlg, '网络密钥').locator('input').fill('Example-Secret-7f3a9c')
await field(dlg, '节点地址').locator('textarea').fill('tcp://relay.example.com:11010')
await field(dlg, '子网代理').locator('textarea').fill('192.168.1.0/24')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await shot(page, 'mesh-vpn-easytier-dialog', dlg)
await page.keyboard.press('Escape')

// 6. 添加 Cloudflare 隧道
await go(page, 'cloudflared')
await page.getByRole('button', { name: '添加隧道' }).first().click()
dlg = page.locator('.el-dialog:visible').first()
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('家里')
await field(dlg, '隧道令牌').locator('input, textarea').first().fill('eyJhIjoiZXhhbXBsZSJ9-example-token')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await shot(page, 'mesh-vpn-cloudflared-dialog', dlg)

await browser.close()
