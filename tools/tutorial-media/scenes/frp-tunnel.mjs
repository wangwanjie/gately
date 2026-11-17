// 教程「frp 内网穿透」的截图：frps 实例列表与编辑、frpc 实例、tcp / https 代理、在线安装卡片
import { open, go, shot, mark } from '../lib.mjs'

// 视口加高，让较长的对话框一次截全
const { browser, page } = await open({ height: 1700 })

// 按表单项标签找到对话框里的字段
const field = (dlg, label) => dlg.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: new RegExp('^' + label + '$') }) }).first()

// 1. VPS 上：服务端 frps 页的实例列表
await go(page, 'frp')
await page.getByRole('tab', { name: '服务端 frps' }).click()
await page.waitForTimeout(600)
await shot(page, 'frp-tunnel-frps-list', page.locator('.el-tab-pane:visible .g-card').first())

// 2. 添加 frps 实例
await page.getByRole('button', { name: '添加 frps 实例' }).click()
let dlg = page.locator('.el-dialog').filter({ hasText: '绑定端口' })
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('公网 frps')
await field(dlg, '绑定端口').locator('input').fill('7000')
await field(dlg, 'QUIC 端口（UDP）').locator('input').fill('7001')
await field(dlg, '令牌').locator('input').fill('Y3k9-example-token-2c7d')
await field(dlg, '代理监听地址').locator('input').fill('127.0.0.1')
await field(dlg, '允许的远程端口').locator('input').fill('15000-15100')
await field(dlg, '自动放行防火墙').locator('.el-switch').click()
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await mark(page, field(dlg, '代理监听地址'))
await shot(page, 'frp-tunnel-frps-dialog', dlg)
await dlg.getByRole('button', { name: '取消' }).click()

// 3. 家里：添加 frpc 实例
await page.getByRole('tab', { name: '客户端 frpc' }).click()
await page.waitForTimeout(400)
await page.getByRole('button', { name: '添加 frpc 实例' }).click()
dlg = page.locator('.el-dialog').filter({ hasText: '服务端地址' })
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('VPS')
await field(dlg, '服务端地址').locator('input').fill('vps.example.com')
await field(dlg, '服务端端口').locator('input').fill('7000')
await field(dlg, '令牌').locator('input').fill('Y3k9-example-token-2c7d')
await field(dlg, '用户名').locator('input').fill('home')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await shot(page, 'frp-tunnel-frpc-dialog', dlg)
await dlg.getByRole('button', { name: '取消' }).click()

// 4. 添加 tcp 代理
await page.getByRole('button', { name: '添加代理' }).click()
dlg = page.locator('.el-dialog').filter({ hasText: '所属实例' })
await dlg.waitFor()
await page.waitForTimeout(400)
await field(dlg, '名称').locator('input').fill('nas-web')
await field(dlg, '本地地址').locator('input').fill('192.168.1.10')
await field(dlg, '本地端口').locator('input').fill('5000')
await field(dlg, '远程端口').locator('input').fill('15000')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await shot(page, 'frp-tunnel-proxy-tcp', dlg)

// 5. 同一对话框改成 https 代理，展示自定义域名与子域名
await field(dlg, '类型').locator('.el-select').click()
await page.locator('.el-select-dropdown:visible .el-select-dropdown__item', { hasText: /^https$/ }).click()
await page.waitForTimeout(300)
await field(dlg, '名称').locator('input').fill('nas-https')
await field(dlg, '本地端口').locator('input').fill('5001')
await field(dlg, '子域名').locator('input').fill('nas')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await shot(page, 'frp-tunnel-proxy-https', dlg)
await dlg.getByRole('button', { name: '取消' }).click()

// 6. 客户端页：frpc 实例与代理列表
await page.waitForTimeout(400)
await shot(page, 'frp-tunnel-frpc-list', page.locator('.el-tab-pane:visible').first())

// 7. 在线安装卡片：演示实例的配置目录是临时目录，截图前换成常见的 Docker 部署路径
await page.evaluate(() => {
  for (const el of document.querySelectorAll('.g-card *')) {
    if (el.childNodes.length === 1 && el.firstChild.nodeType === 3 && el.textContent.includes('/tmp/gately-tut/data')) {
      el.textContent = el.textContent.replace('/tmp/gately-tut/data', '/etc/gately')
    }
  }
})
await shot(page, 'frp-tunnel-install', page.locator('.g-card').filter({ hasText: 'frp 在线安装' }).last())

await browser.close()
