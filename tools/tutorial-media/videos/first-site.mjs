// 视频：第一个反向代理站点（快速建站 → 站点列表 → 编辑站点 → 统计）
import { record } from '../video.mjs'

await record('first-site', {
  title: '第一个反向代理站点',
  summary: '用域名访问内网里的 NAS 或应用',
  steps: ['打开「Web 服务」点「快速建站」', '填域名和后端地址，测试连通', '选 HTTPS 并发布', '在站点列表里查看、编辑与统计']
}, async (v) => {
  const { page } = v
  await v.go('web', 1200)
  await v.step('第 1 步')
  await v.cap('侧边栏打开「Web → Web 服务」，点右上角「快速建站」')
  await v.click(page.getByRole('button', { name: '快速建站' }))
  const d = page.locator('.el-dialog').last()
  await v.cap('填写访问用的域名，域名还没解析过来也可以先建好')
  await v.type(d.locator('#qs-domain'), 'nas.example.com')
  await v.click(d.getByRole('button', { name: '下一步' }))
  await v.step('第 2 步')
  await v.cap('常见应用可一键套用推荐设置；这里直接填 NAS 的内网地址')
  await v.sleep(1500)
  await v.type(d.locator('input:visible').first(), '192.168.1.10:5000')
  await v.cap('不写协议按 http 处理；可以点「测试连通」确认后端能连上')
  await v.point(d.getByRole('button', { name: '测试连通' }))
  await v.sleep(1500)
  await v.click(d.getByRole('button', { name: '下一步' }))
  await v.step('第 3 步')
  await v.cap('推荐选 HTTPS：已有覆盖该域名的证书会直接使用，没有时可以在这里申请')
  await v.sleep(3500)
  await v.click(d.getByRole('button', { name: '发布' }))
  await v.cap('发布完成：页面会说明怎么把域名指到这台设备（DNS 记录、动态域名或本机 hosts）')
  await v.sleep(5000)
  await v.click(d.getByRole('button', { name: '关闭', exact: true }))
  await v.step('第 4 步')
  await v.cap('站点列表：每个站点显示域名、后端地址、证书剩余天数与请求统计')
  await v.sleep(3500)
  const site = page.locator('.site').filter({ hasText: 'git.example.com' }).first()
  await v.cap('点「编辑」可以修改后端、开启网页认证、WAF、访问控制等')
  await v.click(site.getByRole('button', { name: '编辑', exact: true }))
  await v.sleep(2500)
  await v.scroll(600)
  await v.sleep(1500)
  await page.keyboard.press('Escape')
  await v.sleep(800)
  await v.cap('点「统计」查看请求数、错误率、来源 IP 与国家排行')
  await v.click(site.getByRole('button', { name: '统计', exact: true }))
  await v.sleep(5000)
  await page.keyboard.press('Escape')
  await v.sleep(600)
})
