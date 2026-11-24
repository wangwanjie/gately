// 导航面板图标「去除留白」的纯计算测试：像素范围、背景判断、SVG viewBox 改写。node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ICON_SITES, contentBox, squareAround, svgForRender, svgViewBox, trimSvg } from '../src/iconTrim.ts'

// image 生成 w×h 的 RGBA 像素：bg 为背景 [r,g,b,a]，rect 区域填 fg
function image(w, h, bg, rect, fg = [255, 0, 0, 255]) {
  const px = new Uint8ClampedArray(w * h * 4)
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const inside = rect && x >= rect.x && x < rect.x + rect.w && y >= rect.y && y < rect.y + rect.h
      px.set(inside ? fg : bg, (y * w + x) * 4)
    }
  }
  return px
}

test('透明留白：找出图形范围', () => {
  const r = contentBox(image(100, 100, [0, 0, 0, 0], { x: 30, y: 40, w: 20, h: 10 }), 100, 100)
  assert.deepEqual(r, { bg: 'transparent', box: { x: 30, y: 40, w: 20, h: 10 } })
})

test('纯色留白：四角同色时按颜色判断，压缩噪点在容差内', () => {
  const px = image(80, 60, [250, 250, 250, 255], { x: 10, y: 10, w: 30, h: 30 }, [20, 40, 200, 255])
  // 角落附近的轻微噪点不算内容
  px.set([245, 248, 252, 255], (1 * 80 + 1) * 4)
  const r = contentBox(px, 80, 60)
  assert.deepEqual(r, { bg: [250, 250, 250], box: { x: 10, y: 10, w: 30, h: 30 } })
})

test('几乎半透明的像素不算内容', () => {
  const px = image(50, 50, [0, 0, 0, 0], { x: 20, y: 20, w: 5, h: 5 })
  px.set([0, 0, 0, 8], (2 * 50 + 45) * 4)
  const r = contentBox(px, 50, 50)
  assert.deepEqual(r.box, { x: 20, y: 20, w: 5, h: 5 })
})

test('无法裁剪的情况：空白、四角不同色、没有多余留白', () => {
  assert.equal(contentBox(image(20, 20, [0, 0, 0, 0]), 20, 20), 'blank')
  const mixed = image(20, 20, [255, 255, 255, 255])
  mixed.set([0, 0, 0, 255], (19 * 20 + 19) * 4)
  assert.equal(contentBox(mixed, 20, 20), 'mixed')
  assert.equal(contentBox(image(100, 100, [0, 0, 0, 0], { x: 1, y: 1, w: 98, h: 98 }), 100, 100), 'none')
  // 宽图标：较长边几乎占满时同样无需裁剪，只有上下留白也不裁
  assert.equal(contentBox(image(100, 50, [0, 0, 0, 0], { x: 0, y: 20, w: 100, h: 10 }), 100, 50), 'none')
})

test('正方形范围以图形为中心并留边距', () => {
  const sq = squareAround({ x: 30, y: 40, w: 20, h: 10 }, 0.05)
  assert.equal(sq.w, 22)
  assert.equal(sq.h, 22)
  assert.equal(sq.x + sq.w / 2, 40)
  assert.equal(sq.y + sq.h / 2, 45)
})

test('读取 SVG 坐标范围', () => {
  assert.deepEqual(svgViewBox('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path/></svg>'), { x: 0, y: 0, w: 24, h: 24 })
  assert.deepEqual(svgViewBox("<?xml version='1.0'?><svg viewBox='-5,10 200 100'>"), { x: -5, y: 10, w: 200, h: 100 })
  assert.deepEqual(svgViewBox('<svg width="64px" height="32" stroke-width="2">'), { x: 0, y: 0, w: 64, h: 32 })
  assert.equal(svgViewBox('<svg width="100%" height="100%">'), null)
  assert.equal(svgViewBox('<svg viewBox="0 0 0 10">'), null)
  assert.equal(svgViewBox('<html>'), null)
})

test('SVG 渲染尺寸与 viewBox 同比例，宽高与 preserveAspectRatio 被替换', () => {
  const r = svgForRender('<svg width="10" height="300" preserveAspectRatio="none" viewBox="0 0 200 100" stroke-width="3"><g/></svg>', 512)
  assert.equal(r.w, 512)
  assert.equal(r.h, 256)
  assert.match(r.svg, /^<svg viewBox="0 0 200 100" width="512" height="256" stroke-width="3">/)
  assert.doesNotMatch(r.svg, /preserveAspectRatio/)
})

test('SVG 去除留白只改 viewBox，内容不变', () => {
  const src = '<?xml version="1.0"?>\n<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100" viewBox="0 0 200 100"><rect x="50" y="25" width="40" height="20"/></svg>'
  // 按 512×256 渲染时图形位于 (128,64) 起 102.4×51.2 像素
  const out = trimSvg(src, { x: 128, y: 64, w: 102.4, h: 51.2 }, 512, 256, 0)
  assert.equal(out, '<?xml version="1.0"?>\n<svg viewBox="50 15 40 40" width="256" height="256" xmlns="http://www.w3.org/2000/svg"><rect x="50" y="25" width="40" height="20"/></svg>')
  assert.equal(trimSvg('<svg width="50%"></svg>', { x: 0, y: 0, w: 1, h: 1 }, 10, 10), null)
})

test('图标网站都是 https 地址', () => {
  assert.ok(ICON_SITES.length >= 4)
  for (const s of ICON_SITES) {
    assert.equal(new URL(s.url).protocol, 'https:')
  }
})
