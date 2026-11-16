// 判断用户填写的目录与 Gately 配置目录的关系，供界面给出提示（服务端另有完整校验，含符号链接与实际文件对象的比较）。
//   - inside：就是配置目录或位于其中，保存会被拒绝；
//   - contains：包含配置目录（例如整个数据盘），可以保存，配置目录会被自动隐藏；
//   - ''：无关。
export type ConfigDirRelation = 'inside' | 'contains' | ''

// normDir 规范化路径用于比较：统一分隔符、去掉多余的 /、. 与 ..，按需转为小写。
function normDir(p: string, fold: boolean): string {
  let s = p.trim().replace(/\\/g, '/')
  const drive = /^[A-Za-z]:/.test(s) ? s.slice(0, 2) : ''
  if (drive) {
    s = s.slice(2)
  }
  const parts: string[] = []
  for (const seg of s.split('/')) {
    if (seg === '' || seg === '.') {
      continue
    }
    if (seg === '..') {
      parts.pop()
      continue
    }
    parts.push(seg)
  }
  const out = drive + '/' + parts.join('/')
  return fold ? out.toLowerCase() : out
}

// within 判断 child 是否等于 parent 或位于其中，两者都已规范化。
function within(parent: string, child: string): boolean {
  if (parent === child) {
    return true
  }
  const prefix = parent.endsWith('/') ? parent : parent + '/'
  return child.startsWith(prefix)
}

// configDirRelation 返回目录 path 与配置目录 configDir 的关系；fold 为 true 时不区分大小写（macOS、Windows）。
// 不是绝对路径的输入（尚未填完）不提示。
export function configDirRelation(path: string, configDir: string, fold: boolean): ConfigDirRelation {
  const raw = path.trim()
  if (!raw || !configDir || !(raw.startsWith('/') || raw.startsWith('\\') || /^[A-Za-z]:[\\/]/.test(raw))) {
    return ''
  }
  const p = normDir(raw, fold)
  const c = normDir(configDir, fold)
  if (within(c, p)) {
    return 'inside'
  }
  if (within(p, c)) {
    return 'contains'
  }
  return ''
}
