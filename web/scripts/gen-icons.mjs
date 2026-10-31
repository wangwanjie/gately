// 由 public/favicon.svg 的标志生成主屏幕图标（PNG）与 favicon.ico，同时写入管理端 web/public 与官网 website/public。
// 用本机 Chrome 无头模式渲染 SVG（ImageMagick 自带的 SVG 渲染器不支持渐变填充，生成的图标会丢失底色），再用 ImageMagick 合成 ico。
// 用法：node web/scripts/gen-icons.mjs（标志改动后重新执行并提交生成的图片）
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const outDirs = [join(root, 'public'), resolve(root, '../website/public')]
const chrome = [
  process.env.CHROME,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
].find((p) => p && existsSync(p))
if (!chrome) {
  throw new Error('未找到 Chrome/Edge，可通过环境变量 CHROME 指定可执行文件路径')
}

// logo 生成标志 SVG：radius 为底板圆角（0 表示铺满方形），scale 为字形相对中心的缩放比例。
// 字形与 favicon.svg、src/themes.ts 的 logoSvg 保持一致（默认翠绿色系）。
function logo(radius, scale) {
  return (
    `<svg width="32" height="32" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">` +
    `<defs><linearGradient id="g" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">` +
    `<stop offset="0" stop-color="#18a888"/><stop offset="1" stop-color="#0b5f4f"/></linearGradient></defs>` +
    `<rect width="32" height="32" rx="${radius}" fill="url(#g)"/>` +
    `<g transform="translate(16 16) scale(${scale}) translate(-16 -16)">` +
    `<path d="M21.7 10.3A8 8 0 1 0 24 16h-7" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>` +
    `<circle cx="23.6" cy="8.6" r="2.2" fill="#9ff3dc"/></g></svg>`
  )
}

// 圆角版用于浏览器标签页与 manifest 的普通图标；
// 方形版用于 iOS 主屏幕（Web Clip，系统自行裁圆角，透明处会被填成黑色）；
// maskable 版供 Android 按设备形状裁剪，字形缩小到中心 80% 的安全区内。
const targets = [
  { file: 'icon-192.png', size: 192, svg: logo(9, 1) },
  { file: 'icon-512.png', size: 512, svg: logo(9, 1) },
  { file: 'icon-maskable-512.png', size: 512, svg: logo(0, 0.8) },
  { file: 'apple-touch-icon.png', size: 180, svg: logo(0, 0.9) },
  { file: 'ico-16.png', size: 16, svg: logo(9, 1), temp: true },
  { file: 'ico-32.png', size: 32, svg: logo(9, 1), temp: true },
  { file: 'ico-48.png', size: 48, svg: logo(9, 1), temp: true },
]

const work = mkdtempSync(join(tmpdir(), 'gately-icons-'))
try {
  for (const t of targets) {
    const html = join(work, t.file + '.html')
    writeFileSync(
      html,
      `<!doctype html><style>html,body{margin:0;background:transparent}svg{display:block;width:${t.size}px;height:${t.size}px}</style>${t.svg}`,
    )
    execFileSync(chrome, [
      '--headless',
      '--disable-gpu',
      '--hide-scrollbars',
      '--force-device-scale-factor=1',
      '--default-background-color=00000000',
      `--window-size=${t.size},${t.size}`,
      `--screenshot=${join(work, t.file)}`,
      'file://' + html,
    ], { stdio: 'ignore' })
  }
  execFileSync('magick', ['ico-16.png', 'ico-32.png', 'ico-48.png', 'favicon.ico'], { cwd: work })
  for (const dir of outDirs) {
    for (const t of targets.filter((t) => !t.temp)) {
      copyFileSync(join(work, t.file), join(dir, t.file))
    }
    copyFileSync(join(work, 'favicon.ico'), join(dir, 'favicon.ico'))
  }
} finally {
  rmSync(work, { recursive: true, force: true })
}
