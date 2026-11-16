// 构建期用：把目标语言缺少的文案键用回退语言的文案补齐（原地修改 target，返回补了多少个键）。
// 这样运行时无需加载回退语言包，缺键也不会闪现键名。仅在 vite.config.ts 与测试里使用，不进入前端包。

type Tree = Record<string, unknown>

function isTree(v: unknown): v is Tree {
  return !!v && typeof v === 'object' && !Array.isArray(v)
}

export function fillMissing(target: Tree, base: Tree): number {
  let count = 0
  for (const [key, baseValue] of Object.entries(base)) {
    if (!(key in target)) {
      target[key] = baseValue
      count += isTree(baseValue) ? countLeaves(baseValue) : 1
    } else if (isTree(baseValue) && isTree(target[key])) {
      count += fillMissing(target[key], baseValue)
    }
    // 类型不一致（一边是对象一边是字符串）时保持目标语言原样，交给 scripts/check-i18n.mjs 报告
  }
  return count
}

function countLeaves(tree: Tree): number {
  let n = 0
  for (const v of Object.values(tree)) {
    n += isTree(v) ? countLeaves(v) : 1
  }
  return n
}
