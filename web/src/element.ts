// Element Plus 按需注册：只注册界面实际用到的组件，其余组件与样式不进入主包。
// 新增 <el-xxx> 标签、El* 组件引用或 v-loading 之外的指令时，必须同步登记到这里，
// 否则组件会渲染成未知标签且不报错；scripts/check-element.mjs（pnpm test / pnpm build 都会运行）会核对并让缺项失败。
import type { App, Component } from "vue"
import {
  ElAlert,
  ElAutocomplete,
  ElBreadcrumb,
  ElBreadcrumbItem,
  ElButton,
  ElCard,
  ElCheckbox,
  ElCheckboxGroup,
  ElCollapse,
  ElCollapseItem,
  ElColorPicker,
  ElConfigProvider,
  ElDatePicker,
  ElDialog,
  ElDivider,
  ElDrawer,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElInput,
  ElInputNumber,
  ElLink,
  ElOption,
  ElOptionGroup,
  ElPagination,
  ElPopconfirm,
  ElPopover,
  ElProgress,
  ElRadio,
  ElRadioButton,
  ElRadioGroup,
  ElSegmented,
  ElSelect,
  ElSkeleton,
  ElSlider,
  ElStep,
  ElSteps,
  ElSwitch,
  ElTabPane,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTag,
  ElTimeline,
  ElTimelineItem,
  ElTooltip,
  ElUpload,
  ElLoading,
  ElMessage,
  ElMessageBox,
} from "element-plus"

// 样式按组件引入：每个 style/css 已包含其依赖组件（如 select 带上 tag、option、popper）的样式
import "element-plus/es/components/alert/style/css"
import "element-plus/es/components/autocomplete/style/css"
import "element-plus/es/components/breadcrumb/style/css"
import "element-plus/es/components/breadcrumb-item/style/css"
import "element-plus/es/components/button/style/css"
import "element-plus/es/components/card/style/css"
import "element-plus/es/components/checkbox/style/css"
import "element-plus/es/components/checkbox-group/style/css"
import "element-plus/es/components/collapse/style/css"
import "element-plus/es/components/collapse-item/style/css"
import "element-plus/es/components/color-picker/style/css"
import "element-plus/es/components/config-provider/style/css"
import "element-plus/es/components/date-picker/style/css"
import "element-plus/es/components/dialog/style/css"
import "element-plus/es/components/divider/style/css"
import "element-plus/es/components/drawer/style/css"
import "element-plus/es/components/dropdown/style/css"
import "element-plus/es/components/dropdown-item/style/css"
import "element-plus/es/components/dropdown-menu/style/css"
import "element-plus/es/components/empty/style/css"
import "element-plus/es/components/form/style/css"
import "element-plus/es/components/form-item/style/css"
import "element-plus/es/components/input/style/css"
import "element-plus/es/components/input-number/style/css"
import "element-plus/es/components/link/style/css"
import "element-plus/es/components/option/style/css"
import "element-plus/es/components/option-group/style/css"
import "element-plus/es/components/pagination/style/css"
import "element-plus/es/components/popconfirm/style/css"
import "element-plus/es/components/popover/style/css"
import "element-plus/es/components/progress/style/css"
import "element-plus/es/components/radio/style/css"
import "element-plus/es/components/radio-button/style/css"
import "element-plus/es/components/radio-group/style/css"
import "element-plus/es/components/segmented/style/css"
import "element-plus/es/components/select/style/css"
import "element-plus/es/components/skeleton/style/css"
import "element-plus/es/components/slider/style/css"
import "element-plus/es/components/step/style/css"
import "element-plus/es/components/steps/style/css"
import "element-plus/es/components/switch/style/css"
import "element-plus/es/components/tab-pane/style/css"
import "element-plus/es/components/table/style/css"
import "element-plus/es/components/table-column/style/css"
import "element-plus/es/components/tabs/style/css"
import "element-plus/es/components/tag/style/css"
import "element-plus/es/components/timeline/style/css"
import "element-plus/es/components/timeline-item/style/css"
import "element-plus/es/components/tooltip/style/css"
import "element-plus/es/components/upload/style/css"
// 命令式服务与 v-loading 指令的样式
import "element-plus/es/components/message/style/css"
import "element-plus/es/components/message-box/style/css"
import "element-plus/es/components/loading/style/css"

// 以组件形式注册的 Element Plus 组件（模板里写成 <el-xxx>）
export const elementComponents: Component[] = [
  ElAlert,
  ElAutocomplete,
  ElBreadcrumb,
  ElBreadcrumbItem,
  ElButton,
  ElCard,
  ElCheckbox,
  ElCheckboxGroup,
  ElCollapse,
  ElCollapseItem,
  ElColorPicker,
  ElConfigProvider,
  ElDatePicker,
  ElDialog,
  ElDivider,
  ElDrawer,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElInput,
  ElInputNumber,
  ElLink,
  ElOption,
  ElOptionGroup,
  ElPagination,
  ElPopconfirm,
  ElPopover,
  ElProgress,
  ElRadio,
  ElRadioButton,
  ElRadioGroup,
  ElSegmented,
  ElSelect,
  ElSkeleton,
  ElSlider,
  ElStep,
  ElSteps,
  ElSwitch,
  ElTabPane,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTag,
  ElTimeline,
  ElTimelineItem,
  ElTooltip,
  ElUpload,
]

// 以插件形式注册的服务与指令：ElMessage、ElMessageBox 通过 install 记录应用上下文
// （消息框才能继承 ElConfigProvider 提供的语言），ElLoading 注册 v-loading 指令
export const elementPlugins = [ElLoading, ElMessage, ElMessageBox]

export function installElement(app: App) {
  for (const c of elementComponents) {
    app.use(c as any)
    // withNoopInstall 的子组件（如 ElDropdownItem）安装函数为空，需要显式按名称登记
    const name = (c as { name?: string }).name
    if (name && !app.component(name)) {
      app.component(name, c)
    }
  }
  for (const p of elementPlugins) {
    app.use(p as any)
  }
}
