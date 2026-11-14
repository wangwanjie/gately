// AuthUser 为共享认证用户。接口返回的密码与 TOTP 密钥为脱敏占位符：
// 编辑时密码框显示为空（hasPassword 标记已设置），保存时由 toAPI 还原占位符表示不修改
export interface AuthUser {
  // 后端生成的不可变 ID（OIDC 的 sub），编辑时原样提交以便改名后保留密码与身份；新用户不带 ID，由后端生成
  id?: string
  name: string
  password: string
  totpSecret: string
  remark: string
  // 可选的身份信息，供 OIDC 身份提供方作为 email、name、groups 声明提供给应用
  email?: string
  displayName?: string
  groups?: string[]
  hasPassword?: boolean
  // 用户自助注册的 Passkey（只读，已脱敏）：只能由用户自己添加，管理员只能删除
  passkeys?: PasskeyInfo[]
}

export interface PasskeyInfo {
  id: string
  name: string
  createdAt: string
  lastUsed?: string
}

export const SECRET_MASK = '********'

// fromAPI 把接口数据转换为编辑用的格式
export function fromAPI(list: AuthUser[] | null | undefined): AuthUser[] {
  return (list ?? []).map((u) => ({ ...u, hasPassword: u.password === SECRET_MASK, password: u.password === SECRET_MASK ? '' : u.password }))
}

// toAPI 去掉编辑用字段；已设置密码且未输入新密码时提交占位符
export function toAPI(list: AuthUser[]): AuthUser[] {
  return list.map(({ hasPassword, passkeys, ...u }) => ({ ...u, password: u.password === '' && hasPassword ? SECRET_MASK : u.password }))
}
