// 用法：node topo.mjs <locale> <outDir>  录制拓扑页交互 webm
import { chromium } from 'playwright-core'
import fs from 'fs'
const [locale, outDir] = process.argv.slice(2)
const BASE = 'http://127.0.0.1:16750'
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const login = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale })
await login.addInitScript((l) => { localStorage.setItem('gately.locale', l); localStorage.setItem('gately.theme.mode', 'light') }, locale)
let p = await login.newPage()
await p.goto(`${BASE}/#/login`)
await p.fill('input[autocomplete=username]', 'admin'); await p.fill('input[type=password]', 'Gately-Demo-2026'); await p.keyboard.press('Enter')
await p.waitForURL(/overview/)
const state = await login.storageState()
await login.close()
fs.mkdirSync(outDir, { recursive: true })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale, storageState: state, recordVideo: { dir: outDir, size: { width: 1440, height: 900 } } })
await ctx.addInitScript((l) => { localStorage.setItem('gately.locale', l); localStorage.setItem('gately.theme.mode', 'light'); localStorage.removeItem('gately.topology.layout') }, locale)
p = await ctx.newPage()
await p.goto(`${BASE}/#/webtopology`)
await p.waitForTimeout(3500)
async function layout(i) {
  await p.locator('.btn-caret').first().locator('xpath=ancestor::button[1]').click()
  await p.waitForTimeout(700)
  await p.locator('.topo-layout-menu .el-dropdown-menu__item').nth(i).click()
  await p.locator('.zbtn').nth(3).click().catch(() => {})
  await p.waitForTimeout(2800)
}
// 点一个站点节点查看详情
await p.locator('g.node:has-text("Gitea")').first().click().catch(() => {})
await p.waitForTimeout(2600)
await layout(2) // 径向
await layout(3) // 桑基
await layout(0) // 回到树
await p.waitForTimeout(600)
const v = p.video()
await ctx.close()
fs.renameSync(await v.path(), `${outDir}/topology.webm`)
await browser.close()
console.log('done', locale)
