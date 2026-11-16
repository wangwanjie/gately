// 首屏语言包预加载脚本：在模拟的浏览器环境里执行，结果必须与 pickLocale（应用内的语言选择逻辑）一致。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import vm from 'node:vm'
import { localeHintScript } from '../src/i18n/localeHint.ts'
import { LOCALE_STORAGE_KEY, allLocales, fallbackOf, pickLocale } from '../src/i18n/localeMeta.ts'

// 为每个语言造一个带哈希的文件名，哈希里故意含 - 与 _
const fileOf = (code) => `locale-${code}-Ab_c-d1.js`

function build(codes) {
  const available = allLocales.filter((l) => codes.includes(l.code))
  const files = Object.fromEntries(codes.map((c) => [c, fileOf(c)]))
  return { available, fallback: fallbackOf(codes), script: localeHintScript(files, available, fallbackOf(codes), LOCALE_STORAGE_KEY) }
}

// 运行脚本，返回插入的 link 元素；storage 为 localStorage 的内容
function run(script, { languages, language, storage = {}, storageThrows = false }) {
  const appended = []
  const sandbox = {
    document: {
      currentScript: { src: 'http://127.0.0.1:1234/app/gately/assets/locale-hint-xyz.js' },
      createElement: () => ({}),
      head: { appendChild: (el) => appended.push(el) },
    },
    navigator: { languages, language },
    localStorage: {
      getItem: (k) => {
        if (storageThrows) {
          throw new Error('denied')
        }
        return storage[k] ?? null
      },
    },
  }
  vm.runInNewContext(script, sandbox)
  return appended
}

const ALL = allLocales.map((l) => l.code)

test('预加载的语言与应用内的选择一致', () => {
  const { available, fallback, script } = build(ALL)
  const cases = [['zh-CN'], ['zh-TW'], ['zh-HK'], ['zh'], ['zh-Hans-CN'], ['zh-Hant-TW'], ['en-US', 'zh'], ['EN'], ['pt-BR'], ['pt'], ['fr-CA'], ['de-AT'], ['ru'], ['ko-KR'], ['ja-JP'], ['es-419'], ['xx-YY', 'ja'], ['xx'], ['enx'], ['zh-CNx']]
  for (const languages of cases) {
    const links = run(script, { languages, language: languages[0] })
    assert.equal(links.length, 1, JSON.stringify(languages))
    const want = pickLocale(languages, available, fallback)
    assert.equal(links[0].href, `http://127.0.0.1:1234/app/gately/assets/${fileOf(want)}`, JSON.stringify(languages))
    assert.equal(links[0].rel, 'modulepreload')
    assert.equal(links[0].crossOrigin, '')
  }
})

test('没有 navigator.languages 时使用 navigator.language，都没有时用回退语言', () => {
  const { script } = build(ALL)
  assert.match(run(script, { languages: [], language: 'ja' })[0].href, /locale-ja-/)
  assert.match(run(script, { languages: undefined, language: undefined })[0].href, /locale-en-/)
})

test('已保存的语言优先，无效值与读取异常时按浏览器语言', () => {
  const { script } = build(ALL)
  assert.match(run(script, { languages: ['en'], storage: { [LOCALE_STORAGE_KEY]: 'zh-TW' } })[0].href, /locale-zh-TW-/)
  assert.match(run(script, { languages: ['ja'], storage: { [LOCALE_STORAGE_KEY]: 'klingon' } })[0].href, /locale-ja-/)
  assert.match(run(script, { languages: ['ja'], storageThrows: true })[0].href, /locale-ja-/)
})

test('定制构建只含部分语言时只预加载已包含的语言', () => {
  const { script } = build(['zh-CN'])
  // 回退语言为简体中文
  for (const languages of [['en'], ['ja'], ['zh-CN']]) {
    assert.match(run(script, { languages })[0].href, /locale-zh-CN-/)
  }
  assert.match(run(script, { languages: ['en'], storage: { [LOCALE_STORAGE_KEY]: 'en' } })[0].href, /locale-zh-CN-/)
  const some = build(['zh-CN', 'ja', 'en'])
  assert.match(run(some.script, { languages: ['fr', 'ja'] })[0].href, /locale-ja-/)
  assert.match(run(some.script, { languages: ['fr'] })[0].href, /locale-en-/)
})

test('脚本体积足够小', () => {
  const { script } = build(ALL)
  assert.ok(script.length < 2500, `脚本 ${script.length} 字节`)
})
