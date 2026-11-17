// 教程「STUN 穿透」的截图：网络自检、规则列表、添加规则的基本/路由器映射/通知页、DDNS 记录值变量、重定向站点
import { open, go, shot, mark } from '../lib.mjs'

// 视口加高，让较长的对话框一次截全
const { browser, page } = await open({ height: 1500 })

// 1. 网络自检：演示实例不能联网检查，这里只在浏览器里替换接口返回，展示位于运营商 NAT 后面时的结果
await page.route('**/api/system/selfcheck', (route) =>
  route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      results: [
        { id: 'net.publicIPv4', status: 'ok', params: { ip: '203.0.113.10' } },
        { id: 'net.cgnat', status: 'warn', params: { ip: '100.64.12.34' } },
        { id: 'net.publicIPv6', status: 'ok', params: { ip: '2001:db8::10' } },
        { id: 'time.clock', status: 'ok', params: { seconds: '0' } },
        { id: 'port.80', status: 'ok' },
        { id: 'port.443', status: 'ok' }
      ]
    })
  })
)
await go(page, 'overview')
await page.getByText('网络自检', { exact: true }).first().click()
await page.locator('.el-dialog .row').first().waitFor()
await page.waitForTimeout(600)
await shot(page, 'stun-traversal-selfcheck', page.locator('.el-dialog').filter({ hasText: '网络与配置自检' }))

// 2. 规则列表
await go(page, 'stun')
await shot(page, 'stun-traversal-list', page.locator('.grouped-list').first())

// 3. 添加规则：基本页
await page.getByRole('button', { name: '添加规则' }).click()
const dlg = page.locator('.el-dialog').filter({ hasText: '规则名称' })
await dlg.waitFor()
await page.waitForTimeout(400)
const item = (label) => dlg.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: new RegExp('^' + label + '$') }) }).first()
await item('规则名称').locator('input').fill('nas')
await item('通道本地端口').locator('input').fill('50443')
await item('自动放行防火墙').locator('.el-switch').click()
await item('目标地址').locator('input').fill('192.168.1.10')
await item('目标端口').locator('input').fill('5000')
await item('备注').locator('input').fill('NAS 网页，TCP 穿透')
await dlg.locator('.el-dialog__title').click()
await page.waitForTimeout(300)
await mark(page, item('通道本地端口'))
await shot(page, 'stun-traversal-rule-basic', dlg)

// 4. 路由器映射页：开启 UPnP
await dlg.getByRole('tab', { name: '路由器映射' }).click()
await page.waitForTimeout(300)
await item('UPnP').locator('.el-switch').click()
await page.waitForTimeout(300)
await shot(page, 'stun-traversal-rule-mapping', dlg)

// 5. 通知页：开启脚本，展示可用变量
await dlg.getByRole('tab', { name: '通知' }).click()
await page.waitForTimeout(300)
await item('执行脚本').locator('.el-switch').click()
await page.waitForTimeout(200)
await item('脚本内容').locator('textarea').evaluate((el) => { el.spellcheck = false })
await item('脚本内容').locator('textarea').fill('logger -t gately "STUN ${ruleName} 新地址 ${ipAddr}"')
await shot(page, 'stun-traversal-rule-notify', dlg)
await dlg.getByRole('button', { name: '取消' }).click()

// 6. DDNS 任务的解析记录：用 STUN 变量作为记录值
await go(page, 'ddns')
await page.locator('.entity').filter({ hasText: 'Cloudflare：example.com' }).first().getByRole('button', { name: '编辑' }).click()
const ddns = page.locator('.el-dialog').filter({ hasText: '解析记录' }).first()
await ddns.waitFor()
await ddns.getByRole('tab', { name: '解析记录' }).click()
await page.waitForTimeout(400)
const rows = ddns.locator('.rec-edit')
const n = await rows.count()
// 在末尾加两条记录：A 记录指向 STUN 公网 IP，TXT 记录写端口
await ddns.getByRole('button', { name: 'IPv4 记录' }).click()
await page.waitForTimeout(200)
let r = ddns.locator('.rec-edit').nth(n)
await r.locator('.re-cell').nth(1).locator('input').fill('nas')
await r.locator('.re-cell').nth(2).locator('input').fill('example.com')
await r.locator('.re-cell').nth(4).locator('input').fill('{STUN_nas_IP}')
await ddns.getByRole('button', { name: 'IPv4 记录' }).click()
await page.waitForTimeout(200)
r = ddns.locator('.rec-edit').nth(n + 1)
await r.locator('.re-cell').nth(1).locator('input').fill('_port.nas')
await r.locator('.re-cell').nth(2).locator('input').fill('example.com')
await r.locator('.re-cell').nth(3).locator('.el-select').click()
await page.locator('.el-select-dropdown:visible .el-select-dropdown__item', { hasText: /^TXT$/ }).click()
await page.waitForTimeout(200)
await r.locator('.re-cell').nth(4).locator('input').fill('{STUN_nas_PORT}')
await mark(page, ddns.locator('.rec-edit').nth(n))
await mark(page, ddns.locator('.rec-edit').nth(n + 1))
await shot(page, 'stun-traversal-ddns-records', ddns)

await browser.close()
