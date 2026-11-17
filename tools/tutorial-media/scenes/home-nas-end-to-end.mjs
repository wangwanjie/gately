// 教程「家庭 NAS 外网访问全流程」的截图：DDNS 任务、HTTPS 监听与站点、添加站点（基本与安全）、
// 入侵防护模板、通知规则、导航面板、定时备份
import { open, go, shot, mark } from '../lib.mjs'

const S = 'home-nas-end-to-end'
const { browser, page } = await open()
const card = (text) => page.locator('.entity').filter({ hasText: text }).last()

// 1. 动态域名任务卡片
await go(page, 'ddns')
await shot(page, `${S}-ddns`, card('Cloudflare：example.com'))

// 2. HTTPS 监听与其下的站点
await go(page, 'web')
const listener = page.locator('.entity').filter({ hasText: '公网 HTTPS 入口' }).first()
await shot(page, `${S}-listener`, listener)

// 3. 在监听上添加站点：文件服务
await listener.getByRole('button', { name: '添加站点' }).click()
const dlg = page.locator('.el-dialog:visible').last()
await dlg.waitFor()
await page.waitForTimeout(400)
await dlg.getByPlaceholder('每行一个；支持 *.example.com、api-*.example.com 与 *').fill('files.example.com')
await dlg.getByPlaceholder('http://192.168.1.10:8080，多个时轮询').fill('http://192.168.1.10:8080')
const nameItem = dlg.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: /^名称$/ }) }).first()
await nameItem.locator('input').fill('文件').catch(() => {})
await page.waitForTimeout(300)
// 基本页只保留前面几项，避免对话框过长
const keepBasic = ['所属监听', '域名', '名称', '分组', '类型', '后端地址', '选项']
await dlg.locator('.el-tab-pane:visible .el-form-item').evaluateAll((els, k) => {
  for (const e of els) {
    const label = e.querySelector(':scope > .el-form-item__label')?.innerText.trim() || ''
    if (label && !k.includes(label)) {
      e.style.display = 'none'
    }
  }
}, keepBasic)
await shot(page, `${S}-site-basic`, dlg.locator('.el-tab-pane:visible').first())

// 4. 安全页：网页认证 + 共享用户 + WAF 实例
await dlg.getByRole('tab', { name: '安全' }).click()
await page.waitForTimeout(500)
await dlg.getByText('网页认证（浏览器显示登录页）').click()
await page.waitForTimeout(200)
await dlg.getByText('使用共享用户（支持 TOTP 与单点登录）').click()
const wafItem = dlg.locator('.el-form-item').filter({ has: page.locator('.el-form-item__label', { hasText: 'WAF 实例' }) }).first()
await wafItem.locator('.el-select').click()
await page.waitForTimeout(300)
await page.locator('.el-select-dropdown__item:visible').filter({ hasText: '站点防护' }).first().click()
await page.waitForTimeout(300)
const keepSec = ['认证', '共享用户', 'WAF 实例', '拒绝方式']
await dlg.locator('.el-tab-pane:visible .el-form-item').evaluateAll((els, k) => {
  for (const e of els) {
    const label = e.querySelector(':scope > .el-form-item__label')?.innerText.trim() || ''
    e.style.display = k.includes(label) ? '' : 'none'
  }
}, keepSec)
await mark(page, dlg.getByText('使用共享用户（支持 TOTP 与单点登录）'))
await mark(page, wafItem.locator('.el-select'))
await page.waitForTimeout(300)
await shot(page, `${S}-site-security`, dlg.locator('.el-tab-pane:visible').first())

// 5. 入侵防护：设置页（同步到系统防火墙、白名单）
await go(page, 'guard')
await page.getByRole('tab', { name: '设置' }).click()
await page.waitForTimeout(600)
const gpane = page.locator('.el-tab-pane:visible').last()
await gpane.getByPlaceholder(/./).last().fill('203.0.113.50').catch(() => {})
await mark(page, gpane.locator('.el-form-item').filter({ hasText: '同步到系统防火墙' }).first().locator('.el-switch'))
await shot(page, `${S}-guard`, gpane)

// 6. 通知规则
await go(page, 'notify')
await shot(page, `${S}-notify`, page.locator('section').filter({ hasText: '通知规则' }).last().locator('.grouped-list'))

// 7. 导航面板
await go(page, 'web/panels')
await card('家庭服务导航').getByRole('button', { name: '编辑' }).click()
await page.waitForTimeout(1000)
await shot(page, `${S}-panel`, page.locator('.el-dialog:visible').last())

// 8. 定时备份
await go(page, 'settings?tab=backup')
const bk = page.locator('.el-card').filter({ hasText: '定时备份' }).last()
// 演示实例里开关不可用，用描边标出要打开的位置
await mark(page, bk.locator('.el-card__header .el-switch'))
await page.waitForTimeout(300)
await shot(page, `${S}-backup`, bk)

await browser.close()
