// 校验 Element Plus 按需注册：源码里用到的组件、指令与命令式服务必须全部登记在 src/element.ts。
// 漏登记的组件在页面上会渲染成未知标签且不报错，因此在 pnpm test 与 pnpm build 里都运行本脚本。
// 用法：node scripts/check-element.mjs
//
// 静态扫描覆盖：模板里的 <el-xxx> / <ElXxx> 标签、源码里出现的 El* 标识符（具名导入、h()、:is 绑定）、
// 字符串形式的组件名（'el-xxx'、resolveComponent('el-xxx')）、v-xxx 指令、$message 等全局属性。
// 无法静态判断的动态写法（变量拼出的组件名）由 resolveComponent 的非字面量参数告警兜底，其余靠 Playwright 巡检。
import { existsSync, readdirSync, readFileSync, realpathSync, statSync } from 'node:fs'
import { createRequire } from 'node:module'
import { join, relative, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const root = resolve(import.meta.dirname, '..')

// Vue 内置指令，不属于 Element Plus
const VUE_DIRECTIVES = new Set(['if', 'else', 'else-if', 'for', 'show', 'model', 'bind', 'on', 'slot', 'html', 'text', 'once', 'pre', 'cloak', 'memo'])

// 以插件形式安装的服务，对应的样式目录、全局属性与指令
const SERVICES = {
  ElLoading: { style: 'loading', aliases: ['ElLoadingDirective', 'ElLoadingService'], directives: ['loading'], globals: ['$loading'] },
  ElMessage: { style: 'message', aliases: [], directives: [], globals: ['$message'] },
  ElMessageBox: { style: 'message-box', aliases: [], directives: [], globals: ['$msgbox', '$messageBox', '$alert', '$confirm', '$prompt'] },
  ElNotification: { style: 'notification', aliases: [], directives: [], globals: ['$notify'] },
}

// 'FormItem' -> 'form-item'
function kebab(pascal) {
  return pascal.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase()
}

// 'form-item' -> 'ElFormItem'
function pascalTag(tag) {
  return 'El' + tag.split('-').map((s) => s[0].toUpperCase() + s.slice(1)).join('')
}

function lineOf(text, index) {
  let n = 1
  for (let i = 0; i < index; i++) {
    if (text.charCodeAt(i) === 10) {
      n++
    }
  }
  return n
}

// 读取 src/element.ts 的登记内容
export function parseRegistry(text) {
  const arrayOf = (name) => {
    const m = new RegExp(`export const ${name}\\b[^=]*=\\s*\\[([\\s\\S]*?)\\]`).exec(text)
    if (!m) {
      throw new Error(`src/element.ts 缺少 ${name} 数组`)
    }
    return [...m[1].matchAll(/\bEl[A-Za-z0-9]+\b/g)].map((x) => x[0])
  }
  const styles = [...text.matchAll(/^import\s+["']element-plus\/es\/components\/([a-z0-9-]+)\/style\/css["']/gm)].map((x) => x[1])
  return { components: arrayOf('elementComponents'), plugins: arrayOf('elementPlugins'), styles }
}

// 从一个源文件提取用到的 Element Plus 名称，返回 [{ name, line, via }]；directives 为用到的 v- 指令
export function scanSource(file, text, exportsSet) {
  const used = []
  const problems = []
  const add = (name, index, via) => {
    used.push({ name, line: lineOf(text, index), via })
  }
  const isVue = file.endsWith('.vue')
  // 模板部分：去掉 script 与 style 块
  const template = isVue ? text.replace(/<script[\s\S]*?<\/script>/g, (s) => ' '.repeat(s.length)).replace(/<style[\s\S]*?<\/style>/g, (s) => ' '.repeat(s.length)) : ''

  // <el-xxx> 标签
  for (const m of text.matchAll(/<el-([a-z0-9]+(?:-[a-z0-9]+)*)(?=[\s/>])/g)) {
    add(pascalTag(m[1]), m.index, `<el-${m[1]}>`)
  }
  // El* 标识符：具名导入、<ElXxx> 标签、h(ElXxx)、:is="ElXxx"
  for (const m of text.matchAll(/\bEl[A-Z][A-Za-z0-9]*\b/g)) {
    if (exportsSet.has(m[0])) {
      add(m[0], m.index, m[0])
    }
  }
  // 字符串形式的组件名，如 'el-table-column'；带 -- 或 __ 的是样式类名，不匹配
  for (const m of text.matchAll(/(["'`])(el-[a-z0-9]+(?:-[a-z0-9]+)*)\1/g)) {
    const name = pascalTag(m[2].slice(3))
    if (exportsSet.has(name)) {
      add(name, m.index, `字符串 ${m[1]}${m[2]}${m[1]}`)
    }
  }
  // resolveComponent / resolveDynamicComponent：字面量参数按组件名处理，其余无法静态判断，直接报错
  for (const m of text.matchAll(/\bresolve(?:Dynamic)?Component\(\s*([^)]*)\)/g)) {
    const arg = m[1].trim()
    const lit = /^(["'`])([A-Za-z0-9-]+)\1$/.exec(arg)
    if (lit) {
      const name = lit[2].startsWith('el-') ? pascalTag(lit[2].slice(3)) : lit[2]
      if (exportsSet.has(name)) {
        add(name, m.index, `resolveComponent(${arg})`)
      }
    } else {
      problems.push({ line: lineOf(text, m.index), message: `resolveComponent 参数不是字符串字面量（${arg}），无法静态校验，请改成具名导入的组件或在此登记` })
    }
  }
  // 全局属性 $message 等：要求对应服务已作为插件安装
  for (const [svc, def] of Object.entries(SERVICES)) {
    for (const g of def.globals) {
      for (const m of text.matchAll(new RegExp(`(?<![A-Za-z0-9_])${g.replace('$', '\\$')}\\b`, 'g'))) {
        add(svc, m.index, g)
      }
    }
  }
  // 指令：v-xxx
  const directives = []
  if (isVue) {
    // 脚本里以 vXxx 声明的本地指令
    const local = new Set([...text.matchAll(/\bv([A-Z][A-Za-z0-9]*)\b/g)].map((x) => kebab(x[1])))
    for (const m of template.matchAll(/[\s"'](v-([a-z][a-z0-9-]*))(?=[\s=:.>/]|$)/gm)) {
      const name = m[2]
      if (VUE_DIRECTIVES.has(name) || local.has(name)) {
        continue
      }
      directives.push({ name, line: lineOf(text, m.index + 1) })
    }
  }
  return { used, directives, problems }
}

// 整体校验，返回 { errors, warnings, stats }
export function checkElement(rootDir = root) {
  const errors = []
  const warnings = []
  const elementTs = join(rootDir, 'src/element.ts')
  const reg = parseRegistry(readFileSync(elementTs, 'utf8'))

  // element-plus 实际导出的名称，用于判断 El* 标识符是否属于组件库
  const ep = createRequire(join(rootDir, 'package.json'))('element-plus')
  const exportsSet = new Set(Object.keys(ep).filter((k) => /^El[A-Z]/.test(k)))

  const pkgDir = join(rootDir, 'node_modules/element-plus/es/components')
  const registered = new Set([...reg.components, ...reg.plugins])
  const styleSet = new Set(reg.styles)

  // 登记项自身的合法性：必须是组件库导出的组件，并且有同名样式引入
  for (const name of reg.components) {
    if (!exportsSet.has(name)) {
      errors.push(`src/element.ts：${name} 不是 element-plus 的导出`)
      continue
    }
    const style = kebab(name.slice(2))
    if (!styleSet.has(style)) {
      errors.push(`src/element.ts：${name} 缺少样式引入 import "element-plus/es/components/${style}/style/css"`)
    }
  }
  for (const name of reg.plugins) {
    const def = SERVICES[name]
    if (!def) {
      errors.push(`src/element.ts：elementPlugins 里的 ${name} 未在 scripts/check-element.mjs 的 SERVICES 中说明`)
      continue
    }
    if (!styleSet.has(def.style)) {
      errors.push(`src/element.ts：${name} 缺少样式引入 import "element-plus/es/components/${def.style}/style/css"`)
    }
  }
  for (const s of reg.styles) {
    if (!existsSync(join(pkgDir, s, 'style/css.mjs'))) {
      errors.push(`src/element.ts：element-plus/es/components/${s}/style/css 不存在`)
    }
  }

  // 整库注册或整包样式会让按需引入失效
  const full = [/import\s+ElementPlus\b/, /element-plus\/dist\//, /\.use\(\s*ElementPlus\s*\)/, /from\s+['"]element-plus\/lib\b/]

  const usedNames = new Set()
  const usedDirectives = new Set()
  const walk = (dir) => {
    for (const name of readdirSync(dir)) {
      const p = join(dir, name)
      if (statSync(p).isDirectory()) {
        if (name === 'locales') {
          continue
        }
        walk(p)
      } else if (/\.(vue|ts|tsx)$/.test(name) && p !== elementTs && !name.endsWith('.d.ts')) {
        const text = readFileSync(p, 'utf8')
        const rel = relative(rootDir, p)
        for (const re of full) {
          if (re.test(text)) {
            errors.push(`${rel}：使用了整库注册或整包样式（${re}），请改为 src/element.ts 的按需登记`)
          }
        }
        const { used, directives, problems } = scanSource(p, text, exportsSet)
        for (const pr of problems) {
          errors.push(`${rel}:${pr.line}：${pr.message}`)
        }
        for (const u of used) {
          usedNames.add(u.name)
          // 服务别名、全局属性归并到对应的服务插件
          let need = u.name
          for (const [svc, def] of Object.entries(SERVICES)) {
            if (def.aliases.includes(u.name)) {
              need = svc
            }
          }
          if (!registered.has(need)) {
            errors.push(`${rel}:${u.line}：用到 ${u.name}（${u.via}），但未在 src/element.ts 登记 ${need}`)
          }
        }
        for (const d of directives) {
          usedDirectives.add(d.name)
          const svc = Object.entries(SERVICES).find(([, def]) => def.directives.includes(d.name))
          if (!svc) {
            errors.push(`${rel}:${d.line}：用到指令 v-${d.name}，src/element.ts 没有对应的 Element Plus 指令登记`)
          } else if (!registered.has(svc[0])) {
            errors.push(`${rel}:${d.line}：用到 v-${d.name}，但未在 src/element.ts 的 elementPlugins 登记 ${svc[0]}`)
          }
        }
      }
    }
  }
  walk(join(rootDir, 'src'))

  // 已登记但源码里没用到：只提示，便于及时瘦身
  for (const name of registered) {
    const def = SERVICES[name]
    const hit = usedNames.has(name) || (def && (def.aliases.some((a) => usedNames.has(a)) || def.directives.some((d) => usedDirectives.has(d))))
    if (!hit) {
      warnings.push(`src/element.ts：${name} 已登记但源码里没有用到`)
    }
  }

  return { errors, warnings, stats: { components: reg.components.length, plugins: reg.plugins.length, used: usedNames.size } }
}

if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) {
  const { errors, warnings, stats } = checkElement()
  for (const w of warnings) {
    console.warn(`警告：${w}`)
  }
  if (errors.length) {
    for (const e of errors) {
      console.error(`错误：${e}`)
    }
    console.error(`Element Plus 登记校验失败：${errors.length} 处问题`)
    process.exit(1)
  }
  console.log(`Element Plus 登记校验通过：登记 ${stats.components} 个组件、${stats.plugins} 个服务，源码用到 ${stats.used} 个名称`)
}
