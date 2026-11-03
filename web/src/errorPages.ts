// 站点错误页：类型定义与接口封装，对应后端 internal/modules/webservice/errorpages*.go。
import { api } from './api'

// ErrorPageOptions 是站点的错误页设置：style 为空为默认样式、custom 为页面库中的自定义页面，其余为内置风格
export interface ErrorPageOptions {
  style: string
  page: string
  // intercept 为 true 时（仅反向代理站点）把后端返回的 codes 中的状态码换成错误页
  intercept: boolean
  codes: string
}

// 内置风格的 ID，顺序与后端一致；名称与说明在 errorPages 文案中按 ID 查找
export const builtinStyles = ['minimal', 'terminal', 'neon', 'space', 'paper', 'brutalist', 'aurora', 'pixel', 'blueprint', 'status'] as const

// 开启拦截时默认填入的状态码
export const defaultInterceptCodes = '404, 500-599'

// 自定义页面的大小上限，与后端 webpage.MaxCustomPage 一致
export const maxCustomPageBytes = 256 * 1024

// 自定义页面中可用的占位符
export const placeholders = ['{code}', '{status}', '{title}', '{detail}', '{host}', '{path}', '{time}'] as const

export function emptyErrorPage(): ErrorPageOptions {
  return { style: '', page: '', intercept: false, codes: '' }
}

// normalizeErrorPage 补全旧配置中缺少的字段
export function normalizeErrorPage(o?: Partial<ErrorPageOptions> | null): ErrorPageOptions {
  return { ...emptyErrorPage(), ...(o ?? {}) }
}

// ErrorPageSource 为自定义页面的来源：inline 为上传或粘贴、file 为服务器上的文件
export type ErrorPageSource = 'inline' | 'file'

// CustomErrorPage 是页面库条目（读取单个页面时含内容）
export interface CustomErrorPage {
  key?: string
  name: string
  source: ErrorPageSource
  html?: string
  file?: string
  remark: string
}

// ErrorPageView 是页面库列表中的条目，不含页面内容
export interface ErrorPageView {
  key: string
  name: string
  source: ErrorPageSource
  file?: string
  remark: string
  size: number
  updatedAt?: string
  usedBy?: string[]
  // error 为文件页面当前不可用的原因
  error?: string
}

// CheckReport 是页面的检查结果，removed 与 warnings 已按界面语言翻译
export interface CheckReport {
  size: number
  pageTitle: string
  removed: string[] | null
  warnings: string[] | null
  placeholders: string[] | null
}

export interface StylePreview {
  id: string
  html: string
}

const base = '/api/webservice/errorpages'

export const errorPagesApi = {
  list: () => api.get<ErrorPageView[]>(base),
  get: (key: string) => api.get<CustomErrorPage>(`${base}/${encodeURIComponent(key)}`),
  create: (p: CustomErrorPage) => api.post<{ page: ErrorPageView; report: CheckReport }>(base, p),
  update: (key: string, p: CustomErrorPage) => api.put<{ page: ErrorPageView; report: CheckReport }>(`${base}/${encodeURIComponent(key)}`, p),
  remove: (key: string) => api.del(`${base}/${encodeURIComponent(key)}`),
  check: (req: { source: ErrorPageSource; html?: string; file?: string; code?: number }) => api.post<{ report: CheckReport; preview: string }>(`${base}/check`, req),
  styles: (code: number) => api.get<StylePreview[]>(`${base}/styles?code=${code}`),
  preview: (key: string, code: number) => api.get<StylePreview>(`${base}/${encodeURIComponent(key)}/preview?code=${code}`),
}

// 样式预览按状态码缓存：同一会话中反复打开站点编辑框时不必重新请求
const styleCache = new Map<number, Promise<StylePreview[]>>()

export function loadStyles(code: number): Promise<StylePreview[]> {
  let p = styleCache.get(code)
  if (!p) {
    p = errorPagesApi.styles(code)
    // 失败时不缓存，下次重新请求
    p.catch(() => styleCache.delete(code))
    styleCache.set(code, p)
  }
  return p
}
