// 网页认证页面的外观：类型定义与接口封装，对应后端 internal/modules/webservice/loginpage*.go。
import { api } from './api'

// LoginPage 是全局外观设置：style 为空为默认样式，其余为内置样式 ID
export interface LoginPage {
  style: string
  // color 为主色（#rrggbb），空为样式自带的颜色
  color: string
  // background、logo 为 http(s) 图片地址或上传的 data:image 内联图片
  background: string
  // bgDim 为背景图片上的暗色遮罩浓度（0-90）
  bgDim: number
  logo: string
  // title、subtitle 替换登录页的标题与说明（不随访客语言翻译）
  title: string
  subtitle: string
  // hideBrand 隐藏页脚的 Gately 标识
  hideBrand: boolean
  // css 为追加在样式之后的自定义 CSS
  css: string
}

export interface LoginPageView {
  loginPage: LoginPage
  // styles 为内置样式 ID（不含默认样式），顺序即展示顺序
  styles: string[]
  // siteStyles 为单独设置了样式的站点数，按样式 ID 统计
  siteStyles: Record<string, number>
}

export interface LoginStylePreview {
  id: string
  html: string
}

// 预览的页面状态：登录表单、动态验证码对话框、登录失败提示
export type PreviewState = 'login' | 'totp' | 'error'

// 内置样式的 ID，顺序与后端 webpage.LoginSkinIDs 一致；名称在 loginPage 文案中按 ID 查找
export const builtinLoginStyles = ['lockscreen', 'split', 'minimal', 'terminal', 'paper', 'brutalist', 'aurora', 'pixel', 'blueprint']

// 站点设置里「默认样式」的 ID；空字符串表示跟随全局设置
export const LOGIN_STYLE_DEFAULT = 'default'

// 只有深色或只有浅色版本的样式，管理界面据此标注
export const darkOnlyStyles = ['lockscreen', 'terminal', 'aurora', 'pixel', 'blueprint']
export const lightOnlyStyles = ['brutalist']

// 内联图片与文本的上限，与后端一致
export const MAX_LOGO_BYTES = 256 * 1024
export const MAX_TITLE = 60
export const MAX_SUBTITLE = 160
export const MAX_CSS_BYTES = 32 * 1024

// 有背景图片时新填的遮罩浓度
export const DEFAULT_BG_DIM = 30

// 自定义 CSS 可以使用的类名，与登录页模板一致；升级时保持稳定
export const cssClasses = ['body.skin-<id>', '.bg', '.wrap', '.card', '.icon', 'h1', '.sub', '.chip', '.fields', '.ctl input', '.btn.primary', '.btn.ghost', '.or', '.alert', '.modal', '.sheet', '.otp', '.foot']

export function emptyLoginPage(): LoginPage {
  return { style: '', color: '', background: '', bgDim: 0, logo: '', title: '', subtitle: '', hideBrand: false, css: '' }
}

// normalizeLoginPage 补全旧配置中缺少的字段
export function normalizeLoginPage(p?: Partial<LoginPage> | null): LoginPage {
  return { ...emptyLoginPage(), ...(p ?? {}) }
}

export function getLoginPage() {
  return api.get<LoginPageView>('/api/webservice/loginpage')
}

export function saveLoginPage(p: LoginPage) {
  return api.put<LoginPageView>('/api/webservice/loginpage', p)
}

// previewLoginPage 按尚未保存的设置生成预览；styles 为空时返回默认样式与全部内置样式
export function previewLoginPage(p: LoginPage, styles: string[], state: PreviewState) {
  return api.post<LoginStylePreview[]>('/api/webservice/loginpage/preview', { loginPage: p, styles, state })
}
