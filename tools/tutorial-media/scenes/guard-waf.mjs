// 教程「入侵防护与 WAF」的截图：入侵防护总览、设置与白名单、规则模板、Web 诱饵路径规则、
// 诱饵端口与扫描检测、WAF 实例编辑、站点引用 WAF 实例
import { open, go, shot, mark } from '../lib.mjs'

const { browser, page } = await open()

// 1. 入侵防护总览：防火墙状态与封禁列表
await go(page, 'guard')
await shot(page, 'guard-waf-overview', page.locator('.guard-tabs').locator('xpath=..'))

// 2. 设置页签：防火墙同步、封禁时长、白名单
await page.getByRole('tab', { name: '设置' }).click()
await page.waitForTimeout(500)
const wl = page.locator('.guard-tabs textarea').first()
await wl.fill('203.0.113.10\n2001:db8:1::/48')
await mark(page, wl)
await shot(page, 'guard-waf-settings', page.locator('#pane-settings .g-card'))

// 3. 日志规则页签：从模板添加
await page.getByRole('tab', { name: '日志规则' }).click()
await page.waitForTimeout(500)
await page.setViewportSize({ width: 1440, height: 1600 })
await page.getByRole('button', { name: '从模板添加' }).click()
await page.waitForTimeout(600)
await page.mouse.move(5, 5)
await page.waitForTimeout(300)
await shot(page, 'guard-waf-presets', '.el-dropdown-menu:visible', { pad: 4 })
await page.keyboard.press('Escape')
await page.setViewportSize({ width: 1440, height: 900 })

// 4. Web 路径探测规则：从 Web 服务监听卡片的「路径探测封禁」进入，预选监听并套用诱饵路径模板。
// 演示实例不开放规则测试接口，这里按路径规则的写法（* 匹配任意字符、整条路径、不区分大小写）在本地算出测试结果
await page.route('**/api/guard/jails/test', async (route) => {
  const body = route.request().postDataJSON()
  const rules = body.filter.split('\n').map((s) => s.trim()).filter(Boolean)
  const re = rules.map((r) => new RegExp('^' + r.replace(/[.+?^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*') + '$', 'i'))
  const out = body.lines.split('\n').filter((l) => l.trim()).map((line) => ({ line, matched: re.some((r) => r.test(line.trim())) }))
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(out) })
})
// 先离开入侵防护页，带参数重新进入时页面才会重新挂载并打开规则编辑框
await go(page, 'overview')
await go(page, 'guard?webProbe=l-pub')
await page.waitForTimeout(800)
const dlg = page.locator('.el-dialog:visible').first()
await dlg.locator('textarea').last().fill('/.env\n/api/.ENV\n/index.html\n/photos/2026/cat.jpg')
await dlg.getByRole('button', { name: '测试' }).click()
await page.waitForTimeout(600)
await page.addStyleTag({ content: '.el-overlay-dialog{position:absolute!important}.el-dialog{margin-top:20px!important}' })
await page.setViewportSize({ width: 1440, height: 1700 })
await page.waitForTimeout(500)
await shot(page, 'guard-waf-web-decoy', '.el-dialog:visible')
await page.setViewportSize({ width: 1440, height: 900 })
await page.keyboard.press('Escape')

// 5. 诱饵端口与扫描检测
await go(page, 'guard')
await page.getByRole('tab', { name: '诱饵端口与扫描检测' }).click()
await page.waitForTimeout(600)
await page.setViewportSize({ width: 1440, height: 1500 })
await page.waitForTimeout(400)
await shot(page, 'guard-waf-scan', '#pane-scan')
await page.setViewportSize({ width: 1440, height: 900 })

// 6. WAF 实例编辑：新建一个仅检测的实例
await go(page, 'waf')
await page.getByRole('button', { name: '添加实例' }).click()
await page.waitForTimeout(600)
const wd = page.locator('.el-dialog:visible').first()
await wd.locator('input').first().fill('公网站点')
await wd.getByText('仅检测（只记录日志）').click()
await mark(page, wd.locator('.el-radio-group').first())
await page.setViewportSize({ width: 1440, height: 2000 })
await page.waitForTimeout(500)
await shot(page, 'guard-waf-waf-editor', '.el-dialog:visible')
await page.setViewportSize({ width: 1440, height: 900 })
await page.keyboard.press('Escape')

// 7. 站点「安全」页签引用 WAF 实例
await go(page, 'web?editListener=l-pub&editSite=s-blog')
await page.waitForTimeout(1000)
const sd = page.locator('.el-dialog:visible').first()
await sd.getByRole('tab', { name: '安全' }).click()
await page.waitForTimeout(500)
const wafItem = sd.locator('.el-form-item').filter({ hasText: 'WAF 实例' }).first()
await mark(page, wafItem)
await page.setViewportSize({ width: 1440, height: 2000 })
await page.waitForTimeout(500)
await shot(page, 'guard-waf-site', '.el-dialog:visible')

await browser.close()
