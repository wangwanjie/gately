// 用法：node shots.mjs <locale> <outDir> [scene,scene...]
import { chromium } from 'playwright-core'
import fs from 'fs'
const [locale, outDir, only] = process.argv.slice(2)
const BASE = 'http://127.0.0.1:16750'
const scenes = {
  overview: async (p) => { await go(p, 'overview'); await p.waitForTimeout(2500) },
  web: async (p) => { await go(p, 'web'); await p.waitForTimeout(2500) },
  'site-stats': async (p) => {
    await go(p, 'web'); await p.waitForTimeout(1500)
    await p.locator('.site').nth(1).locator('button:has(svg[class*="chart"], svg[class*="bar"])').first().click()
    await p.waitForTimeout(2500)
  },
  protoroute: async (p) => { await go(p, 'protoroute'); await p.waitForTimeout(2000) },
  ddns: async (p) => { await go(p, 'ddns'); await p.waitForTimeout(2500) },
  guard: async (p) => { await go(p, 'guard'); await p.waitForTimeout(2500) },
  frp: async (p) => { await go(p, 'frp'); await p.waitForTimeout(2500) },
  docker: async (p) => { await go(p, 'docker'); await p.waitForTimeout(3000) },
  terminal: async (p) => {
    await go(p, 'terminal'); await p.waitForTimeout(1500)
    await p.locator('.conn-ops .el-button--primary').first().click()
    await p.waitForTimeout(2500)
    await p.locator('.xterm').first().click(); await p.waitForTimeout(500)
    await p.keyboard.type('cd /opt/stacks && ls\n'); await p.waitForTimeout(800)
    await p.keyboard.type('docker ps --format "table {{.Names}}\\t{{.Image}}\\t{{.Status}}"\n'); await p.waitForTimeout(1500)
    await p.keyboard.type('df -h /srv/data | tail -1\n'); await p.waitForTimeout(800)
    await p.keyboard.type('uptime\n'); await p.waitForTimeout(1200)
  },
  'config-history': async (p) => {
    await go(p, 'history'); await p.waitForTimeout(1500)
    await p.locator('.el-select').first().click(); await p.locator('.el-select-dropdown__item', { hasText: 'webservice.json' }).first().click(); await p.waitForTimeout(1200)
    await p.locator('tbody tr').first().locator('button').first().click()
    await p.waitForTimeout(2000)
  },
  'api-docs': async (p) => { await go(p, 'api-docs'); await p.waitForTimeout(2500) },
  webtopology: async (p) => { await go(p, 'webtopology'); await p.waitForTimeout(4000) },
}
async function go(p, route) {
  await p.goto(`${BASE}/#/${route}`)
  await p.waitForLoadState('networkidle', { timeout: 2000 }).catch(() => {})
}
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, locale })
await ctx.addInitScript((l) => { localStorage.setItem('gately.locale', l); localStorage.setItem('gately.theme.mode', 'light'); localStorage.setItem('gately.compact.docker.containers', '1') }, locale)
const page = await ctx.newPage()
await page.goto(`${BASE}/#/login`)
await page.waitForSelector('input[autocomplete=username]')
await page.fill('input[autocomplete=username]', 'admin')
await page.fill('input[type=password]', 'Gately-Demo-2026')
await page.keyboard.press('Enter')
await page.waitForURL(/#\/overview/, { timeout: 15000 })
fs.mkdirSync(outDir, { recursive: true })
for (const name of Object.keys(scenes)) {
  if (only && !only.split(',').includes(name)) { continue }
  await page.goto(`${BASE}/#/overview`); await page.waitForTimeout(300)
  await scenes[name](page)
  await page.screenshot({ path: `${outDir}/${name}.png` })
  console.log('ok', locale, name)
}
await browser.close()
