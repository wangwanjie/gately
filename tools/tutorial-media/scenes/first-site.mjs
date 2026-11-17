// 教程「第一个反向代理站点」的截图：添加 HTTP 监听、添加反向代理站点、建好后的监听卡片、快速建站、站点统计与访问日志。
// 演示实例里已有很多监听，为了让读者看到「刚装好时只有一个监听」的样子，本脚本把监听列表接口替换成一个
// 示例监听（局域网 HTTP :80，站点 nas.example.com → 192.168.1.10:5000）；统计数据借用演示里 NAS 站点的真实数据，
// 访问日志为示例数据。演示实例只读，保存请求由 fakeWrites 拦截。
import { BASE, open, go, shot, mark, fakeWrites } from '../lib.mjs'

const { browser, page } = await open()

async function fresh(route) {
  await page.goto('about:blank')
  await go(page, route)
}

// 读取演示数据，挑一个明文 HTTP 监听作为模板
const all = await (await page.request.get(`${BASE}/api/webservice/listeners`)).json()
const tpl = all.find((l) => !l.tls && l.sites?.length)
const nasHost = all.find((l) => l.sites?.some((s) => s.domains.includes('nas.example.com')))
const nasSite = nasHost.sites.find((s) => s.domains.includes('nas.example.com'))
const realStats = await (await page.request.get(`${BASE}/api/webservice/stats?listener=${nasHost.key}&site=${nasSite.key}&minutes=60`)).json()
// 借用真实的分钟曲线，汇总数改成局域网站点的样子：来源都是内网地址，没有拦截与国家
const sum = (k) => realStats.series.reduce((a, p) => a + (p[k] || 0), 0)
const stats = {
  ...realStats,
  blocked: { ban: 0, country: 0, ip: 0, rateLimit: 0, waf: 0 },
  bytesOut: sum('bytes'),
  status: { '2xx': sum('requests') - sum('s4xx') - sum('s5xx'), '3xx': 0, '4xx': sum('s4xx'), '5xx': sum('s5xx') },
  topCountries: [],
  topIps: [
    { key: '192.168.1.20', count: Math.round(sum('requests') * 0.7) },
    { key: '192.168.1.23', count: Math.round(sum('requests') * 0.25) },
    { key: '192.168.1.31', count: Math.round(sum('requests') * 0.05) }
  ],
  upstreams: [{ ...realStats.upstreams[0], url: 'http://192.168.1.10:5000', requests: sum('requests'), bytes: sum('bytes'), conns: 1 }]
}

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
function listener(withSite) {
  return {
    ...tpl,
    key: 'l-lan',
    name: '局域网 HTTP',
    remark: '',
    port: 80,
    default: { ...tpl.default, enable: false, type: 'proxy', targets: null, name: '' },
    sites: withSite ? [site] : [],
    stats: withSite ? { default: { active: 0, errors: 0, requests: 0 }, 's-nas': { active: 1, errors: 3, requests: 1286 } } : {},
    upstreams: {}
  }
}
// fakeWrites 先注册：Playwright 按注册的倒序匹配路由，下面模拟 GET 的处理器先于它执行
await fakeWrites(page)
let withSite = false
await page.route('**/api/webservice/listeners', async (route) => {
  if (route.request().method() !== 'GET') {
    return route.fallback()
  }
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([listener(withSite)]) })
})
await page.route('**/api/webservice/stats?*', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(stats) }))

// 示例访问日志：只在首次加载时返回，后续轮询返回空
const now = Date.now()
const ua = '"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15"'
const lines = [
  ['192.168.1.20', 'GET', '/', 200, 18],
  ['192.168.1.20', 'GET', '/webman/login.cgi', 200, 42],
  ['192.168.1.20', 'POST', '/webapi/entry.cgi', 200, 65],
  ['192.168.1.23', 'GET', '/favicon.ico', 404, 3],
  ['192.168.1.23', 'GET', '/webapi/query.cgi?api=SYNO.API.Info', 200, 9],
  ['192.168.1.20', 'GET', '/webman/modules/FileBrowser/index.html', 200, 27]
]
const entries = lines.map(([ip, m, p, st, ms], i) => ({
  seq: 1000 + i,
  time: new Date(now - (lines.length - i) * 37000).toISOString(),
  level: st >= 400 ? 'warn' : 'info',
  source: '局域网 HTTP',
  message: `${ip} ${m} nas.example.com${p} ${st} ${ms}ms "-" ${ua}`,
  fields: { ip, host: 'nas.example.com', site: 'NAS', method: m, path: p.split('?')[0], status: String(st), listener: 'l-lan' }
}))
await page.route('**/api/webservice/logs?*', async (route) => {
  const u = new URL(route.request().url())
  const first = u.searchParams.get('since') === '0' || u.searchParams.has('before')
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(first && !u.searchParams.has('before') ? entries : []) })
})
// 1. 添加监听：HTTP 80
{
  await fresh('web')
  await mark(page, page.getByRole('button', { name: '添加监听' }).first())
  await shot(page, 'first-site-toolbar', page.locator('.page-head, .page-header').first())
  await page.getByRole('button', { name: '添加监听' }).first().click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  await dlg.getByLabel('名称').fill('局域网 HTTP')
  const port = dlg.locator('.el-input-number input').first()
  await port.fill('80')
  await port.press('Tab')
  await dlg.locator('.el-switch').first().click()
  await page.waitForTimeout(400)
  await shot(page, 'first-site-listener', dlg)
  await page.keyboard.press('Escape')
}

// 2. 添加站点：nas.example.com → http://192.168.1.10:5000
{
  await fresh('web')
  await page.getByRole('button', { name: '添加站点' }).first().click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  await dlg.locator('textarea').first().fill('nas.example.com')
  await dlg.getByLabel('名称').fill('NAS')
  await dlg.locator('textarea').nth(1).fill('http://192.168.1.10:5000')
  const logSwitch = dlg.locator('.el-form-item', { hasText: '访问日志' }).locator('.el-switch')
  await logSwitch.click()
  await mark(page, logSwitch)
  await page.waitForTimeout(400)
  await shot(page, 'first-site-site', dlg)
  await page.keyboard.press('Escape')
}

// 3. 建好后的监听卡片
withSite = true
{
  await fresh('web')
  const card = page.locator('.entity').filter({ hasText: '局域网 HTTP' }).first()
  await card.waitFor()
  await shot(page, 'first-site-card', card)
}

// 4. 站点统计
{
  await page.getByTitle('统计').first().click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  await page.waitForTimeout(1200)
  await shot(page, 'first-site-stats', dlg)
  await page.keyboard.press('Escape')
  await page.waitForTimeout(500)
}

// 5. 站点访问日志
{
  await page.getByTitle('日志').first().click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  await dlg.getByText('nas.example.com/webman/login.cgi').first().waitFor()
  await page.waitForTimeout(600)
  await shot(page, 'first-site-logs', dlg)
  await page.keyboard.press('Escape')
}

// 6. 快速建站第二步：填后端地址，可选常见应用模板
{
  withSite = false
  await fresh('web')
  await page.getByRole('button', { name: '快速建站' }).click()
  const dlg = page.locator('.el-dialog:visible').last()
  await dlg.waitFor()
  await dlg.locator('input').first().fill('nas.example.com')
  await dlg.getByRole('button', { name: '下一步' }).click()
  await page.waitForTimeout(500)
  await dlg.locator('input:visible').first().fill('http://192.168.1.10:5000')
  await page.waitForTimeout(400)
  await shot(page, 'first-site-quick', dlg)
  await page.keyboard.press('Escape')
}

await browser.close()
