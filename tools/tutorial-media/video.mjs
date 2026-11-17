// 教程视频录制：在演示实例上用 Playwright 录屏，叠加字幕、模拟鼠标指针与片头片尾卡片，转成 mp4。
// 用法：在场景脚本里 import { record } from '../video.mjs'，再
//   await record('first-site', async (v) => { await v.go('web'); await v.cap('…'); … })
// 输出 <仓库>/website/public/videos/tutorials/<名称>.zh-CN.mp4 与同名 .jpg 封面（website/public/videos 不进 git，随 deploy:site 上传）。
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { open, go as goto, fakeWrites } from './lib.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
export const VIDEO_OUT = process.env.TUT_VIDEO_OUT || path.resolve(here, '../../website/public/videos/tutorials')

const LOGO = `<svg width="84" height="84" viewBox="0 0 32 32" fill="none"><defs><linearGradient id="g" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#18a888"/><stop offset="1" stop-color="#0b5f4f"/></linearGradient></defs><rect width="32" height="32" rx="9" fill="url(#g)"/><path d="M21.7 10.3A8 8 0 1 0 24 16h-7" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/><circle cx="23.6" cy="8.6" r="2.2" fill="#9ff3dc"/></svg>`

// 字幕、指针、卡片的样式与操作函数，注入到每个页面
function overlay() {
  const setup = () => {
    if (document.getElementById('v-cap')) {
      return
    }
    const st = document.createElement('style')
    st.textContent = `#v-cap{position:fixed;left:50%;bottom:30px;transform:translateX(-50%);z-index:99999;max-width:80%;padding:12px 24px;border-radius:12px;background:rgba(10,24,22,.88);color:#fff;font:600 22px/1.4 -apple-system,'PingFang SC','Segoe UI',sans-serif;text-align:center;opacity:0;transition:opacity .3s;pointer-events:none}
#v-step{position:fixed;left:24px;top:84px;z-index:99999;padding:6px 14px;border-radius:999px;background:#0e7c66;color:#fff;font:600 15px/1.4 -apple-system,'PingFang SC',sans-serif;opacity:0;transition:opacity .3s;pointer-events:none}
#v-cur{position:fixed;z-index:99998;width:22px;height:22px;border-radius:50%;background:rgba(232,89,12,.30);border:2px solid #e8590c;left:-40px;top:-40px;transition:left .6s ease,top .6s ease,transform .15s;pointer-events:none;margin:-11px 0 0 -11px}
#v-cur.click{transform:scale(.55)}
#v-card{position:fixed;inset:0;z-index:100000;background:linear-gradient(135deg,#0b3f35,#0e7c66 60%,#18a888);color:#fff;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:16px;text-align:center;font-family:-apple-system,'PingFang SC','Segoe UI',sans-serif;opacity:0;pointer-events:none;transition:opacity .45s}
#v-card h1{margin:0;font-size:60px}#v-card h2{margin:0;font-size:34px;font-weight:600}#v-card p{margin:0;font-size:22px;opacity:.9;max-width:980px;line-height:1.6}
#v-card ol{margin:6px 0 0;padding:0;list-style:none;font-size:22px;line-height:1.9;text-align:left;counter-reset:s}#v-card ol li{counter-increment:s}#v-card ol li::before{content:counter(s);display:inline-block;width:30px;height:30px;margin-right:12px;border-radius:50%;background:rgba(255,255,255,.2);text-align:center;line-height:30px;font-size:16px}
.demo-banner{display:none!important}
/* 录屏时关闭页面自身的过渡动画：弹窗淡入期间录屏帧不刷新，会出现弹窗迟迟不显示 */
*:not([id^=v-]),*:not([id^=v-])::before,*:not([id^=v-])::after{transition:none!important;animation-duration:0s!important}`
    document.head.appendChild(st)
    for (const id of ['v-cap', 'v-step', 'v-cur', 'v-card']) {
      const e = document.createElement('div')
      e.id = id
      document.body.appendChild(e)
    }
  }
  window.__cap = (t) => {
    const e = document.getElementById('v-cap')
    if (!t) {
      e.style.opacity = 0
      return
    }
    e.textContent = t
    e.style.opacity = 1
  }
  window.__step = (t) => {
    const e = document.getElementById('v-step')
    if (!t) {
      e.style.opacity = 0
      return
    }
    e.textContent = t
    e.style.opacity = 1
  }
  window.__cur = (x, y) => {
    const e = document.getElementById('v-cur')
    e.style.left = x + 'px'
    e.style.top = y + 'px'
  }
  window.__click = () => {
    const e = document.getElementById('v-cur')
    e.classList.add('click')
    setTimeout(() => e.classList.remove('click'), 180)
  }
  window.__card = (html) => {
    const e = document.getElementById('v-card')
    if (!html) {
      e.style.opacity = 0
      return
    }
    e.innerHTML = html
    e.style.opacity = 1
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', setup)
  } else {
    setup()
  }
}

// 录制一支视频。script(v) 里用 v 提供的方法编排画面；title / summary / steps 用于片头卡片
export async function record(name, { title, summary, steps = [], outro = '完整图文步骤见 gately.cn 教程页' }, script) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), `tutvid-${name}-`))
  // 不用 Playwright 自带录像：它的录屏帧有时不随弹窗出现而刷新。改为循环截图（截图会强制重绘），
  // 记录每帧时间戳，最后按真实间隔合成视频
  const { browser, ctx } = await open({ width: 1440, height: 900 })
  await ctx.addInitScript(overlay)
  const page = await ctx.newPage()
  await fakeWrites(page)
  const frames = []
  let recording = true
  let capturing = false
  const loop = (async () => {
    while (recording) {
      const t = Date.now()
      try {
        const buf = await page.screenshot({ type: 'jpeg', quality: 85, scale: 'css', animations: 'allow', caret: 'initial' })
        const f = path.join(tmp, `f${String(frames.length).padStart(6, '0')}.jpg`)
        fs.writeFileSync(f, buf)
        if (capturing) {
          frames.push({ f, t })
        } else {
          fs.rmSync(f)
        }
      } catch {
        // 页面跳转瞬间截图可能失败，跳过这一帧
      }
      const wait = 80 - (Date.now() - t)
      if (wait > 0) {
        await new Promise((r) => setTimeout(r, wait))
      }
    }
  })()
  const sleep = (ms) => page.waitForTimeout(ms)
  const loc = (t) => (typeof t === 'string' ? page.locator(t).first() : t)
  const v = {
    page,
    sleep,
    cap: (t) => page.evaluate((x) => window.__cap(x), t ?? null),
    step: (t) => page.evaluate((x) => window.__step(x), t ?? null),
    card: (h) => page.evaluate((x) => window.__card(x), h ?? null),
    async go(route, wait = 1500) {
      await goto(page, route)
      await sleep(wait)
    },
    // 指针移到目标中心，click 为 true 时点击
    async point(target, click = false) {
      const l = loc(target)
      await l.scrollIntoViewIfNeeded().catch(() => {})
      const b = await l.boundingBox()
      if (!b) {
        throw new Error(`录屏目标不可见：${target}`)
      }
      const x = b.x + b.width / 2
      const y = b.y + b.height / 2
      await page.evaluate(([a, c]) => window.__cur(a, c), [x, y])
      await page.mouse.move(x, y, { steps: 10 })
      await sleep(700)
      if (click) {
        await page.evaluate(() => window.__click())
        await l.click()
        await sleep(400)
        // 录屏帧有时不会随弹窗出现而刷新（截图正常），滚轮微动一下强制重绘
        await page.mouse.wheel(0, 1)
        await sleep(100)
        await page.mouse.wheel(0, -1)
        await sleep(300)
      }
    },
    click(target) {
      return this.point(target, true)
    },
    // 逐字输入，让观众看清填写的内容
    async type(target, text, delay = 70) {
      await this.point(target, true)
      await loc(target).fill('')
      await loc(target).pressSequentially(text, { delay })
      await sleep(500)
    },
    async scroll(total, stepPx = 100, gap = 110) {
      const dir = Math.sign(total)
      for (let d = 0; d < Math.abs(total); d += stepPx) {
        await page.mouse.wheel(0, dir * stepPx)
        await sleep(gap)
      }
    }
  }
  await goto(page, 'overview')
  await page.evaluate(() => window.__cur(720, 450))
  // 片头：标题、一句话说明、步骤清单
  const list = steps.length ? `<ol>${steps.map((s) => `<li>${s}</li>`).join('')}</ol>` : ''
  await v.card(`${LOGO}<h1>${title}</h1>${summary ? `<p>${summary}</p>` : ''}${list}`)
  await sleep(600)
  // 片头卡片完全显示后才开始收帧，跳过页面加载阶段
  capturing = true
  await sleep(3800 + steps.length * 500)
  await v.card(null)
  await sleep(500)
  await script(v)
  await v.cap(null)
  await v.step(null)
  await v.card(`${LOGO}<h2>${title}</h2><p>${outro}</p><p>在线演示 demo.gately.cn · 开源地址 github.com/wangwanjie/gately</p>`)
  await sleep(4000)
  recording = false
  await loop
  await ctx.close()
  await browser.close()
  // concat 清单：每帧显示到下一帧的时间戳，最后一帧再显示 0.5 秒
  const concat = frames
    .map((fr, i) => `file '${fr.f}'\nduration ${(((frames[i + 1]?.t ?? fr.t + 500) - fr.t) / 1000).toFixed(3)}`)
    .join('\n')
  const listFile = path.join(tmp, 'list.txt')
  fs.writeFileSync(listFile, `${concat}\nfile '${frames[frames.length - 1].f}'\n`)
  fs.mkdirSync(VIDEO_OUT, { recursive: true })
  const mp4 = path.join(VIDEO_OUT, `${name}.zh-CN.mp4`)
  // 统一成 25 fps 的 H.264 + faststart，便于网页边下边播
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'concat', '-safe', '0', '-i', listFile, '-vf', 'fps=25,scale=1440:900', '-an', '-c:v', 'libx264', '-preset', 'slow', '-crf', '28', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', mp4])
  // 封面取片头卡片
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-ss', '1.5', '-i', mp4, '-frames:v', '1', '-q:v', '4', mp4.replace(/\.mp4$/, '.jpg')])
  fs.rmSync(tmp, { recursive: true, force: true })
  const size = (fs.statSync(mp4).size / 1048576).toFixed(1)
  console.log(`已生成 ${mp4}（${size} MB）`)
}
