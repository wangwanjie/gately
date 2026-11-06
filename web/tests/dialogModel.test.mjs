// 受控对话框的静态检查：node --test tests/
// 组件用 :model-value 控制 el-dialog / el-drawer 时，必须转发 update:model-value。
// 否则点右上角 X 或按 Esc 关闭后父组件的可见状态仍为 true，下次再设为 true 不会触发变化，对话框再也打不开。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../src', import.meta.url))

function vueFiles(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      out.push(...vueFiles(p))
    } else if (name.endsWith('.vue')) {
      out.push(p)
    }
  }
  return out
}

// openingTags 取出模板里对话框类组件的开始标签（可跨多行）。
function openingTags(src) {
  const tags = []
  const re = /<(el-dialog|el-drawer|component)\b/g
  let m
  while ((m = re.exec(src))) {
    // 开始标签在第一个不在引号内的 > 处结束
    let i = m.index + m[0].length
    let quote = ''
    for (; i < src.length; i++) {
      const c = src[i]
      if (quote) {
        if (c === quote) {
          quote = ''
        }
      } else if (c === '"' || c === "'") {
        quote = c
      } else if (c === '>') {
        break
      }
    }
    tags.push(src.slice(m.index, i + 1))
  }
  return tags
}

test('用 :model-value 控制的对话框都转发 update:model-value', () => {
  const missing = []
  for (const f of vueFiles(root)) {
    for (const tag of openingTags(readFileSync(f, 'utf8'))) {
      if (/:model-value="modelValue"/.test(tag) && !/@update:model-value=/.test(tag)) {
        missing.push(relative(root, f))
      }
    }
  }
  assert.deepEqual(missing, [])
})

test('开始标签解析能跨行并跳过引号内的 >', () => {
  const tags = openingTags('<el-dialog\n  :model-value="modelValue"\n  :x="a > b"\n  @update:model-value="v => emit(v)"\n>body</el-dialog>')
  assert.equal(tags.length, 1)
  assert.match(tags[0], /@update:model-value=/)
  assert.match(tags[0], /a > b/)
})
