// 发布说明 Markdown 渲染的安全性测试：输出直接用于 v-html，外部文本不能注入标签、属性或脚本链接。node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { renderMarkdown } from '../src/markdown.ts'

// 渲染结果中出现的标签只能是受控的那几种
const allowed = new Set(['p', 'br', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'blockquote', 'hr', 'pre', 'code', 'a', 'strong', 'em', 'del'])
function assertSafe(html) {
  for (const m of html.matchAll(/<\/?([a-zA-Z0-9]+)([^>]*)>/g)) {
    assert.ok(allowed.has(m[1].toLowerCase()), `不应出现标签 <${m[1]}>: ${html}`)
    const attrs = m[2].trim()
    if (attrs) {
      assert.equal(m[1], 'a', `只有链接可以带属性: ${html}`)
      assert.match(attrs, /^href="https?:\/\/[^"<>]*" target="_blank" rel="noopener noreferrer"$/, `链接属性异常: ${html}`)
    }
  }
  // 文本中的 "javascript:"、"onerror=" 已被转义为普通文字，只要求它们不出现在标签内
}

test('HTML 标签被转义', () => {
  const html = renderMarkdown('<script>alert(1)</script>\n<img src=x onerror=alert(1)>\n## <b>Security</b>')
  assertSafe(html)
  assert.ok(html.includes('&lt;script&gt;'))
  assert.ok(html.includes('&lt;img src=x onerror=alert(1)&gt;'))
})

test('链接只允许 http / https', () => {
  for (const src of [
    '[x](javascript:alert(1))',
    '[x](JaVaScRiPt:alert(1))',
    '[x](data:text/html,<script>alert(1)</script>)',
    '[x](vbscript:msgbox)',
    '[x](//evil.example)',
  ]) {
    const html = renderMarkdown(src)
    assertSafe(html)
    assert.ok(!html.includes('<a '), `${src} 不应生成链接: ${html}`)
  }
  assertSafe(renderMarkdown('[ok](https://github.com/wangwanjie/gately)'))
})

test('链接地址与文字中的引号不能跳出属性', () => {
  for (const src of [
    '[x](https://a.example/"onmouseover="alert(1))',
    "[x](https://a.example/'onmouseover='alert(1))",
    'https://a.example/"><script>alert(1)</script>',
    '[<img src=x>](https://a.example)',
    '[**x**](https://a.example/**y**)',
    '`[x](javascript:alert(1))` https://a.example/`code`',
  ]) {
    assertSafe(renderMarkdown(src))
  }
})

test('代码块、引用与列表中的内容同样转义', () => {
  const html = renderMarkdown('```html\n<script>alert(1)</script>\n```\n> <iframe src=x>\n- <svg onload=alert(1)>\n  1. [x](javascript:1)')
  assertSafe(html)
  assert.ok(html.includes('&lt;script&gt;'))
  assert.ok(html.includes('&lt;iframe src=x&gt;'))
})

test('占位符字符不能伪造已保存的链接', () => {
  // 行内代码与链接先替换为 \u0000序号\u0000 占位符，原文中带同样字符时不能取出其他片段或 undefined
  const html = renderMarkdown('\u00000\u0000 [a](https://a.example) \u00001\u0000')
  assertSafe(html)
  assert.ok(!html.includes('undefined'), html)
})
