// 轻量 Markdown 渲染，用于展示 GitHub 发布说明等外部文本。
// 安全策略：先整体转义 HTML，再把有限的语法转换为受控标签；链接只允许 http / https，
// 因此输出可以直接用于 v-html，外部文本无法注入任意标签或脚本。
// 支持：标题、段落、无序 / 有序列表（含一层嵌套）、引用、分隔线、代码块、行内代码、加粗、斜体、删除线、链接与裸链接。

function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')
}

// safeURL 只放行 http / https 链接；入参与返回值都是已转义的文本
function safeURL(escaped: string): string | null {
  return /^https?:\/\//i.test(escaped) ? escaped : null
}

function link(href: string, text: string): string {
  return `<a href="${href}" target="_blank" rel="noopener noreferrer">${text}</a>`
}

// inline 渲染行内语法。行内代码与链接先替换为占位符，避免其内容再被加粗、裸链接等规则改写
function inline(raw: string): string {
  const slots: string[] = []
  const hold = (html: string) => `\u0000${slots.push(html) - 1}\u0000`
  // 去掉原文中的 \u0000，避免与占位符混淆（原文写出 \u00000\u0000 会重复取出已保存的链接或得到 undefined）
  let s = escapeHTML(raw.replace(/\u0000/g, ''))
  s = s.replace(/`([^`]+)`/g, (_, code: string) => hold(`<code>${code}</code>`))
  s = s.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (m, text: string, url: string) => {
    const href = safeURL(url)
    return href ? hold(link(href, text)) : m
  })
  // 裸链接：不吞掉结尾的标点与括号
  s = s.replace(/https?:\/\/[^\s<\u0000]+[^\s<\u0000.,;:!?)\]，。；：！？）、]/gi, (url) => hold(link(url, url)))
  s = s.replace(/\*\*([^*]+)\*\*|__([^_]+)__/g, (_, a?: string, b?: string) => `<strong>${a ?? b}</strong>`)
  s = s.replace(/~~([^~]+)~~/g, '<del>$1</del>')
  s = s.replace(/(^|[^*\w])\*([^*\s][^*]*?)\*(?!\*)/g, '$1<em>$2</em>')
  return s.replace(/\u0000(\d+)\u0000/g, (_, i: string) => slots[Number(i)])
}

interface ListItem {
  text: string
  children: { ordered: boolean; items: string[] } | null
}

const listRe = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/

export function renderMarkdown(src: string): string {
  const lines = src.replace(/\r\n?/g, '\n').split('\n')
  const out: string[] = []
  let para: string[] = []
  const flushPara = () => {
    if (para.length) {
      out.push(`<p>${para.map(inline).join('<br>')}</p>`)
      para = []
    }
  }
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    const trimmed = line.trim()
    if (!trimmed) {
      flushPara()
      i++
      continue
    }
    // 代码块
    if (trimmed.startsWith('```')) {
      flushPara()
      const code: string[] = []
      i++
      while (i < lines.length && !lines[i].trim().startsWith('```')) {
        code.push(lines[i])
        i++
      }
      i++
      out.push(`<pre><code>${escapeHTML(code.join('\n'))}</code></pre>`)
      continue
    }
    const heading = /^(#{1,6})\s+(.*?)\s*#*$/.exec(trimmed)
    if (heading) {
      flushPara()
      out.push(`<h${heading[1].length}>${inline(heading[2])}</h${heading[1].length}>`)
      i++
      continue
    }
    if (/^([-*_])(\s*\1){2,}$/.test(trimmed)) {
      flushPara()
      out.push('<hr>')
      i++
      continue
    }
    if (trimmed.startsWith('>')) {
      flushPara()
      const quote: string[] = []
      while (i < lines.length && lines[i].trim().startsWith('>')) {
        quote.push(lines[i].trim().replace(/^>\s?/, ''))
        i++
      }
      out.push(`<blockquote>${renderMarkdown(quote.join('\n'))}</blockquote>`)
      continue
    }
    const first = listRe.exec(line)
    if (first) {
      flushPara()
      const baseIndent = first[1].length
      const ordered = /\d/.test(first[2])
      const items: ListItem[] = []
      while (i < lines.length) {
        const cur = lines[i]
        if (!cur.trim()) {
          // 空行后若仍是列表项则继续同一列表
          const next = lines[i + 1] === undefined ? null : listRe.exec(lines[i + 1])
          if (next && (next[1].length > baseIndent || /\d/.test(next[2]) === ordered)) {
            i++
            continue
          }
          break
        }
        const m = listRe.exec(cur)
        if (m && m[1].length <= baseIndent && /\d/.test(m[2]) !== ordered) {
          // 同级换成另一种列表，结束当前列表
          break
        }
        if (m && m[1].length <= baseIndent) {
          items.push({ text: m[3], children: null })
        } else if (m && items.length) {
          // 缩进更深的列表项作为上一项的子列表
          const last = items[items.length - 1]
          last.children ??= { ordered: /\d/.test(m[2]), items: [] }
          last.children.items.push(m[3])
        } else if (items.length && /^\s+/.test(cur)) {
          // 缩进的续行并入上一项
          const last = items[items.length - 1]
          if (last.children) {
            last.children.items[last.children.items.length - 1] += ' ' + cur.trim()
          } else {
            last.text += ' ' + cur.trim()
          }
        } else {
          break
        }
        i++
      }
      const tag = ordered ? 'ol' : 'ul'
      const body = items
        .map((it) => {
          const sub = it.children
            ? `<${it.children.ordered ? 'ol' : 'ul'}>${it.children.items.map((c) => `<li>${inline(c)}</li>`).join('')}</${it.children.ordered ? 'ol' : 'ul'}>`
            : ''
          return `<li>${inline(it.text)}${sub}</li>`
        })
        .join('')
      out.push(`<${tag}>${body}</${tag}>`)
      continue
    }
    para.push(trimmed)
    i++
  }
  flushPara()
  return out.join('')
}
