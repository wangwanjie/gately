// 导航面板的皮肤数据 internal/webpage/panelthemes.json 由 tools/panel-themes/gen.mjs 从 themes.ts 与各语言文案生成，
// 修改主题或文案后忘记重新生成时这里失败。
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { build, outPath } from '../../tools/panel-themes/gen.mjs'

test('panelthemes.json 与管理界面的主题一致', () => {
  assert.equal(readFileSync(outPath, 'utf8'), build(), '请运行 node tools/panel-themes/gen.mjs 重新生成')
})
