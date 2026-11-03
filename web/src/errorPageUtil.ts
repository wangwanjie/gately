// 站点错误页的纯函数工具（不依赖接口与界面），便于单元测试。

// formatSize 把字节数格式化为 B 或 KB
export function formatSize(n: number): string {
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(n < 10240 ? 1 : 0)} KB`
}

// decodeUTF8 严格按 UTF-8 解码上传的文件，编码不对时返回 null（不悄悄把乱码替换成 �）；去掉开头的 BOM
export function decodeUTF8(buf: ArrayBuffer | Uint8Array): string | null {
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(buf)
  } catch {
    return null
  }
}

// CodesProblem 是拦截状态码的格式问题：empty 为没有填写，bad 为第一项写错的内容
export interface CodesProblem {
  empty?: boolean
  bad?: string
}

// checkCodes 按与后端 parseStatusCodes 相同的规则检查拦截的状态码：逗号、分号、顿号或空白分隔，
// 支持单个状态码、范围（500-599）与 4xx/5xx 简写，只允许 400~599；没有问题时返回 null
export function checkCodes(s: string): CodesProblem | null {
  const fields = s.split(/[,，;；、\s]+/).filter(Boolean)
  if (!fields.length) {
    return { empty: true }
  }
  for (const f of fields) {
    if (/^[45]xx$/i.test(f)) {
      continue
    }
    const m = /^(\d{3})(?:-(\d{3}))?$/.exec(f)
    if (!m) {
      return { bad: f }
    }
    const lo = Number(m[1])
    const hi = m[2] ? Number(m[2]) : lo
    if (lo < 400 || hi > 599 || lo > hi) {
      return { bad: f }
    }
  }
  return null
}
