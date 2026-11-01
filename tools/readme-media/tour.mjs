// 用法：node tour.mjs <en|zh> <outDir>  录制约 2 分钟的功能巡览（无旁白，字幕叠加）
import { chromium } from 'playwright-core'
import fs from 'fs'
const [lang, outDir] = process.argv.slice(2)
const locale = lang === 'zh' ? 'zh-CN' : 'en'
const BASE = 'http://127.0.0.1:16750'
const T = {
  en: {
    intro: ['Gately', 'The all-in-one network gateway', 'Reverse proxy, HTTPS, DDNS, tunnels, WAF and more, managed from one console'],
    overview: 'One console for the whole edge: system, services and traffic at a glance',
    web: 'Reverse proxy by domain, with WAF, login and rate limits per site',
    stats: 'Per-site statistics: requests, errors, backend health, visitors by IP and country',
    topo: 'Live topology: follow traffic from visitors to listeners, sites and backends',
    layouts: 'Seven layouts for different views, from a tree to a Sankey diagram',
    proto: 'Many services, one port 443: routed by SNI, ALPN or protocol',
    ssl: 'HTTPS certificates issued and renewed automatically',
    ddns: 'Home IP changed? DNS records update by themselves',
    guard: 'Scanners and brute force attempts are banned automatically',
    frp: 'Reach internal services from outside with frp, Tailscale or Cloudflare Tunnel',
    docker: 'Manage containers, Compose projects and image upgrades',
    term: 'A web terminal with SSH, Telnet and a local shell',
    hist: 'Every change is versioned: compare and roll back in one click',
    outro: ['Get started in one command', 'Open source under Apache-2.0', 'github.com/wangwanjie/gately'],
  },
  zh: {
    intro: ['Gately', '一站式网络网关', '反向代理、HTTPS、动态域名、内网穿透、WAF 等，在一个控制台里管理'],
    overview: '一个控制台管理整个网络边缘：系统、服务与流量一目了然',
    web: '按域名做反向代理，每个站点可单独开启 WAF、登录认证与限流',
    stats: '站点统计：请求数、错误、后端健康状态，以及来源 IP 与国家排行',
    topo: '实时拓扑：跟随流量从访客一路看到监听、站点和后端',
    layouts: '七种布局适应不同视角，从树形图到桑基图',
    proto: '一个 443 端口承载多个服务：按 SNI、ALPN 或协议分流',
    ssl: 'HTTPS 证书自动申请、自动续期',
    ddns: '家里的 IP 变了？域名解析自动跟着更新',
    guard: '扫描器与暴力破解自动封禁',
    frp: '用 frp、Tailscale 或 Cloudflare 隧道从外部访问内网服务',
    docker: '管理容器、Compose 项目和镜像升级',
    term: '浏览器里的 Web 终端，支持 SSH、Telnet 和本机 Shell',
    hist: '每次修改都有历史版本：对比差异，一键回滚',
    outro: ['一条命令开始使用', '基于 Apache-2.0 开源', 'github.com/wangwanjie/gately'],
  },
}[lang]
const LOGO = `<svg width="84" height="84" viewBox="0 0 32 32" fill="none"><defs><linearGradient id="g" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#18a888"/><stop offset="1" stop-color="#0b5f4f"/></linearGradient></defs><rect width="32" height="32" rx="9" fill="url(#g)"/><path d="M21.7 10.3A8 8 0 1 0 24 16h-7" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/><circle cx="23.6" cy="8.6" r="2.2" fill="#9ff3dc"/></svg>`

const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const lc = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale })
await lc.addInitScript((l) => { localStorage.setItem('gately.locale', l); localStorage.setItem('gately.theme.mode', 'light'); localStorage.setItem('gately.compact.docker.containers', '1') }, locale)
let p = await lc.newPage()
await p.goto(`${BASE}/#/login`)
await p.fill('input[autocomplete=username]', 'admin'); await p.fill('input[type=password]', 'Gately-Demo-2026'); await p.keyboard.press('Enter')
await p.waitForURL(/overview/)
const state = await lc.storageState()
await lc.close()
fs.mkdirSync(outDir, { recursive: true })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale, storageState: state, recordVideo: { dir: outDir, size: { width: 1440, height: 900 } } })
await ctx.addInitScript(({ l, logo }) => {
  localStorage.setItem('gately.locale', l); localStorage.setItem('gately.theme.mode', 'light'); localStorage.setItem('gately.compact.docker.containers', '1'); localStorage.removeItem('gately.topology.layout')
  const setup = () => {
    if (document.getElementById('g-cap')) { return }
    const st = document.createElement('style')
    st.textContent = `#g-cap{position:fixed;left:50%;bottom:34px;transform:translateX(-50%);z-index:99999;max-width:78%;padding:14px 26px;border-radius:14px;background:rgba(10,24,22,.88);color:#fff;font:600 24px/1.35 -apple-system,'PingFang SC','Segoe UI',sans-serif;text-align:center;opacity:0;transition:opacity .35s;pointer-events:none}
#g-cur{position:fixed;z-index:99998;width:22px;height:22px;border-radius:50%;background:rgba(14,124,102,.35);border:2px solid #0e7c66;left:-40px;top:-40px;transition:left .7s ease,top .7s ease,transform .15s;pointer-events:none;margin:-11px 0 0 -11px}
#g-cur.click{transform:scale(.6)}
#g-card{position:fixed;inset:0;z-index:100000;background:linear-gradient(135deg,#0b3f35,#0e7c66 60%,#18a888);color:#fff;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:16px;text-align:center;font-family:-apple-system,'PingFang SC','Segoe UI',sans-serif;opacity:0;pointer-events:none;transition:opacity .5s}
#g-card h1{margin:0;font-size:84px;letter-spacing:1px}#g-card h2{margin:0;font-size:38px;font-weight:600}#g-card p{margin:0;font-size:24px;opacity:.88;max-width:980px}
#g-card code{display:block;margin-top:14px;padding:18px 28px;border-radius:14px;background:rgba(0,0,0,.35);font:500 22px/1.5 ui-monospace,Menlo,monospace;text-align:left;white-space:pre}`
    document.head.appendChild(st)
    for (const [id, tag] of [['g-cap', 'div'], ['g-cur', 'div'], ['g-card', 'div']]) { const e = document.createElement(tag); e.id = id; document.body.appendChild(e) }
    window.__cap = (t) => { const e = document.getElementById('g-cap'); if (!t) { e.style.opacity = 0; return } e.textContent = t; e.style.opacity = 1 }
    window.__cur = (x, y) => { const e = document.getElementById('g-cur'); e.style.left = x + 'px'; e.style.top = y + 'px' }
    window.__click = () => { const e = document.getElementById('g-cur'); e.classList.add('click'); setTimeout(() => e.classList.remove('click'), 180) }
    window.__card = (html) => { const e = document.getElementById('g-card'); if (!html) { e.style.opacity = 0; return } e.innerHTML = html; e.style.opacity = 1 }
  }
  if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', setup) } else { setup() }
}, { l: locale, logo: LOGO })
p = await ctx.newPage()
const sleep = (ms) => p.waitForTimeout(ms)
const cap = (t) => p.evaluate((x) => window.__cap(x), t)
const card = (h) => p.evaluate((x) => window.__card(x), h)
async function go(route, wait = 1500) { await p.goto(`${BASE}/#/${route}`); await sleep(wait) }
async function moveTo(loc, click = false) {
  const b = await loc.boundingBox()
  if (!b) { return }
  const x = b.x + b.width / 2, y = b.y + b.height / 2
  await p.evaluate(([a, c]) => window.__cur(a, c), [x, y]); await p.mouse.move(x, y, { steps: 8 }); await sleep(800)
  if (click) { await p.evaluate(() => window.__click()); await loc.click(); }
}
async function scroll(total, step = 120, gap = 120) {
  for (let d = 0; d < total; d += step) { await p.mouse.wheel(0, step); await sleep(gap) }
}
async function layout(i) {
  await moveTo(p.locator('.btn-caret').first().locator('xpath=ancestor::button[1]'), true); await sleep(700)
  await moveTo(p.locator('.topo-layout-menu .el-dropdown-menu__item').nth(i), true); await sleep(900)
  await p.locator('.zbtn').nth(3).click().catch(() => {}); await sleep(2600)
}

await p.goto(`${BASE}/#/overview`); await sleep(800)
await p.evaluate(() => window.__cur(720, 450))
// 片头
await card(`${LOGO}<h1>${T.intro[0]}</h1><h2>${T.intro[1]}</h2><p>${T.intro[2]}</p>`); await sleep(4200); await card(null); await sleep(600)
// 概览
await cap(T.overview); await sleep(3000); await scroll(600); await sleep(2500)
// Web 服务
await go('web'); await cap(T.web); await sleep(3500); await scroll(360); await sleep(2200)
await p.mouse.wheel(0, -2000); await sleep(800)
await cap(T.stats)
await moveTo(p.locator('.site').nth(1).locator('button:has(svg[class*="chart"], svg[class*="bar"])').first(), true); await sleep(4800)
await p.keyboard.press('Escape'); await sleep(900)
// 拓扑
await go('webtopology', 3000); await cap(T.topo); await sleep(2500)
await moveTo(p.locator('g.node:has-text("Gitea")').first(), true); await sleep(3500)
await cap(T.layouts); await layout(2); await layout(3); await layout(5); await layout(0); await sleep(800)
// 协议分流
await go('protoroute', 2000); await cap(T.proto); await sleep(5500)
// 证书
await go('ssl', 2000); await cap(T.ssl); await sleep(5500)
// DDNS
await go('ddns', 2000); await cap(T.ddns); await sleep(5500)
// 入侵防护
await go('guard', 2000); await cap(T.guard); await sleep(3500); await scroll(300); await sleep(2500)
// frp
await go('frp', 2000); await cap(T.frp); await sleep(5500)
// Docker
await go('docker', 2500); await cap(T.docker); await sleep(5500)
// 终端
await go('terminal', 1500); await cap(T.term)
await moveTo(p.locator('.conn-ops .el-button--primary').first(), true); await sleep(2200)
await p.locator('.xterm').first().click(); await sleep(400)
for (const c of ['cd /opt/stacks && ls\n', 'docker ps --format "table {{.Names}}\\t{{.Status}}"\n', 'uptime\n']) { await p.keyboard.type(c, { delay: 55 }); await sleep(1200) }
await sleep(1500)
// 配置历史
await go('history', 1500); await cap(T.hist)
await p.locator('.el-select').first().click(); await sleep(600)
await moveTo(p.locator('.el-select-dropdown__item', { hasText: 'webservice.json' }).first(), true); await sleep(1500)
await moveTo(p.locator('tbody tr').first().locator('button').first(), true); await sleep(5000)
// 片尾
await cap(null)
await card(`${LOGO}<h2>${T.outro[0]}</h2><code>docker run -d --name gately --network host --cap-add NET_ADMIN \\\n  -v ./gately-data:/etc/gately \\\n  ghcr.io/wangwanjie/gately:latest</code><p>${T.outro[1]} · ${T.outro[2]}</p>`)
await sleep(6500)
const v = p.video()
await ctx.close()
fs.renameSync(await v.path(), `${outDir}/tour.webm`)
await browser.close()
console.log('done', lang)
