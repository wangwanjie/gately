// 构建时收集各语言教程页的标题、简介、用时与视频，供教程首页的卡片列表使用。
import { createContentLoader } from 'vitepress'

export interface TutorialMeta {
  // 页面地址，例如 /docs/tutorials/first-site、/en/docs/tutorials/one-port-443
  url: string
  title: string
  description: string
  // 预计用时，例如「10 分钟」（frontmatter.time）
  time: string
  // 是否附带演示视频（frontmatter.video）
  video: boolean
}

declare const data: TutorialMeta[]
export { data }

export default createContentLoader('**/docs/tutorials/*.md', {
  includeSrc: true,
  transform(raw): TutorialMeta[] {
    return raw
      .filter((p) => !p.url.endsWith('/tutorials/'))
      .map((p) => ({
        url: p.url.replace(/\.html$/, ''),
        // 标题取正文第一个一级标题
        title: /^#\s+(.+)$/m.exec(p.src ?? '')?.[1].trim() ?? p.url,
        description: String(p.frontmatter.description ?? ''),
        time: String(p.frontmatter.time ?? ''),
        video: !!p.frontmatter.video
      }))
  }
})
