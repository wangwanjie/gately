import { nextTick } from 'vue'
import type { FormInstance } from 'element-plus'

// validateForm 在提交前做前端校验：未通过时标红对应字段、滚动到第一个出错的字段并聚焦它，返回 false。
// beforeFocus 收到出错字段的 prop 列表，可在滚动前切到字段所在的标签页或展开折叠区（隐藏的字段无法滚动和聚焦）。
// form 为空（表单尚未渲染）时视为通过，交给后端校验。
export async function validateForm(form: FormInstance | undefined, beforeFocus?: (props: string[]) => void | Promise<void>): Promise<boolean> {
  if (!form) {
    return true
  }
  try {
    await form.validate()
    return true
  } catch (fields) {
    const props = Object.keys((fields as Record<string, unknown>) ?? {})
    await beforeFocus?.(props)
    await nextTick()
    if (props.length) {
      form.scrollToField(props[0])
    }
    focusFirstError(form.$el as HTMLElement | undefined)
    return false
  }
}

// focusFirstError 聚焦表单中第一个出错字段的输入框
export function focusFirstError(root: HTMLElement | undefined) {
  const el = root?.querySelector<HTMLElement>('.el-form-item.is-error input:not([type=hidden]), .el-form-item.is-error textarea, .is-error-cell input')
  el?.focus({ preventScroll: true })
}
