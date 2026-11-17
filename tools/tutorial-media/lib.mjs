// 官网教程截图与录屏的公共工具：用本机 Chrome 驱动本地演示实例（demo 模式，只读，数据由 tools/demo 生成）。
// 环境变量：
//   GATELY_URL   演示实例地址，默认 http://127.0.0.1:16780
//   GATELY_USER  / GATELY_PASS 管理员账号，默认 admin / Tut-Demo-2026!
//   TUT_OUT      截图输出目录，默认 <仓库>/website/public/tutorials
import { chromium } from 'playwright-core'
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
export const BASE = process.env.GATELY_URL || 'http://127.0.0.1:16780'
const USER = process.env.GATELY_USER || 'admin'
const PASS = process.env.GATELY_PASS || 'Tut-Demo-2026!'
export const OUT = process.env.TUT_OUT || path.resolve(here, '../../website/public/tutorials')
const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

// 截图时隐藏演示横幅等与教程无关的元素，界面看起来与正常安装一致
const HIDE_CSS = `.demo-banner,.el-message{display:none!important}`

async function prepare(ctx, locale) {
  await ctx.addInitScript(({ l, css }) => {
    localStorage.setItem('gately.locale', l)
    localStorage.setItem('gately.theme.mode', 'light')
    const add = () => {
      if (document.getElementById('tut-hide')) {
        return
      }
      const st = document.createElement('style')
      st.id = 'tut-hide'
      st.textContent = css
      document.head.appendChild(st)
    }
    if (document.head) {
      add()
    } else {
      document.addEventListener('DOMContentLoaded', add)
    }
  }, { l: locale, css: HIDE_CSS })
}

// 打开浏览器并以管理员登录，返回 { browser, ctx, page }
export async function open({ locale = 'zh-CN', width = 1440, height = 900, video = null, mobile = false } = {}) {
  const browser = await chromium.launch({ executablePath: CHROME, headless: true })
  const login = await browser.newContext({ viewport: { width, height }, locale })
  await prepare(login, locale)
  const lp = await login.newPage()
  await lp.goto(`${BASE}/#/login`)
  await lp.fill('input[autocomplete=username]', USER)
  await lp.fill('input[type=password]', PASS)
  await lp.keyboard.press('Enter')
  await lp.waitForURL(/#\/(?!login)/, { timeout: 15000 })
  const state = await login.storageState()
  await login.close()
  const opts = { viewport: { width, height }, locale, storageState: state, deviceScaleFactor: 2 }
  if (mobile) {
    Object.assign(opts, { isMobile: true, hasTouch: true, deviceScaleFactor: 3 })
  }
  if (video) {
    fs.mkdirSync(video, { recursive: true })
    opts.recordVideo = { dir: video, size: { width, height } }
    opts.deviceScaleFactor = 1
  }
  const ctx = await browser.newContext(opts)
  await prepare(ctx, locale)
  const page = await ctx.newPage()
  return { browser, ctx, page }
}

// 跳转到某个页面（hash 路由，例如 'webservice'），等网络空闲与动画结束
export async function go(page, route) {
  await page.goto(`${BASE}/#/${route.replace(/^\/?#?\/?/, '')}`)
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(800)
}

// 截图并转成 webp：name 不含扩展名，输出 <OUT>/<name>.zh-CN.webp。
// target 可以是选择器字符串、Locator，或不传（整个视口）；pad 为元素四周留白像素
export async function shot(page, name, target, { pad = 12, locale = 'zh-CN', width = 2000 } = {}) {
  fs.mkdirSync(OUT, { recursive: true })
  const png = path.join('/tmp', `tut-${name}.png`)
  if (!target) {
    await page.screenshot({ path: png })
  } else {
    const loc = typeof target === 'string' ? page.locator(target).first() : target
    await loc.scrollIntoViewIfNeeded()
    await page.waitForTimeout(300)
    const box = await loc.boundingBox()
    if (!box) {
      throw new Error(`截图目标不可见：${name}`)
    }
    const vp = page.viewportSize()
    const x = Math.max(0, box.x - pad)
    const y = Math.max(0, box.y - pad)
    await page.screenshot({
      path: png,
      clip: { x, y, width: Math.min(vp.width - x, box.width + pad * 2), height: Math.min(vp.height - y, box.height + pad * 2) }
    })
  }
  const webp = path.join(OUT, `${name}.${locale}.webp`)
  const args = ['-quiet', '-q', '80']
  // 只缩小不放大：2 倍图宽度超过 width 时缩到 width
  const w = Number(execFileSync('sips', ['-g', 'pixelWidth', png]).toString().match(/pixelWidth: (\d+)/)[1])
  if (w > width) {
    args.push('-resize', String(width), '0')
  }
  execFileSync('cwebp', [...args, png, '-o', webp])
  fs.rmSync(png)
  console.log('已生成', path.relative(process.cwd(), webp))
  return webp
}

// 演示模式拒绝一切写操作。需要展示「保存成功」的界面时调用：拦截页面发出的写请求（POST/PUT/DELETE），
// 原样把请求体当作成功响应返回，界面会按保存成功处理（刷新页面后恢复原数据）。登录与退出请求不拦截。
export async function fakeWrites(page) {
  await page.route('**/api/**', async (route) => {
    const req = route.request()
    const url = req.url()
    if (req.method() === 'GET' || /\/api\/(login|logout|auth)/.test(url)) {
      return route.continue()
    }
    let body = {}
    try {
      body = req.postDataJSON() ?? {}
    } catch {
      body = {}
    }
    if (body && typeof body === 'object' && !Array.isArray(body) && !body.key) {
      body.key = Math.random().toString(16).slice(2, 14)
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body ?? {}) })
  })
}

// 在截图前给元素加醒目的描边，标出要点的按钮或字段
export async function mark(page, target, color = '#e8590c') {
  const loc = typeof target === 'string' ? page.locator(target).first() : target
  await loc.evaluate((el, c) => {
    el.style.outline = `3px solid ${c}`
    el.style.outlineOffset = '3px'
    el.style.borderRadius = el.style.borderRadius || '6px'
  }, color)
}
