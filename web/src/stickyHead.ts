// 页面头部吸顶的纯计算逻辑，与 DOM 解耦，便于单元测试。
// 头部吸在顶栏下方：操作栏换行到标题下方时，标题与描述随滚动缩进顶栏下面，只把操作栏留在可视区；
// 标题与操作栏同一行时整行吸住。桌面端与移动端规则相同。

// 当前负责写入 --g-page-head-h 的页面头部；切换页面时新旧组件的挂载与卸载顺序不固定，据此避免互相覆盖
export const headOwner: { current: symbol | null } = { current: null }

// 吸住时操作栏上方保留的留白（px），与 PageHeader 的上内边距一致
export const HEAD_PAD_TOP = 10

export interface HeadMetrics {
  // 头部整体高度（含内边距）
  headHeight: number
  // 标题区相对头部边框顶端的距离与高度
  mainTop: number
  mainHeight: number
  // 操作栏相对头部边框顶端的距离；没有操作栏时为 null
  actionsTop: number | null
}

export interface HeadLayout {
  // 吸住时向上缩进顶栏的距离：sticky 的 top 取「顶栏高度 - offset」
  offset: number
  // 吸住后实际露出的高度，页内工具栏据此排在它下面
  visible: number
}

// headLayout 计算吸顶偏移：操作栏换行到标题下方时，只露出操作栏；
// 与标题同一行（宽屏、按钮少）时整行吸住。没有操作栏的头部不吸顶。
export function headLayout(m: HeadMetrics): HeadLayout | null {
  if (m.actionsTop === null) {
    return null
  }
  // 操作栏顶部落在标题区底部以下，说明已经换到下一行
  const wrapped = m.actionsTop >= m.mainTop + m.mainHeight - 1
  const offset = wrapped ? Math.max(0, Math.round(m.actionsTop - HEAD_PAD_TOP)) : 0
  return { offset, visible: Math.max(0, Math.round(m.headHeight - offset)) }
}

// isStuck 判断头部是否已吸住：吸住后元素顶端停在「顶栏高度 - offset」处，没滚到之前在它下方。
export function isStuck(rectTop: number, headerH: number, offset: number): boolean {
  return rectTop <= headerH - offset + 0.5
}
