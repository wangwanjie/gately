// 缓存规则路径的前端校验，规则与后端 validateCachePath 一致，便于在保存前就地提示。

// CachePathError 是路径不合法的原因：format 为不以 / 开头或含空格、?、#；
// percent 为含 %（规则按解码后的路径匹配，编码写法永远匹配不到）；segment 为含 .、.. 路径段或连续的 /
export type CachePathError = '' | 'format' | 'percent' | 'segment'

// cachePathError 返回规则路径的错误类型，合法或留空（尚未填写）时返回空字符串。
export function cachePathError(raw: string): CachePathError {
  const p = raw.trim()
  if (p === '') {
    return ''
  }
  if (!p.startsWith('/') || /[ \t?#]/.test(p)) {
    return 'format'
  }
  if (p.includes('%')) {
    return 'percent'
  }
  if (p.includes('//') || `${p}/`.includes('/./') || `${p}/`.includes('/../')) {
    return 'segment'
  }
  return ''
}
