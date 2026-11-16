// Element Plus 按需登记校验：真实源码必须通过，并验证校验器能发现各种漏登记写法。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { checkElement } from '../scripts/check-element.mjs'

const web = resolve(import.meta.dirname, '..')

test('真实源码的 Element Plus 登记完整', () => {
  const { errors } = checkElement(web)
  assert.deepEqual(errors, [])
})

// 在临时目录里放一份真实的 src/element.ts 和若干夹具源码，返回校验错误
function run(files, mutateRegistry = (s) => s) {
  const dir = mkdtempSync(join(tmpdir(), 'check-element-'))
  try {
    mkdirSync(join(dir, 'src'), { recursive: true })
    symlinkSync(join(web, 'node_modules'), join(dir, 'node_modules'))
    writeFileSync(join(dir, 'package.json'), '{}')
    writeFileSync(join(dir, 'src/element.ts'), mutateRegistry(readFileSync(join(web, 'src/element.ts'), 'utf8')))
    for (const [name, text] of Object.entries(files)) {
      mkdirSync(join(dir, 'src', name, '..'), { recursive: true })
      writeFileSync(join(dir, 'src', name), text)
    }
    return checkElement(dir).errors
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

test('登记过的组件与指令不报错', () => {
  const errors = run({ 'A.vue': '<template><el-button v-loading="x"><el-table-column /></el-button></template>' })
  assert.deepEqual(errors, [])
})

test('模板里的未登记标签', () => {
  const errors = run({ 'A.vue': '<template><el-tree :data="d" /></template>' })
  assert.equal(errors.length, 1)
  assert.match(errors[0], /A\.vue:1.*ElTree/)
})

test('PascalCase 标签与具名导入', () => {
  const errors = run({ 'A.vue': '<template><ElCalendar /></template><script setup>import { ElTransfer } from "element-plus"</script>' })
  assert.equal(errors.length, 2)
})

test('渲染函数与动态组件里的字符串、标识符', () => {
  const errors = run({
    'a.ts': "import { h } from 'vue'\nexport const x = () => h('el-rate')\nexport const y = () => resolveComponent('el-carousel')\n",
    'B.vue': '<template><component :is="flag ? ElStatistic : \'div\'" /></template>',
  })
  assert.ok(errors.length >= 3)
  assert.ok(errors.some((e) => /ElRate/.test(e)))
  assert.ok(errors.some((e) => /ElCarousel/.test(e)))
  assert.ok(errors.some((e) => /ElStatistic/.test(e)))
})

test('非字面量的 resolveComponent 直接报错', () => {
  const errors = run({ 'a.ts': 'resolveComponent(name)\n' })
  assert.equal(errors.length, 1)
  assert.match(errors[0], /resolveComponent/)
})

test('未登记的指令与服务', () => {
  const errors = run({
    'A.vue': '<template><div v-infinite-scroll="load" v-if="a" v-model="b" /></template>',
    'a.ts': "import { ElNotification } from 'element-plus'\nElNotification({})\n",
    'b.ts': 'this.$notify({})\n',
  })
  assert.equal(errors.length, 4)
  assert.ok(errors.some((e) => /v-infinite-scroll/.test(e)))
  assert.ok(errors.some((e) => /ElNotification/.test(e)))
})

test('本地声明的指令不报错', () => {
  const errors = run({ 'A.vue': '<template><div v-focus /></template><script setup>const vFocus = {}</script>' })
  assert.deepEqual(errors, [])
})

test('登记项缺少样式引入或移除了组件', () => {
  const noStyle = run({}, (s) => s.replace('import "element-plus/es/components/tag/style/css"\n', ''))
  assert.equal(noStyle.length, 1)
  assert.match(noStyle[0], /ElTag.*样式/)
  const removed = run({ 'A.vue': '<template><el-tag /></template>' }, (s) => s.replace(/(export const elementComponents[\s\S]*?)\n {2}ElTag,/, '$1'))
  assert.ok(removed.some((e) => /A\.vue:1.*ElTag/.test(e)))
})

test('整库注册与整包样式被拒绝', () => {
  const errors = run({ 'main.ts': "import ElementPlus from 'element-plus'\nimport 'element-plus/dist/index.css'\napp.use(ElementPlus)\n" })
  assert.ok(errors.length >= 3)
})

test('样式类名字符串不当作组件', () => {
  const errors = run({ 'a.ts': "const o = { confirmButtonClass: 'el-button--danger', c: 'el-table__row' }\n" })
  assert.deepEqual(errors, [])
})
