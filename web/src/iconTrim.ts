// 导航面板图标「去除留白」：找出图片中图形所在的范围，裁成居中的正方形并留一圈统一的小边距，
// 让四周留白很多的 logo 在卡片里显示得足够大。这里只放纯计算（像素数组与 SVG 文本），画布部分在 PanelIconInput 中。

// TRIM_PAD 为裁剪后图形四周保留的边距（占图形较长边的比例）
export const TRIM_PAD = 0.04
// ALPHA_MIN 为视为「有内容」的最小不透明度；COLOR_TOL 为与背景色视为相同的通道差上限（抗锯齿与压缩噪点）
const ALPHA_MIN = 16
const COLOR_TOL = 24
// MIN_GAIN 为留白占比低于这个值时视为无需裁剪（每边不到 2%）
const MIN_GAIN = 0.02

export interface Box {
  x: number
  y: number
  w: number
  h: number
}

// TrimResult：bg 为背景（transparent 或 rgb 颜色，用于补边距），box 为图形所在范围（像素）
export interface TrimResult {
  bg: 'transparent' | [number, number, number]
  box: Box
}

// TrimFailure：blank 为整张图都是背景，mixed 为四角颜色不一致（无法判断哪部分是留白），none 为已经没有多余留白
export type TrimFailure = 'blank' | 'mixed' | 'none'

function near(a: number, b: number) {
  return Math.abs(a - b) <= COLOR_TOL
}

// contentBox 在 RGBA 像素中找图形范围：四角都透明时按透明度判断；四角是同一种不透明颜色时把这种颜色当作留白
export function contentBox(px: Uint8ClampedArray | Uint8Array, w: number, h: number): TrimResult | TrimFailure {
  const at = (x: number, y: number) => (y * w + x) * 4
  const corners = [at(0, 0), at(w - 1, 0), at(0, h - 1), at(w - 1, h - 1)]
  let bg: TrimResult['bg']
  if (corners.every((i) => px[i + 3] < ALPHA_MIN)) {
    bg = 'transparent'
  } else {
    const c0 = corners[0]
    const same = corners.every((i) => px[i + 3] >= 255 - ALPHA_MIN && near(px[i], px[c0]) && near(px[i + 1], px[c0 + 1]) && near(px[i + 2], px[c0 + 2]))
    if (!same) {
      return 'mixed'
    }
    bg = [px[c0], px[c0 + 1], px[c0 + 2]]
  }
  const isContent = (i: number) => {
    if (bg === 'transparent') {
      return px[i + 3] >= ALPHA_MIN
    }
    return px[i + 3] >= ALPHA_MIN && !(near(px[i], bg[0]) && near(px[i + 1], bg[1]) && near(px[i + 2], bg[2]))
  }
  let minX = w
  let minY = h
  let maxX = -1
  let maxY = -1
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      if (isContent(at(x, y))) {
        if (x < minX) {
          minX = x
        }
        if (x > maxX) {
          maxX = x
        }
        if (y < minY) {
          minY = y
        }
        if (y > maxY) {
          maxY = y
        }
      }
    }
  }
  if (maxX < 0) {
    return 'blank'
  }
  const box = { x: minX, y: minY, w: maxX - minX + 1, h: maxY - minY + 1 }
  // 裁成正方形后图形较长边相对原图较长边几乎不变，说明已经没有多余留白
  const side = Math.max(box.w, box.h) * (1 + 2 * TRIM_PAD)
  if (side >= Math.max(w, h) * (1 - 2 * MIN_GAIN)) {
    return 'none'
  }
  return { bg, box }
}

// squareAround 返回以图形为中心、带边距的正方形范围（可超出原图，超出部分由调用方补背景）
export function squareAround(box: Box, pad = TRIM_PAD): Box {
  const side = Math.max(box.w, box.h) * (1 + 2 * pad)
  return { x: box.x + box.w / 2 - side / 2, y: box.y + box.h / 2 - side / 2, w: side, h: side }
}

function round(n: number) {
  return String(Math.round(n * 1000) / 1000)
}

// svgViewBox 读取 SVG 根元素的坐标范围：优先 viewBox，否则用 width/height（只接受无单位或 px 的数值）
export function svgViewBox(svg: string): Box | null {
  const root = /<svg\b[^>]*>/i.exec(svg)
  if (!root) {
    return null
  }
  const attr = (name: string) => new RegExp(`\\s${name}\\s*=\\s*("([^"]*)"|'([^']*)')`, 'i').exec(root[0])
  const vb = attr('viewBox')
  if (vb) {
    const n = (vb[2] ?? vb[3]).trim().split(/[\s,]+/).map(Number)
    if (n.length === 4 && n.every(Number.isFinite) && n[2] > 0 && n[3] > 0) {
      return { x: n[0], y: n[1], w: n[2], h: n[3] }
    }
    return null
  }
  const num = (m: RegExpExecArray | null) => {
    const v = m ? (m[2] ?? m[3]).trim() : ''
    return /^\d+(\.\d+)?(px)?$/.test(v) ? parseFloat(v) : NaN
  }
  const w = num(attr('width'))
  const h = num(attr('height'))
  return w > 0 && h > 0 ? { x: 0, y: 0, w, h } : null
}

// trimSvg 把 SVG 的 viewBox 改成以图形为中心的正方形（box 为按 renderW×renderH 渲染时的像素范围），
// 宽高改为相同的数值，矢量内容保持不变；读不出坐标范围时返回 null
export function trimSvg(svg: string, box: Box, renderW: number, renderH: number, pad = TRIM_PAD): string | null {
  const vb = svgViewBox(svg)
  if (!vb) {
    return null
  }
  // 调用方用 svgForRender 的结果渲染，像素与 viewBox 坐标按比例换算
  const sx = vb.w / renderW
  const sy = vb.h / renderH
  const sq = squareAround({ x: box.x * sx, y: box.y * sy, w: box.w * sx, h: box.h * sy }, pad)
  const nb = `${round(vb.x + sq.x)} ${round(vb.y + sq.y)} ${round(sq.w)} ${round(sq.h)}`
  return withRootSize(svg, nb, 256, 256)
}

// withRootSize 替换 SVG 根元素的 viewBox 与宽高（去掉 preserveAspectRatio，按默认居中等比缩放）
function withRootSize(svg: string, viewBox: string, w: number, h: number): string {
  const root = /<svg\b[^>]*>/i.exec(svg)
  if (!root) {
    return svg
  }
  let tag = root[0].replace(/\s(viewBox|width|height|preserveAspectRatio)\s*=\s*("[^"]*"|'[^']*')/gi, '')
  tag = tag.replace(/^<svg\b/i, `<svg viewBox="${viewBox}" width="${w}" height="${h}"`)
  return svg.slice(0, root.index) + tag + svg.slice(root.index + root[0].length)
}

// svgForRender 返回分析 SVG 时的渲染尺寸（较长边 edge 像素，比例与 viewBox 相同）与按这个尺寸改写的 SVG：
// 原文件的 width/height 可能与 viewBox 比例不同，统一后像素范围才能准确换算回 viewBox 坐标
export function svgForRender(svg: string, edge = 512): { svg: string; w: number; h: number } | null {
  const vb = svgViewBox(svg)
  if (!vb) {
    return null
  }
  const k = edge / Math.max(vb.w, vb.h)
  const w = Math.max(1, Math.round(vb.w * k))
  const h = Math.max(1, Math.round(vb.h * k))
  return { svg: withRootSize(svg, `${vb.x} ${vb.y} ${vb.w} ${vb.h}`, w, h), w, h }
}

// ICON_SITES 为图标编辑处列出的常用图标网站：在新标签页打开，复制图片地址或下载后上传
export const ICON_SITES = [
  { name: 'Dashboard Icons', url: 'https://dashboardicons.com/' },
  { name: 'selfh.st/icons', url: 'https://selfh.st/icons/' },
  { name: 'Seeklogo', url: 'https://seeklogo.com/' },
  { name: 'Iconify', url: 'https://icon-sets.iconify.design/' },
] as const
