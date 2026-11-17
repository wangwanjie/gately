// 教程「站点访问控制与登录认证」截图：站点安全页的认证、IP 访问控制、路径规则与国家过滤、拒绝方式，
// 认证用户页与 TOTP，Web 服务全局设置中的第三方登录与订阅 IP 列表。
// 用法：在 tools/tutorial-media 目录运行 node scenes/site-access-control.mjs
import { open, go, shot, mark } from '../lib.mjs'

const SLUG = 'site-access-control'

// 视口足够高，长对话框不需要滚动，区域截图可以直接用视口坐标
const { browser, ctx, page: first } = await open({ height: 3000 })
let page = first

// 演示库没有导入 GeoIP 数据库，国家过滤会显示「尚未导入」提示；截图时按已导入展示
await ctx.route('**/api/webservice/geoip', (route) => route.fulfill({
  status: 200,
  contentType: 'application/json',
  body: JSON.stringify({ loaded: true, type: 'GeoLite2-Country', buildTime: '2026-11-11T00:00:00Z', size: 9800000 })
}))
// 「启用 TOTP」需要后端生成密钥（演示实例只读），用公开的示例密钥代替
await ctx.route('**/api/webservice/totp-secret', (route) => {
  const name = route.request().postDataJSON()?.name || 'alice'
  const secret = 'JBSWY3DPEHPK3PXP'
  const url = `otpauth://totp/${encodeURIComponent('Gately:' + name)}?digits=6&issuer=Gately&period=30&secret=${secret}`
  return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ secret, url }) })
})

// 按标签文字精确找到当前对话框里的表单项
function item(scope, label) {
  return scope.locator('.el-form-item__label', { hasText: new RegExp(`^\\s*${label}\\s*$`) }).first()
    .locator('xpath=ancestor::div[contains(concat(" ", normalize-space(@class), " "), " el-form-item ")][1]')
}

// region 在 first 与 last 两个元素覆盖的范围上放一个透明占位层，返回它的 Locator 供 shot 截取
async function region(first, last) {
  const a = await first.boundingBox()
  const b = await last.boundingBox()
  // 四周多留 8px，描边（outline-offset）不会被裁掉
  const x = Math.min(a.x, b.x) - 8
  const y = Math.min(a.y, b.y) - 8
  const right = Math.max(a.x + a.width, b.x + b.width) + 8
  const bottom = Math.max(a.y + a.height, b.y + b.height) + 8
  await page.evaluate(({ x, y, w, h }) => {
    document.getElementById('tut-region')?.remove()
    const d = document.createElement('div')
    d.id = 'tut-region'
    Object.assign(d.style, { position: 'fixed', left: x + 'px', top: y + 'px', width: w + 'px', height: h + 'px', pointerEvents: 'none', zIndex: 99999 })
    document.body.appendChild(d)
  }, { x, y, w: right - x, h: bottom - y })
  return page.locator('#tut-region')
}

// 选择下拉选项
async function pick(select, option) {
  await select.click()
  await page.waitForTimeout(300)
  await page.locator('.el-select-dropdown:visible .el-select-dropdown__item', { hasText: option }).first().click()
  await page.waitForTimeout(200)
}

// ---------- 1. 站点「安全」页：认证 ----------
await go(page, 'web')
const row = page.getByText('photos.example.com', { exact: true }).first()
  .locator('xpath=ancestor::*[contains(@class,"site")][count(.//button)>=5][1]')
await row.getByRole('button', { name: '编辑' }).click()
await page.waitForTimeout(600)
const dlg = page.locator('.el-dialog:visible').first()
await dlg.locator('.el-tabs__item', { hasText: '安全' }).click()
await page.waitForTimeout(500)

await item(dlg, '第三方登录').locator('textarea').fill('github:octocat   # 管理员自己\ngoogle:mom@example.com')
await mark(page, item(dlg, '共享用户').locator('.el-checkbox'))
await shot(page, `${SLUG}-auth`, await region(item(dlg, '认证'), item(dlg, '第三方登录')))

// ---------- 2. IP 访问控制与优先拒绝名单 ----------
const ipItem = item(dlg, 'IP 访问控制')
await ipItem.getByText('白名单', { exact: true }).click()
await page.waitForTimeout(300)
await item(dlg, 'IP / 网段').locator('textarea').fill('192.168.1.0/24\n2001:db8:1234::/48\n203.0.113.8')
await pick(item(dlg, '订阅列表').locator('.el-select'), '办公室出口')
await page.keyboard.press('Escape')
await page.waitForTimeout(200)
await item(dlg, '优先拒绝名单').locator('textarea').fill('198.51.100.0/24')
await mark(page, ipItem.locator('.el-radio-group'))
await shot(page, `${SLUG}-ip-access`, await region(ipItem, item(dlg, '拒绝订阅列表')))

// ---------- 3. 认证范围、路径规则与国家过滤 ----------
// 这一张单独演示路径规则：把站点 IP 访问控制恢复为关闭，免得与上一步的白名单混在一起
await ipItem.getByText('关闭', { exact: true }).click()
await item(dlg, '优先拒绝名单').locator('textarea').fill('')
const rulesItem = item(dlg, '路径规则')
await rulesItem.getByRole('button', { name: '添加规则' }).click()
await rulesItem.getByRole('button', { name: '添加规则' }).click()
await page.waitForTimeout(300)
const rows = rulesItem.locator('.row')
// 规则一：/api 无需认证（App 用自己的令牌访问）
await rows.nth(0).locator('input').first().fill('/api')
await pick(rows.nth(0).locator('.el-select').nth(0), '无需认证')
// 规则二：/admin 只允许内网
await rows.nth(1).locator('input').first().fill('/admin')
await pick(rows.nth(1).locator('.el-select').nth(1), 'IP 白名单')
await page.waitForTimeout(300)
await rows.nth(1).locator(".flex input").last().fill('192.168.1.0/24')
await page.keyboard.press('Tab')
const countryItem = item(dlg, '国家过滤')
await countryItem.getByText('仅允许', { exact: true }).click()
await page.waitForTimeout(300)
const codes = item(dlg, '国家代码').locator('input').first()
await codes.fill('CN, HK')
await page.keyboard.press('Tab')
await page.waitForTimeout(300)
await shot(page, `${SLUG}-path-country`, await region(item(dlg, '认证范围'), item(dlg, '国家代码')))

// ---------- 4. 拒绝方式、后台登录免认证与信任已登录用户 ----------
const denyItem = item(dlg, '拒绝方式')
await denyItem.getByText('断开连接', { exact: true }).click()
await item(dlg, '信任已登录用户').locator('.el-checkbox').click()
await mark(page, denyItem.locator('.el-radio-group'))
await mark(page, item(dlg, '信任已登录用户').locator('.el-checkbox'))
await shot(page, `${SLUG}-deny-trust`, await region(denyItem, item(dlg, '信任已登录用户')))
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// ---------- 5. 认证用户页：共享用户、单点登录域与 Passkey ----------
await go(page, 'webusers')
const usersCard = page.locator('.g-card').filter({ hasText: '单点登录域' }).first()
await usersCard.locator('input[placeholder="example.com"]').fill('example.com')
const switches = usersCard.locator('.el-switch')
// 打开「Passkey 登录」（第二个开关）
await switches.nth(1).click()
await page.waitForTimeout(300)
await mark(page, usersCard.getByRole('button', { name: '添加共享用户' }))
await shot(page, `${SLUG}-webusers`, usersCard)

// ---------- 6. 编辑共享用户并启用 TOTP ----------
await usersCard.getByRole('button', { name: '编辑' }).first().click()
await page.waitForTimeout(500)
const ed = page.locator('.el-dialog:visible').filter({ hasText: '编辑共享用户' }).first()
await ed.getByRole('button', { name: '启用 TOTP' }).click()
await page.waitForTimeout(800)
// 二维码弹窗关掉后，编辑框里显示「已启用 TOTP」
await page.locator('.el-dialog:visible').filter({ hasText: '的 TOTP' }).last().locator('img').first().waitFor()
await page.locator('.el-dialog:visible').filter({ hasText: '的 TOTP' }).last().getByRole('button', { name: '关闭' }).last().click()
await page.waitForTimeout(500)
await mark(page, item(ed, 'TOTP').locator('.totp'))
await shot(page, `${SLUG}-totp`, ed)
await ed.getByRole('button', { name: '取消' }).click()
await page.waitForTimeout(500)

// ---------- 7. Web 服务全局设置：第三方登录 ----------
// 认证用户页有未保存的修改，离开会弹出确认；直接换一个新标签页
page.on('dialog', (d) => d.accept())
await page.close({ runBeforeUnload: false })
page = await ctx.newPage()
await go(page, 'web')
await page.getByRole('button', { name: '全局设置' }).click()
await page.waitForTimeout(800)
const sd = page.locator('.el-dialog:visible').filter({ hasText: 'Web 服务全局设置' }).first()
await item(sd, '回调地址').locator('input').fill('https://auth.example.com')
await sd.getByRole('button', { name: '添加登录方式' }).click()
await page.waitForTimeout(300)
const prov = sd.locator('.provider').first()
await prov.locator('input[placeholder="标识，如 github"]').fill('github')
await prov.locator('input[placeholder="显示名"]').fill('GitHub')
await prov.locator('input').nth(3).fill('Ov23liEXAMPLEclientid')
await prov.locator('input').nth(4).fill('example-client-secret-0123456789abcdef')
await page.waitForTimeout(200)
const thirdDivider = sd.locator('.el-divider', { hasText: '第三方登录' })
await shot(page, `${SLUG}-oauth`, await region(thirdDivider, sd.getByRole('button', { name: '添加登录方式' })))

// ---------- 8. 订阅 IP 列表 ----------
await sd.getByRole('button', { name: '添加 IP 列表' }).click()
await page.waitForTimeout(500)
const ld = page.locator('.el-dialog:visible').filter({ hasText: '添加 IP 列表' }).last()
await item(ld, '名称').locator('input').fill('广东省')
await item(ld, '下载地址').locator('input').fill('https://metowolf.github.io/iplist/data/cncity/440000.txt')
await page.waitForTimeout(200)
await shot(page, `${SLUG}-ip-list`, ld)

await browser.close()
