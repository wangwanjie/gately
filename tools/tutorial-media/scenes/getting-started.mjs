// 教程「安装后的第一步」的截图：登录页、强制改密、使用场景、开始使用清单、网络自检、全局搜索、两步验证、功能模块。
// 演示实例的管理员密码已是强密码、也不是全新安装，改密对话框、场景选择与清单都由本脚本模拟接口返回来触发；
// 网络自检在演示模式下不可用，同样模拟一份示例结果。
import { BASE, open, go, shot } from '../lib.mjs'

const { browser, page } = await open()

// fresh 先离开再打开页面，确保整页重新加载、重新读取被模拟的接口（同一地址的 hash 跳转不会重载）
async function fresh(route) {
  await page.goto('about:blank')
  await go(page, route)
}

// mockJSON 把某个接口的响应替换成给定 JSON（只影响当前页面）
async function mockJSON(p, pattern, data, method = null) {
  await p.route(pattern, async (route) => {
    if (method && route.request().method() !== method) {
      return route.continue()
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })
  })
}

// 1. 登录页：用未登录的新上下文打开，隐藏演示实例特有的「演示账号一键进入」卡片
{
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN', deviceScaleFactor: 2 })
  await ctx.addInitScript(() => {
    localStorage.setItem('gately.locale', 'zh-CN')
    localStorage.setItem('gately.theme.mode', 'light')
    const css = '.demo-banner,.demo-card{display:none!important}'
    const add = () => {
      const st = document.createElement('style')
      st.textContent = css
      document.head.appendChild(st)
    }
    if (document.head) {
      add()
    } else {
      document.addEventListener('DOMContentLoaded', add)
    }
  })
  const lp = await ctx.newPage()
  await lp.goto(`${BASE}/#/login`)
  await lp.waitForLoadState('networkidle').catch(() => {})
  await lp.waitForTimeout(800)
  await lp.fill('input[autocomplete=username]', 'admin')
  await lp.fill('input[type=password]', 'gately.cn')
  await shot(lp, 'getting-started-login')
  await ctx.close()
}

// 2. 强制修改默认密码：会话接口报告仍在使用默认密码
{
  const real = await (await page.request.get(`${BASE}/api/session`)).json()
  await mockJSON(page, '**/api/session', { ...real, passwordIssue: 'default' }, 'GET')
  await fresh('overview')
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  const pw = dlg.locator('input[type=password]')
  await pw.nth(0).fill('gately.cn')
  await pw.nth(1).fill('Blue-Harbor-Lamp-2048')
  await pw.nth(2).fill('Blue-Harbor-Lamp-2048')
  await page.waitForTimeout(800)
  await shot(page, 'getting-started-force-password', dlg)
  await page.unroute('**/api/session')
}

// 3. 选择使用场景：全新安装改完密码后出现
{
  await mockJSON(page, '**/api/system/onboarding', { pending: true, scenePending: true, scenes: [], done: [] }, 'GET')
  await fresh('overview')
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  await dlg.getByText('把内网服务发布成域名').click()
  await dlg.getByText('动态域名', { exact: true }).click()
  await page.waitForTimeout(500)
  await shot(page, 'getting-started-scenes', dlg)
  await page.unroute('**/api/system/onboarding')
}

// 4. 总览顶部的「开始使用」清单
{
  await mockJSON(page, '**/api/system/onboarding', { pending: true, scenePending: false, scenes: ['web', 'ddns'], done: [] }, 'GET')
  await fresh('overview')
  await page.locator('[data-testid=getting-started]').waitFor()
  await page.waitForTimeout(800)
  await shot(page, 'getting-started-overview')
  await page.unroute('**/api/system/onboarding')
}

// 5. 网络自检：模拟一份示例结果
{
  const results = [
    { id: 'net.publicIPv4', status: 'ok', params: { ip: '203.0.113.10' } },
    { id: 'net.publicIPv6', status: 'ok', params: { ip: '2001:db8::10' } },
    { id: 'net.behindNat', status: 'info' },
    { id: 'time.clock', status: 'ok', params: { seconds: '0' } },
    { id: 'port.80', status: 'ok' },
    { id: 'port.443', status: 'info' },
    { id: 'ddns.match', status: 'ok', params: { domain: 'nas.example.com', resolved: '203.0.113.10' } },
    { id: 'cert.ok', status: 'ok', params: { cert: 'example.com', days: '44' } },
    { id: 'web.certOk', status: 'ok', params: { count: '3' } }
  ]
  await mockJSON(page, '**/api/system/selfcheck', { results }, 'POST')
  await fresh('overview')
  await page.getByRole('button', { name: '网络自检' }).click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.getByText('公网 IPv4 地址').waitFor()
  await page.waitForTimeout(600)
  await shot(page, 'getting-started-selfcheck', dlg)
  await page.keyboard.press('Escape')
  await page.unroute('**/api/system/selfcheck')
}

// 6. 全局搜索（⌘K / Ctrl+K）
{
  await fresh('overview')
  await page.keyboard.press('Meta+k')
  await page.waitForTimeout(400)
  await page.keyboard.type('nas')
  await page.waitForTimeout(800)
  await shot(page, 'getting-started-search')
  await page.keyboard.press('Escape')
}

// 8. 启用两步验证的二维码对话框：模拟生成的密钥（RFC 示例值）
{
  await mockJSON(page, '**/api/account/totp/setup', { secret: 'JBSWY3DPEHPK3PXP', url: 'otpauth://totp/Gately:admin?secret=JBSWY3DPEHPK3PXP&issuer=Gately' }, 'POST')
  await fresh('settings')
  await page.getByRole('button', { name: '开始设置' }).click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.locator('img').first().waitFor()
  await page.waitForTimeout(600)
  await shot(page, 'getting-started-totp', dlg)
  await page.keyboard.press('Escape')
}

// 9. 设置 → 功能模块
{
  await fresh('settings?tab=modules')
  const card = page.locator('.g-card', { hasText: '按需开启功能模块' }).first()
  await card.waitFor()
  await shot(page, 'getting-started-modules')
}

await browser.close()
