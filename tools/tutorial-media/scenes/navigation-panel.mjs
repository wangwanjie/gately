// 教程「搭建导航面板」截图。管理界面部分来自演示实例；访客页面部分使用 navigation-panel.pages/ 下预先渲染的页面
// （由 webservice 模块的面板渲染函数按示例面板生成，演示实例不对外提供面板站点）。
import { chromium } from 'playwright-core'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { open, go, shot, mark } from '../lib.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const pages = path.join(here, 'navigation-panel.pages')

const { browser, page } = await open()

// 1. 面板列表
await go(page, 'web/panels')
await shot(page, 'navigation-panel-list')

// 2. 新建面板：页面设置
await page.getByRole('button', { name: '新建面板' }).click()
await page.waitForTimeout(800)
let dlg = page.locator('.el-dialog:visible').first()
await dlg.getByLabel('面板名称').fill('家庭服务导航')
await dlg.getByLabel('页面标题').fill('家里的服务')
await dlg.getByLabel('副标题').fill('常用服务一页直达')
await page.waitForTimeout(300)
await shot(page, 'navigation-panel-new', dlg)

// 3. 添加链接
await dlg.locator('.el-tabs__item', { hasText: '链接' }).click()
await page.waitForTimeout(400)
await dlg.getByRole('button', { name: '添加链接' }).click()
await page.waitForTimeout(800)
const link = page.locator('.el-dialog:visible').last()
await link.locator('input[data-test=link-name], [data-test=link-name] input').fill('Jellyfin')
await link.locator('.el-form-item', { hasText: '分组' }).locator('input').first().fill('存储与影音')
await link.locator('input[data-test=link-name], [data-test=link-name] input').click()
await link.locator('input[data-test=link-url], [data-test=link-url] input').fill('https://jellyfin.example.com')
await link.locator('input[data-test=link-lan-url], [data-test=link-lan-url] input').fill('http://192.168.1.20:8096')
await link.locator('[data-test=alt-add]').click()
await page.waitForTimeout(300)
await link.locator('input[data-test=alt-name], [data-test=alt-name] input').last().fill('IPv6')
await link.locator('input[data-test=alt-url], [data-test=alt-url] input').last().fill('https://jellyfin6.example.com:8443')
await link.locator('.el-form-item', { hasText: '描述' }).locator('input').first().fill('家庭影院')
await link.locator('input[placeholder*="NAS 或"]').fill('builtin:jellyfin')
await page.waitForTimeout(400)
await mark(page, link.getByRole('button', { name: '获取图标' }))
await shot(page, 'navigation-panel-link', link)
await link.getByRole('button', { name: '取消' }).click()
await page.waitForTimeout(400)

// 4. 布局
await dlg.locator('.el-tabs__item', { hasText: '布局' }).click()
await page.waitForTimeout(500)
await dlg.locator('[data-test=layout-style-app]').click()
await page.waitForTimeout(300)
await shot(page, 'navigation-panel-layout', dlg)
await dlg.getByRole('button', { name: '取消' }).click()
await page.waitForTimeout(500)

// 5. 从 Sun-Panel 导入
await page.getByRole('button', { name: '从 Sun-Panel 导入' }).click()
await page.waitForTimeout(800)
await shot(page, 'navigation-panel-sunpanel', page.locator('.el-dialog:visible').last())
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 6. 添加站点，类型选导航面板
await go(page, 'web')
await page.getByRole('button', { name: '添加站点' }).first().click()
await page.waitForTimeout(800)
const site = page.locator('.el-dialog:visible').last()
await site.locator('textarea').first().fill('home.example.com')
await site.getByLabel('名称').fill('导航面板')
await site.getByText('导航面板', { exact: true }).first().click()
await page.waitForTimeout(500)
await site.locator('.el-form-item', { hasText: '选择导航面板' }).locator('.el-select').click().catch(async () => {
  await site.getByPlaceholder('选择导航面板').click()
})
await page.waitForTimeout(400)
await page.locator('.el-select-dropdown__item:visible', { hasText: '家庭服务导航' }).first().click()
await page.waitForTimeout(400)
await site.locator('.el-dialog__body').evaluate((el) => { el.scrollTop = 0 })
await shot(page, 'navigation-panel-site', site)
await browser.close()

// 访客页面：直接在浏览器里打开预先渲染的页面
const b2 = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const ctx = await b2.newContext({ viewport: { width: 1280, height: 720 }, locale: 'zh-CN', deviceScaleFactor: 2 })
const vp = await ctx.newPage()
async function show(file) {
  await vp.unroute('https://home.example.com/**').catch(() => {})
  await vp.route('https://home.example.com/**', (r) => r.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: fs.readFileSync(path.join(pages, file)) }))
  await vp.goto('https://home.example.com/')
  await vp.waitForTimeout(800)
}
await show('card-wan.html')
await shot(vp, 'navigation-panel-visitor')
await show('app-wan.html')
await vp.locator('a.hit[title=Jellyfin]').first().click()
await vp.waitForTimeout(600)
await shot(vp, 'navigation-panel-app')
await b2.close()
