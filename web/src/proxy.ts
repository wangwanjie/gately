import { t } from './i18n'
import { maskProxyPassword } from './netaddr'

// proxyLabel 返回代理设置的展示文本：direct、update 显示为说明文字，代理地址隐藏密码；留空返回空串
export function proxyLabel(setting: string | undefined): string {
  const p = (setting ?? '').trim()
  if (!p) {
    return ''
  }
  if (p.toLowerCase() === 'direct') {
    return t('components.proxyInput.direct')
  }
  if (p.toLowerCase() === 'update') {
    return t('components.proxyInput.update')
  }
  // 卡片上不显示代理密码：协议://用户名:密码@ 中的密码换成 ***
  return maskProxyPassword(p)
}
