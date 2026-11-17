// 教程「申请 HTTPS 证书」的截图：添加证书（DNS-01 + Cloudflare、高级设置、HTTP-01）、续签设置、证书卡片、
// 自动部署、把 HTTP 监听升级为 HTTPS 并跳转，以及升级后站点行显示的证书。
// 监听列表替换成教程里的示例监听（局域网 HTTP :80 → 升级为 HTTPS :443），保存请求由 fakeWrites 拦截。
import { BASE, open, go, shot, mark, fakeWrites } from '../lib.mjs'

const { browser, page } = await open({ height: 1900 })

async function fresh(route) {
  await page.goto('about:blank')
  await go(page, route)
}

// fakeWrites 先注册：Playwright 按注册的倒序匹配路由，下面模拟 GET 的处理器先于它执行
await fakeWrites(page)

const all = await (await page.request.get(`${BASE}/api/webservice/listeners`)).json()
const tpl = all.find((l) => !l.tls && l.sites?.length)
const site = {
  ...tpl.sites[0],
  key: 's-nas',
  name: 'NAS',
  domains: ['nas.example.com'],
  type: 'proxy',
  targets: ['http://192.168.1.10:5000'],
  text: '',
  statusCode: 0,
  contentType: '',
  compress: false,
  forwardedHeaders: true,
  keepPath: true,
  accessLog: true,
  remark: ''
}
let upgraded = false
function listener() {
  return {
    ...tpl,
    key: 'l-lan',
    name: upgraded ? '局域网 HTTPS' : '局域网 HTTP',
    remark: '',
    port: upgraded ? 443 : 80,
    tls: upgraded,
    minTls: '1.2',
    // 升级后站点行显示匹配到的证书（与教程中申请的 nas.example.com 证书一致，约 85 天后到期）
    siteCerts: upgraded ? { 's-nas': [{ domain: 'nas.example.com', name: 'nas.example.com', notAfter: new Date(Date.now() + 85 * 86400e3).toISOString() }] } : {},
    httpPort: upgraded ? 80 : 0,
    httpPortMode: upgraded ? 'redirect' : '',
    default: { ...tpl.default, enable: false, type: 'proxy', targets: null, name: '' },
    sites: [site],
    stats: { default: { active: 0, errors: 0, requests: 0 }, 's-nas': { active: 1, errors: 3, requests: 1286 } },
    upstreams: {}
  }
}
await page.route('**/api/webservice/listeners', async (route) => {
  if (route.request().method() !== 'GET') {
    return route.fallback()
  }
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([listener()]) })
})

const dlg = () => page.locator('.el-dialog:visible').last()

// 打开「添加证书」并选择 DNS 服务商
async function openAdd(provider) {
  await fresh('ssl')
  await page.getByRole('button', { name: '添加证书' }).first().click()
  await dlg().waitFor()
  await dlg().locator('textarea').first().fill(provider === 'http' ? 'nas.example.com\nwww.example.com' : 'nas.example.com')
  if (provider !== 'http') {
    await dlg().locator('.el-select').first().click()
    await page.waitForTimeout(300)
    await page.locator('.el-select-dropdown__item:visible', { hasText: new RegExp(`^${provider}$`) }).first().click()
    await page.waitForTimeout(300)
  }
}

// 1. DNS-01 + Cloudflare
{
  await openAdd('Cloudflare')
  await dlg().locator('.el-form-item', { hasText: 'API Token' }).locator('input').first().fill('cf-token-for-gately-example')
  await page.waitForTimeout(300)
  await shot(page, 'https-certificate-dns', dlg())
}

// 2. 高级设置：账户邮箱、CA、网络
{
  await dlg().getByText('高级设置').click()
  await page.waitForTimeout(300)
  await dlg().locator('.el-form-item', { hasText: '账户邮箱' }).locator('input').fill('me@example.net')
  await dlg().locator('.el-form-item', { hasText: '名称（文件名）' }).locator('input').fill('nas.example.com')
  await page.waitForTimeout(300)
  await shot(page, 'https-certificate-advanced', dlg())
  await page.keyboard.press('Escape')
}

// 3. HTTP-01
{
  await openAdd('http')
  await dlg().getByText('HTTP-01（无需 DNS 服务商）').click()
  await page.waitForTimeout(400)
  await shot(page, 'https-certificate-http01', dlg())
  await page.keyboard.press('Escape')
}

// 4. 续签设置
{
  await fresh('ssl')
  await page.getByRole('button', { name: '续签设置' }).click()
  await dlg().waitFor()
  await page.waitForTimeout(400)
  await shot(page, 'https-certificate-renew', dlg())
  await page.keyboard.press('Escape')
}

// 5. 证书卡片
{
  await fresh('ssl')
  const card = page.locator('.entity').filter({ hasText: '博客证书' }).first()
  await card.waitFor()
  await shot(page, 'https-certificate-card', card)
}

// 6. 自动部署：复制到本机目录
{
  await openAdd('Cloudflare')
  await dlg().getByRole('button', { name: '添加部署目标' }).click()
  await page.waitForTimeout(400)
  const local = page.locator('.el-dropdown-menu__item:visible, .el-select-dropdown__item:visible', { hasText: '复制到本机目录' })
  if (await local.count()) {
    await local.first().click()
    await page.waitForTimeout(400)
  }
  const sec = dlg().locator('.el-form-item', { hasText: '本机目录' }).first()
  await sec.locator('input').first().fill('/opt/nas-proxy/certs')
  const names = dlg().locator('.deploy-box .el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: /^文件名$/ }) }).locator('input')
  if (await names.count() >= 2) {
    await names.nth(0).fill('fullchain.pem')
    await names.nth(1).fill('privkey.pem')
  }
  const cmd = dlg().locator('.el-form-item', { hasText: '复制后执行' }).first().locator('input, textarea').first()
  await cmd.fill('docker restart nas-proxy')
  await page.waitForTimeout(400)
  await shot(page, 'https-certificate-deploy', dlg().locator('.deploy-box').first())
  await page.keyboard.press('Escape')
}

// 7. 把 HTTP 监听升级为 HTTPS：端口 443、开启 HTTPS、额外 HTTP 端口 80 跳转到 HTTPS
{
  await fresh('web')
  await page.getByRole('button', { name: '编辑监听' }).first().click()
  await dlg().waitFor()
  await dlg().getByLabel('名称').fill('局域网 HTTPS')
  const port = dlg().locator('.el-form-item', { hasText: '端口' }).first().locator('input')
  await port.fill('443')
  await port.press('Tab')
  await dlg().locator('.el-form-item', { hasText: 'HTTPS' }).first().locator('.el-switch').click()
  await dlg().getByText('高级设置').click()
  await page.waitForTimeout(300)
  const hp = dlg().locator('.el-form-item', { hasText: 'HTTP 端口' }).locator('input').first()
  await hp.fill('80')
  await hp.press('Tab')
  await page.waitForTimeout(400)
  await mark(page, dlg().locator('.el-form-item', { hasText: 'HTTP 端口' }).locator('.access-row'))
  await shot(page, 'https-certificate-listener', dlg())
  await page.keyboard.press('Escape')
}

// 8. 升级后的监听卡片：站点行显示匹配到的证书与剩余天数
{
  upgraded = true
  await fresh('web')
  const card = page.locator('.entity').filter({ hasText: '局域网 HTTPS' }).first()
  await card.waitFor()
  await page.waitForTimeout(800)
  await shot(page, 'https-certificate-site', card)
}

await browser.close()
