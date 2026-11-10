// 生成导航面板的皮肤数据 internal/webpage/panelthemes.json：色系与调色板取自管理界面的 web/src/themes.ts，
// 色系名称、描述与换肤面板文案取自 web/src/locales/<语言>/themes.json 与 components.json（themePicker），
// 使面板页面的皮肤与管理界面完全一致。修改 themes.ts 或这些文案后运行：
//   node tools/panel-themes/gen.mjs
// 需要 Node 22.18+ / 23.6+（直接载入 .ts 文件）。internal/webpage 的测试会检查生成结果与源文件一致。
import { readFileSync, writeFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
export const outPath = join(root, 'internal', 'webpage', 'panelthemes.json')
const { themeFamilies, ACCENT_PRESETS, DEFAULT_FAMILY } = await import(join(root, 'web', 'src', 'themes.ts'))

// 面板页面用到的换肤面板文案
const uiKeys = ['switchTheme', 'title', 'mode', 'modeLight', 'modeDark', 'modeAuto', 'groupOriginal', 'groupClassic', 'darkOnly', 'lightOnly',
  'custom', 'accent', 'accentCustom', 'accentReset', 'radius', 'radiusSharp', 'radiusDefault', 'radiusRound']

const pick = (p) => (p ? { primary: p.primary, page: p.page, card: p.card, sidebar: p.sidebar, border: p.border, text: p.text, textSub: p.textSub, logo: p.logo } : undefined)
const families = themeFamilies.map((f) => ({ id: f.id, group: f.group, radius: f.radius, light: pick(f.light), dark: pick(f.dark) }))

// lookup 按 themes.<色系>.<字段> 形式的文案键取值，键缺失时报错
function lookup(themes, key) {
  const v = key.split('.').slice(1).reduce((o, k) => o?.[k], themes)
  if (typeof v !== 'string') {
    throw new Error(`文案键 ${key} 缺失`)
  }
  return v
}

// build 返回生成的文件内容；web/tests/panelThemes.test.mjs 用它检查仓库中的文件是否最新
export function build() {
  const texts = {}
  const langs = readdirSync(join(root, 'web', 'src', 'locales'), { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name)
    .sort()
  for (const lang of langs) {
    const themes = JSON.parse(readFileSync(join(root, 'web', 'src', 'locales', lang, 'themes.json'), 'utf8'))
    const picker = JSON.parse(readFileSync(join(root, 'web', 'src', 'locales', lang, 'components.json'), 'utf8')).themePicker
    const names = {}
    const descs = {}
    for (const f of themeFamilies) {
      // 产品名等不翻译的名称直接用 label
      names[f.id] = f.label ?? lookup(themes, f.nameKey)
      descs[f.id] = lookup(themes, f.descKey)
    }
    const ui = {}
    for (const k of uiKeys) {
      if (!picker[k]) {
        throw new Error(`${lang}: components.themePicker.${k} 缺失`)
      }
      ui[k] = picker[k]
    }
    texts[lang] = { names, descs, ui }
  }

  return JSON.stringify({ default: DEFAULT_FAMILY, accents: ACCENT_PRESETS, families, texts }) + '\n'
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  writeFileSync(outPath, build())
  console.log(`已生成 ${families.length} 个色系`)
}
