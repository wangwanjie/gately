// 复制条目（issue #65）：以已有条目为模板打开新增对话框，保存时调用后端的 <集合>/{key}/copy 接口。
// 密码、令牌等被脱敏的密钥保持占位符提交，由后端沿用模板的值，明文不经过浏览器。
import { api } from './api'
import { t } from './i18n'
import { copyName, type CopyNameFormat } from './copyName'

export { asciiCopyName, cloneItem } from './copyName'

// CopySource 是复制的模板条目：key 用于调用复制接口，name 用于在对话框中提示
export interface CopySource {
  key: string
  name: string
}

// localCopyName 按界面语言生成副本名称，例如「名称 副本」「名称 副本 2」
export const localCopyName: CopyNameFormat = (name, n) => (n <= 1 ? t('common.copyName', { name }) : t('common.copyNameN', { name, n }))

// uniqueName 生成与已有名称不重复的副本名称，默认按界面语言的格式
export function uniqueName(name: string | undefined, taken: Iterable<string | undefined>, fmt: CopyNameFormat = localCopyName): string {
  return copyName(name, taken, fmt)
}

// copyUrl 返回复制接口地址
export function copyUrl(base: string, from: CopySource): string {
  return `${base}/${encodeURIComponent(from.key)}/copy`
}

// createItem 新增条目：from 不为空时调用复制接口，否则调用普通新增接口
export function createItem<T = unknown>(base: string, body: unknown, from?: CopySource | null): Promise<T> {
  return api.post<T>(from ? copyUrl(base, from) : base, body)
}
